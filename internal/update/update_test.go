package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
		why             string
	}{
		{"0.8.5", "0.8.4", true, "patch bump"},
		{"0.9.0", "0.8.4", true, "minor bump"},
		{"1.0.0", "0.8.4", true, "major bump"},
		{"0.8.4", "0.8.4", false, "same release"},
		{"0.8.3", "0.8.4", false, "older release"},
		{"v0.8.5", "0.8.4", true, "tag still carrying its v"},
		// The whole reason this is not a string comparison.
		{"0.8.10", "0.8.9", true, "double-digit patch beats single"},
		{"0.8.9", "0.8.10", false, "and not the other way round"},
		{"0.9.0-rc1", "0.8.4", true, "prerelease suffix ignored, numbers still win"},
		{"1.0", "0.8.4", true, "two-part version pads to 1.0.0"},
		// Unparseable input must never produce a banner.
		{"", "0.8.4", false, "empty"},
		{"nightly", "0.8.4", false, "not a number"},
		{"0.8.5", "", false, "no current version to compare against"},
		{"1.2.3.4", "0.8.4", false, "four parts is not a version we understand"},
	} {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v — %s", c.latest, c.current, got, c.want, c.why)
		}
	}
}

// A stand-in for a Windows machine's environment, so the path tables below
// mean the same thing on the Linux and macOS runners.
func winEnv(k string) string {
	switch k {
	case "ProgramFiles":
		return `C:\Program Files`
	case "ProgramFiles(x86)":
		return `C:\Program Files (x86)`
	case "LOCALAPPDATA":
		return `C:\Users\Mike\AppData\Local`
	}
	return ""
}

func TestClassifyWindows(t *testing.T) {
	env := winEnv
	for _, c := range []struct {
		exe  string
		want Channel
		why  string
	}{
		{`C:\Users\Mike\scoop\apps\aetox\current\aetox.exe`, ChannelScoop, "scoop's versioned app dir"},
		{`C:\Program Files\Aetox\Aetox\aetox.exe`, ChannelInstaller, "what release.yml's NSIS build produces"},
		{`C:\Program Files (x86)\Aetox\Aetox\aetox.exe`, ChannelInstaller, "32-bit program files"},
		{`C:\Users\Mike\AppData\Local\Programs\Aetox\aetox.exe`, ChannelInstaller, "WAILS_INSTALL_SCOPE=user"},
		{`D:\Tools\aetox\aetox.exe`, ChannelUnknown, "unpacked zip — Detect turns this into portable"},
		{`C:\Users\Mike\Desktop\aetox.exe`, ChannelUnknown, "loose exe"},
		// The trailing-separator check earns its keep here.
		{`C:\Program FilesX\Aetox\aetox.exe`, ChannelUnknown, "a different folder that merely starts the same"},
		// Windows paths are case-insensitive; the user's PATH entry may not match.
		{`c:\program files\aetox\aetox\AETOX.EXE`, ChannelInstaller, "different case, same place"},
		{`C:\Users\Mike\SCOOP\Apps\Aetox\current\aetox.exe`, ChannelScoop, "scoop, shouting"},
	} {
		if got := classifyWindows(c.exe, env); got != c.want {
			t.Errorf("classifyWindows(%q) = %q, want %q — %s", c.exe, got, c.want, c.why)
		}
	}
}

