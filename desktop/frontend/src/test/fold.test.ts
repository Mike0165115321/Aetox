// The folding transition, checked where it can be: its own configuration.
//
// What it does on screen is not testable here, and the part that would silently
// rot is not the easing — it is the reduced-motion guard. svelte/transition does
// not consult the setting, and the CSS half of the app does, so a fold that
// ignored it would be the one animation in the app that keeps moving after the
// user asked everything to stop.
import { describe, it, expect, afterEach } from 'vitest'
import { fold } from '../lib/fold'

const el = () => document.createElement('div')
const withMotionSetting = (reduce: boolean) => {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string) => ({ matches: reduce && query.includes('reduce'), media: query }),
  })
}

afterEach(() => {
  Reflect.deleteProperty(window, 'matchMedia')
})

describe('the folding transition', () => {
  it('folds over time by default', () => {
    withMotionSetting(false)
    expect(fold(el()).duration).toBe(240)
  })

  it('collapses to no time at all when motion is unwelcome', () => {
    withMotionSetting(true)
    expect(fold(el()).duration).toBe(0)
  })

  // Read per call, not captured at module load: the setting can change while
  // the app is open, and a value read once would need a restart to take effect.
  it('reads the setting again rather than remembering the first answer', () => {
    withMotionSetting(false)
    expect(fold(el()).duration).toBe(240)
    withMotionSetting(true)
    expect(fold(el()).duration).toBe(0)
  })

  // The test environment has no matchMedia at all, and neither will some host
  // the app is embedded in one day. Missing must mean "animate", not "throw".
  it('animates when the platform cannot say', () => {
    Reflect.deleteProperty(window, 'matchMedia')
    expect(fold(el()).duration).toBe(240)
  })

  it('lets a caller ask for a different length', () => {
    withMotionSetting(false)
    expect(fold(el(), { duration: 90 }).duration).toBe(90)
  })
})
