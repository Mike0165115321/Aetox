# Assessment Rules

Failure modes prevented:

The agent describes the system politely and stops — leaving visible debt,
convention drift, boundary violations, and flow conflicts unnamed. Or the
opposite: the agent floods the output with taste-based nitpicks to appear
thorough.

## Rules

1. Assess only what was mapped. No finding without inspected evidence.
2. Judge against two baselines only: the framework's documented conventions
   and the project's own dominant patterns. Personal style is not a baseline.
3. Establish the project's dominant pattern before calling something drift.
   One file doing X while nine do Y is drift; five doing X and five doing Y is
   an open question about which convention the owner wants.
4. Trace flows before judging them. A flow conflict claim requires following
   the actual path from entry point to data, not inferring from file names.
5. State impact in terms of this system: what breaks, what becomes unsafe to
   change, what the next agent will get wrong. "Not best practice" is not an
   impact.
6. Assign severity honestly. Reserve `Critical` for correctness breaks and
   self-contradicting flows. Most debt is `Medium`.
7. Mark confidence. A finding from file structure alone is `Inferred` and
   usually `Verify first: Yes`; a finding from read source is `Direct`.
8. Recommend the smallest safe correction as a proposed change requiring
   approval. Do not design the full fix inside a finding.
9. Point each recommendation toward the structure that stops the debt from
   recurring — a clear boundary, a single source of truth, the framework's own
   mechanism — not just toward deleting the symptom.
10. Never soften a supported finding to be agreeable, and never invent a
    finding to fill a section. `None identified` is a valid assessment.

## Assessment Dimensions

- Architecture debt: duplicated responsibilities, hidden coupling, permanent
  workarounds, missing quality gates.
- Separation of concerns: mixed responsibilities inside one module or layer,
  leaking boundaries, shared mutable state.
- Framework convention drift: fighting documented lifecycle, routing, data,
  or state mechanisms; reimplementing what the framework provides.
- Project convention drift: breaking the codebase's own dominant naming,
  layout, error-handling, or data-access patterns.
- Flow conflicts: two sources of truth for one state, circular dependencies,
  dead paths, side effects crossing boundaries, flows bypassing declared
  layers.

## Finding Shape

Every finding records:

- What was observed
- Evidence (files or observed signals)
- Impact on this system
- Severity: `Critical`, `High`, `Medium`, or `Low`
- Confidence: `Direct` or `Inferred`, plus `Verify first: Yes` when needed
- Smallest safe correction, marked `Proposed` and `Requires approval`

## Not Findings

- Style or naming taste with no convention behind it.
- Working code that is merely unfamiliar.
- Hypothetical scale problems with no evidence of present impact.
- Generic best-practice advice detached from inspected files.

## Depth By Pass Level

- Scan Mode: report only findings that inspection already surfaced.
- Focus Mode: assess the scoped module's boundaries, conventions, and flows.
- Full Mode: assess systematically across every mapped area.

Do not promote the pass level just to hunt for more findings. If evidence
suggests deeper problems outside the current scope, record that as an open
question or recommended next inspection instead.

## Graph-Derived Signals

When codebase graph data is available, its signals feed into the same
dimensions, finding shape, and confidence taxonomy above — no parallel
system. Graph does not pre-assign severity, bypass `Verify first: Yes` for
heuristic results, or turn fan-in outliers into findings without inspected
impact. When graph data and a file read disagree, the file read wins.
