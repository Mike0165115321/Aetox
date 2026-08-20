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
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// DelegateSettings is what the switches look like from the UI's side: one block
// per kind, because they are two acts and the user meets them on two pages.
//
// One block until 2026-08-20, and the fusion showed on screen — the single
// switch was drawn on the เอเจน page only ({#if isAgent} in Settings.svelte)
// while it greyed out every row on the ซับเอเจน page, so somebody looking at
// their helpers saw a whole page of dead buttons with nothing explaining why.
type DelegateSettings struct {
	// Agents is the reach to เอเจน: a colleague who takes a whole job.
	Agents DelegateReach `json:"agents"`
	// Helpers is the reach to ซับเอเจน: the assistant's own hands in a second
	// context, for a step of its own work.
	Helpers DelegateReach `json:"helpers"`
	// Tokens is what the whole tool block costs right now, measured. One number
	// for both pages: it is the size of the thing the two switches are trimming,
	// and two copies of it would invite two answers.
	Tokens int `json:"tokens"`
}

// DelegateReach is one kind's switch, what it is worth, and who is in it.
type DelegateReach struct {
	// Off is this kind's master switch. True means the assistant hands nothing
	// to this kind — and if both are off, `task` is not built at all.
	Off bool `json:"off"`
	// Tokens is what THIS switch is worth right now: the difference between
	// carrying this kind and not, with the other switch left where it is.
	//
	// Marginal rather than absolute, because that is the number somebody is
	// actually deciding with, and the two are not the same: the pair costs 710,
	// เอเจน alone 629 and ซับเอเจน alone 471, so turning เอเจน off gives back
	// 239 while turning ซับเอเจน off gives back 81. An absolute "task costs 710"
	// on both cards would have promised each switch the whole saving.
	Tokens int `json:"tokens"`
	// Workers is every worker of this kind that exists, whether or not the
	// assistant may reach it — a switch you cannot see is a switch you cannot
	// turn back on.
	Workers []DelegateWorker `json:"workers"`
}

// DelegateWorker is one worker as the settings page shows it.
type DelegateWorker struct {
	Name string `json:"name"`
	// For is the clause that says what this worker is for — the half of its
	// description that makes it choosable. Same split the tool block uses.
	For string `json:"for"`
	// Agent separates เอเจน from ซับเอเจน, decided by which home the profile
	// lives in and never by a word inside its description. Kept on the row even
	// though the block it sits in already says so: the UI looks a worker up by
	// name from a shared row snippet, and a row that cannot say its own kind
	// would have to be told by whoever drew it.
	Agent bool `json:"agent"`
	// On is whether the assistant may hand work to it. The worker is reachable
	// by the user either way: this is a reach, not an existence.
	On bool `json:"on"`
}

// shippedReachableAgent is the one เอเจน a machine arrives with in reach.
//
// research rather than none, and rather than all five: an assistant that can
// hand nothing to anybody is a company with one employee, while five in the
// block cost ~21 tokens each in every message for colleagues most people never
// call. Going out to find something out is the errand that comes up on any desk,
// whatever the work is (owner, 20 ส.ค.).
//
// A name rather than a flag in the profile, and it is the honest spelling: this
// is one choice about how the app arrives, not a property of the agent. The day
// a second one earns its place here, this becomes a list.
const shippedReachableAgent = "research"

// shippedDelegation is the delegation a machine gets before anybody answers the
// question — read at startup, never written to disk (App.resolveConfig).
//
// It reads the roster rather than naming four agents, so an เอเจน installed
// later arrives OUT of reach like every other one, instead of quietly switching
// itself on. ซับเอเจน are left out entirely: they are the assistant's own hands
// and ship on, which is the asymmetry config.Config already spells into the two
// switch fields.
func shippedDelegation() (agents bool, workersOff []string) {
	for _, p := range subagent.List() {
		if p.Invalid != "" || p.Desk == "" {
			continue
		}
		if strings.EqualFold(p.Name, shippedReachableAgent) {
			continue
		}
		workersOff = append(workersOff, strings.ToLower(p.Name))
	}
	return true, workersOff
}

