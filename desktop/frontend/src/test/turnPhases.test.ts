import { describe, it, expect } from 'vitest'
import { phasesOf } from '../lib/turnPhases'
import type { ToolStep } from '../lib/types'

const tool = (label: string, extra: Partial<ToolStep> = {}): ToolStep =>
  ({ label, state: 'done', startedAt: 0, secs: 1, ...extra }) as ToolStep
const note = (label: string): ToolStep => ({ label, kind: 'note', state: 'done', startedAt: 0 }) as ToolStep
const thinking = (secs: number): ToolStep => ({ label: '', kind: 'thinking', state: 'done', startedAt: 0, secs }) as ToolStep

describe('phasesOf', () => {
  // The shape the window used to throw away: four stretches of work, each with
  // its own sentence and its own clock, flattened into one answer over two
  // summed numbers.
  it('starts a new phase at every sentence the model wrote', () => {
    const phases = phasesOf([
      thinking(41),
      note('ขอไล่ดูตรงที่ประกอบ path ก่อนครับ'),
      tool('grep placedWrite'),
      tool('read place.go'),
      thinking(96),
      note('เจอแล้ว เดี๋ยวแก้'),
      tool('edit place.go'),
      thinking(54),
      note('แก้แล้วครับ'),
    ])

    expect(phases.map((p) => p.say)).toEqual([
      'ขอไล่ดูตรงที่ประกอบ path ก่อนครับ',
      'เจอแล้ว เดี๋ยวแก้',
      'แก้แล้วครับ',
    ])
    expect(phases.map((p) => p.thinkSecs)).toEqual([41, 96, 54])
    expect(phases.map((p) => p.steps.length)).toEqual([2, 1, 0])
  })

  // The number on the old lump has to still be derivable from the pieces, or
  // the split is a different measurement rather than the same one told
  // honestly. Owner's own turn: 41+96+84+54 = 275, 9+14+8+5 = 36.
  it('keeps the totals the collapsed row used to show', () => {
    const phases = phasesOf([
      thinking(41), note('a'), ...Array.from({ length: 9 }, (_, i) => tool(`t${i}`)),
      thinking(96), note('b'), ...Array.from({ length: 14 }, (_, i) => tool(`u${i}`)),
      thinking(84), note('c'), ...Array.from({ length: 8 }, (_, i) => tool(`v${i}`)),
      thinking(54), note('d'), ...Array.from({ length: 5 }, (_, i) => tool(`w${i}`)),
    ])
    expect(phases.reduce((n, p) => n + p.thinkSecs, 0)).toBe(275)
    expect(phases.reduce((n, p) => n + p.steps.length, 0)).toBe(36)
  })

  // A delegate's rows arrive while the main agent has moved on to the next
  // sentence. Filed by arrival they would sit under a stretch of work they had
  // nothing to do with.
  it('files a sub-agent’s rows under the phase that hired it', () => {
    const phases = phasesOf([
      note('ให้เอเจนไปหาให้'),
      tool('task start', { ref: 'call-1', delegation: true }),
      note('ระหว่างรอ ขอดูโค้ดเอง'),
      tool('read app.go'),
      tool('grep foo', { parent: 'call-1' }),
    ])
    expect(phases).toHaveLength(2)
    expect(phases[0].steps.map((s) => s.label)).toEqual(['task start', 'grep foo'])
    expect(phases[1].steps.map((s) => s.label)).toEqual(['read app.go'])
  })

  // The live half. Prose still arriving is a phase, not a bubble the next tool
  // call erases.
  it('makes the still-streaming sentence a trailing open phase', () => {
    const phases = phasesOf([note('รอบแรก'), tool('read a.go')], 'กำลังจะแก้ตรงนี้')
    expect(phases.map((p) => p.streaming)).toEqual([false, true])
    expect(phases.at(-1)?.say).toBe('กำลังจะแก้ตรงนี้')
  })

  // Thought about, said nothing yet. The seconds must not be quietly added to
  // the stretch above, which finished before that thinking started.
  it('does not backdate thinking onto the phase before it', () => {
    const phases = phasesOf([note('รอบแรก'), tool('read a.go'), thinking(12)])
    expect(phases).toHaveLength(2)
    expect(phases[0].thinkSecs).toBe(0)
    expect(phases[1]).toMatchObject({ say: '', thinkSecs: 12, streaming: false })
  })

  // An answer an interjection re-placed is prose like any other prose now.
  it('treats a demoted answer as an ordinary phase', () => {
    const phases = phasesOf([
      { label: '## เสร็จแล้ว', kind: 'said', state: 'done', startedAt: 0 } as ToolStep,
      note('อ๋อ เดี๋ยวทำต่อ'),
    ])
    expect(phases.map((p) => p.say)).toEqual(['## เสร็จแล้ว', 'อ๋อ เดี๋ยวทำต่อ'])
  })

  it('opens a phase for a turn that called a tool without saying anything', () => {
    const phases = phasesOf([tool('read a.go')])
    expect(phases).toHaveLength(1)
    expect(phases[0]).toMatchObject({ say: '', thinkSecs: 0 })
  })

  it('has no phases at all for a turn that did nothing', () => {
    expect(phasesOf([])).toEqual([])
  })
})
