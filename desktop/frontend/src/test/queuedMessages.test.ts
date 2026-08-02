// Typing while a turn runs must reach the model NOW, not when the turn ends.
// It goes into the running turn through Interject (the engine folds it in on its
// next tool-loop round); the queue below is only the straggler net for a message
// the engine handed back because its turn was already returning.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, sendUserMessage, cancelTurn, queuedMessages, applyMissedInterjections,
  attachFileFromPath,
} from '../lib/stores/cockpit.svelte'
import { SendMessage, Interject, SaveChatFile } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
  queuedMessages.length = 0
  cockpit.awaitingReply = false
})

describe('messages typed during a turn', () => {
  it('goes into the running turn immediately instead of waiting for it', async () => {
    let finishTurn: (reply: { text: string }) => void = () => {}
    SendMessage.mockImplementationOnce(() => new Promise<{ text: string }>((resolve) => { finishTurn = resolve }))

    const inFlight = sendUserMessage('one')
    await vi.waitFor(() => expect(cockpit.awaitingReply).toBe(true))

    await sendUserMessage('two')
    // The whole point: handed over while the first turn is still running.
    expect(Interject).toHaveBeenCalledWith('two')
    expect(queuedMessages).toEqual([])
    // Still one conversation — a second SendMessage would race the live turn.
    expect(SendMessage).toHaveBeenCalledTimes(1)
    // And the user sees their message the moment they send it, not after the turn.
    expect(cockpit.chat.filter((m) => m.role === 'user').map((m) => m.text)).toEqual(['one', 'two'])

    finishTurn({ text: 'reply to one' })
    await inFlight

    // Nothing re-sent: 'two' was answered inside the turn it was typed into.
    expect(SendMessage).toHaveBeenCalledTimes(1)
  })

  it('sends a straggler as its own turn, without a second bubble', async () => {
    let finishTurn: (reply: { text: string }) => void = () => {}
    SendMessage.mockImplementationOnce(() => new Promise<{ text: string }>((resolve) => { finishTurn = resolve }))

    const inFlight = sendUserMessage('one')
    await vi.waitFor(() => expect(cockpit.awaitingReply).toBe(true))
    await sendUserMessage('too late')

    // The engine could not fold it in and handed it back.
    applyMissedInterjections(['too late'])
    finishTurn({ text: 'reply to one' })
    await inFlight

    expect(queuedMessages).toEqual([])
    expect(SendMessage).toHaveBeenLastCalledWith('too late')
    // Its bubble was pushed when it was typed — sending it must not duplicate it.
    expect(cockpit.chat.filter((m) => m.role === 'user' && m.text === 'too late')).toHaveLength(1)
  })

  it('drops a straggler when the user hits Stop', () => {
    applyMissedInterjections(['never mind'])
    expect(queuedMessages).toEqual(['never mind'])

    cancelTurn()
    expect(queuedMessages).toEqual([])
  })

  it('ignores an empty message instead of interjecting a blank turn', async () => {
    cockpit.awaitingReply = true
    await sendUserMessage('   ')
    expect(Interject).not.toHaveBeenCalled()
    expect(queuedMessages).toEqual([])
  })
})

// Attachments used to be left staged in the composer while the message waited in
// the queue. Interjecting sends it now, so they have to be folded in now too —
// otherwise a clip attached mid-turn is silently dropped, or worse, sticks around
// and rides along with the next unrelated message.
describe('an interjected message carries its attachment', () => {
  it('folds the attachment in and clears the composer', async () => {
    let finishTurn: (reply: { text: string }) => void = () => {}
    SendMessage.mockImplementationOnce(() => new Promise<{ text: string }>((resolve) => { finishTurn = resolve }))
    const inFlight = sendUserMessage('งานหลัก')
    await vi.waitFor(() => expect(cockpit.awaitingReply).toBe(true))

    SaveChatFile.mockResolvedValue('.aetox-attachments/1-1.m4a' as never)
    await attachFileFromPath('D:/rec/standup.m4a')
    await sendUserMessage('ถอดเสียงอันนี้ด้วย')

    const handed = Interject.mock.calls[0][0] as string
    expect(handed).toContain('ถอดเสียงอันนี้ด้วย')
    expect(handed).toContain('.aetox-attachments/1-1.m4a')
    expect(handed).toContain('audio_transcribe')
    // Not left behind to ride along with whatever is typed next.
    expect(cockpit.pendingFile).toBeNull()
    // The bubble keeps the label, since the model only ever got the path.
    expect(cockpit.chat.at(-1)).toMatchObject({ role: 'user', attachLabel: 'standup.m4a' })

    finishTurn({ text: 'done' })
    await inFlight
  })
})
