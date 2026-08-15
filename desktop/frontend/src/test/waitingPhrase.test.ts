// The waiting state: the phrase itself carries a sweep of light, and nothing
// else is on the line. What it replaced was three bouncing dots — a second
// ellipsis on a line whose phrase already ends in one, jumping up and down
// directly above a tool list that grows downward.
//
// The one thing here that breaks silently and only on screen: the effect is
// painted with a transparent text fill, so with motion turned off it must be
// handed its colour back. Stopped is not still — stopped is invisible.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

const css = readFileSync('src/style.css', 'utf8')

describe('the waiting phrase', () => {
  it('moves the words themselves, with nothing beside them', () => {
    expect(css).toMatch(/\.typing-status \{[^}]*animation:status-shimmer/)
    // The dots are gone for good: a phrase ending in "..." does not need a
    // second set of them.
    expect(css).not.toContain('typing-dots')
  })

  it('gives the letters their colour back when motion is unwelcome', () => {
    const guard = css.slice(css.indexOf('.typing-status {'))
    const at = guard.indexOf('-webkit-text-fill-color:currentColor')
    expect(at).toBeGreaterThan(-1)
    expect(guard.slice(0, at)).toContain('prefers-reduced-motion')
  })
})
