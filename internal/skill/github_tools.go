package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

const (
	defaultGitHubAPIBaseURL = "https://api.github.com"
	defaultGitHubRawBaseURL = "https://raw.githubusercontent.com"
	aetoxPluginManifestName = "aetox-plugin.json"
)

var (
	reGitHubRepoURL = regexp.MustCompile(`(?i)https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(?:[/?#][^\s]*)?`)
)

type GitHubRepoTarget struct {
	Owner string
	Repo  string
	URL   string
}

type githubRepoSummarySkill struct {
	client *githubRepoClient
}

type pluginInstallSkill struct {
	client      *githubRepoClient
	installRoot string
}

type githubRepoClient struct {
	apiBaseURL string
	rawBaseURL string
	httpClient *http.Client
}

type githubRepoMetadata struct {
	Owner         string
	Repo          string
	FullName      string
	URL           string
	Description   string
	DefaultBranch string
	Language      string
	Stars         int
	Topics        []string
}

type aetoxPluginManifest struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Version string                 `json:"version"`
	Summary string                 `json:"summary"`
	Files   []aetoxPluginFileEntry `json:"files"`
}

type aetoxPluginFileEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func ExtractGitHubRepoURL(raw string) (GitHubRepoTarget, bool) {
	match := reGitHubRepoURL.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) < 3 {
		return GitHubRepoTarget{}, false
	}
	owner := strings.TrimSpace(match[1])
	repo := normalizeGitHubRepoName(match[2])
	if owner == "" || repo == "" {
		return GitHubRepoTarget{}, false
	}
	return GitHubRepoTarget{
		Owner: owner,
		Repo:  repo,
		URL:   fmt.Sprintf("https://github.com/%s/%s", owner, repo),
	}, true
}

func normalizeGitHubRepoName(raw string) string {
	repo := strings.TrimSpace(raw)
	repo = strings.TrimSuffix(repo, ".git")
	repo = strings.TrimRight(repo, ".,);:!?]'\"")
	return strings.TrimSpace(repo)
}

func newGitHubRepoClient(apiBaseURL, rawBaseURL string, httpClient *http.Client) *githubRepoClient {
	apiBaseURL = strings.TrimSpace(apiBaseURL)
	if apiBaseURL == "" {
		apiBaseURL = defaultGitHubAPIBaseURL
	}
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	if rawBaseURL == "" {
		rawBaseURL = defaultGitHubRawBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &githubRepoClient{
		apiBaseURL: strings.TrimSuffix(apiBaseURL, "/"),
		rawBaseURL: strings.TrimSuffix(rawBaseURL, "/"),
		httpClient: httpClient,
	}
}

func (*githubRepoSummarySkill) Name() string { return "github_repo_summary" }

func (*githubRepoSummarySkill) Description() string {
	return "Fetch a concise summary for a GitHub repository URL"
}

func (s *githubRepoSummarySkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repo_url": map[string]any{
				"type":        "string",
				"description": "GitHub repository URL",
			},
		},
		"required":             []string{"repo_url"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "github_repo_summary",
			Description: "Summarize a GitHub repository from its URL using GitHub metadata.",
			Parameters:  payload,
		},
	}
}

func (s *githubRepoSummarySkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: github_repo_summary <github-repo-url>")
		return newToolOutput("github_repo_summary", "github_repo_summary", "", start, false, err), err
	}
	repoURL := strings.TrimSpace(strings.Join(args, " "))
	if repoURL == "" {
		err := errors.New("usage: github_repo_summary <github-repo-url>")
		return newToolOutput("github_repo_summary", "github_repo_summary", "", start, false, err), err
	}
	return s.execute(ctx, repoURL, start)
}

func (s *githubRepoSummarySkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	repoURL, ok := args["repo_url"].(string)
	if !ok || strings.TrimSpace(repoURL) == "" {
		err := errors.New("repo_url is required")
		return newToolOutput("github_repo_summary", "github_repo_summary", "", time.Now(), false, err), err
	}
	return s.execute(ctx, repoURL, time.Now())
}

