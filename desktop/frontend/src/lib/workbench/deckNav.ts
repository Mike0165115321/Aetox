// การเดินเด็ค — กฎล้วน ๆ แยกออกจากแพเนล
//
// อยู่คนละไฟล์กับ SlidesPane.svelte ด้วยเหตุผลเดียวกับที่ `isDeck` อยู่ในสโตร์
// แทนที่จะอยู่ในคอมโพเนนต์: มันเป็นคำถามเกี่ยวกับเอกสาร ไม่ใช่เกี่ยวกับหน้าจอ จึง
// ตอบได้โดยไม่ต้องมีไอเฟรม ไม่ต้องมีเว็บวิว และเทสต์ได้ตรง ๆ
//
// ทั้งสามข้อข้างล่างมีที่มาจากอาการเดียวกันที่เจ้าของเจอเมื่อ 2026-08-20: เปิดเด็ค
// ที่โมเดลเพิ่งเขียน แล้วปุ่มลูกศรของแพเนลกดไม่ไปไหนเลย

/** สไลด์ทุกใบในเอกสาร ตามกฎเดียวกับ internal/deck
 *
 * `<section class="slide">` คือสัญญาและชนะเสมอเมื่อเอกสารมีมัน ส่วน
 * `<div class="slide">` เป็นทางถอยสำหรับไฟล์ที่ไม่มี section เลย ซึ่งเป็นทรงที่
 * เทมเพลตงานนำเสนอข้างนอกเขียนกันมา — ในเอกสารที่ *มี* section อยู่แล้ว div ที่
 * ชื่อคลาสเดียวกันยังเป็นการจัดสไตล์ของใครบางคน ไม่ใช่เส้นแบ่งสไลด์
 *
 * `isDeck` รับ div มาตั้งแต่ §154 แต่แพเนลยังถามหาแต่ section อยู่ ไฟล์ที่เป็น
 * div ล้วนจึงถูกส่งไปห้องสไลด์แล้วนับได้ศูนย์ใบ กลายเป็นห้องว่างที่มีปุ่มกดไม่ได้
 * คำถามเดียวถูกตอบสองที่ ซึ่งเป็นหนี้ที่โปรเจกต์นี้ตั้งใจไม่ให้มี */
export function slideElements(doc: Document): HTMLElement[] {
  const sections = Array.from(doc.querySelectorAll<HTMLElement>('section.slide'))
  if (sections.length > 0) return sections
  return Array.from(doc.querySelectorAll<HTMLElement>('div.slide'))
}

/** เอกสารนี้เลื่อนได้ไหม — คำถามที่แยกเด็คสองทรงออกจากกัน
 *
 * ทรงแรกสไลด์เรียงกันลงมาเป็นสาย แพเนลเลื่อนไปหาทีละใบ ทรงที่สองสไลด์ซ้อนกันอยู่
 * ที่เดียว โชว์ทีละใบ แล้วเด็คสลับเอง — ทรงที่ reveal.js ใช้ และเป็นทรงที่โมเดล
 * เขียนออกมาเองแทบทุกครั้ง
 *
 * วัดเอา ไม่ใช่เดาจากชื่อคลาสหรือจากไลบรารีที่เด็คอาจใช้: เด็คทรงซ้อนไม่มีอะไรให้
 * เลื่อน ซึ่งเป็นข้อเท็จจริงที่อ่านออกจากเอกสารตรง ๆ */
export function documentScrolls(doc: Document): boolean {
  const el = doc.documentElement
  if (!el) return true
  return el.scrollHeight > el.clientHeight + 4
}

/** ใบที่มองเห็นอยู่จริง
 *
 * อ่านจากสไตล์ที่คำนวณแล้ว ไม่ใช่จากชื่อคลาส เพราะ `.active` เป็นธรรมเนียม ไม่ใช่
 * สัญญา เด็คที่ซ่อนด้วย `display`, `visibility` หรือ `opacity` อ่านได้ด้วยกฎเดียว
 * กันหมด และเด็คที่ยังไม่มีใครแตะ (ทุกใบมองเห็น) ตอบใบแรก ซึ่งถูกต้องสำหรับทรง
 * เรียงเป็นสายด้วย
 *
 * fallback คือคำตอบเมื่ออ่านอะไรไม่ได้เลย — ค่าที่แพเนลถืออยู่ ดีกว่าลากตัวนับ
 * กลับไปที่ใบแรกทุกครั้งที่การอ่านล้มเหลว */
export function visibleIndex(slides: HTMLElement[], view: Window, fallback: number): number {
  let best = -1
  let strongest = -1
  for (let i = 0; i < slides.length; i++) {
    const cs = view.getComputedStyle(slides[i])
    if (cs.display === 'none' || cs.visibility === 'hidden') continue
    const alpha = Number(cs.opacity)
    const weight = Number.isNaN(alpha) || cs.opacity === '' ? 1 : alpha
    if (weight > strongest) {
      strongest = weight
      best = i
    }
  }
  return best === -1 ? fallback : best
}

