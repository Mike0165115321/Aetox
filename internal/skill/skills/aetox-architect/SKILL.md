---
name: aetox-architect
description: อ่านระบบที่มีอยู่จริงแล้วกางออกมาเป็นเอกสารสถาปัตยกรรม - เดินเก้าขั้นจากรับโจทย์ ไปสำรวจของจริง ถามสิ่งที่ยังไม่รู้ วาดแผนที่โมดูลกับการไหลของข้อมูล ประเมินหนี้ทางเทคนิคกับความเสี่ยง แล้วเขียนเป็นแม่แบบเอกสาร ใช้ตอนต้องเข้าใจโค้ดเบสก่อนลงมือ ตรวจระบบปัจจุบัน ส่งงานต่อให้คนอื่น หรือทานแผนที่เสนอมากับหลักฐานในระบบจริง
source: https://github.com/Mikedev115 (senior-architect-agent)
license: MIT
copyright: Copyright (c) 2026 Aetox Skills
---

# Aetox Architect

Use this skill to make the agent understand before it acts.

This skill is not a diagram generator. It is a discipline layer for
architecture work.

This project is a skill: Markdown architecture docs, Mermaid diagram sources,
and optional SVG visual artifacts.

Expanded direction:

> This skill helps AI agents unfold existing systems into architecture maps
> that humans and future AI agents can understand, review, and continue from.

A senior architect does not stop at describing the system. Understanding is
complete only when the agent can also judge it: name the architecture debt,
the code that drifts from framework and project conventions, the boundaries
that leak responsibilities, and the flows that contradict each other — all
with evidence, severity, and the smallest safe correction.

Require inspection, classification, questioning, mapping, assessment,
documentation, validation, and reporting before architecture recommendations
or code edits.

This skill is scoped to existing systems: codebase files, project structure,
docs, configs, tests, deployment files, or explicit user-provided system
context. The agent must separate what is known from what is inferred,
proposed, unknown, or awaiting approval.

Sibling skill routing:

If the request is a pure raw idea with no implementation and no existing
system evidence, route to `$idea-to-architecture-agent`. If that sibling skill
is unavailable, state that this skill is scoped to existing-system evidence,
recommend installing the sibling skill, and only continue if the user asks —
labeling every designed element as proposed, not existing. Do not make this
skill depend on the sibling skill.

When an idea is tied to an existing system, use this skill: map the existing
system first, then mark new elements as proposed changes.

Core flow:

```txt
Intake
-> Inspect
-> Classify
-> Question
-> Map
-> Assess
-> Document
-> Validate
-> Report
```

## Operating Rules

0. Default to Scan Mode. Promotion to Focus or Full Mode requires inspected
   evidence, a named trigger, and a stated risk — never promote on speculation.
1. Never design before inspecting the real system.
2. Never modify code before understanding the relevant architecture.
3. Never assume silently.
4. Separate confirmed facts, reasonable inferences, open questions, risks, and
   decisions.
5. Ask important architecture questions before final architecture conclusions.
6. Prefer useful documentation over decorative documentation.
7. Prefer Markdown and Mermaid before SVG.
8. Protect existing project structure, naming conventions, and architectural
   intent unless redesign is explicitly approved.
9. Avoid document bloat. Create only the files needed for the system's
   complexity.
10. Refuse unsupported claims.
11. Never present proposed changes as existing implementation.
12. Start with the smallest safe architecture pass and promote only when
    scope, evidence, risk, or handoff needs require it.
13. Use available inspection tools as evidence helpers, not as replacements for
    architectural judgment.
14. Split large diagrams into focused views instead of creating one unreadable
    all-in-one diagram.
15. Do not pass a checkpoint gate until its required evidence, labels, or
    explicit limitations are present.
16. Describe, then judge. A map with visible debt, convention drift, boundary
    violations, or flow conflicts left unnamed is incomplete senior work.
17. Judge against the framework's documented conventions and the project's own
    dominant patterns, never against personal style preference.
