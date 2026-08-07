package skill

import (
	"embed"
	"path"
	"sort"
	"strings"
)

// Skills that ship inside the binary.
//
// The two-source shape is the one desks (internal/mode), agent profiles
// (internal/subagent) and prompt presets already use, ARCHITECTURE.md §35:
// bundled files are compiled in so a fresh install works with nothing to
// download, and a user folder of the same name in ~/.aetox/skills wins
// outright — editing a shipped skill is copying it out, never fighting the app.
//
// **Why a skill rather than a prompt layer or a tool description**, for the
// knowledge in skills/aetox: a SKILL.md costs nothing until the model opens it
// (progressive.go), while a paragraph in internal/prompt is paid by every
// session on every request whether or not the topic ever came up, and a
// sentence in a tool description is paid on every request by every model in the
// tool block (desktop/tool_budget_test.go). The mechanism was already complete
// — mode.Carries keeps every skill on every desk, and prompt.capability tells
// the model to go and look before saying no — and what was missing was the
// file.
//
//go:embed skills/*/SKILL.md
var bundledSkillFS embed.FS

// bundledSkillRoot is the embedded directory; one folder per skill, mirroring
// the on-disk layout so a bundled skill and an installed one are the same shape.
const bundledSkillRoot = "skills"

// bundledSkills parses the compiled-in documents. A malformed one is dropped
// rather than reported: unlike a user's file it cannot be fixed on the machine
// where it fails, and the tests below are where it is meant to be caught.
func bundledSkills() []DiscoveredSkill {
	entries, err := bundledSkillFS.ReadDir(bundledSkillRoot)
	if err != nil {
		return nil
	}
	out := make([]DiscoveredSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, readErr := bundledSkillFS.ReadFile(path.Join(bundledSkillRoot, entry.Name(), skillFileName))
		if readErr != nil {
			continue
		}
		name, description, body, parseErr := parseSkillMarkdown(string(raw))
		if parseErr != nil {
			continue
		}
		if name == "" {
			name = entry.Name()
		}
		// Dir stays empty, and that is the fact everything else keys on: there
		// is no folder, so there is nothing to reveal, nothing to delete, and
		// no file to read beside the document. Bundled says so out loud rather
		// than making each caller infer it from an empty string.
		out = append(out, DiscoveredSkill{Name: name, Description: description, Bundled: true, body: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// withBundled prepends the bundled skills that no user folder has taken over.
//
// Name match is case-insensitive because that is how skill_view resolves a
// name; a user folder that only differs in case would otherwise install a
// second skill the model reaches by coin flip.
func withBundled(found []DiscoveredSkill) []DiscoveredSkill {
	taken := make(map[string]bool, len(found))
	for _, d := range found {
		taken[strings.ToLower(d.Name)] = true
	}
	out := make([]DiscoveredSkill, 0, len(found)+1)
	for _, b := range bundledSkills() {
		if taken[strings.ToLower(b.Name)] {
			continue
		}
		out = append(out, b)
	}
	return append(out, found...)
}
