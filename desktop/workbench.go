package main

// Workbench tools: the right dock is the AI's workbench — these skills let the
// agent operate it during a chat turn. browser_open opens a real browser tab in
// the workbench (visible to the user) and waits for the page to load;
// browser_read returns the text of the page currently shown there. Registered
// per-engine-bootstrap in app.go alongside the default skill set.

import (
	"context"
	"database/sql"
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

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

var agentBrowserSeq int64

var (
	driveLetterRe = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
	urlSchemeRe   = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)
	bareSchemeRe  = regexp.MustCompile(`(?i)^(about|data|mailto|javascript):`)
)

// normalizeWorkbenchURL turns what the agent asked for into something the
// browser can go to, or reports that it was not an address at all.
//
// A local file first, and only a file that actually exists: every other tool
// speaks sandbox-relative paths, so without this the model has to splice the
// root in by hand to look at what it just made, and "index.html" would fall
// through and navigate to https://index.html. It resolves through
// skill.PlacedPath rather than joining onto the root, because `write` steers a
// new relative file into the session output folder — the model asks for
// "index.html", the file is really at "aetox/output/<session>/index.html", and a
// plain root join finds nothing and degrades to a DNS lookup for a hostname
// called index.html.
//
// Everything else goes to resolveAddress, which is now the single place that
// tells a place from a question (see address.go). What is new is the second
// return: this used to stamp https:// onto ANYTHING left over, so `open("ยูทูป")`
// became a DNS failure that read like a broken website. The agent has
// web_search; being told to use it is a better answer than being quietly
// searched for, which would teach it that `open` is a search box.
func normalizeWorkbenchURL(input, sandboxRoot string, outputSubdir func() string) (resolved, query string) {
	if root := strings.TrimSpace(sandboxRoot); root != "" && !urlSchemeRe.MatchString(input) && !bareSchemeRe.MatchString(input) {
		placed := skill.PlacedPath(root, outputSubdir, input)
		if abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(placed))); err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				return "file:///" + strings.ReplaceAll(abs, `\`, "/"), ""
			}
		}
	}
	addr := resolveAddress(input)
	return addr.URL, addr.Query
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
func (a *App) workbenchOpenBrowser(ctx context.Context, url string, newTab bool) (title, finalURL string, err error) {
	if a.ctx == nil {
		return "", "", fmt.Errorf("UI not ready")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return "", "", fmt.Errorf("url is required")
	}
	url, query := normalizeWorkbenchURL(url, a.cur().cfg.SandboxRoot, a.outputSubdir)
	if query != "" {
		return "", "", fmt.Errorf("%q is not an address, it is something to search for — use web_search, then open a result", query)
	}
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
	// Reuse stays the default. A session that never asks for a second tab
	// behaves exactly as it did before tabs were plural, which is what the
	// original one-tab rule was actually protecting: not scarcity, but the
	// tab-after-tab strandedness the owner watched happen on 2026-08-10.
	id, err := a.agentTab()
	reusing := err == nil && !newTab
	if !reusing {
		id = AgentTabID(fmt.Sprintf(agentTabPrefix+"%d", atomic.AddInt64(&agentBrowserSeq, 1)))
	}
	if reusing {
		// Armed before navigate, so the wait below is this navigation's.
		a.browsers.tab(string(id)).armNavigation()
		// And navigated ON the host thread, like every other call into a
		// webview in this app.
		//
		// This line used to call t.view.navigate directly, which is the one
		// rule browser_shot.go's header states and the only place that broke
		// it: WebView2 is apartment-threaded, so the same COM call made from
		// this goroutine is not slow or racy, it is refused outright. The
		// refusal goes to the engine's own error callback, so nothing
		// navigated, no NavigationCompleted ever fired, and awaitNavigation
		// below spent its full 20 seconds before reporting a page that had
		// never been asked for. Every page after the first in one session, in
		// a tool whose whole job is to open pages.
		//
		// That line no longer compiles: browserTab has no view to reach past
		// this call. See browserHost.onTab.
		a.onTab(string(id), func(v tabView, _ *browserTab) { v.navigate(url) })
	}
	// Emitted either way: for a new tab the frontend creates it, and for one
	// that exists the same handler just raises it — which is what the user
	// needs to actually see the page the agent moved to.
	a.deskEvent("", "open-browser", map[string]string{"id": string(id), "url": url})

	// The frontend creates the tab, which creates the native webview — poll
	// until it exists, then wait out its first navigation.
	deadline := time.Now().Add(20 * time.Second)
	var tab *browserTab
	for tab == nil {
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("browser tab did not open in time")
		}
		if h := a.browsers; h != nil {
			tab = h.tab(string(id))
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
	// meta (title/url) arrives from the page a beat after it loads — give it
	// one. armNavigation cleared whatever the last page left here, so this waits
	// for THIS page rather than succeeding instantly on the previous one's.
	for i := 0; i < 20; i++ {
		if title, finalURL = tab.meta(); title != "" || finalURL != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// A page that never posts meta — one with no script bridge, a PDF, an image
	// — is still a page we went to, and the URL we asked for is the truest thing
	// left to call it. Without this, clearing meta would trade a wrong name for
	// no name.
	if finalURL == "" {
		finalURL = url
	}
	return title, finalURL, nil
}

// AgentTabID names the tab the agent is working, and is a different type from
// the plain tab ids the frontend passes around on purpose.
//
// Every tab in the workbench used to be a string, so every tab was substitutable
// for every other one, and §127.1 is what that costs: the agent read, clicked
// and typed on whichever tab the user happened to be looking at, and nothing in
// the language had an opinion. agentTab is the only constructor, so an agent
// path cannot be handed the user's tab without an explicit conversion — which
// is one greppable token a reviewer can ask about, rather than an ordinary
// argument nobody looks at twice.
//
// It stops short of the plumbing underneath (browserSnapshot, BrowserClickRef)
// because that plumbing genuinely serves both callers: "use this page as
// context" is the user pointing at their own tab, through the same function.
// What is protected is the decision of WHICH tab, which is where the bug was.
type AgentTabID string

// agentTab names the agent's live browsing tab.
//
// One function where there were two. agentBrowserTabID answered "" and
// agentBrowserTabTarget answered an error, for the same question, differing only
// in how they said no — which is the same shape of debt as the lastID/agentID
// confusion they were both written to fix. `open` reads the error as "there is
// nothing to steer, mint one"; everyone else shows it to the model.
//
// "The agent's tab" is the most recent one it opened AND that is still alive: a
// tab the user closed is gone from the host's map, so this refuses and `open`
// mints a new one rather than navigating a corpse.
//
// It used to read the host's lastID and accept it only if it carried the
// web-agent- prefix, which quietly meant "the agent's tab, but only while it
// happens to be the one on screen". Raising a user's tab made it answer nothing
// with the agent's tab sitting right there alive: `open` minted a second one and
// stranded the first, which is the tab-after-tab behaviour reuse was introduced
// to end. The host remembers agentID itself now.
//
// The refusal is worded from the agent's side. "No browser tab open in the
// workbench" was true when the target was whatever was showing; once the target
// is the agent's own tab, the user can have a screenful of them and the honest
// answer is still that the agent has not opened one.
func (a *App) agentTab() (AgentTabID, error) {
	h := a.browsers
	if h == nil {
		return "", errNoAgentTab
	}
	h.mu.Lock()
	id := h.agentID
	goneID, goneWhy := h.goneID, h.goneWhy
	h.mu.Unlock()
	if id != "" && h.live(id) {
		return AgentTabID(id), nil
	}
	if goneID == "" {
		return "", errNoAgentTab
	}
	// Taken, not read, and only on the path that actually says it: this is told
	// once, to the call that runs into it. The second call has nothing new to
	// learn from it, and repeating it would have the model believing the page
	// was closed again. Consuming it here rather than above also stops an
	// unrelated successful call from eating a sentence meant for the one that
	// has no page.
	h.mu.Lock()
	h.goneID, h.goneWhy = "", 0
	h.mu.Unlock()
	switch goneWhy {
	case closedByUser:
		return "", errAgentTabClosed
	case closedByApp:
		return "", errAgentTabGone
	}
	// closedByAgent: it closed the page itself and does not need telling.
	return "", errNoAgentTab
}

// agentTabPeek names the agent's tab without spending anything.
//
// agentTab above cannot be used for this and the difference is not cosmetic: it
// TAKES agentTabClosed, so the flag that tells the model its page was closed
// out from under it is consumed by whoever asks first. The busy signal asks on
// every single tool call, browser or not, and it would have eaten that sentence
// before the browser tool ever got to say it — a UI detail silently deleting a
// message meant for the agent.
//
// So this is the read-only half: no error, no side effect, "" when there is no
// live agent tab. A panel with nothing to point at lights itself instead of a
// chip, which is the honest fallback (§174).
func (a *App) agentTabPeek() string {
	h := a.browsers
	if h == nil {
		return ""
	}
	h.mu.Lock()
	id := h.agentID
	h.mu.Unlock()
	if id == "" || !h.live(id) {
		return ""
	}
	return id
}

var errNoAgentTab = errors.New("the agent has no page open — use open first (tabs the user opened are theirs, not the agent's)")

// errAgentTabClosed is the same "no page" state with the reason attached, and
// the reason is the whole value of it: the agent did not lose its page, the
// user closed it. Worded so the next move is obvious, because a user action is
// something to work around rather than something to stop for.
var errAgentTabClosed = errors.New("the page you were working on was closed while you worked (the user closed the tab) — open it again and carry on from there")

// errAgentTabGone is the third answer, and it exists because the other two were
// both wrong for it. The app itself ended the page — a sweep after the window
// reloaded, a view that died — so "the user closed it" is an accusation of
// somebody who did nothing, and "you have no page open" reads as the agent
// having forgotten to open one. Same next move as errAgentTabClosed, no blame
// attached to it.
var errAgentTabGone = errors.New("the page you were working on is no longer open — open it again and carry on from there")

// browserWhyRefMissed says what is actually wrong with a ref that matched
// nothing, out of what this tab already knows — rather than naming the one
// cause somebody thought of first.
//
// The cause it used to name, "refs expire when the page changes", is real and
// was wrong for the miss that actually arrives. Measured on the owner's own
// session: a full read tagged 150 elements, a second read filtered to "English"
// tagged three, and `type ref 11` was refused with a sentence about the page
// changing on a page that had not changed at all. Every read reassigns refs
// from 1 and clears the ones before it, so the commonest way a ref dies is the
// next read — and the answer has to say so, because the recovery is different:
// re-read WITHOUT the filter, not wait for the page to settle.
func (a *App) browserWhyRefMissed(id AgentTabID, ref int) string {
	var t *browserTab
	if a.browsers != nil {
		t = a.browsers.tab(string(id))
	}
	count, filter, read := t.refs()
	switch {
	case !read:
		return " ยังไม่ได้ read หน้านี้เลย ref จึงยังไม่มีอะไรเลย อ่านก่อนแล้วใช้ ref จากรอบนั้น"
	case count == 0 && filter != "":
		return fmt.Sprintf(" read ครั้งล่าสุดใส่ filter=%q แล้วไม่เจอ element สักตัว จึงไม่มี ref อะไรอยู่บนหน้านี้เลยตอนนี้ อ่านใหม่โดยไม่ใส่ filter", filter)
	case count == 0:
		return " read ครั้งล่าสุดไม่เจอ element ที่กด/พิมพ์ได้เลยบนหน้านี้"
	case ref > count && filter != "":
		return fmt.Sprintf(" read ครั้งล่าสุดใส่ filter=%q ซึ่งแท็กไว้แค่ %d ตัว (ref 1-%d) และลบ ref ของรอบก่อนทิ้งไปแล้ว — ref %d เป็นของรอบก่อนหน้านั้น อ่านใหม่โดยไม่ใส่ filter แล้วใช้ ref จากรอบนั้น", filter, count, count, ref)
	case ref > count:
		return fmt.Sprintf(" read ครั้งล่าสุดแท็กไว้ %d ตัว (ref 1-%d) ref %d อยู่นอกช่วงนั้น อ่านใหม่แล้วใช้ ref จากรอบนั้น", count, count, ref)
	}
	return " ref มาจาก browser_read ครั้งล่าสุด และหมดอายุทันทีที่หน้าเปลี่ยน อ่านหน้าใหม่แล้วใช้ ref จากรอบนั้น"
}

// browserWhere names the page a tab is on, as a parenthetical to hang off the
// end of an action's result, or "" when the tab cannot say.
//
// Owner, 17 ส.ค.: *"อันที่ฉันเปิด มันก็ต้องรุ้ด้วยนะ ว่าปัจจุบันเปิดอะไรอยู่อีก"*. Steering
// one tab of your own is only half an answer — the other half is knowing what is
// in it. `open` and `read` both say so already; `click` and `type` said only
// that they had happened, which left the model to either remember across turns
// or spend a whole `read` asking where it was. A click is also the one action
// that can move the page, so the URL after it is worth more than the URL before.
func (a *App) browserWhere(id AgentTabID) string {
	if a.browsers == nil {
		return ""
	}
	t := a.browsers.tab(string(id))
	if t == nil {
		return ""
	}
	ref := browserPageRef(t.meta())
	if ref == "" {
		return ""
	}
	// Square brackets outside the ref's own parentheses, so "Title (url)" does
	// not end up nested inside a second pair and read as a typo.
	return " [อยู่ที่ " + ref + "]"
}

// workbenchReadBrowser reads the page the agent has open in the workbench
// browser — not whichever tab is showing. See agentTab.
func (a *App) workbenchReadBrowser(filter string) (title, url string, snap browserSnapshot, err error) {
	id, err := a.agentTab()
	if err != nil {
		return "", "", browserSnapshot{}, err
	}
	// string(id) here and at every other plumbing call below: the conversion is
	// deliberate and greppable, and it sits one line under the only function
	// that can produce an AgentTabID — which is the point. See AgentTabID.
	snap, err = a.browserSnapshot(string(id), filter)
	if err != nil {
		return "", "", browserSnapshot{}, err
	}
	if t := a.browsers.tab(string(id)); t != nil {
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
	newTab, _ := args["newTab"].(bool)
	return s.open(ctx, url, newTab)
}

func (s *browserOpenSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	url, _ := input["url"].(string)
	newTab, _ := input["newTab"].(bool)
	return s.open(ctx, url, newTab)
}

func (s *browserOpenSkill) open(ctx context.Context, url string, newTab bool) (skill.Output, error) {
	start := time.Now()
	title, finalURL, err := s.app.workbenchOpenBrowser(ctx, url, newTab)
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
	return browserOpenedPrefix + browserPageRef(title, url)
}

// browserPageRef names a page, and is the one place any browser action spells
// out which page it is talking about.
//
// It exists because there were four spellings and they were added one at a
// time, each perfectly reasonable on its own: `open` said "Title (url)", `read`
// wrote a document header, and `click`/`type`/`capture` each invented a third
// and fourth. One fact, four renderings, in a file that already keeps
// browserOpenedPrefix as a shared constant precisely so `open`'s sentence and
// parseBrowserOpened cannot drift apart. The lesson was already written down
// here; it just was not applied to the sentences added after it.
//
// `read` is the deliberate exception and stays a two-line markdown header. It
// is not a sentence referring to a page, it is the top of a document the model
// reads as a document, and folding it into this shape would make it worse to
// read to make it easier to count.
func browserPageRef(title, url string) string {
	title, url = strings.TrimSpace(title), strings.TrimSpace(url)
	switch {
	case title != "" && url != "":
		return fmt.Sprintf("%s (%s)", title, url)
	case url != "":
		return url // a page that has not told us its title yet
	default:
		return title // rare, and better than saying nothing
	}
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
	if !strings.HasPrefix(s, browserOpenedPrefix) {
		return "", "" // a failure line ("เปิดไม่สำเร็จ: …") or something else entirely
	}
	s = strings.TrimPrefix(s, browserOpenedPrefix)

	// A page with no title is written as the bare address, because
	// "เปิดแล้ว:  (https://x)" is a sentence with a hole in it. Both shapes are
	// browserPageRef's output and both have to come back apart here — this
	// parser and that writer are one contract with two halves, which is what
	// the shared prefix constant above has always been about.
	if !strings.HasSuffix(s, ")") {
		if urlSchemeRe.MatchString(s) || strings.HasPrefix(s, "file:///") {
			return "", s
		}
		return "", "" // truncated, or a sentence that is not this one
	}

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
	seen := map[string]bool{}
	_ = eachRow(db, "agent pages", `
		SELECT args, output, time FROM tool_runs
		 WHERE tool = 'browser_open' AND ok = 1
		 ORDER BY id DESC LIMIT ?`, []any{agentPageScanRows},
		func(rows *sql.Rows) error {
			var args, output, ts string
			if err := rows.Scan(&args, &output, &ts); err != nil {
				return err
			}
			title, pageURL := parseBrowserOpened(output)
			if pageURL == "" {
				pageURL = urlFromArgs(args)
			}
			// Newest wins: the same page opened three times is one row, carrying
			// the most recent time.
			if pageURL == "" || seen[pageURL] || !stillOpenable(pageURL) {
				return nil
			}
			seen[pageURL] = true
			out = append(out, AgentPage{URL: pageURL, Title: title, Time: ts})
			if len(out) == limit {
				// A full page is not a failure, and rows.Err() is nil after a
				// caller-initiated stop.
				return errStopRows
			}
			return nil
		})
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
		map[string]any{"type": "object", "properties": map[string]any{
			"filter": map[string]any{"type": "string", "description": "List only elements whose text contains this"},
		}})
}

func (s *browserReadSkill) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	f, _ := args["filter"].(string)
	return s.Execute(ctx, skill.Input{"filter": f})
}

func (s *browserReadSkill) Execute(_ context.Context, input skill.Input) (skill.Output, error) {
	start := time.Now()
	filter, _ := input["filter"].(string)
	filter = strings.TrimSpace(filter)
	title, url, snap, err := s.app.workbenchReadBrowser(filter)
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
	out.Content = formatBrowserRead(title, url, filter, text, snap)
	out.RawOutput = out.Content
	// A cut element list is a truncated result exactly as a cut page text is.
	// It was not marked as one until 2026-08-22, so every surface that shows
	// the user "this was shortened" showed nothing for the commonest case.
	out.Truncated = truncated || snap.ElementsTotal > len(snap.Elements) || snap.ImagesTotal > len(snap.Images)
	return out, nil
}

// formatBrowserRead is everything the model is told about the page, built from
// one snapshot and nothing else.
//
// Pure, and separate from Execute, because what it writes is the whole point of
// a read and none of it could be tested before: Execute needs a live app window
// and a real webview, so tool_coverage_test marks browser_read "never"
// available. The formatting is the part that has to keep its promises about
// what was left out, and a promise no test can reach is one that quietly stops
// being kept.
func formatBrowserRead(title, url, filter, text string, snap browserSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\nURL: %s\n", title, url)
	if len(snap.Elements) > 0 {
		// The count goes in the HEADER, not only in the line under the list.
		//
		// It was only under the list until 2026-08-22, and two full test passes
		// on two different days read a 150-item list, missed the line below it,
		// and each invented the same explanation — that the output had been cut
		// off. It had not: the whole result was 5,797 bytes and the line sat at
		// character 5,208, well inside everything. A fact placed after 150 lines
		// of "[73] button ..." is a fact nobody reaches, and being right about
		// where it was does not make it read.
		//
		// The line below the list stays, because it is the one that names
		// `filter` — the way past the cap. This one is the number, said before
		// the wall rather than after it.
		count := ""
		if snap.ElementsTotal > len(snap.Elements) {
			count = fmt.Sprintf(" — %d of %d listed", len(snap.Elements), snap.ElementsTotal)
		}
		if filter != "" {
			fmt.Fprintf(&b, "\nClickable/typeable elements whose text contains %q%s (use browser_click/browser_type with ref):\n", filter, count)
		} else {
			fmt.Fprintf(&b, "\nClickable/typeable elements%s (use browser_click/browser_type with ref):\n", count)
		}
		for _, el := range snap.Elements {
			role := el.Role
			if role == "" {
				role = el.Tag
			}
			fmt.Fprintf(&b, "[%d] %s: %q\n", el.Ref, role, el.Text)
		}
		// The line this list did not have until 2026-08-22. A cut list and a
		// short page were the same output, so a model that could not find its
		// button had no way to tell "not on this page" from "past number 150" —
		// and every re-read handed back the same first 150 in the same DOM
		// order. Saying the number is half of it; naming the way past it is the
		// other half, because a limit with no door is only a slower no.
		if extra := snap.ElementsTotal - len(snap.Elements); extra > 0 {
			fmt.Fprintf(&b, "... and %d more not listed. Read again with filter=\"<text>\" to reach them: it lists only the elements whose text contains that.\n", extra)
		}
	} else if filter != "" {
		fmt.Fprintf(&b, "\nNo interactive element on this page has text containing %q. That is an answer about the filter, not about the page: read again without one to see what is here.\n", filter)
	}
	if len(snap.Images) > 0 {
		imgCount := ""
		if snap.ImagesTotal > len(snap.Images) {
			imgCount = fmt.Sprintf(" — %d of %d listed", len(snap.Images), snap.ImagesTotal)
		}
		fmt.Fprintf(&b, "\nImages on the page%s (show one to the user with markdown: ![alt](url)):\n", imgCount)
		for _, im := range snap.Images {
			alt := im.Alt
			if alt == "" {
				alt = "(no alt)"
			}
			fmt.Fprintf(&b, "- %s — %s\n", im.Src, alt)
		}
		if extra := snap.ImagesTotal - len(snap.Images); extra > 0 {
			fmt.Fprintf(&b, "... and %d more images not listed.\n", extra)
		}
	}
	// Reported rather than omitted, for the same reason the counts above are.
	// A cross-origin frame cannot be entered from JS by any technique, so this
	// is a permanent limit of reading a page rather than something to retry —
	// and without the line, a checkout form or an embedded editor simply is not
	// in the text, which reads exactly like a page that has not finished
	// loading. That is the one wrong conclusion read's own guidance pushes the
	// model toward, so the fact has to arrive with the read that lacks it.
	if snap.BlockedFrames > 0 {
		fmt.Fprintf(&b, "\n%d frame(s) on this page come from another site and cannot be read from here. Anything inside them is missing from the text below, and waiting will not bring it.\n", snap.BlockedFrames)
	}
	fmt.Fprintf(&b, "\n%s", text)
	return b.String()
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
	id, err := s.app.agentTab()
	var res browserActResult
	var answered bool
	if err == nil {
		// The ring goes down first, on the agent's path only. browserClickRef
		// is shared with BrowserClickRef, which is the *user* clicking through
		// their own panel — and a mark saying "the agent is working here" drawn
		// over somebody's own click is the one thing this layer must never do.
		s.app.markPageClick(id, ref)
		res, answered, err = s.app.browserClickRef(string(id), ref)
	}
	out.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Content, out.Stderr = "คลิกไม่สำเร็จ: "+err.Error(), err.Error()
		return out, err
	}
	// The answer this tool did not have until 2026-08-22, and the one that
	// turned a small bug upstream into a six-round loop: a ref matching nothing
	// used to report a successful click. See aetoxActJS for the turn.
	if answered && !res.Found {
		msg := fmt.Sprintf("ไม่มี element ref %d บนหน้านี้ ยังไม่ได้คลิกอะไรเลย%s%s",
			ref, s.app.browserWhere(id), s.app.browserWhyRefMissed(id, ref))
		out.Content, out.Stderr = msg, msg
		return out, errors.New(msg)
	}
	time.Sleep(300 * time.Millisecond) // let click-driven navigation/DOM update settle before the next browser_read
	out.Success = true
	// Naming what was clicked, not just that something was. "คลิก ref 2 แล้ว"
	// is unfalsifiable from the outside; the tag and label are what let a caller
	// see it hit the element it meant.
	out.Content = fmt.Sprintf("คลิก %s แล้ว%s ใช้ browser_read เพื่อดูผลลัพธ์", browserActLabel(ref, res, answered), s.app.browserWhere(id))
	out.RawOutput = out.Content
	return out, nil
}

// browserActLabel names the element an action landed on, falling back to the
// bare ref when the page never reported one. The fallback says so out loud
// rather than reading like a confirmed hit.
func browserActLabel(ref int, res browserActResult, answered bool) string {
	if !answered {
		return fmt.Sprintf("ref %d (หน้าไม่ได้ยืนยันกลับมาว่าตรงกับ element ไหน)", ref)
	}
	tag := res.Tag
	if tag == "" {
		tag = "element"
	}
	if res.Label == "" {
		return fmt.Sprintf("%s ref %d", tag, ref)
	}
	return fmt.Sprintf("%s %q (ref %d)", tag, res.Label, ref)
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
	id, err := s.app.agentTab()
	var res browserActResult
	var answered bool
	if err == nil {
		res, answered, err = s.app.browserTypeRef(string(id), ref, text, enter)
	}
	out.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Content, out.Stderr = "พิมพ์ไม่สำเร็จ: "+err.Error(), err.Error()
		return out, err
	}
	// Same rule as click's, and it matters more here: text typed into nothing
	// is invisible on the next read, so a false success sends the model looking
	// for a form it never filled.
	if answered && !res.Found {
		msg := fmt.Sprintf("ไม่มี element ref %d บนหน้านี้ ยังไม่ได้พิมพ์อะไรลงไปเลย%s%s",
			ref, s.app.browserWhere(id), s.app.browserWhyRefMissed(id, ref))
		out.Content, out.Stderr = msg, msg
		return out, errors.New(msg)
	}
	if enter {
		time.Sleep(300 * time.Millisecond) // let Enter-driven navigation settle before the next browser_read
	}
	out.Success = true
	where := browserActLabel(ref, res, answered)
	out.Content = fmt.Sprintf("พิมพ์ลง %s แล้ว%s", where, s.app.browserWhere(id))
	if enter {
		out.Content = fmt.Sprintf("พิมพ์ลง %s และกด Enter แล้ว%s ใช้ browser_read เพื่อดูผลลัพธ์", where, s.app.browserWhere(id))
	}
	out.RawOutput = out.Content
	return out, nil
}
