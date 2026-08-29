package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
)

// roomApp is a focused project whose origin is a GitHub repository, with the
// API pointed at a stub. Everything below is about what the pane is handed.
func roomApp(t *testing.T, routes map[string]string) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "test-token")

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgBody := "[remote \"origin\"]\n\turl = https://github.com/Mikedev115/Aetox.git\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		payload, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no stub for this route"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)

	app := seed(&App{cfg: config.Config{SandboxRoot: root}}, newConversation())
	app.cur().cfg.SandboxRoot = root
	app.projectFocused = true
	app.prAPI = server.URL
	app.prHTTP = server.Client()
	t.Cleanup(func() {
		if app.db != nil {
			_ = app.db.Close()
		}
	})
	return app
}

func TestPullRequestsListsTheProjectsOwn(t *testing.T) {
	app := roomApp(t, map[string]string{
		"GET /repos/Mikedev115/Aetox/pulls": `[
			{"number":12,"title":"เพิ่ม type/multiline ให้ grep","state":"open","draft":false,
			 "user":{"login":"Mike0165115321"},"head":{"ref":"grep-multiline","sha":"abc1234"},
			 "base":{"ref":"main"},"additions":211,"deletions":51,
			 "html_url":"https://github.com/Mikedev115/Aetox/pull/12"}]`,
	})

	room := app.PullRequests()
	if room.Reason != "" {
		t.Fatalf("Reason = %q, want none", room.Reason)
	}
	if room.Repo != "Mikedev115/Aetox" {
		t.Errorf("Repo = %q — the room did not read the project's own origin", room.Repo)
	}
	if len(room.Items) != 1 || room.Items[0].Number != 12 {
		t.Fatalf("Items = %+v, want #12", room.Items)
	}
	got := room.Items[0]
	if got.HeadRef != "grep-multiline" || got.BaseRef != "main" || got.Additions != 211 {
		t.Errorf("the row lost facts it has to draw: %+v", got)
	}
}

// An empty list has four different meanings, and a pane that has to guess which
// will guess wrong. Each one says which.
func TestTheRoomSaysWhyItIsEmpty(t *testing.T) {
	t.Run("no account", func(t *testing.T) {
		app := roomApp(t, nil)
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		room := app.PullRequests()
		if !strings.Contains(room.Reason, "account") {
			t.Errorf("Reason = %q, want it to name the missing account", room.Reason)
		}
		if room.Connected {
			t.Error("Connected = true with no token")
		}
		if room.Items == nil {
			t.Error("Items is nil — §34, a nil slice crashes the frontend")
		}
	})

	t.Run("no project", func(t *testing.T) {
		app := roomApp(t, nil)
		app.projectFocused = false

		room := app.PullRequests()
		if !strings.Contains(room.Reason, "no project") {
			t.Errorf("Reason = %q, want it to say there is no project", room.Reason)
		}
		if !room.Connected {
			t.Error("Connected = false with a token — the two states are separate answers")
		}
	})

	t.Run("not a github repository", func(t *testing.T) {
		app := roomApp(t, nil)
		root := app.cur().cfg.SandboxRoot
		body := "[remote \"origin\"]\n\turl = https://gitlab.com/someone/thing.git\n"
		if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		room := app.PullRequests()
		if !strings.Contains(room.Reason, "gitlab.com") {
			t.Errorf("Reason = %q, want it to name where origin actually points", room.Reason)
		}
	})
}

// GitHub's own patch per file, which is what makes the room need no second
// diff renderer.
func TestPullRequestFilesCarryTheirPatch(t *testing.T) {
	app := roomApp(t, map[string]string{
		"GET /repos/Mikedev115/Aetox/pulls/12/files": `[
			{"filename":"internal/skill/grep.go","status":"modified","additions":211,"deletions":51,
			 "patch":"@@ -1,3 +1,4 @@\n context\n+added\n"},
			{"filename":"assets/logo.png","status":"modified","additions":0,"deletions":0}]`,
	})

	files := app.PullRequestFiles(12)
	if len(files) != 2 {
		t.Fatalf("files = %+v, want two", files)
	}
	if !strings.Contains(files[0].Patch, "+added") {
		t.Errorf("the patch did not come through: %q", files[0].Patch)
	}
	// A binary has no patch, and the pane has to be able to tell that from "no
	// changes" — so the field is empty rather than absent.
	if files[1].Patch != "" {
		t.Errorf("a binary file arrived with a patch: %q", files[1].Patch)
	}
}

