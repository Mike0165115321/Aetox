// Which chat is working, said in the list rather than only in the chat that is
// open. Leaving a turn running is only useful if you can walk away from it and
// still see it is there (owner, 18 ส.ค.).
//
// Rewritten 19 ส.ค. when the read-only peek was deleted. There is no "reading"
// state any more: switching chats mid-turn parks the working one's whole live
// state and hands the arriving one a clean set, so a chat can be working in two
// different places — on screen, where `turnSession` names it, or parked, where
// its own record carries the flag. The ring has to find it in both.
import { describe, it, expect, beforeEach } from 'vitest'
import { cockpit, sessionWorking } from '../lib/stores/cockpit.svelte'
import { emptyTurnSpend } from '../lib/types'
import type { Session, ParkedTurn } from '../lib/types'

const row = (over: Partial<Session> = {}): Session =>
  ({ id: 's1', title: 'งานยาว', ago: '', ...over })

const parked = (over: Partial<ParkedTurn> = {}): ParkedTurn => ({
  chat: [], awaitingReply: true, agentStatus: '', toolSteps: [],
  turnFiles: [], turnProposals: [], streamingText: '', reasoningText: '',
  ask: null, todos: [], turnSpend: emptyTurnSpend(), ...over,
})

beforeEach(() => {
  cockpit.awaitingReply = false
  cockpit.turnSession = ''
  cockpit.openSession = ''
  cockpit.parked = {}
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
    expect(sessionWorking(row())).toBe(false)
  })

  // The case it exists for, and the one the old read-only peek could only half
  // answer: the user walked away and the work went on without them.
  it('keeps ringing a chat that is working off screen', () => {
    cockpit.openSession = 'other'
    cockpit.parked['s1'] = parked()

    expect(sessionWorking(row({ id: 's1' }))).toBe(true)
    expect(sessionWorking(row({ id: 'other' }))).toBe(false)
  })

  // A parked chat whose turn ended before the user came back is not working —
  // the record can outlive the turn, the ring must not.
  it('stops ringing a parked chat whose turn has ended', () => {
    cockpit.openSession = 'other'
    cockpit.parked['s1'] = parked({ awaitingReply: false })

    expect(sessionWorking(row({ id: 's1' }))).toBe(false)
  })

  // Two at once is the whole capability. The list has to be able to say so.
  it('rings both chats when two are working', () => {
    cockpit.openSession = 's2'
    cockpit.turnSession = 's2'
    cockpit.parked['s1'] = parked()

    expect(sessionWorking(row({ id: 's1' }))).toBe(true)
    expect(sessionWorking(row({ id: 's2' }))).toBe(true)
  })
})
