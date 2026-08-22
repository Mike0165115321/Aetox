package agentpkg

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func agentFS(extra map[string]string) fstest.MapFS {
	m := fstest.MapFS{
		"AGENT.md":                  &fstest.MapFile{Data: []byte("---\ndescription: d\n---\nbrief\n")},
		"STARTERS.md":               &fstest.MapFile{Data: []byte("# hi\n")},
		"skills/invoice/SKILL.md":   &fstest.MapFile{Data: []byte("---\nname: invoice\n---\nbody\n")},
		"skills/invoice/refs/a.txt": &fstest.MapFile{Data: []byte("a")},
	}
	for k, v := range extra {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func names(t *testing.T, archive string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer r.Close()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = string(body)
	}
	return out
}

// The one rule that cannot be a checkbox: what a worker learned doing the
// seller's jobs is the seller's, and a buyer must never receive it.
func TestExportLeavesMemoryBehind(t *testing.T) {
	src := agentFS(map[string]string{"MEMORY.md": "ลูกค้า: บริษัทจริงจำกัด, ใบกำกับ 2026-0041"})
	dest := filepath.Join(t.TempDir(), "doc.zip")

	res, err := Export(dest, Options{Name: "doc", Sources: []fs.FS{src}})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got := names(t, dest)
	if _, ok := got[config.AgentMemoryFile]; ok {
		t.Fatalf("MEMORY.md travelled with the package")
	}
	for name, body := range got {
		if strings.Contains(body, "บริษัทจริงจำกัด") {
			t.Fatalf("memory content leaked through %s", name)
		}
	}
	// Reported rather than silently dropped: "where is my MEMORY.md" gets its
	// answer from the screen that did it.
	if len(res.Left) != 1 || res.Left[0] != config.AgentMemoryFile {
		t.Fatalf("Left = %v, want [%s]", res.Left, config.AgentMemoryFile)
	}
}

// A worker the user has half-edited is one worker, not two. The user's file
// wins; everything they never touched still ships.
func TestExportOverlaysUserFolderOverBundled(t *testing.T) {
	user := fstest.MapFS{
		"AGENT.md": &fstest.MapFile{Data: []byte("mine")},
	}
	bundled := agentFS(nil)
	dest := filepath.Join(t.TempDir(), "doc.zip")

	if _, err := Export(dest, Options{Name: "doc", Sources: []fs.FS{user, bundled}}); err != nil {
		t.Fatalf("export: %v", err)
	}
	got := names(t, dest)
	if got["AGENT.md"] != "mine" {
		t.Fatalf("AGENT.md = %q, want the user's copy", got["AGENT.md"])
	}
	if _, ok := got["skills/invoice/SKILL.md"]; !ok {
		t.Fatalf("a shipped skill the user never edited did not travel")
	}
}

// The package carries the server and refuses the token. Anything else is either
// an install that leaves the buyer hand-editing a config file, or a package
// that ships the seller's account.
func TestExportPacksServersAndRefusesSecrets(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "doc.zip")
	res, err := Export(dest, Options{
		Name:    "doc",
		Sources: []fs.FS{agentFS(nil)},
		Servers: []config.MCPServerConfig{{
			Name:        "notion",
			Command:     []string{"npx", "-y", "@notion/mcp"},
			Environment: map[string]string{"NOTION_TOKEN": "secret_live_value", "NOTION_LOCALE": "th"},
			For:         []string{"agent:doc"},
			Source:      "agent:doc",
			Disabled:    true,
		}},
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, ok := names(t, dest)[config.AgentMCPFile]
	if !ok {
		t.Fatalf("no %s in the package", config.AgentMCPFile)
	}
	if strings.Contains(raw, "secret_live_value") {
		t.Fatalf("the seller's token shipped inside the package:\n%s", raw)
	}
	var declared []config.MCPServerConfig
	if err := json.Unmarshal([]byte(raw), &declared); err != nil {
		t.Fatalf("declaration is not readable as mcp-servers.json: %v", err)
	}
	if len(declared) != 1 {
		t.Fatalf("declared %d servers, want 1", len(declared))
	}
	d := declared[0]
	if d.Environment["NOTION_TOKEN"] != "${ask:NOTION_TOKEN}" {
		t.Fatalf("token env = %q, want an ask placeholder", d.Environment["NOTION_TOKEN"])
	}
	// An ordinary setting is not a secret and must not become a question the
	// buyer has to answer to finish an install.
	if d.Environment["NOTION_LOCALE"] != "th" {
		t.Fatalf("plain env = %q, want it carried through", d.Environment["NOTION_LOCALE"])
	}
	// A package declares; it never grants. And it never arrives switched off.
	if len(d.For) != 0 || d.Source != "" {
		t.Fatalf("package granted itself placement: for=%v source=%q", d.For, d.Source)
	}
	if d.Disabled {
		t.Fatalf("package shipped a server that is switched off")
	}
	if len(res.Asked) != 1 || res.Asked[0].Key != "NOTION_TOKEN" || res.Asked[0].Server != "notion" {
		t.Fatalf("Asked = %+v, want the one token", res.Asked)
	}
}

// Byte-identical for identical contents. It is what lets a publisher sign a
// package later and a buyer tell two downloads apart.
func TestExportIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	opt := Options{
		Name:    "doc",
		Sources: []fs.FS{agentFS(nil)},
		Servers: []config.MCPServerConfig{{Name: "b"}, {Name: "a"}},
	}
	first := filepath.Join(dir, "one.zip")
	second := filepath.Join(dir, "two.zip")
	if _, err := Export(first, opt); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if _, err := Export(second, opt); err != nil {
		t.Fatalf("second export: %v", err)
	}
	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("two exports of the same worker differ (%d vs %d bytes)", len(a), len(b))
	}
}

