// The brake on the delegation card in the transcript.
//
// The tray's card has had one since §163. This one never did, and that was
// worse than an oversight: a delegation drawn in the transcript is deliberately
// kept OUT of the tray (drawnDelegations — one delegation, one card), so for the
// whole of the turn that started it this is the only card there is. The
// composer's Stop is not a substitute; it ends the turn, and a delegate outlives
// the turn that started it on purpose. Owner, 30 ส.ค., over a `tester` on its
// 40th second: *"มีปุ่มหยุดเอเจนหลักทำไมไม่มีปุ่มหยุดซับเอเจนหรือเอเจนครับ"*.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, StopBackgroundTask } from './mocks/wailsApp'
import type { BackgroundTask } from '../lib/types'

const baseProps = {
  messages: [{ role: 'user', text: 'go', time: '10:54' }] as any,
  task: { title: '', steps: [] } as any,
  awaitingReply: true,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'ollama', modelName: 'gemma4:31b', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

const delegation = { label: 'task tester', ref: 'call_1', task: 'task_1', delegation: true, agent: 'tester', state: 'run', startedAt: 0 }

const registered = (over: Partial<BackgroundTask> = {}): BackgroundTask => ({
  id: 'task_1', agent: 'tester', label: 'ตรวจ build',
  startedAt: new Date().toISOString(), toolCalls: 4, tokens: 0,
  tokensIn: 0, tokensOut: 0, cachedIn: 0, cacheReported: false,
  state: 'running', collected: false, ...over,
})

beforeEach(() => {
  setLocale('th')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  cockpit.backgroundRuns = []
  cockpit.backgroundSteps = []
  cockpit.backgroundTasks = [registered()]
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
  vi.mocked(StopBackgroundTask).mockClear()
})

describe('stopping a delegate from the card in the transcript', () => {
  it('offers a brake beside the clock while it works', async () => {
    const { container } = render(Chat, { ...baseProps, toolSteps: [delegation] } as any)

    const card = container.querySelector('.bgw-card.run')
    expect(card).toBeTruthy()
    const stop = card!.querySelector('.bgw-head .bgw-stop') as HTMLButtonElement
    expect(stop).toBeTruthy()

    await fireEvent.click(stop)
    expect(vi.mocked(StopBackgroundTask)).toHaveBeenCalledWith('task_1')
  })

  // A delegate waiting for a free slot has not begun, which is the cheapest
  // moment there is to change your mind about it.
  it('offers it to a delegate that has not started yet', () => {
    cockpit.backgroundTasks = [registered({ state: 'queued', toolCalls: 0 })]
    const { container } = render(Chat, { ...baseProps, toolSteps: [delegation] } as any)

    const card = container.querySelector('.bgw-card.is-queued')
    expect(card).toBeTruthy()
    expect(card!.querySelector('.bgw-head .bgw-stop')).toBeTruthy()
  })

  // And never on work that is over. A button that cannot do anything is worse
  // than no button: it is the card claiming there is still something to stop.
  //
  // Asserted over the whole transcript rather than on the card, because a
  // finished delegation folds away with the rest of the finished work — so
  // "the button is gone" has to be true of the page, not of an element that is
  // no longer on it.
  it('takes it away once the work has finished', () => {
    cockpit.backgroundTasks = [registered({ state: 'done' })]
    const { container } = render(Chat, { ...baseProps, toolSteps: [delegation] } as any)

    expect(container.querySelector('.bgw-stop')).toBeNull()
  })
})
