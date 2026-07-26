import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  ListMCPServers, ToggleMCPServer, ListExternalSkills, UsageStats, ListPromptPresets,
  ListAgentProfiles, ListSubagentProfiles, ActiveAgent, SetActiveAgent, ReadAgentProfile,
  SetAgentProfileModel,
  ListModelsForProvider,
} from './mocks/wailsApp'

beforeEach(() => {
  vi.mocked(ListMCPServers).mockResolvedValue([
    { name: 'context7', command: ['npx', '-y', '@upstash/context7-mcp'], disabled: false, status: 'connected', tools: 2 },
    { name: 'exa', url: 'https://mcp.exa.ai/mcp', disabled: true, status: 'disabled', tools: 0 },
  ] as any)
  vi.mocked(ListExternalSkills).mockResolvedValue([
    { name: 'gridgeist', description: 'grid design', dir: 'C:/skills/gridgeist' },
  ] as any)
  vi.mocked(UsageStats).mockResolvedValue({
    today: [{ model: 'deepseek-chat', promptTokens: 1200, completionTokens: 340, calls: 5 }],
    week: [], all: [],
  } as any)
  vi.mocked(ListPromptPresets).mockResolvedValue([
    // Bundled presets ship cover art; a user preset may have none yet.
    { name: 'landing', description: 'สร้างแลนดิ้งเพจ', body: 'ทำแลนดิ้งเพจ $ARGUMENTS', path: '', builtin: true, image: 'data:image/svg+xml;base64,PHN2Zy8+' },
    { name: 'mine', description: 'ชุดคำสั่งของผม', body: 'ของผมเอง', path: 'C:/prompts/mine.md', builtin: false, image: '' },
  ] as any)
  // Two layers, two bindings — the page never receives one mixed list, and the
  // kind comes from which directory the engine found the file in.
  vi.mocked(ListAgentProfiles).mockResolvedValue([
    { name: 'build', description: 'ตัวลงมือทำ', kind: 'agent', prompt: 'role', builtin: true },
    { name: 'plan', description: 'เอเจนวางแผน', kind: 'agent', deny: ['write', 'edit'], prompt: 'role', builtin: true },
  ] as any)
  vi.mocked(ListSubagentProfiles).mockResolvedValue([
    { name: 'explore', description: 'ค้นไฟล์', kind: 'subagent', tools: ['grep', 'glob', 'list', 'read'], prompt: 'role', builtin: true },
    { name: 'backend', description: 'ของผม', kind: 'subagent', model: 'deepseek-v4', steps: 8, prompt: 'role', path: 'C:/subagents/backend.md', builtin: false },
  ] as any)
  vi.mocked(ActiveAgent).mockResolvedValue('build' as any)
  // No `mode:` in the file — the folder it came from is what says primary.
  vi.mocked(ReadAgentProfile).mockResolvedValue('---\ndescription: เอเจนวางแผน\ndeny: write, edit\n---\nYou are planning.' as any)
  vi.mocked(ListModelsForProvider).mockResolvedValue(['deepseek-v4', 'deepseek-chat'] as any)
})

const openSection = async (container: HTMLElement, label: string) => {
  const item = Array.from(container.querySelectorAll('.settings-nav-item'))
    .find((el) => el.textContent?.includes(label))
  if (!item) throw new Error(`nav item "${label}" not found`)
  await fireEvent.click(item)
}

