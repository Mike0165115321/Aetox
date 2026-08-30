package subagent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/mode"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// The two homes (owner's call, 2026-08-05): a file's home is its kind, decided
// in resolve() and nowhere else. What these tests pin is not that folders
// exist — it is the three rules that make the split hold: home decides kind,
// old files move home once, and a name has exactly one owner.

// writeProfile writes into a home in that home's own shape: an agent is a
// folder holding AGENT.md, a helper is one flat file. Shape-aware rather than
// two helpers, so a test that says "put this file in that home" keeps reading
// as the sentence it is.
func writeProfile(t *testing.T, home func() (string, error), name, body string) string {
	t.Helper()
	dir, err := home()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	path := filepath.Join(dir, name+".md")
	if agents, _ := AgentsDir(); dir == agents {
		path = agentDefinition(t, name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// agentDefinition is where an agent's definition lives — asked of config, the
// package that owns the layout, so these tests pin the *rule* rather than a
// second copy of the path that could go on passing after the rule changed.
func agentDefinition(t *testing.T, name string) string {
	t.Helper()
	path, err := config.AgentDefinitionPath(name)
	if err != nil {
		t.Fatalf("AgentDefinitionPath(%q): %v", name, err)
	}
	return path
}

func find(list []Profile, name string) (Profile, bool) {
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// A file in the agents' home that names no desk sits in the office — the
// ordinary hire, written by a person who should not need to know a field
// exists. Naming another desk is allowed and means that desk.
func TestAgentHomeFileSitsInTheOfficeByDefault(t *testing.T) {
	isolate(t)
	writeProfile(t, AgentsDir, "นักแปล", "---\ndescription: แปลเอกสาร\n---\nแปลไทย-อังกฤษ")

	p, ok := Load("นักแปล")
	if !ok {
		t.Fatal("agent-home file did not load")
	}
	if p.Desk != "specialized" {
		t.Fatalf("Desk = %q, want the office", p.Desk)
	}
	if _, ok := find(Chairs("specialized"), "นักแปล"); !ok {
		t.Fatal("the hire is not on the office roster")
	}
	if _, ok := find(Delegates(), "นักแปล"); ok {
		t.Fatal("an agent must not appear among the assistant's own delegates")
	}
}

// The sub-agents' home is closed (owner's call, 2026-08-06): the helpers are
// part of the system, and a user file there — whatever it says — is never read
// as a profile. It must not vanish silently either: a file the user can see on
// disk and cannot explain missing is the debt Conflict exists to refuse.
func TestHelperHomeIsClosedUserFilesAreReportedNotRead(t *testing.T) {
	isolate(t)
	writeProfile(t, Dir, "หลงบ้าน", "---\ndesk: specialized\n---\nอยากเป็นเอเจน")
	writeProfile(t, Dir, "ลูกมือใหม่", "---\ndescription: อยากช่วย\n---\nช่วยทุกอย่าง")

	for _, name := range []string{"หลงบ้าน", "ลูกมือใหม่"} {
		if _, ok := Load(name); ok {
			t.Fatalf("%s: a helper-home user file must not be runnable", name)
		}
		if _, ok := find(List(), name); ok {
			t.Fatalf("%s: a helper-home user file must not reach the list as a profile", name)
		}
		if _, ok := find(Chairs("specialized"), name); ok {
			t.Fatalf("%s: must not reach the office roster", name)
		}
		if _, ok := find(Delegates(), name); ok {
			t.Fatalf("%s: must not reach the delegate roster", name)
		}
		found := false
		for _, c := range Conflicts() {
			if c.Name == name && c.Reason != "" && c.Path != "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: the locked-out file is not reported — it just vanished", name)
		}
	}
}

// The move: a chair file dropped into the shared folder before the homes split
// followed the rules as they stood, and wakes up in its own home rather than
// sick. Twice, because a migration that is not idempotent is a startup crash
// waiting for a second launch.
func TestMigrateMovesOldChairFilesHomeOnce(t *testing.T) {
	isolate(t)
	writeProfile(t, Dir, "นักบัญชี", "---\ndesk: specialized\ndescription: ทำบัญชี\n---\nปิดงบ")
	writeProfile(t, Dir, "ผู้ค้นเว็บ", "---\ndescription: ค้นเว็บ\n---\nหาให้เจอ")

	moved := Migrate()
	if len(moved) != 1 || moved[0] != "นักบัญชี" {
		t.Fatalf("Migrate moved %v, want just the chair file", moved)
	}
	if again := Migrate(); len(again) != 0 {
		t.Fatalf("second Migrate moved %v, want nothing", again)
	}

	p, ok := Load("นักบัญชี")
	if !ok || p.Desk != "specialized" || p.Invalid != "" {
		t.Fatalf("migrated chair = %+v, ok=%v — want healthy, at the office", p, ok)
	}
	if _, err := os.Stat(agentDefinition(t, "นักบัญชี")); err != nil {
		t.Fatal("the moved file is not in the agents' home")
	}
	// The ordinary sub-agent file stays on disk but is no longer read — the
	// helper home is closed, and the file is reported rather than run.
	if _, ok := Load("ผู้ค้นเว็บ"); ok {
		t.Fatal("a leftover helper-home file must not load now that the home is closed")
	}
	if _, ok := findConflict(Conflicts(), "ผู้ค้นเว็บ"); !ok {
		t.Fatal("the leftover helper-home file is not reported")
	}
}

func findConflict(conflicts []Conflict, name string) (Conflict, bool) {
	for _, c := range conflicts {
		if c.Name == name {
			return c, true
		}
	}
	return Conflict{}, false
}

// Migration never overwrites: two files claiming one name is a conflict to
// report, not a race the mover resolves by clobbering whichever came second.
func TestMigrateRefusesToClobberAndReportsTheConflict(t *testing.T) {
	isolate(t)
	writeProfile(t, AgentsDir, "ซ้ำ", "---\n---\nตัวจริง")
	writeProfile(t, Dir, "ซ้ำ", "---\ndesk: specialized\n---\nตัวหลง")

	if moved := Migrate(); len(moved) != 0 {
		t.Fatalf("Migrate moved %v over an existing file", moved)
	}
	conflicts := Conflicts()
	if len(conflicts) != 1 || conflicts[0].Name != "ซ้ำ" || conflicts[0].Reason == "" {
		t.Fatalf("Conflicts() = %+v, want the stranded file with its reason", conflicts)
	}
	// The agents-home file owns the name and keeps working.
	if p, ok := Load("ซ้ำ"); !ok || p.Desk != "specialized" {
		t.Fatalf("owner = %+v, ok=%v — the conflict must not take the owner down", p, ok)
	}
}

// One name, one owner, both directions — memory, jobs and chat history all key
// on the bare name, so the same name as both kinds would be one identity with
// two contradictory files behind it.
func TestANameCannotBeCreatedAsTheOtherKind(t *testing.T) {
	isolate(t)

	// "doc" is a bundled agent; "explore" a bundled sub-agent.
	if err := Save("doc", "---\n---\nปลอมตัวเป็นลูกมือ"); err == nil {
		t.Fatal("Save let a sub-agent take a bundled agent's name")
	} else if !strings.Contains(err.Error(), "เอเจน") {
		t.Fatalf("the refusal does not name the owner: %v", err)
	}
	if err := SaveAgent("explore", "---\n---\nปลอมตัวเป็นเอเจน"); err == nil {
		t.Fatal("SaveAgent let an agent take a bundled sub-agent's name")
	}

	// Each door still edits its own kind, including shadows of bundled files.
	if err := SaveAgent("doc", "---\ndescription: ของฉันเอง\n---\nเอกสารแบบฉัน"); err != nil {
		t.Fatalf("SaveAgent refused the agent's own name: %v", err)
	}
	p, ok := Load("doc")
	if !ok || !p.Overrides || p.Desk != "specialized" {
		t.Fatalf("doc after shadow = %+v, ok=%v — want an override, still at the office", p, ok)
	}
	if _, err := os.Stat(agentDefinition(t, "doc")); err != nil {
		t.Fatal("the agent's shadow landed outside the agents' home")
	}
}

// The generic save door (the settings editor) routes an existing agent's edit
// to the agents' home rather than growing a second copy in the sub-agents'.
func TestSetModelFollowsTheOwnersHome(t *testing.T) {
	isolate(t)
	if err := SetModel("doc", "gpt-6-mini"); err != nil {
		t.Fatalf("SetModel(doc): %v", err)
	}

	if _, err := os.Stat(agentDefinition(t, "doc")); err != nil {
		t.Fatal("the model pin was not written into the agents' home")
	}
	helpersDir, _ := Dir()
	if _, err := os.Stat(filepath.Join(helpersDir, "doc.md")); err == nil {
		t.Fatal("a second doc.md grew in the sub-agents' home")
	}
	p, ok := Load("doc")
	if !ok || p.Model != "gpt-6-mini" || p.Desk != "specialized" {
		t.Fatalf("doc after pin = %+v, ok=%v", p, ok)
	}
}

// What the model is told about who it can hand work to.
//
// The schema used to be one flat list of names under "Which sub-agent to use",
// so the model learned there was one kind of worker and told users exactly
// that — "ซับเอเจน 6 ตัว", with an agent's job described as a chair's. It was
// not hallucinating; it was repeating this string. The product has two kinds,
// so this string has to have two.
func TestTheModelIsToldWhichWorkersAreAgentsAndWhichAreHelpers(t *testing.T) {
	isolate(t)
	choice := agentChoice(List())

	for _, want := range []string{"AGENTS (เอเจน)", "HELPERS (ซับเอเจน)", "doc", "explore"} {
		if !strings.Contains(choice, want) {
			t.Errorf("the agent parameter never mentions %q:\n%s", want, choice)
		}
	}
	// An agent must be described on the agents side, not among the helpers.
	agentsHalf, helpersHalf, split := strings.Cut(choice, "HELPERS")
	if !split {
		t.Fatal("the two kinds are not separated at all")
	}
	if !strings.Contains(agentsHalf, "doc") || strings.Contains(helpersHalf, "doc") {
		t.Errorf("doc is an agent but is not filed as one:\n%s", choice)
	}
	if !strings.Contains(helpersHalf, "explore") || strings.Contains(agentsHalf, "explore") {
		t.Errorf("explore is a helper but is not filed as one:\n%s", choice)
	}
}

// And it must not say what an agent hands back.
//
// It promised "a finished file" for a week, and that clause answered the
// question before the request had been read: a caller told the return type has
// to write every brief as an order to produce one, so "ask doc how a good
// document is put together" went out as "write a manual about…" and came back
// as a manual. doc's own rule against answering a question with a file could
// not save it — the brief that reached doc had no question left in it.
//
// Owner's call, 12 ส.ค., and the general form is the reason it is pinned: a
// specialist has its own instructions and its own tools and already knows what
// its work looks like, so a return type stated up here is a second answer to a
// question that has one. Any wording that puts one back belongs in the agent's
// own file, per job — never in the schema every request is written against.
func TestTheAgentParameterDoesNotDecideWhatComesBack(t *testing.T) {
	isolate(t)
	choice := strings.ToLower(agentChoice(List()))

	for _, promise := range []string{"finished file", "returns a file", "as a file", "a document", "produces a file"} {
		if strings.Contains(choice, promise) {
			t.Errorf("the agent parameter promises %q — the shape of the answer is the agent's to pick:\n%s",
				promise, choice)
		}
	}
}

// A profile's description says what the JOB is. Its kind is decided by which
// home the file lives in, so a kind-word inside the description is a second
// place answering "what kind is this" — and it is the one that goes stale: the
// bundled files still said "เก้าอี้ทำสไลด์" and "ซับเอเจนค้นไฟล์" for a day
// after the split, and the model read them out to users verbatim.
func TestBundledDescriptionsCarryNoKindWord(t *testing.T) {
	isolate(t)
	for _, p := range List() {
		for _, retired := range []string{"เก้าอี้", "ซับเอเจน", "ออฟฟิศ"} {
			if strings.Contains(p.Description, retired) {
				t.Errorf("%s's description says %q — the home already decides the kind: %q",
					p.Name, retired, p.Description)
			}
		}
	}
}

// One agent is one folder (owner's call, 2026-08-06). What this pins is not
// the folder itself — it is that an install written under the flat layout keeps
// working without the user moving anything, and that nothing is left behind for
// a later reader to pick up as a second answer.
func TestFlatAgentFilesMoveIntoTheirFolder(t *testing.T) {
	isolate(t)
	dir, err := AgentsDir()
	if err != nil {
		t.Fatalf("AgentsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	flat := filepath.Join(dir, "นักแปล.md")
	if err := os.WriteFile(flat, []byte("---\ndescription: แปลเอกสาร\n---\nแปลไทย-อังกฤษ"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, ok := Load("นักแปล")
	if !ok || p.Description != "แปลเอกสาร" {
		t.Fatalf("flat-layout agent = %+v, ok=%v — the migration lost it", p, ok)
	}
	if _, err := os.Stat(agentDefinition(t, "นักแปล")); err != nil {
		t.Fatalf("the definition is not in the agent's folder: %v", err)
	}
	if _, err := os.Stat(flat); !os.IsNotExist(err) {
		t.Fatal("the flat file is still there — two files, one name, and only one is read")
	}
}

// Delete means two different things and the folder is what makes them
// different. Reverting a shipped agent the user edited must not throw away what
// that worker learned: the memory belongs to the name, across every rewrite of
// the brief. Deleting an agent the user hired takes the whole folder, because
// the name is gone and a stranger's notes must not seed the next one.
func TestRevertKeepsMemoryAndDeleteTakesTheFolder(t *testing.T) {
	isolate(t)

	// A shipped agent the user edited: reverting drops the edit, keeps memory.
	if err := SaveAgent("doc", "---\ndescription: ของฉันเอง\n---\nเขียนแบบฉัน"); err != nil {
		t.Fatalf("SaveAgent(doc): %v", err)
	}
	memory, err := config.AgentMemoryPath("doc")
	if err != nil {
		t.Fatalf("AgentMemoryPath: %v", err)
	}
	if err := os.WriteFile(memory, []byte("เจ้านายชอบตารางสั้น"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	if err := Delete("doc"); err != nil {
		t.Fatalf("Delete(doc): %v", err)
	}
	if p, ok := Load("doc"); !ok || p.Overrides || !p.Builtin {
		t.Fatalf("doc after revert = %+v, ok=%v — want the bundled one back", p, ok)
	}
	if _, err := os.Stat(memory); err != nil {
		t.Fatalf("reverting the brief destroyed what the agent had learned: %v", err)
	}

	// An agent the user hired: deleting takes the folder, memory included.
	if err := SaveAgent("นักแปล", "---\ndescription: แปลเอกสาร\n---\nแปลไทย-อังกฤษ"); err != nil {
		t.Fatalf("SaveAgent(นักแปล): %v", err)
	}
	hired, err := config.AgentMemoryPath("นักแปล")
	if err != nil {
		t.Fatalf("AgentMemoryPath: %v", err)
	}
	if err := os.WriteFile(hired, []byte("จำสำนวนที่เจ้านายชอบ"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	if err := Delete("นักแปล"); err != nil {
		t.Fatalf("Delete(นักแปล): %v", err)
	}
	if _, ok := Load("นักแปล"); ok {
		t.Fatal("the hired agent is still loadable after being deleted")
	}
	home, err := config.AgentHome("นักแปล")
	if err != nil {
		t.Fatalf("AgentHome: %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("the deleted agent's folder is still on disk — nothing lists it, and its memory would seed the next agent to take the name")
	}
}

// The `chairs:` split, end to end (owner's call, 2026-08-06). The specialized
// desk stopped carrying the writers so the assistant sitting there hands the
// job over — and the whole point is that the agent it hands to must still be
// able to do the work. A ceiling that answered the desk's question for a chair
// too would leave the office unable to make the one thing it exists to make,
// and it would fail silently: the chair keeps its reading tools, accepts the
// brief, and simply never writes a file.
func TestAChairKeepsTheWritersTheDeskPutDown(t *testing.T) {
	isolate(t)
	desk, ok := mode.Load("specialized")
	if !ok || desk == nil {
		t.Fatal("specialized desk missing")
	}
	parent := skill.NewRegistry()
	for _, s := range []skill.Skill{stubTool("doc_write"), stubTool("read"), stubTool("shell"), stubTool("symbol")} {
		if err := parent.Register(s, skill.SourceBuiltin); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	chair, ok := Load("doc")
	if !ok {
		t.Fatal("the doc agent is missing")
	}
	got := FilterRegistry(parent, chair, desk).Names()
	if !slices.Contains(got, "doc_write") {
		t.Fatalf("the doc agent was handed %v — without doc_write it takes the job and writes nothing", got)
	}
	// Shell is in the room on purpose (mode.CarriesForChair), so a chair whose
	// profile asks for one gets it — the doc agent has asked since 2026-08-19.
	// What the ceiling still refuses is a tool the desk names nowhere, and the
	// profile asking for it changes nothing: that is the half worth testing.
	past := Profile{Name: "doc", Desk: mode.Office, Tools: []string{"doc_write", "symbol"}}
	if got := FilterRegistry(parent, past, desk).Names(); slices.Contains(got, "symbol") {
		t.Errorf("the chair reached past the desk: %v", got)
	}

	// A delegate is the caller's own hands in a second context, so it is
	// answered by the desk's own question and `chairs:` is not for it.
	helper, ok := Load("general")
	if !ok {
		t.Fatal("the general helper is missing")
	}
	if got := FilterRegistry(parent, helper, desk).Names(); slices.Contains(got, "doc_write") {
		t.Errorf("a plain delegate picked up a chairs-only tool: %v", got)
	}
}

// stubTool is a name in a registry and nothing else — FilterRegistry only ever
// asks a skill for its name and its source.
type stubTool string

func (s stubTool) Name() string        { return string(s) }
func (s stubTool) Description() string { return string(s) }
func (s stubTool) Execute(context.Context, skill.Input) (skill.Output, error) {
	return skill.Output{}, nil
}

// The Settings page offers an "agent:<name>" toggle per MCP server. This is the
// half that makes it mean something: a server the user pointed at an agent has
// to reach that agent's registry, and no one else's.
//
// It shipped for a day as a switch that saved to disk and changed nothing —
// worse than no switch, because a control that looks like it worked is one the
// user stops checking. What it pins now is the whole path: on for the named
// agent, off for another agent, off for the desk's own assistant, and off for a
// plain delegate.
func TestAServerPointedAtAnAgentReachesOnlyThatAgent(t *testing.T) {
	isolate(t)
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "notion", Command: []string{"npx", "notion"}, For: []string{config.MCPAgentPrefix + "doc"}},
		{Name: "linear", Command: []string{"npx", "linear"}, For: []string{"specialized"}},
	}); err != nil {
		t.Fatal(err)
	}
	desk, ok := mode.Load("specialized")
	if !ok {
		t.Fatal("specialized desk missing")
	}

	parent := skill.NewRegistry()
	for _, s := range []skill.Skill{stubTool("notion_search"), stubTool("linear_issue"), stubTool("read")} {
		source := skill.SourceMCP
		if s.Name() == "read" {
			source = skill.SourceBuiltin
		}
		if err := parent.Register(s, source); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	toolsFor := func(name string) []string {
		t.Helper()
		p, ok := Load(name)
		if !ok {
			t.Fatalf("profile %q missing", name)
		}
		return FilterRegistry(parent, p, desk).Names()
	}

	if got := toolsFor("doc"); !slices.Contains(got, "notion_search") {
		t.Errorf("the doc agent did not get the server pointed at it: %v", got)
	}
	// Pointed at one agent, not at the team.
	if got := toolsFor("sheet"); slices.Contains(got, "notion_search") {
		t.Errorf("a server pointed at doc reached sheet as well: %v", got)
	}
	// A server pointed at the *desk* furnishes the room, so everybody working
	// in it has it. The two placements are different acts and both are the
	// user's: the desk is "anyone here may use this", the agent is "this one".
	//
	// It read the opposite until 31 ส.ค. — a desk-wide server was filtered by
	// the agent's own `tools:` first — and that reading died with the
	// allowlists. It could not have survived them anyway: an author cannot list
	// tool names that arrive from a server months later, which is why an
	// agent-pointed server already skipped the list (Profile.Permits). The one
	// gate is the toggle the user can see, and it is the only one.
	for _, who := range []string{"doc", "sheet"} {
		if got := toolsFor(who); !slices.Contains(got, "linear_issue") {
			t.Errorf("%s did not get the server pointed at its own desk: %v", who, got)
		}
	}
	// A plain delegate is the caller's own hands, so it gets the desk's answer
	// and nothing an agent was pointed at.
	if got := toolsFor("general"); slices.Contains(got, "notion_search") {
		t.Errorf("a plain delegate picked up an agent's server: %v", got)
	}

	// And the desk's own assistant never carries it: pointing a server at an
	// agent is precisely how a tool stays off the main context.
	if desk.Carries("notion_search", skill.SourceMCP) {
		t.Error("the desk's assistant carries a server pointed at an agent")
	}
}

// MCP servers land in the parent registry asynchronously, and the child's copy
// is taken at dispatch. A job started in the seconds after a launch — or right
// after the user flips the toggle, which rebuilds the engine — could otherwise
// hand the agent a registry without the server it was given for the job. It
// would accept the brief, work without the tool, and return something that
// reads like an answer.
//
// Sometimes-wrong-and-silent is the failure mode worth spending code on.
func TestADispatchIsRefusedWhileTheAgentsServerIsStillConnecting(t *testing.T) {
	isolate(t)
	if err := config.SaveMCPServers([]config.MCPServerConfig{
		{Name: "notion", Command: []string{"npx", "notion"}, For: []string{config.MCPAgentPrefix + "doc"}},
	}); err != nil {
		t.Fatal(err)
	}
	doc, ok := Load("doc")
	if !ok {
		t.Fatal("the doc agent is missing")
	}

	// Mid-connect: the built-ins are registered, the server's tools are not.
	connecting := skill.NewRegistry()
	if err := connecting.Register(stubTool("read"), skill.SourceBuiltin); err != nil {
		t.Fatal(err)
	}
	missing := missingAgentServers(connecting, doc)
	if len(missing) != 1 || missing[0] != "notion" {
		t.Fatalf("missingAgentServers = %v, want [notion] so the refusal can name it", missing)
	}

	// Connected: nothing is missing and the job runs.
	connected := skill.NewRegistry()
	if err := connected.Register(stubTool("read"), skill.SourceBuiltin); err != nil {
		t.Fatal(err)
	}
	if err := connected.Register(stubTool("notion_search"), skill.SourceMCP); err != nil {
		t.Fatal(err)
	}
	if missing := missingAgentServers(connected, doc); len(missing) != 0 {
		t.Errorf("missingAgentServers = %v after the server connected, want none", missing)
	}

	// An agent nobody pointed a server at is never held up by this.
	sheet, _ := Load("sheet")
	if missing := missingAgentServers(connecting, sheet); len(missing) != 0 {
		t.Errorf("an agent with no servers of its own was blocked: %v", missing)
	}
}