// By SHA, because the list already carries every head SHA: taking the number
// would mean fetching the pull request again to learn a string the caller has.
func TestPullRequestChecksAreFetchedByCommit(t *testing.T) {
	app := roomApp(t, map[string]string{
		"GET /repos/Mikedev115/Aetox/commits/abc1234/check-runs": `{
			"total_count":2,"check_runs":[
			{"name":"build","status":"completed","conclusion":"success"},
			{"name":"test","status":"completed","conclusion":"failure"}]}`,
	})

	runs := app.PullRequestChecks("abc1234")
	if len(runs) != 2 {
		t.Fatalf("runs = %+v, want two", runs)
	}
	if runs[1].Conclusion != "failure" {
		t.Errorf("the failing run did not come through: %+v", runs[1])
	}
	if got := app.PullRequestChecks(""); got == nil || len(got) != 0 {
		t.Errorf("an empty sha answered %v — want an empty, non-nil list", got)
	}
}

// A refusal from GitHub reaches the pane as a sentence, not as an empty list
// with no explanation.
func TestTheRoomCarriesGitHubsRefusal(t *testing.T) {
	app := roomApp(t, nil) // every route 404s

	room := app.PullRequests()
	if room.Reason == "" {
		t.Fatal("a 404 came back as an ordinary empty list")
	}
	if !strings.Contains(room.Reason, "404") {
		t.Errorf("Reason = %q, want it to carry what GitHub said", room.Reason)
	}
}

// Opening one from the room. No approval gate, and that is not an oversight:
// the gate is for what the MODEL does, and a person filling in a form has
// already given the only approval there is.
func TestCreatePullRequestFromTheRoom(t *testing.T) {
	app := roomApp(t, map[string]string{
		"GET /repos/Mikedev115/Aetox":        `{"default_branch":"main"}`,
		"POST /repos/Mikedev115/Aetox/pulls": `{"number":13,"title":"x","html_url":"https://github.com/Mikedev115/Aetox/pull/13"}`,
	})

	got := app.CreatePullRequest("เพิ่มห้อง PR", "pr-room", "", "เพราะอยากเห็นโดยไม่ต้องถาม", false)
	if got.Error != "" {
		t.Fatalf("Error = %q, want none", got.Error)
	}
	if got.Number != 13 || got.URL == "" {
		t.Errorf("the form got nothing to show for it: %+v", got)
	}
	// base left blank means the repository's own, looked up rather than guessed.
	if got.Base != "main" {
		t.Errorf("Base = %q, want the default branch", got.Base)
	}
}

// GitHub's refusals here are almost always actionable, and the form is where
// they have to land — "failed" would throw away the only useful part.
func TestCreatePullRequestCarriesTheReasonToTheForm(t *testing.T) {
	app := roomApp(t, map[string]string{
		"GET /repos/Mikedev115/Aetox": `{"default_branch":"main"}`,
	})

	got := app.CreatePullRequest("x", "never-pushed", "", "", false)
	if got.Error == "" {
		t.Fatal("a refusal came back as a success")
	}
	if got.Number != 0 {
		t.Errorf("Number = %d on a refusal", got.Number)
	}
}

// No account is answered before anything is sent, in the same words the list
// uses — one state, one sentence, wherever it is met.
func TestCreatePullRequestNeedsAnAccount(t *testing.T) {
	app := roomApp(t, nil)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	got := app.CreatePullRequest("x", "y", "", "", false)
	if !strings.Contains(got.Error, "account") {
		t.Errorf("Error = %q, want it to name the missing account", got.Error)
	}
}
