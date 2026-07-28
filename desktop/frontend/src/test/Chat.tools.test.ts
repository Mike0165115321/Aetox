import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit, applyToolEvent } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, SwitchApprovalMode } from './mocks/wailsApp'

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

  // §59: the model's narration between calls and a round's thinking duration
  // land as rows of their own kind — born finished, never counted as tools.
  it('keeps narration and thinking as their own closed rows', () => {
    applyToolEvent({ action: 'note', name: '', text: '  กำลังไล่หา config  ' })
    applyToolEvent({ action: 'thinking', name: '', secs: 12 })
    applyToolEvent({ action: 'note', name: '', text: '   ' }) // blank narration is noise
    expect(cockpit.toolSteps).toHaveLength(2)
    expect(cockpit.toolSteps[0]).toMatchObject({ kind: 'note', label: 'กำลังไล่หา config', state: 'done' })
    expect(cockpit.toolSteps[1]).toMatchObject({ kind: 'thinking', secs: 12, state: 'done' })
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

  // The latest narration stays on screen while the turn runs — that line is
  // what makes the agent read as working out loud instead of frozen (§59).
  it('keeps the latest narration on screen mid-turn', () => {
    const { container } = render(Chat, {
      ...baseProps, awaitingReply: true,
      messages: [{ role: 'user', text: 'go', time: '10:54' }] as any,
      toolSteps: [
        { kind: 'note', label: 'first thought', state: 'done', startedAt: 0 },
        { kind: 'note', label: 'scanning the loop', state: 'done', startedAt: 0 },
        step('browser_read', 'run'),
      ] as any,
    })
    expect(container.querySelector('.tool-note.headline')?.textContent).toBe('scanning the loop')
  })

  // In the finished timeline the narration and thinking rows render in place,
  // but the "used N tools" count stays a count of tools.
  it('renders narration and thinking inside the finished timeline, uncounted', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'done', time: '10:54',
        steps: [
          { kind: 'thinking', label: '', secs: 8, state: 'done', startedAt: 0 },
          { kind: 'note', label: 'reading the loop first', state: 'done', startedAt: 0 },
          step('read', 'done'),
        ],
      }] as any,
    })
    const toggle = [...container.querySelectorAll('.meta-row .reasoning-toggle')]
      .find((b) => b.textContent?.includes('Used 1'))
    expect(toggle).toBeTruthy()
    await fireEvent.click(toggle!)
    expect(container.querySelector('.tool-note')?.textContent).toBe('reading the loop first')
    expect(container.querySelector('.tool-think')?.textContent).toContain('8')
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

// The approval mode reads off the composer chip and is switched from the model
// menu. Shift+Tab is the shortcut, and it must never be able to REACH
// full-access: that mode never prompts again, so turning it on stays a
// deliberate pick from the menu.
describe('approval mode on the composer', () => {
  it('shows the mode on the chip, red only at full-access', () => {
    const ask = render(Chat, { ...baseProps, messages: [] as any }).container
    expect(ask.querySelector('.model-chip .mode-ic')?.textContent).toBe('✋')
    expect(ask.querySelector('.model-chip .mode-ic.danger')).toBeNull()

    const full = render(Chat, {
      ...baseProps, messages: [] as any,
      model: { ...baseProps.model, approval: 'full-access' },
    }).container
    expect(full.querySelector('.model-chip .mode-ic.danger')?.textContent).toBe('⚡')
  })

  it('shift+tab toggles ask↔unsafe-only and only ever tightens full-access', async () => {
    SwitchApprovalMode.mockClear()
    render(Chat, { ...baseProps, messages: [] as any })
    await fireEvent.keyDown(window, { key: 'Tab', shiftKey: true })
    expect(SwitchApprovalMode).toHaveBeenCalledWith('unsafe-only')

    SwitchApprovalMode.mockClear()
    render(Chat, {
      ...baseProps, messages: [] as any,
      model: { ...baseProps.model, approval: 'full-access' },
    })
    await fireEvent.keyDown(window, { key: 'Tab', shiftKey: true })
    expect(SwitchApprovalMode).toHaveBeenCalledWith('ask')
    expect(SwitchApprovalMode).not.toHaveBeenCalledWith('full-access')
  })
})

