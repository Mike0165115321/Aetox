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
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	gh "github.com/Mikedev115/Aetox/internal/github"
	"github.com/Mikedev115/Aetox/internal/model"
)

const (
	// Both hosts are stated once, in internal/github, and read from there —
	// the package that owns the connection owns where it connects to.
	defaultGitHubAPIBaseURL = gh.APIBaseURL
	defaultGitHubRawBaseURL = gh.RawBaseURL
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

// githubTreeEntry is one row of the git-trees listing used to find a plain
// SKILL.md repository's layout.
type githubTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" | "tree"
	Size int64  `json:"size"`
}

const skillFileName = "SKILL.md"

// There is deliberately no size or file-count ceiling on an install.
//
// An earlier version had one, on the theory that a repository which merely
// happens to contain a SKILL.md might be a monorepo. That is Aetox deciding how
// much of the user's own disk they are allowed to spend on something they went
// and asked for, by name, by URL. Nothing is uploaded, nothing is hosted, no
// cost lands anywhere but their machine — so the judgement was not ours to
// make, and a refusal would only send them to download it by hand anyway.
//
// What stays is the part that is not a policy: the set installs whole or not at
// all, paths that escape the install folder are refused, and a listing GitHub
// truncated is refused because completeness cannot be established from it.
// Those are about correctness and safety, not about size.

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
	return "ติดตั้งสกิลจากที่เก็บบน GitHub"
}

func (s *pluginInstallSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repo_url": map[string]any{
				"type":        "string",
				"description": "GitHub repository URL.",
			},
		},
		"required":             []string{"repo_url"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name: "plugin_install",
			// This said "from a supported GitHub repository manifest", and the
			// parameter said the repo must define aetox-plugin.json. Neither was
			// true — a repo with no manifest is the ordinary case and installs
			// through installPlainSkills — so the model refused repositories
			// this tool installs happily and reported the skill could not be
			// had. The false requirement is gone; what is left says only what
			// the tool does, which is all a description owes.
			Description: "Install skills from a GitHub repository.",
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
		// No manifest is the normal case, not an error: a skill published on
		// GitHub is a folder with a SKILL.md in it. Fall back to reading the
		// repository's own layout.
		return s.installPlainSkills(ctx, client, repo, start)
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

// plainSkill is one SKILL.md found in a repository with no aetox-plugin.json,
// together with the files that sit beside it.
type plainSkill struct {
	name   string   // the folder it installs as
	prefix string   // "" for a SKILL.md at the repo root
	files  []string // repo-relative paths, SKILL.md first
	bytes  int64
}

// notSkillMaterial are directory names that hold other people's code rather
// than a published skill. A SKILL.md underneath one of these belongs to a
// dependency or a build artefact, not to this repository.
var notSkillMaterial = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, "out": true, "coverage": true, "__pycache__": true,
}

