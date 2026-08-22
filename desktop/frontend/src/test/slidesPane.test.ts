// ห้องสไลด์ กับเด็คสองทรง
//
// `deckNav.test.ts` ข้าง ๆ กันพิสูจน์กฎทีละข้อ ไฟล์นี้พิสูจน์การต่อสาย: แพเนลอ่าน
// เอกสารในไอเฟรมตอนโหลด ตัดสินว่าเด็คเป็นทรงไหน แล้วปุ่มของ *ห้อง* พาไปทีละใบจริง
//
// อาการที่เจ้าของรายงาน 2026-08-20 อยู่ตรงรอยต่อนี้พอดี ไม่ใช่ในกฎ: กฎทุกข้อถูก
// อยู่แล้ว แต่แพเนลรู้จักเด็คทรงเดียว เลยเรียก scrollIntoView ใส่เอกสารที่เลื่อน
// ไม่ได้ แล้วเงียบ เทสต์ที่ทดสอบแต่ฟังก์ชันจะผ่านหมดทั้งที่ปุ่มตาย
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import SlidesPane from '../lib/workbench/SlidesPane.svelte'
import { setLocale } from '../lib/i18n.svelte'

// เด็คทรงซ้อน: ทุกใบวางทับกัน โชว์ทีละใบ แล้วเด็คสลับเอง — ทรงที่โมเดลเขียนออกมา
// เองแทบทุกครั้ง และทรงที่ทำให้ปุ่มของห้องตาย
const stacked = `<!DOCTYPE html><html><body>
  <section class="slide" style="visibility:visible" id="s1">หนึ่ง</section>
  <section class="slide" style="visibility:hidden" id="s2">สอง</section>
  <section class="slide" style="visibility:hidden" id="s3">สาม</section>
  <script>
    var at = 0, all = document.querySelectorAll('.slide');
    function show(i){ all.forEach(function(s,n){ s.style.visibility = n===i ? 'visible':'hidden' }); at = i }
    document.addEventListener('keydown', function(e){
      if (e.key === 'ArrowRight') show((at+1) % all.length);
      if (e.key === 'ArrowLeft')  show((at-1+all.length) % all.length);
    });
  <\/script>
</body></html>`

/** jsdom ไม่โหลด src ของไอเฟรม (มันชี้ไปที่ file host ของฝั่ง Go) เอกสารจึงถูก
 *  เขียนลง contentDocument ตรง ๆ แล้วยิง load เอง ซึ่งเป็นลำดับเดียวกับของจริง:
 *  เอกสารพร้อมก่อน แล้ว scan() จึงทำงาน */
async function loadDeck(html: string) {
  const frame = document.querySelector('iframe') as HTMLIFrameElement
  const doc = frame.contentDocument!
  doc.open()
  doc.write(html)
  doc.close()
  await fireEvent.load(frame)
  return doc
}

const visible = (doc: Document) =>
  [...doc.querySelectorAll<HTMLElement>('section.slide')].findIndex(
    (s) => doc.defaultView!.getComputedStyle(s).visibility !== 'hidden',
  )

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})

