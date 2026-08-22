// Which chat a live event belongs to, when the window and the engine disagree.
//
// Getting this wrong in either direction is expensive: leak, and one chat's
// work appears in another; drop, and a turn runs to completion with nothing on
// screen at all. The second failure is exactly what multichat shipped with —
// events were FILTERED against the window's one `turnSession` before they
// could be routed, so starting a second chat's turn silently discarded every
// event of the first, and that chat froze on "กำลังคิด" while its engine
// worked perfectly (owner's screenshot, 22 ส.ค.). The rule now: an event for
// any session this window holds live state for is ROUTED there; only a stamp
// naming a session the window knows nothing about is a correction (adopt) or a
// stray (drop).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, applyToolEvent, applyAgentStatus } from '../lib/stores/cockpit.svelte'

const ENGINE = '20260822-032236.700'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.toolSteps = []
  cockpit.backgroundSteps = []
  cockpit.agentStatus = ''
  cockpit.parked = {}
  cockpit.awaitingReply = false
  cockpit.openSession = ''
  cockpit.turnSession = ''
})

const aTurnFrom = (session: string) => {
  applyAgentStatus({ sessionId: session, data: 'กำลังคิดคำตอบ...' } as any)
  applyToolEvent({
    sessionId: session,
    data: { action: 'call', ref: 'c1', name: 'edit', subject: 'README.md' },
  } as any)
}

describe('a live event whose stamp is not the turn the window is tracking', () => {
  // The window guessed the id of its own turn and guessed wrong. Nothing else
  // is running, so the stamp cannot be anybody else's — it is this turn.
  it('is adopted when the window is tracking nothing for that session', () => {
    cockpit.awaitingReply = true
    cockpit.openSession = 'what-the-window-thinks'
    cockpit.turnSession = 'what-the-window-thinks'

    aTurnFrom(ENGINE)

    expect(cockpit.turnSession).toBe(ENGINE)
    expect(cockpit.toolSteps.length).toBe(1)
    expect(cockpit.agentStatus).toBe('กำลังคิดคำตอบ...')
  })

  // Several conversations at once is the point (§150). A parked chat's work
  // must not leak into the one on screen — AND must not be thrown away: it
  // goes on filling that chat's own parked state, which is what the user
  // finds when they come back. Dropping it here is the freeze this file's
  // header describes.
  it('is routed to the parked chat, not the one on screen', () => {
    cockpit.awaitingReply = true
    cockpit.openSession = 'on-screen'
    cockpit.turnSession = 'on-screen'
    cockpit.parked['the-other-one'] = {
      chat: [], awaitingReply: true, agentStatus: '', toolSteps: [],
      turnFiles: [], turnProposals: [], streamingText: '', reasoningText: '',
      ask: null, todos: [],
    } as any

    aTurnFrom('the-other-one')

    expect(cockpit.turnSession).toBe('on-screen')
    expect(cockpit.toolSteps.length).toBe(0)
    expect(cockpit.agentStatus).toBe('')
    expect(cockpit.parked['the-other-one'].toolSteps.length).toBe(1)
    expect(cockpit.parked['the-other-one'].agentStatus).toBe('กำลังคิดคำตอบ...')
  })

  // An event stamped with the chat on screen is that chat's own — it draws
  // there, whatever the shared cursor happens to say. (It used to be dropped
  // "because the turn being followed is another one", which was the one-turn
  // filter reasoning; a chat's own events have exactly one honest home.)
  it('is drawn into the open chat when the stamp names it', () => {
    cockpit.awaitingReply = true
    cockpit.openSession = ENGINE
    cockpit.turnSession = 'a-turn-started-elsewhere'

    aTurnFrom(ENGINE)

    expect(cockpit.turnSession).toBe('a-turn-started-elsewhere')
    expect(cockpit.toolSteps.length).toBe(1)
    expect(cockpit.agentStatus).toBe('กำลังคิดคำตอบ...')
  })

  // Outside a turn the window has no claim to correct, so nothing is adopted.
  it('is dropped when no turn is in flight', () => {
    cockpit.awaitingReply = false
    cockpit.openSession = 'idle-chat'
    cockpit.turnSession = 'stale'

    aTurnFrom(ENGINE)

    expect(cockpit.turnSession).toBe('stale')
    expect(cockpit.toolSteps.length).toBe(0)
  })
})
