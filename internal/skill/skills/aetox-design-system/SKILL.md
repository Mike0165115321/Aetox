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

## Component Spec Pattern

| Property | Default | Hover | Active | Disabled |
|----------|---------|-------|--------|----------|
| Background | primary | primary-dark | primary-darker | muted |
| Text | white | white | white | muted-fg |
| Border | none | none | none | muted-border |
| Shadow | sm | md | none | none |

## Scripts

| Script | Purpose |
|--------|---------|
| `generate-tokens.cjs` | Generate CSS from JSON token config |
| `validate-tokens.cjs` | Check for hardcoded values in code |
| `search-slides.py` | BM25 search + contextual recommendations |
| `slide-token-validator.py` | Validate slide HTML for token compliance |
| `fetch-background.py` | Fetch images from Pexels/Unsplash |

## Templates

| Template | Purpose |
|----------|---------|
| `design-tokens-starter.json` | Starter JSON with three-layer structure |

## Integration

**With brand:** Extract primitives from brand colors/typography
**With ui-styling:** Component tokens → Tailwind config

**Skill Dependencies:** brand, ui-styling
**Primary Agents:** ui-ux-designer, frontend-developer

## Slide System

Brand-compliant presentations: design tokens, a contextual decision system, and
charts drawn by whatever already travels inside the file.

### There is no stylesheet to import

A deck in Aetox is one self-contained `.html` file — nothing is linked in beside
it, so every token and every `@keyframes` a slide uses is written into that
file. The rows below name what a slide should *be*; you write the CSS that makes
it so. `animation_class` in particular is the entrance a layout wants, not a
class waiting in a library: read it as the intent, spell it however the deck
spells everything else, and follow the resting-state rule in `aetox-slides` so
it survives the export.

### Reading the decision tables

The tables below are the knowledge; there is no search tool to run over them.
Open the one that answers the question you actually have — `skill_view` with the
file's path — and read the rows. Each file is small enough to read whole, and
reading it whole is how you see the row you would not have thought to search
for.

For the deck's own anatomy — what a slide element is, what the slides room can
page through, what survives an export — read the `aetox-slides` skill. That is
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

### Contextual Decision Flow

```
1. Parse goal/context
        ↓
2. Search slide-strategies.csv → Get strategy + emotion beats
        ↓
3. For each slide:
   a. Query slide-layout-logic.csv → layout + break_pattern
   b. Query slide-typography.csv → type scale
   c. Query slide-color-logic.csv → color treatment
   d. Query slide-backgrounds.csv → image if needed
   e. Apply animation class from slide-animations.css
        ↓
4. Generate HTML with design tokens
        ↓
5. Validate with slide-token-validator.py
```

### Pattern Breaking (Duarte Sparkline)

Premium decks alternate between emotions for engagement:
```
"What Is" (frustration) ↔ "What Could Be" (hope)
```

System calculates pattern breaks at 1/3 and 2/3 positions.

### What every slide owes the room

1. **Its own tokens.** A deck is one self-contained file, so the CSS variables
   go in its `<style>` block. There is nothing to import.
2. **Variables rather than literal colours**, so changing the accent is one edit
   and not twenty.
3. **A chart drawn by something already inside the file.** The export prints
   without waiting for a third-party host, so a chart drawn by a library fetched
   from a CDN can come out blank. Carry the library into the file, or draw the
   chart in CSS or SVG. The `css_implementation` column names a route of each
   kind for most rows — either one satisfies this, and the choice is yours.
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


### Command

```bash
/slides:create "10-slide investor pitch for ClaudeKit Marketing"
```

## Best Practices

1. Never use raw hex in components - always reference tokens
2. Semantic layer enables theme switching (light/dark)
3. Component tokens enable per-component customization
4. Use HSL format for opacity control
5. Document every token's purpose
6. **Slides must import design-tokens.css and use var() exclusively**
