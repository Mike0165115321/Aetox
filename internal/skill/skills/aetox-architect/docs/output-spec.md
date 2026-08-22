# Output Spec

This file defines the expected shape of architecture outputs.

## Required Separation

Every architecture output should separate the categories that apply:

- Confirmed architecture
- Confirmed facts
- Reasonable inferences
- Findings (debt, convention drift, boundary violations, flow conflicts)
- Proposed changes
- Assumptions
- Open questions
- Risks
- Decisions made
- Decisions requiring approval

Do not mix these categories in a single undifferentiated summary.

## Architecture Status Labels

Use these labels consistently:

- `Confirmed architecture`: Architecture observed in files, docs, or explicit
  user-provided system context.
- `Confirmed`: Supported by inspected files or explicit user-provided facts.
- `Inferred`: Reasonable from available evidence, but not directly confirmed.
- `Proposed`: Suggested architecture for review, not approved or implemented.
- `Assumed`: Temporary premise used to make progress.
- `Unknown`: Important information not yet available.
- `Requires approval`: A decision the user or owner must accept before work.

## Evidence Strength

Evidence strength describes how strongly a claim is supported. It does not
replace status labels.

- `Direct`: Supported directly by inspected files, existing docs, or explicit
  user-provided facts.
- `Inferred`: Derived from available evidence, but not directly confirmed.
- `Assumed`: Explicit working premise used to make progress.
- `Unverified`: No source yet. Verify, mark clearly, or remove before final
  architecture decisions.

Use `Verify first: Yes` for important claims that humans or future agents
should check before relying on them.

## Preferred Formats

Use Markdown for primary documentation.

Use Mermaid only when requested, required for handoff, or needed to clarify
cross-module relationships:

- `flowchart`
- `sequenceDiagram`
- `classDiagram`
- C4-style flowcharts using Mermaid nodes

SVG may be used as a visual artifact for presentation or review when a complex
architecture needs a clearer visual export. SVG must not replace Markdown and
Mermaid as the editable source of truth.

## Minimum Architecture Output

Use the smallest output that still protects architectural understanding.

Use pass levels to right-size output:

- Scan Mode: compact architecture note for small, bounded, exploratory, or
  low-risk work.
- Focus Mode: scoped notes or maps for one module, workflow, subsystem, or
  clear boundary.
- Full Mode: full output package for whole-system mapping, future-agent
  handoff, unclear ownership, 3+ interacting modules, persistence,
  integrations, payment, authentication, security, deployment, or major
  workflow changes.

Start with the smallest safe pass and promote only when scope, evidence, risk,
or handoff needs require it.

If the task has no architecture impact, output only:

- `No architecture pass required`
- Reason
- Uncertainty that would change the decision, or `None identified`
- Recommended next step

## Artifact Budget

- Scan Mode: one compact architecture note.
- Focus Mode: one to three artifacts.
- Full Mode: full output package only when scope, risk, or handoff requires it.

If output exceeds the budget, state why.

## Compact Architecture Note

Use this shape for Scan Mode, matching `SKILL.md` Step 5:

- Pass level and reason
- Scope
- Evidence checked
- Skipped areas, or `None identified`
- Confirmed facts
- Inferences or assumptions, or `None identified`
- Findings, or `None identified`
- Open questions, or `None identified`
- Risks, or `None identified`
- Safe next actions

For a small existing system, the minimum useful output is usually:

- Architecture overview
- Module map
- Open questions
- AI agent notes

For larger existing systems, add:

- System boundary
- Data flow
- Workflow map
- File responsibility map
- Debt register
- Risk register
- Decision records

Findings follow the shape in `rules/assessment-rules.md`: observation,
evidence, impact, severity, confidence, and the smallest safe correction
marked as `Proposed` and `Requires approval`.

Use larger output packages when the work has persistence or database schema
changes, external integrations, payment or billing logic, authentication,
authorization, security boundaries, deployment or infrastructure changes, major
workflow changes, unclear ownership or module boundaries, 3+ interacting
modules, or explicit future-agent handoff.

## SVG Visual Artifact

When an output package benefits from SVG, include the generated artifact beside
the Mermaid source:

```txt
SVG Visual Artifact:
  Available at `diagram.svg`. Mermaid remains the editable source of truth.
```

Do not create an empty placeholder file. Keep generated artifacts beside the
Mermaid source.

## Traceability

Prefer references to files or directories when making claims.

Before reporting, run the three-question validation gate:

1. Does every important claim have a source?
2. Does the final scope match the intake scope?
3. Does the handoff include unknowns and safe next actions?

If a claim has no source, mark it as `Unverified`, `Assumed`, or remove it.
If scope expanded, state what changed and whether it is approved.
If no unknowns or safe next actions are found, write `None identified`.

Also state the selected pass level, justify the inspection scope, name skipped
areas when relevant, and preserve discipline labels and risks.

If the pass level was promoted, state the trigger, evidence, and risk of
staying in the smaller pass.

If mapping or validation exposed an architecture-changing unknown, state
whether the flow looped back to questions or proceeded with a labeled
assumption.

Example:

```txt
Confirmed:
  API handlers are located under `src/app/api/`.
Inference:
  The application likely uses Next.js App Router because `src/app/` and
  `next.config.*` are present.
Open question:
  Which deployment platform is production?
```
