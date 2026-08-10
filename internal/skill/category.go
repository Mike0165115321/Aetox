package skill

import "strings"

// What each tool is *for*, as a person would group them.
//
// Every surface that lists tools listed them alphabetically with a badge saying
// where they came from — builtin, workbench, mcp — which answers a question
// nobody asks. Forty-four rows in one column is not a list, it is a pile: it
// cannot be read, so it cannot be reasoned about, and "which of these does the
// assistant actually need to carry?" had nowhere to be asked from.
//
// The grouping is by capability rather than by implementation on purpose. Which
// package a tool lives in is our business; what it lets the assistant do is the
// user's. `shell` and `git` sit together because both run commands, even though
// one is a builtin and the other wraps a binary; `browser_*` sits with `web_*`
// because from outside they are one ability.
//
// A table rather than a method on Skill: adding an interface method would make
// every one of the forty-four implement it, and a tool with a category of ""
// would be a silent gap. A table can go stale instead — which is why
// TestEveryToolHasACategory exists, in the package that can see the whole
// registry at once. Adding a tool without deciding what it is for fails there.
const (
	CategoryFiles        = "files"
	CategoryShell        = "shell"
	CategoryMedia        = "media"
	CategoryDeliverables = "deliverables"
	CategoryWeb          = "web"
	CategoryCode         = "code"
	CategoryAgent        = "agent"
)

// CategoryOrder is the order the groups are shown in: what the assistant does
// most often first, what it does rarely last. The list a person reads top to
// bottom should start with the answer they are most likely looking for.
var CategoryOrder = []string{
	CategoryFiles,
	CategoryShell,
	CategoryDeliverables,
	CategoryMedia,
	CategoryWeb,
	CategoryCode,
	CategoryAgent,
}

