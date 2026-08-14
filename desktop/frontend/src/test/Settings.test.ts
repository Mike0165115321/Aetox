import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  ListMCPServers, ToggleMCPServer, ListExternalSkills, UsageStats, ListPromptPresets,
  ListSubagentProfiles, ReadSubagentProfile, SaveSubagentProfile, SetSubagentModel, ListModelsForProvider,
  ListSpeechModels, SetSpeechModel, ListTools, SpeechModelDirs, RevealSpeechModel,
  SignInMethods, SignInStatus, StartSignIn, CompleteSignIn, SupportedProviders, EnabledProviders,
  RemoveMCPServer, RemoveExternalSkill, SetProviderEnabled, TerminalShells,
  SkillsDir, SkillScanIssues, OpenSkillsFolder, InstallSkillFromZip,
  MCPConfigPath, OpenMCPFolder, SaveMCPServer, AppVersion, CheckForUpdate, ListChairs, SaveAgentProfile,
  AgentSkills, AgentNeeds, PlacementTargets, SetMCPServerTargets,
  ChairStarters, SaveChairStarters,
  Connections, ConnectAccount, SetConnectionTargets, VerifyConnection, DisconnectAccount,
  AcceptsAPIKey, ProviderAPIKeyURL, ProviderReady, PriceModels,
} from './mocks/wailsApp'
import { BrowserOpenURL } from './mocks/wailsRuntime'
import { applyTypeScale } from '../lib/typeScale.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'

// The chart plots a window ending today, so a hard-coded date would fall out
// of it and the fixture would stop covering the chart the day after it was
// written. Local, not toISOString: the component keys its columns by local day,
// and in a +07 zone the UTC date is yesterday for the first seven hours.
const d = new Date()
const today = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

