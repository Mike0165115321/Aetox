package main

// Workbench tools: the right dock is the AI's workbench — these skills let the
// agent operate it during a chat turn. browser_open opens a real browser tab in
// the workbench (visible to the user) and waits for the page to load;
// browser_read returns the text of the page currently shown there. Registered
// per-engine-bootstrap in app.go alongside the default skill set.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"

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
//
// A path relative to the sandbox root resolves to file:/// too. Every other
// tool speaks relative paths, so without this the model has to splice the
// sandbox root in by hand to view what it just made, and "index.html" fell
// through to the bare-domain case and navigated to https://index.html.
//
// It resolves through skill.PlacedPath rather than joining onto the root
// directly, because write steers a new relative file into the session output
// folder: the model asks to open "index.html", the file is really at
// "aetox/output/<session>/index.html", and a plain root join finds nothing and
// silently degrades to a DNS lookup for a hostname called index.html.
func normalizeWorkbenchURL(url, sandboxRoot string, outputSubdir func() string) string {
	switch {
	case driveLetterRe.MatchString(url):
		return "file:///" + strings.ReplaceAll(url, `\`, "/")
	case urlSchemeRe.MatchString(url) || bareSchemeRe.MatchString(url):
		return url
	}
	// Only a path that actually exists is treated as a file, so a bare domain
	// still becomes https:// — the check is "is there such a file", not "does
	// this look path-shaped".
	if root := strings.TrimSpace(sandboxRoot); root != "" {
		placed := skill.PlacedPath(root, outputSubdir, url)
		if abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(placed))); err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				return "file:///" + strings.ReplaceAll(abs, `\`, "/")
			}
		}
	}
	return "https://" + url
}

// browserRenderable is what the workbench browser can actually display. A file
// with no extension at all is let through rather than guessed at.
var browserRenderable = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true, ".svg": true, ".pdf": true,
	".txt": true, ".json": true, ".xml": true, ".csv": true, ".log": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true, ".ico": true,
	".mp4": true, ".webm": true, ".mp3": true, ".wav": true, ".ogg": true,
}

// unrenderableFile reports why a local file cannot be shown in the browser, or
// "" when it is worth trying. Only file:// targets are judged: a URL's
// extension says nothing about what the server will actually send back.
//
// Without this, asking for a .ts file navigated to it, WebView2 aborted the
// navigation (a source file is a download, not a page), and that surfaced as
// "page failed to load — not found, or unreachable" — so the model went
// hunting for a path bug that did not exist. The file was right there.
func unrenderableFile(url string) string {
	if !strings.HasPrefix(url, "file:///") {
		return ""
	}
	path := filepath.FromSlash(strings.TrimPrefix(url, "file:///"))
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" || browserRenderable[ext] {
		return ""
	}
	return fmt.Sprintf(
		"%s exists, but a browser cannot display a %s file — it would be downloaded, not rendered. "+
			"Use read to see its contents, or open the .html page that loads it.",
		filepath.Base(path), ext)
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
	url = normalizeWorkbenchURL(url, a.cfg.SandboxRoot, a.outputSubdir)
	// Refused before a tab is opened: the tab would only show the failure too,
	// and the user would be left looking at a download prompt or a blank page.
	if why := unrenderableFile(url); why != "" {
		return "", "", errors.New(why)
	}

	// The agent browses in ONE tab, and every later call steers that tab.
	//
	// It minted a fresh `web-agent-N` per call until 2026-08-10, which meant a
	// session that opened five pages left five tabs — and the browsing tools
	// that follow (read/click/type) all target the most recent one, so a
	// sequence like open → read → click → open was a sequence across two
	// different pages. Reuse is what makes the four tools one flow instead of
	// four unrelated actions, and it is also simply what a person sees: they
	// watched the agent open tab after tab and called it "เปิดใหม่ ๆ รัว ๆ".
	//
	// A new id is minted only when there is no live tab to steer — first call,
	// or the user closed the one that was there.
	id := a.agentBrowserTabID()
	reusing := id != ""
	if !reusing {
		id = fmt.Sprintf("web-agent-%d", atomic.AddInt64(&agentBrowserSeq, 1))
	}
	if reusing {
		// Armed before navigate, so the wait below is this navigation's.
		tab := a.browsers.tab(id)
		tab.armNavigation()
		tab.view.navigate(url)
	}
	// Emitted either way: for a new tab the frontend creates it, and for one
	// that exists the same handler just raises it — which is what the user
	// needs to actually see the page the agent moved to.
	a.emitEvent("workbench:open-browser", map[string]string{"id": id, "url": url})

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

	if err := tab.awaitNavigation(ctx, 20*time.Second); err != nil {
		// Naming the URL matters here: it is usually a path the model built
		// itself, and seeing it back is what tells it the path was the problem.
		return "", "", fmt.Errorf("%w: %s", err, url)
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

// agentBrowserTabID names the agent's live browsing tab, or "" when it has
// none to steer.
//
// "The agent's tab" is the most recent one it opened AND that is still alive:
// a tab the user closed is gone from the host's map, so this answers "" and
// the caller mints a new one rather than navigating a corpse.
func (a *App) agentBrowserTabID() string {
	h := a.browsers
	if h == nil {
		return ""
	}
	h.mu.Lock()
	id := h.lastID
	h.mu.Unlock()
	if id == "" || !strings.HasPrefix(id, "web-agent-") {
		return ""
	}
	if tab := h.tab(id); tab == nil || tab.view == nil {
		return ""
	}
	return id
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
		"Open a URL in the workbench browser (visible to the user) and wait for it to load. Also opens a local file — pass the same path write reported, relative to the sandbox root, no need to build a file:// URL yourself — as long as it is something a browser renders: .html, .svg, .pdf, an image. Source files (.ts, .go, .css) are downloads, not pages; use read for those. Use it to show the user any page you just created, and browser_read afterwards to read it back.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "A URL, or a file path relative to the sandbox root"},
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
	out.Content = browserOpenedLine(title, finalURL)
	out.RawOutput = out.Content
	return out, nil
}

// browserOpenedLine is the one place this sentence is written. It is a function
// rather than an inline Sprintf so the round-trip test can call the real writer
// — sharing only the prefix constant left the test with its own copy of the
// format, which meant editing this line could not fail anything.
func browserOpenedLine(title, url string) string {
	return fmt.Sprintf("%s%s (%s)", browserOpenedPrefix, title, url)
}

// ---------- reading those back ----------
//
// Every browser_open the agent has ever run is already on disk: recordToolRun
// writes one tool_runs row per call and nothing in the app has ever read one
// back. RecentAgentPages is that read, and it is what a new browser tab is made
// of — the tab opens showing where the agent has been rather than a blank slate
// (ARCHITECTURE.md §81).

// AgentPage is one page the agent opened, for the browser tab's start page.
type AgentPage struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Time  string `json:"time"` // RFC3339 — what the frontend's agoLabel() parses
}

const (
	// Shared by the Sprintf above and the parser below so the two cannot drift
	// without the diff putting them side by side.
	browserOpenedPrefix = "เปิดแล้ว: "
	agentPageScanRows   = 200 // rows read before de-duplication
	agentPageMax        = 50
	agentPageDefault    = 24
)

// parseBrowserOpened splits the line `open` writes back apart again.
//
// Parsing our own sentence rather than storing structured output is the
// deliberate half of this: RawOutput is what the *model* reads (see
// toolRunOutput in the turn executor), so making it JSON for the sake of a
// panel would change model-facing text to save a function. The round-trip test
// in workbench_agentpages_test.go is what keeps the pair honest.
func parseBrowserOpened(output string) (title, pageURL string) {
	s := strings.TrimSpace(output)
	if !strings.HasPrefix(s, browserOpenedPrefix) || !strings.HasSuffix(s, ")") {
		return "", "" // a failure line ("เปิดไม่สำเร็จ: …") or something else entirely
	}
	s = strings.TrimPrefix(s, browserOpenedPrefix)
	// LAST " (", so a page whose own title contains one survives.
	open := strings.LastIndex(s, " (")
	if open < 0 {
		return "", ""
	}
	return strings.TrimSpace(s[:open]), s[open+2 : len(s)-1]
}

// urlFromArgs is the fallback when the sentence above cannot be read: a format
// change then costs the title, never the whole list.
func urlFromArgs(args string) string {
	var a struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return ""
	}
	u := strings.TrimSpace(a.URL)
	// browser_open also accepts a sandbox-relative path, and a row rebuilt from
	// one would navigate to https://out/report.html. Only an absolute URL is
	// recoverable from args alone.
	if !strings.Contains(u, "://") {
		return ""
	}
	return u
}

// stillOpenable drops a local file that has since been deleted — session output
// folders age out, and a row that opens the engine's "not found" page is the
// dead end this surface exists to prevent. Remote pages are not checked: a 404
// still has back, reload and the address bar directly above it.
func stillOpenable(pageURL string) bool {
	if !strings.HasPrefix(pageURL, "file:///") {
		return true
	}
	p := filepath.FromSlash(strings.TrimPrefix(pageURL, "file:///"))
	// Unescaped first, then raw: a Thai filename may be percent-encoded on the
	// way into the URL and is not on the way out of os.WriteFile.
	if unescaped, err := neturl.PathUnescape(p); err == nil {
		if _, err := os.Stat(unescaped); err == nil {
			return true
		}
	}
	_, err := os.Stat(p)
	return err == nil
}

// RecentAgentPages returns the pages the agent opened, newest first, one row
// per URL. Machine-wide rather than per-project or per-session: a page the
// agent opened is not project data, and scoping it through sessions would hide
// the row the list exists for.
func (a *App) RecentAgentPages(limit int) []AgentPage {
	out := []AgentPage{}
	if limit <= 0 || limit > agentPageMax {
		limit = agentPageDefault
	}
	db, err := a.database()
	if err != nil {
		// Logged, not propagated, for the reason recordToolRun states: the
		// pane's "nothing here yet" line is the honest content of a database
		// that will not open, too.
		debuglog.Msg("agent pages: db unavailable: %v", err)
		return out
	}
	rows, err := db.Query(
		`SELECT args, output, time FROM tool_runs
		 WHERE tool = 'browser_open' AND ok = 1
		 ORDER BY id DESC LIMIT ?`, agentPageScanRows)
	if err != nil {
		debuglog.Msg("agent pages: query failed: %v", err)
		return out
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var args, output, ts string
		if err := rows.Scan(&args, &output, &ts); err != nil {
			debuglog.Msg("agent pages: scan failed: %v", err)
			return out
		}
		title, pageURL := parseBrowserOpened(output)
		if pageURL == "" {
			pageURL = urlFromArgs(args)
		}
		// Newest wins: the same page opened three times is one row, carrying
		// the most recent time.
		if pageURL == "" || seen[pageURL] || !stillOpenable(pageURL) {
			continue
		}
		seen[pageURL] = true
		out = append(out, AgentPage{URL: pageURL, Title: title, Time: ts})
		if len(out) == limit {
			break
		}
	}
	return out
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
