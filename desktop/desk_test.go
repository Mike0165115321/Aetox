package main

// What a desk does to a real engine (ARCHITECTURE.md §83/§84).
//
// internal/mode has its own unit tests, and they prove that a manifest parses
// and that AllowsTool answers what the file says. None of that would catch the
// failure this file is about: a desk that parses perfectly and is never wired
// to the dispatcher the session actually runs on. So every assertion here goes
// through bootstrap — the engine the app builds — and asks what the *model*
// would be sent.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/prompt"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// bootDeskApp is a real desktop engine at one desk, with the data root and the
// store isolated per test.
func bootDeskApp(t *testing.T, desk string) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{
		ctx:       context.Background(),
		emit:      func(string, ...any) {},
		dbDir:     t.TempDir(),
		sessionID: newSessionID(),
	}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	if desk != "" {
		m, ok := mode.Load(desk)
		if !ok {
			t.Fatalf("mode.Load(%q) failed", desk)
		}
		a.desk = m
	}
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a
}

// toolNames is what this session would send the model, by name.
func toolNames(a *App) []string {
	defs := a.deskTools().ToolDefinitions()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Function.Name)
	}
	slices.Sort(names)
	return names
}

// A session from before desks existed must be untouched by all of this: the
// same tools, and the same system prompt down to the byte. An upgrade that
// quietly narrows what an existing conversation can do — or that spends the
// user's prefix cache by adding a line to its prompt — is the one outcome this
// whole change is not allowed to have.
func TestALegacySessionKeepsTheFullDeskAndTheSamePrompt(t *testing.T) {
	a := bootDeskApp(t, "")

	// Every registered tool that faces the model, with nothing filtered out.
	installed := 0
	for _, name := range a.registry.Names() {
		if source, ok := a.registry.SourceOf(name); ok && source == "skill" {
			continue // skills are never sent; they are reached through skills_list
		}
		if _, ok := a.registry.Get(name); ok {
			installed++
		}
	}
	got := toolNames(a)
	if len(got) == 0 || len(got) > installed {
		t.Fatalf("the legacy desk sends %d tools out of %d registered", len(got), installed)
	}
	for _, want := range []string{"shell", "slides_write", "read", "task", "web_search"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is missing from a legacy session — the full desk carries everything: %v", want, got)
		}
	}

	// And the prompt is what prompt.Build alone produces. Compared against the
	// package rather than a golden string, so it stays true as the prompt itself
	// changes and only fails when a *desk* has added something to it.
	want := prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: a.cfg.SandboxRoot, Open: true})
	messages := a.agent.ContextMessages()
	if len(messages) == 0 {
		t.Fatal("no system prompt")
	}
	if messages[0].Content != want {
		t.Errorf("a legacy session's prompt is no longer prompt.Build's:\n--- got ---\n%s\n--- want ---\n%s",
			messages[0].Content, want)
	}
}

