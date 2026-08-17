package subagent

// What delegation used to say on every message, and now says once per action.
//
// `task` was the largest entry in the whole tool block at 1,568 tokens — twice
// the browser, for one mechanism the model calls with one verb at a time. The
// surprise on opening it was where the weight sat: 249 tokens of description
// against 1,299 of SCHEMA, and 673 of that in the `agent` parameter alone,
// which is 43% of the tool.
//
// That parameter is the roster, and it is the one thing here that could not
// simply be moved. Owner, 18 ส.ค.: *"ทำให้มันฉลาดเลือก ไม่ใช่เดาจากชื่อ"* — the
// model has to be able to CHOOSE a worker, not just name one, and a choice made
// after the first delegation is a choice made too late. So the block keeps every
// worker with the clause that says what it is for, and what moved here is how to
// brief one, which is read at the moment of writing a brief.
//
// Two other things stayed in the block for the same reason, and it is worth
// naming the pattern: WHEN TO USE / WHEN NOT TO, because whether to delegate at
// all is decided before any call exists, so guidance attached to the first
// `start` reaches only a model that already decided to start one.
//
// See internal/skill/guidance.go for the standard.

import "strings"

func (d *delegationTool) Guidance(args map[string]any) string {
	action, _ := args["action"].(string)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "start":
		return startGuidance + "\n\n" + agentRoster(d.start.available())
	case "collect":
		return collectGuidance
	case "answer":
		return answerGuidance
	case "plan":
		return planGuidance
	}
	return ""
}

// The verbatim rule has a real failure behind it, and the failure is the reason
// it is written as a rule about WORDS rather than as advice about care: "ask doc
// how a good document is put together" left here as "write a manual about…" and
// came back as a manual. doc's own instruction against answering a question with
// a file could not save it, because the brief that arrived had no question left
// in it.
const startGuidance = "REPEATED WORK IS ONE JOB: hand the whole list to ONE worker and let it loop. " +
	"Twelve items is one task with twelve items in its prompt, never twelve tasks — each one pays for its own context.\n" +
	"The brief is everything the worker gets; it cannot see this conversation, so \"the file we discussed\" names nothing. " +
	"When the USER asked for this worker themselves, their own words ARE the brief: carry them through as written and add context around them rather than summarising. " +
	"A word you are unsure of crosses over exactly as typed — the worker can ask about it, and a guess cannot be un-asked.\n" +
	"Starting returns immediately with an id. Do other useful work, then collect."

const collectGuidance = "Several ids at once cost the time of the slowest, not the sum, so collect a wave together rather than one at a time.\n" +
	"One that got stuck comes back as a QUESTION rather than an answer. That is not a failure — it is a worker that needs a decision only you can make, and `answer` resumes it.\n" +
	"Collect everything you start. A result nobody reads is work nobody used."

const answerGuidance = "It resumes with everything it had already done still in hand, so answering costs far less than starting the job again.\n" +
	"Name the choice. The worker cannot see this conversation, so \"the first one\" means nothing to it."

const planGuidance = "Declare the stages BEFORE starting any of them, including the ones that have not happened yet, then name a phase on every start.\n" +
	"A phase declared and left empty sits at zero on the user's screen for the whole run, which is what makes a checking round you promised hard to skip quietly."
