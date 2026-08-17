// The no-URL state of a browser tab. It is the only part of the workbench
// browser that is DOM at all — the moment a URL exists a native OS window
// covers the pane — so everything asserted here is about that one window of
// time: what it offers, what it does when there is nothing to offer, and that
// clicking a row navigates THIS tab rather than opening anything new.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import BrowserPane from '../lib/workbench/BrowserPane.svelte'
import { workbench, openBrowserTab, recordVisit, recentVisits } from '../lib/stores/workbench.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { RecentAgentPages, BrowserOpen } from './mocks/wailsApp'

const PANE = { x: 100, y: 50, width: 384, height: 900 }

const page = (url: string, title = '', time = new Date().toISOString()) => ({ url, title, time })

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.removeItem('aetox-browser-history')
  workbench.tabs.length = 0
  workbench.activeId = ''
  cockpit.awaitingReply = false
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  HTMLElement.prototype.getBoundingClientRect = () => ({
    ...PANE, top: PANE.y, left: PANE.x, bottom: PANE.y + PANE.height, right: PANE.x + PANE.width, toJSON: () => '',
  }) as DOMRect
})

/** A real store tab, so the $state proxy is what drives the pane. */
function mountBlankTab(viewport?: { name: string; w: number; h: number }) {
  const id = openBrowserTab()
  const tab = workbench.tabs.find((t) => t.id === id)!
  tab.viewport = viewport
  const r = render(BrowserPane, { tab, active: true, menuOpen: false, dragging: false })
  return { tab, ...r }
}

const rows = (c: HTMLElement) => [...c.querySelectorAll('.bp-row')] as HTMLButtonElement[]

