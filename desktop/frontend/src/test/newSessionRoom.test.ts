// Where "new chat" lands.
//
// Ctrl+N and the + button used to call the engine's bare NewSession, which
// leaves desk and chair standing — so pressing them inside ระบบออโตเมชั่น gave
// you a blank chat still seated with the automation specialist, in the room you
// were trying to leave. They now open at the main desk of the door you are at,
// and no further: mid-task in the workshop, a keystroke must not drop you in
// the storefront (owner, 14 ส.ค.).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, newSession, startTaskChip } from '../lib/stores/cockpit.svelte'
import { shell } from '../lib/shell.svelte'
import { NewSession, NewSessionAt, DismissTaskChip } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.awaitingReply = false
  cockpit.sessionError = ''
  cockpit.space = ''
  shell.name = 'assistant'
})

describe('new chat', () => {
  it('leaves a specialist\u2019s room for the storefront\u2019s main desk', async () => {
    // Standing in ระบบออโตเมชั่น: a chair session, not a desk of its own.
    cockpit.desk = 'specialized'
    cockpit.chair = 'automation'

    await newSession()

    expect(vi.mocked(NewSessionAt)).toHaveBeenCalledWith('assistant')
    expect(cockpit.desk).toBe('assistant')
    expect(cockpit.chair).toBe('')
    expect(cockpit.activeView).toBe('chat')
  })

  it('stays behind the workshop door instead of walking through it', async () => {
    shell.name = 'code'
    cockpit.desk = 'coding'

    await newSession()

    expect(vi.mocked(NewSessionAt)).toHaveBeenCalledWith('coding')
    expect(cockpit.desk).toBe('coding')
  })

  it('lands on the chat even when pressed from a full-page room', async () => {
    cockpit.activeView = 'settings'

    await newSession()

    expect(cockpit.activeView).toBe('chat')
  })

  it('leaves the project behind, which is what the engine already did', async () => {
    cockpit.space = 'D:/work/aetox'

    await newSession()

    expect(cockpit.space).toBe('')
  })

  // A chip is side work the agent in *this* room noticed — a stale doc the
  // workshop grepped past belongs to the workshop. Carrying it out to ผู้ช่วย
  // would hand it to someone who was never in the conversation.
  it('a task chip runs where it was raised, not at the door\u2019s desk', async () => {
    cockpit.desk = 'coding'
    cockpit.chair = ''

    await startTaskChip({ id: 'c1', title: 'ลบ config ที่ตายแล้ว', tldr: '', prompt: 'ลบให้หน่อย', createdAt: '' })

    expect(vi.mocked(DismissTaskChip)).toHaveBeenCalledWith('c1')
    expect(vi.mocked(NewSession)).toHaveBeenCalled()
    expect(vi.mocked(NewSessionAt)).not.toHaveBeenCalled()
    expect(cockpit.desk).toBe('coding')
  })

  // It used to refuse. That was the single live state defending itself: opening
  // a chat emptied the one on screen, so doing it mid-turn wiped the work. The
  // work travels now (cockpit.parked), so a new chat while one runs is an
  // ordinary thing to do — and the room still moves, because that is what the
  // button is for.
  it('opens a new chat while a turn is running, and the work is kept', async () => {
    cockpit.openSession = 'working'
    cockpit.awaitingReply = true
    cockpit.activeView = 'settings'
    cockpit.desk = 'specialized'
    cockpit.chair = 'automation'

    await newSession()

    expect(vi.mocked(NewSessionAt)).toHaveBeenCalled()
    expect(cockpit.activeView).toBe('chat')
    // The chat that was working kept its whole live state, not just its rows.
    expect(cockpit.parked['working']?.awaitingReply).toBe(true)
    // And the new one starts idle rather than inheriting the other's turn.
    expect(cockpit.awaitingReply).toBe(false)
  })
})