func (s *githubRepoSummarySkill) execute(ctx context.Context, repoURL string, start time.Time) (Output, error) {
	client := s.client
	if client == nil {
		client = newGitHubRepoClient("", "", nil)
	}
	repo, err := client.fetchRepoMetadata(ctx, repoURL)
	if err != nil {
		return newToolOutput("github_repo_summary", "github_repo_summary "+repoURL, "", start, false, err), err
	}
	lines := []string{
		"GitHub repo: " + repo.FullName,
		"Description: " + emptyFallback(repo.Description, "(no description)"),
		"Default branch: " + emptyFallback(repo.DefaultBranch, "(unknown)"),
		"Language: " + emptyFallback(repo.Language, "(unknown)"),
		fmt.Sprintf("Stars: %d", repo.Stars),
		"URL: " + repo.URL,
	}
	if len(repo.Topics) > 0 {
		lines = append(lines, "Topics: "+strings.Join(repo.Topics, ", "))
	}
	return newToolOutput("github_repo_summary", "github_repo_summary "+repo.FullName, strings.Join(lines, "\n"), start, false, nil), nil
}

func (*pluginInstallSkill) Name() string { return "plugin_install" }

func (*pluginInstallSkill) Description() string {
	return "Install an Aetox plugin from a GitHub repository that defines aetox-plugin.json"
}

func (s *pluginInstallSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repo_url": map[string]any{
				"type":        "string",
				"description": "GitHub repository URL that defines aetox-plugin.json",
			},
		},
		"required":             []string{"repo_url"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "plugin_install",
			Description: "Install an Aetox plugin from a supported GitHub repository manifest.",
			Parameters:  payload,
		},
	}
}

func (s *pluginInstallSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: plugin_install <github-repo-url>")
		return newToolOutput("plugin_install", "plugin_install", "", start, false, err), err
	}
	repoURL := strings.TrimSpace(strings.Join(args, " "))
	if repoURL == "" {
		err := errors.New("usage: plugin_install <github-repo-url>")
		return newToolOutput("plugin_install", "plugin_install", "", start, false, err), err
	}
	return s.execute(ctx, repoURL, start)
}

func (s *pluginInstallSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	repoURL, ok := args["repo_url"].(string)
	if !ok || strings.TrimSpace(repoURL) == "" {
		err := errors.New("repo_url is required")
		return newToolOutput("plugin_install", "plugin_install", "", time.Now(), false, err), err
	}
	return s.execute(ctx, repoURL, time.Now())
}

func (s *pluginInstallSkill) execute(ctx context.Context, repoURL string, start time.Time) (Output, error) {
	client := s.client
	if client == nil {
		client = newGitHubRepoClient("", "", nil)
	}
	repo, err := client.fetchRepoMetadata(ctx, repoURL)
	if err != nil {
		return newToolOutput("plugin_install", "plugin_install "+repoURL, "", start, false, err), err
	}
	manifest, found, err := client.fetchPluginManifest(ctx, repo)
	if err != nil {
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
	}
	if !found {
		content := fmt.Sprintf("plugin install unsupported: %s does not define %s", repo.FullName, aetoxPluginManifestName)
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, content, start, false, nil), nil
	}
	if strings.TrimSpace(manifest.Type) != "skill-bundle" {
		content := fmt.Sprintf("plugin install unsupported: manifest type %q is not supported", manifest.Type)
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, content, start, false, nil), nil
	}
	if err := validatePluginManifest(manifest); err != nil {
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
	}

	installRoot, err := s.resolveInstallRoot()
	if err != nil {
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
	}
	targetRoot := filepath.Join(installRoot, manifest.Name)
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
	}

	written := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		targetRel, err := normalizeManifestRelativePath(file.Target)
		if err != nil {
			return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
		}
		sourceRel, err := normalizeManifestRelativePath(file.Source)
		if err != nil {
			return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
		}
		body, err := client.fetchRawFile(ctx, repo, sourceRel)
		if err != nil {
			return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
		}
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(targetRel))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
		}
		if err := os.WriteFile(targetPath, body, 0o644); err != nil {
			return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
		}
		written = append(written, filepath.ToSlash(filepath.Join(manifest.Name, targetRel)))
	}

	lines := []string{
		fmt.Sprintf("plugin installed: %s", manifest.Name),
		"Source repo: " + repo.FullName,
		"Install root: " + filepath.ToSlash(targetRoot),
		fmt.Sprintf("Files written: %d", len(written)),
	}
	if len(written) > 0 {
		lines = append(lines, "Installed files: "+strings.Join(written, ", "))
	}
	if strings.TrimSpace(manifest.Version) != "" {
		lines = append(lines, "Version: "+manifest.Version)
	}
	return newToolOutput("plugin_install", "plugin_install "+repo.FullName, strings.Join(lines, "\n"), start, false, nil), nil
}

