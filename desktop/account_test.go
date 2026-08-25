package main

import (
	"testing"

	"github.com/Mikedev115/Aetox/internal/account"
)

func TestAccountStatusAnswersSignedOutWithoutASession(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())

	state := (&App{}).AccountStatus()
	if state.SignedIn {
		t.Fatal("an empty data root reported a signed-in account")
	}
	if state.Display != "" || state.User.ID != "" {
		t.Errorf("signed out must carry no identity: %+v", state)
	}
	// The card needs the doors to render at all, signed in or not.
	if len(state.Providers) == 0 {
		t.Error("no doors to offer")
	}
}

// Closed in every shipped build, because nothing is deployed for it to reach.
// The window reads this to leave the page out of the nav entirely.
func TestTheAccountPageIsClosedWithoutAnIDServer(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("AETOX_ID_URL", "")

	if (&App{}).AccountStatus().Configured {
		t.Fatal("the window would draw a sign-in with nothing behind it")
	}
	if _, err := (&App{}).StartAccountSignIn("github"); err == nil {
		t.Error("StartAccountSignIn opened a sign-in against no server")
	}
}

func TestAnOverrideOpensTheAccountPage(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("AETOX_ID_URL", "http://localhost:8080")

	state := (&App{}).AccountStatus()
	if !state.Configured {
		t.Fatal("AETOX_ID_URL did not open the page")
	}
	if state.Server != "http://localhost:8080" {
		t.Errorf("server = %q — it is named on screen, never guessed at", state.Server)
	}
}

func TestAccountStatusShowsTheStoredUser(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := account.Save(account.Session{
		Access: "a",
		User:   account.User{ID: "u1", Email: "mike@example.com"},
	}); err != nil {
		t.Fatal(err)
	}

	state := (&App{}).AccountStatus()
	if !state.SignedIn {
		t.Fatal("a stored session did not show as signed in")
	}
	// Display resolves name → email → id in one place; the UI must not have to
	// repeat that rule.
	if state.Display != "mike@example.com" {
		t.Errorf("display = %q, want the email fallback", state.Display)
	}
}

func TestCompletingASignInThatWasNeverStartedFails(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	app := &App{}

	// The window reloading mid-sign-in lands here. It is an error, not a panic
	// and not a silent success that leaves the card claiming a session.
	if _, err := app.CompleteAccountSignIn(); err == nil {
		t.Fatal("CompleteAccountSignIn succeeded with nothing pending")
	}
	if app.AccountStatus().SignedIn {
		t.Error("a failed completion left a session behind")
	}
	// And cancelling nothing is not an error either.
	app.CancelAccountSignIn()
}

func TestSigningOutOfNothingIsNotAnError(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := (&App{}).AccountSignOut(); err != nil {
		t.Fatalf("AccountSignOut with no session: %v", err)
	}
}
