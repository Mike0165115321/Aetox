package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/account"
)

// Aetox account bindings. Separate from oauth.go next door on purpose: that
// file signs in to a model provider so a request can be paid for, this one
// signs in to Aetox itself. Nothing here gates anything — the app works signed
// out, and the account exists so a later store knows who bought what.

// AccountState is everything Settings needs to draw the card, and it never
// carries a token.
type AccountState struct {
	// Configured is whether this build has an id server at all. False today:
	// nothing is deployed, so every surface that could show a sign-in leaves it
	// out entirely rather than offering a button that reaches nothing.
	Configured bool         `json:"configured"`
	SignedIn   bool         `json:"signed_in"`
	User       account.User `json:"user"`
	// Display is the name to put on screen, already resolved through the
	// name → email → id fallback so the UI does not repeat that rule.
	Display string `json:"display"`
	// Providers are the doors this id server offers, in the order to show them.
	Providers []string `json:"providers"`
	// Server is shown in full because "which server holds my account" is a
	// question a local-first app should never make anyone guess at.
	Server string `json:"server"`
}

// errNoAccountSignInPending is what CompleteAccountSignIn returns when it was
// called without a Start, which in practice means the window reloaded while a
// sign-in was open.
var errNoAccountSignInPending = errors.New("ไม่มีการเข้าสู่ระบบที่ค้างอยู่")

// pendingAccountSignIn holds the authorization between StartAccountSignIn and
// CompleteAccountSignIn. One at a time: there is one account, and a second
// click means the user gave up on the first attempt.
var pendingAccountSignIn = struct {
	sync.Mutex
	pending *account.Pending
	cancel  context.CancelFunc
	ctx     context.Context
}{}

// AccountStatus answers from disk. The UI calls it on every render of the
// settings page, so it must not touch the network.
func (a *App) AccountStatus() AccountState {
	state := AccountState{
		Configured: account.Configured(),
		Providers:  account.Providers(),
		Server:     account.BaseURL(),
	}
	if user, ok := account.Current(); ok {
		state.SignedIn, state.User, state.Display = true, user, user.Display()
	}
	return state
}

// StartAccountSignIn opens a sign-in and returns the URL for the UI to put in
// front of the user. Nothing is stored until CompleteAccountSignIn succeeds.
func (a *App) StartAccountSignIn(provider string) (string, error) {
	pending, err := account.Start(provider)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	pendingAccountSignIn.Lock()
	defer pendingAccountSignIn.Unlock()
	if prev := pendingAccountSignIn.pending; prev != nil {
		prev.Cancel()
		pendingAccountSignIn.cancel()
	}
	pendingAccountSignIn.pending, pendingAccountSignIn.ctx, pendingAccountSignIn.cancel = pending, ctx, cancel
	return pending.URL, nil
}

// CompleteAccountSignIn blocks until the browser comes back. The UI calls it
// straight after StartAccountSignIn and shows a waiting state meanwhile.
func (a *App) CompleteAccountSignIn() (AccountState, error) {
	pendingAccountSignIn.Lock()
	pending, ctx, cancel := pendingAccountSignIn.pending, pendingAccountSignIn.ctx, pendingAccountSignIn.cancel
	pendingAccountSignIn.Unlock()

	if pending == nil {
		return a.AccountStatus(), errNoAccountSignInPending
	}
	defer func() {
		cancel()
		pendingAccountSignIn.Lock()
		if pendingAccountSignIn.pending == pending {
			pendingAccountSignIn.pending, pendingAccountSignIn.ctx, pendingAccountSignIn.cancel = nil, nil, nil
		}
		pendingAccountSignIn.Unlock()
	}()

	if _, err := pending.Wait(ctx); err != nil {
		return a.AccountStatus(), err
	}
	return a.AccountStatus(), nil
}

// CancelAccountSignIn gives up on an attempt in flight, releasing the local
// listener. Safe to call when nothing is pending.
func (a *App) CancelAccountSignIn() {
	pendingAccountSignIn.Lock()
	defer pendingAccountSignIn.Unlock()
	if pendingAccountSignIn.pending == nil {
		return
	}
	pendingAccountSignIn.pending.Cancel()
	pendingAccountSignIn.cancel()
	pendingAccountSignIn.pending, pendingAccountSignIn.ctx, pendingAccountSignIn.cancel = nil, nil, nil
}

// AccountSignOut signs out here and, if it can be reached, on the server too.
//
// The local half always happens, so the returned error means "you are signed
// out on this machine but the server was not told" rather than "nothing
// happened". The UI says exactly that instead of leaving the card signed in.
func (a *App) AccountSignOut() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return account.SignOut(ctx)
}

// AccountRefresh asks the server who this session belongs to and writes the
// answer back. It is the one binding here that touches the network on purpose:
// it is how a session revoked from the back office stops showing a name.
func (a *App) AccountRefresh() (AccountState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := account.Me(ctx); err != nil {
		return a.AccountStatus(), err
	}
	return a.AccountStatus(), nil
}
