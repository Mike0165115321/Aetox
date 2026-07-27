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

	"github.com/Mike0165115321/Aetox/internal/model"
)

// A coding agent has to see whole files. The old flat 16KB ceiling silently
// hid the tail of any file past ~400 lines, so the model reasoned about code
// it had never seen. Paging by line (like Claude Code and opencode) keeps the
// prompt bounded without hiding anything: the model is told exactly which
// line to resume from.
const (
	readDefaultLines = 2000
	readMaxBytes     = 256 * 1024
	binarySniffBytes = 8192
)

type readSkill struct {
	root         string
	outputSubdir func() string
	// vision turns read into the tool that opens an image, rather than the one
	// that refuses to. False keeps the old refusal, which points at image_ocr.
	vision bool
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
	return "Read a text file under sandbox root"
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
				"description": "1-based line number to start from. Defaults to 1.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "How many lines to read. Defaults to 2000.",
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
			Description: "Read a text file in sandbox root. Every line comes back prefixed with its line number and a tab — that prefix is added by this tool and is not in the file, so strip it before passing text to edit or apply_patch. " +
				"Returns up to 2000 lines; if the output says it was truncated, call read again with the offset it gives you to get the rest.",
			Parameters: payload,
		},
	}
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
		err = errors.New("read target is a binary file — there is no text to read")
		return newToolOutput("read", command, "", start, false, err), err
	}

	content, next, err := readTextLines(file, offset, limit, true)
	if err != nil {
		return newToolOutput("read", command, "", start, false, err), err
	}
	// "\r\n" not "\n": on Windows the last line would otherwise keep a stray
	// carriage return that the old whole-blob TrimSpace used to remove.
	content = strings.TrimRight(content, "\r\n")
	if strings.TrimSpace(content) == "" {
		if offset > 1 {
			content = fmt.Sprintf("(no lines at offset %d)", offset)
		} else {
			content = "(empty file)"
		}
	}
	if next > 0 {
		content += fmt.Sprintf("\n... (truncated — continue with offset=%d)", next)
	}
	return newToolOutput("read", command, content, start, next > 0, nil), nil
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
		err := errors.New("read cannot open an image for this model — use image_ocr to get the text out of it")
		return newToolOutput("read", command, "", start, false, err), err
	}
	if size > readMaxImageBytes {
		err := fmt.Errorf("image is %d MB, too large to send (limit %d MB) — use image_ocr if you only need the text",
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
				// edit's both say so, because an old_string that includes one
				// silently never matches.
				if numbered {
					b.WriteString(fmt.Sprintf("%6d\t%s", line, text))
				} else {
					b.WriteString(text)
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

// intArg accepts the shapes a tool argument arrives in: an int from Go
// callers, a float64 from JSON, a string from a model that quoted the number.
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
