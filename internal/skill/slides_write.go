package skill

// slides_write is phase 2 of OFFICE-EXPORT-PLAN.md: the same trade sheet_write
// made, for PowerPoint.
//
// It supports four things — a title, bullets, one picture, speaker notes — and
// that is a decision, not a stopping point. Those four cover the decks people
// actually ask for; charts, animations and custom masters are where a
// hand-written OOXML writer turns into a second PowerPoint, and the plan rules
// them out (§8). What it must get right instead is Thai, which is the one thing
// every Thai deck exposes and the one thing a Latin-first generator gets wrong.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

type slidesWriteSkill struct {
	root         string
	outputSubdir func() string
}

// A picture is embedded whole, so it is also the whole file size. 20 MB is far
// past any screenshot or chart and well short of the point where a deck stops
// being mailable.
const maxSlideImageBytes = 20 << 20

func (*slidesWriteSkill) Name() string { return "slides_write" }

func (*slidesWriteSkill) Description() string {
	return "สร้างไฟล์ PowerPoint (.pptx) จากโครงสไลด์ — หัวข้อ บุลเล็ต รูป โน้ตผู้บรรยาย ฟอนต์ไทยไม่หลุด"
}

func (*slidesWriteSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Destination path for the deck. The .pptx extension is added if missing.",
			},
			"slides": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "One entry per slide, in order.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{
							"type":        "string",
							"description": "Slide heading. A slide with only a title works as a section divider.",
						},
						"bullets": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "Bullet points. Keep each to one line — a slide is not a document.",
						},
						"image": map[string]any{
							"type":        "string",
							"description": "Path to a .png/.jpg/.gif already on disk to place on this slide. It is embedded, so the deck stays self-contained.",
						},
						"notes": map[string]any{
							"type":        "string",
							"description": "Speaker notes — shown to the presenter, never on the slide.",
						},
					},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"path", "slides"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "slides_write",
			// Capability only, no when-to-pick-me language (owner, 2026-08-04):
			// "the tool tells you what it makes; which shape answers the request
			// is the model's call." The rest teaches how to hold the tool, which
			// no model can guess: notes vs bullets, and that images embed.
			Description: "Create a PowerPoint file (.pptx) — opens in PowerPoint, Google Slides and Keynote. " +
				"Give each slide a short title and a few one-line bullets — write the long version into `notes`, which only the presenter sees. " +
				"An `image` is a path to a picture already on disk and gets embedded, so the file stays self-contained. " +
				"The result reports where the file actually landed.",
			Parameters: payload,
		},
	}
}

// Execute is the command-line shape: slides_write <path> <json>, matching
// sheet_write's. Nobody hand-types it; it keeps the skill reachable from the
// CLI dispatcher like every other one.
func (s *slidesWriteSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("slides_write skill unavailable")
		return newToolOutput("slides_write", "slides_write", "", start, false, err), err
	}
	args, _ := input["args"].([]string)
	if len(args) < 2 {
		err := errors.New("usage: slides_write <path> <slides-json>")
		return newToolOutput("slides_write", "slides_write", "", start, false, err), err
	}
	var slides any
	if err := json.Unmarshal([]byte(strings.Join(args[1:], " ")), &slides); err != nil {
		err = fmt.Errorf("slides must be JSON: %w", err)
		return newToolOutput("slides_write", "slides_write", "", start, false, err), err
	}
	return s.run(start, args[0], slides)
}

func (s *slidesWriteSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("slides_write skill unavailable")
		return newToolOutput("slides_write", "slides_write", "", start, false, err), err
	}
	path, _ := args["path"].(string)
	return s.run(start, path, args["slides"])
}