// The machine's leftovers are not part of what was sold.
func TestExportSkipsMachineLeftovers(t *testing.T) {
	src := agentFS(nil)
	src[".git/config"] = &fstest.MapFile{Data: []byte("[core]")}
	src["Thumbs.db"] = &fstest.MapFile{Data: []byte("x")}
	src[".DS_Store"] = &fstest.MapFile{Data: []byte("x")}
	dest := filepath.Join(t.TempDir(), "doc.zip")

	if _, err := Export(dest, Options{Name: "doc", Sources: []fs.FS{src}}); err != nil {
		t.Fatalf("export: %v", err)
	}
	for name := range names(t, dest) {
		if strings.HasPrefix(name, ".") || strings.EqualFold(filepath.Base(name), "Thumbs.db") {
			t.Fatalf("%s travelled with the package", name)
		}
	}
}

// A folder without AGENT.md is a worker's state, not a worker — the same rule
// the profile resolver keys on.
func TestExportRefusesAFolderThatIsNotAPackage(t *testing.T) {
	src := fstest.MapFS{"MEMORY.md": &fstest.MapFile{Data: []byte("learned")}}
	dest := filepath.Join(t.TempDir(), "doc.zip")

	if _, err := Export(dest, Options{Name: "doc", Sources: []fs.FS{src}}); err == nil {
		t.Fatalf("exported a folder with no %s", config.AgentDefinitionFile)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatalf("a refused export still left a file behind")
	}
}

// Reading back: the two fields a package may not write are dropped rather than
// trusted, so nothing downstream has to remember to check them.
func TestReadDeclaredMCPDropsWhatThePackageMayNotWrite(t *testing.T) {
	pkg := fstest.MapFS{config.AgentMCPFile: &fstest.MapFile{Data: []byte(
		`[{"name":"notion","command":["npx","x"],"for":["assistant"],"source":"agent:someone-else"}]`)}}

	got, err := ReadDeclaredMCP(pkg)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d servers, want 1", len(got))
	}
	if len(got[0].For) != 0 {
		t.Fatalf("a package granted itself a desk: %v", got[0].For)
	}
	if got[0].Source != "" {
		t.Fatalf("a package claimed its own provenance: %q", got[0].Source)
	}
}

// No mcp.json is the normal state for most workers and is not a failure.
func TestReadDeclaredMCPTreatsAbsenceAsNone(t *testing.T) {
	got, err := ReadDeclaredMCP(fstest.MapFS{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d servers from a package that brings none", len(got))
	}
}
