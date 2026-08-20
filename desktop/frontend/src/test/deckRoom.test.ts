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
import { ListDecks, ReadFile, ExportDeck, OpenExport } from './mocks/wailsApp'

const deckHTML = '<html><body><section class="slide"><h1>ยอดขาย</h1></section></body></html>'

const rows = [
  { path: 'output/s2/ใหม่.html', name: 'ใหม่.html', slides: 8, sessionId: 's2', modified: '2026-08-19T02:00:00Z' },
  { path: 'output/s1/เก่า.html', name: 'เก่า.html', slides: 3, sessionId: 's1', modified: '2026-08-18T02:00:00Z' },
]

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  vi.mocked(ListDecks).mockResolvedValue(rows as any)
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

  // ห้องว่างที่บอกแค่ว่าว่าง อ่านเหมือนฟีเจอร์พัง ต้องบอกด้วยว่าทำยังไงถึงจะมี
  it('an empty room says how to fill it', async () => {
    vi.mocked(ListDecks).mockResolvedValue([] as any)
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
    vi.mocked(ListDecks).mockRejectedValue(new Error('no project open'))
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
