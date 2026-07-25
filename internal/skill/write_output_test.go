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
	// at where the file actually is.
	if want := "write done: aetox/output/20260726-021530.123/index.html"; out.Content != want {
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
