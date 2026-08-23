package proc

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The bug this guards against (2026-08-23): the update waiter joined the
// KILL_ON_JOB_CLOSE job like every other child, so the OS shot it at the exact
// moment the app exited — the moment its Wait-Process was waiting for. The
// contract under test spans two pieces that are useless alone: the job must
// open the door (JOB_OBJECT_LIMIT_BREAKAWAY_OK in KillTreeOnExit) and the
// spawn site must walk through it (Breakaway). Only a process that actually
// dies can prove both, so the KillTreeOnExit half runs in a helper copy of
// this test binary, and the assertion is made over its corpse: the breakaway
// grandchild must still be alive after the helper — and its job — are gone.
func TestBreakawayChildSurvivesParentExit(t *testing.T) {
	helper := exec.Command(os.Args[0], "-test.run", "TestBreakawayHelperProcess", "-test.v")
	helper.Env = append(os.Environ(), "PROC_BREAKAWAY_HELPER=1")
	out, runErr := helper.CombinedOutput()

	var childPid uint32
	for sc := bufio.NewScanner(bytes.NewReader(out)); sc.Scan(); {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "HELPER-SKIP:"):
			t.Skip(line)
		case strings.HasPrefix(line, "HELPER-FAIL:"):
			t.Fatal(line)
		case strings.HasPrefix(line, "HELPER-CHILD "):
			pid, err := strconv.ParseUint(strings.TrimPrefix(line, "HELPER-CHILD "), 10, 32)
			if err != nil {
				t.Fatalf("unparseable helper line %q: %v", line, err)
			}
			childPid = uint32(pid)
		}
	}
	if childPid == 0 {
		t.Fatalf("helper reported no child (err: %v):\n%s", runErr, out)
	}
	defer KillTree(int(childPid))

	// The helper is gone and its job handle closed with it. A child still
	// inside would already be dead — the kill is part of that close, not an
	// eventual sweep — so the pause only gives the OS slack, not the bug cover.
	time.Sleep(600 * time.Millisecond)
	if !processAlive(childPid) {
		t.Fatal("breakaway child died with its parent — the update waiter would never run")
	}
}

// TestBreakawayHelperProcess is not a test: it is the parent half of the
// scenario above, run as a separate process. It puts itself in the kill-on-exit
// job, spawns one breakaway child, names it on stdout, and dies.
func TestBreakawayHelperProcess(t *testing.T) {
	if os.Getenv("PROC_BREAKAWAY_HELPER") != "1" {
		t.Skip("helper half of TestBreakawayChildSurvivesParentExit")
	}
	// Probe BEFORE joining our own job: if breakaway already fails here, some
	// outer job (a CI runner's, say) forbids it — that is the environment's
	// verdict, not ours to assert against.
	probe := exec.Command("cmd.exe", "/c", "exit")
	HideConsole(probe)
	Breakaway(probe)
	if err := probe.Start(); err != nil {
		fmt.Printf("HELPER-SKIP: environment forbids breakaway: %v\n", err)
		return
	}
	_ = probe.Wait()

	if !KillTreeOnExit() {
		fmt.Println("HELPER-SKIP: job assignment unavailable in this environment")
		return
	}
	// Long-lived enough to comfortably outlast the parent's assertion window;
	// the parent kills it the moment the assertion is made.
	child := exec.Command("cmd.exe", "/c", "ping -n 30 127.0.0.1 >nul")
	HideConsole(child)
	Breakaway(child)
	if err := child.Start(); err != nil {
		// The probe passed, so this failure is ours: the job stopped
		// permitting what Breakaway asks for.
		fmt.Printf("HELPER-FAIL: breakaway spawn refused by our own job: %v\n", err)
		return
	}
	fmt.Printf("HELPER-CHILD %d\n", child.Process.Pid)
	// Return, don't linger: the process exiting is the event under test.
}

func processAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259 // STATUS_PENDING; x/sys/windows doesn't export it
	return code == stillActive
}
