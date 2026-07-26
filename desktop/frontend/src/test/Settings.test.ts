import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  ListMCPServers, ToggleMCPServer, ListExternalSkills, UsageStats, ListPromptPresets,
  ListSubagentProfiles, ReadSubagentProfile, SetSubagentModel, ListModelsForProvider,
} from './mocks/wailsApp'

// The chart plots a window ending today, so a hard-coded date would fall out
// of it and the fixture would stop covering the chart the day after it was
// written. Local, not toISOString: the component keys its columns by local day,
// and in a +07 zone the UTC date is yesterday for the first seven hours.
const d = new Date()
const today = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

beforeEach(() => {
  vi.mocked(ListMCPServers).mockResolvedValue([
    { name: 'context7', command: ['npx', '-y', '@upstash/context7-mcp'], disabled: false, status: 'connected', tools: 2 },
    { name: 'exa', url: 'https://mcp.exa.ai/mcp', disabled: true, status: 'disabled', tools: 0 },
  ] as any)
  vi.mocked(ListExternalSkills).mockResolvedValue([
    { name: 'gridgeist', description: 'grid design', dir: 'C:/skills/gridgeist' },
  ] as any)
  // Two models on purpose: one hosted (reports cache accounting) and one local
  // (reports none), which is the pair the page has to render differently.
  const deepseek = {
    model: 'deepseek-chat', promptTokens: 1200, completionTokens: 340,
    cachedTokens: 900, uncachedTokens: 300, cacheRows: 5, calls: 5,
  }
  const ollama = {
    model: 'ornith:9b', promptTokens: 400, completionTokens: 60,
    cachedTokens: 0, uncachedTokens: 400, cacheRows: 0, calls: 2,
  }
  vi.mocked(UsageStats).mockResolvedValue({
    today: [deepseek], week: [deepseek, ollama], all: [deepseek, ollama],
    // Today, so the 30-day window always contains it. Both models on the same
    // day: one splits into hit/miss, the local one cannot and must stay whole.
    daily: [
      { day: today, model: 'deepseek-chat', promptTokens: 1200, completionTokens: 340, cachedTokens: 900, cacheRows: 5 },
      { day: today, model: 'ornith:9b', promptTokens: 400, completionTokens: 60, cachedTokens: 0, cacheRows: 0 },
    ],
    heatmap: [{ day: today, model: '', promptTokens: 1200, completionTokens: 340, cachedTokens: 900, cacheRows: 5 }],
    totals: {
      promptTokens: 1600, completionTokens: 400, cachedTokens: 900, uncachedTokens: 700,
      cacheRows: 5, calls: 7, sessions: 3, messages: 21,
      activeDays: 2, currentStreak: 2, topModel: 'deepseek-chat', topModelShare: 77,
    },
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
    const row = await waitFor(() => {
      const found = container.querySelector('.usage-row')
      expect(found).toBeTruthy()
      return found!
    })
    expect(row.querySelector('.u-model')?.textContent).toContain('deepseek-chat')
    const nums = [...row.querySelectorAll('.u-num')].map((n) => n.textContent?.trim())
    expect(nums[0]).toBe('1,200') // input
    expect(nums[2]).toBe('340') // output
  })

  // A provider that reports no cache accounting must render an em dash, not
  // 0%: zero hits and no cache to hit are different claims, and only one of
  // them is the provider's.
  it('Usage page separates a measured cache rate from an unreported one', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สถิติการใช้งาน')

    const rows = await waitFor(() => {
      const found = container.querySelectorAll('.usage-row')
      expect(found.length).toBe(2)
      return found
    })
    const cacheCell = (row: Element) => row.querySelectorAll('.u-num')[1]
    // 900 of 1,200 input tokens reused.
    expect(cacheCell(rows[0]).textContent).toContain('75%')
    expect(cacheCell(rows[1]).textContent?.trim()).toBe('—')

    // The headline cards summarise the same split: 900 cached of 1,600 input.
    const cards = [...container.querySelectorAll('.stat-card')]
    const cacheCard = cards.find((c) => c.textContent?.includes('แคช prompt'))!
    expect(cacheCard.querySelector('.stat-big')?.textContent?.replace(/\s/g, '')).toBe('56%')
    expect(container.querySelector('.stat-model')?.textContent).toBe('deepseek-chat')
  })

  // Plotting only the days that have data turns a month of usage into a handful
  // of fat blocks and silently rescales the x-axis, so a 4-day-old install and
  // a 30-day-old one look identical. Every day in the window gets a column; the
  // empty ones are the point.
  it('Usage chart plots every day in the window, not only the days with data', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สถิติการใช้งาน')

    const columns = await waitFor(() => {
      const found = container.querySelectorAll('.daycol')
      expect(found.length).toBeGreaterThan(0)
      return found
    })
    // The mock carries a single day of usage.
    expect(columns.length).toBe(30)
    expect(container.querySelectorAll('.daycol.idle').length).toBe(29)
    // Gridlines give the bars a scale to be read against.
    expect(container.querySelectorAll('.chart-gridline').length).toBe(5)
    // Axis ticks are rounded, not raw maxima.
    expect(container.querySelector('.chart-y')?.textContent).toContain('0')
  })

  // The idle-day modifier was once called .empty, which also matched the page's
  // .empty utility (padding:16px). Twenty-six padded columns ate the whole
  // track and the days that had data came out half a pixel wide — bars present
  // in the DOM, invisible on screen, and no jsdom test could see it because
  // jsdom does not apply the stylesheet. Guard the name instead.
  it('idle columns do not carry the page-level .empty utility', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สถิติการใช้งาน')
    await waitFor(() => expect(container.querySelectorAll('.daycol').length).toBe(30))
    expect(container.querySelectorAll('.daycol.empty').length).toBe(0)
  })

  // Hue is the model, fill is where the tokens came from. A model that reports
  // no cache accounting cannot be split, and drawing its input as all-miss
  // would claim a measurement the provider never made — it gets its own band.
  it('Usage chart splits each day into cache hit, miss and output', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สถิติการใช้งาน')

    const bar = await waitFor(() => {
      const found = container.querySelector('.daycol:not(.idle) .daybar')
      expect(found).toBeTruthy()
      return found!
    })
    const segs = [...bar.querySelectorAll('span')].map((s) => s.className)
    // deepseek: 900 hit + 300 miss + 340 out. ornith: 400 unsplittable + 60 out.
    expect(segs).toEqual(['k-hit s1', 'k-miss s1', 'k-raw s2', 'k-out s1', 'k-out s2'])
    const flex = (cls: string) =>
      Number((bar.querySelector(`.${cls.replace(' ', '.')}`) as HTMLElement).style.flex.split(' ')[0])
    expect(flex('k-hit s1')).toBe(900)
    expect(flex('k-miss s1')).toBe(300)
    expect(flex('k-raw s2')).toBe(400)
  })

  // The period control swaps which aggregate the table renders.
  it('Usage page switches period', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สถิติการใช้งาน')
    await waitFor(() => expect(container.querySelectorAll('.usage-row').length).toBe(2))

    const today = [...container.querySelectorAll('.seg-btn')].find((b) => b.textContent?.includes('วันนี้'))!
    await fireEvent.click(today)
    await waitFor(() => expect(container.querySelectorAll('.usage-row').length).toBe(1))
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
