// ผลงาน (COMPANY.md §2/§6.7). Two rules carry the page: the files outlive the
// conversations that made them, and this is the only place they are deleted —
// by the user, on purpose, with a confirm step, because they are the user's
// files and nothing else in the product removes them.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Artifacts from '../lib/Artifacts.svelte'
import {
  ListArtifactsIn, OpenArtifact, DeleteArtifact, LoadSessionAnyProject, ArtifactPreview,
  CompressArtifacts,
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
  // One file, in the week the page opens at. The gallery asks for a range and
  // is answered with the range it got, which is what the picker then shows.
  vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [file()], range: 'week', total: 1 } as any)
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
    vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [file({ sessionId: '' })], range: 'week', total: 1 } as any)
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
    expect(vi.mocked(ListArtifactsIn).mock.calls.length).toBeGreaterThan(1)
  })

  it('has an empty state for an install that has made nothing yet', async () => {
    vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [], range: 'week', total: 0 } as any)
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText(/ยังไม่มีไฟล์/)).toBeTruthy())
  })
})

// A grid of filenames answers "what is it called", which is the one thing the
// person coming back here has already forgotten. Two .docx called สรุปผล… and
// นิสัย… are the same card twice until a line of either one is on screen.
describe('what a card shows of the file inside it', () => {
  const withPreview = (name: string, preview: Record<string, unknown>) => {
    vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [file({ name, path: 'C:/x/output/s/' + name })], range: 'week', total: 1 } as any)
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
    vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [file({ name: 'gone.md' })], range: 'week', total: 1 } as any)
    vi.mocked(ArtifactPreview).mockRejectedValue(new Error('ไฟล์นี้ไม่ได้อยู่ในโฟลเดอร์ผลงาน'))
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('gone.md')).toBeTruthy())
    expect(document.querySelector('.art-thumb.plain')).toBeTruthy()
  })
})

