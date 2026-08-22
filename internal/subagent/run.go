package subagent

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A RUN is several delegations that were declared as one job before any of them
// started: a name, a sentence saying what the job is, and the phases it will go
// through in order.
//
// Delegation on its own already fans out — four started before the first collect
// run at the same time (runner.go) — so this adds no parallelism. What it adds is
// the one thing fan-out alone cannot express: a phase that has not happened yet.
//
// That is the whole argument. A model that finds eight things and then decides,
// unprompted, to have them checked is a model doing a favour; the checking round
// is exactly what gets skipped when the answer already looks finished, and
// nothing on screen says it was skipped, because nothing ever said it was coming.
// A declared phase sits at 0/8 in the tray for as long as it is not done, in
// front of the person who asked. The guarantee is visibility, not enforcement —
// this package cannot make a model do a second round, and pretending otherwise
// would be a lie in the type system. It can make not doing it visible.
//
// Owner's call, 16 ส.ค.: declared by the main agent through `task_plan`, not read
// off a file. A file format for runs is the obvious next step and deliberately
// not this step — the shape of a run that has actually been used a few times is
// worth more than a format designed before one has.

// THERE IS NO TOKEN CEILING ON A RUN, and that is a decision rather than an
// omission (owner, 16 ส.ค.: "ไม่ตั้งลิมิตนะครับปล่อยเลย ๆ ให้เอเจนทำงานที่ได้รับเต็มที่").
//
// A ceiling was written here first and taken out the same hour. The argument
// against it is the one this product keeps making: a worker stopped halfway
// through a job it was given has not saved anything, it has spent everything it
// spent and handed back nothing anyone can use. The stop would land at whatever
// point the number happened to fall, which is never the point the work divides
// at.
//
// What replaces it is the count, not the cap. Every delegate's spend is stamped
// with who spent it (runningTask.tokensUsed) and the run's total is on the card
// while it works, so an expensive run is visible from the first phase and the
// person watching can press Stop — which already ends everything (StopAll). A
// number on screen with a hand on the switch is a bound; a number in a constant
// is a guess about a job nobody has run yet.

// RunPhase is one declared stage of a run.
type RunPhase struct {
	Title string
	// Planned is how many delegates the plan says belong in this phase, and 0
	// means the plan did not say. Optional on purpose: how many claims a
	// document turns out to make is not knowable when the run is declared, and a
	// required count would be answered with a guess that then reads as a promise.
	Planned int
}

// Run is one declared job. Everything on it is written once, at declaration —
// the live numbers are counted off the delegations that joined it, so there is
// no second copy of "how far along is this" to keep in step.
type Run struct {
	id      string
	name    string
	brief   string
	phases  []RunPhase
	started time.Time
}

// plan declares a run and makes it the one new delegations join.
//
// A second plan does not end the first: its delegates go on working and its row
// goes on updating, because a run's life is its delegates' lives and nothing
// here can end those. It only stops being the run a bare `task` joins.
func (r *Delegations) plan(name, brief string, phases []RunPhase) (*Run, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("a run needs a name — it is what the user reads above the whole group while it works")
	}
	cleaned := make([]RunPhase, 0, len(phases))
	seen := map[string]bool{}
	for _, p := range phases {
		title := strings.TrimSpace(p.Title)
		if title == "" {
			continue
		}
		if seen[title] {
			return nil, fmt.Errorf("two phases are both called %q — a delegate naming that phase could not say which one it meant", title)
		}
		seen[title] = true
		if p.Planned < 0 {
			p.Planned = 0
		}
		cleaned = append(cleaned, RunPhase{Title: title, Planned: p.Planned})
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("a run with no phases is just a delegate — name the stages this job goes through, including the ones that have not happened yet")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	run := &Run{
		id:      "run_" + strconv.FormatUint(r.runCounter.Add(1), 10),
		name:    name,
		brief:   strings.TrimSpace(brief),
		phases:  cleaned,
		started: time.Now(),
	}
	r.runs[run.id] = run
	r.runOrder = append(r.runOrder, run.id)
	r.openRun = run.id
	return run, nil
}

// currentRun is the run a delegate joins by naming one of its phases, or nil
// when nothing has been declared.
func (r *Delegations) currentRun() *Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.openRun == "" {
		return nil
	}
	return r.runs[r.openRun]
}

// hasPhase answers whether a title is one this run declared. Invented phases are
// refused at the door in task.go, and that refusal is what makes the declaration
// mean something: a run whose phases can be added to while it runs is a log of
// what happened, not a plan anyone can be held to.
func (run *Run) hasPhase(title string) bool {
	for _, p := range run.phases {
		if p.Title == title {
			return true
		}
	}
	return false
}

func (run *Run) phaseTitles() []string {
	out := make([]string, 0, len(run.phases))
	for _, p := range run.phases {
		out = append(out, p.Title)
	}
	return out
}

// PhaseInfo is one phase as the tray draws it: what was promised, and what has
// actually happened.
type PhaseInfo struct {
	Title string `json:"title"`
	// Planned is what the plan said, 0 when it did not say.
	Planned int `json:"planned"`
	// Done counts delegates that finished, Failed included — a phase whose
	// delegates all failed is finished and wrong, not unfinished.
	Done    int `json:"done"`
	Failed  int `json:"failed"`
	Running int `json:"running"`
	Waiting int `json:"waiting"`
	Tokens  int `json:"tokens"`
}

// RunInfo is one run as the tray draws it.
type RunInfo struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Brief   string    `json:"brief,omitempty"`
	Started time.Time `json:"started"`
	// Running means at least one delegate in this run is still going. A run with
	// phases left at 0 and nobody working is not running — it is abandoned, and
	// the difference is exactly what the tray exists to show.
	Running bool `json:"running"`
	// Tokens is what this run has spent so far, every delegate in it added up.
	// Shown rather than enforced against — see the note at the top of this file.
	Tokens int         `json:"tokens"`
	Phases []PhaseInfo `json:"phases"`
}

// Runs lists every run this session declared, newest first, with the live counts
// read off the delegations that joined each one.
func (r *Delegations) Runs() []RunInfo {
	r.mu.Lock()
	runs := make([]*Run, 0, len(r.runOrder))
	for i := len(r.runOrder) - 1; i >= 0; i-- {
		if run, ok := r.runs[r.runOrder[i]]; ok {
			runs = append(runs, run)
		}
	}
	tasks := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, t)
	}
	r.mu.Unlock()

	out := make([]RunInfo, 0, len(runs))
	for _, run := range runs {
		info := RunInfo{
			ID: run.id, Name: run.name, Brief: run.brief,
			Started: run.started,
			Phases:  make([]PhaseInfo, 0, len(run.phases)),
		}
		for _, p := range run.phases {
			phase := PhaseInfo{Title: p.Title, Planned: p.Planned}
			for _, t := range tasks {
				if t.run != run.id || t.phase != p.Title {
					continue
				}
				tokens := t.tokens()
				phase.Tokens += tokens
				info.Tokens += tokens
				switch {
				case !t.finished():
					phase.Running++
					if t.parkedAsk() != nil {
						phase.Waiting++
					}
					info.Running = true
				// Stopped before failed, because a stopped delegate is not a
				// successful one either and would otherwise be counted as
				// broken. The user ending work is not the job going wrong, and
				// a phase that reports one as the other tells them their run
				// hit an error they never had.
				case t.wasStopped():
					phase.Done++
				case !t.output.Success:
					phase.Done++
					phase.Failed++
				default:
					phase.Done++
				}
			}
			info.Phases = append(info.Phases, phase)
		}
		out = append(out, info)
	}
	return out
}
