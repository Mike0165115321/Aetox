package main

// The two switches on the assistant's reach, and the meter that makes them
// honest.
//
// A switch whose only visible effect is somewhere else is a switch nobody
// trusts. These exist to buy back context — 730 tokens for the master, ~21 per
// worker — and that number has to be on screen next to the thing that changes
// it, or the user is choosing blind between "keep a capability" and "keep some
// amount of something they cannot see".
//
// Which is why ToolBlockTokens MEASURES rather than remembers. A constant would
// be right the day it was written and quietly wrong the day somebody moved
// prose out of another tool's description (see internal/skill/guidance.go).
// Counting the block that would actually be sent costs a marshal of ~38 small
// structs, once, when a settings page opens.

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// DelegateSettings is what the switches look like from the UI's side.
type DelegateSettings struct {
	// Off is the master switch. True means the assistant does not delegate at
	// all and `task` is not built.
	Off bool `json:"off"`
	// Workers is every worker that exists, whether or not the assistant may
	// reach it — a switch you cannot see is a switch you cannot turn back on.
	Workers []DelegateWorker `json:"workers"`
	// Tokens is what the tool block costs right now, measured.
	Tokens int `json:"tokens"`
	// TaskTokens is what the master switch is worth: what `task` costs today, or
	// 0 when it is already off and there is nothing to weigh.
	TaskTokens int `json:"taskTokens"`
}

// DelegateWorker is one worker as the settings page shows it.
type DelegateWorker struct {
	Name string `json:"name"`
	// For is the clause that says what this worker is for — the half of its
	// description that makes it choosable. Same split the tool block uses.
	For string `json:"for"`
	// Agent separates เอเจน from ซับเอเจน, decided by which home the profile
	// lives in and never by a word inside its description.
	Agent bool `json:"agent"`
	// On is whether the assistant may hand work to it. The worker is reachable
	// by the user either way: this is a reach, not an existence.
	On bool `json:"on"`
}

// DelegateSwitches reports both switches and what they are worth.
func (a *App) DelegateSwitches() DelegateSettings {
	out := DelegateSettings{Off: a.cfg.DelegateOff, Tokens: a.ToolBlockTokens()}
	off := lowered(a.cfg.AgentsOff)
	for _, p := range subagent.List() {
		if p.Invalid != "" {
			continue // a profile that will not load is the settings page's own error to show, not a row here
		}
		out.Workers = append(out.Workers, DelegateWorker{
			Name:  p.Name,
			For:   subagent.ForClause(p.Description),
			Agent: p.Desk != "",
			On:    !slices.Contains(off, strings.ToLower(p.Name)),
		})
	}
	if !out.Off {
		out.TaskTokens = a.toolTokens("task")
	}
	return out
}

// SetDelegateOff flips the master switch and re-bootstraps, because whether the
// tool exists is decided when the tools are built.
func (a *App) SetDelegateOff(off bool) DelegateSettings {
	if off == a.cfg.DelegateOff {
		return a.DelegateSwitches() // never re-bootstrap to change nothing
	}
	cfg := a.cfg
	cfg.DelegateOff = off
	a.applyConfig(cfg)
	return a.DelegateSwitches()
}

// SetAgentOff takes one worker out of the assistant's reach, or puts it back.
//
// It does NOT disable the worker: the user still opens a chat with it and still
// writes @name. Anything the UI says about this has to name whose reach is
// narrowed, or somebody reads "off" as "gone".
func (a *App) SetAgentOff(name string, off bool) DelegateSettings {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return a.DelegateSwitches()
	}
	current := lowered(a.cfg.AgentsOff)
	if slices.Contains(current, name) == off {
		return a.DelegateSwitches()
	}
	if off {
		current = append(current, name)
	} else {
		current = slices.DeleteFunc(current, func(n string) bool { return n == name })
	}
	cfg := a.cfg
	cfg.AgentsOff = current
	a.applyConfig(cfg)
	return a.DelegateSwitches()
}

// ToolBlockTokens is roughly what this session's tool block costs per request.
//
// Rough on purpose, at the same 4-bytes-per-token rate desktop/tool_budget_test.go
// uses: a real tokenizer here would make the number depend on which model is
// loaded, and this is a figure somebody reads to decide whether a switch is
// worth flipping — not an invoice.
//
// Connection tools an account has not been added for are left out, because they
// are left out of what is sent.
func (a *App) ToolBlockTokens() int {
	total := 0
	a.eachToolDefinition(func(name string, bytes int) {
		total += bytes
	})
	return total / 4
}

func (a *App) toolTokens(want string) int {
	found := 0
	a.eachToolDefinition(func(name string, bytes int) {
		if name == want {
			found = bytes
		}
	})
	return found / 4
}

func (a *App) eachToolDefinition(fn func(name string, bytes int)) {
	if a.registry == nil {
		return
	}
	held := connect.IDs()
	for _, def := range skill.NewDispatcher(a.registry).ToolDefinitions() {
		if !connect.Allows(def.Function.Name, held) {
			continue
		}
		payload, err := json.Marshal(def)
		if err != nil {
			continue
		}
		fn(def.Function.Name, len(payload))
	}
}

func lowered(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			out = append(out, n)
		}
	}
	return out
}
