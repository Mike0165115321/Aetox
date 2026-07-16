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
