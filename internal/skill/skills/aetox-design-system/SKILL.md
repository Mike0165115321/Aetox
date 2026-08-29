---
name: aetox-design-system
description: ระบบดีไซน์โทเคนสามชั้น (primitive → semantic → component) และตารางตัดสินใจของสไลด์ - สเกลระยะห่างกับตัวอักษร สเปกคอมโพเนนต์และสถานะของมัน การต่อกับ Tailwind และตาราง CSV ที่บอกว่าโครงเด็คแบบไหน เลย์เอาต์ไหน กราฟไหน คำโปรยแบบไหน เข้ากับงานตรงหน้า ใช้ตอนอยากให้งานทุกชิ้นหน้าตาเป็นชุดเดียวกันแทนที่จะสวยทีละใบ
source: https://github.com/claudekit (design-system)
license: MIT
copyright: Copyright (c) claudekit contributors
---

# Design System

Token architecture, component specifications, systematic design, slide generation.

## When to Use

- Design token creation
- Component state definitions
- CSS variable systems
- Spacing/typography scales
- Design-to-code handoff
- Tailwind theme configuration
- **Slide/presentation generation**

## Token Architecture

Load: `references/token-architecture.md`

### Three-Layer Structure

```
Primitive (raw values)
       ↓
Semantic (purpose aliases)
       ↓
Component (component-specific)
```

**Example:**
```css
/* Primitive */
--color-blue-600: #2563EB;

/* Semantic */
--color-primary: var(--color-blue-600);

/* Component */
--button-bg: var(--color-primary);
```

## Quick Start

There is no generator to run here and none is needed: writing the `:root` block
is the work, and `write` does it. Read `references/token-architecture.md` for
the three layers, then emit the CSS variables yourself.

Validate by reading, not by running: a hard-coded `#3b82f6` or `16px` anywhere
outside the primitive layer is the defect this system exists to prevent, and
`grep` finds those in one call.

## References

| Topic | File |
|-------|------|
| Token Architecture | `references/token-architecture.md` |
| Primitive Tokens | `references/primitive-tokens.md` |
| Semantic Tokens | `references/semantic-tokens.md` |
| Component Tokens | `references/component-tokens.md` |
| Component Specs | `references/component-specs.md` |
| States & Variants | `references/states-and-variants.md` |
| Tailwind Integration | `references/tailwind-integration.md` |
| Typography & fonts | `references/typography.md` |
| Color & contrast, light/dark | `references/color-and-contrast.md` |
| Accessibility (keyboard, ARIA) | `references/accessibility.md` |
| Motion for generated work | `references/motion.md` |
| Avoiding the generic AI look | `references/anti-generic-look.md` |
| Dense UI & dashboards | `references/dense-ui.md` |

## Component Spec Pattern

| Property | Default | Hover | Active | Disabled |
|----------|---------|-------|--------|----------|
| Background | primary | primary-dark | primary-darker | muted |
| Text | white | white | white | muted-fg |
| Border | none | none | none | muted-border |
| Shadow | sm | md | none | none |

## Templates

| Template | Purpose |
|----------|---------|
| `design-tokens-starter.json` | Starter JSON with three-layer structure |

## Integration

**With `aetox-brand`:** extract primitives from brand colors/typography.

No scripts ship with this skill: a bundled skill has no folder on disk (it
lives inside the binary), so there is nothing for `shell` to execute even if
one were written here. A script only works for a skill installed on a real
disk, under `~/.aetox/skills`.

## Slide System

Brand-compliant presentations: design tokens, a contextual decision system, and
charts drawn by whatever already travels inside the file.

### There is no stylesheet to import

A deck in Aetox is one self-contained `.html` file, nothing is linked in beside
it, so every token and every `@keyframes` a slide uses is written into that
file. The rows below name what a slide should *be*; you write the CSS that makes
it so. `animation_class` in particular is the entrance a layout wants, not a
class waiting in a library: read it as the intent, spell it however the deck
spells everything else, and follow the resting-state rule in `aetox-slides` so
it survives the export.