/** ปุ่มนี้ถูกกดลงในที่ที่คนกำลังพิมพ์อยู่หรือเปล่า
 *
 * ห้องสไลด์ฟังปุ่มที่ *หน้าต่าง* ไม่ใช่ที่ตัวมันเอง เพราะเด็คควรเดินได้โดยไม่ต้อง
 * คลิกที่ตัวสไลด์ก่อน แต่หน้าต่างเดียวกันนั้นมีช่องพร้อมต์ของแชตอยู่ด้วย ปุ่มที่คน
 * กดลงในช่องพิมพ์จึงลอยขึ้นมาถึงห้องสไลด์ แล้ว Space ก็ถูก preventDefault ไปเดิน
 * สไลด์แทนที่จะเว้นวรรค — อาการที่เจ้าของรายงาน 2026-08-22 (เห็นตอนเปิดเบราว์เซอร์
 * ทับด้วย เพราะแท็บที่ไม่ได้ใช้ถูกซ่อน ไม่ได้ถูกถอด ห้องสไลด์จึงยังฟังอยู่)
 *
 * ถามจากปลายทางของปุ่ม ไม่ใช่จากว่าโฟกัสอยู่ที่ไหน: เอกสารในไอเฟรมก็มีช่องกรอก
 * ของมันได้เหมือนกัน และกฎเดียวกันนี้ตอบทั้งสองฝั่ง */
export function typedIntoField(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el || typeof el.closest !== 'function') return false
  return !!el.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])')
}

/** กดปุ่มที่เด็คฟังอยู่แล้วหนึ่งครั้ง
 *
 * แพเนลไม่เดา API ของเด็ค เด็คทรงซ้อนแทบทุกตัวผูกลูกศรซ้าย/ขวาไว้กับเอกสาร
 * (reveal.js ทำ, และสคริปต์ที่โมเดลเขียนเองก็ทำ) การกดปุ่มจึงเป็นทางเดียวที่พูดกับ
 * เด็คได้โดยไม่ต้องรู้จักมัน และเท่ากับคนกดคีย์บอร์ดหนึ่งครั้ง — ไม่มีอะไรถูกฉีดลง
 * ไฟล์ของผู้ใช้ ซึ่งเป็นกติกาที่หัวไฟล์ SlidesPane ประกาศไว้ */
export function sendStepKey(doc: Document, forward: boolean): void {
  const view = doc.defaultView
  if (!view) return
  doc.dispatchEvent(
    new view.KeyboardEvent('keydown', {
      key: forward ? 'ArrowRight' : 'ArrowLeft',
      bubbles: true,
      cancelable: true,
    }),
  )
}

/** ขนาดของสไลด์หนึ่งใบตามสัญญา — 16:9 และเท่ากับหน้า PowerPoint จอกว้างที่ 96dpi */
export const DECK_BASE = { width: 1280, height: 720 }

/** กรอบที่เด็คควรถูกวาดลงไป และอัตราส่วนที่ย่อกรอบนั้นลงมาให้พอดีเวที
 *
 * อยู่ที่นี่ไม่ใช่ในแพเนลด้วยเหตุผลเดียวกับข้างบน: เป็นเลขคณิต ตอบได้โดยไม่ต้องมี
 * ไอเฟรม และเป็นจุดที่พลาดแล้วเห็นเป็นอาการหน้าจอ ไม่ใช่เป็นข้อความผิดพลาด
 *
 * กรอบมาก่อนการวัด เด็คที่วัดตัวเองจากวิวพอร์ต (`min-height:100vh`) จึงถูกถามใน
 * กรอบ 1280 × 720 แล้วตอบกลับมาเท่ากรอบ — ซึ่งคือคำตอบที่ถูก ส่วนเด็คที่ประกาศ
 * ขนาดของตัวเอง (1920 × 1080, 4:3) ตอบขนาดของมัน แล้วกรอบก็ตามไปเป็นขนาดนั้น
 *
 * ไม่มีเพดานที่ 1 เพราะย่อขยายทั้งกรอบไม่ทำให้เด็คจัดหน้าใหม่ สัดส่วนตัวอักษรต่อ
 * สไลด์คงเดิมทุกอัตราส่วน และการนำเสนอเต็มจอต้องขยายขึ้นได้ ไม่งั้นสไลด์ 1280 จะ
 * กองอยู่กลางจอ 1920 */
export function deckFit(
  stage: { width: number; height: number },
  slide: { width: number; height: number },
): { width: number; height: number; scale: number } {
  const width = slide.width > 1 ? Math.round(slide.width) : DECK_BASE.width
  const height = slide.height > 1 ? Math.round(slide.height) : DECK_BASE.height
  return { width, height, scale: Math.min(stage.width / width, stage.height / height) }
}
