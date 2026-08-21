// The working tree as a room (DECISIONS §161.4).
//
// Three claims worth pinning: the room says which desk it belongs to, it does
// no work until a row is opened, and the diff it draws comes from the engine
// rather than from anything computed here.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/svelte'
import GitPane from '../lib/workbench/GitPane.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GitWorkingTree, GitFileDiff } from './mocks/wailsApp'

const CHANGED = [
  { path: 'internal/skill/hunk.go', status: 'U', added: 284, removed: 0 },
  { path: 'ARCHITECTURE.md', status: 'M', added: 5, removed: 1 },
]

const DIFF = '+++ ARCHITECTURE.md\n@@ -565,1 +565,2 @@\n old row\n+| §161 | a new row |'

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  cockpit.project.branch = 'main'
  vi.mocked(GitWorkingTree).mockResolvedValue(CHANGED as any)
  vi.mocked(GitFileDiff).mockResolvedValue(DIFF as any)
})

describe('GitPane', () => {
  it('names the branch, the totals and every changed file', async () => {
    const { container, getByText } = render(GitPane)
    await waitFor(() => expect(container.querySelectorAll('.gp-row').length).toBe(2))

    expect(container.querySelector('.gp-where b')?.textContent).toBe('main')
    expect(getByText('hunk.go')).toBeTruthy()
    expect(getByText('internal/skill')).toBeTruthy()
    // The header totals the rows rather than asking for a second number that
    // could disagree with them.
    expect(container.querySelector('.gp-totals .add')?.textContent).toBe('+289')
    expect(container.querySelector('.gp-totals .del')?.textContent).toBe('-1')
  })

  // Said out loud, because a room that exists on one desk and nowhere else owes
  // the reader that sentence instead of leaving them to hunt.
  it('says it is on the โค้ด desk only', async () => {
    const { container } = render(GitPane)
    await waitFor(() => expect(container.querySelector('.gp-note')).not.toBeNull())
    expect(container.querySelector('.gp-note')?.textContent).toContain('Code desk only')
  })

  // A working tree of forty files is ordinary. Forty diffs fetched for a list
  // nobody has opened is work done on the chance it is wanted.
  it('fetches nothing until a row is opened, then fetches once', async () => {
    const { container } = render(GitPane)
    await waitFor(() => expect(container.querySelectorAll('.gp-row').length).toBe(2))
    expect(vi.mocked(GitFileDiff)).not.toHaveBeenCalled()

    await fireEvent.click(container.querySelectorAll('.gp-row')[1] as HTMLElement)
    await waitFor(() => expect(container.querySelector('.dl.add')).not.toBeNull())
    expect(vi.mocked(GitFileDiff)).toHaveBeenCalledWith('ARCHITECTURE.md')
    expect(container.querySelector('.dl.add .tx')?.textContent).toContain('§161')

    // Shut and reopened, the answer is the one already in hand.
    await fireEvent.click(container.querySelectorAll('.gp-row')[1] as HTMLElement)
    await fireEvent.click(container.querySelectorAll('.gp-row')[1] as HTMLElement)
    expect(vi.mocked(GitFileDiff)).toHaveBeenCalledTimes(1)
  })

  it('says the tree is clean rather than drawing an empty list', async () => {
    vi.mocked(GitWorkingTree).mockResolvedValue([] as any)
    const { container } = render(GitPane)
    await waitFor(() => expect(container.querySelector('.gp-empty')).not.toBeNull())
    expect(container.querySelector('.gp-empty')?.textContent).toContain('Nothing changed')
    expect(container.querySelector('.gp-list')).toBeNull()
  })
})
