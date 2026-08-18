// A turn outlives the window that started it, and a window outlives the wish
// to stay in one chat. Two bugs shipped as one: reloading mid-turn reset the
// window to idle over a working agent (the reply's only route back was the
// dead webview's promise), and switching chats mid-turn carried the answer
// into the newly opened conversation. The fixes this file pins: the reloaded
// window re-arms from TurnInFlight and gets its ending from agent:done, and
// every door out of a running turn's chat answers with a sentence instead of
// obeying silently.
//
// "Refuses" was the whole answer until 18 Aug 2026, and it was one word too
// wide. The doors that re-root the engine (new session, new project, new desk)
// still refuse, because a turn cannot have its memory rewritten underneath it.
// Opening another chat no longer does: it opens for reading, the working chat
// is held, and the answer lands where it was asked.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, loadRealState, applyAgentDone, selectGlobalSession, newSession,
  deleteSession, sendUserMessage, leavePeek, regenerateReply,
} from '../lib/stores/cockpit.svelte'
import {
  TurnInFlight, CurrentSessionID, SessionTranscript, LoadSessionAnyProject,
  NewSessionAt, DeleteSession, SendMessage, GetModelInfo, RegenerateReply,
} from './mocks/wailsApp'

const question = { id: 1, role: 'user', text: 'ไล่บั๊คให้หน่อย', time: '10:00' }
const answer = { id: 2, role: 'agent', text: 'เจอแล้วครับ', time: '10:05' }

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.awaitingReply = false
  cockpit.sessionError = ''
  cockpit.streamingText = ''
  cockpit.peek = null
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
  // Opening another chat for real is what the engine cannot survive mid-turn:
  // LoadSessionAnyProject re-roots the project and rebuilds the agent's memory.
  // Reading one costs nothing, so the click now opens it for reading — and the
  // half that is still refused is writing, not looking.
  it('opens another chat for reading instead of refusing', async () => {
    cockpit.chat = [{ role: 'user', text: 'งานที่กำลังทำ', time: '10:00' }]
    cockpit.awaitingReply = true
    vi.mocked(SessionTranscript).mockResolvedValue([question, answer] as never)

    await selectGlobalSession({ id: 'other', title: 'แชทเก่า', ago: '' })

    expect(vi.mocked(LoadSessionAnyProject)).not.toHaveBeenCalled()
    expect(vi.mocked(SessionTranscript)).toHaveBeenCalledWith('other')
    expect(cockpit.sessionError).toBe('')
    expect(cockpit.chat.map((m) => m.text)).toEqual(['ไล่บั๊คให้หน่อย', 'เจอแล้วครับ'])
    expect(cockpit.peek?.session.id).toBe('other')
    // The working chat is held, not dropped: it is where the answer must land.
    expect(cockpit.peek?.live.map((m) => m.text)).toEqual(['งานที่กำลังทำ'])
  })

  it('will not send from a chat that is only being read', async () => {
    cockpit.awaitingReply = true
    vi.mocked(SessionTranscript).mockResolvedValue([] as never)
    await selectGlobalSession({ id: 'other', title: '', ago: '' })

    await sendUserMessage('พิมพ์ผิดแชท')

    expect(vi.mocked(SendMessage)).not.toHaveBeenCalled()
    expect(cockpit.sessionError).toContain('เปิดอ่าน')
    expect(cockpit.chat).toEqual([])
  })

  // The peek outlives the turn: the user goes on reading after it ends. Every
  // door that acts on "the conversation on screen" has to stay shut for as long
  // as the conversation on screen is not the one the engine is in.
  it('keeps acting-on-this-chat shut after the turn has ended', async () => {
    cockpit.chat = [
      { role: 'user', text: 'คำถาม', time: '10:00' },
      { role: 'agent', text: 'คำตอบ', time: '10:01', variants: [{ text: 'คำตอบ' }] },
    ]
    vi.mocked(SessionTranscript).mockResolvedValue([question, answer] as never)
    cockpit.awaitingReply = true
    await selectGlobalSession({ id: 'other', title: '', ago: '' })
    // The turn ends while the user is still reading.
    cockpit.awaitingReply = false

    await regenerateReply(false)

    expect(vi.mocked(RegenerateReply)).not.toHaveBeenCalled()
    expect(cockpit.sessionError).toContain('เปิดอ่าน')
  })

  // The whole point of the read-only door: the turn goes on being the turn it
  // was, and its answer belongs to the conversation it was asked in — not to
  // whichever chat the user happened to open while waiting.
  it('lands the answer in the working chat, not the one being read', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      vi.mocked(SessionTranscript).mockResolvedValue([question] as never)
      await selectGlobalSession({ id: 'other', title: 'แชทเก่า', ago: '' })
      return { text: 'เสร็จแล้วครับ' } as never
    })

    await sendUserMessage('ทำงานยาวให้ที')

    // On screen: the chat being read, untouched by the turn.
    expect(cockpit.chat.map((m) => m.text)).toEqual(['ไล่บั๊คให้หน่อย'])
    expect(cockpit.peek?.live.map((m) => m.text)).toEqual(['ทำงานยาวให้ที', 'เสร็จแล้วครับ'])

    leavePeek()
    expect(cockpit.peek).toBe(null)
    expect(cockpit.chat.map((m) => m.text)).toEqual(['ทำงานยาวให้ที', 'เสร็จแล้วครับ'])
  })

  it('refuses a new session the same way', async () => {
    cockpit.awaitingReply = true

    await newSession()

    expect(vi.mocked(NewSessionAt)).not.toHaveBeenCalled()
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
    vi.mocked(NewSessionAt).mockRejectedValue(new Error('เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน แล้วค่อยสลับแชท'))

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

// The dead click, reproduced. A chat with no rows comes back from Go as a nil
// slice — `null` by the time it is here, never `[]` — and the map that followed
// threw inside an async function: an unhandled rejection, a row that stayed
// where it was, and nothing on screen saying why. Clicking a fresh chat while
// the agent worked simply did nothing.
describe('opening an empty chat while a turn runs', () => {
  it('opens it instead of dying quietly', async () => {
    cockpit.chat = [{ role: 'user', text: 'งานที่กำลังทำ', time: '10:00' }]
    cockpit.awaitingReply = true
    vi.mocked(SessionTranscript).mockResolvedValue(null as never)

    await selectGlobalSession({ id: 'fresh', title: 'แชทใหม่', ago: '' })

    expect(cockpit.peek?.session.id).toBe('fresh')
    expect(cockpit.chat).toEqual([])
    expect(cockpit.sessionError).toBe('')
    expect(cockpit.peek?.live.map((m) => m.text)).toEqual(['งานที่กำลังทำ'])
  })
})

// The regression the read-only door shipped with: the peek outlived the turn,
// nothing cleared it, and from then on every chat refused to send. Not the
// door doing nothing — the whole app, in every conversation, until restart.
describe('a reading that outlived its turn', () => {
  it('ends the moment a real switch happens, in every chat', async () => {
    cockpit.chat = [{ role: 'user', text: 'งานที่กำลังทำ', time: '10:00' }]
    cockpit.awaitingReply = true
    vi.mocked(SessionTranscript).mockResolvedValue([question] as never)
    await selectGlobalSession({ id: 'other', title: '', ago: '' })
    expect(cockpit.peek).not.toBe(null)

    // The turn finishes while the user is still reading, then they switch.
    cockpit.awaitingReply = false
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([question, answer] as never)
    await selectGlobalSession({ id: 'third', title: '', ago: '' })

    expect(cockpit.peek).toBe(null)

    // And the composer works again — which is the whole complaint.
    vi.mocked(SendMessage).mockResolvedValue({ text: 'ได้ครับ' } as never)
    await sendUserMessage('พิมพ์ได้ไหม')
    expect(vi.mocked(SendMessage)).toHaveBeenCalled()
    expect(cockpit.sessionError).toBe('')
  })

  it('ends when a new chat is opened too', async () => {
    cockpit.awaitingReply = true
    vi.mocked(SessionTranscript).mockResolvedValue([question] as never)
    await selectGlobalSession({ id: 'other', title: '', ago: '' })

    cockpit.awaitingReply = false
    await newSession()

    expect(cockpit.peek).toBe(null)
  })
})
