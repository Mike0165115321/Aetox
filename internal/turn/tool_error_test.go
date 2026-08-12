package turn

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

// The classifier asks the error what it is. Reading the text would work today
// and rot the first time a shell reports a status some other way — and the
// whole reason this field exists is that the text is not evidence.
func TestClassifyToolErrorSeparatesProgramFailuresFromOurOwn(t *testing.T) {
	// A real nonzero exit, produced rather than constructed: *exec.ExitError is
	// only meaningful when a process actually ran.
	name, args := "cmd", []string{"/c", "exit 3"}
	if runtime.GOOS != "windows" {
		name, args = "sh", []string{"-c", "exit 3"}
	}
	exitErr := exec.Command(name, args...).Run()
	if exitErr == nil {
		t.Fatalf("the probe command succeeded; it was supposed to exit 3")
	}
	if got := classifyToolError(exitErr); got != ErrorFromProgram {
		t.Errorf("a program's exit status classified as %q, want %q — it would be learned as a lesson about the tool", got, ErrorFromProgram)
	}
	// Wrapped on the way up is the normal case: the skill layer adds context.
	if got := classifyToolError(fmt.Errorf("shell: %w", exitErr)); got != ErrorFromProgram {
		t.Errorf("a wrapped exit status classified as %q, want %q", got, ErrorFromProgram)
	}

	// Anything this codebase wrote stays unmarked, which is what keeps it
	// readable as a lesson. Unmarked is the conservative default: a failure of
	// unknown origin is still offered to the user as a possible lesson, and the
	// user is the one who says no.
	ours := errors.New("this command builds a path while it runs, so it cannot be checked — write the path out literally")
	if got := classifyToolError(ours); got != "" {
		t.Errorf("a refusal we wrote classified as %q, want unmarked", got)
	}
	if got := classifyToolError(nil); got != "" {
		t.Errorf("no error classified as %q, want unmarked", got)
	}
}

// A tool that refuses without returning a Go error must still be recorded with
// its reason. The report: ten `task` dispatches refused with a precise sentence
// each, all ten stored as the word "ไม่สำเร็จ", and the summarizer then offered
// a memory line teaching the agent to avoid a pattern that word does not name.
func TestAToolThatRefusesIsRecordedWithItsReasonNotThePlaceholder(t *testing.T) {
	reason := "the MCP server(s) sequential-thinking that doc works with have not finished connecting yet"

	// How subagent.task refuses: the sentence in both fields, no Go error, so
	// the model can read it and try something else.
	if got := failureReason(skill.Output{Content: reason, Stderr: reason, Success: false}); got != reason {
		t.Errorf("reason = %q, want the refusal itself", got)
	}
	// Stderr is the field that means "why this went wrong", and wins.
	if got := failureReason(skill.Output{Content: "some output", Stderr: reason}); got != reason {
		t.Errorf("reason = %q, want stderr to win over content", got)
	}
	// A tool that only filled Content still says something.
	if got := failureReason(skill.Output{Content: reason}); got != reason {
		t.Errorf("reason = %q, want the content", got)
	}
	// A label, not a transcript: one line.
	if got := failureReason(skill.Output{Stderr: reason + "\nstack frame\nanother frame"}); got != reason {
		t.Errorf("reason = %q, want only the first line", got)
	}
	// And bounded, because an error is not required to be short.
	long := failureReason(skill.Output{Stderr: strings.Repeat("ก", failureReasonMax*2)})
	if len([]rune(long)) > failureReasonMax+1 {
		t.Errorf("reason is %d runes, want it capped near %d", len([]rune(long)), failureReasonMax)
	}
	// The placeholder survives for what it was always supposed to mean: a tool
	// that failed and said nothing at all.
	if got := failureReason(skill.Output{}); got != "ไม่สำเร็จ" {
		t.Errorf("reason = %q, want the placeholder when there is genuinely nothing to report", got)
	}
}