describe('Settings pages', () => {
  it('MCP page lists servers with transport + tool badges and working toggle', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    // Server rows arrive async from ListMCPServers (presets render instantly
    // and also contain the names — assert on the badge only servers have).
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())
    expect(screen.getAllByText('http').length).toBeGreaterThan(0) // remote badge (exa)

    // Toggling the disabled server calls the binding with disabled=false.
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes.length).toBe(2) // one switch per server row
    await fireEvent.change(checkboxes[1]) // exa row (second server)
    await waitFor(() => expect(vi.mocked(ToggleMCPServer)).toHaveBeenCalledWith('exa', false))
  })

  it('Skills page lists discovered skills with their paths', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('gridgeist')).toBeTruthy())
    expect(screen.getByText('C:/skills/gridgeist')).toBeTruthy()
  })

  it('Usage page shows per-model aggregates', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สถิติการใช้งาน')
    await waitFor(() => expect(screen.getByText('deepseek-chat')).toBeTruthy())
    expect(screen.getByText('1,200')).toBeTruthy()
    expect(screen.getByText('340')).toBeTruthy()
  })

  it('Prompt presets page is a card gallery, badging the bundled ones', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ชุดคำสั่ง')

    await waitFor(() => expect(container.querySelectorAll('.pp-card').length).toBe(3)) // 2 presets + "new"
    expect(screen.getByText('สร้างแลนดิ้งเพจ')).toBeTruthy()
    expect(screen.getAllByText('มากับแอป')).toHaveLength(1)
    // Shipped cover renders as a real image; the one without falls back to the
    // generated cover rather than a broken <img>.
    expect(container.querySelectorAll('.pp-cover img').length).toBe(1)
    expect(container.querySelectorAll('.pp-cover .pp-mono').length).toBe(1)
  })

  it('clicking a preset card opens its full text for editing', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ชุดคำสั่ง')
    await waitFor(() => expect(container.querySelectorAll('.pp-card').length).toBe(3))

    const card = Array.from(container.querySelectorAll('.pp-card'))
      .find((el) => el.textContent?.includes('/landing'))!
    await fireEvent.click(card)

    const body = container.querySelector('.pp-textarea') as HTMLTextAreaElement
    expect(body).toBeTruthy()
    expect(body.value).toBe('ทำแลนดิ้งเพจ $ARGUMENTS')
    // A bundled preset says what saving will do rather than refusing the edit.
    expect(screen.getByText(/สร้างเป็นของคุณทับไว้/)).toBeTruthy()
    // Its name is fixed; a new preset is where you get to choose one.
    expect((container.querySelector('.pp-field input.ctrl') as HTMLInputElement).disabled).toBe(true)
  })

  // An empty 300px box tells you nothing about what belongs in it.
  it('a new preset opens on a starter skeleton, not a blank box', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ชุดคำสั่ง')
    await waitFor(() => expect(container.querySelector('.pp-new')).toBeTruthy())

    await fireEvent.click(container.querySelector('.pp-new')!)
    const body = container.querySelector('.pp-textarea') as HTMLTextAreaElement
    expect(body.value).toContain('$ARGUMENTS')
    expect(body.value.length).toBeGreaterThan(80)
    expect(body.placeholder).toBeTruthy()
    // The one token a preset cannot work without gets its own button.
    expect(screen.getByText('+ $ARGUMENTS')).toBeTruthy()
  })

  // §44.10: two cards from two bindings — an agent can be made active, a
  // sub-agent can only be spawned, and the page can never mix them up because it
  // never receives one list. Built-in vs yours is a badge.
  it('Agents page keeps the two layers apart and badges each row with its real config', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')

    await waitFor(() => expect(screen.getByText('เอเจนวางแผน')).toBeTruthy())
    const groups = container.querySelectorAll('.settings-card')
    expect(groups.length).toBe(2) // primary, sub-agents
    expect(groups[0].textContent).toContain('build')
    expect(groups[0].textContent).toContain('plan')
    expect(groups[1].textContent).toContain('explore')
    expect(groups[1].textContent).not.toContain('build')

    // The active profile says so and offers no "use" button; the other primary
    // one does. A sub-agent never offers it at all.
    expect(screen.getByText('กำลังใช้')).toBeTruthy()
    expect(screen.getAllByText('ใช้เอเจนนี้')).toHaveLength(1)

    // Badges are read off the profile, not written down: an empty tool list
    // means the whole registry, a 4-item list means 4.
    expect(screen.getAllByText('เครื่องมือครบ').length).toBe(3)
    expect(screen.getByText('เครื่องมือ 4 ตัว')).toBeTruthy()
    expect(screen.getByText('ปิด 2 ตัว')).toBeTruthy()   // plan's denials, visible on the row
    expect(screen.getByText('จำกัด 8 รอบ')).toBeTruthy() // the sub-agent's step cap
    expect(screen.getByText('ของคุณ')).toBeTruthy()
    expect(screen.getByText('C:/subagents/backend.md')).toBeTruthy()
    expect(screen.getByText('built-in:explore')).toBeTruthy()
  })

  it('picking a primary agent switches the engine; the model dropdown pins a model', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getByText('ใช้เอเจนนี้')).toBeTruthy())

    await fireEvent.click(screen.getByText('ใช้เอเจนนี้'))
    await waitFor(() => expect(vi.mocked(SetActiveAgent)).toHaveBeenCalledWith('plan'))

    // Every row carries a model select whose empty value means "inherit".
    const selects = Array.from(container.querySelectorAll('.set-row select.ctrl')) as HTMLSelectElement[]
    expect(selects.length).toBe(4)
    expect(selects[0].value).toBe('')
    await fireEvent.change(selects[0], { target: { value: 'deepseek-chat' } })
    await waitFor(() => expect(vi.mocked(SetAgentProfileModel)).toHaveBeenCalledWith('build', 'agent', 'deepseek-chat'))
  })

  it('editing a built-in agent opens its real file and says what saving does', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByText('แก้ไข').length).toBe(4))

    await fireEvent.click(screen.getAllByText('แก้ไข')[0])
    await waitFor(() => expect(container.querySelector('.ag-body')).toBeTruthy())
    const body = container.querySelector('.ag-body') as HTMLTextAreaElement
    expect(body.value).toContain('You are planning.')
    // Editable where ZCode is not — but honest about creating your own copy.
    expect(screen.getByText(/สร้างเป็นของคุณทับไว้/)).toBeTruthy()
    // A built-in has no delete button; there is nothing of yours to remove yet.
    expect(screen.queryByText('ลบ')).toBeNull()
  })

  it('a new agent opens on a frontmatter skeleton, not a blank box', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getByText('สร้างเอเจนใหม่')).toBeTruthy())

    await fireEvent.click(screen.getByText('สร้างเอเจนใหม่'))
    const body = container.querySelector('.ag-body') as HTMLTextAreaElement
    expect(body.value).toContain('description:')
    // No `mode:` line to mistype — the kind is a picker, saved as a folder.
    expect(body.value).not.toContain('mode:')
    expect((container.querySelectorAll('.pp-field select.ctrl')[0] as HTMLSelectElement).value).toBe('agent')
    // A new one gets to choose its name; an existing one does not.
    expect((container.querySelector('.pp-field input.ctrl') as HTMLInputElement).disabled).toBe(false)
  })
})
