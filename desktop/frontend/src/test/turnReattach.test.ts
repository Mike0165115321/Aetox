// A turn outlives the window that started it, and a window outlives the wish
// to stay in one chat. Two bugs shipped as one: reloading mid-turn reset the
// window to idle over a working agent (the reply's only route back was the
// dead webview's promise), and switching chats mid-turn carried the answer
// into the newly opened conversation. The fixes this file pins: the reloaded
// window re-arms from TurnInFlight and gets its ending from agent:done, and
// every door out of a running turn's chat refuses with a sentence instead of
// obeying.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, loadRealState, applyAgentDone, selectGlobalSession, newSession,
  deleteSession, sendUserMessage,
} from '../lib/stores/cockpit.svelte'
import {
  TurnInFlight, CurrentSessionID, SessionTranscript, LoadSessionAnyProject,
  NewSession, DeleteSession, SendMessage, GetModelInfo,
} from './mocks/wailsApp'

const question = { id: 1, role: 'user', text: 'ไล่บั๊คให้หน่อย', time: '10:00' }
const answer = { id: 2, role: 'agent', text: 'เจอแล้วครับ', time: '10:05' }

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.awaitingReply = false
  cockpit.sessionError = ''
  cockpit.streamingText = ''
})

// loadRealState with the engine reporting a turn still running in the current
// session — the state a webview reload lands in mid-answer.
async function reloadMidTurn(): Promise<void> {
  vi.mocked(GetModelInfo).mockResolvedValue({ ...await GetModelInfo(), provider: 'aetox' } as never)
  vi.mocked(CurrentSessionID).mockResolvedValue('s1')
  vi.mocked(TurnInFlight).mockResolvedValue({ running: true, sessionId: 's1' } as never)
  vi.mocked(SessionTranscript).mockResolvedValue([question] as never)
  await loadRealState()
}

describe('a window reloaded while the agent is working', () => {
  it('shows the question again and re-arms the live block', async () => {
    await reloadMidTurn()

    expect(cockpit.chat.map((m) => m.text)).toEqual(['ไล่บั๊คให้หน่อย'])
    // awaitingReply back on is the whole re-attach: the streaming block
    // renders again, typing goes into the running turn, and Stop works.
    expect(cockpit.awaitingReply).toBe(true)
  })

  it('receives the finished answer through agent:done', async () => {
    await reloadMidTurn()
    vi.mocked(SessionTranscript).mockResolvedValue([question, answer] as never)

    await applyAgentDone({ sessionId: 's1' })

    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.chat.map((m) => m.text)).toEqual(['ไล่บั๊คให้หน่อย', 'เจอแล้วครับ'])
  })

  // The turn can end in the exact moment the reload is re-arming: agent:done
  // fires before the flag is up, into a window that skips it, and nothing else
  // would ever take awaitingReply back down. The restore rechecks once after
  // arming for precisely this.
  it('closes the turn that finished while the reload was re-arming', async () => {
    vi.mocked(GetModelInfo).mockResolvedValue({ ...await GetModelInfo(), provider: 'aetox' } as never)
    vi.mocked(CurrentSessionID).mockResolvedValue('s1')
    vi.mocked(TurnInFlight)
      .mockResolvedValueOnce({ running: true, sessionId: 's1' } as never)
      .mockResolvedValueOnce({ running: false, sessionId: '' } as never)
    vi.mocked(SessionTranscript).mockResolvedValue([question, answer] as never)

    await loadRealState()

    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.chat.map((m) => m.text)).toEqual(['ไล่บั๊คให้หน่อย', 'เจอแล้วครับ'])
  })

  // The window that sent the message still has the promise; the event handler
  // acting there too would deliver every answer twice.
  it('ignores agent:done when it still holds the promise', async () => {
    cockpit.chat = [{ role: 'user', text: 'q', time: '10:00' }]
    cockpit.awaitingReply = true

    await applyAgentDone({ sessionId: 's1' })

    expect(cockpit.awaitingReply).toBe(true)
    expect(vi.mocked(SessionTranscript)).not.toHaveBeenCalled()
  })
})

describe('the doors out of a running turn\'s chat', () => {
  it('refuses to open another chat, with a sentence instead of a dead click', async () => {
    cockpit.awaitingReply = true

    await selectGlobalSession({ id: 'other', title: '', ago: '' })

    expect(vi.mocked(LoadSessionAnyProject)).not.toHaveBeenCalled()
    expect(cockpit.sessionError).toContain('กำลังทำงานอยู่')
  })

  it('refuses a new session the same way', async () => {
    cockpit.awaitingReply = true

    await newSession()

    expect(vi.mocked(NewSession)).not.toHaveBeenCalled()
    expect(cockpit.sessionError).toContain('กำลังทำงานอยู่')
  })

  // Only the chat the turn is writing into is protected. Freezing the whole
  // history list for the length of a long turn would be a different bug.
  it('still deletes a chat the turn is not in', async () => {
    cockpit.awaitingReply = true

    await deleteSession({ id: 'old', title: '', ago: '', active: false })
    expect(vi.mocked(DeleteSession)).toHaveBeenCalledWith('old')

    await deleteSession({ id: 's1', title: '', ago: '', active: true })
    expect(vi.mocked(DeleteSession)).not.toHaveBeenCalledWith('s1')
  })

  // The engine's own refusal (the boot moment before awaitingReply is re-armed,
  // a desk file gone) used to die as an unhandled rejection — a click that
  // visibly did nothing. Every refusal is a written sentence; show it.
  it('surfaces an engine refusal instead of swallowing it', async () => {
    vi.mocked(NewSession).mockRejectedValue(new Error('เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน แล้วค่อยสลับแชท'))

    await newSession()

    expect(cockpit.sessionError).toContain('กำลังทำงานอยู่')
  })

  it('takes the banner down when the turn ends', async () => {
    vi.mocked(SendMessage).mockResolvedValue({ text: 'เสร็จแล้ว' } as never)
    cockpit.sessionError = 'เอเจนกำลังทำงานอยู่ — รอให้เสร็จ'

    await sendUserMessage('ต่อเลย')

    // The refusal it explained stopped being true the moment the turn closed.
    expect(cockpit.sessionError).toBe('')
  })
})
