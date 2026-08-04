// The agent produces files this app cannot render. Clicking one used to open an
// editor whose entire contents were the words "binary file cannot be previewed":
// the app promised finished work and then declined to show it.
//
// Decks and documents no longer even ask ReadFile: routed through the text path
// they showed whichever gate fired first — "file too large to preview" for a
// 1.5MB pptx — for files that were never previewable at any size. They go
// straight to the open-externally card with the honest reason. Other binaries
// (a .zip the agent fetched, say) still take the ReadFile route and surface its
// refusal, so both roads are pinned here.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { workbench, openFileTab } from '../lib/stores/workbench.svelte'
import { ReadFile } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
})

describe('a file the editor cannot render', () => {
  it('sends a deck to the card with the real reason, without asking ReadFile', async () => {
    await openFileTab('out/นำเสนอ.pptx')

    const tab = workbench.tabs[0]
    expect(tab.kind).toBe('file')
    expect(tab.unreadable).toBeTruthy()
    expect(tab.unreadable).not.toContain('too large')
    // The gate whose error used to masquerade as the reason must not even run.
    expect(ReadFile).not.toHaveBeenCalled()
    // The distinction the pane switches on: no content means no editor.
    expect(tab.content).toBeUndefined()
  })

  it('marks other binaries unreadable rather than becoming an editor full of the error', async () => {
    ReadFile.mockRejectedValueOnce(new Error('binary file cannot be previewed'))

    await openFileTab('out/archive.zip')

    const tab = workbench.tabs[0]
    expect(tab.unreadable).toContain('binary file cannot be previewed')
    expect(tab.content).toBeUndefined()
  })

  it('leaves a readable file exactly as it was', async () => {
    ReadFile.mockResolvedValueOnce('# notes\n')

    await openFileTab('notes.md')

    const tab = workbench.tabs[0]
    expect(tab.content).toBe('# notes\n')
    expect(tab.unreadable).toBeUndefined()
  })
})
