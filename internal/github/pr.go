package github

// Pull requests, as data.
//
// This file fetches and returns structs. It renders nothing, because two
// different readers need the same facts in different shapes: the model's `pr`
// tool (internal/skill/pr_pack.go) turns them into sentences, and the workbench
// room turns them into rows. One fetcher, two renderers — the other way round
// is how two surfaces start disagreeing about the same pull request.
//
// It lives here rather than beside either reader for the reason this package
// exists: it is the one that owns the GitHub account (auth.go), and everything
// below needs the token.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// prReadLimit bounds one API response. A pull request's file list on a large
// change is the biggest thing here and is still far under this.
const prReadLimit = 8 << 20

// Repo is a repository on GitHub, by the only two things that identify one.
type Repo struct {
	Owner string
	Name  string
}

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// URL is where a person would go to look at it.
func (r Repo) URL() string { return "https://github.com/" + r.Owner + "/" + r.Name }

// PullRequest is one pull request, flattened to what either reader needs.
type PullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	State          string `json:"state"`
	Draft          bool   `json:"draft"`
	Body           string `json:"body"`
	Author         string `json:"author"`
	HeadRef        string `json:"headRef"`
	HeadSHA        string `json:"headSHA"`
	BaseRef        string `json:"baseRef"`
	Mergeable      *bool  `json:"mergeable"`
	MergeableState string `json:"mergeableState"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	ChangedFiles   int    `json:"changedFiles"`
	URL            string `json:"url"`
}

// PRFile is one file a pull request touches. Patch is GitHub's own unified
// diff for it, which is the format this project already draws everywhere
// (internal/skill/hunk.go, CodeDiff.svelte) — so a pull request's hunks need no
// second renderer.
//
// It is empty for a file GitHub judged too large to diff, and for a binary. A
// reader must say so rather than drawing an empty box: "no diff" and "no
// changes" look identical and are not.
type PRFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

// CheckRun is one CI run against a commit.
type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// Done reports whether the run has finished. A run still going is neither a
// pass nor a failure, and collapsing the three into two is how a red badge
// appears over a suite that has not started yet.
func (c CheckRun) Done() bool { return c.Status == "completed" }

// Passed reports a conclusion nobody needs to act on. Neutral and skipped are
// counted as passes deliberately: a check that decided it had nothing to say is
// not a problem to go and look at.
func (c CheckRun) Passed() bool {
	return c.Done() && (c.Conclusion == "success" || c.Conclusion == "neutral" || c.Conclusion == "skipped")
}

// NewPR is what opening one takes.
type NewPR struct {
	Title string
	Head  string
	Base  string
	Body  string
	Draft bool
}

// PRClient talks to the API. The zero value works and points at github.com.
type PRClient struct {
	// BaseURL overrides the API root, so a test can point at a stub.
	BaseURL string
	// HTTP overrides the client, for the same reason.
	HTTP *http.Client
}

func (c *PRClient) base() string {
	if c != nil && strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimSuffix(c.BaseURL, "/")
	}
	return apiBase
}

func (c *PRClient) client() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// ErrNoAccount is what every call here answers when there is no token. Named
// rather than left to a 401, because "connect an account" and "GitHub refused
// this token" send the reader to two different places.
var ErrNoAccount = errors.New("no GitHub account is connected — Settings → การเชื่อมต่อ, or set GITHUB_TOKEN")

func (c *PRClient) List(ctx context.Context, repo Repo, state string, limit int) ([]PullRequest, error) {
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "closed" && state != "all" {
		state = "open"
	}
	if limit <= 0 {
		limit = 20
	}
	body, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=%d", repo.Owner, repo.Name, state, limit))
	if err != nil {
		return nil, err
	}
	var raw []prPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("cannot read the pull request list: %w", err)
	}
	out := make([]PullRequest, 0, len(raw))
	for _, p := range raw {
		out = append(out, p.flatten())
	}
	return out, nil
}

func (c *PRClient) Get(ctx context.Context, repo Repo, number int) (PullRequest, error) {
	if number <= 0 {
		return PullRequest{}, errors.New("number is required")
	}
	body, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d", repo.Owner, repo.Name, number))
	if err != nil {
		return PullRequest{}, err
	}
	var raw prPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return PullRequest{}, fmt.Errorf("cannot read pull request %d: %w", number, err)
	}
	return raw.flatten(), nil
}

func (c *PRClient) Files(ctx context.Context, repo Repo, number, limit int) ([]PRFile, error) {
	if limit <= 0 {
		limit = 40
	}
	body, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=%d", repo.Owner, repo.Name, number, limit))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Patch     string `json:"patch"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("cannot read the file list: %w", err)
	}
	out := make([]PRFile, 0, len(raw))
	for _, f := range raw {
		out = append(out, PRFile{
			Path: f.Filename, Status: f.Status,
			Additions: f.Additions, Deletions: f.Deletions, Patch: f.Patch,
		})
	}
	return out, nil
}