18. Findings are observations, not a redesign license. Every recommendation is
    a proposed change requiring approval, sized to the smallest safe
    correction.
19. Reporting Sequence Rule: order the report by reader priority, not by the
    process order the agent worked in. Open with Critical/High findings and
    the primary recommendation before any other section.

## Checkpoint Gates

Use these gates to keep the flow enforceable:

- Intake gate: initial scope, selected pass level, and output path are
  recorded, or an early exit is declared when no architecture pass is needed,
  or the request is routed to the sibling skill.
- Inspection gate: evidence or user-provided system context is recorded, with
  inspection limitations when something cannot be verified.
- Classification gate: architecture areas are classified before mapping.
- Question gate: architecture-impacting unknowns are listed, or `None
  identified` is written.
- Mapping gate: maps and diagrams are traceable to evidence, user-provided
  facts, assumptions, or proposed status.
- Assessment gate: every finding has evidence, impact, and severity, or
  `None identified` is written. No finding rests on style preference alone.
- Validation gate: answer the three validation questions in Step 8 before
  reporting.

## Discipline

- Evidence-first.
- Confirm claims from files or user-provided context.
- Mark missing categories as not observed.
- Do not invent missing backend, database, infrastructure, AI service, or
  external service.
- Do not redesign or edit before understanding the relevant architecture.
- Mark suggested changes as proposed until the owner approves them.
- Name observed debt, convention drift, boundary violations, and flow
  conflicts with evidence — do not soften findings into vague generalities,
  and do not invent findings to appear thorough.

Primary outputs:

- Architecture overview
- System boundary
- Module map
- Data flow
- Workflow map
- File responsibility map
- Debt register
- Open questions
- Risk register
- AI agent notes
- Mermaid diagrams
- SVG visual artifacts when useful for presentation or review

## Step 1: Intake

Read the user's request and identify available inputs:

- Existing files, project structure, docs, configs, tests, or deployment files
- Explicit constraints, priorities, and requested outputs
- The narrowest useful project area or module scope

Do not start with architecture conclusions.

Routing rule:

If the request is a pure raw idea with no implementation and no existing
system evidence, apply the sibling skill routing above instead of running this
flow.

Prefer module-level mapping when the user asks about one module or workflow.
Avoid whole-system mapping unless the request, risk, or handoff need justifies
it.

Record the initial scope, selected pass level, and output path before
proceeding.

Early Exit Rule:

If the task is a small implementation fix with no architecture impact, do not
run an architecture pass or create architecture documentation. Report:

- `No architecture pass required`
- Why there is no architecture impact
- Any uncertainty that would change that conclusion
- Recommended next step

If architecture impact is unclear, use Scan Mode rather than exiting.

Right-Sized Pass Control:

Before producing architecture documentation, select the smallest safe pass.

- Scan Mode: use for small, bounded, exploratory, or low-risk work. Output a
  compact architecture note.
- Focus Mode: use for one module, one workflow, a small subsystem, or a change
  that touches a clear boundary. Output only the maps or notes needed for that
  scope.
- Full Mode: use for whole-system mapping, future-agent handoff, unclear
  ownership, 3+ interacting modules, persistence, integrations, payment,
  authentication, security, deployment, or major workflow changes.

Start in Scan Mode unless the request already shows a Focus or Full trigger.
Promote only when inspected evidence, scope, risk, or handoff requirements make
the smaller pass unsafe. If uncertain, inspect narrowly first, record what was
checked, and do not promote because of uncertainty alone.

Promotion gate:

- Name the trigger.
- Cite the evidence or user request.
- State the risk of staying in the smaller pass.
- Keep the current pass if the trigger is speculative.

## Step 2: Inspect

Inspect available evidence: project files, folders, docs, configs, tests,
package manifests, build scripts, deployment files, naming patterns, and
architecture signals.

Reuse existing architecture context before re-mapping. Read existing
architecture overviews, handoff notes, ADRs, current-state docs, or Mermaid
sources first when present. Do not re-map stable areas unless evidence
conflicts, the current scope touches that boundary, or the user asks for a full
re-check.

