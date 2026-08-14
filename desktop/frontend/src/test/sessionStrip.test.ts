// สรุปห้อง — what this conversation holds, behind a button in the window corner.
//
// It was built onto the composer first and moved here on the owner's call: the
// composer is where things the room pushes at you belong, and this is a thing
// you go and look at. What the corner costs is that the button says the same
// words whatever is behind it, so the badge is the part that has to work — it
// is the only thing telling anyone there is a plan running at all.
//
// The three sections are readings of things that already exist (todo_write,
// tool_runs, git), so what is guarded here is not the data but the reading:
// that an empty section says so instead of vanishing, that a truncated list
// admits how truncated it is, and that no two rows can read the same.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import SessionStrip from '../lib/SessionStrip.svelte'
import { cockpit, applyTodos } from '../lib/stores/cockpit.svelte'
import { SessionSources, SessionSourceCount, GitChangedFiles } from './mocks/wailsApp'

const plan = (...rows: [string, 'pending' | 'in_progress' | 'completed'][]) =>
  rows.map(([content, status]) => ({ content, status }))

const source = (over: Record<string, unknown> = {}) => ({
  kind: 'file', label: 'notes.md', path: 'D:/work/notes.md', time: '', ...over,
})

const toggle = () => screen.getByRole('button', { name: /สรุปห้องนี้/ })

/** Open the panel and wait for the disk reads it fires on the way in. */
async function openPanel() {
  await fireEvent.click(toggle())
  await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
}

