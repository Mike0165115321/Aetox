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
// it, and the index is `skills_list`: one line per document the worker can open
// (level 1), `skill_view` hands back a body (level 2), and files that body
// points at are read only if the job needs them (level 3).
//
// It was written believing the index was the tool block itself — one entry per
// skill in the worker's tool list. That never reached the model: progressive
// loading withholds every SourceSkill definition (skill.Dispatcher), which is
// what keeps a shelf of fifty from being charged to every request, and it does
// not ask whose shelf. So the documents were registered, executable by name, and
// listed nowhere the model could see, while this comment said otherwise for two
// releases. The doors are handed the worker's shelf now (attachOwnSkills), which
// is what makes the paragraph above true rather than intended.

import (
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/debuglog"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// openDoors are the two progressive-loading tools, which a worker gets its own
// instances of rather than the parent's. Named here because the registry build
// has to drop the inherited pair by the same names it re-registers them under,
// and two spellings of one list is how they drift apart.
var openDoors = []string{"skills_list", "skill_view"}

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
func attachOwnSkills(filtered *skill.Registry, p Profile, doors []string) {
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
		if regErr := filtered.Register(s.AsSkill(), skill.SourceSkill); regErr != nil {
			debuglog.Msg("skill %q for %s not loaded: %v", s.Name, p.Name, regErr)
		}
	}
	// The doors, pointed at this worker's shelf as well as the machine's. Only
	// the ones the profile was already going to be handed: the registry build
	// dropped them by name after running its filters, so a worker whose
	// `tools:` or desk never included skills_list does not gain it here.
	list, view := skill.ProgressiveTools(own)
	for _, door := range doors {
		var s skill.Skill
		switch door {
		case "skills_list":
			s = list
		case "skill_view":
			s = view
		default:
			continue
		}
		if regErr := filtered.Register(s, skill.SourceBuiltin); regErr != nil {
			debuglog.Msg("%s for %s not loaded: %v", door, p.Name, regErr)
		}
	}
	for _, scanErr := range errs {
		debuglog.Msg("%s skills: %v", p.Name, scanErr)
	}
}

// OwnSkills resolves one agent's shelf: the SKILL.md folders in its home, then
// whatever shipped with it under a name the user has not taken.
//
// Returned in the scanner's own detailed form rather than as registered skills,
// because the two consumers need different halves of it: the registry wants an
// invokable skill, `skill_view` wants the body and the folder beside it, and the
// settings page wants the `Bundled` flag so that "why can I not delete this" has
// an answer on the screen rather than in a folder.
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
func OwnSkills(name string) ([]skill.DiscoveredSkill, []error) {
	home, err := config.AgentSkillsPath(name)
	if err != nil {
		return nil, []error{err} // an unusable name; Load would already have refused it
	}
	own, errs := skill.ScopedSkills([]string{home})
	// The user's folder first, then what ships with the worker — the same shape
	// the profile resolver uses for AGENT.md, where editing a shipped worker
	// means copying it out rather than fighting the app.
	out := make([]skill.DiscoveredSkill, 0, len(own))
	written := make(map[string]bool, len(own))
	for _, s := range own {
		out = append(out, s)
		// Folded, unlike the first version of this line. The shared shelf's
		// override match is case-insensitive (bundled_skills.go) and this one was
		// not, so `Tax-Invoice` in a worker's folder shadowed nothing and shipped
		// a second copy of the skill beside the one it meant to replace.
		written[strings.ToLower(s.Name)] = true
	}
	shipped, shippedErrs := skill.EmbeddedSkills(bundledProfiles, bundledSkillsDir(name))
	for _, s := range shipped {
		if written[strings.ToLower(s.Name)] {
			continue // the user wrote their own; that is the one that runs
		}
		out = append(out, s)
	}
	return out, append(errs, shippedErrs...)
}

// bundledSkillsDir is where a shipped worker's skills sit inside the binary.
// Slash-separated because embed.FS is an fs.FS and always is, whatever the host
// platform's separator looks like.
func bundledSkillsDir(name string) string {
	return bundledAgentDir + "/" + name + "/" + config.AgentSkillsDir
}
