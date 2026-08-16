import { describe, it, expect, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import BackgroundWork from '../lib/BackgroundWork.svelte'
import { setLocale } from '../lib/i18n.svelte'
import type { BackgroundPhase, BackgroundRun, BackgroundTask, ToolStep } from '../lib/types'

// The card that replaced a status strip (§105). The strip was accurate and read
// as dead work — the owner saw "ใช้ 1 เครื่องมือ · 1s" frozen under a finished
// turn and called it: "ถ้ามันทำงาน ควรจะไม่นิ่งแบบนี้". So what these tests
// pin is the thing that makes it look alive, and the two joins it depends on.

const task = (over: Partial<BackgroundTask> = {}): BackgroundTask => ({
  id: 'task_1', agent: 'explore', label: 'สรุปไฟล์ .go',
  startedAt: new Date().toISOString(), toolCalls: 12, tokens: 0,
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


// A declared run (internal/subagent/run.go). What the card has to get right is
// the phase nobody has worked in: it was promised before the work existed, and
// drawing it only once somebody gets round to it would make a skipped round
// invisible — which is the whole reason runs exist.
describe('a declared run', () => {
  const phase = (over: Partial<BackgroundPhase> = {}): BackgroundPhase => ({
    title: 'รอบตรวจ', planned: 3, done: 0, failed: 0, running: 0, waiting: 0, tokens: 0, ...over,
  })
  const run = (over: Partial<BackgroundRun> = {}): BackgroundRun => ({
    id: 'run_1', name: 'ตรวจ SKILL.md ให้ตรงกับโค้ด', brief: 'กางข้อกล่าวอ้างออกทีละข้อ',
    startedAt: new Date().toISOString(), running: true, tokens: 0,
    phases: [phase({ done: 2, running: 1 }), phase({ title: 'รอบหักล้าง', planned: 2 })],
    ...over,
  })

  it('draws a phase that has not happened yet, at zero of what it promised', () => {
    const { container } = render(BackgroundWork, {
      tasks: [], runs: [run()], allTasks: [], steps: [], onAnswer: () => {},
    })
    const titles = [...container.querySelectorAll('.bgw-phase-title')].map((el) => el.textContent)
    expect(titles).toEqual(['รอบตรวจ', 'รอบหักล้าง'])
    const counts = [...container.querySelectorAll('.bgw-phase-count')].map((el) => el.textContent?.trim())
    expect(counts).toEqual(['2/3', '0/2'])
  })

  it('groups its workers under their own phase, collected ones included', () => {
    const inRun = [
      task({ id: 'task_1', run: 'run_1', phase: 'รอบตรวจ', state: 'done', collected: true, label: 'ที่เก็บข้อมูล' }),
      task({ id: 'task_2', run: 'run_1', phase: 'รอบตรวจ', state: 'running', label: 'สกิล' }),
    ]
    const { container } = render(BackgroundWork, {
      tasks: [], runs: [run()], allTasks: inRun, steps: [], onAnswer: () => {},
    })
    const labels = [...container.querySelectorAll('.bgw-worker-label')].map((el) => el.textContent)
    expect(labels).toEqual(['ที่เก็บข้อมูล', 'สกิล'])
  })

  // A worker in a run that stops to ask is answered where it stands. A second
  // card for the same worker would say the group is not the whole job.
  it('answers a parked worker inside the group', () => {
    const asking = task({
      id: 'task_9', run: 'run_1', phase: 'รอบตรวจ', state: 'waiting',
      question: 'ให้ถือว่าไฟล์ผิด หรือคนละเรื่องกัน?',
    })
    const { container } = render(BackgroundWork, {
      tasks: [], runs: [run()], allTasks: [asking], steps: [], onAnswer: () => {},
    })
    expect(container.querySelector('.bgw-worker-ask')?.textContent).toContain('คนละเรื่องกัน')
  })

  // Once every worker has been read back, the group has said all it can — the
  // same rule a single row follows.
  it('leaves when the whole run has been collected', () => {
    const done = run({ running: false })
    const { container } = render(BackgroundWork, {
      tasks: [],
      runs: [done],
      allTasks: [task({ id: 'task_1', run: 'run_1', phase: 'รอบตรวจ', state: 'done', collected: true })],
      steps: [], onAnswer: () => {},
    })
    expect(container.querySelector('.bgw-phase')).toBeNull()
  })
})
