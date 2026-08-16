// The chat list in the storefront's column, from the outside.
//
// Two rules are worth pinning. The first is that the list is grouped by
// calendar day: fourteen rows each stamped "3 วันที่แล้ว" is not a list a user
// scans, it is one they read, and the header is what lets the eye skip. The
// second is that the search box is summoned rather than permanent — it and the
// new-session button shrank to two icons on the row above the list, and a
// regression that re-pins them as blocks would take a third of the column's
// height back without anyone noticing.
//
// Search results rank by match, not by date, so they are deliberately NOT
// grouped: a "วันนี้" header printed three times down one list would be
// claiming an order the results do not have.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/svelte'
import Sidebar from '../lib/Sidebar.svelte'
import { SessionMode, CurrentSessionID, LoadSessionAnyProject, OpenProjectPath } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setShell } from '../lib/shell.svelte'
import type { Session } from '../lib/types'

const daysAgo = (n: number, hour = 12): string => {
  const d = new Date()
  d.setDate(d.getDate() - n)
  d.setHours(hour, 0, 0, 0)
  return d.toISOString()
}

const chat = (id: string, title: string, updatedAt: string): Session =>
  ({ id, title, ago: '', updatedAt })

const dayHeads = (): string[] =>
  Array.from(document.querySelectorAll('.sess-day-head')).map((e) => e.textContent?.trim() ?? '')

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.activeView = 'chat'
  cockpit.history.length = 0
  setShell('assistant')
  vi.mocked(CurrentSessionID).mockResolvedValue('20260805-120000.000')
  vi.mocked(SessionMode).mockResolvedValue('')
})

describe('the chat list', () => {
  it('groups rows under the day they belong to, in the list order', () => {
    cockpit.history.push(
      chat('a', 'วันนี้เช้า', daysAgo(0, 9)),
      chat('b', 'เมื่อคืน', daysAgo(1, 23)),
      chat('c', 'อาทิตย์ก่อน', daysAgo(4)),
      chat('d', 'เดือนก่อน', daysAgo(20)),
      chat('e', 'นานมาก', daysAgo(200)),
    )
    render(Sidebar, { onOpenSettings: () => {} })

    expect(dayHeads()).toEqual([
      'วันนี้', 'เมื่อวาน', '7 วันที่ผ่านมา', '30 วันที่ผ่านมา', 'เก่ากว่านั้น',
    ])
  })

  it('opens one header per run, not one per row', () => {
    cockpit.history.push(
      chat('a', 'หนึ่ง', daysAgo(0, 9)),
      chat('b', 'สอง', daysAgo(0, 10)),
      chat('c', 'สาม', daysAgo(0, 11)),
    )
    render(Sidebar, { onOpenSettings: () => {} })

    expect(dayHeads()).toEqual(['วันนี้'])
    expect(document.querySelectorAll('.sess-row').length).toBe(3)
  })

  // A chat that predates the stamp still belongs somewhere. Dropping it would
  // be losing a row to a formatting detail.
  it('keeps a row with no timestamp, at the far end', () => {
    cockpit.history.push(chat('a', 'ไม่มีเวลา', ''))
    render(Sidebar, { onOpenSettings: () => {} })

    expect(dayHeads()).toEqual(['เก่ากว่านั้น'])
    expect(screen.getByText('ไม่มีเวลา')).toBeTruthy()
  })

  // The row above the list holds the field itself, not an icon that reveals
  // one: two glyphs alone left half the row empty, and this is the control
  // that says what the column below it is.
  it('draws the search field on the row above the list', () => {
    render(Sidebar, { onOpenSettings: () => {} })

    const field = document.querySelector('.side-actions .side-search input')
    expect(field).toBeTruthy()
    expect(field?.getAttribute('placeholder')).toBe('ค้นหาประวัติ…')
  })

  // The chunky dashed block that used to sit above this list is an icon on the
  // same row now, and that row serves the project column too — one copy, not
  // one per list. Import sits beside it: bringing a chat in is the column's
  // own action, same as starting one.
  it('keeps the column actions as icons, not blocks above the list', () => {
    render(Sidebar, { onOpenSettings: () => {} })

    expect(screen.queryByText('เริ่มเซสชันใหม่')).toBeNull()
    const labels = Array.from(document.querySelectorAll('.side-actions button'))
      .map((b) => b.getAttribute('aria-label'))
    expect(labels).toEqual(['นำเข้าแชทที่ส่งออกไว้ (.json)', 'เริ่มเซสชันใหม่'])
  })

  // The row's whole job. Every other way into a session switched the view to
  // the chat; this one loaded the conversation and left the user looking at
  // whatever page they were on — the row lit up, nothing else moved, and the
  // only reading available from the outside was that the row does not work.
  it('shows the chat when a row is clicked from another page', async () => {
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([] as never)
    cockpit.activeView = 'settings'
    cockpit.history.push(chat('a', 'เปิดอันนี้', daysAgo(0, 9)))
    render(Sidebar, { onOpenSettings: () => {} })

    await fireEvent.click(screen.getByText('เปิดอันนี้'))

    expect(cockpit.activeView).toBe('chat')
  })

  // The engine writes a sentence for each of its seven refusals — the folder
  // moved, the desk file is gone, the session is not in this project. An
  // unhandled rejection here used to swallow every one of them, which made a
  // session that cannot open indistinguishable from a row that is not wired up.
  it('says why when the engine refuses to open the session', async () => {
    vi.mocked(LoadSessionAnyProject).mockRejectedValue(
      new Error('ไม่พบโปรเจกต์ของเซสชันนี้ (โฟลเดอร์อาจถูกย้ายหรือลบไปแล้ว)'),
    )
    cockpit.sessionError = ''
    cockpit.history.push(chat('a', 'เปิดไม่ได้', daysAgo(0, 9)))
    render(Sidebar, { onOpenSettings: () => {} })

    await fireEvent.click(screen.getByText('เปิดไม่ได้'))

    expect(cockpit.sessionError).toContain('โฟลเดอร์อาจถูกย้าย')
  })

  // The title is the only thing on a row that says what the chat was about, so
  // it gets the line — the chip, the age and the hover buttons share the one
  // below it. They used to sit on the title's line, all of them flex:none, and
  // in a 280px column the title was squeezed to a couple of words.
  it('gives the title a line of its own, above the chip and the age', () => {
    cockpit.history.push({ ...chat('a', 'ค้นไฟล์ทั้งเครื่องแล้วสรุปให้ที', daysAgo(0, 9)), agent: 'ผู้ช่วย' })
    render(Sidebar, { onOpenSettings: () => {} })

    const row = document.querySelector('.sess-row')
    expect(row?.querySelector('.sess-line')?.textContent?.trim()).toBe('ค้นไฟล์ทั้งเครื่องแล้วสรุปให้ที')
    expect(row?.querySelector('.sess-meta')?.textContent).toContain('ผู้ช่วย')
    // The hover controls live on the meta line, in space nothing else wants.
    expect(row?.querySelector('.sess-meta .sess-acts .sess-del')).toBeTruthy()
  })
})

