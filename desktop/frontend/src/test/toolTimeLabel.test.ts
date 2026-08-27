// The collapsed tool row says how long its tools took, and the number has to be
// one the reader can check.
//
// The row is a control whose whole promise is what is behind it. If the summary
// and the detail disagree, the summary is not a summary, it is a second claim
// about the same thing, and the reader has no way to tell which one is wrong.
// So the label carries the SUM of the rows' own seconds, not the wall clock
// across them: tools run in parallel here, the two numbers genuinely differ,
// and only the sum is the one you get back by opening the panel and adding up
// what you see.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
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
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

const step = (label: string, state: 'done' | 'err', secs: number) =>
  ({ label, state, startedAt: 0, secs })

const withSteps = (steps: unknown[]) =>
  render(Chat, {
    ...baseProps,
    messages: [{ role: 'agent', text: 'done', time: '10:54', steps }] as any,
  })

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
})

describe('the collapsed tool row carries its own duration', () => {
  it('adds the seconds up and shows the total', () => {
    const { container } = withSteps([step('read', 'done', 2), step('grep', 'done', 6)])
    const toggle = container.querySelector('.meta-row .reasoning-toggle')
    expect(toggle?.textContent).toContain('Used 2 tools')
    expect(toggle?.textContent).toContain('8s')
  })

  // The claim the whole choice rests on: open it, add what you see, get the
  // number that was on the control.
  it('totals exactly what the rows underneath it show', async () => {
    const { container } = withSteps([step('read', 'done', 2), step('grep', 'done', 6), step('shell', 'done', 5)])
    const toggle = container.querySelector('.meta-row .reasoning-toggle')!
    expect(toggle.textContent).toContain('13s')

    await fireEvent.click(toggle)
    const rows = [...container.querySelectorAll('.tool-step .secs')]
    expect(rows.length).toBe(3)
    const summed = rows.reduce((n, el) => n + Number(el.textContent!.replace(/[^\d]/g, '')), 0)
    expect(summed).toBe(13)
  })

  // Failures are why someone opens the panel; a duration is not. Order says so.
  it('keeps failures next to the count, ahead of the time', () => {
    const { container } = withSteps([step('read', 'done', 2), step('web_fetch', 'err', 9)])
    const text = container.querySelector('.meta-row .reasoning-toggle')!.textContent!
    expect(text).toContain('1 failed')
    expect(text.indexOf('failed')).toBeLessThan(text.indexOf('11s'))
  })

  it('says nothing about time when there is nothing to say', () => {
    const { container } = withSteps([step('read', 'done', 0), step('glob', 'done', 0)])
    const text = container.querySelector('.meta-row .reasoning-toggle')!.textContent!
    expect(text).toContain('Used 2 tools')
    // Not "· 0s": a turn whose tools all came back inside a second has no
    // duration worth a reader's attention, and zero reads as broken.
    expect(text).not.toContain('0s')
  })
})
