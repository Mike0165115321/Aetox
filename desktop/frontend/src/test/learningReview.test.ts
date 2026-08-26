// The approval surface. The whole learning design rests on nothing taking
// effect until a person allows it, which only holds up if the person can see
// what is waiting, judge it, and be told it is there at all.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import Sidebar from '../lib/Sidebar.svelte'
import {
  ListPendingChanges, ListDecidedChanges, LearnedMemory, LearnedEntries, SaveLearnedEntry,
  LearningEnabled, ApprovePendingChange, RejectPendingChange, SetLearningEnabled,
  PendingLearnedCount, LearnedScopeInfos, ForgetMemoryScope, AdoptMemoryScope, RecentProjects,
} from './mocks/wailsApp'
import { cockpit, applyPendingLearned, refreshPendingLearned } from '../lib/stores/cockpit.svelte'

const proposal = (over: Record<string, unknown> = {}) => ({
  id: 1, kind: 'memory', scope: '', target: 'C:/aetox/memory/MEMORY.md',
  op: 'add', before: '', body: 'เครื่องนี้ไม่มี Excel ติดตั้ง',
  reason: 'เปิดไฟล์ .xlsx แล้วไม่มีโปรแกรมรับ', evidence: 'session:1',
  source: 'agent', state: 'pending', createdAt: '2026-08-04T10:00:00Z', decidedAt: '',
  ...over,
})

const openSection = async (container: HTMLElement, label: string) => {
  const item = Array.from(container.querySelectorAll('.settings-nav-item'))
    .find((el) => el.textContent?.includes(label))
  if (!item) throw new Error(`nav item "${label}" not found`)
  await fireEvent.click(item)
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.pendingLearned = 0
  vi.mocked(LearningEnabled).mockResolvedValue(true)
  vi.mocked(LearnedMemory).mockResolvedValue('')
  vi.mocked(LearnedEntries).mockResolvedValue([] as any)
  vi.mocked(LearnedScopeInfos).mockResolvedValue([] as any)
  vi.mocked(ListDecidedChanges).mockResolvedValue([] as any)
  vi.mocked(ListPendingChanges).mockResolvedValue([proposal()] as any)
})

