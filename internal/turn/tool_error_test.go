package turn

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"testing"
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
