package subagent

// The shipped workers held to the same standard as anything sold.
//
// A bundled agent is the only worker whose folder nobody ever assembled by
// hand, so it is the one most likely to have drifted from the layout without
// anyone noticing — and it is also what a seller starts from, because "copy a
// shipped agent and change it" is the documented way to make one. If the five
// that ship cannot be packed up and read back, the standard is a document
// rather than a format.

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/agentpkg"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

func TestEveryShippedAgentIsAValidPackage(t *testing.T) {
	isolate(t)
	shipped := []string{}
	for _, p := range List() {
		if p.Desk != "" && p.Builtin {
			shipped = append(shipped, p.Name)
		}
	}
	if len(shipped) == 0 {
		t.Fatal("no bundled agents found at all")
	}
	t.Logf("bundled agents: %v", shipped)

	for _, name := range shipped {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), name+".zip")
			res, err := agentpkg.Export(dest, agentpkg.Options{Name: name, Sources: PackageSources(name)})
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			r, err := zip.OpenReader(dest)
			if err != nil {
				t.Fatalf("the package it wrote cannot be opened: %v", err)
			}
			defer r.Close()

			var files []string
			for _, f := range r.File {
				files = append(files, f.Name)
			}
			t.Logf("%d files: %v", res.Files, files)

			has := func(want string) bool {
				for _, f := range files {
					if f == want {
						return true
					}
				}
				return false
			}
			if !has(config.AgentDefinitionFile) {
				t.Errorf("no %s at the root", config.AgentDefinitionFile)
			}
			if !has(config.AgentStartersFile) {
				t.Errorf("no %s — a shipped worker opens a chat with its own question, and that has to travel", config.AgentStartersFile)
			}
			// Its knowledge, if it has any. Reported rather than required: one
			// shipped worker genuinely carries none yet.
			skills := 0
			for _, f := range files {
				if strings.HasPrefix(f, config.AgentSkillsDir+"/") && strings.HasSuffix(f, "SKILL.md") {
					skills++
				}
			}
			own, _ := OwnSkills(name)
			if skills != len(own) {
				t.Errorf("carries %d skills but %d travelled", len(own), skills)
			}
			if skills == 0 {
				t.Logf("NOTE: %s ships with no skills of its own", name)
			}
			if has(config.AgentMemoryFile) {
				t.Errorf("%s travelled", config.AgentMemoryFile)
			}
		})
	}
}

// The whole road, end to end: take a worker that ships inside the binary, pack
// it, and unpack it under a different name into a data root that has never seen
// it — which is what a buyer's machine is. Then ask the doors.
//
// Renamed on the way in deliberately. The local id is the folder name and the
// standard says a buyer may change it; a round trip that kept the name would
// prove the folder copies and not that the name is theirs.
func TestAShippedAgentSurvivesTheRoundTrip(t *testing.T) {
	isolate(t)
	dest := filepath.Join(t.TempDir(), "github.zip")
	if _, err := agentpkg.Export(dest, agentpkg.Options{Name: "github", Sources: PackageSources("github")}); err != nil {
		t.Fatalf("export: %v", err)
	}

	// A fresh machine: a data root with nothing in it.
	fresh := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", fresh)
	newName := "จัดรีโป"
	home, err := config.AgentHome(newName)
	if err != nil {
		t.Fatal(err)
	}
	unzipInto(t, dest, home)

	p, ok := Load(newName)
	if !ok {
		t.Fatal("the unpacked folder is not a worker")
	}
	if p.Invalid != "" {
		t.Fatalf("Invalid = %q", p.Invalid)
	}
	if p.Desk != "specialized" {
		t.Errorf("desk = %q", p.Desk)
	}
	if p.Description == "" {
		t.Error("no description survived, so nobody would ever be sent work")
	}
	onRoster := false
	for _, c := range Chairs(p.Desk) {
		if c.Name == newName {
			onRoster = true
		}
	}
	if !onRoster {
		t.Error("not on the roster under its new name")
	}
	own, _ := OwnSkills(newName)
	if len(own) != 4 {
		t.Errorf("carries %d skills, want the 4 that shipped: %v", len(own), own)
	}
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	if _, ok := FilterRegistry(parent, p, nil).Get("pr-workflow"); !ok {
		t.Error("a skill that made the trip is not in the running registry")
	}
	if set := Starters(newName, "th"); set.Headline == "" || len(set.Cards) == 0 {
		t.Errorf("the opening question did not survive: %+v", set)
	}
	// English too: the standard says the home wins for every language, so a
	// worker that was translated arrives translated.
	if set := Starters(newName, "en"); set.Headline == "" {
		t.Error("STARTERS.en.md did not survive the trip")
	}
	// What it needs is declared and still unmet on a machine that has nothing.
	// That is the correct answer, and it is the one the install road exists to
	// change.
	reqs := Requirements(p)
	if len(reqs) == 0 {
		t.Fatal("the needs line did not survive")
	}
	for _, r := range reqs {
		if r.Met {
			t.Errorf("need %q reports met on a machine with nothing connected", r.Entry)
		}
	}
}

// unzipInto is the buyer's hand: extract, nothing else.
func unzipInto(t *testing.T, archive, dir string) {
	t.Helper()
	r, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
