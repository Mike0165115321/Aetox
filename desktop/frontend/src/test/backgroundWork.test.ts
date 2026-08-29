import { describe, it, expect, beforeEach } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
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
  tokensIn: 0, tokensOut: 0, cachedIn: 0, cacheReported: false,
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
      tasks: [task()], steps, onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
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
      onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    const cards = container.querySelectorAll('.bgw-card')
    expect(cards).toHaveLength(2)
    expect(cards[0].textContent).toContain('read skill.go')
    expect(cards[0].textContent).not.toContain('README.md')
    expect(cards[1].textContent).toContain('read README.md')
  })

  // The brake. Until it existed the only door to a running sub-agent was the
  // composer's Stop, which is only on screen while a turn is live — and a
  // delegate deliberately outlives the turn that started it. So the ordinary
  // case, work dispatched and the turn finished, left up to four sub-agents
  // looping with nothing on screen that could reach them.
  it('offers a brake on each running delegate, naming that one', async () => {
    const stopped: string[] = []
    const { container } = render(BackgroundWork, {
      tasks: [task({ id: 'task_1', agent: 'research' }), task({ id: 'task_2', agent: 'doc' })],
      steps: [], onAnswer: () => {}, onStop: (id: string) => stopped.push(id), onStopRun: () => {}, onStopQueue: () => {},
    })
    const buttons = [...container.querySelectorAll<HTMLButtonElement>('.bgw-stop')]
    expect(buttons).toHaveLength(2)
    buttons[1].click()
    // The id, not the index: two delegates are the whole reason a per-row brake
    // had to exist, and a button that stopped the wrong one would be worse than
    // no button.
    expect(stopped).toEqual(['task_2'])
  })

  it('brakes a delegate that is parked on a question too', () => {
    const stopped: string[] = []
    const { container } = render(BackgroundWork, {
      tasks: [task({ state: 'waiting', question: 'เอาแบบไหน' })],
      steps: [], onAnswer: () => {}, onStop: (id: string) => stopped.push(id), onStopRun: () => {}, onStopQueue: () => {},
    })
    // It is spending nothing this second, but it holds one of the four slots
    // and the user may simply not want the answer any more.
    container.querySelector<HTMLButtonElement>('.bgw-stop')?.click()
    expect(stopped).toEqual(['task_1'])
  })

  it('leaves a receipt for stopped work, and says who ended it', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ state: 'stopped', toolCalls: 72, tokensIn: 41200, tokensOut: 830 })],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    const card = container.querySelector('.bgw-card')
    // Not 'ไปต่อไม่ได้': work somebody ended is not work that broke, and the
    // row has to say which of the two happened.
    expect(card?.textContent).toContain('คุณสั่งหยุดไว้')
    expect(card?.textContent).not.toContain('ไปต่อไม่ได้')
    // The counts are the point of a receipt: how far it got, and what it had
    // already cost by the time the brake reached it.
    expect(card?.textContent).toContain('72')
    expect(card?.textContent).toContain('41.2k')
    // Nothing left to stop, so nothing offering to.
    expect(container.querySelector('.bgw-stop')).toBeNull()
  })

  it('shows what a running delegate has read and written, not one lump', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ tokensIn: 12300, tokensOut: 840 })],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    const meta = container.querySelector('.bgw-meta')?.textContent ?? ''
    // Two numbers because they are two different problems: input climbing is a
    // transcript being re-sent every round, output climbing is a model that
    // will not stop writing, and the brake is a different decision for each.
    expect(meta).toContain('12.3k')
    expect(meta).toContain('840')
  })

  it('says nothing about spend before the first round comes back', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ tokensIn: 0, tokensOut: 0 })],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    // "เข้า 0 · ออก 0" reads as a measurement. There has not been one yet.
    expect(container.querySelector('.bgw-meta')?.textContent).not.toContain('เข้า')
  })

  it('puts a parked delegate’s question and a box to answer it on the card', () => {
    const { container, getByPlaceholderText } = render(BackgroundWork, {
      tasks: [task({ state: 'waiting', question: 'แก้ทั้งสองฉบับ หรือฉบับไทยก่อน' })],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    expect(container.querySelector('.bgw-question')?.textContent).toContain('ฉบับไทยก่อน')
    expect(getByPlaceholderText('พิมพ์คำตอบให้ explore…')).toBeTruthy()
  })

  // A collected result is in the conversation now; a card still offering it
  // would be a second copy of the same answer.
  it('drops a collected row', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ state: 'done', collected: true })], steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    expect(container.querySelector('.bgw-card')).toBeNull()
  })

  // Finished but uncollected is a receipt, not a control: the store's poll has
  // already sent the turn that reads the result.
  it('reports a finished delegation without asking for a click', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ state: 'done' })], steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
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
      tasks: [], runs: [run()], allTasks: [], steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
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
      tasks: [], runs: [run()], allTasks: inRun, steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
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
      tasks: [], runs: [run()], allTasks: [asking], steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
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
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    expect(container.querySelector('.bgw-phase')).toBeNull()
  })
})

// Nothing refuses a fan-out any more (internal/subagent/runner.go): four run and
// the rest wait, however many were asked for. So the tray has to be able to say
// "and 196 more" without becoming 196 cards, and it has to offer one brake for
// the line — a queue you can only cancel a row at a time is not one anybody can
// stop, and the user would be clicking while it drains into the bill.
describe('the queue behind the four that are running', () => {
  const queued = (n: number) =>
    Array.from({ length: n }, (_, i) =>
      task({ id: `task_q${i}`, agent: 'explore', state: 'queued', toolCalls: 0 }),
    )

  it('folds the waiting jobs into one row with a count', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ id: 'task_1' }), ...queued(6)],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    // One card for the delegate that is working, and none for the six that are
    // not — they are a line, not six things happening.
    expect(container.querySelectorAll('.bgw-card.is-queued').length).toBe(0)
    expect(container.querySelector('.bgw-queue-count')?.textContent).toContain('6')
  })

  it('opens the line when asked, and closes it again', async () => {
    const { container } = render(BackgroundWork, {
      tasks: queued(3),
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    const head = container.querySelector('.bgw-queue-head') as HTMLButtonElement
    expect(head.getAttribute('aria-expanded')).toBe('false')

    await fireEvent.click(head)
    expect(container.querySelectorAll('.bgw-card.is-queued').length).toBe(3)

    await fireEvent.click(head)
    expect(container.querySelectorAll('.bgw-card.is-queued').length).toBe(0)
  })

  // The brake that stands where the ceiling used to. One press, the whole line.
  it('offers one brake for the whole line', async () => {
    let cleared = 0
    const { container } = render(BackgroundWork, {
      tasks: [task({ id: 'task_1' }), ...queued(4)],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {},
      onStopQueue: () => { cleared++ },
    })
    const stop = container.querySelector('.bgw-queue .bgw-stop') as HTMLButtonElement
    await fireEvent.click(stop)
    expect(cleared).toBe(1)
  })

  // And it is not drawn when there is nothing waiting, which is every ordinary
  // turn: a row saying "0 more" is a row that teaches people to stop reading.
  it('says nothing when nothing is waiting', () => {
    const { container } = render(BackgroundWork, {
      tasks: [task({ id: 'task_1' })],
      steps: [], onAnswer: () => {}, onStop: () => {}, onStopRun: () => {}, onStopQueue: () => {},
    })
    expect(container.querySelector('.bgw-queue')).toBeNull()
  })
})
