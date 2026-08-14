// What you can do to your own message.
//
// The bubble carried nothing but a pencil, and only on the last one. Copying
// what you wrote is always safe and never depends on position, so it belongs on
// every user message; asking the same question again does depend on position,
// and stays on the last one.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor, fireEvent } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, ResendEdited, RetryFailedTurn } from './mocks/wailsApp'

const userMsg = (text: string, time = '13:15') => ({ role: 'user', text, time })
const agentMsg = (text: string, id = 'a1') => ({ role: 'agent', text, id, time: '13:16' })

// The bubbles render from the `messages` prop, but resending reads the
// transcript out of the store — so a test that only seeds one of the two is
// testing a screen the app never shows.
const propsWith = (messages: any[]) => (cockpit.chat = messages as any) && ({
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
  model: { provider: 'codex', modelName: 'gpt-5.6-terra', thinkLevel: 'medium', approval: 'ask', wireFormat: '' } as any,
  messages: messages as any,
})

const userBubbles = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('.msg.user'))

const labels = (el: Element) =>
  Array.from(el.querySelectorAll('button')).map((b) => b.getAttribute('aria-label') ?? '')

const buttonNamed = (el: Element, label: string) =>
  Array.from(el.querySelectorAll('button')).find((b) => b.getAttribute('aria-label') === label)

const lastUserBubble = async (container: HTMLElement) =>
  waitFor(() => {
    const all = userBubbles(container)
    if (!all.length) throw new Error('no user bubble rendered')
    return all[all.length - 1]
  })

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('th')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  cockpit.undoFiles = []
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
})

describe('actions on your own message', () => {
  it('offers copy on every user message, not just the last', async () => {
    const { container } = render(Chat, propsWith([
      userMsg('คำถามแรก'), agentMsg('ตอบแรก'), userMsg('คำถามสอง'), agentMsg('ตอบสอง'),
    ]))

    await waitFor(() => expect(userBubbles(container).length).toBe(2))
    for (const bubble of userBubbles(container)) {
      expect(labels(bubble)).toContain('คัดลอก')
    }
  })

  // Re-asking an earlier question would answer into the middle of a
  // conversation that has since moved on.
  it('offers ask-again only on the last question', async () => {
    const { container } = render(Chat, propsWith([
      userMsg('คำถามแรก'), agentMsg('ตอบแรก'), userMsg('คำถามสอง'), agentMsg('ตอบสอง'),
    ]))

    await waitFor(() => expect(userBubbles(container).length).toBe(2))
    const [first, last] = userBubbles(container)
    expect(labels(first)).not.toContain('ส่งใหม่')
    expect(labels(last)).toContain('ส่งใหม่')
  })

  it('ask-again re-sends the same text', async () => {
    const { container } = render(Chat, propsWith([
      userMsg('ไปหาข้อมูล Lenovo LOQ 15AHP11'), agentMsg('ตอบ'),
    ]))

    const bubble = await lastUserBubble(container)
    await fireEvent.click(buttonNamed(bubble, 'ส่งใหม่')!)

    await waitFor(() =>
      expect(vi.mocked(ResendEdited)).toHaveBeenCalledWith('ไปหาข้อมูล Lenovo LOQ 15AHP11', false))
  })

  // A turn that wrote files would run its tools a second time on top of their
  // own output, so this asks before doing it — the same guard the edit path
  // already had, which is why ask-again goes through that path rather than
  // posting a fresh message.
  it('asks first when the last turn wrote files', async () => {
    cockpit.undoFiles = ['out/report.md']
    const { container } = render(Chat, propsWith([
      userMsg('เขียนไฟล์ให้หน่อย'), agentMsg('เขียนแล้ว'),
    ]))

    const bubble = await lastUserBubble(container)
    await fireEvent.click(buttonNamed(bubble, 'ส่งใหม่')!)

    expect(vi.mocked(ResendEdited)).not.toHaveBeenCalled()
  })
})

// A turn that never reached the model leaves the question live and unanswered.
// The red bubble under it has always offered "try that again"; the question
// itself offered nothing but copy, which is the wrong answer when the reason it
// failed is that it was asked wrong.
const failedMsg = (failedText: string) => ({
  role: 'agent', text: '', failed: true, failedText, time: '13:16',
})

describe('after a turn fails', () => {
  it('the question can be reworded, and reruns through the retry path', async () => {
    const { container } = render(Chat, propsWith([
      userMsg('ถามผิด'), failedMsg('ถามผิด'),
    ]))

    const bubble = await lastUserBubble(container)
    await fireEvent.click(buttonNamed(bubble, 'แก้ข้อความ')!)

    const box = await waitFor(() => {
      const el = container.querySelector('textarea.msg-edit')
      if (!el) throw new Error('no editor')
      return el as HTMLTextAreaElement
    })
    await fireEvent.input(box, { target: { value: 'ถามใหม่' } })
    await fireEvent.keyDown(box, { key: 'Enter' })

    // RetryFailedTurn, not ResendEdited: the tail being removed is a failure,
    // not a completed exchange.
    await waitFor(() => expect(vi.mocked(RetryFailedTurn)).toHaveBeenCalledWith('ถามใหม่'))
    expect(vi.mocked(ResendEdited)).not.toHaveBeenCalled()
  })

  // Retry already sits on the red bubble. A second control sending the very
  // same text from the question above it would be two buttons for one act.
  it('offers no ask-again, because the failed bubble already retries', async () => {
    const { container } = render(Chat, propsWith([
      userMsg('ถามผิด'), failedMsg('ถามผิด'),
    ]))

    const bubble = await lastUserBubble(container)
    expect(labels(bubble)).toContain('แก้ข้อความ')
    expect(labels(bubble)).not.toContain('ส่งใหม่')
  })
})
