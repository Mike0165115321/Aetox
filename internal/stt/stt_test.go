package stt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain doubles as a stand-in for the whisper.cpp binary: the transcribe
// test re-execs this test binary in the binary's place, and the env var below
// tells the child to behave like whisper instead of running the suite — the
// os/exec helper-process idiom, so the real command line is exercised on a
// machine with no whisper.cpp and no model downloaded.
func TestMain(m *testing.M) {
	if canned, ok := os.LookupEnv("AETOX_TEST_FAKE_WHISPER"); ok {
		os.Exit(fakeWhisperMain(canned))
	}
	os.Exit(m.Run())
}

// fakeWhisperMain asserts it was invoked the way whisper.cpp expects before
// printing canned segments: if the engine ever passes a model that isn't there,
// an input that isn't a converted WAV, or drops -l auto / -np, this exits
// non-zero and the calling test fails.
func fakeWhisperMain(canned string) int {
	values, flags := map[string]string{}, map[string]bool{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
			values[arg] = os.Args[i+1]
			i++
			continue
		}
		flags[arg] = true
	}
	switch {
	case !isRegularFile(values["-m"]):
		fmt.Fprintf(os.Stderr, "fake whisper: -m %q is not an existing model file\n", values["-m"])
	case !isRegularFile(values["-f"]) || !strings.HasSuffix(values["-f"], ".wav"):
		fmt.Fprintf(os.Stderr, "fake whisper: -f %q is not an existing .wav\n", values["-f"])
	case values["-l"] != "auto":
		fmt.Fprintf(os.Stderr, "fake whisper: -l = %q, want auto (Thai+English detection)\n", values["-l"])
	case !flags["-np"]:
		fmt.Fprintln(os.Stderr, "fake whisper: -np missing, banner noise would reach the parser")
	default:
		fmt.Print(canned)
		return 0
	}
	return 1
}

func stubLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = previous })
}

// modelsDirWith points the data root at a temp dir and drops the named model
// files in it, returning the models directory.
func modelsDirWith(t *testing.T, names ...string) string {
	t.Helper()
	dataRoot := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", dataRoot)
	dir := filepath.Join(dataRoot, "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stub model"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCatalogHasExactlyOneDefault(t *testing.T) {
	defaults := 0
	for _, d := range Catalog() {
		if d.Default {
			defaults++
		}
		if d.ID == "" || d.Label == "" || d.Install == "" {
			t.Errorf("engine %q is missing ID/Label/Install — the settings UI renders all three", d.ID)
		}
	}
	if defaults != 1 {
		t.Errorf("catalog has %d defaults, want exactly 1 (empty config must resolve to something)", defaults)
	}
}

func TestLookupEmptyIDResolvesToDefault(t *testing.T) {
	got, ok := Lookup("")
	if !ok || !got.Default {
		t.Fatalf("Lookup(\"\") = %+v, %v; want the default engine", got, ok)
	}
	if _, ok := Lookup("  WHISPER-CPP "); !ok {
		t.Error("Lookup should trim and lowercase config values")
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup should reject an unknown engine")
	}
}

func TestNewUnknownEngineNamesWhatIsSupported(t *testing.T) {
	_, err := New(Options{Engine: "vosk"})
	if err == nil {
		t.Fatal("expected an error for an engine that is not in the catalog")
	}
	if !strings.Contains(err.Error(), "whisper-cpp") {
		t.Errorf("error should list what IS supported so the user can fix it; got: %v", err)
	}
}

func TestNewMissingBinaryGivesInstallInstructions(t *testing.T) {
	stubLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	modelsDirWith(t, preferredWhisperModel)

	_, err := New(Options{})
	if err == nil {
		t.Fatal("expected an error when the engine binary is absent")
	}
	if strings.Contains(err.Error(), exec.ErrNotFound.Error()) {
		t.Errorf("raw exec error leaked instead of instructions: %v", err)
	}
	if !strings.Contains(err.Error(), "ไม่พบโปรแกรม") || !strings.Contains(err.Error(), "scoop install whisper-cpp") {
		t.Errorf("error should name the engine and how to install it, in Thai; got: %v", err)
	}
}

func TestNewMissingModelGivesDownloadInstructions(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) { return name, nil })
	dir := modelsDirWith(t)

	_, err := New(Options{})
	if err == nil {
		t.Fatal("expected an error when no model file is present")
	}
	for _, want := range []string{preferredWhisperModel, "141 MB", dir} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("model error should mention %q; got: %v", want, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("nothing may be downloaded automatically, found %d files", len(entries))
	}
}

func TestResolveModelAcceptsAnyMatchingModel(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) { return name, nil })
	dir := modelsDirWith(t, "ggml-tiny-q5_1.bin")

	engine, err := New(Options{})
	if err != nil {
		t.Fatalf("a downloaded tiny model should be accepted, got: %v", err)
	}
	whisper, ok := engine.(*whisperCPP)
	if !ok {
		t.Fatalf("New returned %T, want *whisperCPP", engine)
	}
	if want := filepath.Join(dir, "ggml-tiny-q5_1.bin"); whisper.modelPath != want {
		t.Errorf("modelPath = %q, want %q", whisper.modelPath, want)
	}
}