beforeEach(() => {
  cockpit.settingsIntent = null
  vi.mocked(ListMCPServers).mockResolvedValue([
    // `allowed` is the trim: the server offers more than this and only these
    // are taken (§97.3). Kept on a row that already exists so the counts the
    // other MCP tests assert on do not move.
    { name: 'context7', command: ['npx', '-y', '@upstash/context7-mcp'], disabled: false, status: 'connected', tools: 2, allowed: ['resolve-library-id', 'get-library-docs'] },
    { name: 'exa', url: 'https://mcp.exa.ai/mcp', disabled: true, status: 'disabled', tools: 0 },
  ] as any)
  vi.mocked(ListExternalSkills).mockResolvedValue([
    { name: 'gridgeist', description: 'grid design', dir: 'C:/skills/gridgeist' },
  ] as any)
  // A desktop tool and a built-in one: both belong on the Tools page, and
  // neither is anything the user installed.
  vi.mocked(ListTools).mockResolvedValue([
    { name: 'browser_open', description: 'open a page', source: 'workbench', category: 'web' },
    { name: 'read', description: 'read a file', source: 'builtin', category: 'files' },
    { name: 'audio_transcribe', description: 'transcribe audio', source: 'builtin', category: 'media' },
  ] as any)
  // One in Aetox's own folder, one already sitting in Ollama's — the case the
  // picker exists for, since neither is reachable without naming a full path.
  vi.mocked(SpeechModelDirs).mockResolvedValue([
    { path: 'C:/aetox/models', label: '%APPDATA%/aetox/models' },
    { path: 'C:/Users/x/.ollama/models', label: '~/.ollama/models' },
  ] as any)
  vi.mocked(ListSpeechModels).mockResolvedValue([
    { path: 'C:/aetox/models/ggml-tiny-q5_1.bin', name: 'ggml-tiny-q5_1.bin', sizeMB: 31, store: 'Aetox', managed: true, active: false },
    { path: 'C:/Users/x/.ollama/models/ggml-base.bin', name: 'ggml-base.bin', sizeMB: 141, store: 'Ollama', managed: false, active: true },
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
  // The helpers (explore/general) are system-fixed; everything editable is an
  // agent, so the editable fixtures are chairs — a built-in, one of yours, and
  // a shadow whose delete button has to read as a revert.
  vi.mocked(ListSubagentProfiles).mockResolvedValue([
    { name: 'explore', description: 'ค้นไฟล์', tools: ['grep', 'glob', 'list', 'read'], prompt: 'role', builtin: true },
    { name: 'general', description: 'งานซ้ำ', prompt: 'role', builtin: true },
    { name: 'deck', description: 'ทำสไลด์', prompt: 'role', builtin: true, desk: 'specialized' },
    { name: 'backend', description: 'ของผม', model: 'deepseek-v4', steps: 8, prompt: 'role', path: 'C:/agents/backend.md', builtin: false, desk: 'specialized' },
    { name: 'mine-deck', description: 'ของผมทับ', prompt: 'role', path: 'C:/agents/mine-deck.md', builtin: false, overrides: true, desk: 'specialized' },
  ] as any)
  vi.mocked(ListChairs).mockResolvedValue([{ name: 'deck' }, { name: 'backend' }, { name: 'mine-deck' }] as any)
  // Where a server or an account can be pointed. Re-set every test because one
  // of them replaces it — an override that survived would land on the
  // connections tests, which assert this exact list.
  vi.mocked(PlacementTargets).mockResolvedValue([
    { id: 'assistant', name: 'ผู้ช่วย', kind: 'desk' },
    { id: 'coding', name: 'โค้ด', kind: 'desk' },
    { id: 'agent:researcher', name: 'researcher', kind: 'agent' },
  ] as any)
  vi.mocked(ReadSubagentProfile).mockResolvedValue('---\ndescription: ค้นไฟล์\ntools: grep, read\n---\nYou search files.' as any)
  vi.mocked(ListModelsForProvider).mockResolvedValue(['deepseek-v4', 'deepseek-chat'] as any)
})

// A provider you sign into rather than paste a key for. OpenRouter is the only
// one Aetox ships (§66) and it is the browser flow: Aetox opens the provider's
// page and waits for the redirect.
const seedSignIn = (method: Record<string, unknown> = {}, prompt: Record<string, unknown> = {}) => {
  const provider = (method.provider as string) ?? 'openrouter'
  vi.mocked(SupportedProviders).mockResolvedValue([provider] as any)
  vi.mocked(EnabledProviders).mockResolvedValue([provider] as any)
  vi.mocked(SignInMethods).mockResolvedValue([{
    provider, label: 'OpenRouter', kind: 'browser', risk: 'open',
    note: 'Published OAuth flow. Mints an API key you own and can revoke.',
    ...method,
  }] as any)
  vi.mocked(SignInStatus).mockResolvedValue({ provider, signed_in: false } as any)
  vi.mocked(StartSignIn).mockResolvedValue({
    provider, kind: 'browser', url: 'https://openrouter.ai/auth',
    ...prompt,
  } as any)
}

// Two branches of the sign-in UI that no shipped method produces: the device
// code (Qwen was the last, §65) and the restricted-risk warning (§70 cleared
// the last one when ChatGPT came back without it). Both are the contract a
// future sign-in arrives into — the warning especially, since §70's whole point
// is that it fires on evidence and must still fire when there is some — so they
// keep their coverage on a synthetic method rather than losing it.
const seedDeviceSignIn = () => seedSignIn(
  { provider: 'example-device', label: 'Example', kind: 'device' },
  {
    provider: 'example-device', kind: 'device',
    url: 'https://example.test/activate',
    verification_uri: 'https://example.test/activate',
    user_code: 'ABCD-1234',
  },
)

const seedRestrictedSignIn = () => seedSignIn(
  { provider: 'example-restricted', label: 'Example', risk: 'restricted' },
  { provider: 'example-restricted', url: 'https://example.test/authorize' },
)

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

  // The shelf answered "what is this server" and never "why am I being shown
  // it, and is it for me". Both lines are here now, and the second is derived:
  // an agent's `needs:` is where that fact is decided, so the shelf reads it
  // rather than restating it — otherwise an agent edited to drop a server keeps
  // being advertised for it from a list nobody remembers to update.
  it('says why each preset is recommended and which agent asked for it', async () => {
    vi.mocked(ListSubagentProfiles).mockResolvedValue([
      { name: 'research', description: 'หาข้อมูล', prompt: 'role', builtin: true, desk: 'specialized', needs: ['mcp:firecrawl'] },
      { name: 'deck', description: 'ทำสไลด์', prompt: 'role', builtin: true, desk: 'specialized' },
      { name: 'automation', description: 'ออโต', prompt: 'role', builtin: true, desk: 'specialized', needs: ['connection:n8n | mcp:windmill'] },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')

    // The reason, in the row, for the entry this test is about.
    await waitFor(() => expect(container.textContent).toContain('walks a whole site'))

    // And who asked for it — from the profile, not from the preset list.
    const row = await waitFor(() => {
      const el = Array.from(container.querySelectorAll('.set-row'))
        .find((r) => r.querySelector('.t')?.textContent?.trim().startsWith('firecrawl'))
      if (!el) throw new Error('the firecrawl preset row is not on the page')
      return el
    })
    expect(row.querySelector('.mcp-wanted')?.textContent).toContain('research')

    // A preset nobody declared stays quiet rather than showing an empty label.
    const exa = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.querySelector('.t')?.textContent?.trim().startsWith('exa'))
    expect(exa?.querySelector('.mcp-wanted')).toBeFalsy()
  })

  // The add form is eight controls for something done rarely, so it is closed
  // until asked for (owner, 2026-08-14: it was "เรี่ยราด" laid out permanently
  // under the list).
  //
  // The trap this pins is the second half: the same form is what แก้ไข and a
  // key-needing preset open. A fold that only knew about its own button would
  // leave those two clicking into nothing, silently — which is worse than the
  // sprawl it replaced.
  it('keeps the add-server form closed until something asks for it', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())

    const nameBox = () => container.querySelector('input[placeholder="ชื่อ เช่น context7"]')
    expect(nameBox()).toBeNull()

    // The way in sits in the list's header, beside the folder button.
    await fireEvent.click(screen.getByText('เพิ่ม SERVER'))
    await waitFor(() => expect(nameBox()).toBeTruthy())

    // And it can be shut again — a panel that opens and will not close is the
    // thing the fold was meant to avoid, not a new version of it.
    await fireEvent.click(screen.getByText('ยกเลิก'))
    await waitFor(() => expect(nameBox()).toBeNull())

    // แก้ไข opens the very same form, filled in.
    await fireEvent.click(screen.getAllByText('แก้ไข')[0])
    await waitFor(() => expect(nameBox()).toBeTruthy())
    expect((nameBox() as HTMLInputElement).value).toBe('context7')
  })

  // The allowlist is the one MCP field that is destructive when it round-trips
  // wrong: a form that shows it blank and saves that blank silently widens the
  // server back out to everything it offers. So both directions are pinned —
  // editing shows what is stored, and saving sends it back.
  it('editing a server shows its tool allowlist and saves it back', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())

    const edit = screen.getAllByText('แก้ไข')[0] // context7's row
    await fireEvent.click(edit)

    const box = await waitFor(() => {
      const el = container.querySelector<HTMLTextAreaElement>('textarea[placeholder="บรรทัดละหนึ่งชื่อ"]')
      if (!el) throw new Error('the allowlist box is not on the form')
      return el
    })
    expect(box.value).toBe('resolve-library-id\nget-library-docs')

    await fireEvent.input(box, { target: { value: 'resolve-library-id\n' } })
    await fireEvent.click(screen.getByText('บันทึก'))

    await waitFor(() => expect(vi.mocked(SaveMCPServer)).toHaveBeenCalled())
    const [original, sent] = vi.mocked(SaveMCPServer).mock.calls.at(-1) as [string, any]
    expect(original).toBe('context7')
    // Trimmed, blanks dropped, and sent as an array rather than omitted — the
    // engine reads an absent field as "say nothing" and keeps what it has.
    expect(sent.tools).toEqual(['resolve-library-id'])
  })

  it('Skills page lists discovered skills with their paths', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('gridgeist')).toBeTruthy())
    expect(screen.getByText('C:/skills/gridgeist')).toBeTruthy()
  })

  // The two are different things and now get different pages. Mixing them on
  // one page is what made every tool read as a "skill" the user had installed.
  it('tools and skills are separate pages, neither showing the other', async () => {
    const { container } = render(Settings, { onClose: () => {} })

    await openSection(container, 'เครื่องมือ')
    await waitFor(() => expect(screen.getByText('browser_open')).toBeTruthy())
    expect(screen.queryByText('gridgeist')).toBeNull()

    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('gridgeist')).toBeTruthy())
    expect(screen.queryByText('browser_open')).toBeNull()
  })

  // Models come from three different tools' folders and their paths are long,
  // so the picker has to show where each one lives and hand the engine the
  // exact path — a name alone could not tell two ggml-base.bin apart.
  // An MCP server writes its own tool descriptions and some run to a paragraph.
  // One line each keeps 39 rows scannable; the full text is one click away.
  it('tool descriptions show one line until the row is clicked', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เครื่องมือ')

    await waitFor(() => expect(screen.getByText('browser_open')).toBeTruthy())
    const row = Array.from(container.querySelectorAll('.tool-row'))
      .find((r) => r.textContent?.includes('browser_open'))!
    expect(row.querySelector('.d')!.classList.contains('clamp')).toBe(true)

    await fireEvent.click(row)
    expect(row.querySelector('.d')!.classList.contains('clamp')).toBe(false)

    await fireEvent.click(row)
    expect(row.querySelector('.d')!.classList.contains('clamp')).toBe(true)
  })

  // Grouped by what a tool is FOR, not by where it came from. Source sorts
  // forty-four rows by an implementation detail and answers a question nobody
  // asks; "which of these does the assistant need to carry?" could not be asked
  // from the old page at all, because it could not be read.
  it('tools are grouped by what they are for, not by where they came from', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เครื่องมือ')

    await waitFor(() => expect(screen.getByText('browser_open')).toBeTruthy())
    const heads = Array.from(container.querySelectorAll('.group-head')).map((h) => h.textContent)
    // browser_open is web, read is files — two tools that used to be one group
    // ("built in" / "desktop") and are two abilities.
    expect(heads.some((h) => h?.includes('เว็บ'))).toBe(true)
    expect(heads.some((h) => h?.includes('ไฟล์'))).toBe(true)
    // The old axis is gone: nothing is filed under where it was compiled.
    expect(heads.some((h) => h?.includes('ในตัว') || h?.includes('เดสก์ท็อป'))).toBe(false)
    // Heading and count still sit above the card, not inside it.
    expect(container.querySelector('.settings-card .group-head')).toBeNull()
  })

  // The picker hangs off audio_transcribe's own row: a tool's setting sitting in
  // a card somewhere else is a setting nobody ties back to the tool.
  it('the speech model is picked from audio_transcribe’s own row', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เครื่องมือ')

    await waitFor(() => expect(screen.getByText('audio_transcribe')).toBeTruthy())
    // Closed until asked — the tool list stays one row per tool.
    expect(screen.queryByText('ggml-tiny-q5_1.bin')).toBeNull()

    const toolRow = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('audio_transcribe'))!
    await fireEvent.click(toolRow.querySelector('.ctrl')!)

    await waitFor(() => expect(screen.getByText('ggml-tiny-q5_1.bin')).toBeTruthy())
    expect(screen.getByText('31 MB · Aetox')).toBeTruthy()
    expect(screen.getByText('141 MB · Ollama')).toBeTruthy()

    // A dropdown over the page, not an expanding section: the rows below
    // audio_transcribe must not move when it opens.
    expect(container.querySelector('.rowdrop-list')).toBeTruthy()

    const tiny = Array.from(container.querySelectorAll('.rowdrop-opt'))
      .find((r) => r.textContent?.includes('ggml-tiny-q5_1.bin'))!
    await fireEvent.click(tiny)

    await waitFor(() =>
      expect(vi.mocked(SetSpeechModel)).toHaveBeenCalledWith('C:/aetox/models/ggml-tiny-q5_1.bin'),
    )
    // Picking closes it — an open menu covering the page after the choice is
    // made is just something else to dismiss.
    await waitFor(() => expect(container.querySelector('.rowdrop-list')).toBeNull())

    // And clicking away closes it without choosing anything.
    await fireEvent.click(toolRow.querySelector('.ctrl')!)
    await waitFor(() => expect(container.querySelector('.rowdrop-list')).toBeTruthy())
    await fireEvent.click(container.querySelector('.drop-backdrop')!)
    expect(container.querySelector('.rowdrop-list')).toBeNull()
  })

  // "Where is this file, and where does Aetox even look?" — answerable by
  // clicking, not by reading a path out of a tooltip and pasting it somewhere.
  it('the speech picker opens the folder a model lives in, and the scanned ones', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เครื่องมือ')
    await waitFor(() => expect(screen.getByText('audio_transcribe')).toBeTruthy())

    const toolRow = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('audio_transcribe'))!
    await fireEvent.click(toolRow.querySelector('.ctrl')!)
    await waitFor(() => expect(screen.getByText('ggml-tiny-q5_1.bin')).toBeTruthy())

    const tinyRow = Array.from(container.querySelectorAll('.rowdrop-row'))
      .find((r) => r.textContent?.includes('ggml-tiny-q5_1.bin'))!
    await fireEvent.click(tinyRow.querySelector('.rowdrop-reveal')!)
    await waitFor(() =>
      expect(vi.mocked(RevealSpeechModel)).toHaveBeenCalledWith('C:/aetox/models/ggml-tiny-q5_1.bin'),
    )

    // Every scanned folder is listed, so a missing model has somewhere to go.
    const dirs = Array.from(container.querySelectorAll('.rowdrop-dir')).map((d) => d.textContent?.trim())
    expect(dirs.some((d) => d?.includes('%APPDATA%/aetox/models'))).toBe(true)
    expect(dirs.some((d) => d?.includes('~/.ollama/models'))).toBe(true)
    // No account name anywhere on screen.
    expect(dirs.every((d) => !d?.includes('Users'))).toBe(true)
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

  // เอเจน get their own settings page, listing only them — the office page is
  // where you work with them (chat, job history), this is where you configure
  // them. Both pages are drawn from one markup (profileListPane), so the two
  // lists cannot drift into two different ideas of what a profile row is.
  it('gives agents their own settings page, without the helpers on it', async () => {
    vi.mocked(ListSubagentProfiles).mockResolvedValue([
      { name: 'deck', description: 'ทำสไลด์', prompt: 'role', builtin: true, desk: 'specialized' },
      { name: 'explore', description: 'ค้นไฟล์', prompt: 'role', builtin: true },
    ] as any)
    vi.mocked(ListChairs).mockResolvedValue([{ name: 'deck' }] as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')

    await waitFor(() => expect(screen.getByText('ทำสไลด์')).toBeTruthy())
    expect(screen.queryByText('ค้นไฟล์')).toBeNull()
    // Stays inside Settings — this row is a section, not a link out.
    expect(cockpit.activeView).not.toBe('office')
  })

  // The handshake with the team page. Both halves were tested apart — Office
  // sets cockpit.settingsIntent, Settings consumes it — and a handshake tested
  // only at its two ends is one nobody has actually shaken.
  //
  // The kind rides in the intent because it came off the roster. This page
  // must not re-derive it from the file, which is the whole rule the split
  // rests on, so the assertion is the heading: an agent gets the agent
  // heading, and saving goes out through the agents' door.
  it('opens the editor on the agent the team page sent, through the agents door', async () => {
    vi.mocked(ListSubagentProfiles).mockResolvedValue([
      { name: 'deck', description: 'ทำสไลด์', prompt: 'role', builtin: true, desk: 'specialized' },
      { name: 'explore', description: 'ค้นไฟล์', prompt: 'role', builtin: true },
    ] as any)
    vi.mocked(ListChairs).mockResolvedValue([{ name: 'deck' }] as any)
    vi.mocked(ReadSubagentProfile).mockResolvedValue('---\ndescription: ทำสไลด์\n---\nสร้างสไลด์หนึ่งชุด' as any)
    cockpit.settingsIntent = { section: 'team', agent: 'deck' }

    render(Settings, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ตั้งค่าเอเจน')).toBeTruthy())
    // Consumed once — an intent left behind would reopen this editor on the
    // next plain visit to Settings.
    expect(cockpit.settingsIntent).toBeNull()

    await fireEvent.click(screen.getByText('บันทึก'))
    await waitFor(() => expect(vi.mocked(SaveAgentProfile)).toHaveBeenCalled())
    expect(vi.mocked(SaveSubagentProfile)).not.toHaveBeenCalled()
    expect(vi.mocked(SaveAgentProfile).mock.calls[0][0]).toBe('deck')
  })

  // Where the back button lands, which is the half the two end-tests missed.
  //
  // The intent used to name section 'agents' — the ซับเอเจน page — and the
  // editor still came up correct, because the handler forces kind='agent'
  // regardless. So every assertion above passed while the page *underneath* the
  // editor was the wrong roster: closing it dropped the user on ซับเอเจน,
  // somewhere they had not asked to be and could not have got to from the team
  // page. Only the heading after closing catches that.
  it('goes back to the team roster, not the helpers', async () => {
    vi.mocked(ListSubagentProfiles).mockResolvedValue([
      { name: 'deck', description: 'ทำสไลด์', prompt: 'role', builtin: true, desk: 'specialized' },
    ] as any)
    vi.mocked(ListChairs).mockResolvedValue([{ name: 'deck' }] as any)
    vi.mocked(ReadSubagentProfile).mockResolvedValue('---\ndescription: ทำสไลด์\n---\nสร้างสไลด์' as any)
    cockpit.settingsIntent = { section: 'team', agent: 'deck' }

    render(Settings, { onClose: () => {} })
    await waitFor(() => expect(screen.getByText('ตั้งค่าเอเจน')).toBeTruthy())

    await fireEvent.click(screen.getByText('กลับไปหน้ารวม'))

    await waitFor(() => expect(screen.getByRole('heading', { name: 'เอเจน' })).toBeTruthy())
    expect(screen.queryByRole('heading', { name: 'ซับเอเจน' })).toBeNull()
  })

  it('opens a blank agent form when the team page asks to create one', async () => {
    cockpit.settingsIntent = { section: 'team', createAgent: true }

    render(Settings, { onClose: () => {} })

    await waitFor(() => expect(screen.getByText('ตั้งค่าเอเจน')).toBeTruthy())
    expect(cockpit.settingsIntent).toBeNull()
  })

  // The split (owner's call, 2026-08-05): agents live on the team page, and
  // this page lists only the assistant's own helpers. One profile on two
  // rosters is the overlap the split ended — so a chair name coming back from
  // ListChairs must not appear in either card here, even though the full list
  // (which the shared editor needs) still carries it.
  it('keeps agents off the sub-agents lists', async () => {
    vi.mocked(ListSubagentProfiles).mockResolvedValue([
      { name: 'deck', description: 'เก้าอี้สไลด์', prompt: 'role', builtin: true, desk: 'specialized' },
      { name: 'explore', description: 'ค้นไฟล์', prompt: 'role', builtin: true },
    ] as any)
    vi.mocked(ListChairs).mockResolvedValue([{ name: 'deck' }] as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')

    await waitFor(() => expect(screen.getByText('ค้นไฟล์')).toBeTruthy())
    expect(screen.queryByText('deck')).toBeNull()
    expect(screen.queryByText('เก้าอี้สไลด์')).toBeNull()
  })

  // A file that cannot run is shown with its reason — never silently dropped,
  // never quietly reinterpreted. The row must explain itself even on the
  // read-only roster.
  it('shows a sick file with the reason it cannot run', async () => {
    vi.mocked(ListSubagentProfiles).mockResolvedValue([
      { name: 'หลงบ้าน', description: 'อยากเป็นเอเจน', prompt: 'role', builtin: true,
        invalid: 'ไฟล์นี้เสีย จึงไม่ถูกใช้งาน' },
    ] as any)
    vi.mocked(ListChairs).mockResolvedValue([] as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')

    await waitFor(() => expect(screen.getByText('หลงบ้าน')).toBeTruthy())
    expect(screen.getByText(/ไฟล์นี้เสีย/)).toBeTruthy()
  })

  // The helpers are part of the system (owner's call, 2026-08-06): the page
  // reads. No create button, no editor door, no model pin — and only the
  // bundled set is listed, because "yours" cannot exist.
  it('the sub-agents page is a read-only system roster', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'ซับเอเจน')

    await waitFor(() => expect(screen.getByText('ค้นไฟล์')).toBeTruthy())
    const cards = container.querySelectorAll('.settings-card')
    expect(cards.length).toBe(1)
    expect(cards[0].textContent).toContain('explore')
    expect(cards[0].textContent).toContain('general')
    expect(cards[0].textContent).not.toContain('deck')
    expect(cards[0].textContent).toContain('เพิ่มหรือแก้ไขไม่ได้')

    // Badges are still read off the profile — the roster informs, it just
    // does not edit.
    expect(screen.getByText('เครื่องมือ 4 ตัว')).toBeTruthy()
    expect(screen.getByText('built-in:explore')).toBeTruthy()

    // No doors: nothing to create, configure, or pin. (The description may
    // *mention* creating an agent — it points at the team page — so the check
    // is on buttons, not on prose.)
    const buttonLabels = Array.from(container.querySelectorAll('button')).map((b) => b.textContent ?? '')
    expect(buttonLabels.some((l) => l.includes('สร้าง'))).toBe(false)
    expect(container.querySelector('.set-row button')).toBeNull()
    expect(container.querySelectorAll('.set-row select.ctrl').length).toBe(0)
  })

  it('the model dropdown pins a model per agent', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(container.querySelectorAll('.ag-row select.ctrl').length).toBe(3))

    const selects = Array.from(container.querySelectorAll('.ag-row select.ctrl')) as HTMLSelectElement[]
    expect(selects[0].value).toBe('deepseek-v4') // backend is pinned
    await fireEvent.change(selects[0], { target: { value: '' } })
    await waitFor(() => expect(vi.mocked(SetSubagentModel)).toHaveBeenCalledWith('backend', ''))
  })

  // The tool-picker chips are drawn from the live registry (ListTools), not
  // written down in this file — grep/glob are added to the fixture here,
  // scoped to these two tests, so as not to disturb the Tools page's own
  // "2 built-in tools" count elsewhere in this file.
  const withPickableTools = () => vi.mocked(ListTools).mockResolvedValue([
    { name: 'browser_open', description: 'open a page', source: 'workbench', category: 'web' },
    { name: 'read', description: 'read a file', source: 'builtin', category: 'files' },
    { name: 'audio_transcribe', description: 'transcribe audio', source: 'builtin', category: 'media' },
    { name: 'grep', description: 'search file contents', source: 'builtin' },
    { name: 'glob', description: 'find files by pattern', source: 'builtin' },
  ] as any)

  it('editing a built-in agent splits its real file into fields and says what saving does', async () => {
    withPickableTools()
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))

    // The built-in group's first row (deck) — index 2 overall: yours come first.
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])
    await waitFor(() => expect(container.querySelector('.ag-toolsum')).toBeTruthy())

    // ReadSubagentProfile's mock is '---\ndescription: ค้นไฟล์\ntools: grep, read\n---\nYou search files.'
    // — the frontmatter must land in its own fields, not sit in the role box
    // as text to hand-edit.
    const body = container.querySelector('.ag-body') as HTMLTextAreaElement
    expect(body.value).toBe('You search files.')
    const description = container.querySelector('.pp-field input.ctrl:not([disabled])') as HTMLInputElement
    expect(description.value).toBe('ค้นไฟล์')

    // The summary says what the profile does. "2 selected" would be the same
    // number whether or not a deny list existed, which is the reading the old
    // two-grid layout forced.
    expect(container.querySelector('.ag-toolsum-txt .t')?.textContent).toContain('2')

    // The allow-list itself is one state per tool, in the panel.
    await fireEvent.click(screen.getByText('ตั้งค่า', { selector: '.ag-toolsum button' }))
    await waitFor(() => expect(document.querySelector('.tp-card')).toBeTruthy())
    const allowed = Array.from(document.querySelectorAll('.tp-row'))
      .filter((r) => r.querySelector('.tp-allow.selected'))
      .map((r) => r.querySelector('.tp-name')?.textContent?.trim())
    expect(allowed.sort()).toEqual(['grep', 'read'])
    await fireEvent.click(screen.getByText('เสร็จแล้ว'))

    // Editable where ZCode is not — but honest about creating your own copy.
    expect(screen.getByText(/สร้างเป็นของคุณทับไว้/)).toBeTruthy()
    // A built-in has no delete button; there is nothing of yours to remove yet.
    expect(screen.queryByText('ลบ')).toBeNull()
    expect(screen.queryByText('คืนค่าของแอป')).toBeNull()
  })

  // Setting a tool's state and saving must produce a file the backend can read
  // back exactly the way it was shown — the field split is a display choice,
  // not a new file format.
  const openToolPicker = async (container: HTMLElement, rowIndex: number) => {
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[rowIndex])
    await waitFor(() => expect(container.querySelector('.ag-toolsum')).toBeTruthy())
    await fireEvent.click(screen.getByText('ตั้งค่า', { selector: '.ag-toolsum button' }))
    await waitFor(() => expect(document.querySelector('.tp-card')).toBeTruthy())
  }

  const toolRow = (name: string) =>
    Array.from(document.querySelectorAll('.tp-row'))
      .find((r) => r.querySelector('.tp-name')?.textContent?.trim() === name)

  it('allowing a tool in the picker round-trips through the saved file', async () => {
    withPickableTools()
    const { container } = render(Settings, { onClose: () => {} })
    await openToolPicker(container, 2) // built-in deck

    await fireEvent.click(toolRow('glob')!.querySelector('.tp-allow')!)
    await fireEvent.click(screen.getByText('เสร็จแล้ว'))
    await fireEvent.click(screen.getByText('บันทึก'))

    // An agent's edit goes out through the agents' door.
    await waitFor(() => expect(vi.mocked(SaveAgentProfile)).toHaveBeenCalled())
    const [name, saved] = vi.mocked(SaveAgentProfile).mock.calls.at(-1)!
    expect(name).toBe('deck')
    expect(saved).toContain('tools: grep, read, glob')
    expect(saved.trim().endsWith('You search files.')).toBe(true)
  })

  // The old two-grid layout let the same tool be ticked in both lists at once —
  // a state the engine silently resolves as denied while the UI showed it green
  // above and red below. One state per tool makes it unrepresentable.
  it('a tool cannot be allowed and denied at the same time', async () => {
    withPickableTools()
    const { container } = render(Settings, { onClose: () => {} })
    await openToolPicker(container, 2)

    // grep arrives allowed (the profile's tools list); denying it must remove
    // it from allow rather than adding a second, contradictory entry.
    await fireEvent.click(toolRow('grep')!.querySelector('.tp-deny')!)
    expect(toolRow('grep')!.querySelector('.tp-allow.selected')).toBeNull()
    expect(toolRow('grep')!.querySelector('.tp-deny.selected')).toBeTruthy()

    await fireEvent.click(screen.getByText('เสร็จแล้ว'))
    await fireEvent.click(screen.getByText('บันทึก'))
    await waitFor(() => expect(vi.mocked(SaveAgentProfile)).toHaveBeenCalled())
    const saved = vi.mocked(SaveAgentProfile).mock.calls.at(-1)![1]
    expect(saved).toContain('tools: read')
    expect(saved).toContain('deny: grep')
    expect(saved).not.toContain('tools: grep')
  })

  // The editor draws neither `desk:` nor `needs:` — and used to delete both on
  // save, because serializeAgentFile only wrote back the keys it had fields
  // for. Opening github and pressing Save quietly stripped its `needs:`, so the
  // agent stopped declaring what it cannot work without and the notice it
  // carries in its own prompt went with it. An editor must not delete what it
  // does not draw.
  it('keeps frontmatter it does not draw — desk and needs survive a save', async () => {
    vi.mocked(ReadSubagentProfile).mockResolvedValue(
      '---\ndescription: ดูแล GitHub\ntools: read\nneeds: connection:github, mcp:github\ndesk: specialized\n---\nYou mind the repo.' as any,
    )
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])
    await waitFor(() => expect(container.querySelector('.ag-toolsum')).toBeTruthy())

    await fireEvent.click(screen.getByText('บันทึก'))
    await waitFor(() => expect(vi.mocked(SaveAgentProfile)).toHaveBeenCalled())
    const saved = vi.mocked(SaveAgentProfile).mock.calls.at(-1)![1]
    expect(saved).toContain('needs: connection:github, mcp:github')
    expect(saved).toContain('desk: specialized')
  })

  // Three boxes, because the engine has three mechanisms: tools subtract from
  // the shared set, MCP and skills each add for this one agent alone. Reading
  // them as one list is what sent the user to three pages to answer "what can
  // this one reach".
  it('separates what this agent alone carries — MCP and skills get their own boxes', async () => {
    vi.mocked(PlacementTargets).mockResolvedValue([
      { id: 'assistant', name: 'ผู้ช่วย', kind: 'desk' },
      { id: 'agent:deck', name: 'deck', kind: 'agent' },
    ] as any)
    vi.mocked(AgentSkills).mockResolvedValue([
      { name: 'invoice', description: 'ใบกำกับภาษีต้องมีอะไรบ้าง', bundled: false },
      { name: 'payroll', description: 'โครงไฟล์เงินเดือน', bundled: true },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2]) // deck

    // The skills box reads its own folder, and says which one shipped — that is
    // the fact that decides whether the user's file can replace it.
    await waitFor(() => expect(screen.getByText('invoice')).toBeTruthy())
    expect(screen.getByText('payroll')).toBeTruthy()
    expect(screen.getAllByText('มากับแอป').length).toBeGreaterThan(0)

    // The MCP box offers the enabled servers and not the disabled one: a server
    // that never connects has no tools to carry, whoever it is pointed at.
    expect(screen.getByText('context7')).toBeTruthy()
    const rows = Array.from(container.querySelectorAll('.ag-reachrow'))
    expect(rows.length).toBe(1)

    // Ticking writes the same `for:` list the MCP page writes, through the same
    // call — one register, two doors into it.
    await fireEvent.click(rows[0].querySelector('input[type="checkbox"]')!)
    await waitFor(() => expect(vi.mocked(SetMCPServerTargets)).toHaveBeenCalledWith('context7', ['agent:deck']))
  })

  // The engine has computed unmet needs since needs.go was written and only
  // ever folded them into the agent's own prompt. The page you fix them on
  // showed nothing at all.
  it('says what an agent declared it needs and has not got', async () => {
    vi.mocked(ReadSubagentProfile).mockResolvedValue(
      '---\ndescription: ดูแล GitHub\nneeds: connection:github\n---\nYou mind the repo.' as any,
    )
    vi.mocked(AgentNeeds).mockResolvedValue([
      {
        entry: 'connection:github', met: false,
        options: [{ kind: 'connection', id: 'github', label: 'GitHub', reason: 'unconnected' }],
      },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])

    await waitFor(() => expect(screen.getByText('GitHub')).toBeTruthy())
    // The reason in the user's language, not the code the engine passes.
    expect(container.textContent).toContain('ยังไม่ได้ล็อกอิน')
    expect(container.textContent).not.toContain('unconnected')
    // And a door that names what it opens, rather than a generic "go deal with
    // it" that lands the user on a page with nothing highlighted.
    expect(screen.getByText(/ไปเชื่อมบัญชี/)).toBeTruthy()
  })

  // A bundled agent names the server it was written against, and the user has
  // no way to know which of seven presets that is (owner, 2026-08-14: *"คนเขา
  // ไม่รู้หรอกอันไหนเราทำไว้เพื่อตัวไหน"*). So the fix happens on the agent's own
  // card: install the server AND place the agent on it, in one press.
  //
  // Both halves are asserted because installing alone is the failure that looks
  // like success — the server appears, the need stays unmet as "unplaced", and
  // the button seems to have done nothing.
  it('installs and places the server an agent says it is missing', async () => {
    // This describe block does not clear mocks between tests, so the call
    // history carries earlier saves — cleared here or `.at(-1)` reads somebody
    // else's context7.
    vi.mocked(SaveMCPServer).mockClear()
    vi.mocked(SetMCPServerTargets).mockClear()
    // The agent has to be somewhere a server can point, or placement is skipped
    // and only half the button runs. [2] is deck, as in the box test above.
    vi.mocked(PlacementTargets).mockResolvedValue([
      { id: 'assistant', name: 'ผู้ช่วย', kind: 'desk' },
      { id: 'agent:deck', name: 'deck', kind: 'agent' },
    ] as any)
    // The register answers honestly: firecrawl is not there until it is
    // installed, and the placement half reads it back off this list.
    let installed = false
    const base = [
      { name: 'context7', command: ['npx', '-y', '@upstash/context7-mcp'], disabled: false, status: 'connected', tools: 2, allowed: ['resolve-library-id', 'get-library-docs'] },
      { name: 'exa', url: 'https://mcp.exa.ai/mcp', disabled: true, status: 'disabled', tools: 0 },
    ]
    vi.mocked(SaveMCPServer).mockImplementation(async () => { installed = true })
    vi.mocked(ListMCPServers).mockImplementation(async () => (installed
      ? [...base, { name: 'firecrawl', url: 'https://mcp.firecrawl.dev/v2/mcp', disabled: false, status: 'connected', tools: 0, for: [] }]
      : base) as any)
    vi.mocked(ReadSubagentProfile).mockResolvedValue(
      '---\ndescription: หาข้อมูลเชิงลึก\nneeds: mcp:firecrawl\n---\nYou go and find out.' as any,
    )
    vi.mocked(AgentNeeds).mockResolvedValue([
      {
        entry: 'mcp:firecrawl', met: false,
        options: [{ kind: 'mcp', id: 'firecrawl', label: 'firecrawl', reason: 'missing' }],
      },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])

    await waitFor(() => expect(screen.getByText(/ติดตั้งให้เลย/)).toBeTruthy())
    await fireEvent.click(screen.getByText(/ติดตั้งให้เลย/))

    // Installed from the verified preset, so the URL is the probed one rather
    // than anything typed here.
    await waitFor(() => expect(vi.mocked(SaveMCPServer)).toHaveBeenCalled())
    const [, sent] = vi.mocked(SaveMCPServer).mock.calls.at(-1) as [string, any]
    expect(sent.name).toBe('firecrawl')
    expect(sent.url).toBe('https://mcp.firecrawl.dev/v2/mcp')
    // And placed on this agent — the half that turns "installed" into "met".
    await waitFor(() => expect(vi.mocked(SetMCPServerTargets)).toHaveBeenCalled())
    const [placed, targets] = vi.mocked(SetMCPServerTargets).mock.calls.at(-1) as [string, string[]]
    expect(placed).toBe('firecrawl')
    expect(targets).toContain('agent:deck')
  })

  // A need Aetox has no verified preset for keeps the door to the page. The
  // button exists to remove the *matching* — which of seven presets is this
  // agent's — and it has nothing to offer when there is no entry to match, so
  // pretending otherwise would be a one-click install of nothing.
  it('falls back to the MCP page for a server it has no preset for', async () => {
    vi.mocked(ReadSubagentProfile).mockResolvedValue(
      '---\ndescription: ออโตเมชั่น\nneeds: mcp:windmill\n---\nYou wire things up.' as any,
    )
    vi.mocked(AgentNeeds).mockResolvedValue([
      {
        entry: 'mcp:windmill', met: false,
        options: [{ kind: 'mcp', id: 'windmill', label: 'windmill', reason: 'missing' }],
      },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])

    await waitFor(() => expect(screen.getByText(/ไปเปิดเซิร์ฟเวอร์/)).toBeTruthy())
    expect(screen.queryByText(/ติดตั้งให้เลย/)).toBeNull()
  })

  // `needs: connection:n8n | connection:windmill` is ONE requirement — an
  // automation engine — and either answers it. Drawn as one flat line about
  // whichever was nearest, it read as "n8n is required" and hid both the
  // alternative and which of the two was already on.
  it('reads an either/or requirement as one choice, and says which side is on', async () => {
    vi.mocked(ReadSubagentProfile).mockResolvedValue(
      '---\ndescription: ออโตเมชั่น\nneeds: connection:n8n | connection:windmill\n---\nYou wire things up.' as any,
    )
    vi.mocked(AgentNeeds).mockResolvedValue([
      {
        entry: 'connection:n8n | connection:windmill', met: false,
        options: [
          { kind: 'connection', id: 'n8n', label: 'n8n', reason: 'unconnected' },
          { kind: 'connection', id: 'windmill', label: 'Windmill', reason: 'missing' },
        ],
      },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])

    await waitFor(() => expect(container.textContent).toContain('มีอย่างใดอย่างหนึ่งก็พอ'))
    // Both ways of answering it, each with its own state and its own door.
    expect(container.querySelectorAll('.ag-need-opt').length).toBe(2)
    expect(container.textContent).toContain('Windmill')
    expect(screen.getAllByText(/ไปเชื่อมบัญชี/).length).toBe(2)
    // One requirement, so one warning — not two.
    expect(container.querySelector('.ag-count-warn')?.textContent).toBe('1')
  })

  // The other half of the same fix: an option that is already on has to look
  // like it, or the box is only ever a list of complaints.
  it('marks the side that is already on, and stops warning once either is', async () => {
    vi.mocked(ReadSubagentProfile).mockResolvedValue(
      '---\ndescription: ออโตเมชั่น\nneeds: connection:n8n | connection:windmill\n---\nYou wire things up.' as any,
    )
    vi.mocked(AgentNeeds).mockResolvedValue([
      {
        entry: 'connection:n8n | connection:windmill', met: true,
        options: [
          { kind: 'connection', id: 'n8n', label: 'n8n', reason: '' },
          { kind: 'connection', id: 'windmill', label: 'Windmill', reason: 'missing' },
        ],
      },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])

    await waitFor(() => expect(container.querySelector('.ag-need.met')).toBeTruthy())
    expect(container.querySelectorAll('.ag-need-dot.on').length).toBe(1)
    // Nothing is missing, so nothing is counted as missing.
    expect(container.querySelector('.ag-count-warn')).toBeNull()
  })

  // The opening is a file, and this is a second door onto it — so what the form
  // writes has to be what the chat window would have read from a hand-edit.
  it('edits the opening cards and writes them back as the agent’s own file', async () => {
    vi.mocked(ChairStarters).mockResolvedValue({ headline: 'ถามอะไรดี?', cards: [] } as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2]) // deck

    // Four rows to begin with — the floor, because a pool below a full hand
    // deals a widow into the 2-column grid. It is a floor now, not a ceiling.
    await waitFor(() => expect(container.querySelectorAll('.ag-starter').length).toBe(4))

    const first = container.querySelectorAll('.ag-starter')[0]
    const [title, prompt] = Array.from(first.querySelectorAll('input.ctrl')) as HTMLInputElement[]
    await fireEvent.input(title, { target: { value: 'วางโครงสไลด์' } })
    await fireEvent.input(prompt, { target: { value: 'ช่วยวางโครงสไลด์เรื่องนี้:' } })
    const save = screen.getByText('บันทึกประโยคเปิด') as HTMLButtonElement
    await waitFor(() => expect(save.disabled).toBe(false))
    await fireEvent.click(save)
    // The panel swallows its own failures into this line, so a save that threw
    // would otherwise look exactly like a save that never fired.
    expect(container.querySelector('.mset-error')?.textContent ?? '').toBe('')

    await waitFor(() => expect(vi.mocked(SaveChairStarters)).toHaveBeenCalled())
    const [name, locale, set] = vi.mocked(SaveChairStarters).mock.calls.at(-1)!
    expect(name).toBe('deck')
    expect(locale).toBe('th')
    expect(set.headline).toBe('ถามอะไรดี?')
    // Only the filled row is sent; the three empty ones are not three blank
    // cards the user has to go and delete.
    expect(set.cards.length).toBe(1)
    expect(set.cards[0]).toMatchObject({ title: 'วางโครงสไลด์', prompt: 'ช่วยวางโครงสไลด์เรื่องนี้:' })
  })

  // The grid draws four out of a pool, so an agent can hold more than it shows.
  // While this form was four fixed rows, the form was the thing stopping that:
  // a user could never give a hired agent a fifth card.
  it('grows the opening past the four the grid draws', async () => {
    vi.mocked(ChairStarters).mockResolvedValue({ headline: 'ถามอะไรดี?', cards: [] } as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2]) // deck

    await waitFor(() => expect(container.querySelectorAll('.ag-starter').length).toBe(4))
    await fireEvent.click(container.querySelector('.ag-starter-add')!)
    await fireEvent.click(container.querySelector('.ag-starter-add')!)
    expect(container.querySelectorAll('.ag-starter').length).toBe(6)

    // Removing takes the row away — until the floor, where it clears the row
    // instead, because a pool of three deals a widow into a two-column grid.
    await fireEvent.click(container.querySelectorAll('.ag-starter-drop')[0])
    expect(container.querySelectorAll('.ag-starter').length).toBe(5)
    for (let n = 0; n < 3; n++) {
      await fireEvent.click(container.querySelectorAll('.ag-starter-drop')[0])
    }
    expect(container.querySelectorAll('.ag-starter').length).toBe(4)
  })

  // subagent.forcedDenials never reach a sub-agent whatever the file says.
  // Offering them was a lie the user only discovered after saving.
  it('tools a sub-agent can never get are shown as unavailable, not as choices', async () => {
    vi.mocked(ListTools).mockResolvedValue([
      { name: 'read', description: 'read a file', source: 'builtin' },
      { name: 'ask_user', description: 'ask the human', source: 'builtin' },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openToolPicker(container, 2)

    expect(toolRow('read')!.querySelector('.tp-seg')).toBeTruthy()
    expect(toolRow('ask_user')!.querySelector('.tp-seg')).toBeNull()
    expect(toolRow('ask_user')!.querySelector('.tp-forced')).toBeTruthy()
  })

  it('the picker searches the list rather than making the user scan 35 rows', async () => {
    withPickableTools()
    const { container } = render(Settings, { onClose: () => {} })
    await openToolPicker(container, 2)
    expect(document.querySelectorAll('.tp-row').length).toBeGreaterThan(3)

    await fireEvent.input(document.querySelector('.tp-search')!, { target: { value: 'glo' } })
    const names = Array.from(document.querySelectorAll('.tp-name')).map((n) => n.textContent?.trim())
    expect(names).toEqual(['glob'])
  })

  // The engine reads MaxToolCalls <= 0 as unbounded, and only unbounds on the
  // keyword — a number in the box must never become "no ceiling" by accident.
  it('the loop cap can be removed, and says so in the file as a word', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))
    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[2])
    await waitFor(() => expect(container.querySelector('.ag-steprow')).toBeTruthy())

    const box = container.querySelector('.ag-steps') as HTMLInputElement
    expect(box.disabled).toBe(false)

    await fireEvent.click(container.querySelector('.ag-check input')!)
    // The number box goes dead rather than keeping a value that no longer applies.
    await waitFor(() => expect((container.querySelector('.ag-steps') as HTMLInputElement).disabled).toBe(true))

    await fireEvent.click(screen.getByText('บันทึก'))
    await waitFor(() => expect(vi.mocked(SaveAgentProfile)).toHaveBeenCalled())
    expect(vi.mocked(SaveAgentProfile).mock.calls.at(-1)![1]).toContain('steps: unlimited')
  })

  // Deleting a shadow restores the bundled profile, so the button must not say
  // "delete" — the row is not going away.
  it('a shadow offers to revert, not to delete', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getAllByLabelText('ตั้งค่า').length).toBe(3))

    await fireEvent.click(screen.getAllByLabelText('ตั้งค่า')[1]) // mine-deck, the shadow
    await waitFor(() => expect(screen.getByText('คืนค่าของแอป')).toBeTruthy())
    expect(screen.queryByText('ลบ')).toBeNull()
  })

  it('a new agent opens with guidance in the role field, not a raw frontmatter skeleton', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'เอเจน')
    await waitFor(() => expect(screen.getByText('เพิ่มเอเจนเฉพาะทาง')).toBeTruthy())

    await fireEvent.click(screen.getByText('เพิ่มเอเจนเฉพาะทาง'))
    // Frontmatter is fields now, so a new agent has none of it to see or
    // mistype — the role box only ever holds guidance on what to write.
    const body = container.querySelector('.ag-body') as HTMLTextAreaElement
    expect(body.value).not.toContain('---')
    expect(body.value).not.toContain('description:')
    expect(body.value).not.toContain('steps:')
    expect(body.value).toContain('บอกว่ามันรับงานแบบไหน')
    // Nothing pre-selected: an empty allow list means "every tool", exactly as
    // the badge on the list page already promises for a fresh profile.
    expect(container.querySelectorAll('.ag-tool.active').length).toBe(0)
    // A new one gets to choose its name; an existing one does not.
    expect((container.querySelector('.pp-field input.ctrl') as HTMLInputElement).disabled).toBe(false)
  })

  // The point of the whole sign-in path: a provider you cannot get a key for
  // must still be reachable, and the code has to be on screen while Aetox
  // waits for the provider to say yes.
  it('a sign-in provider shows its device code and waits for approval', async () => {
    seedDeviceSignIn()
    // Hangs on purpose: the real call blocks until the user approves, which is
    // exactly the window the code has to stay readable.
    vi.mocked(CompleteSignIn).mockImplementation(() => new Promise(() => {}))

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    const signInButton = await screen.findByText('เข้าสู่ระบบด้วย Example')
    await fireEvent.click(signInButton)

    await waitFor(() => expect(screen.getByText('ABCD-1234')).toBeTruthy())
    expect(vi.mocked(StartSignIn)).toHaveBeenCalledWith('example-device')
    expect(vi.mocked(CompleteSignIn)).toHaveBeenCalledWith('example-device', '')
  })

  // Reusing another product's OAuth client can get an account cut off, so the
  // warning belongs next to the button, not in the docs.
  it('a restricted sign-in warns before the user commits', async () => {
    seedRestrictedSignIn()
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(container.querySelector('.signin-warn')).toBeTruthy())
  })

  // Codex is a ChatGPT subscription reached at chatgpt.com, and the only key a
  // user could paste is an api.openai.com key — a different host, a different
  // bill, and a guaranteed 401. The field was offering a login that cannot
  // succeed, so the card shows the sign-in and nothing else.
  it('the Codex card offers no API key field, only sign-in', async () => {
    seedSignIn({ provider: 'codex', label: 'ChatGPT', kind: 'browser' })
    vi.mocked(AcceptsAPIKey).mockResolvedValue(false as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(screen.getByText('เข้าสู่ระบบด้วย ChatGPT')).toBeTruthy())
    // The base URL input shares .key-input, so the key field is identified by
    // the thing only it has: a masked value.
    expect(container.querySelector('input[type="password"]')).toBeNull()
    expect(container.textContent).not.toContain('หรือใช้ API key แทน')
  })

  // Every other provider keeps the field: a pasted key is how they are used.
  it('a keyed provider still shows the API key field', async () => {
    seedSignIn({ provider: 'anthropic', label: 'Claude', kind: 'browser' })
    // Set explicitly: the Codex case above opts a provider out, and the shared
    // mock keeps whatever implementation it was last given.
    vi.mocked(AcceptsAPIKey).mockResolvedValue(true as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(container.querySelector('input[type="password"]')).toBeTruthy())
  })

  // The card asks for a key and every provider hides the page that issues one
  // somewhere different, so the answer has to sit next to the field. The URL
  // comes from the Go catalog, which is the only place that knows it.
  it('a keyed provider links out to the page that issues the key', async () => {
    seedSignIn({ provider: 'gemini', label: 'Gemini', kind: 'browser' })
    vi.mocked(AcceptsAPIKey).mockResolvedValue(true as any)
    vi.mocked(ProviderAPIKeyURL).mockResolvedValue('https://aistudio.google.com/apikey' as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    const link = await waitFor(() => {
      const el = container.querySelector('.keylink')
      if (!el) throw new Error('no key link')
      return el
    })
    await fireEvent.click(link)
    expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledWith('https://aistudio.google.com/apikey')
  })

  // A local runtime has no account and a sign-in provider has no key to issue.
  // An empty URL is the catalog saying "nowhere to send them", and a link to
  // nowhere is worse than no link.
  it('a provider with no key page shows no link', async () => {
    seedSignIn({ provider: 'ollama', label: 'Ollama', kind: 'browser' })
    vi.mocked(AcceptsAPIKey).mockResolvedValue(true as any)
    vi.mocked(ProviderAPIKeyURL).mockResolvedValue('' as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(container.querySelector('input[type="password"]')).toBeTruthy())
    expect(container.querySelector('.keylink')).toBeNull()
  })

  it('an already signed-in provider offers sign-out instead of sign-in', async () => {
    seedSignIn()
    vi.mocked(SignInStatus).mockResolvedValue({
      provider: 'openrouter', signed_in: true, label: 'OpenRouter · mike', account: 'mike',
    } as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(screen.getByText('OpenRouter · mike')).toBeTruthy())
    expect(screen.getByText('ออกจากระบบ')).toBeTruthy()
    expect(screen.queryByText('เข้าสู่ระบบด้วย OpenRouter')).toBeNull()
  })
})