The export-safe spellings live in one place, the entrance kit in `aetox-slides`.
Map a row's intent onto it rather than inventing a name: `fade-up` and `stagger`
are `rise` and a staggered `rise`; `scale` and `stagger-scale` are `grow`, which
starts at `.96` and never `0`; `count` is a number whose final value is its DOM
text with the tally run over it; `chart` is a line drawing itself on
`stroke-dashoffset`. `ken-burns` and `pulse` are the exception: ambient loops the
export freezes on whatever frame it lands, so use them for screen life only,
never to carry a point a `.pdf` reader has to receive.

### Reading the decision tables

The tables below are the knowledge; there is no search tool to run over them.
Open the one that answers the question you actually have, `skill_view` with the
file's path, and read the rows. Each file is small enough to read whole, and
reading it whole is how you see the row you would not have thought to search
for.

For the deck's own anatomy, what a slide element is, what the slides room can
page through, what survives an export, read the `aetox-slides` skill. That is
where those facts live; this skill decides what goes *on* the slides.

### Decision System CSVs

| File | Purpose |
|------|---------|
| `data/slide-strategies.csv` | 15 deck structures + emotion arcs + sparkline beats |
| `data/slide-layouts.csv` | 25 layouts + component variants + animations |
| `data/slide-layout-logic.csv` | Goal → Layout + break_pattern flag |
| `data/slide-typography.csv` | Content type → Typography scale |
| `data/slide-color-logic.csv` | Emotion → Color treatment |
| `data/slide-backgrounds.csv` | Slide type → Image category (Pexels/Unsplash) |
| `data/slide-copy.csv` | 25 copywriting formulas (PAS, AIDA, FAB) |
| `data/slide-charts.csv` | 25 chart types, each with a CSS/SVG route and a library route |

These say which layout to reach for and what it is made of. They do not say what
it looks like in markup: `slide-layouts.csv` describes each one in a
`css_structure` column holding a single line of CSS, and everything else about
the composition was left to be invented at write time, every time. That is why
decks built from these same tables came out differently on every run, and mostly
came out as the skeleton with different words in it.

**The markup lives in `aetox-slide-templates`**, one file per layout, ready to paste.
Deciding and copying are two questions, so they are two skills: read the row
here, then open that layout's file there.

### Contextual Decision Flow

```
1. Parse goal/context
        ↓
2. Search slide-strategies.csv → Get strategy + emotion beats
        ↓
3. For each slide:
   a. Query slide-layout-logic.csv → layout + break_pattern
   a2. Open that layout's file in aetox-slide-templates → real markup,
       rather than inventing the composition again
   b. Query slide-typography.csv → type scale
   c. Query slide-color-logic.csv → color treatment
   d. Query slide-backgrounds.csv → image if needed
   e. Apply an entrance from the kit in aetox-slides (spelled in the deck; there
      is no slide-animations.css to import)
        ↓
4. Generate HTML with design tokens
        ↓
5. Validate by reading (see "Checking a finished deck" below), there is no
   slide-token-validator.py to run
```

### Pattern Breaking (Duarte Sparkline)

Premium decks alternate between emotions for engagement:
```
"What Is" (frustration) ↔ "What Could Be" (hope)
```

System calculates pattern breaks at 1/3 and 2/3 positions.

### How a deck ends

**The last two slides are a synthesis and a close, and a deck that stops on its
final step is unfinished.**

Every structure in `slide-strategies.csv` says so already and it is easy to read
past: `1.Title ... 10.Ask`, `... 8.Call to Action 9.Q&A`, `... 9.Synthesis
10.Resources 11.Q&A`. None of the fifteen ends on the last item of its own
middle. A deck is an argument, and an argument that stops when it runs out of
steps has not been concluded, only abandoned.

- **The synthesis** says what the middle added up to, in the deck's own terms.
  Not a list of the slide titles again. For a how-to it is the whole path in one
  view, so somebody who followed along can see the shape of what they just did;
  for a pitch it is the case in one line.
