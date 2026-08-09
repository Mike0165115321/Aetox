package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

// EnginesFor reports the interchangeable services in one family — today only
// "automation" (n8n, Windmill) — with which of them this agent is working on.
//
// The picker in the composer is the caller. It asks by family rather than by a
// list of ids it keeps itself, so the day a third engine ships the picker offers
// it without being edited.
func (a *App) EnginesFor(family, agent string) []connect.Status {
	// Everything in the family, connected or not — the filtering is the caller's
	// to do, and it must not be done here.
	//
	// This filtered to `Connected` once, and the effect was that a user with
	// nothing attached saw no picker at all: the control that would have told
	// them what was missing, and handed them the way to fix it, disappeared
	// precisely when it was needed. Twice now the same mistake in this file —
	// first hiding the chip below two engines, then hiding it below one
	// connected one. The lesson is the same both times: a control that vanishes
	// in the broken state is not a tidy UI, it is a dead end.
	//
	// So the row arrives with `Connected` on it and the page says "ยังไม่ได้เชื่อม"
	// beside the name, with a way into the register.
	return connect.InFamily(connect.Family(family))
}

// UseEngine points an agent at one engine of a family and away from the others.
//
// It writes placement — the same `for:` list the settings register edits, and
// the same gate mode.Carries already reads — rather than inventing a per-session
// "active engine". Two reasons. A second piece of state answering "which engine"
// would be a second truth to keep in step with the first, and this one already
// decides which tools the agent is handed: choosing here means the wrong
// engine's tools are not in the model's list at all, which is stronger than
// telling it which one to prefer and hoping.
//
// It persists, which is the behaviour a person expects from a choice like this:
// you do not re-pick your automation engine every time you open the room.
func (a *App) UseEngine(family, agent, id string) error {
	target := connect.AgentTarget(agent)
	if target == "" {
		return errNoAgentNamed
	}
	for _, row := range connect.InFamily(connect.Family(family)) {
		targets := withoutFold(row.For, target)
		if row.ID == id {
			targets = append(targets, target)
		}
		if err := connect.SetTargets(row.ID, targets); err != nil {
			return err
		}
	}
	// The engine's tools are decided at build time, so the desks have to be
	// rebuilt for the change to reach the next turn rather than the next launch.
	a.applyConfig(a.cfg)
	return nil
}

// SetConnectionStartCommand records how to bring a self-hosted service up.
func (a *App) SetConnectionStartCommand(id, command string) error {
	return connect.SetStartCommand(id, command)
}

// StartConnectionServer runs the command the user gave for this service and
// returns once the address answers, or once it is clear it will not.
//
// Why this exists at all: an engine Aetox talks to is a program on the user's
// own machine, and the page that says "not connected" was until now unable to do
// the one thing that would fix it. Being told to go and find a terminal, from
// the screen you are already on, is the complaint this closes.
//
// The command is the user's own, typed once into the field beside this button.
// Nothing about where n8n or Windmill lives is written in this codebase — a
// guess would be wrong for everyone it was not written for — and the precedent
// is an MCP stdio server, which has always been a command in a config file that
// Aetox launches on request.
//
// Detached on purpose. A server must outlive the click that started it, so the
// process is not tied to any context this app holds; killing it is the user's
// own business, in the window it is running in.
func (a *App) StartConnectionServer(id string) error {
	row, ok := connect.StatusOf(id)
	if !ok {
		return fmt.Errorf("unknown connection: %q", id)
	}
	command := strings.TrimSpace(row.StartCommand)
	if command == "" {
		return errNoStartCommand
	}
	if row.BaseURL == "" {
		return fmt.Errorf("ยังไม่ได้ระบุที่อยู่ของ %s — ใส่ที่อยู่ก่อน จะได้รู้ว่าต้องรอให้อะไรตอบ", row.Label)
	}
	// Already up: starting a second copy would fight the first for the port and
	// the error the user reads would be about a port, not about the thing they
	// clicked.
	if reachable(row.BaseURL) {
		return nil
	}
	if err := launchDetached(command); err != nil {
		return fmt.Errorf("เปิด %s ไม่สำเร็จ: %w", row.Label, err)
	}
	// A server takes time to come up — n8n runs database migrations on a cold
	// start — so the button waits rather than reporting success at the moment a
	// process was spawned, which tells the user nothing.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		if reachable(row.BaseURL) {
			return nil
		}
	}
	return fmt.Errorf("สั่งเปิด %s แล้วแต่ %s ยังไม่ตอบใน 90 วินาที — ดูหน้าต่างที่เปิดขึ้นมาว่ามันบอกอะไร", row.Label, row.BaseURL)
}

// CheckConnectionServer reports whether the address answers at all, without
// caring about the credential.
//
// Separate from VerifyConnection, and both are worth having: this one says "is
// the program running", that one says "does my key work". Told apart they are
// two obvious fixes; run together they are one confusing failure, because a
// dead server and a bad key both come back as "could not connect".
func (a *App) CheckConnectionServer(id string) (bool, error) {
	row, ok := connect.StatusOf(id)
	if !ok {
		return false, fmt.Errorf("unknown connection: %q", id)
	}
	if row.BaseURL == "" {
		return false, fmt.Errorf("ยังไม่ได้ระบุที่อยู่ของ %s", row.Label)
	}
	return reachable(row.BaseURL), nil
}

// reachable asks whether anything is listening and answering HTTP there. Any
// status counts — a 401 is a running server refusing an anonymous request, and
// that is exactly the "it is up" this reports.
func reachable(baseURL string) bool {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return true
}

var errNoStartCommand = fmt.Errorf("ยังไม่ได้บอกว่าจะเปิดเซิร์ฟเวอร์นี้ด้วยคำสั่งอะไร")

func withoutFold(list []string, drop string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if !strings.EqualFold(strings.TrimSpace(v), drop) {
			out = append(out, v)
		}
	}
	return out
}

var errNoAgentNamed = fmt.Errorf("ต้องระบุว่าเป็นเอเจนตัวไหน")

// ConnectAccount attaches an account and places it in one call, because the
// user chose both on one screen. The engine is rebuilt afterwards so the desks
// pick the new tools up on the next turn rather than the next launch — the same
// promise SetMCPServerTargets makes.
// baseURL is ignored by every service that has an address of its own; the ones
// the user hosts refuse without it. Passed on the same call as the token and the
// placement for the reason all three are on the same screen — see connect.Connect.
func (a *App) ConnectAccount(id, token, baseURL string, targets []string) (connect.Account, error) {
	account, err := connect.Connect(context.Background(), id, token, baseURL, targets)
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
