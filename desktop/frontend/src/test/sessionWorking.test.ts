// Which chat is working, said in the list rather than only in the chat that is
// open. Leaving a turn running is only useful if you can walk away from it and
// still see it is there (owner, 18 ส.ค.).
import { describe, it, expect, beforeEach } from 'vitest'
import { cockpit, sessionWorking } from '../lib/stores/cockpit.svelte'
import type { Session } from '../lib/types'

const row = (over: Partial<Session> = {}): Session =>
  ({ id: 's1', title: 'งานยาว', ago: '', ...over })

beforeEach(() => {
  cockpit.awaitingReply = false
  cockpit.peek = null
})

describe('the working ring on a chat row', () => {
  it('marks the chat the turn is running in', () => {
    cockpit.awaitingReply = true
    expect(sessionWorking(row({ active: true }))).toBe(true)
  })

  it('leaves every other chat alone', () => {
    cockpit.awaitingReply = true
    expect(sessionWorking(row({ id: 'other', active: false }))).toBe(false)
  })

  it('says nothing while the agent is idle', () => {
    expect(sessionWorking(row({ active: true }))).toBe(false)
  })

  // The case it exists for: the user is reading another conversation while the
  // work goes on. `active` still names the engine's session, so the row that is
  // working is the one that lights up — not the one on screen.
  it('keeps pointing at the working chat while another one is being read', () => {
    cockpit.awaitingReply = true
    cockpit.peek = { session: row({ id: 'other' }), live: [] }
    expect(sessionWorking(row({ id: 's1', active: true }))).toBe(true)
    expect(sessionWorking(row({ id: 'other', active: false }))).toBe(false)
  })
})
