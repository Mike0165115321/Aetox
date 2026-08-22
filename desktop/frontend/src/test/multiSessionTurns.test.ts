// Two chats working at once, and neither freezing the other.
//
// The owner's screenshot (22 ส.ค.): start a turn in chat A, walk to chat B,
// start one there too — and A sits on "กำลังคิด" forever while its engine
// works perfectly. The root was the one-turn filter: starting B's turn
// overwrote the window's single `turnSession`, and from that moment every
// event stamped A was discarded before writeLive could file it into A's
// parked state. The ending was wrong the same way from the other side:
// runLiveTurn's finally read `cockpit.turnSession` back instead of the id the
// turn was born with, so the first turn to finish closed whichever chat
// started a turn most recently.
//
// This file is the whole journey, driven through the public doors: send, walk
// away (selectSession → park), send again, and watch each chat's events, reply
// and ending land in its own state.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, sendUserMessage, selectSession, sessionWorking, queuedMessages,
  applyAgentStatus, applyToolEvent, applyAgentChunk, applyUsageRound,
  applyMissedInterjections,
} from '../lib/stores/cockpit.svelte'
import { SendMessage, PendingUndo } from './mocks/wailsApp'
import type { Session } from '../lib/types'

const A = '20260822-100000.001'
const B = '20260822-100000.002'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.toolSteps = []
  cockpit.backgroundSteps = []
  cockpit.agentStatus = ''
  cockpit.streamingText = ''
  cockpit.reasoningText = ''
  cockpit.awaitingReply = false
  cockpit.turnSession = ''
  cockpit.openSession = ''
  cockpit.parked = {}
  cockpit.sessions = []
  queuedMessages.length = 0
  vi.mocked(PendingUndo).mockResolvedValue([] as any)
})

/** A SendMessage the test finishes by hand, so two turns can overlap. */
function heldReply(): { resolve: (text: string) => void; promise: Promise<any> } {
  let resolve!: (text: string) => void
  const promise = new Promise<any>((r) => {
    resolve = (text: string) => r({ text, parts: [] })
  })
  return { resolve, promise }
}

