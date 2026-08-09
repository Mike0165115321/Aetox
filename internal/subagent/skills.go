package subagent

// What one worker knows, kept where only that worker can reach it.
//
// An agent's home already holds who it is (AGENT.md) and what it has learned
// (MEMORY.md). This is the third thing: the specialist knowledge that is too
// big to sit in a prompt and too specific to belong to anybody else — the
// required fields of a tax invoice, the shape of a payroll workbook, the
// sections a civil complaint must carry.
//
// The format is not ours. A skill is a folder with a SKILL.md holding
// frontmatter `name` and `description` plus a free-form body — the Agent Skills
// layout, which this package's own scanner already reads for the shared shelf
// (internal/skill/discovery.go). Files written for other agent runtimes drop
// into a worker's folder unchanged, and nothing here had to invent a format.
//
// What *is* ours is the scope, and it is the only part that matters:
//
//   - Discovery runs against the running worker's home and nowhere else. Two
//     workers may each hold a skill called `invoice`; neither can see the
//     other's, because names are per-home rather than global.
//   - The shared shelf (~/.aetox/skills) stays what it is — everyone's. It is
//     the tempting place to put a form spec, and it is the wrong one: it works
//     on the first day and has quietly made every worker a generalist by the
//     thirtieth.
//   - Only agents get one. A helper is the assistant's own hands in a second
//     context and has no home to keep anything in; the main assistant gets
//     nothing at all, because it knows the office has a document writer and
//     must not know what a tax invoice needs.
//
// Loading is the standard's three levels, with no second mechanism invented for
// it: the tool block *is* the index. A skill's name and description sit in the
// worker's tool list (level 1, roughly a hundred tokens each); calling it hands
// back the body (level 2); files the body points at are read from disk only if
// the job needs them (level 3).

import (
	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// attachOwnSkills registers the worker's own skills into the registry that has
// just been filtered for it.
//
// After the filter and outside it, deliberately. The filter answers "which of
// the parent's tools may this worker be handed", and these are not the parent's
// — they are the worker's own, in its own folder, and passing them through an
// allowlist written to name tools would silence a worker's knowledge because it
// listed `doc_write`. Same reasoning that puts `ask_main` and the scoped
// `memory` outside it.
//
// Every failure is logged and skipped. A malformed SKILL.md is a file the user
// can fix; refusing to run the worker over it would take away the job they were
// trying to do with the other four.
func attachOwnSkills(filtered *skill.Registry, p Profile) {
	// `Desk != ""` is what makes a profile an agent (KindOf). A helper has no
	// home, and a scan of a folder that cannot exist is a disk hit per dispatch
	// bought for nothing.
	if filtered == nil || p.Desk == "" {
		return
	}
	own, errs := OwnSkills(p.Name)
	for _, s := range own {
		// A collision keeps what is already there. The worker's tools were
		// filtered by the rules above this line; a file dropped in a folder must
		// not be able to take a built-in tool's name and answer in its place.
		if regErr := filtered.Register(s.Skill, skill.SourceSkill); regErr != nil {
			debuglog.Msg("skill %q for %s not loaded: %v", s.Skill.Name(), p.Name, regErr)
		}
	}
	for _, scanErr := range errs {
		debuglog.Msg("%s skills: %v", p.Name, scanErr)
	}
}

// OwnSkill is one entry of a worker's own shelf, plus where it came from. The
// settings page marks the shipped ones so that "why can I not delete this" has
// an answer on the screen rather than in a folder.
type OwnSkill struct {
	Skill   skill.Skill
	Bundled bool
}

// OwnSkills resolves one agent's shelf: the SKILL.md folders in its home, then
// whatever shipped with it under a name the user has not taken.
//
// Exported because two readers need the same answer — the dispatcher, which
// registers these, and the settings page, which shows them. Resolved here once
// rather than at each: a page listing a set the running agent does not hold is
// the kind of disagreement nobody notices until it matters.
//
// The list is what the agent *has*, not necessarily what it ends up holding: a
// skill whose name collides with a tool already in the registry loses at
// registration, which only the dispatcher can know. That is a folder mistake
// the debug log names, and showing the file that is there beats hiding it.
func OwnSkills(name string) ([]OwnSkill, []error) {
	home, err := config.AgentSkillsPath(name)
	if err != nil {
		return nil, []error{err} // an unusable name; Load would already have refused it
	}
	own, errs := skill.DiscoverScoped([]string{home})
	// The user's folder first, then what ships with the worker — the same shape
	// the profile resolver uses for AGENT.md, where editing a shipped worker
	// means copying it out rather than fighting the app.
	out := make([]OwnSkill, 0, len(own))
	written := make(map[string]bool, len(own))
	for _, s := range own {
		out = append(out, OwnSkill{Skill: s})
		written[s.Name()] = true
	}
	shipped, shippedErrs := skill.DiscoverFS(bundledProfiles, bundledSkillsDir(name))
	for _, s := range shipped {
		if written[s.Name()] {
			continue // the user wrote their own; that is the one that runs
		}
		out = append(out, OwnSkill{Skill: s, Bundled: true})
	}
	return out, append(errs, shippedErrs...)
}

// bundledSkillsDir is where a shipped worker's skills sit inside the binary.
// Slash-separated because embed.FS is an fs.FS and always is, whatever the host
// platform's separator looks like.
func bundledSkillsDir(name string) string {
	return bundledAgentDir + "/" + name + "/" + config.AgentSkillsDir
}
