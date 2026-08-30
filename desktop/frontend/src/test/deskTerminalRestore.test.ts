// The right panel comes back whole, terminals included.
//
// It used to come back in pieces: the snapshot dropped every terminal on the
// grounds that a PTY is a live process, so a desk holding one came back empty
// after the app was closed — *"มาแค่แชทแต่แท็ปข้างที่เปิดไว้ก่อนหน้าหาย"* (30 ส.ค.).
// The process really cannot survive; the terminal's place on the desk can, and
// that is what the panel is.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  workbench, openTerminalTab, adoptWorkbenchSession, switchWorkbenchSession,
  saveWorkbenchSnapshot,
} from '../lib/stores/workbench.svelte'
import { TerminalShells, TerminalStart } from './mocks/wailsApp'

const PWSH = { name: 'PowerShell', path: 'C:\pwsh.exe' }
const CMD = { name: 'Command Prompt', path: 'C:\cmd.exe' }

const saved = (id: string) =>
  JSON.parse(localStorage.getItem(`aetox-workbench:${id}`) ?? '{"tabs":[]}') as {
    tabs: { kind: string; name: string; shell?: string }[]
  }

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
  localStorage.clear()
  vi.mocked(TerminalShells).mockResolvedValue([PWSH, CMD] as any)
  let n = 0
  vi.mocked(TerminalStart).mockImplementation(async () => `pty-${++n}` as any)
})

describe('a terminal on the desk', () => {
  it('is saved with the shell it was started from, and started again on the way back', async () => {
    await adoptWorkbenchSession('chat-a')
    await openTerminalTab(CMD)
    // What the Workbench component's autosave effect does on every tab change.
    saveWorkbenchSnapshot()
    expect(saved('chat-a').tabs).toEqual([
      { kind: 'terminal', name: 'Command Prompt', shell: 'C:\cmd.exe' },
    ])

    // Away and back — the same round trip a restart makes, without the process.
    await switchWorkbenchSession('chat-b')
    expect(workbench.tabs).toHaveLength(0)
    await switchWorkbenchSession('chat-a')

    expect(workbench.tabs).toHaveLength(1)
    expect(workbench.tabs[0].kind).toBe('terminal')
    expect(workbench.tabs[0].name).toBe('Command Prompt')
    // The shell it had, not whichever one happens to be first.
    expect(TerminalStart).toHaveBeenLastCalledWith('C:\cmd.exe', 80, 24)
    // A new PTY, so the tab is keyed on the session that actually exists.
    expect(workbench.tabs[0].id).toBe('pty-2')
  })

  it('falls back by name, then to any shell, when the saved path is gone', async () => {
    // An update moved the exe: the path in the snapshot no longer exists.
    localStorage.setItem('aetox-workbench:moved', JSON.stringify({
      tabs: [{ kind: 'terminal', name: 'PowerShell', shell: 'C:\old\pwsh.exe' }], activeIdx: 0,
    }))
    await switchWorkbenchSession('moved')
    expect(TerminalStart).toHaveBeenLastCalledWith('C:\pwsh.exe', 80, 24)

    // A layout written before terminals were saved names a shell this machine
    // does not have. The desk still comes back, on whatever shell there is.
    localStorage.setItem('aetox-workbench:old', JSON.stringify({
      tabs: [{ kind: 'terminal', name: 'bash' }], activeIdx: 0,
    }))
    await switchWorkbenchSession('old')
    expect(TerminalStart).toHaveBeenLastCalledWith('C:\pwsh.exe', 80, 24)
    expect(workbench.tabs).toHaveLength(1)
  })

  it('leaves the rest of the desk standing when no shell will start', async () => {
    vi.mocked(TerminalShells).mockResolvedValue([] as any)
    localStorage.setItem('aetox-workbench:noshell', JSON.stringify({
      tabs: [{ kind: 'terminal', name: 'PowerShell' }, { kind: 'files', name: 'ไฟล์' }],
      activeIdx: 0,
    }))
    await switchWorkbenchSession('noshell')
    expect(workbench.tabs.map((t) => t.kind)).toEqual(['files'])
  })
})
