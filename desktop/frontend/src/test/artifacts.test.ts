// ผลงาน (COMPANY.md §2/§6.7). Two rules carry the page: the files outlive the
// conversations that made them, and this is the only place they are deleted —
// by the user, on purpose, with a confirm step, because they are the user's
// files and nothing else in the product removes them.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Artifacts from '../lib/Artifacts.svelte'
import {
  ListArtifacts, OpenArtifact, DeleteArtifact, LoadSessionAnyProject, ArtifactPreview,
} from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'

const file = (over: Record<string, unknown> = {}) => ({
  name: 'สรุปยอด.xlsx', path: 'C:/Users/x/aetox/output/20260805-090000.000/สรุปยอด.xlsx',
  sessionId: '20260805-090000.000', size: 20480, modified: new Date().toISOString(),
  root: 'C:/Users/x/aetox', ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.activeView = 'artifacts'
  vi.mocked(ListArtifacts).mockResolvedValue([file()] as any)
})

describe('the work gallery', () => {
  it('shows each file with its size and when it was made', async () => {
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('สรุปยอด.xlsx')).toBeTruthy())
    expect(screen.getByText(/20 KB/)).toBeTruthy()
  })

  it('opens a file with the program that owns it', async () => {
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('สรุปยอด.xlsx')).toBeTruthy())
    await fireEvent.click(screen.getByText('สรุปยอด.xlsx'))

    await waitFor(() => expect(vi.mocked(OpenArtifact).mock.calls[0][0]).toContain('สรุปยอด.xlsx'))
  })

  it('walks back to the chat that produced it', async () => {
    vi.mocked(LoadSessionAnyProject).mockResolvedValue([] as any)
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ไปที่แชทที่ทำไฟล์นี้')).toBeTruthy())
    await fireEvent.click(screen.getByText('ไปที่แชทที่ทำไฟล์นี้'))

    await waitFor(() => expect(vi.mocked(LoadSessionAnyProject).mock.calls[0][0]).toBe('20260805-090000.000'))
    expect(cockpit.activeView).toBe('chat')
  })

  // A file whose session was deleted is still work the user has. It loses the
  // link back and nothing else — §6.7 is about the file outliving the chat.
  it('still shows a file whose conversation is gone', async () => {
    vi.mocked(ListArtifacts).mockResolvedValue([file({ sessionId: '' })] as any)
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('สรุปยอด.xlsx')).toBeTruthy())
    expect(screen.getByText('ไม่รู้ว่ามาจากแชทไหน')).toBeTruthy()
    expect(screen.queryByText('ไปที่แชทที่ทำไฟล์นี้')).toBeNull()
  })

  it('deletes only on the second click, and only through this page', async () => {
    render(Artifacts, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText('สรุปยอด.xlsx')).toBeTruthy())

    const del = document.querySelector('.art-del') as HTMLButtonElement
    await fireEvent.click(del)
    expect(vi.mocked(DeleteArtifact)).not.toHaveBeenCalled()
    expect(del.textContent?.trim()).toBe('ยืนยัน?')

    await fireEvent.click(del)
    await waitFor(() => expect(vi.mocked(DeleteArtifact).mock.calls[0][0]).toContain('สรุปยอด.xlsx'))
    // The list is re-read from disk rather than patched in place: the disk is
    // the index, and a gallery that edited its own copy would be a second one.
    expect(vi.mocked(ListArtifacts).mock.calls.length).toBeGreaterThan(1)
  })

  it('has an empty state for an install that has made nothing yet', async () => {
    vi.mocked(ListArtifacts).mockResolvedValue([] as any)
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText(/ยังไม่มีไฟล์/)).toBeTruthy())
  })
})

// A grid of filenames answers "what is it called", which is the one thing the
// person coming back here has already forgotten. Two .docx called สรุปผล… and
// นิสัย… are the same card twice until a line of either one is on screen.
describe('what a card shows of the file inside it', () => {
  const withPreview = (name: string, preview: Record<string, unknown>) => {
    vi.mocked(ListArtifacts).mockResolvedValue([file({ name, path: 'C:/x/output/s/' + name })] as any)
    vi.mocked(ArtifactPreview).mockResolvedValue(preview as any)
  }

  it('renders a markdown report as the document it is', async () => {
    withPreview('file-scan-report.md', { kind: 'markdown', text: '# รายงาน\n\nพบ 516,374 ไฟล์' })
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('รายงาน')).toBeTruthy())
    expect(document.querySelector('.art-thumb-md h1')).toBeTruthy()
  })

  it('shows a workbook as a grid of its own cells', async () => {
    withPreview('สรุปยอด.xlsx', { kind: 'sheet', sheet: 'สรุป', rows: [['เดือน', 'ยอด'], ['ม.ค.', '1200']] })
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('เดือน')).toBeTruthy())
    expect(screen.getByText('1200')).toBeTruthy()
  })

  // An .html artifact is shown as the page it is. Inside sandbox="" — no
  // scripts, no forms, no same-origin — because this is a file the agent wrote
  // and the gallery is inside the app's own document.
  it('renders an html artifact in a sandboxed frame', async () => {
    withPreview('northstar-brand.html', { kind: 'html', text: '<h1>Northstar</h1>' })
    render(Artifacts, { onClose: () => {} })

    const frame = await waitFor(() => {
      const el = document.querySelector('iframe.art-thumb-frame')
      expect(el).toBeTruthy()
      return el as HTMLIFrameElement
    })
    expect(frame.getAttribute('sandbox')).toBe('')
    expect(frame.getAttribute('srcdoc')).toContain('Northstar')
  })

  // A PDF, a zip, a file deleted underneath us. The card keeps its mark and
  // its name — a preview is a bonus, never the reason the row exists.
  it('falls back to the file mark when there is nothing to draw', async () => {
    withPreview('รายงาน.pdf', { kind: 'none' })
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(document.querySelector('.art-thumb.plain')).toBeTruthy())
    expect(screen.getByText('รายงาน.pdf')).toBeTruthy()
  })

  it('keeps the card when the preview call itself fails', async () => {
    vi.mocked(ListArtifacts).mockResolvedValue([file({ name: 'gone.md' })] as any)
    vi.mocked(ArtifactPreview).mockRejectedValue(new Error('ไฟล์นี้ไม่ได้อยู่ในโฟลเดอร์ผลงาน'))
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('gone.md')).toBeTruthy())
    expect(document.querySelector('.art-thumb.plain')).toBeTruthy()
  })
})
