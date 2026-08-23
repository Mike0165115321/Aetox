// ห้องสไลด์ — ที่ทางของเด็คโดยเฉพาะ
//
// คำถามที่ห้องนี้ตอบคือ "งานนำเสนอที่ทำไว้มีอะไรบ้าง" ซึ่งถามตอนยังไม่รู้ว่าจะ
// เปิดไฟล์ไหน แพเนลไฟล์จึงตอบแทนไม่ได้ เทสต์ตรงนี้กันสามอย่างที่ทำให้ห้องโกหก:
// รายการที่ว่างเปล่าโดยไม่บอกว่าต้องทำยังไงต่อ, ห้องที่เปิดมาแล้วไม่เลือกอะไรให้
// ทั้งที่มีเด็คอยู่, และปุ่มส่งออกที่ล้มแล้วเงียบ
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import DeckRoom from '../lib/workbench/DeckRoom.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { ListDecksIn, ReadFile, ExportDeck, OpenExport } from './mocks/wailsApp'

const deckHTML = '<html><body><section class="slide"><h1>ยอดขาย</h1></section></body></html>'

const rows = [
  { path: 'output/s2/ใหม่.html', name: 'ใหม่.html', slides: 8, sessionId: 's2', modified: '2026-08-19T02:00:00Z' },
  { path: 'output/s1/เก่า.html', name: 'เก่า.html', slides: 3, sessionId: 's1', modified: '2026-08-18T02:00:00Z' },
]

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  vi.mocked(ListDecksIn).mockResolvedValue({ decks: rows, range: 'week', total: rows.length } as any)
  vi.mocked(ReadFile).mockResolvedValue(deckHTML as any)
})

