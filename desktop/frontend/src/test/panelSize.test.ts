// The panel cap. Every number here is arithmetic nobody can see on screen: get
// it wrong and a panel just stops short while being dragged, with nothing to
// say why. That is exactly what happened — with the sidebar collapsed the cap
// still reserved its remembered width and still counted its hidden handle, so
// on a 1417px window the workbench refused the last 286px it was entitled to.
import { describe, it, expect } from 'vitest'
import { clampPanelWidth, maxPanelWidth, MAIN_FLOOR, HANDLE_PX } from '../lib/panelSize'

const INSPECTOR_MIN = 320
const SIDEBAR_WIDTH = 280

describe('how wide a panel may be dragged', () => {
  it('leaves the chat its floor when both panels are on screen', () => {
    const max = maxPanelWidth({
      viewport: 1417, min: INSPECTOR_MIN, otherWidth: SIDEBAR_WIDTH, otherVisible: true,
    })
    expect(max).toBe(1417 - SIDEBAR_WIDTH - MAIN_FLOOR - HANDLE_PX * 2)
    // ...and the grid still fits: panel + both handles + sidebar + floor.
    expect(max + HANDLE_PX * 2 + SIDEBAR_WIDTH + MAIN_FLOOR).toBe(1417)
  })

  it('hands over the space a collapsed panel is not using', () => {
    const withSidebar = maxPanelWidth({
      viewport: 1417, min: INSPECTOR_MIN, otherWidth: SIDEBAR_WIDTH, otherVisible: true,
    })
    const collapsed = maxPanelWidth({
      viewport: 1417, min: INSPECTOR_MIN, otherWidth: 0, otherVisible: false,
    })

    // The bug this pins: 765px, when 1051px of room existed.
    expect(withSidebar).toBe(765)
    expect(collapsed).toBe(1051)
    // Its column and its handle, both gone.
    expect(collapsed - withSidebar).toBe(SIDEBAR_WIDTH + HANDLE_PX)
  })

  it('never reports a ceiling below the panel’s own floor', () => {
    // A window too narrow for the whole layout. The floor wins — the grid
    // overflows, which .app's overflow-x already handles, rather than the
    // panel being clamped to a negative width.
    const max = maxPanelWidth({ viewport: 500, min: INSPECTOR_MIN, otherWidth: 280, otherVisible: true })
    expect(max).toBe(INSPECTOR_MIN)
  })

  it('holds a requested width between the floor and the ceiling', () => {
    const fit = { viewport: 1417, min: INSPECTOR_MIN, otherWidth: 0, otherVisible: false }
    expect(clampPanelWidth(600, fit)).toBe(600)
    expect(clampPanelWidth(50, fit)).toBe(INSPECTOR_MIN)
    expect(clampPanelWidth(9999, fit)).toBe(1051)
  })
})
