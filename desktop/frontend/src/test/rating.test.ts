// Rating a reply. This is the only signal in the app that means "the answer
// was good" — a tool that did not error is not the same claim — so what is
// pinned down here is that the gesture is reversible, that it survives a
// reload, and that it is never offered on a bubble it cannot address.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage, rateReply } from '../lib/stores/cockpit.svelte'
import { SendMessage, RateTurn } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
  cockpit.awaitingReply = false
})

describe('rating a reply', () => {
  it('carries the stored row id onto the bubble, so a fresh answer is ratable at once', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'ยอด 1,250 บาท', messageId: 42 })
    await sendUserMessage('อ่านสลิปนี้')

    const reply = cockpit.chat.at(-1)!
    expect(reply.id).toBe(42)

    await rateReply(reply, 'good')
    expect(RateTurn).toHaveBeenCalledWith(42, 'good')
    expect(reply.rating).toBe('good')
  })

  // A verdict you cannot take back is one people stop giving.
  it('withdraws the rating when the lit thumb is pressed again', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'ok', messageId: 7 })
    await sendUserMessage('q')
    const reply = cockpit.chat.at(-1)!

    await rateReply(reply, 'bad')
    expect(reply.rating).toBe('bad')

    await rateReply(reply, 'bad')
    expect(RateTurn).toHaveBeenLastCalledWith(7, 'unknown')
    expect(reply.rating).toBe('unknown')
  })

  it('switches sides rather than needing to be cleared first', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'ok', messageId: 8 })
    await sendUserMessage('q')
    const reply = cockpit.chat.at(-1)!

    await rateReply(reply, 'good')
    await rateReply(reply, 'bad')
    expect(RateTurn).toHaveBeenLastCalledWith(8, 'bad')
    expect(reply.rating).toBe('bad')
  })

  // A turn that failed was persisted nowhere, so there is no job to rate. The
  // buttons are hidden on such a bubble; this is the store half of that.
  it('does nothing for a bubble that was never stored', async () => {
    SendMessage.mockRejectedValueOnce(new Error('no such host'))
    await sendUserMessage('q')
    const failed = cockpit.chat.at(-1)!

    expect(failed.id).toBeUndefined()
    await rateReply(failed, 'bad')
    expect(RateTurn).not.toHaveBeenCalled()
  })
})
