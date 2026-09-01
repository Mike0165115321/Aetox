// A desk belongs to one conversation (§187).
//
// desk_open used to land its tab on whichever desk was on screen: a chat
// working in the background finished a deck, and the file appeared in front of
// whoever was reading something else — then the on-screen session's next
// snapshot persisted the stray as its own, so the leak survived restarts. The
// event names its session now, and the store routes: on-screen means live,
// background means that session's saved desk, found there when its chat opens.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  workbench, openFileTab, adoptWorkbenchSession, switchWorkbenchSession,
  routeDeskEvent,
} from '../lib/stores/workbench.svelte'

// Through the door, not around it: routeDeskEvent is the single entry every
// desk event takes (§187.3), so entering anywhere else would test a path the
// window no longer uses.
const openAgentFileTabFor = (sessionId: string, path: string, name: string) =>
  Promise.resolve(routeDeskEvent('open-file', { sessionId, path, name }))
const closeAgentFileTabFor = (sessionId: string, path: string) =>
  routeDeskEvent('close-file', { sessionId, path })
import { ReadFile, WorkbenchTabsChanged } from './mocks/wailsApp'

const saved = (id: string) =>
  JSON.parse(localStorage.getItem(`aetox-workbench:${id}`) ?? '{"tabs":[]}') as {
    tabs: { kind: string; path?: string; mine?: boolean }[]
  }

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
  workbench.foreign.length = 0
  localStorage.clear()
  vi.mocked(ReadFile).mockResolvedValue('เนื้อไฟล์' as any)
})

describe('a desk belongs to one conversation', () => {
  it('keeps a background chat’s file off the desk on screen, and on its own', async () => {
    await adoptWorkbenchSession('on-screen')

    await openAgentFileTabFor('background', 'output/background/deck.html', 'deck.html')

    // Not here — the user is reading something else.
    expect(workbench.tabs).toHaveLength(0)
    // But not lost: it is on that session's saved desk, marked the agent's...
    expect(saved('background').tabs).toEqual([
      { kind: 'file', name: 'deck.html', path: 'output/background/deck.html', mine: true },
    ])
    // ...and the Go mirror was told, so desk_list and desk close in that chat
    // judge against the desk its user will actually find.
    expect(WorkbenchTabsChanged).toHaveBeenCalledWith('background', [
      { kind: 'file', name: 'deck.html', path: 'output/background/deck.html', url: '', mine: true },
    ])

    // Opening the background chat finds the file waiting.
    await switchWorkbenchSession('background')
    expect(workbench.tabs.map((t) => t.path)).toEqual(['output/background/deck.html'])
    expect(workbench.tabs[0].mine).toBe(true)
  })

  it('still opens live for the chat on screen', async () => {
    await adoptWorkbenchSession('mine-1')
    await openAgentFileTabFor('mine-1', 'output/mine-1/deck.html', 'deck.html')
    expect(workbench.tabs).toHaveLength(1)
    expect(workbench.tabs[0].mine).toBe(true)
  })

  it('lets a background chat take back only its own tab', async () => {
    await adoptWorkbenchSession('front-2')
    await openAgentFileTabFor('back-2', 'output/back-2/a.html', 'a.html')

    closeAgentFileTabFor('back-2', 'output/back-2/a.html')
    expect(saved('back-2').tabs).toHaveLength(0)
    // And it cannot reach across: closing a path on another session's desk
    // touches nothing here.
    await openFileTab('notes.md')
    closeAgentFileTabFor('back-2', 'notes.md')
    expect(workbench.tabs).toHaveLength(1)
  })

  it('an agent-opened tab is still the agent’s after a switch away and back', async () => {
    await adoptWorkbenchSession('keep-3')
    await openAgentFileTabFor('keep-3', 'output/keep-3/deck.html', 'deck.html')
    await switchWorkbenchSession('elsewhere-3')
    await switchWorkbenchSession('keep-3')
    // `mine` survived the snapshot — desk close's safety rule still knows
    // whose tab this is.
    expect(workbench.tabs[0].mine).toBe(true)
  })

  it('draws nothing for a desk event no policy was written for', () => {
    routeDeskEvent('open-hologram', { sessionId: 'anyone', path: 'x' })
    expect(workbench.tabs).toHaveLength(0)
  })

  // The Go door's '' — a surface with no per-session owner yet (§187.2) —
  // draws live, as a stated policy rather than an accident.
  it('mounts an ownerless surface on the desk on screen', () => {
    routeDeskEvent('open-terminal', { sessionId: '', id: 'pty-1', name: 'pwsh' })
    expect(workbench.tabs.map((t) => t.kind)).toEqual(['terminal'])
  })
})

