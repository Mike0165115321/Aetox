package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/subagent"
)

// The meter has to MEASURE. A constant would be right the day it was written
// and quietly wrong the day somebody moved prose out of another tool — which
// happened twice this week — and a number on a settings page that used to be
// true is worse than no number.
func TestTheMeterCountsTheBlockItWouldActuallySend(t *testing.T) {
	a := newSwitchApp(t)
	// Both kinds on: they ship off, and this test is about what turning them off does.
	a.SetDelegateOff("agents", false)
	a.SetDelegateOff("helpers", false)

	before := a.ToolBlockTokens()
	if before <= 0 {
		t.Fatalf("ToolBlockTokens() = %d with a real registry", before)
	}
	task := a.toolTokens("task")
	if task <= 0 {
		t.Fatal("task is not in the block, so the master switch has nothing to weigh")
	}

	a.SetDelegateOff("agents", true)
	after := a.SetDelegateOff("helpers", true)
	if after.Tokens >= before {
		t.Errorf("switching delegation off did not shrink the block: %d then %d", before, after.Tokens)
	}
	if got := before - after.Tokens; got < task/2 {
		t.Errorf("the block shrank by %d, but task alone was %d — the meter is not counting the same thing the switch changes", got, task)
	}
	// With both off the tool is gone, so each switch is now worth what turning
	// IT back on would cost. Not zero, and not the whole tool either: a switch
	// promised the other one's saving is a switch that lies once it is pressed.
	for _, side := range []struct {
		name  string
		reach DelegateReach
	}{{"agents", after.Agents}, {"helpers", after.Helpers}} {
		if !side.reach.Off {
			t.Errorf("%s reads as on after being switched off", side.name)
		}
		if side.reach.Tokens <= 0 {
			t.Errorf("%s is off and the meter says turning it on costs nothing", side.name)
		}
		if side.reach.Tokens >= task {
			t.Errorf("%s alone is billed %d and the whole tool was %d, so one switch is being promised the other's saving too", side.name, side.reach.Tokens, task)
		}
	}
}

// The two switches are two, all the way to the settings page: turning เอเจน off
// must leave every ซับเอเจน exactly where it was.
//
// This is the bug the split was for. One switch governed both, sat on the เอเจน
// page alone, and took `explore` away from somebody who had only ever said
// anything about colleagues (owner, 20 ส.ค.: แยกชัดเจน).
func TestOneKindSwitchesWithoutTheOther(t *testing.T) {
	a := newSwitchApp(t)
	a.SetDelegateOff("agents", false)
	both := a.SetDelegateOff("helpers", false)
	if both.Agents.Off || both.Helpers.Off {
		t.Fatal("turning both on did not take")
	}
	whole := a.toolTokens("task")

	off := a.SetDelegateOff("agents", true)
	if !off.Agents.Off {
		t.Error("the เอเจน switch did not take")
	}
	if off.Helpers.Off {
		t.Error("switching เอเจน off took ซับเอเจน with it, which is the fusion this split removed")
	}
	if len(off.Helpers.Workers) != len(both.Helpers.Workers) {
		t.Errorf("the ซับเอเจน list changed size when the other switch moved: %d then %d", len(both.Helpers.Workers), len(off.Helpers.Workers))
	}
	// And the tool is still there, carrying one roster instead of two.
	if left := a.toolTokens("task"); left <= 0 {
		t.Error("one kind is still on and the delegation tool is gone entirely")
	} else if left >= whole {
		t.Errorf("the tool did not shrink when a whole roster left it: %d then %d", whole, left)
	}
}

