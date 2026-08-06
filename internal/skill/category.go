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
	"shell":        CategoryShell,
	"shell_output": CategoryShell,
	"shell_kill":   CategoryShell,
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

	"web_search":    CategoryWeb,
	"web_fetch":     CategoryWeb,
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

	"diagnostics":         CategoryCode,
	"symbol":              CategoryCode,
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
