// The rooms behind each door (COMPANY.md §2, §86), from the outside.
//
// What is worth pinning here is not that a row of buttons renders — it is the
// rules a nav that opens sessions has to keep: clicking a desk you are not at
// opens a session THERE, clicking the desk you are already at does not throw
// away the conversation in front of you, ออโต้ is a room with the door built
// and nothing behind it, and — since the split — each door draws only its own
// rooms, so the workshop never shows the office's.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Sidebar from '../lib/Sidebar.svelte'
import { NewSessionAt, ListModes, SessionMode, CurrentSessionID } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setShell } from '../lib/shell.svelte'

const deskButton = (label: string): HTMLButtonElement => {
  const el = Array.from(document.querySelectorAll('.desk-btn'))
    .find((b) => b.querySelector('.t')?.textContent?.trim() === label)
  if (!el) throw new Error(`desk button "${label}" not found`)
  return el as HTMLButtonElement
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.activeView = 'chat'
  cockpit.history.length = 0
  setShell('assistant') // the storefront, which is where a fresh window opens
  vi.mocked(CurrentSessionID).mockResolvedValue('20260805-120000.000')
  vi.mocked(SessionMode).mockResolvedValue('')
})

describe('the rooms behind each door', () => {
  it('offers the storefront rooms, with ออโต้ present and disabled', () => {
    render(Sidebar, { onOpenSettings: () => {} })

    for (const label of ['ผู้ช่วย', 'ทีมเอเจน', 'ออโต้', 'ผลงาน']) {
      expect(deskButton(label)).toBeTruthy()
    }
    // Built, empty, and saying so — a room that appeared later would read as a
    // new feature rather than as the plan it already is (§7).
    expect(deskButton('ออโต้').disabled).toBe(true)
    expect(deskButton('ออโต้').textContent).toContain('เร็ว ๆ นี้')
  })

  // The whole point of §86: the workshop is not the storefront with extra
  // furniture, it is a different building. โค้ด hands work to no one, so a
  // window showing it must not offer the office, its files, or its routines.
  it('draws only the workshop behind the code door', () => {
    setShell('code')
    render(Sidebar, { onOpenSettings: () => {} })

    expect(deskButton('โค้ด')).toBeTruthy()
    for (const hidden of ['ผู้ช่วย', 'ทีมเอเจน', 'ออโต้', 'ผลงาน']) {
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

  // The column is the chat history, not the chat history of the room you are
  // standing in. It used to filter to the desk you were at, and that emptied it
  // on the first click: a session is only stamped with a desk when it is opened
  // through a desk button, so every chat begun by opening the app and typing is
  // held at no desk and vanished the moment you walked into one.
  it('shows every chat, whichever desk you are at', () => {
    cockpit.desk = 'assistant'
    cockpit.history.push(chat('a', 'assistant'), chat('b', 'coding'), chat('c', ''))
    render(Sidebar, { onOpenSettings: () => {} })

    for (const title of ['chat a', 'chat b', 'chat c']) {
      expect(screen.queryByText(title)).toBeTruthy()
    }
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
