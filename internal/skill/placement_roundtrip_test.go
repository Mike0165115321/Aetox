package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolCall drives a skill exactly the way a model does — through ExecuteTool,
// with the argument names in its published schema — so the test exercises the
// dispatch path rather than a Go-only shortcut past it.
func toolCall(t *testing.T, r *Registry, name string, args map[string]any) Output {
	t.Helper()
	s, ok := r.Get(name)
	if !ok {
		t.Fatalf("%s is not registered", name)
	}
	tool, ok := s.(Tool)
	if !ok {
		t.Fatalf("%s is not model-facing", name)
	}
	out, err := tool.ExecuteTool(context.Background(), args)
	if err != nil {
		t.Fatalf("%s(%v) failed: %v", name, args, err)
	}
	return out
}

// Unfocused, write steers a new relative file into the session output folder.
// Everything the model might do to that file afterwards names it by the path it
// asked write for — "index.html", not the placed path — because that is the
// path it chose and the one it remembers. Twice now a tool has resolved that
// name against the sandbox root instead, found nothing, and either failed
// outright (edit) or silently degraded (browser_open navigated to
// https://index.html). So every model-facing file tool is exercised here
// against that exact scenario, in one place, rather than trusted per tool.
func TestEveryFileToolFindsWhatWritePlaced(t *testing.T) {
	root := t.TempDir()
	const subdir = "aetox/output/s1"
	registry := NewDefaultRegistry(RegistryOptions{
		SandboxRoot:  root,
		OutputSubdir: func() string { return subdir },
	})
	placed := filepath.Join(root, filepath.FromSlash(subdir), "index.html")

	// write: the model asks for a bare name and gets told where it really went.
	out := toolCall(t, registry, "write", map[string]any{
		"path": "index.html", "content": "<h1>alpha</h1>\n<p>beta</p>\n",
	})
	if want := "write done: " + subdir + "/index.html (on disk: " + placed + ")"; out.Content != want {
		t.Fatalf("write receipt = %q, want %q", out.Content, want)
	}
	if _, err := os.Stat(placed); err != nil {
		t.Fatalf("file did not land in the output folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err == nil {
		t.Fatal("file also landed at the sandbox root")
	}

	// From here on every call uses "index.html" — the name the model knows.
	t.Run("read", func(t *testing.T) {
		out := toolCall(t, registry, "read", map[string]any{"path": "index.html"})
		if !strings.Contains(out.Content, "alpha") {
			t.Errorf("read returned %q", out.Content)
		}
		if !strings.Contains(out.Command, subdir) {
			t.Errorf("read echoed %q — it should report where the file really is", out.Command)
		}
	})

	t.Run("grep", func(t *testing.T) {
		out := toolCall(t, registry, "grep", map[string]any{"pattern": "beta", "path": "index.html"})
		if !strings.Contains(out.Content, "beta") {
			t.Errorf("grep found nothing: %q", out.Content)
		}
	})

	t.Run("glob", func(t *testing.T) {
		out := toolCall(t, registry, "glob", map[string]any{"pattern": "**/index.html"})
		if !strings.Contains(out.Content, "index.html") {
			t.Errorf("glob found nothing: %q", out.Content)
		}
	})

	t.Run("list", func(t *testing.T) {
		// A directory the model never sees at the root either.
		out := toolCall(t, registry, "list", map[string]any{"path": subdir})
		if !strings.Contains(out.Content, "index.html") {
			t.Errorf("list returned %q", out.Content)
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		// No language server exists for .html; what matters is that the path
		// resolved rather than erroring out before it got that far.
		out := toolCall(t, registry, "diagnostics", map[string]any{"path": "index.html"})
		if !strings.Contains(out.Command, subdir) {
			t.Errorf("diagnostics echoed %q — path never resolved to the placed file", out.Command)
		}
	})

	t.Run("apply_patch", func(t *testing.T) {
		toolCall(t, registry, "apply_patch", map[string]any{
			"edits": []any{map[string]any{
				"path": "index.html", "old_string": "<p>beta</p>", "new_string": "<p>gamma</p>",
			}},
		})
		data, err := os.ReadFile(placed)
		if err != nil || !strings.Contains(string(data), "gamma") {
			t.Errorf("apply_patch did not reach the placed file: %q, %v", data, err)
		}
	})

	t.Run("edit", func(t *testing.T) {
		toolCall(t, registry, "edit", map[string]any{
			"path": "index.html", "old_string": "alpha", "new_string": "delta",
		})
		data, err := os.ReadFile(placed)
		if err != nil || !strings.Contains(string(data), "delta") {
			t.Errorf("edit did not reach the placed file: %q, %v", data, err)
		}
	})

	// Last, because it removes what the others need.
	t.Run("delete", func(t *testing.T) {
		toolCall(t, registry, "delete", map[string]any{"path": "index.html"})
		if _, err := os.Stat(placed); err == nil {
			t.Error("delete did not reach the placed file")
		}
	})
}

// Focused on a project there is no output folder, so the rule must be inert:
// a relative path is the project's own path and nothing may redirect it.
func TestFileToolsUnchangedWhenFocusedOnAProject(t *testing.T) {
	root := t.TempDir()
	registry := NewDefaultRegistry(RegistryOptions{
		SandboxRoot:  root,
		OutputSubdir: func() string { return "" },
	})

	out := toolCall(t, registry, "write", map[string]any{"path": "main.go", "content": "package main\n"})
	if out.Content != "write done: main.go" {
		t.Fatalf("write receipt = %q, want the path as asked", out.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "main.go")); err != nil {
		t.Fatalf("file did not land at the project root: %v", err)
	}
	read := toolCall(t, registry, "read", map[string]any{"path": "main.go"})
	if read.Command != "read main.go" {
		t.Errorf("read echoed %q, want the path unchanged", read.Command)
	}
}

// A real file must never be shadowed by a same-named artifact in the output
// folder — that would make a tool silently operate on the wrong file.
func TestPlacedPathNeverShadowsARealFile(t *testing.T) {
	root := t.TempDir()
	const subdir = "aetox/output/s1"
	registry := NewDefaultRegistry(RegistryOptions{
		SandboxRoot:  root,
		OutputSubdir: func() string { return subdir },
	})

	for path, body := range map[string]string{
		"notes.md":           "the real one",
		subdir + "/notes.md": "the artifact",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := toolCall(t, registry, "read", map[string]any{"path": "notes.md"})
	if !strings.Contains(out.Content, "the real one") {
		t.Errorf("read returned %q — the artifact shadowed the real file", out.Content)
	}
}

// The CLI wires no output folder at all. A nil hook must be ordinary, not a
// crash: this is the path every non-desktop caller takes.
func TestFileToolsWithoutAnOutputFolderHook(t *testing.T) {
	root := t.TempDir()
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: root})

	toolCall(t, registry, "write", map[string]any{"path": "a.txt", "content": "hi"})
	out := toolCall(t, registry, "read", map[string]any{"path": "a.txt"})
	if !strings.Contains(out.Content, "hi") {
		t.Errorf("read returned %q", out.Content)
	}
}