// Starting work in a project, from the project's own row.
//
// Until this there was one way in: click the project's name. That works —
// opening a project starts a fresh session on it — but every other list in the
// app treats a name as "show me this", so the action was hidden inside a
// gesture that reads as navigation (owner, 16 ส.ค., pointing at the row:
// "เนี้ยมันไม่มี").
describe('the project rows in the workshop column', () => {
  it('offers a new chat on each project, and opens that project when pressed', async () => {
    setShell('code')
    cockpit.projects = [
      { key: 'a', name: 'frontend', path: 'D:/work/frontend', active: false },
      { key: 'b', name: 'senior-architect-agent', path: 'D:/work/agent', active: true },
    ] as any
    vi.mocked(OpenProjectPath).mockResolvedValue({ focused: true, name: 'frontend' } as any)

    const { container } = render(Sidebar, { onOpenSettings: () => {} })

    const adds = Array.from(container.querySelectorAll('.proj-group-new'))
    expect(adds.length).toBe(2)
    // Named, because it is an icon: the row has two other buttons on it and a
    // bare + is only obvious to whoever put it there.
    expect(adds[0].getAttribute('aria-label')).toBe('แชทใหม่ในโปรเจกต์นี้')
    // And the tooltip has to open leftward: the button is against the column's
    // right edge, which is where the panel clips, so a centred one is cut in
    // half (owner, 16 ส.ค., holding a screenshot of exactly that).
    expect(adds[0].classList.contains('tip-r')).toBe(true)

    await fireEvent.click(adds[0])
    expect(vi.mocked(OpenProjectPath)).toHaveBeenCalledWith('D:/work/frontend')
    // And it lands in the chat it just made, rather than leaving the user on
    // whatever page the click was pressed from.
    expect(cockpit.activeView).toBe('chat')
  })

  it('is not in the storefront column, which focuses no project at all', () => {
    setShell('assistant')
    cockpit.projects = [{ key: 'a', name: 'frontend', path: 'D:/work/frontend', active: false }] as any

    const { container } = render(Sidebar, { onOpenSettings: () => {} })

    expect(container.querySelector('.proj-group-new')).toBeNull()
  })
})
