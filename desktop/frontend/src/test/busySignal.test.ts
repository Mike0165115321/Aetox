// ไฟบอกสถานะ, driven by events and by nothing else.
//
// The one behaviour worth pinning here is the one the whole layer's credibility
// rests on: the light goes on because a browser call started and goes out
// because that call finished. A timer anywhere in this store would make it a
// light that says the agent is working when it is not, which costs more than
// the light was ever worth.
import { describe, it, expect, beforeEach } from 'vitest'
import { busyWork, applyBusyEvent, clearBusyWork } from '../lib/stores/busySignal.svelte'

const call = (over: Record<string, unknown> = {}) =>
  ({ action: 'call', name: 'browser', ref: 'c1', act: 'click', ...over }) as never
const result = (over: Record<string, unknown> = {}) =>
  ({ action: 'result', name: 'browser', ref: 'c1', act: 'click', ok: true, ...over }) as never

beforeEach(() => clearBusyWork())

describe('busy signal, live', () => {
  it('lights on a browser call and goes out on its result', () => {
    expect(busyWork.running).toBe(false)
    applyBusyEvent(call({ act: 'open', subject: 'https://a.test', tab: 'web-agent-1' }))
    expect(busyWork.running).toBe(true)
    expect(busyWork.act).toBe('open')
    expect(busyWork.subject).toBe('https://a.test')
    expect(busyWork.tab).toBe('web-agent-1')

    applyBusyEvent(result({ act: 'open', subject: 'https://a.test' }))
    expect(busyWork.running).toBe(false)
  })

  // The bar keeps saying what was just done, with its light out. A record of
  // the last action is true; a blank strip taking up the same room is not
  // better, it is just emptier.
  it('remembers the last action after it finishes', () => {
    applyBusyEvent(call({ act: 'read', subject: 'https://a.test' }))
    applyBusyEvent(result({ act: 'read', subject: 'https://a.test' }))
    expect(busyWork.running).toBe(false)
    expect(busyWork.seen).toBe(true)
    expect(busyWork.act).toBe('read')
  })

  // A round can carry more than one browser call, and a single boolean would be
  // cleared by whichever finished first while the other was still going.
  it('stays lit while a second call in the same round is still running', () => {
    applyBusyEvent(call({ ref: 'c1', act: 'read' }))
    applyBusyEvent(call({ ref: 'c2', act: 'capture' }))
    applyBusyEvent(result({ ref: 'c1', act: 'read' }))
    expect(busyWork.running).toBe(true)
    applyBusyEvent(result({ ref: 'c2', act: 'capture' }))
    expect(busyWork.running).toBe(false)
  })

  it('ignores every tool that is not the browser', () => {
    applyBusyEvent({ action: 'call', name: 'write', subject: 'a.go' } as never)
    applyBusyEvent({ action: 'call', name: 'shell', act: 'run' } as never)
    expect(busyWork.running).toBe(false)
    expect(busyWork.seen).toBe(false)
  })

  // The events arrive stamped with the session they happened in. An older
  // engine sends them bare, and a panel that only understood one shape would go
  // dark against the other.
  it('reads a stamped event and a bare one the same way', () => {
    applyBusyEvent({ sessionId: 's1', data: call({ act: 'scroll' }) } as never)
    expect(busyWork.act).toBe('scroll')
    clearBusyWork()
    applyBusyEvent(call({ act: 'type' }))
    expect(busyWork.act).toBe('type')
  })

  // The far end, and not a fallback for a missing result: a stopped turn owes
  // nobody one. Without it the panel would still be glowing tomorrow morning.
  it('goes dark when the turn ends, result or no result', () => {
    applyBusyEvent(call({ act: 'wait', tab: 'web-agent-1' }))
    expect(busyWork.running).toBe(true)
    clearBusyWork()
    expect(busyWork.running).toBe(false)
    expect(busyWork.seen).toBe(false)
    expect(busyWork.tab).toBe('')
    // And the next turn starts from nothing rather than from a stuck flag.
    applyBusyEvent(result({ act: 'wait' }))
    expect(busyWork.running).toBe(false)
  })

  // A tab the engine could not name is a real state — the first `open` of a
  // session happens before any tab exists. The panel lights itself and points
  // at no chip, which is honest; a guess would point at somebody else's page.
  it('accepts a call with no tab to point at', () => {
    applyBusyEvent(call({ act: 'open', subject: 'https://a.test', tab: undefined }))
    expect(busyWork.running).toBe(true)
    expect(busyWork.tab).toBe('')
  })
})
