package skill

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubGitHub is the API as these tests need it: routes keyed by method+path,
// so a test says what it is answering and nothing else, and an unexpected call
// fails loudly rather than falling through to a zero value.
type stubGitHub struct {
	t       *testing.T
	routes  map[string]func(body []byte) (int, string)
	seen    map[string][]byte
	server  *httptest.Server
	unknown []string
}

func newStubGitHub(t *testing.T) *stubGitHub {
	t.Helper()
	s := &stubGitHub{t: t, routes: map[string]func([]byte) (int, string){}, seen: map[string][]byte{}}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		s.seen[key] = body
		handler, ok := s.routes[key]
		if !ok {
			s.unknown = append(s.unknown, key+"?"+r.URL.RawQuery)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no stub for this route"}`))
			return
		}
		status, payload := handler(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *stubGitHub) on(methodAndPath string, status int, payload string) {
	s.routes[methodAndPath] = func([]byte) (int, string) { return status, payload }
}

// prTool builds the tool pointed at the stub, with a token in the environment
// so the account check passes.
func prTool(t *testing.T, stub *stubGitHub, root string) *prSkill {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &prSkill{root: root, apiBaseURL: stub.server.URL, httpClient: stub.server.Client()}
}

func prCall(t *testing.T, s *prSkill, args map[string]any) Output {
	t.Helper()
	out, err := s.ExecuteTool(context.Background(), args)
	if err != nil {
		t.Fatalf("pr %v: %v", args["action"], err)
	}
	return out
}

// A repository with an origin remote answers "which repo" by itself. Both
// spellings a .git/config actually holds are read.
func TestPRResolvesTheProjectsOwnRepo(t *testing.T) {
	for _, tc := range []struct {
		name, url, want string
	}{
		{"https", "https://github.com/Mikedev115/Aetox.git", "Mikedev115/Aetox"},
		{"ssh", "git@github.com:Mikedev115/Aetox.git", "Mikedev115/Aetox"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeGitConfig(t, root, tc.url)
			target, err := (&prSkill{root: root}).repoTarget("")
			if err != nil {
				t.Fatalf("repoTarget: %v", err)
			}
			if got := target.Owner + "/" + target.Repo; got != tc.want {
				t.Errorf("repoTarget = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPRRepoArgumentBeatsTheRemote(t *testing.T) {
	root := t.TempDir()
	writeGitConfig(t, root, "https://github.com/Mikedev115/Aetox.git")
	s := &prSkill{root: root}

	for _, raw := range []string{"golang/example", "https://github.com/golang/example"} {
		target, err := s.repoTarget(raw)
		if err != nil {
			t.Fatalf("repoTarget(%q): %v", raw, err)
		}
		if got := target.Owner + "/" + target.Repo; got != "golang/example" {
			t.Errorf("repoTarget(%q) = %q, want golang/example", raw, got)
		}
	}
}

// A folder with no remote says so in words that name the way out, rather than
// failing later against a URL built from nothing.
func TestPRWithoutARemoteSaysWhatToPass(t *testing.T) {
	_, err := (&prSkill{root: t.TempDir()}).repoTarget("")
	if err == nil {
		t.Fatal("a folder with no git config resolved a repository")
	}
	if !strings.Contains(err.Error(), "owner/name") {
		t.Errorf("error = %q, want it to name the argument to pass", err)
	}
}

// Every act needs an account, and saying so beats a 401 the reader would go
// looking for a bug behind.
func TestPRWithoutAnAccountRefusesByName(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	s := &prSkill{root: t.TempDir()}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"action": "list"})
	if err == nil {
		t.Fatal("pr list ran with no account connected")
	}
	if out.Success {
		t.Error("Success = true on a refusal")
	}
	if !strings.Contains(err.Error(), "GitHub account") {
		t.Errorf("error = %q, want it to name the missing account", err)
	}
}

func TestPRListRendersOpenPullRequests(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("GET /repos/golang/example/pulls", 200, `[
		{"number":7,"title":"Fix the parser","state":"open","draft":false,
		 "user":{"login":"mike"},"head":{"ref":"fix-parser"},"base":{"ref":"main"}},
		{"number":8,"title":"Docs","state":"open","draft":true,
		 "user":{"login":"mike"},"head":{"ref":"docs"},"base":{"ref":"main"}}]`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{"action": "list", "repo": "golang/example"})
	for _, want := range []string{"#7 Fix the parser", "fix-parser → main", "#8 Docs (draft)"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("list is missing %q:\n%s", want, out.Content)
		}
	}
}

func TestPRListSaysSoWhenThereAreNone(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("GET /repos/golang/example/pulls", 200, `[]`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{"action": "list", "repo": "golang/example"})
	if !strings.Contains(out.Content, "no open pull requests") {
		t.Errorf("empty list said %q", out.Content)
	}
}

// read is one call for the whole picture, and the picture has to include the
// parts a reader would otherwise need two more calls for.
func TestPRReadCarriesStateFilesAndChecks(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("GET /repos/golang/example/pulls/7", 200, `{
		"number":7,"title":"Fix the parser","state":"open","draft":false,
		"user":{"login":"mike"},"head":{"ref":"fix-parser","sha":"abc1234def"},
		"base":{"ref":"main"},"mergeable":false,"mergeable_state":"dirty",
		"additions":12,"deletions":3,"changed_files":2,
		"body":"why this change exists",
		"html_url":"https://github.com/golang/example/pull/7"}`)
	stub.on("GET /repos/golang/example/pulls/7/files", 200, `[
		{"filename":"parser.go","status":"modified","additions":10,"deletions":3},
		{"filename":"parser_test.go","status":"added","additions":2,"deletions":0}]`)
	stub.on("GET /repos/golang/example/commits/abc1234def/check-runs", 200, `{
		"total_count":3,"check_runs":[
		{"name":"build","status":"completed","conclusion":"success"},
		{"name":"test","status":"completed","conclusion":"failure"},
		{"name":"lint","status":"in_progress","conclusion":""}]}`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{"action": "read", "repo": "golang/example", "number": 7})
	for _, want := range []string{
		"#7 Fix the parser",
		"+12 -3 across 2 files",
		"does NOT merge cleanly (dirty)",
		"why this change exists",
		"modified parser.go +10 -3",
		"FAILED: test (failure)",
		"1 still running",
		"1 passed",
		"https://github.com/golang/example/pull/7",
	} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("read is missing %q:\n%s", want, out.Content)
		}
	}
}