describe('two chats with turns in flight', () => {
  it('keeps filling the parked chat, and each ending closes only its own turn', async () => {
    cockpit.openSession = A

    // Chat A asks, and its answer is not ready yet.
    const turnA = heldReply()
    vi.mocked(SendMessage).mockReturnValueOnce(turnA.promise)
    const sendA = sendUserMessage('งานแรก')
    await Promise.resolve()
    expect(cockpit.awaitingReply).toBe(true)
    expect(cockpit.turnSession).toBe(A)

    // The user walks away mid-turn: A parks with its work, B arrives clean.
    await selectSession({ id: B } as Session)
    expect(cockpit.openSession).toBe(B)
    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.parked[A]?.awaitingReply).toBe(true)

    // ...and starts a second turn. This is the moment that used to orphan A.
    const turnB = heldReply()
    vi.mocked(SendMessage).mockReturnValueOnce(turnB.promise)
    const sendB = sendUserMessage('งานที่สอง')
    await Promise.resolve()
    expect(cockpit.turnSession).toBe(B)
    expect(sessionWorking({ id: A })).toBe(true)
    expect(sessionWorking({ id: B })).toBe(true)

    // A's engine keeps talking. Every event lands in A's parked state — the
    // freeze was these being dropped on the floor.
    applyAgentStatus({ sessionId: A, data: 'กำลังใช้เครื่องมือ...' } as any)
    applyToolEvent({ sessionId: A, data: { action: 'call', ref: 'a1', name: 'read', subject: 'docs.md' } } as any)
    applyAgentChunk({ sessionId: A, data: { text: 'ครึ่งคำตอบของ A', replace: false } } as any)
    applyUsageRound({ session: A, in: 100, out: 20, priced: true, cost: 0.01 })
    expect(cockpit.parked[A].agentStatus).toBe('กำลังใช้เครื่องมือ...')
    expect(cockpit.parked[A].toolSteps.length).toBe(1)
    expect(cockpit.parked[A].streamingText).toBe('ครึ่งคำตอบของ A')
    expect(cockpit.parked[A].turnSpend.in).toBe(100)
    // ...and nothing of it leaked onto B's screen.
    expect(cockpit.agentStatus).toBe('')
    expect(cockpit.toolSteps.length).toBe(0)
    expect(cockpit.streamingText).toBe('')
    expect(cockpit.turnSpend.in).toBe(0)

    // B's own events still draw on screen.
    applyAgentStatus({ sessionId: B, data: 'กำลังคิดคำตอบ...' } as any)
    expect(cockpit.agentStatus).toBe('กำลังคิดคำตอบ...')

    // A finishes first, off screen. Its reply belongs to A's transcript, its
    // ending closes A alone — B's live block must not so much as blink.
    turnA.resolve('คำตอบของ A')
    await sendA
    const aChat = cockpit.parked[A].chat
    expect(aChat.at(-1)?.text).toBe('คำตอบของ A')
    expect(cockpit.chat.some((m) => m.text === 'คำตอบของ A')).toBe(false)
    expect(cockpit.parked[A].awaitingReply).toBe(false)
    expect(sessionWorking({ id: A })).toBe(false)
    expect(cockpit.awaitingReply).toBe(true)
    expect(cockpit.turnSession).toBe(B)
    expect(cockpit.agentStatus).toBe('กำลังคิดคำตอบ...')

    // B finishes on screen, the ordinary way.
    turnB.resolve('คำตอบของ B')
    await sendB
    expect(cockpit.awaitingReply).toBe(false)
    expect(cockpit.turnSession).toBe('')
    expect(cockpit.chat.at(-1)?.text).toBe('คำตอบของ B')
  })

  it('a turn that fails off screen leaves its error bubble in its own chat', async () => {
    cockpit.openSession = A

    let fail!: (err: Error) => void
    vi.mocked(SendMessage).mockReturnValueOnce(new Promise((_r, reject) => { fail = reject }))
    const sendA = sendUserMessage('งานที่จะล้ม')
    await Promise.resolve()

    await selectSession({ id: B } as Session)
    const turnB = heldReply()
    vi.mocked(SendMessage).mockReturnValueOnce(turnB.promise)
    const sendB = sendUserMessage('งานที่สอง')
    await Promise.resolve()

    fail(new Error('provider said no'))
    await sendA
    const aLast = cockpit.parked[A].chat.at(-1)
    expect(aLast?.failed).toBe(true)
    expect(cockpit.chat.some((m) => m.failed)).toBe(false)
    // B is untouched by A's failure.
    expect(cockpit.awaitingReply).toBe(true)

    turnB.resolve('เสร็จ')
    await sendB
    expect(cockpit.awaitingReply).toBe(false)
  })

  // The straggler net was one module global: a message the engine handed back
  // from chat A's ending turn, while the user stood in chat B, went out as B's
  // next turn. It waits with A now, and goes out when A is reopened.
  it('a straggler missed off screen waits with its chat and drains on return', async () => {
    cockpit.openSession = A

    const turnA = heldReply()
    vi.mocked(SendMessage).mockReturnValueOnce(turnA.promise)
    const sendA = sendUserMessage('งานแรก')
    await Promise.resolve()

    await selectSession({ id: B } as Session)

    // The engine hands back a message it could not fold into A's ending turn.
    applyMissedInterjections({ sessionId: A, data: ['ตามด้วยอันนี้'] } as any)
    expect(cockpit.parked[A].queued).toEqual(['ตามด้วยอันนี้'])
    expect(queuedMessages).toEqual([])

    // A's ending must not send it — the user is standing in B.
    turnA.resolve('คำตอบของ A')
    await sendA
    expect(vi.mocked(SendMessage)).toHaveBeenCalledTimes(1)
    expect(cockpit.parked[A].queued).toEqual(['ตามด้วยอันนี้'])

    // Reopening A is the moment it goes out, as A's own turn.
    vi.mocked(SendMessage).mockResolvedValueOnce({ text: 'รับทราบ', parts: [] } as any)
    await selectSession({ id: A } as Session)
    await vi.waitFor(() => expect(vi.mocked(SendMessage)).toHaveBeenLastCalledWith('ตามด้วยอันนี้'))
    expect(queuedMessages).toEqual([])
  })
})
