// The rooms behind each door (COMPANY.md §2, §86), from the outside.
//
// What is worth pinning here is not that a row of buttons renders — it is the
// rules a nav that opens sessions has to keep: clicking a desk you are not at
// opens a session THERE, clicking the desk you are already at does not throw
// away the conversation in front of you, ระบบออโตเมชั่น is a room with the door built
// and nothing behind it, and — since the split — each door draws only its own
// rooms, so the workshop never shows the office's.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Sidebar from '../lib/Sidebar.svelte'
import { NewSessionAt, NewChairSession, ListModes, SessionMode, CurrentSessionID } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setShell, deskFilterFor } from '../lib/shell.svelte'

const deskButton = (label: string): HTMLButtonElement => {
  const el = Array.from(document.querySelectorAll('.desk-btn'))
    .find((b) => b.querySelector('.t')?.textContent?.trim() === label)
  if (!el) throw new Error(`desk button "${label}" not found`)
  return el as HTMLButtonElement
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.chair = ''
  cockpit.activeView = 'chat'
  cockpit.history.length = 0
  setShell('assistant') // the storefront, which is where a fresh window opens
  vi.mocked(CurrentSessionID).mockResolvedValue('20260805-120000.000')
  vi.mocked(SessionMode).mockResolvedValue('')
})

describe('the rooms behind each door', () => {
  it('offers every storefront room, and none of them still claims to be coming', () => {
    render(Sidebar, { onOpenSettings: () => {} })

    // โปรเจกต์ opened on 2026-08-07 (§90), ระบบออโตเมชั่น on 2026-08-09. A room
    // that has been built must stop saying it is coming: the badge is the
    // promise, and a promise left up after it is kept is the same lie as one
    // that was never kept. Every room behind this door is now open, so the
    // whole row is checked rather than a list of exceptions — the next `soon`
    // room to be added has to come back here and say so out loud.
    for (const label of ['ผู้ช่วย', 'โปรเจกต์', 'ทีมเอเจน', 'ระบบออโตเมชั่น', 'ผลงาน']) {
      expect(deskButton(label)).toBeTruthy()
      expect(deskButton(label).disabled).toBe(false)
      expect(deskButton(label).textContent).not.toContain('เร็ว ๆ นี้')
    }
  })

  // The whole point of §86: the workshop is not the storefront with extra
  // furniture, it is a different building. โค้ด hands work to no one, so a
  // window showing it must not offer the office, its files, or its routines.
  it('draws only the workshop behind the code door', () => {
    setShell('code')
    render(Sidebar, { onOpenSettings: () => {} })

    expect(deskButton('โค้ด')).toBeTruthy()
    for (const hidden of ['ผู้ช่วย', 'โปรเจกต์', 'ทีมเอเจน', 'ระบบออโตเมชั่น', 'ผลงาน']) {
      expect(() => deskButton(hidden)).toThrow()
    }
  })

  // One list per door, no switch between them: the workshop's column is
  // projects (chats nested under them), the storefront's is conversations.
  // The tab pair that used to sit here was the mixing §86 ended.
  it('gives each door one list and no tab back to the other', () => {
    const { unmount } = render(Sidebar, { onOpenSettings: () => {} })
    expect(document.querySelector('.side-tabs')).toBeNull()
    expect(screen.queryByText('เพิ่มโปรเจกต์')).toBeNull()
    unmount()

    setShell('code')
    render(Sidebar, { onOpenSettings: () => {} })
    expect(document.querySelector('.side-tabs')).toBeNull()
    expect(screen.queryByText('เพิ่มโปรเจกต์')).toBeTruthy()
  })

  it('opens a session at the desk that was clicked', async () => {
    render(Sidebar, { onOpenSettings: () => {} })

    await fireEvent.click(deskButton('ผู้ช่วย'))

    await waitFor(() => expect(vi.mocked(NewSessionAt).mock.calls[0][0]).toBe('assistant'))
    expect(cockpit.desk).toBe('assistant')
    expect(cockpit.activeView).toBe('chat')
  })

  // A nav button that discarded the conversation you were in the middle of
  // would make the whole row unusable — you could never click the room you are
  // already standing in.
  it('does not start a second session when you click the desk you are at', async () => {
    cockpit.desk = 'coding'
    setShell('code')
    render(Sidebar, { onOpenSettings: () => {} })

    await fireEvent.click(deskButton('โค้ด'))

    expect(vi.mocked(NewSessionAt)).not.toHaveBeenCalled()
    expect(cockpit.activeView).toBe('chat')
  })

  it('sends the pages to their own view without touching the session', async () => {
    render(Sidebar, { onOpenSettings: () => {} })

    await fireEvent.click(deskButton('ผลงาน'))
    expect(cockpit.activeView).toBe('artifacts')

    await fireEvent.click(deskButton('ทีมเอเจน'))
    expect(cockpit.activeView).toBe('office')
    expect(vi.mocked(NewSessionAt)).not.toHaveBeenCalled()
  })

  // ระบบออโตเมชั่น is a conversation, and the person on the other side is the
  // automation specialist rather than the assistant. It opens through the same
  // door the office roster's "แชทกับเอเจนนี้" uses — one way in, so a chat
  // started from the nav and one started from the roster are the same chat with
  // the same tools, memory and prompt.
  //
  // It was a page for a day, drawing cards for the engines you could connect.
  // Connecting an account is register work and the register already does it, so
  // the room was answering a question that had a home while the thing people
  // come here for had none.
  it('opens ระบบออโตเมชั่น as a chat with the automation agent', async () => {
    render(Sidebar, { onOpenSettings: () => {} })

    await fireEvent.click(deskButton('ระบบออโตเมชั่น'))

    await waitFor(() => expect(vi.mocked(NewChairSession)).toHaveBeenCalledWith('automation'))
    expect(cockpit.activeView).toBe('chat')
    expect(cockpit.chair).toBe('automation')
    // Not a desk session: the agent brings its own tools and prompt, and asking
    // for a desk as well would be two masters for one turn.
    expect(vi.mocked(NewSessionAt)).not.toHaveBeenCalled()
  })

  // A chair chat runs at the office desk, so lighting the nav by desk would
  // leave ทีมเอเจน lit while the user is standing in ระบบออโตเมชั่น.
  it('lights the room by who you are talking to, not by the desk', async () => {
    cockpit.desk = 'specialized'
    cockpit.chair = 'automation'
    cockpit.activeView = 'chat'
    render(Sidebar, { onOpenSettings: () => {} })

    expect(deskButton('ระบบออโตเมชั่น').className).toContain('active')
    expect(deskButton('ทีมเอเจน').className).not.toContain('active')
  })

  // The button's tooltip is the desk's own description, so editing a mode file
  // changes what the button says about itself and nothing else has to be told.
  it('describes a desk with its own manifest', async () => {
    vi.mocked(ListModes).mockResolvedValue([
      { name: 'assistant', description: 'โต๊ะที่ผมแก้เอง', prompt: '', builtin: true },
    ] as any)
    render(Sidebar, { onOpenSettings: () => {} })

    await waitFor(() => expect(deskButton('ผู้ช่วย').title).toBe('โต๊ะที่ผมแก้เอง'))
  })
})

