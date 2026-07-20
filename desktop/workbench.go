package main

// Workbench tools: the right dock is the AI's workbench — these skills let the
// agent operate it during a chat turn. browser_open opens a real browser tab in
// the workbench (visible to the user) and waits for the page to load;
// browser_read returns the text of the page currently shown there. Registered
// per-engine-bootstrap in app.go alongside the default skill set.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var agentBrowserSeq int64

var schemeRe = regexp.MustCompile(`^https?://`)

// workbenchOpenBrowser asks the frontend to open a workbench browser tab, then
// waits until the native tab exists and its first navigation completes.
func (a *App) workbenchOpenBrowser(ctx context.Context, url string) (title, finalURL string, err error) {
	if a.ctx == nil {
		return "", "", fmt.Errorf("UI not ready")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return "", "", fmt.Errorf("url is required")
	}
	if !schemeRe.MatchString(url) {
		url = "https://" + url
	}

	id := fmt.Sprintf("web-agent-%d", atomic.AddInt64(&agentBrowserSeq, 1))
	wailsruntime.EventsEmit(a.ctx, "workbench:open-browser", map[string]string{"id": id, "url": url})

	// The frontend creates the tab, which creates the native webview — poll
	// until it exists, then wait out its first navigation.
	deadline := time.Now().Add(20 * time.Second)
	var tab *browserTab
	for tab == nil {
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("browser tab did not open in time")
		}
		if h := a.browsers; h != nil {
			tab = h.tab(id)
		}
		if tab == nil {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	select {
	case <-tab.navDone:
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-time.After(20 * time.Second):
		return "", "", fmt.Errorf("page did not finish loading")
	}
	// meta (title/url) arrives just after navigation — give it a beat.
	for i := 0; i < 20; i++ {
		if title, finalURL = tab.meta(); title != "" || finalURL != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return title, finalURL, nil
}

// workbenchReadBrowser reads the page currently shown in the workbench browser.
func (a *App) workbenchReadBrowser() (title, url, text string, err error) {
	h := a.browsers
	if h == nil {
		return "", "", "", fmt.Errorf("no browser tab open in the workbench")
	}
	h.mu.Lock()
	id := h.lastID
	h.mu.Unlock()
	if id == "" {
		return "", "", "", fmt.Errorf("no browser tab open in the workbench")
	}
	text, err = a.BrowserGetText(id)
	if err != nil {
		return "", "", "", err
	}
	if t := h.tab(id); t != nil {
		title, url = t.meta()
	}
	return title, url, text, nil
}

// ---------------------------------------------------------------------------
// skill.Tool implementations
// ---------------------------------------------------------------------------

func toolDef(name, description string, schema map[string]any) model.ToolDefinition {
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  payload,
		},
	}
}

type browserOpenSkill struct{ app *App }

func (*browserOpenSkill) Name() string { return "browser_open" }

func (*browserOpenSkill) Description() string {
	return "เปิดเว็บในเบราว์เซอร์ของ workbench (ผู้ใช้เห็นหน้าเว็บจริง)"
}

func (*browserOpenSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("browser_open",
		"Open a URL in the workbench browser (visible to the user) and wait for it to load. Use browser_read afterwards to read the page.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "The URL to open"},
			},
			"required": []string{"url"},
		})
}

func (s *browserOpenSkill) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	url, _ := args["url"].(string)
	return s.open(ctx, url)
}

func (s *browserOpenSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	url, _ := input["url"].(string)
	return s.open(ctx, url)
}

func (s *browserOpenSkill) open(ctx context.Context, url string) (skill.Output, error) {
	start := time.Now()
	title, finalURL, err := s.app.workbenchOpenBrowser(ctx, url)
	out := skill.Output{
		Name:       "browser_open",
		Command:    "browser_open " + url,
		Success:    err == nil,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		out.Content = "เปิดไม่สำเร็จ: " + err.Error()
		out.Stderr = err.Error()
		return out, err
	}
	out.Content = fmt.Sprintf("เปิดแล้ว: %s (%s)", title, finalURL)
	out.RawOutput = out.Content
	return out, nil
}

type browserReadSkill struct{ app *App }

func (*browserReadSkill) Name() string { return "browser_read" }

func (*browserReadSkill) Description() string {
	return "อ่านเนื้อหาหน้าเว็บที่เปิดอยู่ในเบราว์เซอร์ของ workbench"
}

func (*browserReadSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("browser_read",
		"Read the visible text of the page currently open in the workbench browser. Use after browser_open, or when the user asks about the page they have open.",
		map[string]any{"type": "object", "properties": map[string]any{}})
}

func (s *browserReadSkill) ExecuteTool(ctx context.Context, _ map[string]any) (skill.Output, error) {
	return s.Execute(ctx, skill.Input{})
}

func (s *browserReadSkill) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	start := time.Now()
	title, url, text, err := s.app.workbenchReadBrowser()
	out := skill.Output{
		Name:       "browser_read",
		Command:    "browser_read",
		Success:    err == nil,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		out.Content = "อ่านไม่สำเร็จ: " + err.Error()
		out.Stderr = err.Error()
		return out, err
	}
	const maxChars = 60000 // keep tool output within a sane context budget
	truncated := false
	if len(text) > maxChars {
		text = text[:maxChars] + "\n... (truncated)"
		truncated = true
	}
	out.Content = fmt.Sprintf("# %s\nURL: %s\n\n%s", title, url, text)
	out.RawOutput = out.Content
	out.Truncated = truncated
	return out, nil
}
