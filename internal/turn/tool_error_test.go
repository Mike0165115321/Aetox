package turn

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"runtime"
	"time"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/statereport"
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

// A state report is the third origin an error can have, and like the exit
// status it is invisible in the text: "n8n ปฏิเสธ API key" reads exactly like a
// refusal, but it describes a machine at a moment, not behaviour to correct.
// Only the author knows, so the author marks it.
func TestAuthoredStateReportsClassifyAsWorldState(t *testing.T) {
	down := statereport.New("ติดต่อ n8n ไม่ได้ — ตรวจว่าเซิร์ฟเวอร์เปิดอยู่")
	if got := classifyToolError(down); got != ErrorFromWorld {
		t.Errorf("a state report classified as %q, want %q — it would become a permanent lesson about tonight's outage", got, ErrorFromWorld)
	}
	// Wrapped on the way up, as the skill layer does.
	if got := classifyToolError(fmt.Errorf("n8n list: %w", down)); got != ErrorFromWorld {
		t.Errorf("a wrapped state report classified as %q, want %q", got, ErrorFromWorld)
	}
	// The explicit mark outranks inference: an exit status the author knows to
	// be a state report reads as one.
	if got := classifyToolError(statereport.Mark(errors.New("exit status 7"))); got != ErrorFromWorld {
		t.Errorf("a marked error classified as %q, want the author's word to win", got)
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

// The defect: a sub-agent asked to produce a file it had no tool to write, then
// asked to read it back. Three GetFileAttributesEx failures reached the problems
// page as if the agent had broken a rule with a remedy to quote (2026-08-18).
// A missing file carries no remedy, and it is not the agent's behaviour.
func TestFilesystemErrorIsAWorldReport(t *testing.T) {
	missing := &fs.PathError{Op: "GetFileAttributesEx", Path: `C:\Users\ASUS\aetox\sheet_result.txt`, Err: fs.ErrNotExist}
	if got := classifyToolError(missing); got != ErrorFromWorld {
		t.Errorf("a missing file classified as %q, want %q", got, ErrorFromWorld)
	}
	if got := classifyToolError(fmt.Errorf("read: %w", missing)); got != ErrorFromWorld {
		t.Errorf("a wrapped missing file classified as %q, want %q", got, ErrorFromWorld)
	}
}

// The other half of the same rule, and the one that keeps it honest: the sandbox
// denylist and every other refusal this codebase enforces are built with
// errors.New/fmt.Errorf, so they stay unmarked and remain offerable as lessons.
// If this ever goes red, the filesystem case above has started swallowing them.
func TestSandboxRefusalIsNotAWorldReport(t *testing.T) {
	refusal := errors.New("path is inside a credential store (.ssh) and stays off-limits in every mode")
	if got := classifyToolError(refusal); got != "" {
		t.Errorf("a refusal classified as %q, want unmarked so it can still be learned from", got)
	}
}

// A soft failure carries no error at all — the tool returns Success:false and a
// nil error so the model reads a result rather than a crash. That is exactly the
// shape the classifier could not see, and every "not ready yet" wait took it.
func TestSoftFailureCanDeclareItselfAWorldReport(t *testing.T) {
	waiting := skill.Output{Success: false, FromWorld: true, Content: "the MCP server github ... have not finished connecting yet"}
	if got := classifyToolOutcome(waiting, nil); got != ErrorFromWorld {
		t.Errorf("a declared world report classified as %q, want %q", got, ErrorFromWorld)
	}

	// Unmarked soft failures stay unmarked: the default has to remain the
	// conservative one, or marking becomes a way to hide real refusals.
	plain := skill.Output{Success: false, Content: "sub-agent \"ghost\" does not exist"}
	if got := classifyToolOutcome(plain, nil); got != "" {
		t.Errorf("an unmarked soft failure classified as %q, want unmarked", got)
	}

	// The flag is meaningless on success and must not be read there.
	won := skill.Output{Success: true, FromWorld: true}
	if got := classifyToolOutcome(won, nil); got != "" {
		t.Errorf("a successful call classified as %q, want unmarked", got)
	}
}

// A tool that died because the turn was stopped failed at nothing: the user
// pressed the brake, and every call in flight at that moment reports the same
// sentence at the same second. One Stop over three parallel web_fetch calls
// became "ล้มซ้ำ 3 ครั้ง" on the problems page (25 ส.ค.) — a card asking the
// user to report their own Stop to the developer.
func TestAStoppedTurnClassifiesAsCancelNotAsALesson(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The honest form: a tool that wraps ctx.Err().
	if got := classifyToolError(fmt.Errorf("fetch %s: %w", "https://x", ctx.Err())); got != ErrorFromCancel {
		t.Errorf("a wrapped cancellation classified as %q, want %q", got, ErrorFromCancel)
	}
	// The flattened form: a tool that stringified it somewhere on the way up —
	// which is what the real web_fetch rows carried.
	if got := classifyToolError(errors.New("context canceled")); got != ErrorFromCancel {
		t.Errorf("a flattened cancellation classified as %q, want %q", got, ErrorFromCancel)
	}
	// The Stop outranks even an author's own mark: whatever the tool was doing,
	// the stop is why it ended.
	if got := classifyToolError(fmt.Errorf("%w: %s", ctx.Err(), statereport.New("x").Error())); got != ErrorFromCancel {
		t.Errorf("a canceled state-probe classified as %q, want %q", got, ErrorFromCancel)
	}
	// A deadline is not a Stop. A tool that keeps timing out is a real
	// pattern worth clustering, so it stays unmarked.
	dctx, dcancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer dcancel()
	<-dctx.Done()
	if got := classifyToolError(fmt.Errorf("fetch: %w", dctx.Err())); got != "" {
		t.Errorf("a timeout classified as %q, want unmarked — repeated timeouts are worth hearing about", got)
	}
}
