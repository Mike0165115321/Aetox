// What a turn that never finished leaves behind, and what survives a reload.
//
// Three separate holes met in the same place. Half an answer was kept when the
// user pressed Stop and thrown away when a quota ran out, though the turn ended
// just as abruptly either way. The red box and its ลองใหม่ button lived only in
// the window, so reopening the chat showed a question sitting alone with nothing
// saying why. And the live block's reset list had drifted from what the live
// block draws: the previous turn's checklist came back, every item already
// struck through, the instant the next turn started. That last one was settled
// by moving the checklist off the live block entirely — it lives on the strip
// above the composer now, so what is guarded here has flipped: the question
// card must not survive a turn boundary, and the plan must.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, sendUserMessage, applyTodos, applyAskUser, restoreTranscript,
  setActiveView, restoreActiveView,
} from '../lib/stores/cockpit.svelte'
import { SendMessage } from './mocks/wailsApp'
import type { main } from '../../wailsjs/go/models'

const row = (m: Partial<main.SessionMessage>): main.SessionMessage =>
  ({ role: 'user', text: '', time: '10:10', ...m }) as main.SessionMessage

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.awaitingReply = false
  cockpit.streamingText = ''
  cockpit.todos = []
  cockpit.ask = null
  cockpit.activeView = 'chat'
  sessionStorage.clear()
})

describe('an answer cut off mid-sentence', () => {
  // The engine keeps the partial reply and hands it back beside the error, but
  // Wails drops a return value when the error is non-nil — so the live preview
  // is the only copy that reaches the window, for a quota exactly as much as for
  // a cancel.
  it('is kept when the failure is not a Stop', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      cockpit.streamingText = 'อ่านไฟล์ครบแล้ว สรุปได้ว่า'
      throw new Error("codex: the free plan's limit is used up")
    })

    await sendUserMessage('สรุปให้หน่อย')

    const last = cockpit.chat.at(-1)
    expect(last?.text).toContain('อ่านไฟล์ครบแล้ว สรุปได้ว่า')
    expect(last?.text).toContain('เกิดข้อผิดพลาด')
    expect(last?.text).toContain("limit is used up")
    expect(last?.failed).toBe(true)
    expect(last?.failedText).toBe('สรุปให้หน่อย')
  })
})

// The owner pasted this at 10:54 on 22 ส.ค. and asked what it was:
//
//   เกิดข้อผิดพลาด: read tcp 192.168.1.40:47800->3.173.21.63:443: wsarecv: An
//   existing connection was forcibly closed by the remote host.
//
// Which is the transport talking to itself. The engine retries this twice now
// and labels what it gives up on, so the window has something to key on that is
// not the wording of a syscall — the same failure reads "connection reset by
// peer" on Linux.
describe('a connection that dropped mid-answer', () => {
  const dropped = 'aetox: connection dropped: read tcp 192.168.1.40:47800->3.173.21.63:443: '
    + 'wsarecv: An existing connection was forcibly closed by the remote host.'

  it('is a sentence, not the transport talking to itself', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => { throw new Error(dropped) })

    await sendUserMessage('อะไรก็ได้เทส')

    const last = cockpit.chat.at(-1)
    expect(last?.failed).toBe(true)
    expect(last?.text).toContain('การเชื่อมต่อ')
    expect(last?.text).not.toContain('wsarecv')
    expect(last?.text).not.toContain('192.168.1.40')
  })

  // The half that made unifying the two copies worth doing. Worded one way on
  // screen and another way after a reload is the app disagreeing with itself
  // about what happened to the same turn.
  it('reads the same after a reload as it did on screen', () => {
    const chat = restoreTranscript([
      row({ role: 'user', text: 'อะไรก็ได้เทส' }),
      row({ role: 'agent', text: '', errorText: dropped }),
    ])

    expect(chat[1].failed).toBe(true)
    expect(chat[1].text).toContain('การเชื่อมต่อ')
    expect(chat[1].text).not.toContain('wsarecv')
    expect(chat[1].failedText).toBe('อะไรก็ได้เทส')
  })

  // Only the labelled one. An error this window has no sentence for is an error
  // the user is better off being able to paste somewhere, so the verbatim path
  // has to survive the change that added a friendlier one beside it.
  it('leaves an unlabelled failure verbatim', () => {
    const chat = restoreTranscript([
      row({ role: 'user', text: 'เทสๆ' }),
      row({ role: 'agent', text: '', errorText: 'deepseek request failed with status 402: no balance' }),
    ])

    expect(chat[1].text).toContain('status 402')
    expect(chat[1].text).toContain('no balance')
  })
})

describe('a failed turn read back from the store', () => {
  it('comes back as a retryable bubble, not a question with no answer', () => {
    const chat = restoreTranscript([
      row({ role: 'user', text: 'เทสๆ' }),
      row({ role: 'agent', text: '', errorText: "codex: the free plan's limit is used up" }),
    ])

    expect(chat).toHaveLength(2)
    expect(chat[1].failed).toBe(true)
    // Exactly what was sent, so pressing ลองใหม่ re-sends the same thing rather
    // than a reconstruction of it.
    expect(chat[1].failedText).toBe('เทสๆ')
    expect(chat[1].text).toContain('เกิดข้อผิดพลาด')
  })

  it('keeps the half answer above the reason it stopped', () => {
    const chat = restoreTranscript([
      row({ role: 'user', text: 'เขียนสรุป' }),
      row({ role: 'agent', text: 'ครึ่งแรกของคำตอบ', errorText: 'connection refused' }),
    ])

    expect(chat[1].text).toContain('ครึ่งแรกของคำตอบ')
    expect(chat[1].text).toContain('connection refused')
  })

  // The store keeps the error; the wording is composed on this side. A cancel
  // must not come back as "เกิดข้อผิดพลาด" after a reload any more than it does
  // while the user is watching.
  it('reads a stored cancel as stopped, not as an error', () => {
    const chat = restoreTranscript([
      row({ role: 'user', text: 'งานยาว' }),
      row({ role: 'agent', text: '', errorText: 'context canceled' }),
    ])

    expect(chat[1].text).toBe('หยุดการทำงานแล้ว')
    expect(chat[1].text).not.toContain('เกิดข้อผิดพลาด')
    expect(chat[1].failed).toBe(true)
  })

  it('leaves a turn that worked completely alone', () => {
    const chat = restoreTranscript([
      row({ role: 'user', text: 'ถาม' }),
      row({ role: 'agent', text: 'ตอบ' }),
    ])

    expect(chat[1].failed).toBeUndefined()
    expect(chat[1].text).toBe('ตอบ')
  })
})

