package repomap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A little repo with a clear center of gravity: two Go files import pkg/core,
// one Svelte file imports lib/util, and one markdown file has headings. The
// map must put the referenced files first and show real symbols with line
// numbers.
func seedRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/demo\n\ngo 1.22\n")
	write("pkg/core/core.go", `package core

// Answer is the load-bearing function.
func Answer() int { return 42 }

type Engine struct {
	Ready bool
}
`)
	write("a.go", "package main\n\nimport \"example.com/demo/pkg/core\"\n\nfunc main() { core.Answer() }\n")
	write("b.go", "package main\n\nimport \"example.com/demo/pkg/core\"\n\nfunc helper() int { return core.Answer() }\n")
	write("lib/util.ts", "export function slugify(s: string): string { return s }\nexport const VERSION = () => 3\n")
	write("App.svelte", "<script>\nimport { slugify } from './lib/util'\n</script>\n")
	write("docs/GUIDE.md", "# Guide\n\nprose\n\n## Install\n\n```sh\n# not a heading\n```\n")
	return root
}

func TestBuildRanksReferencedFilesFirst(t *testing.T) {
	out, err := Build(context.Background(), Options{Root: seedRepo(t)})
	if err != nil {
		t.Fatal(err)
	}
	core := strings.Index(out, "pkg/core/core.go")
	util := strings.Index(out, "lib/util.ts")
	app := strings.Index(out, "App.svelte")
	if core < 0 || util < 0 {
		t.Fatalf("referenced files missing from map:\n%s", out)
	}
	// core.go is imported by two files, util.ts by one, App.svelte by none —
	// the render order must say so.
	if !(core < util) {
		t.Errorf("core.go (2 refs) should rank above util.ts (1 ref):\n%s", out)
	}
	if app >= 0 && app < util {
		t.Errorf("App.svelte (0 refs) should not outrank util.ts:\n%s", out)
	}
	if !strings.Contains(out, "(referenced by 2)") {
		t.Errorf("core.go's two importers should be counted:\n%s", out)
	}
}

