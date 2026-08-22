package main

import (
	"fmt"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Mike0165115321/Aetox/internal/agentpkg"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// Sending a worker somewhere else.
//
// The export button exists before the install button on purpose: it is the test
// of the package standard. Anything that fails to travel is something still
// coupled to this app rather than to the worker, and the only way to find out
// which is which is to pack one up and look
// (docs/architecture/agent-package-standard-2026-08-08.md, v2).
//
// Thin, like every binding on this page: what a package is lives in
// internal/agentpkg, and the rule that a worker is an overlay of the user's
// folder over the shipped one lives in internal/subagent. This assembles the
// two and names the file.

// ExportAgentPackage writes one worker to a .zip the user picks.
//
// Returns "" with no error when the picker was dismissed. Cancelling is not a
// failure and must not raise one, the same contract InstallSkillFromZip keeps.
func (a *App) ExportAgentPackage(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("ไม่รู้ว่าจะส่งออกเอเจนตัวไหน")
	}
	// Only agents. A helper is the assistant's own hands in a second context and
	// has no folder to pack, so the honest answer is the reason rather than an
	// empty archive (COMPANY.md §4).
	if kind := subagent.KindOf(name); kind != subagent.KindAgent {
		return "", fmt.Errorf("%s ไม่ใช่เอเจน จึงไม่มีโฟลเดอร์ให้ส่งออก", name)
	}
	sources := subagent.PackageSources(name)
	if len(sources) == 0 {
		return "", fmt.Errorf("ไม่พบโฟลเดอร์ของ %s", name)
	}
	// Read before the dialog opens. A broken mcp-servers.json should say so
	// while the user is still looking at the button they pressed, not after they
	// have chosen a filename.
	servers, err := agentpkg.PlacedServers(name)
	if err != nil {
		return "", err
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "ส่งออกเอเจน",
		DefaultFilename: name + "-agent.zip",
		Filters:         []wailsruntime.FileFilter{{DisplayName: "Agent package (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || strings.TrimSpace(path) == "" {
		return "", err
	}

	res, err := agentpkg.Export(path, agentpkg.Options{Name: name, Sources: sources, Servers: servers})
	if err != nil {
		return "", err
	}
	return exportSummary(res), nil
}

// exportSummary says what is in the file and, just as importantly, what is not.
//
// The three lines after the first are the whole reason this returns prose
// rather than a path: a seller has to be able to check that their own memory
// stayed behind and their own token did not travel, and the moment to show them
// that is the moment it happened.
func exportSummary(res agentpkg.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ส่งออก %s แล้ว %d ไฟล์\nไฟล์: %s", res.Name, res.Files, res.Path)
	if len(res.Servers) > 0 {
		fmt.Fprintf(&b, "\nเซิร์ฟเวอร์ที่ติดไปด้วย: %s", strings.Join(res.Servers, ", "))
	}
	if len(res.Asked) > 0 {
		keys := make([]string, 0, len(res.Asked))
		for _, f := range res.Asked {
			keys = append(keys, f.Key)
		}
		fmt.Fprintf(&b, "\nค่าที่ถอดออกและจะถามคนติดตั้งแทน: %s", strings.Join(keys, ", "))
	}
	if len(res.Left) > 0 {
		fmt.Fprintf(&b, "\nไม่ได้ติดไปด้วย: %s (ความจำเป็นของเครื่องนี้ ไม่ใช่ของที่ขาย)", strings.Join(res.Left, ", "))
	}
	return b.String()
}