func (s *slidesWriteSkill) run(start time.Time, requestPath string, rawSlides any) (Output, error) {
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		// Same shape as doc_write's, and for the reason written there.
		err := errors.New(`path is required, and so is slides — a call with neither is an empty call. ` +
			`Smallest real one: {"path":"เสนองาน.pptx","slides":[{"title":"หัวข้อ","bullets":["ประเด็นแรก"]}]}. ` +
			`path is just a filename; it lands in this session's output folder on its own`)
		return newToolOutput("slides_write", "slides_write", "", start, false, err), err
	}
	if !strings.EqualFold(filepath.Ext(requestPath), ".pptx") {
		requestPath = strings.TrimSuffix(requestPath, filepath.Ext(requestPath)) + ".pptx"
	}

	slides, err := s.parseSlides(rawSlides)
	if err != nil {
		return newToolOutput("slides_write", "slides_write "+requestPath, "", start, false, err), err
	}

	original := requestPath
	requestPath = placedWrite(s.outputSubdir, requestPath)
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("slides_write", "slides_write "+requestPath, "", start, false, err), err
	}
	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("slides_write", "slides_write "+requestPath, "", start, false, err), err
	}

	parts, err := ooxml.BuildPPTX(slides)
	if err != nil {
		return newToolOutput("slides_write", "slides_write "+requestPath, "", start, false, err), err
	}
	if err := ooxml.WriteFile(targetPath, parts); err != nil {
		return newToolOutput("slides_write", "slides_write "+requestPath, "", start, false, err), err
	}

	output := fmt.Sprintf("slides_write done: %s (%d slide(s))", requestPath, len(slides))
	if requestPath != original {
		output += " (on disk: " + targetPath + ")"
	}
	out := newToolOutput("slides_write", "slides_write "+requestPath, output, start, false, nil)
	out.Artifacts = []string{requestPath}
	return out, nil
}

func (s *slidesWriteSkill) parseSlides(raw any) ([]ooxml.Slide, error) {
	list, ok := raw.([]any)
	if !ok {
		if raw == nil {
			return nil, errors.New("slides is required: an array of {title, bullets, image, notes}")
		}
		return nil, errors.New("slides must be an array of {title, bullets, image, notes}")
	}
	if len(list) == 0 {
		return nil, errors.New("slides is empty: a deck needs at least one slide")
	}

	slides := make([]ooxml.Slide, 0, len(list))
	for i, entry := range list {
		object, ok := entry.(map[string]any)
		if !ok {
			// A slide sent as a bare string has one sensible reading — a title —
			// and rejecting it costs the model a whole turn to learn that.
			if text, isText := entry.(string); isText {
				slides = append(slides, ooxml.Slide{Title: text})
				continue
			}
			return nil, fmt.Errorf("slide %d must be an object with title, bullets, image or notes", i+1)
		}
		slide := ooxml.Slide{}
		slide.Title, _ = object["title"].(string)
		slide.Notes, _ = object["notes"].(string)
		for _, bullet := range asList(object["bullets"]) {
			if text, ok := bullet.(string); ok {
				slide.Bullets = append(slide.Bullets, text)
				continue
			}
			slide.Bullets = append(slide.Bullets, fmt.Sprint(bullet))
		}

		if raw, ok := object["image"].(string); ok && strings.TrimSpace(raw) != "" {
			picture, err := s.loadImage(strings.TrimSpace(raw))
			if err != nil {
				return nil, fmt.Errorf("slide %d image: %w", i+1, err)
			}
			slide.Image = picture
		}

		if slide.Title == "" && len(slide.Bullets) == 0 && slide.Image == nil && strings.TrimSpace(slide.Notes) == "" {
			return nil, fmt.Errorf("slide %d is empty: give it a title, bullets, an image or notes", i+1)
		}
		slides = append(slides, slide)
	}
	return slides, nil
}

// loadImage reads a picture off disk and measures it.
//
// The dimensions come from the file rather than from the model because the
// model does not know them and would have to guess, and a guessed aspect ratio
// is a stretched screenshot — the kind of wrong that looks deliberate. Failing
// loudly here is on purpose too: a deck that silently dropped the chart it was
// asked for is worse than one that was never written.
func (s *slidesWriteSkill) loadImage(requestPath string) (*ooxml.SlideImage, error) {
	// Same fallback every file-consuming skill uses, so a picture an earlier
	// tool wrote into the session output folder is found by the name the model
	// remembers (see RegistryOptions.OutputSubdir).
	resolved := PlacedPath(s.root, s.outputSubdir, requestPath)
	full, err := resolveSandboxPath(s.root, resolved)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("%s not found", requestPath)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", requestPath)
	}
	if info.Size() > maxSlideImageBytes {
		return nil, fmt.Errorf("%s is %d bytes, over the %d limit for an embedded picture", requestPath, info.Size(), maxSlideImageBytes)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}

	// Decoding the header also validates the format: a .png that is really a
	// PDF would otherwise produce a deck PowerPoint declares damaged, naming no
	// cause the user could act on.
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s is not a readable png, jpeg or gif", requestPath)
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	return &ooxml.SlideImage{
		Ext:      ext,
		Data:     data,
		WidthPx:  config.Width,
		HeightPx: config.Height,
		AltText:  filepath.Base(requestPath),
	}, nil
}
