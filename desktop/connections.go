package main

import (
	"context"

	"github.com/Mike0165115321/Aetox/internal/connect"
)

// Connection bindings — external accounts the agent works on the user's behalf
// with, and which desks may use them.
//
// Every method takes a connection id rather than naming a service, so adding
// Gmail adds an entry to internal/connect's catalog and nothing here. That is
// the whole reason this file replaced a set of GitHub-shaped bindings: the
// second service would have doubled them, and the third would have tripled a
// pattern nobody meant to establish.
//
// Placement uses PlacementTargets (desktop/mcp.go) — the same list, the same
// ids, as an MCP server's `for:`.

// Connections lists every connection this build knows, with its account state
// and placement. Never a token.
func (a *App) Connections() []connect.Status {
	return connect.List()
}

// ConnectAccount attaches an account and places it in one call, because the
// user chose both on one screen. The engine is rebuilt afterwards so the desks
// pick the new tools up on the next turn rather than the next launch — the same
// promise SetMCPServerTargets makes.
func (a *App) ConnectAccount(id, token string, targets []string) (connect.Account, error) {
	account, err := connect.Connect(context.Background(), id, token, targets)
	if err != nil {
		return connect.Account{}, err
	}
	a.applyConfig(a.cfg)
	return account, nil
}

// SetConnectionTargets moves an existing connection between desks and agents.
//
// Separate from ConnectAccount for the reason SetMCPServerTargets is separate
// from SaveMCPServer: flipping a switch on a row must not send the token field
// the page happens to be holding back to the engine.
func (a *App) SetConnectionTargets(id string, targets []string) error {
	if err := connect.SetTargets(id, targets); err != nil {
		return err
	}
	a.applyConfig(a.cfg)
	return nil
}

// VerifyConnection re-checks one connection against the service it belongs to.
func (a *App) VerifyConnection(id string) (connect.Account, error) {
	return connect.Verify(context.Background(), id)
}

// DisconnectAccount forgets the credential and keeps the placement, so
// reconnecting does not ask the user where it belongs a second time.
func (a *App) DisconnectAccount(id string) error {
	if err := connect.Disconnect(id); err != nil {
		return err
	}
	a.applyConfig(a.cfg)
	return nil
}
