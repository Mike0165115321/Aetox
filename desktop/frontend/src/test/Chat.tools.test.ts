import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit, applyToolEvent, resetBackgroundWork } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, SwitchApprovalMode, SupportedThinkLevels, BackgroundTasks } from './mocks/wailsApp'

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
  // The turn timeline and the background tray draw the same card now (§105.5),
  // so a leftover background task would be counted as one of the turn's own.
  cockpit.backgroundTasks = []
  cockpit.backgroundSteps = []
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

  // An answer the user typed over arrives on the same channel as narration and
  // is not the same thing: it is a finished reply, and it kept its own kind so
  // the chat can draw it as one.
  it('keeps an answer an interjection re-placed as prose, not as a narration row', () => {
    applyToolEvent({ action: 'said', name: '', text: '  ## สรุป\n\n- ข้อหนึ่ง  ' })
    applyToolEvent({ action: 'said', name: '', text: '  ' }) // nothing said is nothing to keep
    expect(cockpit.toolSteps).toHaveLength(1)
    expect(cockpit.toolSteps[0]).toMatchObject({
      kind: 'said', label: '## สรุป\n\n- ข้อหนึ่ง', state: 'done',
    })
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

  // A sub-agent outlives the turn that started it, so its steps keep arriving
  // after the live block is gone. toolSteps is cleared at both ends of a turn,
  // so without a second home those rows land in the NEXT turn's timeline and
  // are drawn as work the user's new question caused.
  describe('a delegate still working after its turn ended', () => {
    beforeEach(() => { cockpit.backgroundSteps = [] })

    it('keeps its steps out of the next turn and in the background list', () => {
      // Turn 1: the delegation opens and its first step lands in the timeline.
      applyToolEvent({ action: 'call', ref: 'task_call', name: 'task', agent: 'explore' })
      applyToolEvent({ action: 'call', ref: 'g1', parent: 'task_call', name: 'grep', subject: 'Resolve' })
      expect(cockpit.toolSteps).toHaveLength(2)
      expect(cockpit.backgroundSteps).toHaveLength(0)

      // The turn ends: the answer went out, the live lists are cleared.
      cockpit.toolSteps = []

      // The delegate is still working, and says so. The result for g1 closes
      // nothing — that row went into turn 1's snapshot and is not here to
      // close — but the new call has somewhere to go that is not turn 2.
      applyToolEvent({ action: 'result', ref: 'g1', parent: 'task_call', name: 'grep', ok: true })
      applyToolEvent({ action: 'call', ref: 'r1', parent: 'task_call', name: 'read', subject: 'turn/executor.go' })
      expect(cockpit.toolSteps).toHaveLength(0)
      expect(cockpit.backgroundSteps).toHaveLength(1)
      expect(cockpit.backgroundSteps[0]).toMatchObject({ label: 'read turn/executor.go', state: 'run' })
    })

    it('still puts a delegate started in this turn into this turn', () => {
      applyToolEvent({ action: 'call', ref: 'task_call', name: 'task', agent: 'doc' })
      applyToolEvent({ action: 'call', ref: 'r1', parent: 'task_call', name: 'read', subject: 'README.md' })
      expect(cockpit.backgroundSteps).toHaveLength(0)
      expect(cockpit.toolSteps).toHaveLength(2)
    })

    // The tray's rows come from the engine's register, and the first background
    // event is what turns the poll on — without this a reloaded window would
    // never learn that anything is running.
    it('starts the tray poll when the first background event arrives', async () => {
      vi.mocked(BackgroundTasks).mockResolvedValue([
        { id: 'task_9', agent: 'explore', label: 'x', startedAt: new Date().toISOString(), toolCalls: 3, state: 'running', collected: false },
      ] as any)
      applyToolEvent({ action: 'call', ref: 'r9', parent: 'ghost_task', name: 'read', subject: 'a.go' })
      await vi.waitFor(() => expect(cockpit.backgroundTasks).toHaveLength(1))
      expect(cockpit.backgroundTasks[0]).toMatchObject({ id: 'task_9', state: 'running' })
    })
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
    expect(toggles[1].textContent).toContain('1 failed') // failures stay visible while collapsed
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

  // Owner, 16 ส.ค.: interjecting mid-turn looked like it wrecked everything the
  // agent had already answered. It was drawn as a note — raw source at --fs-xs
  // in --text-muted — and then filed behind the tools toggle. It is prose, so it
  // is drawn as prose, in the bubble, both while the turn runs and after.
  it('draws an answer the user typed over as markdown in the live block', () => {
    const { container } = render(Chat, {
      ...baseProps, awaitingReply: true,
      messages: [{ role: 'user', text: 'go', time: '10:54' }] as any,
      toolSteps: [
        { kind: 'said', label: '## สรุป\n\nเรียบร้อยครับ', state: 'done', startedAt: 0 },
        step('grep', 'run'),
      ] as any,
    })
    const said = container.querySelector('.said-block')
    expect(said?.querySelector('h2')?.textContent).toBe('สรุป')
    // Not as a narration row, and never inside the tool timeline.
    expect(container.querySelector('.tool-note')).toBeNull()
    expect(said?.closest('.tool-step')).toBeNull()
  })

  it('keeps that answer in the bubble after the turn ends, uncounted as a tool', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'เช็คแล้วครับ', time: '10:54',
        steps: [
          { kind: 'said', label: '## สรุป\n\nเรียบร้อยครับ', state: 'done', startedAt: 0 },
          step('grep', 'done'),
        ],
      }] as any,
    })
    expect(container.querySelector('.said-block h2')?.textContent).toBe('สรุป')
    // The answer that ended the turn is still the body below it.
    expect(container.textContent).toContain('เช็คแล้วครับ')

    const toggle = [...container.querySelectorAll('.meta-row .reasoning-toggle')]
      .find((b) => b.textContent?.includes('Used 1'))
    expect(toggle).toBeTruthy() // one grep, not two "tools"
    await fireEvent.click(toggle!)
    // Opening the tools panel shows the grep and nothing else — an answer is
    // not a step, and a paragraph in that list is a paragraph pretending to be
    // one.
    expect(container.querySelectorAll('.tool-step')).toHaveLength(1)
    expect(container.querySelectorAll('.said-block')).toHaveLength(1)
  })

  // The button and the number on it have to be asked in the same terms. They
  // were not: the button appeared for any own step, the count excluded
  // narration and thinking, and a turn that only narrated offered "Used 0
  // tools" — a control whose whole promise is that there is nothing behind it.
  it('offers no tool toggle for a turn that only narrated', () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'done', time: '10:54',
        steps: [{ kind: 'note', label: 'looking at the sidebar first', state: 'done', startedAt: 0 }],
      }] as any,
    })

    const toggles = [...container.querySelectorAll('.meta-row .reasoning-toggle')]
    expect(toggles.some((b) => b.textContent?.includes('Used 0'))).toBe(false)
    // And no empty bar left standing where the toggle would have been.
    expect(container.querySelector('.meta-row')).toBeNull()
  })

  it('offers no tool toggle mid-turn either, on the same narration', () => {
    const { container } = render(Chat, {
      ...baseProps, awaitingReply: true,
      messages: [{ role: 'user', text: 'go', time: '10:54' }] as any,
      toolSteps: [{ kind: 'note', label: 'looking at the sidebar first', state: 'done', startedAt: 0 }] as any,
    })

    const toggles = [...container.querySelectorAll('.reasoning-toggle')]
    expect(toggles.some((b) => b.textContent?.includes('Used 0'))).toBe(false)
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

// The approval mode is switched from the model menu. Shift+Tab is the
// shortcut, and it must never be able to REACH full-access: that mode never
// prompts again, so turning it on stays a deliberate pick from the menu.
describe('approval mode on the composer', () => {
  // The mode had a glyph on the chip until the model name moved back beside it
  // and three marks at 14px stopped reading as three facts (owner's call). It
  // is named in words in the menu, which is where it is changed — so the chip
  // must not grow one back without that decision being revisited.
  it('keeps the approval glyph off the chip, whatever the mode', () => {
    for (const approval of ['ask', 'unsafe-only', 'full-access']) {
      const { container } = render(Chat, {
        ...baseProps, messages: [] as any,
        model: { ...baseProps.model, approval },
      })
      expect(container.querySelector('.model-chip .mode-ic')).toBeNull()
    }
  })

  // What the chip does carry: which provider is answering, and which of its
  // models. The name was off for a while on width grounds and is back by the
  // owner's call — reading it should not cost a menu. Width is handled by
  // truncation, not by omission (see the vendor-prefix test below).
  it('carries the provider as a mark and names the model', () => {
    const { container } = render(Chat, { ...baseProps, messages: [] as any })
    const chip = container.querySelector('.model-chip')!
    expect(chip.querySelector('.pv svg, .pv .pv-letter')).toBeTruthy()
    expect(chip.textContent).toContain('v4')
    // The full id stays in the title, which is what the truncated name points at.
    expect(chip.getAttribute('title')).toBe('v4')
  })

  // OpenRouter and Together spell their ids `vendor/model`. The mark beside the
  // text already says who made it, so repeating the vendor spends width on the
  // half of the string that is never the answer to "which model is this".
  it('drops the vendor prefix from the name but keeps it in the title', () => {
    const { container } = render(Chat, {
      ...baseProps, messages: [] as any,
      model: { ...baseProps.model, modelName: 'deepseek/deepseek-r1' },
    })
    const chip = container.querySelector('.model-chip')!
    expect(chip.querySelector('.t')!.textContent).toBe('deepseek-r1')
    expect(chip.getAttribute('title')).toBe('deepseek/deepseek-r1')
  })

  // The menu below only draws a think-level row when there are two levels to
  // pick between, so keying the badge off model.thinkLevel alone advertised a
  // setting the menu then offered no way to move.
  it('badges the think level only when the menu can actually change it', async () => {
    SupportedThinkLevels.mockResolvedValueOnce(['high'])
    const one = render(Chat, { ...baseProps, messages: [] as any }).container
    await tick(); await tick()
    expect(one.querySelector('.model-chip .lvl')).toBeNull()

    SupportedThinkLevels.mockResolvedValueOnce(['low', 'high'])
    const two = render(Chat, { ...baseProps, messages: [] as any }).container
    await tick(); await tick()
    expect(two.querySelector('.model-chip .lvl')?.textContent).toContain('high')
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
    // The `task` row first, as the engine sends it: a delegate's parent is the
    // ref of the call that spawned it (executor.go stamps both from call.ID),
    // and it is that row's presence in this turn that says the work is this
    // turn's — see listFor.
    applyToolEvent({ action: 'call', name: 'task', ref: 'task_1', agent: 'explore' })
    applyToolEvent({ action: 'call', name: 'grep', subject: 'needle', ref: 'main_1' })
    applyToolEvent({ action: 'call', name: 'grep', subject: 'needle', ref: 'sub_1', parent: 'task_1' })
    expect(cockpit.toolSteps.length).toBe(3)
    expect(cockpit.toolSteps[1].parent).toBeUndefined()
    expect(cockpit.toolSteps[2].parent).toBe('task_1')

    // Each result lands on its own row.
    applyToolEvent({ action: 'result', name: 'grep', subject: 'needle', ref: 'sub_1', parent: 'task_1', ok: true })
    expect(cockpit.toolSteps[2].state).toBe('done')
    expect(cockpit.toolSteps[1].state).toBe('run')
  })

  it('carries the sub-agent name and brief onto the task row', () => {
    applyToolEvent({
      action: 'call', name: 'task', subject: 'find every caller', ref: 'task_1',
      agent: 'explore', brief: 'search internal/ for callers of Resolve and list the paths',
    })
    expect(cockpit.toolSteps[0].agent).toBe('explore')
    expect(cockpit.toolSteps[0].brief).toContain('callers of Resolve')
  })

  // The row is usually born from the streaming progress announcement, fired
  // while the model is still writing the call's arguments — no agent, no
  // brief, no kind yet. The executor's own call event arrives after, carrying
  // all three, and must land on the SAME row. Dropping them was a live turn
  // reading "ซับเอเจน 1 ตัว" for a job the doc agent did, while the
  // persisted parts said agent/doc — two answers from one turn.
  it('fills in the delegation facts when they arrive after the row was born', () => {
    applyToolEvent({ action: 'call', name: 'task', ref: 'task_1' })
    expect(cockpit.toolSteps[0].agent).toBeUndefined()

    applyToolEvent({
      action: 'call', name: 'task', subject: 'ทำสรุป 5 ข้อ', ref: 'task_1',
      agent: 'doc', agentKind: 'agent', brief: 'สร้างไฟล์เอกสาร .docx',
    })
    expect(cockpit.toolSteps.length).toBe(1)
    expect(cockpit.toolSteps[0].agent).toBe('doc')
    expect(cockpit.toolSteps[0].agentKind).toBe('agent')
    expect(cockpit.toolSteps[0].brief).toContain('.docx')
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

    const block = container.querySelector('.bgw-card')
    expect(block).toBeTruthy()
    expect(block?.querySelector('.bgw-agent')?.textContent).toContain('explore')
    expect(block?.querySelector('.bgw-brief')?.textContent).toContain('find every caller')
    expect(block?.querySelector('.bgw-longbrief')?.textContent).toContain('callers of Resolve')
    // The delegate's tools live inside the block, not in the agent's own list.
    expect(block?.querySelectorAll('.bgw-steps .tool-step').length).toBe(2)
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

    const blocks = container.querySelectorAll('.bgw-card')
    expect(blocks.length).toBe(2)
    expect(blocks[0].querySelector('.bgw-agent')?.textContent).toContain('explore')
    expect(blocks[1].querySelector('.bgw-agent')?.textContent).toContain('general')
    expect(blocks[0].querySelectorAll('.bgw-steps .tool-step').length).toBe(2)
    expect(blocks[1].querySelectorAll('.bgw-steps .tool-step').length).toBe(1)
    // No cross-contamination: the second delegate's edit is not in the first block.
    expect(blocks[0].textContent).not.toContain('edit beta.go')
    expect(blocks[1].textContent).not.toContain('grep alpha')
    expect(blocks[0].querySelector('.bgw-longbrief')?.textContent).toContain('brief one')
    expect(blocks[1].querySelector('.bgw-longbrief')?.textContent).toContain('brief two')
  })

  // The `task` tool call finishes the instant the worker is spawned, so the row
  // it leaves behind says "done, 8s" for the whole job. The card is drawn from
  // the engine's register instead — the same source the tray reads.
  describe('a delegation card follows the register, not its task row', () => {
    const registered = (over: Record<string, unknown> = {}) => ({
      id: 'task_1', agent: 'research', label: 'ตรวจ CRM ไทย 20 เจ้า',
      startedAt: new Date(Date.now() - 92_000).toISOString(),
      toolCalls: 27, state: 'running', collected: false, ...over,
    })
    // The row exactly as the engine leaves it: closed, eight seconds, a tick.
    const spawnedRow = {
      label: 'task ตรวจ CRM ไทย 20 เจ้า', ref: 'call_1', task: 'task_1',
      agent: 'research', agentKind: 'agent', state: 'done', secs: 8, startedAt: Date.now() - 92_000,
    }

    it('keeps the clock running while the worker is still on its tools', () => {
      cockpit.backgroundTasks = [registered()] as any
      const { container } = render(Chat, {
        ...baseProps,
        awaitingReply: true,
        toolSteps: [spawnedRow, { label: 'web_fetch crm.co.th', parent: 'call_1', task: 'task_1', state: 'done', startedAt: 0, secs: 2 }] as any,
        messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
      })

      const card = container.querySelector('.bgw-card')
      expect(card?.className).toContain('run')
      expect(card?.querySelector('.bgw-badge.run')).toBeTruthy()
      // Counted off the delegation's own start, not frozen at the spawn's 8s.
      expect(card?.querySelector('.bgw-meta')?.textContent?.trim()).toBe('1m 32s')
      // And it stays where the user can watch it, instead of collapsing behind
      // the "used N agents" toggle the moment it starts.
      expect(card?.querySelector('.bgw-steps')).toBeTruthy()
    })

    // A finished delegation collapses behind its count, so both readings below
    // are taken after opening it — which is also the only place the number is
    // ever read a second time.
    const openFinished = async (container: Element) => {
      const toggle = [...container.querySelectorAll('.meta-row .reasoning-toggle')]
        .find((b) => b.textContent?.includes('Agents'))
      await fireEvent.click(toggle!)
      return container.querySelector('.tool-steps .bgw-card')
    }

    it('shows the delegation’s real total once the register says it finished', async () => {
      cockpit.backgroundTasks = [registered({ state: 'done', elapsedMs: 214_000, collected: true })] as any
      const { container } = render(Chat, {
        ...baseProps,
        awaitingReply: true,
        toolSteps: [spawnedRow] as any,
        messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
      })
      const card = await openFinished(container)
      expect(card?.querySelector('.bgw-meta')?.textContent?.trim()).toBe('3m 34s')
      expect(card?.querySelector('.bgw-badge.run')).toBeNull()
    })

    // A turn read back from the database has no register entry — the row is all
    // there is, and it still has to draw.
    it('falls back to the row when the register has never heard of the task', async () => {
      cockpit.backgroundTasks = []
      const { container } = render(Chat, {
        ...baseProps,
        awaitingReply: true,
        toolSteps: [spawnedRow] as any,
        messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
      })
      const card = await openFinished(container)
      expect(card?.querySelector('.bgw-meta')?.textContent?.trim()).toBe('8s')
    })

    // The id is only ever on the events from *inside* the delegate: the `task`
    // call completes before the register has a handle to give.
    it('takes the delegation id off the first step its worker runs', () => {
      cockpit.toolSteps = []
      applyToolEvent({ action: 'call', ref: 'call_1', name: 'task', agent: 'research' })
      expect(cockpit.toolSteps[0].task).toBeUndefined()
      applyToolEvent({ action: 'call', ref: 'r1', parent: 'call_1', task: 'task_1', name: 'web_fetch', subject: 'crm.co.th' })
      expect(cockpit.toolSteps[0].task).toBe('task_1')
    })

    // One delegation, one card. Reported as "แล้วจะแสดงซ้อนกันทำไมอ่ะ" with the
    // worker drawn once in the previous turn's message and once in the tray —
    // the case a filter over the live list alone cannot catch, because that list
    // was emptied when the collecting turn began.
    it('drops a tray row for a delegation the transcript is already drawing', async () => {
      cockpit.backgroundTasks = [registered()] as any
      const { container } = render(Chat, {
        ...baseProps,
        awaitingReply: true,
        toolSteps: [{ label: 'task_result task_1', state: 'run', startedAt: Date.now() }] as any,
        messages: [{
          role: 'agent', text: 'ส่งงานให้ซับเอเจนแล้ว', time: '10:54',
          steps: [spawnedRow],
        }] as any,
      })
      expect(container.querySelectorAll('.bgw .bgw-card').length).toBe(0)
      // Still reachable where the work was started, which is the whole reason
      // the tray row is redundant.
      const card = await openFinished(container)
      expect(card?.querySelector('.bgw-badge.run')).toBeTruthy()
    })

    // Unless it is stuck: the tray row is the only one with somewhere to type.
    it('keeps the tray row when the delegate is parked on a question', async () => {
      cockpit.backgroundTasks = [registered({ state: 'waiting', question: 'เอาไฟล์ไหน' })] as any
      const { container } = render(Chat, {
        ...baseProps,
        awaitingReply: false,
        messages: [{ role: 'agent', text: 'ส่งงานแล้ว', time: '10:54', steps: [spawnedRow] }] as any,
      })
      expect(container.querySelector('.bgw .bgw-answer')).toBeTruthy()
    })

    // The card can only ask the register if somebody fetched it. The poll used
    // to start on a *background* event or at the end of the turn, so a delegate
    // working inside its turn had nothing to read and the clock stayed frozen
    // for exactly as long as the user was watching it.
    it('starts polling the register as soon as a delegate works inside the turn', async () => {
      cockpit.toolSteps = []
      resetBackgroundWork()
      vi.mocked(BackgroundTasks).mockResolvedValue([
        { id: 'task_1', agent: 'explore', label: 'ตรวจไฟล์ทดสอบ', startedAt: new Date().toISOString(), toolCalls: 9, state: 'running', collected: false },
      ] as any)
      applyToolEvent({ action: 'call', ref: 'call_1', name: 'task', agent: 'explore' })
      applyToolEvent({ action: 'call', ref: 'r1', parent: 'call_1', task: 'task_1', name: 'list' })
      await vi.waitFor(() => expect(cockpit.backgroundTasks).toHaveLength(1))
    })
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
    expect(container.querySelector('.bgw-card')).toBeNull()
    expect(container.textContent).toContain('read a.txt')
    expect(container.querySelectorAll('.tool-steps .tool-step').length).toBe(1)

    subsBtn.click()
    await tick()
    // The tools panel closed with it — one slot, same as thinking.
    expect(container.querySelectorAll('.tool-steps > .tool-step').length).toBe(0)
    const block = container.querySelector('.bgw-card')
    expect(block).toBeTruthy()
    expect(block?.querySelector('.bgw-longbrief')?.textContent).toContain('go hunt')
    // Finished, so its steps are folded away — see the fold's own test below.
    expect(block?.textContent).not.toContain('grep needle')
  })

  // Four finished delegations with their steps all open turned the transcript
  // into a wall ("มันติดกันจนดูยังไงไม่รู้"). A finished delegation's steps are
  // a record nobody asked to re-read; a running one's are the evidence it is
  // alive. Same rows, opposite defaults.
  describe('the step fold on a delegation', () => {
    const delegation = (state: string) => [
      { label: 'task hunt', ref: 't1', agent: 'explore', brief: 'go hunt', state, startedAt: Date.now() },
      { label: 'grep needle', parent: 't1', state, startedAt: Date.now() },
      { label: 'read hay.txt', parent: 't1', state, startedAt: Date.now() },
    ]

    it('is open while the delegate is still working', () => {
      const { container } = render(Chat, {
        ...baseProps, awaitingReply: true,
        toolSteps: delegation('run') as any,
        messages: [{ role: 'agent', text: 'done', time: '10:54' }] as any,
      })
      expect(container.querySelectorAll('.bgw-steps .tool-step').length).toBe(2)
    })

    it('is folded once it has finished, and says how many are behind it', async () => {
      const { container } = render(Chat, {
        ...baseProps,
        messages: [{ role: 'agent', text: 'done', time: '10:54', steps: delegation('done') }] as any,
      })
      const subsBtn = [...container.querySelectorAll('.meta-row .reasoning-toggle')].at(-1) as HTMLElement
      subsBtn.click()
      await tick()

      const fold = container.querySelector('.bgw-toggle') as HTMLElement
      expect(fold.textContent).toContain('Used 2 tools')
      expect(container.querySelectorAll('.bgw-steps .tool-step').length).toBe(0)

      fold.click()
      await tick()
      expect(container.querySelectorAll('.bgw-steps .tool-step').length).toBe(2)
    })
  })

  // เอเจน and ซับเอเจน are different piles (COMPANY.md §4), and the chip
  // used to lump them: a turn where the doc agent made the file read "ซับเอเจน
  // 1 ตัว". The kind rides on the step (stamped by the engine from which home
  // the profile lives in), and each pile gets its own toggle and panel. A step
  // with no kind — every turn persisted before the split — stays a helper.
  it('counts an agent-kind delegation apart from the sub-agents', async () => {
    const { container } = render(Chat, {
      ...baseProps,
      messages: [{
        role: 'agent', text: 'done', time: '10:54',
        steps: [
          { label: 'read a.txt', state: 'done', startedAt: 0 },
          { label: 'task make the report', ref: 't1', agent: 'doc', agentKind: 'agent', brief: 'write the meeting summary', state: 'done', startedAt: 0 },
          { label: 'task hunt callers', ref: 't2', agent: 'explore', agentKind: 'helper', brief: 'find the callers', state: 'done', startedAt: 0 },
        ],
      }] as any,
    })
    const toggles = [...container.querySelectorAll('.meta-row .reasoning-toggle')] as HTMLElement[]
    const labels = toggles.map((b) => b.textContent ?? '')
    expect(labels.length).toBe(3)
    expect(labels[0]).toContain('Used 1 tools')
    expect(labels[1]).toContain('Agents: 1')
    expect(labels[2]).toContain('Sub-agents: 1')

    // Each toggle opens only its own pile.
    toggles[1].click()
    await tick()
    let blocks = [...container.querySelectorAll('.bgw-card')]
    expect(blocks.length).toBe(1)
    expect(blocks[0].querySelector('.bgw-agent')?.textContent).toContain('doc')

    toggles[2].click()
    await tick()
    blocks = [...container.querySelectorAll('.bgw-card')]
    expect(blocks.length).toBe(1)
    expect(blocks[0].querySelector('.bgw-agent')?.textContent).toContain('explore')
  })
})
