package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/update"
	"github.com/Mike0165115321/Aetox/internal/version"
)

// The desktop binding for the version the app calls itself. Trivial, and
// pinned anyway: the About page renders whatever comes back, so a binding that
// returned "" would put a dash where the version belongs and nothing would
// fail. internal/version's own test is what keeps this value honest.
func TestAppVersionIsTheOneConstant(t *testing.T) {
	if got := (&App{}).AppVersion(); got != version.Current {
		t.Errorf("AppVersion() = %q, want %q", got, version.Current)
	}
}

// The binding's one job beyond calling the package: "the user switched this
// off" has to arrive as a Status the About page can render, not as a rejected
// promise the page would show as a red failure under a setting they chose.
func TestCheckForUpdateReportsDisabledAsAStatusNotAnError(t *testing.T) {
	t.Setenv(update.DisableEnv, "1")

	st, err := (&App{}).CheckForUpdate()
	if err != nil {
		t.Fatalf("err = %v, want nil — a disabled check is not a failure", err)
	}
	if !st.Disabled {
		t.Error("Status.Disabled is false after the check was switched off")
	}
	if st.Available {
		t.Error("a check that never ran must not claim an update is available")
	}
	// Still renderable: the About page shows the version and the release link
	// whether or not a check happened.
	if st.Current != version.Current || st.URL == "" {
		t.Errorf("nothing usable to render: %+v", st)
	}
}

// A nil Wails context is what a binding sees if anything ever calls it before
// startup. It must fall back to a real context rather than panic on the way
// into http.NewRequestWithContext.
func TestCheckForUpdateSurvivesANilContext(t *testing.T) {
	a := &App{}
	if a.ctx != nil {
		t.Fatal("this test is only meaningful with no Wails context")
	}
	if _, err := a.CheckForUpdate(); err != nil {
		t.Fatalf("err = %v — in a test the check is skipped, not failed", err)
	}
}

// An update kills the process, and the process is where the turn lives — so
// the refusal is the same one every session switch gets. The sentence is not:
// this one arrives under the update dialog's "อัปเดตไม่สำเร็จ", where advice
// about switching chats would read as the update itself having broken.
func TestApplyUpdateRefusesMidTurnInItsOwnWords(t *testing.T) {
	a := &App{}
	if err := a.beginTurn(); err != nil {
		t.Fatalf("beginTurn() = %v", err)
	}
	defer a.endTurn()

	err := a.ApplyUpdate()
	if err == nil {
		t.Fatal("ApplyUpdate() = nil while a turn is running — it would kill the turn with the process")
	}
	if !errors.Is(err, errTurnBusyUpdate) {
		t.Errorf("err = %v, want the update-specific refusal", err)
	}
	if strings.Contains(err.Error(), "สลับแชท") {
		t.Error("the refusal points at a door the user is not standing in")
	}
}
