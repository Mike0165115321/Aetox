// The Stop button's aftermath. Pressing it is a command that succeeded, not an
// error — but the engine reports the cancelled turn as `context canceled`, and
// the catch-all bubble read that back as "เกิดข้อผิดพลาด: context canceled".
// Worse, whatever the model had already streamed vanished with the live panel.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage } from '../lib/stores/cockpit.svelte'
import { SendMessage } from './mocks/wailsApp'

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
  })
})
