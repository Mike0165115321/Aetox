// What Aetox changed, under the row that changed it (โค้ด desk only).
//
// Two claims worth pinning. The first is the numbering: a diff whose line
// numbers are its own rather than the file's sends the reader to the wrong
// place, which is worse than showing nothing. The second is the desk gate — a
// diff was here once, in the Review panel removed on 2026-08-03, and the reason
// it went is still true in every room but this one.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import CodeDiff from '../lib/CodeDiff.svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit, applyToolEvent } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics } from './mocks/wailsApp'

const DIFF = [
  '@@ -10,5 +10,6 @@',
  ' func main() {',
  '-\tprintln("old")',
  '+\tprintln("new")',
  '+\treturn',
  ' }',
].join('\n')

describe('CodeDiff', () => {
  it('numbers each line the way the file does, not the way the diff does', () => {
    const { container } = render(CodeDiff, { diff: DIFF })
    const rows = [...container.querySelectorAll('.dl')].map((el) => [
      el.querySelector('.ln')?.textContent,
      el.className.includes('add') ? '+' : el.className.includes('del') ? '-' : ' ',
      el.querySelector('.tx')?.textContent,
    ])
    expect(rows).toEqual([
      ['10', ' ', 'func main() {'],
      // The removed line is only findable in the old file, so it wears the old
      // number; everything after it is numbered in the new one.
      ['11', '-', '\tprintln("old")'],
      ['11', '+', '\tprintln("new")'],
      ['12', '+', '\treturn'],
      ['13', ' ', '}'],
    ])
  })

  it('names the file when one call changed several', () => {
    const diff = `+++ a.go\n@@ -1,1 +1,1 @@\n-x\n+y\n+++ b.go\n@@ -1,1 +1,1 @@\n-p\n+q`
    const { container } = render(CodeDiff, { diff })
    expect([...container.querySelectorAll('.fname')].map((e) => e.textContent)).toEqual(['a.go', 'b.go'])
  })

  // "+++ " is also how an added line beginning "++ " arrives. What separates
  // the two is whether a hunk follows.
  it('does not mistake an added line for a file header', () => {
    const diff = '@@ -1,1 +1,2 @@\n x\n+++ not a filename'
    const { container } = render(CodeDiff, { diff })
    expect(container.querySelector('.fname')).toBeNull()
    expect(container.querySelector('.dl.add .tx')?.textContent).toBe('++ not a filename')
  })

  // A cut that will not say it is a cut reads as the whole change.
  it('says how much it is not showing', () => {
    setLocale('en')
    const { container } = render(CodeDiff, { diff: '@@ -1,1 +1,1 @@\n-a\n+b\n~57' })
    expect(container.querySelector('.dmore')?.textContent).toContain('57')
  })
})

describe('the diff fold-out in the chat timeline', () => {
  beforeEach(() => {
    setLocale('en')
    cockpit.chat = []
    cockpit.todos = []
    cockpit.ask = null
    cockpit.toolSteps = []
    cockpit.backgroundTasks = []
    cockpit.backgroundSteps = []
    vi.mocked(GuideTopics).mockResolvedValue([] as any)
  })

  const props = {
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

  // A reply the user has scrolled back to, which is the case that matters most:
  // the diff is written down with the turn, so a row expanded tomorrow still
  // shows what that call did. A finished turn keeps its tool list behind the
  // "Used N tools" toggle, so every one of these opens that first.
  const replyWith = (over: Record<string, unknown>) => [{
    role: 'agent', text: 'done', time: '10:54',
    steps: [{ label: 'edit main.go', ref: 'c1', state: 'done', startedAt: 0, secs: 1, added: 2, removed: 1, ...over }],
  }] as any

  it('carries the diff from the result event onto the row', () => {
    applyToolEvent({ action: 'call', ref: 'c1', name: 'edit', subject: 'main.go' } as any)
    applyToolEvent({
      action: 'result', ref: 'c1', name: 'edit', subject: 'main.go',
      ok: true, added: 2, removed: 1, diff: DIFF,
    } as any)
    expect(cockpit.toolSteps[0].diff).toBe(DIFF)
  })

  const openTheTimeline = async (container: HTMLElement) => {
    await fireEvent.click(container.querySelector('.meta-row .reasoning-toggle') as HTMLElement)
  }

  it('folds shut, and opens on the โค้ด desk when the row is clicked', async () => {
    cockpit.desk = 'coding'
    const { container } = render(Chat, { ...props, messages: replyWith({ diff: DIFF }) })
    await tick()
    await openTheTimeline(container)

    expect(container.querySelector('.tool-diff')).toBeNull()
    const row = container.querySelector('.tool-step.foldable') as HTMLElement
    expect(row).not.toBeNull()
    expect(row.getAttribute('aria-expanded')).toBe('false')

    await fireEvent.click(row)
    expect(container.querySelector('.tool-diff')).not.toBeNull()
    expect(container.querySelector('.dl.del .tx')?.textContent).toBe('	println("old")')

    // And shuts again, on the same row.
    await fireEvent.click(container.querySelector('.tool-step.foldable') as HTMLElement)
    expect(container.querySelector('.tool-diff')).toBeNull()
  })

  // The Review panel's removal reasoning still holds everywhere else: this is a
  // product whose promise is finished work, and someone watching the assistant
  // write a letter is not owed the letter's hunks.
  it('leaves every other desk the row it always had', async () => {
    cockpit.desk = 'assistant'
    const { container } = render(Chat, { ...props, messages: replyWith({ diff: DIFF }) })
    await tick()
    await openTheTimeline(container)

    expect(container.querySelector('.tool-step')).not.toBeNull()
    expect(container.querySelector('.tool-step.foldable')).toBeNull()
    expect(container.querySelector('.tool-diff')).toBeNull()
  })

  // A row that read a file has nothing to unfold, and must not look like it
  // does: a control that sometimes responds teaches the user to stop trying it.
  it('leaves rows that wrote no file alone', async () => {
    cockpit.desk = 'coding'
    const { container } = render(Chat, {
      ...props,
      messages: [{
        role: 'agent', text: 'done', time: '10:54',
        steps: [{ label: 'read main.go', ref: 'r1', state: 'done', startedAt: 0, secs: 1 }],
      }] as any,
    })
    await tick()
    await openTheTimeline(container)

    expect(container.querySelector('.tool-step')).not.toBeNull()
    expect(container.querySelector('.tool-step.foldable')).toBeNull()
  })
})
