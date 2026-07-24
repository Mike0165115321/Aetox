import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  ListMCPServers, ToggleMCPServer, ListExternalSkills, UsageStats, ListCustomCommands,
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
  vi.mocked(ListCustomCommands).mockResolvedValue([
    { name: 'review', description: 'Review helper', path: 'C:/cmd/review.md' },
  ] as any)
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

  it('Commands page lists custom commands with slash names', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'คำสั่ง')
    await waitFor(() => expect(screen.getByText('/review')).toBeTruthy())
    expect(screen.getByText('Review helper')).toBeTruthy()
  })
})