func TestResolveModelPinnedPathWins(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) { return name, nil })
	dir := modelsDirWith(t, preferredWhisperModel)
	pinned := filepath.Join(dir, "ggml-small.bin")
	if err := os.WriteFile(pinned, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{ModelPath: pinned})
	if err != nil {
		t.Fatalf("New with a pinned model: %v", err)
	}
	if got := engine.(*whisperCPP).modelPath; got != pinned {
		t.Errorf("modelPath = %q, want the pinned %q", got, pinned)
	}

	if _, err := New(Options{ModelPath: filepath.Join(dir, "gone.bin")}); err == nil {
		t.Error("a pinned model that no longer exists must fail loudly, not fall back silently")
	}
}

func TestInstalledModelsListsWhatTheUIWouldShow(t *testing.T) {
	dir := modelsDirWith(t, "ggml-base.bin", "ggml-tiny.bin", "notes.txt")
	desc, _ := Lookup("")

	// Filter to the managed store: a dev machine may legitimately have Ollama
	// or LM Studio directories, and those are none of this test's business.
	var got []InstalledModel
	for _, m := range InstalledModels(desc, Options{}) {
		if m.Managed {
			got = append(got, m)
		}
	}

	want := []string{"ggml-base.bin", "ggml-tiny.bin"} // notes.txt must not match
	if len(got) != len(want) {
		t.Fatalf("InstalledModels() managed entries = %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("model %d name = %q, want %q", i, got[i].Name, want[i])
		}
		if got[i].Path != filepath.Join(dir, want[i]) {
			t.Errorf("model %d path = %q, want it under the managed dir %q", i, got[i].Path, dir)
		}
		if got[i].Bytes == 0 || got[i].Store == "" {
			t.Errorf("model %d = %+v, want size and store label filled in for the UI", i, got[i])
		}
	}
}

// A model in someone else's folder is usable but never ours to delete.
func TestInstalledModelsMarksExternalStoresUnmanaged(t *testing.T) {
	modelsDirWith(t) // empty managed store
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "ggml-small.bin"), []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	desc, _ := Lookup("")

	found := InstalledModels(desc, Options{ExtraModelDirs: []string{external}})
	var external_ *InstalledModel
	for i := range found {
		if found[i].Name == "ggml-small.bin" {
			external_ = &found[i]
		}
	}
	if external_ == nil {
		t.Fatalf("a model in a user folder should be found, got %+v", found)
	}
	if external_.Managed {
		t.Error("a model in the user's own folder must never be marked managed — Aetox may not delete it")
	}
}

func TestStoresAlwaysOffersTheManagedDirectoryFirst(t *testing.T) {
	modelsDirWith(t)
	stores := Stores(Options{})
	if len(stores) == 0 || !stores[0].Managed {
		t.Fatalf("Stores() = %+v, want the Aetox-managed directory first", stores)
	}
	for _, s := range stores[1:] {
		if s.Managed {
			t.Errorf("more than one managed store: %+v — downloads must have exactly one destination", s)
		}
	}
}

func TestParseWhisperOutput(t *testing.T) {
	raw := strings.Join([]string{
		"whisper_init_from_file_with_params_no_state: loading model",
		"[00:00:00.000 --> 00:00:03.480]   สวัสดีครับ ยินดีต้อนรับ",
		"[00:00:03.480 --> 00:00:07.000]  This is the second line.",
		"[00:00:07.000 --> 00:00:11.000]  This is the second line.",
		"[00:01:05.120 --> 00:01:09.000]   หลังจากผ่านไปหนึ่งนาที",
		"[00:02:00.000 --> 00:02:04.000]   ",
		"[malformed line without an arrow]",
		"",
	}, "\n")

	got := parseWhisperOutput(raw)
	want := []Segment{
		{StartMs: 0, EndMs: 3480, Text: "สวัสดีครับ ยินดีต้อนรับ"},
		{StartMs: 3480, EndMs: 7000, Text: "This is the second line."},
		{StartMs: 65120, EndMs: 69000, Text: "หลังจากผ่านไปหนึ่งนาที"},
	}
	if len(got) != len(want) {
		t.Fatalf("parseWhisperOutput() returned %d segments, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseWhisperTimestamp(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"00:00:00.000", 0, true},
		{"00:00:07.480", 7480, true},
		{"00:01:05.120", 65120, true},
		{"01:02:03.000", 3723000, true},
		{"02:05", 125000, true},
		{"00:00:01.5", 1500, true},
		{"12", 0, false},
		{"00:00:00:00:00", 0, false},
		{"aa:bb.000", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseWhisperTimestamp(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseWhisperTimestamp(%q) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// The real command line, with whisper itself stubbed: the stub fails this test
// unless the engine hands it an existing model, an existing .wav, -l auto and
// -np. Proven to fail — flipping -l auto to -l th in whisper_cpp.go turns it red.
func TestWhisperTranscribeCommandLine(t *testing.T) {
	stubLookPath(t, func(string) (string, error) { return os.Args[0], nil })
	modelsDirWith(t, preferredWhisperModel)
	t.Setenv("AETOX_TEST_FAKE_WHISPER", "[00:00:02.000 --> 00:00:04.500]   เสียงทดสอบ\n")

	wav := filepath.Join(t.TempDir(), "audio.wav")
	if err := os.WriteFile(wav, []byte("RIFF....WAVE"), 0o600); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := engine.Transcribe(context.Background(), wav)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	want := []Segment{{StartMs: 2000, EndMs: 4500, Text: "เสียงทดสอบ"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Transcribe() = %+v, want %+v", got, want)
	}
	if engine.ID() != "whisper-cpp" {
		t.Errorf("ID() = %q, want whisper-cpp", engine.ID())
	}
}
