// The engine picker in the composer of ระบบออโตเมชั่น.
//
// Three things it has to do, and one of them is the reason it exists at all: a
// connection reaches an agent only when placed on it BY NAME (silence is not a
// grant for an agent, deliberately), so the ordinary path — connect n8n, walk
// into the room — produces a specialist holding no tools. That state was
// invisible until this chip was drawn on one engine rather than only on two.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { EnginesFor, UseEngine, VerifyConnection } from './mocks/wailsApp'

// The props Chat cannot render without. Nothing here is what this file is
// about — it is the scaffolding every Chat test carries.
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

const engine = (over: Record<string, unknown> = {}) => ({
  id: 'n8n', label: 'n8n', kind: 'token', connected: true, env_override: false,
  configured: true, tools: ['n8n_workflow_list'], family: 'automation',
  needs_base_url: true, base_url: 'http://localhost:5678',
  for: [] as string[],
  ...over,
})

const chip = () =>
  Array.from(document.querySelectorAll('.focus-chip.focus-btn'))
    .find((b) => /n8n|Windmill|ยังไม่ได้เลือก|ยังไม่มีเครื่องมือ/.test(b.textContent ?? '')) as HTMLButtonElement | undefined

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.chair = 'automation'
  cockpit.desk = 'specialized'
  cockpit.activeView = 'chat'
  vi.mocked(EnginesFor).mockResolvedValue([engine()] as never)
  vi.mocked(VerifyConnection).mockResolvedValue({ login: 'localhost:5678' } as never)
})

describe('the automation engine picker', () => {
  // The state this control was hidden in TWICE — first below two engines, then
  // below one connected one. Both times the reasoning was "there is nothing to
  // choose between", and both times the result was that the person with nothing
  // set up lost the only thing on screen that could tell them so.
  //
  // A control that vanishes in the broken state is not a tidy UI. It is a dead
  // end, and this is the test that says so out loud.
  it('stays on screen when no engine is connected at all, and leads to Settings', async () => {
    vi.mocked(EnginesFor).mockResolvedValue([
      engine({ connected: false, base_url: '' }),
      engine({ id: 'windmill', label: 'Windmill', connected: false, base_url: '' }),
    ] as never)
    render(Chat, baseProps as never)

    await waitFor(() => expect(chip()).toBeTruthy())
    expect(chip()!.textContent).toContain('ยังไม่มีเครื่องมือ')
    expect(chip()!.className).toContain('unset')

    // And the menu is a way out rather than a list of dead options.
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('จัดการการเชื่อมต่อ')).toBeTruthy())
    expect(screen.getAllByText('ยังไม่ได้เชื่อม').length).toBe(2)
  })

  // Picking one that is not connected must not place it — the agent would be
  // handed tools that fail on their first call. It opens Settings instead.
  it('sends you to Settings instead of placing an engine you have not connected', async () => {
    vi.mocked(EnginesFor).mockResolvedValue([engine({ connected: false, base_url: '' })] as never)
    render(Chat, baseProps as never)

    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)
    await waitFor(() => expect(screen.getByText('n8n')).toBeTruthy())
    await fireEvent.click(screen.getByText('n8n'))

    expect(vi.mocked(UseEngine)).not.toHaveBeenCalled()
  })

  // Drawn on one engine, not two. The shell picker's "a choice of one is
  // furniture" rule was copied here and was wrong: this chip is also the answer
  // to "what will this agent touch", and the unplaced state has no other cure.
  it('warns when an engine is connected but not yet given to the agent', async () => {
    render(Chat, baseProps as never)

    await waitFor(() => expect(chip()).toBeTruthy())
    expect(chip()!.textContent).toContain('ยังไม่ได้เลือก')
    expect(chip()!.className).toContain('unset')
  })

  it('shows the engine once it is placed on the agent', async () => {
    vi.mocked(EnginesFor).mockResolvedValue([engine({ for: ['agent:automation'] })] as never)
    render(Chat, baseProps as never)

    await waitFor(() => expect(chip()?.textContent).toContain('n8n'))
    expect(chip()!.className).not.toContain('unset')
  })

  it('hands the choice to the engine gate rather than to a setting of its own', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)

    await waitFor(() => expect(screen.getByText('n8n')).toBeTruthy())
    await fireEvent.click(screen.getByText('n8n'))

    await waitFor(() =>
      expect(vi.mocked(UseEngine)).toHaveBeenCalledWith('automation', 'automation', 'n8n'))
  })

  // A key that worked when it was pasted can stop working with nothing on this
  // screen changing — n8n's carry an expiry the user chose at creation. The menu
  // is where somebody is standing when they start to doubt it.
  it('tests the engine without switching to it, and says what happened', async () => {
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)

    const test = await waitFor(() =>
      document.querySelector('.focus-row .icobtn') as HTMLButtonElement)
    await fireEvent.click(test)

    await waitFor(() => expect(vi.mocked(VerifyConnection)).toHaveBeenCalledWith('n8n'))
    await waitFor(() => expect(document.querySelector('.conn-test.ok')).toBeTruthy())
    // Testing is not choosing: a user checking whether the other engine still
    // answers must not be switched onto it.
    expect(vi.mocked(UseEngine)).not.toHaveBeenCalled()
  })

  it('shows the refusal in the engine own words when it fails', async () => {
    vi.mocked(VerifyConnection).mockRejectedValueOnce(new Error('n8n ปฏิเสธ API key นี้'))
    render(Chat, baseProps as never)
    await waitFor(() => expect(chip()).toBeTruthy())
    await fireEvent.click(chip()!)

    const test = await waitFor(() =>
      document.querySelector('.focus-row .icobtn') as HTMLButtonElement)
    await fireEvent.click(test)

    await waitFor(() => expect(screen.getByText(/ปฏิเสธ API key/)).toBeTruthy())
    expect(document.querySelector('.conn-test.ok')).toBeNull()
  })

  // The chip belongs to one room. Every other chat would be showing a control
  // for an agent it is not talking to.
  it('is not drawn outside the automation agent chat', async () => {
    cockpit.chair = ''
    cockpit.desk = 'assistant'
    render(Chat, baseProps as never)

    await waitFor(() => expect(vi.mocked(EnginesFor)).not.toHaveBeenCalled())
    expect(chip()).toBeUndefined()
  })
})
