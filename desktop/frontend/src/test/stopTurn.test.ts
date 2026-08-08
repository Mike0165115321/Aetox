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
