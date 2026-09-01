// เอเจนเฉพาะทาง (COMPANY.md §4). The page is a roster and a feed, and the one
// claim worth pinning is what the roster shows: the tools each chair *gets*,
// after the office ceiling — not the ones its file asked for. This is the row a
// person checks the ceiling on, so a page that echoed the request back would
// quietly defeat the structure it is reporting on.
//
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Office from '../lib/Office.svelte'
import {
  ListChairs, ListReceivedJobs, LoadSessionAnyProject, NewChairSession,
  DelegateSwitches, SetAgentOff,
} from './mocks/wailsApp'
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

    await waitFor(() => expect(container.querySelector('.agent-face svg')).toBeTruthy())
  })

  // The half of that promise the icon cannot keep. A face is drawn from the
  // NAME, and the prop the agent holds is the only part `icon:` decides — so a
  // profile that names none still arrives as somebody rather than an empty
  // square. This is the case that made a drawn face worth having over a stored
  // picture: it is the shape of every agent a user writes themselves.
  it('draws a face for a chair whose profile names no icon', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair({ icon: '' })] as any)
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(container.querySelector('.agent-face svg')).toBeTruthy())
  })

  it('says plainly when a chair has never been handed anything', async () => {
    render(Office, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText('ยังไม่เคยรับงาน')).toBeTruthy())
  })

  it('counts the work a chair has done', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair({ jobs: 3, lastUsed: new Date().toISOString() })] as any)
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('3')).toBeTruthy())
    // In the card's body since 30 ส.ค., not in the foot. It is a fact about the
    // agent like the sentence above it; the foot is the card's actions, and a
    // number sharing that row is what kept the chat button down to an icon.
    expect(container.querySelector('.chair-stat')?.textContent).toMatch(/3\s*งาน/)
  })

  // The hiring door is the section's own control rather than a card in the
  // grid. It opens the shared profile editor with kind=agent carried in the
  // intent — from this roster, kind is known by construction, and the editor
  // must never re-derive it from a file.
  it('offers a create-agent door that opens the editor as an agent', async () => {
    render(Office, { onClose: () => {} })
    const door = await screen.findByText('เพิ่มเอเจนเฉพาะทาง')

    await fireEvent.click(door)

    // 'team' is เอเจน. Naming the ซับเอเจน page here still opened the right
    // editor — the handler forces the kind — so the bug hid behind a correct
    // form and only showed itself when the user closed it.
    expect(cockpit.settingsIntent).toEqual({ section: 'team', createAgent: true })
    expect(cockpit.activeView).toBe('settings')
  })

  // Configure on the card: the agent's desk is its home page, and the gear is
  // its own door into the one editor.
  it('sends a card\'s gear to the editor with that agent\'s name', async () => {
    render(Office, { onClose: () => {} })
    await screen.findByText('เก้าอี้ร่างเอกสาร')

    const gear = screen.getAllByLabelText('ตั้งค่า')[0]
    await fireEvent.click(gear)

    expect(cockpit.settingsIntent).toEqual({ section: 'team', agent: 'doc' })
    expect(cockpit.activeView).toBe('settings')
  })

  // Walking into the room (§85): the card's chat button opens a session bound
  // to that agent, and the view moves to the chat that session now owns.
  // In words, and naming the agent. It was a 13px sparkles icon sharing the
  // foot with the job count until 30 ส.ค. — the smallest thing on the card,
  // wearing a mark that means "chat" to nobody, on a page whose whole purpose
  // is walking in and talking to a specialist. Read by its visible text here on
  // purpose: an aria-label would pass this test with the icon back.
  it('opens a direct chat from a button that says so in words', async () => {
    vi.mocked(NewChairSession).mockResolvedValue('20260805-100000.000' as any)
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('คุยกับ doc')).toBeTruthy())
    expect(container.querySelector('.chair-talk')?.textContent?.trim()).toBe('คุยกับ doc')
    await fireEvent.click(screen.getByText('คุยกับ doc'))

    await waitFor(() => expect(vi.mocked(NewChairSession).mock.calls[0][0]).toBe('doc'))
    expect(cockpit.activeView).toBe('chat')
    expect(cockpit.chair).toBe('doc')
    expect(cockpit.desk).toBe('specialized')
  })
})