// Nothing on this page may destroy anything on the first click. Three of these
// rows used to do exactly that, while three others armed on the first click and
// deleted on the second — so the user learned one rule and lost data to the
// other. Every row now goes through the same dialog.
describe('Settings destructive actions', () => {
  // Call history only — the outer beforeEach's resolved values survive
  // mockClear, and every assertion here is "was the binding reached at all",
  // which a previous test's call would answer for it.
  beforeEach(() => { vi.clearAllMocks() })

  const dialog = () => document.querySelector('.confirm-overlay')
  const clickRemoveIn = async (row: Element, label = 'ลบ') => {
    const btn = Array.from(row.querySelectorAll('button')).find((b) => b.textContent?.trim() === label)
    if (!btn) throw new Error(`"${label}" button not found in row`)
    await fireEvent.click(btn)
  }
  const confirmDialog = async () => {
    const btn = document.querySelector('.confirm-go')
    if (!btn) throw new Error('confirm button not found')
    await fireEvent.click(btn)
  }

  it('removing an MCP server asks first and does nothing until confirmed', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())

    const row = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('context7') && r.querySelector('.mswitch'))!
    await clickRemoveIn(row)

    // The dialog is up and the binding has NOT been called — this is the whole
    // point: the server survives until the user agrees to lose it.
    expect(dialog()).toBeTruthy()
    expect(vi.mocked(RemoveMCPServer)).not.toHaveBeenCalled()
    // The name being destroyed is shown verbatim, not just described.
    expect(document.querySelector('.confirm-detail')?.textContent?.trim()).toBe('context7')

    await confirmDialog()
    await waitFor(() => expect(vi.mocked(RemoveMCPServer)).toHaveBeenCalledWith('context7'))
  })

  it('cancelling keeps the server', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())

    const row = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('context7') && r.querySelector('.mswitch'))!
    await clickRemoveIn(row)
    await fireEvent.click(document.querySelector('.confirm-cancel')!)

    expect(dialog()).toBeNull()
    expect(vi.mocked(RemoveMCPServer)).not.toHaveBeenCalled()
  })

  it('Escape cancels, and focus starts on Cancel so a stray Enter cannot delete', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())

    const row = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('context7') && r.querySelector('.mswitch'))!
    await clickRemoveIn(row)

    expect(document.activeElement).toBe(document.querySelector('.confirm-cancel'))

    await fireEvent.keyDown(document.querySelector('.confirm-overlay')!, { key: 'Escape' })
    await waitFor(() => expect(dialog()).toBeNull())
    expect(vi.mocked(RemoveMCPServer)).not.toHaveBeenCalled()
  })

  it('removing a skill asks first and names the folder that will be deleted', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('gridgeist')).toBeTruthy())

    const row = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('gridgeist'))!
    await clickRemoveIn(row)

    expect(vi.mocked(RemoveExternalSkill)).not.toHaveBeenCalled()
    // A folder is about to leave the disk, so the path is what gets checked.
    expect(document.querySelector('.confirm-detail')?.textContent?.trim()).toBe('C:/skills/gridgeist')

    await confirmDialog()
    await waitFor(() => expect(vi.mocked(RemoveExternalSkill)).toHaveBeenCalledWith('gridgeist'))
  })

  it('removing the running provider warns that the engine will move', async () => {
    vi.mocked(SupportedProviders).mockResolvedValue(['aetox', 'openrouter'] as any)
    vi.mocked(EnabledProviders).mockResolvedValue(['aetox', 'openrouter'] as any)
    // Settings renders on its own here; nothing has run loadRealState(), so the
    // store has to be told which provider the engine is actually on.
    cockpit.model.provider = 'aetox'

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')
    await waitFor(() => expect(container.querySelectorAll('.mset-prov-row').length).toBe(2))

    const row = Array.from(container.querySelectorAll('.mset-prov-row'))
      .find((r) => r.textContent?.includes('aetox'))!
    await fireEvent.click(row.querySelector('.icobtn')!)

    expect(vi.mocked(SetProviderEnabled)).not.toHaveBeenCalled()
    // Removing the running provider silently moves the engine to aetox. That
    // side effect is the reason this confirm is worth reading, so it has to be
    // in the message rather than discovered afterwards.
    expect(document.querySelector('.confirm-message')?.textContent).toContain('aetox')
    expect(document.querySelector('.confirm-detail')?.textContent?.trim()).toBe('aetox')
    cockpit.model.provider = ''
  })

  it('a provider that is not the running one gets no engine warning', async () => {
    vi.mocked(SupportedProviders).mockResolvedValue(['aetox', 'openrouter'] as any)
    vi.mocked(EnabledProviders).mockResolvedValue(['aetox', 'openrouter'] as any)
    cockpit.model.provider = 'openrouter'

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')
    await waitFor(() => expect(container.querySelectorAll('.mset-prov-row').length).toBe(2))

    const row = Array.from(container.querySelectorAll('.mset-prov-row'))
      .find((r) => r.textContent?.includes('aetox'))!
    await fireEvent.click(row.querySelector('.icobtn')!)

    expect(document.querySelector('.confirm-message')?.textContent).not.toContain('aetox')
    cockpit.model.provider = ''
  })
})

