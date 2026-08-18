package main

// Where a window opens (COMPANY.md §2).
//
// The engine used to boot at no desk at all. Since a session's desk is stamped
// when its first turn is written, that meant every conversation begun by
// opening the app and typing was recorded as belonging to no desk, while the
// sidebar drew a room as the one you were standing in — and the history list,
// which filtered by desk at the time, emptied itself the moment you clicked
// anything.
//
// What is pinned here is the rule that replaced it: the window opens where the
// user left it, and the entrance is a seed rather than a policy. A boot desk
// hardcoded to mode.Default would be the same bug wearing a fix's clothes —
// walk into a room, close the app, find yourself back in the hall.

import (
	"context"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/safety"
)

// bootFreshApp is an engine at the point startup() has it: a blank session, no
// desk yet, its own data root so the preference file is this test's alone.
func bootFreshApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{
		ctx:   context.Background(),
		emit:  func(string, ...any) {},
		dbDir: t.TempDir(),
	}, &conversation{id: newSessionID()})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a
}

func TestAFirstRunOpensAtTheEntrance(t *testing.T) {
	a := bootFreshApp(t)

	a.openAtRememberedDesk()

	if got := a.cur().desk.DeskName(); got != mode.Default {
		t.Fatalf("first run opened at %q, want the entrance %q", got, mode.Default)
	}
}

// The whole point: a desk the user walked to outlives the window.
func TestTheWindowReopensAtTheDeskYouLeftItAt(t *testing.T) {
	a := bootFreshApp(t)

	if err := a.setStation("coding", ""); err != nil {
		t.Fatalf("setStation(coding): %v", err)
	}
	pref, ok, err := config.LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("preference not written: ok=%v err=%v", ok, err)
	}
	if pref.LastDesk != "coding" {
		t.Fatalf("remembered %q, want coding", pref.LastDesk)
	}

	// Same data root, new window.
	next := seed(&App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir()}, &conversation{id: newSessionID()})
	t.Cleanup(func() {
		if next.db != nil {
			_ = next.db.Close()
		}
	})
	next.applyConfig(next.cur(), config.Config{
		SandboxRoot: t.TempDir(), ModelProvider: "aetox", ModelName: "aetox-tools:test",
		ApprovalMode: string(safety.ApprovalFullAccess),
	})
	next.openAtRememberedDesk()

	if got := next.cur().desk.DeskName(); got != "coding" {
		t.Fatalf("reopened at %q, want coding — the entrance is a seed, not a policy", got)
	}
}

// Opening a conversation from before desks existed is not a statement about
// where you want to start next time: "" is also the value that means nothing
// has been remembered, so writing it would silently forget the real answer.
func TestAPreDeskSessionDoesNotOverwriteTheRememberedDesk(t *testing.T) {
	a := bootFreshApp(t)
	if err := a.setStation("coding", ""); err != nil {
		t.Fatalf("setStation(coding): %v", err)
	}

	if err := a.setStation("", ""); err != nil {
		t.Fatalf("setStation(legacy full desk): %v", err)
	}

	pref, _, err := config.LoadModelPreference()
	if err != nil {
		t.Fatalf("LoadModelPreference: %v", err)
	}
	if pref.LastDesk != "coding" {
		t.Fatalf("remembered %q after reopening a legacy session, want coding", pref.LastDesk)
	}
}

// A start that cannot be explained is worse than a start somewhere ordinary:
// this runs before there is a window to show an error in.
func TestADeletedDeskFileFallsBackToTheEntrance(t *testing.T) {
	a := bootFreshApp(t)
	pref, _, err := config.LoadModelPreference()
	if err != nil {
		t.Fatalf("LoadModelPreference: %v", err)
	}
	pref.LastDesk = "a-desk-that-was-deleted"
	if err := config.SaveModelPreference(pref); err != nil {
		t.Fatalf("SaveModelPreference: %v", err)
	}

	a.openAtRememberedDesk()

	if got := a.cur().desk.DeskName(); got != mode.Default {
		t.Fatalf("fell back to %q, want the entrance %q", got, mode.Default)
	}
}

// The sidebar lights the room you are standing in by asking the engine which
// desk the live session is at. That session has no row until its first turn,
// so an answer read only from the table is "" — no room lit, while the engine
// is standing in one.
func TestTheLiveSessionReportsItsDeskBeforeItsFirstTurn(t *testing.T) {
	a := bootFreshApp(t)
	a.openAtRememberedDesk()

	if got := a.SessionMode(a.cur().id); got != mode.Default {
		t.Fatalf("live session reported desk %q, want %q", got, mode.Default)
	}
	// A stored session with no row is genuinely unknown — answering with
	// wherever this window happens to be would be inventing one.
	if got := a.SessionMode("20200101-000000.000"); got != "" {
		t.Fatalf("unknown session reported desk %q, want \"\"", got)
	}
}