describe('the live block at the start of a turn', () => {
  // The question card is drawn inside `{#if awaitingReply}`, so a stale one is
  // invisible while the chat is idle and then reappears the moment the next turn
  // starts — offering options to a tool that stopped listening.
  it('carries no question card over from the turn before', async () => {
    applyAskUser({ question: 'เอาแบบไหน?', options: ['A', 'B'] })

    let duringTurn: unknown
    vi.mocked(SendMessage).mockImplementation(async () => {
      duringTurn = cockpit.ask
      return { text: 'เปิดให้แล้วครับ' } as never
    })

    await sendUserMessage('เปิดให้ผมดูหน่อย')

    expect(duringTurn).toBeNull()
    expect(cockpit.ask).toBeNull()
  })

  // The checklist was wiped at both ends of a turn for the same reason as the
  // question card, and while it lived in that same live block the reason held —
  // the previous turn's todos, every item already struck through, sitting under
  // "กำลังคิดคำตอบ…" for work nobody asked for. It is on the strip above the
  // composer now, under แผน, where the rows are not a claim about this second:
  // they are the session's latest plan. A plan that evaporates when the turn
  // that wrote it ends is a plan nobody can work from, which is the whole point
  // of วางแผน mode. todo_write replaces it wholesale; a turn that does not plan
  // leaves the last plan standing.
  it('keeps the plan standing across the turn that follows it', async () => {
    applyTodos([
      { content: 'ตรวจสอบสถานะ migration', status: 'completed' },
      { content: 'Build ด้วย Ruvyxa', status: 'in_progress' },
    ])

    let duringTurn = 0
    vi.mocked(SendMessage).mockImplementation(async () => {
      duringTurn = cockpit.todos.length
      return { text: 'เปิดให้แล้วครับ' } as never
    })

    await sendUserMessage('เปิดให้ผมดูหน่อย')

    expect(duringTurn).toBe(2)
    expect(cockpit.todos).toHaveLength(2)
    expect(cockpit.todos[1].status).toBe('in_progress')
  })

  // The half of the old rule worth keeping. A plan with every row struck through
  // is a receipt, and asking the next question is the moment it stops being
  // about anything — left in, it sat over every later turn in a panel that had
  // no off switch, because clearing used to be somebody else's job.
  it('drops a plan that is finished when the next turn starts', async () => {
    applyTodos([
      { content: 'ตรวจสอบสถานะ migration', status: 'completed' },
      { content: 'Build ด้วย Ruvyxa', status: 'completed' },
    ])

    let duringTurn = -1
    vi.mocked(SendMessage).mockImplementation(async () => {
      duringTurn = cockpit.todos.length
      return { text: 'เรียบร้อยครับ' } as never
    })

    await sendUserMessage('ต่อเลย')

    expect(duringTurn).toBe(0)
    expect(cockpit.todos).toHaveLength(0)
  })

  // A turn that dies must not take the plan with it: what it was working from is
  // exactly what the user needs on screen to decide what happens next.
  it('keeps the plan when the turn dies', async () => {
    applyTodos([{ content: 'ย้าย schema ไป v8', status: 'in_progress' }])
    vi.mocked(SendMessage).mockImplementation(async () => {
      throw new Error('connection refused')
    })

    await sendUserMessage('ลุยเลย')

    expect(cockpit.todos).toHaveLength(1)
  })

  // A turn that died with a question still up left a card whose tool is no
  // longer listening — pressing an option answered nothing.
  it('clears the question card when the turn dies holding one', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      applyAskUser({ question: 'ไปต่อไหม?', options: ['ไป', 'หยุด'] })
      throw new Error('connection refused')
    })

    await sendUserMessage('ลุยเลย')

    expect(cockpit.ask).toBeNull()
  })
})

describe('the room a reload lands back on', () => {
  // Two spellings of the same set, and each new room had to be remembered in
  // both. โปรเจกต์ was missed when it opened and ระบบออโตเมชั่น the day after,
  // so the list is now written once and this guards every room in it.
  it('remembers every room the nav can open', () => {
    for (const view of ['chat', 'settings', 'office', 'artifacts', 'projects']) {
      setActiveView(view)
      cockpit.activeView = 'chat'
      restoreActiveView()
      expect(cockpit.activeView).toBe(view)
    }
  })

  // A file tab does not survive the reload that closed it, so a stored path
  // would point at nothing.
  it('does not remember an open file tab', () => {
    setActiveView('artifacts')
    setActiveView('D:/work/notes.md')
    cockpit.activeView = 'chat'
    restoreActiveView()
    expect(cockpit.activeView).toBe('artifacts')
  })
})
