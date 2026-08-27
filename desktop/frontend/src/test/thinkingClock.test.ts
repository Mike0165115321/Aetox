// The live thinking row counts, and the finished one it turns into agrees.
//
// It was the one live row in the product with nothing moving on it. A running
// tool has said "· 12s" for a long time and the finished bubble says "Thought
// for 34s"; between them sat the longest wait in the app, static, because
// `liveStatus` deliberately blanks the moment reasoning starts (it would be
// duplicating the toggle right below it, and would stop being true). That
// decision was right. Reading as a hang was the unintended half.
//
// A number rather than a pulse: a pulse only says the process is alive, while a
// clock says whether the wait is still worth it.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit, applyReasoningChunk, liveThinkSecs } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'

const baseProps = {
  task: { title: '', steps: [] } as any,
  awaitingReply: true,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  messages: [] as any[],
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  cockpit.turnSession = 'sess-1'
  cockpit.reasoningText = ''
})

describe('the live thinking row carries a clock', () => {
  it('counts from the first reasoning chunk', () => {
    vi.useFakeTimers()
    try {
      vi.setSystemTime(new Date('2026-08-27T00:00:00Z'))
      applyReasoningChunk('thinking...')
      const start = Date.now()

      // Rounded, and floored at 1: a turn that has begun thinking has thought
      // for at least a second as far as anyone reading the row is concerned,
      // and "0s" reads as broken rather than as fast.
      expect(liveThinkSecs(start)).toBe(1)
      expect(liveThinkSecs(start + 12_000)).toBe(12)
      expect(liveThinkSecs(start + 247_000)).toBe(247)
    } finally {
      vi.useRealTimers()
    }
  })

  it('says nothing for a turn that is not thinking', () => {
    cockpit.turnSession = 'a-session-that-never-reasoned'
    expect(liveThinkSecs(Date.now())).toBeUndefined()
  })

  it('shows the count on the row, and the plain word before any of it', async () => {
    vi.useFakeTimers()
    try {
      vi.setSystemTime(new Date('2026-08-27T00:00:00Z'))
      applyReasoningChunk('hmm')
      vi.setSystemTime(new Date('2026-08-27T00:00:09Z'))

      // A message, because with none of them Chat draws the empty room
      // rather than a transcript, and the live bubble lives in the transcript.
      const { container } = render(Chat, {
        ...baseProps,
        messages: [{ role: 'user', text: 'hi', time: '10:00' }] as any,
        reasoningText: 'hmm',
      })
      const toggle = container.querySelector('.typing-bubble .meta-row .reasoning-toggle')
      expect(toggle?.textContent).toContain('Thinking 9s')
      // The word alone is the fallback, never the steady state.
      expect(toggle?.textContent).not.toBe('Thinking')
    } finally {
      vi.useRealTimers()
    }
  })
})

// The claim the whole design leans on: the live row counts UP TO the number
// that replaces it. Two different formulas here would make the row jump at the
// exact moment the user is reading it.
describe('live and finished agree', () => {
  it('uses the same arithmetic the finished label is built from', () => {
    vi.useFakeTimers()
    try {
      vi.setSystemTime(new Date('2026-08-27T00:00:00Z'))
      applyReasoningChunk('a')
      const start = Date.now()
      vi.setSystemTime(new Date('2026-08-27T00:00:34Z'))
      applyReasoningChunk('b') // the last chunk: what turnArtifacts measures to

      const finished = Math.max(1, Math.round((Date.now() - start) / 1000))
      expect(liveThinkSecs(Date.now())).toBe(finished)
      expect(finished).toBe(34)
    } finally {
      vi.useRealTimers()
    }
  })
})
