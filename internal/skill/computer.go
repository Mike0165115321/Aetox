package skill

// computer gives a model hands on the user's real desktop: move and click the
// mouse, type on the keyboard, press key combos, read the screen size, and
// screenshot into the sandbox. Paired with image_ocr this closes the loop for
// models with no vision: screenshot → OCR → decide → click. Every action is
// RiskHigh in internal/safety, so the user approves each call — same brake as
// shell.
//
// Windows-only today (the whole desktop app is; see computer_windows.go).
// Other platforms get a clear "not supported" error, not a silent no-op.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

type computerSkill struct {
	root string
}

func (*computerSkill) Name() string { return "computer" }

func (*computerSkill) Description() string {
	return "ควบคุมเครื่องจริง: ขยับเมาส์ คลิก พิมพ์ กดคีย์ลัด และจับภาพหน้าจอ (คู่กับ image_ocr เพื่อให้โมเดลที่ไม่มีตาเห็นหน้าจอได้)"
}

func (*computerSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"screen_info", "mouse_move", "click", "type", "key", "screenshot"},
				"description": "screen_info: screen size + cursor position. mouse_move: move cursor to x,y. click: click at x,y (or current position if omitted). type: type literal text. key: press a key or combo like 'enter' or 'ctrl+c'. screenshot: save the screen as PNG in the sandbox — then read it with image_ocr.",
			},
			"x":      map[string]any{"type": "integer", "description": "Screen X coordinate for mouse_move/click"},
			"y":      map[string]any{"type": "integer", "description": "Screen Y coordinate for mouse_move/click"},
			"button": map[string]any{"type": "string", "enum": []string{"left", "right", "double"}, "description": "Mouse button for click (default left; double = double left click)"},
			"text":   map[string]any{"type": "string", "description": "Literal text to type (supports Thai and any Unicode)"},
			"combo":  map[string]any{"type": "string", "description": "Key or combo for the key action, e.g. 'enter', 'tab', 'ctrl+c', 'ctrl+shift+s'"},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "computer",
			Description: "Control the user's real desktop: move/click the mouse, type text, press key combos, get screen info, or take a screenshot into the sandbox (read it afterwards with image_ocr). Use when the task needs acting on the actual screen. Every call requires user approval.",
			Parameters:  payload,
		},
	}
}

func (s *computerSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: computer <screen_info|mouse_move x y|click [x y] [button]|type <text>|key <combo>|screenshot>")
		return newToolOutput("computer", "computer", "", start, false, err), err
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	p := computerParams{x: -1, y: -1}
	switch action {
	case "mouse_move", "click":
		if len(rest) >= 2 {
			x, errX := strconv.Atoi(rest[0])
			y, errY := strconv.Atoi(rest[1])
			if errX == nil && errY == nil {
				p.x, p.y = x, y
				rest = rest[2:]
			}
		}
		if action == "click" && len(rest) > 0 {
			p.button = strings.ToLower(rest[0])
		}
	case "type":
		p.text = strings.Join(rest, " ")
	case "key":
		if len(rest) > 0 {
			p.combo = rest[0]
		}
	}
	return s.run(ctx, start, action, p)
}

func (s *computerSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	action, _ := args["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		err := errors.New("action is required")
		return newToolOutput("computer", "computer", "", time.Now(), false, err), err
	}
	p := computerParams{x: -1, y: -1}
	if v, ok := args["x"].(float64); ok {
		p.x = int(v)
	}
	if v, ok := args["y"].(float64); ok {
		p.y = int(v)
	}
	if v, ok := args["button"].(string); ok {
		p.button = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := args["text"].(string); ok {
		p.text = v
	}
	if v, ok := args["combo"].(string); ok {
		p.combo = strings.TrimSpace(v)
	}
	return s.run(ctx, time.Now(), action, p)
}

type computerParams struct {
	x, y   int
	button string
	text   string
	combo  string
}

func (s *computerSkill) run(ctx context.Context, start time.Time, action string, p computerParams) (Output, error) {
	command := strings.TrimSpace("computer " + action)

	var result string
	var err error
	switch action {
	case "screen_info":
		var w, h, cx, cy int
		w, h, cx, cy, err = computerScreenInfo()
		if err == nil {
			result = fmt.Sprintf("screen %dx%d, cursor at (%d, %d)", w, h, cx, cy)
		}
	case "mouse_move":
		if p.x < 0 || p.y < 0 {
			err = errors.New("mouse_move needs x and y (non-negative screen coordinates)")
			break
		}
		command = fmt.Sprintf("computer mouse_move %d %d", p.x, p.y)
		if err = computerMouseMove(p.x, p.y); err == nil {
			result = fmt.Sprintf("moved mouse to (%d, %d)", p.x, p.y)
		}
	case "click":
		button := p.button
		if button == "" {
			button = "left"
		}
		if button != "left" && button != "right" && button != "double" {
			err = fmt.Errorf("unknown button %q (want left, right, or double)", button)
			break
		}
		command = strings.TrimSpace(fmt.Sprintf("computer click %s", button))
		if err = computerClick(p.x, p.y, button); err == nil {
			if p.x >= 0 && p.y >= 0 {
				result = fmt.Sprintf("%s click at (%d, %d)", button, p.x, p.y)
			} else {
				result = button + " click at current cursor position"
			}
		}
	case "type":
		if p.text == "" {
			err = errors.New("type needs non-empty text")
			break
		}
		if err = computerType(p.text); err == nil {
			result = fmt.Sprintf("typed %d characters", len([]rune(p.text)))
		}
	case "key":
		if p.combo == "" {
			err = errors.New("key needs a combo, e.g. 'enter' or 'ctrl+c'")
			break
		}
		command = "computer key " + p.combo
		if err = computerKey(p.combo); err == nil {
			result = "pressed " + p.combo
		}
	case "screenshot":
		rel := filepath.Join("screenshots", "screen_"+time.Now().Format("20060102_150405")+".png")
		var abs string
		if abs, err = resolveSandboxPath(s.root, rel); err == nil {
			if err = os.MkdirAll(filepath.Dir(abs), 0o755); err == nil {
				if err = computerScreenshot(ctx, abs); err == nil {
					result = fmt.Sprintf("saved screenshot to %s — read its text with image_ocr %q", rel, rel)
				}
			}
		}
	default:
		err = fmt.Errorf("unknown action %q (want screen_info, mouse_move, click, type, key, or screenshot)", action)
	}

	if err != nil {
		return newToolOutput("computer", command, "", start, false, err), err
	}
	return newToolOutput("computer", command, result, start, false, nil), nil
}
