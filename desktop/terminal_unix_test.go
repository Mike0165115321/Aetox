//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The check behind unixPTY.Close's session sweep. It cannot be a unit test of
// pure logic: the whole claim is about what the kernel does with process groups
// and sessions, and only a real backgrounded grandchild can show it.
//
// Measured before the sweep existed (bash and dash, job control on and off): a
// backgrounded job lands in its own process group but keeps the shell's
// session, so kill(-shellPid) misses it every time and `npm run dev &` outlives
// the tab. If this test starts failing, that regression is back.
func TestCloseKillsBackgroundedGrandchild(t *testing.T) {
	a := &App{}
	shells := a.TerminalShells()
	if len(shells) == 0 {
		t.Skip("no shell found on this machine")
	}
	dir := t.TempDir() // only holds the pid file; the shell gets no cwd of its own
	p, err := startPTY(shells[0].Path, 80, 24, "")
	if err != nil {
		t.Skipf("startPTY failed (no pty support in this environment?): %v", err)
	}

	// Drain the master, or the shell blocks once its output fills the buffer.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := p.Read(buf); err != nil {
				return
			}
		}
	}()

	// $! is the background job's own pid, and exec makes the subshell become
	// sleep — so the file holds the pid of a process the shell deliberately
	// detached, which is exactly the case a process-group kill misses.
	pidFile := filepath.Join(dir, "grandchild.pid")
	if _, err := p.Write([]byte("(exec sleep 120) & echo $! > " + pidFile + "\n")); err != nil {
		t.Fatalf("writing to the pty: %v", err)
	}

	pid := 0
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(pidFile); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && n > 0 {
				pid = n
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid == 0 {
		_ = p.Close()
		t.Fatal("the shell never reported a background pid — the test never reached what it checks")
	}
	// Whatever happens below, do not leave a stray sleep behind.
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("grandchild %d was not alive before Close: %v", pid, err)
	}

	if err := p.Close(); err != nil {
		t.Errorf("Close: unexpected error: %v", err)
	}

	// Signal 0 only probes for existence; ESRCH is the process being gone.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if err := syscall.Kill(pid, 0); err != nil {
			return // gone, which is the whole point
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("backgrounded grandchild %d outlived Close — the session sweep is not reaching it", pid)
}
