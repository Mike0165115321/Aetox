// Answering a question the model asked. Free text was always accepted — but
// only through the composer at the bottom of the window, so the card had to
// spend a line telling the user to look somewhere else. These pin down that
// the answer can now be written where the question is.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, AnswerUserQuestion } from './mocks/wailsApp'

const baseProps = {
  messages: [] as any[],
  task: { title: '', steps: [] } as any,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

// The card only exists inside a live turn, which is the only moment the model
// can be asking anything — and an empty transcript draws the welcome screen
// instead of the conversation, so a question needs one exchange behind it.
const asking = {
  ...baseProps,
  awaitingReply: true,
  messages: [{ role: 'user', text: 'ทำอะไรให้ดูหน่อย', time: '19:45' }] as any[],
}

const ask = () => ({
  question: 'อยากให้ผมทำแบบไหนให้คุณดูเป็นอย่างแรก?',
  options: ['การ์ดต้อนรับ', 'แดชบอร์ดข้อมูล', 'แปลงสื่อเป็นบทสรุป'],
})

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.toolSteps = []
  cockpit.ask = null
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
})

describe('the question card', () => {
  it('carries a field for an answer that is not one of the options', async () => {
    cockpit.ask = ask()
    const { container } = render(Chat, asking)
    await tick()

    const input = container.querySelector('.ask-own-input') as HTMLInputElement
    expect(input).toBeTruthy()

    await fireEvent.input(input, { target: { value: 'ขอเป็นสไลด์แทน' } })
    await fireEvent.submit(container.querySelector('.ask-own') as HTMLFormElement)

    expect(AnswerUserQuestion).toHaveBeenCalledWith('ขอเป็นสไลด์แทน')
    // Answering closes the question, exactly as clicking an option does.
    expect(cockpit.ask).toBeNull()
  })

  it('focuses the field as the card appears, so typing just works', async () => {
    cockpit.ask = ask()
    const { container } = render(Chat, asking)
    await tick()

    expect(document.activeElement).toBe(container.querySelector('.ask-own-input'))
  })

  it('refuses to send an empty or blank answer', async () => {
    cockpit.ask = ask()
    const { container } = render(Chat, asking)
    await tick()

    const send = container.querySelector('.ask-own-send') as HTMLButtonElement
    expect(send.disabled).toBe(true)

    const input = container.querySelector('.ask-own-input') as HTMLInputElement
    await fireEvent.input(input, { target: { value: '   ' } })
    await fireEvent.submit(container.querySelector('.ask-own') as HTMLFormElement)
    expect(AnswerUserQuestion).not.toHaveBeenCalled()
    expect(cockpit.ask).not.toBeNull()
  })

  it('still answers when an option is clicked', async () => {
    cockpit.ask = ask()
    const { container } = render(Chat, asking)
    await tick()

    const first = container.querySelector('.ask-opt') as HTMLButtonElement
    await fireEvent.click(first)
    expect(AnswerUserQuestion).toHaveBeenCalledWith('การ์ดต้อนรับ')
  })

  // A draft left over from the previous question is an answer to something
  // that is no longer being asked.
  it('starts empty when a new question arrives', async () => {
    cockpit.ask = ask()
    const { container } = render(Chat, asking)
    await tick()

    const input = container.querySelector('.ask-own-input') as HTMLInputElement
    await fireEvent.input(input, { target: { value: 'ยังพิมพ์ไม่เสร็จ' } })

    cockpit.ask = { question: 'คำถามใหม่', options: ['ก', 'ข'] }
    await tick()

    expect((container.querySelector('.ask-own-input') as HTMLInputElement).value).toBe('')
  })

  // The guide chips on the welcome screen share the option styling but are not
  // a question the model asked — no answer field belongs on them.
  it('does not put an answer field on the guide chips', async () => {
    vi.mocked(GuideTopics).mockResolvedValue([{ id: 'skills', question: 'How do skills work?' }] as any)
    // The guide only appears on the built-in engine, between turns, with a
    // conversation already on screen.
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{ role: 'user', text: 'hi', time: '19:45' }] as any[],
      model: { ...baseProps.model, provider: 'aetox' },
    })
    await tick()
    await tick()

    expect(container.querySelector('.guide-card')).toBeTruthy()
    expect(container.querySelector('.ask-own-input')).toBeNull()
  })
})