var toolCategories = map[string]string{
	// Reading and changing what is on disk — the hands.
	"read":          CategoryFiles,
	"write":         CategoryFiles,
	"edit":          CategoryFiles,
	"apply_patch":   CategoryFiles,
	"delete":        CategoryFiles,
	"list":          CategoryFiles,
	"glob":          CategoryFiles,
	"grep":          CategoryFiles,
	"fs":            CategoryFiles,
	"notebook_edit": CategoryFiles,

	// Running things. `git` belongs here rather than under code: what it is, to
	// the person deciding, is a command that touches their repository.
	//
	// `shell` is the tool the model is offered; the other two are the action
	// names it gates on inside (packed.go). Both kinds are listed for the reason
	// the browser's are — a desk or a profile narrows with the action names, and
	// an unlisted one would fall to CategoryAgent, which every desk carries and
	// which would quietly widen the narrower.
	"shell":        CategoryShell,
	"shell_output": CategoryShell,
	"shell_kill":   CategoryShell,
	"shell_list":   CategoryShell,
	"git":          CategoryShell,

	// Handing work back as a file somebody else's program opens.
	"doc_write":    CategoryDeliverables,
	"sheet_write":  CategoryDeliverables,
	"slides_write": CategoryDeliverables,

	// Senses a model does not have on its own — the group Aetox exists for.
	"image_ocr":        CategoryMedia,
	"video_ocr":        CategoryMedia,
	"pdf_read":         CategoryMedia,
	"audio_transcribe": CategoryMedia,

	// The automation engine the user connected (§92.3). Filed under web because
	// that is what it is from outside — an HTTP service somewhere on the network
	// that Aetox reaches — and NOT under code, where the github_* tools sit: an
	// automation is something a person builds to run their month-end, not a
	// developer tool, and putting it on the coding desk alone would hide it from
	// the desk the automation room belongs to.
	//
	// A second gate sits in front of these regardless: connect.Allows will not
	// carry them onto a desk that does not hold the n8n connection, and the
	// model is never shown a door it cannot open.
	"n8n_workflow_list":     CategoryWeb,
	"n8n_workflow_read":     CategoryWeb,
	"n8n_workflow_create":   CategoryWeb,
	"n8n_workflow_update":   CategoryWeb,
	"n8n_workflow_activate": CategoryWeb,
	"n8n_server_start":      CategoryWeb,

	"windmill_workspace_list": CategoryWeb,
	"windmill_flow_list":      CategoryWeb,
	"windmill_flow_read":      CategoryWeb,
	"windmill_flow_create":    CategoryWeb,
	"windmill_flow_update":    CategoryWeb,
	"windmill_server_start":   CategoryWeb,

	"web_search":    CategoryWeb,
	"web_fetch":     CategoryWeb,
	// The tool the model is offered, and the four action names it gates on
	// inside (desktop/browser_tool.go). The actions are listed too because a
	// desk or a profile narrows with them, and an unlisted name would fall to
	// CategoryAgent — which every desk carries, quietly widening the narrower.
	"browser":       CategoryWeb,
	"browser_open":  CategoryWeb,
	"browser_read":  CategoryWeb,
	"browser_click": CategoryWeb,
	"browser_type":  CategoryWeb,

	// Putting things in front of the user on their own desk, and seeing what is
	// there. Filed under agent rather than files: these do not read or change a
	// file, they change what the person is looking at — which is the same kind
	// of act as asking a question or delegating, and it belongs on every desk.
	//
	// Categorised wrong once, as shell, which took them off the specialized desk
	// (no shell there, by design) — the one desk whose whole job is producing a
	// document to hand back. A tool for showing the user what you made, missing
	// from the desk that makes things.
	"desk_open": CategoryAgent,
	"desk_list": CategoryAgent,
	// This one really is shell: it starts a shell and types into it. The
	// specialized desk carries no shell and must not carry this either.
	"desk_terminal": CategoryShell,

	"diagnostics": CategoryCode,
	"symbol":      CategoryCode,
	// The tool the model is offered, and the four action names it gates on
	// inside (github_pack.go) — listed for the same reason the shell and
	// browser action names are.
	"github":              CategoryCode,
	"github_search":       CategoryCode,
	"github_read_file":    CategoryCode,
	"github_list_files":   CategoryCode,
	"github_repo_summary": CategoryCode,

	// How the assistant runs itself: delegating, asking, remembering, and
	// reaching the documents and history it is not carrying.
	"task":           CategoryAgent,
	"task_result":    CategoryAgent,
	"task_answer":    CategoryAgent,
	"ask_main":       CategoryAgent,
	"ask_user":       CategoryAgent,
	"todo_write":     CategoryAgent,
	"suggest_task":   CategoryAgent,
	"memory":         CategoryAgent,
	"session_search": CategoryAgent,
	"skills_list":    CategoryAgent,
	"skill_view":     CategoryAgent,
	"plugin_install": CategoryAgent,
	"help":           CategoryAgent,
	"time":           CategoryAgent,
	"echo":           CategoryAgent,
	// Deliberately not `code`, though it runs a script. Categories are how a
	// desk decides what it carries, and `code` is the developer group that two
	// of the three desks refuse on purpose — which would have left the desk
	// that answers "แล้วรวมเป็นเท่าไหร่" as the one desk that cannot count.
	// Running a script is how this tool works; working out a number is what it
	// is for, and that belongs beside time and memory.
	"calc": CategoryAgent,
}

// CategoryOf reports what a tool is for. An unknown name — an MCP server's
// tool, which is named by whoever wrote that server — answers CategoryAgent
// rather than "": every tool has to land in some group, or it falls off the
// page that is supposed to show all of them.
func CategoryOf(name string) string {
	if c, ok := toolCategories[strings.ToLower(strings.TrimSpace(name))]; ok {
		return c
	}
	return CategoryAgent
}

// HasCategory reports whether a tool was actually placed, as opposed to falling
// back. Only the completeness test uses it — CategoryOf is what callers want.
func HasCategory(name string) bool {
	_, ok := toolCategories[strings.ToLower(strings.TrimSpace(name))]
	return ok
}
