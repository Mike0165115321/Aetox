package subagent

import (
	"strings"
	"testing"
)

// The master switch. Off means the tool is not built at all — not built and
// refusing, which would still cost its 730 tokens in every request to say no.
func TestDelegationOffBuildsNoToolAtAll(t *testing.T) {
	isolate(t)

	if tools := NewTaskTools(TaskOptions{}); len(tools) == 0 {
		t.Fatal("delegation is missing with the switch untouched — the default has to be on")
	}
	if tools := NewTaskTools(TaskOptions{NoAgents: true, NoHelpers: true}); len(tools) != 0 {
		t.Errorf("switching delegation off still built %d tool(s); the saving is the tool not existing", len(tools))
	}
}

// The per-agent switch narrows who the ASSISTANT can hand work to. It does not
// disable anybody: the user still opens a chat with that worker and still writes
// @name at it, which is the user's own act and none of this tool's business.
func TestAnAgentSwitchedOffLeavesTheRosterButNotTheWorld(t *testing.T) {
	isolate(t)

	full := namedTool(t, NewTaskTools(TaskOptions{}), "task")
	narrowed := namedTool(t, NewTaskTools(TaskOptions{WorkersOff: []string{"explore"}}), "task")

	fullSchema := string(toolDefOf(t, NewTaskTools(TaskOptions{}), "task").Function.Parameters)
	if !strings.Contains(fullSchema, "explore") {
		t.Fatal("explore is not in the roster to begin with, so this test proves nothing")
	}

	schema := string(toolDefOf(t, NewTaskTools(TaskOptions{WorkersOff: []string{"explore"}}), "task").Function.Parameters)
	if strings.Contains(schema, "explore") {
		t.Errorf("explore is switched off but still offered to the assistant: %s", schema)
	}
	// Everyone else is untouched.
	if !strings.Contains(schema, "general") {
		t.Errorf("switching one worker off took another with it: %s", schema)
	}
	// The profile still exists. Nothing here disables a worker; it narrows a
	// reach, and the user's own doors are not this tool's to close.
	if _, ok := Load("explore"); !ok {
		t.Error("the worker itself was disabled, which no switch here may do")
	}
	if full == nil || narrowed == nil {
		t.Error("the tool disappeared over a per-agent switch; only the master switch may do that")
	}
}

// Case and stray spaces come from a settings file a human may have edited.
func TestTheAgentSwitchIsNotCaseSensitive(t *testing.T) {
	isolate(t)

	schema := string(toolDefOf(t, NewTaskTools(TaskOptions{WorkersOff: []string{"  ExPloRe "}}), "task").Function.Parameters)
	if strings.Contains(schema, "explore") {
		t.Errorf("a name switched off with different case was ignored: %s", schema)
	}
}

// The hole this closes, and the reason it was worth stopping for: hiding a
// worker from the roster is not the same as refusing it.
//
// available() filtered the list while begin() resolved the name with Load() and
// ran it. So the desk's ceiling was a real gate and the user's switch was a
// suggestion — two entries in one list, looking identical, one of them
// enforceable. Somebody would eventually have relied on the wrong one.
func TestASwitchedOffWorkerIsRefusedAndNotMerelyUnlisted(t *testing.T) {
	isolate(t)
	tool := taskToolOf(t, TaskOptions{WorkersOff: []string{"explore"}})

	out, _ := tool.begin(t.Context(), map[string]any{
		"agent":  "explore",
		"prompt": "a brief long enough to be a real one",
	}, nil)

	if out.Success {
		t.Fatal("a worker the user switched off still ran when named directly")
	}
	if !strings.Contains(out.Content, "switched off") {
		t.Errorf("the refusal does not say why: %q", out.Content)
	}
	// And it says the worker is still reachable the other ways, because it is.
	if !strings.Contains(out.Content, "@explore") {
		t.Errorf("the refusal reads like the worker is gone: %q", out.Content)
	}
}

// A config file can switch every worker off while leaving delegation on. The
// settings page will not let you, but a hand-edited file will, and a `task` with
// an empty roster is a dead end rather than a tool.
func TestEveryWorkerOffIsRefusedWithSomethingToDo(t *testing.T) {
	isolate(t)
	var all []string
	for _, p := range List() {
		all = append(all, p.Name)
	}
	tool := taskToolOf(t, TaskOptions{WorkersOff: all})

	out, _ := tool.begin(t.Context(), map[string]any{
		"agent":  "explore",
		"prompt": "a brief long enough to be a real one",
	}, nil)

	if out.Success {
		t.Fatal("a delegation ran with nobody available")
	}
	if !strings.Contains(out.Content, "settings") {
		t.Errorf("the refusal does not say what to do about it: %q", out.Content)
	}
}

func taskToolOf(t *testing.T, opts TaskOptions) *taskTool {
	t.Helper()
	tools := NewTaskTools(opts)
	if len(tools) == 0 {
		t.Fatal("no task tool was built")
	}
	d, ok := tools[0].(*delegationTool)
	if !ok {
		t.Fatalf("task is a %T, not the packed tool", tools[0])
	}
	return d.start
}
