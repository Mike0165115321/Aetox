// Typing while a turn runs must reach the model NOW, not when the turn ends.
// It goes into the running turn through Interject (the engine folds it in on its
// next tool-loop round); the queue below is only the straggler net for a message
// the engine handed back because its turn was already returning.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, sendUserMessage, cancelTurn, queuedMessages, applyMissedInterjections,
  attachFileFromPath,
} from '../lib/stores/cockpit.svelte'
import { SendMessage, Interject, SaveChatFile, CurrentSessionID } from './mocks/wailsApp'
import { render, fireEvent, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'

beforeEach(() => {
  localStorage.clear()
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
    expect(SendMessage).toHaveBeenLastCalledWith('too late', '')
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
    expect(cockpit.pendingFiles).toEqual([])
    // The bubble keeps the label, since the model only ever got the path.
    expect(cockpit.chat.at(-1)).toMatchObject({ role: 'user', files: [{ label: 'standup.m4a', kind: 'audio' }] })

    finishTurn({ text: 'done' })
    await inFlight
  })
})

// Typing into a running turn is supported by the engine (Interject), and the
// composer was the half that did not keep up: while a turn ran the only button
// in the send slot was Stop, so the one gesture a person makes after typing
// threw the work away instead of sending it. Asked directly: "ตอนเอเจนกำลัง
// ทำงาน เราส่งข้อความต่อไปทันทีได้ไหม … เช็ค UI ด้วยว่ามันทำงานสอดรับกันไหม".
describe('the composer while a turn is running', () => {
  const props = {
    task: { title: '', steps: [] } as any,
    agentStatus: '', toolSteps: [] as any[], streamingText: '', reasoningText: '',
    messages: [] as any[], onSend: () => {}, onSwitchProvider: async () => {},
    onSwitchThinkLevel: async () => {}, onSwitchModel: async () => {}, onSubmitAPIKey: async () => {},
    model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
  }

  it('offers only the brake when nothing has been typed', async () => {
    const { container } = render(Chat, { ...props, awaitingReply: true })

    await waitFor(() => expect(container.querySelectorAll('.composer .send').length).toBe(1))
    expect(container.querySelector('.composer .send.stop')).toBeTruthy()
  })

  it('puts send back the moment there is something to send, and keeps the brake', async () => {
    const { container } = render(Chat, { ...props, awaitingReply: true })

    const input = container.querySelector('.composer .input') as HTMLTextAreaElement
    await fireEvent.input(input, { target: { value: 'เพิ่มอีกข้อ' } })

    await waitFor(() => {
      // Send is present and is NOT the stop button — clicking after typing must
      // never cancel the work.
      const buttons = Array.from(container.querySelectorAll('.composer .send'))
      expect(buttons.length).toBe(2)
      expect(buttons.filter((b) => b.classList.contains('stop')).length).toBe(1)
    })
  })
})

// A half-typed message is the one piece of text in the app that exists nowhere
// else — not in the transcript, not on disk, not in the engine. Losing it to a
// reload is the cheapest bad UX there is: "พิมพ์อะไรไว้ตอนรีเฟรชไม่ควรจะหาย".
describe('the composer draft across a reload', () => {
  const props = {
    task: { title: '', steps: [] } as any, awaitingReply: false,
    agentStatus: '', toolSteps: [] as any[], streamingText: '', reasoningText: '',
    messages: [] as any[], onSend: () => {}, onSwitchProvider: async () => {},
    onSwitchThinkLevel: async () => {}, onSwitchModel: async () => {}, onSubmitAPIKey: async () => {},
    model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
  }

  it('comes back to the chat it was typed in', async () => {
    vi.mocked(CurrentSessionID).mockResolvedValue('session-a')
    const first = render(Chat, props)
    await fireEvent.input(first.container.querySelector('.composer .input') as HTMLTextAreaElement,
      { target: { value: 'ยังพิมพ์ไม่เสร็จ' } })
    await waitFor(() => expect(localStorage.getItem('aetox-composer-draft')).toContain('ยังพิมพ์ไม่เสร็จ'))
    first.unmount()

    const second = render(Chat, props) // the reload
    await waitFor(() =>
      expect((second.container.querySelector('.composer .input') as HTMLTextAreaElement).value).toBe('ยังพิมพ์ไม่เสร็จ'))
  })

  // The other half, and the one that would be worse: a draft restored into
  // someone else's conversation looks like something they wrote.
  it('does not follow the user into a different chat', async () => {
    vi.mocked(CurrentSessionID).mockResolvedValue('session-a')
    const first = render(Chat, props)
    await fireEvent.input(first.container.querySelector('.composer .input') as HTMLTextAreaElement,
      { target: { value: 'ของแชท A' } })
    await waitFor(() => expect(localStorage.getItem('aetox-composer-draft')).toContain('ของแชท A'))
    first.unmount()

    vi.mocked(CurrentSessionID).mockResolvedValue('session-b')
    const second = render(Chat, props)
    await new Promise((r) => setTimeout(r, 20))
    expect((second.container.querySelector('.composer .input') as HTMLTextAreaElement).value).toBe('')
  })
})
