package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The seven settings are the whole of what Aetox changes about somebody else's
// renderer, and six of them are a promise this project already made elsewhere:
// nothing goes back to HeyGen, nothing updates itself, nothing installs itself,
// and nothing is written outside our own folder. The seventh is about the
// picture, and has its own test below.
//
// A test rather than a comment because they disappear silently. A missing
// HYPERFRAMES_NO_AUTO_INSTALL does not fail — it downloads a browser on the
// user's machine and renders perfectly well afterwards, which is the shape of
// failure nobody files a bug about.
func TestTheSceneRendererIsToldWhatItMayNotDo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	env := hyperframesEnvironment(root, true, func(string) string { return "" })

	for key, want := range map[string]string{
		"HYPERFRAMES_NO_TELEMETRY":    "1",
		"HYPERFRAMES_NO_UPDATE_CHECK": "1",
		"HYPERFRAMES_NO_AUTO_INSTALL": "1",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}

	// Inside our folder, all of it. The point is not the exact path but that
	// none of these lands in the user's home directory, which is where the
	// renderer puts them when nobody says otherwise.
	for _, key := range []string{"HYPERFRAMES_BROWSER_PATH", "HYPERFRAMES_FONT_CACHE_DIR", "HYPERFRAMES_EXTRACT_CACHE_DIR"} {
		if !strings.HasPrefix(env[key], root) {
			t.Errorf("%s = %q, want it under %q", key, env[key], root)
		}
	}
}

// The fast capture path is off on every machine, and the veiled failure it
// causes is the reason.
//
// It reads Chrome's paint records instead of screenshotting, and to do that it
// moves `[data-composition-id]` into a canvas of its own — so a scene with no
// composition root dies at the first frame with HF_DE_COMPOSITION_ROOT_MISSING.
// Nine of the library's scenes carry that attribute and thirteen do not.
//
// Pinned rather than left to the default because the default is a GPU probe:
// upstream engages this "where it can", which on Windows means the same scene
// renders on one machine and fails on the next. Measured 31 ส.ค. — statement-title
// failed in 28s with this on, and rendered in 12s with it off.
func TestTheRendererDoesNotTakeTheFastPathThatSkipsHalfTheLibrary(t *testing.T) {
	env := hyperframesEnvironment(t.TempDir(), true, func(string) string { return "" })
	if env["PRODUCER_EXPERIMENTAL_FAST_CAPTURE"] != "false" {
		t.Errorf("PRODUCER_EXPERIMENTAL_FAST_CAPTURE = %q, want %q", env["PRODUCER_EXPERIMENTAL_FAST_CAPTURE"], "false")
	}
	// And without a DataRoot too: this one is not about paths, so a machine
	// whose data folder cannot be resolved must not silently get it back.
	if bare := hyperframesEnvironment("", false, func(string) string { return "" }); bare["PRODUCER_EXPERIMENTAL_FAST_CAPTURE"] != "false" {
		t.Error("the fast capture path came back on when DataRoot could not be resolved")
	}
}

// An ffmpeg that is not on the machine must not be named as if it were.
//
// The editor's own variables deliberately name a path before the download (see
// VideoEditorEnvironment), because that entry is written once and read at every
// later launch. This one is built fresh on every call, so naming a file that is
// not there buys nothing and costs the renderer its own "no ffmpeg" message,
// which is a better sentence than ours.
func TestNoFFmpegMeansNoFFmpegVariable(t *testing.T) {
	env := hyperframesEnvironment(t.TempDir(), true, func(string) string { return "" })
	for _, key := range []string{"HYPERFRAMES_FFMPEG_PATH", "HYPERFRAMES_FFPROBE_PATH"} {
		if _, named := env[key]; named {
			t.Errorf("%s was set to %q with no ffmpeg on the machine", key, env[key])
		}
	}

	found := hyperframesEnvironment(t.TempDir(), true, func(name string) string { return "C:/somewhere/" + name })
	if found["HYPERFRAMES_FFMPEG_PATH"] != "C:/somewhere/ffmpeg" {
		t.Errorf("HYPERFRAMES_FFMPEG_PATH = %q, want the copy that was found", found["HYPERFRAMES_FFMPEG_PATH"])
	}
}

// Without a DataRoot there are no paths to hand out, and the three switches are
// still true. A machine whose data folder cannot be resolved is a machine with
// a real problem; it is not a reason to let telemetry back on.
func TestTheSwitchesSurviveAMissingDataRoot(t *testing.T) {
	env := hyperframesEnvironment("", false, func(string) string { return "" })
	if env["HYPERFRAMES_NO_TELEMETRY"] != "1" {
		t.Error("telemetry was left on when DataRoot could not be resolved")
	}
	if _, named := env["HYPERFRAMES_BROWSER_PATH"]; named {
		t.Error("a browser path was invented without a DataRoot to put it in")
	}
}