// The browser joined §187 on 1 ก.ย., after its page was the last thing still
// leaking: a background chat opened the Aetox landing page and it drew, fronted
// itself, and was snapshotted onto the desk the owner was actually reading.
// A page differs from a file in one way that shapes everything here: the tab is
// a live native window the background agent is still browsing, so parking it
// must keep the window alive (the shadow rack) rather than file a dead chip.
describe('a browser page belongs to one conversation', () => {
  it('keeps a background chat’s page off the desk on screen, alive on the rack, and on its own desk', async () => {
    await adoptWorkbenchSession('on-screen')
    routeDeskEvent('open-browser', { sessionId: 'background', id: 'web-agent-1', url: 'https://a.test/page' })

    // Not on the strip, and not fronted — the user is reading something else.
    expect(workbench.tabs).toHaveLength(0)
    expect(workbench.activeId).toBe('')
    // Alive on the rack, so the agent's window keeps existing…
    expect(workbench.foreign.map((t) => t.id)).toEqual(['web-agent-1'])
    // …and waiting on its own session's saved desk, id and all.
    expect(saved('background').tabs).toEqual([
      { kind: 'browser', id: 'web-agent-1', name: 'a.test', url: 'https://a.test/page', mine: true },
    ])

    // Opening that chat adopts the live window — same id, no rebuilt copy.
    await switchWorkbenchSession('background')
    expect(workbench.tabs.map((t) => t.id)).toEqual(['web-agent-1'])
    expect(workbench.foreign).toHaveLength(0)
  })

  it('still draws live, and fronts, for the chat on screen', async () => {
    await adoptWorkbenchSession('mine-b')
    routeDeskEvent('open-browser', { sessionId: 'mine-b', id: 'web-agent-2', url: 'https://b.test' })
    expect(workbench.tabs.map((t) => t.id)).toEqual(['web-agent-2'])
    expect(workbench.activeId).toBe('web-agent-2')
    expect(workbench.tabs[0].mine).toBe(true)
  })

  it('parks an agent page on the rack across a switch away, and re-adopts it on the way back', async () => {
    await adoptWorkbenchSession('keep-b')
    routeDeskEvent('open-browser', { sessionId: 'keep-b', id: 'web-agent-3', url: 'https://keep.test' })

    await switchWorkbenchSession('elsewhere-b')
    // Not torn down under a possibly-working agent — parked, window alive.
    expect(workbench.tabs).toHaveLength(0)
    expect(workbench.foreign.map((t) => t.id)).toEqual(['web-agent-3'])

    await switchWorkbenchSession('keep-b')
    expect(workbench.tabs.map((t) => t.id)).toEqual(['web-agent-3'])
    expect(workbench.tabs[0].mine).toBe(true)
    expect(workbench.foreign).toHaveLength(0)
  })

  it('lets a background chat claim a tab off the shown desk, and takes it with it', async () => {
    await adoptWorkbenchSession('front-b')
    routeDeskEvent('open-browser', { sessionId: 'front-b', id: 'web-agent-4', url: 'https://x.test' })
    expect(workbench.activeId).toBe('web-agent-4')

    // The shared agent pool lets another chat steer the same tab (§ the tabs
    // pack): the page is that chat's now, so it leaves this strip too.
    routeDeskEvent('open-browser', { sessionId: 'claimer-b', id: 'web-agent-4', url: 'https://y.test' })
    expect(workbench.tabs).toHaveLength(0)
    expect(workbench.foreign.map((t) => t.sessionId)).toEqual(['claimer-b'])
    expect(saved('claimer-b').tabs.map((t) => (t as { id?: string }).id)).toEqual(['web-agent-4'])
  })

  it('a close reaches a parked page everywhere it is remembered', async () => {
    await adoptWorkbenchSession('front-c')
    routeDeskEvent('open-browser', { sessionId: 'back-c', id: 'web-agent-5', url: 'https://z.test' })
    routeDeskEvent('close-browser', { sessionId: '', id: 'web-agent-5' })
    expect(workbench.foreign).toHaveLength(0)
    expect(saved('back-c').tabs).toHaveLength(0)
  })
})