// The two desks a person picks between differ where COMPANY.md says they do,
// and the check runs on the tool block the model would receive rather than on
// the manifest that was parsed to build it.
func TestEachDeskSendsOnlyItsOwnTools(t *testing.T) {
	coding := toolNames(bootDeskApp(t, "coding"))
	assistant := toolNames(bootDeskApp(t, "assistant"))

	for _, c := range []struct {
		desk  string
		tools []string
		want  bool
		names []string
	}{
		{"coding", coding, true, []string{"shell", "read", "edit", "diagnostics", "task"}},
		{"coding", coding, false, []string{"slides_write", "doc_write", "sheet_write", "image_ocr"}},
		// COMPANY.md §2: the assistant desk does everything on this machine
		// except the developer tools. It has the shell — safety is the gate's
		// job, not a missing tool's (§6.2) — and no diagnostics or symbol.
		{"assistant", assistant, true, []string{"read", "edit", "shell", "web_search", "memory", "task"}},
		{"assistant", assistant, false, []string{"diagnostics", "symbol", "github_search"}},
		// The three writers left this desk on the owner's call (2026-08-06):
		// *"เมนไม่ควรทำเองสิครับ มันคืองานของเอเจนเฉพาะทางที่เราสร้างมาแล้ว"*.
		// The office already had a chair per format — doc, deck, sheet, each
		// carrying exactly one writer at desk: specialized — and leaving the
		// writers here too meant the assistant did the job itself whenever it
		// looked small, which is a choice made by mood rather than by rule.
		// `task` above is what stays: the way to have one made.
		{"assistant", assistant, false, []string{"doc_write", "sheet_write", "slides_write"}},
	} {
		for _, name := range c.names {
			if got := slices.Contains(c.tools, name); got != c.want {
				t.Errorf("%s desk: %s sent=%v, want %v (block: %v)", c.desk, name, got, c.want, c.tools)
			}
		}
	}

	// Narrower is the point, not a side effect: the desk exists to cut the tool
	// block, and a desk that sent as much as the full one would have cost the
	// user a decision for nothing.
	full := toolNames(bootDeskApp(t, ""))
	if len(coding) >= len(full) || len(assistant) >= len(full) {
		t.Errorf("a desk sent no fewer tools than the full desk: coding=%d assistant=%d full=%d",
			len(coding), len(assistant), len(full))
	}
}

// The desk's direction and its own memory reach the prompt, the identity layer
// is still the only thing answering who the assistant is (§44.0), and what one
// desk learned costs the others nothing — which is the whole reason memory has
// a desk scope at all.
func TestADeskAddsDirectionAndItsOwnMemory(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := learned.Apply(learned.ModeScope("coding"), learned.OpAdd, "", "CODING-DESK-MARKER"); err != nil {
		t.Fatalf("write desk memory: %v", err)
	}
	if err := learned.Apply(learned.MainScope, learned.OpAdd, "", "CROSS-DESK-MARKER"); err != nil {
		t.Fatalf("write shared memory: %v", err)
	}
	// bootDeskApp sets a data root of its own, so this test builds its apps by
	// hand under the one the memory was just written to.
	boot := func(desk string) string {
		t.Helper()
		a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
		t.Cleanup(func() {
			if a.db != nil {
				_ = a.db.Close()
			}
		})
		if desk != "" {
			m, ok := mode.Load(desk)
			if !ok {
				t.Fatalf("mode.Load(%q) failed", desk)
			}
			a.desk = m
		}
		a.applyConfig(config.Config{
			SandboxRoot:   t.TempDir(),
			ModelProvider: "aetox",
			ModelName:     "aetox-tools:test",
			ApprovalMode:  string(safety.ApprovalFullAccess),
		})
		messages := a.agent.ContextMessages()
		if len(messages) == 0 {
			t.Fatal("no system prompt")
		}
		return messages[0].Content
	}

	coding := boot("coding")
	desk, _ := mode.Load("coding")
	if !strings.Contains(coding, strings.TrimSpace(desk.Prompt)) {
		t.Errorf("the coding desk's direction is not in the prompt:\n%s", coding)
	}
	if !strings.Contains(coding, "You are Aetox") {
		t.Error("the identity layer went missing when a desk was added")
	}
	if !strings.Contains(coding, "CODING-DESK-MARKER") {
		t.Error("what the coding desk learned never reached its own prompt")
	}
	if !strings.Contains(coding, "CROSS-DESK-MARKER") {
		t.Error("the shared memory stopped being shared once a desk had one of its own")
	}

	assistant := boot("assistant")
	if strings.Contains(assistant, "CODING-DESK-MARKER") {
		t.Error("the assistant desk is paying for what coding work taught the agent")
	}
	if !strings.Contains(assistant, "CROSS-DESK-MARKER") {
		t.Error("the shared memory did not reach the assistant desk")
	}
}