// Neither block may hold the other's workers. The kind is decided by which home
// the profile file lives in, and nothing else gets a vote.
func TestEachBlockHoldsOnlyItsOwnKind(t *testing.T) {
	a := newSwitchApp(t)
	switches := a.DelegateSwitches()

	if len(switches.Agents.Workers) == 0 || len(switches.Helpers.Workers) == 0 {
		t.Fatalf("a block is empty, so this proves nothing: %d เอเจน, %d ซับเอเจน", len(switches.Agents.Workers), len(switches.Helpers.Workers))
	}
	for _, w := range switches.Agents.Workers {
		if !w.Agent {
			t.Errorf("%s is a ซับเอเจน sitting in the เอเจน block", w.Name)
		}
	}
	for _, w := range switches.Helpers.Workers {
		if w.Agent {
			t.Errorf("%s is an เอเจน sitting in the ซับเอเจน block", w.Name)
		}
	}
}

// Every worker is listed whether or not the assistant may reach it. A switch
// you cannot see is a switch you cannot turn back on.
func TestSwitchedOffWorkersStayOnTheList(t *testing.T) {
	a := newSwitchApp(t)

	full := allWorkers(a.DelegateSwitches())
	if len(full) == 0 {
		t.Fatal("no workers at all, so this test proves nothing")
	}
	name := full[0].Name

	after := allWorkers(a.SetAgentOff(name, true))
	if len(after) != len(full) {
		t.Errorf("switching %s off removed it from the settings list: %d rows then %d", name, len(full), len(after))
	}
	for _, w := range after {
		if w.Name == name && w.On {
			t.Errorf("%s reads as on after being switched off", name)
		}
	}
	// And back on again, because a one-way switch is a trap.
	back := allWorkers(a.SetAgentOff(name, false))
	for _, w := range back {
		if w.Name == name && !w.On {
			t.Errorf("%s could not be switched back on", name)
		}
	}
}

// Each row carries what the worker is FOR, from the same split the tool block
// uses. Names alone would make the settings page as guessy as a bare enum.
func TestEachWorkerRowSaysWhatItIsFor(t *testing.T) {
	a := newSwitchApp(t)

	for _, w := range allWorkers(a.DelegateSwitches()) {
		if strings.TrimSpace(w.For) == "" {
			t.Errorf("%s is listed with nothing saying what it is for", w.Name)
		}
	}
}

// allWorkers is both blocks end to end, for the assertions that are about a
// worker rather than about a kind.
func allWorkers(s DelegateSettings) []DelegateWorker {
	return append(append([]DelegateWorker{}, s.Agents.Workers...), s.Helpers.Workers...)
}

func newSwitchApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	isolateUserDirs(t)
	a := seed(&App{ctx: nil, emit: func(string, ...any) {}, dbDir: t.TempDir()}, &conversation{id: newSessionID()})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(a.cur(), config.Config{SandboxRoot: t.TempDir(), ModelProvider: "aetox", ModelName: "aetox-tools:test"})
	return a
}

