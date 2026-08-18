// Which chat is working, said in the list rather than only in the chat that is
// open. Leaving a turn running is only useful if you can walk away from it and
// still see it is there (owner, 18 ส.ค.).
//
// The answer used to be inferred: `active` (the engine's open session) plus
// `awaitingReply` (a turn is running somewhere). Two facts about the app,
// standing in for one fact about a chat — right until the moment it was asked
// the question that matters, "is the chat I am about to open the one that is
// working?", which is the question the door out of a running turn asks. It is
// stamped now, from the engine's own session id at the moment the turn starts.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, sessionWorking, selectSession, peekAtSession, leavePeek, sendUserMessage,
} from '../lib/stores/cockpit.svelte'
import { SessionTranscript, LoadSession, CurrentSessionID, SendMessage } from './mocks/wailsApp'
import type { Session, ChatMessage } from '../lib/types'

const row = (over: Partial<Session> = {}): Session =>
  ({ id: 's1', title: 'งานยาว', ago: '', ...over })

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.awaitingReply = false
  cockpit.turnSession = ''
  cockpit.chat = []
  cockpit.peek = null
  cockpit.sessionError = ''
})

describe('the working ring on a chat row', () => {
  it('marks the chat the turn is running in', () => {
    cockpit.turnSession = 's1'
    expect(sessionWorking(row())).toBe(true)
  })

  it('leaves every other chat alone', () => {
    cockpit.turnSession = 's1'
    expect(sessionWorking(row({ id: 'other' }))).toBe(false)
  })

  it('says nothing while the agent is idle', () => {
    expect(sessionWorking(row({ active: true }))).toBe(false)
  })

  // The case it exists for: the user is reading another conversation while the
  // work goes on. The stamp names the working chat, so that is the row that
  // lights up — not the one on screen, and not whichever one the engine's
  // cursor happens to be pointing at.
  it('keeps pointing at the working chat while another one is being read', () => {
    cockpit.turnSession = 's1'
    cockpit.peek = { session: row({ id: 'other' }), live: [] }
    expect(sessionWorking(row({ id: 's1' }))).toBe(true)
    expect(sessionWorking(row({ id: 'other' }))).toBe(false)
  })
})