// A cross-desk dispatch runs on the *target* desk's manifest, and the chair's
// own frontmatter can only narrow that, never widen it (§84). A chair that
// writes `tools: shell` into itself is the case this has to answer, because it
// is the one a user can create by hand.
func TestAChairIsCappedByTheOfficeCeiling(t *testing.T) {
	a := bootDeskApp(t, "assistant")

	root, err := config.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	// The agents' home — since the homes split, a chair file anywhere else is
	// sick by rule, not a chair.
	dir := filepath.Join(root, "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Everything this chair asks for: two tools the office carries, and `shell`,
	// which the office does not have — but the desk dispatching it *does*. That
	// is what makes this the carve-out test rather than a repeat of the profile
	// allowlist: the ceiling has to come from the desk the job runs at, so
	// inheriting the caller's would hand this chair a shell.
	const greedy = "---\ndescription: เก้าอี้ทดสอบ\ndesk: specialized\ntools: doc_write, write, shell\n---\nWrite the thing.\n"
	if err := os.WriteFile(filepath.Join(dir, "greedy.md"), []byte(greedy), 0o644); err != nil {
		t.Fatal(err)
	}

	profile, ok := subagent.Load("greedy")
	if !ok {
		t.Fatal("the test chair did not load")
	}
	office, ok := mode.Load(mode.Office)
	if !ok {
		t.Fatal("the office desk is missing")
	}
	child := subagent.FilterRegistry(a.registry, profile, office)
	if child == nil {
		t.Fatal("no child registry")
	}
	names := child.Names()

	if !slices.Contains(names, "doc_write") {
		t.Errorf("the chair lost the writer its job is: %v", names)
	}
	if !slices.Contains(names, "write") {
		t.Errorf("the chair lost a tool the office's own manifest names: %v", names)
	}
	// The caller has shell (COMPANY.md §2) and the chair asked for it. It is
	// absent because the job runs on the office's manifest — the ceiling comes
	// from the desk the work is done at, never from the desk that sent it.
	if !a.desk.AllowsTool("shell") {
		t.Fatal("this test needs a caller that has shell for the ceiling to be worth asserting")
	}
	if slices.Contains(names, "shell") {
		t.Errorf("a chair got shell from its caller's desk — the office ceiling is not being applied: %v", names)
	}
	if slices.Contains(names, "task") {
		t.Errorf("a chair can start its own delegates: %v", names)
	}
}

// The dispatch door, end to end on Aetox's own model (§45): an assistant-desk
// session hands a whole job to a chair, the chair works in its own context on
// the office's manifest, and what crosses back is a file plus a line of result.
//
// The recorded half matters as much as the file. `jobs` is what every later
// layer reads, and a delegation that ran but left no row with `parent_ref` on
// it is a job nobody can attribute afterwards — which is the office page's
// whole feed.
func TestTheAssistantDeskHandsAJobToTheOfficeAndGetsAFileBack(t *testing.T) {
	a := bootDeskApp(t, "assistant")

	reply, err := a.SendMessage("subagent office: ทำเอกสารสรุปให้หน่อย")
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if strings.TrimSpace(reply.Text) == "" {
		t.Fatal("the parent turn came back empty")
	}

	// The file landed in the caller's own output folder, not the chair's:
	// nothing about crossing desks moves where a session's work is kept.
	produced := filepath.Join(a.cfg.SandboxRoot, "output", a.sessionID, "office-demo.docx")
	if info, err := os.Stat(produced); err != nil || info.Size() == 0 {
		t.Fatalf("no document at %s (err=%v)", produced, err)
	}

	jobs := readJobs(t, a)
	var chairJob *storedJob
	for i := range jobs {
		if jobs[i].agent == "doc" {
			chairJob = &jobs[i]
		}
	}
	if chairJob == nil {
		t.Fatalf("no job filed under the chair that did the work: %+v", jobs)
	}
	if chairJob.parentRef == "" {
		t.Error("the chair's job has no parent_ref — nothing says which call sent it")
	}
	if !strings.Contains(chairJob.toolSeq, "doc_write") {
		t.Errorf("tool_seq = %q, want the writer the chair actually ran", chairJob.toolSeq)
	}

	// And the office page reads exactly that row back.
	received := a.ListReceivedJobs(10)
	if len(received) != 1 || received[0].Chair != "doc" {
		t.Fatalf("ListReceivedJobs() = %+v, want the one job the office took in", received)
	}
	if received[0].SessionID != a.sessionID {
		t.Errorf("the feed files the job under session %q, want the caller's %q", received[0].SessionID, a.sessionID)
	}

	// The roster now shows the chair as having done something, from the same rows.
	for _, c := range a.ListChairs() {
		if c.Name == "doc" && c.Jobs != 1 {
			t.Errorf("the doc chair reports %d jobs after doing one", c.Jobs)
		}
	}
}

// The coding desk talks to nobody (COMPANY.md §3): it declares no `dispatch:`,
// so a chair is not in its `task` schema and naming one anyway is refused with
// a reason rather than quietly run.
func TestTheCodingDeskCannotHandWorkToTheOffice(t *testing.T) {
	a := bootDeskApp(t, "coding")

	defs := a.deskTools().ToolDefinitions()
	var schema string
	for _, d := range defs {
		if d.Function.Name == "task" {
			schema = string(d.Function.Parameters) + d.Function.Description
		}
	}
	if schema == "" {
		t.Fatal("the coding desk has no task tool at all")
	}
	if strings.Contains(schema, `"deck"`) || strings.Contains(schema, "deck —") {
		t.Errorf("a chair is offered to a desk that cannot dispatch to it:\n%s", schema)
	}
	if !strings.Contains(schema, `"explore"`) {
		t.Errorf("the desk's own delegates went missing with it:\n%s", schema)
	}

	out, ran, err := a.deskTools().ExecuteTool(context.Background(), "task", map[string]any{
		"description": "make a deck",
		"prompt":      "build a deck about anything",
		"agent":       "deck",
	})
	if err != nil {
		t.Fatalf("task returned a hard error rather than a refusal the model can read: %v", err)
	}
	if !ran {
		t.Fatal("the coding desk has no task tool to refuse with")
	}
	if out.Success {
		t.Fatal("the coding desk dispatched to the office")
	}
	if !strings.Contains(out.Content, "specialized") {
		t.Errorf("the refusal does not say which desk that work belongs to: %q", out.Content)
	}
}

// A session is born at a desk, stored with it, and comes back to it — and
// there is deliberately no way to move one (COMPANY.md §6.3). The history list
// filters on the same column, which is what puts a button's own conversations
// behind that button.
func TestASessionIsStoredWithItsDeskAndReopensThere(t *testing.T) {
	a := bootDeskApp(t, "")

	write := func(desk, text string) string {
		t.Helper()
		id, err := a.NewSessionAt(desk)
		if err != nil {
			t.Fatalf("NewSessionAt(%q): %v", desk, err)
		}
		a.appendTurn(
			SessionMessage{Role: "user", Text: text, Time: "t"},
			SessionMessage{Role: "agent", Text: "ok", Time: "t"})
		return id
	}
	assistantID := write("assistant", "ช่วยจดไว้หน่อย")
	codingID := write("coding", "แก้บั๊กให้ที")
	legacyID := write("", "ก่อนมีโหมด")

	for id, want := range map[string]string{assistantID: "assistant", codingID: "coding", legacyID: ""} {
		if got := a.SessionMode(id); got != want {
			t.Errorf("SessionMode(%s) = %q, want %q", id, got, want)
		}
	}

	only := func(desk string) []string {
		var ids []string
		for _, s := range a.ListSessionsAt(desk) {
			ids = append(ids, s.ID)
		}
		return ids
	}
	if got := only("assistant"); !slices.Contains(got, assistantID) || slices.Contains(got, codingID) {
		t.Errorf("the assistant list is %v — it should hold its own sessions and no others", got)
	}
	if got := only(""); !slices.Contains(got, assistantID) || !slices.Contains(got, codingID) || !slices.Contains(got, legacyID) {
		t.Errorf("the combined list %v dropped a session; a session held at no desk has nowhere else to appear", got)
	}

	// Reopening goes back to the desk the conversation was held at.
	if _, err := a.LoadSession(codingID); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if a.desk.DeskName() != "coding" {
		t.Errorf("reopened at desk %q, want coding", a.desk.DeskName())
	}
	if _, err := a.LoadSession(legacyID); err != nil {
		t.Fatalf("LoadSession(legacy): %v", err)
	}
	if a.desk != nil {
		t.Errorf("a session from before desks reopened at %q instead of the full desk", a.desk.DeskName())
	}

	// A name nothing answers to is refused rather than quietly widened to the
	// full desk, which is what a stale picker would otherwise buy the user.
	if _, err := a.NewSessionAt("no-such-desk"); err == nil {
		t.Error("an unknown desk name was accepted")
	}
}

// The ผลงาน page reads the disk, so what it reports is what is actually there.
func TestListArtifactsFindsWhatASessionProduced(t *testing.T) {
	isolateUserDirs(t)
	a := bootDeskApp(t, "assistant")

	dir := filepath.Join(a.cfg.SandboxRoot, "output", a.sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.docx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := a.ListArtifacts()
	var got *Artifact
	for i := range found {
		if found[i].Name == "report.docx" {
			got = &found[i]
		}
	}
	if got == nil {
		t.Fatalf("ListArtifacts() = %+v, missing the file on disk", found)
	}
	if got.SessionID != a.sessionID {
		t.Errorf("artifact filed under session %q, want %q — the folder name is the only record of it",
			got.SessionID, a.sessionID)
	}
	if got.Size == 0 || got.Modified == "" {
		t.Errorf("artifact has no size or time: %+v", got)
	}

	// Deleting the conversation does not delete the work (COMPANY.md §6.7):
	// the report survives the email thread that asked for it.
	a.appendTurn(
		SessionMessage{Role: "user", Text: "ทำรายงานให้ที", Time: "t"},
		SessionMessage{Role: "agent", Text: "เสร็จแล้ว", Time: "t"})
	if err := a.DeleteSession(a.sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatalf("deleting the conversation took its work with it: %v", err)
	}

	// And the ผลงาน page is where a file actually dies — nowhere else, and
	// nothing outside an output folder can be named.
	if err := a.DeleteArtifact(filepath.Join(a.cfg.SandboxRoot, "..", "elsewhere.txt")); err == nil {
		t.Error("DeleteArtifact accepted a path outside the output folders")
	}
	if err := a.DeleteArtifact(got.Path); err != nil {
		t.Fatalf("DeleteArtifact: %v", err)
	}
	if _, err := os.Stat(got.Path); !os.IsNotExist(err) {
		t.Errorf("the file is still there after the user deleted it: %v", err)
	}
	// Asking again is not an error: the user asked for it to be gone, and it is.
	if err := a.DeleteArtifact(got.Path); err != nil {
		t.Errorf("deleting an already-deleted artifact reported %v", err)
	}
}

// stubTool is a tool with no behaviour, for asking where a *name* lands. Only
// the registry source matters to the desk filter, so a real MCP server would
// answer the same question at fifty times the cost.
type stubTool struct{ name string }

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return "stub" }
func (s *stubTool) Execute(context.Context, skill.Input) (skill.Output, error) {
	return skill.Output{}, nil
}
func (s *stubTool) ExecuteTool(context.Context, map[string]any) (skill.Output, error) {
	return skill.Output{}, nil
}
func (s *stubTool) ToolDefinition() model.ToolDefinition {
	return model.ToolDefinition{Type: "function", Function: model.ToolFunction{Name: s.name}}
}

// A server reaches exactly the desks it named, and the reason this is a desk
// test rather than a unit one is the trap it sits on: CategoryOf answers
// `agent` for every name it does not know, so a desk carrying the agent group
// — all of them do — would pick up every installed server if membership went
// through AllowsTool. That is the pile modes exist to take apart, arriving
// through the side door.
//
// Which way round the naming goes changed on 2026-08-06: the server names its
// desks in `for:`, not the desk its servers. A manifest is compiled in before
// any of the user's servers exist, so it could never name one — which is why
// every configured server was connected, registered and then filtered off
// every desk, and the assistant reported having no MCP tools at all.
func TestAServerReachesOnlyTheDesksThatNamedIt(t *testing.T) {
	a := bootDeskApp(t, "")

	root, err := config.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "modes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const manifest = "---\ndescription: โต๊ะทดสอบ\ncategories: agent\n---\nA desk with one server.\n"
	if err := os.WriteFile(filepath.Join(dir, "withmcp.md"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "notion", Command: []string{"npx", "notion"}, For: []string{"withmcp"}},
		{Name: "linear", Command: []string{"npx", "linear"}, For: []string{}},
	}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"notion_search", "linear_issue"} {
		if err := a.registry.Register(&stubTool{name: name}, skill.SourceMCP); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}

	sees := func(desk string) []string {
		t.Helper()
		if desk == "" {
			a.desk = nil
		} else {
			m, ok := mode.Load(desk)
			if !ok {
				t.Fatalf("mode.Load(%q) failed", desk)
			}
			a.desk = m
		}
		return toolNames(a)
	}

	if got := sees("withmcp"); !slices.Contains(got, "notion_search") || slices.Contains(got, "linear_issue") {
		t.Errorf("a desk naming notion saw %v — it should have that server and no other", got)
	}
	if got := sees("assistant"); slices.Contains(got, "notion_search") || slices.Contains(got, "linear_issue") {
		t.Errorf("a desk that names no servers was handed one anyway: %v", got)
	}
	if got := sees(""); !slices.Contains(got, "notion_search") || !slices.Contains(got, "linear_issue") {
		t.Errorf("the legacy full desk lost a server it always had: %v", got)
	}
}