// The memory list is the one thing on this page the *user* writes. Approving is
// how the agent's proposals get in; this is how a line that got in and turned
// out to be noise gets fixed, without leaving the app for a text editor.
describe('editing what is already remembered', () => {
  // The case the row index exists for. A real file collects lines that differ
  // only at the end, and the fourth must be the one that moves.
  const shell = (status: string) =>
    `เครื่องมือ shell เคยล้มซ้ำ ๆ ด้วยเหตุเดียวกัน: "exit status ${status}"`

  const openMemory = async () => {
    vi.mocked(LearnedScopeInfos).mockResolvedValue([{ scope: '', orphan: false }] as any)
    vi.mocked(LearnedEntries).mockResolvedValue(
      ['เครื่องผู้ใช้เป็น Windows', shell('1'), shell('2'), shell('124')] as any,
    )
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(container.querySelectorAll('.mem-row').length).toBe(4))
    return container
  }

  it('gives every remembered line its own row', async () => {
    const container = await openMemory()
    const rows = container.querySelectorAll('.mem-row .mem-text')
    expect(rows[0].textContent).toContain('Windows')
    expect(rows[3].textContent).toContain('exit status 124')
  })

  it('saves the row that was edited, by its position and not by its text', async () => {
    const container = await openMemory()
    const rows = Array.from(container.querySelectorAll('.mem-row'))
    await fireEvent.click(rows[3].querySelector('.icobtn')!)

    const box = container.querySelector('.mem-input') as HTMLTextAreaElement
    expect(box.value).toContain('exit status 124')
    await fireEvent.input(box, { target: { value: 'shell timeout บ่อยในโปรเจกต์นี้' } })
    // Scoped to the open row: the approval card above has a .ctrl-primary too,
    // and it comes first in the document.
    await fireEvent.click(container.querySelector('.mem-row.editing .ctrl-primary')!)

    // Index 3 — the row on screen. The three lines above it all contain
    // "exit status", so anything matching on text would take the wrong one.
    await waitFor(() =>
      expect(SaveLearnedEntry).toHaveBeenCalledWith('', 3, 'shell timeout บ่อยในโปรเจกต์นี้'))
  })

  it('forgets a line by saving it empty, and reloads so the rows renumber', async () => {
    const container = await openMemory()
    const rows = Array.from(container.querySelectorAll('.mem-row'))
    await fireEvent.click(rows[1].querySelector('.mem-forget')!)

    await waitFor(() => expect(SaveLearnedEntry).toHaveBeenCalledWith('', 1, ''))
    // A delete moves every row below it, so the positions the next edit sends
    // have to come from the file again rather than from the stale array.
    // Awaited: the reload asks which scopes exist before it asks for any lines,
    // so the re-read lands a tick later than it used to.
    await waitFor(() => expect(LearnedEntries).toHaveBeenCalledTimes(2))
  })

  it('says why a save failed instead of quietly leaving the line as it was', async () => {
    const container = await openMemory()
    vi.mocked(SaveLearnedEntry).mockRejectedValueOnce(new Error('memory is full'))
    await fireEvent.click(container.querySelectorAll('.mem-row')[0].querySelector('.icobtn')!)
    await fireEvent.input(container.querySelector('.mem-input')!, { target: { value: 'x' } })
    // Scoped to the open row: the approval card above has a .ctrl-primary too,
    // and it comes first in the document.
    await fireEvent.click(container.querySelector('.mem-row.editing .ctrl-primary')!)

    await waitFor(() => expect(container.textContent).toContain('memory is full'))
  })

  // A line can land in three different files now — the shared one, a desk's, a
  // project's — and this page was built when there was only one. A file it
  // cannot show is a file only the folder knows about, which is the whole thing
  // the page exists to avoid.
  it('shows every memory that holds something, each under whose it is', async () => {
    vi.mocked(LearnedScopeInfos).mockResolvedValue([
      { scope: '', orphan: false }, { scope: 'mode:coding', orphan: false }, { scope: 'project:Aetox-1a2b3c4d', orphan: false },
    ] as any)
    vi.mocked(LearnedEntries).mockImplementation(async (scope: string) => {
      if (scope === '') return ['เครื่องผู้ใช้เป็น Windows'] as any
      if (scope === 'mode:coding') return ['เจ้าของอ่านดิฟก่อนเสมอ'] as any
      return ['เราตกลงกันว่า statereport เป็นคนบอกว่า error มาจากโลกภายนอก'] as any
    })

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(container.querySelectorAll('.mem-row').length).toBe(3))

    const heads = Array.from(container.querySelectorAll('.mem-scope')).map((el) => el.textContent?.trim())
    expect(heads).toEqual(['ผู้ช่วยหลัก', 'โต๊ะโค้ด', 'โปรเจกต์ Aetox'])
    // The hash half of a project key is identity, not information — a person
    // recognises the folder, not the digest.
    expect(container.textContent).not.toContain('1a2b3c4d')
  })

  // A project's memory file is keyed by the folder's path, so a moved or
  // deleted folder strands its file — readable in the folder, reachable by no
  // session, and indistinguishable on disk from a live one. The page is the
  // only place that can say so, and a label without its exits is a nagging
  // sign: move the lines to the project the folder became, or let them go.
  it('names an orphaned project memory and offers its two exits', async () => {
    vi.mocked(LearnedScopeInfos).mockResolvedValue([
      { scope: '', orphan: false }, { scope: 'project:old-app-99aa88bb', orphan: true },
    ] as any)
    vi.mocked(LearnedEntries).mockImplementation(async (scope: string) =>
      (scope === '' ? ['เครื่องผู้ใช้เป็น Windows'] : ['ตกลงกันว่าใช้ PowerShell']) as any)
    vi.mocked(RecentProjects).mockResolvedValue([
      { key: 'old-app-11223344', name: 'old-app', rootPath: 'D:/work/old-app', openedAt: '', snippet: '' },
    ] as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(container.querySelectorAll('.mem-row').length).toBe(2))

    // The live file carries no mark; the orphan carries exactly one.
    expect(container.querySelectorAll('.mem-orphan').length).toBe(1)
    const heads = Array.from(container.querySelectorAll('.mem-scope'))
    expect(heads[0].querySelector('.mem-orphan')).toBeNull()
    expect(heads[1].textContent).toContain('โฟลเดอร์นี้ไม่อยู่แล้ว')

    // Exit one: move into a project the store still knows.
    await fireEvent.click(heads[1].querySelector('.mem-orphan-actions .ctrl')!)
    const target = Array.from(container.querySelectorAll('.mem-adopt .ctrl'))
      .find((el) => el.textContent?.includes('old-app'))
    await fireEvent.click(target!)
    await waitFor(() =>
      expect(AdoptMemoryScope).toHaveBeenCalledWith('project:old-app-99aa88bb', 'D:/work/old-app'))

    // Exit two: delete — and it reaches the whole-file door, never a row's.
    await fireEvent.click(heads[1].querySelector('.mem-orphan-actions .mem-forget')!)
    await waitFor(() =>
      expect(ForgetMemoryScope).toHaveBeenCalledWith('project:old-app-99aa88bb'))
  })

  // Editing has to reach the file the row came from. Every group counts its own
  // rows from zero, so a save that forgot the scope would rewrite line 0 of the
  // main memory while the user was looking at line 0 of a project's.
  it('edits the line in the file it belongs to', async () => {
    vi.mocked(LearnedScopeInfos).mockResolvedValue([{ scope: '', orphan: false }, { scope: 'project:Aetox-1a2b3c4d', orphan: false }] as any)
    vi.mocked(LearnedEntries).mockImplementation(async (scope: string) =>
      (scope === '' ? ['เครื่องผู้ใช้เป็น Windows'] : ['ตกลงกันว่าใช้ PowerShell']) as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(container.querySelectorAll('.mem-row').length).toBe(2))

    const projectRow = container.querySelectorAll('.mem-row')[1]
    await fireEvent.click(projectRow.querySelector('.mem-forget')!)

    await waitFor(() =>
      expect(SaveLearnedEntry).toHaveBeenCalledWith('project:Aetox-1a2b3c4d', 0, ''))
  })

  // The folder is the promise that this is plain markdown you can take away.
  it('keeps the way out to the folder', async () => {
    const container = await openMemory()
    expect(container.querySelector('.learn-foot .ctrl')?.textContent)
      .toContain('เปิดโฟลเดอร์ความจำ')
  })
})

