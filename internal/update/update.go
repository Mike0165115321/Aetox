// Package update answers one question — "is there a newer Aetox than this
// one?" — and then hands the answer to whoever is allowed to act on it.
//
// It never writes to the installed application. See channel.go for why that is
// the design and not a limitation.
//
// Everything here fails open. Offline, rate-limited, behind a proxy that eats
// the response, GitHub having a bad day: the check reports an error, the UI
// says it could not check, and the app carries on exactly as it did before.
// That is the same discipline internal/rtk/install.go already applies to its
// own download path — an optional convenience must never become a way for the
// app to break.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
)

const (
	repoOwner = "Mikedev115"
	repoName  = "Aetox"

	// ReleasesPage is the fallback every channel we cannot upgrade for gets
	// sent to, and the "what changed" link on all of them.
	ReleasesPage = "https://github.com/" + repoOwner + "/" + repoName + "/releases"

	// StorePage is where a ChannelStore install is sent instead. 9N4KKBRRSCZZ
	// is the Store ID Partner Center assigned to AetoxAI.Aetox; it is the same
	// identity desktop/build/windows/msix/AppxManifest.xml declares, and the
	// two must be bumped together if the listing is ever recreated.
	//
	// Sending a Store user to the GitHub releases page would be worse than
	// saying nothing: the files there install a SECOND copy beside the one they
	// already have, from a channel that then fights the Store over which is
	// current.
	StorePage = "https://apps.microsoft.com/detail/9N4KKBRRSCZZ"

	// DisableEnv turns the check off entirely, for people who do not want the
	// app talking to github.com at all and for anything running in CI. Named
	// after opencode's OPENCODE_DISABLE_AUTOUPDATE, which exists for the same
	// two reasons.
	DisableEnv = "AETOX_DISABLE_UPDATE_CHECK"

	defaultTimeout = 10 * time.Second
)

// defaultAPIURL is the release LIST, newest first, and it used to be
// /releases/latest — which broke every install in the world on 2026-08-21.
//
// /releases/latest answers "the newest published release in this repository",
// and this app is not the only thing this repository releases: the ffmpeg and
// Tesseract archives internal/capability downloads are published here too, as
// tools-ffmpeg-n9.0.1 and tools-tesseract-5.4.0.20240606. From the moment the
// first of those went out, GitHub's answer to "latest" was a tool archive, its
// tag parsed as no version at all, Newer() answered false the way it is
// supposed to for anything it cannot read — and every Aetox on earth, on every
// version, said "นี่คือเวอร์ชันล่าสุดแล้ว". Measured on the owner's v0.9.6
// install with four app releases published above it.
//
// So the question is asked properly now: give me the releases, and I will pick
// the newest one that is an Aetox. GitHub omits drafts for an unauthenticated
// caller, so release.yml's draft-first flow still cannot notify anybody early,
// and appTagIsNewer refuses anything that is not a v-prefixed version, so a
// repository can publish whatever else it likes without ever being mistaken
// for the app again.
//
// per_page=30 is GitHub's default and roughly a year of releases at this
// project's pace; the tools archives are counted in it, which is the reason to
// state the number rather than leave it implied.
const defaultAPIURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases?per_page=30"

// apiURL is a var so this package's own tests can point it at an httptest
// server. Nothing else should assign to it.
var apiURL = defaultAPIURL

// ErrDisabled means no check was attempted, which is different from a check
// that ran and failed — the UI says "off", not "could not reach GitHub".
var ErrDisabled = errors.New("update check disabled")