// The bug this pins, in the owner's words on 19 ส.ค.: "ตอนสั่งงานเอเจนกำลังทำงาน
// พอสลับเซสชั่นปุ๊บ งานเอเจนหายไปเลย เหมือนมันผูกกับหน้าที่ผู้ใช้เปิด".
//
// Nothing was ever lost. The turn is the engine's and it runs on; the live
// messages sit in `peek.live` one field away. What was missing was the way
// back: clicking the working chat in the list mid-turn went through
// peekAtSession like every other row, and peekAtSession reads the STORE — so
// the conversation came back as the question with nothing under it, no
// timeline, composer locked, and a bar along the top explaining that the work
// was happening in another chat while pointing straight at this one. The only
// door that actually returned was one button on that bar.
describe('clicking the chat that is working, mid-turn', () => {
  const live: ChatMessage[] = [
    { role: 'user', text: 'สั่งงาน', time: '10:00' },
    { role: 'agent', text: 'ทำไปแล้วครึ่งทาง', time: '10:02' },
  ]

  // The state after switching away: 'other' on screen, the working chat held.
  function readingElsewhere(): void {
    cockpit.awaitingReply = true
    cockpit.turnSession = 's1'
    cockpit.chat = [{ role: 'user', text: 'แชทเก่า', time: '09:00' }]
    cockpit.peek = { session: row({ id: 'other', title: 'แชทอื่น' }), live }
  }

  it('puts the live work back on screen instead of reading it from the store', async () => {
    readingElsewhere()
    await selectSession(row({ id: 's1' }))
    expect(cockpit.peek).toBe(null)
    expect(cockpit.chat.map((m) => m.text)).toEqual(['สั่งงาน', 'ทำไปแล้วครึ่งทาง'])
    // Neither door was opened: reading it would have shown the stored copy,
    // and loading it would have rewritten the memory the turn is thinking with.
    expect(SessionTranscript).not.toHaveBeenCalled()
    expect(LoadSession).not.toHaveBeenCalled()
  })

  // Straight at the function the rows call, because the guard belongs to it
  // rather than to each door: whatever else learns to open a chat later gets
  // this for free.
  it('is not a peek, however the row asks for it', async () => {
    readingElsewhere()
    await peekAtSession(row({ id: 's1' }))
    expect(cockpit.peek).toBe(null)
    expect(cockpit.chat.map((m) => m.text)).toEqual(['สั่งงาน', 'ทำไปแล้วครึ่งทาง'])
  })

  // Clicking the chat you are already in is not a request to go anywhere. It
  // used to stash the live chat and hand back the stored one, which is the
  // same disappearance reached by standing still.
  it('does nothing when the working chat is already the one on screen', async () => {
    cockpit.awaitingReply = true
    cockpit.turnSession = 's1'
    cockpit.chat = live.slice()
    await selectSession(row({ id: 's1' }))
    expect(cockpit.peek).toBe(null)
    expect(cockpit.chat.map((m) => m.text)).toEqual(['สั่งงาน', 'ทำไปแล้วครึ่งทาง'])
    expect(SessionTranscript).not.toHaveBeenCalled()
  })

  // Every other row is still read-only mid-turn: that half was never the bug.
  it('still opens any other chat for reading only', async () => {
    cockpit.awaitingReply = true
    cockpit.turnSession = 's1'
    cockpit.chat = live.slice()
    vi.mocked(SessionTranscript).mockResolvedValue([
      { id: 9, role: 'user', text: 'แชทเก่า', time: '09:00' },
    ] as never)
    await selectSession(row({ id: 'other' }))
    expect(cockpit.peek?.session.id).toBe('other')
    expect(cockpit.peek?.live.map((m) => m.text)).toEqual(['สั่งงาน', 'ทำไปแล้วครึ่งทาง'])
    expect(LoadSession).not.toHaveBeenCalled()
    // And the way back is still there for the bar that offers it.
    leavePeek()
    expect(cockpit.chat.map((m) => m.text)).toEqual(['สั่งงาน', 'ทำไปแล้วครึ่งทาง'])
  })

  // Where the stamp comes from. Asked of the engine at the moment the turn
  // starts, which is the same value Go stamps the turn with (App.SendMessage
  // captures a.sessionID on the same event) — so the window and the engine
  // cannot end up disagreeing about whose work this is. And given back at the
  // end, because a stamp nobody clears is a chat that stays unopenable.
  it('is taken from the engine when the turn starts, and given back when it ends', async () => {
    vi.mocked(CurrentSessionID).mockResolvedValue('s1' as never)
    let stampedDuringTurn = ''
    vi.mocked(SendMessage).mockImplementation(async () => {
      stampedDuringTurn = cockpit.turnSession
      return { text: 'เสร็จแล้วครับ' } as never
    })

    await sendUserMessage('ทำงานหน่อย')

    expect(stampedDuringTurn).toBe('s1')
    expect(cockpit.turnSession).toBe('')
  })

  // Once the turn ends the stamp goes, and every row is an ordinary row again —
  // including the one that was working, which now opens for real.
  it('opens the chat normally again once nothing is running', async () => {
    cockpit.awaitingReply = false
    cockpit.turnSession = ''
    vi.mocked(CurrentSessionID).mockResolvedValue('s1' as never)
    vi.mocked(LoadSession).mockResolvedValue([
      { id: 1, role: 'user', text: 'สั่งงาน', time: '10:00' },
      { id: 2, role: 'agent', text: 'เสร็จแล้วครับ', time: '10:09' },
    ] as never)
    await selectSession(row({ id: 's1' }))
    expect(LoadSession).toHaveBeenCalledWith('s1')
    expect(cockpit.chat.map((m) => m.text)).toEqual(['สั่งงาน', 'เสร็จแล้วครับ'])
  })
})
