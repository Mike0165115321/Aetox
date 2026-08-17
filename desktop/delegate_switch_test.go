package main

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// The meter has to MEASURE. A constant would be right the day it was written
// and quietly wrong the day somebody moved prose out of another tool — which
// happened twice this week — and a number on a settings page that used to be
// true is worse than no number.
func TestTheMeterCountsTheBlockItWouldActuallySend(t *testing.T) {
	a := newSwitchApp(t)

	before := a.ToolBlockTokens()
	if before <= 0 {
		t.Fatalf("ToolBlockTokens() = %d with a real registry", before)
	}
	task := a.toolTokens("task")
	if task <= 0 {
		t.Fatal("task is not in the block, so the master switch has nothing to weigh")
	}

	after := a.SetDelegateOff(true)
	if after.Tokens >= before {
		t.Errorf("switching delegation off did not shrink the block: %d then %d", before, after.Tokens)
	}
	if got := before - after.Tokens; got < task/2 {
		t.Errorf("the block shrank by %d, but task alone was %d — the meter is not counting the same thing the switch changes", got, task)
	}
	// Nothing left to weigh once it is off, and claiming otherwise would be the
	// meter promising a saving already taken.
	if after.TaskTokens != 0 {
		t.Errorf("TaskTokens = %d while delegation is off", after.TaskTokens)
	}
}

// Every worker is listed whether or not the assistant may reach it. A switch
// you cannot see is a switch you cannot turn back on.
func TestSwitchedOffWorkersStayOnTheList(t *testing.T) {
	a := newSwitchApp(t)

	full := a.DelegateSwitches()
	if len(full.Workers) == 0 {
		t.Fatal("no workers at all, so this test proves nothing")
	}
	name := full.Workers[0].Name

	after := a.SetAgentOff(name, true)
	if len(after.Workers) != len(full.Workers) {
		t.Errorf("switching %s off removed it from the settings list: %d rows then %d", name, len(full.Workers), len(after.Workers))
	}
	for _, w := range after.Workers {
		if w.Name == name && w.On {
			t.Errorf("%s reads as on after being switched off", name)
		}
	}
	// And back on again, because a one-way switch is a trap.
	back := a.SetAgentOff(name, false)
	for _, w := range back.Workers {
		if w.Name == name && !w.On {
			t.Errorf("%s could not be switched back on", name)
		}
	}
}

// Each row carries what the worker is FOR, from the same split the tool block
// uses. Names alone would make the settings page as guessy as a bare enum.
func TestEachWorkerRowSaysWhatItIsFor(t *testing.T) {
	a := newSwitchApp(t)

	for _, w := range a.DelegateSwitches().Workers {
		if strings.TrimSpace(w.For) == "" {
			t.Errorf("%s is listed with nothing saying what it is for", w.Name)
		}
	}
}

func newSwitchApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	isolateUserDirs(t)
	a := &App{ctx: nil, emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{SandboxRoot: t.TempDir(), ModelProvider: "aetox", ModelName: "aetox-tools:test"})
	return a
}
