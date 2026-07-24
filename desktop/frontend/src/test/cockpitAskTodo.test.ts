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
    expect(AnswerUserQuestion).toHaveBeenCalledWith('B')
    expect(cockpit.ask).toBeNull()
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
