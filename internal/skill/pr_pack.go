package skill

// Pull requests: the half of GitHub the read-only `github` pack was never
// allowed to be.
//
// `github` (github_pack.go) is "anyone's repository, read from here" - no
// account needed, nothing changed. A pull request is the other thing: it is
// about the user's OWN repository, it needs their token, and two of the five
// acts here write to a service other people can see. Folding those into a pack
// whose whole promise is reading would have made "may read a repository" and
// "may open a pull request in my name" one grant, which is the distinction
// github_pack.go was careful not to lose with `plugin_install`.
//
// **This pack straddles the read/write line on purpose, and that is new.** Every
// other pack in this package was drawn so it did not have to
// (search_pack.go/change_pack.go), because a desk and a stance could only ever
// take a tool whole. Since Step 0 they can take one act at a time
// (mode.AllowsAction, skill.Dispatcher.WithActions), so วางแผน carries `pr`
// narrowed to `list`, `read` and `checks` - it can read the state of the work
// without being able to announce anything. The rule that replaced the old one:
// a pack may straddle a gate that can narrow, and may not straddle one that
// cannot. `parallelToolCalls` still cannot, so `pr` is simply absent from it.
//
// Not here, deliberately: merging and closing. Both end an argument rather than
// making one, both are one click on a page the user already has open, and
// neither is a thing an agent should be able to do while the user is looking
// somewhere else. `create` and `comment` are additive - a wrong one is deleted
// in a second - and they are what the loop actually needs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	gh "github.com/Mikedev115/Aetox/internal/github"
	"github.com/Mikedev115/Aetox/internal/model"
)

// prReadLimit bounds one API response. A pull request's file list on a large
// change is the biggest thing here and is still far under this.
const prReadLimit = 4 << 20

// maxPRList and maxPRFiles keep one answer readable. A repository with sixty
// open pull requests does not need all sixty in a tool result, and the count
// says how many were left.
const (
	maxPRList  = 20
	maxPRFiles = 40
)

type prSkill struct {
	// root is the project the session is focused on, and the only reason this
	// tool can be called without naming a repository: the origin remote in that
	// folder is what "this project" means.
	root string
	// apiBaseURL is the GitHub API, overridable so tests can point at a stub.
	apiBaseURL string
	httpClient *http.Client
	// actions this caller may use, nil for all of them. See shellSkill.
	actions []string
}

func (*prSkill) Name() string { return "pr" }

func (*prSkill) Description() string {
	return "จัดการ pull request ของโปรเจกต์นี้, ดูรายการ อ่านรายละเอียด ดูผล CI เปิดใหม่ และคอมเมนต์"
}

