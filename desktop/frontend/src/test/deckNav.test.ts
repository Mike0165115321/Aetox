// การเดินเด็คสองทรง
//
// อาการที่เทสต์นี้กัน เจ้าของเจอเมื่อ 2026-08-20: เปิดเด็คที่โมเดลเพิ่งเขียน แล้ว
// ปุ่มลูกศรของแพเนลกดไม่ไปไหน ตัวนับก็ไม่ตรงกับที่เห็น
//
// ต้นเหตุคือแพเนลรู้จักเด็คทรงเดียว — สไลด์เรียงกันลงมาเป็นสายแล้วเลื่อนไปหา — แต่
// เด็คที่โมเดลเขียนออกมาเองแทบทุกครั้งเป็นอีกทรงหนึ่ง: สไลด์ซ้อนกันอยู่ที่เดียว
// โชว์ทีละใบ แล้วเด็คสลับเอง ทรงนั้นไม่มีอะไรให้เลื่อน scrollIntoView จึงเงียบ
import { describe, it, expect, beforeEach } from 'vitest'
import { documentScrolls, sendStepKey, slideElements, visibleIndex } from '../lib/workbench/deckNav'

// เขียนลงเอกสารจริงของ jsdom ไม่ใช่เอกสารที่ DOMParser สร้าง เพราะ getComputedStyle
// อ่านค่าได้ก็ต่อเมื่อ element อยู่ในเอกสารที่ผูกกับหน้าต่างนี้ — ซึ่งเป็นเงื่อนไข
// เดียวกับของจริง: แพเนลอ่านสไลด์ในไอเฟรมผ่าน defaultView ของไอเฟรมนั้น
function load(html: string): HTMLElement[] {
  document.body.innerHTML = html
  return slideElements(document)
}

// jsdom ตอบ scrollHeight/clientHeight เป็นศูนย์เสมอ ซึ่งจะทำให้ทุกเด็คอ่านว่าเป็น
// ทรงซ้อน รวมทั้งทรงที่ควรเลื่อน ค่าทั้งสองจึงถูกกำหนดตรงนี้
function layout(scrollHeight: number, clientHeight: number) {
  Object.defineProperty(document.documentElement, 'scrollHeight', { value: scrollHeight, configurable: true })
  Object.defineProperty(document.documentElement, 'clientHeight', { value: clientHeight, configurable: true })
}

const stackedDeck = `
  <section class="slide" style="visibility:visible">หนึ่ง</section>
  <section class="slide" style="visibility:hidden">สอง</section>
  <section class="slide" style="visibility:hidden">สาม</section>`

beforeEach(() => {
  document.body.innerHTML = ''
  layout(720, 720)
})

describe('slideElements', () => {
  it('reads sections when the document has them', () => {
    expect(load(stackedDeck).length).toBe(3)
  })

  // isDeck รับ div มาตั้งแต่ §154 ไฟล์ที่เป็น div ล้วนจึงมาถึงแพเนลนี้ได้ ก่อนหน้านี้
  // มันนับได้ศูนย์ใบ แล้วเปิดขึ้นมาเป็นห้องว่างที่มีปุ่มกดไม่ได้
  it('falls back to divs in a document with no sections', () => {
    expect(load('<div class="slide">หนึ่ง</div><div class="slide">สอง</div>').length).toBe(2)
  })

  // ในเอกสารที่มี section อยู่แล้ว div ที่ชื่อคลาสเดียวกันเป็นการจัดสไตล์ของใครบางคน
  // ไม่ใช่เส้นแบ่งสไลด์ — กฎเดียวกับ internal/deck.slideTag
  it('does not mix divs into a deck that already has sections', () => {
    expect(load('<section class="slide"><div class="slide">ข้างใน</div></section>').length).toBe(1)
  })
})

describe('documentScrolls', () => {
  it('says no for a deck whose slides are stacked in one place', () => {
    load(stackedDeck)
    layout(720, 720)
    expect(documentScrolls(document)).toBe(false)
  })

  it('says yes for a deck laid out as a column of slides', () => {
    load(stackedDeck)
    layout(2160, 720)
    expect(documentScrolls(document)).toBe(true)
  })
})

describe('visibleIndex', () => {
  it('finds the one slide that is showing', () => {
    const slides = load(stackedDeck)
    slides[0].style.visibility = 'hidden'
    slides[2].style.visibility = 'visible'
    expect(visibleIndex(slides, window, 0)).toBe(2)
  })

  // เด็คซ่อนสไลด์ได้สามวิธี และ `.active` เป็นธรรมเนียม ไม่ใช่สัญญา การอ่านจึงอ่าน
  // จากสไตล์ที่คำนวณแล้ว ไม่ใช่จากชื่อคลาส
  it('reads display and opacity too, not just visibility', () => {
    const slides = load(`
      <section class="slide" style="display:none">หนึ่ง</section>
      <section class="slide" style="opacity:0">สอง</section>
      <section class="slide" style="opacity:1">สาม</section>`)
    expect(visibleIndex(slides, window, 0)).toBe(2)
  })

  // อ่านไม่ได้เลยต้องตอบค่าที่แพเนลถืออยู่ ไม่ใช่ลากตัวนับกลับไปใบแรกทุกครั้ง
  it('keeps the caller’s answer when every slide is hidden', () => {
    const slides = load('<section class="slide" style="display:none"></section>')
    expect(visibleIndex(slides, window, 4)).toBe(4)
  })
})

describe('sendStepKey', () => {
  // แพเนลไม่รู้จัก API ของเด็ค มันกดปุ่มที่เด็คฟังอยู่แล้ว — เท่ากับคนกดคีย์บอร์ด
  // หนึ่งครั้ง ไม่มีอะไรถูกฉีดลงไฟล์ของผู้ใช้
  it('presses the arrow key the deck itself listens for', () => {
    load(stackedDeck)
    const seen: string[] = []
    const spy = (e: Event) => seen.push((e as KeyboardEvent).key)
    document.addEventListener('keydown', spy)
    sendStepKey(document, true)
    sendStepKey(document, false)
    document.removeEventListener('keydown', spy)
    expect(seen).toEqual(['ArrowRight', 'ArrowLeft'])
  })

  // เด็คจริงเดินตัวเองด้วยปุ่มนั้น — สคริปต์แบบเดียวกับที่โมเดลเขียนออกมา
  it('actually advances a deck that navigates itself', () => {
    const slides = load(stackedDeck)
    let at = 0
    const show = (i: number) => {
      slides.forEach((s, n) => (s.style.visibility = n === i ? 'visible' : 'hidden'))
      at = i
    }
    const drive = (e: Event) => {
      const key = (e as KeyboardEvent).key
      if (key === 'ArrowRight') show((at + 1) % slides.length)
      if (key === 'ArrowLeft') show((at - 1 + slides.length) % slides.length)
    }
    document.addEventListener('keydown', drive)

    sendStepKey(document, true)
    expect(visibleIndex(slides, window, 0)).toBe(1)
    sendStepKey(document, true)
    expect(visibleIndex(slides, window, 0)).toBe(2)
    sendStepKey(document, false)
    expect(visibleIndex(slides, window, 0)).toBe(1)
    document.removeEventListener('keydown', drive)
  })
})
