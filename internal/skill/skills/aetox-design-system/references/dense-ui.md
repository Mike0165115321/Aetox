# Dense UI and Dashboards

Layout discipline for screens that have to show a lot at once, a dashboard, an admin panel, a data-heavy artifact, where the failure mode isn't "too plain," it's "too much, unreadably."

## Density is a deliberate mode, not a default

| Mode | Touch target | Use when |
|---|---|---|
| Cozy | 44px | Touch input, or a screen with room to spare |
| Compact | 32px | Mouse/keyboard, moderate density |
| Condensed | 24px | Mouse/keyboard, maximum density (trading/monitoring-style screens) |

**Never mix density modes at the same hierarchy level.** A row of buttons at Compact next to a row of buttons at Condensed reads as inconsistent even when each row is internally correct.

## Hard numbers, not vibes

- Gap between grid cells: 16–24px minimum for a Cozy/Compact dashboard; a dense/Condensed one can go tighter, but stated deliberately, not by accident
- Widget count: 6–8 per screen for an executive/overview dashboard, not 20+; an operational/monitoring dashboard can run 8–15 for an audience that lives in it daily
- KPI count by audience: 3–5 headline metrics for an executive view, 8–15 for an operational one, audience decides the number, not available space
- Numerals: `font-variant-numeric: tabular-nums` wherever digits appear in a column, always

## Anti-patterns worth naming explicitly

- 3D charts of any kind
- A pie chart with more than 5 categories, see `data/slide-charts.csv` in this same skill for the chart-type-to-route table
- A full-page loading state for a dashboard that's mostly still usable while one widget loads, skeleton-load the individual widget instead
- Boxed KPI cards at high density, a `divide-x` separator row reads as less cluttered once density passes a threshold

## The value-density question

Before making a screen denser to fit more in, ask whether the added information is actually earning its space: value shown ÷ (time to find it + space it occupies). A screen that's visually sparse but takes three clicks to find the one number someone needs is not actually less dense than it looks, the density just moved from the screen to the interaction. Where that's the case, surface the 2–3 numbers that are actually acted on, and put the rest behind a deliberate "more" affordance rather than flattening everything onto one screen.

## Related

- `states-and-variants.md`, component states apply the same at any density.
