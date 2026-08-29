// Who a message is addressed to, and what it takes to address one.
//
// On 30 ส.ค. a user pasted their own release-notes draft into the composer —
// 8,486 characters ending in a design brief for four pictures. Four thousand
// characters in, inside a code span, the draft happened to say
// `เรียกใช้ได้ด้วย @reviewer`. The engine read the address out of the text, so
// the whole brief went to `reviewer`: a worker with four read-only tools, which
// spent 78 seconds listing files and could not have drawn a picture if it had
// tried. From the chat it looked like the app had gone quiet.
//
// The fix is that the text is no longer the evidence. A message goes to a worker
// only when the user picked one off the menu, and the composer sends that choice
// as its own argument. These tests hold that line from the window's side; the
// engine holds it again on arrival (subagent.Mention), and both halves matter —
// this one is what stops the choice being re-derived from words later.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, ListChairs } from './mocks/wailsApp'

const sent: Array<[string, string | undefined]> = []

const baseProps = {
  messages: [] as any[],
  task: { title: '', steps: [] } as any,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  onSend: (text: string, to?: string) => { sent.push([text, to]) },
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

beforeEach(() => {
  setLocale('en')
  sent.length = 0
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  cockpit.backgroundTasks = []
  cockpit.backgroundSteps = []
  cockpit.pendingImages = []
  cockpit.pendingFiles = []
  cockpit.pendingContexts = []
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
  vi.mocked(ListChairs).mockResolvedValue([
    { name: 'doc', description: 'เอกสาร', tools: [], builtin: true, icon: 'doc', jobs: 0 },
  ] as any)
})

/** The composer's own textarea — the transcript has none. */
function composer(container: HTMLElement): HTMLTextAreaElement {
  return container.querySelector('.composer textarea.input') as HTMLTextAreaElement
}

async function type(input: HTMLTextAreaElement, text: string) {
  await fireEvent.input(input, { target: { value: text } })
  await tick()
}

describe('addressing a worker from the composer', () => {
  // The regression, in one test. Every character of it was typed or pasted and
  // nothing was chosen, so it goes to the assistant like any other message.
  it('sends a pasted @name to the assistant, not to the worker it quotes', async () => {
    const { container } = render(Chat, baseProps as any)
    const input = composer(container)

    await type(input, 'เรียกใช้ได้ด้วย `@reviewer`\n\nทำภาพ 4 ภาพให้หน่อย')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(sent).toHaveLength(1)
    expect(sent[0][1]).toBe('') // nobody was addressed
    // And the composer never claimed otherwise while it was being written.
    expect(container.querySelector('.addressed')).toBeNull()
  })

  // The other direction: choosing off the menu is what sends a message
  // somewhere, and the message still carries the words it was written in.
  it('sends the chosen worker along with the message', async () => {
    const { container } = render(Chat, baseProps as any)
    const input = composer(container)

    await type(input, 'ช่วยดูให้หน่อย ')
    await fireEvent.keyDown(input, { key: '@' })
    await type(input, 'ช่วยดูให้หน่อย @')
    await vi.waitFor(() => expect(container.querySelector('.mention-item')).toBeTruthy())
    await fireEvent.click(container.querySelector('.mention-item')!)
    await tick()

    // Said out loud before it is sent, which is the half that was missing when
    // a turn could leave the room without the chat showing it.
    expect(container.querySelector('.addressed')?.textContent).toContain('@doc')

    await fireEvent.keyDown(composer(container), { key: 'Enter' })
    expect(sent).toHaveLength(1)
    expect(sent[0][0]).toContain('@doc')
    expect(sent[0][1]).toBe('doc')
  })

  // Deleting the token is changing your mind, and it should not need a second
  // act to take effect — the choice cannot outlive the words that carried it.
  it('drops the address when the token is deleted again', async () => {
    const { container } = render(Chat, baseProps as any)
    const input = composer(container)

    await fireEvent.keyDown(input, { key: '@' })
    await type(input, '@')
    await vi.waitFor(() => expect(container.querySelector('.mention-item')).toBeTruthy())
    await fireEvent.click(container.querySelector('.mention-item')!)
    await tick()
    expect(container.querySelector('.addressed')).toBeTruthy()

    await type(composer(container), 'ถามเฉย ๆ')
    expect(container.querySelector('.addressed')).toBeNull()

    await fireEvent.keyDown(composer(container), { key: 'Enter' })
    expect(sent[0][1]).toBe('')
  })

  // Mid-turn, what is typed goes INTO the running turn, which has no door to a
  // worker. Offering the menu there would take a choice and quietly drop it.
  it('does not open the roster while this chat is mid-turn', async () => {
    const { container } = render(Chat, { ...baseProps, awaitingReply: true } as any)
    const input = composer(container)

    await fireEvent.keyDown(input, { key: '@' })
    await type(input, '@')
    await tick()
    expect(container.querySelector('.mention-item')).toBeNull()
  })
})
