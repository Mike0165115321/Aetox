// The problems room, and the wall between it and the approval queue.
//
// For a year the summarizer wrote its failure clusters into the queue whose
// copy promises "อนุมัติแล้วจะเข้าไปอยู่ในความจำ" — seventeen of the twenty-two
// cards that queue ever held, one of them a usable lesson. What is asserted
// here is the split those numbers bought: a repeated failure is a problem you
// may report, never a thing you are asked to remember
// (docs/architecture/system-problems-vs-learning-2026-08-18.md).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  ListPendingChanges, ListDecidedChanges, LearnedMemory, LearnedEntries, LearnedScopes,
  LearningEnabled, ListSystemIssues, MarkIssueReported, RejectPendingChange,
  AppVersion, CheckForUpdate, RecentDebugLog,
} from './mocks/wailsApp'
import { BrowserOpenURL } from './mocks/wailsRuntime'
import { cockpit } from '../lib/stores/cockpit.svelte'

const issue = (over: Record<string, unknown> = {}) => ({
  id: 22, kind: 'issue', scope: '', target: '', op: 'add', before: '',
  body: 'เครื่องมือ image_ocr ล้มซ้ำด้วยเหตุเดียวกัน: "ไม่พบโปรแกรม Tesseract ในเครื่อง"',
  reason: 'เกิด 3 ครั้ง — ตัวอย่างล่าสุดที่ล้ม: {"path": "output/page-33.png"}',
  evidence: 'tool_runs:41,55,60',
  source: 'summarizer', state: 'pending', createdAt: '2026-08-18T05:22:24Z', decidedAt: '',
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
  cockpit.pendingIssues = 0
  vi.mocked(LearningEnabled).mockResolvedValue(true)
  vi.mocked(LearnedMemory).mockResolvedValue('')
  vi.mocked(LearnedEntries).mockResolvedValue([] as any)
  vi.mocked(LearnedScopes).mockResolvedValue([] as any)
  vi.mocked(ListDecidedChanges).mockResolvedValue([] as any)
  vi.mocked(ListPendingChanges).mockResolvedValue([] as any)
  vi.mocked(ListSystemIssues).mockResolvedValue([issue()] as any)
})

