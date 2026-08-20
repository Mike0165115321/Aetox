// The panel cap. Every number here is arithmetic nobody can see on screen: get
// it wrong and a panel just stops short while being dragged, with nothing to
// say why. That is exactly what happened — with the sidebar collapsed the cap
// still reserved its remembered width and still counted its hidden handle, so
// on a 1417px window the workbench refused the last 286px it was entitled to.
import { describe, it, expect } from 'vitest'
import { clampPanelWidth, fitPanelsToWindow, maxPanelWidth, MAIN_FLOOR, HANDLE_PX } from '../lib/panelSize'

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

// The other half of the same arithmetic, and the half nothing was asking until
// 2026-08-20: the cap runs while a handle is being dragged, but the WINDOW
// changes size too. A workbench dragged wide on a maximised window stayed that
// wide when the window came back down, and from then on the grid was wider than
// the frame with nothing on screen to say so — until a deck loaded, called
// scrollIntoView, and shoved the whole shell sideways into the room that
// overflow had left lying around.
describe('re-fitting the panels when the window changes size', () => {
  const sidebar = { width: 280, min: 200, visible: true }
  const inspector = { width: 900, min: 320, visible: true }

  it('leaves a grid that still fits exactly alone', () => {
    // A width the user chose is not up for renegotiation just because a resize
    // happened; only an overflow is.
    const viewport = 280 + 900 + MAIN_FLOOR + HANDLE_PX * 2
    expect(fitPanelsToWindow(viewport, [sidebar, inspector])).toEqual([280, 900])
    expect(fitPanelsToWindow(viewport + 400, [sidebar, inspector])).toEqual([280, 900])
  })

  it('takes the overflow from both, in proportion to the slack each has', () => {
    const viewport = 280 + 900 + MAIN_FLOOR + HANDLE_PX * 2
    const got = fitPanelsToWindow(viewport - 100, [sidebar, inspector])

    // The grid fits again...
    expect(got[0] + got[1] + MAIN_FLOOR + HANDLE_PX * 2).toBeLessThanOrEqual(viewport - 100)
    // ...and neither was pinned to its floor while the other kept everything,
    // which is what refitting them one after another does.
    expect(got[0]).toBeGreaterThan(sidebar.min)
    expect(got[1]).toBeGreaterThan(inspector.min)
    // The inspector has 580px of slack to the sidebar's 80, so it gives up most.
    expect(inspector.width - got[1]).toBeGreaterThan(sidebar.width - got[0])
  })

  it('counts no room and no handle for a collapsed panel', () => {
    const collapsed = { ...sidebar, visible: false }
    const viewport = 900 + MAIN_FLOOR + HANDLE_PX
    expect(fitPanelsToWindow(viewport, [collapsed, inspector])).toEqual([280, 900])
    // ...and its remembered width is left where it is, not shrunk on its behalf.
    expect(fitPanelsToWindow(viewport - 200, [collapsed, inspector])[0]).toBe(280)
  })

  it('stops at the floors rather than drawing a panel below its minimum', () => {
    const tiny = fitPanelsToWindow(100, [sidebar, inspector])
    expect(tiny[0]).toBeGreaterThanOrEqual(sidebar.min)
    expect(tiny[1]).toBeGreaterThanOrEqual(inspector.min)
    // Already on the floors and still too small: nothing left to give, and the
    // widths stay put rather than going below what a panel needs to work.
    const onFloors = [
      { width: 200, min: 200, visible: true },
      { width: 320, min: 320, visible: true },
    ]
    expect(fitPanelsToWindow(100, onFloors)).toEqual([200, 320])
  })

  // The bug this arithmetic was correct through, and still lost to.
  //
  // Every number here is a CSS pixel, and the shell is not measured in the same
  // pixels as the window: the UI zoom control writes `zoom` to <body>, so the
  // grid's box is innerWidth ÷ zoom while innerWidth itself never moves. Fed
  // the window's number, this function reserves space the grid does not have,
  // and the overflow lands on whatever is furthest right — on 20 ส.ค. that was
  // the workbench's export button, hanging 44px off the edge of a 1424px window
  // whose shell measured 1380.
  //
  // Nothing in the function can defend against being handed the wrong viewport,
  // which is the point: App.svelte asks the shell element for its own width
  // (gridWidth), and this is the case that says why.
  it('reserves against the width it is given, so the caller must give it the grid', () => {
    const zoomed = { width: 1058, min: 320, visible: true }
    const window1424 = fitPanelsToWindow(1424, [{ ...sidebar, visible: false }, zoomed])
    const shell1380 = fitPanelsToWindow(1380, [{ ...sidebar, visible: false }, zoomed])

    // The window's number leaves the panel exactly where the user dragged it...
    expect(window1424[1]).toBe(1058)
    // ...and the shell's number is the one that gives the 44px back.
    expect(shell1380[1]).toBe(1058 - 44)
    expect(window1424[1] - shell1380[1]).toBe(1424 - 1380)
  })
})
