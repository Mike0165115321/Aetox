import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderMarkdown } from '../lib/markdown'
import {
  cockpit, applyTaskChips, startTaskChip, dismissTaskChip,
} from '../lib/stores/cockpit.svelte'
import { DismissTaskChip, NewSession, SendMessage } from './mocks/wailsApp'
import { setRunnableLanguages } from '../lib/markdown'

// The renderer holds no list of its own any more: it draws a Run button where
// the engine says this machine can run one (App.RunnableLanguages, backed by
// internal/runlang). So these tests state the machine they are testing against
// rather than inheriting whatever is installed on the one running the suite.
beforeEach(() => {
  setRunnableLanguages({
    bash: 'shell', sh: 'shell', shell: 'shell', powershell: 'shell', ps1: 'shell',
    python: 'script', py: 'script',
  })
})

beforeEach(() => {
  cockpit.taskChips = []
  cockpit.chat = []
  vi.mocked(DismissTaskChip).mockClear()
  vi.mocked(NewSession).mockClear()
  vi.mocked(SendMessage).mockClear()
})

describe('run button on fenced blocks', () => {
  it('shell-tagged blocks get a Run button and are handed over as written', () => {
    const shell = renderMarkdown('```bash\necho hi\n```')
    expect(shell).toContain('code-run')
    // No data-script: the text of a shell block IS the command.
    expect(shell).not.toContain('data-script')

    const ps = renderMarkdown('```powershell\nGet-ChildItem\n```')
    expect(ps).toContain('code-run')
  })

  // A model that answers a question with a Python script has produced something
  // to run. The block is a file rather than a command, so it is marked as one
  // and the engine writes it out and runs it through an interpreter
  // (desktop/run_script.go) — the click handler must not have to guess.
  it('source-tagged blocks get a Run button and say which interpreter', () => {
    for (const lang of ['python', 'py', 'Python']) {
      const html = renderMarkdown('```' + lang + '\nprint(1)\n```')
      expect(html, `lang=${lang}`).toContain('code-run')
      expect(html, `lang=${lang}`).toContain(`data-script="${lang.toLowerCase()}"`)
    }
  })

  it('leaves everything that is neither a command nor a runnable file alone', () => {
    // `console` and `text` are transcripts of output, and a `$` in front of a
    // line does not make it a command. JSON is markup with nothing to run.
    for (const lang of ['json', 'console', 'text', '']) {
      const html = renderMarkdown('```' + lang + '\nnot a command\n```')
      expect(html, `lang=${lang || '(none)'}`).not.toContain('code-run')
      expect(html, `lang=${lang || '(none)'}`).not.toContain('data-script')
    }
  })

  // The reason the list moved out of the renderer: a `python` block is runnable
  // on a machine with Python on it and is a picture of a program on one without.
  // A button that cannot do anything must not be drawn, and the safe answer when
  // the engine has not said anything yet is no buttons at all.
  it('draws no Run button on a machine that answered nothing', () => {
    setRunnableLanguages({})

    for (const lang of ['bash', 'python', 'powershell']) {
      expect(renderMarkdown('```' + lang + '\nx\n```'), `lang=${lang}`).not.toContain('code-run')
    }
  })

  it('draws no Run button for a language this machine cannot run', () => {
    setRunnableLanguages({ bash: 'shell' })

    expect(renderMarkdown('```python\nprint(1)\n```')).not.toContain('code-run')
    expect(renderMarkdown('```bash\nls\n```')).toContain('code-run')
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
