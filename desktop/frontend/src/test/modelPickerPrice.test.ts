// The model picker in the composer, and the price it was not showing.
//
// The settings page has drawn "ฟรี" against a free model since the provider
// work landed; the picker in the composer — the place the model is actually
// chosen, and the only one most people ever open — listed the same names with
// nothing beside them. One fact with two answers depending on which screen you
// were standing on, and the screen that answered was the one you were not on.
//
// The rule under the badge is the provider's own marker, not a rate of zero
// (ARCHITECTURE/DECISIONS: of 22 OpenRouter models priced at zero, 15 carry
// `:free` and the other 7 are routers and previews nobody has published a price
// for). That rule lives in Go and is tested there; what these tests pin is that
// the answer it gives reaches this menu at all, and that an unknown price still
// draws nothing rather than a zero.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor, fireEvent } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { ListModelsForProvider, PriceModels, ModelPriceSource } from './mocks/wailsApp'

const baseProps = {
  task: { title: '', steps: [] } as never,
  messages: [] as never,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as never,
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: {
    provider: 'openrouter', modelName: 'poolside/laguna-s-2.1',
    thinkLevel: 'high', approval: 'ask', wireFormat: '',
  } as never,
}

const listing = (model: string, over: Record<string, unknown> = {}) => ({
  model, input: 0, output: 0, priced: false, free: false, context: 0, ...over,
})

// Open the composer's model chip, then the model row's dropdown inside it.
async function openModelList() {
  const chip = document.querySelector('.model-chip') as HTMLButtonElement
  expect(chip).toBeTruthy()
  await fireEvent.click(chip)
  const trigger = Array.from(document.querySelectorAll('.updrop-trigger'))
    .find((b) => b.textContent?.includes('laguna')) as HTMLButtonElement
  expect(trigger).toBeTruthy()
  await fireEvent.click(trigger)
  return () => Array.from(document.querySelectorAll('.updrop-opt'))
}

const rowFor = (opts: Element[], name: string) =>
  opts.find((o) => o.querySelector('.t')?.textContent === name)

beforeEach(() => {
  vi.clearAllMocks()
  // jsdom implements no layout, so Element.scrollIntoView is simply absent —
  // and this list scrolls the current model into view the moment it opens.
  Element.prototype.scrollIntoView = () => {}
  cockpit.chat = []
  cockpit.activeView = 'chat'
  vi.mocked(ListModelsForProvider).mockResolvedValue([
    'poolside/laguna-s-2.1',
    'poolside/laguna-s-2.1:free',
    'openrouter/pareto-code',
  ] as never)
  vi.mocked(PriceModels).mockResolvedValue([
    listing('poolside/laguna-s-2.1', { input: 0.09, output: 0.18, priced: true }),
    listing('poolside/laguna-s-2.1:free', { free: true }),
    // Zero rate, no marker: the catalog has no price for it. Not free.
    listing('openrouter/pareto-code'),
  ] as never)
})

