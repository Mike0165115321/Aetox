<script lang="ts">
  import { onMount } from 'svelte'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import { editorFont, applyEditorFontSize } from './editorFont.svelte'
  import { chatFont, applyChatFontSize } from './chatFont.svelte'
  import { uiFont, applyUiFont, UI_FONTS, type UiFontName } from './uiFont.svelte'
  import { editorTheme, setBuiltinEditorTheme, setAutoEditorTheme, importThemeFile } from './editorTheme.svelte'
  import { treeFont, applyTreeFontSize } from './treeFont.svelte'
  import { systemZoom, applySystemZoom, SYSTEM_BASE_PX } from './systemFont.svelte'
  import { i18n, t, setLocale, localeNames, type Locale } from './i18n.svelte'
  import {
    SupportedProviders, HasAPIKey, RequiresAPIKey, TerminalShells,
    ListModelsForProvider, ProviderBaseURL, ProviderBaseURLIsCustom,
    ProviderWireFormats, TestProviderConnection,
    EnabledProviders, SetProviderEnabled,
    ListMCPServers, SaveMCPServer, RemoveMCPServer, TestMCPServer, ToggleMCPServer,
    ListExternalSkills, ListTools, InstallSkillFromGitHub, RemoveExternalSkill, RefreshSkills,
    ListSpeechModels, SetSpeechModel, SpeechStatus, RevealSpeechModel, SpeechModelDirs, OpenSpeechModelDir,
    UsageStats, ListPromptPresets, OpenPromptsFolder,
    SavePromptPreset, DeletePromptPreset, PickPresetImage, RemovePresetImage,
    ListSubagentProfiles, ReadSubagentProfile, SaveSubagentProfile,
    DeleteSubagentProfile, SetSubagentModel, OpenSubagentsFolder,
  } from '../../wailsjs/go/main/App'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import promptPayQR from '../assets/images/promptpay-qr.png'
  import { config } from '../../wailsjs/go/models'
  import { cockpit, switchProvider, switchModel, submitAPIKey, switchApprovalMode, switchWireFormat, setProviderBaseURL, retryActiveProvider } from './stores/cockpit.svelte'
  import {
    identity, loadIdentityFiles, openIdentityFile, saveIdentityFile,
    createIdentityFile, deleteIdentityFile, identityTemplates,
  } from './identity.svelte'

  let { onClose }: { onClose: () => void } = $props()

  // ---------- AI identity (moved out of the sidebar: it is configuration you
  // edit once in a while, not a list you navigate between chats) ----------
  let newIdentityName = $state('')
  const identityDirty = $derived(identity.draft !== identity.saved)
  const missingTemplates = $derived(
    identity.loaded && identity.files
      ? identityTemplates.filter((tpl) => !(identity.files || []).some((f) => f.name === tpl.name))
      : [],
  )
  function addIdentityFile() {
    if (!newIdentityName.trim()) return
    createIdentityFile(newIdentityName)
    newIdentityName = ''
  }

  const approvalOptions = [
    { value: 'ask', label: t('chat.approvalAsk') },
    { value: 'unsafe-only', label: t('chat.approvalUnsafeOnly') },
    { value: 'full-access', label: t('chat.approvalFullAccess') },
  ]

  // ---------- General: default shell ----------
  let shells = $state<{ name: string; path: string }[]>([])
  let defaultShell = $state(localStorage.getItem('defaultShell') ?? '')

  function saveDefaultShell() {
    localStorage.setItem('defaultShell', defaultShell)
  }

  // ---------- Appearance: code theme import ----------
  let themeImportError = $state('')

  async function onThemeFileChosen(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0]
    if (!file) return
    themeImportError = ''
    try {
      await importThemeFile(file)
    } catch (err) {
      themeImportError = t('settings.importThemeError', { err: String(err) })
    }
    ;(e.currentTarget as HTMLInputElement).value = ''
  }

  // ---------- Model settings ----------
  type ProviderRow = { name: string; requiresKey: boolean; hasKey: boolean }

  let providers = $state<ProviderRow[]>([])
  let enabledNames = $state<string[]>([])
  let showAddProvider = $state(false)
  let selected = $state('')
  let baseURL = $state('')
  let wireFormats = $state<string[]>([])
  let models = $state<string[]>([])
  let loadingModels = $state(false)
  let keyDraft = $state('')
  let showKey = $state(false)
  let customModel = $state('')
  let busy = $state('')
  let errorMsg = $state('')

  const selectedRow = $derived(providers.find((p) => p.name === selected))
  const isActiveProvider = $derived(cockpit.model.provider === selected)
  const enabledRows = $derived(providers.filter((p) => enabledNames.includes(p.name)))
  const addableRows = $derived(providers.filter((p) => !enabledNames.includes(p.name)))
  // Only meaningful while this provider is the active one — otherwise nothing
  // has been bootstrapped for it yet, so show what would be the default.
  const currentWireFormat = $derived(isActiveProvider ? cockpit.model.wireFormat : (wireFormats[0] ?? ''))

  onMount(async () => {
    shells = await TerminalShells()
    if (!shells.some((s) => s.path === defaultShell)) defaultShell = shells[0]?.path ?? ''

    await refreshProviders()
    await refreshEnabledProviders()
    selectProvider(cockpit.model.provider || enabledRows[0]?.name || providers[0]?.name || '')

    await loadMCP()
    await loadSkills()
  })

  async function refreshProviders() {
    const names = await SupportedProviders()
    providers = await Promise.all(names.map(async (name) => ({
      name,
      requiresKey: await RequiresAPIKey(name),
      hasKey: await HasAPIKey(name),
    })))
  }

  async function refreshEnabledProviders() {
    enabledNames = await EnabledProviders()
  }

  const addProvider = (name: string) => run('enable:' + name, async () => {
    enabledNames = await SetProviderEnabled(name, true)
    showAddProvider = false
    await selectProvider(name)
  })

  const removeProvider = (name: string) => run('disable:' + name, async () => {
    const wasActiveEngine = cockpit.model.provider === name
    enabledNames = await SetProviderEnabled(name, false)
    if (selected === name) await selectProvider(enabledNames[0] ?? '')
    // Removing the provider Aetox is actually running on must move the engine
    // too — otherwise it keeps running unlisted while the picker shows a
    // provider that's no longer selectable. Falls back to aetox (Aetox's own
    // built-in engine, always available, needs no key) rather than an
    // arbitrary "next" provider, since that's the deliberate safe default.
    if (wasActiveEngine) await switchProvider('aetox')
  })

  async function selectProvider(name: string) {
    if (!name) return
    selected = name
    errorMsg = ''
    keyDraft = ''
    connTest = ''
    connTestModel = ''
    baseURL = await ProviderBaseURL(name)
    baseURLDraft = baseURL
    baseURLIsCustom = await ProviderBaseURLIsCustom(name)
    wireFormats = await ProviderWireFormats(name)
    loadingModels = true
    models = []
    try {
      const res = await ListModelsForProvider(name)
      models = Array.isArray(res) ? res : []
      // Discovery just proved this endpoint answers. If the engine is still on
      // the fallback from a switch made while it was down, this is the moment
      // it can get off — otherwise the warning sits there next to the model
      // list that disproves it.
      if (models.length > 0 && cockpit.model.warning) await retryActiveProvider()
    } finally {
      loadingModels = false
    }
  }

  async function run(label: string, fn: () => Promise<void>) {
    busy = label
    errorMsg = ''
    try {
      await fn()
    } catch (err) {
      errorMsg = String(err)
    } finally {
      busy = ''
    }
  }

  const useProvider = () => run('provider', async () => {
    await switchProvider(selected)
  })

  const useModel = (m: string) => run(m, async () => {
    if (!isActiveProvider) await switchProvider(selected)
    await switchModel(m)
  })

  // Runtime identifiers ("anthropic", "openai-compatible") aren't meant for
  // display; map to a short human label. Falls back to the raw value for any
  // future format this list doesn't know about yet.
  function wireFormatLabel(format: string): string {
    switch (format) {
      case 'anthropic': return 'Anthropic'
      case 'openai-compatible': return 'OpenAI'
      default: return format
    }
  }

  const useFormat = (fmt: string) => run('format:' + fmt, async () => {
    if (!isActiveProvider) await switchProvider(selected)
    await switchWireFormat(fmt)
  })

  // Connection test: a real 1-token completion through the chat path, run per
  // model so a model can be proven before switching to it. connTestModel says
  // which row the result belongs under; connTest is '' = untested, 'ok:…' /
  // 'err:…' render as success / failure.
  let connTest = $state('')
  let connTestModel = $state('')
  const testConnection = (name: string) => run('test:' + name, async () => {
    connTest = ''
    connTestModel = name
    try {
      connTest = 'ok:' + await TestProviderConnection(selected, name)
    } catch (err) {
      connTest = 'err:' + String(err)
    }
  })

  // Endpoint override. Saving '' clears it — that is the reset, so the button
  // is enabled on an empty box rather than treated as "nothing to save".
  let baseURLDraft = $state('')
  let baseURLIsCustom = $state(false)
  const saveBaseURL = (value: string) => run('baseUrl', async () => {
    await setProviderBaseURL(selected, value)
    await selectProvider(selected)
  })

  const saveKey = () => run('key', async () => {
    const key = keyDraft.trim()
    if (!key) return
    await submitAPIKey(selected, key)
    keyDraft = ''
    await refreshProviders()
    await selectProvider(selected)
  })

  // ---------- MCP servers ----------
  type MCPRow = {
    name: string; command?: string[]; url?: string
    environment?: Record<string, string>; headers?: Record<string, string>
    disabled: boolean; status: string; tools: number; err?: string
  }
  let mcpServers = $state<MCPRow[]>([])
  let mcpQuery = $state('')
  let mcpBusy = $state('')
  let mcpError = $state('')

  // Add/edit form. mcpOriginal === '' means add mode; otherwise it holds the
  // name of the server being edited.
  let mcpOriginal = $state('')
  let mcpKind = $state<'stdio' | 'http'>('stdio')
  let mcpName = $state('')
  let mcpCommand = $state('')
  let mcpUrl = $state('')
  let mcpEnvText = $state('')
  let mcpHeadersText = $state('')

  const mcpFiltered = $derived(mcpServers.filter((s) => {
    const q = mcpQuery.trim().toLowerCase()
    if (!q) return true
    return s.name.toLowerCase().includes(q)
      || (s.command ?? []).join(' ').toLowerCase().includes(q)
      || (s.url ?? '').toLowerCase().includes(q)
  }))

  const mcpFormValid = $derived(
    mcpName.trim() !== '' && (mcpKind === 'stdio' ? mcpCommand.trim() !== '' : mcpUrl.trim() !== ''),
  )

  async function loadMCP() {
    mcpServers = await ListMCPServers()
  }

  async function runMCP(label: string, fn: () => Promise<void>) {
    mcpBusy = label
    mcpError = ''
    try {
      await fn()
    } catch (err) {
      mcpError = String(err)
    } finally {
      mcpBusy = ''
    }
  }

  // "KEY=VALUE" / "Header: value" lines → map; blank and separator-less lines
  // are dropped rather than erroring, the backend trims further.
  function parseLines(text: string, sep: '=' | ':'): Record<string, string> {
    const out: Record<string, string> = {}
    for (const line of text.split('\n')) {
      const i = line.indexOf(sep)
      if (i <= 0) continue
      out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
    }
    return out
  }

  function mapToLines(m: Record<string, string> | undefined, sep: string): string {
    return Object.entries(m ?? {}).map(([k, v]) => `${k}${sep}${v}`).join('\n')
  }

  function resetMCPForm() {
    mcpOriginal = ''
    mcpKind = 'stdio'
    mcpName = ''
    mcpCommand = ''
    mcpUrl = ''
    mcpEnvText = ''
    mcpHeadersText = ''
  }

  function editMCP(s: MCPRow) {
    mcpOriginal = s.name
    mcpKind = s.url ? 'http' : 'stdio'
    mcpName = s.name
    mcpCommand = (s.command ?? []).join(' ')
    mcpUrl = s.url ?? ''
    mcpEnvText = mapToLines(s.environment, '=')
    mcpHeadersText = mapToLines(s.headers, ': ')
    mcpError = ''
  }

  const saveMCP = () => runMCP('save', async () => {
    const server = new config.MCPServerConfig({
      name: mcpName.trim(),
      command: mcpKind === 'stdio' ? mcpCommand.trim().split(/\s+/).filter(Boolean) : [],
      url: mcpKind === 'http' ? mcpUrl.trim() : '',
      environment: mcpKind === 'stdio' ? parseLines(mcpEnvText, '=') : {},
      headers: mcpKind === 'http' ? parseLines(mcpHeadersText, ':') : {},
    })
    await SaveMCPServer(mcpOriginal, server)
    resetMCPForm()
    await loadMCP()
  })

  const removeMCP = (name: string) => runMCP('rm:' + name, async () => {
    await RemoveMCPServer(name)
    if (mcpOriginal === name) resetMCPForm()
    await loadMCP()
  })

  const testMCP = (name: string) => runMCP('test:' + name, async () => {
    await TestMCPServer(name)
    await loadMCP()
  })

  const toggleMCP = (s: MCPRow) => runMCP('toggle:' + s.name, async () => {
    await ToggleMCPServer(s.name, !s.disabled)
    await loadMCP()
  })

  // Curated quick-adds; every package name verified against the npm registry
  // (or, for URLs, the provider's published MCP endpoint) before listing.
  const mcpPresets: { name: string; desc: string; command?: string[]; url?: string }[] = [
    { name: 'context7', desc: 'Up-to-date library docs', command: ['npx', '-y', '@upstash/context7-mcp'] },
    { name: 'sequential-thinking', desc: 'Step-by-step reasoning scratchpad', command: ['npx', '-y', '@modelcontextprotocol/server-sequential-thinking'] },
    { name: 'memory', desc: 'Knowledge-graph memory', command: ['npx', '-y', '@modelcontextprotocol/server-memory'] },
    { name: 'js-repl', desc: 'Run JavaScript/Node code', command: ['npx', '-y', 'mcp-repl'] },
    { name: 'exa', desc: 'Web search (needs API key header)', url: 'https://mcp.exa.ai/mcp' },
  ]

  const presetTaken = (name: string) => mcpServers.some((s) => s.name.toLowerCase() === name.toLowerCase())

  const addPreset = (p: (typeof mcpPresets)[number]) => runMCP('preset:' + p.name, async () => {
    await SaveMCPServer('', new config.MCPServerConfig({
      name: p.name, command: p.command ?? [], url: p.url ?? '',
    }))
    await loadMCP()
  })

  function statusColor(status: string): string {
    const c = status === 'connected' ? '#3fb950' : status === 'failed' ? '#f85149' : '#8b949e'
    return `background:${c}`
  }

  // ---------- Skills (discovered SKILL.md + plugin install) ----------
  type SkillRow = { name: string; description: string; dir: string }
  let extSkills = $state<SkillRow[]>([])
  // Read-only: every tool the AI can run — Aetox's own plus anything an MCP
  // server bridged in. Separate from the skills below, which are documents, not
  // things it runs.
  let tools = $state<{ name: string; description: string; source: string }[]>([])
  // One card per source instead of one list with a badge repeated on every row:
  // where a tool comes from is a property of the group, not of each line.
  const TOOL_SOURCES = ['builtin', 'workbench', 'mcp'] as const
  const toolGroups = $derived(
    TOOL_SOURCES
      .map((key) => ({ key, items: tools.filter((s) => s.source === key) }))
      .filter((g) => g.items.length > 0),
  )
  let expandedTool = $state('') // name of the row showing its full description
  // The speech picker belongs to audio_transcribe, so it hangs off that tool's
  // row rather than sitting in a card of its own — a setting parked away from
  // the thing it configures is a setting nobody connects to it.
  const SPEECH_TOOL = 'audio_transcribe'
  let speechOpen = $state(false)
  const activeSpeechLabel = $derived(
    speechModels.find((m) => m.active)?.name ?? t('settings.speechAuto'),
  )
  let skillBusy = $state('')
  let skillError = $state('')
  let skillInstallUrl = $state('')
  let skillInstallResult = $state('')
  let skillConfirm = $state('') // name pending delete confirmation

  async function loadSkills() {
    extSkills = await ListExternalSkills()
    tools = await ListTools()
    await loadSpeech()
  }

  // ---------- Speech model (what audio_transcribe runs on) ----------
  // Models differ by an order of magnitude in size and accuracy, and a machine
  // can hold several — including ones Ollama or LM Studio already downloaded.
  // Without this the engine just took whichever it found first.
  type SpeechRow = { path: string; name: string; sizeMB: number; store: string; managed: boolean; active: boolean }
  let speechModels = $state<SpeechRow[]>([])
  let speechStatus = $state('') // engine's own reason it cannot run; '' means ready
  let speechBusy = $state(false)
  let speechError = $state('')

  let speechDirs = $state<{ path: string; label: string }[]>([])

  async function loadSpeech() {
    speechModels = await ListSpeechModels()
    speechStatus = await SpeechStatus()
    speechDirs = await SpeechModelDirs()
  }

  // '' pins nothing, which is how the user gets back to auto-discovery.
  async function pickSpeechModel(path: string) {
    speechBusy = true
    speechError = ''
    try {
      await SetSpeechModel(path)
      await loadSpeech()
      speechOpen = false // the choice is made; leaving it open just covers the page
    } catch (err) {
      speechError = String(err) // stays open so the reason is readable
    } finally {
      speechBusy = false
    }
  }

  async function runSkill(label: string, fn: () => Promise<void>) {
    skillBusy = label
    skillError = ''
    try {
      await fn()
    } catch (err) {
      skillError = String(err)
    } finally {
      skillBusy = ''
    }
  }

  const installSkill = () => runSkill('install', async () => {
    skillInstallResult = ''
    skillInstallResult = await InstallSkillFromGitHub(skillInstallUrl.trim())
    skillInstallUrl = ''
    await loadSkills()
  })

  const removeSkill = (name: string) => {
    if (skillConfirm !== name) {
      skillConfirm = name
      return
    }
    skillConfirm = ''
    void runSkill('rm:' + name, async () => {
      await RemoveExternalSkill(name)
      await loadSkills()
    })
  }

  const refreshSkills = () => runSkill('refresh', async () => {
    await RefreshSkills()
    await loadSkills()
  })

  // ---------- Usage stats ----------
  // cacheRows counts the calls whose provider reported cache accounting at all.
  // Zero means "no cache to report" (a local runtime), which must render as an
  // em dash — a 0% hit rate would be a claim the provider never made.
  type UsageRow = {
    model: string; promptTokens: number; completionTokens: number
    cachedTokens: number; uncachedTokens: number; cacheRows: number; calls: number
  }
  type DayPoint = {
    day: string; model: string; promptTokens: number; completionTokens: number
    cachedTokens: number; cacheRows: number
  }
  type UsageTotals = {
    promptTokens: number; completionTokens: number; cachedTokens: number; uncachedTokens: number
    cacheRows: number; calls: number; sessions: number; messages: number
    activeDays: number; currentStreak: number; topModel: string; topModelShare: number
  }
  type Usage = {
    today: UsageRow[]; week: UsageRow[]; all: UsageRow[]
    totals: UsageTotals; daily: DayPoint[]; heatmap: DayPoint[]
  }

  let usage = $state<Usage | null>(null)
  let usageError = $state('')
  let usagePeriod = $state<'today' | 'week' | 'all'>('week')

  async function loadUsage() {
    usageError = ''
    try {
      usage = await UsageStats() as Usage
    } catch (err) {
      usageError = String(err)
    }
  }

  const fmtTokens = (n: number) => n.toLocaleString('en-US')
  // Headline numbers reach eight digits; the cards need the shape, not the digits.
  const fmtCompact = (n: number) =>
    n >= 1e9 ? (n / 1e9).toFixed(1) + 'B'
    : n >= 1e6 ? (n / 1e6).toFixed(1) + 'M'
    : n >= 1e4 ? Math.round(n / 1e3) + 'K'
    : n.toLocaleString('en-US')
  const pct = (part: number, whole: number) => (whole > 0 ? Math.round((part / whole) * 100) : 0)
  const dayKey = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

  const usageRows = $derived(usage ? usage[usagePeriod] : [])
  const usageTotal = (r: UsageRow) => r.promptTokens + r.completionTokens
  const periodTotal = $derived(usageRows.reduce((sum, r) => sum + usageTotal(r), 0))

  // Colour follows the entity, not its row number: the top five all-time
  // models take the five slots and keep them, so switching the period filter
  // never repaints the models that survive it. The tail shares one mute slot —
  // a sixth hue could not stay distinguishable under colour-vision deficiency.
  const seriesOf = $derived.by(() => {
    const map = new Map<string, number>()
    const top = (usage?.all ?? []).slice(0, 5).map((r) => r.model).sort()
    top.forEach((model, i) => map.set(model, i + 1))
    return map
  })
  const slotOf = (model: string) => seriesOf.get(model) ?? 0

  // Round a maximum up to a clean axis top, so the ticks read 0 / 250K / 500K
  // instead of 0 / 231,904 / 463,808.
  const niceMax = (value: number) => {
    if (value <= 0) return 1
    const mag = Math.pow(10, Math.floor(Math.log10(value)))
    for (const step of [1, 1.5, 2, 2.5, 3, 4, 5, 7.5]) {
      if (value <= step * mag) return step * mag
    }
    return 10 * mag
  }

  // Every day in the window gets a column, including the empty ones. Plotting
  // only the days that have data turns a month into four fat blocks and quietly
  // rescales the x-axis — the gaps ARE the story on a usage chart.
  const CHART_DAYS = 30

  // A column carries two encodings at once: hue is the model, fill is where the
  // tokens came from. Stacking is kind-outer, model-inner, so the hit|miss and
  // in|out boundaries land at the same depth in every column and can be read
  // straight across — the model split then reads as hue inside each band.
  //
  // 'raw' is input from a model that reported no cache accounting that day. It
  // is its own band on purpose: folding it into miss would claim a cache the
  // provider never said it had, which is the same lie the table renders as "—".
  type Kind = 'hit' | 'miss' | 'raw' | 'out'
  const KINDS: Kind[] = ['hit', 'miss', 'raw', 'out']
  // Same words the headline card already uses for the same split — a second
  // vocabulary for hit/miss would make the two read as different measurements.
  const kindLabel: Record<Kind, string> = $derived({
    hit: t('settings.usageHit'),
    miss: t('settings.usageMiss'),
    raw: t('settings.usageInput'),
    out: t('settings.usageOutput'),
  })

  const dailyChart = $derived.by(() => {
    if (!usage) return null
    // day -> kind -> model -> tokens
    const byDay = new Map<string, Map<Kind, Map<string, number>>>()
    const add = (day: string, kind: Kind, model: string, value: number) => {
      if (value <= 0) return
      let kinds = byDay.get(day)
      if (!kinds) { kinds = new Map(); byDay.set(day, kinds) }
      let models = kinds.get(kind)
      if (!models) { models = new Map(); kinds.set(kind, models) }
      models.set(model, (models.get(model) ?? 0) + value)
    }
    for (const p of usage.daily) {
      if (p.cacheRows > 0) {
        add(p.day, 'hit', p.model, Math.min(p.cachedTokens, p.promptTokens))
        add(p.day, 'miss', p.model, p.promptTokens - p.cachedTokens)
      } else {
        add(p.day, 'raw', p.model, p.promptTokens)
      }
      add(p.day, 'out', p.model, p.completionTokens)
    }
    if (byDay.size === 0) return null

    const today = new Date()
    today.setHours(0, 0, 0, 0)
    const days = []
    for (let i = CHART_DAYS - 1; i >= 0; i--) {
      const d = new Date(today)
      d.setDate(d.getDate() - i)
      const key = dayKey(d)
      const kinds = byDay.get(key)
      const parts: { kind: Kind; model: string; value: number }[] = []
      const byKind = {} as Record<Kind, number>
      const byModel = new Map<string, number>()
      for (const kind of KINDS) {
        const models = [...(kinds?.get(kind) ?? new Map<string, number>())]
          // Stack in slot order so a model sits at the same depth every column.
          .sort((a, b) => slotOf(a[0]) - slotOf(b[0]))
        byKind[kind] = models.reduce((s, [, value]) => s + value, 0)
        for (const [model, value] of models) {
          parts.push({ kind, model, value })
          byModel.set(model, (byModel.get(model) ?? 0) + value)
        }
      }
      const models = [...byModel].sort((a, b) => slotOf(a[0]) - slotOf(b[0]))
      days.push({ day: key, total: parts.reduce((s, p) => s + p.value, 0), parts, byKind, models })
    }
    const max = niceMax(Math.max(...days.map((d) => d.total)))
    // Four gridlines top-down, the last being the baseline.
    const ticks = [1, 0.75, 0.5, 0.25, 0].map((f) => ({ frac: f, value: Math.round(max * f) }))
    return { days, max, ticks }
  })

  // Five x-labels evenly spaced; more collide at this width.
  const chartXLabels = $derived.by(() => {
    const days = dailyChart?.days ?? []
    if (days.length === 0) return []
    const every = Math.max(1, Math.round(days.length / 5))
    return days.map((d, i) => (i % every === 0 || i === days.length - 1 ? d.day.slice(5) : ''))
  })

  let hoverDay = $state<number | null>(null)
  const hoveredColumn = $derived(hoverDay === null ? null : (dailyChart?.days[hoverDay] ?? null))

  // 26 whole weeks ending with the current one. Cells past today are rendered
  // blank rather than as zero-activity days that have not happened yet.
  const heatmap = $derived.by(() => {
    const totals = new Map<string, number>()
    for (const p of usage?.heatmap ?? []) {
      totals.set(p.day, (totals.get(p.day) ?? 0) + p.promptTokens + p.completionTokens)
    }
    const today = new Date()
    today.setHours(0, 0, 0, 0)
    const end = new Date(today)
    end.setDate(end.getDate() + (6 - end.getDay()))
    const cells: { day: string; value: number; future: boolean }[] = []
    for (let i = 26 * 7 - 1; i >= 0; i--) {
      const d = new Date(end)
      d.setDate(d.getDate() - i)
      const key = dayKey(d)
      cells.push({ day: key, value: totals.get(key) ?? 0, future: d > today })
    }
    const max = Math.max(1, ...cells.map((c) => c.value))
    const weeks: (typeof cells)[] = []
    for (let w = 0; w < 26; w++) weeks.push(cells.slice(w * 7, w * 7 + 7))
    return { weeks, max }
  })
  const heatLevel = (value: number, max: number) => (value <= 0 ? 0 : Math.min(4, Math.ceil((value / max) * 4)))

  $effect(() => {
    if (active === 'usage') void loadUsage()
  })

  // ---------- Prompt presets ----------
  type PresetRow = { name: string; description: string; body: string; path: string; builtin: boolean; image: string }
  let presets = $state<PresetRow[]>([])
  // null = the gallery. Anything else = the editor, on a copy of that preset.
  let editing = $state<PresetRow | null>(null)
  let draftName = $state('')
  let draftBody = $state('')
  let draftImage = $state('')
  let presetBusy = $state('')
  let presetError = $state('')
  let confirmDelete = $state(false)

  async function loadPresets() {
    presets = await ListPromptPresets()
  }

  // Bundled presets have no cover to ship, so a card without an image gets a
  // stable colour derived from its name — the gallery reads as a gallery on a
  // fresh install, with no assets in the installer.
  function coverHue(name: string): number {
    let h = 0
    for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) % 360
    return h
  }

  function openPreset(p: PresetRow) {
    editing = p
    draftName = p.name
    draftBody = p.body
    draftImage = p.image
    presetError = ''
    confirmDelete = false
  }

  // A blank 300px textarea tells you nothing about what belongs in it, so a new
  // preset starts on the skeleton every good prompt shares (role and goal,
  // hard constraints, where the arguments go) — edit-and-replace beats
  // stare-at-nothing.
  function newPreset() {
    editing = { name: '', description: '', body: '', path: '', builtin: false, image: '' }
    draftName = ''
    draftBody = t('settings.promptStarter')
    draftImage = ''
    presetError = ''
    confirmDelete = false
  }

  // Inserts at the caret, because $ARGUMENTS is the one token a preset cannot
  // work without and the one nobody remembers how to spell.
  let bodyEl = $state<HTMLTextAreaElement | null>(null)
  function insertArguments() {
    const el = bodyEl
    if (!el) { draftBody += '$ARGUMENTS'; return }
    const at = el.selectionStart ?? draftBody.length
    draftBody = draftBody.slice(0, at) + '$ARGUMENTS' + draftBody.slice(el.selectionEnd ?? at)
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(at + 10, at + 10)
    })
  }

  async function runPreset(label: string, fn: () => Promise<void>) {
    presetBusy = label
    presetError = ''
    try {
      await fn()
    } catch (err) {
      presetError = String(err)
    } finally {
      presetBusy = ''
    }
  }

  const savePreset = () => runPreset('save', async () => {
    await SavePromptPreset(draftName.trim(), draftBody)
    await loadPresets()
    editing = null
  })

  const deletePreset = () => {
    if (!confirmDelete) { confirmDelete = true; return }
    void runPreset('delete', async () => {
      await DeletePromptPreset(draftName.trim())
      await loadPresets()
      editing = null
    })
  }

  // A cover can only be attached to a preset that exists on disk, so an unsaved
  // one is saved first — otherwise the image would have nothing to belong to.
  const pickImage = () => runPreset('image', async () => {
    const name = draftName.trim()
    if (!name) { presetError = t('settings.promptNameFirst'); return }
    if (!presets.some((p) => p.name === name && !p.builtin)) {
      await SavePromptPreset(name, draftBody || ' ')
    }
    const dataUrl = await PickPresetImage(name)
    if (dataUrl) draftImage = dataUrl
    await loadPresets()
  })

  const dropImage = () => runPreset('image', async () => {
    await RemovePresetImage(draftName.trim())
    draftImage = ''
    await loadPresets()
  })

  // ---------- Sub-agents (ARCHITECTURE.md §44) ----------
  // Only sub-agents live here. The main agent is the assistant — one identity,
  // configured by the identity files — and is not chosen from a list (§44.0).
  type SubagentRow = {
    name: string; description: string; model?: string
    tools?: string[]; deny?: string[]; steps?: number; prompt: string
    path?: string; builtin: boolean; overrides?: boolean
  }
  let subagents = $state<SubagentRow[]>([])
  // Split by who wrote it, which is what a user actually asks of this page. A
  // file of yours that shadows a bundled one counts as yours — it IS your file —
  // and carries a badge saying so, because deleting it reverts rather than removes.
  const mySubagents = $derived(subagents.filter((a) => !a.builtin))
  const builtinSubagents = $derived(subagents.filter((a) => a.builtin))
  // null = the list. Anything else = the editor on that profile's raw file.
  let agentEditing = $state<SubagentRow | null>(null)
  let agentDraftName = $state('')
  let agentDraftBody = $state('')
  let agentBusy = $state('')
  let agentError = $state('')
  let agentConfirmDelete = $state(false)

  // The per-row model dropdown offers the current provider's models — a pin to a
  // model from some other provider still shows (as its own option) rather than
  // silently reading as "inherit".
  let agentModels = $state<string[]>([])

  async function loadAgents() {
    subagents = await ListSubagentProfiles()
    try {
      agentModels = await ListModelsForProvider(cockpit.model.provider)
    } catch {
      agentModels = [] // no key / offline: the dropdown still offers "inherit"
    }
  }

  async function runAgent(label: string, fn: () => Promise<void>) {
    agentBusy = label
    agentError = ''
    try {
      await fn()
    } catch (err) {
      agentError = String(err)
    } finally {
      agentBusy = ''
    }
  }

  // The dropdown on a row: '' means inherit whatever model the chat is on.
  const pinModel = (name: string, model: string) => runAgent('model:' + name, async () => {
    await SetSubagentModel(name, model)
    await loadAgents()
  })

  // Editing opens the raw .md — including for a bundled profile, where saving
  // writes your own copy over it (the engine already prefers user files).
  const openAgent = (a: SubagentRow) => runAgent('open:' + a.name, async () => {
    agentDraftBody = await ReadSubagentProfile(a.name)
    agentDraftName = a.name
    agentEditing = a
    agentConfirmDelete = false
  })

  function newAgent() {
    agentEditing = { name: '', description: '', prompt: '', builtin: false }
    agentDraftName = ''
    agentDraftBody = t('settings.agentStarter')
    agentError = ''
    agentConfirmDelete = false
  }

  const saveAgent = () => runAgent('save', async () => {
    await SaveSubagentProfile(agentDraftName.trim(), agentDraftBody)
    await loadAgents()
    agentEditing = null
  })

  const deleteAgent = () => {
    if (!agentConfirmDelete) { agentConfirmDelete = true; return }
    void runAgent('delete', async () => {
      await DeleteSubagentProfile(agentDraftName.trim())
      await loadAgents()
      agentEditing = null
    })
  }

  // What the row says about tools has to be what the sub-agent actually gets: an
  // empty list means the whole registry, not zero tools.
  const toolBadge = (a: SubagentRow) =>
    a.tools && a.tools.length > 0 ? t('settings.agentToolCount', { n: a.tools.length }) : t('settings.agentAllTools')

  $effect(() => {
    if (active === 'agents') void loadAgents()
  })

  $effect(() => {
    if (active === 'prompts') void loadPresets()
  })
  $effect(() => {
    if (active === 'identity') loadIdentityFiles()
  })

  // ---------- Nav ----------
  const sections = $derived([
    { group: t('settings.groupPersonal'), items: [
      { id: 'general', label: t('settings.general'), icon: '⚙' },
      { id: 'appearance', label: t('settings.appearance'), icon: '🎨' },
      { id: 'identity', label: t('sidebar.identity'), icon: '👤' },
    ]},
    { group: t('settings.groupModels'), items: [
      { id: 'models', label: t('settings.modelSettings'), icon: '🧠' },
      { id: 'agents', label: t('settings.subagents'), icon: '🤖' },
    ]},
    { group: t('settings.groupTools'), items: [
      // Two pages, not two cards on one: a tool is something the AI runs, a
      // skill is a document telling it how. Sharing a page is what made them
      // read as one thing.
      { id: 'tools', label: t('settings.tools'), icon: '🛠' },
      { id: 'skills', label: t('settings.skills'), icon: '🧩' },
      { id: 'mcp', label: t('settings.mcpServers'), icon: '🔌' },
      { id: 'prompts', label: t('settings.prompts'), icon: '✨' },
      { id: 'usage', label: t('settings.usage'), icon: '📊' },
    ]},
    { group: t('settings.groupAbout'), items: [
      { id: 'sponsor', label: t('settings.sponsor'), icon: '❤' },
    ]},
  ])

  const SPONSOR_URL = 'https://github.com/Mike0165115321/Aetox/blob/main/SPONSOR.md'
  const SITE_URL = 'https://mike0165115321.github.io/Aetox/'

  let active = $state('general')
  let query = $state('')

  const filteredSections = $derived(
    sections
      .map((g) => ({ ...g, items: g.items.filter((it) => it.label.toLowerCase().includes(query.trim().toLowerCase())) }))
      .filter((g) => g.items.length > 0),
  )