// Status is one check's result, shaped for the Settings page that displays it.
//
// Disabled means the check never ran — the user switched it off, or a test
// did. It is its own field rather than something the UI infers from an error
// string: "the check is switched off" and "the check ran and failed" are two
// different sentences to show the user, and matching on error text to tell
// them apart is how that breaks the first time the wording changes.
type Status struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"` // "" when the check did not complete
	Available bool   `json:"available"`
	Disabled  bool   `json:"disabled"`
	Channel   string `json:"channel"`
	Hint      string `json:"hint"` // e.g. "scoop update aetox"; "" if none
	URL       string `json:"url"`  // the release page to open
	CheckedAt string `json:"checkedAt"`
	// PublishedAt is when the release went out (RFC3339, "" if GitHub did not
	// say). Shown beside the version, because "v0.9.7" alone says nothing about
	// whether this is a week old or an hour old — and that is most of what
	// somebody deciding whether to restart now actually wants to know.
	PublishedAt string `json:"publishedAt"`
	// CanAuto means Apply can take it from here: this channel knows how to
	// download, verify and swap itself, and the release actually carries the
	// file this install would need. The UI's one-click restart-and-update
	// button exists exactly when this is true.
	CanAuto bool `json:"canAuto"`
}

// Asset is one downloadable file on a release, as much of it as Apply needs.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// Disabled reports whether the user (or CI) has switched the check off.
func Disabled() bool {
	v := strings.TrimSpace(os.Getenv(DisableEnv))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// Check asks GitHub for the newest published release and compares it against
// current. A cached ETag makes the repeat call a 304 with no body, which is
// what keeps a once-a-day check off GitHub's 60-requests-per-hour budget for
// unauthenticated callers.
func Check(ctx context.Context, current string) (Status, error) {
	st, _, err := checkWithAssets(ctx, current)
	return st, err
}

// checkWithAssets is Check plus the release's downloadable files — which stay
// internal because only Apply consumes them; the Settings page has no use for
// a URL list it must not build its own download logic on.
func checkWithAssets(ctx context.Context, current string) (Status, []Asset, error) {
	return checkOn(ctx, current, Detect())
}

// checkOn is checkWithAssets with the channel handed in rather than detected.
// Same reason detectFrom takes its OS as a parameter: the rule worth testing
// here is "a Store install never reaches github.com", and no test can make the
// machine it runs on into a Store install.
func checkOn(ctx context.Context, current string, ch Channel) (Status, []Asset, error) {
	st := Status{
		Current: current,
		Channel: string(ch),
		URL:     ReleasesPage,
	}
	st.Hint = UpgradeHint(Channel(st.Channel))

	// Before Disabled(), because this one is not a preference anyone set and
	// cannot be switched back on: a packaged app does not update itself, and
	// Microsoft Store certification asks for exactly this. Windows has already
	// installed the newer package by the time we would have noticed it exists.
	if Channel(st.Channel) == ChannelStore {
		st.Disabled = true
		st.URL = StorePage
		return st, nil, ErrDisabled
	}

	if Disabled() {
		st.Disabled = true
		return st, nil, ErrDisabled
	}
	// A test in some unrelated package must never reach out to github.com just
	// because it constructed an App. internal/rtk/install.go learned this the
	// hard way on CI's first green-field run. This package's own tests replace
	// apiURL, so they are unaffected.
	if testing.Testing() && apiURL == defaultAPIURL {
		st.Disabled = true
		return st, nil, ErrDisabled
	}

	prev := loadCache()

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return st, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// GitHub rejects unauthenticated calls with no User-Agent outright.
	req.Header.Set("User-Agent", "Aetox/"+current)
	// The ETag is only worth sending when the cache could actually answer a
	// 304 — which now includes the asset list. A cache written by a build from
	// before assets were recorded would otherwise answer every 304 with an
	// empty list, and the one-click button would stay dark until the NEXT
	// release changed the ETag. One full-priced request repairs it instead.
	if prev.ETag != "" && len(prev.Assets) > 0 {
		req.Header.Set("If-None-Match", prev.ETag)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return st, nil, err
	}
	defer resp.Body.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	next := prev

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Nothing changed since last time; the cached answer is still the answer.
		if prev.Latest == "" {
			return st, nil, fmt.Errorf("github returned 304 with no cached release")
		}
	case http.StatusOK:
		type ghRelease struct {
			TagName     string `json:"tag_name"`
			HTMLURL     string `json:"html_url"`
			PublishedAt string `json:"published_at"`
			Draft       bool   `json:"draft"`
			Pre         bool   `json:"prerelease"`
			Assets      []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
				Size int64  `json:"size"`
			} `json:"assets"`
		}
		var rels []ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
			return st, nil, fmt.Errorf("decode releases: %w", err)
		}
		// Newest Aetox in the list, by version rather than by position: GitHub
		// orders by creation date, and a patch cut after a minor — v1.2.4 was —
		// would otherwise outrank it forever.
		var rel ghRelease
		found := false
		for _, r := range rels {
			// A draft is not something to notify about, and a prerelease is not
			// something to notify about by default. Neither should reach an
			// unauthenticated caller; both are refused here anyway, because the
			// cost of the check is nothing and the cost of being wrong is the
			// whole install base being told to upgrade to a draft.
			if r.Draft || r.Pre || !isAppTag(r.TagName) {
				continue
			}
			if !found || Newer(r.TagName, rel.TagName) {
				rel, found = r, true
			}
		}
		if !found {
			return st, nil, fmt.Errorf("no published Aetox release among the newest %d", len(rels))
		}
		next.ETag = resp.Header.Get("ETag")
		next.Latest = strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
		next.PublishedAt = rel.PublishedAt
		if rel.HTMLURL != "" {
			next.URL = rel.HTMLURL
		}
		// The files ride the cache with the answer they belong to, so a 304
		// still knows what the release carries — Apply after a cached check
		// must not need a second uncached round-trip.
		next.Assets = nil
		for _, a := range rel.Assets {
			next.Assets = append(next.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
		}
	default:
		return st, nil, fmt.Errorf("github returned %s", resp.Status)
	}

	next.CheckedAt = now
	saveCache(next)

	st.Latest = next.Latest
	st.CheckedAt = next.CheckedAt
	st.PublishedAt = next.PublishedAt
	if next.URL != "" {
		st.URL = next.URL
	}
	st.Available = Newer(next.Latest, current)
	st.CanAuto = st.Available && canAuto(Channel(st.Channel), next.Assets)
	return st, next.Assets, nil
}