describe('the slides room', () => {
  it('lists every deck with its slide count', async () => {
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('ใหม่.html')).toBeTruthy())
    expect(screen.getByText('เก่า.html')).toBeTruthy()
    expect(screen.getByText(/8 slides/)).toBeTruthy()
    expect(screen.getByText(/3 slides/)).toBeTruthy()
  })

  // เปิดห้องมาแล้วต้องเห็นเด็คเลย ไม่ใช่ต้องคลิกอีกทีเพื่อดูสิ่งที่ตั้งใจมาดู
  // ตัวแรกคือตัวใหม่สุด เพราะเด็คที่เพิ่งทำเสร็จคือเด็คที่คนกำลังจะดู
  it('opens on the newest deck without a click', async () => {
    render(DeckRoom)
    await waitFor(() => expect(vi.mocked(ReadFile).mock.calls.length).toBeGreaterThan(0))
    expect(vi.mocked(ReadFile).mock.calls[0][0]).toBe('output/s2/ใหม่.html')
  })

  it('reads the deck the user picks', async () => {
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('เก่า.html')).toBeTruthy())

    await fireEvent.click(screen.getByText('เก่า.html'))
    await waitFor(() =>
      expect(vi.mocked(ReadFile).mock.calls.some((c) => c[0] === 'output/s1/เก่า.html')).toBe(true),
    )
  })

  // ห้องนี้โหลดใหม่ทุกครั้งที่เทิร์นจบ และแถวหนึ่งแถวคือการอ่านไฟล์ทั้งไฟล์
  // แล้วแจงเป็น HTML การเปิดมาที่ "สัปดาห์นี้" จึงไม่ใช่รสนิยม แต่คือเพดาน
  it('opens on this week rather than on everything', async () => {
    render(DeckRoom)
    await waitFor(() => expect(vi.mocked(ListDecksIn).mock.calls.length).toBeGreaterThan(0))
    expect(vi.mocked(ListDecksIn).mock.calls[0][0]).toBe('week')
  })

  // ปุ่มช่วงเวลาผูกกับช่วงที่ฝั่งโกตอบมา ไม่ใช่ช่วงที่กด สัปดาห์ที่ว่างถูกขยาย
  // ให้เองฝั่งโน้น ปุ่มที่ยังค้างอยู่ที่ "สัปดาห์นี้" ทั้งที่จอแสดงทั้งหมด คือปุ่มโกหก
  it('the range control says what is on screen, not what was clicked', async () => {
    vi.mocked(ListDecksIn).mockResolvedValue({ decks: rows, range: 'all', total: rows.length } as any)
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('ใหม่.html')).toBeTruthy())
    expect(screen.getByRole('button', { name: 'All' }).className).toContain('on')
    expect(screen.getByRole('button', { name: 'This week' }).className).not.toContain('on')
  })

  it('asks Go for the range the user picks', async () => {
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('ใหม่.html')).toBeTruthy())
    await fireEvent.click(screen.getByRole('button', { name: 'This month' }))
    await waitFor(() =>
      expect(vi.mocked(ListDecksIn).mock.calls.some((c) => c[0] === 'month')).toBe(true),
    )
  })

  // หัววันมาจาก dayBucket ตัวเดียวกับประวัติแชทและหน้าผลงาน หัวจะโผล่เฉพาะตอน
  // วันเปลี่ยน รายการจึงอ่านเป็นไทม์ไลน์
  it('groups the rows under day headings', async () => {
    const today = new Date().toISOString()
    const yesterday = new Date(Date.now() - 86_400_000).toISOString()
    vi.mocked(ListDecksIn).mockResolvedValue({
      decks: [
        { path: 'a.html', name: 'a.html', slides: 2, modified: today },
        { path: 'b.html', name: 'b.html', slides: 2, modified: today },
        { path: 'c.html', name: 'c.html', slides: 2, modified: yesterday },
      ],
      range: 'week',
      total: 3,
    } as any)
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('c.html')).toBeTruthy())
    expect(screen.getAllByText('Today')).toHaveLength(1)
    expect(screen.getAllByText('Yesterday')).toHaveLength(1)
  })

  // ทุกแถวในช่วงมาถึงแล้ว ปุ่มนี้คุมแค่ว่าวาดกี่แถว และต้องบอกจำนวนที่ซ่อนอยู่
  // "แสดงเพิ่ม" เฉย ๆ ไม่บอกว่าเหลือสี่แถวหรือสี่ร้อย
  it('draws a screenful and says how many it is holding back', async () => {
    const many = Array.from({ length: 34 }, (_, i) => ({
      path: `d${i}.html`, name: `d${i}.html`, slides: 2, modified: '2026-08-19T02:00:00Z',
    }))
    vi.mocked(ListDecksIn).mockResolvedValue({ decks: many, range: 'all', total: 34 } as any)
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('d0.html')).toBeTruthy())
    expect(screen.queryByText('d33.html')).toBeNull()

    await fireEvent.click(screen.getByText(/Show 4 more/))
    await waitFor(() => expect(screen.getByText('d33.html')).toBeTruthy())
  })

  // ห้องว่างที่บอกแค่ว่าว่าง อ่านเหมือนฟีเจอร์พัง ต้องบอกด้วยว่าทำยังไงถึงจะมี
  it('an empty room says how to fill it', async () => {
    vi.mocked(ListDecksIn).mockResolvedValue({ decks: [], range: 'all', total: 0 } as any)
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText(/Ask the agent for a presentation/i)).toBeTruthy())
    expect(vi.mocked(ReadFile)).not.toHaveBeenCalled()
  })

  // เมนูมาจากฝั่ง Go ไม่ใช่รายการที่แพเนลเก็บเอง แถวที่ยังไม่พร้อมต้องเห็นแต่
  // กดไม่ได้ เพราะแถวที่หายไปทำให้ไม่รู้ว่ามีอะไรกำลังมา และแถวที่ดูกดได้แล้ว
  // ปฏิเสธคือคำโกหกที่ต้องกดถึงจะรู้
  it('offers the formats Go says it can write, and greys out the rest', async () => {
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('Export')).toBeTruthy())

    await fireEvent.click(screen.getByText('Export'))

    const pptx = await screen.findByRole('menuitem', { name: /\.pptx/ })
    const pdf = screen.getByRole('menuitem', { name: /\.pdf/ })
    expect(pptx.hasAttribute('disabled')).toBe(false)
    expect(pdf.hasAttribute('disabled')).toBe(true)
    expect(screen.getAllByText(/not ready yet/).length).toBe(2) // .pdf and .png
  })

  it('shows where an export landed, and offers to open it', async () => {
    vi.mocked(ExportDeck).mockResolvedValue('C:/Users/me/Downloads/ใหม่.pptx' as any)
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('Export')).toBeTruthy())

    await fireEvent.click(screen.getByText('Export'))
    await fireEvent.click(await screen.findByRole('menuitem', { name: /\.pptx/ }))

    // แถบแจ้งเตือนโชว์ชื่อไฟล์ ไม่ใช่พาธเต็ม — พาธเต็มยาวจนโดนตัดตรงชื่อไฟล์
    // ซึ่งเป็นส่วนเดียวที่คนอยากรู้
    await waitFor(() => expect(screen.getByText('ใหม่.pptx')).toBeTruthy())
    expect(screen.queryByText(/C:\/Users/)).toBeNull()
    expect(vi.mocked(ExportDeck).mock.calls[0]).toEqual(['output/s2/ใหม่.html', 'pptx'])

    // OpenExport rather than OpenFileExternally: Downloads is outside the
    // project, and the sandbox-checked opener refuses it by design.
    await fireEvent.click(screen.getByText(/Open it/))
    await waitFor(() =>
      expect(vi.mocked(OpenExport).mock.calls[0][0]).toBe('C:/Users/me/Downloads/ใหม่.pptx'),
    )
  })

  // ล้มแล้วต้องพูด เพราะสิ่งที่ผู้ใช้เห็นตอนล้มเงียบคือปุ่มที่กดแล้วไม่เกิดอะไร
  // ซึ่งอ่านเหมือนแอปค้าง ไม่ใช่เหมือนไฟล์มีปัญหา
  it('says why an export failed instead of going quiet', async () => {
    vi.mocked(ExportDeck).mockRejectedValue(new Error('no slides here'))
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('Export')).toBeTruthy())

    await fireEvent.click(screen.getByText('Export'))
    await fireEvent.click(await screen.findByRole('menuitem', { name: /\.pptx/ }))

    await waitFor(() => expect(screen.getByText(/no slides here/)).toBeTruthy())
    expect(screen.queryByText(/Saved to/)).toBeNull()
  })

  // ปุ่มเปิดด้วยโปรแกรมในเครื่องถูกเอาออกตามที่สั่ง เทสต์นี้กันไม่ให้มันกลับมา
  // โดยไม่ตั้งใจตอนแก้หัวแพเนลรอบหน้า ส่วนลิงก์ "เปิดไฟล์" หลังส่งออกคนละตัวกัน
  // และยังอยู่ เพราะมันเปิดไฟล์ที่เพิ่งสร้าง ไม่ใช่เปิดเด็ค
  it('has no open-with-my-computer button on the deck itself', async () => {
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText('Export')).toBeTruthy())
    expect(screen.queryByText(/Open with my computer/i)).toBeNull()
  })

  it('survives a listing that fails without going blank', async () => {
    vi.mocked(ListDecksIn).mockRejectedValue(new Error('no project open'))
    render(DeckRoom)
    await waitFor(() => expect(screen.getByText(/no project open/)).toBeTruthy())
  })

  // ปุ่มบนแถบเด็คที่มีแต่ไอคอน — ลูกศรเดินสไลด์ ปุ่มชี้ และปุ่มวาด — ไม่มีคำอยู่
  // บนตัวมันเลย ชื่อจึงต้องอยู่ที่ aria-label กับ title ปุ่มไอคอนที่ไม่มีชื่อคือ
  // ปุ่มที่ต้องกดถึงจะรู้ว่ามันทำอะไร
  it('names every icon-only button on the deck toolbar', async () => {
    const { container } = render(DeckRoom)
    await waitFor(() => expect(screen.getByText('Export')).toBeTruthy())

    const head = container.querySelector('.deck-head')
    expect(head).toBeTruthy()
    const iconOnly = Array.from(head!.querySelectorAll('button')).filter(
      (b) => (b.textContent ?? '').trim() === '',
    )
    expect(iconOnly.length).toBeGreaterThanOrEqual(4)
    for (const b of iconOnly) {
      expect(b.getAttribute('aria-label')?.trim() || '').not.toBe('')
      expect(b.getAttribute('title')?.trim() || '').not.toBe('')
    }
  })
})
