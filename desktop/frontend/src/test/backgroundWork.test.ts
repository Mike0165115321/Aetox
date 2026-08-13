import { describe, it, expect, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import BackgroundWork from '../lib/BackgroundWork.svelte'
import { setLocale } from '../lib/i18n.svelte'
import type { BackgroundTask, ToolStep } from '../lib/types'

// The card that replaced a status strip (§105). The strip was accurate and read
// as dead work — the owner saw "ใช้ 1 เครื่องมือ · 1s" frozen under a finished
// turn and called it: "ถ้ามันทำงาน ควรจะไม่นิ่งแบบนี้". So what these tests
// pin is the thing that makes it look alive, and the two joins it depends on.

const task = (over: Partial<BackgroundTask> = {}): BackgroundTask => ({
  id: 'task_1', agent: 'explore', label: 'สรุปไฟล์ .go',
  startedAt: new Date().toISOString(), toolCalls: 12,
  state: 'running', collected: false, ...over,
})

const step = (over: Partial<ToolStep> = {}): ToolStep => ({
  label: 'read a.go', state: 'done', startedAt: 0, task: 'task_1', ...over,
})

beforeEach(() => setLocale('th'))

describe('the background work card', () => {
  it('shows the last few steps of the delegation, which is what says "alive"', () => {
    const steps = ['a.go', 'b.go', 'c.go', 'd.go'].map((f, i) =>
      step({ label: `read ${f}`, ref: `r${i}` }),
    )
    const { container } = render(BackgroundWork, {
      tasks: [task()], steps, onAnswer: () => {},
    })
    const rows = [...container.querySelectorAll('.tool-step .lbl')].map((el) => el.textContent)
    // Three, newest last: forty rows would bury the conversation under it.
    expect(rows).toEqual(['read b.go', 'read c.go', 'read d.go'])
  })

  // Steps carry the delegation's id because `parent` is the provider's call id
  // — a different namespace from the register's. Without the join, two
  // delegates running at once would each show the other's work.
  it('keeps two delegations’ steps apart', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ id: 'task_1', agent: 'explore' }), task({ id: 'task_2', agent: 'doc' })],
      steps: [
        step({ task: 'task_1', label: 'read skill.go', ref: 'r1' }),
        step({ task: 'task_2', label: 'read README.md', ref: 'r2' }),
      ],
      onAnswer: () => {},
    })
    const cards = container.querySelectorAll('.bgw-card')
    expect(cards).toHaveLength(2)
    expect(cards[0].textContent).toContain('read skill.go')
    expect(cards[0].textContent).not.toContain('README.md')
    expect(cards[1].textContent).toContain('read README.md')
  })

  it('puts a parked delegate’s question and a box to answer it on the card', () => {
    const { container, getByPlaceholderText } = render(BackgroundWork, {
      tasks: [task({ state: 'waiting', question: 'แก้ทั้งสองฉบับ หรือฉบับไทยก่อน' })],
      steps: [], onAnswer: () => {},
    })
    expect(container.querySelector('.bgw-question')?.textContent).toContain('ฉบับไทยก่อน')
    expect(getByPlaceholderText('พิมพ์คำตอบให้ explore…')).toBeTruthy()
  })

  // A collected result is in the conversation now; a card still offering it
  // would be a second copy of the same answer.
  it('drops a collected row', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ state: 'done', collected: true })], steps: [], onAnswer: () => {},
    })
    expect(container.querySelector('.bgw-card')).toBeNull()
  })

  // Finished but uncollected is a receipt, not a control: the store's poll has
  // already sent the turn that reads the result.
  it('reports a finished delegation without asking for a click', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ state: 'done' })], steps: [], onAnswer: () => {},
    })
    expect(container.querySelector('.bgw-card.is-done')).toBeTruthy()
    expect(container.querySelectorAll('button')).toHaveLength(0)
  })
})