describe('the gallery only draws what it needs to', () => {
  const many = (n: number) =>
    Array.from({ length: n }, (_, i) =>
      file({ name: `f${i}.txt`, path: `C:/x/output/s/f${i}.txt` }))

  it('holds the rest behind a button instead of dropping it', async () => {
    // Everything in range arrives in one reply — a file the page will not send
    // is a file the user cannot find — so the count in the button is the proof
    // that nothing was lost on the way in.
    vi.mocked(ListArtifactsIn).mockResolvedValue({ files: many(75), range: 'week', total: 75 } as any)
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('f0.txt')).toBeTruthy())
    expect(screen.queryByText('f70.txt')).toBeNull()

    // By its own text, not by the number: "15" also appears in a file name and
    // in the size on every card.
    const more = await screen.findByText(/แสดงเพิ่มอีก 15/)
    await fireEvent.click(more)
    await waitFor(() => expect(screen.getByText('f70.txt')).toBeTruthy())
  })

  it('shows the range the engine answered with, not the one it was asked for', async () => {
    // An empty week widens on the Go side. The picker follows what came back:
    // a control reading "สัปดาห์นี้" over a month of files is lying about what
    // is on screen.
    vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [file()], range: 'month', total: 1 } as any)
    render(Artifacts, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('สรุปยอด.xlsx')).toBeTruthy())
    const month = screen.getByText('เดือนนี้')
    expect(month.className).toContain('on')
    expect(screen.getByText('สัปดาห์นี้').className).not.toContain('on')
  })

  // A subfolder under a session is one thing, so it is one card. The reason it
  // matters: browser screenshots land in work/, and 46 of the 244 files in the
  // owner's gallery were those — nine in one session, with the document he had
  // actually asked for sitting as the tenth card in a row of lookalikes.
  describe('a folder under a session', () => {
    const shot = (n: number) => file({
      name: `page-${n}.png`,
      path: `C:/Users/x/aetox/output/20260805-090000.000/work/page-${n}.png`,
      folder: 'work', size: 1024,
    })

    it('draws one stack instead of one card per file', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({
        files: [file(), shot(1), shot(2), shot(3)], range: 'week', total: 4,
      } as any)
      render(Artifacts, { onClose: () => {} })

      // The deliverable keeps its own card; the three shots become one.
      await waitFor(() => expect(screen.getByText('สรุปยอด.xlsx')).toBeTruthy())
      expect(screen.getByText('ไฟล์ระหว่างทำงาน')).toBeTruthy()
      expect(screen.queryByText('page-1.png')).toBeNull()
      expect(screen.queryByText('page-3.png')).toBeNull()
    })

    it('spills the files when the stack is opened', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({
        files: [shot(1), shot(2)], range: 'week', total: 2,
      } as any)
      render(Artifacts, { onClose: () => {} })

      const deck = await screen.findByText('ไฟล์ระหว่างทำงาน')
      await fireEvent.click(deck)
      await waitFor(() => expect(screen.getByText('page-1.png')).toBeTruthy())
      expect(screen.getByText('page-2.png')).toBeTruthy()
    })

    // Two projects can each hold a work/ folder on the same day. They are two
    // piles, and a key made of the folder name alone would merge them.
    it('keeps the folders of two projects apart', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({
        files: [shot(1), file({
          name: 'page-9.png', path: 'D:/other/output/s2/work/page-9.png',
          folder: 'work', root: 'D:/other', sessionId: 's2',
        })],
        range: 'week', total: 2,
      } as any)
      render(Artifacts, { onClose: () => {} })

      await waitFor(() => expect(screen.getAllByText('ไฟล์ระหว่างทำงาน').length).toBe(2))
    })

    // Deleting the stack means deleting what is in it. One door per file, the
    // same door a single card uses, behind the same confirm step.
    it('deletes every file in the stack, and only after a confirm', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({
        files: [shot(1), shot(2)], range: 'week', total: 2,
      } as any)
      render(Artifacts, { onClose: () => {} })

      const del = await screen.findByLabelText('ลบไฟล์ในกองนี้ทั้งหมด')
      await fireEvent.click(del)
      expect(vi.mocked(DeleteArtifact)).not.toHaveBeenCalled()

      await fireEvent.click(screen.getByText('ยืนยัน?'))
      await waitFor(() => expect(vi.mocked(DeleteArtifact).mock.calls.length).toBe(2))
    })
  })

  // Clearing a gallery one confirm at a time is the thing this replaces
  // (owner: "บางทีจะไปเคลียร์หรือลบอ่ะลำบากมาก").
  describe('picking more than one', () => {
    const many = (n: number) => Array.from({ length: n }, (_, i) =>
      file({ name: `page-${i}.png`, path: `C:/Users/x/aetox/output/s1/page-${i}.png`, size: 1024 }))

    it('takes the whole screen in one press, then deletes it behind a confirm', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: many(4), range: 'week', total: 4 } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByText('เลือกทั้งหมด'))
      expect(screen.getByText('เลือกไว้ 4 ไฟล์')).toBeTruthy()

      await fireEvent.click(screen.getByText('ลบที่เลือก'))
      expect(vi.mocked(DeleteArtifact)).not.toHaveBeenCalled()

      await fireEvent.click(screen.getByText('ลบ 4 ไฟล์จริงไหม'))
      await waitFor(() => expect(vi.mocked(DeleteArtifact).mock.calls.length).toBe(4))
    })

    it('picks one card without opening it', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: many(3), range: 'week', total: 3 } as any)
      render(Artifacts, { onClose: () => {} })

      const ticks = await screen.findAllByLabelText('เลือกไฟล์นี้')
      await fireEvent.click(ticks[1])

      expect(screen.getByText('เลือกไว้ 1 ไฟล์')).toBeTruthy()
      // The card's own click still means "open this" — the tick is not a mode.
      expect(vi.mocked(OpenArtifact)).not.toHaveBeenCalled()
    })

    it('lets go of the selection again', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: many(2), range: 'week', total: 2 } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByText('เลือกทั้งหมด'))
      await fireEvent.click(screen.getByText('ยกเลิกที่เลือก'))
      await waitFor(() => expect(screen.getByText('เลือกทั้งหมด')).toBeTruthy())
      expect(screen.queryByText('ลบที่เลือก')).toBeNull()
    })

    // A folder card stands for what is in it, so picking it picks those.
    it('picking a folder picks every file in it', async () => {
      const shot = (n: number) => file({
        name: `page-${n}.png`, path: `C:/Users/x/aetox/output/s1/work/page-${n}.png`,
        folder: 'work', size: 1024,
      })
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [shot(1), shot(2), shot(3)], range: 'week', total: 3 } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByLabelText('เลือกไฟล์นี้'))
      expect(screen.getByText('เลือกไว้ 3 ไฟล์')).toBeTruthy()
    })

    // The number is the whole point of the button.
    it('says how much space compressing gave back', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: many(2), range: 'week', total: 2 } as any)
      vi.mocked(CompressArtifacts).mockResolvedValue(
        { files: 1, skipped: 0, before: 1_000_000, after: 200_000 } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByText('เลือกทั้งหมด'))
      await fireEvent.click(screen.getByText('บีบอัดรูป 2 ไฟล์'))

      await waitFor(() => expect(screen.getByText(/ได้พื้นที่คืน 1\.5 MB/)).toBeTruthy())
    })

    // One call per file is what lets the page count at all, and the count is
    // what tells the user it is working rather than wedged.
    it('goes one file at a time and counts up as it goes', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: many(3), range: 'week', total: 3 } as any)
      let release: (() => void) | null = null
      vi.mocked(CompressArtifacts).mockImplementation(async (paths: any) => {
        expect(paths.length).toBe(1) // never the whole list in one go
        if (release) { const go = release; release = null; await new Promise<void>((r) => { go(); r() }) }
        return { files: 1, skipped: 0, before: 500_000, after: 100_000 } as any
      })
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByText('เลือกทั้งหมด'))
      await fireEvent.click(screen.getByText('บีบอัดรูป 3 ไฟล์'))

      // Finishes having called once per file, and having shown a running total.
      await waitFor(() => expect(vi.mocked(CompressArtifacts).mock.calls.length).toBe(3))
      await waitFor(() => expect(screen.getByText(/บีบอัด 3 ไฟล์/)).toBeTruthy())
    })

    // A .md is not an image, so the compress button must not offer to squeeze it.
    it('does not offer to compress things that are not images', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [file()], range: 'week', total: 1 } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByText('เลือกทั้งหมด'))
      expect(screen.getByText('เลือกไว้ 1 ไฟล์')).toBeTruthy()
      expect(screen.queryByText(/บีบอัดรูป/)).toBeNull()
    })
  })

  // "Select all" that can also mean less than all of it.
  describe('picking by time', () => {
    const at = (daysBack: number, name: string) => {
      const d = new Date()
      d.setHours(12, 0, 0, 0)
      d.setDate(d.getDate() - daysBack)
      return file({ name, path: `C:/Users/x/aetox/output/s1/${name}`, modified: d.toISOString() })
    }

    it('takes only yesterday when yesterday is what was asked', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({
        files: [at(0, 'a.png'), at(1, 'b.png'), at(1, 'c.png'), at(5, 'd.png')],
        range: 'week', total: 4,
      } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByLabelText('เลือกตามช่วงเวลา'))
      await fireEvent.click(screen.getByRole('menuitem', { name: 'เมื่อวาน' }))

      await waitFor(() => expect(screen.getByText('เลือกไว้ 2 ไฟล์')).toBeTruthy())
    })

    it('counts today separately from the last seven days', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({
        files: [at(0, 'a.png'), at(1, 'b.png'), at(6, 'c.png')], range: 'week', total: 3,
      } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByLabelText('เลือกตามช่วงเวลา'))
      await fireEvent.click(screen.getByRole('menuitem', { name: 'วันนี้' }))
      await waitFor(() => expect(screen.getByText('เลือกไว้ 1 ไฟล์')).toBeTruthy())
    })

    // A span wider than what is loaded has to widen the range first, or it
    // selects nothing and says nothing about why.
    it('loads a wider range before selecting a wider span', async () => {
      vi.mocked(ListArtifactsIn).mockImplementation(async (want: any) => (
        want === 'all'
          ? { files: [at(0, 'a.png'), at(200, 'old.png')], range: 'all', total: 2 }
          : { files: [at(0, 'a.png')], range: 'week', total: 1 }
      ) as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByLabelText('เลือกตามช่วงเวลา'))
      await fireEvent.click(screen.getByRole('menuitem', { name: 'ปีนี้' }))

      await waitFor(() => expect(screen.getByText('เลือกไว้ 2 ไฟล์')).toBeTruthy())
      expect(vi.mocked(ListArtifactsIn).mock.calls.map((c) => c[0])).toContain('all')
    })

    it('closes the menu with Escape', async () => {
      vi.mocked(ListArtifactsIn).mockResolvedValue({ files: [at(0, 'a.png')], range: 'week', total: 1 } as any)
      render(Artifacts, { onClose: () => {} })

      await fireEvent.click(await screen.findByLabelText('เลือกตามช่วงเวลา'))
      expect(screen.getByRole('menuitem', { name: 'เมื่อวาน' })).toBeTruthy()
      await fireEvent.keyDown(document, { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('menuitem', { name: 'เมื่อวาน' })).toBeNull())
    })
  })
})