describe('the history list', () => {
  const chat = (id: string, mode: string) => ({ id, title: `chat ${id}`, ago: '', mode })

  // The list is the history of the *door*, not of the room you are standing in
  // (owner's call, 2026-08-06: "แสดงแค่ฝั่งผู้ช่วยก็พอครับ โค้ดก็ส่วนโค้ดแยกกัน").
  // ผู้ช่วย keeps conversations, โค้ด keeps projects with their chats nested
  // underneath, and a coding session in the storefront's list is the mixing the
  // split exists to end.
  //
  // Filtering by *desk* was tried before and emptied the column on the first
  // click — a session is only stamped with a desk when opened through a desk
  // button, so every chat begun by opening the app and typing is held at no
  // desk and matched nothing. That regression is guarded below and must stay
  // guarded: shellForDesk sends '' to the storefront, which is where a session
  // predating the split is at home, so it is filed rather than dropped.
  it('draws every chat the store hands it', () => {
    setShell('assistant')
    cockpit.desk = 'assistant'
    cockpit.history.push(chat('a', 'assistant'), chat('c', ''))
    render(Sidebar, { onOpenSettings: () => {} })

    for (const title of ['chat a', 'chat c']) {
      expect(screen.queryByText(title)).toBeTruthy()
    }
  })

  // Which door a session belongs to is decided once, by deskFilterFor, and
  // applied in SQL — see ListSessionsForDoor for why filtering a fetched page
  // instead starves one list once the history is longer than one page.
  //
  // Filtering by *desk* was tried once and emptied the column on the first
  // click: a session is only stamped with a desk when opened through a desk
  // button, so every chat begun by opening the app and typing is held at no
  // desk and matched nothing. The storefront asking for everything *except*
  // the workshop's desk is what keeps that fixed — '' is not on any list to be
  // left off.
  it('scopes each door by excluding the other, never by naming its own', () => {
    expect(deskFilterFor('code')).toEqual({ desks: ['coding'], exclude: false })
    expect(deskFilterFor('assistant')).toEqual({ desks: ['coding'], exclude: true })
  })

  it('labels each chat with its desk', () => {
    cockpit.desk = ''
    cockpit.history.push(chat('a', 'assistant'), chat('c', ''))
    render(Sidebar, { onOpenSettings: () => {} })

    const rows = Array.from(document.querySelectorAll('.sess-row'))
    const labelled = rows.find((r) => r.textContent?.includes('chat a'))
    expect(labelled?.querySelector('.sess-desk')?.textContent).toBe('ผู้ช่วย')
    // A pre-desk chat gets no badge: the column deliberately does not know
    // which desk it was, and a guessed label would be inventing one.
    const legacy = rows.find((r) => r.textContent?.includes('chat c'))
    expect(legacy?.querySelector('.sess-desk')).toBeNull()
  })
})
