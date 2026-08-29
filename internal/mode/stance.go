package mode

import (
	"strings"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// Stance is the second axis (DECISIONS.md §106): not *what is on the desk* —
// that is Mode, and it is fixed for a session's life — but *how this turn
// runs*, which the user changes at any moment from the composer.
//
// The rule that lets the two coexist, and the only reason switching mid-session
// does not break COMPANY.md §6.3:
//
//	**A stance may only subtract from the desk. It can never add.**
//
// §6.3 freezes the desk because a context must never carry another desk's
// tools. Removing tools cannot contaminate anything, and returning to
// StanceAct restores only what this desk already had — so the guarantee holds
// unchanged while the dial moves.
//
// Three things a stance is deliberately not:
//
//   - **Not a permission tier.** The safety gate and the approval rules are
//     identical in every stance (§6.2). What a stance trims is which tool
//     definitions a session carries at all — a token decision, made at the same
//     seam Mode makes it. The user's dial for approval strength is
//     safety.ApprovalMode, which is a different hand entirely and is named
//     ระดับการอนุมัติ on screen precisely so the two cannot be read as siblings.
//   - **Not an identity.** A stance says what the work is, never who is doing
//     it (§44.0). Direction() is held to the same line every manifest is.
//   - **Not a file.** Modes are manifests on disk because a fourth desk should
//     be a file rather than a release. Stances are a closed set in code because
//     each one is a behaviour the engine implements, not a list it reads — a
//     user-written stance would be a prompt with no mechanism behind it, which
//     §106.3 rules out by name.
type Stance string

const (
	// StanceAct is ลงมือ: today's behaviour, unchanged.
	//
	// The empty string on purpose. It is the zero value, so a session opened
	// before stances existed, a database column that defaults to '', and a
	// caller that never sets the field all mean the same thing and mean it
	// without a branch — the same trick sessions.agent uses for "the main
	// assistant" (desktop/db.go, migration v9).
	StanceAct Stance = ""

	// StanceConsult is คู่คิด: the conversation without the errand.
	//
	// It carries no tool definitions at all, which is the whole mechanism and
	// the whole measurement — the floor desktop/tool_budget_test.go pins at
	// ~9,700 tokens is not paid by a turn that was only ever going to be an
	// answer. It exists because asking for an opinion and getting six tool
	// calls is a real and daily failure, and no amount of prompt wording fixes
	// a tool the model can see.
	StanceConsult Stance = "consult"

	// StancePlan is วางแผน: look at everything, change nothing.
	//
	// It keeps every tool that only reads and withholds every tool that writes,
	// runs, or hands work to somebody who will. The turn ends in a plan, and the
	// user moves back to ลงมือ to have it carried out — which is one press, and
	// theirs.
	//
	// It is also the missing half of ประตูส่งไม้ (COMPANY.md §8, "ยังไม่สร้าง"):
	// that door needs a plan written from a conversation, and this is the mode
	// that produces one.
	StancePlan Stance = "plan"
)

// planKeeps is วางแผน's allow-list: the tools that only ever look.
//
// **An allow-list, not a deny-list, and the direction is the whole point.** A
// tool added next month is withheld here until somebody decides it reads —
// which is the safe way for this to fail. A deny-list would hand a new writing
// tool to the one mode whose entire promise is that it changes nothing, and it
// would do it silently.
//
// Whole tools only, which costs something worth naming. `browser` is packed
// (open/read/click/type) and `shell` is packed (run/output/kill/list), so both
// go entirely, taking two reads with them — a plan cannot click through a page,
// and has `web_fetch` instead. Narrowing a pack per stance is possible (the
// machinery is skill.Packed, and subagent.FilterRegistry already does it for a
// profile) and is deliberately not wired here yet: a stance narrows at the same
// granularity a desk does, and making it the first thing to narrow finer is a
// change to make on purpose rather than in passing.
//
// `github` is the pack that survives whole — all four of its actions read.
var planKeeps = map[string]bool{
	// Looking at the disk.
	"read": true, "list": true, "glob": true, "grep": true,
	// Senses. Reading a PDF or a screenshot changes nothing.
	"image_ocr": true, "video_ocr": true, "pdf_read": true, "audio_transcribe": true,
	// Looking things up. web_fetch is a GET the user's machine makes; it is the
	// same act as reading a file somebody else owns.
	"web_search": true, "web_fetch": true,
	// The browser's reading half, which วางแผน could not have until a stance
	// could narrow a pack (Mode.AllowsAction, Dispatcher.WithActions). Before
	// that this list could only say "browser" or nothing, and saying nothing
	// cost a plan every page that has to be clicked into existence — the ones
	// web_fetch cannot reach and the ones a plan most often needs to see.
	//
	// `browser_read` is two actions: `read` and `scroll` share it (packed.go),
	// because a page's second screen is not a second permission.
	//
	// Absent on purpose, each for the reason its own note in packed.go gives:
	// `browser_click`, `browser_type` and `browser_dialog` act on the page;
	// `browser_tabs` can take one away from under the user; and
	// `browser_capture` is a right of its own — "may read this page" and "may
	// see it" are different grants, and a screenshot carries whatever else the
	// user has on screen. An allow-list fails safe by leaving something out,
	// so any of these is one line whenever it is decided on.
	"browser_open": true, "browser_read": true, "browser_wait": true,
	"browser_back": true, "browser_console": true, "browser_network": true,
	// Reading code and repositories. `repo_map` is the shape of a project for
	// about a thousand tokens, which is the single most useful thing a plan can
	// be built on and was missing here only because it landed after this list
	// was written.
	"diagnostics": true, "symbol": true, "repo_map": true,
	"github":      true, "github_search": true, "github_read_file": true,
	"github_list_files": true, "github_repo_summary": true,
	// Pull requests, the reading half only. This is the first pack วางแผน
	// carries in PART rather than whole or not at all - `pr_create` and
	// `pr_comment` are on the same tool and are absent, which the per-action
	// filter (AllowsAction) is what makes possible. Reading the state of the
	// work is exactly what a plan is built from.
	"pr_list": true, "pr_read": true, "pr_checks": true,
	// Reading the automations that exist, never starting or changing one.
	"n8n_workflow_list": true, "n8n_workflow_read": true,
	"windmill_workspace_list": true, "windmill_flow_list": true, "windmill_flow_read": true,
	// How the assistant runs itself while planning. `todo_write` writes nothing
	// to the machine — it is the plan taking shape where the user can watch it,
	// which is this mode's own output. `calc` runs a script in a sandbox that
	// cannot reach a file or the network (see its note in category.go). `memory`
	// is deliberately absent: it proposes something for the user to approve
	// later, and a mode that changes nothing should not be leaving anything
	// behind to be decided.
	"ask_user": true, "todo_write": true, "calc": true, "time": true,
	"skills_list": true, "skill_view": true, "session_search": true,
	// The desk, whole. It is the one pack วางแผน can carry entire, and for a
	// reason worth stating rather than discovering: every action in it only
	// looks — put a file in front of the user, see what is there, take one
	// away. Nothing on disk changes. `desk_terminal` is deliberately not part
	// of it (packed.go), which is exactly why this line can be one word.
	"desk": true, "desk_list": true, "help": true, "echo": true,
}

// planShape is the four headings a plan comes back under, with what belongs
// beneath each — and it is the one place the shape is written down.
//
// **Two callers produce a plan, and they are not the same caller.** This stance
// is the main agent writing to the person who will decide; the `plan` sub-agent
// profile (internal/subagent/profiles/subagents/plan.md) is a delegate writing
// to the main agent that will act. They differ legitimately in the gloss under
// each heading — that profile is a coding helper and can say "the file and line
// you read it in", while a stance is available at every desk and cannot assume
// the work has files at all.
//
// What they must not differ on is the headings, and that is what this list is
// for. A user who asks for a plan twice and is handed two different shapes has
// learned the shape of neither. subagent.TestThePlanProfileKeepsTheSharedPlanShape
// is the coupling that makes drift a failing test rather than a discovery
// somebody makes months later.
//
// Why a shape at all, when the direction below already said what to talk about:
// it said what to *mention* and never what to *produce*, so the turn came back
// as prose and the user could not tell a plan from an answer. Nothing else in
// internal/prompt fills that in — `longform` is about where a long answer lands
// (a .md file), never how it is built, and วางแผน does not even get that layer
// because it has no `write`. This is the only mode whose entire output is one
// document, so it is the only one that had to say so.
var planShape = []struct{ Heading, Under string }{
	{
		"What is there now",
		"the few facts about how things actually stand that decide the approach, each with where you " +
			"found it. Not a summary of everything you looked at.",
	},
	{
		"What to change",
		"the steps in the order they should happen, each naming the actual thing it touches — concrete " +
			"enough that carrying it out is following it rather than working it out again.",
	},
	{
		"What could go wrong",
		"what the obvious version breaks. If you looked and found nothing, say that — it is a finding, " +
			"not an empty heading.",
	},
	{
		"What you are unsure of",
		"the questions whose answers would change the plan, specific enough that someone can answer them " +
			"without redoing your reading.",
	},
}

// PlanHeadings reports the plan's headings in order, for the callers that must
// agree with them rather than restate them.
//
// A copy, for the reason Stances() hands back a copy: a caller that sorts or
// truncates the result must not be able to edit the shape every plan is written
// against.
func PlanHeadings() []string {
	out := make([]string, 0, len(planShape))
	for _, s := range planShape {
		out = append(out, s.Heading)
	}
	return out
}

// planShapeBlock renders the shape as the prompt states it. Built from
// planShape rather than written out again below, so the headings in the prompt
// are literally the ones PlanHeadings reports and the two cannot drift inside
// this one file.
func planShapeBlock() string {
	var b strings.Builder
	for _, s := range planShape {
		b.WriteString("**" + s.Heading + "** — " + s.Under + "\n")
	}
	return b.String()
}

// stances is the whole set, in the order the picker draws them. StanceAct
// leads because it is the way back: a control you can enter and not leave is
// not a switch (§106.3).
//
// Ordered least restrictive to most, after the way back: everything, then
// everything that only looks, then nothing. A picker whose positions get
// steadily narrower reads as one dial rather than three unrelated buttons.
//
// Three, and the fourth is not coming. มุ่งเป้า was designed in full and
// declined by the owner on 2026-08-14 (§106.10): it filters nothing, so it is
// not a stance in the sense this file implements — it is a control loop in the
// turn executor, with a verification call every round and a cost that
// multiplies. If it is ever reopened it needs a pinned goal, a checkable finish
// condition, a checker that is not the model that just declared victory, and a
// hard ceiling. Not a constant here.
var stances = []Stance{StanceAct, StancePlan, StanceConsult}

// Stances reports every stance a session may be put into, in picker order.
//
// The names are ids, not labels. What each one is called on screen is a locale
// string, the same split COMPANY.md §2 already keeps between a desk's code name
// and its room's label — so a translation cannot invent a stance and the engine
// cannot dictate a word.
func Stances() []Stance {
	out := make([]Stance, len(stances))
	copy(out, stances)
	return out
}

// NormalizeStance reads a stance name off a database column, a preference file
// or a frontend call and answers with one this build implements.
//
// Anything unknown becomes StanceAct rather than an error, and that is the
// safe direction on purpose: the value arrives from a row that may have been
// written by a later build (a session left in วางแผน, opened after a
// downgrade), and the honest fallback for "a way of working I do not have" is
// the one that withholds nothing. Failing closed here would open a session that
// silently carries no tools and gives no reason why.
func NormalizeStance(name string) Stance {
	s := Stance(strings.ToLower(strings.TrimSpace(name)))
	for _, known := range stances {
		if s == known {
			return s
		}
	}
	return StanceAct
}

// String makes Stance printable and gives the desktop layer something to store.
// StanceAct is "" — see the constant for why that is the right stored value.
func (s Stance) String() string { return string(s) }

// AllowsTool reports whether this stance leaves the named built-in or
// workbench tool on the desk. The mirror of Mode.AllowsTool, and like it a
// token decision rather than the safety gate.
//
// **For built-in and workbench tools only**, exactly as Mode.AllowsTool is: an
// MCP tool or a skill has to go through Carries, which knows the difference.
func (s Stance) AllowsTool(name string) bool {
	switch s {
	case StanceConsult:
		return false
	case StancePlan:
		return planKeeps[strings.ToLower(strings.TrimSpace(name))]
	}
	return true
}

// AllowsAction is AllowsTool one level down, for a packed tool: not "does this
// stance keep `browser`" but "does it keep `browser_read`".
//
// Two ways an action is kept, and the second is what makes the tables above
// readable. Either the action's own permission name is on the list — which is
// how `desk_list` and the four `github_*` entries already read — or the PACKED
// name is, which is the one-word way of saying "this pack, whole", exactly what
// `"desk": true` and `"github": true` were already written to mean.
//
// วางแผน is the stance this exists for. Before it, a pack holding one act that
// writes had to go entirely, and the note above this file's allow-list said so:
// browser and shell took four reads with them because they each carry one act
// that does not read. Now they do not have to.
func (s Stance) AllowsAction(tool, action string) bool {
	switch s {
	case StanceConsult:
		return false
	case StancePlan:
		if planKeeps[strings.ToLower(strings.TrimSpace(action))] {
			return true
		}
		return planKeeps[strings.ToLower(strings.TrimSpace(tool))]
	}
	return true
}

// CarriesNothing reports whether this stance withholds every tool there is.
//
// A question of its own rather than something derived, because it cannot be
// derived: AllowsTool answers one name at a time, and probing it with a sample
// name makes the answer depend on which name got picked. That is not
// hypothetical — the first version of this asked `AllowsTool("")` and worked
// while คู่คิด was the only narrowing stance. วางแผน answers from an allow-list,
// the empty string is not in it, and the probe reported วางแผน as carrying
// nothing at all: it lost its prompt layers and read as a broken คู่คิด.
//
// The prompt is the caller that needs it (prompt.Desk.ToolLess): half those
// layers name no tool that Carries could be asked about.
func (s Stance) CarriesNothing() bool { return s == StanceConsult }

// Carries is the stance's half of "is this registered tool on this desk?",
// asked the same way and answered next to Mode.Carries so the two cannot drift.
//
// A skill is the one thing a stance never takes away, which is the sentence
// Mode.Carries already holds: a SKILL.md costs nothing until the model asks for
// it, contributes no tool definition (Dispatcher.ToolDefinitions skips
// SourceSkill outright), and is how the user's own `/skill-name` command
// reaches the dispatcher. Withholding it would buy no tokens and would break a
// command the user typed on purpose.
//
// Everything else — built-ins, the desktop's workbench tools, and MCP — is one
// question here, and that is the difference from Mode.Carries worth stating:
// Mode has to know which registry a name came from, because its lists group
// tools by what they do and no such grouping can express "this one reaches a
// server". A stance groups by nothing. It is a statement about the *kind of
// turn* this is, and a turn that is not touching anything is not touching an
// MCP server either.
func (s Stance) Carries(name string, source skill.Source) bool {
	if source == skill.SourceSkill {
		return true
	}
	// An MCP tool is withheld by วางแผน without being looked at, because there
	// is nothing here that could look. A server's tools arrive named by whoever
	// wrote the server, and `jira_create_issue` and `jira_search` are the same
	// kind of string to this package — so an allow-list cannot cover them and a
	// guess would be a mode that promises to change nothing while calling
	// something that does.
	//
	// The cost is real: a user who works mostly through MCP plans without it.
	// The alternative is a promise this code cannot keep, and the honest fix is
	// per-server or per-tool declarations from the server side, which is a
	// decision of its own rather than a default chosen here.
	if source == skill.SourceMCP && s == StancePlan {
		return false
	}
	// Same rule as Mode.Carries, and it has to be the same or the two axes
	// disagree about what a pack is: carried when any action survives, because
	// what the session is handed is a copy offering only those.
	if actions := skill.PackedActions(name); len(actions) > 0 {
		for _, action := range actions {
			if s.AllowsAction(name, action) {
				return true
			}
		}
		return false
	}
	return s.AllowsTool(name)
}

// Direction is what the system prompt gains from this stance, folded in after
// the desk's own direction (§106.4 — later context outweighs earlier, so
// position is the only mechanism there is for saying which of two instructions
// wins). Empty for StanceAct, which adds nothing because it changes nothing.
//
// Held to the same line every manifest is: it says what the work is, never who
// the assistant is (§44.0). "This turn is thinking work" is a description of
// the work; "you are a thoughtful advisor" would be the thing §44.0 deleted.
func (s Stance) Direction() string {
	if s == StancePlan {
		// The sentence that does the work is the one about not asking for
		// permission. A model that has just been refused a writing tool offers
		// to do the thing anyway — "shall I go ahead and create it?" — which
		// hands the decision back at the exact moment the user had already made
		// it by turning the dial. What they want is the plan, good enough to act
		// on, not a request to be let out of the mode they chose.
		return "This turn is planning work: you can look at anything and change nothing. " +
			"Reading, searching, fetching and inspecting are all available; writing, editing, running " +
			"commands and handing work to an agent are not, because the user asked for the plan first.\n\n" +
			"Go and look before you plan. A plan written without opening the files it is about is a guess " +
			"with numbered steps, and the reading tools are here precisely so it does not have to be one.\n\n" +
			"Then give the plan under these four headings, in this order and in these words:\n" +
			planShapeBlock() + "\n" +
			"A small job does not need a long plan — a heading can be a single line, and saying so is the " +
			"correct plan rather than a lazy one. It still gets the shape: the user turned this dial to be " +
			"handed a plan, and a plan they can read the same way every time is the thing they turned it " +
			"for. Keep anything you quote short enough to identify what you mean; this is the plan, not the " +
			"work.\n\n" +
			"Do not ask for permission to proceed and do not offer to do it anyway: the user turned this " +
			"dial deliberately and turning it back is one press. End with the plan, not with a question " +
			"about whether to start."
	}
	if s != StanceConsult {
		return ""
	}
	// The paragraph about what to do instead of checking is the load-bearing
	// one. Told only that it has no tools, a model narrates the tool it does
	// not have — "I would run X to confirm" — which spends the turn describing
	// an errand the user just said they did not want. What they asked for is
	// the answer and how far it can be trusted without the check.
	return "This turn is thinking work rather than doing work: you are carrying no tools at all. " +
		"That is the user's choice about this turn, not a limit on you and not a statement about the request — " +
		"they asked for the conversation rather than the errand.\n\n" +
		"Answer from what you already know and from what is in this conversation. Where you would ordinarily " +
		"look something up, run something or read a file, do not narrate the tool you would have reached for. " +
		"Give the answer you would expect to find, say how confident you are without having checked, and name " +
		"what would settle it.\n\n" +
		"If the thing being asked genuinely cannot be settled by talking — it needs a file read, a command run, " +
		"a page fetched — say that in a sentence and stop there. Switching back is one press and it is the " +
		"user's to make, so offer the finding rather than asking for permission to go and get it."
}
