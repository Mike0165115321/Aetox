// The Stop button's aftermath. Pressing it is a command that succeeded, not an
// error — but the engine reports the cancelled turn as `context canceled`, and
// the catch-all bubble read that back as "เกิดข้อผิดพลาด: context canceled".
// Worse, whatever the model had already streamed vanished with the live panel.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage } from '../lib/stores/cockpit.svelte'
import { SendMessage, SessionTranscript } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.awaitingReply = false
  cockpit.streamingText = ''
  cockpit.toolSteps = []
  cockpit.reasoningText = ''
})

describe('pressing Stop mid-turn', () => {
  it('says stopped — not error — and keeps what had already streamed', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      // The engine streamed half an answer before the press killed the turn.
      cockpit.streamingText = 'กำลังไล่ดูโค้ดให้ครับ พบว่า'
      throw new Error('Post "https://provider/chat": context canceled')
    })

    await sendUserMessage('ช่วยไล่บั๊คที')

    const last = cockpit.chat.at(-1)
    expect(last?.text).toContain('กำลังไล่ดูโค้ดให้ครับ พบว่า')
    expect(last?.text).toContain('หยุดการทำงานแล้ว')
    expect(last?.text).not.toContain('เกิดข้อผิดพลาด')
    // Still retryable: a stopped question is one the user may want re-run.
    expect(last?.failed).toBe(true)
    expect(last?.failedText).toBe('ช่วยไล่บั๊คที')
    // And marked as the user's own doing, so the chip under it is drawn in the
    // ordinary colour instead of the one the app uses for a crash.
    expect(last?.stopped).toBe(true)
  })

  // The case that actually happens. Nobody presses Stop mid-sentence; they press
  // it while a tool is running — and by then the engine has already erased the
  // streamed preview for that round, so the bubble's only surviving copy of the
  // turn is the timeline. It used to be cleared one line later, leaving a chat
  // that read as if the agent had done nothing at all.
  it('keeps the tool timeline of a turn stopped between rounds', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      cockpit.toolSteps = [
        { label: 'shell รอ Backend', state: 'done', startedAt: Date.now() },
        { kind: 'note', label: 'กำลังตรวจว่า Backend พร้อมหรือยัง', state: 'done', startedAt: Date.now() },
        { label: 'read api/main.py', state: 'run', startedAt: Date.now() },
      ]
      cockpit.reasoningText = 'ไล่ดูพอร์ตก่อน'
      // The round ended in a tool call, so the preview is empty by construction.
      cockpit.streamingText = ''
      throw new Error('context canceled')
    })

    await sendUserMessage('เปิดโปรเจกต์ให้ที')

    const last = cockpit.chat.at(-1)
    expect(last?.text).toBe('หยุดการทำงานแล้ว')
    expect(last?.steps?.map((s) => s.label)).toEqual([
      'shell รอ Backend', 'กำลังตรวจว่า Backend พร้อมหรือยัง', 'read api/main.py',
    ])
    expect(last?.reasoning).toBe('ไล่ดูพอร์ตก่อน')
  })

  it('says only stopped when nothing had streamed yet', async () => {
    vi.mocked(SendMessage).mockRejectedValue(new Error('context canceled'))

    await sendUserMessage('เริ่มงานใหญ่')

    expect(cockpit.chat.at(-1)?.text).toBe('หยุดการทำงานแล้ว')
  })

  it('still reports a real failure as an error', async () => {
    vi.mocked(SendMessage).mockRejectedValue(new Error('connection refused'))

    await sendUserMessage('ทำต่อ')

    const last = cockpit.chat.at(-1)
    expect(last?.text).toContain('เกิดข้อผิดพลาด')
    expect(last?.text).toContain('connection refused')
    // The half of the split that must stay red: nobody pressed anything here.
    expect(last?.failed).toBe(true)
    expect(last?.stopped).toBe(false)
  })
})