describe('Settings resilience and state', () => {
  it('a backend that will not answer shows why, not a blank page', async () => {
    vi.mocked(TerminalShells).mockRejectedValueOnce(new Error('engine not ready'))

    const { container } = render(Settings, { onClose: () => {} })
    await waitFor(() => expect(container.querySelector('.settings-banner')).toBeTruthy())
    expect(screen.getByText('โหลดหน้าตั้งค่าไม่สำเร็จ')).toBeTruthy()
    // The raw reason is kept — "something went wrong" is not a bug report.
    expect(container.querySelector('.settings-banner')?.textContent).toContain('engine not ready')
    expect(screen.getByText('ลองใหม่')).toBeTruthy()
  })

  it('reloading reopens the page you were on, not the first one', async () => {
    sessionStorage.setItem('aetox.settingsSection', 'mcp')
    const { container } = render(Settings, { onClose: () => {} })

    const activeItem = container.querySelector('.settings-nav-item.active')
    expect(activeItem?.textContent).toContain('MCP servers')
    sessionStorage.clear()
  })

  it('a section id that no longer exists falls back instead of rendering nothing', async () => {
    sessionStorage.setItem('aetox.settingsSection', 'a-page-that-was-deleted')
    const { container } = render(Settings, { onClose: () => {} })

    expect(container.querySelector('.settings-nav-item.active')?.textContent).toContain('ทั่วไป')
    sessionStorage.clear()
  })

  it('search finds a page by what is on it, not only by its name', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    const search = container.querySelector('.settings-search') as HTMLInputElement

    // "ธีม" is not the Appearance page's name — it is a control sitting on it,
    // which is exactly the case the old label-only search could not answer.
    await fireEvent.input(search, { target: { value: 'ธีม' } })
    const visible = Array.from(container.querySelectorAll('.settings-nav-item')).map((el) => el.textContent)
    expect(visible.some((label) => label?.includes('รูปลักษณ์'))).toBe(true)
    expect(visible.some((label) => label?.includes('สปอนเซอร์'))).toBe(false)
  })

  it('a search that matches nothing says so rather than emptying the rail', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    const search = container.querySelector('.settings-search') as HTMLInputElement

    await fireEvent.input(search, { target: { value: 'zzzznope' } })
    expect(container.querySelectorAll('.settings-nav-item').length).toBe(0)
    expect(container.querySelector('.settings-nav-empty')).toBeTruthy()
  })
})