func (s *prSkill) allowedActions() []string {
	p := packs["pr"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *prSkill) Actions() []string { return packs["pr"].permissions() }

func (s *prSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["pr"].narrow(named)
	return &narrowed
}

func (s *prSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	lines := map[string]string{
		"list":    "`list` (repo?, state?), the pull requests on a repository. state is open (default), closed or all.",
		"read":    "`read` (number, repo?), one pull request whole: state, branches, whether it merges cleanly, its files with +/-, and the latest CI result.",
		"checks":  "`checks` (number, repo?), just the CI runs for a pull request's head commit, with their conclusions.",
		"create":  "`create` (title, head, base?, body?, draft?, repo?), open a pull request. head is the branch with the work; base defaults to the repository's default branch.",
		"comment": "`comment` (number, body, repo?), add a comment to a pull request. It is posted as the connected account and other people can see it.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	properties := map[string]any{
		"action": map[string]any{
			"type": "string", "enum": allowed,
			"description": "What to do",
		},
		"repo": map[string]any{
			"type":        "string",
			"description": "owner/name or a github.com URL. Omit for this project's own origin remote.",
		},
	}
	if slices.Contains(allowed, "read") || slices.Contains(allowed, "checks") || slices.Contains(allowed, "comment") {
		properties["number"] = map[string]any{
			"type": "integer", "description": "The pull request number.",
		}
	}
	if slices.Contains(allowed, "list") {
		properties["state"] = map[string]any{
			"type": "string", "enum": []string{"open", "closed", "all"},
			"description": "action=list: which pull requests, open by default.",
		}
	}
	if slices.Contains(allowed, "create") {
		properties["title"] = map[string]any{"type": "string", "description": "action=create: the pull request title."}
		properties["head"] = map[string]any{"type": "string", "description": "action=create: the branch holding the work."}
		properties["base"] = map[string]any{"type": "string", "description": "action=create: the branch to merge into, default the repository's own."}
		properties["draft"] = map[string]any{"type": "boolean", "description": "action=create: open it as a draft."}
	}
	if slices.Contains(allowed, "create") || slices.Contains(allowed, "comment") {
		properties["body"] = map[string]any{
			"type": "string", "description": "action=create: the description. action=comment: the comment text.",
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"action"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "pr",
			Description: "Pull requests on GitHub, this project's by default. Actions:\n" +
				actions.String(),
			Parameters: payload,
		},
	}
}

// Guidance carries what the signature cannot: which of these is safe to reach
// for, and the one fact about `create` that costs a wasted call to learn.
func (*prSkill) Guidance(args map[string]any) string {
	switch actionOf(args) {
	case "create":
		return "create opens something other people can see, under the connected account, and it needs the head branch to " +
			"be PUSHED already - GitHub compares branches it has, not the working tree. Push first, then open.\n" +
			"A title is the one line most readers ever see. The body is where the reasoning goes; a body that restates " +
			"the diff tells them nothing the diff does not."
	case "comment":
		return "A comment is public and is posted under the connected account, so write it as the user would sign it. " +
			"It cannot be edited from here - the way back is the page itself."
	case "read":
		return "read is the whole picture: state, branches, mergeability, the file list with +/- and the latest CI. " +
			"Prefer it over list+checks, which is two calls for less.\n" +
			"The file list is capped and says so when it was cut. Reading the code itself is the file tools' job on a " +
			"checkout, or `github read_file` on a branch that is not checked out here."
	}
	return ""
}

func (s *prSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: pr <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("pr", "pr", "", start, false, err), err
	}
	call := map[string]any{"action": strings.ToLower(strings.TrimSpace(args[0]))}
	// The CLI form is `pr read 12` / `pr list`. Anything past the action is the
	// number when it is one, and the repository when it is not.
	if len(args) > 1 {
		if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil {
			call["number"] = n
		} else {
			call["repo"] = strings.TrimSpace(args[1])
		}
	}
	return s.ExecuteTool(ctx, call)
}

func (s *prSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	action := actionOf(args)
	fail := func(err error) (Output, error) {
		return newToolOutput("pr", "pr "+action, "", start, false, err), err
	}
	if action == "" {
		return fail(errors.New("action is required, one of: " + strings.Join(s.allowedActions(), ", ")))
	}
	if _, known := packs["pr"].names[action]; !known {
		return fail(fmt.Errorf("unknown pr action %q, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", ")))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return fail(fmt.Errorf("pr %s is not available here, this session may use: %s",
			action, strings.Join(s.allowedActions(), ", ")))
	}
	if gh.Token() == "" {
		// Named rather than generic: every one of these calls needs an account,
		// and "401" would send the reader to look for a bug in the request.
		return fail(errors.New("no GitHub account is connected — Settings → การเชื่อมต่อ, or set GITHUB_TOKEN"))
	}

	target, err := s.repoTarget(stringArg(args["repo"]))
	if err != nil {
		return fail(err)
	}

	var content string
	switch action {
	case "list":
		content, err = s.list(ctx, target, stringArg(args["state"]))
	case "read":
		content, err = s.read(ctx, target, intArg(args["number"]))
	case "checks":
		content, err = s.checks(ctx, target, intArg(args["number"]))
	case "create":
		content, err = s.create(ctx, target, args)
	case "comment":
		content, err = s.comment(ctx, target, intArg(args["number"]), stringArg(args["body"]))
	}
	if err != nil {
		return fail(err)
	}
	command := "pr " + action + " " + target.Owner + "/" + target.Repo
	if n := intArg(args["number"]); n > 0 {
		command += " #" + strconv.Itoa(n)
	}
	return newToolOutput("pr", command, content, start, false, nil), nil
}

