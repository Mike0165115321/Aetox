// Memory is the one tool whose work does not happen when it runs: it queues a
// proposal and waits for a person. Everything here guards the consequence of
// that — the asking has to reach the user where the work happened, say what it
// wants in words they can judge, and stop asking once they have answered.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import MemoryCard from '../lib/MemoryCard.svelte'
import { cockpit, applyToolEvent, sendUserMessage, selectSession } from '../lib/stores/cockpit.svelte'
import {
  SendMessage, LoadSession, PendingChangeByID, ApprovePendingChange, RejectPendingChange,
} from './mocks/wailsApp'
import type { ToolEvent } from '../lib/types'

const call = (ref: string, name: string): ToolEvent => ({ action: 'call', name, ref })
const result = (ref: string, name: string, proposalId?: number): ToolEvent =>
  ({ action: 'result', name, ref, ok: true, proposalId })

const proposal = (over: Record<string, unknown> = {}) => ({
  id: 7, kind: 'memory', scope: '', target: 'C:/aetox/memory/MEMORY.md',
  op: 'add', before: '', body: 'เครื่องนี้ไม่มี Excel ติดตั้ง',
  reason: 'เปิดไฟล์ .xlsx แล้วไม่มีโปรแกรมรับ', evidence: 'session:1',
  source: 'agent', state: 'pending', createdAt: '2026-08-16T10:00:00Z', decidedAt: '',
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
  cockpit.toolSteps.length = 0
  cockpit.turnFiles.length = 0
  cockpit.turnProposals.length = 0
  cockpit.pendingLearned = 0
  cockpit.awaitingReply = false
  vi.mocked(PendingChangeByID).mockResolvedValue(proposal() as any)
})

describe('what a turn asked to remember', () => {
  it('rides back on the answer that asked', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'memory'))
      applyToolEvent(result('c1', 'memory', 7))
      return { text: 'จำไว้ให้แล้วครับ รออนุมัติ' }
    })

    await sendUserMessage('เครื่องนี้ไม่มี Excel นะ')

    expect(cockpit.chat.at(-1)!.proposals).toEqual([7])
  })

  it('draws one card for a proposal the turn made twice', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'memory'))
      applyToolEvent(result('c1', 'memory', 7))
      // The model forgot it had already asked. The engine answers the second
      // attempt with the id already waiting, so this is one proposal.
      applyToolEvent(call('c2', 'memory'))
      applyToolEvent(result('c2', 'memory', 7))
      return { text: 'ok' }
    })

    await sendUserMessage('จำไว้ด้วย')

    expect(cockpit.chat.at(-1)!.proposals).toEqual([7])
  })

  it('leaves a turn that proposed nothing without a card', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'read'))
      applyToolEvent(result('c1', 'read'))
      return { text: 'อ่านให้แล้ว' }
    })

    await sendUserMessage('อ่านไฟล์นี้ที')

    expect(cockpit.chat.at(-1)!.proposals).toBeUndefined()
  })

  it('does not follow the next turn around', async () => {
    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c1', 'memory'))
      applyToolEvent(result('c1', 'memory', 7))
      return { text: 'ok' }
    })
    await sendUserMessage('one')

    SendMessage.mockImplementationOnce(async () => {
      applyToolEvent(call('c2', 'read'))
      applyToolEvent(result('c2', 'read'))
      return { text: 'read it' }
    })
    await sendUserMessage('two')

    expect(cockpit.chat.at(-1)!.proposals).toBeUndefined()
  })

  // Same failure the file card had: restart, reopen the session, and the thing
  // the answer was about is gone. A proposal nobody can see is a proposal
  // nobody decides.
  it('comes back on a reopened session', async () => {
    LoadSession.mockResolvedValueOnce([
      { role: 'user', text: 'เครื่องนี้ไม่มี Excel นะ', time: '10:00' },
      {
        role: 'agent', text: 'รับทราบครับ', time: '10:00',
        parts: [
          { kind: 'tool', tool: { name: 'memory', ok: true, proposalId: 7 } },
          { kind: 'text', text: 'รับทราบครับ' },
        ],
      },
    ] as any)

    await selectSession({ id: 'session-1', title: '', ago: '' })

    expect(cockpit.chat.at(-1)!.proposals).toEqual([7])
  })
})

