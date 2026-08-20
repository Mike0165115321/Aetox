// กรอบของเด็คกับอัตราส่วนที่ย่อมันลงมา
//
// อาการที่เทสต์ชุดนี้เฝ้าอยู่คือของจริง: 2026-08-20 เจ้าของเปิดเด็คในห้องสไลด์แล้ว
// เห็นสไลด์ถูกบีบจนล้นออกนอกกรอบและเลื่อนเอง สาเหตุคือเด็คที่วัดตัวเองจากวิวพอร์ต
// ถูกวางในกรอบเท่าแพเนล มันจึงขยายตัวเองไปเท่าแพเนลพอดี แล้วอัตราส่วนที่คำนวณได้
// เป็น 1 ทุกครั้ง ไม่มีอะไรถูกย่อเลย
import { describe, expect, it } from 'vitest'
import { DECK_BASE, deckFit } from '../lib/workbench/deckNav'

describe('deckFit', () => {
  it('เด็คที่วัดจากวิวพอร์ตได้กรอบ 16:9 แล้วถูกย่อจริง', () => {
    // สไลด์ตอบเท่ากรอบมาตรฐาน เพราะนั่นคือกรอบที่มันเพิ่งถูกวางลงไป
    const fit = deckFit({ width: 1000, height: 700 }, { width: 1280, height: 720 })
    expect(fit.width).toBe(DECK_BASE.width)
    expect(fit.height).toBe(DECK_BASE.height)
    expect(fit.scale).toBeCloseTo(1000 / 1280)
  })

  it('พอดีทั้งใบ ไม่ใช่พอดีแค่ด้านกว้าง', () => {
    // เวทีเตี้ย ด้านสูงคือด้านที่บีบ
    const fit = deckFit({ width: 1920, height: 540 }, { width: 1280, height: 720 })
    expect(fit.scale).toBeCloseTo(540 / 720)
  })

  it('เด็คที่ประกาศขนาดของตัวเองได้กรอบของมัน', () => {
    const fit = deckFit({ width: 1280, height: 720 }, { width: 1920, height: 1080 })
    expect(fit.width).toBe(1920)
    expect(fit.height).toBe(1080)
    expect(fit.scale).toBeCloseTo(1280 / 1920)
  })

  it('ขยายเกิน 1 ได้ เพื่อให้การนำเสนอเต็มจอเต็มจริง', () => {
    const fit = deckFit({ width: 1920, height: 1080 }, { width: 1280, height: 720 })
    expect(fit.scale).toBeCloseTo(1.5)
  })

  it('ใบที่ซ่อนอยู่วัดได้ศูนย์ ตกกลับไปที่กรอบมาตรฐาน แทนที่จะหารด้วยศูนย์', () => {
    const fit = deckFit({ width: 640, height: 360 }, { width: 0, height: 0 })
    expect(fit.width).toBe(DECK_BASE.width)
    expect(fit.height).toBe(DECK_BASE.height)
    expect(fit.scale).toBeCloseTo(0.5)
  })
})
