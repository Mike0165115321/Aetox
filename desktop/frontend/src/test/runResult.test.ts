// The panel the Run button leaves behind.
//
// Before 16 ส.ค. it was a bare <pre> on the same --surface-code as the code
// above it, separated by one hairline, so a result read as more program — and
// it threw away everything the engine already knew about the run. The design
// that replaced it is one row that is the status bar when open and the whole
// receipt when folded, a rail down the left edge, and a fold that is for long
// output only.
//
// What is pinned here is the behaviour that had to be decided rather than the
// look: when it folds, what the summary is, and that a failure can be handed
// back to the model without the user retyping it.

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit, resetBackgroundWork } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { RunChatCommand, RunChatScript, SendMessage, BackgroundTasks } from './mocks/wailsApp'
import { setRunnableLanguages } from '../lib/markdown'

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

const result = (over: Partial<Record<string, unknown>> = {}) => ({
  output: 'done', success: true, durationMs: 900, lines: 1, truncated: false, ...over,
})

const lines = (n: number) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join('\n')

beforeEach(() => {
  setLocale('en')
  // The Run button is drawn where the engine says this machine can run one, so
  // the machine is stated rather than inherited from whoever runs the suite.
  setRunnableLanguages({ bash: 'shell', python: 'script' })
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  cockpit.taskChips = []
  resetBackgroundWork()
  vi.mocked(BackgroundTasks).mockResolvedValue([])
  vi.mocked(RunChatCommand).mockReset()
  vi.mocked(RunChatScript).mockReset()
  vi.mocked(SendMessage).mockReset()
})

// Renders one agent reply carrying a fenced block, clicks its Run button, and
// hands back the panel that appeared.
async function runBlock(fence: string, res: Record<string, unknown>) {
  vi.mocked(RunChatCommand).mockResolvedValue(res as any)
  vi.mocked(RunChatScript).mockResolvedValue(res as any)
  const { container } = render(Chat, {
    ...baseProps,
    messages: [{ role: 'agent', text: fence, time: '10:54', id: 1, steps: [] }],
  } as any)
  await tick()
  const run = container.querySelector('.code-run')
  expect(run, 'the block was rendered without a Run button').not.toBeNull()
  await fireEvent.click(run!)
  await tick()
  await tick()
  return container
}

const SHELL = '```bash\necho hi\n```'

describe('the result panel', () => {
  it('is its own surface with a verdict rail, not more code', async () => {
    const c = await runBlock(SHELL, result())
    const panel = c.querySelector('.run-res')

    expect(panel).not.toBeNull()
    expect(panel!.classList.contains('ok')).toBe(true)
    expect(c.querySelector('.run-res-body')!.textContent).toBe('done')
  })

  it('says how long it took, which the engine already knew and nothing used', async () => {
    const c = await runBlock(SHELL, result({ durationMs: 1240, lines: 3, output: lines(3) }))

    const meta = c.querySelector('.run-res-meta')!.textContent ?? ''
    expect(meta).toContain('1.2')
    expect(meta).toContain('3')
  })

  // A panel that scrolls to a bottom which is not the bottom is the one way a
  // result can lie about what happened.
  it('says so when the output was cut short', async () => {
    const cut = await runBlock(SHELL, result({ truncated: true, lines: 900, output: lines(900) }))
    expect(cut.querySelector('.run-res-more')).not.toBeNull()

    const whole = await runBlock(SHELL, result())
    expect(whole.querySelector('.run-res-more')).toBeNull()
  })
})

describe('when it folds', () => {
  it('leaves short output open, with no arrow to press', async () => {
    const c = await runBlock(SHELL, result({ lines: 4, output: lines(4) }))

    expect(c.querySelector('.run-res')!.classList.contains('folded')).toBe(false)
    // A control that reveals nothing teaches the user that it does nothing.
    expect(c.querySelector('.run-res-chev')!.classList.contains('none')).toBe(true)
    expect(c.querySelector('.run-res-head')!.getAttribute('role')).toBeNull()
  })

  it('folds long output that succeeded, and shows the program\'s last line', async () => {
    const c = await runBlock(SHELL, result({ lines: 40, output: `${lines(39)}\nTEST PASSED` }))
    const panel = c.querySelector('.run-res')!

    expect(panel.classList.contains('folded')).toBe(true)
    expect(c.querySelector('.run-res-summary')!.textContent).toBe('TEST PASSED')
  })

  // The moment the output is the thing to read is the moment folding it hides
  // the answer.
  it('never folds a failure, however long it is', async () => {
    const c = await runBlock(SHELL, result({ success: false, lines: 400, output: lines(400) }))
    const panel = c.querySelector('.run-res')!

    expect(panel.classList.contains('failed')).toBe(true)
    expect(panel.classList.contains('folded')).toBe(false)
  })

  it('opens and closes when the row is pressed, not only the arrow', async () => {
    const c = await runBlock(SHELL, result({ lines: 40, output: lines(40) }))
    const panel = c.querySelector('.run-res')!

    await fireEvent.click(c.querySelector('.run-res-head')!)
    expect(panel.classList.contains('folded')).toBe(false)

    await fireEvent.click(c.querySelector('.run-res-head')!)
    expect(panel.classList.contains('folded')).toBe(true)
  })
})

describe('handing a failure back to the model', () => {
  it('offers ให้แก้ให้ only when the run failed', async () => {
    const bad = await runBlock(SHELL, result({ success: false, output: 'AssertionError' }))
    expect(bad.querySelector('.run-res-fix')).not.toBeNull()

    const good = await runBlock(SHELL, result())
    expect(good.querySelector('.run-res-fix')).toBeNull()
  })

  // Both halves go, because the error alone does not say what was run and the
  // code alone is what the user was already looking at.
  it('sends the code and the error together', async () => {
    const c = await runBlock(
      '```python\nassert 1 == 2\n```',
      result({ success: false, output: 'AssertionError: nope' }),
    )

    await fireEvent.click(c.querySelector('.run-res-fix')!)
    await tick()

    const sent = String(vi.mocked(SendMessage).mock.calls[0]?.[0] ?? '')
    expect(sent).toContain('assert 1 == 2')
    expect(sent).toContain('AssertionError: nope')
  })
})

describe('the controls on the row', () => {
  it('copies the output rather than the code', async () => {
    const write = vi.fn(async () => {})
    Object.assign(navigator, { clipboard: { writeText: write } })

    const c = await runBlock(SHELL, result({ output: 'the answer' }))
    await fireEvent.click(c.querySelector('.run-res-copy')!)

    expect(write).toHaveBeenCalledWith('the answer')
  })

  it('closes the panel and leaves the block behind', async () => {
    const c = await runBlock(SHELL, result())

    await fireEvent.click(c.querySelector('.run-res-close')!)

    expect(c.querySelector('.run-res')).toBeNull()
    expect(c.querySelector('.codeblock')).not.toBeNull()
  })

  it('replaces the previous result instead of stacking a second one', async () => {
    const c = await runBlock(SHELL, result({ output: 'first' }))

    vi.mocked(RunChatCommand).mockResolvedValue(result({ output: 'second' }) as any)
    await fireEvent.click(c.querySelector('.code-run')!)
    await tick()
    await tick()

    expect(c.querySelectorAll('.run-res')).toHaveLength(1)
    expect(c.querySelector('.run-res-body')!.textContent).toBe('second')
  })
})
