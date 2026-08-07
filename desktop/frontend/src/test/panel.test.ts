// A panel in an answer (internal/prompt's `panel` layer), from the outside.
//
// The third capability in this app found already working and never used: the
// chat renders sanitized markdown inside the app's own document, so a <div>
// with a style attribute lays out normally and var(--surface-panel) inside that
// attribute resolves against the live theme. Nothing here *enables* that. What
// these pin is that nothing may quietly take it away — and the failure would be
// silent, exactly as it was for drawings: a model doing what it was told, and
// answers that render as a wall of text or as raw markup.
import { describe, it, expect } from 'vitest'
import { renderMarkdown, renderStreamingMarkdown } from '../lib/markdown'

const panel = `<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(0,1fr));gap:12px">
<div style="border:1px solid var(--border-subtle);border-radius:12px;background:var(--surface-panel);padding:14px">
<div style="font-weight:600;color:var(--text-primary)">deck</div>
<div style="height:6px;border-radius:99px;background:var(--interactive);width:70%"></div>
</div>
</div>`

describe('a panel in an answer', () => {
  it('renders, layout and theme colours intact', () => {
    const out = renderMarkdown(panel)

    expect(out).toContain('grid-template-columns')
    // The whole reason a panel is written this way: the colour is the user's
    // theme, not a value the model guessed. A sanitizer that dropped style
    // attributes, or stripped var(), would leave the panel unpainted and no
    // test would have noticed.
    expect(out).toContain('var(--surface-panel)')
    expect(out).toContain('var(--interactive)')
  })

  // Same guarantee the drawing has: it appears as it arrives rather than in one
  // jump at the end. The parser closes what is still open, so a half-written
  // panel is a panel with fewer cards in it — never raw markup on screen.
  it('builds itself as it streams', () => {
    const out = renderStreamingMarkdown(panel.slice(0, 180))

    expect(out).toContain('<div')
    expect(out).not.toContain('&lt;div')
  })

  // The two hazards the sanitizer already handles. Worth pinning because the
  // prompt promises the model they are handled — an answer that could execute
  // by being displayed would be a way around every gate in the app.
  it('drops the parts that could execute', () => {
    const out = renderMarkdown(
      '<div style="color:var(--text-primary)"><style>body{display:none}</style><script>fetch("http://evil")</script>ok</div>',
    )

    expect(out).toContain('ok')
    expect(out).not.toContain('<script')
    expect(out).not.toContain('evil')
    expect(out).not.toContain('display:none')
  })
})