// The work a stopped turn had already done, which the window kept throwing away.
//
// `App.runTurn` builds the agent message once for BOTH endings — same reply,
// same reasoning, same parts — and `appendFailedTurn` stores it, so a turn
// stopped after sixteen rounds has sixteen parts in the database. None of it
// reached the bubble: Wails discards a return value when the error is non-nil,
// so the window fell back to `streamingText`, which holds only the round being
// written now and which the engine erases at the end of every round that ends
// in a tool call. That is exactly when somebody presses Stop.
//
// Owner, 22 ส.ค., watching it happen: "แชทมันจะหายไปเลย ทั้งที่เมื่อกี้มันคิดมายาวมาก".
describe('what a stopped turn leaves behind', () => {
  // A chat that exists, which is what the store is read by. Without one the
  // lookup has no session to ask about and falls straight through to the live
  // snapshot, which is the path these two tests exist to get past.
  beforeEach(() => { cockpit.openSession = 'sess_1' })

  const storedRow = (over: Record<string, unknown> = {}) => ({
    role: 'agent', text: 'PowerShell ไม่มี `pwd`/`ls` แบบนั้น', time: '05:53',
    errorText: 'context canceled',
    parts: [
      { kind: 'text', text: 'ผมจะช่วยทำสไลด์เรื่องกาแฟสามคลื่นให้ครับ' },
      { kind: 'tool', tool: { ref: 'call_1', name: 'skills_list', ok: true } },
      { kind: 'text', text: 'มี python-pptx พร้อมใช้งาน' },
      { kind: 'tool', tool: { ref: 'call_2', name: 'shell', subject: 'ตรวจโฟลเดอร์', ok: true } },
    ],
    ...over,
  })

  it('reads the turn back from the store instead of from an erased preview', async () => {
    vi.mocked(SessionTranscript).mockResolvedValue([
      { role: 'user', text: 'ทำสไลด์กาแฟสามคลื่น', time: '05:50' },
      storedRow(),
    ] as any)
    vi.mocked(SendMessage).mockImplementation(async () => {
      // Empty, because the engine discarded the preview at the end of the round
      // that called a tool. This is the real state at the moment of a Stop.
      cockpit.streamingText = ''
      throw new Error('context canceled')
    })

    await sendUserMessage('ทำสไลด์กาแฟสามคลื่น')

    const last = cockpit.chat.at(-1)
    // What he was reading when he pressed the button, still there.
    expect(last?.text).toContain('PowerShell')
    expect(last?.text).toContain('หยุดการทำงานแล้ว')
    // And the rounds behind it: the narration and the tool calls both.
    expect(last?.steps?.length).toBeGreaterThan(0)
    expect(JSON.stringify(last?.steps)).toContain('skills_list')
    // Still retryable — a stopped question is one the user may want re-run.
    expect(last?.failed).toBe(true)
    expect(last?.failedText).toBe('ทำสไลด์กาแฟสามคลื่น')
    // Read back through the store, and still the user's own Stop.
    expect(last?.stopped).toBe(true)
  })

  it('will not answer this question with an older turn`s wreckage', async () => {
    // The rejection never reached appendFailedTurn (engine gone, Wails itself),
    // so the newest failed row belongs to a turn nobody asked about now.
    vi.mocked(SessionTranscript).mockResolvedValue([
      { role: 'user', text: 'คำถามเมื่อวาน', time: '09:00' },
      storedRow({ text: 'งานของเมื่อวาน' }),
    ] as any)
    vi.mocked(SendMessage).mockImplementation(async () => {
      cockpit.streamingText = 'กำลังอ่านไฟล์'
      throw new Error('context canceled')
    })

    await sendUserMessage('คำถามวันนี้')

    const last = cockpit.chat.at(-1)
    expect(last?.text).not.toContain('งานของเมื่อวาน')
    expect(last?.text).toContain('กำลังอ่านไฟล์')
    expect(last?.failedText).toBe('คำถามวันนี้')
  })
})
