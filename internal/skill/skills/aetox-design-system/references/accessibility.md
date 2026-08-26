# Accessibility

Consolidates and extends what was previously scattered across other reference files. Keyboard, screen-reader, and contrast concerns belong here; `states-and-variants.md` links to this file rather than re-explaining it.

## The floor, restated as one line

Every interactive state has to be distinguishable by **more than color**, every interactive element has to be reachable and operable **by keyboard alone**, and every non-text cue that carries meaning needs a text equivalent a screen reader can announce.

## Keyboard and focus

| Pattern | Rule |
|---|---|
| Focus indicator | Always visible on `:focus-visible`; never `outline: none` without a replacement |
| Focus indicator in forced-colors / high-contrast OS modes | Use the `outline` property specifically, it is the one focus technique that survives a forced-colors override; a box-shadow-only focus ring can vanish there |
| Tab order | Matches visual order; a component whose DOM order and visual order diverge needs `tabindex` correction, not a CSS-only fix |
| Composite widgets (tabs, comboboxes, date pickers, carousels) | Roving `tabindex`, one stop for the whole widget, arrow keys move inside it, not one tab-stop per item |

## ARIA states, matched to the state table

| Visual state (from `states-and-variants.md`) | ARIA |
|---|---|
| disabled | `disabled` attribute for native form controls; `aria-disabled="true"` where a real `disabled` attribute isn't available |
| loading | `aria-busy="true"` plus a visually-hidden live-region string naming what's loading, never a bare spinner with nothing for a screen reader to announce |
| error | `aria-invalid="true"` on the field, `aria-describedby` pointing at the error text, error text itself in a `role="alert"` region |
| selected | `aria-pressed` (toggle) or `aria-selected` (item in a set), matched to which semantic the component actually is |

## Verifying, not just designing

A design-token system with correct-looking values can still fail in practice, the same token can compute to a passing contrast ratio in one component and a failing one in another once real content and real states combine. Where this system's own output can be checked programmatically (a generated page's computed styles are inspectable), check the *rendered* contrast of interactive elements across their real states, not just the token table in isolation.

## Related

- `states-and-variants.md`, the state definitions this file adds keyboard/ARIA coverage to.
- `color-and-contrast.md`, the contrast math this file assumes.
