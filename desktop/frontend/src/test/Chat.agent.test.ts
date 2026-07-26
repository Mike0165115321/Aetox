import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, PrimaryAgents, SetActiveAgent } from './mocks/wailsApp'

// Which agent is answering has to be visible without opening anything: it
// decides the role AND which tools exist, so `plan` refusing to edit is only
// explicable if the composer says `plan` (ARCHITECTURE.md §44).
const baseProps = {
  messages: [] as any[],
  task: { title: '', steps: [] } as any,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '', agent: 'plan' } as any,
}

beforeEach(() => {
  setLocale('en')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
  vi.mocked(PrimaryAgents).mockResolvedValue(['assistant', 'code', 'plan'] as any)
})

describe('composer agent readout', () => {
  it('names the active agent on the model chip, ahead of the model', () => {
    const { container } = render(Chat, baseProps)
    const chip = container.querySelector('.model-chip')!
    expect(chip.querySelector('.agent')?.textContent).toBe('plan')
    // Reading order is agent → model → think level.
    expect(chip.textContent?.replace(/\s+/g, ' ')).toContain('plan v4 high')
  })

  it('omits the badge rather than showing a blank one before the engine answers', () => {
    const { container } = render(Chat, { ...baseProps, model: { ...baseProps.model, agent: '' } as any })
    expect(container.querySelector('.model-chip .agent')).toBeNull()
  })

  it('the chip menu switches agent through the same picker as provider/model', async () => {
    const { container, getByText } = render(Chat, baseProps)
    await fireEvent.click(container.querySelector('.model-chip')!)

    // Agent is the first row of the menu it already opened — one readout, not two.
    const rows = container.querySelectorAll('.model-menu .mm-row')
    expect(rows[0].textContent).toContain('Agent')
    expect(rows[0].textContent).toContain('plan')

    await fireEvent.click(rows[0].querySelector('.updrop-trigger')!)
    await fireEvent.click(getByText('code'))
    await waitFor(() => expect(vi.mocked(SetActiveAgent)).toHaveBeenCalledWith('code'))
  })
})