describe('the learning review page', () => {
  // Approving a sentence without its reasoning is signing for an assertion
  // with no provenance, which is the thing this page exists to prevent.
  it('shows what would be remembered, whose memory it is, and why', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')

    await waitFor(() => expect(screen.getByText('เครื่องนี้ไม่มี Excel ติดตั้ง')).toBeTruthy())
    expect(screen.getByText('เปิดไฟล์ .xlsx แล้วไม่มีโปรแกรมรับ')).toBeTruthy()
    expect(screen.getByText('ผู้ช่วยหลัก')).toBeTruthy()
  })

  // A delegate's memory is not the assistant's, and the row has to say so —
  // scope is the difference between "everything you ask it" and "one job".
  it('names the sub-agent when the proposal is not the main assistant\'s', async () => {
    vi.mocked(ListPendingChanges).mockResolvedValue([proposal({ scope: 'explore' })] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')

    await waitFor(() => expect(screen.getByText('explore')).toBeTruthy())
  })

  // What a change overwrites is part of the decision.
  it('shows the line a replacement would overwrite', async () => {
    vi.mocked(ListPendingChanges).mockResolvedValue([
      proposal({ op: 'replace', before: 'สแกนเนอร์เขียนลง D:\\Scans', body: 'สแกนเนอร์เขียนลง E:\\Scans' }),
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')

    await waitFor(() => expect(screen.getByText('สแกนเนอร์เขียนลง D:\\Scans')).toBeTruthy())
    expect(screen.getByText('สแกนเนอร์เขียนลง E:\\Scans')).toBeTruthy()
  })

  it('approves and discards through to the engine', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(screen.getByText('อนุมัติ')).toBeTruthy())

    await fireEvent.click(screen.getByText('อนุมัติ'))
    await waitFor(() => expect(ApprovePendingChange).toHaveBeenCalledWith(1))

    await fireEvent.click(screen.getByText('ไม่เอา'))
    await waitFor(() => expect(RejectPendingChange).toHaveBeenCalledWith(1))
  })

  // An approval that could not be applied leaves the proposal in the list, and
  // a button that appears to do nothing reads as a broken feature.
  //
  // The mocked error is the one Apply can still produce — the scope filled up
  // between the proposal and the click. It used to be "no remembered line
  // contains", which §139 removed: a stale revision now lands instead of
  // erroring, so a test still waving that message would be rehearsing a
  // failure the engine can no longer send.
  it('says so when an approval could not be applied', async () => {
    vi.mocked(ApprovePendingChange).mockRejectedValueOnce(
      new Error("this scope's memory is full (8215 bytes, limit 8192) — merge or drop an existing line first"))
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(screen.getByText('อนุมัติ')).toBeTruthy())

    await fireEvent.click(screen.getByText('อนุมัติ'))
    await waitFor(() => expect(screen.getByText(/memory is full/)).toBeTruthy())
  })

  it('carries the kill switch, and turning it off reaches the engine', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(screen.getByText('ให้ Aetox เรียนรู้จากงานที่ทำ')).toBeTruthy())

    const box = container.querySelector('.mswitch input') as HTMLInputElement
    expect(box.checked).toBe(true)
    await fireEvent.change(box)
    await waitFor(() => expect(SetLearningEnabled).toHaveBeenCalledWith(false))
  })
})

describe('being told there is something waiting', () => {
  it('marks the way into settings, and only when something is waiting', async () => {
    const { container } = render(Sidebar, {
      onOpenSettings: () => {}, onOpenFile: () => {}, collapsed: false, onToggle: () => {},
    } as any)
    expect(container.querySelector('.gear.has-pending')).toBeNull()

    applyPendingLearned(2)
    await waitFor(() => expect(container.querySelector('.gear.has-pending')).toBeTruthy())
  })

  it('counts the row that opens them', async () => {
    applyPendingLearned(3)
    const { container } = render(Settings, { onClose: () => {} })
    const badge = container.querySelector('.nav-count')
    expect(badge?.textContent?.trim()).toBe('3')
  })

  // Anything left undecided in an earlier session is still undecided, and
  // nothing would emit an event for it.
  it('picks up a queue left over from a previous session', async () => {
    vi.mocked(PendingLearnedCount).mockResolvedValue(4)
    await refreshPendingLearned()
    expect(cockpit.pendingLearned).toBe(4)
  })
})
