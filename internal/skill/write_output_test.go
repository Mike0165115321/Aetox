package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Unfocused, the sandbox root is the user's home directory. A bare "write
// index.html" there littered the home folder, and the next chat writing
// index.html silently overwrote the first one.
func TestWritePlacesNewFilesInTheSessionOutputFolder(t *testing.T) {
	root := t.TempDir()
	s := &writeSkill{root: root, outputSubdir: func() string { return "aetox/output/20260726-021530.123" }}

	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"path": "index.html", "content": "<h1>a</h1>",
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	landed := filepath.Join(root, "aetox", "output", "20260726-021530.123", "index.html")
	if _, statErr := os.Stat(landed); statErr != nil {
		t.Fatalf("file did not land in the output folder: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "index.html")); statErr == nil {
		t.Error("file also landed at the sandbox root — that is the littering this prevents")
	}
	// The echoed path is what the model will read/edit next; it has to point
	// at where the file actually is. The on-disk path rides along because the
	// user's question is "where is it on my machine", and a model composing
	// root + the name it typed answers that wrong.
	want := "write done: aetox/output/20260726-021530.123/index.html (on disk: " + landed + ")"
	if out.Content != want {
		t.Errorf("receipt = %q, want %q", out.Content, want)
	}
}

func TestWritePlacementLeavesExplicitDestinationsAlone(t *testing.T) {
	root := t.TempDir()
	subdir := func() string { return "aetox/output/s1" }

	// Focused on a project, there is no output folder: files go where asked.
	plain := &writeSkill{root: root, outputSubdir: func() string { return "" }}
	if got := plain.placed("index.html"); got != "index.html" {
		t.Errorf("with no output folder, placed(index.html) = %q, want unchanged", got)
	}

	// An absolute path is a destination the caller spelled out, not a stray file.
	s := &writeSkill{root: root, outputSubdir: subdir}
	abs := filepath.Join(root, "docs", "index.html")
	if got := s.placed(abs); got != abs {
		t.Errorf("placed(%q) = %q, want unchanged", abs, got)
	}

	// A relative sub-path is part of the same piece of work — it belongs in
	// the session folder too, not scattered next to it.
	if got := s.placed("assets/style.css"); got != "aetox/output/s1/assets/style.css" {
		t.Errorf("placed(assets/style.css) = %q", got)
	}

	// No host wired one up at all (the CLI): behave exactly as before.
	none := &writeSkill{root: root}
	if got := none.placed("index.html"); got != "index.html" {
		t.Errorf("with a nil hook, placed(index.html) = %q, want unchanged", got)
	}
}

// The bug this closes: write steered index.html into the session folder, then
// "edit index.html" resolved against the root, failed, and the model spent a
// dozen tool calls re-finding the file it had just written.
func TestReadAndEditFindWhatWriteJustPlaced(t *testing.T) {
	root := t.TempDir()
	subdir := func() string { return "aetox/output/s1" }
	ctx := context.Background()

	w := &writeSkill{root: root, outputSubdir: subdir}
	if _, err := w.ExecuteTool(ctx, map[string]any{"path": "index.html", "content": "<h1>a</h1>"}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// The model repeats the path it originally asked for, not the placed one.
	r := &readSkill{root: root, outputSubdir: subdir}
	out, err := r.Execute(ctx, Input{"args": []string{"index.html"}})
	if err != nil {
		t.Fatalf("read of the original path failed: %v", err)
	}
	if out.Command != "read aetox/output/s1/index.html" {
		t.Errorf("read echoed %q, want the placed path so the model learns where it is", out.Command)
	}

	e := &editSkill{root: root, outputSubdir: subdir}
	if _, err := e.Execute(ctx, Input{"args": []string{"index.html", "<h1>a</h1>", "<h1>b</h1>"}}); err != nil {
		t.Fatalf("edit of the original path failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "aetox", "output", "s1", "index.html"))
	if err != nil || string(data) != "<h1>b</h1>" {
		t.Errorf("edit landed wrong: %q, %v", data, err)
	}
}

// The fallback must never shadow a real file — that would make a focused
// project read stale artifacts instead of its own source.
func TestPlacedFallbackPrefersTheLiteralPath(t *testing.T) {
	root := t.TempDir()
	subdir := func() string { return "aetox/output/s1" }

	for _, rel := range []string{"index.html", "aetox/output/s1/index.html"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := PlacedPath(root, subdir, "index.html"); got != "index.html" {
		t.Errorf("both exist: got %q, want the literal path to win", got)
	}

	// Nothing anywhere: report the path the caller asked for, so the error names it.
	if got := PlacedPath(root, subdir, "missing.html"); got != "missing.html" {
		t.Errorf("neither exists: got %q, want the original path", got)
	}
}
