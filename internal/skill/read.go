package skill

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
)

// A coding agent has to see whole files. The old flat 16KB ceiling silently
// hid the tail of any file past ~400 lines, so the model reasoned about code
// it had never seen. Paging by line (like Claude Code and opencode) keeps the
// prompt bounded without hiding anything: the model is told exactly which
// line to resume from.
const (
	readDefaultLines = 2000
	// readMaxBytes was 256KB, which is not a ceiling so much as a promise that
	// nothing catastrophic happens. Measured on this machine 2026-08-27: 27 of
	// 461 read calls (5.9%) carried 1.95MB, half of everything read has ever
	// returned, and the largest single call was 130,823 bytes — around 33,000
	// tokens of tool result, riding every later round of that conversation.
	//
	// 64KB is still a generous single answer and it is one the line cap can
	// actually reach: the truncation marker already names the offset to resume
	// from, so nothing is hidden, only paged.
	readMaxBytes = 64 * 1024
	// readMaxLineLen bounds one line, because the line cap above cannot.
	//
	// `{"path": ".../scripts.js", "limit": 110}` returned 57,437 bytes in this
	// machine's history: the model asked for 110 lines, as carefully as anyone
	// could ask, and paid for 57KB because the lines were generated. A cap of
	// 2000 characters is longer than any line a person writes and shorter than
	// any line a bundler emits, which is the whole distinction being drawn.
	readMaxLineLen   = 2000
	binarySniffBytes = 8192
)

type readSkill struct {
	root         string
	outputSubdir func() string
	// vision turns read into the tool that opens an image, rather than the one
	// that refuses to. False keeps the old refusal, which points at image_ocr.
	vision bool
	// files is the shared record (filestate.go). read fills it: a file the
	// agent has looked at is one a later whole-file write can be held to.
	files *FileState
}

// readMaxImageBytes bounds what read will hand a provider inline. Every one of
// them re-encodes an image to base64 in the request body, so the wire cost is
// a third larger again than this; a screenshot is well under it and a scanned
// poster is not something to send by accident.
const readMaxImageBytes = 5 << 20

// imageMediaTypes is the set read will open as a picture. Deliberately short:
// these are what the three providers agree they accept, and a format only one
// of them takes is worse than a clear refusal.
var imageMediaTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

func (*readSkill) Name() string { return "read" }

func (*readSkill) Description() string {
	return "Read a file under sandbox root, text, notebook, Word, PowerPoint or Excel"
}

func (*readSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Relative file path to read",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "1-based line to start from. Defaults to 1.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "How many lines. Defaults to 2000.",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "read",
			Description: "Read a text file in sandbox root. Every line is prefixed with its line number and a tab, which this tool adds and the file does not contain, so strip it before passing text to edit or edits. " +
				"Reads up to 2000 lines. When you already know which part you need, read only that part with offset and limit. " +
				"A truncated result names the offset to resume from.",
			Parameters: payload,
		},
	}
}

// Guidance is where the habit lives, because the block entry cannot afford it
// and the habit is worth more than any of the mechanics it would displace.
//
// read has taken offset and limit since paging replaced the flat 16KB ceiling,
// and the model reaches for them: 205 of 461 calls in this machine's history
// passed one. What the block never said is when to WANT less. It explained how
// to ask for the rest of a file and never how to ask for part of one, which is
// a description that teaches only the expensive direction.
//
// The cost is all in the tail. 27 of those 461 calls, 5.9%, carried 1.95MB
// between them, half of everything read has ever returned, and the largest was
// one 130,823-byte answer that then rode every later round of the conversation
// it landed in. Nothing about the average is worth changing.
func (*readSkill) Guidance(map[string]any) string {
	return "Find the place before reading it. grep and glob answer where something lives for a fraction of what a file costs, " +
		"and a read of the range around a known line is the cheap end of this tool.\n" +
		"A whole-file read is right when the file is small or you genuinely need all of it. On anything large it is the most " +
		"expensive call available, and the bytes stay in the conversation for every round that follows, not just this one.\n" +
		"A page stops at 2000 lines or 64KB, whichever comes first, and says which line to resume from. Nothing is hidden by that, " +
		"only deferred: ask for the next page when you need it rather than in case you do.\n" +
		"Lines past 2000 characters are clipped and marked. That is generated code, and a clipped line is no longer the file's " +
		"exact text, so an edit built from one will not match."
}

