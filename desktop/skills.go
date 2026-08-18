package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ListExternalSkills reports every discovered SKILL.md with its location, for
// the Settings → Skills page. Fresh scan per call — the page is the manager,
// it must see what's on disk right now, not what the engine loaded at boot.
func (a *App) ListExternalSkills() []skill.DiscoveredSkill {
	return jsonSlice(skill.ListDiscovered(skill.DefaultDiscoveryPaths()))
}

// SkillsDir is where skills actually live, for the page to display.
//
// Read from skill.DefaultSkillsDir rather than written into the UI's strings:
// two of the three places the Skills page named a path had it wrong (they said
// ~/.agents/skills, which belongs to opencode and which Aetox never reads), so
// anyone following the instructions dropped files somewhere nothing scans.
// A path the UI cannot state independently cannot drift from the truth.
func (a *App) SkillsDir() string {
	return skill.DefaultSkillsDir()
}

// SkillScanIssues reports SKILL.md files that were found but could not be read.
//
// The scan has always collected these and ListDiscovered has always dropped
// them, so a file with broken frontmatter simply never appeared and nothing
// said why. Being told the file is wrong is the difference between a two-minute
// fix and giving up on the feature.
func (a *App) SkillScanIssues() []string {
	_, errs := skill.ScanIssues(skill.DefaultDiscoveryPaths())
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

// InstallSkillFromZip installs a skill from a .zip the user picks.
//
// The third road in, and the one that covers everything the other two do not: a
// skill that arrived by email, by download, as a release asset, or from someone
// who does not publish it on GitHub at all.
//
// Returns "" with no error when the picker was dismissed — cancelling is not a
// failure and must not raise one.
func (a *App) InstallSkillFromZip() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "เลือกไฟล์ zip ของสกิล",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Skill archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || strings.TrimSpace(path) == "" {
		return "", err
	}
	dir := skill.DefaultSkillsDir()
	if dir == "" {
		return "", fmt.Errorf("หาโฟลเดอร์บ้านของผู้ใช้ไม่เจอ")
	}
	res, err := skill.InstallSkillsFromZip(path, dir)
	if err != nil {
		return "", err
	}
	a.rebuildMCP() // same re-bootstrap the GitHub route does: usable immediately
	return fmt.Sprintf("ติดตั้งแล้ว %d สกิล (%d ไฟล์): %s\nลงที่: %s",
		len(res.Names), res.Files, strings.Join(res.Names, ", "), res.Root), nil
}

// OpenSkillsFolder creates the skills directory if needed and reveals it, so
// installing by hand is "drop a folder here" — the same contract the prompts
// and sub-agents folders already had, and which this page was missing.
func (a *App) OpenSkillsFolder() error {
	dir := skill.DefaultSkillsDir()
	if dir == "" {
		return fmt.Errorf("หาโฟลเดอร์บ้านของผู้ใช้ไม่เจอ")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return a.revealInFileManager(dir)
}

// InstallSkillFromGitHub runs the plugin_install tool directly (a Settings
// button is explicit user consent — no agent loop, no approval prompt) and
// then re-bootstraps the engine so a bundle that contains a SKILL.md is
// usable immediately, closing plugin_install's old install-then-restart gap.
func (a *App) InstallSkillFromGitHub(repoURL string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", fmt.Errorf("repo url is required")
	}
	if a.cur().registry == nil {
		return "", fmt.Errorf("engine is not ready yet")
	}
	s, ok := a.cur().registry.Get("plugin_install")
	if !ok {
		return "", fmt.Errorf("plugin_install tool is not available")
	}
	tool, ok := s.(skill.Tool)
	if !ok {
		return "", fmt.Errorf("plugin_install tool is not invokable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := tool.ExecuteTool(ctx, map[string]any{"repo_url": repoURL})
	if err != nil {
		return "", err
	}
	a.rebuildMCP() // full re-bootstrap: re-discovers skills too
	return out.Content, nil
}

// RemoveExternalSkill deletes a discovered skill's directory and re-bootstraps
// the engine so its tool disappears immediately. Resolving by name (not a
// caller-supplied path) keeps deletion confined to the discovery roots.
func (a *App) RemoveExternalSkill(name string) error {
	for _, s := range skill.ListDiscovered(skill.DefaultDiscoveryPaths()) {
		if strings.EqualFold(s.Name, name) {
			// A bundled skill has no folder, and os.RemoveAll("") succeeds — so
			// without this the button would report success, the skill would
			// still be there, and the app would look broken rather than
			// principled. Say what to do instead: the override road is real.
			if s.Bundled {
				return fmt.Errorf("สกิล %q ติดมากับแอป ลบไม่ได้ — ถ้าอยากได้เนื้อหาอื่น ให้สร้างโฟลเดอร์ชื่อเดียวกันใน %s แล้วเขียน SKILL.md ทับ",
					s.Name, skill.DefaultSkillsDir())
			}
			if err := os.RemoveAll(s.Dir); err != nil {
				return err
			}
			a.rebuildMCP()
			return nil
		}
	}
	return fmt.Errorf("skill %q not found", name)
}

// RefreshSkills re-bootstraps the engine, picking up skills added or edited
// on disk outside the app.
func (a *App) RefreshSkills() {
	a.rebuildMCP()
}
