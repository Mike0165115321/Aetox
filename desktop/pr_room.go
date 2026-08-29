package main

// The pull requests of the focused project, as a room.
//
// The `pr` tool (internal/skill/pr_pack.go) answers the model in sentences.
// This answers the window in rows, from the same fetcher (internal/github),
// because two fetchers is how two surfaces start disagreeing about the same
// pull request — the rule GitPane already follows for the working tree
// (DECISIONS §161.4).
//
// What the room adds that the tool cannot: seeing without asking. A CI result
// that is on screen while you work is a different thing from one you have to
// remember to go and request.

import (
	"context"
	"strings"

	gh "github.com/Mikedev115/Aetox/internal/github"
)

// prClient is the room's client, and the one seam a test needs: prAPI and
// prHTTP are empty in every real build, which is exactly the zero PRClient
// pointing at github.com.
func (a *App) prClient() *gh.PRClient {
	return &gh.PRClient{BaseURL: a.prAPI, HTTP: a.prHTTP}
}

// maxRoomPRs is what one list shows. A repository with sixty open pull
// requests does not need all sixty drawn before anybody has scrolled.
const maxRoomPRs = 30

// PRRoom is the whole answer for the pane: the list, and — when there is no
// list — why.
//
// One binding rather than a list plus a separate status call, because an empty
// list has four different meanings (no account, not a GitHub repo, no project,
// genuinely none open) and a pane that has to guess which will guess wrong. The
// reason travels with the emptiness that needs it.
type PRRoom struct {
	// Repo is owner/name, "" when there is none to name.
	Repo string `json:"repo"`
	// Reason is why the list is empty, "" when nothing is wrong. It is a
	// sentence for a person, already in the language the engine speaks.
	Reason string `json:"reason"`
	// Connected reports whether a GitHub account is attached at all, so the
	// pane can offer the way to attach one rather than only saying it is not.
	Connected bool             `json:"connected"`
	Items     []gh.PullRequest `json:"items"`
}

// PullRequests lists the open pull requests of the project this chat is
// focused on.
func (a *App) PullRequests() PRRoom {
	room := PRRoom{Items: []gh.PullRequest{}, Connected: gh.Token() != ""} // never nil: §34
	repo, ok := a.roomRepo(&room)
	if !ok {
		return room
	}
	items, err := a.prClient().List(context.Background(), repo, "open", maxRoomPRs)
	if err != nil {
		room.Reason = err.Error()
		return room
	}
	room.Items = items
	return room
}

// PullRequestFiles is one pull request's files, each with GitHub's own unified
// diff. Fetched when a row is expanded, never before: a pull request of forty
// files is ordinary, and drawing none of them costs nothing.
func (a *App) PullRequestFiles(number int) []gh.PRFile {
	files := []gh.PRFile{} // never nil: §34
	var room PRRoom
	repo, ok := a.roomRepo(&room)
	if !ok || number <= 0 {
		return files
	}
	got, err := a.prClient().Files(context.Background(), repo, number, maxPRRoomFiles)
	if err != nil {
		return files
	}
	return got
}

// maxPRRoomFiles caps one expanded row. Past this the change is too big to read
// in a side panel, and the pull request's own page is the right place for it.
const maxPRRoomFiles = 60

// PullRequestChecks is the CI runs for one commit.
//
// By SHA rather than by pull request number, and the reason is the row it
// draws: the list already carries every head SHA, so a badge per row is one
// call each. Taking the number would mean fetching the pull request again just
// to learn a string the caller is already holding.
func (a *App) PullRequestChecks(sha string) []gh.CheckRun {
	runs := []gh.CheckRun{} // never nil: §34
	var room PRRoom
	repo, ok := a.roomRepo(&room)
	if !ok || strings.TrimSpace(sha) == "" {
		return runs
	}
	got, err := a.prClient().Checks(context.Background(), repo, sha)
	if err != nil {
		return runs
	}
	return got
}

// PRCreated is what opening one from the room answers with: the pull request,
// or the sentence explaining why there is none.
//
// An error rather than a Go error, because this one is READ by the pane and
// shown in the form the user is standing in. GitHub's refusals here are almost
// always actionable — "No commits between main and feature" means push first,
// "A pull request already exists" means it is already open — and a dialog that
// said "failed" would throw away the only useful part.
type PRCreated struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Base   string `json:"base"`
	Error  string `json:"error"`
}

// CreatePullRequest opens one for the focused project.
//
// No approval gate, and that is not an oversight: the gate exists for acts the
// MODEL performs (safety.AssessCommand marks `pr_create` high-risk for exactly
// that), and a person pressing a button in a form they filled in has already
// given the only approval there is. The same reason saving from the editor does
// not ask.
//
// base empty means the repository's own default branch, which internal/github
// looks up — the form leaves it blank far more often than it fills it in.
func (a *App) CreatePullRequest(title, head, base, body string, draft bool) PRCreated {
	var room PRRoom
	repo, ok := a.roomRepo(&room)
	if !ok {
		return PRCreated{Error: room.Reason}
	}
	created, err := a.prClient().Create(context.Background(), repo, gh.NewPR{
		Title: strings.TrimSpace(title),
		Head:  strings.TrimSpace(head),
		Base:  strings.TrimSpace(base),
		Body:  body,
		Draft: draft,
	})
	if err != nil {
		return PRCreated{Error: err.Error()}
	}
	return PRCreated{Number: created.Number, URL: created.URL, Base: created.BaseRef}
}

// roomRepo is the three checks every call here shares, in the order that gives
// the most useful answer: no account beats no repository, because connecting
// one is the thing to do either way.
func (a *App) roomRepo(room *PRRoom) (gh.Repo, bool) {
	if gh.Token() == "" {
		room.Reason = "no GitHub account is connected"
		return gh.Repo{}, false
	}
	root := a.cur().cfg.SandboxRoot
	if !a.projectFocused || root == "" {
		room.Reason = "no project is open"
		return gh.Repo{}, false
	}
	repo, err := gh.OriginRepo(root)
	if err != nil {
		room.Reason = err.Error()
		return gh.Repo{}, false
	}
	room.Repo = repo.String()
	return repo, true
}
