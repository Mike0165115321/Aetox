package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
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
