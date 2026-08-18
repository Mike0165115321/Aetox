package main

// The browser tab's start page is a read of tool_runs, so its correctness is
// two things: the sentence browser_open writes can be read back apart, and the
// query answers "where has the agent been" rather than "what rows exist".
//
// The round-trip test is the load-bearing one. The format and its parser are a
// pair held together by nothing but a shared constant, and the failure mode if
// they drift is not a crash — it is a start page that quietly shows an empty
// list forever.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/turn"
)

func recordPage(t *testing.T, a *App, url, output string, ok bool) {
	t.Helper()
	a.recordToolRun(a.cur(), turn.ToolRun{
		Name:   "browser_open",
		Args:   fmt.Sprintf(`{"url":%q}`, url),
		Output: output,
		OK:     ok,
	})
}

// The REAL writer, not a copy of it. A second Sprintf here would move with the
// format whenever someone edited it, and this test would stay green while the
// parser silently stopped matching — which is the exact drift it exists to
// catch.
func opened(title, url string) string {
	return browserOpenedLine(title, url)
}

// If anyone edits browserOpenSkill.open's Sprintf without editing the parser,
// this is what fails.
func TestParseBrowserOpenedRoundTripsTheSkillsOwnFormat(t *testing.T) {
	title, url := parseBrowserOpened(opened("รายงานยอดขาย", "file:///D:/work/out/report.html"))
	if title != "รายงานยอดขาย" {
		t.Errorf("title = %q, want %q", title, "รายงานยอดขาย")
	}
	if url != "file:///D:/work/out/report.html" {
		t.Errorf("url = %q", url)
	}
}

func TestParseBrowserOpened(t *testing.T) {
	cases := []struct {
		name, output, title, url string
	}{
		{"plain", opened("Example", "https://example.test"), "Example", "https://example.test"},
		// A page whose own title carries " (" — LastIndex is why this survives.
		{"parens in title", opened("Go (golang) docs", "https://go.dev"), "Go (golang) docs", "https://go.dev"},
		{"empty title", opened("", "https://example.test"), "", "https://example.test"},
		{"thai title", opened("สรุปค่าใช้จ่าย สิงหาคม", "https://x.test/a"), "สรุปค่าใช้จ่าย สิงหาคม", "https://x.test/a"},
		// The other line the same skill writes. Reading a failure as a page is
		// the one parse error that would put a dead row on the desk.
		{"failure line", "เปิดไม่สำเร็จ: connection refused", "", ""},
		{"truncated", "เปิดแล้ว: Example (https://example.test", "", ""},
		{"unrelated", "ยอดเงิน 1,250 บาท", "", ""},
		{"empty", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, url := parseBrowserOpened(c.output)
			if title != c.title || url != c.url {
				t.Errorf("got (%q, %q), want (%q, %q)", title, url, c.title, c.url)
			}
		})
	}
}

func TestRecentAgentPagesNewestFirstAndDeduped(t *testing.T) {
	a := newRunApp(t)
	recordPage(t, a, "https://a.test", opened("A", "https://a.test"), true)
	recordPage(t, a, "https://b.test", opened("B", "https://b.test"), true)
	recordPage(t, a, "https://a.test", opened("A again", "https://a.test"), true)

	pages := a.RecentAgentPages(24)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2 (the repeat collapses): %+v", len(pages), pages)
	}
	// Newest wins, and it carries the newest title.
	if pages[0].URL != "https://a.test" || pages[0].Title != "A again" {
		t.Errorf("first = %+v, want the most recent open of a.test", pages[0])
	}
	if pages[1].URL != "https://b.test" {
		t.Errorf("second = %+v, want b.test", pages[1])
	}
}

