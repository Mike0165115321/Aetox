// A file pane, and the agent editing the file it is showing.
//
// Owner, 24 ส.ค., with a document open beside a working turn: *"ผมทำงานอยู่ มัน
// ปรับเนื้อหาในเอกสารแล้วผมยังเห็นอันเก่าอยู่"*. A tab read its file once, when
// it was opened, and nothing ever told it the file had moved on — so the panel
// whose whole job is showing what the agent produced kept showing what it had
// produced before.
//
// Two halves are guarded here: the store putting fresh bytes on the tab, and
// its refusal to rebuild the pane while it does — a rebuilt editor loses the
// caret, and this path exists precisely for somebody who is still typing.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { workbench, openFileTab, filesChangedOnDisk } from '../lib/stores/workbench.svelte'
import { ReadFile } from './mocks/wailsApp'

const tab = (path: string) => workbench.tabs.find((t) => t.path === path)!

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
  vi.mocked(ReadFile).mockResolvedValue('the first version' as any)
})

describe('a file changing under an open pane', () => {
  it('puts the new bytes on the tab', async () => {
    await openFileTab('notes.md')
    expect(tab('notes.md').content).toBe('the first version')

    vi.mocked(ReadFile).mockResolvedValue('what the agent just wrote' as any)
    await filesChangedOnDisk(['notes.md'])

    expect(tab('notes.md').content).toBe('what the agent just wrote')
  })

  // rev is what rebuilds the pane, and rebuilding is the opposite of what this
  // path wants: FileEditor patches the change into its model in place so the
  // caret, the scroll and the undo stack all survive.
  it('does not rebuild the pane for a text file it re-read', async () => {
    await openFileTab('notes.md')
    const before = tab('notes.md').rev

    await filesChangedOnDisk(['notes.md'])

    expect(tab('notes.md').rev).toBe(before)
  })

  // Unless the file stopped being something the editor can draw. Then the pane
  // on screen is one for a file that no longer exists, and it has to go.
  it('does rebuild when the file stops being readable', async () => {
    await openFileTab('notes.md')
    const before = tab('notes.md').rev

    vi.mocked(ReadFile).mockRejectedValue(new Error('binary file cannot be previewed'))
    await filesChangedOnDisk(['notes.md'])

    expect(tab('notes.md').rev).toBeGreaterThan(before!)
    expect(tab('notes.md').unreadable).toContain('binary file')
  })

  // Windows is the reference platform: these are one file there, and a
  // comparison that says otherwise leaves the pane stale for the reason the
  // user can least guess at.
  it('matches a path spelled with other slashes and other case', async () => {
    await openFileTab('docs/Notes.md')

    vi.mocked(ReadFile).mockResolvedValue('fresh' as any)
    await filesChangedOnDisk(['docs\\notes.md'])

    expect(tab('docs/Notes.md').content).toBe('fresh')
  })

  it('leaves the files nobody touched alone', async () => {
    await openFileTab('a.md')
    await openFileTab('b.md')

    vi.mocked(ReadFile).mockResolvedValue('only a moved' as any)
    await filesChangedOnDisk(['a.md'])

    expect(tab('a.md').content).toBe('only a moved')
    expect(tab('b.md').content).toBe('the first version')
  })

  it('is a no-op when nothing is open at that path', async () => {
    await openFileTab('a.md')

    vi.mocked(ReadFile).mockClear()
    await filesChangedOnDisk(['somewhere/else.md'])

    expect(ReadFile).not.toHaveBeenCalled()
  })
})