- **The close** is the one thing to do, remember, or decide next. A command to
  run, a link to keep, an ask, a line worth quoting.

Not a "thank you for your attention" slide, which is the failure the close
replaces rather than a smaller version of it. The `aetox-anti-slop` skill bans
that slide in its presentation notes, and that is where this rule was
half-written: a ban with no requirement beside it produces a deck that correctly
avoids the empty ending and then simply has none. On 29 ส.ค. a seven-slide guide
to SSH keys ran "สร้างคีย์ → เปิดดู public key → ใส่ key บน GitHub → ทดสอบ →
พร้อม push แล้ว" and stopped there, on a step.

A short deck does not get an exemption. Two slides of the seven is a heavier
proportion than two of twenty, and that is the right proportion: the shorter the
deck, the more of it the ending is.

### Checking a finished deck before calling it done

**Check it by reading it. There is no photograph step here.**

A deck is one HTML file on disk and `read` returns all of it in one call, so
every question worth asking before calling it done is answered exactly, in the
source, for the price of one tool call:

- **Line budgets.** Count characters against the layout's stated max-width per
  line. A `slide-layouts.csv` row saying "short headline, 2-line max body" is a
  budget to check text against, not a suggestion.
- **The fold.** A slide is 1280x720 with `overflow:hidden`, so content past
  720px is cut in silence, in the room and in the export both. Count the lines
  to the fold yourself. A slide that has to be squeezed to fit is two slides.
- **Charts.** Re-read every `css_implementation` route against what the file
  actually carries. A chart drawn by a library fetched from a CDN is the
  specific failure the export warns about elsewhere in this document, and it is
  found by reading, never by looking.
- **Selectors.** A rule scoped to the wrong element, a `z-index` on a box that
  is not positioned, a stacking context nobody meant to create. All of it is in
  the text.
- **The ending.** Read the last two slides and ask what the deck concluded. If
  the answer is the last step of the middle, it has no ending. See "How a deck
  ends" above.

Taking a picture of the deck was written into this document on 26 ส.ค. and is
taken back out on 29 ส.ค., because of what it cost when a model followed it. A
run took 22 screenshots of a 7-slide deck by scrolling one screen at a time,
narrating that each slide "renders correctly", and re-sent those pictures 620
times between them across the turn. It was reading a 17 KB file by photographing
it, and everything it found was in the source.

The `browser` tool is still there and still takes pictures. Nothing here asks
for one, and "I have not looked at it" is not a reason to withhold a deck that
reads correctly.

### What every slide owes the room

1. **Its own tokens.** A deck is one self-contained file, so the CSS variables
   go in its `<style>` block. There is nothing to import.
2. **Variables rather than literal colours**, so changing the accent is one edit
   and not twenty.
3. **A chart drawn by something already inside the file.** The export prints
   without waiting for a third-party host, so a chart drawn by a library fetched
   from a CDN can come out blank. Carry the library into the file, or draw the
   chart in CSS or SVG. The `css_implementation` column names a route of each
   kind for most rows, either one satisfies this, and the choice is yours.
4. **Nothing of its own for navigation.** The room already pages, presents
   full-screen and exports. A deck that brings its own controls puts two sets in
   one frame and only the room's survives an export.
5. **Left-aligned paragraphs and lists.** Centre titles and single statements;
   centring a paragraph costs the reader the left edge they scan down.

### Token Compliance

```css
/* CORRECT - uses token */
background: var(--slide-bg);
color: var(--color-primary);
font-family: var(--typography-font-heading);

/* WRONG - hardcoded */
background: #0D0D0D;
color: #FF6B6B;
font-family: 'Space Grotesk';
```


## Best Practices

1. Never use raw hex in components - always reference tokens
2. Semantic layer enables theme switching (light/dark)
3. Component tokens enable per-component customization
4. Use HSL format for opacity control
5. Document every token's purpose
6. **Slides must import design-tokens.css and use var() exclusively**
