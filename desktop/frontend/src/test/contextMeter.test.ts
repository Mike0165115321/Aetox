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

const breakdown = (measured: boolean, used: number, cachedTokens = 0) => ({
  usedTokens: used,
  maxTokens: 1000000,
  measured,
  cachedTokens,
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

  // The provider reported that most of the prompt was a cache hit. A bar that
  // presents all 12k as paid at full rate every round is where "Aetox eats
  // tokens" comes from, so the meter must say what actually happened.
  it('says how much of the last round was a cache hit', async () => {
    GetContextBreakdown.mockResolvedValue(breakdown(true, 12040, 9800) as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    expect(await screen.findByText(/9.8k.*(cache|แคช)/)).toBeTruthy()
  })

  it('claims no cache hit when the provider reported none', async () => {
    GetContextBreakdown.mockResolvedValue(breakdown(true, 12040, 0) as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    await screen.findByText(/12.0k \/ 1000.0k/)
    expect(screen.queryByText(/cache|แคช/)).toBeNull()
  })
})

// maxTokens 0 is the backend saying nobody publishes this model's window
// (App.contextWindowTokens). It used to say a number instead, borrowed from the
// engine's char budget, and every Codex user read "32.0k" for a model the
// catalog puts at 1,050,000. The denominator is the part that has to disappear,
// not the meter: the size of the request is known and still worth showing.
describe('the context meter when the window is unknown', () => {
  const noWindow = (used: number) => ({
    usedTokens: used,
    maxTokens: 0,
    measured: true,
    cachedTokens: 0,
    // No free slice: nothing to subtract from.
    slices: [
      { key: 'system', tokens: 622 },
      { key: 'tools', tokens: 9500 },
      { key: 'messages', tokens: used - 10122 },
    ],
  })

  it('shows the size of the request and no fraction at all', async () => {
    GetContextBreakdown.mockResolvedValue(noWindow(12040) as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    expect(await screen.findByText(/12.0k (tokens|โทเคน)/)).toBeTruthy()
    // The shape that must never appear: a number over a number nobody knows.
    expect(screen.queryByText(/12.0k \/ /)).toBeNull()
    expect(screen.getByText(/window is not known|Nobody publishes|ยังไม่รู้ว่าโมเดลนี้รับได้เท่าไร/)).toBeTruthy()
  })

  it('never draws a free slice it cannot compute', async () => {
    GetContextBreakdown.mockResolvedValue(noWindow(12040) as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    await screen.findByText(/12.0k (tokens|โทเคน)/)
    expect(screen.queryByText(/Free space|พื้นที่ว่าง/)).toBeNull()
  })
})

// A prompt bigger than the window we believe in means the window is wrong, and
// that is exactly the state the old meter was incapable of showing: ctxPct was
// clamped to 100 and the bar just sat full. Nine rounds of one real install's
// history were in this state against a fabricated 32k ceiling, and the UI drew
// them as a normal, comfortably full bar.
describe('the context meter when the round exceeds the stated window', () => {
  it('reports the true percentage instead of pinning at 100', async () => {
    GetContextBreakdown.mockResolvedValue({
      usedTokens: 43434,
      maxTokens: 32000,
      measured: true,
      cachedTokens: 0,
      slices: [
        { key: 'system', tokens: 3400 },
        { key: 'tools', tokens: 8100 },
        { key: 'messages', tokens: 31934 },
        { key: 'free', tokens: 0 },
      ],
    } as any)

    render(Chat, props())
    const button = await screen.findByRole('button', { name: /Context window|หน้าต่างคอนเท็กซ์/ })
    button.click()

    expect(await screen.findByText(/43.4k \/ 32.0k \(136%\)/)).toBeTruthy()
    expect(screen.getByText(/larger than the window|ใหญ่กว่าหน้าต่าง/)).toBeTruthy()
  })
})
