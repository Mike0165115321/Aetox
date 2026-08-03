// A chat nobody has typed into showed "10.1k / 1000.0k" and a 9.5k line for the
// tool list. The owner's reaction was the point: it reads as a bill already run
// up, when in fact nothing has been sent and no tokens have been spent.
//
// Two numbers wear the same label — what the window *holds* and what the next
// request *will cost* — and the fix is to say which one is on screen.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { GetContextBreakdown, EnabledProviders, ListModelsForProvider } from './mocks/wailsApp'

const breakdown = (measured: boolean, used: number) => ({
  usedTokens: used,
  maxTokens: 1000000,
  measured,
  slices: [
    { key: 'system', tokens: 622 },
    { key: 'tools', tokens: 9500 },
    { key: 'messages', tokens: measured ? used - 10122 : 0 },
    { key: 'free', tokens: 1000000 - used },
  ],
})

const props = () => ({
  messages: [],
  task: { elapsed: '', steps: [] },
  model: {
    provider: 'aetox', modelName: 'test', thinkLevel: '', contextUsed: 0,
    contextMax: 0, approval: 'ask' as const, wireFormat: '', warning: '',
  },
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [],
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
})

beforeEach(() => {
  vi.clearAllMocks()
  EnabledProviders.mockResolvedValue(['aetox'])
  ListModelsForProvider.mockResolvedValue(['test'])
})

describe('the context meter before anything is sent', () => {
  it('presents the figure as what the first message will cost, not as spent', async () => {
    GetContextBreakdown.mockResolvedValue(breakdown(false, 10122) as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    // The heading changes, so the same 10.1k cannot be read as consumption.
    expect(await screen.findByText(/first message will use|ข้อความแรกจะใช้ประมาณ/)).toBeTruthy()
    // And it says outright that nothing has been spent.
    expect(screen.getByText(/Nothing has been sent yet|ยังไม่ได้ส่งอะไรเลย/)).toBeTruthy()
  })

  it('drops the caveat once the provider has counted a real round', async () => {
    GetContextBreakdown.mockResolvedValue(breakdown(true, 12040) as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    expect(await screen.findByText(/12.0k \/ 1000.0k/)).toBeTruthy()
    expect(screen.queryByText(/Nothing has been sent yet|ยังไม่ได้ส่งอะไรเลย/)).toBeNull()
    expect(screen.queryByText(/first message will use|ข้อความแรกจะใช้ประมาณ/)).toBeNull()
  })
})
