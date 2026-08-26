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
