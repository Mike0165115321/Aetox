# Anti-Overengineering Rules

Failure mode prevented:

The agent turns a small architecture task into a large framework, process, or
documentation system.

## Rules

1. Match output size to system complexity.
2. Do not introduce new abstractions without a specific problem.
3. Do not recommend architecture patterns just because they are common.
4. Do not create ADRs for obvious or temporary decisions.
5. Do not create separate files when one concise document is enough.
6. Do not convert every uncertainty into a blocking question.
7. Do not redesign existing structure unless redesign is requested or clearly justified.
8. Start with the smallest safe pass and promote only when scope, evidence,
   risk, or handoff needs require it.
9. Exit early when the task has no architecture impact.
10. Do not exceed the artifact budget without stating why.

## Pass Levels

Use:

- Scan Mode for small, bounded, exploratory, or low-risk work.
- Focus Mode for one module, one workflow, a small subsystem, or a clear
  boundary.
- Full Mode for whole-system mapping, future-agent handoff, unclear ownership,
  3+ interacting modules, persistence, integrations, payment, authentication,
  security, deployment, or major workflow changes.

If uncertain, inspect narrowly first. Record what was checked, stay in the
smallest safe pass unless a real trigger is found, and do not promote because
of uncertainty alone.

Promotion requires:

- A real trigger
- Evidence or an explicit user request
- A risk statement for staying in the smaller pass

Do not promote because the system merely appears complex.

## Lean Default

For Scan Mode, prefer a compact architecture note with:

- Pass level and reason
- Scope
- Evidence checked
- Skipped areas
- Confirmed facts
- Inferences or assumptions
- Open questions
- Risks
- Safe next actions

Add more only when the project has enough complexity to justify it.

## Artifact Budget

- Scan Mode: one compact architecture note.
- Focus Mode: one to three artifacts.
- Full Mode: full output package only when scope, risk, or handoff requires it.

If the task has no architecture impact, write `No architecture pass required`
and stop architecture documentation.

## Full Package Threshold

Use a full output package when the work involves:

- 3+ modules with meaningful interaction
- Unclear system boundaries
- Persistence changes
- External integrations
- Payment or billing logic
- Authentication, authorization, or security boundaries
- Deployment, infrastructure, or environment changes
- Major workflow or business flow changes
- Future-agent handoff

Otherwise, prefer a compact architecture note that still separates facts,
inferences, assumptions, unknowns, risks, and decisions.
