import { describe, it, expect } from 'vitest'
import { isDeck } from '../lib/stores/workbench.svelte'

// เครื่องหมายเดียวที่บอกว่าไฟล์ .html เป็นเด็ค
//
// `section.slide` ทำสองหน้าที่โดยตั้งใจ — เป็นเส้นแบ่งที่แพเนลเดินตามและที่ตัว
// ส่งออกตัดตาม จึงเป็นตัวระบุไฟล์ไปด้วย (docs/architecture/html-deck-2026-08-19.md)
// เทสต์นี้กันสองทางพร้อมกัน: หน้าเว็บธรรมดาต้องไม่โดนลากไปเปิดเป็นสไลด์ และเด็ค
// จริงต้องไม่หลุดไปเปิดเป็นซอร์ส ซึ่งอย่างหลังคือหน้าตาของฟีเจอร์ที่หายไปเฉย ๆ
describe('isDeck', () => {
  const deck = '<html><body><section class="slide"><h1>ยอดขาย</h1></section></body></html>'

  it('รู้จักเด็คจาก section.slide', () => {
    expect(isDeck('out/deck.html', deck)).toBe(true)
  })

  it('รับได้ทั้ง single และ double quote และคลาสที่มีหลายตัว', () => {
    expect(isDeck('d.html', `<section class='intro slide dark'>x</section>`)).toBe(true)
    expect(isDeck('d.html', '<section id="a" class="slide">x</section>')).toBe(true)
  })

  it('หน้าเว็บธรรมดายังเปิดเป็นซอร์ส', () => {
    expect(isDeck('index.html', '<html><body><section><h1>สวัสดี</h1></section></body></html>')).toBe(false)
  })

  // "slideshow" มีคำว่า slide อยู่ข้างใน การจับแบบ substring จะลากหน้าเว็บที่ไม่
  // เกี่ยวอะไรเลยเข้ามาเปิดเป็นสไลด์เปล่า ๆ ขอบคลาสจึงต้องเป็นขอบคำ
  it('ไม่หลงคลาสที่แค่มีคำว่า slide อยู่ข้างใน', () => {
    expect(isDeck('d.html', '<section class="slideshow-wrapper">x</section>')).toBe(false)
  })

  it('ไม่ใช่ไฟล์ .html ก็ไม่ใช่เด็ค แม้ข้อความจะตรง', () => {
    expect(isDeck('notes.md', deck)).toBe(false)
    expect(isDeck('deck.txt', deck)).toBe(false)
  })

  it('รับ .htm ด้วย', () => {
    expect(isDeck('old.htm', deck)).toBe(true)
  })

  it('ไฟล์ว่างไม่ใช่เด็ค', () => {
    expect(isDeck('empty.html', '')).toBe(false)
  })
})
