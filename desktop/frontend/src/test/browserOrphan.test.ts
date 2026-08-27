// A native browser window must never outlive the pane that was steering it.
//
// It is a real OS window composited above the app's own webview, so an orphaned
// one is not a stale rectangle in the DOM that the next paint clears. It sits
// there, on top of the chat, at whatever bounds it last had, and nothing in the
// window can reach it: the pane that owned it is gone, so no effect will ever
// hide it or move it again. The owner's screenshot on 27 ส.ค. is exactly that.
//
// The hole was the seam between two correct decisions. BrowserPane's onDestroy
// stopped closing anything on 2026-08-25, because an unmount happens for
// several reasons and reporting them all as "the user closed my page" was
// lying to the agent. restoreWorkbench had been relying on that teardown since
// before it existed, and said so in a comment that quietly became false.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import BrowserPane from '../lib/workbench/BrowserPane.svelte'
import { BrowserSetVisible, BrowserSetBounds, BrowserCloseForTeardown, BrowserClose } from './mocks/wailsApp'
import {
  workbench, openBrowserTab, openFilesTab, adoptWorkbenchSession, switchWorkbenchSession,
} from '../lib/stores/workbench.svelte'

const PANE = { x: 0, y: 0, width: 400, height: 900 }

beforeEach(() => {
  vi.clearAllMocks()
  window.devicePixelRatio = 1
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  HTMLElement.prototype.getBoundingClientRect = () => ({
    ...PANE, top: 0, left: 0, bottom: PANE.height, right: PANE.width, toJSON: () => '',
  }) as DOMRect
})

describe('an unmounted browser pane leaves nothing on screen', () => {
  it('hides the native window when the pane goes away', async () => {
    const { unmount } = render(BrowserPane, {
      tab: { id: 'web-1', kind: 'browser' as const, name: 'x', url: 'https://a.test' },
      active: true, menuOpen: false, dragging: false,
    })
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenCalledWith('web-1', true))

    BrowserSetVisible.mockClear()
    unmount()
    expect(BrowserSetVisible).toHaveBeenCalledWith('web-1', false)
    // Hiding, not closing. Closing is a lifetime event that has to name who
    // asked, and an unmount cannot answer that question — which is the whole
    // reason this hook stopped closing in the first place.
    expect(BrowserClose).not.toHaveBeenCalled()
    expect(BrowserCloseForTeardown).not.toHaveBeenCalled()
  })

  it('says nothing about a window it never opened', async () => {
    const { unmount } = render(BrowserPane, {
      tab: { id: 'web-2', kind: 'browser' as const, name: 'x', url: '' },
      active: true, menuOpen: false, dragging: false,
    })
    unmount()
    expect(BrowserSetVisible).not.toHaveBeenCalled()
  })
})

// The other half, one layer up. A pane hiding itself is the net; the caller
// that throws the whole strip away is the one that has to actually close.
describe('switching sessions closes the pages it discards', () => {
  beforeEach(() => {
    workbench.tabs.length = 0
    workbench.activeId = ''
    localStorage.clear()
  })

  it('closes an open browser tab, as a teardown rather than as a person', async () => {
    await adoptWorkbenchSession('first')
    const id = openBrowserTab()
    const tab = workbench.tabs.find((t) => t.id === id)!
    tab.url = 'https://a.test'

    await switchWorkbenchSession('second')

    expect(BrowserCloseForTeardown).toHaveBeenCalledWith(id)
    // Not BrowserClose: that one means a person clicked ×, and telling the
    // agent its page was shut by the user because the user opened a different
    // conversation is the exact lie browser-tab-lifetime-2026-08-25 named.
    expect(BrowserClose).not.toHaveBeenCalled()
    expect(workbench.tabs).toHaveLength(0)
  })

  it('leaves a session with no page open with nothing to close', async () => {
    await adoptWorkbenchSession('first')
    openFilesTab()
    await switchWorkbenchSession('second')
    expect(BrowserCloseForTeardown).not.toHaveBeenCalled()
  })
})

// The owner's actual repro, and the one the hand-written list of reasons could
// never have covered: the pane is still mounted, still the active tab, still in
// a room that is not an overlay — and it has no box, because an ancestor five
// levels up went display:none.
//
// Walking from the code door to แชทผู้ช่วย empties the strip; App.svelte then
// sets inspectorCollapsed from `workbench.tabs.length === 0`; style.css hides
// `.inspector` outright. Nothing in that chain has ever heard of BrowserPane,
// which is exactly why the answer had to become a measurement.
describe('a pane with no box hides its native window', () => {
  let fire: ((on: boolean) => void) | undefined

  beforeEach(() => {
    fire = undefined
    vi.stubGlobal('IntersectionObserver', class {
      cb: (e: unknown[]) => void
      constructor(cb: (e: unknown[]) => void) { this.cb = cb }
      observe(el: Element) {
        fire = (on: boolean) => this.cb([{ target: el, isIntersecting: on, intersectionRatio: on ? 1 : 0 }])
        fire(true)
      }
      unobserve() {}
      disconnect() {}
    })
  })

  it('hides when the pane loses its box, and comes back when it returns', async () => {
    render(BrowserPane, {
      tab: { id: 'web-9', kind: 'browser' as const, name: 'x', url: 'https://a.test' },
      active: true, menuOpen: false, dragging: false,
    })
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenCalledWith('web-9', true))

    // The inspector collapses. The pane is untouched: same tab, still active,
    // same room. Only its box is gone.
    BrowserSetVisible.mockClear()
    fire!(false)
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenLastCalledWith('web-9', false))

    // And back. Bounds are re-glued on the way in, because the pane has usually
    // moved while it was away.
    BrowserSetVisible.mockClear()
    BrowserSetBounds.mockClear()
    fire!(true)
    await vi.waitFor(() => expect(BrowserSetVisible).toHaveBeenLastCalledWith('web-9', true))
    expect(BrowserSetBounds).toHaveBeenCalled()
  })
})