describe('Type scale', () => {
  it('each preset writes its factor to the root so every --fs step follows', () => {
    applyTypeScale('large')
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1.18')

    applyTypeScale('compact')
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('0.92')

    applyTypeScale('default')
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1')
  })

  it('the overall-size box reports the px the user actually sees', async () => {
    applyTypeScale('large')
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'รูปลักษณ์')

    // 15.5px base * 1.18 = 18.29 -> 18.3. Without folding the text scale in,
    // this box would keep claiming 15.5 while the app rendered at 18.3.
    const box = container.querySelector('input[type="number"]') as HTMLInputElement
    expect(Number(box.value)).toBeCloseTo(18.3, 1)
    applyTypeScale('default')
  })
})

// The page used to name its own install path in three places and get two of
// them wrong — they said ~/.agents/skills, which is opencode's and which Aetox
// never scans, so anyone following the instructions dropped files where nothing
// was looking. It now asks the engine.
describe('Skills page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // clearAllMocks wipes call history but keeps implementations, so a
    // mockResolvedValue from one test would otherwise leak into the next.
    vi.mocked(SkillScanIssues).mockResolvedValue([] as any)
    vi.mocked(SkillsDir).mockResolvedValue('C:/Users/x/.aetox/skills')
  })

  it('shows the folder the engine actually scans, not one of its own', async () => {
    vi.mocked(SkillsDir).mockResolvedValue('C:/Users/x/.aetox/skills')
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')

    await waitFor(() => expect(screen.getByText('C:/Users/x/.aetox/skills')).toBeTruthy())
    // No hardcoded path may survive anywhere on the page.
    expect(container.textContent).not.toContain('.agents/skills')
  })

  it('offers to open that folder, like the prompts and sub-agent pages do', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('gridgeist')).toBeTruthy())

    await fireEvent.click(screen.getByText('เปิดโฟลเดอร์'))
    expect(vi.mocked(OpenSkillsFolder)).toHaveBeenCalled()
  })

  it('says when a SKILL.md was found but could not be read', async () => {
    // Previously the scan collected these and the list dropped them, so a file
    // with broken frontmatter was indistinguishable from an unwatched folder.
    vi.mocked(SkillScanIssues).mockResolvedValue([
      'C:/Users/x/.aetox/skills/broken/SKILL.md: missing description',
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')

    await waitFor(() => expect(container.querySelector('.skill-issues')).toBeTruthy())
    expect(screen.getByText(/broken\/SKILL\.md/)).toBeTruthy()
  })

  it('stays quiet when every file read cleanly', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('gridgeist')).toBeTruthy())
    expect(container.querySelector('.skill-issues')).toBeNull()
  })
})

