// Which chat a live event belongs to, when the window and the engine disagree.
//
// Two situations wear the same shape and only one of them is a stray event.
// Telling them apart is the whole of this file, because getting it wrong in
// either direction is expensive: leak, and one chat's work appears in another;
// drop, and a turn runs to completion with nothing on screen at all.
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
  // must not leak into the one on screen.
  it('is dropped when that session is a chat parked and working', () => {
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
  })

  // The chat on screen is somebody the window is tracking too: its turn is not
  // the one being followed, so its events belong to it and not here.
  it('is dropped when that session is the chat on screen', () => {
    cockpit.awaitingReply = true
    cockpit.openSession = ENGINE
    cockpit.turnSession = 'a-turn-started-elsewhere'

    aTurnFrom(ENGINE)

    expect(cockpit.turnSession).toBe('a-turn-started-elsewhere')
    expect(cockpit.toolSteps.length).toBe(0)
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