// isAppTag reports whether a release tag names a version of this application.
//
// Two conditions, and both are load-bearing. The "v" prefix is what
// release.yml is triggered by and therefore the only mark an Aetox release
// carries that nothing else in the repository does; parsing is what keeps a
// hand-made "vNext" or "v2-beta" out. tools-ffmpeg-n9.0.1 fails the first
// test, and would fail the second too — that it fails both is the point, since
// the next thing published here will not be an ffmpeg archive and the rule has
// to hold for whatever it turns out to be.
func isAppTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if !strings.HasPrefix(tag, "v") {
		return false
	}
	_, ok := parse(tag)
	return ok
}

// Newer reports whether latest is a strictly newer release than current.
//
// Compares the numeric parts, never the strings: "0.8.10" is newer than
// "0.8.9" and a string comparison gets that exactly backwards. Anything it
// cannot parse returns false — the failure that shows no banner is much
// cheaper than the one that nags about an update that does not exist.
func Newer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	// Drop any prerelease/build suffix — "0.9.0-rc1" compares as 0.9.0.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return out, false
	}
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// cacheEntry is what survives between checks: an ETag so the next call costs
// nothing, and the last answer so a 304 still has something to report.
type cacheEntry struct {
	ETag        string  `json:"etag,omitempty"`
	Latest      string  `json:"latest,omitempty"`
	URL         string  `json:"url,omitempty"`
	CheckedAt   string  `json:"checked_at,omitempty"`
	PublishedAt string  `json:"published_at,omitempty"`
	Assets      []Asset `json:"assets,omitempty"`
}

func cachePath() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "update-check.json"), nil
}

// LastCheckedAt is the timestamp of the most recent completed check, or the
// zero time if there has never been one. Used to decide whether an automatic
// check is due.
func LastCheckedAt() time.Time {
	t, err := time.Parse(time.RFC3339, loadCache().CheckedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func loadCache() cacheEntry {
	var c cacheEntry
	path, err := cachePath()
	if err != nil {
		return c
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	// A corrupt cache is not an error worth surfacing: the next check just
	// costs one un-conditioned request and rewrites it.
	_ = json.Unmarshal(b, &c)
	return c
}

func saveCache(c cacheEntry) {
	path, err := cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}
