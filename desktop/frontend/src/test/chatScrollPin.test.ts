// Pinned auto-scroll must let go the moment the user scrolls up — however
// little. The old rule ("unpinned = more than 80px from the bottom") judged a
// touchpad's few-px wheel ticks as still-at-bottom, and the next stream chunk
// snapped the view back down before the next tick could escape the band. Mid-
// reply, the transcript felt glued to the floor: "เลื่อนขึ้นไม่ได้เลย".
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics } from './mocks/wailsApp'

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
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
  messages: [{ role: 'user', text: 'ไปสิ', time: '22:19' }] as any,
}

beforeEach(() => {
  setLocale('en')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
})

/** A .chat element with fake geometry — jsdom has no layout, so the two
 * read-only dimensions are pinned by hand and scrollTop is set per step. */
function chatWithGeometry(container: HTMLElement, scrollHeight: number, clientHeight: number): HTMLDivElement {
  const el = container.querySelector('.chat') as HTMLDivElement
  Object.defineProperty(el, 'scrollHeight', { configurable: true, value: scrollHeight })
  Object.defineProperty(el, 'clientHeight', { configurable: true, value: clientHeight })
  return el
}

// The button is the pin state made visible: it exists exactly while unpinned.
const unpinned = (c: HTMLElement) => c.querySelector('.scroll-bottom') !== null

describe('chat scroll pinning', () => {
  it('unpins on the smallest upward scroll, not only past an 80px band', async () => {
    const { container } = render(Chat, baseProps)
    const el = chatWithGeometry(container, 1000, 400)

    el.scrollTop = 600 // at the bottom
    await fireEvent.scroll(el)
    await tick()
    expect(unpinned(container)).toBe(false)

    el.scrollTop = 595 // one touchpad tick up — 5px, well inside the old band
    await fireEvent.scroll(el)
    await tick()
    expect(unpinned(container)).toBe(true)
  })

  it('re-pins when the user comes back near the bottom', async () => {
    const { container } = render(Chat, baseProps)
    const el = chatWithGeometry(container, 1000, 400)

    el.scrollTop = 600
    await fireEvent.scroll(el)
    el.scrollTop = 300 // reading something above
    await fireEvent.scroll(el)
    await tick()
    expect(unpinned(container)).toBe(true)

    el.scrollTop = 590 // scrolled back down, within the re-pin band
    await fireEvent.scroll(el)
    await tick()
    expect(unpinned(container)).toBe(false)
  })

  it('stays pinned when the transcript shrinks and clamps scrollTop', async () => {
    const { container } = render(Chat, baseProps)
    const el = chatWithGeometry(container, 1000, 400)

    el.scrollTop = 600
    await fireEvent.scroll(el)

    // Undo/regenerate shortened the transcript: the browser clamps scrollTop
    // to the new bottom. scrollTop dropped, but the user did nothing — a clamp
    // lands AT the bottom, which is what tells it apart from a real scroll.
    chatWithGeometry(container, 800, 400)
    el.scrollTop = 400
    await fireEvent.scroll(el)
    await tick()
    expect(unpinned(container)).toBe(false)
  })
})