func TestRecentAgentPagesExcludesWhatWasNeverOpened(t *testing.T) {
	a := newRunApp(t)
	// A failed open: the page was never shown, so it is not somewhere the agent
	// has been.
	a.recordToolRun(a.cur(), turn.ToolRun{
		Name: "browser_open", Args: `{"url":"https://dead.test"}`,
		Output: "เปิดไม่สำเร็จ: connection refused", OK: false,
	})
	// A different tool that also carries a url-shaped argument.
	a.recordToolRun(a.cur(), turn.ToolRun{
		Name: "web_fetch", Args: `{"url":"https://fetched.test"}`,
		Output: opened("Fetched", "https://fetched.test"), OK: true,
	})
	recordPage(t, a, "https://real.test", opened("Real", "https://real.test"), true)

	pages := a.RecentAgentPages(24)
	if len(pages) != 1 || pages[0].URL != "https://real.test" {
		t.Fatalf("got %+v, want only the successful browser_open", pages)
	}
}

func TestRecentAgentPagesFallsBackToArgs(t *testing.T) {
	a := newRunApp(t)
	// A row whose sentence cannot be read — a format change, a truncated
	// output. It must degrade to "right URL, no title", never to no row.
	recordPage(t, a, "https://x.test/page", "something else entirely", true)
	// ...but only when args hold a real URL. browser_open also takes a
	// sandbox-relative path, and https://out/report.html is a dead end.
	recordPage(t, a, "out/report.html", "something else entirely", true)

	pages := a.RecentAgentPages(24)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1: %+v", len(pages), pages)
	}
	if pages[0].URL != "https://x.test/page" || pages[0].Title != "" {
		t.Errorf("got %+v, want the args URL with no title", pages[0])
	}
}

func TestRecentAgentPagesDropsFilesThatAreGone(t *testing.T) {
	a := newRunApp(t)
	dir := t.TempDir()

	live := filepath.Join(dir, "live.html")
	if err := os.WriteFile(live, []byte("<h1>hi</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A Thai filename the engine percent-encodes on the way into the URL but
	// os.WriteFile stored unencoded — the unescape-then-raw pair.
	thai := filepath.Join(dir, "สรุป.html")
	if err := os.WriteFile(thai, []byte("<h1>สรุป</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}

	fileURL := func(p string) string {
		return "file:///" + filepath.ToSlash(p)
	}
	recordPage(t, a, fileURL(live), opened("Live", fileURL(live)), true)
	recordPage(t, a, fileURL(thai), opened("สรุป", strings.ReplaceAll(fileURL(thai), "สรุป", "%E0%B8%AA%E0%B8%A3%E0%B8%B8%E0%B8%9B")), true)
	gone := fileURL(filepath.Join(dir, "deleted.html"))
	recordPage(t, a, gone, opened("Gone", gone), true)

	pages := a.RecentAgentPages(24)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2 (the deleted one is dropped): %+v", len(pages), pages)
	}
	for _, p := range pages {
		if strings.Contains(p.URL, "deleted.html") {
			t.Errorf("a file that no longer exists is still on the desk: %+v", p)
		}
	}
}

func TestRecentAgentPagesClampsLimit(t *testing.T) {
	a := newRunApp(t)
	for i := 0; i < 30; i++ {
		u := fmt.Sprintf("https://p%d.test", i)
		recordPage(t, a, u, opened(fmt.Sprintf("P%d", i), u), true)
	}
	for _, c := range []struct {
		limit, want int
	}{{5, 5}, {0, agentPageDefault}, {-1, agentPageDefault}, {9999, agentPageDefault}} {
		if got := len(a.RecentAgentPages(c.limit)); got != c.want {
			t.Errorf("RecentAgentPages(%d) returned %d pages, want %d", c.limit, got, c.want)
		}
	}
}

// agoLabel() on the frontend feeds this straight to Date.parse.
func TestRecentAgentPagesTimeIsRFC3339(t *testing.T) {
	a := newRunApp(t)
	recordPage(t, a, "https://a.test", opened("A", "https://a.test"), true)

	pages := a.RecentAgentPages(24)
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if _, err := time.Parse(time.RFC3339, pages[0].Time); err != nil {
		t.Errorf("time %q does not parse as RFC3339: %v", pages[0].Time, err)
	}
}

// The panel does .length on this the moment it resolves.
func TestRecentAgentPagesIsNeverNil(t *testing.T) {
	a := newRunApp(t)
	if pages := a.RecentAgentPages(24); pages == nil {
		t.Error("empty history returned a nil slice")
	}
}
