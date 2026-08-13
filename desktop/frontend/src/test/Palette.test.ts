import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Palette from '../lib/Palette.svelte'
import { ListPromptPresets, ToolCounts } from './mocks/wailsApp'

const noop = () => {}

beforeEach(() => {
  vi.mocked(ListPromptPresets).mockResolvedValue([
    { name: 'landing', description: 'แลนดิ้งเพจไฟล์เดียว', path: '', builtin: true },
    { name: 'review', description: 'รีวิวหาบั๊กจริง', path: '', builtin: true },
  ] as any)
  vi.mocked(ToolCounts).mockResolvedValue({ builtin: 22, workbench: 6, skill: 1, mcp: 3 } as any)
})

describe('Composer palette', () => {
  it('"/" mode lists presets and picking one writes it into the message', async () => {
    const inserted: string[] = []
    render(Palette, {
      mode: 'prompts', oninsert: (s: string) => inserted.push(s),
      onclose: noop, onopenmodel: noop, onswitchthink: noop,
    })

    await waitFor(() => expect(screen.getByText('/landing')).toBeTruthy())
    expect(screen.getByText('แลนดิ้งเพจไฟล์เดียว')).toBeTruthy()

    await fireEvent.click(screen.getByText('/landing'))
    // A trailing space: the user types their arguments straight after.
    expect(inserted).toEqual(['/landing '])
  })

  it('"/" mode shows only prompts — nothing that acts on the app', async () => {
    render(Palette, { mode: 'prompts', oninsert: noop, onclose: noop, onopenmodel: noop, onswitchthink: noop })
    await waitFor(() => expect(screen.getByText('/landing')).toBeTruthy())
    expect(screen.queryByText('โมเดล')).toBeNull()
    expect(screen.queryByText('คีย์ลัด')).toBeNull()
  })

  it('"+" mode adds the app actions, the live tool counts, and the shortcuts', async () => {
    const { container } = render(Palette, {
      mode: 'all', oninsert: noop, onclose: noop, onopenmodel: noop, onswitchthink: noop,
    })

    // Counts land a tick after the presets — wait for the row that needs them.
    await waitFor(() => expect(screen.getByText('เครื่องมือ 28 · สกิล 1 · MCP 3')).toBeTruthy())
    expect(screen.getByText('สลับโมเดล…')).toBeTruthy()
    expect(screen.getByText('ระดับการอนุมัติ')).toBeTruthy()
    // The only place the app tells anyone its shortcuts exist.
    expect(screen.getByText('Ctrl+,')).toBeTruthy()

    // Status rows must not look or behave like buttons.
    const toolsRow = screen.getByText('เครื่องมือ 28 · สกิล 1 · MCP 3').closest('.pal-row')
    expect(toolsRow?.classList.contains('static')).toBe(true)
    expect(container.querySelectorAll('.pal-row').length).toBeGreaterThan(4)
  })

  it('typing filters across labels and values', async () => {
    const { container } = render(Palette, {
      mode: 'all', oninsert: noop, onclose: noop, onopenmodel: noop, onswitchthink: noop,
    })
    await waitFor(() => expect(screen.getByText('/landing')).toBeTruthy())

    await fireEvent.input(container.querySelector('.pal-search')!, { target: { value: 'landing' } })
    await waitFor(() => expect(screen.queryByText('สลับโมเดล…')).toBeNull())
    expect(screen.getByText('/landing')).toBeTruthy()
    expect(screen.queryByText('/review')).toBeNull()
  })

  it('Escape closes', async () => {
    let closed = false
    const { container } = render(Palette, {
      mode: 'prompts', oninsert: noop, onclose: () => (closed = true), onopenmodel: noop, onswitchthink: noop,
    })
    await fireEvent.keyDown(container.querySelector('.pal-search')!, { key: 'Escape' })
    expect(closed).toBe(true)
  })
})
