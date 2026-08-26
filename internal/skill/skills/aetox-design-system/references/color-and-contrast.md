# Color and Contrast

Color science and the light/dark/theme decision, kept separate from `tailwind-integration.md`, which is the implementation mechanics once a decision here is made.

## Contrast: measure it, don't eyeball it

WCAG 2.x contrast math (the `4.5:1` / `3:1` figures already used elsewhere in this system) is the widely-supported baseline. It is also a known-imperfect model, WCAG's formula misjudges some real color pairs, particularly among mid-tones. APCA (Advanced Perceptual Contrast Algorithm) is the more accurate successor where it's available in tooling, but WCAG numbers remain the safe default to *cite* since they're what's actually enforced by most external checkers.

A concrete fact worth carrying: only a small fraction of possible color pairs pass WCAG AA at 4.5:1 by chance, passing contrast is the result of a deliberate check, never an assumption that a pairing is "readable enough" because it looks fine on one screen.

## Tokens as derivation rules, not frozen values

The primitive → semantic → component chain already documented in `token-architecture.md` works best when semantic and component tokens are *formulas* referencing the primitive, not separately hand-picked values:

```css
--semantic-warning: var(--ref-red-600);
--component-button-hover-bg: color-mix(in oklch, var(--semantic-primary) 88%, black 12%);
```

Written this way, changing one primitive (a rebrand, a palette swap) recomputes every dependent token automatically. A hand-picked hover color has to be manually revisited every time the primitive changes underneath it, and in practice it usually isn't.

## Deciding light, dark, or system-synced

Not every product should default to the same answer:

| Product context | Default |
|---|---|
| Content-heavy, read for long stretches, general audience | Light default, dark available |
| Trading/monitoring/dense-data tools, technical audience | Dark default is often the *better* choice, not just an option |
| General SaaS / consumer app | System-synced (`prefers-color-scheme`), user override remembered |

The theming contract worth matching everywhere this system generates output: an explicit user choice always wins; failing that, the page reads `prefers-color-scheme` rather than defaulting to one mode. Two states, not three, there is no separate "system" toggle to build, because the un-set state already behaves that way.

## Dark mode is not light mode inverted

See `tailwind-integration.md` for the concrete numbers (contrast ceiling, base color, elevation-via-tint instead of shadow, reduced brand saturation), that file is where the implementation values live; this section exists so the *reason* isn't lost: a dark surface changes what "readable" and "comfortable" mean, it doesn't just recolor the same rules.

## What color alone must never carry

State (error, success, selected, disabled) needs a second signal, an icon, a label, a border weight, a position, never color by itself. This is already the rule in `states-and-variants.md`; it's restated here because color-and-contrast work is exactly where it's easiest to forget, since two colors *can* pass a contrast check against their background and still be indistinguishable from each other to a color-blind reader.

## Related

- `tailwind-integration.md`, the CSS/config implementation once a light/dark decision is made here.
- `accessibility.md`, where color-alone-carries-meaning becomes a concrete ARIA/markup rule.