// The third install route. A GitHub URL needs the skill published there; the
// folder button needs it already on this machine. A zip is everything else.
describe('Skills page — zip install', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(SkillScanIssues).mockResolvedValue([] as any)
    vi.mocked(SkillsDir).mockResolvedValue('C:/Users/x/.aetox/skills')
  })

  it('installs from a picked archive and reports what landed', async () => {
    vi.mocked(InstallSkillFromZip).mockResolvedValue(
      'ติดตั้งแล้ว 1 สกิล (5 ไฟล์): pdf\nลงที่: C:/Users/x/.aetox/skills' as any,
    )
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('เลือกไฟล์ zip…')).toBeTruthy())

    await fireEvent.click(screen.getByText('เลือกไฟล์ zip…'))
    await waitFor(() => expect(container.querySelector('.skill-result')).toBeTruthy())
    expect(container.querySelector('.skill-result')?.textContent).toContain('5 ไฟล์')
    // The list has to be re-read, or the skill just installed is not on screen.
    expect(vi.mocked(ListExternalSkills).mock.calls.length).toBeGreaterThan(1)
  })

  it('treats a dismissed picker as nothing happening, not as a failure', async () => {
    // The binding returns "" when the native dialog is cancelled.
    vi.mocked(InstallSkillFromZip).mockResolvedValue('' as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('เลือกไฟล์ zip…')).toBeTruthy())

    await fireEvent.click(screen.getByText('เลือกไฟล์ zip…'))
    expect(container.querySelector('.skill-result')).toBeNull()
    expect(container.querySelector('.mset-error')).toBeNull()
  })

  it('surfaces a refused archive instead of failing silently', async () => {
    vi.mocked(InstallSkillFromZip).mockRejectedValue(
      new Error('ไฟล์ zip มีเส้นทางที่ออกนอกโฟลเดอร์ติดตั้ง: ../../evil.txt'),
    )
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'สกิล')
    await waitFor(() => expect(screen.getByText('เลือกไฟล์ zip…')).toBeTruthy())

    await fireEvent.click(screen.getByText('เลือกไฟล์ zip…'))
    await waitFor(() => expect(container.querySelector('.mset-error')).toBeTruthy())
    expect(container.querySelector('.mset-error')?.textContent).toContain('evil.txt')
  })
})

// Four things the MCP page knew and did not say.
describe('MCP servers page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(MCPConfigPath).mockResolvedValue('C:/Users/x/AppData/Roaming/aetox/mcp-servers.json' as any)
  })

  const openMcp = async (container: HTMLElement) => {
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('2 เครื่องมือ')).toBeTruthy())
  }

  it('shows the file the servers are persisted to, and opens it', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openMcp(container)

    expect(screen.getByText('C:/Users/x/AppData/Roaming/aetox/mcp-servers.json')).toBeTruthy()
    await fireEvent.click(screen.getByText('เปิดโฟลเดอร์'))
    expect(vi.mocked(OpenMCPFolder)).toHaveBeenCalled()
  })

  // Two of the three colours here were --c-green-500 and --c-red-500 copied by
  // value, so the dot stayed dark-theme green under a light theme.
  it('paints the status dot from theme tokens, not hex literals', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openMcp(container)

    const dots = Array.from(container.querySelectorAll('.set-row .dot'))
      .map((d) => d.getAttribute('style') ?? '')
    expect(dots.length).toBeGreaterThan(0)
    for (const style of dots) {
      expect(style).toMatch(/var\(--/)
      expect(style).not.toMatch(/#[0-9a-f]{3,6}/i)
    }
  })

  // A preset that needs a key used to be written to disk without one, so the
  // click produced a server that could never connect.
  //
  // The shelf carries only key-needing entries now — it lists the servers this
  // product's own agents ask for by name, not a directory of popular ones — so
  // addPreset's straight-to-disk branch has no fixture left to drive it and is
  // deliberately unpinned until a preset without headers is listed again.
  // This used to assert the opposite — that clicking github opened the form and
  // saved nothing, because the user still had to paste a token. They no longer
  // do: the header carries ${connect:github}, resolved at connect time from the
  // account already connected on the การเชื่อมต่อ page, so nothing is typed and
  // nothing is copied into mcp-servers.json. The behaviour the old test guarded
  // is still guarded, one condition along: a header with no reference in it
  // opens the form instead of saving something that cannot connect.
  it('saves a preset whose key comes from a connection, without asking for a paste', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openMcp(container)

    const row = Array.from(container.querySelectorAll('.set-row'))
      .find((r) => r.textContent?.includes('Repos, pull requests, issues, CI'))!
    await fireEvent.click(row.querySelector('button')!)

    await waitFor(() => expect(vi.mocked(SaveMCPServer)).toHaveBeenCalled())
    const saved = vi.mocked(SaveMCPServer).mock.calls[0][1] as any
    expect(saved.headers.Authorization).toBe('Bearer ${connect:github}')
    // The secret itself is never here. That is the point of the reference.
    expect(JSON.stringify(saved)).not.toMatch(/gh[pous]_/)
  })

  // The state the reported github server was actually in: a header naming its
  // scheme and carrying no credential. It can never authenticate, and the page
  // used to save it and then report the server's "Bad Request" — true, and
  // impossible to trace back to the empty box.
  it('refuses to save a header left as a scheme with no credential', async () => {
    // The http row from the default mock, so openMcp's own wait still has the
    // counts it looks for. It has to be the http one: headers only exist on a
    // remote server, and the stdio row would have none for the guard to read.
    const { container } = render(Settings, { onClose: () => {} })
    await openMcp(container)

    const row = Array.from(container.querySelectorAll('.set-row')).find((r) => r.textContent?.includes('exa'))!
    await fireEvent.click(Array.from(row.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'แก้ไข')!)

    const headers = container.querySelector('.mcp-lines') as HTMLTextAreaElement
    await fireEvent.input(headers, { target: { value: 'Authorization: Bearer' } })
    await fireEvent.click(screen.getByText('บันทึก'))

    expect(vi.mocked(SaveMCPServer)).not.toHaveBeenCalled()
    expect(container.textContent).toContain('Authorization')
  })

  // Both fields were in the stored config from the start with no way to reach
  // them, so editing a server silently dropped whatever was set.
  it('round-trips the working directory and timeout', async () => {
    vi.mocked(ListMCPServers).mockResolvedValue([
      { name: 'local', command: ['node', 'server.js'], cwd: 'D:/work', timeoutMs: 45000, disabled: false, status: 'connected', tools: 2 },
    ] as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'MCP servers')
    await waitFor(() => expect(screen.getByText('local')).toBeTruthy())

    const row = Array.from(container.querySelectorAll('.set-row')).find((r) => r.textContent?.includes('local'))!
    await fireEvent.click(Array.from(row.querySelectorAll('button')).find((b) => b.textContent?.trim() === 'แก้ไข')!)

    const inputs = Array.from(container.querySelectorAll('.mcp-more-body input')) as HTMLInputElement[]
    expect(inputs.map((i) => i.value)).toEqual(['D:/work', '45000'])

    await fireEvent.click(screen.getByText('บันทึก'))
    await waitFor(() => expect(vi.mocked(SaveMCPServer)).toHaveBeenCalled())
    const saved = vi.mocked(SaveMCPServer).mock.calls.at(-1)![1] as any
    expect(saved.cwd).toBe('D:/work')
    expect(saved.timeoutMs).toBe(45000)
  })
})