// The two switches ship opposite ways, and a zero Config is the only place that
// can be proved.
//
// A zero Config, not the desktop app's startup state: since 20 ส.ค. a machine
// where nobody has answered resolves to เอเจน ON with research the only one in
// reach (App.resolveConfig, shippedDelegation), which is a different fact and
// has its own test below. What is asserted here is what a Config built anywhere
// else means — a test, another entry point, the CLI.
//
// เอเจน OFF (owner, 18 ส.ค.): handing a whole job to a colleague is a decision,
// and it costs. ซับเอเจน ON (owner, 20 ส.ค.: "ซับเอเจน ควรจะไประบุที่หน้าตั้งค่า
// และเปิดเป็นค่าเริ่มต้น"): those are the assistant's own hands, and an
// assistant that cannot take a step of its own work into a second context is
// missing part of how it works rather than a permission somebody withheld.
//
// Which is why the two are stored spelled differently — `delegate_agents_on`
// and `delegate_helpers_off` — so that in both cases an absent field means what
// the product ships. internal/subagent keeps its own defaults (both reachable),
// because a library that does nothing until you opt in is a library that gets
// called wrong; the flip lives in one place, at the bootstrap boundary.
func TestDelegationShipsOffForAgentsAndOnForHelpers(t *testing.T) {
	a := newSwitchApp(t)

	switches := a.DelegateSwitches()
	if !switches.Agents.Off {
		t.Error("a fresh install hands whole jobs to เอเจน; that switch was supposed to ship off")
	}
	if switches.Helpers.Off {
		t.Error("a fresh install has no hands; ซับเอเจน were supposed to ship on")
	}
	// So the tool IS there on an install nobody has touched, carrying one roster.
	shipped := a.toolTokens("task")
	if shipped <= 0 {
		t.Fatal("ซับเอเจน are on and there is no task tool at all")
	}
	// And the workers are still listed, because a switch you cannot see is a
	// switch you cannot turn on.
	if len(allWorkers(switches)) == 0 {
		t.Error("nothing to turn on: the settings page shows no workers")
	}

	on := a.SetDelegateOff("agents", false)
	if on.Agents.Off {
		t.Fatal("turning เอเจน on did not take")
	}
	if both := a.toolTokens("task"); both <= shipped {
		t.Errorf("the tool did not grow when the second roster arrived: %d then %d", shipped, both)
	}
	if on.Tokens <= switches.Tokens {
		t.Errorf("the block did not grow when the capability was added: %d then %d", switches.Tokens, on.Tokens)
	}
}

// What a machine arrives with before anybody answers the question.
//
// The switch fields still ship the way the test above proves; this is the layer
// over them, and it exists because "may my assistant hand work to a colleague"
// answered NO for everybody meant a company whose employees were never asked to
// do anything. The three shipped in reach are the errands that come up on any
// desk; the two left out cannot start without a token or a server.
func TestNobodyAnsweredMeansTheShippedThreeAndNobodyElse(t *testing.T) {
	agents, off := shippedDelegation()
	if !agents {
		t.Fatal("the assistant cannot hand work to anybody on a fresh machine")
	}
	lower := lowered(off)
	for _, want := range shippedReachableAgents {
		if slices.Contains(lower, want) {
			t.Errorf("%s is meant to be in reach and it is switched off: %v", want, off)
		}
	}
	var agentNames, helperNames []string
	for _, p := range subagent.List() {
		if p.Invalid != "" {
			continue
		}
		if p.Desk != "" {
			agentNames = append(agentNames, strings.ToLower(p.Name))
		} else {
			helperNames = append(helperNames, strings.ToLower(p.Name))
		}
	}
	for _, name := range agentNames {
		if slices.Contains(shippedReachableAgents, name) {
			continue
		}
		if !slices.Contains(lower, name) {
			t.Errorf("%s is in reach on a machine nobody has touched; only %v were meant to be", name, shippedReachableAgents)
		}
	}
	// ซับเอเจน are the assistant's own hands and ship on. A default that reached
	// into their list would take away hands nobody asked to have taken.
	for _, name := range helperNames {
		if slices.Contains(lower, name) {
			t.Errorf("ซับเอเจน %s was switched off by a default that is only about เอเจน", name)
		}
	}
}

// The shipped default has to stop applying the moment somebody answers, even
// when their answer is the same as the default — otherwise the next start reads
// "nobody answered" and hands back a state the user had just left.
func TestAnsweringOnceStopsTheShippedDefault(t *testing.T) {
	a := newSwitchApp(t)
	if a.cur().cfg.DelegateSet {
		t.Fatal("a config nobody has touched already claims to be an answer")
	}
	a.SetAgentOff(shippedReachableAgents[0], true)
	if !a.cur().cfg.DelegateSet {
		t.Error("switching one agent off is an answer and was not recorded as one")
	}

	b := newSwitchApp(t)
	b.SetDelegateOff("agents", false)
	if !b.cur().cfg.DelegateSet {
		t.Error("flipping the master switch is an answer and was not recorded as one")
	}
}
