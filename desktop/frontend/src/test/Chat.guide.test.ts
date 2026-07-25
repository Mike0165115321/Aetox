import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor, fireEvent } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics } from './mocks/wailsApp'

const TOPICS = [
  { id: 'skills', question: 'สกิล กับ พรอมต์สำเร็จรูป ต่างกันยังไง' },
  { id: 'prompts', question: 'พรอมต์สำเร็จรูปใช้ยังไง' },
  { id: 'connect', question: 'ต่อโมเดลจริงยังไง ทำไมต้องต่อเอง' },
]

const baseProps = {
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
}

const aetox = { provider: 'aetox', modelName: 'aetox-grid', thinkLevel: 'low', approval: 'ask', wireFormat: '' }
const reply = (text: string) => ({ role: 'agent', text, time: '10:54' })
const asked = (text: string) => ({ role: 'user', text, time: '10:54' })

beforeEach(() => {
  setLocale('th')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  vi.mocked(GuideTopics).mockResolvedValue(TOPICS as any)
})

describe('guide options', () => {
  it('renders the engine-supplied questions as lettered options', async () => {
    const { container } = render(Chat, {
      ...baseProps, model: aetox as any, messages: [reply('สวัสดีครับ')] as any,
    })

    await waitFor(() => expect(container.querySelectorAll('.guide-card .ask-opt').length).toBe(3))
    const opts = container.querySelectorAll('.guide-card .ask-opt')
    expect(opts[0].querySelector('.ask-key')?.textContent).toBe('A')
    expect(opts[0].textContent).toContain('สกิล')
  })

  // The whole point of §42: a click is an ordinary send. Everything else —
  // streaming, persistence, scrolling — follows from that and needs no code.
  it('clicking sends the question through the normal send path', async () => {
    const sent: string[] = []
    const { container } = render(Chat, {
      ...baseProps, model: aetox as any, onSend: (t: string) => sent.push(t),
      messages: [reply('สวัสดีครับ')] as any,
    })
    await waitFor(() => expect(container.querySelector('.guide-card')).toBeTruthy())

    await fireEvent.click(container.querySelectorAll('.guide-card .ask-opt')[1])
    expect(sent).toEqual(['พรอมต์สำเร็จรูปใช้ยังไง'])
  })

  // "Already asked" is read off the transcript, so it survives a reload and
  // cannot drift from what is on screen.
  it('drops questions already present in the transcript', async () => {
    const { container } = render(Chat, {
      ...baseProps, model: aetox as any,
      messages: [reply('hi'), asked('พรอมต์สำเร็จรูปใช้ยังไง'), reply('...')] as any,
    })

    await waitFor(() => expect(container.querySelectorAll('.guide-card .ask-opt').length).toBe(2))
    const labels = Array.from(container.querySelectorAll('.guide-card .ask-label')).map((el) => el.textContent)
    expect(labels).not.toContain('พรอมต์สำเร็จรูปใช้ยังไง')
  })

  it('stays hidden for a real provider and while a reply is coming', async () => {
    const real = render(Chat, {
      ...baseProps, model: { ...aetox, provider: 'deepseek' } as any, messages: [reply('hi')] as any,
    })
    expect(real.container.querySelector('.guide-card')).toBeNull()

    const busy = render(Chat, {
      ...baseProps, awaitingReply: true, model: aetox as any, messages: [reply('hi')] as any,
    })
    expect(busy.container.querySelector('.guide-card')).toBeNull()
  })

  // The layout hooks the stylesheet targets — jsdom applies no external CSS,
  // so this pins the markup contract rather than the pixels.
  it('keeps the markup the two-column grid is written against', async () => {
    const { container } = render(Chat, {
      ...baseProps, model: aetox as any, messages: [reply('hi')] as any,
    })
    await waitFor(() => expect(container.querySelector('.guide-card')).toBeTruthy())

    const grid = container.querySelector('.guide-card .ask-opts')!
    expect(Array.from(grid.children).every((el) => el.classList.contains('ask-opt'))).toBe(true)
  })
})
