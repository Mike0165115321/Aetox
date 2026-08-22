// A turn that ran under an id the window did not expect still has to END.
//
// Reported 2026-08-22 with a screenshot: one reply drawn twice — once as a
// finished bubble with its copy/ลองใหม่ row, once again below it with none —
// over a Stop button that could not be dismissed. The engine was innocent; its
// log shows a single turn, a single answer, 2.6s, clean exit.
//
// The window had appended the reply to the transcript AND left the live block
// up holding the same text, because `runTurn`'s finally cleared
// `cockpit.turnSession` one line before `writeLive` needed to read it. The
// write landed nowhere, so `awaitingReply`, `streamingText` and `agentStatus`
// all survived a turn that was over.
//
// It bites the first message of a new chat, where CurrentSessionID() answers
// with the session Go still holds while the engine stamps the one it creates.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage, applyAgentStatus, applyAgentChunk } from '../lib/stores/cockpit.svelte'
import { SendMessage, CurrentSessionID, PendingUndo } from './mocks/wailsApp'

const ENGINE_STAMP = '20260822-071118.712'
const STALE = '20260822-070542.315'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.toolSteps = []
  cockpit.backgroundSteps = []
  cockpit.agentStatus = ''
  cockpit.streamingText = ''
  cockpit.turnSession = ''
  cockpit.parked = {}
  vi.mocked(PendingUndo).mockResolvedValue([] as any)
})

describe('a turn under an unexpected engine stamp', () => {
  // The window is opening a brand-new chat, so the id it can name is the one
  // the engine held a moment ago, not the one it is about to stamp with.
  const openUnderAStaleId = () => {
    cockpit.openSession = STALE
    cockpit.sessions = [{ id: STALE, active: true }] as any
    vi.mocked(CurrentSessionID).mockResolvedValue(STALE as any)
  }

  it('takes the live block down, so the reply is drawn once', async () => {
    openUnderAStaleId()
    const reply = 'พร้อมทำงานครับ! มีอะไรให้ผมช่วยดูแลหรือพัฒนาก็บอกมาได้เลยครับ'
    vi.mocked(SendMessage).mockImplementation(async () => {
      applyAgentStatus({ sessionId: ENGINE_STAMP, data: 'กำลังคิดคำตอบ...' } as any)
      // The finished reply, delivered as the authority over whatever streamed.
      applyAgentChunk({ sessionId: ENGINE_STAMP, data: { text: reply, replace: true } } as any)
      return { text: reply, parts: [] } as any
    })

    await sendUserMessage('เทสครับทำงานได้ไหม')

    expect(cockpit.streamingText).toBe('')

    const drawn = cockpit.chat.filter((m) => m.text === reply)
    expect(drawn).toHaveLength(1)
  })

  it('lets go of the Stop button and the thinking indicator', async () => {
    openUnderAStaleId()
    vi.mocked(SendMessage).mockImplementation(async () => {
      applyAgentStatus({ sessionId: ENGINE_STAMP, data: 'กำลังคิดคำตอบ...' } as any)
      return { text: 'ok', parts: [] } as any
    })

    await sendUserMessage('เทส')

    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.agentStatus).toBe('')
  })

  // The same ending, for the turn that failed rather than the one that worked.
  // This is the shape the owner hit first: a red bubble with "กำลังคิดคำตอบ..."
  // still turning underneath it.
  it('ends the same way when the turn fails', async () => {
    openUnderAStaleId()
    vi.mocked(SendMessage).mockImplementation(async () => {
      applyAgentStatus({ sessionId: ENGINE_STAMP, data: 'กำลังคิดคำตอบ...' } as any)
      throw new Error("codex: the free plan's limit is used up. It resets in 19 days.")
    })

    await sendUserMessage('เช็คโค้ด')

    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.agentStatus).toBe('')
    expect(cockpit.streamingText).toBe('')
  })
})