// DelegateSwitches reports both switches and what each is worth.
func (a *App) DelegateSwitches() DelegateSettings {
	cfg := a.cur().cfg
	out := DelegateSettings{
		Agents:  DelegateReach{Off: !cfg.DelegateAgents},
		Helpers: DelegateReach{Off: cfg.DelegateHelpersOff},
		Tokens:  a.ToolBlockTokens(),
	}
	off := lowered(cfg.WorkersOff)
	for _, p := range subagent.List() {
		if p.Invalid != "" {
			continue // a profile that will not load is the settings page's own error to show, not a row here
		}
		row := DelegateWorker{
			Name:  p.Name,
			For:   subagent.ForClause(p.Description),
			Agent: p.Desk != "",
			On:    !slices.Contains(off, strings.ToLower(p.Name)),
		}
		if row.Agent {
			out.Agents.Workers = append(out.Agents.Workers, row)
		} else {
			out.Helpers.Workers = append(out.Helpers.Workers, row)
		}
	}
	// What each switch is worth, with the other one exactly where the user left
	// it. Both directions from one subtraction: on a kind that is on it reads as
	// what turning it off gives back, on a kind that is off as what turning it
	// on will cost.
	here := a.delegationCost(!cfg.DelegateAgents, cfg.DelegateHelpersOff)
	out.Agents.Tokens = abs(here - a.delegationCost(cfg.DelegateAgents, cfg.DelegateHelpersOff))
	out.Helpers.Tokens = abs(here - a.delegationCost(!cfg.DelegateAgents, !cfg.DelegateHelpersOff))
	return out
}

// delegationCost is what the delegation tool would cost with these two switches,
// measured rather than remembered — same 4-bytes-per-token rate and the same
// reason as ToolBlockTokens: a constant would be right the day it was written.
//
// Built fresh rather than read off the registry, because the question is about a
// state this session is NOT in. Only the roster-shaping options are filled in:
// nothing here runs, and the definition is all that gets measured.
func (a *App) delegationCost(noAgents, noHelpers bool) int {
	tools := subagent.NewTaskTools(subagent.TaskOptions{
		Desk:       a.cur().desk,
		WorkersOff: a.cur().cfg.WorkersOff,
		NoAgents:   noAgents,
		NoHelpers:  noHelpers,
	})
	if len(tools) == 0 {
		return 0
	}
	def, ok := tools[0].(interface{ ToolDefinition() model.ToolDefinition })
	if !ok {
		return 0
	}
	payload, err := json.Marshal(def.ToolDefinition())
	if err != nil {
		return 0
	}
	return len(payload) / 4
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// SetDelegateOff flips one kind's switch and re-bootstraps, because what the
// tool carries is decided when the tools are built.
//
// kind is "agents" or "helpers" — the same two words the two settings pages are
// named for. Anything else is refused rather than guessed at: a typo that fell
// through to a default would silently flip the switch the caller did not mean.
func (a *App) SetDelegateOff(kind string, off bool) DelegateSettings {
	cfg := a.cfg
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "agents":
		if off == !a.cur().cfg.DelegateAgents {
			return a.DelegateSwitches() // never re-bootstrap to change nothing
		}
		cfg.DelegateAgents = !off
	case "helpers":
		if off == a.cur().cfg.DelegateHelpersOff {
			return a.DelegateSwitches()
		}
		cfg.DelegateHelpersOff = off
	default:
		return a.DelegateSwitches()
	}
	// Somebody has now answered, so the shipped default stops applying — including
	// when the answer is the same as the default. Without this the next start would
	// resolve as "nobody answered" and hand back a state the user had just left.
	cfg.DelegateSet = true
	a.applyConfig(a.cur(), cfg)
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
	current := lowered(a.cur().cfg.WorkersOff)
	if slices.Contains(current, name) == off {
		return a.DelegateSwitches()
	}
	if off {
		current = append(current, name)
	} else {
		current = slices.DeleteFunc(current, func(n string) bool { return n == name })
	}
	cfg := a.cfg
	cfg.WorkersOff = current
	cfg.DelegateSet = true
	a.applyConfig(a.cur(), cfg)
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
	if a.cur().registry == nil {
		return
	}
	held := connect.IDs()
	for _, def := range skill.NewDispatcher(a.cur().registry).ToolDefinitions() {
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