// A pull request whose checks cannot be read is still worth reading: the
// failure must not take the rest of the answer with it.
func TestPRReadSurvivesChecksThatCannotBeRead(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("GET /repos/golang/example/pulls/7", 200, `{
		"number":7,"title":"Fix","state":"open","user":{"login":"mike"},
		"head":{"ref":"f","sha":"deadbeef"},"base":{"ref":"main"},
		"html_url":"https://github.com/golang/example/pull/7"}`)
	stub.on("GET /repos/golang/example/pulls/7/files", 500, `{"message":"boom"}`)
	stub.on("GET /repos/golang/example/commits/deadbeef/check-runs", 500, `{"message":"boom"}`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{"action": "read", "repo": "golang/example", "number": 7})
	if !strings.Contains(out.Content, "#7 Fix") {
		t.Errorf("read gave up entirely when a side call failed:\n%s", out.Content)
	}
	if strings.Contains(out.Content, "checks:") {
		t.Errorf("read invented a checks line from a failed call:\n%s", out.Content)
	}
}

func TestPRChecksNamesOnlyTheFailures(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("GET /repos/golang/example/pulls/7", 200,
		`{"number":7,"head":{"sha":"abcdef1234"},"base":{"ref":"main"},"state":"open"}`)
	stub.on("GET /repos/golang/example/commits/abcdef1234/check-runs", 200, `{
		"total_count":2,"check_runs":[
		{"name":"unit","status":"completed","conclusion":"success"},
		{"name":"e2e","status":"completed","conclusion":"timed_out"}]}`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{"action": "checks", "repo": "golang/example", "number": 7})
	if !strings.Contains(out.Content, "FAILED: e2e (timed_out)") {
		t.Errorf("checks did not name the failure:\n%s", out.Content)
	}
	if strings.Contains(out.Content, "unit") {
		t.Errorf("checks named a passing run; a reader needs which one to look at:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "abcdef1") {
		t.Errorf("checks did not say which commit it read:\n%s", out.Content)
	}
}

// create asks the repository which branch to merge into rather than assuming,
// and sends exactly what it was given.
func TestPRCreateFillsInTheBaseBranchAndPostsTheRest(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("GET /repos/golang/example", 200, `{"default_branch":"trunk"}`)
	stub.on("POST /repos/golang/example/pulls", 201, `{
		"number":9,"title":"Add the thing",
		"html_url":"https://github.com/golang/example/pull/9"}`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{
		"action": "create", "repo": "golang/example",
		"title": "Add the thing", "head": "feature", "body": "because", "draft": true,
	})
	if !strings.Contains(out.Content, "opened #9 Add the thing (feature → trunk)") {
		t.Errorf("create receipt = %q", out.Content)
	}

	var sent map[string]any
	if err := json.Unmarshal(stub.seen["POST /repos/golang/example/pulls"], &sent); err != nil {
		t.Fatalf("the POST body is not JSON: %v", err)
	}
	for key, want := range map[string]any{
		"title": "Add the thing", "head": "feature", "base": "trunk", "body": "because", "draft": true,
	} {
		if sent[key] != want {
			t.Errorf("POST %s = %v, want %v", key, sent[key], want)
		}
	}
}

func TestPRCreateKeepsAnExplicitBase(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("POST /repos/golang/example/pulls", 201, `{"number":9,"title":"x","html_url":"u"}`)
	s := prTool(t, stub, t.TempDir())

	prCall(t, s, map[string]any{
		"action": "create", "repo": "golang/example",
		"title": "x", "head": "feature", "base": "release",
	})
	var sent map[string]any
	_ = json.Unmarshal(stub.seen["POST /repos/golang/example/pulls"], &sent)
	if sent["base"] != "release" {
		t.Errorf("base = %v, want release — an explicit base must not be looked up", sent["base"])
	}
	if len(stub.unknown) > 0 {
		t.Errorf("an explicit base still cost a lookup: %v", stub.unknown)
	}
}

// GitHub says why in the body far more often than the status does. A refusal
// that drops it leaves the reader with "422" and nothing to act on.
func TestPRCreateCarriesGitHubsOwnReason(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("POST /repos/golang/example/pulls", 422, `{
		"message":"Validation Failed",
		"errors":[{"message":"No commits between main and feature"}]}`)
	s := prTool(t, stub, t.TempDir())

	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"action": "create", "repo": "golang/example", "title": "x", "head": "feature", "base": "main",
	})
	if err == nil {
		t.Fatal("a 422 came back as success")
	}
	if !strings.Contains(err.Error(), "No commits between main and feature") {
		t.Errorf("error = %q, want GitHub's own reason in it", err)
	}
}

