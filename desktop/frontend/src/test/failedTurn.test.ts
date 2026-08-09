// What a turn that never finished leaves behind, and what survives a reload.
//
// Three separate holes met in the same place. Half an answer was kept when the
// user pressed Stop and thrown away when a quota ran out, though the turn ended
// just as abruptly either way. The red box and its ลองใหม่ button lived only in
// the window, so reopening the chat showed a question sitting alone with nothing
// saying why. And the live block's reset list had drifted from what the live
// block draws: the previous turn's checklist came back, every item already
// struck through, the instant the next turn started.
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
  // The checklist is drawn inside `{#if awaitingReply}`, so a stale one is
  // invisible while the chat is idle and then reappears the moment the next turn
  // starts. It was cleared only when the session changed — switching chats hid
  // the bug, and staying in one did not.
  it('carries no checklist or question card over from the turn before', async () => {
    applyTodos([
      { content: 'ตรวจสอบสถานะ migration', status: 'completed' },
      { content: 'Build ด้วย Ruvyxa', status: 'completed' },
    ])
    applyAskUser({ question: 'เอาแบบไหน?', options: ['A', 'B'] })

    let duringTurn: { todos: number; ask: unknown } | undefined
    vi.mocked(SendMessage).mockImplementation(async () => {
      duringTurn = { todos: cockpit.todos.length, ask: cockpit.ask }
      return { text: 'เปิดให้แล้วครับ' } as never
    })

    await sendUserMessage('เปิดให้ผมดูหน่อย')

    expect(duringTurn?.todos).toBe(0)
    expect(duringTurn?.ask).toBeNull()
    expect(cockpit.todos).toHaveLength(0)
    expect(cockpit.ask).toBeNull()
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