// Whether the main assistant may hand each teammate work — the one thing that
// decides if anyone on this page ever gets used, and until 31 ส.ค. the roster
// could not say it. It lived on the settings page as a column of switches over
// a list of rows, which meant the page you open to LOOK at your team and the
// page that decides whether the team works were two different pages.
describe('the roster and delegation', () => {
  const switches = (workers: { name: string; on: boolean }[], off = false) => ({
    agents: { off, tokens: 0, workers },
    helpers: { off: false, tokens: 0, workers: [] },
    tokens: 0,
  })

  it('splits the roster by whether the assistant can reach each agent', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair(), chair({ name: 'sheet' })] as any)
    vi.mocked(DelegateSwitches).mockResolvedValue(
      switches([{ name: 'doc', on: true }, { name: 'sheet', on: false }]) as any,
    )
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('อยู่ในมือผู้ช่วยหลัก')).toBeTruthy())
    expect(screen.getByText('ยังไม่ได้เปิด')).toBeTruthy()
    // Two decks, one agent each, and the band an agent sits in is its state —
    // which is why no card carries a badge saying the same thing twice.
    const decks = container.querySelectorAll('.office-grid')
    expect(decks.length).toBe(2)
    expect(decks[0].textContent).toContain('doc')
    expect(decks[1].textContent).toContain('sheet')
  })

  // An undelegated card looks exactly like a delegated one (owner, 31 ส.ค.):
  // *"มันเหมือนไม่เปิดใช้งาน ทั้งที่มันก็แชทได้ปกติ"*. The card used to cool and
  // carry a switch in the off position, which said "disabled" twice about an
  // agent whose chat opens normally — and the drawn face made it worse, because
  // a person with the colour pulled out of them reads as gone rather than as
  // undelegated. The band heading above the card is where that state lives now,
  // and it is the only place it is drawn.
  it('draws an undelegated card exactly like a delegated one', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair(), chair({ name: 'sheet' })] as any)
    vi.mocked(DelegateSwitches).mockResolvedValue(
      switches([{ name: 'doc', on: true }, { name: 'sheet', on: false }]) as any,
    )
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(container.querySelectorAll('.chair-card.agc').length).toBe(2))
    expect(container.querySelectorAll('.chair-card.agc.off').length).toBe(0)
    expect(container.querySelectorAll('.agent-face.off').length).toBe(0)
    expect(screen.getByText('คุยกับ sheet')).toBeTruthy()
  })

  // The state left the card but not the page: it is the band, with a count and
  // a line saying the chat still opens. Losing this while the switch was being
  // taken out would leave the roster unable to answer the question at all.
  it('still says which agents the assistant may hand work to', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair(), chair({ name: 'sheet' })] as any)
    vi.mocked(DelegateSwitches).mockResolvedValue(
      switches([{ name: 'doc', on: true }, { name: 'sheet', on: false }]) as any,
    )
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(container.querySelectorAll('.office-grid').length).toBe(2))
    expect(screen.getByText('อยู่ในมือผู้ช่วยหลัก')).toBeTruthy()
    expect(screen.getByText('ยังไม่ได้เปิด')).toBeTruthy()
  })

  // The per-agent switch itself moved to Settings › เอเจน, where the gear on
  // each card already goes. Its wiring is tested there (Settings.test.ts,
  // "hands the agent switch straight to the delegation setting") — this page no
  // longer draws one, and that is what is asserted here.
  it('draws no per-agent switch on a card', async () => {
    vi.mocked(ListChairs).mockResolvedValue([chair()] as any)
    vi.mocked(DelegateSwitches).mockResolvedValue(switches([{ name: 'doc', on: true }]) as any)
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(container.querySelector('.chair-card.agc')).toBeTruthy())
    expect(container.querySelector('.chair-card .mswitch')).toBeNull()
  })

  // The roster's whole job is showing who works here, and it can do that job
  // without the switches. When they cannot be read the page is exactly what it
  // was before today: one deck, no bands, no control it cannot honour.
  it('still draws the roster when the switches cannot be read', async () => {
    // Said here rather than left to the fixture: clearAllMocks resets calls,
    // not implementations, so a resolved value set by the test above would
    // still be in place and this one would quietly assert nothing.
    vi.mocked(DelegateSwitches).mockRejectedValue(new Error('unavailable'))
    const { container } = render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('เก้าอี้ร่างเอกสาร')).toBeTruthy())
    expect(container.querySelectorAll('.office-grid').length).toBe(1)
    expect(screen.queryByText('อยู่ในมือผู้ช่วยหลัก')).toBeNull()
    expect(container.querySelector('.mswitch')).toBeNull()
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
  // between a delivered file and the conversation that asked for it. The whole
  // row is the door — a boxed button repeated down the right edge was the
  // loudest thing on a page whose subject is the line beside it.
  //
  // Since §158 that walk crosses a door: the caller is usually the assistant
  // and this room is behind ทีม. Nothing here has to know that — loading a
  // session re-reads its desk and the door follows (refreshDesk).
  it('walks back to the chat that sent the job', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([job()] as any)
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByLabelText('ไปที่แชท')).toBeTruthy())
    await fireEvent.click(screen.getByLabelText('ไปที่แชท'))

    await waitFor(() => expect(vi.mocked(LoadSessionAnyProject).mock.calls[0][0]).toBe('20260805-090000.000'))
    expect(cockpit.activeView).toBe('chat')
  })

  // A duration is only worth a slot when it says something. Every row printing
  // "0.0s" was six copies of "this was instant" competing with the line that
  // says what the job actually was.
  it('leaves out a duration too small to mean anything', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([job({ durationMs: 40 })] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ทำเอกสารสรุปยอดเดือนนี้')).toBeTruthy())
    expect(screen.queryByText('0.0s')).toBeNull()
  })

  // Grouped by calendar day, so the eye can skip a day it does not want —
  // rather than reading "2 วัน" printed once per row all the way down.
  it('files the feed under the day it came in', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([job()] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('วันนี้')).toBeTruthy())
  })

  // The filter is the question this list is asked once more than one teammate
  // has worked, and furniture before that.
  it('filters the feed by teammate', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([
      job(), job({ id: 2, chair: 'sheet', brief: 'รวมยอดค่าใช้จ่าย' }),
    ] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ทั้งหมด')).toBeTruthy())
    await fireEvent.click(screen.getByText('sheet'))

    expect(screen.getByText('รวมยอดค่าใช้จ่าย')).toBeTruthy()
    expect(screen.queryByText('ทำเอกสารสรุปยอดเดือนนี้')).toBeNull()
  })

  it('does not draw a filter when one teammate is the whole feed', async () => {
    vi.mocked(ListReceivedJobs).mockResolvedValue([job()] as any)
    render(Office, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ทำเอกสารสรุปยอดเดือนนี้')).toBeTruthy())
    expect(screen.queryByText('ทั้งหมด')).toBeNull()
  })

  it('has an empty state that says what to do rather than nothing', async () => {
    render(Office, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText(/ยังไม่มีงานส่งเข้ามา/)).toBeTruthy())
  })
})
