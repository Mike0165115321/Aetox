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
  deleteSession, sendUserMessage,
} from '../lib/stores/cockpit.svelte'
import {
  TurnInFlight, CurrentSessionID, SessionTranscript, LoadSessionAnyProject,
  NewSessionAt, DeleteSession, SendMessage, GetModelInfo, Interject,
} from './mocks/wailsApp'

const question = { id: 1, role: 'user', text: 'ไล่บั๊คให้หน่อย', time: '10:00' }
const answer = { id: 2, role: 'agent', text: 'เจอแล้วครับ', time: '10:05' }

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.awaitingReply = false
  cockpit.sessionError = ''
  cockpit.streamingText = ''
  cockpit.parked = {}
  cockpit.openSession = ''
  cockpit.turnSession = ''
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
    // And which chat it belongs to, taken from the engine's answer rather than
    // guessed from the list — the fresh window has no other way to know, and
    // without it every row in the sidebar leads away from the work.
    expect(cockpit.turnSession).toBe('s1')
  })

  it('receives the finished answer through agent:done', async () => {
    await reloadMidTurn()
    vi.mocked(SessionTranscript).mockResolvedValue([question, answer] as never)

    await applyAgentDone({ sessionId: 's1' })

    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.turnSession).toBe('')
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

describe('working in one chat while another one runs', () => {
  // The read-only peek is gone. It was the honest half-answer for a day: you
  // could LOOK at another chat while a turn ran, and the composer was locked
  // with a bar explaining why. What the owner asked for three times was the
  // other half - type in it - and that needs the working chat's whole live
  // state to travel with it rather than one field of it.
  it('really opens the other chat and parks the working one', async () => {
    cockpit.openSession = 's1'
    cockpit.turnSession = 's1'
    cockpit.chat = [{ role: 'user', text: 'งานที่กำลังทำ', time: '10:00' }]
    cockpit.streamingText = 'กำลังเขียน…'
    cockpit.awaitingReply = true
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([question, answer] as never)

    await selectGlobalSession({ id: 'other', title: 'แชทเก่า', ago: '' })

    // Opened for real, not read.
    expect(vi.mocked(LoadSessionAnyProject)).toHaveBeenCalledWith('other')
    expect(cockpit.openSession).toBe('other')
    expect(cockpit.chat.map((m) => m.text)).toEqual(['ไล่บั๊คให้หน่อย', 'เจอแล้วครับ'])
    // The arriving chat gets a clean live state - none of the other one's work.
    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.streamingText).toBe('')
    // And the working chat kept everything, not just its messages.
    expect(cockpit.parked['s1'].awaitingReply).toBe(true)
    expect(cockpit.parked['s1'].chat.map((m) => m.text)).toEqual(['งานที่กำลังทำ'])
    expect(cockpit.parked['s1'].streamingText).toBe('กำลังเขียน…')
  })

  it('sends from the chat you switched to, without touching the running turn', async () => {
    cockpit.openSession = 's1'
    cockpit.turnSession = 's1'
    cockpit.awaitingReply = true
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([] as never)
    await selectGlobalSession({ id: 'other', title: '', ago: '' })

    vi.mocked(SendMessage).mockResolvedValue({ text: 'ได้ครับ' } as never)
    await sendUserMessage('พิมพ์ในแชทที่สอง')

    // A real turn in the chat on screen - not an interjection into the other
    // one, which is where this used to go.
    expect(vi.mocked(SendMessage)).toHaveBeenCalledWith('พิมพ์ในแชทที่สอง')
    expect(vi.mocked(Interject)).not.toHaveBeenCalled()
    expect(cockpit.sessionError).toBe('')
    // The first chat is still working, untouched.
    expect(cockpit.parked['s1'].awaitingReply).toBe(true)
  })

  it('brings the work back mid-flight when you return to it', async () => {
    cockpit.openSession = 'other'
    cockpit.parked['s1'] = {
      chat: [{ role: 'user', text: 'งานที่กำลังทำ', time: '10:00' }],
      awaitingReply: true, agentStatus: 'กำลังรันเครื่องมือ', toolSteps: [],
      turnFiles: [], turnProposals: [], streamingText: 'ครึ่งประโยค',
      reasoningText: '', ask: null, todos: [],
    }
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([question] as never)

    await selectGlobalSession({ id: 's1', title: '', ago: '' })

    // The live messages, not the stored ones: the turn is ahead of the store.
    expect(cockpit.chat.map((m) => m.text)).toEqual(['งานที่กำลังทำ'])
    expect(cockpit.awaitingReply).toBe(true)
    expect(cockpit.streamingText).toBe('ครึ่งประโยค')
    expect(cockpit.agentStatus).toBe('กำลังรันเครื่องมือ')
    expect(cockpit.parked['s1']).toBeUndefined()
  })

  it('lets a new chat be opened while one works', async () => {
    vi.mocked(CurrentSessionID).mockResolvedValue('fresh' as never)
    cockpit.openSession = 's1'
    cockpit.turnSession = 's1'
    cockpit.awaitingReply = true

    await newSession()

    expect(vi.mocked(NewSessionAt)).toHaveBeenCalled()
    expect(cockpit.sessionError).toBe('')
    expect(cockpit.parked['s1'].awaitingReply).toBe(true)
  })

  // Only the chat the turn is writing into is protected. Freezing the whole
  // history list for the length of a long turn would be a different bug.
  it('still deletes a chat the turn is not in', async () => {
    cockpit.awaitingReply = true
    cockpit.turnSession = 's1'

    await deleteSession({ id: 'old', title: '', ago: '', active: false })
    expect(vi.mocked(DeleteSession)).toHaveBeenCalledWith('old')

    await deleteSession({ id: 's1', title: '', ago: '', active: true })
    expect(vi.mocked(DeleteSession)).not.toHaveBeenCalledWith('s1')
  })

  // The engine's own refusal (a desk file gone, a project door mid-turn) used
  // to die as an unhandled rejection - a click that visibly did nothing. Every
  // refusal is a written sentence; show it.
  it('surfaces an engine refusal instead of swallowing it', async () => {
    vi.mocked(NewSessionAt).mockRejectedValue(new Error('เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน'))

    await newSession()

    expect(cockpit.sessionError).toContain('กำลังทำงานอยู่')
  })

  it('takes the banner down when the turn ends', async () => {
    vi.mocked(SendMessage).mockResolvedValue({ text: 'เสร็จแล้ว' } as never)
    cockpit.sessionError = 'เอเจนกำลังทำงานอยู่ — รอให้เสร็จ'

    await sendUserMessage('ต่อเลย')

    expect(cockpit.sessionError).toBe('')
  })
})

// The dead click, reproduced. A chat with no rows comes back from Go as a nil
// slice - `null` by the time it is here, never `[]` - and the map that followed
// threw inside an async function: an unhandled rejection, a row that stayed
// where it was, and nothing on screen saying why.
describe('opening an empty chat while a turn runs', () => {
  it('opens it instead of dying quietly', async () => {
    cockpit.openSession = 's1'
    cockpit.turnSession = 's1'
    cockpit.chat = [{ role: 'user', text: 'งานที่กำลังทำ', time: '10:00' }]
    cockpit.awaitingReply = true
    vi.mocked(LoadSessionAnyProject).mockResolvedValue(null as never)

    await selectGlobalSession({ id: 'fresh', title: 'แชทใหม่', ago: '' })

    expect(cockpit.openSession).toBe('fresh')
    expect(cockpit.chat).toEqual([])
    expect(cockpit.sessionError).toBe('')
    expect(cockpit.parked['s1'].chat.map((m) => m.text)).toEqual(['งานที่กำลังทำ'])
  })
})
