package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepSkillFindsMatchesWithLineNumbers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc TargetFunc() {}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "TargetFunc", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
	if !strings.Contains(out.Content, "a.go:3:func TargetFunc() {}") {
		t.Errorf("Content = %q, want match with path:line:text", out.Content)
	}
}

func TestGrepSkillNoMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("nothing here"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "absent"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "(no matches)") {
		t.Errorf("Content = %q, want (no matches)", out.Content)
	}
}

func TestGrepSkillScopedToSubdir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "inner.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "path": "sub", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if strings.Contains(out.Content, "top.txt") {
		t.Errorf("Content = %q, should not include files outside sub/", out.Content)
	}
	if !strings.Contains(out.Content, "sub/inner.txt:1:needle") {
		t.Errorf("Content = %q, want sub/inner.txt match", out.Content)
	}
}

func TestGrepSkillSkipsDotDirsAndBinary(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte("needle\x00"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "(no matches)") {
		t.Errorf("Content = %q, want dot-dir and binary skipped", out.Content)
	}
}

func TestGrepSkillExecuteCLIPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"needle", "."}, "show": grepModeContent})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt:1:needle") {
		t.Errorf("Content = %q, want a.txt:1:needle", out.Content)
	}
}

func TestGrepSkillCapsResults(t *testing.T) {
	root := t.TempDir()
	lines := strings.Repeat("needle\n", 250)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(lines), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true at result cap")
	}
	// The marker names the next offset, not just the fact of the cap — that is
	// the whole point of paging, and a marker without it sends the model back to
	// inventing a narrower pattern.
	if !strings.Contains(out.Content, "offset=200") {
		t.Errorf("Content missing the resume offset: %q", out.Content[len(out.Content)-100:])
	}
}

// The three output shapes, on one tree, so the cheap ones are provably cheaper
// than the expensive one rather than merely different.
func TestGrepSkillOutputModes(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"a.txt": "needle\nneedle\nother\n",
		"b.txt": "needle\n",
		"c.txt": "nothing here\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	s := &grepSkill{root: root}

	run := func(t *testing.T, args map[string]any) string {
		t.Helper()
		args["pattern"] = "needle"
		out, err := s.ExecuteTool(context.Background(), args)
		if err != nil {
			t.Fatalf("ExecuteTool(%v): %v", args, err)
		}
		return out.Content
	}

	files := run(t, map[string]any{"show": "files_with_matches"})
	if !strings.Contains(files, "a.txt") || !strings.Contains(files, "b.txt") {
		t.Errorf("files_with_matches = %q, want both matching files", files)
	}
	if strings.Contains(files, "c.txt") || strings.Contains(files, ":1:") {
		t.Errorf("files_with_matches = %q, want paths only and no non-matching file", files)
	}

	counts := run(t, map[string]any{"show": "count"})
	if !strings.Contains(counts, "a.txt:2") || !strings.Contains(counts, "b.txt:1") {
		t.Errorf("count = %q, want a.txt:2 and b.txt:1", counts)
	}

	// limit and offset are one mechanism seen from both ends: page one and
	// page two must not overlap, and together must cover everything.
	first := run(t, map[string]any{"limit": 2, "show": grepModeContent})
	second := run(t, map[string]any{"limit": 2, "offset": 2, "show": grepModeContent})
	if strings.Count(first, "needle") != 2 {
		t.Errorf("limit=2 returned %q, want two matches", first)
	}
	if strings.Contains(second, "a.txt:1:") {
		t.Errorf("offset=2 returned %q, want the first page skipped", second)
	}
	if !strings.Contains(first, "offset=2") {
		t.Errorf("limit=2 returned %q, want a resume hint", first)
	}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "show": "nonsense"}); err == nil {
		t.Error("show=nonsense was accepted; an unknown mode must fail rather than silently return content")
	}
}

func TestGrepSkillTruncatesLongLines(t *testing.T) {
	root := t.TempDir()
	long := "needle " + strings.Repeat("x", 500)
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(long), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	for _, line := range strings.Split(out.Content, "\n") {
		if len(line) > 250 {
			t.Errorf("line length %d exceeds truncation cap: %q...", len(line), line[:80])
		}
	}
	if !strings.Contains(out.Content, "...") {
		t.Errorf("Content = %q, want truncated line marker", out.Content)
	}
}

func TestGrepSkillCaseInsensitiveFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("NeeDLe"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "(?i)needle", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt:1:NeeDLe") {
		t.Errorf("Content = %q, want case-insensitive match", out.Content)
	}
}

func TestGrepSkillSingleFileTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("needle"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "path": "only.txt", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "only.txt:1:needle") || strings.Contains(out.Content, "other.txt") {
		t.Errorf("Content = %q, want only.txt match only", out.Content)
	}
}

