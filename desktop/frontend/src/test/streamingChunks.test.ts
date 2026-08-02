// The live answer bubble now has three kinds of writer on one event: streamed
// fragments, an erase when a round turns out not to be the answer, and the
// finished reply. Getting the append/replace rule wrong shows the answer twice,
// which is exactly what the old append-only handler would have done the moment
// content started streaming.
import { describe, it, expect, beforeEach } from 'vitest'
import { cockpit, applyAgentChunk } from '../lib/stores/cockpit.svelte'

beforeEach(() => {
  cockpit.streamingText = ''
})

describe('the live answer bubble', () => {
  it('appends streamed fragments in order', () => {
    applyAgentChunk({ text: 'ชาร์จ', replace: false })
    applyAgentChunk({ text: 'ไปเรื่อยๆ ', replace: false })
    applyAgentChunk({ text: 'ครับ', replace: false })
    expect(cockpit.streamingText).toBe('ชาร์จไปเรื่อยๆ ครับ')
  })

  it('erases everything when a round turns out not to be the answer', () => {
    applyAgentChunk({ text: 'กำลังอ่านไฟล์ให้ครับ', replace: false })
    applyAgentChunk({ text: '', replace: true })
    expect(cockpit.streamingText).toBe('')
  })

  it('never doubles the answer, however much of it streamed first', () => {
    // The exact scenario the replace flag exists for: the preview already holds
    // the whole answer when the authoritative delivery arrives.
    const answer = 'ชาร์จไปเรื่อยๆ ครับ แบตจะขึ้นทีละน้อย'
    applyAgentChunk({ text: answer, replace: false })
    applyAgentChunk({ text: '', replace: true })
    applyAgentChunk({ text: answer, replace: true })
    expect(cockpit.streamingText).toBe(answer)
  })

  it('lands the reply intact even if nothing streamed', () => {
    applyAgentChunk({ text: 'คำตอบเต็ม', replace: true })
    expect(cockpit.streamingText).toBe('คำตอบเต็ม')
  })

  it('discards a narration round before the answer arrives', () => {
    applyAgentChunk({ text: 'กำลังหาไฟล์ให้ครับ', replace: false })
    applyAgentChunk({ text: '', replace: true }) // round was narration
    applyAgentChunk({ text: 'เจอแล้ว', replace: false })
    applyAgentChunk({ text: 'ครับ', replace: false })
    expect(cockpit.streamingText).toBe('เจอแล้วครับ')
    expect(cockpit.streamingText).not.toContain('กำลังหา')
  })

  // A frontend reloaded against an engine that still emits bare strings must
  // degrade to the old behaviour, not render "[object Object]".
  it('still accepts the old bare-string payload', () => {
    applyAgentChunk('one ')
    applyAgentChunk('two')
    expect(cockpit.streamingText).toBe('one two')
  })
})
