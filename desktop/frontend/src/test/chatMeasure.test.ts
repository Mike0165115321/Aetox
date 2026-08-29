// How wide a line of the answer is allowed to get.
//
// Measured in the running app on 30 ส.ค. before anything was changed: at a
// 1400px window the chat column is 1097px and an assistant paragraph ran the
// full 1020px of it — 157 characters a line at 15.5px, where comfortable
// reading is 45–75 and even generous UI guidance stops near 90. Nothing was
// holding it, so a wider monitor only made it worse.
//
// The asymmetry is what gave it away: `.msg.user` has been capped at 620px for
// as long as it existed, so the user's own line — usually a sentence — was held
// to a readable measure while the long text on the screen was not.
//
// Read off disk like themeContrast and composerNarrow do: vitest stubs CSS
// imports to "", and a rule checked against an empty stylesheet passes.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

const css = readFileSync('src/style.css', 'utf8')

/** The declarations of the rule whose selector list ends with `selector`. */
function rule(selector: string): string {
  const at = css.indexOf(selector + ' {')
  if (at < 0) throw new Error('no rule for ' + selector)
  const open = css.indexOf('{', at)
  const close = css.indexOf('}', open)
  return css.slice(open + 1, close)
}

/** The `ch` figure in a declaration block, or NaN. */
function capInCh(decls: string): number {
  const key = 'max-width:'
  const at = decls.indexOf(key)
  if (at < 0) return NaN
  const rest = decls.slice(at + key.length).trim()
  const digits = rest.slice(0, rest.indexOf('ch'))
  return Number(digits.trim())
}

const PROSE = '.msg:not(.user) .bubble .markdown-body > h6'

describe('the answer is held to a reading measure', () => {
  it('caps the prose in an assistant bubble', () => {
    expect(rule(PROSE)).toContain('max-width:')
  })

  // In `ch`, never px: the reader can change the chat's font size, and a cap in
  // pixels would tighten as the type grew — worst exactly for the person who
  // enlarged it because they were struggling to read.
  it('measures the cap in characters, so it follows the reader’s font size', () => {
    expect(rule(PROSE)).not.toContain('px')
    expect(Number.isNaN(capInCh(rule(PROSE)))).toBe(false)
  })

  // The cap is on the prose children and not on .bubble, which is the whole
  // reason code, tables, images and diagrams still get the column. Capping the
  // bubble would have taken the room away from the things that need it most.
  it('does not cap the bubble itself', () => {
    expect(rule('.bubble')).not.toContain('max-width')
  })

  // Verified in the app at the value this shipped with: 60ch resolved to 587px,
  // and a real Thai paragraph came out 93 characters a line against 81 for
  // English — 60 rather than the 70–75 the English guidance names, because `ch`
  // is the width of "0" (9.78px in this face) while a Thai glyph averages
  // 6.5px, and Thai has no spaces for the eye to rest on.
  it('keeps the cap tight enough for Thai, which has no word spaces', () => {
    const ch = capInCh(rule(PROSE))
    expect(ch).toBeGreaterThanOrEqual(50)
    expect(ch).toBeLessThanOrEqual(66)
  })
})
