# Reference: Toolkit — concrete swap-ins

Positive alternatives to reach for instead of the defaults: font pairings, palettes,
scales, icon choices, and aesthetic stances.

> **Read this first — the anti-monoculture rule.** These are a **menu to calibrate
> against, not a default to apply.** If you always pick option 1, you've only traded the
> purple-Inter monoculture for a new one. In testing, unguided use of this file made ~8/10
> outputs converge on **warm cream paper + a terracotta/clay accent + Fraunces** — that
> combination is now *itself* a tell. So:
>
> 1. **Treat the whole second-order kit as banned-by-default**, exactly like purple/Inter:
>    **warm cream/dark paper + a high-contrast serif (Fraunces/Playfair) + an earthy accent
>    (terracotta/clay/ember/radish) + mono micro-labels + a corner page-counter + a "colored
>    last word" headline + a "sample data" disclaimer.** Any two of these together is a smell;
>    the full set is the current tell. Only use warm-editorial if the brief genuinely calls for
>    it *and* nothing else fits — and then vary the rest.
> 2. **Derive from the brief first.** Let the specific product, audience, and emotional
>    tone pick the direction. Cold, loud, mono, dark, maximal, and neutral are all on the
>    table — reach past the first warm-minimal instinct.
> 3. **Vary deliberately.** If you built something terracotta/serif last time, go somewhere
>    else this time. Feel free to derive fresh hexes/faces; the exact values below matter
>    less than *deciding on purpose*. These are altitude calibration, not a shopping list.
> 4. **Making a set? Use a divergence matrix.** Before building multiple artifacts, assign
>    each one a *different* row on the primary axes so the set spans the space instead of
>    collapsing to a point. No two should share **ground family + display-face family**:
>
>    | Artifact | Ground | Display family | Accent | Stance |
>    |---|---|---|---|---|
>    | A | cool near-white | geometric grotesk | cobalt | Swiss |
>    | B | dark graphite | neutral grotesk + mono | amber | ops/technical |
>    | C | saturated field | bold condensed/display | cream-on-color | poster |
>    | D | warm newsprint | quirky display grotesque | duotone | riso/print |
>    | E | white | humanist sans | one signal color | data/editorial |
>
>    Fill it from the *actual* briefs; the point is the spread, not these exact cells. If a
>    serif truly fits one piece, use a *different* serif than any tutorial default, and don't
>    let it spread to the others.

## Typography — pick a real pairing (highest leverage)

Never the system stack or Inter/Geist/Poppins used flat. Pick a **display face for
headings + a distinct, readable body face.** Some pairings with personality:

| Mood | Display / headings | Body / UI | Reads as |
|---|---|---|---|
| Editorial, trustworthy | Fraunces or Playfair Display | Inter Tight / Source Serif | magazine, considered |
| Modern, sharp product | Bricolage Grotesque or Clash Display | IBM Plex Sans | opinionated startup |
| Warm, human | Instrument Serif or Gambetta | Work Sans / Public Sans | approachable, editorial |
| Technical, precise | Space Grotesk (headings only) | JetBrains Mono / Geist Mono | engineering, exact |
| Bold, confident | Archivo / Anton (tight) | Inter / Satoshi | poster-like, punchy |
| Classic, premium | GT-Sectra-like serif / Libre Caslon | Söhne-like grotesk / Karla | luxury, established |

Rules: self-host or load deliberately; set tracking **per role**, not blanket negative
letter-spacing; use a real modular scale (below), not `clamp()` on everything; **sentence
case** for headings. One display + one body is enough — resist a third face.

## Color — decide a dominant + one ownable accent

Method: choose **one dominant color** that carries the mood and **one accent** that is
slightly *off* from the obvious — a hue no averaging model lands on. Make color
**functional** (it signals meaning), and name tokens semantically (`--action`, `--surface`,
`--emphasis`), never `--purple`. Avoid: Tailwind `slate` as-is, `#6366f1/#8b5cf6/#818cf8`,
`sky-400 → indigo-400`, and the generic green/amber/red trio at matching opacity.

Example palettes that are *not* the default (each commits to a point of view):

- **Warm editorial:** ink `#1a1a17`, paper `#f7f3ec`, accent terracotta `#c4552f`,
  secondary olive `#6b6f3f`. (warmth + restraint)
- **Deep & confident:** near-black `#0c0d0f`, bone `#e8e6e1`, accent electric-lime
  `#c3f53c` used sparingly, muted steel `#5b6470`. (bold, tech-forward, not neon-purple)
- **Calm clinical:** off-white `#fbfbfa`, ink `#20242a`, accent muted teal-not-cyan
  `#2f7e78`, sand `#d8cbb4`. (trustworthy, healthcare/finance)
- **Vivid brand:** cream `#fff8ef`, ink `#161311`, accent cobalt `#2b4cff` OR persimmon
  `#ff5a36` (pick one), charcoal support `#3a3733`. (energetic consumer)
- **Monochrome + one pop:** true grayscale ramp, plus a single saturated accent used on
  <5% of the surface. Restraint reads as confidence.

Contrast and accessibility still matter — verify text contrast regardless of palette.

## Scales — hierarchy, not uniformity

- **Radius hierarchy:** small controls `4px`, cards `8–10px`, large containers `16px`.
  Not one radius everywhere; not 24px on a small chip.
- **Spacing:** an 8px base scale (4, 8, 12, 16, 24, 32, 48, 64). Vary padding by
  importance — the primary panel gets more room than a secondary one.
- **Type scale:** a real ratio (e.g. 1.25 major-third): 14 / 16 / 20 / 25 / 31 / 39 / 49.
  Pick sizes from the scale; don't emit `clamp(min, vw, max)` on every element.
- **Elevation:** 2–3 defined shadow levels tied to real layering intent. Not one
  0.1-opacity shadow on everything, and not a hairline border *and* a shadow on the same card.

## Icons — real set, chosen with intent

Never emoji-as-icons, never CSS rotated-square bullets. Choose ONE set and a consistent
weight/style: e.g. Phosphor (has weights), Lucide (if you restyle stroke/size so it's not
the shadcn default look), Tabler, Radix, or Heroicons. Use icons **sparingly and with
meaning** — not one centered above every heading. Better still for hero/product moments:
real screenshots or a custom illustration over any icon.

## Motion — a small, purposeful kit

- Transition **specific properties** (`transform`, `opacity`, `background-color`) with
  tuned easing (`cubic-bezier(0.2, 0, 0, 1)` for entrances, faster for exits). Never
  `transition: all`.
- Give hover/focus/active **real, distinct** states that reflect the affordance.
- Motion only to (1) signal a state change, (2) direct attention at a moment, or (3)
  express brand character. Delete decorative uniform fade-ins and the pulsing "breathing orb".

## Aesthetic stances (pick one and commit)

Taking an ownable position is the opposite of averaging. Choose per brief:
- **Swiss / editorial:** strong grid, generous whitespace, one accent, big type, few effects.
- **Neo-brutalist:** visible structure, thick borders, blunt oversized type, high-contrast
  clashing color — *only if the imperfection is clearly intentional*.
- **Warm-minimal:** paper tones, serif display, soft real photography, quiet motion.
- **Technical-precise:** mono type, dense data, exact alignment, restrained color (Stripe-like).
- **Maximal-expressive:** a signature color and shape language used boldly and consistently.

Whatever the stance: **restraint + intention + specificity.** Fewer choices, each
defensible. If you can't say why a choice was made, it's probably the default.