Minimum inspection targets when available:

- Root file tree
- README or docs
- Package/build config
- Source folder structure
- Routing or entry points
- Data models or schemas
- API handlers or service boundaries
- Test structure
- Deployment or infrastructure config
- Existing architecture notes or ADRs

Inspection budget for large repositories:

- Start with top-level structure.
- Read README and existing docs.
- Read package, build, framework, and environment config.
- Identify entry points.
- Inspect routing, module registries, or service registries.
- Inspect relevant modules only.
- Go deeper only when evidence, risk, or the selected scope requires it.

Use available inspection tools such as file search, file tree inspection, git
history, validators, Mermaid checks, and codebase graph queries when they
help. When a graph tool is available, listing modules with their consumers
early can reveal high-impact inspection targets and shared coupling. Treat
tool output as evidence to interpret, not as architecture by itself.

Record only what is visible from files, explicitly provided by the user, or
clearly labeled as assumption.

If evidence or user-provided context is missing for an important area, record
the inspection limitation instead of filling the gap with a guess.

## Step 3: Classify

Classify the system into applicable areas:

- Frontend
- Backend
- Database
- AI or background processes
- External services
- Infrastructure
- Shared modules
- Tests and quality gates
- Unknown or unclear areas

Mark missing categories as not observed.

Do not map before classification is complete.

## Step 4: Question

Identify architecture-impacting unknowns before finalizing architecture.

Ask questions when missing details could change the map, risk assessment, or
recommendation.

Ask only important questions. Do not block on minor details that can be marked
as assumptions.

Separate:

- Confirmed facts
- Reasonable inferences
- Proposed changes
- Assumptions
- Open questions
- Risks
- Decisions already made
- Decisions still needed
- Decisions requiring approval

If no architecture-impacting questions are found, write `None identified` so
the absence is explicit.

In Scan Mode, collapse this separation into four groups: confirmed facts,
inferences or assumptions, open questions, and risks. Use the full separation
only in Focus Mode and Full Mode.

Loopback rule:

If mapping, documentation, or validation exposes an architecture-changing
unknown, return to Step 4 before final conclusions. Either ask the question or
mark the output as incomplete and proceed only with a clearly labeled
assumption.

## Step 5: Map

Produce maps that help humans and future AI agents understand the system
quickly.

Use the output path selected during intake.

For Scan Mode, produce one compact architecture note and nothing else. Steps
may be merged; only the discipline must survive. The note contains: pass level
and reason, scope, evidence checked, skipped areas, confirmed facts, inferences
or assumptions, findings, open questions, risks, and safe next actions. Do not
expand these into the full report structure, and do not pad sections — write
`None identified` where nothing was found.

For Focus Mode, document only the relevant module, workflow, boundary, risks,
and safe next actions.

Use the smallest useful set:

- Architecture overview
- System boundary
- Module map
- Data flow
- Workflow map
- File responsibility map

Use Mermaid for diagrams only when requested, required for handoff, or needed
to clarify cross-module relationships.

Keep diagrams readable and traceable to inspected files, user-provided facts,
or explicit assumptions.

For large systems, split diagrams by architecture question:

- Boundary
- Module relationships
- Workflow
- Data flow

Treat SVG as an optional visual artifact for presentation or review. Generate
it from Mermaid when it helps, and do not use SVG as the architecture source of
truth.

Do not include untraceable components in maps or diagrams. Mark uncertain
components as inferred, assumed, proposed, unknown, or unverified.

Knowledge-Graph-Assisted Mapping (optional):

When a codebase intelligence MCP is available, query dependency and caller
data to accelerate mapping. Graph-derived data maps into the existing
evidence-strength taxonomy:

- Static-analysis-resolved relationships: `Direct`, with source noted as
  `[from graph: <tool>]`
- Heuristic or estimated relationships: `Inferred` + `Verify first: Yes`