// findPlainSkills reads a repository listing the way a person would: the folder
// that directly contains a SKILL.md is the skill.
//
// Depth is not part of that definition, and an earlier version that searched
// only the root and one level down missed the most common published layout of
// all — a `skills/` directory with a folder per skill, which is what
// shadcn/ui and Anthropic's own repositories use. It reported "no skill found"
// on repositories that plainly had two.
//
// What is excluded is by kind, not by depth: dependency and build directories,
// and anything under a dot-directory. A SKILL.md inside node_modules came with
// somebody else's package.
func findPlainSkills(entries []githubTreeEntry, repoName string) []plainSkill {
	blobs := make([]githubTreeEntry, 0, len(entries))
	for _, e := range entries {
		if e.Type == "blob" {
			blobs = append(blobs, e)
		}
	}

	// Root SKILL.md wins outright: the repository is the skill, so a nested one
	// is part of its material rather than a sibling skill.
	for _, e := range blobs {
		if e.Path == skillFileName {
			return []plainSkill{collectPlainSkill(blobs, "", sanitizeSkillName(repoName))}
		}
	}

	var dirs []string
	seen := map[string]bool{}
	for _, e := range blobs {
		dir, file := path.Split(e.Path)
		if file != skillFileName {
			continue
		}
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" || seen[dir] || !isSkillLocation(dir) {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	// Name by the folder, since that is what the author called the skill —
	// `skills/shadcn/` installs as `shadcn`, not as `skills-shadcn`. Two folders
	// at different paths can share a basename, so a collision falls back to the
	// full path to stay distinguishable rather than overwriting.
	basenames := map[string]int{}
	for _, dir := range dirs {
		basenames[path.Base(dir)]++
	}
	out := make([]plainSkill, 0, len(dirs))
	for _, dir := range dirs {
		base := path.Base(dir)
		name := base
		if basenames[base] > 1 {
			name = strings.ReplaceAll(dir, "/", "-")
		}
		out = append(out, collectPlainSkill(blobs, dir+"/", sanitizeSkillName(name)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// isSkillLocation rejects directories whose contents came from somewhere else.
func isSkillLocation(dir string) bool {
	for _, seg := range strings.Split(dir, "/") {
		if strings.HasPrefix(seg, ".") || notSkillMaterial[strings.ToLower(seg)] {
			return false
		}
	}
	return true
}

// collectPlainSkill gathers the whole set under prefix — SKILL.md and every
// file beside it, at any depth, because that is what a skill is: the document
// plus the scripts, references and templates it sends the model to.
//
// SKILL.md is listed first so it is written first, and the whole set is taken
// or none of it — see plainSkill.tooBig.
func collectPlainSkill(blobs []githubTreeEntry, prefix, name string) plainSkill {
	skill := plainSkill{name: name, prefix: prefix}
	var rest []githubTreeEntry
	for _, e := range blobs {
		if prefix != "" && !strings.HasPrefix(e.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(e.Path, prefix)
		if rel == "" || strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.") {
			continue // dotfiles: .github workflows and the like are not skill material
		}
		if rel == skillFileName {
			skill.files = append(skill.files, e.Path)
			continue
		}
		rest = append(rest, e)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Path < rest[j].Path })

	for _, e := range rest {
		skill.bytes += e.Size
		skill.files = append(skill.files, e.Path)
	}
	return skill
}

// sanitizeSkillName turns a repo or folder name into a single safe path
// segment — the install root must never be escaped by something named in a
// repository we do not control.
func sanitizeSkillName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r == ' ' || r == '.':
			return '-'
		default:
			return -1
		}
	}, name)
	name = strings.Trim(name, "-_")
	if name == "" {
		return "skill"
	}
	return name
}

// installPlainSkills handles a repository with no aetox-plugin.json.
func (s *pluginInstallSkill) installPlainSkills(ctx context.Context, client *githubRepoClient, repo githubRepoMetadata, start time.Time) (Output, error) {
	fail := func(err error) (Output, error) {
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, "", start, false, err), err
	}

	entries, truncated, err := client.fetchRepoTree(ctx, repo)
	if err != nil {
		return fail(err)
	}
	skills := findPlainSkills(entries, repo.Repo)
	if len(skills) == 0 {
		content := fmt.Sprintf(
			"no skill found in %s: no folder in it contains a %s (directories like node_modules, vendor and build output are not searched). It also does not define %s.",
			repo.FullName, skillFileName, aetoxPluginManifestName)
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, content, start, false, nil), nil
	}

	installRoot, err := s.resolveInstallRoot()
	if err != nil {
		return fail(err)
	}

	// A truncated listing means the set is not knowably complete, and a skill
	// has to arrive whole. Stopping here is the only honest answer.
	if truncated {
		content := fmt.Sprintf(
			"cannot install from %s: GitHub truncated its file listing, so the skill's full set of files cannot be determined. Install it by hand, or point at the specific skill's repository.",
			repo.FullName)
		return newToolOutput("plugin_install", "plugin_install "+repo.FullName, content, start, false, nil), nil
	}

	lines := []string{fmt.Sprintf("installed %d skill(s) from %s", len(skills), repo.FullName), "Install root: " + filepath.ToSlash(installRoot)}
	for _, sk := range skills {
		targetRoot := filepath.Join(installRoot, sk.name)
		if err := os.MkdirAll(targetRoot, 0o755); err != nil {
			return fail(err)
		}
		for _, srcPath := range sk.files {
			rel, err := normalizeManifestRelativePath(strings.TrimPrefix(srcPath, sk.prefix))
			if err != nil {
				return fail(fmt.Errorf("invalid path %q in %s: %w", srcPath, repo.FullName, err))
			}
			body, err := client.fetchRawFile(ctx, repo, srcPath)
			if err != nil {
				return fail(err)
			}
			targetPath := filepath.Join(targetRoot, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fail(err)
			}
			if err := os.WriteFile(targetPath, body, 0o644); err != nil {
				return fail(err)
			}
		}
		lines = append(lines, fmt.Sprintf("  %s — %d file(s), %d KB", sk.name, len(sk.files), sk.bytes/1024))
	}
	return newToolOutput("plugin_install", "plugin_install "+repo.FullName, strings.Join(lines, "\n"), start, false, nil), nil
}

