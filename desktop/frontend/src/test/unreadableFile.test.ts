// The agent produces files this app cannot render. Clicking one used to open an
// editor whose entire contents were the words "binary file cannot be previewed":
// the app promised finished work and then declined to show it.
//
// A .pptx rather than a .xlsx, deliberately. Workbooks took their own route once
// the grid preview landed (ARCHITECTURE.md §79) — a deck would need a rendering
// engine and a document a layout engine, so both still come here, and this is
// the case that has to keep working.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { workbench, openFileTab } from '../lib/stores/workbench.svelte'
import { ReadFile } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
})

describe('a file the editor cannot render', () => {
  it('is marked unreadable rather than becoming an editor full of the error', async () => {
    ReadFile.mockRejectedValueOnce(new Error('binary file cannot be previewed'))

    await openFileTab('out/นำเสนอ.pptx')

    const tab = workbench.tabs[0]
    expect(tab.kind).toBe('file')
    expect(tab.unreadable).toContain('binary file cannot be previewed')
    // The distinction the pane switches on: no content means no editor.
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