func TestGrepSkillRejectsBadRegex(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "("}); err == nil {
		t.Fatal("expected regex compile error, got nil")
	}
}

func TestGrepSkillRejectsEscape(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "x", "path": "../outside"}); err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

// The one this whole change exists for: a folder that is not there must not
// come back as a search that found nothing. "(no matches)" is a statement about
// the caller's files, and a walk that opened none of them has not earned it.
func TestGrepRefusesAFolderThatIsNotThereInsteadOfReportingNoMatches(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "password", "path": "not-here"})
	if err == nil {
		t.Fatal("a missing folder searched clean — the agent would report the pattern is absent from code it never read")
	}
	if strings.Contains(out.Content, "no matches") {
		t.Errorf("Content = %q, want no trace of the empty-result sentence", out.Content)
	}
	// Both spellings, because the mangled one is the whole diagnosis: without it
	// the model cannot tell a typo from a path that resolved somewhere else.
	if !strings.Contains(err.Error(), "not-here") || !strings.Contains(err.Error(), s.root) {
		t.Errorf("error should name what was asked for and where it resolved to, got: %v", err)
	}
}

func TestGrepSkillExecuteToolMissingPattern(t *testing.T) {
	s := &grepSkill{root: t.TempDir()}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected error for missing pattern, got nil")
	}
}

// A search that walks node_modules is not a search: on the Aetox repo that is
// 10,826 of 12,073 files, so the 200-result cap fills with vendored code
// before reaching src/.
func TestGrepSkipsDependencyAndBuildDirs(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/app.go", "func handleClick() {}\n")
	write("node_modules/pkg/index.js", "function handleClick() {}\n")
	write("dist/bundle.js", "function handleClick() {}\n")
	write("vendor/lib/x.go", "func handleClick() {}\n")

	s := &grepSkill{root: root}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "handleClick"})
	if err != nil {
		t.Fatalf("grep failed: %v", err)
	}
	if !strings.Contains(out.Content, "src/app.go") {
		t.Errorf("the user's own code was not found:\n%s", out.Content)
	}
	for _, noise := range []string{"node_modules", "dist", "vendor"} {
		if strings.Contains(out.Content, noise) {
			t.Errorf("grep descended into %s:\n%s", noise, out.Content)
		}
	}
}

// Without a glob, a search for a common word drags in every language in the
// repo; without context, the model must call read again just to see enough
// code to build an exact edit — one wasted round trip per fix.
func TestGrepGlobAndContext(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package main\n\nfunc target() {\n\tprintln(1)\n}\n")
	write("b.ts", "export function target() {}\n")
	write("c.svelte", "<script>function target() {}</script>\n")
	s := &grepSkill{root: root}

	only := func(args map[string]any) string {
		out, err := s.ExecuteTool(context.Background(), args)
		if err != nil {
			t.Fatalf("grep failed: %v", err)
		}
		return out.Content
	}

	got := only(map[string]any{"pattern": "target", "glob": "*.go"})
	if !strings.Contains(got, "a.go") || strings.Contains(got, "b.ts") || strings.Contains(got, "c.svelte") {
		t.Errorf("glob *.go did not narrow the search:\n%s", got)
	}

	got = only(map[string]any{"pattern": "target", "glob": "*.{ts,svelte}"})
	if !strings.Contains(got, "b.ts") || !strings.Contains(got, "c.svelte") || strings.Contains(got, "a.go") {
		t.Errorf("brace alternation did not work:\n%s", got)
	}

	// Context lines use ripgrep's separators: ':' on the match, '-' around it.
	got = only(map[string]any{"pattern": "func target", "path": "a.go", "context": 1})
	if !strings.Contains(got, "a.go:3:func target() {") {
		t.Errorf("match line missing or mis-numbered:\n%s", got)
	}
	if !strings.Contains(got, "a.go-2-") || !strings.Contains(got, "a.go-4-\tprintln(1)") {
		t.Errorf("context lines missing:\n%s", got)
	}

	// Zero context stays exactly as before.
	got = only(map[string]any{"pattern": "func target", "path": "a.go"})
	if strings.Contains(got, "a.go-") {
		t.Errorf("context appeared without being asked for:\n%s", got)
	}
}

func TestMatchesGlob(t *testing.T) {
	cases := []struct {
		glob, name string
		want       bool
	}{
		{"*.go", "app.go", true},
		{"*.go", "app.ts", false},
		{"*.{ts,svelte}", "Chat.svelte", true},
		{"*.{ts,svelte}", "main.go", false},
		{"**/*.go", "app.go", true}, // a pattern copied from another tool still works
		{"", "anything", true},
		{"app.go", "app.go", true},
	}
	for _, c := range cases {
		if got := matchesGlob(c.glob, c.name); got != c.want {
			t.Errorf("matchesGlob(%q, %q) = %v, want %v", c.glob, c.name, got, c.want)
		}
	}
}

