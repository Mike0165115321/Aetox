// The branch chip in the composer's context row.
//
// It drew the focused project's branch from the day project focus existed, and
// it drew it as a `<span>`. That is the whole bug this covers: the chip answered
// "where am I" and had nothing to say to the question anybody who reads a branch
// name asks next, while looking exactly like the chips beside it that do open.
//
// What these pin is the half that is not cosmetic — that a refused switch leaves
// the chip telling the truth. git refuses to move when the working tree holds
// changes the switch would overwrite, and a picker that optimistically drew the
// branch it was clicked on would be lying about where the repository is.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { GitBranches, GitSwitchBranch, GitCreateBranch, GetProjectStatus } from './mocks/wailsApp'

const baseProps = {
  task: { title: '', steps: [] } as never,
  messages: [] as never,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as never,
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as never,
}

const chip = () => document.querySelector('.branch-chip') as HTMLButtonElement | null

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.activeView = 'chat'
  Object.assign(cockpit.project, { focused: true, name: 'Aetox', path: 'D:/Aetox', branch: 'main' })
  vi.mocked(GitBranches).mockResolvedValue([
    { name: 'main', current: true },
    { name: 'Dev', current: false },
    { name: 'feature/new-payment-system', current: false },
  ] as never)
  vi.mocked(GetProjectStatus).mockResolvedValue({ focused: true, name: 'Aetox', branch: 'main' } as never)
})

describe('the branch chip', () => {
  // The bug, stated as a test: it has to be something you can press. A <span>
  // passes every visual review and fails the only interaction anybody tries.
  it('is a button, not a label', async () => {
    render(Chat, baseProps as never)

    await waitFor(() => expect(chip()).toBeTruthy())
    expect(chip()!.tagName).toBe('BUTTON')
    expect(chip()!.textContent).toContain('main')
  })

  it('opens the list of branches, current one marked', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())

    await fireEvent.click(chip()!)

    await waitFor(() => expect(screen.getByText('Dev')).toBeTruthy())
    expect(screen.getByText('feature/new-payment-system')).toBeTruthy()
    expect(document.querySelector('.branch-item.on')?.textContent).toContain('main')
  })

  it('narrows the list as you type', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('Dev')).toBeTruthy())

    const search = document.querySelector('.branch-search input') as HTMLInputElement
    await fireEvent.input(search, { target: { value: 'payment' } })

    await waitFor(() => expect(document.querySelectorAll('.branch-item:not(.create)').length).toBe(1))
    expect(screen.getByText('feature/new-payment-system')).toBeTruthy()
  })

  it('switches to the branch that was clicked', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('Dev')).toBeTruthy())

    await fireEvent.click(screen.getByText('Dev'))

    await waitFor(() => expect(GitSwitchBranch).toHaveBeenCalledWith('Dev'))
    // The menu closes on success, which is how the user knows it took.
    await waitFor(() => expect(document.querySelector('.branch-menu')).toBeNull())
  })

  // The one that matters. git refuses when the switch would overwrite work the
  // user has not committed, and its refusal names the files — so it is shown as
  // it arrived, the menu stays open, and the chip keeps saying where the
  // repository actually is.
  it('shows git\u2019s refusal and stays where it was', async () => {
    vi.mocked(GitSwitchBranch).mockRejectedValue(
      new Error('error: Your local changes to the following files would be overwritten by checkout:\n\tdesktop/app.go'),
    )
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('Dev')).toBeTruthy())

    await fireEvent.click(screen.getByText('Dev'))

    await waitFor(() => expect(document.querySelector('.branch-error')).toBeTruthy())
    expect(document.querySelector('.branch-error')!.textContent).toContain('desktop/app.go')
    expect(document.querySelector('.branch-menu')).toBeTruthy()
    expect(chip()!.textContent).toContain('main')
  })

  // The row that started life hidden until a name had been typed — which put
  // the only way to make a branch behind already knowing it was there, the same
  // shape of bug as the chip that was a <span>. It is on screen from the moment
  // the menu opens, exactly as it is in the editors this was modelled on.
  it('offers to create a branch before anything is typed', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())

    await fireEvent.click(chip()!)

    const create = await waitFor(() => {
      const el = document.querySelector('.branch-item.create') as HTMLButtonElement | null
      expect(el).toBeTruthy()
      return el!
    })
    expect(create.disabled).toBe(false)
    // With an empty box it is the way in rather than a dead click: it puts the
    // cursor where the name goes.
    await fireEvent.click(create)
    expect(document.activeElement).toBe(document.querySelector('.branch-search input'))
    expect(GitCreateBranch).not.toHaveBeenCalled()
  })

  it('creates the branch once a new name is typed', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('Dev')).toBeTruthy())

    const search = document.querySelector('.branch-search input') as HTMLInputElement
    await fireEvent.input(search, { target: { value: 'release/1.0.0' } })
    await waitFor(() => expect(document.querySelector('.branch-item.create')!.textContent).toContain('release/1.0.0'))

    await fireEvent.click(document.querySelector('.branch-item.create')!)
    await waitFor(() => expect(GitCreateBranch).toHaveBeenCalledWith('release/1.0.0'))
  })

  // Disabled rather than removed for a name that is taken. A row that vanishes
  // as you type reads as the menu glitching; a dim one with its reason on hover
  // answers the question the user was about to ask.
  it('refuses, without disappearing, when the name is already a branch', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('Dev')).toBeTruthy())

    const search = document.querySelector('.branch-search input') as HTMLInputElement
    await fireEvent.input(search, { target: { value: 'Dev' } })

    const create = await waitFor(() => {
      const el = document.querySelector('.branch-item.create') as HTMLButtonElement
      expect(el.disabled).toBe(true)
      return el
    })
    expect(create.getAttribute('title')).toBeTruthy()
    expect(GitCreateBranch).not.toHaveBeenCalled()
  })

  // Nothing to switch when there is no project: the chip is not drawn at all,
  // which is the behaviour it already had as a label.
  it('is absent without a focused project', async () => {
    Object.assign(cockpit.project, { focused: false, branch: '' })
    render(Chat, baseProps as never)

    await waitFor(() => expect(document.querySelector('.composer')).toBeTruthy())
    expect(chip()).toBeNull()
  })
})
