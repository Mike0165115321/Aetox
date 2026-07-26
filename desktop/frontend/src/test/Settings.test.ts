import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  ListMCPServers, ToggleMCPServer, ListExternalSkills, UsageStats, ListPromptPresets,
  ListSubagentProfiles, ReadSubagentProfile, SetSubagentModel, ListModelsForProvider,
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
  vi.mocked(ListSubagentProfiles).mockResolvedValue([
    { name: 'explore', description: 'ค้นไฟล์', tools: ['grep', 'glob', 'list', 'read'], prompt: 'role', builtin: true },
    { name: 'general', description: 'งานซ้ำ', prompt: 'role', builtin: true },
    { name: 'backend', description: 'ของผม', model: 'deepseek-v4', steps: 8, prompt: 'role', path: 'C:/subagents/backend.md', builtin: false },
    // A file of yours that shadows a bundled one: counts as yours, and its
    // delete button has to read as a revert.
    { name: 'mine-explore', description: 'ของผมทับ', prompt: 'role', path: 'C:/subagents/mine-explore.md', builtin: false, overrides: true },
  ] as any)
  vi.mocked(ReadSubagentProfile).mockResolvedValue('---\ndescription: ค้นไฟล์\ntools: grep, read\n---\nYou search files.' as any)
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

  // §44.0: there is no agent picker — the main agent is the assistant. This page
  // manages only who it delegates to, split by who wrote each profile (§44.10).
  it('Sub-agents page splits yours from the ones that ship with Aetox', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')

    await waitFor(() => expect(screen.getByText('ค้นไฟล์')).toBeTruthy())
    const cards = container.querySelectorAll('.settings-card')
    expect(cards.length).toBe(2)
    // Yours first: a fresh install has only built-ins, and the list you grow is
    // the interesting one.
    expect(cards[0].textContent).toContain('ของคุณ')
    expect(cards[0].textContent).toContain('backend')
    expect(cards[0].textContent).toContain('mine-explore')
    expect(cards[0].textContent).not.toContain('general')
    expect(cards[1].textContent).toContain('มากับแอป')
    expect(cards[1].textContent).toContain('explore')
    expect(cards[1].textContent).toContain('general')
    expect(cards[1].textContent).not.toContain('backend')

    // A shadow sits under yours and says what it is, because deleting it reverts.
    expect(screen.getByText('ทับของแอป')).toBeTruthy()

    // Badges are read off the profile, not written down.
    expect(screen.getByText('เครื่องมือ 4 ตัว')).toBeTruthy()
    expect(screen.getAllByText('เครื่องมือครบ').length).toBe(3)
    expect(screen.getByText('จำกัด 8 รอบ')).toBeTruthy()          // backend's own steps
    expect(screen.getAllByText('จำกัด 24 รอบ').length).toBe(3)    // the rest fall back to the cap
    expect(screen.getByText('C:/subagents/backend.md')).toBeTruthy()
    expect(screen.getByText('built-in:explore')).toBeTruthy()

    // No row offers to become the agent you talk to — that concept is gone.
    expect(screen.queryByText('ใช้เอเจนนี้')).toBeNull()
    expect(screen.queryByText('กำลังใช้')).toBeNull()
  })

  it('the model dropdown pins a model per sub-agent', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')
    await waitFor(() => expect(container.querySelectorAll('.set-row select.ctrl').length).toBe(4))

    const selects = Array.from(container.querySelectorAll('.set-row select.ctrl')) as HTMLSelectElement[]
    expect(selects[0].value).toBe('deepseek-v4') // backend is pinned
    await fireEvent.change(selects[0], { target: { value: '' } })
    await waitFor(() => expect(vi.mocked(SetSubagentModel)).toHaveBeenCalledWith('backend', ''))
  })

  it('editing a built-in sub-agent opens its real file and says what saving does', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')
    await waitFor(() => expect(screen.getAllByText('แก้ไข').length).toBe(4))

    // The built-in group's first row (explore) — index 2 overall: yours come first.
    await fireEvent.click(screen.getAllByText('แก้ไข')[2])
    await waitFor(() => expect(container.querySelector('.ag-body')).toBeTruthy())
    const body = container.querySelector('.ag-body') as HTMLTextAreaElement
    expect(body.value).toContain('You search files.')
    // Editable where ZCode is not — but honest about creating your own copy.
    expect(screen.getByText(/สร้างเป็นของคุณทับไว้/)).toBeTruthy()
    // A built-in has no delete button; there is nothing of yours to remove yet.
    expect(screen.queryByText('ลบ')).toBeNull()
    expect(screen.queryByText('คืนค่าของแอป')).toBeNull()
  })

  // Deleting a shadow restores the bundled profile, so the button must not say
  // "delete" — the row is not going away.
  it('a shadow offers to revert, not to delete', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')
    await waitFor(() => expect(screen.getAllByText('แก้ไข').length).toBe(4))

    await fireEvent.click(screen.getAllByText('แก้ไข')[1]) // mine-explore, the shadow
    await waitFor(() => expect(screen.getByText('คืนค่าของแอป')).toBeTruthy())
    expect(screen.queryByText('ลบ')).toBeNull()
  })

  it('a new sub-agent opens on a frontmatter skeleton, not a blank box', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')
    await waitFor(() => expect(screen.getByText('สร้างซับเอเจนใหม่')).toBeTruthy())

    await fireEvent.click(screen.getByText('สร้างซับเอเจนใหม่'))
    const body = container.querySelector('.ag-body') as HTMLTextAreaElement
    expect(body.value).toContain('description:')
    expect(body.value).toContain('steps:')
    // No mode/kind key to mistype: there is only one kind of profile now.
    expect(body.value).not.toContain('mode:')
    expect(body.value).not.toContain('kind:')
    // A new one gets to choose its name; an existing one does not.
    expect((container.querySelector('.pp-field input.ctrl') as HTMLInputElement).disabled).toBe(false)
  })
})
