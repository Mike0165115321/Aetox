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
import { SessionMode, CurrentSessionID, LoadSessionAnyProject } from './mocks/wailsApp'
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
  // one per list.
  it('keeps new-session as an icon, not a block above the list', () => {
    render(Sidebar, { onOpenSettings: () => {} })

    expect(screen.queryByText('เริ่มเซสชันใหม่')).toBeNull()
    const labels = Array.from(document.querySelectorAll('.side-actions button'))
      .map((b) => b.getAttribute('aria-label'))
    expect(labels).toEqual(['เริ่มเซสชันใหม่'])
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
})
