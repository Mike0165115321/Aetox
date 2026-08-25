package subagent

// "Hiring one is dropping one more file in the agents home" is the sentence the
// whole package standard rests on (COMPANY.md §4). These two tests are what
// keeps it from being a sentence: one copies a complete folder in and asks every
// door whether the worker is there, and the other copies in folders that are
// wrong and checks that each one says so while still behaving exactly as it did.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// dropPackage writes a whole package folder the way a person copying one in
// would: files, nothing registered, no restart.
func dropPackage(t *testing.T, name string, files map[string]string) {
	t.Helper()
	home, err := config.AgentHome(name)
	if err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Every door a colleague has to be reachable through, asked of a folder that
// was copied in and nothing else. The one door that does not open is named
// rather than skipped: an mcp.json in a hand-copied folder is inert until the
// install road lands (standard v2, slice 2).
func TestDroppedFolderIsAWorkingColleague(t *testing.T) {
	isolate(t)
	name := "ทนาย"
	dropPackage(t, name, map[string]string{
		"AGENT.md": `---
description: ตรวจและร่างสัญญา
tools: read, grep, list, skills_list, skill_view
needs: mcp:legal-mcp
publisher: Mike
package: mike/lawyer
version: 1.0.0
requires-app: 1.4.0
icon: scale
---
คุณคือทนายของบริษัทนี้
`,
		"skills/สัญญาเช่า/SKILL.md":               "---\nname: สัญญาเช่า\ndescription: ข้อที่สัญญาเช่าต้องมี\n---\n\nแปดข้อ\n",
		"skills/สัญญาเช่า/references/ตัวอย่าง.md": "ตัวอย่าง",
		"STARTERS.md": "# วันนี้ให้ช่วยเรื่องอะไร\n\n- ตรวจสัญญา | ช่วยตรวจสัญญาฉบับนี้ให้หน่อย: | fileText\n- ร่างใหม่ | ร่างสัญญาเช่าจากเงื่อนไขนี้: | penLine\n",
		"mcp.json":    `[{"name":"legal-mcp","command":["npx","-y","@example/legal-mcp"],"environment":{"LEGAL_TOKEN":"${ask:LEGAL_TOKEN}"}}]`,
	})

	p, ok := Load(name)
	if !ok {
		t.Fatalf("the folder was copied in and Load does not see a worker")
	}
	if p.Invalid != "" {
		t.Fatalf("Invalid = %q", p.Invalid)
	}
	if p.Desk != "specialized" {
		t.Errorf("desk = %q, want the office — a package must never have to write this itself", p.Desk)
	}
	if p.Version != "1.0.0" || p.Package != "mike/lawyer" {
		t.Errorf("the shipping label did not survive the copy: %+v", p)
	}

	onRoster := false
	for _, c := range Chairs(p.Desk) {
		if c.Name == name {
			onRoster = true
		}
	}
	if !onRoster {
		t.Error("not on the office roster, which is also the chat picker — a colleague nobody can reach")
	}
	if k := KindOf(name); k != KindAgent {
		t.Errorf("KindOf = %q, want %q, or the assistant cannot hand it a job", k, KindAgent)
	}

	own, errs := OwnSkills(name)
	if len(own) != 1 || own[0].Name != "สัญญาเช่า" {
		t.Fatalf("OwnSkills = %v (errs %v)", own, errs)
	}

	// Registered is not the same as reachable — that gap cost two releases once
	// (see skills.go). Both are asserted, and the second is the one that matters.
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	reg := FilterRegistry(parent, p, nil)
	if _, ok := reg.Get("สัญญาเช่า"); !ok {
		t.Error("the skill in the copied folder is not in the running registry")
	}
	if _, ok := reg.Get("skills_list"); !ok {
		t.Error("no skills_list, so neither its own shelf nor the machine's is readable")
	}

	set := Starters(name, "th")
	if set.Headline == "" || len(set.Cards) != 2 {
		t.Errorf("STARTERS.md did not reach the empty chat: headline=%q cards=%d", set.Headline, len(set.Cards))
	}

	// The door that does not open. The folder declares a server and brings its
	// own mcp.json; nothing reads it, so the machine has not configured it and
	// the unmet need is the honest report of that.
	if got := config.MCPServersForAgent(name); len(got) != 0 {
		t.Fatalf("something now reads a package's mcp.json (%v) — the standard's slice 2 has landed, and this test and the doc's measured table should say so", got)
	}
	reqs := Requirements(p)
	if len(reqs) != 1 || reqs[0].Met {
		t.Fatalf("needs = %+v, want the declared server reported unmet", reqs)
	}
}

// The two that used to be silent. Trap 1 now says something and still behaves
// exactly as it did; trap 2 is reported by the roster, in desktop/office_test.go,
// because only a live registry can tell what a worker actually got handed.
func TestDroppedFolderThatIsWrongSaysSo(t *testing.T) {
	isolate(t)

	// A package that writes its own desk keeps working, at the desk it named,
	// and is no longer invisible about being missing from the office.
	dropPackage(t, "ก", map[string]string{"AGENT.md": "---\ndescription: d\ndesk: coding\n---\nbrief"})
	p, ok := Load("ก")
	if !ok {
		t.Fatal("ก did not load at all")
	}
	if p.Invalid != "" {
		t.Fatalf("Invalid = %q — a notice must never become a removal", p.Invalid)
	}
	if p.Desk != "coding" {
		t.Fatalf("desk = %q — the line the user typed was rewritten, which is this package overruling them", p.Desk)
	}
	if p.Notice == "" {
		t.Fatal("a folder writing desk: coding is still silent about vanishing from the office")
	}
	if !strings.Contains(p.Notice, "coding") {
		t.Errorf("the notice does not name the desk that caused it: %q", p.Notice)
	}
	for _, c := range Chairs("specialized") {
		if c.Name == "ก" {
			t.Fatal("behaviour changed: it now reaches the office, which this fix was not allowed to do")
		}
	}

	// And the ordinary case stays quiet. A notice on a healthy worker is worse
	// than no notice at all, because it teaches people to ignore the line.
	dropPackage(t, "ข", map[string]string{"AGENT.md": "---\ndescription: d\n---\nbrief"})
	q, _ := Load("ข")
	if q.Notice != "" || q.Invalid != "" {
		t.Fatalf("a healthy worker was given something to say: notice=%q invalid=%q", q.Notice, q.Invalid)
	}
	if q.Desk != "specialized" {
		t.Errorf("desk = %q, want the office by default", q.Desk)
	}

	// A worker that names a tool this build does not have still runs with what
	// is left — unchanged, deliberately. What it gets is now reported by the
	// roster rather than by refusing the file.
	dropPackage(t, "ค", map[string]string{"AGENT.md": "---\ndescription: d\ntools: read, slides_write\n---\nbrief"})
	r, _ := Load("ค")
	if r.Invalid != "" || r.Notice != "" {
		t.Fatalf("an unknown tool became a file-level complaint: invalid=%q notice=%q", r.Invalid, r.Notice)
	}
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	if got := FilterRegistry(parent, r, nil).Names(); len(got) != 1 || got[0] != "read" {
		t.Fatalf("the worker holds %v — behaviour changed", got)
	}
}
