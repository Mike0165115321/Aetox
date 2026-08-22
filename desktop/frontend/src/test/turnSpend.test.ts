// What the turn is costing, while there is still something to do about it.
//
// The app knew this number all along and never said it: every model round has
// always gone to recordTokenUsage, which wrote it to a table nobody reads until
// the next day. The meter on the composer is a different fact that happens to
// share a unit — how full the context window is — and it cannot answer "am I
// being drained", which is the question a person asks while four sub-agents are
// looping.
import { describe, it, expect, beforeEach } from 'vitest'
import { cockpit, applyUsageRound, selectSession } from '../lib/stores/cockpit.svelte'
import { emptyTurnSpend } from '../lib/types'

beforeEach(() => {
  cockpit.turnSpend = emptyTurnSpend()
  cockpit.turnSession = 'sess_1'
  cockpit.openSession = 'sess_1'
})

describe('the live spend meter', () => {
  it('adds each round up, keeping input and output apart', () => {
    applyUsageRound({ session: 'sess_1', in: 4011, out: 120 })
    applyUsageRound({ session: 'sess_1', in: 4180, out: 65 })
    // Two numbers, not one: input climbing means a transcript re-sent every
    // round, output climbing means a model that will not stop writing, and only
    // the reader can tell which of those is worth the brake.
    expect(cockpit.turnSpend.in).toBe(8191)
    expect(cockpit.turnSpend.out).toBe(185)
  })

  it('drops a round another chat spent', () => {
    applyUsageRound({ session: 'sess_1', in: 100, out: 10 })
    applyUsageRound({ session: 'sess_2', in: 90000, out: 5000 })
    // Several conversations work at once. A meter that added everything it
    // heard would put one chat's bill under another chat's composer.
    expect(cockpit.turnSpend.in).toBe(100)
    expect(cockpit.turnSpend.out).toBe(10)
  })

  it('counts a sub-agent round the same as the main agent`s', () => {
    // A delegate's tokens are the user's tokens: they reach this through the
    // parent's own usage reporter, and leaving them out would make the meter
    // read lowest exactly while four delegates were spending the most.
    applyUsageRound({ session: 'sess_1', in: 1000, out: 50 })
    applyUsageRound({ session: 'sess_1', in: 30000, out: 900 })
    expect(cockpit.turnSpend.in).toBe(31000)
  })

  it('stays silent about a cache nobody reported', () => {
    applyUsageRound({ session: 'sess_1', in: 900, out: 40, cacheReported: false })
    // A local runtime accounts for no cache. Drawing that as a zero would be
    // the tooltip answering a question the provider never let anyone ask.
    expect(cockpit.turnSpend.cacheReported).toBe(false)
    expect(cockpit.turnSpend.cached).toBe(0)
  })

  it('keeps a cache figure that was truthfully measured, once', () => {
    applyUsageRound({ session: 'sess_1', in: 4011, out: 100, cached: 3968, cacheReported: true })
    applyUsageRound({ session: 'sess_1', in: 900, out: 40, cacheReported: false })
    // A turn can run partly on a provider that reports cache and partly on one
    // that does not. Dropping the flag on the second round would hide a number
    // that was really measured on the first.
    expect(cockpit.turnSpend.cacheReported).toBe(true)
    expect(cockpit.turnSpend.cached).toBe(3968)
  })

  it('leaves the count behind when the chat does', async () => {
    applyUsageRound({ session: 'sess_1', in: 45300, out: 1400 })
    await selectSession({ id: 'sess_2', title: 'แชทใหม่', ago: '' })
    // The meter belongs to the turn it was counting, and that turn was in the
    // chat being left. Carried across, a brand-new conversation opened under a
    // reading of 45.3k while its own panel said nothing had been spent yet —
    // two numbers on one screen, disagreeing (owner, 22 ส.ค.).
    expect(cockpit.turnSpend).toEqual(emptyTurnSpend())
  })
})

describe('the money on the meter', () => {
  it('adds up only what the engine could price', () => {
    applyUsageRound({ session: 'sess_1', in: 4011, out: 120, cost: 0.0021, priced: true })
    applyUsageRound({ session: 'sess_1', in: 900, out: 40, cost: 0.0004, priced: true })
    expect(cockpit.turnSpend.cost).toBeCloseTo(0.0025, 6)
    expect(cockpit.turnSpend.unpriced).toBe(0)
  })

  it('counts a round nobody publishes a rate for, rather than calling it free', () => {
    applyUsageRound({ session: 'sess_1', in: 4011, out: 120, cost: 0.0021, priced: true })
    applyUsageRound({ session: 'sess_1', in: 900, out: 40, priced: false })
    // The panel hides the money while this is non-zero. A running total quietly
    // missing a round is a number the user would trust and should not, and
    // "not in the price catalog" is not "cost nothing".
    expect(cockpit.turnSpend.unpriced).toBe(1)
    expect(cockpit.turnSpend.cost).toBeCloseTo(0.0021, 6)
  })

  it('does not count a round that spent nothing as unpriced', () => {
    applyUsageRound({ session: 'sess_1', in: 0, out: 0, priced: false })
    expect(cockpit.turnSpend.unpriced).toBe(0)
  })
})