describe('problems and lessons are two rooms', () => {
  // The card in the screenshot that started this: a missing Tesseract, offered
  // under "อนุมัติแล้วจะเข้าไปอยู่ในความจำ". It belongs on the other page, and
  // the verb it is offered with is not "จำไว้".
  it('shows a repeated failure with report and dismiss, never with approve', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ปัญหาของระบบ')

    expect(await screen.findByText(/ไม่พบโปรแกรม Tesseract/)).toBeTruthy()
    expect(screen.getByText('เกิด 3 ครั้ง — ตัวอย่างล่าสุดที่ล้ม: {"path": "output/page-33.png"}')).toBeTruthy()
    expect(screen.getByText('แจ้งปัญหานี้')).toBeTruthy()
    expect(screen.getByText('ไม่เป็นไร')).toBeTruthy()
    expect(screen.queryByText('อนุมัติ')).toBeNull()
  })

  // The learning page is the half that had to get quieter. It reads its own
  // list, and a summarizer row is not in it.
  it('leaves the approval queue empty of failures', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การเรียนรู้')

    expect(await screen.findByText('ยังไม่มีอะไรรออนุมัติ')).toBeTruthy()
    expect(screen.queryByText(/ไม่พบโปรแกรม Tesseract/)).toBeNull()
  })

  // One door out of the machine, and the user is the last reader before
  // anything leaves it: the button opens GitHub's own form, prefilled, in their
  // browser. Nothing is sent from here.
  it('reports through the existing GitHub door, with the cluster and its evidence prefilled', async () => {
    vi.mocked(AppVersion).mockResolvedValue('1.2.0' as never)
    vi.mocked(RecentDebugLog).mockResolvedValue([] as never)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ปัญหาของระบบ')

    await fireEvent.click(await screen.findByText('แจ้งปัญหานี้'))

    await waitFor(() => expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledTimes(1))
    const url = vi.mocked(BrowserOpenURL).mock.calls[0][0] as string
    expect(url.startsWith('https://github.com/Mikedev115/Aetox/issues/new?body=')).toBe(true)
    const body = decodeURIComponent(url.split('body=')[1])
    expect(body).toContain('ไม่พบโปรแกรม Tesseract')
    expect(body).toContain('เกิด 3 ครั้ง')
    // The rows it was drawn from: a report whose evidence stayed behind makes
    // the reader go hunting for what the app already knew.
    expect(body).toContain('tool_runs:41,55,60')
    expect(body).toContain('v1.2.0')

    // And the row is marked afterwards, so it stops asking.
    await waitFor(() => expect(vi.mocked(MarkIssueReported)).toHaveBeenCalledWith(22))
  })

  // The button that did nothing. A Thai debug log capped at 4,000 characters is
  // ~36,000 characters once percent-encoded (three bytes per character, three
  // characters per byte), Windows caps a command line at 32,767, and Wails hands
  // the URL to rundll32 and logs its own failure somewhere no user sees. Pressing
  // แจ้งปัญหานี้ opened nothing at all, with no error (2026-08-18).
  it('keeps the prefilled URL short enough for the OS to open, however long the log', async () => {
    vi.mocked(AppVersion).mockResolvedValue('1.2.0' as never)
    // Thai, because Thai is what this log is full of and what makes it explode.
    vi.mocked(RecentDebugLog).mockResolvedValue(
      Array.from({ length: 400 }, (_, i) => `เปิดเซิร์ฟเวอร์ไม่สำเร็จ ลองใหม่อีกครั้ง #${i}`) as never,
    )
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ปัญหาของระบบ')

    await fireEvent.click(await screen.findByText('แจ้งปัญหานี้'))

    await waitFor(() => expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledTimes(1))
    const url = vi.mocked(BrowserOpenURL).mock.calls[0][0] as string
    expect(url.length).toBeLessThanOrEqual(8000)

    // Short is not enough on its own: the report still has to carry the problem.
    // A URL trimmed to nothing would pass a length check and be useless.
    const body = decodeURIComponent(url.split('body=')[1])
    expect(body).toContain('ไม่พบโปรแกรม Tesseract')
    expect(body).toContain('v1.2.0')
  })

  // The other half of why the button did nothing, and the half that fires first:
  // Wails validates the URL before opening it and refuses one containing any of
  // ;|`$\<>*{}[]()~! or whitespace, logging the refusal somewhere no user sees.
  // encodeURIComponent is specified NOT to encode !~*'() — so a single ordinary
  // parenthesis, and every problem reason has one, was enough to stop it dead.
  it('encodes the characters Wails refuses to open a URL with', async () => {
    vi.mocked(AppVersion).mockResolvedValue('1.2.0' as never)
    vi.mocked(RecentDebugLog).mockResolvedValue([] as never)
    vi.mocked(ListSystemIssues).mockResolvedValue([
      issue({ reason: 'เกิด 3 ครั้ง (ตัวอย่าง) ~ * ! [x] {y}' }),
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ปัญหาของระบบ')

    await fireEvent.click(await screen.findByText('แจ้งปัญหานี้'))

    await waitFor(() => expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledTimes(1))
    const url = vi.mocked(BrowserOpenURL).mock.calls[0][0] as string
    // Wails' own list, spelled as characters rather than a regex so a failure
    // names which one leaked. Copied rather than imported: it lives inside a
    // vendored package this side cannot reach.
    const FORBIDDEN = [';', '|', '`', '$', '\\', '<', '>', '*', '{', '}', '[', ']', '(', ')', '~', '!', ' ', '\t', '\n', '\r']
    expect(FORBIDDEN.filter((c) => url.includes(c))).toEqual([])

    // Still a real report on the other side of the encoding.
    const body = decodeURIComponent(url.split('body=')[1])
    expect(body).toContain('(ตัวอย่าง)')
  })

  // "ไม่เป็นไร" is the same act as "ไม่เอา" on a lesson: read, and turned down.
  // One record, one function, and it never comes back.
  it('dismisses a problem without sending anything', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ปัญหาของระบบ')

    await fireEvent.click(await screen.findByText('ไม่เป็นไร'))

    await waitFor(() => expect(vi.mocked(RejectPendingChange)).toHaveBeenCalledWith(22))
    expect(vi.mocked(BrowserOpenURL)).not.toHaveBeenCalled()
    expect(vi.mocked(MarkIssueReported)).not.toHaveBeenCalled()
  })

  // The mark goes on the problems row and nowhere else. A failure cluster must
  // not light the gear in the sidebar: that mark means "only you can decide
  // this", and a problem can wait until someone comes looking.
  it('marks the problems row in the nav and leaves the learning badge alone', async () => {
    cockpit.pendingIssues = 3
    const { container } = render(Settings, { onClose: () => {} })

    await waitFor(() => {
      const row = Array.from(container.querySelectorAll('.settings-nav-item'))
        .find((el) => el.textContent?.includes('ปัญหาของระบบ'))
      expect(row?.querySelector('.nav-count')?.textContent?.trim()).toBe('3')
    })
    const learningRow = Array.from(container.querySelectorAll('.settings-nav-item'))
      .find((el) => el.textContent?.includes('การเรียนรู้'))
    expect(learningRow?.querySelector('.nav-count')).toBeNull()
  })
})