func (s *readSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("read skill unavailable")
		return newToolOutput("read", "read", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: read <path>")
		return newToolOutput("read", "read", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(strings.Join(args, " "))
	if requestPath == "" {
		err := errors.New("usage: read <path>")
		return newToolOutput("read", "read", "", start, false, err), err
	}
	offset := intArg(input["offset"])
	limit := intArg(input["limit"])

	requestPath = PlacedPath(s.root, s.outputSubdir, requestPath)
	command := "read " + requestPath
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}
	if info.IsDir() {
		err = errors.New("read target is a directory")
		return newToolOutput("read", command, "", start, false, err), err
	}
	// One place for every kind of read below — text, image, notebook, Office.
	// Looking at a file is what gives a later whole-file write something to be
	// stale against (filestate.go); a file this session has never opened is one
	// `write` is free to replace, which is what that tool is for.
	s.files.Note(targetPath)
	if mediaType, isImage := imageMediaTypes[strings.ToLower(filepath.Ext(targetPath))]; isImage {
		return s.readImage(targetPath, requestPath, command, mediaType, info.Size(), start)
	}
	// A notebook is JSON with the code escaped inside it and every past output
	// embedded. Handing that over raw costs a fortune in context to show five
	// lines of Python, so it is rendered as cells instead — and those cell
	// numbers are what notebook_edit takes.
	if strings.EqualFold(filepath.Ext(targetPath), notebookExt) {
		nb, nbErr := loadNotebook(targetPath)
		if nbErr != nil {
			return newToolOutput("read", command, "", start, false, nbErr), nbErr
		}
		rendered, truncated := limitLines(renderNotebook(nb, requestPath), defaultToolOutputLineLimit)
		return newToolOutput("read", command, rendered, start, truncated, nil), nil
	}

	// An Office file is a zip, so the binary sniff below is right about the
	// bytes and wrong about the file: a .docx is nothing but text, and refusing
	// it left the agent whose job is documents unable to open one.
	if ext := officeExtOf(targetPath); ext != "" {
		rendered, officeErr := s.readOffice(targetPath, requestPath, ext, info.Size())
		if officeErr != nil {
			return newToolOutput("read", command, "", start, false, officeErr), officeErr
		}
		limited, truncated := limitLines(rendered, defaultToolOutputLineLimit)
		return newToolOutput("read", command, limited, start, truncated, nil), nil
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}
	defer func() {
		_ = file.Close()
	}()

	binary, err := looksBinary(file)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}
	// An error, not a "(binary file)" note with err == nil: that reported
	// Success and drew a green tick for a read that returned nothing usable,
	// so the model treated a dead end as a result it merely hadn't understood
	// and kept guessing at other ways in. edit already fails the same way on
	// the same condition.
	if binary {
		err = errors.New("read target is a binary file, there is no text to read")
		return newToolOutput("read", command, "", start, false, err), err
	}

	content, next, err := readTextLines(file, offset, limit, true)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}
	// "\r\n" not "\n": on Windows the last line would otherwise keep a stray
	// carriage return that the old whole-blob TrimSpace used to remove.
	content = strings.TrimRight(content, "\r\n")
	// Counted before the placeholders and the truncation marker join in: those
	// are messages about the read, not lines of the file.
	returned := 0
	if strings.TrimSpace(content) != "" {
		returned = strings.Count(content, "\n") + 1
	}
	if strings.TrimSpace(content) == "" {
		if offset > 1 {
			content = fmt.Sprintf("(no lines at offset %d)", offset)
		} else {
			content = "(empty file)"
		}
	}
	if next > 0 {
		content += fmt.Sprintf("\n... (truncated, continue with offset=%d)", next)
	}
	out := newToolOutput("read", command, content, start, next > 0, nil)
	if returned > 0 {
		first := offset
		if first < 1 {
			first = 1
		}
		out.ResultCount = returned
		out.ResultRange = fmt.Sprintf("%d-%d", first, first+returned-1)
	}
	return out, nil
}

func (s *readSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	path, ok := args["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		err := errors.New("path is required")
		return newToolOutput("read", "read", "", time.Now(), false, err), err
	}
	return s.Execute(ctx, Input{
		"args":   []string{path},
		"offset": args["offset"],
		"limit":  args["limit"],
	})
}

