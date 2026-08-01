import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderMarkdown } from '../lib/markdown'
import {
  cockpit, applyTaskChips, startTaskChip, dismissTaskChip,
} from '../lib/stores/cockpit.svelte'
import { DismissTaskChip, NewSession, SendMessage } from './mocks/wailsApp'

beforeEach(() => {
  cockpit.taskChips = []
  cockpit.chat = []
  vi.mocked(DismissTaskChip).mockClear()
  vi.mocked(NewSession).mockClear()
  vi.mocked(SendMessage).mockClear()
})

describe('run button on fenced blocks', () => {
  it('shell-tagged blocks get a Run button, prose languages do not', () => {
    const shell = renderMarkdown('```bash\necho hi\n```')
    expect(shell).toContain('code-run')

    const ps = renderMarkdown('```powershell\nGet-ChildItem\n```')
    expect(ps).toContain('code-run')

    // Source files and printed output are not commands — running them would
    // be a wrong guess with a real side effect.
    for (const lang of ['python', 'go', 'json', 'console', 'text', '']) {
      const html = renderMarkdown('```' + lang + '\nnot a command\n```')
      expect(html, `lang=${lang || '(none)'}`).not.toContain('code-run')
    }
  })

  it('every runnable block still has its copy button', () => {
    const html = renderMarkdown('```sh\nls\n```')
    expect(html).toContain('code-copy')
  })
})

describe('task chip store flow', () => {
  const chip = {
    id: 'chip-1',
    title: 'Fix stale badge',
    tldr: 'The README badge is dead.',
    prompt: 'In README.md the CI badge points at a renamed workflow; update the URL.',
    createdAt: '2026-08-02T00:00:00Z',
  }

  it('applyTaskChips replaces the list wholesale and tolerates junk', () => {
    applyTaskChips([chip])
    expect(cockpit.taskChips).toHaveLength(1)
    applyTaskChips(undefined as never)
    expect(cockpit.taskChips).toEqual([])
  })

  it('starting a chip consumes it, opens a fresh session, and sends the prompt as-is', async () => {
    applyTaskChips([chip])
    await startTaskChip(chip)
    expect(vi.mocked(DismissTaskChip).mock.calls[0][0]).toBe('chip-1')
    expect(vi.mocked(NewSession)).toHaveBeenCalled()
    // The prompt travels verbatim — it was written to stand alone, and any
    // "helpful" rewriting here would break that contract invisibly.
    expect(vi.mocked(SendMessage).mock.calls[0][0]).toBe(chip.prompt)
  })

  it('dismissing only dismisses', async () => {
    await dismissTaskChip('chip-9')
    expect(vi.mocked(DismissTaskChip).mock.calls[0][0]).toBe('chip-9')
    expect(vi.mocked(NewSession)).not.toHaveBeenCalled()
    expect(vi.mocked(SendMessage)).not.toHaveBeenCalled()
  })
})