// The roster the office page draws is the profiles that say they sit there,
// and the tools it shows are the ones the chair really gets.
func TestListChairsReportsTheRosterUnderTheCeiling(t *testing.T) {
	a := bootDeskApp(t, "assistant")
	chairs := a.ListChairs()
	if len(chairs) != 4 {
		t.Fatalf("ListChairs() = %d, want the four bundled chairs", len(chairs))
	}
	byName := map[string]Chair{}
	for _, c := range chairs {
		byName[c.Name] = c
	}
	// This roster is one of the three ways a specialist is reachable, and the
	// same call feeds the chat page's picker. An agent absent here is absent
	// from the product, however correct its own file looks.
	if _, ok := byName["github"]; !ok {
		t.Errorf("the github chair is missing from the roster: %+v", chairs)
	}
	deck, ok := byName["deck"]
	if !ok {
		t.Fatalf("the deck chair is missing from the roster: %+v", chairs)
	}
	if !slices.Contains(deck.Tools, "slides_write") {
		t.Errorf("the deck chair is listed without its writer: %v", deck.Tools)
	}
	if slices.Contains(deck.Tools, "shell") || slices.Contains(deck.Tools, "task") {
		t.Errorf("the roster shows a chair holding something the office has no ceiling for: %v", deck.Tools)
	}
	if deck.Jobs != 0 || deck.LastUsed != "" {
		t.Errorf("a chair that has never been handed anything reports %d jobs at %q", deck.Jobs, deck.LastUsed)
	}
}