// readTextLines returns lines [offset, offset+limit) of f. The second result
// is the 1-based line the caller should resume from, or 0 when the file ended.
// readImage answers "open this picture" — the request §51 made possible for a
// user's attachment and this makes possible for a file the model found itself.
//
// When the model cannot see, the answer is the same refusal read has always
// given, and it names the tool that can help. That refusal is deliberate: a
// tool that returned "(binary file)" with a green tick taught the model it had
// merely failed to understand a result, and it kept guessing at other ways in.
func (s *readSkill) readImage(targetPath, shown, command, mediaType string, size int64, start time.Time) (Output, error) {
	if !s.vision {
		err := errors.New("read cannot open an image for this model, use image_ocr to get the text out of it")
		return newToolOutput("read", command, "", start, false, err), err
	}
	if size > readMaxImageBytes {
		err := fmt.Errorf("image is %d MB, too large to send (limit %d MB), use image_ocr if you only need the text",
			size>>20, int64(readMaxImageBytes)>>20)
		return newToolOutput("read", command, "", start, false, err), err
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}
	out := newToolOutput("read", command, "image attached: "+shown, start, false, nil)
	out.Images = []model.Image{{MediaType: mediaType, Data: data}}
	return out, nil
}

// numbered is false for the CLI-facing `fs cat`, which prints to a human who
// asked for the file, not to a model that has to cite it.
func readTextLines(f *os.File, offset, limit int, numbered bool) (string, int, error) {
	if offset < 1 {
		offset = 1
	}
	if limit < 1 {
		limit = readDefaultLines
	}

	// bufio.Reader.ReadString, not Scanner: a minified bundle or a one-line
	// JSON blob blows past Scanner's 64KB token limit and errors out.
	reader := bufio.NewReader(f)
	var b strings.Builder
	line, taken := 0, 0
	for {
		text, err := reader.ReadString('\n')
		if text != "" {
			line++
			if line >= offset {
				if taken == limit || b.Len() >= readMaxBytes {
					return b.String(), line, nil
				}
				// Numbered like `cat -n`, which is what Claude Code and
				// opencode both hand their models: without it the model can
				// only cite a location by quoting the code back, and every
				// "which line is this" question costs a second call to grep.
				// The prefix is not in the file — read's description and
				// edit's both say so, because a find text that includes one
				// silently never matches.
				if numbered {
					b.WriteString(fmt.Sprintf("%6d\t%s", line, clipLine(text)))
				} else {
					b.WriteString(clipLine(text))
				}
				taken++
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return b.String(), 0, nil
			}
			return "", 0, err
		}
	}
}

// looksBinary reports whether the head of f contains a NUL byte, rewinding f
// so the caller can still read from the top.
func looksBinary(f *os.File) (bool, error) {
	head := make([]byte, binarySniffBytes)
	n, err := f.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	return bytes.IndexByte(head[:n], 0) >= 0, nil
}

// IntArg accepts the shapes a tool argument arrives in: an int from Go
// callers, a float64 from JSON, a string from a model that quoted the number.
//
// Exported on 2026-08-22, when the desktop's own copy of this — which handled
// the first two shapes and not the third — was found returning 0 for every
// `{"ref": "1"}` the model sent. Twelve calls in this machine's tool_runs had
// gone that way. A second place answering the same question is the debt this
// project has a name for, and the fix is one answer rather than two that agree
// for a while.
func IntArg(value any) int { return intArg(value) }

// BoolArg is the same rule for a boolean: a model that quotes "true" means
// true, and a tool that reads that as false refuses an option it was given.
func BoolArg(value any) bool {
	switch b := value.(type) {
	case bool:
		return b
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(b))
		return err == nil && parsed
	}
	return false
}

func intArg(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		parsed, err := n.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return parsed
		}
	}
	return 0
}

// clipLine bounds one line at readMaxLineLen, and says so where it cut.
//
// The marker is not decoration. A clipped line is no longer the file's text,
// so an edit built from it will not match — and `edit` failing loudly on a
// find text that is not there is the correct outcome, provided the model can
// see why. Silence here would turn a bounded read into a mystery.
func clipLine(text string) string {
	end := len(text)
	nl := ""
	if strings.HasSuffix(text, "\n") {
		end--
		nl = "\n"
		if end > 0 && text[end-1] == '\r' {
			end--
			nl = "\r\n"
		}
	}
	if end <= readMaxLineLen {
		return text
	}
	return text[:readMaxLineLen] +
		fmt.Sprintf(" ... (line clipped, %d more characters, not the file's exact text)", end-readMaxLineLen) + nl
}