func TestBuildShowsRealSymbolsWithLines(t *testing.T) {
	out, err := Build(context.Background(), Options{Root: seedRepo(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func Answer() int",       // Go func, brace trimmed
		"type Engine struct",      // Go type
		"export function slugify", // ts function
		"## Install",              // md heading outside the fence
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "not a heading") {
		t.Errorf("a # inside a code fence was mapped as a heading:\n%s", out)
	}
	// Answer is defined on line 4 of core.go and the map must say which line,
	// or "read only the range you need" has nothing to aim at.
	if !strings.Contains(out, "4: func Answer() int") {
		t.Errorf("symbol lines should carry their line number:\n%s", out)
	}
}

func TestBuildRespectsTheBudget(t *testing.T) {
	root := seedRepo(t)
	out, err := Build(context.Background(), Options{Root: root, Budget: 30})
	if err != nil {
		t.Fatal(err)
	}
	// 30 tokens ≈ 120 chars: room for the header and roughly one file. The
	// contract is whole files within roughly one file-block of the line, never
	// the whole map regardless.
	if len(out) > 30*4+400 {
		t.Errorf("a 30-token budget rendered %d chars:\n%s", len(out), out)
	}
	if !strings.Contains(out, "more files under the budget line") {
		t.Errorf("a cut map must say what it cut:\n%s", out)
	}
	full, err := Build(context.Background(), Options{Root: root, Budget: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if len(full) <= len(out) {
		t.Errorf("a bigger budget should render a bigger map (%d vs %d)", len(full), len(out))
	}
}

func TestBuildSkipsIgnoredAndDotDirs(t *testing.T) {
	root := seedRepo(t)
	for _, rel := range []string{"node_modules/big/dep.ts", ".git/objects/junk.md"} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("export function hidden() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := Build(context.Background(), Options{Root: root, Ignore: map[string]bool{"node_modules": true}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "hidden") || strings.Contains(out, "node_modules") || strings.Contains(out, ".git") {
		t.Errorf("ignored directories leaked into the map:\n%s", out)
	}
}

func TestBuildRefusesAMissingRoot(t *testing.T) {
	if _, err := Build(context.Background(), Options{Root: filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Fatal("a root that does not exist must be an error, not an empty map")
	}
}

// The declRules languages, one probe each: a map line with the right symbol
// proves the family's scanner sees declarations; this is deliberately not a
// parser test suite — a line scan owns no grammar to pin.
func TestBuildScansTheMainstreamLanguages(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// One folder each, or the per-directory breadth cap (maxFilesPerDir) would
	// cut the probe list before the scanners it exists to exercise ever ran.
	write("jvm/A.java", "public class Invoice {\n    public void settle() {}\n}\n")
	write("rust/lib.rs", "pub fn parse_all(input: &str) -> Vec<Token> {\n    todo!()\n}\npub struct Token {}\n")
	write("php/app.php", "<?php\nclass Router {\n    public function dispatch($req) {}\n}\n")
	write("ruby/worker.rb", "class Worker\n  def perform(job)\n  end\nend\n")
	write("swift/View.swift", "public struct ProfileView {\n    func render() {}\n}\n")
	write("c/core.c", "struct buffer { int n; };\nint flush_buffer(struct buffer *b)\n{\n    return 0;\n}\n")

	out, err := Build(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"public class Invoice",
		"pub fn parse_all",
		"class Router",
		"def perform",
		"public struct ProfileView",
		"int flush_buffer",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map is missing %q:\n%s", want, out)
		}
	}
}

// Python's import graph: an absolute import resolves through the suffix walk
// (the package root lives under backend/, as it does in real repos), and the
// imported module outranks its importer.
func TestBuildResolvesPythonImports(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("backend/app/db/gate.py", "def open_gate(cfg):\n    pass\n")
	write("backend/app/api/routes.py", "from app.db.gate import open_gate\n\ndef register(app):\n    pass\n")
	write("backend/app/api/admin.py", "from app.db.gate import open_gate\n\ndef admin(app):\n    pass\n")
	write("backend/app/api/util.py", "from .routes import register\n\ndef helper():\n    pass\n")

	out, err := Build(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	gate := strings.Index(out, "backend/app/db/gate.py")
	routes := strings.Index(out, "backend/app/api/routes.py")
	if gate < 0 {
		t.Fatalf("gate.py missing from map:\n%s", out)
	}
	if !strings.Contains(out, "backend/app/db/gate.py  (referenced by 2)") {
		t.Errorf("absolute imports should credit gate.py twice:\n%s", out)
	}
	if routes >= 0 && routes < gate {
		t.Errorf("gate.py (2 refs) should outrank routes.py (1 ref):\n%s", out)
	}
}

// The graph face: same ranking as the text map, real edges between kept
// nodes, and a Go package-level import landing on one spokesperson file
// rather than fanning out to lines nobody wrote.
func TestGraphSharesRankingAndResolvesEdges(t *testing.T) {
	nodes, edges, total, err := Graph(context.Background(), Options{Root: seedRepo(t)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total < 6 {
		t.Fatalf("total = %d, want every parsed file counted", total)
	}
	if len(nodes) == 0 || nodes[0].Path != "pkg/core/core.go" {
		t.Fatalf("the most-referenced file must lead the graph as it leads the map, got %+v", nodes)
	}
	find := func(path string) int {
		for i, n := range nodes {
			if n.Path == path {
				return i
			}
		}
		return -1
	}
	core, app, util := find("pkg/core/core.go"), find("App.svelte"), find("lib/util.ts")
	if core < 0 || app < 0 || util < 0 {
		t.Fatalf("expected nodes missing: %+v", nodes)
	}
	has := func(from, to int) bool {
		for _, e := range edges {
			if e.From == from && e.To == to {
				return true
			}
		}
		return false
	}
	if !has(find("a.go"), core) || !has(find("b.go"), core) {
		t.Errorf("Go imports should land on the package spokesperson: %+v", edges)
	}
	if !has(app, util) {
		t.Errorf("the svelte import of lib/util should be an edge: %+v", edges)
	}
	for _, e := range edges {
		if e.From == e.To {
			t.Errorf("self edge slipped through: %+v", e)
		}
	}
}

// The aliased imports every Next/Vite project actually writes: `@/lib/db`
// resolves by suffix, same walk as Python absolutes — without it a frontend
// draws as dots with no lines, which is how the gap was found.
func TestBuildResolvesAliasImports(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("frontend/src/lib/db.ts", "export function connect(): void {}\n")
	write("frontend/src/app/page.tsx", "import { connect } from '@/lib/db'\n\nexport default function Page() { return null }\n")
	write("frontend/src/app/admin.tsx", "import { connect } from '@/lib/db'\n\nexport default function Admin() { return null }\n")

	out, err := Build(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "frontend/src/lib/db.ts  (referenced by 2)") {
		t.Errorf("@/ imports should credit db.ts twice:\n%s", out)
	}
}