func TestDetectFrom(t *testing.T) {
	noLinks := func(p string) (string, error) { return p, nil }
	// A Scoop shim: what runs is `…\scoop\shims\aetox.exe`, which resolves into
	// the versioned app dir. Either half alone is enough to identify it, and
	// the reverse case (a symlink *out* of scoop) must not be misread.
	intoScoop := func(string) (string, error) {
		return `C:\Users\Mike\scoop\apps\aetox\0.8.4\aetox.exe`, nil
	}
	brokenLink := func(string) (string, error) { return "", os.ErrNotExist }

	for _, c := range []struct {
		name    string
		exe     string
		resolve func(string) (string, error)
		goos    string
		want    Channel
	}{
		{"scoop shim resolving into the app dir", `C:\Users\Mike\scoop\shims\aetox.exe`, intoScoop, "windows", ChannelScoop},
		{"scoop path with no symlink to follow", `C:\Users\Mike\scoop\apps\aetox\current\aetox.exe`, noLinks, "windows", ChannelScoop},
		{"installer", `C:\Program Files\Aetox\Aetox\aetox.exe`, noLinks, "windows", ChannelInstaller},
		// The fallback that makes portable the default rather than a guess.
		{"loose exe is portable", `D:\Tools\aetox\aetox.exe`, noLinks, "windows", ChannelPortable},
		// A resolver that fails must not lose the answer the raw path already had.
		{"unresolvable symlink still classifies", `C:\Program Files\Aetox\Aetox\aetox.exe`, brokenLink, "windows", ChannelInstaller},
		{"unresolvable and unrecognised is still portable", `D:\Tools\aetox.exe`, brokenLink, "windows", ChannelPortable},
		// Until §48's port ships, every other OS gets the releases page.
		{"linux is not classified yet", "/usr/bin/aetox", noLinks, "linux", ChannelUnknown},
		{"macos is not classified yet", "/Applications/Aetox.app/Contents/MacOS/aetox", noLinks, "darwin", ChannelUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := detectFrom(c.exe, c.resolve, c.goos, winEnv); got != c.want {
				t.Errorf("detectFrom(%q, %s) = %q, want %q", c.exe, c.goos, got, c.want)
			}
		})
	}
}

// Whatever this machine actually is, Detect must answer with a channel the UI
// knows how to render — never a panic and never an empty string.
func TestDetectAlwaysAnswersWithAKnownChannel(t *testing.T) {
	switch got := Detect(); got {
	case ChannelScoop, ChannelInstaller, ChannelPortable, ChannelUnknown:
	default:
		t.Fatalf("Detect() = %q, which no Settings → About branch handles", got)
	}
}

func TestUpgradeHintOnlyWhereWeCanGiveOne(t *testing.T) {
	if got := UpgradeHint(ChannelScoop); got != "scoop update aetox" {
		t.Errorf("scoop hint = %q", got)
	}
	// Every other channel must stay silent rather than invent a command: the
	// UI falls back to the releases page, which is always right.
	for _, c := range []Channel{ChannelInstaller, ChannelPortable, ChannelUnknown} {
		if got := UpgradeHint(c); got != "" {
			t.Errorf("UpgradeHint(%q) = %q, want no command", c, got)
		}
	}
}

// serve stands in for GitHub, and points the package at itself. Also isolates
// the on-disk cache into the test's own temp dir, so a real check the developer
// ran on this machine cannot change the result.
func serve(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv(DisableEnv, "")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	prev := apiURL
	apiURL = srv.URL
	t.Cleanup(func() { apiURL = prev })
}

func TestCheckFindsANewerRelease(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("no User-Agent — GitHub rejects unauthenticated calls without one")
		}
		w.Header().Set("ETag", `W/"abc123"`)
		_, _ = w.Write([]byte(`[{"tag_name":"v0.9.0","html_url":"https://example.invalid/r/v0.9.0"}]`))
	})

	st, err := Check(context.Background(), "0.8.4")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !st.Available {
		t.Error("0.9.0 over 0.8.4 should be available")
	}
	if st.Latest != "0.9.0" {
		t.Errorf("Latest = %q, want 0.9.0 (the v belongs to the tag, not the version)", st.Latest)
	}
	if st.URL != "https://example.invalid/r/v0.9.0" {
		t.Errorf("URL = %q, want the release's own page", st.URL)
	}
	if st.CheckedAt == "" {
		t.Error("CheckedAt is empty after a completed check")
	}
}

func TestCheckIsQuietWhenUpToDate(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v0.8.4"}]`))
	})

	st, err := Check(context.Background(), "0.8.4")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if st.Available {
		t.Error("the release we are already running is not an update")
	}
	if st.Latest != "0.8.4" {
		t.Errorf("Latest = %q, want the version we asked about", st.Latest)
	}
}

