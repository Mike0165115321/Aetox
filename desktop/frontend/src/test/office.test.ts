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
  cockpit.settingsIntent = null
  vi.mocked(ListChairs).mockResolvedValue([chair()] as any)
  vi.mocked(ListReceivedJobs).mockResolvedValue([] as any)
})

describe('the office roster', () => {
  // A card is a face, not an inventory (2026-08-07). The tool chips were six
  // per card and five of the six were identical on every card — the office
  // ceiling hands everyone the same set — so the list took half the card to say
  // nothing about who anyone is. What the card answers now is who this is and
  // what they make; the tools moved to the editor behind the gear, which is
  // also the only place they can be changed.
  it('shows who a chair is, and no longer lists their tools', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair({ tools: ['doc_write', 'read'] })] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getAllByText('doc').length).toBeGreaterThan(0))
    expect(screen.getByText('เก้าอี้ร่างเอกสาร')).toBeTruthy()
    expect(screen.queryByText('doc_write')).toBeNull()
    expect(screen.queryByText('read')).toBeNull()
  })

  // Every agent arrives with a face, including one whose profile names no icon
  // — the roster derives it from what they produce (desktop/office.go). A card
  // that could render blank would be the feature shipping broken for everyone
  // who never opens the editor.
  it('draws a mark for every chair', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair({ icon: 'fileText' })] as any)
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(container.querySelector('.chair-face svg')).toBeTruthy())
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

  // The gallery's first card is the create door. It opens the shared profile
  // editor with kind=agent carried in the intent — from this roster, kind is
  // known by construction, and the editor must never re-derive it from a file.
  it('offers a create-agent card that opens the editor as an agent', async () => {
    render(Office, { onClose: () => {} })
    const card = await screen.findByText('สร้างตัวแทนใหม่')

    await fireEvent.click(card)

    expect(cockpit.settingsIntent).toEqual({ section: 'agents', createAgent: true })
    expect(cockpit.activeView).toBe('settings')
  })

  // Configure on the card: the agent's desk is its home page, and the gear is
  // its own door into the one editor.
  it('sends a card\'s gear to the editor with that agent\'s name', async () => {
    render(Office, { onClose: () => {} })
    await screen.findByText('เก้าอี้ร่างเอกสาร')

    const gear = screen.getAllByLabelText('ตั้งค่า')[0]
    await fireEvent.click(gear)

    expect(cockpit.settingsIntent).toEqual({ section: 'agents', agent: 'doc' })
    expect(cockpit.activeView).toBe('settings')
  })

  // Walking into the room (§85): the card's chat button opens a session bound
  // to that agent, and the view moves to the chat that session now owns.
  it('opens a direct chat with the agent on its card', async () => {
    vi.mocked(NewChairSession).mockResolvedValue('20260805-100000.000' as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('แชทกับตัวแทนนี้')).toBeTruthy())
    await fireEvent.click(screen.getByText('แชทกับตัวแทนนี้'))

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
