package main

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// Detecting a skill that keeps misfiring — the read side of the self-optimize
// loop (docs/architecture/self-optimize-loop-2026-08-26.md), stage one.
//
// The summarizer (summarize.go) reads tool_runs failures: crashes, refusals,
// the errors a tool returns. It is blind to the failure that matters most for a
// skill — one that ran fine and answered badly. `tool_runs.ok` cannot see it (a
// confidently wrong result returns ok=1), so the only sensor is jobs.outcome:
// the human's 👍 / 👎 under a reply, or the "answer again" that says the first
// one missed. jobs.go records it and, until this, nothing read it back — the one
// signal the floor collects and never spends.
//
// Deterministic on purpose, like the summarizer: a GROUP BY over the rated jobs,
// no model call. It answers one question — which skill correlates with bad
// outcomes — and proposes nothing. The generator and the human-approval gate are
// later stages; a misfire flagged here is a candidate for a second look, never a
// change anything acts on.

// skillMisfireMinBad is how many bad outcomes make a skill worth a second look.
// Three, like the summarizer's threshold: two is a coincidence a human would not
// want raised, three is the same skill missing three separate times.
const skillMisfireMinBad = 3

// skillMisfire is one skill that took part in bad-rated work. The good count is
// the baseline, kept because a skill that is three-bad-of-two-hundred is not the
// same problem as one that is three-bad-of-three, and it is also the frame a
// later stage watches for regression — a skill that was good and turns bad after
// an edit.
type skillMisfire struct {
	scope  string  // jobs.agent — the office the work was done in
	skill  string  // the skill named in the job's tool_seq
	bad    int     // bad-rated jobs it took part in
	good   int     // good-rated jobs (the baseline and the regression frame)
	jobIDs []int64 // the bad job rows, for the evidence a proposal will carry
}

// rate is the share of this skill's rated jobs that went bad.
func (m skillMisfire) rate() float64 {
	total := m.bad + m.good
	if total == 0 {
		return 0
	}
	return float64(m.bad) / float64(total)
}

// detectSkillMisfires reads the rated jobs and attributes each to the skills it
// ran. A job's tool_seq is the ">"-joined shape of its calls (jobs.go toolShape),
// and a token that is a known skill name is that skill taking part in the job.
// Only names in `skills` count: a bad turn that ran `shell` and `read` blames
// neither, because neither is a skill anyone could refine.
//
// Pure over (db, skills). The caller supplies the skill set, so this tests
// without a discovery scan and the same code backs the App reader below.
func detectSkillMisfires(db *sql.DB, skills map[string]bool, minBad int) []skillMisfire {
	if db == nil || len(skills) == 0 {
		return nil
	}
	type acc struct {
		bad, good int
		ids       []int64
	}
	byKey := map[string]*acc{}
	var order []string // first-seen order, so the sort's ties resolve the same way twice

	_ = eachRow(db, "skilltune: reading rated jobs", `
		SELECT id, agent, tool_seq, outcome FROM jobs
		  WHERE outcome IN (?, ?) ORDER BY id`,
		[]any{outcomeGood, outcomeBad},
		func(rows *sql.Rows) error {
			var id int64
			var agent, seq, outcome string
			if err := rows.Scan(&id, &agent, &seq, &outcome); err != nil {
				return err
			}
			counted := map[string]bool{} // a skill named twice in one job is one job
			for _, tok := range strings.Split(seq, ">") {
				if !skills[tok] || counted[tok] {
					continue
				}
				counted[tok] = true
				key := agent + "\x00" + tok
				c := byKey[key]
				if c == nil {
					c = &acc{}
					byKey[key] = c
					order = append(order, key)
				}
				if outcome == outcomeBad {
					c.bad++
					c.ids = append(c.ids, id)
				} else {
					c.good++
				}
			}
			return nil
		})

	var out []skillMisfire
	for _, key := range order {
		c := byKey[key]
		if c.bad < minBad {
			continue
		}
		sep := strings.IndexByte(key, 0)
		out = append(out, skillMisfire{
			scope: key[:sep], skill: key[sep+1:],
			bad: c.bad, good: c.good, jobIDs: c.ids,
		})
	}
	// Worst bad-rate first, then most bad, then name — the strongest candidate
	// leads and two passes over the same store return the same order.
	sort.Slice(out, func(i, j int) bool {
		if ri, rj := out[i].rate(), out[j].rate(); ri != rj {
			return ri > rj
		}
		if out[i].bad != out[j].bad {
			return out[i].bad > out[j].bad
		}
		return out[i].skill < out[j].skill
	})
	return out
}

// skillMisfires is the App-facing read: it supplies the live skill set — the
// bundled skills plus the user's ~/.aetox/skills — so only names that are really
// skills are attributed. Side-effect-free; what to do about a flagged skill is a
// later stage's decision, always behind human approval.
func (a *App) skillMisfires() []skillMisfire {
	db, err := a.database()
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, d := range skill.ListDiscovered(skill.DefaultDiscoveryPaths()) {
		if d.Name != "" {
			names[d.Name] = true
		}
	}
	return detectSkillMisfires(db, names, skillMisfireMinBad)
}
