// `desk close` — the desk's fourth door, and the only one that takes something away
//
// The desk could be filled and never emptied: an agent that opened five files in
// one turn buried the one the user was reading, and nothing but the user's own
// click could undo that. What makes the door safe is a single rule, and it is
// the rule this file pins: the agent may only take back what the agent put
// there. §81 says what the user is doing on their own machine is not the
// agent's to read; closing a tab they opened themselves is the same rule with a
// heavier hand.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { workbench, openFileTab, closeAgentFileTab } from '../lib/stores/workbench.svelte'
import { ReadFile } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
  vi.mocked(ReadFile).mockResolvedValue('เนื้อไฟล์' as any)
})

describe('closing a file on the desk', () => {
  it('takes back a file the agent opened', async () => {
    await openFileTab('output/s1/deck.html', 'deck.html', true)
    expect(workbench.tabs).toHaveLength(1)
    expect(workbench.tabs[0].mine).toBe(true)

    closeAgentFileTab('output/s1/deck.html')
    expect(workbench.tabs).toHaveLength(0)
  })

  it('leaves a file the user opened alone', async () => {
    await openFileTab('notes.md')

    closeAgentFileTab('notes.md')
    expect(workbench.tabs).toHaveLength(1)
  })

  // Who put a tab there is a fact about the first time. If re-opening could
  // claim it, the agent would only have to name a file the user is reading to
  // win the right to close it — which is the rule above, undone by a detail.
  it('does not claim a user’s tab by re-opening it', async () => {
    await openFileTab('notes.md')
    await openFileTab('notes.md', 'notes.md', true)

    expect(workbench.tabs).toHaveLength(1)
    expect(workbench.tabs[0].mine).toBeFalsy()
    closeAgentFileTab('notes.md')
    expect(workbench.tabs).toHaveLength(1)
  })

  // The Go side has already refused a path it cannot see, off a mirror that is
  // one report behind by construction. The array here is the copy that is never
  // stale, so it asks again.
  it('does nothing for a path that is not on the desk', async () => {
    await openFileTab('output/s1/deck.html', 'deck.html', true)

    closeAgentFileTab('output/s1/other.html')
    expect(workbench.tabs).toHaveLength(1)
  })
})