func TestPRCreateNeedsATitleAndABranch(t *testing.T) {
	stub := newStubGitHub(t)
	s := prTool(t, stub, t.TempDir())
	_, err := s.ExecuteTool(context.Background(), map[string]any{
		"action": "create", "repo": "golang/example", "title": "only a title",
	})
	if err == nil {
		t.Fatal("create ran with no head branch")
	}
	if len(stub.seen) > 0 {
		t.Errorf("a call was made before the arguments were checked: %v", stub.seen)
	}
}

// A comment goes to the issues endpoint, not the pulls one: /pulls/{n}/comments
// is the line-by-line review kind and needs a file and a position.
func TestPRCommentPostsToTheIssuesEndpoint(t *testing.T) {
	stub := newStubGitHub(t)
	stub.on("POST /repos/golang/example/issues/7/comments", 201,
		`{"html_url":"https://github.com/golang/example/pull/7#issuecomment-1"}`)
	s := prTool(t, stub, t.TempDir())

	out := prCall(t, s, map[string]any{
		"action": "comment", "repo": "golang/example", "number": 7, "body": "looks right to me",
	})
	if !strings.Contains(out.Content, "commented on #7") {
		t.Errorf("comment receipt = %q", out.Content)
	}
	var sent map[string]any
	_ = json.Unmarshal(stub.seen["POST /repos/golang/example/issues/7/comments"], &sent)
	if sent["body"] != "looks right to me" {
		t.Errorf("comment body = %v", sent["body"])
	}
}

// The narrowing Step 0 exists for, seen from inside the tool: a stance that
// keeps the reading half must not be able to announce anything.
func TestPRNarrowedToReadsRefusesToWrite(t *testing.T) {
	stub := newStubGitHub(t)
	full := prTool(t, stub, t.TempDir())
	reading, ok := full.Narrow([]string{"pr_list", "pr_read", "pr_checks"}).(*prSkill)
	if !ok {
		t.Fatal("Narrow did not return a pr tool")
	}

	if got := reading.allowedActions(); len(got) != 3 {
		t.Fatalf("narrowed to %v, want the three reading acts", got)
	}
	_, err := reading.ExecuteTool(context.Background(), map[string]any{
		"action": "create", "repo": "golang/example", "title": "x", "head": "y",
	})
	if err == nil {
		t.Fatal("a pr narrowed to reads still opened a pull request")
	}
	if !strings.Contains(err.Error(), "not available here") {
		t.Errorf("refusal = %q, want it to say what this session may use", err)
	}
	if len(stub.seen) > 0 {
		t.Errorf("the refusal still called GitHub: %v", stub.seen)
	}
}

func TestPRUnknownActionIsRefusedByName(t *testing.T) {
	stub := newStubGitHub(t)
	s := prTool(t, stub, t.TempDir())
	_, err := s.ExecuteTool(context.Background(), map[string]any{"action": "merge", "number": 1})
	if err == nil {
		t.Fatal("an action this tool does not have was accepted")
	}
	if !strings.Contains(err.Error(), "unknown pr action") {
		t.Errorf("error = %q, want it to say the action is unknown", err)
	}
}

func writeGitConfig(t *testing.T, root, url string) {
	t.Helper()
	dir := filepath.Join(root, ".git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + url +
		"\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n[branch \"main\"]\n\tremote = origin\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