// The ETag round-trip is what keeps a daily check inside GitHub's 60/hour
// budget for unauthenticated callers: the second call sends If-None-Match and
// gets a bodiless 304 back, and must still report the same answer.
func TestSecondCheckReusesTheETag(t *testing.T) {
	var calls, conditional int
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("If-None-Match") == `W/"abc123"` {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `W/"abc123"`)
		// Assets ride along, as every real release's do — a cached answer
		// without them is treated as unable to answer a 304 (see the
		// If-None-Match condition in checkWithAssets).
		_, _ = w.Write([]byte(`[{"tag_name":"v0.9.0","html_url":"https://example.invalid/r",
			"assets":[{"name":"checksums.txt","browser_download_url":"https://example.invalid/c","size":1}]}]`))
	})

	if _, err := Check(context.Background(), "0.8.4"); err != nil {
		t.Fatalf("first Check: %v", err)
	}
	st, err := Check(context.Background(), "0.8.4")
	if err != nil {
		t.Fatalf("second Check: %v", err)
	}
	if conditional != 1 {
		t.Errorf("second call sent If-None-Match %d times, want 1 (calls=%d)", conditional, calls)
	}
	if !st.Available || st.Latest != "0.9.0" {
		t.Errorf("304 lost the cached answer: %+v", st)
	}
	if LastCheckedAt().IsZero() {
		t.Error("LastCheckedAt is zero after two completed checks")
	}
}

// A cache written by a build from before assets were recorded can answer
// "which version" but not "which files" — sending its ETag would freeze the
// one-click button dark until the NEXT release changed the tag. The check
// pays one full-priced request to repair the cache, then goes back to 304s.
// Found live: the very first drill of Apply on this machine hit it.
func TestOldCacheWithoutAssetsRepairsItself(t *testing.T) {
	var conditional, full int
	withAssets := false
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		full++
		w.Header().Set("ETag", `W/"e1"`)
		body := `[{"tag_name":"v0.9.0","html_url":"https://example.invalid/r"}]`
		if withAssets {
			body = `[{"tag_name":"v0.9.0","html_url":"https://example.invalid/r",
				"assets":[{"name":"checksums.txt","browser_download_url":"https://example.invalid/c","size":1},
				          {"name":"aetox-windows-amd64-portable.zip","browser_download_url":"https://example.invalid/z","size":9}]}]`
		}
		_, _ = w.Write([]byte(body))
	})

	// The old build's cache: version and ETag, no assets.
	if _, err := Check(context.Background(), "0.8.4"); err != nil {
		t.Fatalf("seed Check: %v", err)
	}
	// The new build checks: it must NOT trust that cache into a 304.
	withAssets = true
	if _, err := Check(context.Background(), "0.8.4"); err != nil {
		t.Fatalf("repair Check: %v", err)
	}
	if conditional != 0 || full != 2 {
		t.Errorf("repair path: conditional=%d full=%d, want 0 and 2 — the assetless cache must not answer", conditional, full)
	}
	// Repaired: the budget-saving 304 path resumes.
	if _, err := Check(context.Background(), "0.8.4"); err != nil {
		t.Fatalf("third Check: %v", err)
	}
	if conditional != 1 {
		t.Errorf("after repair, conditional=%d, want 1", conditional)
	}
}

func TestCheckReportsGitHubFailuresWithoutPanicking(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	})

	st, err := Check(context.Background(), "0.8.4")
	if err == nil {
		t.Fatal("a 403 should be reported as an error")
	}
	// Failing open: the caller still gets something it can render.
	if st.Current != "0.8.4" || st.URL == "" {
		t.Errorf("failed check returned nothing usable: %+v", st)
	}
	if st.Available {
		t.Error("a failed check must never claim an update is available")
	}
}

