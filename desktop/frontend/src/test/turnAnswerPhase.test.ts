// The closing sentence is the one text part the live event stream never sends.
//
// The engine emits a `note` for the prose of every round that is followed by
// tool calls and deliberately not for the last one (`if r.Final { return }` in
// internal/turn/executor.go) — the old bubble took the answer from Reply and
// drew it separately, so nothing needed it twice. Once the bubble is drawn from
// the sequence, a step list without it is a turn whose answer is not in it: the
// last phase never exists and the reply is on screen nowhere at all.
//
// Owner caught it by eye on 29 ส.ค. — "คำตอบสุดท้ายมันหายไปไหน" — with a full
// suite green, which is why this file exists.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, sendUserMessage } from '../lib/stores/cockpit.svelte'
import { phasesOf } from '../lib/turnPhases'
import { SendMessage } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.awaitingReply = false
  cockpit.toolSteps = []
  cockpit.streamingText = ''
  cockpit.reasoningText = ''
  cockpit.turnFiles = []
  cockpit.turnProposals = []
})

const answer = 'เจอแล้วครับ อยู่บรรทัดที่ 12'

/** A turn that narrated once, ran a tool, then answered — the events exactly as
 *  the engine sends them, closing sentence absent by design. */
const liveTurn = () => {
  cockpit.toolSteps = [
    { kind: 'note', label: 'ขอไล่ดูก่อนครับ', state: 'done', startedAt: 0 },
    { label: 'read note.txt', state: 'done', startedAt: 0, secs: 2 },
  ] as any
}

describe('the answer of a turn that just finished', () => {
  it('is the last phase, not something the sequence forgot', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      liveTurn()
      return { text: answer, parts: [{ kind: 'text', text: answer }] } as any
    })

    await sendUserMessage('อ่านไฟล์ให้หน่อย')

    const reply = cockpit.chat.at(-1)
    const phases = phasesOf(reply?.steps ?? [])
    expect(phases.map((p) => p.say)).toEqual(['ขอไล่ดูก่อนครับ', answer])
    // And it is the phase nothing ran after, which is the only thing that makes
    // it the answer in this layout.
    expect(phases.at(-1)?.steps).toHaveLength(0)
  })

  // An interjection demotes a finished answer, and THAT one the engine does
  // send, as `said`. Appending the reply again would put it on screen twice.
  it('is not doubled when the engine already sent it', async () => {
    vi.mocked(SendMessage).mockImplementation(async () => {
      cockpit.toolSteps = [
        { kind: 'said', label: answer, state: 'done', startedAt: 0 },
      ] as any
      return { text: answer, parts: [{ kind: 'text', text: answer, demoted: true }] } as any
    })

    await sendUserMessage('อ่านไฟล์ให้หน่อย')

    const steps = cockpit.chat.at(-1)?.steps ?? []
    expect(steps.filter((s) => s.label === answer)).toHaveLength(1)
  })

  // A turn that answered without calling anything has no live rows at all. It
  // still has to arrive as a phase, or the whole bubble is empty.
  it('stands alone on a turn that ran nothing', async () => {
    vi.mocked(SendMessage).mockImplementation(async () =>
      ({ text: answer, parts: [{ kind: 'text', text: answer }] }) as any)

    await sendUserMessage('สวัสดี')

    const phases = phasesOf(cockpit.chat.at(-1)?.steps ?? [])
    expect(phases.map((p) => p.say)).toEqual([answer])
  })
})
