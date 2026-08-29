// Opening a project is a session switch, and the desk has to follow it.
//
// The engine opens a NEW chat when a project is opened (startNewSession, inside
// OpenProjectPath / OpenProjectFolder / ClearProjectFocus). The window did not
// say so: the three doors cleared cockpit.chat by hand and never arrived
// anywhere, so the workbench stayed bound to the chat that had been left. The
// owner's screenshot on 29 ส.ค. is that: a code map of the project he had just
// left, still drawn on the right of a brand-new chat in another one — and
// autosaved onto the old session on every keystroke after it.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, openProject, openFolder, clearProjectFocus } from '../lib/stores/cockpit.svelte'
import {
  workbench, openFilesTab, adoptWorkbenchSession,
} from '../lib/stores/workbench.svelte'
import { CurrentSessionID } from './mocks/wailsApp'

beforeEach(async () => {
  vi.clearAllMocks()
  localStorage.clear()
  workbench.tabs.length = 0
  workbench.activeId = ''
  cockpit.sessionError = ''
  cockpit.openSession = ''
  // Standing in one project, with something open on the right.
  vi.mocked(CurrentSessionID).mockResolvedValue('before')
  await adoptWorkbenchSession('before')
  openFilesTab()
})

describe('a project door arrives at the chat it opened', () => {
  for (const [name, go] of [
    ['the sidebar\u2019s project list', () => openProject('D:/other')],
    ['the folder dialog', () => openFolder()],
    ['dropping project focus', () => clearProjectFocus()],
  ] as const) {
    it(`clears the previous chat's desk: ${name}`, async () => {
      vi.mocked(CurrentSessionID).mockResolvedValue('after')

      await go()

      // The window is on the new chat...
      expect(cockpit.openSession).toBe('after')
      expect(cockpit.chat).toEqual([])
      // ...and the right-hand desk is that chat's, which is empty.
      expect(workbench.tabs).toEqual([])
      // Not lost — it belongs to the chat that had it open.
      expect(JSON.parse(localStorage.getItem('aetox-workbench:before') ?? '{}').tabs)
        .toEqual([{ kind: 'files', name: expect.any(String), url: undefined, path: undefined, mine: undefined }])
    })
  }
})

describe('a room the desk saves is a room it can restore', () => {
  it('brings the room back with the chat that had it open', async () => {
    vi.mocked(CurrentSessionID).mockResolvedValue('after')
    await openProject('D:/other')
    expect(workbench.tabs).toEqual([])

    // Back to the chat that had it: the room is where it was left.
    const { switchWorkbenchSession } = await import('../lib/stores/workbench.svelte')
    await switchWorkbenchSession('before')

    expect(workbench.tabs.map((t) => t.kind)).toEqual(['files'])
    expect(workbench.activeId).toBe('files')
  })
})