// Checks reports the runs against one commit. It answers with no error for a
// commit that has none, because "no checks" is an ordinary state and not a
// failure to report.
func (c *PRClient) Checks(ctx context.Context, repo Repo, sha string) ([]CheckRun, error) {
	if strings.TrimSpace(sha) == "" {
		return nil, nil
	}
	body, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", repo.Owner, repo.Name, sha))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Runs []CheckRun `json:"check_runs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("cannot read the checks: %w", err)
	}
	return payload.Runs, nil
}

func (c *PRClient) DefaultBranch(ctx context.Context, repo Repo) (string, error) {
	body, err := c.get(ctx, fmt.Sprintf("/repos/%s/%s", repo.Owner, repo.Name))
	if err != nil {
		return "", err
	}
	var payload struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.DefaultBranch) == "" {
		return "", errors.New("cannot tell which branch to merge into — pass base")
	}
	return payload.DefaultBranch, nil
}

func (c *PRClient) Create(ctx context.Context, repo Repo, pr NewPR) (PullRequest, error) {
	if strings.TrimSpace(pr.Title) == "" || strings.TrimSpace(pr.Head) == "" {
		return PullRequest{}, errors.New("title and head are required")
	}
	payload := map[string]any{"title": pr.Title, "head": pr.Head}
	if body := strings.TrimSpace(pr.Body); body != "" {
		payload["body"] = body
	}
	if pr.Draft {
		payload["draft"] = true
	}
	base := strings.TrimSpace(pr.Base)
	if base == "" {
		var err error
		if base, err = c.DefaultBranch(ctx, repo); err != nil {
			return PullRequest{}, err
		}
	}
	payload["base"] = base

	body, status, err := c.send(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", repo.Owner, repo.Name), payload)
	if err != nil {
		return PullRequest{}, err
	}
	if status < 200 || status > 299 {
		return PullRequest{}, apiError("open a pull request", status, body)
	}
	var raw prPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return PullRequest{}, fmt.Errorf("the pull request was created but its reply could not be read: %w", err)
	}
	created := raw.flatten()
	created.BaseRef = base
	return created, nil
}

// Comment adds a comment and answers with its URL.
//
// The issues endpoint, not the pulls one: a pull request IS an issue for
// commenting, and /pulls/{n}/comments is the line-by-line review kind, which
// needs a file and a position this does not take.
func (c *PRClient) Comment(ctx context.Context, repo Repo, number int, text string) (string, error) {
	text = strings.TrimSpace(text)
	if number <= 0 || text == "" {
		return "", errors.New("number and body are required")
	}
	body, status, err := c.send(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", repo.Owner, repo.Name, number),
		map[string]any{"body": text})
	if err != nil {
		return "", err
	}
	if status < 200 || status > 299 {
		return "", apiError("comment", status, body)
	}
	var created struct {
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(body, &created)
	return created.HTMLURL, nil
}

// --- repository identity -------------------------------------------------

// reRepoURL reads a whole argument as a repository. Distinct from
// internal/skill's ExtractGitHubRepoURL, which hunts a URL out of prose — this
// one is handed a value somebody typed as a repository and says whether it is.
var reRepoURL = regexp.MustCompile(`^(?:https?://)?(?:www\.)?github\.com/([^/\s]+)/([^/\s?#]+)`)

// ParseRepo reads the three spellings a person or a config file actually uses.
func ParseRepo(raw string) (Repo, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Repo{}, false
	}
	if rest, ok := strings.CutPrefix(raw, "git@github.com:"); ok {
		raw = "https://github.com/" + rest
	}
	if match := reRepoURL.FindStringSubmatch(raw); len(match) == 3 {
		return Repo{Owner: match[1], Name: trimRepoName(match[2])}, true
	}
	owner, name, ok := strings.Cut(raw, "/")
	if !ok || owner == "" || name == "" || strings.ContainsAny(name, "/ \t") {
		return Repo{}, false
	}
	return Repo{Owner: owner, Name: trimRepoName(name)}, true
}

