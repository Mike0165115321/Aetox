// The first message in a brand-new chat, and why it used to arrive in silence.
//
// Reported 2026-08-21/22 as "the chat page is frozen": the engine ran the turn,
// finished it and stored it, while the window showed no status, no tool rows and
// no streamed text until the answer landed at the end. Three times, always the
// first message of a new chat, never a chat that had been opened by clicking it.
//
// The cause was one link in a fallback chain — `sessions.find((s) => s.active)`
// — answering "which session did the engine last hold" to a question that asked
// "which chat is the user in". forLiveTurn drops on mismatch, so the wrong
// answer did not misplace the turn's progress, it deleted it.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage, applyToolEvent, applyAgentStatus } from '../lib/stores/cockpit.svelte'
import { SendMessage, CurrentSessionID, PendingUndo } from './mocks/wailsApp'

const NEW_SESSION = '20260822-025736.000'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.toolSteps = []
  cockpit.backgroundSteps = []
  cockpit.agentStatus = ''
  cockpit.turnSession = ''
  cockpit.parked = {}
  vi.mocked(PendingUndo).mockResolvedValue([] as any)
})

describe('the first message of a new chat', () => {
  it('shows the turn working, with yesterday’s session still marked active', async () => {
    // The window is on a chat it has not been told the id of yet, and the
    // restored list still flags the previous one.
    cockpit.openSession = ''
    cockpit.sessions = [{ id: '20260821-121123.124', active: true }] as any
    vi.mocked(CurrentSessionID).mockResolvedValue('' as any) // Go creates it on send

    let statusMidTurn = ''
    let stepsMidTurn = -1
    vi.mocked(SendMessage).mockImplementation(async () => {
      // Everything the engine emits carries the session it just created.
      applyAgentStatus({ sessionId: NEW_SESSION, data: 'กำลังคิดคำตอบ...' } as any)
      applyToolEvent({
        sessionId: NEW_SESSION,
        data: { action: 'call', ref: 'c1', name: 'edit', subject: 'README.md' },
      } as any)
      statusMidTurn = cockpit.agentStatus
      stepsMidTurn = cockpit.toolSteps.length
      return { text: 'เรียบร้อยครับ', parts: [] } as any
    })

    await sendUserMessage('เทสระบบ')

    expect(statusMidTurn).toBe('กำลังคิดคำตอบ...')
    expect(stepsMidTurn).toBe(1)
  })

  // The chat on screen still wins whenever the window knows it — that is what
  // keeps a turn started here from being routed into the chat running there.
  it('still belongs to the chat on screen when the window knows which it is', async () => {
    cockpit.openSession = 'on-screen'
    cockpit.sessions = [{ id: 'somebody-else', active: true }] as any
    vi.mocked(CurrentSessionID).mockResolvedValue('somebody-else' as any)

    let stampedWith = ''
    vi.mocked(SendMessage).mockImplementation(async () => {
      stampedWith = cockpit.turnSession
      return { text: 'ok', parts: [] } as any
    })

    await sendUserMessage('hi')

    expect(stampedWith).toBe('on-screen')
  })
})
