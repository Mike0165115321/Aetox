package subagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two homes (owner's call, 2026-08-05): a file's home is its kind, decided
// in resolve() and nowhere else. What these tests pin is not that folders
// exist — it is the three rules that make the split hold: home decides kind,
// old files move home once, and a name has exactly one owner.

func writeProfile(t *testing.T, home func() (string, error), name, body string) string {
	t.Helper()
	dir, err := home()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
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

// A sub-agent-home file that claims a desk is a contradiction between where it
// sits and what it says. It is refused loudly — visible in the list with its
// reason — and never run, because either quiet reading (obey the field, or
// strip it) would be this package overriding something the user wrote.
func TestHelperHomeFileClaimingADeskIsSickNotReinterpreted(t *testing.T) {
	isolate(t)
	writeProfile(t, Dir, "หลงบ้าน", "---\ndesk: specialized\n---\nอยากเป็นตัวแทน")

	p, ok := find(List(), "หลงบ้าน")
	if !ok {
		t.Fatal("a sick file must stay visible — a file the user can see on disk cannot just vanish from the app")
	}
	if p.Invalid == "" {
		t.Fatal("the contradiction carries no reason")
	}
	if _, ok := Load("หลงบ้าน"); ok {
		t.Fatal("a sick file must not be runnable")
	}
	if _, ok := find(Chairs("specialized"), "หลงบ้าน"); ok {
		t.Fatal("a sick file must not reach the office roster")
	}
	if _, ok := find(Delegates(), "หลงบ้าน"); ok {
		t.Fatal("a sick file must not reach the delegate roster either")
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
	agentsDir, _ := AgentsDir()
	if _, err := os.Stat(filepath.Join(agentsDir, "นักบัญชี.md")); err != nil {
		t.Fatal("the moved file is not in the agents' home")
	}
	if h, ok := Load("ผู้ค้นเว็บ"); !ok || h.Desk != "" {
		t.Fatal("the ordinary sub-agent must stay where it was")
	}
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

	// "deck" is a bundled agent; "explore" a bundled sub-agent.
	if err := Save("deck", "---\n---\nปลอมตัวเป็นลูกมือ"); err == nil {
		t.Fatal("Save let a sub-agent take a bundled agent's name")
	} else if !strings.Contains(err.Error(), "ตัวแทน") {
		t.Fatalf("the refusal does not name the owner: %v", err)
	}
	if err := SaveAgent("explore", "---\n---\nปลอมตัวเป็นตัวแทน"); err == nil {
		t.Fatal("SaveAgent let an agent take a bundled sub-agent's name")
	}

	// Each door still edits its own kind, including shadows of bundled files.
	if err := SaveAgent("deck", "---\ndescription: ของฉันเอง\n---\nสไลด์แบบฉัน"); err != nil {
		t.Fatalf("SaveAgent refused the agent's own name: %v", err)
	}
	p, ok := Load("deck")
	if !ok || !p.Overrides || p.Desk != "specialized" {
		t.Fatalf("deck after shadow = %+v, ok=%v — want an override, still at the office", p, ok)
	}
	agentsDir, _ := AgentsDir()
	if _, err := os.Stat(filepath.Join(agentsDir, "deck.md")); err != nil {
		t.Fatal("the agent's shadow landed outside the agents' home")
	}
}

// The generic save door (the settings editor) routes an existing agent's edit
// to the agents' home rather than growing a second copy in the sub-agents'.
func TestSetModelFollowsTheOwnersHome(t *testing.T) {
	isolate(t)
	if err := SetModel("deck", "gpt-6-mini"); err != nil {
		t.Fatalf("SetModel(deck): %v", err)
	}

	agentsDir, _ := AgentsDir()
	if _, err := os.Stat(filepath.Join(agentsDir, "deck.md")); err != nil {
		t.Fatal("the model pin was not written into the agents' home")
	}
	helpersDir, _ := Dir()
	if _, err := os.Stat(filepath.Join(helpersDir, "deck.md")); err == nil {
		t.Fatal("a second deck.md grew in the sub-agents' home")
	}
	p, ok := Load("deck")
	if !ok || p.Model != "gpt-6-mini" || p.Desk != "specialized" {
		t.Fatalf("deck after pin = %+v, ok=%v", p, ok)
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

	for _, want := range []string{"AGENTS (ตัวแทน)", "HELPERS (ผู้ช่วยตัวแทน)", "deck", "explore"} {
		if !strings.Contains(choice, want) {
			t.Errorf("the agent parameter never mentions %q:\n%s", want, choice)
		}
	}
	// An agent must be described on the agents side, not among the helpers.
	agentsHalf, helpersHalf, split := strings.Cut(choice, "HELPERS")
	if !split {
		t.Fatal("the two kinds are not separated at all")
	}
	if !strings.Contains(agentsHalf, "deck") || strings.Contains(helpersHalf, "deck") {
		t.Errorf("deck is an agent but is not filed as one:\n%s", choice)
	}
	if !strings.Contains(helpersHalf, "explore") || strings.Contains(agentsHalf, "explore") {
		t.Errorf("explore is a helper but is not filed as one:\n%s", choice)
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