// repoTarget answers "which repository" — the argument when there is one, and
// this project's origin remote when there is not.
//
// Both readings come from internal/github now (ParseRepo, OriginRepo), so the
// room in the workbench and this tool cannot disagree about what "this project"
// means. The old GitHubRepoTarget shape is kept as the return type because it
// is what the rest of this package speaks; nothing about it is parsed here.
func (s *prSkill) repoTarget(raw string) (GitHubRepoTarget, error) {
	as := func(r gh.Repo) GitHubRepoTarget {
		return GitHubRepoTarget{Owner: r.Owner, Repo: r.Name, URL: r.URL()}
	}
	if raw = strings.TrimSpace(raw); raw != "" {
		repo, ok := gh.ParseRepo(raw)
		if !ok {
			return GitHubRepoTarget{}, fmt.Errorf("cannot read %q as a repository — use owner/name or a github.com URL", raw)
		}
		return as(repo), nil
	}
	if strings.TrimSpace(s.root) == "" {
		return GitHubRepoTarget{}, errors.New("no repo given and no project is open — pass repo as owner/name")
	}
	repo, err := gh.OriginRepo(s.root)
	if err != nil {
		return GitHubRepoTarget{}, err
	}
	return as(repo), nil
}

// --- the acts -----------------------------------------------------------
//
// Each one is a fetch through internal/github and a rendering here. The split
// is the point: the room in the workbench draws the same facts as rows
// (desktop/pr_room.go), and one fetcher with two renderers cannot disagree with
// itself about a pull request the way two fetchers would.

func (s *prSkill) client() *gh.PRClient {
	return &gh.PRClient{BaseURL: s.apiBaseURL, HTTP: s.httpClient}
}

