// A browser tab closed from the Go side, and the chip it used to leave behind.
//
// `workbench:open-browser` had no partner. The file side has said both halves
// all along (`workbench:close-file`), so a tab the agent closed — or one the
// orphan sweep took after a window reload — stayed on the strip for good,
// pointing at a native view that no longer existed. And BrowserPane latches
// `opened` once it has called BrowserOpen, so the pane behind it never tried
// again either: a black rectangle with a URL in the address bar (owner, 24 ส.ค.).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  workbench, openBrowserTab, openFilesTab, browserTabClosedByEngine,
} from '../lib/stores/workbench.svelte'

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