// The default is the file list, and content is one word away.
//
// Both halves matter, and the second is the one to be careful about: flipping a
// default is only honest while the thing it replaced stays fully available.
// This fails if either half stops being true.
func TestGrepSkillDefaultsToFilesWithMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc TargetFunc() {}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	byDefault, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "TargetFunc"})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(byDefault.Content, "a.go") {
		t.Errorf("default = %q, want the matching path", byDefault.Content)
	}
	if strings.Contains(byDefault.Content, "func TargetFunc") {
		t.Errorf("default = %q, want paths only: the default must not carry the matching lines", byDefault.Content)
	}

	asked, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "TargetFunc", "show": grepModeContent})
	if err != nil {
		t.Fatalf("ExecuteTool(show=content): %v", err)
	}
	if !strings.Contains(asked.Content, "a.go:3:func TargetFunc() {}") {
		t.Errorf("show=content = %q, want path:line:text", asked.Content)
	}
	if len(asked.Content) <= len(byDefault.Content) {
		t.Errorf("show=content is %d bytes against the default's %d; the default is meant to be the cheap one",
			len(asked.Content), len(byDefault.Content))
	}
}

// Asking for context is asking for the lines. Answering that with a file list
// would be a green tick over an empty answer.
func TestGrepSkillContextSelectsContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc target() {\n\tprintln(1)\n}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "func target", "context": 1})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !strings.Contains(out.Content, "a.go:3:func target() {") {
		t.Errorf("Content = %q, want the match line", out.Content)
	}
	if !strings.Contains(out.Content, "a.go-4-\tprintln(1)") {
		t.Errorf("Content = %q, want the context line that was asked for", out.Content)
	}

	// An explicit show still wins: context is a nudge, not an override.
	paths, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "func target", "context": 1, "show": grepModeFiles})
	if err != nil {
		t.Fatalf("ExecuteTool(show=files_with_matches): %v", err)
	}
	if strings.Contains(paths.Content, "println") {
		t.Errorf("show=files_with_matches = %q, want the explicit mode honoured", paths.Content)
	}
}

// The two search tools now teach, and this is what stops that being undone by
// accident. The delivery mechanism is pinned generically in
// block_standard_test.go against a probe; what a probe cannot pin is whether
// the tools a session actually reaches for implement Guided at all.
func TestSearchToolsTeachOnFirstUse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: root})
	d := NewDispatcher(registry)

	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		// Through the pack, and that is the assertion: guidance is keyed per
		// action (guidance.go), so a session that greps is taught grep's and a
		// session that only lists is taught nothing.
		{"search", map[string]any{"action": "grep", "pattern": "package"}, "mapping first and reading second"},
		{"read", map[string]any{"path": "a.go"}, "Find the place before reading it"},
	} {
		first, ok, err := d.ExecuteTool(t.Context(), tc.tool, tc.args)
		if !ok || err != nil {
			t.Fatalf("%s: ok=%v err=%v", tc.tool, ok, err)
		}
		if !strings.Contains(first.RawOutput, tc.want) {
			t.Errorf("%s never taught the model: %q", tc.tool, first.RawOutput)
		}
		second, _, _ := d.ExecuteTool(t.Context(), tc.tool, tc.args)
		if strings.Contains(second.RawOutput, tc.want) {
			t.Errorf("%s taught twice in one session", tc.tool)
		}
	}
}

// A wide window is the point of raising the cap, so both ends are pinned: 50
// lines is honoured, and 51 is clamped rather than accepted or refused.
func TestGrepContextReachesFiftyLines(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		if i == 100 {
			b.WriteString("needle\n")
			continue
		}
		b.WriteString(fmt.Sprintf("line-%d\n", i))
	}
	if err := os.WriteFile(filepath.Join(root, "wide.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &grepSkill{root: root}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "context": 50})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	// 50 either side of line 100 reaches line 50 and line 150.
	for _, want := range []string{"wide.txt-50-line-50", "wide.txt:100:needle", "wide.txt-150-line-150"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("context=50 did not reach %q", want)
		}
	}
	if strings.Contains(out.Content, "wide.txt-49-") {
		t.Error("context=50 reached further than 50 lines")
	}

	// Past the cap it clamps. Refusing would cost a round trip to learn a
	// number the schema already states.
	over, err := s.ExecuteTool(context.Background(), map[string]any{"pattern": "needle", "context": 500})
	if err != nil {
		t.Fatalf("ExecuteTool(context=500): %v", err)
	}
	if strings.Contains(over.Content, "wide.txt-49-") {
		t.Error("context=500 was not clamped to the cap")
	}
}