func (s *pluginInstallSkill) resolveInstallRoot() (string, error) {
	if strings.TrimSpace(s.installRoot) != "" {
		return filepath.Abs(strings.TrimSpace(s.installRoot))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve plugin install root: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

func validatePluginManifest(manifest *aetoxPluginManifest) error {
	if manifest == nil {
		return errors.New("plugin manifest is required")
	}
	name, err := normalizeManifestRelativePath(manifest.Name)
	if err != nil {
		return fmt.Errorf("invalid manifest name %q: %w", manifest.Name, err)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("invalid manifest name %q: must be a single path segment", manifest.Name)
	}
	manifest.Name = name
	if len(manifest.Files) == 0 {
		return errors.New("plugin manifest missing files")
	}
	for _, file := range manifest.Files {
		if _, err := normalizeManifestRelativePath(file.Source); err != nil {
			return fmt.Errorf("invalid manifest source %q: %w", file.Source, err)
		}
		if _, err := normalizeManifestRelativePath(file.Target); err != nil {
			return fmt.Errorf("invalid manifest target %q: %w", file.Target, err)
		}
	}
	return nil
}

func normalizeManifestRelativePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("path is empty")
	}
	value = filepath.ToSlash(value)
	if strings.HasPrefix(value, "/") || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return "", errors.New("absolute path is not allowed")
	}
	clean := pathCleanSlash(value)
	if clean == "." || clean == "" {
		return "", errors.New("path resolves to root")
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return "", errors.New("path traversal is not allowed")
		}
	}
	return clean, nil
}

