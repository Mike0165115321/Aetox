// Package orchestrator tracks zero or more cognitive.Agent instances by ID
// within a single process, so a front end can run more than one agent at a
// time (e.g. a MAIN agent plus sub-agents) without inventing its own
// bookkeeping. See ARCHITECTURE.md §10.
package orchestrator

import (
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
)

// Info is the serializable view of a tracked agent — safe to cross a future
// local-RPC boundary without exposing *cognitive.Agent itself.
type Info struct {
	ID        string
	Model     string
	CreatedAt time.Time
}

type entry struct {
	agent     *cognitive.Agent
	model     string
	createdAt time.Time
}

// Orchestrator is deliberately ID/state-based rather than closure-based:
// a future local-RPC wrapper (front end as thin client) can sit on top of
// Spawn/Get/Stop/List without this type being redesigned.
type Orchestrator struct {
	mu      sync.RWMutex
	agents  map[string]*entry
	counter atomic.Uint64
}

func New() *Orchestrator {
	return &Orchestrator{agents: make(map[string]*entry)}
}

// Spawn creates a new agent and returns its ID.
func (o *Orchestrator) Spawn(cfg cognitive.AgentConfig) string {
	id := o.nextID()
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agents[id] = &entry{
		agent:     cognitive.NewAgent(cfg),
		model:     cfg.Model,
		createdAt: time.Now(),
	}
	return id
}

// Get returns the agent for id, or false if it was never spawned or already stopped.
func (o *Orchestrator) Get(id string) (*cognitive.Agent, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	e, ok := o.agents[id]
	if !ok {
		return nil, false
	}
	return e.agent, true
}

// Stop discards the agent for id. Returns an error if id was never spawned.
func (o *Orchestrator) Stop(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.agents[id]; !ok {
		return errors.New("orchestrator: unknown agent id " + id)
	}
	delete(o.agents, id)
	return nil
}

// List returns a serializable snapshot of every tracked agent.
func (o *Orchestrator) List() []Info {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]Info, 0, len(o.agents))
	for id, e := range o.agents {
		out = append(out, Info{ID: id, Model: e.model, CreatedAt: e.createdAt})
	}
	return out
}

func (o *Orchestrator) nextID() string {
	n := o.counter.Add(1)
	return time.Now().Format("20060102-150405.000") + "-" + strconv.FormatUint(n, 10)
}
