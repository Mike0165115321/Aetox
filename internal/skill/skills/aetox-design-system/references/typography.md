# Typography

Font choice and type-scale decisions, the sharpest gap in this design system until now, and the one most likely to make a piece of work look unfinished even when every token is technically correct.

## Two questions before picking a typeface

1. **What is it for?** Headline/button text is read for *a moment*, optimize for personality and presence. Body text is read *to live with*, optimize for endurance over many minutes, not for how it looks in a screenshot. Pick different faces for these two jobs; the same face rarely serves both well.
2. **What script does it actually have to render?** A pairing that only covers Latin glyphs and gets handed Thai text at runtime is not a font decision that was made, it's one that was skipped.

## Type roles and scale

| Role | Weight range | Notes |
|---|---|---|
| Display / headline | 600–900 | Used sparingly, this is the face with character |
| Subhead | 500–600 | |
| Body | 400 | Endurance, not personality |
| Caption / data | 300–400, often a mono face | `font-variant-numeric: tabular-nums` wherever digits line up in a column |

Default modular-scale ratio: **1.2 (Minor Third)** for dense UI, **1.25–1.333** for editorial/marketing pieces where more contrast between sizes reads as more considered. A scale ratio is a starting point to justify, not a rule to skip picking, see `aetox-design`'s "think twice before drawing" step for how to check a choice isn't just the reflexive default.

Avoid on body text, named explicitly because they show up often in AI-generated output and read as unconsidered: Inter and Roboto as an unexamined default pairing, and any strongly display-flavored face (script, heavily condensed, all-caps-only) set at body size.

## Thai text specifically

Thai is not an edge case for Aetox, treat it as the primary script, not a fallback.

- **Line-height floor: 1.55.** Optimal range 1.6–1.8 for body text, 1.2–1.35 for headings. Below the floor, stacked tone marks and vowels (as in น้ำ, ที่) start colliding with the line above.
- **Never apply `letter-spacing` to Thai text.** It breaks the visual connection between a consonant and its stacked marks. Scope any tracking adjustment to `:lang(en)` specifically so it never reaches Thai runs in mixed content.
- **Pick looped (มีหัว) or loopless (ไม่มีหัว) consistently within one piece, not mixed.** Loopless reads better at heading sizes; looped is the safer default for body text.
- **Fallback chain:** `'Noto Sans Thai', 'Sukhumvit Set', 'Thonburi', system-ui`, always end on a system fallback, never let a Thai run fall through to a Latin-only stack.
- **Never `word-break: break-all` on Thai.** Thai has no spaces between words; a naive break-all cuts mid-word. Use `Intl.Segmenter('th', {granularity: 'word'})` where word-boundary logic is actually needed, and otherwise let Thai text wrap at any character (its normal behavior) rather than forcing a Latin word-break rule onto it.
- **Display faces to avoid at body size** (decorative/handwriting-styled Thai faces that break down below headline size): Pattaya, Charmonman, Charm, Srisakdi, Itim, Mali, Sriracha, Chonburi.
- Sarabun is the safest general-purpose Thai body choice when nothing more specific is called for.

## Two techniques worth applying deliberately

- **Tune line-height inversely with column width**, not as one fixed constant, a narrower measure needs *more* line-height to stay readable, a wide measure needs less.
- **When text sits on a dark surface, correct three things together**: line-height, tracking, and weight each move up one notch as a single bundled correction, not three separate ad hoc tweaks. Text on dark grounds needs more of all three to read as comfortably as the same text on light.

## Auditing existing type choices

Before calling a piece finished, check it against this list rather than eyeballing it:

- Is there a font-loading fallback that isn't a generic system font, for every custom face in use?
- Does every place digits appear in a column use tabular figures?
- Is there anywhere tracking was applied to non-Latin text?
- Does any block of body text sit below the Thai line-height floor?
- Is the same face doing both the headline and the body job?

## Related

- `aetox-th-locale`, Thai-specific data correctness (dates, IDs, addresses); this file is about how Thai *renders*, that skill is about whether Thai *content* is correct.
- `aetox-design`'s "think twice before drawing" step, for the genericness check to run before locking in a type pairing.
