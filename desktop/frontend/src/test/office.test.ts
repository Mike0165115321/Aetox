// ทีมเอเจน (COMPANY.md §4). The page is a roster and a feed, and the one claim
// worth pinning is what the roster shows: the tools each chair *gets*, after
// the office ceiling — not the ones its file asked for. This is the row a
// person checks the ceiling on, so a page that echoed the request back would
// quietly defeat the structure it is reporting on.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Office from '../lib/Office.svelte'
import { ListChairs, ListReceivedJobs, LoadSessionAnyProject, NewChairSession } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'

const chair = (over: Record<string, unknown> = {}) => ({
  name: 'doc', description: 'เก้าอี้ร่างเอกสาร', tools: ['doc_write', 'read', 'pdf_read'],
  builtin: true, jobs: 0, lastUsed: '', ...over,
})

const job = (over: Record<string, unknown> = {}) => ({
  id: 1, chair: 'doc', sessionId: '20260805-090000.000',
  request: 'ทำเอกสารสรุปยอดเดือนนี้', answer: 'เขียนเสร็จแล้ว',
  toolSeq: 'pdf_read>doc_write', toolCount: 2, durationMs: 4200,
  outcome: 'unknown', time: new Date().toISOString(), ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.activeView = 'office'
  vi.mocked(ListChairs).mockResolvedValue([chair()] as any)
  vi.mocked(ListReceivedJobs).mockResolvedValue([] as any)
})

describe('the office roster', () => {
  it('lists each chair with the tools it actually gets', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair({ tools: ['doc_write', 'read'] })] as any)
    render(Office, { onClose: () => {} })

    // The name appears on both the card's cover and its title — that is the
    // gallery design, so the assertion is "present", not "present once".
    await waitFor(() => expect(screen.getAllByText('doc').length).toBeGreaterThan(0))
    expect(screen.getByText('เก้าอี้ร่างเอกสาร')).toBeTruthy()
    expect(screen.getByText('doc_write')).toBeTruthy()
    // Nothing invents a `shell` chip: the page draws the list the engine
    // computed under the ceiling, so it cannot show more than the chair has.
    expect(screen.queryByText('shell')).toBeNull()
  })

  it('says plainly when a chair has never been handed anything', async () => {
    render(Office, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText('ยังไม่เคยรับงาน')).toBeTruthy())
  })

  it('counts the work a chair has done', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair({ jobs: 3, lastUsed: new Date().toISOString() })] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('3')).toBeTruthy())
    expect(screen.getByText(/งานที่ทำแล้ว/)).toBeTruthy()
  })

  // Hiring is dropping a file (§84), and the gallery's first card is the door
  // to the folder where that file goes.
  it('offers a create-agent card', async () => {
    render(Office, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText('สร้างเอเจนใหม่')).toBeTruthy())
  })

  // Walking into the room (§85): the card's chat button opens a session bound
  // to that agent, and the view moves to the chat that session now owns.
  it('opens a direct chat with the agent on its card', async () => {
    vi.mocked(NewChairSession).mockResolvedValue('20260805-100000.000' as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('แชทกับเอเจนนี้')).toBeTruthy())
    await fireEvent.click(screen.getByText('แชทกับเอเจนนี้'))

    await waitFor(() => expect(vi.mocked(NewChairSession).mock.calls[0][0]).toBe('doc'))
    expect(cockpit.activeView).toBe('chat')
    expect(cockpit.chair).toBe('doc')
    expect(cockpit.desk).toBe('specialized')
  })
})

describe('the received-work feed', () => {
  it('shows what came in, from whom, and what it cost', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([job()] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ทำเอกสารสรุปยอดเดือนนี้')).toBeTruthy())
    expect(screen.getByText('เรียกเครื่องมือ 2 ครั้ง')).toBeTruthy()
    expect(screen.getByText('4.2s')).toBeTruthy()
  })

  // The job row carries the caller's session id, and that is the only link
  // between a delivered file and the conversation that asked for it.
  it('walks back to the chat that sent the job', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([job()] as any)
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ไปที่แชท')).toBeTruthy())
    await fireEvent.click(screen.getByText('ไปที่แชท'))

    await waitFor(() => expect(vi.mocked(LoadSessionAnyProject).mock.calls[0][0]).toBe('20260805-090000.000'))
    expect(cockpit.activeView).toBe('chat')
  })

  it('has an empty state that says what to do rather than nothing', async () => {
    render(Office, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText(/ยังไม่มีงานส่งเข้ามา/)).toBeTruthy())
  })
})