// A sub-agent's tool calls arrive on the same channel as the main agent's, told
// apart only by ToolEvent.parent (§44.5). The timeline has to show whose work is
// whose, and two delegates running the same tool must not share a row.
describe('sub-agent tool events', () => {
  beforeEach(() => { cockpit.toolSteps = [] })

  it('keeps a delegate’s row separate from an identical call by the main agent', () => {
    applyToolEvent({ action: 'call', name: 'grep', subject: 'needle', ref: 'main_1' })
    applyToolEvent({ action: 'call', name: 'grep', subject: 'needle', ref: 'sub_1', parent: 'task_1' })
    expect(cockpit.toolSteps.length).toBe(2)
    expect(cockpit.toolSteps[0].parent).toBeUndefined()
    expect(cockpit.toolSteps[1].parent).toBe('task_1')

    // Each result lands on its own row.
    applyToolEvent({ action: 'result', name: 'grep', subject: 'needle', ref: 'sub_1', parent: 'task_1', ok: true })
    expect(cockpit.toolSteps[1].state).toBe('done')
    expect(cockpit.toolSteps[0].state).toBe('run')
  })

  it('carries the sub-agent name and brief onto the task row', () => {
    applyToolEvent({
      action: 'call', name: 'task', subject: 'find every caller', ref: 'task_1',
      agent: 'explore', brief: 'search internal/ for callers of Resolve and list the paths',
    })
    expect(cockpit.toolSteps[0].agent).toBe('explore')
    expect(cockpit.toolSteps[0].brief).toContain('callers of Resolve')
  })

  // The delegation block is the whole point: one titled group per sub-agent,
  // with its own steps inside it and the brief the main agent wrote. A flat list
  // of rows cannot say who did what.
  it('draws a delegation as its own named block with the delegate’s steps inside', () => {
    const { container } = render(Chat, {
      ...baseProps,
      // The live rows only exist during a turn, which is when a delegate runs.
      awaitingReply: true,
      // 'run' because a finished step collapses behind the toggle; the live rows
      // are the ones on screen while a delegate is actually working.
      toolSteps: [
        {
          label: 'task find every caller', ref: 'task_1', agent: 'explore',
          brief: 'search internal/ for callers of Resolve', state: 'run', startedAt: Date.now(),
        },
        { label: 'grep needle', parent: 'task_1', state: 'run', startedAt: Date.now() },
        { label: 'read hay.txt', parent: 'task_1', state: 'run', startedAt: Date.now() },
      ] as any,
      messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
    })

    const block = container.querySelector('.subagent')
    expect(block).toBeTruthy()
    expect(block?.querySelector('.ag-name')?.textContent).toContain('explore')
    expect(block?.querySelector('.ag-job')?.textContent).toContain('find every caller')
    expect(block?.querySelector('.subagent-brief')?.textContent).toContain('callers of Resolve')
    // The delegate's tools live inside the block, not in the agent's own list.
    expect(block?.querySelectorAll('.subagent-steps .tool-step').length).toBe(2)
    expect(container.querySelectorAll('.tool-steps > .tool-step').length).toBe(0)
  })

  // Two delegates run at once and their events interleave on one channel — the
  // reason ToolEvent.parent exists at all. Each block must hold its own steps.
  it('keeps two concurrent delegations in separate blocks', () => {
    const { container } = render(Chat, {
      ...baseProps,
      awaitingReply: true,
      toolSteps: [
        { label: 'task hunt callers', ref: 't1', agent: 'explore', brief: 'brief one', state: 'run', startedAt: Date.now() },
        { label: 'task rename them', ref: 't2', agent: 'general', brief: 'brief two', state: 'run', startedAt: Date.now() },
        { label: 'grep alpha', parent: 't1', state: 'run', startedAt: Date.now() },
        { label: 'edit beta.go', parent: 't2', state: 'run', startedAt: Date.now() },
        { label: 'grep gamma', parent: 't1', state: 'run', startedAt: Date.now() },
      ] as any,
      messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
    })

    const blocks = container.querySelectorAll('.subagent')
    expect(blocks.length).toBe(2)
    expect(blocks[0].querySelector('.ag-name')?.textContent).toContain('explore')
    expect(blocks[1].querySelector('.ag-name')?.textContent).toContain('general')
    expect(blocks[0].querySelectorAll('.subagent-steps .tool-step').length).toBe(2)
    expect(blocks[1].querySelectorAll('.subagent-steps .tool-step').length).toBe(1)
    // No cross-contamination: the second delegate's edit is not in the first block.
    expect(blocks[0].textContent).not.toContain('edit beta.go')
    expect(blocks[1].textContent).not.toContain('grep alpha')
    expect(blocks[0].querySelector('.subagent-brief')?.textContent).toContain('brief one')
    expect(blocks[1].querySelector('.subagent-brief')?.textContent).toContain('brief two')
  })

  // A child whose task row is not in the list must still be visible. It happens
  // on a persisted turn and on out-of-order arrival, and silently dropping a row
  // means work that vanished.
  it('shows an orphaned delegate step rather than dropping it', () => {
    const { container } = render(Chat, {
      ...baseProps,
      awaitingReply: true,
      toolSteps: [
        { label: 'grep orphan', parent: 'gone', state: 'run', startedAt: Date.now() },
      ] as any,
      messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
    })
    expect(container.textContent).toContain('grep orphan')
  })

  it('counts sub-agents separately from tools in the collapsed line', () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'done', time: '10:54',
        steps: [
          { label: 'read a.txt', state: 'done', startedAt: 0 },
          { label: 'task find every caller', ref: 'task_1', agent: 'explore', state: 'done', startedAt: 0 },
          { label: 'grep needle', parent: 'task_1', state: 'done', startedAt: 0 },
          { label: 'task_result task_1', state: 'done', startedAt: 0 },
        ],
      }] as any,
    })
    // Two separate toggles, like thinking and tools are separate: what the agent
    // did itself, and what it handed to someone else.
    const toggles = [...container.querySelectorAll('.meta-row .reasoning-toggle')]
      .map((b) => b.textContent ?? '')
    expect(toggles.length).toBe(2)
    // Two tools of its own (read, task_result) — the delegate's grep is counted
    // inside its block — and one sub-agent.
    expect(toggles[0]).toContain('Used 2 tools')
    expect(toggles[1]).toContain('Sub-agents: 1')
    expect(toggles[0]).not.toContain('Sub-agents')
  })

  // Opening one panel closes the other, and each shows only its own kind.
  it('keeps the tools panel and the sub-agents panel apart', async () => {
    const steps = [
      { label: 'read a.txt', state: 'done', startedAt: 0 },
      { label: 'task hunt', ref: 't1', agent: 'explore', brief: 'go hunt', state: 'done', startedAt: 0 },
      { label: 'grep needle', parent: 't1', state: 'done', startedAt: 0 },
    ]
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{ role: 'agent', text: 'done', time: '10:54', steps }] as any,
    })
    const [toolsBtn, subsBtn] = [...container.querySelectorAll('.meta-row .reasoning-toggle')] as HTMLElement[]

    toolsBtn.click()
    await tick()
    expect(container.querySelector('.subagent')).toBeNull()
    expect(container.textContent).toContain('read a.txt')
    expect(container.querySelectorAll('.tool-steps .tool-step').length).toBe(1)

    subsBtn.click()
    await tick()
    // The tools panel closed with it — one slot, same as thinking.
    expect(container.querySelectorAll('.tool-steps > .tool-step').length).toBe(0)
    const block = container.querySelector('.subagent')
    expect(block).toBeTruthy()
    expect(block?.textContent).toContain('grep needle')
    expect(block?.querySelector('.subagent-brief')?.textContent).toContain('go hunt')
  })
})
