# Brief: aetox-ui-styling (our own, unbuilt)

Written 2026-08-26. A brief, not a skill yet — the thing to build later. Study
claudekit's `ui-styling` and `aesthetic` for ideas **only**: that repo ships no
LICENSE, so nothing may be copied. Write ours from scratch.

## Why it exists

The four skills adopted 2026-08-26 (`aetox-frontend-design`, `aetox-ui-design`,
`aetox-ux-review`, `aetox-anti-slop`) cover deciding the look, building it,
auditing it, and keeping it un-generic. What none of them owns is the **last
pass over a UI that already works but reads as plain** — the polish that makes it
feel considered rather than merely correct. That is what claudekit's `ui-styling`
(accessible shadcn/Tailwind styling, dark mode) and `aesthetic` (inspiration
analysis, micro-interactions, storytelling) were reaching for.

## What it should own (and what it must not duplicate)

Own:
- **Refinement of a working UI** — take a plain-but-functional screen and lift
  it: spacing rhythm, one considered accent, restraint, the Chanel "remove one
  thing" pass.
- **Micro-interaction polish as a finishing step** — the hover, the state
  transition, the loading moment, tied to Aetox's motion tokens (`DESIGN.md` §4).
- **Concrete styling recipes** — shadcn/ui + Tailwind component styling, dark
  mode done right — the level of "here is the class list," which the current
  skills describe in principle but do not spell.
- **Inspiration → taste** — reading a reference and naming *why* it works, then
  applying the principle (not the pixels).

Must NOT duplicate (or it becomes [[aetox-single-source-of-truth]] debt):
- Deciding the aesthetic direction from scratch → `aetox-frontend-design`.
- Tokens, theming, responsive, the implementation guides → `aetox-ui-design`.
- Auditing finished work against heuristics → `aetox-ux-review`.
- Catching the generic AI defaults → `aetox-anti-slop`.

So its remit is narrow: **the polish pass**, hung on the tokens and motion
Aetox already defines, not a second design system.

## Patterns worth stealing (from studying Hermes, 2026-08-26)

Hermes's skills are study-only (per-skill licenses vary), but two of its design
skills point at the right shape for ours — copy the *approach*, not the files:

- **A curated structural-reference pack.** Hermes's `popular-web-designs` ships
  ~54 real design systems (Stripe, Linear, Vercel) as HTML/CSS the agent anchors
  to instead of inventing an aesthetic per prompt. This is exactly
  [[aetox-owner-design-taste]] — "borrowed structure, never borrowed ornament."
  Our skill should carry a small pack of *structure* references (layouts, spacing
  rhythms, component skeletons drawn from real systems, described in our own
  words / rebuilt, not copied), so the polish pass starts from a real skeleton.
- **Design contracts + required states as gates.** Hermes's `anti-ui-slop`
  enforces that every UI handles its required states (empty / loading / error /
  success) and passes gates before it is called done. Fold this in as a checklist
  the polish pass runs: a screen is not finished until each state is designed.
  (This is a different job from `aetox-anti-slop`, which catches the *generic
  look*; this catches *missing states*.)

Also confirmed for the sibling office work: Hermes's `excel-author` frames a sheet
as an **auditable workbook of formulas**, not a values dump — the same lesson as
[[aetox-sheet-needs-own-pass]]. Whatever office/sheet skill we build should be
formula-first with an audit trail, and its docx/pptx/xlsx/pdf generation should
lean on the MIT/BSD libraries (python-docx, python-pptx, openpyxl, pypdf), never
on Anthropic's source-available wrappers.

## Open question to settle first

Is this a *new* skill, or a section inside `aetox-frontend-design`? Lean toward a
section unless the recipes (shadcn/Tailwind class lists, dark-mode patterns) and
the structural-reference pack grow big enough to be their own reference tree.
Decide by size, after drafting the recipes.