</script>

<div class="settings-page">
  <aside class="settings-nav">
    <button class="settings-back" onclick={onClose}>{t('settings.backToApp')}</button>
    <input class="settings-search" placeholder={t('settings.searchPlaceholder')} bind:value={query} />
    {#each filteredSections as g}
      <div class="settings-group-label eyebrow">{g.group}</div>
      {#each g.items as it}
        <button class="settings-nav-item" class:active={active === it.id} onclick={() => (active = it.id)}>
          <span class="ic">{it.icon}</span> {it.label}
        </button>
      {/each}
    {/each}
  </aside>

  <div class="settings-content">
    <div class="settings-inner">
    {#if active === 'general'}
      <h2>{t('settings.general')}</h2>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.shellTitle')}</div>
            <div class="d">{t('settings.shellDesc')}</div>
          </div>
          {#if shells.length === 0}
            <span class="muted">{t('settings.noShells')}</span>
          {:else}
            <select class="ctrl" bind:value={defaultShell} onchange={saveDefaultShell}>
              {#each shells as s}
                <option value={s.path}>{s.name}</option>
              {/each}
            </select>
          {/if}
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.approvalTitle')}</div>
            <div class="d">{t('settings.approvalDesc')}</div>
          </div>
          <select class="ctrl" value={cockpit.model.approval} onchange={(e) => switchApprovalMode(e.currentTarget.value)}>
            {#each approvalOptions as opt}<option value={opt.value}>{opt.label}</option>{/each}
          </select>
        </div>
      </div>
    {:else if active === 'appearance'}
      <h2>{t('settings.appearance')}</h2>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.languageTitle')}</div>
            <div class="d">{t('settings.languageDesc')}</div>
          </div>
          <select class="ctrl" value={i18n.locale} onchange={(e) => setLocale(e.currentTarget.value as Locale)}>
            {#each Object.entries(localeNames) as [code, name]}
              <option value={code}>{name}</option>
            {/each}
          </select>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.themeTitle')}</div>
            <div class="d">{t('settings.themeDesc')}</div>
          </div>
          <select class="ctrl" value={theme.name} onchange={(e) => applyTheme(e.currentTarget.value as ThemeName)}>
            {#each THEMES as th}
              <option value={th.value}>{th.label}</option>
            {/each}
          </select>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.uiFontTitle')}</div>
            <div class="d">{t('settings.uiFontDesc')}</div>
          </div>
          <select class="ctrl" value={uiFont.name} onchange={(e) => applyUiFont(e.currentTarget.value as UiFontName)}>
            {#each UI_FONTS as f}
              <option value={f.value}>{t(f.labelKey)}</option>
            {/each}
          </select>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.systemZoomTitle')}</div>
            <div class="d">{t('settings.systemZoomDesc')}</div>
          </div>
          <input
            class="ctrl" type="number" min="12" max="20" step="0.5"
            value={Math.round(systemZoom.value * SYSTEM_BASE_PX * 10) / 10}
            onchange={(e) => applySystemZoom(parseFloat(e.currentTarget.value) / SYSTEM_BASE_PX)}
          />
          <span class="muted" style="margin-left:6px">px</span>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.editorFontTitle')}</div>
            <div class="d">{t('settings.editorFontDesc')}</div>
          </div>
          <input
            class="ctrl" type="number" min="10" max="24" step="0.5"
            value={editorFont.size}
            onchange={(e) => applyEditorFontSize(parseFloat(e.currentTarget.value))}
          />
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.chatFontTitle')}</div>
            <div class="d">{t('settings.chatFontDesc')}</div>
          </div>
          <input
            class="ctrl" type="number" min="12" max="22" step="0.5"
            value={chatFont.size}
            onchange={(e) => applyChatFontSize(parseFloat(e.currentTarget.value))}
          />
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.treeFontTitle')}</div>
            <div class="d">{t('settings.treeFontDesc')}</div>
          </div>
          <input
            class="ctrl" type="number" min="11" max="18" step="0.5"
            value={treeFont.size}
            onchange={(e) => applyTreeFontSize(parseFloat(e.currentTarget.value))}
          />
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.codeThemeTitle')}</div>
            <div class="d">{t('settings.codeThemeDesc')}</div>
          </div>
          <select class="ctrl" value={editorTheme.choice} onchange={(e) => {
            const v = e.currentTarget.value
            if (v === 'auto') setAutoEditorTheme()
            else if (v === 'vs-dark' || v === 'vs') setBuiltinEditorTheme(v)
          }}>
            <option value="auto">{t('settings.codeThemeAuto')}</option>
            <option value="vs-dark">{t('settings.codeThemeDark')}</option>
            <option value="vs">{t('settings.codeThemeLight')}</option>
            {#if editorTheme.importedName}
              <option value="imported">{editorTheme.importedName}</option>
            {/if}
          </select>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.importThemeTitle')}</div>
            <div class="d">{t('settings.importThemeDesc')}</div>
          </div>
          <label class="ctrl">
            {t('settings.importThemeButton')}
            <input type="file" accept=".json,application/json" style="display:none" onchange={onThemeFileChosen} />
          </label>
        </div>
        {#if themeImportError}<div class="mset-error">{themeImportError}</div>{/if}
      </div>
    {:else if active === 'models'}
      <h2>{t('settings.modelSettings')}</h2>
      <p class="muted set-sub">{t('settings.modelsDesc')}</p>

      <div class="settings-card mset">
        <aside class="mset-side">
          <div class="settings-group-label eyebrow">{t('settings.providers')}</div>
          {#each enabledRows as p (p.name)}
            <div class="mset-prov-row">
              <button class="mset-prov" class:selected={selected === p.name} onclick={() => selectProvider(p.name)}>
                {p.name}
                <span class="dot" class:green={p.hasKey}></span>
              </button>
              {#if enabledRows.length > 1}
                <button class="icobtn tiny" disabled={busy === 'disable:' + p.name}
                  aria-label={t('settings.remove')} onclick={() => removeProvider(p.name)}>✕</button>
              {/if}
            </div>
          {/each}

          <button class="mset-prov mset-add-toggle" onclick={() => (showAddProvider = !showAddProvider)}>
            + {t('settings.addProvider')}
          </button>
          {#if showAddProvider}
            <div class="mset-add-list">
              {#each addableRows as p (p.name)}
                <button class="mset-prov" disabled={busy === 'enable:' + p.name} onclick={() => addProvider(p.name)}>
                  {p.name}
                  <span class="dot">{busy === 'enable:' + p.name ? '…' : '+'}</span>
                </button>
              {/each}
              {#if addableRows.length === 0}
                <div class="muted" style="font-size:12px; padding:4px 10px">{t('settings.noMoreProviders')}</div>
              {/if}
            </div>
          {/if}
        </aside>

        <div class="mset-detail">
          {#if selectedRow}
            <div class="mset-head">
              <span class="mset-name">{selected}</span>
              {#if isActiveProvider}
                <span class="badge on">{t('settings.active')}</span>
              {:else}
                <button class="ctrl" disabled={busy !== ''} onclick={useProvider}>
                  {busy === 'provider' ? t('settings.switching') : t('settings.useThisProvider')}
                </button>
              {/if}
            </div>

            {#if isActiveProvider && cockpit.model.warning}
              <!-- Without this the "Active" badge above claims a provider the
                   engine never reached (LM Studio with its server off). -->
              <div class="conn-test">{t('chat.providerFallback')} · {cockpit.model.warning}</div>
            {/if}

            <div class="mset-field">
              <div class="eyebrow">{t('settings.baseUrl')}</div>
              <div class="muted" style="font-size:12px; margin-bottom:6px">{t('settings.baseUrlDesc')}</div>
              <div class="mset-keyrow">
                <input
                  class="ctrl key-input" placeholder={baseURL || 'http://localhost:1234/v1'}
                  bind:value={baseURLDraft}
                  onkeydown={(e) => e.key === 'Enter' && saveBaseURL(baseURLDraft)}
                />
                <button class="ctrl" disabled={busy !== '' || baseURLDraft.trim() === baseURL} onclick={() => saveBaseURL(baseURLDraft)}>
                  {busy === 'baseUrl' ? t('settings.saving') : t('settings.save')}
                </button>
                {#if baseURLIsCustom}
                  <button class="ctrl" disabled={busy !== ''} onclick={() => saveBaseURL('')}>{t('settings.baseUrlReset')}</button>
                {/if}
              </div>
            </div>

            {#if wireFormats.length > 1}
              <div class="mset-field">
                <div class="eyebrow">{t('settings.wireFormat')}</div>
                <div class="muted" style="font-size:12px; margin-bottom:6px">{t('settings.wireFormatDesc')}</div>
                <div class="mset-keyrow">
                  {#each wireFormats as fmt}
                    {#if currentWireFormat === fmt}
                      <span class="badge on">{wireFormatLabel(fmt)}</span>
                    {:else}
                      <button class="ctrl" disabled={busy !== ''} onclick={() => useFormat(fmt)}>
                        {busy === 'format:' + fmt ? t('settings.switching') : wireFormatLabel(fmt)}
                      </button>
                    {/if}
                  {/each}
                </div>
              </div>
            {/if}

            {#if selectedRow.requiresKey}
              <div class="mset-field">
                <div class="eyebrow">{t('settings.apiKeyLabel')}</div>
                <div class="mset-keyrow">
                  <input
                    class="ctrl key-input" type={showKey ? 'text' : 'password'}
                    placeholder={selectedRow.hasKey ? t('settings.keySetPlaceholder') : t('settings.pasteKeyPlaceholder')}
                    bind:value={keyDraft}
                    onkeydown={(e) => e.key === 'Enter' && saveKey()}
                  />
                  <button class="icobtn tiny" aria-label={t('settings.showKey')} onclick={() => (showKey = !showKey)}>👁</button>
                  <button class="ctrl" disabled={busy === 'key' || !keyDraft.trim()} onclick={saveKey}>
                    {busy === 'key' ? t('settings.saving') : t('settings.save')}
                  </button>
                </div>
              </div>
            {/if}

            <div class="mset-field">
              <div class="eyebrow">{t('settings.modelList')}</div>
              {#if loadingModels}
                <div class="muted">{t('settings.loadingModels')}</div>
              {:else if models.length === 0}
                <div class="muted">{t('settings.noModels')}</div>
              {:else}
                {#each models as m}
                  <div class="mrow">
                    <span class="mname">{m}</span>
                    <button
                      class="icobtn tiny" title={t('settings.testConnection')} aria-label={t('settings.testConnection')}
                      disabled={busy !== ''} onclick={() => testConnection(m)}
                    >{busy === 'test:' + m ? '…' : '🔌'}</button>
                    {#if isActiveProvider && cockpit.model.modelName === m}
                      <span class="badge on">{t('settings.inUse')}</span>
                    {:else}
                      <button class="ctrl" disabled={busy !== ''} onclick={() => useModel(m)}>
                        {busy === m ? t('settings.switching') : t('settings.use')}
                      </button>
                    {/if}
                  </div>
                  {#if connTestModel === m && connTest}
                    <div class="conn-test" class:ok={connTest.startsWith('ok:')}>
                      {connTest.startsWith('ok:') ? '✓ ' + t('settings.connOk') + ' — ' + connTest.slice(3) : '✕ ' + connTest.slice(4)}
                    </div>
                  {/if}
                {/each}
              {/if}
              <div class="mset-keyrow">
                <input
                  class="ctrl key-input" placeholder={t('settings.customModelPlaceholder')}
                  bind:value={customModel}
                  onkeydown={(e) => e.key === 'Enter' && customModel.trim() && useModel(customModel.trim())}
                />
                <button class="ctrl" disabled={busy !== '' || !customModel.trim()} onclick={() => useModel(customModel.trim())}>{t('settings.use')}</button>
              </div>
            </div>

            {#if errorMsg}
              <div class="mset-error">{errorMsg}</div>
            {/if}
          {/if}
        </div>
      </div>
    {:else if active === 'tools'}
      <h2>{t('settings.toolsHeading', { n: tools.length })}</h2>
      <p class="muted set-sub">{t('settings.toolsDesc')}</p>

      {#each toolGroups as g (g.key)}
        <!-- Heading outside the card, not boxed in with the rows: the card is
             the list, and a title sealed inside its own border reads as one
             more entry in it. -->
        <div class="group-head">
          <span class="group-title">{t('settings.toolSource_' + g.key)}</span>
          <span class="group-count">{t('settings.itemCount', { n: g.items.length })}</span>
        </div>
        <div class="settings-card">
          {#each g.items as s (s.name)}
            <div class="set-row">
              <button
                class="tool-row"
                onclick={() => (expandedTool = expandedTool === s.name ? '' : s.name)}
              >
                <div class="set-txt">
                  <div class="t">{s.name}</div>
                  <div class="d" class:clamp={expandedTool !== s.name}>{s.description || '—'}</div>
                </div>
              </button>
              {#if s.name === SPEECH_TOOL}
                <!-- A dropdown, not an expanding section: picking a model must
                     not shove the rest of the tool list down the page. -->
                <div class="tool-setting">
                  <button class="ctrl" disabled={speechBusy} onclick={() => (speechOpen = !speechOpen)}>
                    {activeSpeechLabel} {speechOpen ? '▴' : '▾'}
                  </button>
                  {#if speechOpen}
                    <button
                      class="drop-backdrop"
                      aria-label={t('settings.close')}
                      onclick={() => (speechOpen = false)}
                    ></button>
                    <div class="rowdrop-list">
                      {#if speechStatus}<div class="rowdrop-note mset-error">{speechStatus}</div>{/if}
                      {#if speechModels.length === 0}
                        <div class="rowdrop-note muted">{t('settings.speechNoModels')}</div>
                      {:else}
                        <button
                          class="rowdrop-opt"
                          class:selected={speechModels.every((m) => !m.active)}
                          onclick={() => pickSpeechModel('')}
                        >
                          <div class="t">{t('settings.speechAuto')}</div>
                          <div class="sub">{t('settings.speechAutoDesc')}</div>
                        </button>
                        {#each speechModels as m (m.path)}
                          <div class="rowdrop-row">
                            <button
                              class="rowdrop-opt"
                              class:selected={m.active}
                              onclick={() => pickSpeechModel(m.path)}
                            >
                              <div class="t">{m.name}</div>
                              <div class="sub">{m.sizeMB} MB · {m.store}</div>
                            </button>
                            <button
                              class="rowdrop-reveal"
                              title={m.where}
                              aria-label={t('settings.speechOpenFolder')}
                              onclick={() => RevealSpeechModel(m.path)}
                            >📁</button>
                          </div>
                        {/each}
                      {/if}
                      {#if speechError}<div class="rowdrop-note mset-error">{speechError}</div>{/if}

                      <!-- Where the scan looks. Without it a missing model is a
                           dead end; with it, it is "put the file in one of
                           these". -->
                      <div class="rowdrop-sep"></div>
                      <div class="rowdrop-note muted">{t('settings.speechScanned')}</div>
                      {#each speechDirs as d (d.path)}
                        <button class="rowdrop-opt rowdrop-dir" onclick={() => OpenSpeechModelDir(d.path)}>
                          📁 {d.label}
                        </button>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/each}
    {:else if active === 'skills'}
      <h2>{t('settings.skills')}</h2>
      <p class="muted set-sub">{t('settings.skillsDesc')}</p>

      <div class="settings-card">
        <div class="card-form">
          <div class="mset-keyrow">
            <div class="eyebrow" style="flex:1">{t('settings.skillsInstalled')}</div>
            <button class="ctrl" disabled={skillBusy !== ''} onclick={refreshSkills}>
              {skillBusy === 'refresh' ? t('settings.refreshing') : t('settings.refresh')}
            </button>
          </div>
        </div>
        {#if extSkills.length === 0}
          <div class="set-row"><div class="muted">{t('settings.noSkills')}</div></div>
        {:else}
          {#each extSkills as s (s.dir)}
            <div class="set-row">
              <div class="set-txt">
                <div class="t">{s.name}</div>
                <div class="d">{s.description || '—'}</div>
                <div class="d mono-dim">{s.dir}</div>
              </div>
              <button class="ctrl" class:danger={skillConfirm === s.name} disabled={skillBusy !== ''} onclick={() => removeSkill(s.name)}>
                {skillConfirm === s.name ? t('settings.confirmRemove') : t('settings.remove')}
              </button>
            </div>
          {/each}
        {/if}
      </div>

      <div class="settings-card">
        <div class="card-form">
          <div class="eyebrow">{t('settings.skillInstall')}</div>
          <div class="mset-keyrow">
            <input
              class="ctrl key-input" placeholder={t('settings.skillInstallPlaceholder')}
              bind:value={skillInstallUrl}
              onkeydown={(e) => e.key === 'Enter' && skillInstallUrl.trim() && installSkill()}
            />
            <button class="ctrl" disabled={skillBusy !== '' || !skillInstallUrl.trim()} onclick={installSkill}>
              {skillBusy === 'install' ? t('settings.installing') : t('settings.install')}
            </button>
          </div>
          <div class="d muted">{t('settings.skillInstallHint')}</div>
          {#if skillInstallResult}<pre class="skill-result">{skillInstallResult}</pre>{/if}
          {#if skillError}<div class="mset-error">{skillError}</div>{/if}
        </div>
      </div>

    {:else if active === 'agents'}
      <h2>{t('settings.subagents')}</h2>
      <p class="muted set-sub">{t('settings.subagentsDesc')}</p>
      <!-- Real and editable, but nothing can spawn one until the `task` tool
           lands (§44 step 4). Saying so beats looking broken. -->
      <p class="muted set-sub">{t('settings.agentsSubSoon')}</p>

      {#if agentEditing === null}
        <div class="pp-bar">
          <button class="ctrl" onclick={newAgent}>{t('settings.agentNew')}</button>
          <button class="ctrl" onclick={() => loadAgents()}>{t('settings.refresh')}</button>
          <button class="ctrl" onclick={() => OpenSubagentsFolder()}>{t('settings.agentsFolder')}</button>
        </div>
        {#if agentError}<div class="mset-error">{agentError}</div>{/if}

        <!-- Two cards, split by who wrote it — the question this page is actually
             asked. Built-ins are second because a fresh install has only those and
             the interesting list is the one you grow. -->
        {#each [{ id: 'mine', rows: mySubagents, label: t('settings.subagentsMine'), hint: t('settings.subagentsMineHint') },
                { id: 'builtin', rows: builtinSubagents, label: t('settings.subagentsBuiltin'), hint: t('settings.subagentsBuiltinHint') }] as group (group.id)}
          <div class="settings-card">
            <div class="card-form">
              <div class="eyebrow">{group.label} <span class="ag-count">{group.rows.length}</span></div>
              <div class="d muted">{group.hint}</div>
            </div>
            {#if group.rows.length === 0}
              <div class="set-row"><div class="muted">{t('settings.subagentsNoneOfMine')}</div></div>
            {/if}
            {#each group.rows as a (a.name)}
              <div class="set-row">
                <div class="set-txt">
                  <div class="t">
                    {a.name}
                    {#if a.overrides}<span class="tag ag-override">{t('settings.agentOverrides')}</span>{/if}
                    {#if a.model}<span class="tag">{a.model}</span>{/if}
                    <span class="tag">{toolBadge(a)}</span>
                    {#if a.deny && a.deny.length > 0}<span class="tag ag-deny">{t('settings.agentDenyCount', { n: a.deny.length })}</span>{/if}
                    <span class="tag">{t('settings.agentSteps', { n: a.steps || 24 })}</span>
                  </div>
                  <div class="d">{a.description || '—'}</div>
                  <div class="d mono-dim">{a.path || 'built-in:' + a.name}</div>
                </div>
                <select
                  class="ctrl" value={a.model ?? ''} disabled={agentBusy !== ''}
                  onchange={(e) => pinModel(a.name, e.currentTarget.value)}
                >
                  <option value="">{t('settings.agentModelInherit')}</option>
                  {#each agentModels as m}<option value={m}>{m}</option>{/each}
                  {#if a.model && !agentModels.includes(a.model)}<option value={a.model}>{a.model}</option>{/if}
                </select>
                <button class="ctrl" disabled={agentBusy !== ''} onclick={() => openAgent(a)}>{t('settings.edit')}</button>
              </div>
            {/each}
          </div>
        {/each}
        <p class="muted set-sub">{t('settings.agentsHint')}</p>
      {:else}
        <div class="pp-bar">
          <button class="ctrl" onclick={() => (agentEditing = null)}>← {t('settings.agentBack')}</button>
          <div style="flex:1"></div>
          {#if !agentEditing.builtin && agentEditing.name}
            <button class="ctrl" class:danger={agentConfirmDelete} disabled={agentBusy !== ''} onclick={deleteAgent}>
              {agentConfirmDelete
                ? t('settings.confirmRemove')
                : agentEditing.overrides ? t('settings.agentRevert') : t('settings.remove')}
            </button>
          {/if}
          <button class="ctrl" disabled={agentBusy !== '' || !agentDraftName.trim() || !agentDraftBody.trim()} onclick={saveAgent}>
            {agentBusy === 'save' ? t('settings.saving') : t('settings.promptSave')}
          </button>
        </div>

        {#if agentEditing.builtin}
          <p class="muted set-sub">{t('settings.agentOverrideNote')}</p>
        {/if}
        {#if agentError}<div class="mset-error">{agentError}</div>{/if}

        <div class="settings-card">
          <div class="card-form pp-edit">
            <label class="pp-field">
              <span class="eyebrow">{t('settings.agentName')}</span>
              <input class="ctrl" bind:value={agentDraftName} placeholder="backend" disabled={agentEditing.name !== ''} />
            </label>
            <label class="pp-field">
              <span class="eyebrow">{t('settings.agentBody')}</span>
              <textarea class="ctrl ag-body" bind:value={agentDraftBody} spellcheck="false"></textarea>
              <span class="d muted">{t('settings.agentBodyHint')}</span>
            </label>
          </div>
        </div>
      {/if}

    {:else if active === 'prompts'}
      <h2>{t('settings.prompts')}</h2>
      <p class="muted set-sub">{t('settings.promptsDesc')}</p>

      {#if editing === null}
        <div class="pp-bar">
          <button class="ctrl" onclick={() => loadPresets()}>{t('settings.refresh')}</button>
          <button class="ctrl" onclick={() => OpenPromptsFolder()}>{t('settings.promptsFolder')}</button>
        </div>
        <div class="pp-grid">
          <button class="pp-card pp-new" onclick={newPreset}>
            <span class="pp-plus">+</span>
            <span class="pp-newtxt">{t('settings.promptNew')}</span>
          </button>
          {#each presets as p (p.name)}
            <button class="pp-card" onclick={() => openPreset(p)}>
              <span class="pp-cover" style="--h:{coverHue(p.name)}">
                {#if p.image}
                  <img src={p.image} alt="" />
                {:else}
                  <span class="pp-mono">/{p.name}</span>
                {/if}
              </span>
              <span class="pp-body">
                <span class="pp-title">
                  /{p.name}
                  {#if p.builtin}<span class="badge on">{t('settings.promptBuiltin')}</span>{/if}
                </span>
                <span class="pp-desc">{p.description || '—'}</span>
              </span>
            </button>
          {/each}
        </div>
        <p class="muted set-sub">{t('settings.promptsHint')}</p>
      {:else}
        <div class="pp-bar">
          <button class="ctrl" onclick={() => (editing = null)}>← {t('settings.promptBack')}</button>
          <div style="flex:1"></div>
          {#if !editing.builtin && editing.name}
            <button class="ctrl" class:danger={confirmDelete} disabled={presetBusy !== ''} onclick={deletePreset}>
              {confirmDelete ? t('settings.confirmRemove') : t('settings.remove')}
            </button>
          {/if}
          <button class="ctrl" disabled={presetBusy !== '' || !draftName.trim() || !draftBody.trim()} onclick={savePreset}>
            {presetBusy === 'save' ? t('settings.installing') : t('settings.promptSave')}
          </button>
        </div>

        {#if editing.builtin}
          <p class="muted set-sub">{t('settings.promptOverrideNote')}</p>
        {/if}

        <div class="settings-card">
          <div class="card-form pp-edit">
            <label class="pp-field">
              <span class="eyebrow">{t('settings.promptName')}</span>
              <input class="ctrl" bind:value={draftName} placeholder="landing" disabled={editing.name !== ''} />
            </label>

            <div class="pp-field">
              <span class="eyebrow">{t('settings.promptCover')}</span>
              <div class="pp-coveredit">
                <span class="pp-cover lg" style="--h:{coverHue(draftName || 'x')}">
                  {#if draftImage}<img src={draftImage} alt="" />{:else}<span class="pp-mono">/{draftName || '…'}</span>{/if}
                </span>
                <div class="pp-coverbtns">
                  <button class="ctrl" disabled={presetBusy !== ''} onclick={pickImage}>{t('settings.promptPickImage')}</button>
                  {#if draftImage}
                    <button class="ctrl" disabled={presetBusy !== ''} onclick={dropImage}>{t('settings.promptDropImage')}</button>
                  {/if}
                  <div class="d muted">{t('settings.promptCoverHint')}</div>
                </div>
              </div>
            </div>

            <div class="pp-field">
              <div class="pp-bodyhead">
                <span class="eyebrow" style="flex:1">{t('settings.promptBody')}</span>
                <button class="ctrl tiny" onclick={insertArguments}>+ $ARGUMENTS</button>
              </div>
              <textarea
                class="ctrl pp-textarea"
                bind:this={bodyEl}
                bind:value={draftBody}
                spellcheck="false"
                placeholder={t('settings.promptBodyPlaceholder')}
              ></textarea>
              <div class="d muted">{t('settings.promptBodyHint')}</div>
            </div>

            {#if presetError}<div class="mset-error">{presetError}</div>{/if}
          </div>
        </div>
      {/if}
    {:else if active === 'identity'}
      <h2>{t('sidebar.identity')}</h2>
      <div class="settings-card">
        <div class="identity-body">
          <div class="identity-files">
            {#each identity.files as f (f.name)}
              <div class="identity-file" class:active={identity.activeName === f.name}>
                <button type="button" class="identity-file-open" onclick={() => openIdentityFile(f.name)}>
                  <span class="ic">📄</span>
                  <span class="t">{f.name}</span>
                </button>
                <button type="button" class="identity-file-del" aria-label={t('settings.remove')} onclick={() => deleteIdentityFile(f.name)}>✕</button>
              </div>
            {/each}
            {#if identity.files.length === 0}
              <div class="empty">{t('sidebar.noIdentityFiles')}</div>
            {/if}
          </div>
          {#if missingTemplates.length > 0}
            <div class="identity-templates">
              {#each missingTemplates as tpl (tpl.name)}
                <button type="button" class="identity-template" onclick={() => createIdentityFile(tpl.name, tpl.content)}>
                  ＋ {tpl.name}
                </button>
              {/each}
            </div>
          {/if}
          <div class="identity-newfile">
            <input
              class="identity-newfile-input" placeholder={t('sidebar.newIdentityFile')}
              bind:value={newIdentityName}
              onkeydown={(e) => e.key === 'Enter' && addIdentityFile()}
            />
            <button type="button" class="icobtn tiny" aria-label={t('sidebar.newIdentityFile')} onclick={addIdentityFile}>＋</button>
          </div>
          {#if identity.activeName}
            <textarea
              class="identity-input" placeholder={t('sidebar.identityPlaceholder')}
              bind:value={identity.draft}
            ></textarea>
            <button
              type="button" class="ctrl identity-save"
              disabled={!identityDirty || identity.saving}
              onclick={saveIdentityFile}
            >
              {identity.saving ? t('settings.saving') : t('settings.save')}
            </button>
          {/if}
        </div>
      </div>
    {:else if active === 'usage'}
      <h2>{t('settings.usage')}</h2>
      <p class="muted set-sub">{t('settings.usageDesc')}</p>

      {#if usageError}<div class="mset-error">{usageError}</div>{/if}

      {#if usage && usage.totals.calls > 0}
        {@const tot = usage.totals}
        <div class="stat-cards">
          <div class="stat-card wide">
            <div class="eyebrow">{t('settings.usageTotalTokens')}</div>
            <div class="stat-big">{fmtCompact(tot.promptTokens + tot.completionTokens)}</div>
            <div class="stat-split" aria-hidden="true">
              <span class="seg in" style="flex:{Math.max(tot.promptTokens, 1)}"></span>
              <span class="seg out" style="flex:{Math.max(tot.completionTokens, 1)}"></span>
            </div>
            <div class="stat-legend">
              <span><i class="dot in"></i>{t('settings.usageInput')} {fmtCompact(tot.promptTokens)}</span>
              <span><i class="dot out"></i>{t('settings.usageOutput')} {fmtCompact(tot.completionTokens)}</span>
            </div>
          </div>

          <div class="stat-card wide">
            <div class="eyebrow">{t('settings.usageCacheHitRate')}</div>
            {#if tot.cacheRows === 0}
              <div class="stat-big dim">—</div>
              <div class="stat-sub">{t('settings.usageCacheUnreported')}</div>
            {:else}
              <div class="stat-big">{pct(tot.cachedTokens, tot.promptTokens)}<span class="unit">%</span></div>
              <div class="stat-split" aria-hidden="true">
                <span class="seg hit" style="flex:{Math.max(tot.cachedTokens, 1)}"></span>
                <span class="seg miss" style="flex:{Math.max(tot.uncachedTokens, 1)}"></span>
              </div>
              <div class="stat-legend">
                <span><i class="dot hit"></i>{t('settings.usageHit')} {fmtCompact(tot.cachedTokens)}</span>
                <span><i class="dot miss"></i>{t('settings.usageMiss')} {fmtCompact(tot.uncachedTokens)}</span>
              </div>
            {/if}
          </div>

          <div class="stat-card">
            <div class="eyebrow">{t('settings.usageCalls')}</div>
            <div class="stat-big">{fmtCompact(tot.calls)}</div>
            <div class="stat-sub">{t('settings.usageMessages')} {fmtTokens(tot.messages)}</div>
          </div>

          <div class="stat-card">
            <div class="eyebrow">{t('settings.usageSessions')}</div>
            <div class="stat-big">{fmtCompact(tot.sessions)}</div>
            <div class="stat-sub">{t('settings.usageActiveDays')} {tot.activeDays}</div>
          </div>

          <div class="stat-card">
            <div class="eyebrow">{t('settings.usageStreak')}</div>
            <div class="stat-big">{tot.currentStreak}<span class="unit">{t('settings.usageDaysUnit')}</span></div>
            <div class="stat-sub">{t('settings.usageActiveDays')} {tot.activeDays}</div>
          </div>

          <div class="stat-card">
            <div class="eyebrow">{t('settings.usageTopModel')}</div>
            <div class="stat-model">{tot.topModel || '—'}</div>
            <div class="stat-sub">{tot.topModelShare}% {t('settings.usageOfTokens')}</div>
          </div>
        </div>

        {#if dailyChart}
          <div class="settings-card wide-card">
            <div class="card-form">
              <div class="chart-head">
                <div class="eyebrow">{t('settings.usagePerDay')}</div>
                <!-- two keys, because the bar carries two encodings: hue names
                     the model, fill names where the tokens came from -->
                <div class="chart-legend">
                  {#each usage.all.slice(0, 5) as r (r.model)}
                    <span><i class="dot s{slotOf(r.model)}"></i>{r.model}</span>
                  {/each}
                  {#if usage.all.length > 5}
                    <span><i class="dot s0"></i>{t('settings.usageOther')}</span>
                  {/if}
                </div>
              </div>
              <div class="chart-head">
                <div class="chart-legend kind-legend">
                  {#each KINDS as kind (kind)}
                    <span title={kind === 'raw' ? t('settings.usageCacheUnreported') : ''}>
                      <i class="dot k-{kind}"></i>{kindLabel[kind]}
                    </span>
                  {/each}
                </div>
              </div>

              <div class="chart-body">
                <div class="chart-y" aria-hidden="true">
                  {#each dailyChart.ticks as tick (tick.frac)}
                    <span>{fmtCompact(tick.value)}</span>
                  {/each}
                </div>
                <div class="chart-plot" role="img" aria-label={t('settings.usagePerDay')}>
                  {#each dailyChart.ticks as tick (tick.frac)}
                    <div class="chart-gridline" style="bottom:{tick.frac * 100}%"></div>
                  {/each}
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <div class="daychart" onpointerleave={() => (hoverDay = null)}>
                    {#each dailyChart.days as d, i (d.day)}
                      <div
                        class="daycol"
                        class:on={hoverDay === i}
                        class:idle={d.total === 0}
                        onpointerenter={() => (hoverDay = i)}
                      >
                        <!-- idle days get their baseline tick from CSS; an inline
                             height:0 here would win and erase it -->
                        <div class="daybar" style={d.total === 0 ? '' : `height:${Math.max(2, (d.total / dailyChart.max) * 100)}%`}>
                          {#each d.parts as part (part.kind + part.model)}
                            <span class="k-{part.kind} s{slotOf(part.model)}" style="flex:{part.value}"></span>
                          {/each}
                        </div>
                      </div>
                    {/each}
                  </div>
                  {#if hoveredColumn && hoverDay !== null}
                    <div
                      class="chart-tip"
                      style="left:{((hoverDay + 0.5) / dailyChart.days.length) * 100}%; bottom:{Math.min(88, (hoveredColumn.total / dailyChart.max) * 100 + 6)}%"
                    >
                      <div class="tip-day">{hoveredColumn.day}</div>
                      {#if hoveredColumn.total === 0}
                        <div class="tip-row muted">{t('settings.usageNoActivity')}</div>
                      {:else}
                        {#each KINDS as kind (kind)}
                          {#if hoveredColumn.byKind[kind] > 0}
                            <div class="tip-row">
                              <i class="dot k-{kind}"></i>{kindLabel[kind]}
                              <span class="val">{fmtTokens(hoveredColumn.byKind[kind])}</span>
                            </div>
                          {/if}
                        {/each}
                        <div class="tip-sep"></div>
                        {#each hoveredColumn.models as [model, value] (model)}
                          <div class="tip-row">
                            <i class="dot s{slotOf(model)}"></i>{model}
                            <span class="val">{fmtTokens(value)}</span>
                          </div>
                        {/each}
                      {/if}
                    </div>
                  {/if}
                </div>
                <div></div>
                <div class="chart-x" aria-hidden="true">
                  {#each chartXLabels as label, i (i)}<span>{label}</span>{/each}
                </div>
              </div>
            </div>
          </div>
        {/if}

        <div class="settings-card wide-card">
          <div class="card-form">
            <div class="eyebrow">{t('settings.usageHeatmap')}</div>
            <div class="heatmap">
              {#each heatmap.weeks as week, w (w)}
                <div class="heat-week">
                  {#each week as cell (cell.day)}
                    <span
                      class="heat-cell l{cell.future ? 'x' : heatLevel(cell.value, heatmap.max)}"
                      title={cell.future ? '' : `${cell.day} · ${fmtTokens(cell.value)}`}
                    ></span>
                  {/each}
                </div>
              {/each}
            </div>
            <div class="chart-legend heat-scale">
              <span>{t('settings.usageLess')}</span>
              <i class="heat-cell l0"></i><i class="heat-cell l1"></i><i class="heat-cell l2"></i>
              <i class="heat-cell l3"></i><i class="heat-cell l4"></i>
              <span>{t('settings.usageMore')}</span>
            </div>
          </div>
        </div>
      {/if}

      <div class="settings-card wide-card">
        <div class="card-form">
          <div class="usage-toolbar">
            <div class="eyebrow">{t('settings.usageByModel')}</div>
            <div class="seg-ctrl">
              {#each [
                { id: 'today', label: t('settings.usageToday') },
                { id: 'week', label: t('settings.usageWeek') },
                { id: 'all', label: t('settings.usageAll') },
              ] as opt (opt.id)}
                <button
                  type="button"
                  class="seg-btn"
                  class:selected={usagePeriod === opt.id}
                  onclick={() => (usagePeriod = opt.id as typeof usagePeriod)}
                >{opt.label}</button>
              {/each}
            </div>
          </div>
        </div>
        {#if usageRows.length === 0}
          <div class="set-row"><div class="muted">{t('settings.usageEmpty')}</div></div>
        {:else}
          <div class="set-row usage-head">
            <div class="u-model">{t('settings.usageModel')}</div>
            <div class="u-num">{t('settings.usageInput')}</div>
            <div class="u-num">{t('settings.usageCached')}</div>
            <div class="u-num">{t('settings.usageOutput')}</div>
            <div class="u-num sm">{t('settings.usageCalls')}</div>
            <div class="u-num sm">{t('settings.usageAvgCall')}</div>
          </div>
          {#each usageRows as r (r.model)}
            <div class="set-row usage-row">
              <div class="u-model">
                <i class="dot s{slotOf(r.model)}"></i>{r.model}
                <span class="u-share" style="width:{pct(usageTotal(r), periodTotal)}%"></span>
              </div>
              <div class="u-num">{fmtTokens(r.promptTokens)}</div>
              <div class="u-num">
                {#if r.cacheRows === 0}
                  <span class="dim" title={t('settings.usageCacheUnreported')}>—</span>
                {:else}
                  {pct(r.cachedTokens, r.promptTokens)}%
                  <span class="u-sub">{fmtCompact(r.cachedTokens)}</span>
                {/if}
              </div>
              <div class="u-num">{fmtTokens(r.completionTokens)}</div>
              <div class="u-num sm">{fmtTokens(r.calls)}</div>
              <div class="u-num sm">{fmtCompact(Math.round(usageTotal(r) / Math.max(r.calls, 1)))}</div>
            </div>
          {/each}
        {/if}
      </div>
    {:else if active === 'mcp'}
      <h2>{t('settings.mcpServers')}</h2>
      <p class="muted set-sub">{t('settings.mcpDesc')}</p>

      <div class="settings-card">
        {#if mcpServers.length > 3}
          <div class="card-form">
            <input class="ctrl" placeholder={t('settings.mcpSearchPlaceholder')} bind:value={mcpQuery} />
          </div>
        {/if}
        {#if mcpServers.length === 0}
          <div class="muted">{t('settings.noMcpServers')}</div>
        {:else}
          {#each mcpFiltered as s (s.name)}
            <div class="set-row" class:mcp-off={s.disabled}>
              <div class="set-txt">
                <div class="t">
                  <span class="dot" style={statusColor(s.status)}></span> {s.name}
                  <span class="mcp-badge">{s.url ? 'http' : 'stdio'}</span>
                  {#if s.tools > 0}<span class="mcp-badge">{t('settings.mcpToolCount', { n: String(s.tools) })}</span>{/if}
                </div>
                <div class="d">{s.url || (s.command ?? []).join(' ')}{s.err ? ' — ' + s.err : ''}</div>
              </div>
              <div style="display:flex; gap:8px; align-items:center">
                <label class="mswitch" title={s.disabled ? t('settings.add') : ''}>
                  <input type="checkbox" checked={!s.disabled} disabled={mcpBusy !== ''} onchange={() => toggleMCP(s)} />
                  <span></span>
                </label>
                <button class="ctrl" disabled={mcpBusy !== '' || s.disabled} onclick={() => testMCP(s.name)}>
                  {mcpBusy === 'test:' + s.name ? t('settings.testing') : t('settings.test')}
                </button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => editMCP(s)}>{t('settings.edit')}</button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => removeMCP(s.name)}>{t('settings.remove')}</button>
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <div class="settings-card">
        <div class="card-form">
          <div class="eyebrow">{mcpOriginal ? t('settings.editServer') : t('settings.addServer')}</div>

          <div class="mset-keyrow">
            <select class="ctrl mcp-kind" bind:value={mcpKind}>
              <option value="stdio">stdio</option>
              <option value="http">http</option>
            </select>
            <input class="ctrl key-input" placeholder={t('settings.mcpNamePlaceholder')} bind:value={mcpName} />
          </div>

          {#if mcpKind === 'stdio'}
            <input class="ctrl" placeholder={t('settings.mcpCommandPlaceholder')} bind:value={mcpCommand} />
          {:else}
            <input class="ctrl" placeholder={t('settings.mcpUrlPlaceholder')} bind:value={mcpUrl} />
          {/if}

          {#if mcpKind === 'stdio'}
            <textarea class="ctrl mcp-lines" rows="2" placeholder={t('settings.mcpEnvPlaceholder')} bind:value={mcpEnvText}></textarea>
          {:else}
            <textarea class="ctrl mcp-lines" rows="2" placeholder={t('settings.mcpHeadersPlaceholder')} bind:value={mcpHeadersText}></textarea>
          {/if}

          <div class="mset-keyrow">
            <button class="ctrl" disabled={mcpBusy !== '' || !mcpFormValid} onclick={saveMCP}>
              {mcpBusy === 'save' ? t('settings.saving') : (mcpOriginal ? t('settings.save') : t('settings.add'))}
            </button>
            {#if mcpOriginal}
              <button class="ctrl" disabled={mcpBusy !== ''} onclick={resetMCPForm}>{t('settings.cancel')}</button>
            {/if}
          </div>
          {#if mcpError}<div class="mset-error">{mcpError}</div>{/if}
        </div>
      </div>

      <div class="settings-card">
        <div class="card-form">
          <div class="eyebrow">{t('settings.mcpPresets')}</div>
        </div>
        {#each mcpPresets as p (p.name)}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{p.name} <span class="mcp-badge">{p.url ? 'http' : 'stdio'}</span></div>
              <div class="d">{p.desc} — {p.url ?? p.command?.join(' ')}</div>
            </div>
            <button class="ctrl" disabled={mcpBusy !== '' || presetTaken(p.name)} onclick={() => addPreset(p)}>
              {mcpBusy === 'preset:' + p.name ? t('settings.adding') : t('settings.add')}
            </button>
          </div>
        {/each}
      </div>
    {:else if active === 'sponsor'}
      <h2>{t('settings.sponsor')}</h2>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.sponsorIntro')}</div>
            <div class="d">{t('settings.sponsorDesc')}</div>
          </div>
          <button class="ctrl" onclick={() => BrowserOpenURL(SITE_URL)}>{t('settings.sponsorOpenSite')}</button>
          <button class="ctrl" onclick={() => BrowserOpenURL(SPONSOR_URL)}>{t('settings.sponsorOpenGitHub')}</button>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">PromptPay</div>
            <div class="d">{t('settings.sponsorScanHint')}</div>
          </div>
        </div>
        <div class="set-row" style="justify-content:center">
          <img src={promptPayQR} alt="PromptPay QR" style="width:260px;max-width:100%;border-radius:12px" />
        </div>
      </div>
    {/if}
    </div>
  </div>
</div>