func TestDisableEnvStopsTheCheckEntirely(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be made when the check is disabled")
	})
	t.Setenv(DisableEnv, "1")

	if !Disabled() {
		t.Fatal("Disabled() should be true")
	}
	st, err := Check(context.Background(), "0.8.4")
	if err != ErrDisabled {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
	if st.Available {
		t.Error("a disabled check must not report an update")
	}
}

// "0" and "false" are how someone turns the switch back off in a shell profile
// they cannot easily edit conditionally; treating them as "disabled" would be
// the opposite of what they wrote.
func TestDisableEnvOffValues(t *testing.T) {
	for _, v := range []string{"", "0", "false", "FALSE"} {
		t.Setenv(DisableEnv, v)
		if Disabled() {
			t.Errorf("%s=%q should not disable the check", DisableEnv, v)
		}
	}
	for _, v := range []string{"1", "true", "yes"} {
		t.Setenv(DisableEnv, v)
		if !Disabled() {
			t.Errorf("%s=%q should disable the check", DisableEnv, v)
		}
	}
}

// Nothing in `go test ./...` may reach github.com — internal/rtk/install.go
// learned this when a clean CI runner downloaded a third-party binary mid-suite.
func TestCheckRefusesToUseTheRealAPIFromATest(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv(DisableEnv, "")
	if apiURL != defaultAPIURL {
		t.Fatalf("apiURL leaked from another test: %q", apiURL)
	}
	st, err := Check(context.Background(), "0.8.4")
	if err != ErrDisabled {
		t.Errorf("err = %v, want ErrDisabled — a test just tried to call GitHub", err)
	}
	// The guard reports "did not run", same as the user's own switch — a
	// Status that looked like a completed check would let any other package's
	// test assert against an answer nobody actually got.
	if !st.Disabled {
		t.Error("the in-test guard should mark the status as not-run")
	}
}

// release.yml publishes every release as a draft, so /releases/latest should
// never hand us one. If it ever does, an unpublished build must not go out and
// tell users to install something that is not there.
func TestAnUnpublishedReleaseNeverNotifiesAnyone(t *testing.T) {
	for _, body := range []string{
		`[{"tag_name":"v0.9.0","draft":true}]`,
		`[{"tag_name":"v0.9.0","prerelease":true}]`,
	} {
		t.Run(body, func(t *testing.T) {
			serve(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			})
			st, err := Check(context.Background(), "0.8.4")
			if err == nil {
				t.Fatal("want an error rather than a silent update prompt")
			}
			if st.Available || st.Latest != "" {
				t.Errorf("an unpublished release leaked into the status: %+v", st)
			}
		})
	}
}

// The bug this endpoint changed for, in the shape it actually shipped in.
//
// internal/capability publishes its pinned ffmpeg and Tesseract archives as
// releases of this same repository, so from 2026-08-21 GitHub's answer to
// "latest release" was tools-ffmpeg-n9.0.1. Its tag parses as no version,
// Newer() answered false the way it must for anything unreadable, and every
// Aetox on every version was told it was up to date — measured on a v0.9.6
// install with v1.0.0 through v1.4.0 published above it.
//
// The list below is that repository, in GitHub's own order.
func TestAToolArchiveIsNotAVersionOfThisApp(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.5.0","draft":true},
			{"tag_name":"tools-ffmpeg-n9.0.1","html_url":"https://example.invalid/ff"},
			{"tag_name":"tools-tesseract-5.4.0.20240606","html_url":"https://example.invalid/ts"},
			{"tag_name":"v1.4.0","html_url":"https://example.invalid/r/v1.4.0"},
			{"tag_name":"v1.2.4","html_url":"https://example.invalid/r/v1.2.4"}
		]`))
	})

	st, err := Check(context.Background(), "0.9.6")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !st.Available {
		t.Fatal("v1.4.0 is published and newer than 0.9.6 — the check must say so")
	}
	if st.Latest != "1.4.0" {
		t.Errorf("Latest = %q, want 1.4.0: a tool archive outranked the app, or a draft did", st.Latest)
	}
	if st.URL != "https://example.invalid/r/v1.4.0" {
		t.Errorf("URL = %q — the download link points at the wrong release", st.URL)
	}
}

// Position in the list is creation order, and a patch cut after a minor is an
// ordinary thing this project has already done: v1.2.4 was published after
// v1.2.0. Picking the first match rather than the highest version would pin
// the whole install base to whichever release happened to be typed last.
func TestTheNewestVersionWinsNotTheNewestEntry(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.2.4","html_url":"https://example.invalid/r/v1.2.4"},
			{"tag_name":"v1.4.0","html_url":"https://example.invalid/r/v1.4.0"}
		]`))
	})

	st, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if st.Latest != "1.4.0" {
		t.Errorf("Latest = %q, want 1.4.0", st.Latest)
	}
}

