import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit, applyToolEvent } from '../lib/stores/cockpit.svelte'
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
}

const step = (label: string, state: 'run' | 'done' | 'err') => ({ label, state, startedAt: 0, secs: 1 })

beforeEach(() => {
  setLocale('en')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
})

// turn.ToolEvent carries the outcome as a field. It used to be a formatted
// string the frontend matched the Thai word "สำเร็จ" against, so translating
// that word would have marked every call failed.
describe('tool events from the engine', () => {
  beforeEach(() => { cockpit.toolSteps = [] })

  it('reads success, the failure reason, and line counts off the event', () => {
    applyToolEvent({ action: 'call', name: 'write', subject: 'internal/skill/edit.go' })
    expect(cockpit.toolSteps[0]).toMatchObject({ label: 'write internal/skill/edit.go', state: 'run' })

    applyToolEvent({ action: 'result', name: 'write', ok: true, added: 9, removed: 0 })
    expect(cockpit.toolSteps[0]).toMatchObject({ state: 'done', added: 9 })
    expect(cockpit.toolSteps[0].removed).toBeUndefined() // 0 shows nothing

    applyToolEvent({ action: 'call', name: 'web_fetch', subject: 'https://openai.com/codex' })
    applyToolEvent({ action: 'result', name: 'web_fetch', ok: false, error: 'HTTP 403' })
    expect(cockpit.toolSteps[1]).toMatchObject({ state: 'err', error: 'HTTP 403' })
  })

  // The engine announces a call twice: once while its arguments stream, once
  // when it runs. One row, one clock.
  it('keeps one row when the streamed call is announced again on execution', () => {
    applyToolEvent({ action: 'call', name: 'write', subject: 'landing.html', added: 1 })
    applyToolEvent({ action: 'call', name: 'write', subject: 'landing.html', added: 40 })
    applyToolEvent({ action: 'call', name: 'write', subject: 'landing.html' })
    expect(cockpit.toolSteps).toHaveLength(1)
    expect(cockpit.toolSteps[0].added).toBe(40) // the counter climbs in place

    // ...but a genuine second call to the same file, after the first finished,
    // is its own row.
    applyToolEvent({ action: 'result', name: 'write', ok: true, added: 40, removed: 0 })
    applyToolEvent({ action: 'call', name: 'write', subject: 'landing.html' })
    expect(cockpit.toolSteps).toHaveLength(2)
  })

  it('drops the label to the bare tool name when a call has no subject', () => {
    applyToolEvent({ action: 'call', name: 'todo_write' })
    expect(cockpit.toolSteps[0].label).toBe('todo_write')
  })

  // Argument order is the model's choice: when a write's "content" streams
  // before its "path" the row appears unnamed and has to name itself later.
  // Keyed on the label, the arrival of the name drew a second row.
  it('names a row that started before its subject arrived, without splitting it', () => {
    applyToolEvent({ action: 'call', ref: 'call_1', name: 'write', added: 12 })
    expect(cockpit.toolSteps).toHaveLength(1)
    expect(cockpit.toolSteps[0]).toMatchObject({ label: 'write', added: 12 })

    applyToolEvent({ action: 'call', ref: 'call_1', name: 'write', added: 260 })
    applyToolEvent({ action: 'call', ref: 'call_1', name: 'write', subject: 'landing.html', added: 402 })
    expect(cockpit.toolSteps).toHaveLength(1)
    expect(cockpit.toolSteps[0]).toMatchObject({ label: 'write landing.html', added: 402 })

    applyToolEvent({ action: 'result', ref: 'call_1', name: 'write', subject: 'landing.html', ok: true, added: 402 })
    expect(cockpit.toolSteps).toHaveLength(1)
    expect(cockpit.toolSteps[0]).toMatchObject({ label: 'write landing.html', state: 'done' })
  })

  // Two writes in flight at once are two rows, even though both start unnamed
  // and share a tool name.
  it('keeps concurrent calls apart by ref', () => {
    applyToolEvent({ action: 'call', ref: 'call_1', name: 'write', added: 3 })
    applyToolEvent({ action: 'call', ref: 'call_2', name: 'write', added: 5 })
    expect(cockpit.toolSteps).toHaveLength(2)

    applyToolEvent({ action: 'result', ref: 'call_2', name: 'write', subject: 'b.html', ok: true })
    expect(cockpit.toolSteps[1]).toMatchObject({ label: 'write b.html', state: 'done' })
    expect(cockpit.toolSteps[0].state).toBe('run')
  })
})

describe('tool timeline collapsing', () => {
  it('collapses a finished turn behind a count, next to the thinking toggle', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'done', time: '10:54', reasoning: 'hmm', thinkSecs: 34,
        steps: [step('web_fetch', 'done'), step('web_fetch', 'err'), step('read', 'done')],
      }] as any,
    })

    const toggles = container.querySelectorAll('.meta-row .reasoning-toggle')
    expect(toggles.length).toBe(2)
    expect(toggles[0].textContent).toContain('Thought for 34s')
    expect(toggles[1].textContent).toContain('Used 3 tools')
    expect(toggles[1].textContent).toContain('✕1') // failures stay visible while collapsed
    expect(container.querySelector('.tool-step')).toBeNull()

    await fireEvent.click(toggles[1])
    expect(container.querySelectorAll('.tool-step').length).toBe(3)
  })

  // The two panels swap, they never stack: one slot, one open at a time.
  it('opening the tool list closes the thinking it replaces', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'done', time: '10:54', reasoning: 'hmm', thinkSecs: 34,
        steps: [step('web_fetch', 'done')],
      }] as any,
    })
    const [think, tools] = container.querySelectorAll('.meta-row .reasoning-toggle')

    await fireEvent.click(think)
    expect(container.querySelector('.reasoning-body')).toBeTruthy()

    await fireEvent.click(tools)
    expect(container.querySelector('.reasoning-body')).toBeNull()
    expect(container.querySelectorAll('.tool-step').length).toBe(1)

    await fireEvent.click(tools) // clicking the open one closes it again
    expect(container.querySelector('.tool-step')).toBeNull()
  })

  it('keeps only the running tool on screen mid-turn', () => {
    const { container } = render(Chat, {
      ...baseProps, awaitingReply: true,
      messages: [{ role: 'user', text: 'go', time: '10:54' }] as any,
      toolSteps: [step('web_fetch', 'done'), step('web_fetch', 'done'), step('browser_read', 'run')],
    })

    const steps = container.querySelectorAll('.tool-step')
    expect(steps.length).toBe(1)
    expect(steps[0].textContent).toContain('browser_read')
    expect(container.querySelector('.meta-row .reasoning-toggle')?.textContent).toContain('Used 2 tools')
  })
})
