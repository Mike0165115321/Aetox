package proc

import (
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillOnCancel makes context cancellation (Stop button, tool timeout) kill the
// whole child tree instead of only the process we spawned. exec.CommandContext
// kills the direct child alone, so cancelling `npm install` would leave npm,
// node and their children running as orphans for the rest of the session —
// §24's Job Object only reaps them when the app itself exits, which is far too
// late for a user who pressed Stop.
//
// WaitDelay matters just as much: Cmd.Run copies output through a pipe that a
// surviving grandchild still holds open, so without it Run blocks past the
// kill. Every long-running exec.Cmd should pass through here.
func KillOnCancel(cmd *exec.Cmd) {
	cmd.WaitDelay = 3 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		killTree(uint32(cmd.Process.Pid))
		return nil
	}
}

// killTree kills the process and every descendant, twice.
//
// This used to be one `taskkill /T /F`, and taskkill has a hole this exists to
// close: it reads the process tree once, then kills. A child born between that
// snapshot and its parent's death is invisible to it and survives as an orphan
// — for a shell that spawns its command near-instantly (cmd.exe) the window
// was academic, but PowerShell spends whole seconds starting up before it
// spawns anything, and a Stop pressed in that gap reliably orphaned the very
// command being stopped (found 2026-08-15 as a test's temp dir held open for
// 60 seconds by an orphaned ping).
//
// The second pass is what taskkill cannot do: an orphan's ParentProcessID
// still names its dead parent in the next snapshot, so walking the tree again
// from the same root finds exactly the children the first pass raced with.
// Root-first kill order in each pass keeps the window one-sided — a dead root
// spawns nothing more — so one repeat is enough.
func killTree(root uint32) {
	terminateTree(root)
	time.Sleep(150 * time.Millisecond)
	terminateTree(root)
}

func terminateTree(root uint32) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)

	children := make(map[uint32][]uint32)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		children[entry.ParentProcessID] = append(children[entry.ParentProcessID], entry.ProcessID)
	}

	// Breadth-first from the root, so the root is also the first kill. The
	// seen map is not decoration: PID reuse can wire a stale ParentProcessID
	// into a cycle, and an unguarded walk would append forever.
	pids := []uint32{root}
	seen := map[uint32]bool{root: true}
	for i := 0; i < len(pids); i++ {
		for _, child := range children[pids[i]] {
			if !seen[child] {
				seen[child] = true
				pids = append(pids, child)
			}
		}
	}
	for _, pid := range pids {
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			// Already gone, or not ours to kill; either way there is nothing
			// more this pass can do about it.
			continue
		}
		_ = windows.TerminateProcess(handle, 1)
		_ = windows.CloseHandle(handle)
	}
}