func trimRepoName(raw string) string {
	name := strings.TrimSuffix(strings.TrimSpace(raw), ".git")
	return strings.TrimRight(name, ".,);:!?]'\"")
}

// reOriginURL finds the origin remote's url in a git config.
var reOriginURL = regexp.MustCompile(`(?ms)^\[remote "origin"\][^\[]*?^\s*url\s*=\s*(\S+)`)

// OriginRepo answers "which GitHub repository is this folder", by reading the
// origin remote out of .git/config.
//
// The file rather than `git remote get-url`, because this is one line of a
// config file and spawning a process to read it would put a shell in the path
// of two callers that have no other reason to need one.
func OriginRepo(root string) (Repo, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		return Repo{}, errors.New("this folder has no git remote to read — pass repo as owner/name")
	}
	match := reOriginURL.FindSubmatch(raw)
	if len(match) < 2 {
		return Repo{}, errors.New("this repository has no origin remote — pass repo as owner/name")
	}
	url := strings.TrimSpace(string(match[1]))
	repo, ok := ParseRepo(url)
	if !ok {
		return Repo{}, fmt.Errorf("this project's origin is %q, which is not a github.com repository", url)
	}
	return repo, nil
}

// --- plumbing ------------------------------------------------------------

type prPayload struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Body   string `json:"body"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Mergeable      *bool  `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	ChangedFiles   int    `json:"changed_files"`
	HTMLURL        string `json:"html_url"`
}

func (p prPayload) flatten() PullRequest {
	return PullRequest{
		Number: p.Number, Title: p.Title, State: p.State, Draft: p.Draft, Body: p.Body,
		Author:  p.User.Login,
		HeadRef: p.Head.Ref, HeadSHA: p.Head.SHA, BaseRef: p.Base.Ref,
		Mergeable: p.Mergeable, MergeableState: p.MergeableState,
		Additions: p.Additions, Deletions: p.Deletions, ChangedFiles: p.ChangedFiles,
		URL: p.HTMLURL,
	}
}

func (c *PRClient) get(ctx context.Context, endpoint string) ([]byte, error) {
	body, status, err := c.send(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, apiError("read from GitHub", status, body)
	}
	return body, nil
}

func (c *PRClient) send(ctx context.Context, method, endpoint string, payload any) ([]byte, int, error) {
	token := Token()
	if token == "" {
		return nil, 0, ErrNoAccount
	}
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Aetox-CLI")
	req.Header.Set("Accept", "application/vnd.github+json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, prReadLimit))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// apiError turns a refusal into a sentence somebody can act on. GitHub says why
// in the body far more often than the status alone does — "head branch not
// found" and "a pull request already exists" both arrive as 422.
func apiError(what string, status int, body []byte) error {
	var payload struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &payload)
	detail := strings.TrimSpace(payload.Message)
	for _, e := range payload.Errors {
		if msg := strings.TrimSpace(e.Message); msg != "" {
			detail += " — " + msg
		} else if e.Field != "" {
			detail += " — " + e.Field + " " + e.Code
		}
	}
	if detail == "" {
		detail = "no reason given"
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("GitHub refused to %s (%d): %s — the connected token may not have `repo` scope on this repository",
			what, status, detail)
	case http.StatusNotFound:
		return fmt.Errorf("could not %s (404): %s — check the repository name, and that the connected token can see it if it is private",
			what, detail)
	}
	return fmt.Errorf("could not %s (%d): %s", what, status, detail)
}

// ShortSHA is the seven characters a person reads a commit by.
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// PRNumber reads a pull request number out of whatever a caller had.
func PRNumber(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#")))
	if err != nil {
		return 0
	}
	return n
}
