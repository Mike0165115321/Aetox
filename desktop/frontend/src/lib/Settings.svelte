<script lang="ts">
  import { onMount } from 'svelte'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import { editorFont, applyEditorFontSize } from './editorFont.svelte'
  import { chatFont, applyChatFontSize } from './chatFont.svelte'
  import { uiFont, applyUiFont, UI_FONTS, type UiFontName } from './uiFont.svelte'
  import { editorTheme, setBuiltinEditorTheme, setAutoEditorTheme, importThemeFile } from './editorTheme.svelte'
  import { treeFont, applyTreeFontSize } from './treeFont.svelte'
  import { systemZoom, applySystemZoom, SYSTEM_BASE_PX } from './systemFont.svelte'
  import { typeScale, applyTypeScale, TYPE_SCALES, type TypeScaleName } from './typeScale.svelte'
  import { i18n, t, setLocale, localeNames, type Locale, type TKey } from './i18n.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import ProviderMark from './ProviderMark.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'
  import {
    SupportedProviders, HasAPIKey, RequiresAPIKey, TerminalShells,
    ListModelsForProvider, ProviderBaseURL, ProviderBaseURLIsCustom,
    ProviderWireFormats, TestProviderConnection,
    EnabledProviders, SetProviderEnabled,
    ListMCPServers, SaveMCPServer, RemoveMCPServer, TestMCPServer, ToggleMCPServer,
    ListExternalSkills, ListTools, InstallSkillFromGitHub, RemoveExternalSkill, RefreshSkills,
    SkillsDir, SkillScanIssues, OpenSkillsFolder, InstallSkillFromZip,
    MCPConfigPath, OpenMCPFolder,
    ListSpeechModels, SetSpeechModel, SpeechStatus, RevealSpeechModel, SpeechModelDirs, OpenSpeechModelDir,
    UsageStats, ListPromptPresets, OpenPromptsFolder,
    SavePromptPreset, DeletePromptPreset, PickPresetImage, RemovePresetImage,
    ListSubagentProfiles, ReadSubagentProfile, SaveSubagentProfile,
    DeleteSubagentProfile, SetSubagentModel, OpenSubagentsFolder,
    SignInMethods, SignInStatus, StartSignIn, CancelSignIn, ImportableSignIns,
    AppVersion, CheckForUpdate,
    LearningEnabled, SetLearningEnabled, ListPendingChanges, ListDecidedChanges,
    ApprovePendingChange, RejectPendingChange, LearnedMemory, OpenMemoryFolder,
  } from '../../wailsjs/go/main/App'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import promptPayQR from '../assets/images/promptpay-qr.png'
  import { config, update, main } from '../../wailsjs/go/models'
  import { cockpit, switchProvider, switchModel, submitAPIKey, switchApprovalMode, switchWireFormat, setProviderBaseURL, retryActiveProvider, completeSignIn, signOutProvider, importSignIn } from './stores/cockpit.svelte'
  import {
    identity, loadIdentityFiles, openIdentityFile, saveIdentityFile,
    createIdentityFile, deleteIdentityFile, identityTemplates,
  } from './identity.svelte'

  let { onClose }: { onClose: () => void } = $props()

  // ---------- Destructive actions ----------
  // One gate for everything that cannot be undone. This page used to have two
  // different answers to the same question: Skills, Prompts and Sub-agents
  // armed on the first click and deleted on the second, while MCP servers,
  // providers and identity files deleted on the first click with no warning at
  // all. Learning "it asks first" from one page and then losing a configured
  // MCP server on the next is the worst of both.
  type PendingConfirm = {
    title: string
    message: string
    /** The exact name/path being destroyed — shown verbatim for checking. */
    detail?: string
    confirmLabel: string
    run: () => void
  }
  let pendingConfirm = $state<PendingConfirm | null>(null)

  function askConfirm(req: PendingConfirm) {
    pendingConfirm = req
  }

  function runPendingConfirm() {
    const req = pendingConfirm
    pendingConfirm = null
    req?.run()
  }

  // Leaving a full-page editor with unsaved work is the same class of loss as a
  // delete — the work is gone and nothing says so — so it goes through the same
  // gate. Dirty is measured against a snapshot taken when the editor opened,
  // not against a field-by-field comparison: the sub-agent editor has seven
  // drafts and the diff only ever gets asked one question.
  function guardUnsaved(dirty: boolean, leave: () => void) {
    if (!dirty) { leave(); return }
    askConfirm({
      title: t('settings.unsavedTitle'),
      message: t('settings.unsavedMessage'),
      confirmLabel: t('settings.unsavedAction'),
      run: leave,
    })
  }

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

  const removeIdentityFile = (name: string) => askConfirm({
    title: t('settings.confirmIdentityTitle'),
    message: t('settings.confirmIdentityMessage'),
    detail: name,
    confirmLabel: t('settings.confirmDeleteAction'),
    run: () => deleteIdentityFile(name),
  })

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

  // ---------- Sign-in (use the plan you already pay for) ----------
  type SignInMethod = { provider: string; label: string; kind: string; risk: string; note: string }
  type SignInPrompt = { provider: string; kind: string; url: string; user_code?: string; verification_uri?: string }

  let signInMethods = $state<SignInMethod[]>([])
  let signedIn = $state<Record<string, { signed_in: boolean; label?: string; account?: string }>>({})
  // The authorization currently on screen. Only one at a time: the flow blocks
  // on the user, and two half-finished sign-ins is a state nobody can reason
  // about.
  let signInPrompt = $state<SignInPrompt | null>(null)
  let signInCode = $state('')
  let signInError = $state('')
  // Providers whose official CLI is already signed in on this machine, so the
  // user can adopt that session instead of authorizing the same account twice.
  let importable = $state<string[]>([])

  const signInProviderNames = $derived(new Set(signInMethods.map((m) => m.provider)))
  const signInMethod = $derived(signInMethods.find((m) => m.provider === selected) ?? null)
  const signInStatus = $derived(signedIn[selected] ?? null)
  let busy = $state('')
  let errorMsg = $state('')

  const selectedRow = $derived(providers.find((p) => p.name === selected))
  const isActiveProvider = $derived(cockpit.model.provider === selected)
  const enabledRows = $derived(providers.filter((p) => enabledNames.includes(p.name)))
  const addableRows = $derived(providers.filter((p) => !enabledNames.includes(p.name)))
  // Split, because the two kinds ask for completely different things: one wants
  // a browser and the plan you already pay for, the other wants a key you have
  // to go find. Mixing them in one alphabetical list hid the sign-ins.
  const addableSignIn = $derived(addableRows.filter((p) => signInProviderNames.has(p.name)))
  const addableKeyed = $derived(addableRows.filter((p) => !signInProviderNames.has(p.name)))
  // Only meaningful while this provider is the active one — otherwise nothing
  // has been bootstrapped for it yet, so show what would be the default.
  const currentWireFormat = $derived(isActiveProvider ? cockpit.model.wireFormat : (wireFormats[0] ?? ''))

  // Whether the first load finished, and why it didn't. Without this the whole
  // page was one unguarded await chain: a single throw from TerminalShells()
  // left providers, sign-in, MCP and skills all unloaded, and the user got a
  // blank Settings page with nothing saying anything had gone wrong.
  let booting = $state(true)
  let bootError = $state('')

  async function bootSettings() {
    booting = true
    bootError = ''
    try {
      // Three independent groups, run together rather than in a queue. They
      // were sequential, which made the tool list — needed by the sub-agent
      // editor — the last thing to arrive after every provider round-trip, so
      // opening a sub-agent quickly could find it still empty. Only the
      // provider chain has an internal order.
      await Promise.all([
        (async () => {
          shells = await TerminalShells()
          if (!shells.some((s) => s.path === defaultShell)) defaultShell = shells[0]?.path ?? ''
        })(),
        loadMCP(),
        loadSkills(),
        (async () => {
          await refreshProviders()
          await refreshEnabledProviders()
          await refreshSignIn()
          await selectProvider(cockpit.model.provider || enabledRows[0]?.name || providers[0]?.name || '')
        })(),
      ])
    } catch (err) {
      bootError = String(err)
    } finally {
      booting = false
    }
  }

  onMount(bootSettings)

  // ---------- About ----------
  // Kept out of bootSettings on purpose. The version is a constant the Go side
  // always has, and folding it into that Promise.all would let a stumble here
  // take the whole Settings page down with it for nothing.
  let appVersion = $state('')
  let updateStatus = $state<update.Status | null>(null)
  let updateChecking = $state(false)
  let updateError = $state('')
  let hintCopied = $state(false)

  onMount(async () => {
    try {
      appVersion = await AppVersion()
    } catch {
      /* the About page shows a dash rather than an error */
    }
  })

  const CHANNEL_LABELS: Record<string, TKey> = {
    scoop: 'settings.aboutChannelScoop',
    installer: 'settings.aboutChannelInstaller',
    portable: 'settings.aboutChannelPortable',
    unknown: 'settings.aboutChannelUnknown',
  }

  async function checkForUpdate() {
    updateChecking = true
    updateError = ''
    try {
      updateStatus = await CheckForUpdate()
    } catch (err) {
      // Offline, rate-limited, proxy in the way: say so and change nothing
      // else. A failed check is not a broken app.
      updateStatus = null
      updateError = String(err)
    } finally {
      updateChecking = false
    }
  }

  async function copyUpgradeHint(command: string) {
    try {
      await navigator.clipboard.writeText(command)
      hintCopied = true
      setTimeout(() => (hintCopied = false), 1500)
    } catch {
      /* clipboard blocked — the command is on screen to be typed */
    }
  }

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

  async function refreshSignIn() {
    signInMethods = (await SignInMethods()) ?? []
    const entries = await Promise.all(
      signInMethods.map(async (m) => [m.provider, await SignInStatus(m.provider)] as const),
    )
    signedIn = Object.fromEntries(entries)
    importable = (await ImportableSignIns()) ?? []
  }

  // Two calls, not one: the first returns what to show the user (a code to
  // type, a page to visit), the second blocks until they finish. Device and
  // browser flows chain straight into the wait; only the paste flow stops here
  // for input.
  async function startSignIn() {
    const method = signInMethod
    if (!method) return
    signInError = ''
    signInCode = ''
    try {
      signInPrompt = await StartSignIn(method.provider)
    } catch (e) {
      signInError = String(e)
      return
    }
    if (signInPrompt.url) BrowserOpenURL(signInPrompt.url)
    if (method.kind !== 'paste') await finishSignIn()
  }

  async function finishSignIn() {
    const prompt = signInPrompt
    if (!prompt) return
    busy = 'signin'
    signInError = ''
    try {
      await completeSignIn(prompt.provider, signInCode.trim())
      signInPrompt = null
      signInCode = ''
      await refreshSignIn()
      await refreshProviders()
      // Re-select to pick up the model list, which was unreachable until now.
      await selectProvider(prompt.provider)
    } catch (e) {
      signInError = String(e)
    } finally {
      busy = ''
    }
  }

  async function abortSignIn() {
    const prompt = signInPrompt
    signInPrompt = null
    signInCode = ''
    signInError = ''
    if (prompt) await CancelSignIn(prompt.provider)
  }

  const doImport = (name: string) => run('import:' + name, async () => {
    await importSignIn(name)
    await refreshSignIn()
    await refreshProviders()
    await selectProvider(name)
  })

  const doSignOut = (name: string) => run('signout:' + name, async () => {
    await signOutProvider(name)
    await refreshSignIn()
    await refreshProviders()
  })

  const addProvider = (name: string) => run('enable:' + name, async () => {
    enabledNames = await SetProviderEnabled(name, true)
    showAddProvider = false
    await selectProvider(name)
  })

  const removeProvider = (name: string) => askConfirm({
    title: t('settings.confirmProviderTitle'),
    // Removing the running provider moves the engine as a side effect, which
    // is exactly the kind of thing a confirm exists to say out loud.
    message: cockpit.model.provider === name
      ? t('settings.confirmProviderMessage') + ' ' + t('settings.confirmProviderActive')
      : t('settings.confirmProviderMessage'),
    detail: name,
    confirmLabel: t('settings.remove'),
    run: () => run('disable:' + name, async () => {
      const wasActiveEngine = cockpit.model.provider === name
      enabledNames = await SetProviderEnabled(name, false)
      if (selected === name) await selectProvider(enabledNames[0] ?? '')
      // Removing the provider Aetox is actually running on must move the engine
      // too — otherwise it keeps running unlisted while the picker shows a
      // provider that's no longer selectable. Falls back to aetox (Aetox's own
      // built-in engine, always available, needs no key) rather than an
      // arbitrary "next" provider, since that's the deliberate safe default.
      if (wasActiveEngine) await switchProvider('aetox')
    }),
  })

  async function selectProvider(name: string) {
    if (!name) return
    // Walking away from a half-finished sign-in must release the listener it
    // opened, not leave it waiting for a redirect nobody will send.
    if (signInPrompt && signInPrompt.provider !== name) await abortSignIn()
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
    cwd?: string; timeoutMs?: number
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
  // Both were in the stored config all along with no field to reach them, so a
  // server needing a working directory or a slower start could only be set up
  // by editing the JSON — which the page did not say the location of either.
  let mcpCwd = $state('')
  let mcpTimeout = $state('')
  // Set when a preset was handed to the form because it needs a key, so the
  // form can say why it opened instead of just appearing.
  let mcpNeedsKey = $state(false)
  // Where the servers are persisted. From the engine, not written here.
  let mcpPath = $state('')

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
    mcpPath = await MCPConfigPath()
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
    mcpCwd = ''
    mcpTimeout = ''
    mcpNeedsKey = false
  }

  function editMCP(s: MCPRow) {
    mcpOriginal = s.name
    mcpKind = s.url ? 'http' : 'stdio'
    mcpName = s.name
    mcpCommand = (s.command ?? []).join(' ')
    mcpUrl = s.url ?? ''
    mcpEnvText = mapToLines(s.environment, '=')
    mcpHeadersText = mapToLines(s.headers, ': ')
    mcpCwd = s.cwd ?? ''
    mcpTimeout = s.timeoutMs ? String(s.timeoutMs) : ''
    mcpNeedsKey = false
    mcpError = ''
  }

  const saveMCP = () => runMCP('save', async () => {
    const server = new config.MCPServerConfig({
      name: mcpName.trim(),
      command: mcpKind === 'stdio' ? mcpCommand.trim().split(/\s+/).filter(Boolean) : [],
      url: mcpKind === 'http' ? mcpUrl.trim() : '',
      environment: mcpKind === 'stdio' ? parseLines(mcpEnvText, '=') : {},
      headers: mcpKind === 'http' ? parseLines(mcpHeadersText, ':') : {},
      cwd: mcpCwd.trim(),
      // A blank box means "no override", which is 0 — not a timeout of zero.
      timeoutMs: Number.parseInt(mcpTimeout, 10) > 0 ? Number.parseInt(mcpTimeout, 10) : 0,
    })
    await SaveMCPServer(mcpOriginal, server)
    resetMCPForm()
    await loadMCP()
  })

  const removeMCP = (name: string) => askConfirm({
    title: t('settings.confirmMcpTitle'),
    message: t('settings.confirmMcpMessage'),
    detail: name,
    confirmLabel: t('settings.remove'),
    run: () => runMCP('rm:' + name, async () => {
      await RemoveMCPServer(name)
      if (mcpOriginal === name) resetMCPForm()
      await loadMCP()
    }),
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
  // `headers` names what the server cannot work without. A preset that needs a
  // key used to be saved straight to disk with none, so one click produced a
  // server that could never connect and the page never said which header it
  // wanted — it knew, and did not tell.
  const mcpPresets: { name: string; desc: string; command?: string[]; url?: string; headers?: string[] }[] = [
    { name: 'context7', desc: 'Up-to-date library docs', command: ['npx', '-y', '@upstash/context7-mcp'] },
    { name: 'sequential-thinking', desc: 'Step-by-step reasoning scratchpad', command: ['npx', '-y', '@modelcontextprotocol/server-sequential-thinking'] },
    { name: 'memory', desc: 'Knowledge-graph memory', command: ['npx', '-y', '@modelcontextprotocol/server-memory'] },
    { name: 'js-repl', desc: 'Run JavaScript/Node code', command: ['npx', '-y', 'mcp-repl'] },
    { name: 'exa', desc: 'Web search', url: 'https://mcp.exa.ai/mcp', headers: ['x-api-key'] },
  ]

  const presetTaken = (name: string) => mcpServers.some((s) => s.name.toLowerCase() === name.toLowerCase())

  const addPreset = (p: (typeof mcpPresets)[number]) => runMCP('preset:' + p.name, async () => {
    if (p.headers?.length) {
      // Hand it to the form with the header names already in, rather than
      // saving something that cannot connect. Nothing is written until the key
      // is pasted and Save is pressed.
      resetMCPForm()
      mcpKind = p.url ? 'http' : 'stdio'
      mcpName = p.name
      mcpUrl = p.url ?? ''
      mcpCommand = (p.command ?? []).join(' ')
      mcpHeadersText = p.headers.map((h) => `${h}: `).join('\n')
      mcpNeedsKey = true
      return
    }
    await SaveMCPServer('', new config.MCPServerConfig({
      name: p.name, command: p.command ?? [], url: p.url ?? '',
    }))
    await loadMCP()
  })

  // Colours come from the theme, not from three hex literals. theme.css states
  // that every rule references only semantic tokens, and two of the three that
  // were here were --c-green-500 and --c-red-500 copied by value — so the dot
  // stayed dark-theme green on a light theme.
  function statusVar(status: string): string {
    if (status === 'connected') return 'background:var(--status-success)'
    if (status === 'failed') return 'background:var(--status-danger)'
    return 'background:var(--text-dim)'
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
  let skillBusy = $state('')
  let skillError = $state('')
  let skillInstallUrl = $state('')
  let skillInstallResult = $state('')
  // Where skills actually live, and which SKILL.md files were found but could
  // not be read. Both come from the engine: a path the page states on its own
  // authority is a path that can drift from the one being scanned, which is
  // exactly what had happened.
  let skillsDir = $state('')
  let skillIssues = $state<string[]>([])

  async function loadSkills() {
    extSkills = await ListExternalSkills()
    tools = await ListTools()
    skillsDir = await SkillsDir()
    skillIssues = (await SkillScanIssues()) ?? []
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

  // Below the state it reads, not above it: $derived is lazy so the old
  // ordering worked at runtime, but it put speechModels in its own temporal
  // dead zone as far as the compiler was concerned.
  const activeSpeechLabel = $derived(
    speechModels.find((m) => m.active)?.name ?? t('settings.speechAuto'),
  )

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

  // The picker is native, so there is nothing to pass in. An empty result means
  // the dialog was dismissed — cancelling is not a failure and must not leave a
  // stale report on screen.
  const installSkillZip = () => runSkill('zip', async () => {
    skillInstallResult = ''
    const report = await InstallSkillFromZip()
    if (!report) return
    skillInstallResult = report
    await loadSkills()
  })

  const removeSkill = (name: string, dir: string) => askConfirm({
    title: t('settings.confirmSkillTitle'),
    message: t('settings.confirmSkillMessage'),
    // The folder, not the name: this deletes something off disk, so the path
    // is the thing worth checking before agreeing to it.
    detail: dir || name,
    confirmLabel: t('settings.remove'),
    run: () => runSkill('rm:' + name, async () => {
      await RemoveExternalSkill(name)
      await loadSkills()
    }),
  })

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

  const presetDraftKey = () => JSON.stringify([draftName, draftBody, draftImage])
  let presetSnapshot = ''

  function openPreset(p: PresetRow) {
    editing = p
    draftName = p.name
    draftBody = p.body
    draftImage = p.image
    presetError = ''
    presetSnapshot = presetDraftKey()
  }

  const closePresetEditor = () =>
    guardUnsaved(presetDraftKey() !== presetSnapshot, () => { editing = null })

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
    presetSnapshot = presetDraftKey()
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

  const deletePreset = () => askConfirm({
    title: t('settings.confirmPromptTitle'),
    message: t('settings.confirmPromptMessage'),
    detail: '/' + draftName.trim(),
    confirmLabel: t('settings.confirmDeleteAction'),
    run: () => runPreset('delete', async () => {
      await DeletePromptPreset(draftName.trim())
      await loadPresets()
      editing = null
    }),
  })

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
  // The .md file is still `--- key: value ---` plus a role prompt underneath —
  // that has not changed, and SaveSubagentProfile still only ever receives
  // that same text. What changed is that the editor stopped asking a person to
  // read and hand-edit it: each frontmatter key gets its own field below, and
  // agentDraftModel is carried through untouched because the per-row dropdown
  // (pinModel) already owns that one key.
  let agentDraftDescription = $state('')
  let agentDraftModel = $state('')
  let agentDraftTools = $state<string[]>([])
  let agentDraftDeny = $state<string[]>([])
  let agentDraftSteps = $state('')
  let agentDraftPrompt = $state('')
  let agentBusy = $state('')
  let agentError = $state('')

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

  // The agent file is as long as the role you wrote, so a fixed box means
  // scrolling a small window inside a page that has room to spare. This grows
  // the field to its content instead; `min-height` in the CSS is still the
  // floor, so a short file looks exactly as it did before.
  //
  // Takes the text as its parameter rather than listening to `input` alone:
  // switching to another sub-agent replaces the value without any keystroke,
  // and a field left at the previous file's height is the bug this fixes.
  function autogrow(node: HTMLTextAreaElement, _value: string) {
    const fit = () => {
      // Collapse first: scrollHeight of an already-tall box reports the box,
      // not the text, so without this the field can only ever grow.
      node.style.height = 'auto'
      node.style.height = node.scrollHeight + 'px'
    }
    fit()
    node.addEventListener('input', fit)
    return {
      update: () => fit(),
      destroy: () => node.removeEventListener('input', fit),
    }
  }

  // Editing opens the raw .md — including for a bundled profile, where saving
  // writes your own copy over it (the engine already prefers user files).
  // Everything the editor can change, in one string. Compared against the value
  // captured when the editor opened (and re-captured on save) to answer the one
  // question the Back button needs answered.
  const agentDraftKey = () => JSON.stringify([
    agentDraftName, agentDraftDescription, agentDraftModel,
    agentDraftTools, agentDraftDeny, agentDraftSteps, agentDraftPrompt,
  ])
  let agentSnapshot = ''

  const openAgent = (a: SubagentRow) => runAgent('open:' + a.name, async () => {
    const parsed = parseAgentFile(await ReadSubagentProfile(a.name))
    agentDraftName = a.name
    agentDraftDescription = parsed.description
    agentDraftModel = parsed.model
    agentDraftTools = parsed.tools
    agentDraftDeny = parsed.deny
    agentDraftSteps = parsed.steps
    agentDraftPrompt = parsed.body
    agentEditing = a
    agentSnapshot = agentDraftKey()
  })

  function newAgent() {
    agentEditing = { name: '', description: '', prompt: '', builtin: false }
    agentDraftName = ''
    agentDraftDescription = ''
    agentDraftModel = ''
    agentDraftTools = []
    agentDraftDeny = []
    agentDraftSteps = ''
    agentDraftPrompt = t('settings.agentStarter')
    agentError = ''
    agentSnapshot = agentDraftKey()
  }

  const closeAgentEditor = () =>
    guardUnsaved(agentDraftKey() !== agentSnapshot, () => { agentEditing = null })

  const saveAgent = () => runAgent('save', async () => {
    await SaveSubagentProfile(agentDraftName.trim(), serializeAgentFile({
      description: agentDraftDescription,
      model: agentDraftModel,
      tools: agentDraftTools,
      deny: agentDraftDeny,
      steps: agentDraftSteps,
      body: agentDraftPrompt,
    }))
    await loadAgents()
    agentEditing = null
  })

  // Two different actions behind one button: deleting a profile the user wrote,
  // versus dropping an override so a built-in goes back to how it shipped. They
  // lose different things, so they say different things.
  const deleteAgent = () => {
    const reverting = agentEditing?.overrides === true
    askConfirm({
      title: reverting ? t('settings.confirmAgentRevertTitle') : t('settings.confirmAgentTitle'),
      message: reverting ? t('settings.confirmAgentRevertMessage') : t('settings.confirmAgentMessage'),
      detail: agentEditing?.path || agentDraftName.trim(),
      confirmLabel: reverting ? t('settings.confirmAgentRevertAction') : t('settings.confirmDeleteAction'),
      run: () => runAgent('delete', async () => {
        await DeleteSubagentProfile(agentDraftName.trim())
        await loadAgents()
        agentEditing = null
      }),
    })
  }

  // What the row says about tools has to be what the sub-agent actually gets: an
  // empty list means the whole registry, not zero tools.
  const toolBadge = (a: SubagentRow) =>
    a.tools && a.tools.length > 0 ? t('settings.agentToolCount', { n: a.tools.length }) : t('settings.agentAllTools')

  // A count alone ("2 denied") sends you to the .md file to find out which two.
  // The names are already on the row — the badge just has to say them.
  const toolBadgeTip = (a: SubagentRow) =>
    a.tools && a.tools.length > 0
      ? t('settings.agentToolsTip', { list: a.tools.join(', ') })
      : t('settings.agentAllToolsTip')

  const denyTip = (a: SubagentRow) => t('settings.agentDenyTip', { list: (a.deny ?? []).join(', ') })

  // What you may put in `tools:`/`deny:`. The editor is a raw .md field, so the
  // question it leaves you with is "what are the names?" — asking the running
  // registry beats a list written down here that drifts the day a tool is added.
  //
  // AGENT_FORCED_DENIALS mirrors subagent.forcedDenials: names a sub-agent never
  // gets no matter what the file says. Listing them as available would be a lie
  // the user only discovers after saving.
  const AGENT_FORCED_DENIALS = ['task', 'task_result', 'task_answer', 'help', 'ask_user', 'todo_write']
  // Mirrors subagent.stepsUnlimitedKeyword. The frontmatter carries a word
  // rather than a sentinel number because the file is hand-editable.
  const STEPS_UNLIMITED = 'unlimited'

  type AgentFields = {
    description: string; model: string; tools: string[]; deny: string[]; steps: string; body: string
  }

  // Mirrors internal/subagent/profile.go's parse(): a leading `---`-fenced block
  // of `key: value` lines, then the role prompt underneath. Duplicated here
  // rather than asked of the backend because this is purely a display choice —
  // the file format itself has not changed, so there's nothing to add to the
  // Go side for it. Falls back to treating the whole thing as the prompt when
  // there's no recognizable frontmatter, so a hand-edited or malformed file is
  // never silently emptied under the user.
  function parseAgentFile(raw: string): AgentFields {
    const asPromptOnly = { description: '', model: '', tools: [] as string[], deny: [] as string[], steps: '', body: raw.trim() }
    const normalized = raw.replace(/\r\n/g, '\n').replace(/^\n+/, '')
    if (!normalized.startsWith('---\n')) return asPromptOnly
    const rest = normalized.slice(4)
    const end = rest.indexOf('\n---')
    if (end < 0) return asPromptOnly
    const fields: Record<string, string> = {}
    for (const line of rest.slice(0, end).split('\n')) {
      const t = line.trim()
      const i = t.indexOf(':')
      if (i < 0) continue
      const key = t.slice(0, i).trim().toLowerCase()
      if (key) fields[key] = t.slice(i + 1).trim().replace(/^["']+|["']+$/g, '')
    }
    const list = (v?: string) => (v ?? '').split(',').map((s) => s.trim().toLowerCase()).filter(Boolean)
    return {
      description: fields.description ?? '',
      model: fields.model ?? '',
      tools: list(fields.tools),
      deny: list(fields.deny),
      steps: (fields.steps ?? '').trim(),
      body: rest.slice(end + 4).trim(),
    }
  }

  // The inverse of parseAgentFile. What SaveSubagentProfile receives here is
  // exactly what ReadSubagentProfile would hand back for it afterwards — the
  // backend never has to know the editor stopped showing it the raw text.
  function serializeAgentFile(f: AgentFields): string {
    const lines = ['---', `description: ${f.description.trim()}`]
    if (f.model.trim()) lines.push(`model: ${f.model.trim()}`)
    if (f.tools.length) lines.push(`tools: ${f.tools.join(', ')}`)
    if (f.deny.length) lines.push(`deny: ${f.deny.join(', ')}`)
    // The keyword, not a number: internal/subagent/profile.go only unbounds a
    // loop on this exact word, so a typo'd ceiling falls back to the default
    // rather than removing it.
    if (f.steps.trim().toLowerCase() === STEPS_UNLIMITED) {
      lines.push(`steps: ${STEPS_UNLIMITED}`)
    } else {
      const steps = parseInt(f.steps, 10)
      if (Number.isFinite(steps) && steps > 0) lines.push(`steps: ${steps}`)
    }
    lines.push('---', '', f.body.trim())
    return lines.join('\n')
  }

  // ---------- Sub-agent tool permissions ----------
  // The engine resolves one question per tool, not two lists
  // (internal/subagent/profile.go AllowsTool): a forced denial always wins, then
  // deny, then an empty allow-list means everything, then membership.
  //
  // The editor used to draw those two lists as two identical 35-chip grids, so
  // the user had to join them in their head to find out what a tool actually
  // ended up as — and nothing stopped the same tool being ticked in both, a
  // contradiction the engine silently resolves as denied. One row, one state.
  type ToolState = 'default' | 'allow' | 'deny'

  function toolStateOf(name: string): ToolState {
    if (agentDraftDeny.includes(name)) return 'deny'
    if (agentDraftTools.includes(name)) return 'allow'
    return 'default'
  }

  function setToolState(name: string, state: ToolState) {
    agentDraftDeny = agentDraftDeny.filter((n) => n !== name)
    agentDraftTools = agentDraftTools.filter((n) => n !== name)
    if (state === 'deny') agentDraftDeny = [...agentDraftDeny, name]
    if (state === 'allow') agentDraftTools = [...agentDraftTools, name]
  }

  let toolPickerOpen = $state(false)
  let toolQuery = $state('')

  // Back to the default every profile starts on: no allow-list, no deny-list,
  // which the engine reads as "whatever the registry has".
  function resetAgentTools() {
    agentDraftTools = []
    agentDraftDeny = []
  }

  // Grouped by where the tool came from, same split the Tools page uses — with
  // 35+ entries "which of these did I install" is the question that narrows the
  // list fastest.
  const agentToolGroups = $derived.by(() => {
    const q = toolQuery.trim().toLowerCase()
    return TOOL_SOURCES
      .map((key) => ({
        key,
        items: tools
          .filter((s) => s.source === key && (!q || s.name.toLowerCase().includes(q)))
          .map((s) => ({ name: s.name, forced: AGENT_FORCED_DENIALS.includes(s.name) }))
          .sort((a, b) => a.name.localeCompare(b.name)),
      }))
      .filter((g) => g.items.length > 0)
  })

  const toolMatchCount = $derived(agentToolGroups.reduce((n, g) => n + g.items.length, 0))

  // Says what the profile actually does, not how many boxes are ticked. An
  // empty allow-list is the common case and means "everything", which a bare
  // "0 selected" reads as the opposite of.
  const agentToolSummary = $derived(
    agentDraftTools.length === 0 && agentDraftDeny.length === 0
      ? t('settings.agentToolsAllOf')
      : agentDraftTools.length === 0
        ? t('settings.agentToolsAllExcept', { n: agentDraftDeny.length })
        : agentDraftDeny.length === 0
          ? t('settings.agentToolsOnly', { n: agentDraftTools.length })
          : t('settings.agentToolsOnlyExcept', { n: agentDraftTools.length, d: agentDraftDeny.length }),
  )

  // ---------- Step limit ----------
  const agentStepsUnlimited = $derived(agentDraftSteps.trim().toLowerCase() === STEPS_UNLIMITED)

  // Ticking remembers nothing, unticking restores the default rather than an
  // empty box: the field is disabled while unlimited is on, so whatever was in
  // it is not something the user can see or was looking at.
  function toggleStepsUnlimited() {
    agentDraftSteps = agentStepsUnlimited ? '' : STEPS_UNLIMITED
  }

  $effect(() => {
    if (active === 'agents') void loadAgents()
  })

  $effect(() => {
    if (active === 'prompts') void loadPresets()
  })
  $effect(() => {
    if (active === 'identity') loadIdentityFiles()
  })

  // ---------- Learning ----------
  //
  // This page exists because the agent proposing things is only half the
  // design. Without somewhere to see what it wants to remember, why, and what
  // it already remembers, "the agent learns" is indistinguishable from "the
  // agent changes itself" — and the second one is what nobody should have to
  // take on trust.
  let learningOn = $state(true)
  let pendingChanges = $state<main.PendingChange[]>([])
  let decidedChanges = $state<main.PendingChange[]>([])
  let mainMemory = $state('')
  let learningError = $state('')
  let learningBusy = $state(0)

  async function loadLearning() {
    try {
      learningError = ''
      learningOn = await LearningEnabled()
      pendingChanges = await ListPendingChanges()
      decidedChanges = await ListDecidedChanges(20)
      mainMemory = await LearnedMemory('')
    } catch (err) {
      learningError = String(err)
    }
  }

  async function toggleLearning() {
    try {
      await SetLearningEnabled(!learningOn)
      await loadLearning()
    } catch (err) {
      learningError = String(err)
    }
  }

  async function decideChange(id: number, approve: boolean) {
    learningBusy = id
    try {
      learningError = ''
      if (approve) await ApprovePendingChange(id)
      else await RejectPendingChange(id)
      await loadLearning()
    } catch (err) {
      // Shown rather than swallowed: an approval that could not be applied
      // leaves the proposal in the list, and a button that appears to do
      // nothing is how a user concludes the feature is broken.
      learningError = String(err)
    } finally {
      learningBusy = 0
    }
  }

  // Whose memory a proposal is for. Empty scope is the assistant itself; a
  // named one is a sub-agent, and saying which matters — it is the difference
  // between "everything you ask it" and "one job it does".
  function scopeLabel(scope: string): string {
    return scope ? scope : t('settings.learningScopeMain')
  }

  $effect(() => {
    if (active === 'learning') void loadLearning()
  })

  // ---------- Nav ----------
  // `terms` is what the page is actually about, not just what it is called.
  // Search used to match the nav label alone, so "font" and "ธีม" — two of the
  // most likely things anyone types into a settings search — found nothing,
  // even though the Appearance page has five font controls on it. The terms are
  // the page's own setting titles, so they translate with everything else.
  type NavItem = { id: string; label: string; icon: IconName; terms: string[] }
  const sections: { group: string; items: NavItem[] }[] = $derived([
    { group: t('settings.groupPersonal'), items: [
      { id: 'general', label: t('settings.general'), icon: 'slidersHorizontal',
        terms: [t('settings.shellTitle'), t('settings.approvalTitle')] },
      { id: 'appearance', label: t('settings.appearance'), icon: 'palette',
        terms: [
          t('settings.languageTitle'), t('settings.themeTitle'), t('settings.uiFontTitle'),
          t('settings.typeScaleTitle'), t('settings.systemZoomTitle'), t('settings.editorFontTitle'),
          t('settings.chatFontTitle'), t('settings.treeFontTitle'), t('settings.codeThemeTitle'),
        ] },
      { id: 'identity', label: t('sidebar.identity'), icon: 'userRound', terms: [] },
      // Next to identity, because they answer the same question from two
      // sides: what the user told the agent, and what the agent worked out.
      { id: 'learning', label: t('settings.learning'), icon: 'brain',
        terms: [t('settings.learningPending'), t('settings.learningMemory')] },
    ]},
    { group: t('settings.groupModels'), items: [
      { id: 'models', label: t('settings.modelSettings'), icon: 'brain',
        terms: [t('settings.providers'), t('settings.apiKeyLabel'), t('settings.baseUrl'), t('settings.signInLabel'), t('settings.modelList')] },
      { id: 'agents', label: t('settings.subagents'), icon: 'bot',
        terms: [t('settings.subagentsMine'), t('settings.subagentsBuiltin')] },
    ]},
    { group: t('settings.groupTools'), items: [
      // Two pages, not two cards on one: a tool is something the AI runs, a
      // skill is a document telling it how. Sharing a page is what made them
      // read as one thing.
      { id: 'tools', label: t('settings.tools'), icon: 'wrench', terms: [SPEECH_TOOL] },
      { id: 'skills', label: t('settings.skills'), icon: 'puzzle', terms: [t('settings.skillInstall')] },
      { id: 'mcp', label: t('settings.mcpServers'), icon: 'plug', terms: [t('settings.mcpPresets'), t('settings.addServer')] },
      { id: 'prompts', label: t('settings.prompts'), icon: 'sparkles', terms: [t('settings.promptNew')] },
      { id: 'usage', label: t('settings.usage'), icon: 'chartColumn',
        terms: [t('settings.usageByModel'), t('settings.usageTotalTokens'), t('settings.usageCacheHitRate')] },
    ]},
    { group: t('settings.groupAbout'), items: [
      { id: 'about', label: t('settings.about'), icon: 'package',
        terms: [t('settings.aboutVersion'), t('settings.aboutCheck')] },
      { id: 'sponsor', label: t('settings.sponsor'), icon: 'heart', terms: ['PromptPay', 'GitHub'] },
    ]},
  ])

  const SPONSOR_URL = 'https://github.com/Mike0165115321/Aetox/blob/main/SPONSOR.md'
  const SITE_URL = 'https://aetox-puce.vercel.app/'
  // The one link that is right on every channel, whether or not a check ran.
  const RELEASES_URL = 'https://github.com/Mike0165115321/Aetox/releases'

  // Which page is open survives an F5. Same reasoning as the chat/settings view
  // itself (see setActiveView in stores/cockpit.svelte.ts): sessionStorage, not
  // localStorage, because reopening the app should always start from the top —
  // but a reload during a run should not throw away where you were. Reloading
  // while three pages deep into MCP config and landing back on General is a
  // small thing that happens every single time.
  const SECTION_KEY = 'aetox.settingsSection'
  const SECTION_IDS = new Set(['general', 'appearance', 'identity', 'learning', 'models', 'agents', 'tools', 'skills', 'mcp', 'prompts', 'usage', 'about', 'sponsor'])

  function restoredSection(): string {
    try {
      const saved = sessionStorage.getItem(SECTION_KEY)
      // Validated, not trusted: a page that was removed since the value was
      // written would otherwise render nothing at all.
      if (saved && SECTION_IDS.has(saved)) return saved
    } catch {
      /* storage unavailable — start where a fresh open would */
    }
    return 'general'
  }

  let active = $state(restoredSection())
  let query = $state('')

  function openSection(id: string) {
    active = id
    try {
      sessionStorage.setItem(SECTION_KEY, id)
    } catch {
      /* storage unavailable — the page just won't be remembered */
    }
  }

  const filteredSections = $derived.by(() => {
    const q = query.trim().toLowerCase()
    if (!q) return sections
    return sections
      .map((g) => ({
        ...g,
        items: g.items.filter((it) =>
          it.label.toLowerCase().includes(q) || it.terms.some((term) => term.toLowerCase().includes(q)),
        ),
      }))
      .filter((g) => g.items.length > 0)
  })

  const noSearchResults = $derived(query.trim() !== '' && filteredSections.length === 0)
</script>

<div class="settings-page">
  <aside class="settings-nav">
    <button class="settings-back" onclick={onClose}><Icon name="arrowLeft" size={14} /> {t('settings.backToApp')}</button>
    <input class="settings-search" placeholder={t('settings.searchPlaceholder')} bind:value={query} />
    {#each filteredSections as g}
      <div class="settings-group-label eyebrow">{g.group}</div>
      {#each g.items as it}
        <button class="settings-nav-item" class:active={active === it.id} onclick={() => openSection(it.id)}>
          <span class="ic"><Icon name={it.icon} /></span> {it.label}
          {#if it.id === 'learning' && cockpit.pendingLearned > 0}
            <span class="nav-count" title={t('settings.learningWaiting', { count: String(cockpit.pendingLearned) })}>
              {cockpit.pendingLearned}
            </span>
          {/if}
        </button>
      {/each}
    {/each}
    {#if noSearchResults}
      <div class="settings-nav-empty">{t('settings.searchNoResults', { q: query.trim() })}</div>
    {/if}
  </aside>

  <div class="settings-content">
    <div class="settings-inner" style:--content-max={active === 'usage' ? '960px' : null}>
    {#if bootError}
      <!-- The whole page used to be one unguarded await chain, so a backend
           that wasn't up yet produced a blank page and no explanation. -->
      <div class="settings-banner">
        <div class="set-txt">
          <div class="t">{t('settings.bootErrorTitle')}</div>
          <div class="d">{t('settings.bootErrorHint')}</div>
          <div class="d mono-dim">{bootError}</div>
        </div>
        <button class="ctrl ctrl-primary" disabled={booting} onclick={bootSettings}>
          {booting ? t('settings.loading') : t('settings.retry')}
        </button>
      </div>
    {/if}
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
      <!-- Every zone carries a heading, including the first. One unlabelled card
           above three labelled ones reads as an oversight, not as an intro. -->
      <div class="group-head">
        <span class="group-title">{t('settings.zoneLook')}</span>
      </div>
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
      </div>

      <div class="group-head">
        <span class="group-title">{t('settings.zoneTextSize')}</span>
      </div>
      <div class="settings-card">
        <!-- Text size sits above overall size on purpose: it is the one people
             actually come here for, and reading the two in this order is what
             makes the difference between them land. -->
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.typeScaleTitle')}</div>
            <div class="d">{t('settings.typeScaleDesc')}</div>
          </div>
          <div class="seg-ctrl">
            {#each TYPE_SCALES as s (s.value)}
              <button
                type="button" class="seg-btn" class:selected={typeScale.name === s.value}
                onclick={() => applyTypeScale(s.value as TypeScaleName)}
              >{t(s.labelKey)}</button>
            {/each}
          </div>
        </div>
        <!-- Three steps of the scale at once. A single sample line cannot show
             what a scale does — the thing being chosen is the gap between the
             heading and the caption, not any one size. -->
        <div class="set-row type-preview">
          <div class="tsp-heading">{t('settings.typeScalePreviewHeading')}</div>
          <div class="tsp-body">{t('settings.typeScalePreviewBody')}</div>
          <div class="tsp-caption">{t('settings.typeScalePreviewCaption')}</div>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.systemZoomTitle')}</div>
            <div class="d">{t('settings.systemZoomDesc')}</div>
          </div>
          <input
            class="ctrl set-num" type="number" min="12" max="20" step="0.5"
            value={Math.round(systemZoom.value * SYSTEM_BASE_PX * typeScale.scale * 10) / 10}
            onchange={(e) => applySystemZoom(parseFloat(e.currentTarget.value) / (SYSTEM_BASE_PX * typeScale.scale))}
          />
          <span class="muted set-unit">px</span>
        </div>
      </div>

      <div class="group-head">
        <span class="group-title">{t('settings.zonePaneSizes')}</span>
        <span class="group-count">{t('settings.zonePaneSizesHint')}</span>
      </div>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.editorFontTitle')}</div>
            <div class="d">{t('settings.editorFontDesc')}</div>
          </div>
          <input
            class="ctrl set-num" type="number" min="10" max="24" step="0.5"
            value={editorFont.size}
            onchange={(e) => applyEditorFontSize(parseFloat(e.currentTarget.value))}
          />
          <span class="muted set-unit">px</span>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.chatFontTitle')}</div>
            <div class="d">{t('settings.chatFontDesc')}</div>
          </div>
          <input
            class="ctrl set-num" type="number" min="12" max="22" step="0.5"
            value={chatFont.size}
            onchange={(e) => applyChatFontSize(parseFloat(e.currentTarget.value))}
          />
          <span class="muted set-unit">px</span>
        </div>
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.treeFontTitle')}</div>
            <div class="d">{t('settings.treeFontDesc')}</div>
          </div>
          <input
            class="ctrl set-num" type="number" min="11" max="18" step="0.5"
            value={treeFont.size}
            onchange={(e) => applyTreeFontSize(parseFloat(e.currentTarget.value))}
          />
          <span class="muted set-unit">px</span>
        </div>
      </div>

      <div class="group-head">
        <span class="group-title">{t('settings.zoneCode')}</span>
      </div>
      <div class="settings-card">
        <div class="set-row">
          <span class="muted set-unit">px</span>
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
          <span class="muted set-unit">px</span>
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
                <ProviderMark name={p.name} size={15} />
                <span class="mset-prov-name">{p.name}</span>
                <span class="dot" class:green={p.hasKey}></span>
              </button>
              {#if enabledRows.length > 1}
                <button class="icobtn tiny" disabled={busy === 'disable:' + p.name}
                  aria-label={t('settings.remove')} onclick={() => removeProvider(p.name)}><Icon name="x" size={13} /></button>
              {/if}
            </div>
          {/each}

          <button class="mset-prov mset-add-toggle" onclick={() => (showAddProvider = !showAddProvider)}>
            <Icon name="plus" size={14} /> {t('settings.addProvider')}
          </button>
          {#if showAddProvider}
            <div class="mset-add-list">
              {#if addableSignIn.length > 0}
                <div class="mset-add-group">{t('settings.groupSignIn')}</div>
                {#each addableSignIn as p (p.name)}
                  <button class="mset-prov" disabled={busy === 'enable:' + p.name} onclick={() => addProvider(p.name)}>
                    <ProviderMark name={p.name} size={15} />
                    <span class="mset-prov-name">{p.name}</span>
                    <span class="dot">{busy === 'enable:' + p.name ? '…' : '+'}</span>
                  </button>
                {/each}
              {/if}
              {#if addableKeyed.length > 0}
                <div class="mset-add-group">{t('settings.groupApiKey')}</div>
                {#each addableKeyed as p (p.name)}
                  <button class="mset-prov" disabled={busy === 'enable:' + p.name} onclick={() => addProvider(p.name)}>
                    <ProviderMark name={p.name} size={15} />
                    <span class="mset-prov-name">{p.name}</span>
                    <span class="dot">{busy === 'enable:' + p.name ? '…' : '+'}</span>
                  </button>
                {/each}
              {/if}
              {#if addableRows.length === 0}
                <div class="muted set-note">{t('settings.noMoreProviders')}</div>
              {/if}
            </div>
          {/if}
        </aside>

        <div class="mset-detail">
          {#if selectedRow}
            <div class="mset-head">
              <ProviderMark name={selected} size={22} />
              <span class="mset-name">{selected}</span>
              {#if isActiveProvider}
                <span class="badge on">{t('settings.active')}</span>
              {:else}
                <button class="ctrl ctrl-primary" disabled={busy !== ''} onclick={useProvider}>
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
              <div class="muted set-hint">{t('settings.baseUrlDesc')}</div>
              <div class="mset-keyrow">
                <input
                  class="ctrl key-input" placeholder={baseURL || 'http://localhost:1234/v1'}
                  bind:value={baseURLDraft}
                  onkeydown={(e) => e.key === 'Enter' && saveBaseURL(baseURLDraft)}
                />
                <button class="ctrl ctrl-primary" disabled={busy !== '' || baseURLDraft.trim() === baseURL} onclick={() => saveBaseURL(baseURLDraft)}>
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
                <div class="muted set-hint">{t('settings.wireFormatDesc')}</div>
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

            {#if signInMethod}
              <div class="mset-field">
                <div class="eyebrow">{t('settings.signInLabel')}</div>

                {#if signInStatus?.signed_in}
                  <div class="mset-keyrow">
                    <span class="badge on">{signInStatus.label || t('settings.signedInAs')}</span>
                    <button class="ctrl" disabled={busy !== ''} onclick={() => doSignOut(signInMethod.provider)}>
                      {busy === 'signout:' + signInMethod.provider ? '…' : t('settings.signOut')}
                    </button>
                  </div>
                {:else if signInPrompt}
                  {@const prompt = signInPrompt}
                  <div class="signin-flow">
                    {#if prompt.kind === 'device'}
                      <div class="muted">{t('settings.signInDeviceStep')}</div>
                      <div class="signin-code">{prompt.user_code}</div>
                      <div class="mset-keyrow">
                        <button class="ctrl" onclick={() => BrowserOpenURL(prompt.verification_uri || prompt.url)}>
                          {t('settings.signInOpenPage')}
                        </button>
                        <button class="ctrl" onclick={abortSignIn}>{t('settings.signInCancel')}</button>
                      </div>
                      <div class="muted">{t('settings.signInWaiting')}</div>
                    {:else if prompt.kind === 'paste'}
                      <div class="muted">{t('settings.signInPasteStep')}</div>
                      <div class="mset-keyrow">
                        <input
                          class="ctrl key-input" type="password"
                          placeholder={t('settings.signInPastePlaceholder')}
                          bind:value={signInCode}
                          onkeydown={(e) => e.key === 'Enter' && finishSignIn()}
                        />
                        <button class="ctrl" disabled={busy === 'signin' || !signInCode.trim()} onclick={finishSignIn}>
                          {busy === 'signin' ? '…' : t('settings.signInSubmit')}
                        </button>
                        <button class="ctrl" onclick={abortSignIn}>{t('settings.signInCancel')}</button>
                      </div>
                    {:else}
                      <div class="muted">{t('settings.signInWaiting')}</div>
                      <div class="mset-keyrow">
                        <button class="ctrl" onclick={() => BrowserOpenURL(prompt.url)}>
                          {t('settings.signInOpenPage')}
                        </button>
                        <button class="ctrl" onclick={abortSignIn}>{t('settings.signInCancel')}</button>
                      </div>
                    {/if}
                  </div>
                {:else}
                  <div class="mset-keyrow">
                    <button class="ctrl ctrl-primary" disabled={busy !== ''} onclick={startSignIn}>
                      {t('settings.signInWith', { label: signInMethod.label })}
                    </button>
                    {#if importable.includes(signInMethod.provider)}
                      <button class="ctrl" disabled={busy !== ''} onclick={() => doImport(signInMethod.provider)}>
                        {busy === 'import:' + signInMethod.provider ? '…' : t('settings.signInImport')}
                      </button>
                    {/if}
                  </div>
                  <div class="muted">{signInMethod.note}</div>
                  {#if signInMethod.risk === 'restricted'}
                    <div class="signin-warn">{t('settings.signInRestricted')}</div>
                  {/if}
                {/if}

                {#if signInError}
                  <div class="conn-test">{signInError}</div>
                {/if}
              </div>
            {/if}

            {#if selectedRow.requiresKey}
              <div class="mset-field">
                <div class="eyebrow">
                  {signInMethod ? t('settings.signInOrKey') : t('settings.apiKeyLabel')}
                </div>
                <div class="mset-keyrow">
                  <input
                    class="ctrl key-input" type={showKey ? 'text' : 'password'}
                    placeholder={selectedRow.hasKey ? t('settings.keySetPlaceholder') : t('settings.pasteKeyPlaceholder')}
                    bind:value={keyDraft}
                    onkeydown={(e) => e.key === 'Enter' && saveKey()}
                  />
                  <button class="icobtn tiny" aria-label={t('settings.showKey')} onclick={() => (showKey = !showKey)}><Icon name="eye" size={14} /></button>
                  <button class="ctrl ctrl-primary" disabled={busy === 'key' || !keyDraft.trim()} onclick={saveKey}>
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
                    >{#if busy === 'test:' + m}…{:else}<Icon name="plugZap" size={14} />{/if}</button>
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
                      {#if connTest.startsWith('ok:')}
                        <Icon name="check" size={13} /> {t('settings.connOk')}: {connTest.slice(3)}
                      {:else}
                        <Icon name="x" size={13} /> {connTest.slice(4)}
                      {/if}
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
          {:else if booting}
            <!-- refreshProviders fans out one IPC round-trip per provider, so
                 this pane sat completely blank on every open. A skeleton says
                 "coming"; nothing says "broken". -->
            <div class="mset-skeleton" aria-label={t('settings.loading')}>
              <span class="sk sk-head"></span>
              <span class="sk sk-line"></span>
              <span class="sk sk-line short"></span>
              <span class="sk sk-block"></span>
            </div>
          {:else}
            <div class="mset-empty muted">{t('settings.noProviderSelected')}</div>
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
          <!-- Template literal, not concatenation: TOOL_SOURCES is a literal
               union, so this resolves to a real message key and a source added
               without its label becomes a compile error. -->
          <span class="group-title">{t(`settings.toolSource_${g.key}`)}</span>
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
                    {activeSpeechLabel} <Icon name={speechOpen ? 'chevronUp' : 'chevronDown'} size={13} />
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
                            <!-- data-tip, not title: the app has its own tooltip
                                 and the native one is slow and unstyleable. The
                                 path is what the tip is for — `m.where` was
                                 never a field on this row, so it showed
                                 nothing. -->
                            <button
                              class="rowdrop-reveal"
                              data-tip={m.path}
                              aria-label={t('settings.speechOpenFolder')}
                              onclick={() => RevealSpeechModel(m.path)}
                            ><Icon name="folderOpen" size={14} /></button>
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
                          <Icon name="folderOpen" size={13} /> {d.label}
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
            <div class="eyebrow eyebrow-grow">{t('settings.skillsInstalled')}</div>
            <button class="ctrl" disabled={skillBusy !== ''} onclick={() => OpenSkillsFolder()}>
              {t('settings.skillsFolder')}
            </button>
            <button class="ctrl" disabled={skillBusy !== ''} onclick={refreshSkills}>
              {skillBusy === 'refresh' ? t('settings.refreshing') : t('settings.refresh')}
            </button>
          </div>
          <!-- The real path, read from the engine. Two of the three places this
               page used to name one had it wrong (~/.agents/skills, which is
               opencode's and which Aetox never scans), so anyone who followed
               the instructions put files where nothing was looking. -->
          <div class="d mono-dim">{skillsDir}</div>
        </div>
        {#if skillIssues.length > 0}
          <!-- Files that are in the right folder and still did not appear. The
               scan has always collected these and the list has always dropped
               them, so a broken SKILL.md looked exactly like a folder the app
               was not reading. -->
          <div class="set-row skill-issues">
            <div class="set-txt">
              <div class="t">{t('settings.skillIssues', { n: skillIssues.length })}</div>
              {#each skillIssues as issue (issue)}
                <div class="d mono-dim" title={issue}>{issue}</div>
              {/each}
            </div>
          </div>
        {/if}
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
              <button class="ctrl ctrl-danger" disabled={skillBusy !== ''} onclick={() => removeSkill(s.name, s.dir)}>
                {t('settings.remove')}
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
            <button class="ctrl ctrl-primary" disabled={skillBusy !== '' || !skillInstallUrl.trim()} onclick={installSkill}>
              {skillBusy === 'install' ? t('settings.installing') : t('settings.install')}
            </button>
          </div>
          <div class="d muted">{t('settings.skillInstallHint')}</div>

          <!-- The third way in. A GitHub URL needs the skill to be published
               there; the folder button needs it to already be on this machine.
               A zip is what a skill looks like arriving by any other road. -->
          <div class="mset-keyrow skill-zip">
            <div class="d muted eyebrow-grow">{t('settings.skillZipHint')}</div>
            <button class="ctrl" disabled={skillBusy !== ''} onclick={installSkillZip}>
              {skillBusy === 'zip' ? t('settings.installing') : t('settings.skillZip')}
            </button>
          </div>
          {#if skillInstallResult}<pre class="skill-result">{skillInstallResult}</pre>{/if}
          {#if skillError}<div class="mset-error">{skillError}</div>{/if}
        </div>
      </div>

    {:else if active === 'agents'}
      <h2>{t('settings.subagents')}</h2>
      <p class="muted set-sub">{t('settings.subagentsDesc')}</p>

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
                    <span class="tag" title={toolBadgeTip(a)}>{toolBadge(a)}</span>
                    {#if a.deny && a.deny.length > 0}<span class="tag ag-deny" title={denyTip(a)}>{t('settings.agentDenyCount', { n: a.deny.length })}</span>{/if}
                    <span class="tag" title={t('settings.agentStepsTip', { n: a.steps || 24 })}>{t('settings.agentSteps', { n: a.steps || 24 })}</span>
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
                <button class="ctrl" disabled={agentBusy !== ''} onclick={() => openAgent(a)}>{t('settings.agentConfigure')}</button>
              </div>
            {/each}
          </div>
        {/each}
        <p class="muted set-sub">{t('settings.agentsHint')}</p>
      {:else}
        <div class="pp-bar">
          <button class="ctrl" onclick={closeAgentEditor}><Icon name="arrowLeft" size={14} /> {t('settings.agentBack')}</button>
          <div class="pp-bar-gap"></div>
          {#if !agentEditing.builtin && agentEditing.name}
            <button class="ctrl ctrl-danger" disabled={agentBusy !== ''} onclick={deleteAgent}>
              {agentEditing.overrides ? t('settings.agentRevert') : t('settings.remove')}
            </button>
          {/if}
          <button class="ctrl ctrl-primary" disabled={agentBusy !== '' || !agentDraftName.trim() || !agentDraftPrompt.trim()} onclick={saveAgent}>
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
              <span class="eyebrow">{t('settings.agentDescription')}</span>
              <input class="ctrl" bind:value={agentDraftDescription} placeholder={t('settings.agentDescriptionPlaceholder')} />
            </label>
            <label class="pp-field">
              <span class="eyebrow">{t('settings.agentBody')}</span>
              <textarea class="ctrl ag-body" bind:value={agentDraftPrompt} spellcheck="false" use:autogrow={agentDraftPrompt}></textarea>
              <span class="d muted">{t('settings.agentBodyHint')}</span>
            </label>
            <!-- A summary and a way in, not seventy chips. What each tool ends
                 up as is one question, so it is answered one row at a time in
                 the panel below rather than across two grids the reader has to
                 join themselves. -->
            <div class="pp-field">
              <span class="eyebrow">{t('settings.agentTools')}</span>
              <div class="ag-toolsum">
                <div class="ag-toolsum-txt">
                  <div class="t">{agentToolSummary}</div>
                  <div class="d muted">{t('settings.agentToolsRule')}</div>
                </div>
                <button type="button" class="ctrl" onclick={() => (toolPickerOpen = true)}>
                  {t('settings.agentToolsConfigure')} <Icon name="chevronRight" size={13} />
                </button>
              </div>
            </div>

            <div class="pp-field">
              <span class="eyebrow">{t('settings.agentStepsField')}</span>
              <div class="ag-steprow">
                <input
                  class="ctrl ag-steps" bind:value={agentDraftSteps} inputmode="numeric" placeholder="24"
                  disabled={agentStepsUnlimited}
                  aria-label={t('settings.agentStepsField')}
                />
                <label class="ag-check">
                  <input type="checkbox" checked={agentStepsUnlimited} onchange={toggleStepsUnlimited} />
                  {t('settings.agentStepsUnlimited')}
                </label>
              </div>
              <span class="d muted">
                {agentStepsUnlimited ? t('settings.agentStepsUnlimitedWarn') : t('settings.agentStepsFieldHint')}
              </span>
            </div>
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
          <button class="ctrl" onclick={closePresetEditor}><Icon name="arrowLeft" size={14} /> {t('settings.promptBack')}</button>
          <div class="pp-bar-gap"></div>
          {#if !editing.builtin && editing.name}
            <button class="ctrl ctrl-danger" disabled={presetBusy !== ''} onclick={deletePreset}>
              {t('settings.remove')}
            </button>
          {/if}
          <button class="ctrl ctrl-primary" disabled={presetBusy !== '' || !draftName.trim() || !draftBody.trim()} onclick={savePreset}>
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
                <span class="eyebrow eyebrow-grow">{t('settings.promptBody')}</span>
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
                  <span class="ic"><Icon name="fileText" size={14} /></span>
                  <span class="t">{f.name}</span>
                </button>
                <button type="button" class="identity-file-del" aria-label={t('settings.remove')} onclick={() => removeIdentityFile(f.name)}><Icon name="x" size={13} /></button>
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
                  <Icon name="plus" size={13} /> {tpl.name}
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
            <button type="button" class="icobtn tiny" aria-label={t('sidebar.newIdentityFile')} onclick={addIdentityFile}><Icon name="plus" size={14} /></button>
          </div>
          {#if identity.activeName}
            <textarea
              class="identity-input" placeholder={t('sidebar.identityPlaceholder')}
              bind:value={identity.draft}
            ></textarea>
            <button
              type="button" class="ctrl identity-save ctrl-primary"
              disabled={!identityDirty || identity.saving}
              onclick={saveIdentityFile}
            >
              {identity.saving ? t('settings.saving') : t('settings.save')}
            </button>
          {/if}
        </div>
      </div>
    {:else if active === 'learning'}
      <h2>{t('settings.learning')}</h2>
      <p class="muted set-sub">{t('settings.learningDesc')}</p>

      {#if learningError}<div class="mset-error">{learningError}</div>{/if}

      <div class="settings-card">
        <div class="mcp-row">
          <div class="mcp-row-main">
            <div class="n">{t('settings.learningEnabled')}</div>
            <div class="d">{t('settings.learningEnabledHint')}</div>
          </div>
          <div class="mcp-row-actions">
            <label class="mswitch">
              <input type="checkbox" checked={learningOn} onchange={toggleLearning} />
              <span></span>
            </label>
          </div>
        </div>
      </div>

      <h3 class="set-h3">{t('settings.learningPending')}</h3>
      <p class="muted set-sub">{t('settings.learningPendingHint')}</p>
      <div class="settings-card">
        {#each pendingChanges as c (c.id)}
          <div class="learn-row">
            <div class="learn-main">
              <div class="learn-head">
                <span class="learn-scope">{scopeLabel(c.scope)}</span>
                <span class="learn-op">{c.op}</span>
              </div>
              {#if c.before}
                <!-- What it replaces, shown next to what it becomes: approving a
                     change without seeing what it overwrites is not a decision. -->
                <div class="learn-before">{c.before}</div>
              {/if}
              <div class="learn-body">{c.body}</div>
              {#if c.reason}<div class="learn-why">{c.reason}</div>{/if}
            </div>
            <div class="learn-actions">
              <button type="button" class="ctrl ctrl-primary" disabled={learningBusy === c.id}
                onclick={() => decideChange(c.id, true)}>{t('settings.learningApprove')}</button>
              <button type="button" class="ctrl" disabled={learningBusy === c.id}
                onclick={() => decideChange(c.id, false)}>{t('settings.learningReject')}</button>
            </div>
          </div>
        {/each}
        {#if pendingChanges.length === 0}
          <div class="empty">{t('settings.learningNothingPending')}</div>
        {/if}
      </div>

      <h3 class="set-h3">{t('settings.learningMemory')}</h3>
      <p class="muted set-sub">{t('settings.learningMemoryHint')}</p>
      <div class="settings-card">
        {#if mainMemory}
          <pre class="learn-memory">{mainMemory}</pre>
        {:else}
          <div class="empty">{t('settings.learningMemoryEmpty')}</div>
        {/if}
        <button type="button" class="ctrl" onclick={() => OpenMemoryFolder()}>
          <Icon name="folderOpen" size={13} /> {t('settings.learningOpenFolder')}
        </button>
      </div>

      {#if decidedChanges.length > 0}
        <h3 class="set-h3">{t('settings.learningHistory')}</h3>
        <p class="muted set-sub">{t('settings.learningHistoryHint')}</p>
        <div class="settings-card">
          {#each decidedChanges as c (c.id)}
            <div class="learn-row past">
              <div class="learn-main">
                <div class="learn-head">
                  <span class="learn-scope">{scopeLabel(c.scope)}</span>
                  <span class="learn-op" class:rejected={c.state === 'rejected'}>{c.state}</span>
                  <span class="learn-when">{c.decidedAt.slice(0, 10)}</span>
                </div>
                <div class="learn-body">{c.body}</div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
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
        <div class="card-form">
          <div class="mset-keyrow">
            <div class="eyebrow eyebrow-grow">{t('settings.mcpConfigured')}</div>
            <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => OpenMCPFolder()}>
              {t('settings.skillsFolder')}
            </button>
          </div>
          <!-- The file the servers live in. A server that will not connect is
               inspectable and backup-able only if this is findable. -->
          <div class="d mono-dim">{mcpPath}</div>
          {#if mcpServers.length > 3}
            <input class="ctrl" placeholder={t('settings.mcpSearchPlaceholder')} bind:value={mcpQuery} />
          {/if}
        </div>
        {#if mcpServers.length === 0}
          <div class="set-row"><div class="muted">{t('settings.noMcpServers')}</div></div>
        {:else}
          {#each mcpFiltered as s (s.name)}
            <div class="set-row" class:mcp-off={s.disabled}>
              <div class="set-txt">
                <div class="t">
                  <span class="dot" style={statusVar(s.status)}></span> {s.name}
                  <span class="mcp-badge">{s.url ? 'http' : 'stdio'}</span>
                  {#if s.tools > 0}<span class="mcp-badge">{t('settings.mcpToolCount', { n: String(s.tools) })}</span>{/if}
                </div>
                <div class="d">{s.url || (s.command ?? []).join(' ')}{s.err ? ' · ' + s.err : ''}</div>
              </div>
              <div class="mcp-row-actions">
                <label class="mswitch" title={s.disabled ? t('settings.add') : ''}>
                  <input type="checkbox" checked={!s.disabled} disabled={mcpBusy !== ''} onchange={() => toggleMCP(s)} />
                  <span></span>
                </label>
                <button class="ctrl" disabled={mcpBusy !== '' || s.disabled} onclick={() => testMCP(s.name)}>
                  {mcpBusy === 'test:' + s.name ? t('settings.testing') : t('settings.test')}
                </button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => editMCP(s)}>{t('settings.edit')}</button>
                <button class="ctrl ctrl-danger" disabled={mcpBusy !== ''} onclick={() => removeMCP(s.name)}>{t('settings.remove')}</button>
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

          <!-- Both fields the stored config always had and the form never
               offered. Folded away because the common server needs neither. -->
          <details class="mcp-more">
            <summary>{t('settings.mcpAdvanced')}</summary>
            <div class="mcp-more-body">
              <label class="pp-field">
                <span class="eyebrow">{t('settings.mcpCwd')}</span>
                <input class="ctrl" placeholder={t('settings.mcpCwdPlaceholder')} bind:value={mcpCwd} />
              </label>
              <label class="pp-field">
                <span class="eyebrow">{t('settings.mcpTimeout')}</span>
                <div class="mset-keyrow">
                  <input class="ctrl set-num" inputmode="numeric" placeholder="0" bind:value={mcpTimeout} />
                  <span class="muted set-unit">ms</span>
                </div>
                <span class="d muted">{t('settings.mcpTimeoutHint')}</span>
              </label>
            </div>
          </details>

          <div class="mset-keyrow">
            <button class="ctrl ctrl-primary" disabled={mcpBusy !== '' || !mcpFormValid} onclick={saveMCP}>
              {mcpBusy === 'save' ? t('settings.saving') : (mcpOriginal ? t('settings.save') : t('settings.add'))}
            </button>
            {#if mcpOriginal || mcpNeedsKey}
              <button class="ctrl" disabled={mcpBusy !== ''} onclick={resetMCPForm}>{t('settings.cancel')}</button>
            {/if}
          </div>
          {#if mcpNeedsKey}
            <!-- Says why the form filled itself in. Without it the preset's
                 button appears to have done nothing. -->
            <div class="d muted">{t('settings.mcpNeedsKey', { name: mcpName })}</div>
          {/if}
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
              <div class="d">{p.desc} · {p.url ?? p.command?.join(' ')}</div>
            </div>
            <button class="ctrl" disabled={mcpBusy !== '' || presetTaken(p.name)} onclick={() => addPreset(p)}>
              {mcpBusy === 'preset:' + p.name ? t('settings.adding') : t('settings.add')}
            </button>
          </div>
        {/each}
      </div>
    {:else if active === 'about'}
      <h2>{t('settings.about')}</h2>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.aboutVersion')}</div>
            <div class="d">
              {appVersion ? 'v' + appVersion : '—'}
              {#if updateStatus}
                · {t(CHANNEL_LABELS[updateStatus.channel] ?? 'settings.aboutChannelUnknown')}
              {/if}
              {#if updateStatus?.checkedAt}
                · {t('settings.aboutLastChecked', { when: new Date(updateStatus.checkedAt).toLocaleString() })}
              {/if}
            </div>
          </div>
          <button class="ctrl" disabled={updateChecking} onclick={checkForUpdate}>
            {updateChecking ? t('settings.aboutChecking') : t('settings.aboutCheck')}
          </button>
        </div>

        <!-- Four outcomes, four different sentences. "Switched off" is not a
             failure and must not read like one, and a failed check must never
             leave the impression that something in the app broke. -->
        {#if updateError}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{t('settings.aboutCheckFailed')}</div>
              <div class="d">{t('settings.aboutCheckFailedHint')}</div>
            </div>
          </div>
        {:else if updateStatus?.disabled}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{t('settings.aboutCheckOff')}</div>
              <div class="d">{t('settings.aboutCheckOffHint', { env: 'AETOX_DISABLE_UPDATE_CHECK' })}</div>
            </div>
          </div>
        {:else if updateStatus?.available}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{t('settings.aboutNewVersion', { version: updateStatus.latest })}</div>
              <!-- Scoop installed us, so Scoop upgrades us: Aetox never writes
                   into someone else's package directory. Every other channel
                   gets the release page, which is always a correct answer. -->
              <div class="d">
                {updateStatus.hint ? t('settings.aboutRunCommand') : t('settings.aboutDownloadHint')}
              </div>
              {#if updateStatus.hint}
                <code class="about-cmd">{updateStatus.hint}</code>
              {/if}
            </div>
            <!-- One action, not two. Offering a Scoop user the download page
                 next to `scoop update aetox` invites them to unpack a zip on
                 top of a Scoop-managed install, which is exactly the mess this
                 whole design exists to avoid. What changed is still one row
                 down, for everybody. -->
            {#if updateStatus.hint}
              <button class="ctrl" onclick={() => copyUpgradeHint(updateStatus!.hint)}>
                {hintCopied ? t('settings.aboutCopied') : t('settings.aboutCopy')}
              </button>
            {:else}
              <button class="ctrl" onclick={() => BrowserOpenURL(updateStatus!.url)}>
                {t('settings.aboutOpenRelease')}
              </button>
            {/if}
          </div>
        {:else if updateStatus}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{t('settings.aboutUpToDate')}</div>
            </div>
          </div>
        {/if}

        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.aboutReleaseNotes')}</div>
            <div class="d">{RELEASES_URL}</div>
          </div>
          <button class="ctrl" onclick={() => BrowserOpenURL(RELEASES_URL)}>{t('settings.aboutOpenRelease')}</button>
        </div>
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
        <div class="set-row sponsor-center">
          <img src={promptPayQR} alt="PromptPay QR" class="sponsor-qr" />
        </div>
        <!-- Attribution, not decoration: this is the one place in the running app
             that names who wrote it and where it came from. Untranslated on
             purpose — a name, a licence id and a URL read the same in every
             language, and a translated copyright line is a mistranslated one. -->
        <div class="set-row">
          <div class="set-txt">
            <div class="t">Aetox</div>
            <div class="d">© 2026 Chayaphon Phromsawana (Mike) · Apache-2.0 · github.com/Mike0165115321/Aetox</div>
          </div>
        </div>
      </div>
    {/if}
    </div>
  </div>
</div>

{#if toolPickerOpen}
  <!-- Edits land straight on the draft — there is no OK/Cancel here, because
       the editor behind it already has a Save and a discard guard, and a second
       layer of "are you sure" over the same draft is one too many. -->
  <div class="tp-overlay" role="dialog" aria-modal="true" aria-labelledby="tp-title">
    <button class="confirm-backdrop" aria-label={t('settings.close')} onclick={() => (toolPickerOpen = false)}></button>
    <div class="tp-card">
      <div class="tp-head">
        <div>
          <h3 id="tp-title" class="tp-title">{t('settings.agentToolsTitle', { name: agentDraftName || '—' })}</h3>
          <div class="d muted">{t('settings.agentToolsRule')}</div>
        </div>
        <button class="icobtn" aria-label={t('settings.close')} onclick={() => (toolPickerOpen = false)}>
          <Icon name="x" size={15} />
        </button>
      </div>

      <input
        class="ctrl tp-search" placeholder={t('settings.agentToolsSearch')}
        bind:value={toolQuery}
      />

      <div class="tp-body">
        {#if toolMatchCount === 0}
          <!-- Two different nothings: the registry has not arrived yet, or it
               has and the search excluded everything. Saying "no matches" while
               the list is still loading is the wrong answer to both. -->
          <div class="muted tp-empty">
            {#if tools.length === 0}{t('settings.loading')}
            {:else}{t('settings.searchNoResults', { q: toolQuery.trim() })}{/if}
          </div>
        {/if}
        {#each agentToolGroups as g (g.key)}
          <div class="group-head tp-group">
            <span class="group-title">{t(`settings.toolSource_${g.key}`)}</span>
            <span class="group-count">{t('settings.itemCount', { n: g.items.length })}</span>
          </div>
          {#each g.items as item (item.name)}
            <div class="tp-row" class:forced={item.forced}>
              <span class="tp-name">{item.name}</span>
              {#if item.forced}
                <!-- Listing these as choosable was a lie the user only found out
                     about after saving — see subagent.forcedDenials. -->
                <span class="tp-forced">{t('settings.agentToolsForced')}</span>
              {:else}
                {@const state = toolStateOf(item.name)}
                <div class="seg-ctrl tp-seg">
                  {#each [
                    { id: 'default', label: t('settings.agentToolDefault') },
                    { id: 'allow', label: t('settings.agentToolAllow') },
                    { id: 'deny', label: t('settings.agentToolDeny') },
                  ] as opt (opt.id)}
                    <button
                      type="button" class="seg-btn tp-{opt.id}"
                      class:selected={state === opt.id}
                      aria-pressed={state === opt.id}
                      onclick={() => setToolState(item.name, opt.id as ToolState)}
                    >{opt.label}</button>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        {/each}
      </div>

      <div class="tp-foot">
        <span class="muted">{agentToolSummary}</span>
        <div class="pp-bar-gap"></div>
        <button class="ctrl" onclick={resetAgentTools} disabled={agentDraftTools.length === 0 && agentDraftDeny.length === 0}>
          {t('settings.agentToolsReset')}
        </button>
        <button class="ctrl ctrl-primary" onclick={() => (toolPickerOpen = false)}>{t('settings.done')}</button>
      </div>
    </div>
  </div>
{/if}

{#if pendingConfirm}
  {@const req = pendingConfirm}
  <ConfirmDialog
    title={req.title}
    message={req.message}
    detail={req.detail ?? ''}
    confirmLabel={req.confirmLabel}
    onConfirm={runPendingConfirm}
    onCancel={() => (pendingConfirm = null)}
  />
{/if}
