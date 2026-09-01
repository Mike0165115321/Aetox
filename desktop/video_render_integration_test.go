package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// One clip, made the way the agent makes one.
//
// **Why this is not part of TestEveryToolRunsThroughTheRealDispatcher.** That
// test moves DataRoot into a temp directory so it can never touch what is
// installed on the machine running it, which is right — and it means the scene
// renderer is invisible to it, so `video check` and `video render` are recorded
// there as skipped for ever. Proving those two needs the real installation, and
// a test that reaches for the real installation has to be a different test with
// its own honest skip.
//
// **The path is the product's path.** The registry, the dispatcher and the
// arguments are what a model's tool call arrives as; nothing is assembled by
// hand here. The temptation this file exists to refuse is copying a scene into
// a folder, setting six environment variables and running the renderer
// directly: that proves Hyperframes works, which nobody doubted, while the tool
// the agent actually calls stays unexercised.
func TestOneSceneBecomesAClip(t *testing.T) {
	// **Pointed back at the real data root, deliberately, and only to read.**
	//
	// TestMain sends every test in this package to a throwaway root, for a good
	// reason it states in full: tests used to overwrite the developer's own
	// model preference. That guard is right and stays. But the renderer is not
	// something a test can install — it is 267MB the user fetched by pressing a
	// button — so a test that never looks at the real root can only ever skip,
	// and `video render` would ship having never run anywhere.
	//
	// The exception is narrow. Everything this test WRITES goes to a temp
	// sandbox below; the real root is read for one thing, which is where the
	// renderer and its browser live. And if this path is wrong on some machine,
	// the checks underneath find nothing and the test skips — a wrong guess here
	// cannot turn into a pass.
	installed, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user config directory on this machine")
	}
	t.Setenv("AETOX_DATA_ROOT", filepath.Join(installed, "aetox"))

	if node, _ := hyperframesParts(); node == "" {
		t.Skip("no scene renderer on this machine — งานวิดีโอ installs it")
	}
	if findSceneBrowser() == "" {
		t.Skip("no browser for the renderer on this machine")
	}
	if findProgram("ffmpeg") == "" {
		t.Skip("no ffmpeg on this machine")
	}

	root := t.TempDir()
	app := &App{}
	app.cur().cfg.SandboxRoot = root

	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	for _, s := range app.workbenchSkills(app.cur(), root) {
		if err := registry.Register(s, skill.SourceBuiltin); err != nil {
			t.Fatalf("register %s: %v", s.Name(), err)
		}
	}
	dispatcher := skill.NewDispatcher(registry)

	// A render is minutes of a real machine, so the budget is generous and the
	// failure is a stated timeout rather than a hung test.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// `statement-title` is the simplest thing in the library: one self-contained
	// file, CSS keyframes, no sub-scenes and no audio. If this cannot render,
	// nothing can, and the failure is in the pipe rather than in the scene.
	out, found, err := dispatcher.ExecuteTool(ctx, "video", map[string]any{
		"action":   "new",
		"template": "statement-title",
		"path":     "clip",
		"seconds":  4,
	})
	if !found {
		t.Fatal("the dispatcher does not know a tool called video")
	}
	if err != nil || !out.Success {
		t.Fatalf("video new: %v / %s", err, out.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "clip", "index.html")); err != nil {
		t.Fatalf("video new reported success and wrote no index.html: %v", err)
	}

	// The middle move, which is the one the loop is worth having. `check` opens
	// the project once and reports what a person would have seen; the whole
	// argument for it is that it costs seconds where a render costs minutes, so
	// a test that proves the two ends and skips it proves the cheap half of the
	// loop is decoration.
	out, _, err = dispatcher.ExecuteTool(ctx, "video", map[string]any{
		"action": "check",
		"path":   "clip",
	})
	if err != nil || !out.Success {
		t.Fatalf("video check: %v / %s", err, strings.TrimSpace(out.Content+out.Stderr))
	}
	// And the report itself came back, not the last line of the font loader.
	//
	// `check` exits non-zero on any finding, and this scene has several that are
	// its own design — three headline lines that deliberately overlap. Reading
	// that exit code as a failure threw away the whole report and handed the
	// agent "Fetched 5 font face(s) for Space Grotesk" instead, which is the
	// bug this line is here to catch coming back.
	for _, want := range []string{"Lint", "Layout", "Contrast"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("video check reported success with no %s section in it: %q", want, out.Content)
		}
	}

	out, _, err = dispatcher.ExecuteTool(ctx, "video", map[string]any{
		"action":  "render",
		"path":    "clip",
		"quality": "draft",
	})
	if err != nil || !out.Success {
		t.Fatalf("video render: %v / %s", err, strings.TrimSpace(out.Content+out.Stderr))
	}

	// The tool reports where it put the file; the assertion is on the file.
	made := filepath.Join(root, "clip.mp4")
	info, err := os.Stat(made)
	if err != nil {
		t.Fatalf("render said it succeeded and there is no file at %s: %v", made, err)
	}
	// A few hundred bytes of container with no frames in it is what a broken
	// encode leaves behind, and it passes an existence check.
	if info.Size() < 10<<10 {
		t.Fatalf("the clip is %d bytes, which is a header and no video", info.Size())
	}
	t.Logf("rendered %s (%.2f MB)", made, float64(info.Size())/(1<<20))
}
