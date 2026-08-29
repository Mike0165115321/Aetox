// Opening a pull request asks "which files". Every patch drawn at the same
// time answers a question nobody asked yet, and a request of forty files
// arrives as thousands of diff lines that bury the list the click was for —
// "เปิดเข้าไปแล้วมันตู้มเลย". So the file rows come collapsed, the way the
// Git pane's rows already do, and a patch is one more click.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import PRPane from '../lib/workbench/PRPane.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { PullRequests, PullRequestFiles, PullRequestChecks } from './mocks/wailsApp'

const pr = {
  number: 7, title: 'Collapse the diffs', url: 'https://example.test/pr/7',
  state: 'open', draft: false, author: 'mike', headRef: 'work', baseRef: 'main',
  headSHA: 'abc123', mergeable: true, additions: 40, deletions: 3, changedFiles: 2,
}

const files = [
  { path: 'a.go', status: 'modified', additions: 20, deletions: 1, patch: '@@ -1 +1 @@\n-old line a\n+new line a' },
  { path: 'b.go', status: 'modified', additions: 20, deletions: 2, patch: '@@ -1 +1 @@\n-old line b\n+new line b' },
]

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  vi.mocked(PullRequests).mockResolvedValue({ repo: 'o/r', reason: '', connected: true, items: [pr] } as any)
  vi.mocked(PullRequestFiles).mockResolvedValue(files as any)
  vi.mocked(PullRequestChecks).mockResolvedValue([] as any)
})

describe('the pull request room', () => {
  it('lists the files without drawing a single patch', async () => {
    render(PRPane)
    await waitFor(() => expect(screen.getByText(/Collapse the diffs/)).toBeTruthy())
    await fireEvent.click(screen.getByText(/Collapse the diffs/))

    await waitFor(() => expect(screen.getByText('a.go')).toBeTruthy())
    expect(screen.getByText('b.go')).toBeTruthy()
    // The list is the answer; the patches are not on screen yet.
    expect(screen.queryByText(/new line a/)).toBeNull()
    expect(screen.queryByText(/new line b/)).toBeNull()
  })

  it('draws only the file you opened', async () => {
    render(PRPane)
    await waitFor(() => expect(screen.getByText(/Collapse the diffs/)).toBeTruthy())
    await fireEvent.click(screen.getByText(/Collapse the diffs/))
    await waitFor(() => expect(screen.getByText('a.go')).toBeTruthy())

    await fireEvent.click(screen.getByText('a.go'))
    await waitFor(() => expect(screen.getByText(/new line a/)).toBeTruthy())
    // Its neighbour stays shut: one click, one patch.
    expect(screen.queryByText(/new line b/)).toBeNull()

    await fireEvent.click(screen.getByText('a.go'))
    await waitFor(() => expect(screen.queryByText(/new line a/)).toBeNull())
  })
})
