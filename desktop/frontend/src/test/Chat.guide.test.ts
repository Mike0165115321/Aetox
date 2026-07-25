import { describe, it, expect, beforeEach } from 'vitest'
import { render, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'

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

beforeEach(() => {
  setLocale('th')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
})

describe('guide card visibility', () => {
  it('shows the lettered options once Aetox has replied on its own engine', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      model: aetox as any,
      messages: [
        { role: 'user', text: 'ดีครับ', time: '10:54' },
        { role: 'agent', text: 'สวัสดีครับ Aetox ยังไม่ได้เชื่อมต่อกับโมเดลจริง', time: '10:54' },
      ] as any,
    })

    await waitFor(() => {
      expect(container.querySelector('.guide-card'), 'guide card should be in the transcript').toBeTruthy()
    })
    const opts = container.querySelectorAll('.guide-card .ask-opt')
    expect(opts.length).toBe(6)
    expect(opts[0].querySelector('.ask-key')?.textContent).toBe('A')
    expect(opts[5].querySelector('.ask-key')?.textContent).toBe('F')
  })

  // The layout hooks the stylesheet targets. jsdom applies no external CSS, so
  // this pins the markup contract rather than the pixels: the grid lives on
  // .guide-card .ask-opts, and every option is a direct child of it — an option
  // wrapped in anything else would silently fall out of the grid.
  it('keeps the markup the two-column grid is written against', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      model: aetox as any,
      messages: [{ role: 'agent', text: 'hi', time: '10:54' }] as any,
    })
    await waitFor(() => expect(container.querySelector('.guide-card')).toBeTruthy())

    const grid = container.querySelector('.guide-card .ask-opts')!
    expect(grid).toBeTruthy()
    expect(Array.from(grid.children).every((el) => el.classList.contains('ask-opt'))).toBe(true)
    // Full-width menu, not a 72% chat bubble.
    expect(container.querySelector('.msg.guide-card')).toBeTruthy()
  })

  it('stays hidden once a real provider is configured', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      model: { ...aetox, provider: 'deepseek', modelName: 'deepseek-chat' } as any,
      messages: [{ role: 'agent', text: 'hello', time: '10:54' }] as any,
    })
    expect(container.querySelector('.guide-card')).toBeNull()
  })

  it('stays hidden while a reply is still coming', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      awaitingReply: true,
      model: aetox as any,
      messages: [{ role: 'agent', text: 'hi', time: '10:54' }] as any,
    })
    expect(container.querySelector('.guide-card')).toBeNull()
  })
})