func pathCleanSlash(raw string) string {
	parts := []string{}
	for _, part := range strings.Split(strings.ReplaceAll(raw, "\\", "/"), "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(parts) == 0 {
				return ".."
			}
			parts = parts[:len(parts)-1]
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

func (c *githubRepoClient) fetchRepoMetadata(ctx context.Context, rawURL string) (githubRepoMetadata, error) {
	target, ok := ExtractGitHubRepoURL(rawURL)
	if !ok {
		return githubRepoMetadata{}, errors.New("invalid GitHub repository URL")
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s", c.apiBaseURL, url.PathEscape(target.Owner), url.PathEscape(target.Repo))
	body, statusCode, err := c.doJSONRequest(ctx, endpoint)
	if err != nil {
		return githubRepoMetadata{}, err
	}
	if statusCode == http.StatusNotFound {
		return githubRepoMetadata{}, fmt.Errorf("github repo not found: %s", target.URL)
	}
	if statusCode < 200 || statusCode >= 300 {
		return githubRepoMetadata{}, fmt.Errorf("github repo metadata failed with status %d", statusCode)
	}
	var parsed struct {
		FullName      string   `json:"full_name"`
		HTMLURL       string   `json:"html_url"`
		Description   string   `json:"description"`
		DefaultBranch string   `json:"default_branch"`
		Language      string   `json:"language"`
		Stars         int      `json:"stargazers_count"`
		Topics        []string `json:"topics"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return githubRepoMetadata{}, fmt.Errorf("parse github repo metadata: %w", err)
	}
	return githubRepoMetadata{
		Owner:         target.Owner,
		Repo:          target.Repo,
		FullName:      emptyFallback(parsed.FullName, target.Owner+"/"+target.Repo),
		URL:           emptyFallback(parsed.HTMLURL, target.URL),
		Description:   strings.TrimSpace(parsed.Description),
		DefaultBranch: strings.TrimSpace(parsed.DefaultBranch),
		Language:      strings.TrimSpace(parsed.Language),
		Stars:         parsed.Stars,
		Topics:        parsed.Topics,
	}, nil
}

func (c *githubRepoClient) fetchPluginManifest(ctx context.Context, repo githubRepoMetadata) (*aetoxPluginManifest, bool, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/%s/%s", c.rawBaseURL, repo.Owner, repo.Repo, repo.DefaultBranch, aetoxPluginManifestName)
	body, statusCode, err := c.doRawRequest(ctx, endpoint)
	if err != nil {
		return nil, false, err
	}
	if statusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, false, fmt.Errorf("fetch plugin manifest failed with status %d", statusCode)
	}
	var manifest aetoxPluginManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, false, fmt.Errorf("parse plugin manifest: %w", err)
	}
	return &manifest, true, nil
}

func (c *githubRepoClient) fetchRawFile(ctx context.Context, repo githubRepoMetadata, sourcePath string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/%s/%s", c.rawBaseURL, repo.Owner, repo.Repo, repo.DefaultBranch, sourcePath)
	body, statusCode, err := c.doRawRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, fmt.Errorf("plugin source file not found: %s", sourcePath)
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("fetch plugin source failed with status %d", statusCode)
	}
	return body, nil
}

func (c *githubRepoClient) doJSONRequest(ctx context.Context, endpoint string) ([]byte, int, error) {
	return c.doRequest(ctx, endpoint, "application/vnd.github+json")
}

func (c *githubRepoClient) doRawRequest(ctx context.Context, endpoint string) ([]byte, int, error) {
	return c.doRequest(ctx, endpoint, "application/octet-stream")
}

func (c *githubRepoClient) doRequest(ctx context.Context, endpoint string, accept string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Aetox-CLI")
	if strings.TrimSpace(accept) != "" {
		req.Header.Set("Accept", accept)
	}
	// Optional token lifts the anonymous rate limit (60/h -> 5000/h) and
	// opens private repos the user can access. Fine without one.
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func emptyFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func githubToken() string {
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// github_search / github_read_file / github_list_files — read-only GitHub
// access beyond a single repo summary, all through the same githubRepoClient.
// ---------------------------------------------------------------------------

type githubSearchSkill struct {
	client *githubRepoClient
}

func (*githubSearchSkill) Name() string { return "github_search" }

func (*githubSearchSkill) Description() string {
	return "ค้นหา repository บน GitHub"
}

func (s *githubSearchSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (GitHub repo search syntax, e.g. 'terminal ui language:go')",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "github_search",
			Description: "Search GitHub repositories. Returns name, stars, description, and URL per result. Follow up with github_list_files / github_read_file.",
			Parameters:  payload,
		},
	}
}

func (s *githubSearchSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: github_search <query>")
		return newToolOutput("github_search", "github_search", "", time.Now(), false, err), err
	}
	return s.search(ctx, strings.TrimSpace(strings.Join(args, " ")))
}

func (s *githubSearchSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	query, _ := args["query"].(string)
	return s.search(ctx, strings.TrimSpace(query))
}

func (s *githubSearchSkill) search(ctx context.Context, query string) (Output, error) {
	start := time.Now()
	command := "github_search " + query
	if query == "" {
		err := errors.New("query is required")
		return newToolOutput("github_search", "github_search", "", start, false, err), err
	}
	client := s.client
	if client == nil {
		client = newGitHubRepoClient("", "", nil)
	}
	endpoint := fmt.Sprintf("%s/search/repositories?per_page=10&q=%s", client.apiBaseURL, url.QueryEscape(query))
	body, statusCode, err := client.doJSONRequest(ctx, endpoint)
	if err != nil {
		return newToolOutput("github_search", command, "", start, false, err), err
	}
	if statusCode < 200 || statusCode >= 300 {
		err := fmt.Errorf("github search failed with status %d", statusCode)
		return newToolOutput("github_search", command, "", start, false, err), err
	}
	var parsed struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			FullName    string `json:"full_name"`
			HTMLURL     string `json:"html_url"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
			Language    string `json:"language"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return newToolOutput("github_search", command, "", start, false, err), err
	}
	if len(parsed.Items) == 0 {
		return newToolOutput("github_search", command, "(no results)", start, false, nil), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GitHub repos for %q (%d total):\n", query, parsed.TotalCount)
	for i, item := range parsed.Items {
		fmt.Fprintf(&b, "\n%d. %s ★%d %s\n   %s\n   %s\n",
			i+1, item.FullName, item.Stars, emptyFallback(item.Language, ""),
			emptyFallback(item.Description, "(no description)"), item.HTMLURL)
	}
	return newToolOutput("github_search", command, strings.TrimSpace(b.String()), start, false, nil), nil
}

type githubReadFileSkill struct {
	client *githubRepoClient
}

func (*githubReadFileSkill) Name() string { return "github_read_file" }

func (*githubReadFileSkill) Description() string {
	return "อ่านไฟล์จาก GitHub repository"
}

func (s *githubReadFileSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repo_url": map[string]any{
				"type":        "string",
				"description": "GitHub repository URL (https://github.com/owner/repo)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File path inside the repo, e.g. README.md or src/main.go",
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "Branch, tag, or commit (default: the repo's default branch)",
			},
		},
		"required":             []string{"repo_url", "path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "github_read_file",
			Description: "Read one file from a GitHub repository (raw content). Use github_list_files first if you don't know the path.",
			Parameters:  payload,
		},
	}
}

func (s *githubReadFileSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) < 2 {
		err := errors.New("usage: github_read_file <repo-url> <path> [ref]")
		return newToolOutput("github_read_file", "github_read_file", "", time.Now(), false, err), err
	}
	ref := ""
	if len(args) > 2 {
		ref = args[2]
	}
	return s.read(ctx, args[0], args[1], ref)
}

func (s *githubReadFileSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	repoURL, _ := args["repo_url"].(string)
	path, _ := args["path"].(string)
	ref, _ := args["ref"].(string)
	return s.read(ctx, strings.TrimSpace(repoURL), strings.TrimSpace(path), strings.TrimSpace(ref))
}

func (s *githubReadFileSkill) read(ctx context.Context, repoURL, path, ref string) (Output, error) {
	start := time.Now()
	command := strings.TrimSpace("github_read_file " + repoURL + " " + path + " " + ref)
	if repoURL == "" || path == "" {
		err := errors.New("repo_url and path are required")
		return newToolOutput("github_read_file", command, "", start, false, err), err
	}
	cleanPath, err := normalizeManifestRelativePath(path)
	if err != nil {
		return newToolOutput("github_read_file", command, "", start, false, err), err
	}
	client := s.client
	if client == nil {
		client = newGitHubRepoClient("", "", nil)
	}
	repo, err := client.fetchRepoMetadata(ctx, repoURL)
	if err != nil {
		return newToolOutput("github_read_file", command, "", start, false, err), err
	}
	if ref == "" {
		ref = repo.DefaultBranch
	}
	target := repo
	target.DefaultBranch = ref
	body, err := client.fetchRawFile(ctx, target, cleanPath)
	if err != nil {
		return newToolOutput("github_read_file", command, "", start, false, err), err
	}
	content := string(body)
	truncated := false
	const maxChars = 60000
	if len(content) > maxChars {
		content = content[:maxChars] + "\n... (truncated)"
		truncated = true
	}
	header := fmt.Sprintf("%s @ %s — %s\n\n", repo.FullName, ref, cleanPath)
	return newToolOutput("github_read_file", command, header+content, start, truncated, nil), nil
}

type githubListFilesSkill struct {
	client *githubRepoClient
}

func (*githubListFilesSkill) Name() string { return "github_list_files" }

func (*githubListFilesSkill) Description() string {
	return "ดูรายชื่อไฟล์ใน GitHub repository"
}

func (s *githubListFilesSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repo_url": map[string]any{
				"type":        "string",
				"description": "GitHub repository URL (https://github.com/owner/repo)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path inside the repo (default: repo root)",
			},
		},
		"required":             []string{"repo_url"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "github_list_files",
			Description: "List the files and directories at a path in a GitHub repository. Use before github_read_file to find paths.",
			Parameters:  payload,
		},
	}
}

func (s *githubListFilesSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: github_list_files <repo-url> [path]")
		return newToolOutput("github_list_files", "github_list_files", "", time.Now(), false, err), err
	}
	path := ""
	if len(args) > 1 {
		path = args[1]
	}
	return s.list(ctx, args[0], path)
}

func (s *githubListFilesSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	repoURL, _ := args["repo_url"].(string)
	path, _ := args["path"].(string)
	return s.list(ctx, strings.TrimSpace(repoURL), strings.TrimSpace(path))
}

func (s *githubListFilesSkill) list(ctx context.Context, repoURL, path string) (Output, error) {
	start := time.Now()
	command := strings.TrimSpace("github_list_files " + repoURL + " " + path)
	if repoURL == "" {
		err := errors.New("repo_url is required")
		return newToolOutput("github_list_files", command, "", start, false, err), err
	}
	client := s.client
	if client == nil {
		client = newGitHubRepoClient("", "", nil)
	}
	target, ok := ExtractGitHubRepoURL(repoURL)
	if !ok {
		err := errors.New("invalid GitHub repository URL")
		return newToolOutput("github_list_files", command, "", start, false, err), err
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents", client.apiBaseURL, url.PathEscape(target.Owner), url.PathEscape(target.Repo))
	if path != "" {
		cleanPath, err := normalizeManifestRelativePath(path)
		if err != nil {
			return newToolOutput("github_list_files", command, "", start, false, err), err
		}
		endpoint += "/" + cleanPath
	}
	body, statusCode, err := client.doJSONRequest(ctx, endpoint)
	if err != nil {
		return newToolOutput("github_list_files", command, "", start, false, err), err
	}
	if statusCode == http.StatusNotFound {
		err := fmt.Errorf("path not found in %s/%s: %s", target.Owner, target.Repo, emptyFallback(path, "(root)"))
		return newToolOutput("github_list_files", command, "", start, false, err), err
	}
	if statusCode < 200 || statusCode >= 300 {
		err := fmt.Errorf("github list failed with status %d", statusCode)
		return newToolOutput("github_list_files", command, "", start, false, err), err
	}
	var entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
		Size int    `json:"size"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		// A file path returns an object, not an array — point the model at the right tool.
		err := errors.New("path is a file, not a directory — use github_read_file")
		return newToolOutput("github_list_files", command, "", start, false, err), err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s — %s:\n", target.Owner, target.Repo, emptyFallback(path, "(root)"))
	for _, e := range entries {
		if e.Type == "dir" {
			fmt.Fprintf(&b, "%s/\n", e.Path)
			continue
		}
		fmt.Fprintf(&b, "%s (%d bytes)\n", e.Path, e.Size)
	}
	return newToolOutput("github_list_files", command, strings.TrimSpace(b.String()), start, false, nil), nil
}
