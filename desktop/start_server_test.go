package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// Starting a self-hosted engine from the settings page.
//
// The page that says "not connected" was, until now, unable to do the one thing
// that fixes it — the user had to leave, find a terminal, and come back. What
// makes this safe to build is that the command is theirs: nothing about where
// n8n or Windmill lives is written in this codebase, because a guess would be
// wrong for everyone it was not written for.

func startApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	return bootDeskApp(t, "assistant")
}

// The command is remembered, and it is remembered beside the address rather
// than in the vault: it is a setting, not a secret, and a user has to be able to
// read it back and correct a typo.
func TestTheStartCommandIsRememberedAsASetting(t *testing.T) {
	a := startApp(t)

	if err := a.SetConnectionStartCommand("n8n", "  D:\\lab\\start.ps1  "); err != nil {
		t.Fatalf("SetConnectionStartCommand: %v", err)
	}
	if got := config.ConnectionStartCommand("n8n"); got != "D:\\lab\\start.ps1" {
		t.Errorf("stored %q, want it trimmed and kept verbatim otherwise", got)
	}
	rows := a.Connections()
	for _, r := range rows {
		if r.ID == "n8n" && r.StartCommand != "D:\\lab\\start.ps1" {
			t.Errorf("the page is told %q, want the command back so the field is not blank on return", r.StartCommand)
		}
	}
}

// A hosted service is not something this machine starts. Storing a command for
// one would put a button on a row where it can only ever fail.
func TestOnlySelfHostedServicesTakeAStartCommand(t *testing.T) {
	a := startApp(t)

	if err := a.SetConnectionStartCommand("github", "echo hi"); err == nil {
		t.Error("github accepted a start command; there is no local GitHub to start")
	}
}

// Pressing start with no command must say so rather than run nothing and
// report success.
func TestStartingWithoutACommandIsRefused(t *testing.T) {
	a := startApp(t)
	if err := config.SetConnectionBaseURL("n8n", "http://127.0.0.1:1"); err != nil {
		t.Fatalf("SetConnectionBaseURL: %v", err)
	}

	err := a.StartConnectionServer("n8n")
	if err == nil {
		t.Fatal("StartConnectionServer succeeded with nothing to run")
	}
	if !strings.Contains(err.Error(), "คำสั่ง") {
		t.Errorf("error = %q; want it to name the missing command", err)
	}
}

// An address with no server behind it, and one with. This is the question the
// page could not ask before, and it is deliberately a different question from
// "does my key work" — a dead server and a bad key used to arrive as the same
// unhelpful sentence.
func TestCheckingSaysWhetherTheProgramIsRunning(t *testing.T) {
	a := startApp(t)

	// Nothing listening: a port on loopback nobody bound.
	if err := config.SetConnectionBaseURL("n8n", "http://127.0.0.1:1"); err != nil {
		t.Fatalf("SetConnectionBaseURL: %v", err)
	}
	up, err := a.CheckConnectionServer("n8n")
	if err != nil {
		t.Fatalf("CheckConnectionServer: %v", err)
	}
	if up {
		t.Error("reported a server running on a port nothing is bound to")
	}

	// Something listening, and refusing anonymous callers — which is exactly
	// what a healthy n8n does. A 401 is "the program is up", and reading it as
	// down would send the user to restart a server that was already fine.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if err := config.SetConnectionBaseURL("n8n", srv.URL); err != nil {
		t.Fatalf("SetConnectionBaseURL: %v", err)
	}
	up, err = a.CheckConnectionServer("n8n")
	if err != nil {
		t.Fatalf("CheckConnectionServer: %v", err)
	}
	if !up {
		t.Error("a server answering 401 was reported as down — 401 is a running server refusing an anonymous request")
	}
}

// Starting something that is already up must be a no-op rather than a second
// copy: two servers fighting for one port fails with a message about the port,
// which is nothing to do with what the user clicked.
func TestStartingAnEngineThatIsAlreadyUpDoesNothing(t *testing.T) {
	a := startApp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if err := config.SetConnectionBaseURL("n8n", srv.URL); err != nil {
		t.Fatalf("SetConnectionBaseURL: %v", err)
	}
	// A command that would fail loudly if it ever ran.
	if err := a.SetConnectionStartCommand("n8n", "exit 1"); err != nil {
		t.Fatalf("SetConnectionStartCommand: %v", err)
	}

	if err := a.StartConnectionServer("n8n"); err != nil {
		t.Fatalf("StartConnectionServer on a live server = %v, want it to notice and stop", err)
	}
}

// Without an address there is nothing to wait for, so the refusal has to name
// that rather than time out for ninety seconds and blame the server.
func TestStartingWithoutAnAddressIsRefusedImmediately(t *testing.T) {
	a := startApp(t)
	if err := a.SetConnectionStartCommand("n8n", "echo hi"); err != nil {
		t.Fatalf("SetConnectionStartCommand: %v", err)
	}

	err := a.StartConnectionServer("n8n")
	if err == nil {
		t.Fatal("StartConnectionServer succeeded with no address to check")
	}
	if !strings.Contains(err.Error(), "ที่อยู่") {
		t.Errorf("error = %q; want it to name the missing address", err)
	}
}