func (s *prSkill) list(ctx context.Context, t GitHubRepoTarget, state string) (string, error) {
	repo := gh.Repo{Owner: t.Owner, Name: t.Repo}
	prs, err := s.client().List(ctx, repo, state, maxPRList)
	if err != nil {
		return "", err
	}
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "closed" && state != "all" {
		state = "open"
	}
	if len(prs) == 0 {
		return fmt.Sprintf("no %s pull requests on %s", state, repo), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s pull requests on %s:\n", state, repo)
	for _, pr := range prs {
		mark := ""
		if pr.Draft {
			mark = " (draft)"
		}
		fmt.Fprintf(&b, "#%d %s%s — %s, %s → %s\n",
			pr.Number, pr.Title, mark, emptyFallback(pr.Author, "unknown"), pr.HeadRef, pr.BaseRef)
	}
	if len(prs) == maxPRList {
		fmt.Fprintf(&b, "(first %d; there may be more)\n", maxPRList)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *prSkill) read(ctx context.Context, t GitHubRepoTarget, number int) (string, error) {
	repo := gh.Repo{Owner: t.Owner, Name: t.Repo}
	client := s.client()
	pr, err := client.Get(ctx, repo, number)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s\n", pr.Number, pr.Title)
	fmt.Fprintf(&b, "%s by %s, %s → %s, +%d -%d across %d files\n",
		prState(pr), emptyFallback(pr.Author, "unknown"), pr.HeadRef, pr.BaseRef,
		pr.Additions, pr.Deletions, pr.ChangedFiles)
	if pr.Mergeable != nil && !*pr.Mergeable {
		fmt.Fprintf(&b, "does NOT merge cleanly (%s)\n", emptyFallback(pr.MergeableState, "conflict"))
	}
	if trimmed := strings.TrimSpace(pr.Body); trimmed != "" {
		fmt.Fprintf(&b, "\n%s\n", clampPRText(trimmed, 1200))
	}

	// The files, which is the question after "what is this". A failure here
	// must not take the rest of the answer with it: a pull request whose file
	// list cannot be read is still worth reading.
	if files, ferr := client.Files(ctx, repo, number, maxPRFiles); ferr == nil && len(files) > 0 {
		b.WriteString("\nfiles:\n")
		for _, f := range files {
			fmt.Fprintf(&b, "  %s %s +%d -%d\n", f.Status, f.Path, f.Additions, f.Deletions)
		}
		if len(files) == maxPRFiles && pr.ChangedFiles > maxPRFiles {
			fmt.Fprintf(&b, "  ... %d more\n", pr.ChangedFiles-maxPRFiles)
		}
	}
	if runs := summariseChecks(client, ctx, repo, pr.HeadSHA); runs != "" {
		fmt.Fprintf(&b, "\nchecks: %s\n", runs)
	}
	fmt.Fprintf(&b, "\n%s", pr.URL)
	return b.String(), nil
}

func (s *prSkill) checks(ctx context.Context, t GitHubRepoTarget, number int) (string, error) {
	repo := gh.Repo{Owner: t.Owner, Name: t.Repo}
	client := s.client()
	pr, err := client.Get(ctx, repo, number)
	if err != nil {
		return "", err
	}
	runs := summariseChecks(client, ctx, repo, pr.HeadSHA)
	if runs == "" {
		return fmt.Sprintf("#%d has no checks reported on %s", number, gh.ShortSHA(pr.HeadSHA)), nil
	}
	return fmt.Sprintf("#%d at %s: %s", number, gh.ShortSHA(pr.HeadSHA), runs), nil
}

// summariseChecks is one line, or "" when there is nothing to say. It never
// returns an error: checks that cannot be read must not take a pull request's
// own answer down with them.
//
// Failures first and by name; everything else is a count. A reader needs to
// know which check to go and look at, never which forty passed.
func summariseChecks(client *gh.PRClient, ctx context.Context, repo gh.Repo, sha string) string {
	runs, err := client.Checks(ctx, repo, sha)
	if err != nil || len(runs) == 0 {
		return ""
	}
	var failed, running, passed []string
	for _, r := range runs {
		switch {
		case !r.Done():
			running = append(running, r.Name)
		case r.Passed():
			passed = append(passed, r.Name)
		default:
			failed = append(failed, r.Name+" ("+emptyFallback(r.Conclusion, "failed")+")")
		}
	}
	parts := make([]string, 0, 3)
	if len(failed) > 0 {
		parts = append(parts, "FAILED: "+strings.Join(failed, ", "))
	}
	if len(running) > 0 {
		parts = append(parts, strconv.Itoa(len(running))+" still running")
	}
	if len(passed) > 0 {
		parts = append(parts, strconv.Itoa(len(passed))+" passed")
	}
	return strings.Join(parts, "; ")
}

func (s *prSkill) create(ctx context.Context, t GitHubRepoTarget, args map[string]any) (string, error) {
	created, err := s.client().Create(ctx, gh.Repo{Owner: t.Owner, Name: t.Repo}, gh.NewPR{
		Title: strings.TrimSpace(stringArg(args["title"])),
		Head:  strings.TrimSpace(stringArg(args["head"])),
		Base:  strings.TrimSpace(stringArg(args["base"])),
		Body:  stringArg(args["body"]),
		Draft: boolArg(args["draft"]),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("opened #%d %s (%s → %s)\n%s",
		created.Number, created.Title,
		strings.TrimSpace(stringArg(args["head"])), created.BaseRef, created.URL), nil
}

func (s *prSkill) comment(ctx context.Context, t GitHubRepoTarget, number int, text string) (string, error) {
	url, err := s.client().Comment(ctx, gh.Repo{Owner: t.Owner, Name: t.Repo}, number, text)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("commented on #%d\n%s", number, url), nil
}

func prState(pr gh.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return emptyFallback(pr.State, "unknown")
}

// clampPRText cuts a description to something a tool result can carry, on a
// rune boundary: half a Thai character is worse than one character less.
func clampPRText(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "..."
}
