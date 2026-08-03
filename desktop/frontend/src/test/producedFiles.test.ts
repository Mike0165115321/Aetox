// A turn that produces a file has to hand it back on the answer, not leave it
// in the tree to be found. Before this the workbook existed and the reply named
// it in a sentence, and reaching it meant opening the file panel and hunting —
// four clicks from a product whose promise is finished work.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, applyToolEvent, sendUserMessage, selectSession } from '../lib/stores/cockpit.svelte'
import { SendMessage, LoadSession } from './mocks/wailsApp'
import type { ToolEvent } from '../lib/types'

const call = (ref: string, name: string): ToolEvent => ({ action: 'call', name, ref })
const result = (ref: string, name: string, artifacts?: string[]): ToolEvent =>
  ({ action: 'result', name, ref, ok: true, artifacts })

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
  cockpit.toolSteps.length = 0
  cockpit.turnFiles.length = 0
  cockpit.awaitingReply = false
})

describe('a file the turn produced', () => {
  it('rides back on the answer that announced it', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'sheet_write'))
      applyToolEvent(result('c1', 'sheet_write', ['out/สรุปสลิป.xlsx']))
      return { text: 'ทำไฟล์ให้แล้วครับ' }
    })

    await sendUserMessage('สรุปสลิปให้หน่อย')

    const reply = cockpit.chat.at(-1)!
    expect(reply.role).toBe('agent')
    expect(reply.producedFiles).toEqual(['out/สรุปสลิป.xlsx'])
  })

  it('does not follow the next turn around', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'sheet_write'))
      applyToolEvent(result('c1', 'sheet_write', ['book.xlsx']))
      return { text: 'done' }
    })
    await sendUserMessage('one')

    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c2', 'read'))
      applyToolEvent(result('c2', 'read'))
      return { text: 'read it' }
    })
    await sendUserMessage('two')

    expect(cockpit.chat.at(-1)!.producedFiles).toBeUndefined()
  })

  it('offers a workbook written twice in one turn only once', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'sheet_write'))
      applyToolEvent(result('c1', 'sheet_write', ['book.xlsx']))
      // The correction pass — same file, written again.
      applyToolEvent(call('c2', 'sheet_write'))
      applyToolEvent(result('c2', 'sheet_write', ['book.xlsx']))
      return { text: 'fixed' }
    })

    await sendUserMessage('fix the total')

    expect(cockpit.chat.at(-1)!.producedFiles).toEqual(['book.xlsx'])
  })

  it('stays empty for tools that only edit files, so a coding turn shows no cards', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'write'))
      applyToolEvent(result('c1', 'write'))
      applyToolEvent(call('c2', 'edit'))
      applyToolEvent(result('c2', 'edit'))
      return { text: 'edited two files' }
    })

    await sendUserMessage('rename the function')

    expect(cockpit.chat.at(-1)!.producedFiles).toBeUndefined()
  })
})

// The failure this half exists for, found by the owner within hours of the card
// shipping: restart the app, reopen the session, and every open button was gone.
// The workbook was still on disk and the answer still named it, which is exactly
// the "the file exists and you cannot reach it" problem the card was built to
// remove — reintroduced by a reload.
describe('a reopened session', () => {
  it('brings the open button back from the stored parts', async () => {
    LoadSession.mockResolvedValueOnce([
      { role: 'user', text: 'สรุปสลิปให้หน่อย', time: '01:32' },
      {
        role: 'agent', text: 'ทำไฟล์ให้แล้วครับ', time: '01:32',
        parts: [
          { kind: 'tool', tool: { name: 'sheet_write', ok: true, artifacts: ['out/สรุปสลิป.xlsx'] } },
          { kind: 'text', text: 'ทำไฟล์ให้แล้วครับ' },
        ],
      },
    ] as any)

    await selectSession({ id: 'session-1', title: '', ago: '' })

    const reply = cockpit.chat.at(-1)!
    expect(reply.producedFiles).toEqual(['out/สรุปสลิป.xlsx'])
  })

  it('leaves a turn that produced nothing without cards', async () => {
    LoadSession.mockResolvedValueOnce([
      {
        role: 'agent', text: 'อ่านให้แล้ว', time: '01:40',
        parts: [{ kind: 'tool', tool: { name: 'read', ok: true } }, { kind: 'text', text: 'อ่านให้แล้ว' }],
      },
    ] as any)

    await selectSession({ id: 'session-2', title: '', ago: '' })

    expect(cockpit.chat.at(-1)!.producedFiles).toBeUndefined()
  })
})
