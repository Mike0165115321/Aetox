# Agent Handoff Rules

Failure mode prevented: Future AI agents repeat inspection work, miss uncertainty, or act on stale assumptions.

## Rules

1. Write handoff notes for future agents after architecture inspection or documentation work.
2. Include what was inspected.
3. Include confirmed facts and inferences separately.
4. Include files or directories that matter most.
5. Include open questions and risks.
6. Include safe next actions.
7. Include actions the next agent should avoid until questions are answered.
8. Keep handoff notes short enough to read at session start.
9. Write `None identified` when no unknowns, risks, or open questions were
   found.

## Handoff Standard

A good handoff note lets the next agent understand:

- Current system shape
- Current documentation state
- Known unknowns
- Safe boundaries for future edits

## Minimum Bar

Every handoff must include:

- What was inspected or provided
- Current scope
- Confirmed facts
- Inferences
- Assumptions or proposed items
- Unknowns, or `None identified`
- Risks, or `None identified`
- Safe next actions
- Actions to avoid until clarified
