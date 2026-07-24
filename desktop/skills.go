package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// ListExternalSkills reports every discovered SKILL.md with its location, for
// the Settings → Skills page. Fresh scan per call — the page is the manager,
// it must see what's on disk right now, not what the engine loaded at boot.
func (a *App) ListExternalSkills() []skill.DiscoveredSkill {
	return skill.ListDiscovered(skill.DefaultDiscoveryPaths())
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
	if a.registry == nil {
		return "", fmt.Errorf("engine is not ready yet")
	}
	s, ok := a.registry.Get("plugin_install")
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
