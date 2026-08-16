// A reopened answer used to come back with empty toggles: tool calls were never
// written down anywhere a message could reach. The stored sequence is what fixed
// that — folded back into the same collapsed timeline the bubble has always
// drawn, because that layout reads better than the sequence drawn inline.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, selectSession } from '../lib/stores/cockpit.svelte'
import { LoadSession } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
})

const storedTurn = [
  { role: 'user', text: 'อ่านไฟล์ให้หน่อย', time: '00:34' },
  {
    role: 'agent', text: 'เจอแล้วครับ อยู่บรรทัดที่ 12', time: '00:34',
    parts: [
      { kind: 'text', text: 'กำลังอ่านไฟล์ให้ครับ' },
      { kind: 'thinking', secs: 4 },
      { kind: 'tool', tool: { ref: 'c1', name: 'read', subject: 'note.txt', ok: true, secs: 2 } },
      { kind: 'tool', tool: { ref: 'c2', name: 'grep', subject: 'battery', ok: false, error: 'ไม่พบ' } },
      { kind: 'text', text: 'เจอแล้วครับ อยู่บรรทัดที่ 12' },
    ],
  },
]

describe('reopening a session', () => {
  it('brings the tool timeline back with the answer', async () => {
    LoadSession.mockResolvedValueOnce(storedTurn as any)

    await selectSession({ id: 's1', title: 'x', ago: '' })

    const reply = cockpit.chat[1]
    expect(reply.text).toBe('เจอแล้วครับ อยู่บรรทัดที่ 12')

    const steps = reply.steps ?? []
    // The narration in front of the work is a note row, where it always sat.
    expect(steps[0]).toMatchObject({ kind: 'note', label: 'กำลังอ่านไฟล์ให้ครับ' })
    expect(steps[1]).toMatchObject({ kind: 'thinking', secs: 4 })
    expect(steps[2]).toMatchObject({ label: 'read note.txt', state: 'done', secs: 2 })
    expect(steps[3]).toMatchObject({ label: 'grep battery', state: 'err', error: 'ไม่พบ' })
    // The closing answer is the bubble, not a note — it must not appear twice.
    expect(steps.filter((s) => s.label === 'เจอแล้วครับ อยู่บรรทัดที่ 12')).toHaveLength(0)
  })

  // An interjection demotes a finished answer to a non-last text part. Read back
  // as a note, that reply came home at --fs-xs in --text-muted with its markdown
  // showing as source, behind the tools toggle. The sequence records which text
  // parts were answers, so reopening the session gets the answer back.
  it('brings back an answer the user typed over as an answer', async () => {
    LoadSession.mockResolvedValueOnce([
      { role: 'user', text: 'เช็คหน่อย', time: '00:34' },
      {
        role: 'agent', text: 'เช็ค memory แล้วครับ', time: '00:35',
        parts: [
          { kind: 'text', text: '## สรุป\n\nเรียบร้อยครับ', demoted: true },
          { kind: 'text', text: 'กำลังเช็คเพิ่มให้ครับ' },
          { kind: 'tool', tool: { ref: 'c1', name: 'grep', subject: 'memory', ok: true } },
          { kind: 'text', text: 'เช็ค memory แล้วครับ' },
        ],
      },
    ] as any)

    await selectSession({ id: 's3', title: 'x', ago: '' })

    const steps = cockpit.chat[1].steps ?? []
    expect(steps[0]).toMatchObject({ kind: 'said', label: '## สรุป\n\nเรียบร้อยครับ' })
    // The narration beside the grep is still narration — only the answer moved.
    expect(steps[1]).toMatchObject({ kind: 'note', label: 'กำลังเช็คเพิ่มให้ครับ' })
    expect(steps[2]).toMatchObject({ label: 'grep memory', state: 'done' })
  })

  it('leaves a pre-sequence answer exactly as it was', async () => {
    LoadSession.mockResolvedValueOnce([
      { role: 'user', text: 'ถาม', time: '00:34' },
      { role: 'agent', text: 'ตอบ', time: '00:34' },
    ] as any)

    await selectSession({ id: 's2', title: 'x', ago: '' })

    // No sequence stored, so no timeline is invented for it.
    expect(cockpit.chat[1].steps).toBeUndefined()
    expect(cockpit.chat[1].text).toBe('ตอบ')
  })
})
