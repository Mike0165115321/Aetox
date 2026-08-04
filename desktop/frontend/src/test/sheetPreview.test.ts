// A .xlsx opens as a grid, not as the "this file can't be shown" card. The
// spreadsheet is the one produced format worth previewing — a table is exactly
// what a pane can draw — and it is also the flagship promise on the site, so
// the routing decision is worth pinning.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import { workbench, openFileTab } from '../lib/stores/workbench.svelte'
import { ReadWorkbook, ReadFile, OpenFileExternally } from './mocks/wailsApp'
import SheetPane from '../lib/workbench/SheetPane.svelte'

const preview = {
  sheets: [{
    name: 'สรุปสลิป',
    rows: [
      ['เลขที่', 'ร้าน', 'ยอด'],
      ['0012', 'ร้านกาแฟ', '185.5'],
      ['0013', 'ค่าไฟ', '1240'],
    ],
    truncated: false,
    totalRows: 3,
  }],
}

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
})

describe('opening a workbook', () => {
  it('loads it as a grid instead of trying to read it as text', async () => {
    ReadWorkbook.mockResolvedValueOnce(preview as any)

    await openFileTab('out/สรุปสลิป.xlsx')

    const tab = workbench.tabs[0]
    expect(tab.sheet).toEqual(preview)
    expect(tab.unreadable).toBeUndefined()
    // Reading a ZIP as text is what produced the error-message-as-file-content
    // this whole path replaced.
    expect(ReadFile).not.toHaveBeenCalled()
  })

  it('falls back to the open-externally card when the workbook cannot be read', async () => {
    ReadWorkbook.mockRejectedValueOnce(new Error('workbook too large to preview'))

    await openFileTab('huge.xlsx')

    const tab = workbench.tabs[0]
    expect(tab.sheet).toBeUndefined()
    // Still a workbook Excel can open, so the failure must not be a dead end.
    expect(tab.unreadable).toContain('workbook too large')
  })

  it('leaves other files on the text path', async () => {
    ReadFile.mockResolvedValueOnce('# notes')

    await openFileTab('notes.md')

    expect(ReadWorkbook).not.toHaveBeenCalled()
    expect(workbench.tabs[0].content).toBe('# notes')
  })

  // Routing a deck through the text path made the card blame whichever gate
  // fired first — "file too large to preview" for a 1.5MB pptx — when the real
  // reason is that slides are never previewed at any size. The card must tell
  // the truth, and neither reader should even be asked.
  it('routes decks and documents straight to the open-externally card', async () => {
    for (const name of ['xiaomi-sales-deck.pptx', 'สัญญาเช่า.docx']) {
      await openFileTab(name)
      const tab = workbench.tabs.find((t) => t.id === `file-${name}`)!
      expect(tab.unreadable).toBeTruthy()
      expect(tab.unreadable).not.toContain('too large')
      expect(tab.content).toBeUndefined()
    }
    expect(ReadFile).not.toHaveBeenCalled()
    expect(ReadWorkbook).not.toHaveBeenCalled()
  })
})

describe('the grid', () => {
  it('draws the rows and keeps the way out to the real app', async () => {
    render(SheetPane, { path: 'out/book.xlsx', preview: preview as any })

    expect(screen.getByText('ร้านกาแฟ')).toBeTruthy()
    expect(screen.getByText('185.5')).toBeTruthy()
    // The row numbers are Excel's own, so they must start at 1 — the header row
    // IS row 1. Blank there made the column read as starting at 2.
    expect(screen.getByRole('columnheader', { name: '1' })).toBeTruthy()
    // The preview is a glance, never a replacement — the button that hands the
    // file to Excel is part of the pane, not somewhere else.
    const open = screen.getByRole('button', { name: /Open with|เปิดด้วย/ })
    open.click()
    await Promise.resolve()
    expect(OpenFileExternally).toHaveBeenCalledWith('out/book.xlsx')
  })

  it('says when it is showing only part of a sheet', async () => {
    render(SheetPane, {
      path: 'big.xlsx',
      preview: { sheets: [{ ...preview.sheets[0], truncated: true, totalRows: 9000 }] } as any,
    })
    // Displaying a prefix as if it were the whole sheet is the one failure here
    // that misleads rather than annoys.
    expect(screen.getByText(/9000/)).toBeTruthy()
  })
})