describe('the composer model picker', () => {
  it('marks a free model as free, the same word the settings list uses', async () => {
    render(Chat, baseProps as never)
    const opts = await openModelList()

    await waitFor(() => expect(rowFor(opts(), 'poolside/laguna-s-2.1:free')).toBeTruthy())
    await waitFor(() => {
      const tag = rowFor(opts(), 'poolside/laguna-s-2.1:free')!.querySelector('.utag')
      expect(tag?.textContent).toBe('ฟรี')
      expect(tag?.className).toContain('free')
    })
  })

  it('prices the paid model per million rather than leaving it bare', async () => {
    render(Chat, baseProps as never)
    const opts = await openModelList()

    await waitFor(() => {
      const tag = rowFor(opts(), 'poolside/laguna-s-2.1')!.querySelector('.utag')
      expect(tag?.textContent).toContain('$0.09')
      expect(tag?.textContent).toContain('$0.18')
      expect(tag?.className).not.toContain('free')
    })
  })

  // The failure this guards against is the one that costs money: a row whose
  // price nobody has published rendering as "$0 / $0", which reads as free and
  // gets picked for exactly that reason.
  it('says nothing at all about a model it has no price for', async () => {
    render(Chat, baseProps as never)
    const opts = await openModelList()

    await waitFor(() => expect(rowFor(opts(), 'openrouter/pareto-code')).toBeTruthy())
    expect(rowFor(opts(), 'openrouter/pareto-code')!.querySelector('.utag')).toBeNull()
  })

  // Prices are fetched without awaiting them, so the names have to be pickable
  // before the money lands — and a provider that never answers must leave a
  // usable list rather than an empty one.
  it('still lists every model when the prices never arrive', async () => {
    vi.mocked(PriceModels).mockRejectedValue(new Error('no catalog') as never)
    render(Chat, baseProps as never)
    const opts = await openModelList()

    await waitFor(() => expect(opts().filter((o) => o.querySelector('.t')).length).toBeGreaterThanOrEqual(3))
    expect(document.querySelectorAll('.updrop-opt .utag').length).toBe(0)
  })
})

// Where the numbers come from, said on the menu that shows them.
//
// Owner, 28 ส.ค.: "ราคามันปลอม ทำไมเป็นแบบนี้เนี้ย". He was right, and Aetox
// had not invented anything: models.dev prices deepseek-v4-flash at
// $0.14/$0.28 while DeepSeek's own page states $0.22/$0.66 off-peak and
// $0.44/$1.32 peak, with cache hits at $0.007. The catalog is wrong, we copy it
// faithfully, and the row said nothing about either fact.
//
// The stats page has carried this qualification since it shipped ("estimated
// from published list prices, not an invoice"). What is pinned here is that the
// place the price is actually read carries it too — and, just as firmly, that
// a machine with no catalog says nothing rather than a sentence about a source
// it never had.
describe('where the picker says its prices came from', () => {
  it('names the source and the day it was fetched, under the priced rows', async () => {
    vi.mocked(ModelPriceSource).mockResolvedValue({
      name: 'models.dev', fetched: '2026-08-28T16:17:22+07:00',
    } as never)
    render(Chat, baseProps as never)
    await openModelList()

    await waitFor(() => {
      const foot = document.querySelector('.updrop-foot')
      expect(foot?.textContent).toContain('models.dev')
      // The date is rendered in the viewer's own locale, which for this app's
      // Thai users means the Buddhist year (28 ส.ค. 2569, not 2026). So the
      // assertion is the day and the absence of the raw timestamp, rather than
      // a format this test would have to duplicate and get wrong.
      expect(foot?.textContent).toContain('28')
      expect(foot?.textContent).not.toContain('T16:17')
    })
  })

  it('says nothing when no catalog has ever been fetched', async () => {
    vi.mocked(ModelPriceSource).mockResolvedValue({ name: '', fetched: '' } as never)
    render(Chat, baseProps as never)
    await openModelList()

    await waitFor(() => expect(document.querySelectorAll('.updrop-opt').length).toBeGreaterThan(0))
    expect(document.querySelector('.updrop-foot')).toBeNull()
  })

  // A source line over a list where nothing carries a price is a footnote to
  // nothing — and on a provider Aetox cannot price at all it would be the only
  // sentence about money on the screen.
  it('says nothing when not one row could be priced', async () => {
    vi.mocked(ModelPriceSource).mockResolvedValue({
      name: 'models.dev', fetched: '2026-08-28T16:17:22+07:00',
    } as never)
    vi.mocked(PriceModels).mockResolvedValue([
      listing('poolside/laguna-s-2.1'),
      listing('poolside/laguna-s-2.1:free'),
      listing('openrouter/pareto-code'),
    ] as never)
    render(Chat, baseProps as never)
    await openModelList()

    await waitFor(() => expect(document.querySelectorAll('.updrop-opt').length).toBeGreaterThan(0))
    expect(document.querySelector('.updrop-foot')).toBeNull()
  })
})