// The About page is the only place in the running app that answers "which
// version am I on, and is there a newer one" — and the only place the
// install-channel design becomes visible to the user.
describe('Settings About page', () => {
  const openAbout = async () => {
    const r = render(Settings, { onClose: () => {} })
    await openSection(r.container, 'เกี่ยวกับ Aetox')
    return r
  }

  it('shows the installed version without anyone asking GitHub anything', async () => {
    await openAbout()
    await waitFor(() => expect(screen.getByText('v0.8.4')).toBeTruthy())
    expect(vi.mocked(AppVersion)).toHaveBeenCalled()
    // Nothing leaves the machine until the button is pressed.
    expect(vi.mocked(CheckForUpdate)).not.toHaveBeenCalled()
  })

  it('tells a Scoop install to run scoop, and offers no download to unpack over it', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue({
      current: '0.8.4', latest: '0.9.0', available: true, disabled: false,
      channel: 'scoop', hint: 'scoop update aetox',
      url: 'https://example.invalid/r/v0.9.0', checkedAt: '',
    } as any)
    const { container } = await openAbout()
    await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))

    await waitFor(() => expect(screen.getByText('มีเวอร์ชันใหม่ v0.9.0')).toBeTruthy())
    expect(screen.getByText('scoop update aetox')).toBeTruthy()
    expect(container.textContent).toContain('ติดตั้งผ่าน Scoop')
    // Exactly one — the release-notes row at the bottom. The update row itself
    // must not hand a Scoop user a zip.
    expect(screen.getAllByText('เปิดหน้าดาวน์โหลด').length).toBe(1)
  })

  it('sends a portable install to the release page instead of inventing a command', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue({
      current: '0.8.4', latest: '0.9.0', available: true, disabled: false,
      channel: 'portable', hint: '', url: 'https://example.invalid/r/v0.9.0', checkedAt: '',
    } as any)
    await openAbout()
    await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))

    await waitFor(() => expect(screen.getByText('มีเวอร์ชันใหม่ v0.9.0')).toBeTruthy())
    expect(screen.queryByText('scoop update aetox')).toBeNull()
    expect(screen.getAllByText('เปิดหน้าดาวน์โหลด').length).toBe(2)
  })

  it('says so plainly when there is nothing to update to', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue({
      current: '0.8.4', latest: '0.8.4', available: false, disabled: false,
      channel: 'installer', hint: '', url: 'https://example.invalid/r', checkedAt: '',
    } as any)
    await openAbout()
    await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))
    await waitFor(() => expect(screen.getByText('นี่คือเวอร์ชันล่าสุดแล้ว')).toBeTruthy())
  })

  // Offline is the common case, not the exceptional one. It must not read like
  // the app broke, and it must not dump a Go error string at the user.
  it('a failed check reassures instead of alarming', async () => {
    vi.mocked(CheckForUpdate).mockRejectedValue(new Error('dial tcp: no such host'))
    const { container } = await openAbout()
    await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))

    await waitFor(() => expect(screen.getByText('ตรวจหาการอัปเดตไม่สำเร็จ')).toBeTruthy())
    expect(container.textContent).not.toContain('dial tcp')
  })

  // The command exists to be run, and retyping it from a screenshot of a
  // settings page is exactly the friction the copy button removes.
  it('the scoop command can be copied without retyping it', async () => {
    const writeText = vi.fn(async () => {})
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    vi.mocked(CheckForUpdate).mockResolvedValue({
      current: '0.8.4', latest: '0.9.0', available: true, disabled: false,
      channel: 'scoop', hint: 'scoop update aetox',
      url: 'https://example.invalid/r/v0.9.0', checkedAt: '',
    } as any)
    await openAbout()
    await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))
    await waitFor(() => expect(screen.getByText('scoop update aetox')).toBeTruthy())

    await fireEvent.click(screen.getByText('คัดลอก'))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('scoop update aetox'))
    expect(screen.getByText('คัดลอกแล้ว')).toBeTruthy()
  })

  // "Where do I see my version" is a search, not a place people remember. The
  // page has to be findable by what is on it, like every other settings page.
  it('is reachable by searching for what it answers', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    const search = container.querySelector('.settings-search') as HTMLInputElement

    await fireEvent.input(search, { target: { value: 'อัปเดต' } })
    const visible = Array.from(container.querySelectorAll('.settings-nav-item')).map((el) => el.textContent)
    expect(visible.some((label) => label?.includes('เกี่ยวกับ Aetox'))).toBe(true)
  })

  // Switched off is a choice the user made, not a failure to report.
  it('a disabled check is reported as off, naming the switch', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue({
      current: '0.8.4', latest: '', available: false, disabled: true,
      channel: 'portable', hint: '', url: 'https://example.invalid/r', checkedAt: '',
    } as any)
    const { container } = await openAbout()
    await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))

    await waitFor(() => expect(screen.getByText('การตรวจหาการอัปเดตถูกปิดไว้')).toBeTruthy())
    expect(container.textContent).toContain('AETOX_DISABLE_UPDATE_CHECK')
    expect(screen.queryByText('ตรวจหาการอัปเดตไม่สำเร็จ')).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// Connections - a register of external accounts: one line per service until
// you open it, and inside, who may use it.
// ---------------------------------------------------------------------------

// Render Settings and walk to one page. The register lives behind two nav items
// now (accounts and automation engines), so the helper takes the label.
const renderAt = async (label: string) => {
  const rendered = render(Settings, { onClose: () => {} })
  await openSection(rendered.container, label)
  return rendered
}

const openConnections = () => renderAt('การเชื่อมต่อ')

// Opens the row itself, which is where everything but the name and the state
// lives now.
const expandRow = async (container: HTMLElement) => {
  const head = container.querySelector('.reg-head')
  if (!head) throw new Error('no connection row to open')
  await fireEvent.click(head)
}

const githubRow = (over: Record<string, unknown> = {}) => [{
  id: 'github', label: 'GitHub', kind: 'token',
  token_url: 'https://github.com/settings/tokens/new',
  connected: false, env_override: false, for: [], configured: false,
  tools: ['github', 'plugin_install'],
  ...over,
}]

const connectedRow = (over: Record<string, unknown> = {}) => githubRow({
  connected: true, login: 'mike', source: 'connection', for: ['coding'], configured: true, ...over,
})

// A service the user hosts. n8n and Windmill live wherever the user put them,
// so the row has to ask where before a token means anything.
const selfHostedRow = (over: Record<string, unknown> = {}) => [{
  id: 'n8n', label: 'n8n', kind: 'token',
  connected: false, env_override: false, for: [], configured: false,
  tools: ['n8n_workflow_list', 'n8n_workflow_create'],
  needs_base_url: true, base_url_hint: 'http://localhost:5678',
  ...over,
}]

// Two pages over one register, split by the catalog's `family`.
//
// They were one page for a day and the owner's verdict was blunt: an automation
// engine is a machine you run, it takes an address as well as a key, and setting
// one up is a different conversation from signing an account in. Filing them
// together made the page answer two questions and buried the automation half.
//
// What these pin is that the split is decided by `family` and not by a list of
// ids in the component — because the moment it is a list, a third engine lands
// on the wrong page and nobody notices until a user cannot find it.
describe('the split between accounts and automation engines', () => {
  it('keeps automation engines off the accounts register', async () => {
    vi.mocked(Connections).mockResolvedValue([
      ...githubRow(), ...selfHostedRow({ family: 'automation' }),
    ] as any)
    await openConnections()

    await waitFor(() => expect(screen.getByText('GitHub')).toBeTruthy())
    expect(screen.queryByText('n8n')).toBeNull()
  })

  it('shows only automation engines on their own page', async () => {
    vi.mocked(Connections).mockResolvedValue([
      ...githubRow(), ...selfHostedRow({ family: 'automation' }),
    ] as any)
    await renderAt('ระบบออโตเมชั่น')

    await waitFor(() => expect(screen.getByText('n8n')).toBeTruthy())
    expect(screen.queryByText('GitHub')).toBeNull()
  })

  // A service whose family this build does not recognise must land somewhere.
  // Vanishing from both pages is the one outcome with no way back for a user
  // holding a working account.
  it('keeps an unfamiliar service on the register rather than losing it', async () => {
    vi.mocked(Connections).mockResolvedValue(
      githubRow({ id: 'gmail', label: 'Gmail', family: 'mail' }) as any)
    await openConnections()

    await waitFor(() => expect(screen.getByText('Gmail')).toBeTruthy())
  })
})

describe('a row speaks for its own service', () => {
  // These four strings were GitHub's copy hardcoded, so the n8n row asked for a
  // "PERSONAL ACCESS TOKEN" starting `ghp_…` and promised to check it with
  // GitHub. Wrong on every row but one, and wrong in the way that makes a person
  // doubt they are looking at the right screen.
  it('does not put GitHub words on a self-hosted engine', async () => {
    vi.mocked(Connections).mockResolvedValue(selfHostedRow({ family: 'automation' }) as any)
    const { container } = await renderAt('ระบบออโตเมชั่น')
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    expect(container.textContent).not.toContain('GitHub')
    expect(container.querySelector('input[type="password"]')?.getAttribute('placeholder')).toBe('')
    // And it names the service it will actually check the key with.
    expect(container.textContent).toContain('n8n')
  })

  // Two questions, and the page has to look like it knows they are two: whether
  // the PROGRAM is up, and whether the KEY works. Side by side in one column the
  // two check buttons read as two spellings of one button.
  it('splits the server from the account, in order', async () => {
    vi.mocked(Connections).mockResolvedValue(selfHostedRow({ family: 'automation' }) as any)
    const { container } = await renderAt('ระบบออโตเมชั่น')
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    const heads = Array.from(container.querySelectorAll('.conn-part-head'))
      .map((el) => el.textContent?.trim())
    expect(heads).toEqual(['1 · ตัวเซิร์ฟเวอร์', '2 · บัญชีและคีย์'])
  })
})

describe('disconnecting', () => {
  // The one destructive button on this page that just did it. An n8n key is
  // shown once at creation and never again, so a mis-click costs a trip to
  // another program to mint a new one.
  it('asks before throwing a credential away', async () => {
    vi.mocked(Connections).mockResolvedValue(connectedRow() as any)
    vi.mocked(DisconnectAccount).mockClear()
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    await expandRow(container)

    await fireEvent.click(screen.getByText('เลิกเชื่อม'))
    expect(vi.mocked(DisconnectAccount)).not.toHaveBeenCalled()

    // And what survives is said, because "disconnect" reads like "start over"
    // and it is not: the address and the placement are still there.
    await waitFor(() => expect(screen.getByText(/เลิกเชื่อม GitHub\?/)).toBeTruthy())
  })
})

