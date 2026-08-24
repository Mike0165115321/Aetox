// A browser tab closed from the Go side, and the chip it used to leave behind.
//
// `workbench:open-browser` had no partner. The file side has said both halves
// all along (`workbench:close-file`), so a tab the agent closed — or one the
// orphan sweep took after a window reload — stayed on the strip for good,
// pointing at a native view that no longer existed. And BrowserPane latches
// `opened` once it has called BrowserOpen, so the pane behind it never tried
// again either: a black rectangle with a URL in the address bar (owner, 24 ส.ค.).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor } from '@testing-library/svelte'
import {
  workbench, openBrowserTab, openFilesTab, browserTabClosedByEngine, closeTab,
} from '../lib/stores/workbench.svelte'
import { BrowserClose, BrowserOpen } from './mocks/wailsApp'
import BrowserPane from '../lib/workbench/BrowserPane.svelte'

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
})

describe('a browser tab the engine closed', () => {
  it('comes off the strip', () => {
    const id = openBrowserTab()

    browserTabClosedByEngine(id)

    expect(workbench.tabs.find((t) => t.id === id)).toBeUndefined()
  })

  it('hands the strip on rather than leaving nothing selected', () => {
    const first = openBrowserTab()
    const second = openBrowserTab()

    browserTabClosedByEngine(second)

    expect(workbench.activeId).toBe(first)
  })

  it('leaves the other tabs alone', () => {
    openFilesTab()
    const doomed = openBrowserTab()
    const kept = openBrowserTab()

    browserTabClosedByEngine(doomed)

    expect(workbench.tabs.map((t) => t.id)).toEqual(['files', kept])
  })

  // The event is the mirror of one act on one browser tab. A file tab that
  // happened to share an id would not be its business, and neither is an id
  // nothing holds — which is what arrives when the pane's own teardown closes a
  // tab this event has already removed.
  it('says nothing about an id it does not hold', () => {
    openFilesTab()

    browserTabClosedByEngine('files')
    browserTabClosedByEngine('web-nothing')

    expect(workbench.tabs.map((t) => t.id)).toEqual(['files'])
  })
})

// Who ended a tab has to be said where it happened. It used to be inferred from
// the pane's teardown, and a teardown is a lifecycle event rather than an
// intent: the engine closing a tab takes the chip off the strip, which unmounts
// the pane, which closed the tab again — and the engine read that second call
// as a person pressing ×. The agent was then told a user had shut its page.
describe('who closed a browser tab', () => {
  it('the × says so to the engine itself', async () => {
    const id = openBrowserTab()
    const tab = workbench.tabs.find((t) => t.id === id)!

    await closeTab(tab)

    expect(BrowserClose).toHaveBeenCalledWith(id)
  })

  it('a pane going away does not close anything', async () => {
    // jsdom has no ResizeObserver and the pane keeps its native window glued to
    // its own rect with one. Nothing here measures anything, so a stub is enough.
    ;(globalThis as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
    const id = openBrowserTab()
    const tab = workbench.tabs.find((t) => t.id === id)!
    // A page, and then the wait for it: the pane only latches `opened` once it
    // has actually called BrowserOpen, and a pane that never opened would not
    // have closed anything under the old code either — so without this the test
    // passes whatever the pane does, which is no test at all.
    tab.url = 'https://example.com'
    const { unmount } = render(BrowserPane, {
      props: { tab, active: true, menuOpen: false, dragging: false },
    })
    await waitFor(() => expect(BrowserOpen).toHaveBeenCalled())

    unmount()

    expect(BrowserClose).not.toHaveBeenCalled()
  })
})
