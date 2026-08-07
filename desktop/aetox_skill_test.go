package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/audit"
	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/learned"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// The bundled `aetox` skill is the assistant's answer to "where does Aetox keep
// its own things". A document like that fails in one direction only — a path
// moves, the file keeps saying the old one, and nothing anywhere goes red. The
// assistant then reports the wrong folder with exactly as much confidence as it
// reported the right one, which is the failure mode the whole skill exists to
// prevent, reintroduced by the skill itself.
//
// So every path it names is checked against the function that produces it. When
// a constant moves, this test fails and names the line to fix.
//
// It lives in the desktop package for the same reason tool_budget_test.go does:
// only here is the whole map visible at once. internal/skill cannot import mode
// or subagent (they import it), and spacesRoot/outputSubdir/unfocusedRoot are
// unexported here. Its other half — the credential denylist and the skills
// directory — is checked in internal/skill/bundled_skills_test.go, where those
// values live.
//
// Read off disk rather than through the registry, the same way
// providers_test.go reads the frontend's .ts: the assertion is about what the
// file says, and going through the loader would let a file that fails to embed
// pass as an empty string.
func aetoxSkillBody(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "internal", "skill", "skills", "aetox", "SKILL.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// mustRel is the documented spelling of a path: relative to the root it hangs
// off, slash-separated, which is how the document writes it on every platform.
func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("%q is not under %q: %v", target, base, err)
	}
	return filepath.ToSlash(rel)
}

func TestTheAetoxSkillNamesTheRealDataRootPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	body := aetoxSkillBody(t)

	cases := []struct {
		what string
		fn   func() (string, error)
	}{
		{"identity layer", config.IdentityDir},
		{"agent memory", learned.Dir},
		{"desks", mode.Dir},
		{"ตัวแทน", config.AgentsRoot},
		{"ผู้ช่วยตัวแทน", subagent.Dir},
		{"โปรเจกต์", spacesRoot},
		{"prompt presets", command.PresetsDir},
		{"MCP servers", config.MCPServersPath},
		{"permissions", config.PermissionsPath},
		{"hooks", config.HooksPath},
		{"provider keys", config.CredentialsPath},
		{"model preference", config.PreferencePath},
		{"env file", config.EnvFilePath},
		{"shell audit log", audit.ShellAuditLogPath},
	}
	for _, c := range cases {
		got, err := c.fn()
		if err != nil {
			t.Fatalf("%s: %v", c.what, err)
		}
		want := "`<DataRoot>/" + mustRel(t, root, got) + "`"
		if !strings.Contains(body, want) {
			t.Errorf("%s lives at %s, and the aetox skill never says so — "+
				"add or fix the row that documents it", c.what, want)
		}
	}
}

// One agent is one folder, and the two filenames inside it are what makes the
// folder readable to a person. The skill names both; if either constant is
// renamed, so must the document be.
func TestTheAetoxSkillNamesTheAgentFolderLayout(t *testing.T) {
	body := aetoxSkillBody(t)
	for _, name := range []string{config.AgentDefinitionFile, config.AgentMemoryFile} {
		if !strings.Contains(body, name) {
			t.Errorf("an agent's folder holds %s and the aetox skill never names it", name)
		}
	}
}

// The database is the one path with no function behind it — the filename is
// written inline where the DSN is built. So it is checked by opening one and
// looking at what appeared on disk, which is a stronger check than reading the
// constant back would have been.
func TestTheAetoxSkillNamesTheRealDatabaseFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	a := &App{ctx: context.Background(), emit: func(string, ...any) {}}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	if _, err := a.database(); err != nil {
		t.Fatalf("open database: %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read data root: %v", err)
	}
	var dbFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".db") {
			dbFile = e.Name()
			break
		}
	}
	if dbFile == "" {
		t.Fatal("opening the database created no .db file under the data root")
	}
	want := "`<DataRoot>/" + dbFile + "`"
	if !strings.Contains(aetoxSkillBody(t), want) {
		t.Errorf("chat history is in %s and the aetox skill never says so", want)
	}
}

// Where a brand new file lands is the question users ask most and the one the
// assistant got wrong most cheaply, because the answer is assembled from two
// halves in two files. Both halves are checked, and so is the joined form the
// document promises.
func TestTheAetoxSkillNamesTheRealOutputLocation(t *testing.T) {
	home := isolateUserDirs(t)
	body := aetoxSkillBody(t)

	root := unfocusedRoot()
	if root == "" {
		t.Fatal("unfocusedRoot is empty under an isolated home")
	}
	wantRoot := "`<home>/" + mustRel(t, home, root) + "`"
	if !strings.Contains(body, wantRoot) {
		t.Errorf("the unfocused working root is %s and the aetox skill never says so", wantRoot)
	}

	a := &App{ctx: context.Background(), emit: func(string, ...any) {}, sessionID: newSessionID()}
	sub := a.outputSubdir()
	if sub == "" {
		t.Fatal("outputSubdir is empty for an unfocused session with an id")
	}
	// The session id varies, so the folder is what is checked — the document
	// writes it as output/<session>, which is the shape, not the value.
	wantSub := "`" + strings.TrimSuffix(sub, a.sessionID) + "<session>`"
	if !strings.Contains(body, wantSub) {
		t.Errorf("new files go to %s and the aetox skill never says so", wantSub)
	}
	if !strings.Contains(body, "`<home>/"+mustRel(t, home, root)+"/"+strings.TrimSuffix(sub, a.sessionID)+"<session>`") {
		t.Error("the aetox skill never writes the joined absolute destination; " +
			"the two halves are only useful together")
	}
}

// The two folder names that are constants rather than paths: a project's
// context folder, and where chat attachments are copied.
func TestTheAetoxSkillNamesTheFolderConstants(t *testing.T) {
	body := aetoxSkillBody(t)
	if !strings.Contains(body, "`<DataRoot>/project/<name>/"+contextDirName+"/`") {
		t.Errorf("a project's context files live in %q and the aetox skill never says so", contextDirName)
	}
	if !strings.Contains(body, "`<sandbox root>/"+attachmentsDir+"/<session>/`") {
		t.Errorf("attachments are copied into %q and the aetox skill never says so", attachmentsDir)
	}
}

// The whole point of the file is that it costs nothing until somebody opens it.
// A bundled skill that started arriving in the tool block would be the mistake
// this design was chosen to avoid, and the budget test would report it as a
// tool nobody could name.
func TestTheBundledSkillIsNotInTheToolBlock(t *testing.T) {
	isolateUserDirs(t)
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	if _, ok := a.registry.Get("aetox"); !ok {
		t.Fatal("the bundled skill is not registered at all — it would never reach skills_list either")
	}
	for _, d := range skill.NewDispatcher(a.registry).ToolDefinitions() {
		if d.Function.Name == "aetox" {
			t.Fatal("the bundled aetox skill is being sent as a tool definition; " +
				"it must be reachable only through skills_list/skill_view")
		}
	}
}
