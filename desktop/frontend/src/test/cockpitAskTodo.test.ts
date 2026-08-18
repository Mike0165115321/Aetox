import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, applyAskUser, applyAskDone, answerAsk, applyTodos,
} from '../lib/stores/cockpit.svelte'
import { AnswerUserQuestion } from './mocks/wailsApp'

beforeEach(() => {
  cockpit.ask = null
  cockpit.todos = []
  vi.mocked(AnswerUserQuestion).mockClear()
})

describe('ask_user store flow', () => {
  it('applyAskUser exposes the pending question and applyAskDone clears it', () => {
    applyAskUser({ question: 'เลือกอันไหน?', options: ['A', 'B'] })
    expect(cockpit.ask).toEqual({ question: 'เลือกอันไหน?', options: ['A', 'B'] })
    applyAskDone()
    expect(cockpit.ask).toBeNull()
  })

  it('answerAsk delivers the choice to the backend and clears the panel', () => {
    applyAskUser({ question: 'q', options: ['A', 'B'] })
    answerAsk('B')
    // The chat is named as well as the answer: the engine holds one answer
    // channel per conversation now, so an unaddressed answer has nowhere to go.
    expect(AnswerUserQuestion).toHaveBeenCalledWith('', 'B')
    expect(cockpit.ask).toBeNull()
  })

  // A card raised by a chat is answered in that chat, whichever one the event
  // named — not "whatever the window thinks is current".
  it('answers the chat the question came from', () => {
    cockpit.turnSession = 's1'
    applyAskUser({ sessionId: 's1', data: { question: 'q', options: ['A', 'B'] } })
    answerAsk('A')
    expect(AnswerUserQuestion).toHaveBeenCalledWith('s1', 'A')
    cockpit.turnSession = ''
  })

  it('answerAsk ignores blank input (panel stays up for a real answer)', () => {
    applyAskUser({ question: 'q', options: ['A', 'B'] })
    answerAsk('   ')
    expect(AnswerUserQuestion).not.toHaveBeenCalled()
    expect(cockpit.ask).not.toBeNull()
  })
})

describe('todo_write store flow', () => {
  it('applyTodos replaces the checklist wholesale', () => {
    applyTodos([
      { content: 'งานแรก', status: 'completed' },
      { content: 'งานสอง', status: 'in_progress' },
    ])
    expect(cockpit.todos).toHaveLength(2)
    applyTodos([])
    expect(cockpit.todos).toHaveLength(0)
  })

  it('non-array payloads clear instead of crashing', () => {
    applyTodos(undefined as never)
    expect(cockpit.todos).toEqual([])
  })
})