When graph output and file reading conflict, file reading wins. Code is the
source of truth; graph data may be stale or incomplete.

Useful graph queries during mapping:

- Module dependency chains from entry point to persistence.
- Caller lists for key functions or modules — the blast radius of a change.
- Shared modules with high consumer counts (inspect further; not a finding
  by itself).
- Validation of the manually built map: divergence between the map and
  graph data is a signal to re-inspect, and file reading decides.

Always cross-check critical graph findings with direct file reading. Graph is
a navigation aid, not a replacement for architectural judgment (Rule 13).

If no graph tool is available, map with file search and file reads as usual —
the mapping discipline is unchanged.

## Step 6: Assess

Judge what was mapped. Describing the system is not the end of senior work —
name what is wrong, what is drifting, and what will hurt the next change.

Assess these dimensions against inspected evidence:

- Architecture debt: structures that work today but tax every future change —
  duplicated responsibilities, hidden coupling, missing quality gates,
  workarounds that became permanent.
- Separation of concerns: modules or layers that mix responsibilities the
  structure claims to separate, and boundaries that leak (UI reading storage
  directly, business logic inside handlers, shared mutable state).
- Framework convention drift: code that fights the framework's documented
  conventions — wrong lifecycle usage, bypassed routing or data layers,
  reimplemented framework features.
- Project convention drift: code that breaks the project's own dominant
  patterns — naming, module layout, error handling, or data access done one
  way in most places and another way in a few.
- Flow conflicts: trace the main flows from entry point to data and back, and
  flag contradictions — two sources of truth for the same state, circular
  dependencies, dead or unreachable paths, side effects crossing module
  boundaries, flows that bypass declared layers.

Every finding must state:

- Evidence: the files or observed signals that show it.
- Impact: why it matters — what it breaks, slows, or makes unsafe to change.
- Severity: `Critical` (breaks correctness or contradicts the system's own
  flow), `High` (actively harms change safety), `Medium` (taxes future work),
  `Low` (worth noting, not worth acting on now).
- Confidence: `Direct` or `Inferred`, with `Verify first: Yes` when the
  finding needs confirmation before anyone acts on it.
- Direction: the smallest safe correction, marked as a proposed change
  requiring approval. Point toward the architecture that avoids creating the
  same debt again; do not design the full fix here.

What is not a finding:

- Style or naming taste with no convention behind it.
- Working code that is merely different from how the agent would write it.
- Hypothetical scaling or "best practice" concerns with no evidence of impact
  in this system.
- Anything the agent cannot trace to inspected evidence.

Graph-Derived Signals (when available):

Codebase intelligence data feeds into the existing assessment dimensions — it
does not create new ones. Treat graph results as signals that accelerate
inspection, not as findings that skip the finding shape.

- Circular dependencies (graph cycles): a candidate for the `Flow conflicts`
  dimension. Must still state evidence, impact, severity, and confidence
  through the standard finding shape. A circular dependency between two
  modules that the language tolerates may be `Medium`; one that breaks
  correctness is `Critical`. No severity is pre-assigned.
- Layer boundary violations (a graph edge that bypasses declared layers, such
  as UI importing persistence directly): a candidate for `Separation of
  concerns`. Cross-check with file reading before filing.
- High-consumer modules (fan-in outlier in this codebase): a signal to
  inspect — not a finding. Shared utilities are designed to have high fan-in.
  Inspect whether the coupling is structural or incidental; file a finding
  only when evidence confirms harm.
- Zero consumers in graph: `Inferred` + `Verify first: Yes`. Graph cannot
  detect dynamic imports, dependency injection containers, reflection, or
  public API surfaces. An unreferenced export is not dead code until
  confirmed by search and human judgment.

All graph-derived findings follow the same confidence taxonomy
(`Direct`/`Inferred`/`Verify first: Yes`) and the same finding shape as
file-derived findings. Graph does not bypass assessment discipline.

