package proc

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The observable contract on Windows: after KillTreeOnExit reports success,
// the current process really is inside a job (the kill-on-close behavior
// itself can only be observed by dying, which a test can't do in-process).
// x/sys/windows doesn't wrap IsProcessInJob, so call kernel32 directly.
func TestKillTreeOnExitJoinsJob(t *testing.T) {
	if !KillTreeOnExit() {
		t.Skip("job assignment unavailable in this environment")
	}
	isProcessInJob := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	var inJob int32
	r1, _, callErr := isProcessInJob.Call(
		uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)),
	)
	if r1 == 0 {
		t.Fatalf("IsProcessInJob: %v", callErr)
	}
	if inJob == 0 {
		t.Fatal("KillTreeOnExit returned true but process is not in a job")
	}
}
