// The running beam, checked as CSS text rather than as pixels.
//
// The bug this pins was invisible in every unit test and obvious on screen: a
// streamed answer is drawn with {@html}, so the markup inside .markdown-body is
// REPLACED on every token. An animation declared on the ring restarts with the
// element that carries it, so the light twitched in place instead of
// travelling ("ตอนวาดแผน อนิเมชั่นพังครับ เพราะมันสตรีมอ่ะ").
//
// The fix is structural — the clock runs on an ancestor the stream does not
// touch, and the phase inherits down to whatever ring exists this frame — so
// the thing worth pinning is the structure, not the look.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

const css = readFileSync('src/style.css', 'utf8')
const rule = (selector: string) =>
  css.slice(css.indexOf(selector)).slice(0, css.slice(css.indexOf(selector)).indexOf('}') + 1)

describe('the running beam', () => {
  it('runs its clock on the ancestor, never on the ring', () => {
    expect(css).toContain('.markdown-body:has(.live) { animation:beam-phase')
    // The ring reads the phase; if it ever animates it again, a streamed block
    // is back to restarting sixty times a second.
    const ring = rule('.bgw-card.run::before,')
    expect(ring).toContain('var(--beam-phase)')
    expect(ring).not.toContain('animation:')
  })

  it('inherits the phase, or a ring built this frame starts from zero', () => {
    expect(css).toMatch(/@property --beam-phase\s*\{[^}]*inherits:\s*true/)
  })

  it('is worn only by what is still being produced', () => {
    for (const selector of ['.plan-card.live', '.codeblock.live', '.drawing-box.live', '.bgw-card.run']) {
      expect(css).toContain(`${selector}::before`)
    }
    // A finished delegation is a record, and a record that glows asks to be
    // re-read. `.done`/`.err` must never pick this up.
    expect(css).not.toContain('.bgw-card.done::before')
  })

  it('drops the motion but keeps the signal when motion is unwelcome', () => {
    const at = css.indexOf('.markdown-body:has(.live) { animation:none; }')
    expect(at).toBeGreaterThan(-1)
    // The nearest at-rule above it is the guard, not some other block it drifted
    // into: stopping the clock unconditionally would leave a still gradient at
    // whatever angle it happened to be on.
    const before = css.slice(0, at)
    expect(before.slice(before.lastIndexOf('@media'))).toContain('prefers-reduced-motion')
    // Still lit, so which one is working is still answerable without motion.
    expect(css.slice(at, at + 500)).toContain('--interactive')
  })
})