describe('Connections page', () => {
  // Collapsed is the state a returning user arrives in, so the line has to
  // carry the whole answer: which service, whether it is attached, and a way in.
  it('lists each service as one line with its state and a way to connect', async () => {
    vi.mocked(Connections).mockResolvedValue(githubRow() as any)
    const { container } = await openConnections()

    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    expect(screen.getByText('GitHub')).toBeTruthy()
    expect(screen.getByText('เชื่อม')).toBeTruthy()
    // Nothing is open, so no token box is on screen waiting to be pasted into.
    expect(container.querySelector('input[type="password"]')).toBeNull()
    expect(container.querySelector('.conn-body')).toBeNull()
  })

  it('opens one row at a time, and the connect button opens it too', async () => {
    vi.mocked(Connections).mockResolvedValue(githubRow() as any)
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())

    await fireEvent.click(screen.getByText('เชื่อม'))
    await waitFor(() => expect(container.querySelector('.conn-body')).toBeTruthy())
    expect(container.querySelector('input[type="password"]')).toBeTruthy()

    // And the header closes it again.
    await expandRow(container)
    await waitFor(() => expect(container.querySelector('.conn-body')).toBeNull())
  })

  // A connected row says where it reaches without being opened, by name -
  // "2 desks" would make you open it to find out which two.
  it('names the desks on the collapsed row of a connected service', async () => {
    vi.mocked(Connections).mockResolvedValue(connectedRow() as any)
    const { container } = await openConnections()

    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    expect(container.querySelector('.mcp-badge')?.textContent?.trim()).toBe('โค้ด')
  })

  // Never placed is not "off" - it is carried everywhere, and the row must say
  // which of the two it is.
  it('says every desk on a connected service nobody has placed yet', async () => {
    vi.mocked(Connections).mockResolvedValue(connectedRow({ for: [], configured: false }) as any)
    const { container } = await openConnections()

    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    expect(container.querySelector('.mcp-badge')?.textContent?.trim()).toBe('ทุกโต๊ะ')
    expect(container.querySelector('.mcp-badge-warn')).toBeNull()
  })

  // Connected and placed nowhere looks healthy and reaches no one - the one
  // state worth interrupting for, same as the MCP register calls out.
  it('calls out a connection that serves nobody', async () => {
    vi.mocked(Connections).mockResolvedValue(connectedRow({ for: [] }) as any)
    const { container } = await openConnections()

    await waitFor(() => expect(screen.getByText('ไม่มีใคร')).toBeTruthy())
    expect(container.querySelector('.mcp-badge-warn')).toBeTruthy()
  })

  it('offers every desk and agent as a placement, desks picked and agents not', async () => {
    vi.mocked(Connections).mockResolvedValue(githubRow() as any)
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    const chips = Array.from(container.querySelectorAll('.conn-chip'))
    expect(chips.map((c) => c.textContent?.trim())).toEqual(['ผู้ช่วย', 'โค้ด', 'researcher'])
    // An agent is handed things on purpose; a desk is where work already happens.
    expect(chips.map((c) => c.getAttribute('aria-pressed'))).toEqual(['true', 'true', 'false'])
  })

  // A service Aetox hosts nowhere cannot be reached until the user says where
  // it is, and a token checked against the wrong host fails in a way that reads
  // as a bad token — so the address is asked for first and the button stays
  // down until it is there.
  it('asks a self-hosted service where it lives before it will connect', async () => {
    vi.mocked(Connections).mockResolvedValue(selfHostedRow() as any)
    vi.mocked(ConnectAccount).mockClear()
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    const address = container.querySelector('input[type="text"].key-input') as HTMLInputElement
    expect(address).toBeTruthy()
    expect(address.placeholder).toBe('http://localhost:5678')

    const token = container.querySelector('input[type="password"]') as HTMLInputElement
    await fireEvent.input(token, { target: { value: 'n8n_api_key' } })
    const connect = screen.getByText('เชื่อม') as HTMLButtonElement
    expect(connect.disabled).toBe(true)

    await fireEvent.input(address, { target: { value: '  http://box.local:5678  ' } })
    expect(connect.disabled).toBe(false)
    await fireEvent.click(connect)

    // The address goes trimmed, and beside the token in one call — placement is
    // whatever the row's chips are showing and is not what this test is about.
    await waitFor(() => expect(vi.mocked(ConnectAccount)).toHaveBeenCalled())
    expect(vi.mocked(ConnectAccount).mock.calls[0].slice(0, 3))
      .toEqual(['n8n', 'n8n_api_key', 'http://box.local:5678'])
  })

  // GitHub is one host for everybody and states it as a constant. Drawing an
  // address field on its row would ask a question with one possible answer.
  it('does not ask a hosted service for an address', async () => {
    vi.mocked(Connections).mockResolvedValue(githubRow() as any)
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    expect(container.querySelector('input[type="text"].key-input')).toBeNull()
  })

  it('connects with the pasted token and the chosen desks in one call', async () => {
    vi.mocked(Connections)
      .mockResolvedValueOnce(githubRow() as any)
      .mockResolvedValue(connectedRow() as any)
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    const chips = Array.from(container.querySelectorAll('.conn-chip'))
    await fireEvent.click(chips[0])

    const field = container.querySelector('input[type="password"]') as HTMLInputElement
    await fireEvent.input(field, { target: { value: '  ghp_example  ' } })
    await fireEvent.click(screen.getByText('เชื่อม'))

    // The empty string is the base URL: GitHub is one host for everybody and
    // states it as a constant, so the field is not even drawn on its row.
    await waitFor(() =>
      expect(vi.mocked(ConnectAccount)).toHaveBeenCalledWith('github', 'ghp_example', '', ['coding']))
    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    expect(container.textContent).toContain('สิทธิ์: repo')
    expect(container.querySelector('input[type="password"]')).toBeNull()
  })

  // Flipping a switch on a connected row writes straight through. It must not
  // go via the connect path, which would send the token field along with it.
  it('moves a connected account between desks without touching its token', async () => {
    vi.mocked(Connections).mockResolvedValue(connectedRow() as any)
    // Cleared rather than assumed clean: this is the one assertion in the file
    // that a binding was *not* called, so a call left over from an earlier test
    // would fail it for a reason that has nothing to do with the page.
    vi.mocked(ConnectAccount).mockClear()
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    await expandRow(container)

    const chips = Array.from(container.querySelectorAll('.conn-chip'))
    await fireEvent.click(chips[0])

    await waitFor(() =>
      expect(vi.mocked(SetConnectionTargets)).toHaveBeenCalledWith('github', ['coding', 'assistant']))
    expect(vi.mocked(ConnectAccount)).not.toHaveBeenCalled()
  })

  it('keeps the typed token when the service rejects it', async () => {
    vi.mocked(Connections).mockResolvedValue(githubRow() as any)
    vi.mocked(ConnectAccount).mockRejectedValueOnce(new Error('GitHub rejected this token'))
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
    await expandRow(container)

    const field = container.querySelector('input[type="password"]') as HTMLInputElement
    await fireEvent.input(field, { target: { value: 'ghp_wrong' } })
    await fireEvent.click(screen.getByText('เชื่อม'))

    await waitFor(() => expect(container.textContent).toContain('GitHub rejected this token'))
    expect((container.querySelector('input[type="password"]') as HTMLInputElement).value).toBe('ghp_wrong')
  })

  // Disconnect lives inside the row on purpose: it is not something to do by
  // accident while scanning a list.
  it('disconnects from inside the opened row', async () => {
    vi.mocked(Connections)
      .mockResolvedValueOnce(connectedRow() as any)
      .mockResolvedValue(githubRow() as any)
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    expect(screen.queryByText('เลิกเชื่อม')).toBeNull()

    await expandRow(container)
    await fireEvent.click(screen.getByText('เลิกเชื่อม'))
    // Through the confirm gate now: a credential thrown away is a credential
    // that has to be minted again somewhere else. The dialog's own button
    // carries the same word, so the second click is the one that acts.
    const confirm = await waitFor(() =>
      Array.from(document.querySelectorAll('.confirm-actions button, .modal button'))
        .find((b) => b.textContent?.trim() === 'เลิกเชื่อม') as HTMLButtonElement)
    await fireEvent.click(confirm)

    await waitFor(() => expect(vi.mocked(DisconnectAccount)).toHaveBeenCalledWith('github'))
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เชื่อม')).toBeTruthy())
  })

  // A token exported in a shell profile is why GitHub keeps working for someone
  // who never connected anything. The page has to say so, and must still offer
  // the form, since connecting an account is how you take that override back.
  it('says when the token comes from the environment, and still offers to connect', async () => {
    vi.mocked(Connections).mockResolvedValue(
      githubRow({ connected: true, source: 'environment', env_override: true }) as any)
    const { container } = await openConnections()

    await waitFor(() => expect(screen.getByText('กำลังใช้ token จาก environment')).toBeTruthy())
    await expandRow(container)
    expect(container.querySelector('input[type="password"]')).toBeTruthy()
    expect(screen.queryByText('เลิกเชื่อม')).toBeNull()
  })

  it('discloses an environment token that the connected account is overriding', async () => {
    vi.mocked(Connections).mockResolvedValue(connectedRow({ env_override: true }) as any)
    const { container } = await openConnections()
    await waitFor(() => expect(screen.getByText('เชื่อมแล้วในชื่อ mike')).toBeTruthy())
    await expandRow(container)

    expect(container.textContent).toContain('แต่ Aetox ใช้บัญชีที่เชื่อมไว้')
  })
})

// The sidebar dot used to be painted from HasAPIKey, which returns true for
// anything needing no key — so LM Studio and Ollama showed green whether or not
// a server was listening, on a card that said "no models found" beside it.
describe('the provider dot says what is actually usable', () => {
  const seedRow = (provider: string) => {
    vi.mocked(SupportedProviders).mockResolvedValue([provider] as any)
    vi.mocked(EnabledProviders).mockResolvedValue([provider] as any)
    vi.mocked(SignInMethods).mockResolvedValue([] as any)
  }
  const dot = (container: HTMLElement) => container.querySelector('.mset-prov .dot')

  it('stays un-green while the answer is still in flight', async () => {
    seedRow('lmstudio')
    // Never resolves: exactly the window in which the old dot was already green.
    vi.mocked(ProviderReady).mockImplementation(() => new Promise(() => {}) as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(dot(container)).toBeTruthy())
    expect(dot(container)!.classList.contains('green')).toBe(false)
    expect(dot(container)!.classList.contains('unknown')).toBe(true)
  })

  it('stays un-green when the local server is not running', async () => {
    seedRow('lmstudio')
    vi.mocked(ProviderReady).mockResolvedValue(false as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(dot(container)?.classList.contains('unknown')).toBe(false))
    expect(dot(container)!.classList.contains('green')).toBe(false)
  })

  it('goes green once the engine confirms it', async () => {
    seedRow('deepseek')
    vi.mocked(ProviderReady).mockResolvedValue(true as any)

    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(dot(container)?.classList.contains('green')).toBe(true))
  })
})

// 411 models in alphabetical order is a list you scroll, not one you choose
// from. The price is the other half: unknown has to look unlike free, because
// on a list this long a zero is a decision the user would act on.
describe('the model list can be searched and priced', () => {
  const manyModels = [
    'aion-labs/aion-2.0', 'ai21/jamba-large-1.7', 'deepseek/deepseek-v4-flash',
    'deepseek/deepseek-v4-pro', 'google/gemma-4-31b-it:free', 'meta/llama-4',
    'mistral/mistral-medium', 'openai/gpt-5-nano', 'qwen/qwen3.6-flash',
  ]
  const seedList = () => {
    vi.mocked(SupportedProviders).mockResolvedValue(['openrouter'] as any)
    vi.mocked(EnabledProviders).mockResolvedValue(['openrouter'] as any)
    vi.mocked(SignInMethods).mockResolvedValue([] as any)
    vi.mocked(ListModelsForProvider).mockResolvedValue(manyModels as any)
    vi.mocked(PriceModels).mockResolvedValue([
      { model: 'deepseek/deepseek-v4-flash', input: 0.14, output: 0.28, priced: true, free: false, context: 1000000 },
      { model: 'deepseek/deepseek-v4-pro', input: 0.435, output: 0.87, priced: true, free: false, context: 1000000 },
      { model: 'google/gemma-4-31b-it:free', input: 0, output: 0, priced: false, free: true, context: 32768 },
      // The rest are absent on purpose: the catalog covers 84% of OpenRouter.
    ] as any)
  }
  const rowNames = (container: HTMLElement) =>
    Array.from(container.querySelectorAll('.mrow .mname')).map((e) => e.textContent?.trim() ?? '')

  it('filters the list by what is typed', async () => {
    seedList()
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')
    await waitFor(() => expect(rowNames(container).length).toBe(manyModels.length))

    const search = container.querySelector('.mlist-search') as HTMLInputElement
    expect(search).toBeTruthy()
    await fireEvent.input(search, { target: { value: 'deepseek' } })

    await waitFor(() => expect(rowNames(container)).toEqual([
      'deepseek/deepseek-v4-flash', 'deepseek/deepseek-v4-pro', // cheapest first
    ]))
  })

  it('shows a price where there is one and a dash where there is not', async () => {
    seedList()
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')

    await waitFor(() => expect(container.querySelector('.mprice')).toBeTruthy())
    const text = container.textContent ?? ''
    expect(text).toContain('$0.14 / $0.28')
    // A model the catalog never priced must not read as free.
    const dashes = container.querySelectorAll('.mprice.dim').length
    expect(dashes).toBe(manyModels.length - 3)
  })

  it('can show only the free models, keyed off the vendor marker', async () => {
    seedList()
    const { container } = render(Settings, { onClose: () => {} })
    await openSection(container, 'การตั้งค่าโมเดล')
    await waitFor(() => expect(rowNames(container).length).toBe(manyModels.length))

    const toggle = Array.from(container.querySelectorAll('.conn-chip'))
      .find((b) => b.textContent?.includes('เฉพาะฟรี'))
    expect(toggle).toBeTruthy()
    await fireEvent.click(toggle!)

    await waitFor(() => expect(rowNames(container)).toEqual(['google/gemma-4-31b-it:free']))
  })
})
