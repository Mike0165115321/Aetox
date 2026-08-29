// Walking into a โปรเจกต์ from the sidebar rail (§84, §90).
//
// The rail got a project list on 30 ส.ค. so that reaching a conversation stopped
// costing four clicks through a page nobody's destination was. Three rules came
// out of that afternoon, and all three are here because all three were got wrong
// once before they were got right:
//
//   - clicking a project opens a NEW blank chat in it, not the newest one it
//     already has. Landing on the old thread left no way at all to start a
//     second chat inside the project you were standing in;
//   - the project's own chat list is refreshed by that click. It was not, and
//     the heading sat over an empty list saying "แชทใน Aetox โพสต์";
//   - the blank chat appears in that list, the way the global history has
//     carried the open draft all along.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, openSpace, newSession, refreshSpaceHistory } from '../lib/stores/cockpit.svelte'
import { shell } from '../lib/shell.svelte'
import { NewSessionInSpace, SessionsInSpace, CurrentSessionID, CurrentSpace } from './mocks/wailsApp'

const held = [
  { id: 'old-2', title: 'โครงบทที่ 3', updatedAt: '2026-08-30T09:00:00Z', mode: 'assistant', agent: '' },
  { id: 'old-1', title: 'หาตัวอย่างประกอบ', updatedAt: '2026-08-29T09:00:00Z', mode: 'assistant', agent: '' },
]

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.awaitingReply = false
  cockpit.sessionError = ''
  cockpit.space = ''
  cockpit.spaceHistory = []
  cockpit.chat = []
  cockpit.desk = ''
  cockpit.chair = ''
  shell.name = 'assistant'
  vi.mocked(SessionsInSpace).mockResolvedValue(held as any)
  vi.mocked(CurrentSessionID).mockResolvedValue('new-blank')
  vi.mocked(CurrentSpace).mockResolvedValue('Aetox โพสต์')
})

describe('walking into a project from the rail', () => {
  it('opens a blank chat in it rather than reopening the newest one', async () => {
    await openSpace('Aetox โพสต์')

    expect(vi.mocked(NewSessionInSpace)).toHaveBeenCalledWith('Aetox โพสต์')
    expect(cockpit.space).toBe('Aetox โพสต์')
    expect(cockpit.chat).toEqual([])
    expect(cockpit.activeView).toBe('chat')
  })

  // The bug this file exists for. afterNewSession refreshed the global history
  // and never this list, which went unnoticed for as long as the only way into
  // a project was selectGlobalSession — that one goes through refreshDesk, which
  // does refresh it. The rail is a second door, and it arrived stale.
  it('refreshes the project’s own chat list on the way in', async () => {
    await openSpace('Aetox โพสต์')

    expect(cockpit.spaceHistory.map((s) => s.title)).toContain('โครงบทที่ 3')
    expect(cockpit.spaceHistory.map((s) => s.title)).toContain('หาตัวอย่างประกอบ')
  })

  // A chat with nothing said in it has no row in the database, so the list has
  // to carry it the way the global history already carries the open draft —
  // otherwise the click hands you a chat that is nowhere on the column drawn
  // directly beside it.
  it('carries the blank chat it just made at the top of that list', async () => {
    await openSpace('Aetox โพสต์')

    expect(cockpit.spaceHistory[0].id).toBe('new-blank')
    expect(cockpit.spaceHistory[0].draft).toBe(true)
  })

  // Wanting a second chat about the project you are in is the whole reason this
  // row opens a blank one — so being in the project is NOT enough to refuse.
  // Sitting on an untouched blank chat in it is: that already is what the click
  // asks for, and making another would leave two แชทใหม่ rows and move the
  // cursor off the one being typed in.
  it('does not stack a second blank chat on an untouched one', async () => {
    cockpit.space = 'Aetox โพสต์'
    cockpit.chat = []

    await openSpace('Aetox โพสต์')

    expect(vi.mocked(NewSessionInSpace)).not.toHaveBeenCalled()
  })

  it('does open another once something has been said in the first', async () => {
    cockpit.space = 'Aetox โพสต์'
    cockpit.chat = [{ role: 'user', content: 'เริ่มเลย' } as any]

    await openSpace('Aetox โพสต์')

    expect(vi.mocked(NewSessionInSpace)).toHaveBeenCalledWith('Aetox โพสต์')
  })

  // The same call that fills the list is what empties it: a chat started at a
  // desk is in no project, and a rail still showing the last project's chats
  // under a heading naming it would be the stale list in the other direction.
  it('empties the list when the new chat is in no project', async () => {
    cockpit.space = 'Aetox โพสต์'
    await refreshSpaceHistory()
    expect(cockpit.spaceHistory.length).toBeGreaterThan(0)

    await newSession()

    expect(cockpit.space).toBe('')
    expect(cockpit.spaceHistory).toEqual([])
  })
})
