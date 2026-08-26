# Self-optimize: the skill-refine loop (spec, unbuilt)

Written 2026-08-26. The third stage of [[aetox-learning-loop-plan]] —
remember ✅ → summarize ✅ → **self-optimize** (this). Human-approved, never
silent. Grounded in the code map of the learning loop (file:line below); design
enriched by studying Hermes.

## The gap this closes

`jobs.outcome` — the human's 👍 / 👎 / "answer again" score — is recorded
(`desktop/jobs.go:267 RateTurn`, `:283 markTurnRedone`) and **never read back for
improvement**. The summarizer (`desktop/summarize.go:106`) reads only
`tool_runs WHERE ok=0` — crashes, not quality. So a skill that *runs fine and
gives bad guidance* leaves no trace anything acts on. The thumbs is the only
sensor for that, and it is the signal this loop consumes.

## The signal — treat the thumbs as first-class

Read the full outcome, not just failures:

- **👎** — the only catch for "ran, but wrong." The reason this loop reads
  `jobs.outcome`, not `tool_runs`.
- **👍** — the baseline ("this skill works, don't touch it") *and* the regression
  detector: a skill that was 👍 and turns 👎 after an edit means the edit made it
  worse — roll back.
- **redo ("answer again")** — the richest: a labelled before/after pair. The
  generator learns the fix from what actually changed, not from a guess.
- **density + scope (`jobs.agent`)** — one 👎 is noise; a 👎 pattern across
  projects is a real skill flaw (fix the skill — learning crosses projects,
  [[aetox-memory-scopes]]); one project only may be a project note instead.

## The four stages (each ships and tests on its own)

1. **Detector** (backend, deterministic, no model). A sibling of
   `summarizeFailures`, called from the same spot (`desktop/jobs.go:153`), but
   reads **`jobs`** (which the summarizer ignores): correlate `outcome='bad'` /
   `outcome_source IN ('thumb','redo')` with a skill name via `jobs.tool_seq` ∕
   `tool_runs.tool` (a `markdownSkill` invocation, `internal/skill/discovery.go:22`),
   scoped by `jobs.agent`. Flags a skill crossing a repeat threshold. Also tracks
   👍 as the baseline for regression.

2. **`skill.Apply` writer + approval gate** (backend). New `skill.Apply(dir, op,
   before, body)` modelled on `internal/learned/learned.go:311 Apply` (`os.WriteFile`),
   writing into `~/.aetox/skills/<name>/`. **Copy-on-first-edit:** because
   `internal/skill/bundled_skills.go:89 withBundled` drops a bundled skill when a
   disk folder of the same name exists, the first edit of a bundled skill must
   **copy the WHOLE folder out first** (SKILL.md *and* references/ — a partial
   copy cripples it), then edit on disk. Extend the `switch c.Kind` default at
   `desktop/pending.go:259` with `case kindSkill:` calling this.

3. **Generator** (model call, background, gated). When the detector flags a
   skill, draft a SKILL.md edit (op/before/body) and queue it through the door
   that already exists: `learned.Proposal{Kind:"skill", …}` (the comment at
   `internal/learned/tool.go:18` already reserves "skill") → `desktop/pending.go:88
   proposeLearned`. **Evidence-grounded (Hermes ethos):** the proposal carries the
   misfires that justify it — the redo-delta first, else the 👎 rows + scrubbed
   args/output — into `pending_changes.evidence`, so the human reviews *why*, not
   just the diff. Prefer the redo-delta as the primary teaching material.

4. **Approval UI** (frontend). A distinct **skill card** in the learning room
   (`desktop/frontend/src/lib/Settings.svelte:3000`), reusing the `decideChange`
   path (~2926) but rendering the SKILL.md diff (`before`/`body`) and the evidence
   trail. Approve → `ApprovePendingChange` → `skill.Apply`. Nothing writes without
   this click ([[aetox-permission-single-gate]]).

## Enrichments carried in from studying Hermes

- **Evidence-grounded proposals** (stage 3) — every proposed edit cites the
  concrete misfires, the way Hermes's `grounded-citations` makes a claim carry its
  source. A skill edit the human can't audit is a skill edit that shouldn't ship.
- **Regression watch** — after an approved edit, keep watching that skill's 👍/👎.
  Confirms the edit helped, or flags a rollback. This is what closes the loop
  rather than firing once.
- **Framing** — in the five-layer harness model (Instructions / Constraints /
  Feedback / Memory / Orchestration), Aetox's **Feedback** layer is the thinnest,
  and this loop is what fills it. Worth a one-page self-audit of the other four
  later.
- **Future extension, not v1:** Hermes also *creates* a skill when a successful
  pattern recurs and no skill covers it (👍 on a repeated work-shape with no
  owning skill). Our v1 is refine-only; create-a-skill is the natural next step
  once refine is trusted.

## Human-approval guarantee

No stage writes a skill. Detector and generator only *propose*; `skill.Apply`
runs only from an approved `pending_changes` row. This is the same single gate as
memory, extended — never a hidden second path ([[aetox-permission-single-gate]]).

## The one open decision

Skill cards in the **existing learning room** (new card kind, recommended — it is
a learning) vs a **third room**. Recommendation: learning room, distinct card, so
a skill edit reads differently from a memory line but lives where the human
already reviews learnings.

## Not part of this loop (noted so it isn't confused for it)

`skill import` (adopting portable agentskills.io/OpenClaw skills through a gate,
turning this session's hand-vendoring into a feature) is a separate, adjacent
idea for [[aetox-account-and-marketplace]] — related, not this.
