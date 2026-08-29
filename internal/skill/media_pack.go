package skill

// One tool for one thing: reading a file a model cannot read by itself.
//
// It was three - `image_ocr`, `video_ocr`, `audio_transcribe` - and they are
// three names for one act on three kinds of file: turn what is inside into text
// this session can work with. These are the senses Aetox exists to add, and one
// entry is how the next one costs an action instead of another line in the tool
// block of every desk that carries them.
//
// Gates, the same check every pack in this package is held to (search_pack.go):
//
//   - `planKeeps` (internal/mode/stance.go) holds all three: reading a
//     screenshot or a recording changes nothing, so วางแผน keeps the pack whole.
//   - `parallelToolCalls` (internal/cognitive/agent.go) allows none of them,
//     deliberately - they run ffmpeg and whisper, and four at once is a machine
//     that stops answering rather than a faster turn. All three sit on the same
//     side, so the pack does not straddle it either.
//
// **`pdf_read` is deliberately not one of them**, and it is the parallel list
// that decides it: a PDF is read straight off the disk and IS parallel-safe,
// where these three are not. Folding it in would have cost it that, to save
// about ninety tokens on a tool that already costs ninety-five.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/stt"
)

type mediaReadSkill struct {
	root   string
	speech stt.Options
	// actions this caller may use, nil for all of them. See shellSkill.
	actions []string
}

func (*mediaReadSkill) Name() string { return "media_read" }

func (*mediaReadSkill) Description() string {
	return "อ่านสิ่งที่อยู่ในไฟล์สื่อ, ข้อความในภาพ ข้อความบนจอในวิดีโอ และคำพูดในเสียง"
}

func (s *mediaReadSkill) allowedActions() []string {
	p := packs["media_read"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *mediaReadSkill) Actions() []string { return packs["media_read"].permissions() }

func (s *mediaReadSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["media_read"].narrow(named)
	return &narrowed
}

func (s *mediaReadSkill) inner(action string) (Tool, error) {
	p := packs["media_read"]
	if _, known := p.names[action]; !known {
		return nil, fmt.Errorf("unknown media_read action %q, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return nil, fmt.Errorf("media_read %s is not available here, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	switch action {
	case "image":
		return &imageOCRSkill{root: s.root}, nil
	case "video":
		return &videoOCRSkill{root: s.root}, nil
	case "audio":
		return &audioTranscribeSkill{root: s.root, speech: s.speech}, nil
	}
	return nil, fmt.Errorf("media_read action %q has no implementation", action)
}

func (s *mediaReadSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	lines := map[string]string{
		"image": "`image` (path), text in a picture, by OCR (Thai+English). For an attached image you have no vision of.",
		"video": "`video` (path, interval_seconds?), on-screen text in a video, by sampling frames and reading them. Returns '[m:ss] text' lines.",
		"audio": "`audio` (path), the spoken words in an audio or video file, transcribed offline. Returns '[m:ss] text' lines, and is the one to use for a video whose screen carries no text.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	properties := map[string]any{
		"action": map[string]any{
			"type": "string", "enum": allowed,
			"description": "What to do",
		},
		"path": map[string]any{
			"type":        "string",
			"description": "Relative path to the file",
		},
	}
	if slices.Contains(allowed, "video") {
		properties["interval_seconds"] = map[string]any{
			"type":        "integer",
			"description": "action=video: sample one frame every this many seconds (default 5, min 1, max 60).",
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"action", "path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "media_read",
			Description: "Read what is inside a media file, as text. Actions:\n" +
				actions.String(),
			Parameters: payload,
		},
	}
}

func (s *mediaReadSkill) Guidance(args map[string]any) string {
	inner, err := s.inner(actionOf(args))
	if err != nil {
		return ""
	}
	return guidanceFor(inner, args)
}

func (s *mediaReadSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: media_read <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("media_read", "media_read", "", start, false, err), err
	}
	inner, err := s.inner(strings.ToLower(strings.TrimSpace(args[0])))
	if err != nil {
		return newToolOutput("media_read", "media_read", "", start, false, err), err
	}
	return inner.Execute(ctx, Input{"args": args[1:]})
}

func (s *mediaReadSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	action := actionOf(args)
	if action == "" {
		err := errors.New("action is required, one of: " + strings.Join(s.allowedActions(), ", "))
		return newToolOutput("media_read", "media_read", "", start, false, err), err
	}
	inner, err := s.inner(action)
	if err != nil {
		return newToolOutput("media_read", "media_read", "", start, false, err), err
	}
	return inner.ExecuteTool(ctx, args)
}