describe('a new browser tab', () => {
  it('opens a page the agent had opened, in this same tab', async () => {
    // Explicit, distinct stamps: the list is sorted newest-first, so a fixture
    // that leaves both rows at "now" decides its own order on whether the two
    // constructor calls straddled a millisecond. It did, about one full-suite
    // run in three.
    RecentAgentPages.mockResolvedValueOnce([
      page('https://aetox.dev/docs', 'Aetox docs', '2026-08-04T05:00:00+07:00'),
      page('https://example.test', 'Example', '2026-08-04T04:00:00+07:00'),
    ])
    const { tab, container } = mountBlankTab()
    await vi.waitFor(() => expect(rows(container)).toHaveLength(2))

    rows(container)[0].click()
    await vi.waitFor(() => expect(tab.url).toBe('https://aetox.dev/docs'))

    // Same two assignments the address bar makes — the tab is renamed for its
    // host, and the native window opens on this pane.
    expect(tab.name).toBe('aetox.dev')
    await vi.waitFor(() => expect(BrowserOpen).toHaveBeenCalledTimes(1))
    // One row, one destination: never a second tab.
    expect(workbench.tabs).toHaveLength(1)
  })

  it('falls back to the URL when the agent never got a title', async () => {
    RecentAgentPages.mockResolvedValueOnce([page('https://titleless.test/a/b', '')])
    const { container } = mountBlankTab()
    await vi.waitFor(() => expect(rows(container)).toHaveLength(1))

    // A row with a blank line where its name goes reads as broken data.
    expect(rows(container)[0].textContent).toContain('titleless.test')
  })

  it('says there is nothing yet instead of showing an empty box', async () => {
    RecentAgentPages.mockResolvedValueOnce([])
    const { container } = mountBlankTab()

    await vi.waitFor(() => expect(container.querySelector('.bp-note')).toBeTruthy())
    expect(rows(container)).toHaveLength(0)
    expect(BrowserOpen).not.toHaveBeenCalled()
  })

  it('reads a database that will not open as the same nothing', async () => {
    RecentAgentPages.mockRejectedValueOnce(new Error('db unavailable'))
    const { container } = mountBlankTab()

    // Not an error card: "there is nothing to show you" is the honest content
    // of both, and this must not throw on the way there.
    await vi.waitFor(() => expect(container.querySelector('.bp-note')).toBeTruthy())
  })

  it('expands the list in place rather than sending the user elsewhere', async () => {
    RecentAgentPages.mockResolvedValueOnce(
      Array.from({ length: 10 }, (_, i) => page(`https://p${i}.test`, `P${i}`)),
    )
    const { container } = mountBlankTab()
    await vi.waitFor(() => expect(rows(container)).toHaveLength(6))

    const more = container.querySelector('.bp-more') as HTMLButtonElement
    expect(more).toBeTruthy()
    more.click()

    await vi.waitFor(() => expect(rows(container)).toHaveLength(10))
    expect(container.querySelector('.bp-more')).toBeNull()
    // Expanding is not navigating.
    expect(BrowserOpen).not.toHaveBeenCalled()
    expect(workbench.tabs).toHaveLength(1)
  })

  it('drops to the head alone inside a device preset', async () => {
    RecentAgentPages.mockResolvedValue([page('https://a.test', 'A')])
    const { container } = mountBlankTab({ name: 'iPhone SE', w: 375, h: 667 })
    await vi.waitFor(() => expect(container.querySelector('.bp-head')).toBeTruthy())

    // A letterboxed preset can leave under 280px of width. The head fits there;
    // a list does not, and neither does a 420px watermark.
    expect(container.querySelector('.bp-start.compact')).toBeTruthy()
    expect(rows(container)).toHaveLength(0)
    expect(container.querySelector('.brand-ground')).toBeNull()
  })

  it('stands on the same ground as every other empty room in the app', async () => {
    // Not a watermark of its own: `.brand-ground` is one rule shared with the
    // empty chat and the first-run screen, so all three cannot drift apart.
    RecentAgentPages.mockResolvedValue([page('https://a.test', 'A')])
    const { container } = mountBlankTab()
    await vi.waitFor(() => expect(container.querySelector('.bp-head')).toBeTruthy())

    expect(container.querySelector('.brand-ground svg')).toBeTruthy()
  })

  it('gives one instruction, not two', async () => {
    // The caption pinned to the bottom of the pane told the user about the
    // address bar from further away than anything else on screen, while the
    // line under the title said the agent opens pages itself. One screen, one
    // instruction — so the bottom one is gone and its half was folded in.
    RecentAgentPages.mockResolvedValue([])
    const { container } = mountBlankTab()
    await vi.waitFor(() => expect(container.querySelector('.bp-note')).toBeTruthy())

    expect(container.querySelector('.bp-foot')).toBeNull()
    expect(container.querySelectorAll('.bp-head .d')).toHaveLength(1)
  })

  it('lists the pages the user went to, not only the ones the agent opened', async () => {
    // The complaint that produced this: three rows, all of them files the agent
    // had made, and nothing the user had actually browsed to.
    RecentAgentPages.mockResolvedValueOnce([
      page('file:///C:/out/index.html', 'Aetox — landing', '2026-08-03T22:01:02+07:00'),
    ])
    recordVisit('https://go.dev/doc', 'Go Docs')

    const { container } = mountBlankTab()
    await vi.waitFor(() => expect(rows(container)).toHaveLength(2))

    // Newest first, across both sources — compared as instants, since Go writes
    // an offset and the frontend writes UTC.
    expect(rows(container)[0].textContent).toContain('go.dev')
    expect(rows(container)[1].textContent).toContain('Aetox — landing')
  })

  it('shows a page that is in both sources once', async () => {
    RecentAgentPages.mockResolvedValueOnce([page('https://a.test', 'A', '2026-08-01T10:00:00+07:00')])
    // The agent opened it, so it also navigated — both records exist.
    recordVisit('https://a.test', 'A')

    const { container } = mountBlankTab()
    await vi.waitFor(() => expect(rows(container)).toHaveLength(1))
  })

  it('refetches when a turn ends, and at no other time', async () => {
    RecentAgentPages.mockResolvedValue([page('https://a.test', 'A')])
    mountBlankTab()
    await vi.waitFor(() => expect(RecentAgentPages).toHaveBeenCalledTimes(1))

    // A turn just ended is the only moment a new browser_open row can exist.
    cockpit.awaitingReply = true
    await vi.waitFor(() => expect(RecentAgentPages).toHaveBeenCalledTimes(1))
    cockpit.awaitingReply = false
    await vi.waitFor(() => expect(RecentAgentPages).toHaveBeenCalledTimes(2))

    // No timer, no poll: nothing else moves it.
    await new Promise((r) => setTimeout(r, 30))
    expect(RecentAgentPages).toHaveBeenCalledTimes(2)
  })
})

describe('the pages this browser has shown', () => {
  it('keeps one row per page, newest first', () => {
    recordVisit('https://a.test', 'A')
    recordVisit('https://b.test', 'B')
    recordVisit('https://a.test', 'A again') // a reload, or coming back later

    const visits = recentVisits()
    expect(visits.map((v) => v.url)).toEqual(['https://a.test', 'https://b.test'])
    expect(visits[0].title).toBe('A again')
  })

  it('leaves local files to the side that can check they still exist', () => {
    // A file:// row remembered here could not be stat'd from the frontend, and
    // a row that opens the engine's "not found" page is the dead end the start
    // page exists to prevent. RecentAgentPages already covers these, checked.
    recordVisit('file:///C:/out/index.html', 'Report')
    recordVisit('about:blank', '')
    expect(recentVisits()).toHaveLength(0)
  })

  it('survives a history that is not what it expects', () => {
    localStorage.setItem('aetox-browser-history', 'not json')
    expect(recentVisits()).toEqual([])

    localStorage.setItem('aetox-browser-history', JSON.stringify([{ nope: 1 }, null, { url: 'https://ok.test', time: 'x' }]))
    expect(recentVisits().map((v) => v.url)).toEqual(['https://ok.test'])
  })
})