describe('the card itself', () => {
  // Everything needed to judge it is on screen without a click. The first cut
  // folded the proposal behind a chevron, which asked the user to open
  // something before they could read what they were being asked.
  it('says what it wants, whose memory it is and why, with nothing to open', async () => {
    const { container } = render(MemoryCard, { props: { id: 7 } })

    await waitFor(() => expect(screen.getByText('เครื่องนี้ไม่มี Excel ติดตั้ง')).toBeTruthy())
    expect(screen.getByText('ขอจำเรื่องนี้ไว้')).toBeTruthy()
    expect(screen.getByText('ผู้ช่วยหลัก')).toBeTruthy()
    // Approving an assertion with no provenance is not a decision.
    expect(container.textContent).toContain('เปิดไฟล์ .xlsx แล้วไม่มีโปรแกรมรับ')
    // The caveat every reader assumes wrongly.
    expect(container.textContent).toContain('มีผลตั้งแต่แชทถัดไป')
    // Nothing is folded away, so nothing offers to unfold it.
    expect(container.querySelector('.chev')).toBeNull()

    vi.mocked(PendingChangeByID).mockResolvedValue(proposal({ state: 'approved' }) as any)
    await fireEvent.click(screen.getByText('อนุมัติ'))

    await waitFor(() => expect(ApprovePendingChange).toHaveBeenCalledWith(7))
    // Decided is history, not a question: the card gives way to one quiet line.
    await waitFor(() => expect(screen.getByText('จำไว้แล้ว')).toBeTruthy())
    expect(container.querySelector('.memcard')).toBeNull()
    expect(container.querySelector('.memdone .chev')).toBeTruthy()
  })

  // The verb lives in the heading, which is what let the raw `ADD` enum go. A
  // replacement is the one the heading cannot carry alone: what it overwrites
  // has to be visible next to what it becomes.
  it('a replacement shows the line it would overwrite', async () => {
    vi.mocked(PendingChangeByID).mockResolvedValue(proposal({
      op: 'replace', before: 'เจ้าของใช้ cmd เป็นเชลล์หลัก', body: 'เจ้าของใช้ PowerShell เป็นเชลล์หลัก',
    }) as any)

    const { container } = render(MemoryCard, { props: { id: 7 } })

    await waitFor(() => expect(screen.getByText('ขอแก้สิ่งที่จำไว้')).toBeTruthy())
    expect(container.querySelector('.memcard-was')?.textContent).toContain('cmd')
    expect(container.querySelector('.memcard-line')?.textContent).toContain('PowerShell')
    // The database's own word for the operation never reaches the screen.
    expect(container.textContent).not.toContain('replace')
  })

  it('turning it down leaves the answer alone and stops asking', async () => {
    render(MemoryCard, { props: { id: 7 } })
    await waitFor(() => expect(screen.getByText('ขอจำเรื่องนี้ไว้')).toBeTruthy())

    vi.mocked(PendingChangeByID).mockResolvedValue(proposal({ state: 'rejected' }) as any)
    await fireEvent.click(screen.getByText('ไม่เอา'))

    await waitFor(() => expect(RejectPendingChange).toHaveBeenCalledWith(7))
    await waitFor(() => expect(screen.getByText('ไม่ได้จำ')).toBeTruthy())
    expect(screen.queryByText('อนุมัติ')).toBeNull()
  })

  // Decided in Settings, reopened here. The card reads the queue rather than a
  // copy of the sentence frozen into the transcript, so it cannot ask twice for
  // the same thing.
  it('a proposal already decided elsewhere does not ask again', async () => {
    vi.mocked(PendingChangeByID).mockResolvedValue(proposal({ state: 'approved' }) as any)

    render(MemoryCard, { props: { id: 7 } })

    await waitFor(() => expect(screen.getByText('จำไว้แล้ว')).toBeTruthy())
    expect(screen.queryByText('อนุมัติ')).toBeNull()
  })

  it('draws nothing when the row is gone', async () => {
    vi.mocked(PendingChangeByID).mockResolvedValue({ id: 0 } as any)

    const { container } = render(MemoryCard, { props: { id: 7 } })

    await waitFor(() => expect(PendingChangeByID).toHaveBeenCalled())
    expect(container.querySelector('.memcard')).toBeNull()
  })

  it('says an approval that failed, instead of a button that did nothing', async () => {
    vi.mocked(ApprovePendingChange).mockRejectedValueOnce(new Error('memory is full'))

    render(MemoryCard, { props: { id: 7 } })
    await waitFor(() => expect(screen.getByText('ขอจำเรื่องนี้ไว้')).toBeTruthy())
    await fireEvent.click(screen.getByText('ขอจำเรื่องนี้ไว้'))
    await fireEvent.click(screen.getByText('อนุมัติ'))

    await waitFor(() => expect(screen.getByText(/memory is full/)).toBeTruthy())
    expect(screen.getByText('ขอจำเรื่องนี้ไว้')).toBeTruthy()
  })
})
