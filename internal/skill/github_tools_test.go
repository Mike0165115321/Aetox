package skill

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGitHubSearchFormatsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/search/repositories") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "terminal ui language:go" {
			t.Errorf("query = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count": 2, "items": [
			{"full_name":"a/tui","html_url":"https://github.com/a/tui","description":"nice TUI","stargazers_count":1200,"language":"Go"},
			{"full_name":"b/term","html_url":"https://github.com/b/term","stargazers_count":88}
		]}`))
	}))
	defer server.Close()

	s := &githubSearchSkill{client: newGitHubRepoClient(server.URL, server.URL, &http.Client{Timeout: 5 * time.Second})}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"query": "terminal ui language:go"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	for _, want := range []string{"a/tui", "★1200", "nice TUI", "https://github.com/b/term", "(no description)"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("missing %q in:\n%s", want, out.Content)
		}
	}
}

func TestGitHubReadFileFetchesRawContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"full_name":"a/tui","html_url":"https://github.com/a/tui","default_branch":"main"}`))
		case r.URL.Path == "/a/tui/main/README.md":
			_, _ = w.Write([]byte("# TUI\nhello"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	s := &githubReadFileSkill{client: newGitHubRepoClient(server.URL, server.URL, &http.Client{Timeout: 5 * time.Second})}
	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"repo_url": "https://github.com/a/tui",
		"path":     "README.md",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(out.Content, "# TUI") || !strings.Contains(out.Content, "a/tui @ main") {
		t.Errorf("unexpected content:\n%s", out.Content)
	}

	// path traversal must be rejected before any request
	if _, err := s.ExecuteTool(context.Background(), map[string]any{
		"repo_url": "https://github.com/a/tui",
		"path":     "../../etc/passwd",
	}); err == nil {
		t.Fatal("traversal path must be rejected")
	}
}

func TestGitHubListFilesFormatsEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/a/tui/contents/cmd" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"main.go","path":"cmd/main.go","type":"file","size":420},
			{"name":"internal","path":"cmd/internal","type":"dir"}
		]`))
	}))
	defer server.Close()

	s := &githubListFilesSkill{client: newGitHubRepoClient(server.URL, server.URL, &http.Client{Timeout: 5 * time.Second})}
	out, err := s.ExecuteTool(context.Background(), map[string]any{
		"repo_url": "https://github.com/a/tui",
		"path":     "cmd",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(out.Content, "cmd/main.go (420 bytes)") || !strings.Contains(out.Content, "cmd/internal/") {
		t.Errorf("unexpected content:\n%s", out.Content)
	}
}

func TestValidatePluginManifestName(t *testing.T) {
	base := func(name string) *aetoxPluginManifest {
		return &aetoxPluginManifest{
			Name:  name,
			Files: []aetoxPluginFileEntry{{Source: "skill.md", Target: "skill.md"}},
		}
	}

	for _, bad := range []string{"", "..", "../escape", "..\\escape", "/abs", "a/../..", "nested/name", "C:\\evil"} {
		if err := validatePluginManifest(base(bad)); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}

	m := base("  my-plugin  ")
	if err := validatePluginManifest(m); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	if m.Name != "my-plugin" {
		t.Errorf("name not normalized: %q", m.Name)
	}
}

// A skill published on GitHub is a folder with a SKILL.md in it — almost none
// carry aetox-plugin.json. These cover the shapes that actually exist.
func TestFindPlainSkillsRootIsOneSkill(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "SKILL.md", Type: "blob", Size: 100},
		{Path: "README.md", Type: "blob", Size: 50},
		{Path: "references", Type: "tree"},
		{Path: "references/api.md", Type: "blob", Size: 80},
		{Path: ".github/workflows/ci.yml", Type: "blob", Size: 20},
	}
	got := findPlainSkills(entries, "my-skill")
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1", len(got))
	}
	if got[0].name != "my-skill" {
		t.Errorf("name = %q, want the repo name", got[0].name)
	}
	// SKILL.md first, so a truncated install still leaves the defining file.
	if got[0].files[0] != "SKILL.md" {
		t.Errorf("files[0] = %q, want SKILL.md first", got[0].files[0])
	}
	for _, f := range got[0].files {
		if strings.HasPrefix(f, ".") {
			t.Errorf("dotfile %q was collected; CI config is not skill material", f)
		}
	}
	if !slices.Contains(got[0].files, "references/api.md") {
		t.Error("material beside the SKILL.md was not collected")
	}
}

func TestFindPlainSkillsOneFolderDeepIsACollection(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "README.md", Type: "blob", Size: 10},
		{Path: "pdf/SKILL.md", Type: "blob", Size: 100},
		{Path: "pdf/forms.md", Type: "blob", Size: 40},
		{Path: "xlsx/SKILL.md", Type: "blob", Size: 100},
		// Excluded by kind, not by depth: a SKILL.md under vendor/ came with
		// somebody else's package.
		{Path: "vendor/nested/deep/SKILL.md", Type: "blob", Size: 10},
	}
	got := findPlainSkills(entries, "anthropic-skills")
	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.name)
	}
	if !slices.Equal(names, []string{"pdf", "xlsx"}) {
		t.Fatalf("names = %v, want [pdf xlsx]", names)
	}
	for _, s := range got {
		for _, f := range s.files {
			if !strings.HasPrefix(f, s.name+"/") {
				t.Errorf("%s collected %q from outside its own folder", s.name, f)
			}
		}
	}
}

// A root SKILL.md means the repository is the skill, so a nested one is its
// material rather than a second skill to install alongside.
func TestFindPlainSkillsRootWinsOverNested(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "SKILL.md", Type: "blob", Size: 10},
		{Path: "examples/SKILL.md", Type: "blob", Size: 10},
	}
	got := findPlainSkills(entries, "repo")
	if len(got) != 1 || got[0].name != "repo" {
		t.Fatalf("got %v, want a single skill named after the repo", got)
	}
}

// A big repository is not refused. An earlier version capped an install at a
// file count and a byte total; that was Aetox rationing the user's own disk for
// something they asked for by URL, with nothing hosted or paid for on our side.
// The set is taken whole, however large it is.
func TestFindPlainSkillsTakesLargeRepositoriesWhole(t *testing.T) {
	entries := []githubTreeEntry{{Path: "SKILL.md", Type: "blob", Size: 10}}
	for i := 0; i < 5000; i++ {
		entries = append(entries, githubTreeEntry{Path: fmt.Sprintf("f%04d.md", i), Type: "blob", Size: 64 << 10})
	}
	got := findPlainSkills(entries, "big")
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1", len(got))
	}
	if len(got[0].files) != len(entries) {
		t.Errorf("collected %d of %d files; nothing may be left behind", len(got[0].files), len(entries))
	}
}

// Nested material is part of the set, not something to leave behind — this is
// what "comes as a complete bundle" means in practice.
func TestFindPlainSkillsTakesTheWholeSubtree(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "pdf/SKILL.md", Type: "blob", Size: 10},
		{Path: "pdf/scripts/fill_form.py", Type: "blob", Size: 20},
		{Path: "pdf/scripts/lib/util.py", Type: "blob", Size: 20},
		{Path: "pdf/references/forms.md", Type: "blob", Size: 20},
		{Path: "pdf/assets/template.pdf", Type: "blob", Size: 4096},
	}
	got := findPlainSkills(entries, "repo")
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1", len(got))
	}
	if len(got[0].files) != len(entries) {
		t.Fatalf("collected %d of %d files: %v", len(got[0].files), len(entries), got[0].files)
	}
	for _, want := range []string{"pdf/scripts/lib/util.py", "pdf/assets/template.pdf"} {
		if !slices.Contains(got[0].files, want) {
			t.Errorf("%s was left behind; the skill would break at run time", want)
		}
	}
}

func TestFindPlainSkillsNoSkillFile(t *testing.T) {
	entries := []githubTreeEntry{{Path: "README.md", Type: "blob", Size: 10}}
	if got := findPlainSkills(entries, "repo"); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

// The install root must not be escapable by a name from a repository we do not
// control.
func TestSanitizeSkillName(t *testing.T) {
	cases := map[string]string{
		"My Skill":    "my-skill",
		"../../etc":   "etc",
		"skills/pdf":  "skillspdf",
		"..":          "skill",
		"":            "skill",
		"good_name-1": "good_name-1",
		"Ω":           "skill",
		"skill.v2":    "skill-v2",
	}
	for in, want := range cases {
		if got := sanitizeSkillName(in); got != want {
			t.Errorf("sanitizeSkillName(%q) = %q, want %q", in, got, want)
		}
		if got := sanitizeSkillName(in); strings.ContainsAny(got, `/\`) || got == ".." {
			t.Errorf("sanitizeSkillName(%q) = %q, which can leave the install root", in, got)
		}
	}
}

// End to end over a stubbed GitHub: a repository that publishes a skill the
// ordinary way — a folder with a SKILL.md, no aetox-plugin.json — must install.
// Before this it could not, so the install box only worked for repositories
// written for Aetox, of which there are approximately none.
func TestPluginInstallAcceptsAPlainSkillRepo(t *testing.T) {
	files := map[string]string{
		"pdf/SKILL.md":      "---\nname: pdf\ndescription: read pdfs\n---\nbody",
		"pdf/references.md": "more",
		"xlsx/SKILL.md":     "---\nname: xlsx\ndescription: sheets\n---\nbody",
		"README.md":         "not a skill",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+aetoxPluginManifestName):
			w.WriteHeader(http.StatusNotFound) // the case this test exists for
		case strings.Contains(r.URL.Path, "/git/trees/"):
			rows := make([]string, 0, len(files))
			for p := range files {
				rows = append(rows, fmt.Sprintf(`{"path":%q,"type":"blob","size":%d}`, p, len(files[p])))
			}
			fmt.Fprintf(w, `{"tree":[%s],"truncated":false}`, strings.Join(rows, ","))
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			fmt.Fprint(w, `{"full_name":"acme/skills","name":"skills","owner":{"login":"acme"},"default_branch":"main"}`)
		default:
			for p, body := range files {
				if strings.HasSuffix(r.URL.Path, "/"+p) {
					fmt.Fprint(w, body)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	s := &pluginInstallSkill{
		client:      newGitHubRepoClient(server.URL, server.URL, &http.Client{Timeout: 5 * time.Second}),
		installRoot: root,
	}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"repo_url": "https://github.com/acme/skills"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.Content, "installed 2 skill(s)") {
		t.Fatalf("content = %q, want both folders installed", out.Content)
	}

	// The files have to land where discovery scans, in the shape it expects.
	for _, name := range []string{"pdf", "xlsx"} {
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			t.Errorf("%s/SKILL.md not written: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "pdf", "references.md")); err != nil {
		t.Errorf("material beside the SKILL.md was not installed: %v", err)
	}
	// README.md sits outside both skill folders and must not be dragged in.
	if _, err := os.Stat(filepath.Join(root, "pdf", "README.md")); err == nil {
		t.Error("a file from outside the skill folder was installed into it")
	}

	// And the whole point: what was installed is now discoverable.
	found := ListDiscovered([]string{root})
	if len(found) != 2 {
		t.Fatalf("discovery found %d skills after install, want 2", len(found))
	}
}

// A repository with no SKILL.md anywhere must say so plainly rather than
// failing with a message about a manifest format the user never heard of.
func TestPluginInstallExplainsARepoWithNoSkill(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+aetoxPluginManifestName):
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(r.URL.Path, "/git/trees/"):
			fmt.Fprint(w, `{"tree":[{"path":"README.md","type":"blob","size":4}],"truncated":false}`)
		default:
			fmt.Fprint(w, `{"full_name":"acme/empty","name":"empty","owner":{"login":"acme"},"default_branch":"main"}`)
		}
	}))
	defer server.Close()

	s := &pluginInstallSkill{
		client:      newGitHubRepoClient(server.URL, server.URL, &http.Client{Timeout: 5 * time.Second}),
		installRoot: t.TempDir(),
	}
	out, err := s.ExecuteTool(context.Background(), map[string]any{"repo_url": "https://github.com/acme/empty"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "SKILL.md") {
		t.Errorf("content = %q, want it to name what was looked for", out.Content)
	}
}

// The bug behind "parse repository listing: unexpected end of JSON input".
//
// io.ReadAll(io.LimitReader(r, n)) returns a truncated body with a nil error,
// so a 1.6 MB listing read under a 1 MB limit arrived cut in half and surfaced
// as a syntax error with the real cause — a size limit — nowhere on screen.
func TestReadBoundedReportsOverflowInsteadOfTruncating(t *testing.T) {
	body := strings.Repeat("x", 100)

	got, err := readBounded(strings.NewReader(body), 100)
	if err != nil {
		t.Fatalf("a response exactly at the limit was rejected: %v", err)
	}
	if len(got) != 100 {
		t.Errorf("read %d bytes, want 100", len(got))
	}

	// One byte over must be an error, not a shorter slice.
	if _, err := readBounded(strings.NewReader(body+"x"), 100); err == nil {
		t.Fatal("a response past the limit was returned truncated with no error")
	} else if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error %q does not say the response was too large", err)
	}
}

// End to end: a listing bigger than the metadata limit must not come back as a
// JSON parse failure. The real repository that hit this had 6,362 files.
func TestFetchRepoTreeSurvivesALargeListing(t *testing.T) {
	const entryCount = 20000
	rows := make([]string, 0, entryCount)
	for i := 0; i < entryCount; i++ {
		rows = append(rows, fmt.Sprintf(`{"path":"src/very/long/path/to/some/file%05d.ts","type":"blob","size":2048}`, i))
	}
	payload := fmt.Sprintf(`{"tree":[%s],"truncated":false}`, strings.Join(rows, ","))
	if len(payload) <= metadataReadLimit {
		t.Fatalf("fixture is %d bytes, not over the %d limit it must exercise", len(payload), metadataReadLimit)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	c := newGitHubRepoClient(server.URL, server.URL, &http.Client{Timeout: 10 * time.Second})
	entries, truncated, err := c.fetchRepoTree(context.Background(),
		githubRepoMetadata{Owner: "a", Repo: "b", DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("a large listing failed: %v", err)
	}
	if truncated {
		t.Error("truncated was set on a complete listing")
	}
	if len(entries) != entryCount {
		t.Errorf("parsed %d entries, want %d", len(entries), entryCount)
	}
}

// The URL the user pasted ended in .git; the repo name must not.
func TestExtractGitHubRepoURLStripsDotGit(t *testing.T) {
	target, ok := ExtractGitHubRepoURL("https://github.com/Mike0165115321/ui.git")
	if !ok {
		t.Fatal("a .git clone URL was not recognised")
	}
	if target.Repo != "ui" {
		t.Errorf("repo = %q, want %q", target.Repo, "ui")
	}
}

// The layout that made this report: a `skills/` directory with a folder per
// skill, two levels down. shadcn/ui and Anthropic's repositories both publish
// this way, and the old root-or-one-level rule reported "no skill found" on a
// repository that plainly had two.
func TestFindPlainSkillsFindsASkillsDirectory(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "package.json", Type: "blob", Size: 10},
		{Path: "apps/v4/content/docs/skills.mdx", Type: "blob", Size: 10},
		{Path: "skills/shadcn/SKILL.md", Type: "blob", Size: 100},
		{Path: "skills/shadcn/reference.md", Type: "blob", Size: 40},
		{Path: "skills/migrate-radix-to-base/SKILL.md", Type: "blob", Size: 100},
		{Path: "skills/migrate-radix-to-base/menus.md", Type: "blob", Size: 40},
	}
	got := findPlainSkills(entries, "ui")
	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.name)
	}
	if !slices.Equal(names, []string{"migrate-radix-to-base", "shadcn"}) {
		t.Fatalf("names = %v, want both skills named after their own folder", names)
	}
	// Each takes its own material and nothing from its sibling.
	for _, s := range got {
		for _, f := range s.files {
			if !strings.HasPrefix(f, "skills/"+s.name+"/") {
				t.Errorf("%s collected %q from outside its folder", s.name, f)
			}
		}
	}
}

// Depth is not what disqualifies a folder — where its contents came from is.
func TestFindPlainSkillsIgnoresDependencyAndBuildDirectories(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "deep/nested/path/mine/SKILL.md", Type: "blob", Size: 10},
		{Path: "node_modules/pkg/SKILL.md", Type: "blob", Size: 10},
		{Path: "vendor/other/SKILL.md", Type: "blob", Size: 10},
		{Path: "dist/bundled/SKILL.md", Type: "blob", Size: 10},
		{Path: ".github/actions/thing/SKILL.md", Type: "blob", Size: 10},
	}
	got := findPlainSkills(entries, "repo")
	if len(got) != 1 || got[0].name != "mine" {
		names := []string{}
		for _, s := range got {
			names = append(names, s.name)
		}
		t.Fatalf("names = %v, want only [mine] — depth is fine, other people's code is not", names)
	}
}

// Two folders can share a basename at different paths; neither may overwrite
// the other on the way to disk.
func TestFindPlainSkillsDisambiguatesCollidingNames(t *testing.T) {
	entries := []githubTreeEntry{
		{Path: "a/pdf/SKILL.md", Type: "blob", Size: 10},
		{Path: "b/pdf/SKILL.md", Type: "blob", Size: 10},
	}
	got := findPlainSkills(entries, "repo")
	if len(got) != 2 {
		t.Fatalf("got %d skills, want 2", len(got))
	}
	if got[0].name == got[1].name {
		t.Fatalf("both installed as %q; one would overwrite the other", got[0].name)
	}
}