describe('the slides room', () => {
  it('drives a deck that navigates itself, from the room’s own arrows', async () => {
    render(SlidesPane, { path: 'output/s1/deck.html', name: 'deck.html', content: stacked })
    const doc = await loadDeck(stacked)

    // ตัวนับของห้องต้องเห็นสามใบ ไม่ใช่ศูนย์
    await waitFor(() => expect(screen.getByText('1 / 3')).toBeTruthy())
    expect(visible(doc)).toBe(0)

    await fireEvent.click(screen.getByLabelText('Next slide'))
    expect(visible(doc)).toBe(1)
    await waitFor(() => expect(screen.getByText('2 / 3')).toBeTruthy())

    await fireEvent.click(screen.getByLabelText('Next slide'))
    expect(visible(doc)).toBe(2)

    await fireEvent.click(screen.getByLabelText('Previous slide'))
    expect(visible(doc)).toBe(1)
    await waitFor(() => expect(screen.getByText('2 / 3')).toBeTruthy())
  })

  // isDeck รับ div มาตั้งแต่ §154 ไฟล์ที่เป็น div ล้วนจึงมาถึงแพเนลนี้ได้ ก่อนหน้านี้
  // มันนับได้ศูนย์ใบ เปิดมาเป็นห้องว่างที่มีปุ่มกดไม่ได้
  it('counts a div-based deck instead of opening an empty room', async () => {
    const divDeck = `<!DOCTYPE html><html><body>
      <div class="slide">หนึ่ง</div><div class="slide">สอง</div></body></html>`
    render(SlidesPane, { path: 'output/s1/talk.html', name: 'talk.html', content: divDeck })
    await loadDeck(divDeck)

    await waitFor(() => expect(screen.getByText('1 / 2')).toBeTruthy())
  })

  // เด็คที่เรียงเป็นสาย (ทรงของเด็คมาตรฐาน 4 ส.ค.) ต้องไม่ถูกลากไปเดินแบบทรงซ้อน
  // การเลื่อนคือทางที่ถูกของมัน และมันต้องเป็นการเลื่อน "เอกสารในไอเฟรม" เท่านั้น
  it('leaves a flowing deck to be scrolled, not stepped', async () => {
    const flowing = `<!DOCTYPE html><html><body>
      <section class="slide">หนึ่ง</section><section class="slide">สอง</section></body></html>`
    render(SlidesPane, { path: 'output/s1/flow.html', name: 'flow.html', content: flowing })
    const doc = await loadDeck(flowing)
    // เอกสารที่เลื่อนได้ — jsdom ตอบศูนย์เสมอ ต้องบอกเอง
    Object.defineProperty(doc.documentElement, 'scrollHeight', { value: 1440, configurable: true })
    Object.defineProperty(doc.documentElement, 'clientHeight', { value: 720, configurable: true })
    await fireEvent.load(document.querySelector('iframe') as HTMLIFrameElement)

    const slides = [...doc.querySelectorAll('section.slide')] as HTMLElement[]
    slides[1].getBoundingClientRect = () => ({ top: 720, height: 720 }) as DOMRect

    await waitFor(() => expect(screen.getByText('1 / 2')).toBeTruthy())
    await fireEvent.click(screen.getByLabelText('Next slide'))

    await waitFor(() => expect(screen.getByText('2 / 2')).toBeTruthy())
    // ใบที่สองอยู่ต่ำลงไปหนึ่งจอ เอกสารในไอเฟรมจึงต้องถูกเลื่อนลงไปหามัน
    expect(doc.documentElement.scrollTop).toBeGreaterThan(0)
  })

  // อาการที่เจ้าของรายงาน 2026-08-20: "กดสไลด์ขึ้นมาแล้ว พอโหลดสไลด์แล้วมันดัน"
  // แถบซ้ายของแอปถูกดันออกนอกจอ และขวาก็โดนด้วย
  //
  // ต้นเหตุคือ scrollIntoView: สเปกบอกให้มันเดินขึ้นไปเลื่อน "ทุกกล่องที่เลื่อน
  // ได้" เหนือ element นั้น และเอกสารในไอเฟรมนี้เป็น same-origin กับหน้าต่างแอป
  // มันจึงไม่หยุดที่ขอบไอเฟรม แต่ไปเลื่อนกริดของทั้งแอปต่อ
  //
  // เทสต์นี้ผูกกับกติกา ไม่ใช่กับอาการ: แพเนลห้ามเรียก scrollIntoView ใส่อะไรที่
  // อยู่ในไอเฟรมเลย ไม่ว่าจะตอนโหลดหรือตอนกดปุ่ม เพราะการเลื่อนที่ทะลุออกไปนอก
  // กรอบตัวเองได้ คือความสามารถที่แพเนลนี้ไม่ควรมีตั้งแต่แรก
  it('never scrolls anything outside its own iframe', async () => {
    const flowing = `<!DOCTYPE html><html><body>
      <section class="slide">หนึ่ง</section><section class="slide">สอง</section></body></html>`
    render(SlidesPane, { path: 'output/s1/flow.html', name: 'flow.html', content: flowing })
    const doc = await loadDeck(flowing)
    Object.defineProperty(doc.documentElement, 'scrollHeight', { value: 1440, configurable: true })
    Object.defineProperty(doc.documentElement, 'clientHeight', { value: 720, configurable: true })

    const reached: string[] = []
    for (const el of doc.querySelectorAll('section.slide')) {
      ;(el as HTMLElement).scrollIntoView = () => reached.push('slide')
    }

    // ทั้งสองทาง: ตอนไอเฟรมโหลดเสร็จ (fit) และตอนกดไปใบถัดไป (goto)
    await fireEvent.load(document.querySelector('iframe') as HTMLIFrameElement)
    await waitFor(() => expect(screen.getByText('1 / 2')).toBeTruthy())
    await fireEvent.click(screen.getByLabelText('Next slide'))
    await waitFor(() => expect(screen.getByText('2 / 2')).toBeTruthy())

    expect(reached).toEqual([])
  })

  // อาการที่เจ้าของรายงาน 2026-08-22: เปิดแถบพรีวิวค้างไว้ที่ห้องสไลด์ (หรือเปิด
  // เบราว์เซอร์ทับไว้ ซึ่งไม่ถอดห้องสไลด์ออกจากหน้าจอ มันแค่ถูกซ่อน) แล้วพิมพ์ใน
  // ช่องพร้อมต์ กด Space ไม่เว้นวรรคเลยสักครั้ง
  //
  // ห้องนี้ฟังปุ่มที่ "หน้าต่าง" ไม่ใช่ที่ตัวมันเอง ปุ่มที่คนกดลงในช่องพิมพ์จึงลอย
  // ขึ้นมาถึงมัน แล้วโดน preventDefault ไปเดินสไลด์แทน — คีย์บอร์ดของทั้งแอปถูก
  // ห้องเดียวยึดไป
  it('leaves the prompt box alone: a space typed there is still a space', async () => {
    render(SlidesPane, { path: 'output/s1/deck.html', name: 'deck.html', content: stacked })
    await loadDeck(stacked)
    await waitFor(() => expect(screen.getByText('1 / 3')).toBeTruthy())

    // ช่องพิมพ์ของแชต อยู่คนละที่กับห้องสไลด์ แต่อยู่ในหน้าต่างเดียวกัน
    const box = document.createElement('textarea')
    document.body.appendChild(box)
    box.focus()

    for (const key of [' ', 'ArrowRight', 'ArrowLeft', 'Home', 'End']) {
      const e = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
      box.dispatchEvent(e)
      expect(`${key}: ${e.defaultPrevented}`).toBe(`${key}: false`)
    }
    // และไม่มีใบไหนถูกเดินไปเพราะการพิมพ์
    expect(screen.getByText('1 / 3')).toBeTruthy()
  })

  // ห้องที่ถูกซ่อนอยู่ก็ยังฟังหน้าต่างอยู่ เพราะโต๊ะเก็บทุกแท็บค้างไว้แล้วซ่อนที่ไม่ได้ใช้
  // (เทอร์มินัลกับเบราว์เซอร์ถูกถอดไม่ได้) กดลูกศรตอนดูเบราว์เซอร์จึงเคยเดินสไลด์ที่มองไม่เห็นให้
  it('stays out of the keyboard while its tab is hidden', async () => {
    render(SlidesPane, {
      path: 'output/s1/deck.html', name: 'deck.html', content: stacked, active: false,
    })
    const doc = await loadDeck(stacked)
    await waitFor(() => expect(screen.getByText('1 / 3')).toBeTruthy())

    for (const key of ['ArrowRight', ' ', 'End']) {
      const e = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
      document.body.dispatchEvent(e)
      expect(`${key}: ${e.defaultPrevented}`).toBe(`${key}: false`)
    }
    expect(visible(doc)).toBe(0)
    expect(screen.getByText('1 / 3')).toBeTruthy()
  })
})