describe('the session summary button', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    cockpit.todos = []
    cockpit.project = { ...cockpit.project, focused: false, branch: '' }
  })

  // The corner is always the same corner — the reason a control that moves
  // costs a search on every use (the note on the + button beside it).
  it('is there in a room with nothing in it', () => {
    render(SessionStrip)

    expect(toggle().getAttribute('aria-expanded')).toBe('false')
  })

  it('opens and closes the panel', async () => {
    render(SessionStrip)

    await openPanel()
    expect(toggle().getAttribute('aria-expanded')).toBe('true')

    await fireEvent.click(toggle())
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  // Reading the store and shelling out to git are both disk work. A panel
  // nobody has opened has no business doing either.
  it('touches the disk only once it is opened', async () => {
    render(SessionStrip)
    expect(SessionSources).not.toHaveBeenCalled()
    expect(GitChangedFiles).not.toHaveBeenCalled()

    await openPanel()
    expect(SessionSources).toHaveBeenCalled()
    expect(GitChangedFiles).toHaveBeenCalled()
  })

  // Every section draws its heading whether or not it has anything, because in
  // a panel somebody opened on purpose "there is nothing" is the answer they
  // came for — and the heading teaches what will appear there later.
  it('names all three sections and says which are empty', async () => {
    render(SessionStrip)

    await openPanel()
    for (const heading of ['แผน', 'แหล่งที่มา', 'ที่เก็บโค้ด']) {
      expect(screen.getByText(heading)).toBeTruthy()
    }
    expect(screen.getByText('ยังไม่มีแผนในห้องนี้')).toBeTruthy()
    expect(screen.getByText('ห้องนี้ยังไม่ได้เปิดไฟล์หรือเว็บอะไร')).toBeTruthy()
    expect(screen.getByText('ห้องนี้ไม่ได้โฟกัสโปรเจกต์')).toBeTruthy()
  })

  it('lists the plan with each row in its own state', async () => {
    applyTodos(plan(
      ['อ่าน schema เดิม', 'completed'],
      ['เขียน migration v8', 'in_progress'],
      ['ต่อ UI', 'pending'],
    ))
    const { container } = render(SessionStrip)

    await openPanel()
    expect([...container.querySelectorAll('.todo-item')].map((e) => e.className)).toEqual([
      'todo-item completed', 'todo-item in_progress', 'todo-item pending',
    ])
  })

  it('lists what the room read', async () => {
    vi.mocked(SessionSources).mockResolvedValue([
      source({ kind: 'url', label: 'example.invalid/docs', path: 'https://example.invalid/docs' }),
      source(),
    ])
    vi.mocked(SessionSourceCount).mockResolvedValue(2)
    render(SessionStrip)

    await openPanel()
    await waitFor(() => expect(screen.getByText('example.invalid/docs')).toBeTruthy())
    expect(screen.getByText('notes.md')).toBeTruthy()
  })

  // The folder is only ever present because two labels collided, so it has to
  // be shown — a row that shortened away the one thing distinguishing it is the
  // failure this section exists to prevent.
  it('shows the folder on rows whose names collide', async () => {
    vi.mocked(SessionSources).mockResolvedValue([
      source({ label: 'code.html', path: 'D:/w/site/code.html', dir: 'D:/w/site' }),
      source({ label: 'code.html', path: 'D:/w/docs/code.html', dir: 'D:/w/docs' }),
    ])
    vi.mocked(SessionSourceCount).mockResolvedValue(2)
    const { container } = render(SessionStrip)

    await openPanel()
    await waitFor(() => expect(container.querySelectorAll('.summary-row .dir')).toHaveLength(2))
    expect([...container.querySelectorAll('.summary-row .dir')].map((e) => e.textContent))
      .toEqual(['D:/w/site', 'D:/w/docs'])
  })

  // "View all" with no number says the list is incomplete without saying how
  // incomplete, which is a different decision for the person reading it.
  it('says how many rows it is not showing', async () => {
    vi.mocked(SessionSources).mockResolvedValue(
      Array.from({ length: 10 }, (_, i) => source({ label: `f${i}.md`, path: `D:/w/f${i}.md` })),
    )
    vi.mocked(SessionSourceCount).mockResolvedValue(31)
    render(SessionStrip)

    await openPanel()
    const more = await screen.findByText(/ดูอีก 25 รายการ/)
    await fireEvent.click(more)
    expect(screen.getByText('f9.md')).toBeTruthy()
  })

  it('shows the branch and the files that changed under it', async () => {
    cockpit.project = { ...cockpit.project, focused: true, branch: 'feat/session-strip' }
    vi.mocked(GitChangedFiles).mockResolvedValue([
      { path: 'desktop/sources.go', status: 'U' },
      { path: 'desktop/frontend/src/style.css', status: 'M' },
    ] as never)
    render(SessionStrip)

    await openPanel()
    await waitFor(() => expect(screen.getByText('feat/session-strip')).toBeTruthy())
    expect(screen.getByText('desktop/sources.go')).toBeTruthy()
    expect(screen.getByText('desktop/frontend/src/style.css')).toBeTruthy()
  })

  // Behind a glyph that never changes, the badge is the only thing that can say
  // work is under way without being opened. It counts what is LEFT: a 30px
  // button has room for one number, and that is the one that changes what you
  // do next.
  it('counts what is left on the button while the plan is unfinished', () => {
    applyTodos(plan(
      ['อ่าน schema เดิม', 'completed'],
      ['เขียน migration v8', 'in_progress'],
      ['ต่อ UI', 'pending'],
    ))
    const { container } = render(SessionStrip)

    expect(container.querySelector('.summary-badge')?.textContent).toBe('2')
  })

  // A finished plan is a record, and a badge that never clears is one nobody
  // reads — it has to mean "something is running", or it means nothing.
  it('drops the badge once every row is struck through', () => {
    applyTodos(plan(['อ่าน schema เดิม', 'completed'], ['เขียน migration v8', 'completed']))
    const { container } = render(SessionStrip)

    expect(container.querySelector('.summary-badge')).toBeNull()
  })

  it('carries no badge in a room with no plan at all', () => {
    const { container } = render(SessionStrip)

    expect(container.querySelector('.summary-badge')).toBeNull()
  })

  // Making the plan outlive its turn took away the only thing that ever cleared
  // it, and nothing replaced it: an abandoned checklist sat there for the rest
  // of the session with no way to put it down. This list is written only by the
  // model, so the person who abandoned it is the only one who can say so.
  it('lets the user put down a plan that is not going to be finished', async () => {
    applyTodos(plan(['เขียน migration v8', 'in_progress'], ['ต่อ UI', 'pending']))
    const { container } = render(SessionStrip)

    await openPanel()
    expect(screen.getByText('เขียน migration v8')).toBeTruthy()

    await fireEvent.click(container.querySelector('.summary-clear')!)
    expect(screen.queryByText('เขียน migration v8')).toBeNull()
    expect(screen.getByText('ยังไม่มีแผนในห้องนี้')).toBeTruthy()
    expect(container.querySelector('.summary-badge')).toBeNull()
  })

  // Nothing to put down, nothing to offer — a control that clears an empty list
  // is a button that does nothing.
  it('offers nothing to clear when there is no plan', async () => {
    const { container } = render(SessionStrip)

    await openPanel()
    expect(container.querySelector('.summary-clear')).toBeNull()
  })
})
