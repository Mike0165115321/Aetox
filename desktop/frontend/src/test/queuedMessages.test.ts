// Typing while a turn runs must not be swallowed. The Go engine holds one
// conversation, so a second SendMessage fired into a live turn races the first
// and is lost — the message waits instead, and goes out when the engine frees.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage, cancelTurn, queuedMessages } from '../lib/stores/cockpit.svelte'
import { SendMessage } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
  queuedMessages.length = 0
  cockpit.awaitingReply = false
})

describe('messages typed during a turn', () => {
  it('waits for the running turn, then sends', async () => {
    let finishTurn: (reply: string) => void = () => {}
    SendMessage.mockImplementationOnce(() => new Promise<string>((resolve) => { finishTurn = resolve }))

    const inFlight = sendUserMessage('one')
    await vi.waitFor(() => expect(cockpit.awaitingReply).toBe(true))

    await sendUserMessage('two')
    expect(queuedMessages).toEqual(['two'])
    expect(SendMessage).toHaveBeenCalledTimes(1) // never fired into the live turn

    finishTurn('reply to one')
    await inFlight

    expect(queuedMessages).toEqual([])
    expect(SendMessage).toHaveBeenCalledTimes(2)
    expect(SendMessage).toHaveBeenLastCalledWith('two')
  })

  it('drops what is waiting when the user hits Stop', async () => {
    cockpit.awaitingReply = true
    await sendUserMessage('never mind')
    expect(queuedMessages).toEqual(['never mind'])

    cancelTurn()
    expect(queuedMessages).toEqual([])
  })

  it('ignores an empty message instead of queueing a blank turn', async () => {
    cockpit.awaitingReply = true
    await sendUserMessage('   ')
    expect(queuedMessages).toEqual([])
  })
})