func (s *pluginInstallSkill) resolveInstallRoot() (string, error) {
	if strings.TrimSpace(s.installRoot) != "" {
		return filepath.Abs(strings.TrimSpace(s.installRoot))
	}
	dir := DefaultSkillsDir()
	if dir == "" {
		return "", fmt.Errorf("resolve plugin install root: cannot determine home directory")
	}
	return dir, nil
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

// fetchRepoTree lists every file in the repository at its default branch.
//
// Needed because almost no repository in the world ships aetox-plugin.json:
// a skill published on GitHub is a folder with a SKILL.md in it. Without a
// listing there is no way to find that folder, so install only ever worked for
// repositories written specifically for Aetox — which is close to none.
//
// GitHub caps this response and sets `truncated` rather than paginating. That
// is reported to the caller instead of being swallowed: a partial listing that
// looks complete is how a skill silently installs with half its files.
func (c *githubRepoClient) fetchRepoTree(ctx context.Context, repo githubRepoMetadata) ([]githubTreeEntry, bool, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		c.apiBaseURL, url.PathEscape(repo.Owner), url.PathEscape(repo.Repo), url.PathEscape(repo.DefaultBranch))
	// Its own limit, not doJSONRequest's: this response is sized by the
	// repository, and the shared 1 MB metadata limit silently halved it.
	body, statusCode, err := c.doRequest(ctx, endpoint, "application/vnd.github+json", treeReadLimit)
	if err != nil {
		return nil, false, fmt.Errorf("list repository files: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, false, fmt.Errorf("list repository files failed with status %d", statusCode)
	}
	var payload struct {
		Tree      []githubTreeEntry `json:"tree"`
		Truncated bool              `json:"truncated"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, fmt.Errorf("parse repository listing: %w", err)
	}
	return payload.Tree, payload.Truncated, nil
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

// Read limits, per kind of response — one number for all of them was the bug.
//
//   - metadataReadLimit: search results, repo metadata, a plugin manifest. All
//     small and bounded by their own shape.
//   - treeReadLimit: a recursive file listing, whose size tracks the repository
//     rather than the skill. 6,362 files came to 1.6 MB, so the old 1 MB was
//     under real-world traffic, not above it.
//   - fileReadLimit: one file being installed. Bounded by what a skill may
//     weigh in total, since a single template or asset can be most of one.
const (
	metadataReadLimit = 1 << 20 // 1 MiB
	treeReadLimit     = 32 << 20
	fileReadLimit     = 256 << 20
)

func (c *githubRepoClient) doJSONRequest(ctx context.Context, endpoint string) ([]byte, int, error) {
	return c.doRequest(ctx, endpoint, "application/vnd.github+json", metadataReadLimit)
}

func (c *githubRepoClient) doRawRequest(ctx context.Context, endpoint string) ([]byte, int, error) {
	return c.doRequest(ctx, endpoint, "application/octet-stream", fileReadLimit)
}

func (c *githubRepoClient) doRequest(ctx context.Context, endpoint string, accept string, limit int64) ([]byte, int, error) {
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
	if token := gh.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := readBounded(resp.Body, limit)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// readBounded reads at most limit bytes and says so when there were more.
//
// The obvious spelling, io.ReadAll(io.LimitReader(r, limit)), cannot tell "the
// response was exactly limit bytes" from "the response was larger and I stopped
// early" — it returns the truncated bytes with a nil error. That is how a
// 1.6 MB repository listing arrived cut in half and surfaced as
// "parse repository listing: unexpected end of JSON input": a size problem
// wearing a syntax problem's clothes, with the real number nowhere on screen.
//
// Reading one byte past the limit makes the overflow detectable, and the error
// names both figures so the next one is diagnosable from the message alone.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response is larger than the %d KB this request allows", limit/1024)
	}
	return body, nil
}

func emptyFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
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
