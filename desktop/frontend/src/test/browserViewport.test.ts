// Device-size presets. The pairing under test: the tab's native window is
// letterboxed to the device's aspect inside the pane, and the page zoom is that
// SAME factor — which is what leaves the page a CSS viewport of exactly the
// device's width (so its media queries fire as they would on the device). Break
// either half and you get a small window showing a desktop layout.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import BrowserPane from '../lib/workbench/BrowserPane.svelte'
import { workbench, openBrowserTab } from '../lib/stores/workbench.svelte'
import { BrowserSetBounds, BrowserSetZoom, BrowserSetVisible } from './mocks/wailsApp'

const PANE = { x: 100, y: 50, width: 400, height: 900 }
// Pane pixels the native window never gets, on every side. ไฟบอกสถานะ's
// border light is drawn by the app, and the app draws behind this window — so
// the strip it runs in has to be kept back from the page (BrowserPane
// PANE_FRAME, §174). Every expectation below measures from the framed box, not
// from the pane.
const FRAME = 3
const BOX = {
  x: PANE.x + FRAME, y: PANE.y + FRAME,
  width: PANE.width - FRAME * 2, height: PANE.height - FRAME * 2,
}

beforeEach(() => {
  vi.clearAllMocks()
  window.devicePixelRatio = 1
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  HTMLElement.prototype.getBoundingClientRect = () => ({
    ...PANE, top: PANE.y, left: PANE.x, bottom: PANE.y + PANE.height, right: PANE.x + PANE.width, toJSON: () => '',
  }) as DOMRect
})

function mount(viewport?: { name: string; w: number; h: number }) {
  render(BrowserPane, {
    tab: { id: 'web-1', kind: 'browser' as const, name: 'x', url: 'https://a.test', viewport },
    active: true,
    menuOpen: false,
    dragging: false,
  })
  return vi.waitFor(() => expect(BrowserSetZoom).toHaveBeenCalled())
}

const lastBounds = () => BrowserSetBounds.mock.lastCall as unknown as [string, number, number, number, number]
const lastZoom = () => (BrowserSetZoom.mock.lastCall as unknown as [string, number])[1]

describe('browser device presets', () => {
  it('fills the pane inside its frame at zoom 1 when no preset is chosen', async () => {
    await mount()
    expect(lastZoom()).toBe(1)
    expect(lastBounds()).toEqual(['web-1', BOX.x, BOX.y, BOX.width, BOX.height])
  })

  it('centers a device that already fits, without upscaling it', async () => {
    await mount({ name: 'iPhone 12 Pro', w: 390, h: 844 })
    expect(lastZoom()).toBe(1)
    expect(lastBounds()).toEqual(['web-1', 105, 78, 390, 844]) // centered in the 394x894 framed box
  })

  it('shrinks a device larger than the pane and zooms to match', async () => {
    await mount({ name: 'iPad Mini', w: 768, h: 1024 })
    const [, , , w, h] = lastBounds()
    expect(lastZoom()).toBeCloseTo(BOX.width / 768)
    // The invariant: window size ÷ zoom is the device's real CSS viewport.
    expect(w / lastZoom()).toBeCloseTo(768)
    expect(Math.abs(h / lastZoom() - 1024)).toBeLessThan(1) // bounds round to whole device pixels
  })

  // The path a user actually takes: page already open, THEN pick a preset from
  // the ⋮ menu. Uses a real store tab so the $state proxy is what drives it.
  it('reflows when a preset is picked after the page is open', async () => {
    const id = openBrowserTab()
    const tab = workbench.tabs.find((t) => t.id === id)!
    tab.url = 'https://a.test'
    render(BrowserPane, { tab, active: true, menuOpen: false, dragging: false })
    await vi.waitFor(() => expect(BrowserSetZoom).toHaveBeenCalled())
    BrowserSetBounds.mockClear()
    BrowserSetZoom.mockClear()

    tab.viewport = { name: 'iPhone 14 Pro Max', w: 430, h: 932 }
    await vi.waitFor(() => expect(BrowserSetBounds).toHaveBeenCalled())
    expect(lastZoom()).toBeCloseTo(BOX.width / 430)
    expect(lastBounds()[3] / lastZoom()).toBeCloseTo(430)
  })

  // The tab is a real OS window drawn above the app's own webview, so a dropdown
  // opened over the pane is invisible unless the window steps aside.
  it('hides the native window while a workbench menu is open', async () => {
    const props = { tab: { id: 'web-1', kind: 'browser' as const, name: 'x', url: 'https://a.test' }, active: true, menuOpen: false, dragging: false }
    const { rerender } = render(BrowserPane, props)
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenCalledWith('web-1', true))

    await rerender({ ...props, menuOpen: true })
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenLastCalledWith('web-1', false))

    await rerender({ ...props, menuOpen: false })
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenLastCalledWith('web-1', true))
  })
})