Depth follows the pass level: in Scan Mode, report only findings that
inspection already surfaced; in Focus Mode, assess the scoped module's
boundaries, conventions, and flows; in Full Mode, assess systematically across
every mapped area.

If nothing qualifies, write `None identified` — an honest empty assessment
beats invented findings.

## Step 7: Document

Use templates from `templates/` when creating architecture outputs:

- `architecture-overview.md`
- `system-boundary.md`
- `module-map.md`
- `data-flow.md`
- `workflow-map.md`
- `file-responsibility-map.md`
- `debt-register.md`
- `open-questions.md`
- `risk-register.md`
- `ai-agent-notes.md`
- `decision-record.md`

Do not create every template by default.

Choose based on actual complexity and user need.

Artifact budget:

- Scan Mode: one compact architecture note.
- Focus Mode: one to three artifacts.
- Full Mode: full output package only when scope, risk, or handoff requires it.

If output exceeds the budget, state why before creating extra artifacts.


## Step 8: Validate

Before reporting, answer these three validation questions:

1. Claim traceability:
   Does every important claim have a source?
   If not, mark it as `Unverified`, `Assumed`, or remove it before passing.
2. Scope alignment:
   Does the final scope match the intake scope?
   If it expanded, state what was added, why, and whether it is approved.
3. Handoff readiness:
   Does the handoff include unknowns and safe next actions?
   If none are found, write `None identified` instead of leaving the section
   blank.

Use evidence strength for important claims:

- `Direct`: supported directly by files or user-provided facts.
- `Inferred`: derived from evidence but not directly confirmed.
- `Assumed`: explicit working premise.
- `Unverified`: no source yet; verify, mark, or remove before final decisions.

Use `Verify first: Yes` for claims that humans or future agents should check
before relying on them.

Also confirm that proposed changes are labeled as proposed until approved,
diagrams match the written map, SVG artifacts match their Mermaid source, and
documentation is lean enough to maintain.

Also confirm:

- Selected pass level is stated.
- Inspection scope is justified.
- Skipped areas are named when relevant.
- Discipline labels and risks are preserved.
- Any pass promotion has a trigger, evidence, and risk statement.
- Output stays within artifact budget, or the over-budget reason is stated.
- Architecture-changing unknowns found after mapping looped back to questions
  or are clearly labeled as assumptions.
- Every finding has evidence, impact, severity, and confidence. Findings that
  rest on taste, missing evidence, or hypothetical concerns are removed.

## Step 9: Report

In Scan Mode, the compact architecture note from Step 5 is the report. Do not
add the full structure below on top of it.

For Focus Mode and Full Mode, report in two layers per the Reporting
Sequence Rule (Rule 19): reader priority first, process order second.

Layer 1 — Synthesis (always required, 3-5 lines, opens the report):

- Critical/High findings, each with its evidence reference
- The primary recommendation

If no Critical/High findings exist, state that plainly and lead with the
primary recommendation instead.

Layer 2 — Detail on demand (unchanged content, give only when the user asks
for detail or a checkpoint gate requires it in writing):

1. What was inspected
2. Selected pass level and why
3. Confirmed architecture facts
4. Reasonable inferences
5. Findings: debt, convention drift, boundary violations, flow conflicts
   (full list, including Medium/Low)
6. Proposed changes and assumptions, when changes are suggested
7. Open questions
8. Risks or unclear boundaries
9. Decisions requiring approval
10. Validation gate answers
11. Documentation created or updated
12. Recommended next steps

## Rule References

Load these files when deeper guidance is needed:

- `rules/inspection-rules.md`
- `rules/question-rules.md`
- `rules/assessment-rules.md`
- `rules/documentation-rules.md`
- `rules/diagram-rules.md`
- `rules/anti-overengineering-rules.md`
- `rules/agent-handoff-rules.md`
- `docs/anti-patterns.md`

## Philosophy Reference

Keep operational work concise.

Read `docs/philosophy.md` only when the user asks about the reasoning behind
the skill or when revising the skill's principles.