func TestGarbageFromGitHubIsAnErrorNotAPanic(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><title>captive portal</title>`))
	})
	st, err := Check(context.Background(), "0.8.4")
	if err == nil {
		t.Fatal("want a decode error")
	}
	if st.Available {
		t.Error("undecodable body must not produce an update prompt")
	}
}

// A 304 says "same as the ETag you sent" — if we have no cached answer to go
// with that ETag, we have nothing to report and must say so rather than
// silently rendering an empty version.
func TestA304WithNothingCachedIsAnError(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	if _, err := Check(context.Background(), "0.8.4"); err == nil {
		t.Fatal("want an error when a 304 arrives with an empty cache")
	}
}

// The claim in loadCache's comment, made testable: a cache file that cannot be
// parsed costs one un-conditioned request and is rewritten, not surfaced.
func TestACorruptCacheIsRewrittenRatherThanFatal(t *testing.T) {
	var sentETag string
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		sentETag = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"fresh"`)
		_, _ = w.Write([]byte(`[{"tag_name":"v0.9.0"}]`))
	})
	path := filepath.Join(os.Getenv("AETOX_DATA_ROOT"), "update-check.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := Check(context.Background(), "0.8.4")
	if err != nil {
		t.Fatalf("a corrupt cache should not fail the check: %v", err)
	}
	if sentETag != "" {
		t.Errorf("sent If-None-Match %q out of an unreadable cache", sentETag)
	}
	if !st.Available {
		t.Error("the check itself should still have worked")
	}
	if got := loadCache().ETag; got != `W/"fresh"` {
		t.Errorf("cache not rewritten: ETag = %q", got)
	}
}

// Failing open means the *previous* good answer survives a bad check. Without
// this, one rate-limited call would throw away the ETag and make the next call
// unconditioned — the opposite of what the cache is for.
func TestAFailedCheckKeepsTheLastGoodAnswer(t *testing.T) {
	fail := false
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "rate limited", http.StatusForbidden)
			return
		}
		w.Header().Set("ETag", `W/"keep-me"`)
		_, _ = w.Write([]byte(`[{"tag_name":"v0.9.0"}]`))
	})

	if _, err := Check(context.Background(), "0.8.4"); err != nil {
		t.Fatalf("first Check: %v", err)
	}
	before := LastCheckedAt()

	fail = true
	if _, err := Check(context.Background(), "0.8.4"); err == nil {
		t.Fatal("second Check should have failed")
	}
	if got := loadCache().ETag; got != `W/"keep-me"` {
		t.Errorf("a failed check destroyed the cached ETag: %q", got)
	}
	if !LastCheckedAt().Equal(before) {
		t.Error("a failed check moved the last-checked timestamp")
	}
}

func TestCheckGivesUpWhenTheCallerCancels(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a cancelled context should never reach the server")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	st, err := Check(ctx, "0.8.4")
	if err == nil {
		t.Fatal("want an error from a cancelled context")
	}
	if st.Available {
		t.Error("a cancelled check must not claim an update")
	}
}

func TestLastCheckedAtIsZeroBeforeTheFirstCheck(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if got := LastCheckedAt(); !got.IsZero() {
		t.Errorf("LastCheckedAt = %v on a machine that has never checked", got)
	}
}
