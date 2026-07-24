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

var (
	driveLetterRe = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
	urlSchemeRe   = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)
	bareSchemeRe  = regexp.MustCompile(`(?i)^(about|data|mailto|javascript):`)
)

// normalizeWorkbenchURL mirrors the frontend's normalizeUrl (Workbench.svelte):
// bare Windows paths become file:/// URLs, anything already carrying a scheme
// passes through, and only bare domains get https://. The old ^https?://-only
// check stamped https:// onto file:/// URLs, navigating to a blank
// https://file///... page.
func normalizeWorkbenchURL(url string) string {
	switch {
	case driveLetterRe.MatchString(url):
		return "file:///" + strings.ReplaceAll(url, `\`, "/")
	case urlSchemeRe.MatchString(url) || bareSchemeRe.MatchString(url):
		return url
	default:
		return "https://" + url
	}
}

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
	url = normalizeWorkbenchURL(url)

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

// workbenchLastTabID returns the id of the most recently opened/shown
// workbench browser tab — the target for browser_read/browser_click/browser_type.
func (a *App) workbenchLastTabID() (string, error) {
	h := a.browsers
	if h == nil {
		return "", fmt.Errorf("no browser tab open in the workbench")
	}
	h.mu.Lock()
	id := h.lastID
	h.mu.Unlock()
	if id == "" {
		return "", fmt.Errorf("no browser tab open in the workbench")
	}
	return id, nil
}

// workbenchReadBrowser reads the page currently shown in the workbench browser.
func (a *App) workbenchReadBrowser() (title, url string, snap browserSnapshot, err error) {
	id, err := a.workbenchLastTabID()
	if err != nil {
		return "", "", browserSnapshot{}, err
	}
	snap, err = a.browserSnapshot(id)
	if err != nil {
		return "", "", browserSnapshot{}, err
	}
	if t := a.browsers.tab(id); t != nil {
		title, url = t.meta()
	}
	return title, url, snap, nil
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
		"Read the visible text of the page currently open in the workbench browser, plus a numbered list of clickable/typeable elements. Use after browser_open, or when the user asks about the page they have open. Use the [ref] numbers with browser_click/browser_type.",
		map[string]any{"type": "object", "properties": map[string]any{}})
}

func (s *browserReadSkill) ExecuteTool(ctx context.Context, _ map[string]any) (skill.Output, error) {
	return s.Execute(ctx, skill.Input{})
}

func (s *browserReadSkill) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	start := time.Now()
	title, url, snap, err := s.app.workbenchReadBrowser()
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
	text := snap.Text
	const maxChars = 60000 // keep tool output within a sane context budget
	truncated := false
	if len(text) > maxChars {
		text = text[:maxChars] + "\n... (truncated)"
		truncated = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\nURL: %s\n", title, url)
	if len(snap.Elements) > 0 {
		b.WriteString("\nClickable/typeable elements (use browser_click/browser_type with ref):\n")
		for _, el := range snap.Elements {
			role := el.Role
			if role == "" {
				role = el.Tag
			}
			fmt.Fprintf(&b, "[%d] %s: %q\n", el.Ref, role, el.Text)
		}
	}
	if len(snap.Images) > 0 {
		b.WriteString("\nImages on the page (show one to the user with markdown: ![alt](url)):\n")
		for _, im := range snap.Images {
			alt := im.Alt
			if alt == "" {
				alt = "(no alt)"
			}
			fmt.Fprintf(&b, "- %s — %s\n", im.Src, alt)
		}
	}
	fmt.Fprintf(&b, "\n%s", text)
	out.Content = b.String()
	out.RawOutput = out.Content
	out.Truncated = truncated
	return out, nil
}

type browserClickSkill struct{ app *App }

func (*browserClickSkill) Name() string { return "browser_click" }

func (*browserClickSkill) Description() string {
	return "คลิก element ในหน้าเว็บของ workbench ตาม ref จาก browser_read"
}

func (*browserClickSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("browser_click",
		"Click an element on the page currently open in the workbench browser. ref is one of the [n] numbers browser_read returns — call browser_read first to get valid refs, then browser_read again afterwards to see the result.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref": map[string]any{"type": "integer", "description": "Element ref number from browser_read's output"},
			},
			"required": []string{"ref"},
		})
}

func (s *browserClickSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	ref, _ := args["ref"].(float64)
	return s.click(int(ref))
}

func (s *browserClickSkill) Execute(_ context.Context, input skill.Input) (skill.Output, error) {
	ref, _ := input["ref"].(float64)
	return s.click(int(ref))
}

func (s *browserClickSkill) click(ref int) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "browser_click", Command: fmt.Sprintf("browser_click %d", ref)}
	id, err := s.app.workbenchLastTabID()
	if err == nil {
		err = s.app.BrowserClickRef(id, ref)
	}
	out.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Content, out.Stderr = "คลิกไม่สำเร็จ: "+err.Error(), err.Error()
		return out, err
	}
	time.Sleep(300 * time.Millisecond) // let click-driven navigation/DOM update settle before the next browser_read
	out.Success = true
	out.Content = fmt.Sprintf("คลิก ref %d แล้ว ใช้ browser_read เพื่อดูผลลัพธ์", ref)
	out.RawOutput = out.Content
	return out, nil
}

type browserTypeSkill struct{ app *App }

func (*browserTypeSkill) Name() string { return "browser_type" }

func (*browserTypeSkill) Description() string {
	return "พิมพ์ข้อความลงใน input/textarea ในหน้าเว็บของ workbench ตาม ref จาก browser_read"
}

func (*browserTypeSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("browser_type",
		"Type text into an input/textarea/select/contenteditable element on the page currently open in the workbench browser. ref is one of the [n] numbers browser_read returns. For a select element, text must match one of its [options: ...] shown by browser_read. Set enter=true to press Enter/submit afterwards (for search boxes without a button); otherwise click a submit button via browser_click.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref":   map[string]any{"type": "integer", "description": "Element ref number from browser_read's output"},
				"text":  map[string]any{"type": "string", "description": "Text to type, or the option to choose for a select element"},
				"enter": map[string]any{"type": "boolean", "description": "Press Enter after typing (submits most search/login forms)"},
			},
			"required": []string{"ref", "text"},
		})
}

func (s *browserTypeSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	ref, _ := args["ref"].(float64)
	text, _ := args["text"].(string)
	enter, _ := args["enter"].(bool)
	return s.typeText(int(ref), text, enter)
}

func (s *browserTypeSkill) Execute(_ context.Context, input skill.Input) (skill.Output, error) {
	ref, _ := input["ref"].(float64)
	text, _ := input["text"].(string)
	enter, _ := input["enter"].(bool)
	return s.typeText(int(ref), text, enter)
}

func (s *browserTypeSkill) typeText(ref int, text string, enter bool) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "browser_type", Command: fmt.Sprintf("browser_type %d", ref)}
	id, err := s.app.workbenchLastTabID()
	if err == nil {
		err = s.app.BrowserTypeRef(id, ref, text, enter)
	}
	out.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Content, out.Stderr = "พิมพ์ไม่สำเร็จ: "+err.Error(), err.Error()
		return out, err
	}
	if enter {
		time.Sleep(300 * time.Millisecond) // let Enter-driven navigation settle before the next browser_read
	}
	out.Success = true
	out.Content = fmt.Sprintf("พิมพ์ลง ref %d แล้ว", ref)
	if enter {
		out.Content = fmt.Sprintf("พิมพ์ลง ref %d และกด Enter แล้ว ใช้ browser_read เพื่อดูผลลัพธ์", ref)
	}
	out.RawOutput = out.Content
	return out, nil
}
