// The approval surface. The whole learning design rests on nothing taking
// effect until a person allows it, which only holds up if the person can see
// what is waiting, judge it, and be told it is there at all.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import Sidebar from '../lib/Sidebar.svelte'
import {
  ListPendingChanges, ListDecidedChanges, LearnedMemory, LearningEnabled,
  ApprovePendingChange, RejectPendingChange, SetLearningEnabled, PendingLearnedCount,
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
  vi.mocked(ListDecidedChanges).mockResolvedValue([] as any)
  vi.mocked(ListPendingChanges).mockResolvedValue([proposal()] as any)
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
  it('says so when an approval could not be applied', async () => {
    vi.mocked(ApprovePendingChange).mockRejectedValueOnce(new Error('no remembered line contains "x"'))
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')
    await waitFor(() => expect(screen.getByText('อนุมัติ')).toBeTruthy())

    await fireEvent.click(screen.getByText('อนุมัติ'))
    await waitFor(() => expect(screen.getByText(/no remembered line/)).toBeTruthy())
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
