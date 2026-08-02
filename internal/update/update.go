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

	"github.com/Mike0165115321/Aetox/internal/config"
)

const (
	repoOwner = "Mike0165115321"
	repoName  = "Aetox"

	// ReleasesPage is the fallback every channel we cannot upgrade for gets
	// sent to, and the "what changed" link on all of them.
	ReleasesPage = "https://github.com/" + repoOwner + "/" + repoName + "/releases"

	// DisableEnv turns the check off entirely, for people who do not want the
	// app talking to github.com at all and for anything running in CI. Named
	// after opencode's OPENCODE_DISABLE_AUTOUPDATE, which exists for the same
	// two reasons.
	DisableEnv = "AETOX_DISABLE_UPDATE_CHECK"

	defaultTimeout = 10 * time.Second
)

// defaultAPIURL is the release GitHub itself considers current — which
// excludes drafts and prereleases, so an unpublished draft release (release.yml
// creates every release as a draft) can never notify anybody by accident.
const defaultAPIURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"

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
	st := Status{
		Current: current,
		Channel: string(Detect()),
		URL:     ReleasesPage,
	}
	st.Hint = UpgradeHint(Channel(st.Channel))

	if Disabled() {
		st.Disabled = true
		return st, ErrDisabled
	}
	// A test in some unrelated package must never reach out to github.com just
	// because it constructed an App. internal/rtk/install.go learned this the
	// hard way on CI's first green-field run. This package's own tests replace
	// apiURL, so they are unaffected.
	if testing.Testing() && apiURL == defaultAPIURL {
		st.Disabled = true
		return st, ErrDisabled
	}

	prev := loadCache()

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return st, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// GitHub rejects unauthenticated calls with no User-Agent outright.
	req.Header.Set("User-Agent", "Aetox/"+current)
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	next := prev

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Nothing changed since last time; the cached answer is still the answer.
		if prev.Latest == "" {
			return st, fmt.Errorf("github returned 304 with no cached release")
		}
	case http.StatusOK:
		var rel struct {
			TagName string `json:"tag_name"`
			HTMLURL string `json:"html_url"`
			Draft   bool   `json:"draft"`
			Pre     bool   `json:"prerelease"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return st, fmt.Errorf("decode release: %w", err)
		}
		if rel.Draft || rel.Pre {
			// /releases/latest should never return these, but if the API ever
			// does, a draft is by definition not something to notify about.
			return st, fmt.Errorf("latest release is not published")
		}
		next.ETag = resp.Header.Get("ETag")
		next.Latest = strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
		if rel.HTMLURL != "" {
			next.URL = rel.HTMLURL
		}
	default:
		return st, fmt.Errorf("github returned %s", resp.Status)
	}

	next.CheckedAt = now
	saveCache(next)

	st.Latest = next.Latest
	st.CheckedAt = next.CheckedAt
	if next.URL != "" {
		st.URL = next.URL
	}
	st.Available = Newer(next.Latest, current)
	return st, nil
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
	ETag      string `json:"etag,omitempty"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
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
