<script lang="ts">
  import { onMount, untrack, tick } from 'svelte'
  import { workerFace } from './workerFace'
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
  import ProviderAccount from './ProviderAccount.svelte'
  import Icon from './Icon.svelte'
  import { coverHue } from './coverHue'
  import { armFirstRunReplay } from './firstRun'
  import { scopeLabel } from './memoryScope'
  import { setShell } from './shell.svelte'
  import type { IconName } from './icons'
  import {
    SupportedProviders, HasAPIKey, APIKeyHint, RequiresAPIKey, AcceptsAPIKey, ProviderAccountFor, TerminalShells,
    ListModelsForProvider, ProviderBaseURL, ProviderBaseURLIsCustom, ProviderAPIKeyURL, ProviderReady, PriceModels,
    ProviderWireFormats, TestProviderConnection,
    EnabledProviders, SetProviderEnabled,
    ListMCPServers, SaveMCPServer, RemoveMCPServer, TestMCPServer, ToggleMCPServer,
    DelegateSwitches, SetDelegateOff, SetAgentOff,
    PlacementTargets, SetMCPServerTargets,
    ListExternalSkills, ListTools, InstallSkillFromGitHub, RemoveExternalSkill, RefreshSkills,
    SkillsDir, SkillScanIssues, OpenSkillsFolder, InstallSkillFromZip,
    MCPConfigPath, OpenMCPFolder,
    ListSpeechModels, SetSpeechModel, SpeechStatus, RevealSpeechModel, SpeechModelDirs, OpenSpeechModelDir,
    UsageStats, ListPromptPresets, OpenPromptsFolder,
    SavePromptPreset, DeletePromptPreset, PickPresetImage, RemovePresetImage,
    ListSubagentProfiles, ReadSubagentProfile, SaveSubagentProfile, SaveAgentProfile,
    DeleteSubagentProfile, SetSubagentModel, OpenAgentsFolder, ListChairs,
    AgentSkills, AgentNeeds, OpenAgentSkillsFolder,
    ChairStarters, SaveChairStarters, ChairStartersFile,
    SignInMethods, SignInStatus, StartSignIn, CancelSignIn, ImportableSignIns,
    Connections, ConnectAccount, SetConnectionTargets, VerifyConnection, DisconnectAccount,
    SetConnectionStartCommand, StartConnectionServer, CheckConnectionServer,
    AppVersion, AppCredit, CheckForUpdate, RecentDebugLog,
    LearningEnabled, SetLearningEnabled, ListPendingChanges, ListDecidedChanges,
    ApprovePendingChange, RejectPendingChange, LearnedEntries, LearnedScopeInfos, SaveLearnedEntry, OpenMemoryFolder,
    ForgetMemoryScope, AdoptMemoryScope, RecentProjects,
    ListSystemIssues, MarkIssueReported, ListDecidedIssues,
    AccountStatus, StartAccountSignIn, CompleteAccountSignIn, CancelAccountSignIn,
    AccountSignOut, AccountRefresh,
  } from '../../wailsjs/go/main/App'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  // Deliberately alongside the issue button rather than instead of it: an issue
  // carries the version and the log, the group carries the half-formed question
  // that is not a bug report yet.
  import { COMMUNITY_URL } from './links'
  import promptPayQR from '../assets/images/promptpay-qr.png'
  import { config, update, main, subagent } from '../../wailsjs/go/models'
  import { cockpit, startChatWith, setActiveView, switchProvider, switchModel, submitAPIKey, switchApprovalMode, switchWireFormat, setProviderBaseURL, retryActiveProvider, completeSignIn, signOutProvider, importSignIn, SETTINGS_SECTION_KEY } from './stores/cockpit.svelte'
  import {
    identity, loadIdentityFiles, openIdentityFile, saveIdentityFile,
    createIdentityFile, deleteIdentityFile, identityTemplates,
  } from './identity.svelte'
  import { updater, updatePct, startDownload, restartToUpdate } from './selfUpdate.svelte'

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
      ? identityTemplates().filter((tpl) => !(identity.files || []).some((f) => f.name === tpl.name))
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

  // ---------- General: replay the first run ----------
  // The first screen anyone sees is the one nobody who works on Aetox can see:
  // this machine has been onboarded, has a key, and the wizard's own shortcuts
  // make sure of it. This puts the window back into that state so it can be
  // looked at. Through the same gate as a delete, because a window that
  // reloads and forgets your theme without warning is the same surprise even
  // when nothing is destroyed.
  const replayFirstRun = () => askConfirm({
    title: t('settings.firstRunConfirmTitle'),
    message: t('settings.firstRunConfirmMessage'),
    confirmLabel: t('settings.firstRunAction'),
    run: () => {
      armFirstRunReplay()
      window.location.reload()
    },
  })

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
  // acceptsKey is not the negation of requiresKey: Codex requires credentials
  // and takes no key, because the only key a user could paste belongs to a
  // different host and a different bill.
  // ready is null until the engine answers. A local runtime cannot be known to
  // be up without asking it, and the dot used to be painted green on the
  // strength of "needs no key" — which is why LM Studio looked connected on a
  // card that said no models were found. Unknown must look like unknown.
  type ProviderRow = {
    name: string; requiresKey: boolean; acceptsKey: boolean; hasKey: boolean
    // The masked tail of the key that would actually be sent, or "" when
    // there is none. hasKey answers whether one exists; this answers which
    // one, which is the question a blank field could not.
    keyHint: string
    ready: boolean | null
  }

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
  // Where this provider's key is actually created. Empty for the rows that have
  // no such page (a local runtime, a sign-in provider), which is also the
  // signal not to draw the link.
  let keyPageURL = $state('')
  let customModel = $state('')
  // The model list is unusable without these on any hosted aggregator: 411
  // rows, alphabetical, so reaching "deepseek" means scrolling past every
  // aion-labs build first.
  let modelFilter = $state('')
  let freeOnly = $state(false)
  type ModelListing = {
    model: string; input: number; output: number
    priced: boolean; free: boolean; context: number
  }
  let priced = $state<Record<string, ModelListing>>({})

  // Cheapest first once prices are known, because a price nobody can sort by is
  // a number to look at rather than one to decide with. Ties and unpriced rows
  // keep the provider's own order, which is the only order they have.
  const visibleModels = $derived.by(() => {
    const needle = modelFilter.trim().toLowerCase()
    const rows = models.filter((m) => {
      if (needle && !m.toLowerCase().includes(needle)) return false
      if (freeOnly && !priced[m]?.free) return false
      return true
    })
    return rows.sort((a, b) => {
      const pa = priced[a], pb = priced[b]
      if (pa?.priced && pb?.priced && pa.input !== pb.input) return pa.input - pb.input
      if (pa?.priced !== pb?.priced) return pa?.priced ? -1 : 1
      return 0
    })
  })
  const freeCount = $derived(models.filter((m) => priced[m]?.free).length)

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

  // ---------- Connections (accounts the agent acts on your behalf with) -------
  // Next to sign-in because they are the same act from the user's side, and
  // apart from it because they answer different questions: a sign-in above buys
  // thinking, a connection here buys reach. The store underneath is shared; the
  // page is not.
  type ConnectionRow = {
    id: string; label: string; kind: string; token_url?: string
    connected: boolean; login?: string; source?: string; env_override: boolean
    for: string[]; configured: boolean; tools: string[]
    // A service the user hosts has no address of its own. GitHub is one host
    // for everybody and states it as a constant; n8n and Windmill live wherever
    // the user put them, so the row carries the address, the example to show in
    // an empty field, and the fact that it needs one at all.
    needs_base_url: boolean; base_url?: string; base_url_hint?: string
    // Which page the row belongs on. "automation" is n8n and Windmill; a
    // service that stands alone declares none and stays on the register.
    family?: string
    // The one agent this connection is locked to, when it has one — the row
    // draws the fact rather than a picker. Present on connect.Status all along;
    // this hand-written mirror had simply drifted behind it.
    home_agent?: string
    /** Agents this connection already reaches before anybody places it
     *  (connect.Provider.DefaultAgents). Ticked, because the engine grants it. */
    default_agents?: string[]
    // How to bring this one up, for the services the user runs themselves.
    start_command?: string
  }

  let connections = $state<ConnectionRow[]>([])
  // Keyed by connection id, because the page draws one card per service and two
  // of them must not share a token box, an error, or a spinner.
  let connToken = $state<Record<string, string>>({})
  // The address of a self-hosted service, seeded from what is stored so a
  // reconnect after a rotated key does not ask the user to retype where their
  // own server lives.
  let connBaseURL = $state<Record<string, string>>({})
  let connError = $state<Record<string, string>>({})
  // Scopes come from the last live answer rather than from storage: a token's
  // grants can change on the service's side, and a remembered list would keep
  // claiming access that was revoked this morning.
  let connScopes = $state<Record<string, string[]>>({})
  // The placement chosen *before* connecting. Once connected, the toggles write
  // straight through, so this only exists for the not-yet-connected card.
  let connDraft = $state<Record<string, string[]>>({})
  // '' | '<id>:connect' | '<id>:verify' — one field, so no two buttons anywhere
  // on the page can both be spinning.
  let connBusy = $state('')
  // Which row is open. One at a time, the same as the MCP register: two token
  // forms on screen is two places to paste into and one of them is wrong.
  let connOpen = $state('')

  // What the collapsed row says about placement — the desks by name, because
  // "2 desks" makes you open the row to find out which two.
  function placementSummary(row: ConnectionRow): string {
    if (!row.configured) return t('settings.connForEveryone')
    const names = row.for.map((id) => mcpTargets.find((tt) => tt.id === id)?.name ?? id)
    return names.join(', ')
  }

  // One line per service saying what connecting it buys. Kept here rather than
  // in the Go catalog because it is a sentence a Thai-first app has to
  // translate, and a Go string literal cannot be. A service with no line drawn
  // yet renders nothing rather than a raw key.
  const connBlurb: Record<string, string> = $derived({ github: t('settings.ghDesc') })

  // Desks are pre-picked and agents are not — an agent is handed things on
  // purpose, which is the asymmetry the resolver applies to a connection nobody
  // has placed yet (config.ConnectionsForAgent).
  //
  // With one exception the catalog writes down: a service can name the agents
  // it starts at (connect.Provider.DefaultAgents — GitHub names the github
  // agent). Ticked here because the engine ALREADY grants it, and a chip drawn
  // unticked over a grant that is in force is the page saying something untrue.
  // Everything stays clickable; this decides where an untouched row begins.
  const defaultDraft = (row?: ConnectionRow) => [
    ...mcpTargets.filter((t) => t.kind === 'desk').map((t) => t.id),
    ...(row?.default_agents ?? [])
      .map((name) => mcpTargets.find((t) => t.kind === 'agent' && t.name === name)?.id)
      .filter((id): id is string => !!id),
  ]

  async function loadConnections() {
    if (mcpTargets.length === 0) {
      try {
        mcpTargets = await PlacementTargets()
      } catch {
        /* the toggles are simply absent rather than the page failing */
      }
    }
    try {
      connections = ((await Connections()) ?? []) as ConnectionRow[]
    } catch {
      connections = []
      return
    }
    for (const row of connections) {
      // Only when the box is untouched: a reload while the user is mid-typing
      // must not overwrite what they are typing with what is stored.
      if (row.needs_base_url && connBaseURL[row.id] === undefined) {
        connBaseURL[row.id] = row.base_url ?? ''
      }
      if (row.needs_base_url && connStart[row.id] === undefined) {
        connStart[row.id] = row.start_command ?? ''
      }
      if (connDraft[row.id]) continue
      // A connection already placed keeps its list when it is reconnected; one
      // that has never been placed starts where the resolver would put it.
      connDraft[row.id] = row.configured ? [...row.for] : defaultDraft(row)
    }
  }

  // Every service, on one page.
  //
  // There were two for nine days. The reasoning was real — an automation engine
  // is a machine you run, it takes an address as well as a key, and setting one
  // up is a different conversation from signing an account in — and the owner's
  // verdict on 19 ส.ค. was that the difference is smaller than the thing they
  // share: *"มันคืออันเดียวกันแท้ๆ เชื่อมต่อแอปภายนอก เอาคีย์ไปใส่"*. Two pages
  // for one question meant looking in the wrong one first, every time.
  //
  // The rows already carry the difference: a self-hosted engine draws its
  // address section and its own service's words, an account draws neither. So
  // the page does not need to sort them; it needed to stop hiding half of them.
  //
  // `family` stays where it was, in the Go catalog, because it answers a
  // different question and always did: which engines substitute for each other
  // in the composer's picker (connect.InFamily). It was never the page's fact.
  const visibleConnections = $derived(connections)

  // What a card's toggles are currently showing: the live placement once it is
  // connected, the draft while it is not.
  function targetsOf(row: ConnectionRow): string[] {
    return row.source === 'connection' ? row.for : (connDraft[row.id] ?? [])
  }

  function servesNobody(row: ConnectionRow): boolean {
    return row.configured && row.for.length === 0
  }

  async function toggleConnectionTarget(row: ConnectionRow, targetID: string) {
    const current = targetsOf(row)
    const next = current.includes(targetID)
      ? current.filter((t) => t !== targetID)
      : [...current, targetID]

    if (row.source !== 'connection') {
      connDraft[row.id] = next // nothing to write yet — it lands with Connect
      return
    }
    if (connBusy) return
    connBusy = row.id + ':connect'
    connError[row.id] = ''
    try {
      await SetConnectionTargets(row.id, next)
      connDraft[row.id] = next
      await loadConnections()
    } catch (e) {
      connError[row.id] = String(e)
    } finally {
      connBusy = ''
    }
  }

  /** A service the user hosts needs its address before the token means anything;
   *  the rest need only the token. Read by the button so it cannot be pressed
   *  into a request that was always going to be refused. */
  function connectable(row: ConnectionRow): boolean {
    if (!(connToken[row.id] ?? '').trim()) return false
    return !row.needs_base_url || (connBaseURL[row.id] ?? '').trim() !== ''
  }

  async function connectAccount(row: ConnectionRow) {
    const token = (connToken[row.id] ?? '').trim()
    if (!connectable(row) || connBusy) return
    connBusy = row.id + ':connect'
    connError[row.id] = ''
    try {
      const account = await ConnectAccount(
        row.id, token, (connBaseURL[row.id] ?? '').trim(), connDraft[row.id] ?? [])
      connScopes[row.id] = account.scopes ?? []
      // Cleared on success only. A token that failed stays in the box, because
      // the usual reason is a truncated paste and retyping the whole thing is a
      // punishment for the app's own unhelpfulness.
      connToken[row.id] = ''
      await loadConnections()
    } catch (e) {
      connError[row.id] = String(e)
    } finally {
      connBusy = ''
    }
  }

  // Bringing a self-hosted engine up, and asking whether it is up.
  //
  // `srvState[id]` is 'ok:…' / 'err:…' like the model connection test, so the
  // two read the same on screen — one shape for "I asked something and here is
  // what it said", rather than a new spelling per page.
  let connStart = $state<Record<string, string>>({})
  let srvBusy = $state('')
  let srvState = $state<Record<string, string>>({})

  /** Saved when the field loses focus rather than behind a Save button: it is
   *  one line, and a command typed and then lost because nobody pressed save is
   *  the kind of small betrayal this page should not commit. */
  async function saveStartCommand(row: ConnectionRow) {
    const next = (connStart[row.id] ?? '').trim()
    if (next === (row.start_command ?? '')) return
    try {
      await SetConnectionStartCommand(row.id, next)
      await loadConnections()
    } catch (e) {
      srvState[row.id] = 'err:' + String(e)
    }
  }

  async function startServer(row: ConnectionRow) {
    if (srvBusy) return
    srvBusy = row.id + ':start'
    srvState[row.id] = ''
    try {
      // Saved first: pressing start with an edited, unsaved command would run
      // the old one and report on the new.
      await saveStartCommand(row)
      await StartConnectionServer(row.id)
      srvState[row.id] = 'ok:' + t('settings.connUp')
    } catch (e) {
      srvState[row.id] = 'err:' + String(e)
    } finally {
      srvBusy = ''
    }
  }

  async function checkServer(row: ConnectionRow) {
    if (srvBusy) return
    srvBusy = row.id + ':check'
    srvState[row.id] = ''
    try {
      const up = await CheckConnectionServer(row.id)
      srvState[row.id] = up ? 'ok:' + t('settings.connUp') : 'err:' + t('settings.connDown')
    } catch (e) {
      srvState[row.id] = 'err:' + String(e)
    } finally {
      srvBusy = ''
    }
  }

  /** Disconnecting throws away a credential the user cannot get back — an n8n
   *  key is shown once at creation and never again — so it goes through the same
   *  gate as every other loss on this page. It was the one destructive button
   *  here that just did it. */
  function askDisconnect(row: ConnectionRow) {
    askConfirm({
      title: t('settings.connDisconnectTitle', { name: row.label }),
      message: t('settings.connDisconnectMessage', { name: row.label }),
      // What survives is worth saying: coming back later means pasting a key,
      // not choosing the desks all over again.
      detail: t('settings.connDisconnectDetail'),
      confirmLabel: t('settings.ghDisconnect'),
      run: () => void disconnectAccount(row),
    })
  }

  async function disconnectAccount(row: ConnectionRow) {
    if (connBusy) return
    connBusy = row.id + ':connect'
    connError[row.id] = ''
    try {
      await DisconnectAccount(row.id)
      delete connScopes[row.id]
      await loadConnections()
    } catch (e) {
      connError[row.id] = String(e)
    } finally {
      connBusy = ''
    }
  }

  async function verifyConnection(row: ConnectionRow) {
    if (connBusy) return
    connBusy = row.id + ':verify'
    connError[row.id] = ''
    try {
      const account = await VerifyConnection(row.id)
      connScopes[row.id] = account.scopes ?? []
      await loadConnections()
    } catch (e) {
      connError[row.id] = String(e)
    } finally {
      connBusy = ''
    }
  }

  const signInProviderNames = $derived(new Set(signInMethods.map((m) => m.provider)))
  const signInMethod = $derived(signInMethods.find((m) => m.provider === selected) ?? null)
  const signInStatus = $derived(signedIn[selected] ?? null)
  let busy = $state('')
  let errorMsg = $state('')

  const selectedRow = $derived(providers.find((p) => p.name === selected))
  // Fetched per provider rather than for all of them at once: opening a card is
  // what asks, so a user who never opens Settings never spends a round trip on
  // a balance nobody is looking at.
  let accounts = $state<Record<string, any>>({})
  const account = $derived(accounts[selected] ?? null)

  async function loadAccount(name: string) {
    if (!name) return
    try {
      accounts = { ...accounts, [name]: await ProviderAccountFor(name) }
    } catch {
      // A provider that cannot answer leaves its card without the line, which
      // is the same as never having asked. Nothing else on the page depends
      // on it, so this must not surface as a page-level failure.
    }
  }

  async function refreshAccount() {
    busy = 'account'
    await loadAccount(selected)
    busy = ''
  }
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

  // The team page's two doors land here: configure-on-a-card and the create
  // form both open this page with the editor already holding the right
  // profile — and the right *kind*, which the intent carries because it came
  // from the roster, not from re-reading any file. Consumed once and cleared:
  // an intent that survived into the next plain visit would reopen an editor
  // nobody asked for.
  onMount(async () => {
    const intent = cockpit.settingsIntent
    if (!intent) return
    cockpit.settingsIntent = null
    openSection(intent.section)
    if (intent.section !== 'team') return
    if (intent.createAgent) {
      newAgent('agent')
      return
    }
    if (intent.agent) {
      await loadAgents()
      const row = subagents.find((a) => a.name === intent.agent)
      if (row) await openAgent(row, 'agent')
    }
  })

  // ---------- About ----------
  // Kept out of bootSettings on purpose. The version is a constant the Go side
  // always has, and folding it into that Promise.all would let a stumble here
  // take the whole Settings page down with it for nothing.
  let appVersion = $state('')
  let appCredit = $state('')
  let updateStatus = $state<update.Status | null>(null)
  let updateChecking = $state(false)
  let updateError = $state('')
  let hintCopied = $state(false)
  // Downloading / verifying / swapping / it failed does NOT live here. That
  // state belongs to the act, not to this page: the same update can be started
  // from the notice the automatic check raises, and two private copies of
  // "42%, restarting, here is what went wrong" are two answers to one question
  // waiting to disagree. selfUpdate.svelte owns it; this page reads it, and the
  // progress dialog it drives (Updater.svelte) covers this window either way.
  onMount(() => {
    void (async () => {
      try {
        appVersion = await AppVersion()
        appCredit = await AppCredit()
      } catch {
        /* the About page shows a dash rather than an error */
      }
    })()
  })

  const CHANNEL_LABELS: Record<string, TKey> = {
    scoop: 'settings.aboutChannelScoop',
    installer: 'settings.aboutChannelInstaller',
    portable: 'settings.aboutChannelPortable',
    store: 'settings.aboutChannelStore',
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
      acceptsKey: await AcceptsAPIKey(name),
      hasKey: await HasAPIKey(name),
      keyHint: await APIKeyHint(name),
      ready: null,
    })))
    // Readiness is asked for separately and not awaited with the rest: proving
    // a local runtime is up means opening a connection to it, and a dead port
    // costs a timeout. The list must draw immediately and fill in, rather than
    // hold the whole page for the one provider that is switched off.
    for (const row of providers) {
      ProviderReady(row.name)
        .then((ready) => { row.ready = ready })
        .catch(() => { row.ready = false })
    }
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

  // The Aetox account, which is a different sign-in from the ones above: those
  // decide who pays for a request, this one is who you are to Aetox itself.
  let aetoxAccount = $state<main.AccountState | null>(null)
  let aetoxBusy = $state(false)
  let aetoxError = $state('')

  async function loadAetoxAccount() {
    aetoxAccount = await AccountStatus()
  }

  // Same two-call shape as a provider sign-in, and for the same reason: the
  // first call hands back a URL to open, the second blocks until the browser
  // comes back. Nothing is stored unless the second one succeeds.
  async function aetoxSignIn(provider: string) {
    aetoxError = ''
    aetoxBusy = true
    try {
      const url = await StartAccountSignIn(provider)
      if (url) BrowserOpenURL(url)
      aetoxAccount = await CompleteAccountSignIn()
    } catch (e) {
      aetoxError = String(e)
      await loadAetoxAccount()
    } finally {
      aetoxBusy = false
    }
  }

  async function aetoxAbort() {
    await CancelAccountSignIn()
    aetoxBusy = false
    aetoxError = ''
  }

  async function aetoxSignOut() {
    aetoxError = ''
    aetoxBusy = true
    try {
      await AccountSignOut()
    } catch (e) {
      // The local half of a sign-out always happened, so this is a warning
      // about the server, not a failure to sign out. The card says so.
      aetoxError = t('settings.accountSignOutPartial')
    } finally {
      await loadAetoxAccount()
      aetoxBusy = false
    }
  }

  async function aetoxCheck() {
    aetoxError = ''
    aetoxBusy = true
    try {
      aetoxAccount = await AccountRefresh()
    } catch (e) {
      aetoxError = String(e)
      await loadAetoxAccount()
    } finally {
      aetoxBusy = false
    }
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
    connTesting = {}
    connResult = {}
    baseURL = await ProviderBaseURL(name)
    baseURLDraft = baseURL
    baseURLIsCustom = await ProviderBaseURLIsCustom(name)
    keyPageURL = await ProviderAPIKeyURL(name)
    // Not awaited: a slow provider must not hold up the rest of the card.
    loadAccount(name)
    wireFormats = await ProviderWireFormats(name)
    loadingModels = true
    models = []
    modelFilter = ''
    freeOnly = false
    priced = {}
    try {
      const res = await ListModelsForProvider(name)
      models = Array.isArray(res) ? res : []
      // Prices for the list that is on screen, not for a list fetched again:
      // OpenRouter alone returns 411 models and a second discovery call could
      // answer differently. Not awaited — the names are usable before the
      // money arrives.
      if (models.length) {
        PriceModels(name, models)
          .then((rows) => {
            const next: Record<string, ModelListing> = {}
            for (const r of rows ?? []) next[r.model] = r
            priced = next
          })
          .catch(() => { priced = {} })
      }
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
      case 'responses': return 'Responses'
      default: return format
    }
  }

  const useFormat = (fmt: string) => run('format:' + fmt, async () => {
    if (!isActiveProvider) await switchProvider(selected)
    await switchWireFormat(fmt)
  })

  // Connection test: a real 1-token completion through the chat path, run per
  // model so a model can be proven before switching to it.
  //
  // Deliberately outside run(). That helper takes the page's single `busy`
  // lock, which is the right shape for switching a provider or saving a key —
  // one of those at a time is the only sane number. A probe is not one of
  // those: it changes nothing, it waits however long the provider waits, and
  // holding the whole page while it does meant a list of twelve models could
  // only be checked twelve waits deep, one after another. Owner, 22 ส.ค.:
  // "กดแล้วไม่ต้องรอ ไปกดอันอื่นได้เลย".
  //
  // Two records rather than two strings, for the same reason: one string holds
  // one answer, so testing a second model erased the first one's result — the
  // comparison you were running the tests to make.
  //
  // It also leaves errorMsg alone. A failed probe belongs under the row that
  // failed, where it stays; the page-level banner holds one message, and with
  // several probes in flight the last failure would speak for all of them.
  let connTesting = $state<Record<string, boolean>>({})
  let connResult = $state<Record<string, string>>({}) // per model: 'ok:…' | 'err:…'
  const testConnection = async (name: string) => {
    if (connTesting[name]) return
    // Which provider asked. A probe can now outlive the page state it started
    // in, so a late answer must not land under a row belonging to whatever
    // provider has been clicked into since. Only possible because these run
    // concurrently — the lock used to make it unreachable.
    const asked = selected
    connTesting[name] = true
    delete connResult[name]
    try {
      const ok = 'ok:' + await TestProviderConnection(asked, name)
      if (asked === selected) connResult[name] = ok
    } catch (err) {
      if (asked === selected) connResult[name] = 'err:' + String(err)
    } finally {
      delete connTesting[name]
    }
  }

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
    // The allowlist, if one was written. `tools` above is the count the server
    // offers; this is which of them are taken. Absent means all.
    allowed?: string[]
    // Who carries this server's tools. Absent from the row means the engine
    // sent nothing, which is not the same as "nobody" — treated as [] here and
    // shown as attached nowhere.
    for?: string[]
  }
  // `detail` is the desk's own description — a paragraph, which is why it is a
  // tooltip and `name` is what the chip prints.
  type MCPTargetRow = { id: string; name: string; detail?: string; kind: string }
  let mcpServers = $state<MCPRow[]>([])
  // Everywhere a server can be pointed, from the engine — the desks and the
  // team that actually exist. Not a list typed in here, so hiring an agent
  // puts it on these switches without this page being edited.
  let mcpTargets = $state<MCPTargetRow[]>([])
  // Which row is expanded. One at a time: the switches are the reason to open
  // a row, and two rows open at once turns a register into a wall.
  let mcpOpen = $state('')
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
  // One tool name per line, and blank means take all of them. A textarea rather
  // than a list of checkboxes because the names are not known until the server
  // has been connected once, and this form is where a server is first written.
  let mcpToolsText = $state('')
  // Set when a preset was handed to the form because it needs a key, so the
  // form can say why it opened instead of just appearing.
  let mcpNeedsKey = $state(false)
  // Whether the add/edit form is on screen at all.
  //
  // Closed by default: adding a server is a rare act, and eight controls laid
  // out permanently under the list read as part of the page rather than as a
  // thing you do. Owner, 2026-08-14: *"ทำเป็นปุ่มกด เพิ่ม SERVER แล้วค่อยแสดง
  // ดีกว่ามาเรี่ยราดแบบนี้"*.
  //
  // **Explicit state rather than derived from the fields**, because the one
  // state that has to be distinguishable — add mode, freshly opened — is the
  // one where every field is empty, which is exactly what closed looks like.
  //
  // Three things open it and they must all keep doing so: the button, `editMCP`
  // (the row's แก้ไข), and a preset that needs a key pasted. The second and
  // third are the ones a naive fold breaks — the click appears to do nothing,
  // and it does it silently.
  let mcpFormOpen = $state(false)
  let mcpFormEl = $state<HTMLElement | null>(null)
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
    mcpTargets = await PlacementTargets()
  }

  const mcpTargetsOf = (s: MCPRow) => s.for ?? []
  const mcpServesNobody = (s: MCPRow) => mcpTargetsOf(s).length === 0

  // Flipping one switch sends the whole list back, because that is what the
  // engine stores — a per-target call would need the engine to merge, and two
  // places deciding what the list is now is how one of them ends up wrong.
  const toggleMCPTarget = (s: MCPRow, id: string) => runMCP('target:' + s.name + ':' + id, async () => {
    const current = mcpTargetsOf(s)
    const next = current.includes(id) ? current.filter((x) => x !== id) : [...current, id]
    await SetMCPServerTargets(s.name, next)
    await loadMCP()
  })

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
    mcpToolsText = ''
    mcpNeedsKey = false
    // Every caller of this — Cancel, a successful save, deleting the server
    // being edited — is a moment the form is finished with. Closing here rather
    // than at each call site is what stops one of them from being forgotten.
    mcpFormOpen = false
  }

  // Open the form and make sure it is actually looked at. Every way in is a
  // click *above* where the form appears — the header's button, a row's แก้ไข, a
  // preset — so without the scroll the answer to "did that work" is somewhere
  // off the bottom of the page.
  //
  // `await tick()` and not a microtask: the form does not exist yet when this
  // runs. It is created by the `{#if}` reacting to the line above, so
  // `mcpFormEl` is still null until Svelte has flushed — scrolling before that
  // is scrolling to nothing, silently.
  async function openMCPForm() {
    mcpFormOpen = true
    await tick()
    // Optional *call*, not just optional element: the scroll is a courtesy and
    // must never be the thing that fails. jsdom has no scrollIntoView, so a
    // plain call turns every test that opens this form into an unhandled
    // rejection — noise that goes on to hide a real one.
    mcpFormEl?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })
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
    mcpToolsText = (s.allowed ?? []).join('\n')
    mcpNeedsKey = false
    mcpError = ''
    openMCPForm()
  }

  // An auth header that names its scheme and carries no credential.
  //
  // "Authorization: Bearer" is what a preset hands the form so a token can be
  // pasted after the prefix, and it is also exactly what lands on disk when the
  // user presses Save without pasting one. The server then answers 400 and the
  // page reports "Bad Request", which is true and useless: nothing on screen
  // connects that to the empty box (owner, 2026-08-14, on a github server doing
  // precisely this).
  //
  // Caught here rather than deeper down because this is the only place that
  // knows the value was typed rather than received. The engine cannot tell an
  // empty credential from a server that wants none.
  const AUTH_SCHEMES = ['bearer', 'basic', 'token', 'apikey']
  function credentiallessHeader(headers: Record<string, string>): string {
    for (const [key, value] of Object.entries(headers)) {
      const v = value.trim()
      // A ${env:X} or ${connect:x} reference is a value — it is resolved at
      // connect time (internal/bootstrap resolveSecretRefs), and the whole
      // point of it is that the secret never gets typed here.
      if (/\$\{(env|connect):[^}]+\}/.test(v)) continue
      if (v === '') return key
      if (AUTH_SCHEMES.includes(v.toLowerCase())) return key
    }
    return ''
  }

  const saveMCP = () => runMCP('save', async () => {
    const headers = mcpKind === 'http' ? parseLines(mcpHeadersText, ':') : {}
    const empty = credentiallessHeader(headers)
    if (empty) {
      // Thrown, not warned: runMCP shows it and nothing is written. A server
      // saved in this state can never connect, so saving it is not a smaller
      // failure than refusing to.
      throw new Error(t('settings.mcpHeaderNoValue', { header: empty }))
    }
    const server = new config.MCPServerConfig({
      name: mcpName.trim(),
      command: mcpKind === 'stdio' ? mcpCommand.trim().split(/\s+/).filter(Boolean) : [],
      url: mcpKind === 'http' ? mcpUrl.trim() : '',
      environment: mcpKind === 'stdio' ? parseLines(mcpEnvText, '=') : {},
      headers,
      cwd: mcpCwd.trim(),
      // A blank box means "no override", which is 0 — not a timeout of zero.
      timeoutMs: Number.parseInt(mcpTimeout, 10) > 0 ? Number.parseInt(mcpTimeout, 10) : 0,
      // Always an array, never omitted: the engine keeps whatever it has stored
      // when this field is absent, so omitting it on an empty box would make
      // the list unclearable from the only screen that shows it.
      tools: mcpToolsText.split('\n').map((line) => line.trim()).filter(Boolean),
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

  // What the shelf is for. **The rule changed on 2026-08-14 and both halves of
  // it are recorded here, because the older one is still good reasoning and the
  // next person deserves to see why it was set aside rather than forgotten.**
  //
  // *Until 12 ส.ค.:* only the servers this product's own agents declare they
  // need. It had carried five general-purpose picks (context7,
  // sequential-thinking, memory, js-repl, exa) and none of the things a bundled
  // agent asks for by name, which was backwards in both directions — the github
  // agent ships `needs: mcp:github` in its own file and sent the user to a page
  // with nothing on it, while recommending a server Aetox does not depend on is
  // a recommendation it has no standing to make.
  //
  // *From 14 ส.ค. (owner: "เพิ่มมาเลยครับ พวกที่ต้อง OAuth ตัดออกก็ได้"):* also
  // the hosted servers that add something Aetox genuinely cannot do itself and
  // work on one click. The standing objection is answered by the second half of
  // that sentence rather than by dropping it — what made the old five a bad
  // shelf was not that they were popular, it was that a list is a promise, and
  // an entry that cannot connect breaks it. So the bar is now:
  //
  //   1. **It reaches something Aetox has no tool for.** No preset for a
  //      filesystem, a fetcher or a browser — those already exist here, and a
  //      second one is a slower path to the same place plus a tool-block bill.
  //   2. **It works with one click.** Static-header auth at worst. Anything
  //      that wants OAuth is left off: internal/mcp/client.go carries only
  //      static headers ("OAuth stays deferred until a real need appears"), so
  //      a Notion or Linear entry would be a button leading to a form asking
  //      for a token the user has no way to obtain — the exact failure the
  //      paragraph below was written about.
  //   3. **The endpoint was answered by the provider, not remembered.**
  //
  // Every URL below was verified on 2026-08-14 by sending a real MCP
  // `initialize` and reading the reply: the unauthenticated ones returned a
  // protocol handshake, and github returned 401 naming the header it wants.
  // firecrawl was added the same day and probed the same way — it answered
  // twice, once bare and once with a deliberately invalid bearer token, and
  // served the same tool set both times, which is what established that the key
  // is optional rather than merely unchecked on the handshake.
  // Notion, Linear, Sentry and Atlassian all answered `invalid_token` — real
  // servers, all four blocked by rule 2 until the client learns OAuth. Stripe
  // takes a static key and is a one-line addition whenever it is wanted.
  //
  // A first pass had four of those reading 403 and nearly went in the notes as
  // "needs auth". It was Cloudflare's bot check refusing the probe's own user
  // agent. **A verification that can fail for its own reasons has to be read
  // twice**, which is the whole argument for rule 3.
  //
  // `headers` names what the server cannot work without, and an entry may carry
  // the value's prefix after a colon — GitHub wants `Authorization: Bearer
  // <token>`, and a form pre-filled with only the header name is one a token
  // gets pasted into raw. A preset that needs a key used to be saved straight
  // to disk with none, so one click produced a server that could never connect
  // and the page never said which header it wanted — it knew, and did not tell.
  // `why` is rule 1 said out loud, per entry: what this reaches that Aetox has
  // no tool for. It is on screen because the shelf never answered the question
  // a user actually has in front of it — not "what is this" but "why is it
  // being recommended to me". An entry whose `why` cannot be written without
  // hedging is an entry that does not pass rule 1 and should not be here.
  const mcpPresets: { name: string; desc: string; why: string; command?: string[]; url?: string; headers?: string[] }[] = [
    { name: 'github', desc: 'Repos, pull requests, issues, CI', why: "Aetox's own github tool only reads. This is the half that acts — opening a pull request, commenting, moving an issue.", url: 'https://api.githubcopilot.com/mcp/', headers: ['Authorization: Bearer ${connect:github}'] },
    // Second because it is the other one a bundled agent asks for by name — the
    // research agent ships `needs: mcp:firecrawl`, and the 12 ส.ค. half of the
    // rule above is exactly this case.
    //
    // **No `headers` entry, and that is the finding rather than an omission.**
    // Probed 2026-08-14: this endpoint answers a full handshake with no
    // credential at all (firecrawl-fastmcp 3.24.0, protocol 2025-06-18) and
    // serves search, scrape and parse under a usage limit — so it clears rule 2
    // more cleanly than github, which cannot connect until a token exists. A
    // key raises the limits and unlocks the account tools, and it goes in as
    // `Authorization: Bearer ${env:FIRECRAWL_API_KEY}` through แก้ไข. Listing
    // the header here instead would make needsPaste open the form and demand a
    // key for a server that works without one.
    //
    // Rule 1 is the judgment call, and it is a split: scrape overlaps web_fetch
    // and is not why this is here. `firecrawl_map` (enumerate every URL under a
    // site) and `firecrawl_agent` (multi-source research, collected later via
    // firecrawl_agent_status) are both things Aetox has no tool for — web_fetch
    // reads one page and web_search returns eight results.
    { name: 'firecrawl', desc: 'Crawl a whole site, and multi-source research', why: 'Aetox reads one page at a time and gets eight search results. This walks a whole site and researches across many sources at once.', url: 'https://mcp.firecrawl.dev/v2/mcp' },
    { name: 'context7', desc: 'Up-to-date docs for a library, by version', why: 'Docs for the version actually installed. Fetching a documentation page cannot tell you which release it describes.', url: 'https://mcp.context7.com/mcp' },
    { name: 'deepwiki', desc: 'Ask questions about any public GitHub repository', why: 'Answers about a repository without cloning it first — reading one that size through file tools costs a whole context.', url: 'https://mcp.deepwiki.com/mcp' },
    { name: 'exa', desc: 'Web search built for models to read', why: 'Results returned as text to read rather than as pages to open, so an answer costs one call instead of a search and five fetches.', url: 'https://mcp.exa.ai/mcp' },
    { name: 'huggingface', desc: 'Search models, datasets and spaces', why: 'Aetox has no index of models, datasets or spaces, and a web search finds blog posts about them rather than the things.', url: 'https://huggingface.co/mcp' },
    { name: 'cloudflare-docs', desc: "Search Cloudflare's documentation", why: "Cloudflare's own index of its own docs, which is a different thing from a web search that happens to land there.", url: 'https://docs.mcp.cloudflare.com/mcp' },
  ]

  const presetTaken = (name: string) => mcpServers.some((s) => s.name.toLowerCase() === name.toLowerCase())

  // Which agents declare they need this server, **read off their own files**
  // rather than written beside the preset.
  //
  // The shelf is where a user asks "why is this here and is it for me", and the
  // honest answer to the second half is a fact the agent already states: its
  // `needs:` line. Restating it here would be a second answer to that question,
  // and it would be the one that goes stale — an agent edited to drop a server
  // would keep being advertised for it, from a list nobody thinks to update.
  //
  // Alternatives are split because one entry may say `connection:n8n |
  // mcp:windmill`, which counts as needing windmill.
  const agentsNeeding = (id: string): string[] =>
    subagents
      .filter((p) => (p.needs ?? []).some((entry: string) =>
        entry.split('|').some((alt: string) => alt.trim().toLowerCase() === 'mcp:' + id.toLowerCase())))
      .map((p) => p.name)

  // A header entry that already carries a ${...} reference needs nothing from
  // the user: the value resolves at connect time from a secret the app already
  // holds. Only a header still waiting for a paste opens the form.
  const needsPaste = (headers?: string[]) =>
    (headers ?? []).some((h) => !/\$\{(env|connect):[^}]+\}/.test(h))

  const addPreset = (p: (typeof mcpPresets)[number]) => runMCP('preset:' + p.name, async () => {
    if (p.headers?.length && needsPaste(p.headers)) {
      // Hand it to the form with the header names already in, rather than
      // saving something that cannot connect. Nothing is written until the key
      // is pasted and Save is pressed.
      resetMCPForm()
      mcpKind = p.url ? 'http' : 'stdio'
      mcpName = p.name
      mcpUrl = p.url ?? ''
      mcpCommand = (p.command ?? []).join(' ')
      // An entry that already carries the value's prefix ("Authorization:
      // Bearer") keeps it and gets one space; a bare header name gets the colon.
      mcpHeadersText = p.headers.map((h) => (h.includes(':') ? `${h} ` : `${h}: `)).join('\n')
      mcpNeedsKey = true
      // After resetMCPForm above, which closes it.
      openMCPForm()
      return
    }
    await SaveMCPServer('', new config.MCPServerConfig({
      name: p.name, command: p.command ?? [], url: p.url ?? '',
      headers: Object.fromEntries((p.headers ?? []).map((h) => {
        const at = h.indexOf(':')
        return [h.slice(0, at).trim(), h.slice(at + 1).trim()]
      })),
    }))
    await loadMCP()
  })

  // Install the server an agent says it is missing, and place the agent on it,
  // from the agent's own card.
  //
  // **The door used to lead to a page rather than to the fix.** An agent that
  // declares `needs: mcp:firecrawl` sent the user to the MCP section, where the
  // thing they needed sat in a shelf of seven with nothing saying which one it
  // was — owner, 2026-08-14: *"คนเขาไม่รู้หรอกอันไหนเราทำไว้เพื่อตัวไหน"*. That
  // is true, and it is the whole complaint: not the clicking, the matching.
  //
  // **Why this is a button and not a default.** Installing a bundled agent's
  // server at startup would wire a fresh install to a third party the user never
  // chose — for firecrawl, an outbound dependency on mcp.firecrawl.dev before
  // anyone has opened the agent once. It also inverts needs.go's one rule:
  // `needs:` declares and never grants, `for:` grants and is the only thing that
  // does. Pressed here, the user is standing in front of the agent that wants
  // it, with the reason on screen — which is the same one click, and still
  // their decision. What is removed is the matching, not the consent.
  //
  // Only for a preset that connects with no key. One that wants a token pasted
  // cannot be finished in one press, so it keeps the door to the page, where the
  // form is waiting with the header names already filled in.
  const presetFor = (id: string) =>
    mcpPresets.find((p) => p.name.toLowerCase() === id.toLowerCase() && !needsPaste(p.headers))

  const installNeeded = (o: subagent.Need) => runMCP('need:' + o.id, async () => {
    const p = presetFor(o.id)
    if (!p) return
    await SaveMCPServer('', new config.MCPServerConfig({
      name: p.name, command: p.command ?? [], url: p.url ?? '',
      headers: Object.fromEntries((p.headers ?? []).map((h) => {
        const at = h.indexOf(':')
        return [h.slice(0, at).trim(), h.slice(at + 1).trim()]
      })),
    }))
    await loadMCP()
    // Placing it is the half that makes the need met — installing alone leaves
    // the card saying "unplaced", which from the user's side is the same button
    // having done nothing.
    if (agentMCPId) {
      const row = mcpServers.find((s) => s.name === p.name)
      if (row) {
        await SetMCPServerTargets(p.name, [...(row.for ?? []), agentMCPId])
        await loadMCP()
      }
    }
    if (agentReachFor) agentNeeds = await AgentNeeds(agentReachFor)
  })

  // Colours come from the theme, not from three hex literals. theme.css states
  // that every rule references only semantic tokens, and two of the three that
  // were here were --c-green-500 and --c-red-500 copied by value — so the dot
  // stayed dark-theme green on a light theme.
  function statusVar(status: string): string {
    if (status === 'connected') return 'background:var(--status-success)'
    if (status === 'failed') return 'background:var(--status-danger)'
    // idle is enabled-and-waiting (deferred servers sit here until their agent
    // starts), which is not the same state as disabled — but both rendered
    // --text-dim, and on a dark theme that reads as "dead", so a working
    // server looked broken. Amber says "will connect when called".
    if (status === 'idle') return 'background:var(--status-warn)'
    return 'background:var(--text-dim)'
  }

  // ---------- Skills (discovered SKILL.md + plugin install) ----------
  type SkillRow = { name: string; description: string; dir: string; bundled?: boolean }
  let extSkills = $state<SkillRow[]>([])
  // Read-only: every tool the AI can run — Aetox's own plus anything an MCP
  // server bridged in. Separate from the skills below, which are documents, not
  // things it runs.
  let tools = $state<{ name: string; description: string; source: string; category: string }[]>([])
  // Grouped by what a tool is *for*, not by where it came from.
  //
  // It used to be one card per source — builtin, workbench, mcp — which sorts
  // forty-four rows by an implementation detail and answers a question nobody
  // asks. "Which of these does the assistant need to carry everywhere?" had
  // nowhere to be asked from, because the page could not be read at all.
  //
  // The order comes from Go (internal/skill/category.go) rather than being
  // restated here, so the grouping the user sees and the grouping the engine
  // knows are one list.
  const TOOL_CATEGORIES = ['files', 'shell', 'deliverables', 'media', 'web', 'code', 'agent'] as const
  const toolGroups = $derived(
    TOOL_CATEGORIES
      .map((key) => ({ key, items: tools.filter((s) => (s.category || 'agent') === key) }))
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
  // provider is half of what a row IS, not a decoration on it: usageByModel
  // groups by model AND provider, because the same model id is sold per token
  // by one company and included in a subscription by another.
  // These used to be hand-written copies of the Go structs, and a copy of a
  // shape is a second place answering the same question: `cost`, `pricedCalls`
  // and `pricesFetched` were added to usage.go and never here, so the markup
  // below read three fields the local type said did not exist. The `as Usage`
  // cast on the call is what let it compile anyway. The generated bindings are
  // the one description of what the Go side returns; use them.
  type UsageRow = main.UsageRow
  type Usage = main.UsageStats

  let usage = $state<Usage | null>(null)
  let usageError = $state('')
  let usagePeriod = $state<'today' | 'week' | 'all'>('week')

  // null is a third answer, not an empty one: nobody has asked yet, the effect
  // below has not run, or the engine is still walking the history. It covers
  // the frame before the load starts as well as the load itself, which a
  // busy-flag set inside loadUsage would not. A reload with data already on
  // screen keeps the data — a skeleton flashed over numbers the user is
  // reading would be motion that says nothing.
  const usagePending = $derived(!usage && !usageError)

  async function loadUsage() {
    usageError = ''
    try {
      usage = await UsageStats()
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
  // Hue belongs to the model and not to the row: the chart aggregates on the
  // model alone (usageByDay groups on it), and two rows of one model are one
  // model on two bills. Deduped by name, so the five slots hold five models
  // rather than four and a repeat.
  const allModels = $derived([...new Set((usage?.all ?? []).map((r) => r.model))])
  const topModels = $derived(allModels.slice(0, 5))
  const seriesOf = $derived.by(() => {
    const map = new Map<string, number>()
    const sorted = [...topModels].sort()
    sorted.forEach((model, i) => map.set(model, i + 1))
    return map
  })
  const slotOf = (model: string) => seriesOf.get(model) ?? 0

  // A row is a (provider, model) pair, and only the pair identifies it. The
  // name stopped being an identity the day the provider column arrived (db.go
  // migration 15): every model used on both sides of that line now has two
  // rows, one under the provider that served it and one under the blank the
  // older rows carry. Keyed on the name those two are one row claimed twice,
  // and Svelte refuses a keyed list it cannot tell apart — it throws, the
  // section never renders, and what the user sees is a sidebar entry that does
  // nothing rather than a table that looks wrong.
  // The pair itself is the key, rather than the two halves glued with a
  // separator: a separator has to be a character neither half can contain, and
  // the honest candidates are control characters that do not belong typed into
  // a source file. Fourteen rows do not need the cheaper spelling.
  const rowKey = (r: UsageRow) => JSON.stringify([r.provider, r.model])

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
    path?: string; builtin: boolean; overrides?: boolean; invalid?: string; notice?: string; icon?: string
    // Already resolved by the backend (applyHomeRules fills the default), which
    // is why the editor shows this rather than the raw `desk:` it keeps: the
    // default is a constant in internal/mode and spelling it again here is how
    // the screen ends up naming a desk the engine does not use.
    desk?: string
    // What the profile declares it needs, verbatim — `mcp:firecrawl`, or an
    // alternation like `connection:n8n | mcp:windmill`. Carried so the MCP
    // shelf can say which agents asked for a server without restating it
    // (agentsNeeding); the *state* of a need is the engine's answer, not this.
    needs?: string[]
  }
  // Searching the roster. Name and description both, because half the time the
  // thing a person remembers about an agent is what it does rather than what it
  // is called. No filter control beside it: this page is already split into
  // "yours" and "built-in", which is the same question a filter would ask and
  // answers it without a click.
  let agentQuery = $state('')
  const matchesQuery = (a: SubagentRow) => {
    const q = agentQuery.trim().toLowerCase()
    if (!q) return true
    return a.name.toLowerCase().includes(q) || (a.description ?? '').toLowerCase().includes(q)
  }
  // Every profile, both kinds — kept whole because the shared editor opens
  // agents here too (the team page's doors land on it with a name). What the
  // *lists* below draw is only the sub-agents: the agents' roster is the team
  // page, and one profile on two rosters is the overlap this split ended.
  let subagents = $state<SubagentRow[]>([])
  // Split by who wrote it, which is what a user actually asks of this page. A
  // file of yours that shadows a bundled one counts as yours — it IS your file —
  // and carries a badge saying so, because deleting it reverts rather than removes.
  // One split, applied twice: which kind, then who wrote it. Both pages read
  // these — the markup is shared (profileListPane), so a rule added here lands
  // on both without either page knowing about the other.
  const teamRows = $derived({
    mine: subagents.filter((a) => !a.builtin && chairNames.has(a.name)),
    builtin: subagents.filter((a) => a.builtin && chairNames.has(a.name)),
  })
  const helperRows = $derived({
    mine: subagents.filter((a) => !a.builtin && !chairNames.has(a.name)),
    builtin: subagents.filter((a) => a.builtin && !chairNames.has(a.name)),
  })
  // Which profiles are agents — asked of ListChairs, the same answer the team
  // page draws, never re-derived from a file's fields here.
  let chairNames = $state(new Set<string>())
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
  let agentDraftIcon = $state('')
  // What the picker offers. A hand-picked subset of the app's marks, not all of
  // them: forty icons is a wall to scan and most of them mean nothing as a
  // person — these are the shapes that read as a job. Adding one is adding a
  // name here, and the value stored is that name, so an icon dropped from the
  // set later shows as the derived mark rather than as a blank square.
  const AGENT_ICONS: IconName[] = [
    'layoutList', 'fileText', 'chartColumn', 'fileCode', 'terminal', 'globe',
    'search', 'brain', 'palette', 'clapperboard', 'headphones', 'package',
    'compass', 'puzzle', 'bot',
  ]
  let agentDraftPrompt = $state('')
  // The role is the whole of what an agent is, and for a bundled one that is a
  // hundred lines of prose — opening the editor to change a model meant
  // scrolling past all of it to reach anything else on the page. So it is a
  // preview until asked for.
  //
  // Focus opens it, always: collapsed is a reading state, and the moment a
  // caret goes in, the person is writing and cannot write what they cannot see.
  // The threshold is lines rather than characters because what is being
  // measured is how far the rest of the page got pushed down.
  let agentBodyOpen = $state(false)
  const agentBodyLines = $derived(agentDraftPrompt.split('\n').length)
  const agentBodyLong = $derived(agentBodyLines > 16)
  let agentBusy = $state('')
  // The delegation switches, and the measured cost of having them on.
  //
  // Re-read after every flip rather than patched locally, because flipping one
  // re-bootstraps the engine and the block's size is the thing this page is
  // showing. A local edit would be a number that agreed with the switch and
  // disagreed with the request that gets sent.
  let delegate = $state<main.DelegateSettings | null>(null)
  let delegateBusy = $state('')
  async function loadDelegate() {
    try {
      delegate = await DelegateSwitches()
    } catch {
      delegate = null // the switches are simply absent rather than the page failing
    }
  }
  async function toggleDelegate(kind: 'agents' | 'helpers') {
    if (!delegate || delegateBusy) return
    delegateBusy = kind
    try {
      delegate = await SetDelegateOff(kind, delegate[kind].off === false)
    } finally {
      delegateBusy = ''
    }
  }
  // One worker looked up across both blocks, and its own kind decides which
  // switch grays it out. The row snippet is shared by the two pages and is
  // handed a worker rather than a page, so asking the row is the only way that
  // cannot disagree with where it was drawn.
  function reachOf(name: string): { on: boolean; off: boolean } | null {
    if (!delegate) return null
    for (const block of [delegate.agents, delegate.helpers]) {
      const w = block.workers.find((x) => x.name === name)
      if (w) return { on: w.on, off: block.off }
    }
    return null
  }
  async function toggleAgentReach(name: string, on: boolean) {
    if (delegateBusy) return
    delegateBusy = name
    try {
      delegate = await SetAgentOff(name, on)
    } finally {
      delegateBusy = ''
    }
  }
  let agentError = $state('')
  // Read from the file, shown, and written back untouched. Not `Draft` because
  // nothing here edits them — see AgentFields for why they exist at all.
  let agentKeptDesk = $state('')
  let agentKeptNeeds = $state<string[]>([])

  // ---------- What one agent reaches and knows ----------
  //
  // Three panels below the tool picker, and three different kinds of fact, which
  // is why they are three boxes rather than one list (owner, 10 ส.ค.):
  //
  //   เครื่องมือ  subtracts from the shared set — allow/deny over what every
  //              agent starts with.
  //   MCP        adds, and only to this one. A server pointed at an agent
  //              skips the profile's allow-list entirely and reaches past the
  //              desk's ceiling (internal/subagent/store.go).
  //   สกิล       adds, and only to this one. Attached *after* the filter and
  //              outside it, deliberately (internal/subagent/skills.go).
  //
  // Reading them as one box was the complaint, and it was right: the page was
  // describing one mechanism where the engine has three.
  let agentSkills = $state<main.AgentSkillInfo[]>([])
  let agentNeeds = $state<subagent.Requirement[]>([])
  // Its own memory (MEMORY.md in its folder) and its own opening (STARTERS.md),
  // both read-only here. Neither is edited on this page — memory is approved on
  // the Learning page and the opening is a file — but an agent whose page never
  // mentions them is a page that quietly claims they do not exist.
  let agentMemory = $state<string[]>([])
  let agentStarters = $state<subagent.StarterSet | null>(null)
  let agentReachFor = $state('') // whose panels are loaded, so a stale answer cannot land on the next agent

  // ---------- The opening, as a form ----------
  //
  // A growable list, floored at four and ceilinged at the pool the engine will
  // return (maxStarters in internal/subagent/starters.go).
  //
  // It was four fixed rows, for a reason that stopped being true: the window
  // drew every card a file held, so a fifth row could never appear on screen
  // and an Add button would have been a dead control. The window now deals four
  // out of a pool (starters.ts), so an agent the user hired can hold more than
  // it shows — and a form capped at four would be the one thing standing
  // between them and that. The floor stays because a pool below a full hand
  // deals a widow into the grid; the ceiling is the engine's, so the form
  // cannot accept a card that would be silently dropped on read.
  const STARTER_MIN = 4
  const STARTER_MAX = 24
  const blankStarter = () => ({ title: '', prompt: '', icon: '' })
  const blankStarters = () => Array.from({ length: STARTER_MIN }, blankStarter)
  let startersHeadline = $state('')
  let startersCards = $state(blankStarters())
  let startersFile = $state('')
  let startersBusy = $state(false)
  let startersError = $state('')
  // What was on screen when it was last read or saved, so the Save button
  // answers "is there anything to write" rather than "is this form non-empty".
  let startersSnapshot = $state('')
  // True while the cards shown are the bundled agent's rather than the user's.
  // Saving turns that into a file of your own — the same "copy it out to change
  // it" shape as AGENT.md, said before the click instead of after.
  let startersInherited = $state(false)

  const startersKey = () => JSON.stringify([startersHeadline.trim(), startersCards])
  const startersDirty = $derived(startersKey() !== startersSnapshot)
  const startersEmpty = $derived(
    startersHeadline.trim() === '' && startersCards.every((c) => !c.title.trim() && !c.prompt.trim()),
  )
  // The filename, asked of the engine rather than assembled here: which of
  // STARTERS.md / STARTERS.<lang>.md is written follows the same rule the
  // reader resolves by, and two places spelling it is two places to get it
  // wrong (config.AgentStartersName).
  const agentStartersFile = $derived(startersFile)

  function fillStarters(set: subagent.StarterSet | null) {
    startersHeadline = set?.headline ?? ''
    // The trailing space after a colon is put back by the reader and must not
    // come back through the form as an edit nobody made.
    const cards = (set?.cards ?? []).slice(0, STARTER_MAX).map((c) => ({
      title: c.title,
      prompt: (c.prompt ?? '').trimEnd(),
      icon: c.icon ?? '',
    }))
    // Always at least the floor, so an agent with two cards still shows a form
    // that can be filled to a usable pool rather than a form that ends early.
    while (cards.length < STARTER_MIN) cards.push(blankStarter())
    startersCards = cards
    startersSnapshot = startersKey()
    startersError = ''
  }

  const canAddStarter = $derived(startersCards.length < STARTER_MAX)
  function addStarterRow() {
    if (canAddStarter) startersCards = [...startersCards, blankStarter()]
  }
  // Removing below the floor is not offered rather than refused: a button that
  // is there and says no is worse than a button that is not there.
  function removeStarterRow(i: number) {
    if (startersCards.length > STARTER_MIN) {
      startersCards = startersCards.filter((_, k) => k !== i)
    } else {
      startersCards[i] = blankStarter()
    }
  }

  const saveStarters = () => runStarters(async () => {
    await SaveChairStarters(agentDraftName.trim(), i18n.locale, subagent.StarterSet.createFrom({
      headline: startersHeadline.trim(),
      cards: startersCards
        .filter((c) => c.title.trim() && c.prompt.trim())
        .map((c) => ({ title: c.title.trim(), prompt: c.prompt.trim(), icon: c.icon.trim() })),
    }))
    // Read back rather than assumed: the engine caps, trims and refuses, and
    // the form must end up showing what is now in the file.
    fillStarters(await ChairStarters(agentDraftName.trim(), i18n.locale))
    startersInherited = false
  })

  // Clearing is "I do not want my own opening" — the file goes, and whatever
  // was underneath it answers again: the shipped cards, or the ordinary four.
  const clearStarters = () => runStarters(async () => {
    await SaveChairStarters(agentDraftName.trim(), i18n.locale, subagent.StarterSet.createFrom({ headline: '', cards: [] }))
    const back = await ChairStarters(agentDraftName.trim(), i18n.locale)
    fillStarters(back)
    startersInherited = (back.cards ?? []).length > 0 || !!back.headline
  })

  async function runStarters(fn: () => Promise<void>) {
    startersBusy = true
    startersError = ''
    try {
      await fn()
    } catch (e) {
      startersError = String(e)
    } finally {
      startersBusy = false
    }
  }

  async function loadAgentReach(name: string) {
    agentReachFor = name
    agentSkills = []
    agentNeeds = []
    agentMemory = []
    agentStarters = null
    // The MCP register is the source for "which servers is this agent on", and
    // it is already loaded for its own page. Asked for here too, because the
    // editor can be the first page opened in a session.
    const [skills, needs, memory, starters, file] = await Promise.all([
      AgentSkills(name),
      AgentNeeds(name),
      LearnedEntries(name),
      ChairStarters(name, i18n.locale),
      ChairStartersFile(i18n.locale),
      mcpServers.length === 0 ? loadMCP() : Promise.resolve(),
    ])
    if (agentReachFor !== name) return // the user moved on while the disk was being read
    agentSkills = skills
    agentNeeds = needs
    agentMemory = memory
    agentStarters = starters
    startersFile = file
    // Whether these cards are already this agent's own is not a question the
    // reader can answer — a bundled agent's opening looks identical to one you
    // wrote. Only a profile with a file of its own can be showing its own.
    startersInherited = ((starters.cards ?? []).length > 0 || !!starters.headline) && agentEditing?.path === ''
    fillStarters(starters)
  }

  // Which servers this agent carries, read off the register rather than stored
  // twice: `for:` on the server is the only thing that grants (needs.go), so a
  // second list here would be a second answer to a question that has one.
  //
  // The owner id comes from PlacementTargets, never from pasting "agent:" in
  // front of a name. config.MCPAgentPrefix carries a warning about exactly that
  // — three places say the prefix, and one of them spelling it by hand is a
  // switch that silently stops matching. Empty for an agent with no file yet,
  // which is what the panel's "save first" state is for.
  const agentMCPId = $derived(
    mcpTargets.find((x) => x.kind === 'agent' && x.name === agentDraftName.trim())?.id ?? '',
  )
  // Every enabled server, not only the ones already ticked: the panel answers
  // "what does this one carry" and "what could it" in one read, and a list of
  // just the ticked ones is a list you cannot add to.
  const agentServerCandidates = $derived(mcpServers.filter((s) => !s.disabled))
  const agentServerCount = $derived(
    agentMCPId ? agentServerCandidates.filter((s) => (s.for ?? []).includes(agentMCPId)).length : 0,
  )

  // Toggling here writes the same `for:` list the MCP page writes, through the
  // same call. It applies at once and does not wait for Save — the panel says
  // so, because a switch inside a form with a Save button is otherwise read as
  // part of the draft.
  const toggleAgentServer = (s: MCPRow) => runMCP('target:' + s.name + ':' + agentMCPId, async () => {
    if (!agentMCPId) return
    const current = s.for ?? []
    const next = current.includes(agentMCPId)
      ? current.filter((x) => x !== agentMCPId)
      : [...current, agentMCPId]
    await SetMCPServerTargets(s.name, next)
    await loadMCP()
    // A need met by that click stops being unmet — recomputed rather than
    // guessed at, since only the engine knows what counts as met.
    if (agentReachFor) agentNeeds = await AgentNeeds(agentReachFor)
  })

  // The per-row model dropdown offers the current provider's models — a pin to a
  // model from some other provider still shows (as its own option) rather than
  // silently reading as "inherit".
  let agentModels = $state<string[]>([])

  async function loadAgents() {
    subagents = await ListSubagentProfiles()
    // Asked, not worked out from the profile's `desk`. Which profiles sit in
    // the office is decided in one place (ListChairs), and a second reading of
    // the same rule here is a second answer waiting to disagree with the page
    // that actually draws the roster.
    try {
      chairNames = new Set((await ListChairs()).map((c) => c.name))
    } catch {
      chairNames = new Set() // engine not up: rows just carry no room label
    }
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
    agentDraftTools, agentDraftDeny, agentDraftSteps, agentDraftIcon, agentDraftPrompt,
  ])
  let agentSnapshot = ''

  // Which kind the editor is holding — เอเจน or ซับเอเจน. Set by the door
  // the editor was opened through and never by reading the file: the same rule
  // the storage layer lives by (a file's home is its kind), carried up. Saving
  // goes out through the matching door, so an edit cannot change what
  // something is as a side effect of where a button happened to be.
  let agentEditKind = $state<'agent' | 'helper'>('helper')

  const openAgent = (a: SubagentRow, kind?: 'agent' | 'helper') => runAgent('open:' + a.name, async () => {
    const parsed = parseAgentFile(await ReadSubagentProfile(a.name))
    agentDraftName = a.name
    agentDraftDescription = parsed.description
    agentDraftModel = parsed.model
    agentDraftTools = parsed.tools
    agentDraftDeny = parsed.deny
    // No `steps:` line means no ceiling (§110), so the box opens ticked. Seeded
    // rather than inferred from a blank field, so clearing the number to type a
    // new one does not disable the field under the user's cursor.
    agentDraftSteps = parsed.steps.trim() || STEPS_UNLIMITED
    agentDraftIcon = parsed.icon
    agentKeptDesk = parsed.desk
    agentKeptNeeds = parsed.needs
    agentDraftPrompt = parsed.body
    // Every open starts collapsed, including the second open of the same agent:
    // the state belongs to this reading of the page, not to the file.
    agentBodyOpen = false
    agentEditing = a
    // A row opened from this page answers from the roster the page already
    // asked for (ListChairs) — never from the file's own fields.
    agentEditKind = kind ?? (chairNames.has(a.name) ? 'agent' : 'helper')
    agentSnapshot = agentDraftKey()
    // The three panels that describe an agent's reach and knowledge. Fetched
    // after the fields are in, and not awaited by the editor: a slow disk scan
    // must not hold up the form the user came to type in.
    if (agentEditKind === 'agent') void loadAgentReach(a.name)
  })

  function newAgent(kind: 'agent' | 'helper' = 'helper') {
    agentEditing = { name: '', description: '', prompt: '', builtin: false }
    agentDraftName = ''
    agentDraftDescription = ''
    agentDraftModel = ''
    agentDraftTools = []
    agentDraftDeny = []
    agentDraftSteps = STEPS_UNLIMITED // a new worker starts uncapped, like every shipped one
    agentDraftIcon = ''
    agentKeptDesk = ''
    agentKeptNeeds = []
    agentDraftPrompt = t('settings.agentStarter')
    agentBodyOpen = true // a new agent is opened to be written in, not read
    agentError = ''
    agentEditKind = kind
    agentSkills = []
    agentNeeds = []
    agentSnapshot = agentDraftKey()
  }

  const closeAgentEditor = () =>
    guardUnsaved(agentDraftKey() !== agentSnapshot, () => { agentEditing = null })

  const saveAgent = () => runAgent('save', async () => {
    const body = serializeAgentFile({
      description: agentDraftDescription,
      model: agentDraftModel,
      tools: agentDraftTools,
      deny: agentDraftDeny,
      steps: agentDraftSteps,
      icon: agentDraftIcon,
      desk: agentKeptDesk,
      needs: agentKeptNeeds,
      body: agentDraftPrompt,
    })
    // Out through the door that matches the kind — the backend refuses a name
    // the other kind owns, so the wrong door is an error message, never a file
    // in the wrong home.
    if (agentEditKind === 'agent') await SaveAgentProfile(agentDraftName.trim(), body)
    else await SaveSubagentProfile(agentDraftName.trim(), body)
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
  const AGENT_FORCED_DENIALS = ['task', 'task_result', 'task_answer', 'task_plan', 'help', 'ask_user', 'todo_write']
  // Mirrors subagent.stepsUnlimitedKeyword. The frontmatter carries a word
  // rather than a sentinel number because the file is hand-editable.
  const STEPS_UNLIMITED = 'unlimited'

  // `desk` and `needs` are here to be *kept*, not to be edited. Neither had a
  // field, and neither survived a save: opening github or automation and
  // pressing Save wrote a shadow with `needs:` gone, so the agent quietly
  // stopped declaring what it cannot work without and the notice it carries in
  // its own prompt (subagent.PromptFor) went with it. `desk:` was the same
  // silent loss with a worse ending — an agent on a named desk fell back to the
  // office ceiling. An editor must not delete what it does not draw.
  type AgentFields = {
    description: string; model: string; tools: string[]; deny: string[]; steps: string; icon: string
    desk: string; needs: string[]; body: string
  }

  // Mirrors internal/subagent/profile.go's parse(): a leading `---`-fenced block
  // of `key: value` lines, then the role prompt underneath. Duplicated here
  // rather than asked of the backend because this is purely a display choice —
  // the file format itself has not changed, so there's nothing to add to the
  // Go side for it. Falls back to treating the whole thing as the prompt when
  // there's no recognizable frontmatter, so a hand-edited or malformed file is
  // never silently emptied under the user.
  function parseAgentFile(raw: string): AgentFields {
    const asPromptOnly = {
      description: '', model: '', tools: [] as string[], deny: [] as string[],
      steps: '', icon: '', desk: '', needs: [] as string[], body: raw.trim(),
    }
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
      icon: (fields.icon ?? '').trim(),
      desk: (fields.desk ?? '').trim().toLowerCase(),
      // Not lowercased and not split on anything but the comma: an entry may
      // carry alternatives ("connection:n8n | connection:windmill"), which the
      // engine splits on `|` itself (subagent.alternatives). Touching the
      // inside of an entry here would be this editor deciding something the
      // author wrote down.
      needs: (fields.needs ?? '').split(',').map((s) => s.trim()).filter(Boolean),
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
    // Written back exactly as they were read. The editor shows both and edits
    // neither: what an agent requires and which desk it sits at are the
    // author's statements about the job, and a form that cannot express them
    // must at least not swallow them.
    if (f.desk) lines.push(`desk: ${f.desk}`)
    if (f.needs.length) lines.push(`needs: ${f.needs.join(', ')}`)
    // The mark this agent wears on the roster. Written only when the user chose
    // one: an absent field means the roster derives it from what the agent
    // makes, which is the right answer for every profile nobody has opened.
    if (f.icon.trim()) lines.push(`icon: ${f.icon.trim()}`)
    // The keyword, not a number. Leaving the line out would mean the same thing
    // today (the default is no ceiling since §110), but writing it says so in
    // the file, where the next person to read it is looking.
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

  // Grouped by what the tool is for, same split the Tools page uses. Choosing
  // what a delegate may be handed is exactly the "what is this for" question —
  // "which of these did I install" never was, and it is the one the old
  // source-based split answered.
  const agentToolGroups = $derived.by(() => {
    const q = toolQuery.trim().toLowerCase()
    return TOOL_CATEGORIES
      .map((key) => ({
        key,
        items: tools
          .filter((s) => (s.category || 'agent') === key && (!q || s.name.toLowerCase().includes(q)))
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

  // Ticking remembers nothing, unticking leaves an empty box for a number: the
  // field is disabled while unlimited is on, so whatever was in it is not
  // something the user can see or was looking at. An empty box saves no
  // `steps:` line at all, which is the same "no ceiling" by a quieter route.
  function toggleStepsUnlimited() {
    agentDraftSteps = agentStepsUnlimited ? '' : STEPS_UNLIMITED
  }

  // The MCP page needs them too, though it draws no agent: each preset says
  // which agents asked for it, and that is read from the profiles' own `needs:`
  // (agentsNeeding). Without this the line is simply missing on the one page
  // where somebody is deciding whether a server is for them.
  $effect(() => {
    if (active === 'agents' || active === 'team' || active === 'mcp') void loadAgents()
    // Both pages, since each now carries its own switch: 'team' is เอเจน and
    // 'agents' is ซับเอเจน (see the render below). Loading on one page only is
    // how the ซับเอเจน page ended up with rows it could not explain.
    if (active === 'team' || active === 'agents') void loadDelegate()
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
  // The decided list is a record, not a queue: nothing is waiting on it and the
  // reason it is kept at all is so "why does it think that?" can be answered
  // months later. Twenty rows of it sat open above everything else on this page
  // (owner, 2026-08-14: "ตอนนี้มันรกเกิน"), which is a lot of screen for
  // something nobody came here to read.
  //
  // Four, then a count. Same shape the sidebar's project groups already use —
  // preview, then a button — and the count is there because "ดูเพิ่มเติม" on its
  // own does not say whether it hides two rows or two hundred.
  const DECIDED_PREVIEW = 4
  let decidedExpanded = $state(false)
  let learningError = $state('')
  let learningBusy = $state(0)
  // One row per remembered line. The file is a bullet list and always was —
  // showing it as one <pre> was the app describing the file rather than being
  // a way into it, so the only way to fix a line was to go and open the folder.
  // One group per scope that holds anything: the main agent's file, each desk's,
  // each project's. It was the main agent's alone until a desk and a project
  // could be the destination — and a line approved into a file this page could
  // not show is a line only the folder knows about.
  let memoryGroups = $state<{ scope: string; lines: string[]; orphan: boolean }[]>([])
  // The projects the store still knows, for the orphan group's ย้ายไปที่…
  // picker. Loaded with the memory list because the two are one question:
  // which of these files can a session still arrive at, and where else could
  // an orphaned one go.
  let knownProjects = $state<{ name: string; rootPath: string }[]>([])
  // Which orphan group has its move picker open (by scope), '' for none.
  let adoptOpen = $state('')
  let memoryScopeError = $state('')
  // Which row is open for editing, and in which scope. -1 for none. One at a
  // time: these lines are short, and a page of open textareas is a form nobody
  // knows the state of. The scope rides along because the same index exists in
  // every group, and the save has to reach the right file.
  let memoryEditing = $state(-1)
  let memoryEditingScope = $state('')
  let memoryDraft = $state('')
  let memorySaving = $state(false)

  // The other queue, and deliberately not one variable of the block above:
  // nothing here is a proposal, nothing here can be approved, and nothing here
  // ends up in a file. Sharing state would be the same conflation this split
  // exists to undo (docs/architecture/system-problems-vs-learning-2026-08-18.md).
  let systemIssues = $state<main.PendingChange[]>([])
  let decidedIssues = $state<main.PendingChange[]>([])
  let issuesExpanded = $state(false)
  let issuesError = $state('')
  let issuesBusy = $state(0)

  async function loadIssues() {
    try {
      issuesError = ''
      systemIssues = await ListSystemIssues()
      decidedIssues = await ListDecidedIssues(20)
    } catch (err) {
      issuesError = String(err)
    }
  }

  // Take one problem to the assistant instead of to the developer.
  //
  // Whose fault a repeated failure is — this machine, Aetox, or the agent's own
  // way of calling the tool — is exactly what the user is being asked to judge
  // with nothing to go on, and it is a question the assistant can go and answer.
  // So the message names all three possibilities rather than asserting one.
  //
  // It goes in as the user's own visible message (owner's requirement, and the
  // honest shape): the problem is the first thing in the new chat, in words they
  // can read and edit the follow-up to. Nothing is decided by asking — the row
  // stays waiting, because the answer may send it either way.
  //
  // The evidence row ids are deliberately left out. They are for the GitHub
  // form, where somebody can query the database; in a chat they are noise, and
  // the agent can find the runs from the sentence itself (session_search reads
  // tool_runs).
  function consultPrompt(c: main.PendingChange): string {
    return t('settings.issuesConsultPrompt', { body: c.body, reason: c.reason })
  }

  async function consultIssue(c: main.PendingChange) {
    onClose()
    await startChatWith(consultPrompt(c))
  }

  // The three extension pages — skills, MCP, ชุดคำสั่ง — all ask the same thing
  // of the user, and it is the thing a settings page cannot ask: *what do you
  // do?* Every road they offer today (a GitHub URL, a .zip, a server address, an
  // empty editor) requires the user to already know what exists, which is the
  // real reason the shelf looks bare on a fresh install. A button that starts a
  // conversation is the only door that does not go stale.
  //
  // One function, three prompts, and the prompts live in the locale file because
  // what is about to be said on the user's behalf should be readable by somebody
  // who is not reading the code. Their VALUES are English in every locale (see
  // the note there): the label is for the user, the sentence is for the model.
  //
  // onClose() first, exactly as consultIssue does: the new chat is the answer, so
  // leaving the user on the settings page to discover it would be the wrong
  // ending.
  async function askAssistant(promptKey: 'settings.aiFindSkillPrompt' | 'settings.aiFindMCPPrompt' | 'settings.aiFindPresetPrompt') {
    onClose()
    await startChatWith(t(promptKey))
  }

  // Reporting is the About page's door with this cluster written into the body:
  // same URL builder, same prefill, same "the user reads the whole thing on
  // GitHub and presses send themselves". A second door would be a second
  // privacy story to keep true, and this one is already written down.
  //
  // Marked reported after the form opens, never before — and marked even though
  // the user may read it and close the tab. What this side can honestly record
  // is that the problem was carried out the door, which is exactly what stops
  // it sitting here asking again.
  async function reportIssue(c: main.PendingChange) {
    issuesBusy = c.id
    try {
      issuesError = ''
      await openIssueForm('problem', c)
      await MarkIssueReported(c.id)
      await loadIssues()
    } catch (err) {
      issuesError = String(err)
    } finally {
      issuesBusy = 0
    }
  }

  async function dismissIssue(id: number) {
    issuesBusy = id
    try {
      issuesError = ''
      await RejectPendingChange(id)
      await loadIssues()
    } catch (err) {
      issuesError = String(err)
    } finally {
      issuesBusy = 0
    }
  }

  async function loadLearning() {
    try {
      learningError = ''
      learningOn = await LearningEnabled()
      pendingChanges = await ListPendingChanges()
      decidedChanges = await ListDecidedChanges(20)
      const scopes = await LearnedScopeInfos()
      memoryGroups = await Promise.all(
        scopes.map(async ({ scope, orphan }) => ({ scope, orphan, lines: await LearnedEntries(scope) })),
      )
      // Offered as move targets only when an orphan needs one — but loaded
      // here so the picker opens filled rather than after a spinner.
      if (memoryGroups.some((g) => g.orphan)) {
        knownProjects = (await RecentProjects()).map((p: { name: string; rootPath: string }) => ({ name: p.name, rootPath: p.rootPath }))
      }
    } catch (err) {
      learningError = String(err)
    }
  }

  async function adoptScope(scope: string, rootPath: string) {
    memoryScopeError = ''
    try {
      await AdoptMemoryScope(scope, rootPath)
      adoptOpen = ''
      await loadLearning()
    } catch (err) {
      memoryScopeError = String(err)
    }
  }

  async function forgetScope(scope: string) {
    memoryScopeError = ''
    try {
      await ForgetMemoryScope(scope)
      await loadLearning()
    } catch (err) {
      memoryScopeError = String(err)
    }
  }

  function startMemoryEdit(scope: string, i: number) {
    memoryEditing = i
    memoryEditingScope = scope
    memoryDraft = memoryGroups.find((g) => g.scope === scope)?.lines[i] ?? ''
  }

  function isEditing(scope: string, i: number): boolean {
    return memoryEditing === i && memoryEditingScope === scope
  }

  function cancelMemoryEdit() {
    memoryEditing = -1
    memoryEditingScope = ''
    memoryDraft = ''
  }

  // Saving and forgetting are the same write with a different body — an empty
  // one removes the line (learned.EditEntry). Keeping them one call means the
  // two paths cannot drift on which row they address.
  async function commitMemory(scope: string, index: number, text: string) {
    memorySaving = true
    try {
      learningError = ''
      await SaveLearnedEntry(scope, index, text)
      cancelMemoryEdit()
      // Re-read rather than patching the array: the row positions the next edit
      // sends have to be the file's, and a delete moves every line below it.
      await loadLearning()
    } catch (err) {
      learningError = String(err)
    } finally {
      memorySaving = false
    }
  }

  function onMemoryKeydown(e: KeyboardEvent, scope: string, index: number) {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation() // Escape also closes Settings; one press closes one layer
      cancelMemoryEdit()
      return
    }
    // Enter saves, Shift+Enter breaks the line. A remembered line is one
    // sentence, so the common key does the common thing.
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (memoryDraft.trim()) void commitMemory(scope, index, memoryDraft)
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
  // named one is a sub-agent, a desk or a project, and saying which matters —
  // it is the difference between "everything you ask it", "one job it does",
  // and "only in this folder". Shared with the card in the chat (memoryScope):
  // the same proposal is judged in both places and must not be labelled two
  // different ways.

  $effect(() => {
    // untracked, because loadConnections reads the state it writes — the
    // placement list and the per-card drafts. Without this the effect depends
    // on its own output and runs a second time the moment the first load
    // lands, which is one wasted round trip per open and a race between two
    // in-flight loads for which one gets to set `connections`.
    if (active === 'connections') untrack(() => void loadConnections())
  })

  $effect(() => {
    if (active === 'learning') void loadLearning()
  })

  $effect(() => {
    // Reads from disk only, so opening the page costs nothing and works with
    // no network. Asking the server is the separate "check again" button.
    if (active === 'account') void loadAetoxAccount()
  })

  // Asked once, before any page is drawn, because the answer decides whether
  // the account page is in the nav at all. False in every shipped build today:
  // nothing is deployed for it to talk to, and a settings page that offers a
  // sign-in reaching nothing is the placeholder the 1.0.0 bar forbids.
  $effect(() => {
    void loadAetoxAccount()
  })

  $effect(() => {
    if (active === 'issues') void loadIssues()
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
        terms: [t('settings.shellTitle'), t('settings.approvalTitle'), t('settings.firstRunTitle')] },
      { id: 'appearance', label: t('settings.appearance'), icon: 'palette',
        terms: [
          t('settings.languageTitle'), t('settings.themeTitle'), t('settings.uiFontTitle'),
          t('settings.typeScaleTitle'), t('settings.systemZoomTitle'), t('settings.editorFontTitle'),
          t('settings.chatFontTitle'), t('settings.treeFontTitle'), t('settings.codeThemeTitle'),
        ] },
      // The icon is deliberately not `userRound` — the เอเจน page below owns
      // that, and this page is not about a person in the team.
      { id: 'identity', label: t('settings.identity'), icon: 'fileText',
        terms: ['identity.md', 'thinking.md', 'context.md', 'skills.md'] },
      // Next to identity, because they answer the same question from two
      // sides: what the user told the agent, and what the agent worked out.
      { id: 'learning', label: t('settings.learning'), icon: 'brain',
        terms: [t('settings.learningPending'), t('settings.learningMemory')] },
      // Next to learning because that is where these used to arrive, and the
      // adjacency is the point: what Aetox worked out about you, and what keeps
      // going wrong, are two different things that spent a year in one queue
      // (docs/architecture/system-problems-vs-learning-2026-08-18.md).
      { id: 'issues', label: t('settings.issues'), icon: 'alertTriangle',
        terms: [t('settings.issuesReport'), t('settings.aboutReport')] },
    ]},
    { group: t('settings.groupModels'), items: [
      { id: 'models', label: t('settings.modelSettings'), icon: 'brain',
        terms: [t('settings.providers'), t('settings.apiKeyLabel'), t('settings.baseUrl'), t('settings.signInLabel'), t('settings.modelList')] },
      { id: 'team', label: t('settings.team'), icon: 'userRound',
        terms: [t('settings.teamNew'), t('settings.agentConfigure'), t('desk.office')] },
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
      // Below MCP and not beside the model sign-ins: both pages here extend
      // what the agent can reach, which is the question a user arrives with.
      // The icon is deliberately not `plug` — MCP owns that, and two plugs in
      // one group is a list you have to read twice.
      // One page for everything the agent connects to, accounts and self-run
      // engines alike (owner, 19 ส.ค. — reversing the 10 ส.ค. split). The
      // automation engines' search terms moved here with them: somebody typing
      // "n8n" is looking for the page it is now on, and a term left pointing at
      // a page that no longer exists is a search box that lies.
      { id: 'connections', label: t('settings.connections'), icon: 'globe',
        terms: ['GitHub', t('settings.ghTokenLabel'), 'n8n', 'Windmill', t('settings.connBaseURLLabel')] },
      { id: 'prompts', label: t('settings.prompts'), icon: 'sparkles', terms: [t('settings.promptNew')] },
    ]},
    { group: t('settings.groupAbout'), items: [
      // First in this group and nowhere near the model sign-ins: those decide
      // which account pays for a request, this one is who you are to Aetox.
      // It is in About rather than up top because it configures nothing the
      // app does today.
      // Left out entirely when this build has no id server, which is every
      // shipped build today. Not greyed out and not marked "soon": there is
      // nothing behind it yet, and a row that opens onto nothing is worse than
      // a row that is not there.
      ...(aetoxAccount?.configured
        ? [{ id: 'account', label: t('settings.account'), icon: 'circleUser',
             terms: ['GitHub', 'Google', t('settings.accountSignOut')] } satisfies NavItem]
        : []),
      // Usage lives here rather than under Tools: it is a report about the app,
      // not a thing to configure, which is the same kind of page as About.
      { id: 'usage', label: t('settings.usage'), icon: 'chartColumn',
        terms: [t('settings.usageByModel'), t('settings.usageTotalTokens'), t('settings.usageCacheHitRate')] },
      { id: 'about', label: t('settings.about'), icon: 'package',
        terms: [t('settings.aboutVersion'), t('settings.aboutCheck'), t('settings.aboutReport'), t('settings.aboutFeedback')] },
      { id: 'sponsor', label: t('settings.sponsor'), icon: 'heart', terms: ['PromptPay', 'GitHub'] },
    ]},
  ])

  const SPONSOR_URL = 'https://github.com/Mikedev115/Aetox/blob/main/SPONSOR.md'
  // The repository, not the marketing site: the site is one page now, and the
  // place a supporter actually wants to land — the code, the releases, the
  // issues, the name on the commits — is here.
  const SITE_URL = 'https://github.com/Mikedev115/Aetox'
  // The one link that is right on every channel, whether or not a check ran.
  const RELEASES_URL = 'https://github.com/Mikedev115/Aetox/releases'
  const ISSUES_URL = 'https://github.com/Mikedev115/Aetox/issues'

  // What the user has to say about Aetox goes to the developer — as a GitHub
  // issue the user submits, not as anything the app sends. Two rows, one
  // destination: a problem report and plain feedback differ only in the hint
  // they open with, because a suggestion forced through a bug template arrives
  // apologising for not being a bug. The URL prefills the new-issue form with
  // the two facts the user should not have to hunt for (version, OS);
  // everything else is theirs to write, on a page where they read the whole
  // message before pressing send, signed in as themselves. This is the whole
  // privacy story: the last reader before anything leaves the machine is the
  // person it belongs to.
  //
  // Deliberately NOT the learning loop's door. That queue is for lessons about
  // this user and this machine; "Aetox is broken" and "I wish Aetox did X" are
  // facts about the product, and the two kinds of report must never share a
  // path (see summarize.go on state reports, the third kind, which goes
  // nowhere at all).
  // `cluster` is one row from the problems room: the same form, opened with the
  // failure already written into it. It goes above the blank lines, not below
  // the ---, because it is the subject of the report and the user is about to
  // write around it. Everything else on this path is unchanged, which is the
  // point — one door, one prefill, one story about what leaves the machine.
  async function openIssueForm(kind: 'problem' | 'feedback', cluster?: main.PendingChange): Promise<void> {
    const ua = navigator.userAgent
    const os = ua.includes('Windows') ? 'Windows' : ua.includes('Mac') ? 'macOS' : 'Linux'
    const version = (appVersion ? 'v' + appVersion : t('settings.aboutReportUnknown'))
      + (updateStatus?.channel ? ` (${updateStatus.channel})` : '')
    const hint = kind === 'problem' ? t('settings.aboutReportBodyHint') : t('settings.aboutFeedbackBodyHint')
    const lines = [`<!-- ${hint} -->`]
    if (cluster) {
      lines.push('', cluster.body)
      if (cluster.reason) lines.push('', cluster.reason)
      if (cluster.evidence) lines.push('', cluster.evidence)
    }
    lines.push(
      '', '', '---',
      `${t('settings.aboutReportVersion')}: ${version}`,
      `${t('settings.aboutReportOS')}: ${os}`,
    )
    if (kind === 'problem') {
      // The evidence: what the app most recently complained about internally
      // (already secret-scrubbed at the moment each line was written). Only
      // the problem door carries it — feedback needs no logs. Trimmed from
      // the OLD end when over budget: a prefill URL has a length limit, and
      // the newest lines are the ones about whatever just went wrong. Folded
      // in <details> and labelled deletable, because it is the user's form.
      try {
        let log = (await RecentDebugLog()) ?? []
        while (log.length > 0 && !issueURLFits(lines, log)) log = log.slice(1)
        const text = log.join('\n')
        if (text) {
          lines.push('', `<details><summary>${t('settings.aboutReportLogTitle')}</summary>`, '', '```', text, '```', '</details>')
        }
      } catch {
        // No log is not a reason to block a report.
      }
    }
    // Trimmed from the end if the report itself is over budget even with no log
    // at all. A cluster body plus a stack trace can do it, and the alternative
    // is the button doing nothing, which is what it did before.
    let body = lines.join('\n')
    while (body.length > 0 && issueURL(body).length > ISSUE_URL_BUDGET) {
      body = body.slice(0, -256)
    }
    BrowserOpenURL(issueURL(body))
  }

  // encodeURIComponent leaves !~*'() alone — they are "unreserved marks" it is
  // specified not to touch. Wails refuses to open a URL containing any of
  // !~*() as a shell-metacharacter risk (ValidateAndSanitizeURL), and it
  // refuses by logging to a place no user sees and returning, so the button
  // did nothing and said nothing. One ordinary parenthesis was enough, and the
  // problem bodies are full of them: "เกิด 3 ครั้ง (ตัวอย่างล่าสุด...)".
  //
  // ' is encoded too. Wails does not object to it, but a body that is
  // percent-encoded except for one character is a rule with an exception to
  // remember, and there is nothing to gain by keeping it readable in a URL.
  const encodeIssueBody = (s: string) =>
    encodeURIComponent(s).replace(/[!~*'()]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase())

  const issueURL = (body: string) => `${ISSUES_URL}/new?body=${encodeIssueBody(body)}`

  // ISSUE_URL_BUDGET is the whole URL, encoded, and it exists because the old
  // budget counted the wrong thing and the button silently did nothing.
  //
  // The cap used to be 4000 characters of raw log text. Thai is three bytes per
  // character in UTF-8 and each byte becomes three characters again once
  // percent-encoded, so a 4000-character Thai log is ~36,000 characters of URL.
  // Windows caps a command line at 32,767, and `BrowserOpenURL` hands the URL to
  // `rundll32` — over that it fails, and Wails logs the failure to a place no
  // user sees rather than returning it, so pressing แจ้งปัญหานี้ produced no
  // window, no error, nothing (measured 2026-08-18: 4,251 raw characters became
  // a 35,659-character URL).
  //
  // 8000 rather than anything near the OS ceiling: GitHub itself stops honouring
  // very long prefill URLs well before Windows stops accepting them, and a
  // report that opens with the body quietly cut is worse than a shorter log.
  const ISSUE_URL_BUDGET = 8000

  function issueURLFits(head: string[], log: string[]): boolean {
    const body = [...head, '', '<details><summary>x</summary>', '', '```', log.join('\n'), '```', '</details>'].join('\n')
    return issueURL(body).length <= ISSUE_URL_BUDGET
  }

  // Which page is open survives an F5. Same reasoning as the chat/settings view
  // itself (see setActiveView in stores/cockpit.svelte.ts): sessionStorage, not
  // localStorage, because reopening the app should always start from the top —
  // but a reload during a run should not throw away where you were. Reloading
  // while three pages deep into MCP config and landing back on General is a
  // small thing that happens every single time.
  // Imported rather than spelled again: a room can send the user straight to a
  // section (openSettingsAt), and two spellings of this key would fail silently
  // and look like the page ignoring where it was told to go.
  const SECTION_KEY = SETTINGS_SECTION_KEY
  const SECTION_IDS = new Set(['general', 'appearance', 'identity', 'learning', 'models', 'team', 'agents', 'tools', 'skills', 'mcp', 'connections', 'prompts', 'account', 'usage', 'about', 'sponsor'])

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

<!-- Starting a self-hosted engine, and asking whether it is up.
     Two questions this page could not answer before, and both belong to it: a
     row that says "not connected" was unable to do the one thing that fixes
     that, and telling somebody to go and find a terminal from the screen they
     are already on is the complaint this closes.

     Deliberately separate from ตรวจสอบ. That button asks whether the KEY works;
     these ask whether the PROGRAM is running. Told apart they are two obvious
     fixes; run together they were one confusing failure, because a dead server
     and a dead key both come back as "could not connect".

     The command is the user's own, typed once and remembered. Nothing about
     where n8n or Windmill lives is written in this codebase — a guess would be
     wrong for everyone it was not written for — and the precedent is an MCP
     stdio server, which has always been a command in a config file. -->
<!-- The offer that makes an extension page usable by somebody who does not
     already know what exists.

     Its own card at the top of the page rather than a third button in the row
     with เปิดโฟลเดอร์ and รีเฟรช: those are janitorial, and no amount of colour
     makes a hero out of the third item in a utility row. DESIGN.md §1 —
     ยืมโครงได้ ห้ามยืมเครื่องประดับ — so this is the ordinary set-row shape and
     the ordinary .ctrl-primary, with position and copy doing the work.

     The button says what pressing it does (opens a chat), not what it hopes will
     happen. Nothing is installed by pressing it. -->
{#snippet aiFindCard(titleKey: TKey, descKey: TKey, promptKey: 'settings.aiFindSkillPrompt' | 'settings.aiFindMCPPrompt' | 'settings.aiFindPresetPrompt')}
  <div class="settings-card">
    <div class="set-row set-hero">
      <span class="set-hero-ic"><Icon name="sparkles" size={18} /></span>
      <div class="set-txt">
        <div class="t">{t(titleKey)}</div>
        <div class="d">{t(descKey)}</div>
      </div>
      <button class="ctrl ctrl-primary ctrl-icon" onclick={() => askAssistant(promptKey)}>
        <Icon name="messageSquare" size={13} />
        {t('settings.aiFind')}
      </button>
    </div>
  </div>
{/snippet}

{#snippet serverControls(row: ConnectionRow)}
  <!-- Its own bordered block, and the heading says which of the two questions
       this half answers. They were a run of fields under the address and the
       owner could not tell them apart from the credential check below — which
       is fair, because "ตรวจสอบ" and "เช็คว่าขึ้นหรือยัง" side by side in one
       column read as two spellings of one button. -->
  <div class="conn-part">
    <div class="conn-part-head">
      <Icon name="monitor" size={13} />
      <span>{t('settings.connServerPart')}</span>
    </div>
    <div class="d muted">{t('settings.connServerPartHint')}</div>

    <div class="eyebrow conn-eyebrow">{t('settings.connStartLabel')}</div>
    <div class="mset-keyrow">
      <input
        class="ctrl key-input" type="text" autocomplete="off" spellcheck="false"
        placeholder={t('settings.connStartPlaceholder')}
        value={connStart[row.id] ?? ''}
        oninput={(e) => (connStart[row.id] = e.currentTarget.value)}
        onblur={() => saveStartCommand(row)}
      />
      <button
        class="ctrl"
        disabled={srvBusy !== '' || !(connStart[row.id] ?? '').trim()}
        onclick={() => startServer(row)}
      >
        {srvBusy === row.id + ':start' ? t('settings.connStarting') : t('settings.connStart')}
      </button>
      <button class="ctrl" disabled={srvBusy !== ''} onclick={() => checkServer(row)}>
        {srvBusy === row.id + ':check' ? t('settings.connChecking') : t('settings.connCheck')}
      </button>
    </div>
    <div class="d muted">{t('settings.connStartHint')}</div>
    {#if srvState[row.id]}
      <div class="conn-test" class:ok={srvState[row.id].startsWith('ok:')}>
        {#if srvState[row.id].startsWith('ok:')}
          <Icon name="check" size={13} /> {srvState[row.id].slice(3)}
        {:else}
          {srvState[row.id].slice(4)}
        {/if}
      </div>
    {/if}
  </div>
{/snippet}

<!-- ซับเอเจน only. The two pages used to share this markup, and the sharing was
     right while both were lists of files; they stopped being the same kind of
     thing the moment เอเจน became people you pick by face (agentCard above).
     A helper is not picked at all — it is part of the system, nobody chooses
     one, and a row is the honest shape for an inventory you read and close. -->
{#snippet profileRow(a: SubagentRow)}
  <div class="set-row">
    <!-- The same mark this profile wears everywhere else, resolved once in Go
         so one profile cannot show two faces on two pages. Untinted: the
         colour is an agent's own, and a helper does not have one.

         The fallback is the KIND's mark, never a constant: `|| 'bot'` put the
         ซับเอเจน glyph on three of the five bundled เอเจน, on the เอเจน page,
         under a sidebar item wearing `userRound` (see workerFace). -->
    <span class="ag-rowicon" style="--h:{coverHue(a.name)}">
      <Icon name={workerFace(a.icon, chairNames.has(a.name))} size={15} />
    </span>
    <div class="set-txt">
      <div class="t">
        {a.name}
        {#if a.model}<span class="tag">{a.model}</span>{/if}
        <span class="tag" title={toolBadgeTip(a)}>{toolBadge(a)}</span>
        {#if a.deny && a.deny.length > 0}<span class="tag ag-deny" title={denyTip(a)}>{t('settings.agentDenyCount', { n: a.deny.length })}</span>{/if}
        <!-- 0 is an absent `steps:` and a negative is the keyword; both mean no
             ceiling (§110). Only a positive number is a cap worth a badge. -->
        <span class="tag" title={(a.steps ?? 0) > 0 ? t('settings.agentStepsTip', { n: a.steps ?? 0 }) : t('settings.agentStepsNoneTip')}>
          {(a.steps ?? 0) > 0 ? t('settings.agentSteps', { n: a.steps ?? 0 }) : t('settings.agentStepsNone')}
        </span>
      </div>
      <div class="d">{a.description || '—'}</div>
      <!-- A file that cannot run says why, where its owner will look — never a
           silent reinterpretation, never a row that just vanishes (the file is
           still on the user's disk). -->
      {#if a.invalid}<div class="d ag-invalid">{a.invalid}</div>{/if}
      <!-- Not fatal, and deliberately a different colour: this file runs, it is
           just doing something its author probably did not mean. -->
      {#if a.notice}<div class="d ag-notice">{a.notice}</div>{/if}
      <div class="d mono-dim">{a.path || 'built-in:' + a.name}</div>
    </div>
  </div>
{/snippet}

<!-- A teammate is a row again (owner, 12 ส.ค.), and this reverses the 10 ส.ค.
     call that made it a card. Both had a point and the row is written to keep
     the card's: what was wrong with the ORIGINAL row is that the name sat in a
     line of five grey tags, so there was nothing to recognise anyone by; what a
     grid of 240px cards then cost is that a team no longer fits on a screen,
     and each card spent a border, a colour band and a foot saying what a
     divider says for free.
     So the name leads, at the size the card gave it, with its face beside it
     and the tags trailing behind; the description gets its own line; and the
     two settings you change without opening anything sit on the right, out of
     the column the eye scans. The roster on the team page keeps the cards —
     there you are picking a person to talk to, here you are finding a file to
     configure, and those are different acts (§85).

     The file path stays in the name's tooltip, as it has since the card: on a
     row it was a fourth line of dim monospace under every entry, answering a
     question nobody asks while scanning. -->
{#snippet agentRow(a: SubagentRow)}
  <div class="set-row ag-row">
    <span class="ag-rowicon" style="--h:{coverHue(a.name)}">
      <Icon name={workerFace(a.icon, chairNames.has(a.name))} size={16} />
    </span>
    <div class="set-txt">
      <div class="t" title={a.path || 'built-in:' + a.name}>
        {a.name}
        {#if a.overrides}<span class="tag ag-override">{t('settings.agentOverrides')}</span>{/if}
        <span class="tag" title={toolBadgeTip(a)}>{toolBadge(a)}</span>
        {#if a.deny && a.deny.length > 0}<span class="tag ag-deny" title={denyTip(a)}>{t('settings.agentDenyCount', { n: a.deny.length })}</span>{/if}
        <!-- 0 is an absent `steps:` and a negative is the keyword; both mean no
             ceiling (§110). Only a positive number is a cap worth a badge. -->
        <span class="tag" title={(a.steps ?? 0) > 0 ? t('settings.agentStepsTip', { n: a.steps ?? 0 }) : t('settings.agentStepsNoneTip')}>
          {(a.steps ?? 0) > 0 ? t('settings.agentSteps', { n: a.steps ?? 0 }) : t('settings.agentStepsNone')}
        </span>
      </div>
      <div class="d">{a.description || '—'}</div>
      <!-- A file that cannot run says why, where its owner will look — never a
           silent reinterpretation, never a row that just vanishes (the file is
           still on the user's disk). -->
      {#if a.invalid}<div class="d ag-invalid">{a.invalid}</div>{/if}
      <!-- Not fatal, and deliberately a different colour: this file runs, it is
           just doing something its author probably did not mean. -->
      {#if a.notice}<div class="d ag-notice">{a.notice}</div>{/if}
    </div>
    <div class="ag-actions">
      <!-- Whether the MAIN assistant may hand this one work. Not whether the
           agent exists: the user still opens a chat with it from the composer
           and still writes @name, and no switch on this page reaches those. That
           is what the tooltip says, and why the control carries no "เปิด/ปิด"
           wording of its own — "off" would read as "gone" while the agent is
           standing right there.

           The app's switch (style.css .mswitch), in the same shape the MCP shelf
           uses one row below: a row whose left half is a name and a description,
           and whose right half is the switch followed by the row's action
           buttons. It was a `.ctrl` chip that lit up when on, which is a state
           drawn as a button — the thing the owner sent back twice on 2026-08-20.
           A chip that is on and a chip that is merely hoverable look the same
           until you learn the colour; a switch does not have to be learned.

           Disabled, not hidden, while this kind is switched off entirely. A row
           that vanished would leave somebody wondering where their agent went; a
           row that is greyed out under the switch above it explains itself. -->
      {#if delegate}
        {@const w = reachOf(a.name)}
        {#if w}
          <label class="mswitch" title={t('settings.agentReachTip')}>
            <input
              type="checkbox" checked={w.on && !w.off}
              disabled={w.off || delegateBusy !== ''}
              aria-label={t('settings.agentReach')}
              onchange={() => toggleAgentReach(a.name, w.on)}
            />
            <span></span>
          </label>
        {/if}
      {/if}
      <select
        class="ctrl ag-model" value={a.model ?? ''} disabled={agentBusy !== ''}
        aria-label={t('settings.agentModelPick')}
        onchange={(e) => pinModel(a.name, e.currentTarget.value)}
      >
        <option value="">{t('settings.agentModelInherit')}</option>
        {#each agentModels as m}<option value={m}>{m}</option>{/each}
        {#if a.model && !agentModels.includes(a.model)}<option value={a.model}>{a.model}</option>{/if}
      </select>
      <button
        class="icobtn tiny tip-l" disabled={agentBusy !== ''}
        aria-label={t('settings.agentConfigure')} data-tip={t('settings.agentConfigure')}
        onclick={() => openAgent(a, 'agent')}
      >
        <Icon name="settings" size={13} />
      </button>
    </div>
  </div>
{/snippet}

{#snippet profileListPane(kind: 'agent' | 'helper')}
  {@const isAgent = kind === 'agent'}
  {@const rows = isAgent ? teamRows : helperRows}
  <h2>{isAgent ? t('settings.team') : t('settings.subagents')}</h2>
  <p class="muted set-sub">{isAgent ? t('settings.teamDesc') : t('settings.subagentsDesc')}</p>

  {#if isAgent}
    <div class="pp-bar">
      <button class="ctrl" onclick={() => newAgent(kind)}>{t('settings.teamNew')}</button>
      <button class="ctrl" onclick={() => loadAgents()}>{t('settings.refresh')}</button>
      <button class="ctrl" onclick={() => OpenAgentsFolder()}>{t('settings.agentsFolder')}</button>
      <!-- The roster with its job history and its chat doors is a page of its
           own; this one configures. Said out loud rather than left for the user
           to discover, because two places holding one kind of thing is exactly
           what needs a stated rule. -->
      <div class="pp-bar-gap"></div>
      <!-- The door goes with the room. เอเจนเฉพาะทาง is the storefront's, and
           Settings can be opened from either door — landing on the page without
           moving the door would draw one door's room inside the other's
           sidebar. -->
      <button class="ctrl" onclick={() => { setShell('assistant'); setActiveView('office') }}>{t('settings.teamOpenPage')} <Icon name="arrowRight" size={13} /></button>
    </div>
  {/if}
  {#if agentError}<div class="mset-error">{agentError}</div>{/if}

  <!-- This page's own switch. Delegation ships off, so this page is where
       somebody turns it on.

       Drawn on BOTH pages since 2026-08-20. It used to be `{#if isAgent}`, one
       switch on the เอเจน page governing both kinds — so somebody who opened
       the ซับเอเจน page while it was off found every row greyed out and nothing
       on the page explaining who had done it.

       **The token figures came off this row the same day, on the owner's call:
       "เลขตรงนี้เหมือนจะบั๊ค ๆ … เปิดไม่เปิดก็พอละ".** They were not wrong and
       that is what made them worse. Each switch showed its MARGINAL cost — what
       flipping it changes with the other switch left alone — so the number on
       this row moved when you touched the OTHER one, and swung sevenfold doing
       it: with เอเจน off, turning ซับเอเจน off removes the whole `task` tool
       (~599) where it would otherwise remove ~81. A figure that is honest,
       unpredictable from where the user is standing, and sitting next to a
       second figure with a different meaning (the whole block) reads as broken.
       A switch nobody can predict is worse than a switch with no number on it.

       If a cost belongs anywhere it is one number in one place, on a page about
       what the assistant is carrying — not a per-switch figure the reader has
       to hold two switches in their head to interpret. -->
  {#if delegate}
    {@const side = isAgent
      ? { kind: 'agents' as const, reach: delegate.agents, label: 'settings.delegateAgents' as const, on: 'settings.delegateAgentsOn' as const, off: 'settings.delegateAgentsOff' as const }
      : { kind: 'helpers' as const, reach: delegate.helpers, label: 'settings.delegateHelpers' as const, on: 'settings.delegateHelpersOn' as const, off: 'settings.delegateHelpersOff' as const }}
    <div class="settings-card reach-card">
      <div class="set-row">
        <div class="set-txt">
          <div class="t">{t(side.label)}</div>
          <div class="d">
            {t(side.reach.off ? side.off : side.on, { n: side.reach.tokens.toLocaleString() })}
          </div>
        </div>
        <div class="ag-actions">
          <!-- The app's switch (style.css .mswitch), the same one the learning
               page and the MCP shelf wear. It was a `.ctrl` button reading
               "เปิด"/"ปิด" for an afternoon, which is the one control shape this
               settings page does not use for an on/off state: a button labelled
               "ปิด" is read twice, once as "it is off" and once as "press to
               turn it off", and which one it means depends on knowing the
               convention. A switch is the state and the control at once, and
               the owner asked for the standard one (20 ส.ค.).

               A real checkbox inside a label, not a role="switch" button: this
               row is not itself a button, so the checkbox drives the face and
               brings the keyboard and the screen reader with it for free. -->
          <label class="mswitch">
            <input
              type="checkbox" checked={!side.reach.off} disabled={delegateBusy !== ''}
              aria-label={t(side.label)}
              onchange={() => toggleDelegate(side.kind)}
            />
            <span></span>
          </label>
        </div>
      </div>
    </div>
  {/if}

  <!-- Drawn once there is enough to search. A box over three rows is furniture
       that explains nothing; over thirty it is the only way back to the one you
       meant. -->
  {#if rows.mine.length + rows.builtin.length > 6}
    <label class="ag-search">
      <Icon name="search" size={13} />
      <input bind:value={agentQuery} placeholder={t('settings.agentSearch')} />
    </label>
  {/if}

  {#if isAgent}
    <!-- Two grids, split by who wrote it — the question this page is actually
         asked. Built-ins are second because a fresh install has only those and
         the interesting list is the one you grow.
         The group heading is a bare label rather than a card wrapping the grid:
         a card around cards is a box around boxes, and the border did nothing
         the gap between the two groups was not already saying. -->
    {#each [{ id: 'mine', rows: rows.mine.filter(matchesQuery), label: t('settings.subagentsMine'), hint: t('settings.teamMineHint') },
            { id: 'builtin', rows: rows.builtin.filter(matchesQuery), label: t('settings.subagentsBuiltin'), hint: t('settings.subagentsBuiltinHint') }] as group (group.id)}
      <!-- Count beside the title, the sentence pushed to the right margin: the
           left edge is the column being scanned, and a hint under the heading
           put prose between every group and its first name. -->
      <div class="group-head">
        <span class="group-title">{group.label}</span>
        <span class="group-count">{group.rows.length}</span>
        <span class="group-hint">{group.hint}</span>
      </div>
      <div class="settings-card">
        {#each group.rows as a (a.name)}{@render agentRow(a)}{/each}
        {#if group.rows.length === 0}
          <div class="set-row ag-row empty"><div class="set-txt"><div class="d">
            {agentQuery.trim() ? t('settings.agentNoMatches') : t('settings.teamNoneOfMine')}
          </div></div></div>
        {/if}
      </div>
    {/each}
    <p class="muted set-sub">{t('settings.agentsHint')}</p>
  {:else}
    <!-- The helpers are part of the system (owner's call, 2026-08-06): the
         bundled three are the whole set, so this page reads — no create, no
         editor, no per-row model pin. One card, because "yours" cannot exist. -->
    <div class="settings-card">
      <div class="card-form">
        <div class="eyebrow">{t('settings.subagentsBuiltin')} <span class="ag-count">{rows.builtin.length}</span></div>
        <div class="d muted">{t('settings.helpersFixedHint')}</div>
      </div>
      {#each rows.builtin as a (a.name)}{@render profileRow(a)}{/each}
    </div>
  {/if}
{/snippet}

<!-- One editor for both kinds. Which kind it is holding was decided by the
     door it was opened through (agentEditKind), never by reading the file —
     that is the same rule the storage layer lives by, carried up. -->
{#snippet agentEditorPane()}
  {#if agentEditing !== null}
    <h2>{agentEditKind === 'agent' ? t('settings.editAgentTitle') : t('settings.subagents')}</h2>
    <p class="muted set-sub">{agentEditKind === 'agent' ? t('settings.editAgentDesc') : t('settings.subagentsDesc')}</p>

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

    <!-- Five groups, in the order the questions get asked: who is this, what
         does it think with, what can it reach, what does it know, how does it
         open. Before this the page was one long card and half the answers were
         on other pages entirely. -->
    <div class="ag-sec">{t('settings.agentSecIdentity')}</div>
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
        <!-- The face this agent wears on the roster. A row of the app's own
             marks rather than a text field: the value is a name from a fixed
             set, and a field where a typo silently means "no icon" is a field
             that fails without saying so. Choosing none is a real choice and
             the default — the roster then derives one from what the agent
             makes, which is right for every profile nobody has opened. -->
        <div class="pp-field">
          <span class="eyebrow">{t('settings.agentIcon')}</span>
          <div class="ag-icons">
            <button type="button" class="ag-icon" class:on={agentDraftIcon === ''}
              title={t('settings.agentIconAuto')} onclick={() => (agentDraftIcon = '')}>
              <Icon name="sparkles" size={16} />
            </button>
            {#each AGENT_ICONS as name (name)}
              <button type="button" class="ag-icon" class:on={agentDraftIcon === name}
                title={name} onclick={() => (agentDraftIcon = name)}>
                <Icon {name} size={16} />
              </button>
            {/each}
          </div>
          <span class="d muted">{agentDraftIcon === '' ? t('settings.agentIconAutoHint') : agentDraftIcon}</span>
        </div>
        <!-- A div rather than the `label` every other field uses: the toggle is
             a button, and a button inside a label puts the caret in the textarea
             on every click of it. -->
        <div class="pp-field">
          <div class="ag-bodyhead">
            <span class="eyebrow">{t('settings.agentBody')}</span>
            {#if agentBodyLong}
              <button type="button" class="ag-bodymore" onclick={() => (agentBodyOpen = !agentBodyOpen)}>
                {agentBodyOpen ? t('settings.agentBodyLess') : t('settings.agentBodyMore', { n: agentBodyLines })}
              </button>
            {/if}
          </div>
          <div class="ag-bodywrap" class:collapsed={agentBodyLong && !agentBodyOpen}>
            <textarea
              class="ctrl ag-body" bind:value={agentDraftPrompt} spellcheck="false"
              use:autogrow={agentDraftPrompt}
              onfocus={() => (agentBodyOpen = true)}
            ></textarea>
          </div>
          <span class="d muted">{t('settings.agentBodyHint')}</span>
        </div>
      </div>
    </div>

    <!-- ── สมอง ── which model answers, and how long it may go on for. Two
         settings, one question, and the model one used to be answerable only
         from the card behind this page: a page called "configure the agent"
         that could not configure the agent's model. -->
    <div class="ag-sec">{t('settings.agentSecBrain')}</div>
    <div class="settings-card">
      <div class="card-form pp-edit">
        <label class="pp-field">
          <span class="eyebrow">{t('settings.agentModelPick')}</span>
          <select class="ctrl" bind:value={agentDraftModel}>
            <option value="">{t('settings.agentModelInherit')}</option>
            {#each agentModels as m}<option value={m}>{m}</option>{/each}
            {#if agentDraftModel && !agentModels.includes(agentDraftModel)}
              <option value={agentDraftModel}>{agentDraftModel}</option>
            {/if}
          </select>
          <span class="d muted">{t('settings.agentModelHint')}</span>
        </label>

        <div class="pp-field">
          <span class="eyebrow">{t('settings.agentStepsField')}</span>
          <div class="ag-steprow">
            <input
              class="ctrl ag-steps" bind:value={agentDraftSteps} inputmode="numeric" placeholder="40"
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

    <!-- ── เอื้อมถึงอะไร ── the group this whole restructure exists for. Read
         top to bottom it is the engine's own order: the ceiling the desk sets,
         then what is subtracted from the shared set, then what is added for
         this one alone. Three mechanisms, three boxes. -->
    <div class="ag-sec">{t('settings.agentSecReach')}</div>
    {#if agentEditKind === 'agent' && agentEditing.name}
      <!-- The ceiling, stated rather than editable. A desk is named in the file
           and defaults to the office; what matters on screen is that the reader
           knows a ceiling exists at all — ticking a tool the desk will never
           hand over is otherwise a switch that does nothing and says nothing.
           Not drawn while creating: the desk is resolved by the backend when
           the file lands, and printing a guess at it here would be this page
           inventing an answer it does not have. -->
      <div class="settings-card">
        <div class="set-row">
          <span class="ag-rowicon"><Icon name="package" size={15} /></span>
          <div class="set-txt">
            <div class="t">{t('settings.agentDeskTitle')} <span class="tag">{agentEditing.desk || agentKeptDesk || '—'}</span></div>
            <div class="d">{t('settings.agentDeskHint')}</div>
          </div>
        </div>
      </div>
    {/if}

    <div class="settings-card">
      <div class="card-form pp-edit">
        <!-- A summary and a way in, not seventy chips. What each tool ends up
             as is one question, so it is answered one row at a time in the
             picker rather than across two grids the reader has to join
             themselves. -->
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
      </div>
    </div>

    {#if agentEditKind === 'agent'}
      {@render agentMCPBox()}
      {@render agentNeedsBox()}

      <!-- Everything below reads a folder that does not exist until the file
           is saved. Four empty boxes are a worse first impression of a new
           agent than four boxes that appear once there is something to put in
           them. -->
      {#if agentEditing.name}
        <div class="ag-sec">{t('settings.agentSecKnowledge')}</div>
        {@render agentSkillsBox()}
        {@render agentMemoryBox()}

        <div class="ag-sec">{t('settings.agentSecOpening')}</div>
        {@render agentStartersBox()}
      {/if}
    {/if}
  {/if}
{/snippet}

<!-- The servers pointed at this one agent. A box of its own, beside เครื่องมือ
     rather than inside it, because it is the opposite operation: `for:` on a
     server ADDS, skipping this profile's allow-list and reaching past the
     desk's ceiling (internal/subagent/store.go). Reading the two as one list
     was the complaint, and the complaint was a true statement about the code. -->
{#snippet agentMCPBox()}
  <div class="settings-card">
    <div class="card-form">
      <div class="eyebrow">
        {t('settings.agentMCPTitle')}
        <span class="ag-count">{agentServerCount}</span>
      </div>
      <div class="d muted">{t('settings.agentMCPHint')}</div>
    </div>
    {#if !agentMCPId}
      <!-- A server's `for:` names an agent, so there is nothing to point at
           until the file exists. Said plainly instead of drawing switches that
           would silently write nothing. -->
      <div class="set-row"><div class="muted">{t('settings.agentMCPSaveFirst')}</div></div>
    {:else if agentServerCandidates.length === 0}
      <div class="set-row">
        <div class="set-txt"><div class="d">{t('settings.agentMCPNone')}</div></div>
        <button class="ctrl" onclick={() => openSection('mcp')}>{t('settings.mcpServers')} <Icon name="arrowRight" size={13} /></button>
      </div>
    {:else}
      {#each agentServerCandidates as s (s.name)}
        <label class="set-row ag-reachrow">
          <input
            type="checkbox" checked={(s.for ?? []).includes(agentMCPId)}
            disabled={mcpBusy !== ''} onchange={() => toggleAgentServer(s)}
          />
          <div class="set-txt">
            <div class="t">{s.name}{#if s.tools > 0}<span class="tag">{t('settings.mcpToolCount', { n: String(s.tools) })}</span>{/if}</div>
            <div class="d">{s.url || (s.command ?? []).join(' ')}</div>
          </div>
        </label>
      {/each}
      <div class="set-row"><div class="d muted">{t('settings.agentMCPInstant')}</div></div>
    {/if}
  </div>
{/snippet}

<!-- What the agent said it cannot work without, and where each of those stands.
     The engine has computed this since needs.go was written and only ever
     folded it into the agent's own prompt — so an agent that could not work
     said so in the chat, while the page you fix it on showed nothing.

     One row per *requirement*, not per thing. `needs: connection:n8n |
     connection:windmill` is one requirement — an automation engine — and either
     answers it. Drawn as two flat rows it read as "n8n is required" beside a
     second demand for a product the user had deliberately not installed. -->
{#snippet agentNeedsBox()}
  {#if agentKeptNeeds.length > 0}
    {@const unmet = agentNeeds.filter((r) => !r.met).length}
    <div class="settings-card">
      <div class="card-form">
        <div class="eyebrow">
          {t('settings.agentNeedsTitle')}
          {#if unmet > 0}<span class="ag-count ag-count-warn">{unmet}</span>{/if}
        </div>
        <div class="d muted">{t('settings.agentNeedsHint')}</div>
      </div>
      {#each agentNeeds as req (req.entry)}
        {@const options = req.options ?? []}
        <div class="ag-need" class:met={req.met}>
          <!-- The requirement's own line. With one option it is that option's
               name; with more it says the choice out loud, because "either of
               these" is the fact the flat list was losing. -->
          <div class="ag-need-head">
            <span class="ag-rowicon" class:ag-rowicon-warn={!req.met}>
              <Icon name={req.met ? 'check' : (options[0]?.kind === 'connection' ? 'globe' : 'plug')} size={15} />
            </span>
            <div class="set-txt">
              <div class="t">
                {options.length > 1
                  ? options.map((o) => o.label).join(' / ')
                  : (options[0]?.label ?? req.entry)}
              </div>
              <div class="d">
                {#if req.met}{t('settings.agentNeedMet')}
                {:else if options.length > 1}{t('settings.agentNeedEitherOf')}
                {:else}{t(`settings.agentNeedReason_${options[0]?.reason ?? 'unknown'}` as TKey)}{/if}
              </div>
            </div>
            {#if !req.met && options.length === 1}
              {@render needDoor(options[0])}
            {/if}
          </div>

          <!-- With a choice, each way of answering it gets its own line and its
               own state — "อันไหนเปิดอยู่ก็บอกว่าเปิด". The door beside each
               goes where THAT one is switched on, which a single shared button
               could never do. -->
          {#if options.length > 1}
            {#each options as o (o.kind + ':' + o.id)}
              <div class="ag-need-opt">
                <span class="ag-need-dot" class:on={!o.reason}></span>
                <div class="set-txt">
                  <div class="t">{o.label}</div>
                  <div class="d">
                    {o.reason ? t(`settings.agentNeedReason_${o.reason}` as TKey) : t('settings.agentNeedOptionOn')}
                  </div>
                </div>
                {#if o.reason}{@render needDoor(o)}{/if}
              </div>
            {/each}
          {/if}
        </div>
      {/each}
    </div>
  {/if}
{/snippet}

<!-- Where one unmet option is actually switched on. A server this agent is not
     placed on is fixed in the MCP box a few rows up, so that one says so
     instead of sending the user to a page they just came from. -->
{#snippet needDoor(o: subagent.Need)}
  {#if o.kind === 'mcp' && o.reason === 'unplaced'}
    <span class="d muted ag-need-here">{t('settings.agentNeedFixHere')}</span>
  {:else if o.kind === 'mcp' && o.reason === 'missing' && presetFor(o.id)}
    <!-- The server this agent was written against, installed and placed from
         here. Anywhere else and the user has to know which of seven presets
         belongs to which agent, which is the thing they cannot know. -->
    <button class="ctrl ctrl-primary ctrl-icon" disabled={mcpBusy !== ''} onclick={() => installNeeded(o)}>
      {mcpBusy === 'need:' + o.id ? t('settings.agentNeedInstalling') : t('settings.agentNeedInstall')}
    </button>
  {:else}
    <button class="ctrl" onclick={() => openSection(o.kind === 'connection' ? 'connections' : 'mcp')}>
      {o.kind === 'connection' ? t('settings.agentNeedConnect') : t('settings.agentNeedServer')}
      <Icon name="arrowRight" size={13} />
    </button>
  {/if}
{/snippet}

<!-- Its own shelf. Reads and does not edit, because a skill is a folder: the
     honest control is the one that opens it. -->
{#snippet agentSkillsBox()}
  <div class="settings-card">
    <div class="card-form">
      <div class="eyebrow">{t('settings.agentSkillsTitle')} <span class="ag-count">{agentSkills.length}</span></div>
      <div class="d muted">{t('settings.agentSkillsHint')}</div>
    </div>
    {#each agentSkills as s (s.name)}
      <div class="set-row">
        <span class="ag-rowicon"><Icon name="puzzle" size={15} /></span>
        <div class="set-txt">
          <div class="t">{s.name}{#if s.bundled}<span class="tag">{t('settings.agentSkillBundled')}</span>{/if}</div>
          <div class="d">{s.description || '—'}</div>
        </div>
      </div>
    {/each}
    {#if agentSkills.length === 0}
      <div class="set-row"><div class="muted">{t('settings.agentSkillsNone')}</div></div>
    {/if}
    <div class="set-row">
      <div class="set-txt"><div class="d muted">{t('settings.agentSkillsFolderHint')}</div></div>
      <button class="ctrl" disabled={!agentEditing?.name} onclick={() => OpenAgentSkillsFolder(agentDraftName.trim())}>
        {t('settings.agentSkillsOpenFolder')}
      </button>
    </div>
  </div>
{/snippet}

<!-- Memory is approved on the Learning page and lives in this agent's folder.
     Shown here as a count and a door: moving the approval flow would be a
     second place to approve, which is the one thing it must not become. -->
{#snippet agentMemoryBox()}
  <div class="settings-card">
    <div class="set-row">
      <span class="ag-rowicon"><Icon name="brain" size={15} /></span>
      <div class="set-txt">
        <div class="t">{t('settings.agentMemoryTitle')} <span class="tag">{t('settings.itemCount', { n: agentMemory.length })}</span></div>
        <div class="d">{agentMemory.length > 0 ? agentMemory[0] : t('settings.agentMemoryNone')}</div>
      </div>
      <button class="ctrl" onclick={() => openSection('learning')}>
        {t('settings.learning')} <Icon name="arrowRight" size={13} />
      </button>
    </div>
  </div>
{/snippet}

<!-- How this agent opens a conversation (STARTERS.md in its folder), edited
     here rather than only in a text editor (owner, 10 ส.ค.).
     A form over the file, not a replacement for it: hand-editing stays exactly
     as valid, which is why what Save writes is a heading and a list and
     nothing a person would not have typed themselves.
     A growable list. It was four fixed rows while the window drew every card a
     file held; now the window deals four out of a pool, so the form has to be
     able to build a pool deeper than the grid. Floored at four so nothing can
     be saved that deals a widow, ceilinged at what the engine will read back. -->
{#snippet agentStartersBox()}
  <div class="settings-card">
    <div class="card-form pp-edit">
      <div class="pp-field">
        <div class="pp-bodyhead">
          <span class="eyebrow eyebrow-grow">{t('settings.agentStartersTitle')}</span>
          <span class="d muted mono-dim">{agentStartersFile}</span>
        </div>
        <div class="d muted">{t('settings.agentStartersHint')}</div>
      </div>

      <label class="pp-field">
        <span class="eyebrow">{t('settings.agentStartersHeadline')}</span>
        <input class="ctrl" bind:value={startersHeadline} placeholder={t('settings.agentStartersHeadlinePlaceholder')} />
      </label>

      {#each startersCards as card, i (i)}
        <div class="pp-field ag-starter">
          <div class="ag-starter-head">
            <span class="eyebrow eyebrow-grow">{t('settings.agentStarterCard', { n: i + 1 })}</span>
            <button
              class="ag-starter-drop"
              title={t('settings.agentStarterRemove')}
              aria-label={t('settings.agentStarterRemove')}
              onclick={() => removeStarterRow(i)}
            >
              <Icon name="x" size={13} />
            </button>
          </div>
          <div class="ag-starter-row">
            <input class="ctrl" bind:value={card.title} placeholder={t('settings.agentStarterTitlePlaceholder')} />
            <select class="ctrl ag-starter-icon" bind:value={card.icon} aria-label={t('settings.agentIcon')}>
              <option value="">{t('settings.agentStarterNoIcon')}</option>
              {#each AGENT_ICONS as name (name)}<option value={name}>{name}</option>{/each}
            </select>
          </div>
          <input class="ctrl" bind:value={card.prompt} placeholder={t('settings.agentStarterPromptPlaceholder')} />
        </div>
      {/each}

      {#if canAddStarter}
        <button class="ctrl ag-starter-add" onclick={addStarterRow}>
          <Icon name="plus" size={13} />
          <span>{t('settings.agentStarterAdd')}</span>
        </button>
      {/if}

      <!-- The trailing-colon rule is the author's, not a quirk to discover: a
           prompt that ends in ":" is the deliberate half-sentence the user
           finishes in the composer. The pool line beside it is the other thing
           an author cannot see from here: the grid draws four of these. -->
      <div class="d muted">{t('settings.agentStartersColonHint')}</div>
      <div class="d muted">{t('settings.agentStartersPoolHint', { shown: 4, held: startersCards.length })}</div>

      {#if startersError}<div class="mset-error">{startersError}</div>{/if}

      <div class="pp-bar">
        <span class="d muted">
          {startersInherited ? t('settings.agentStartersInherited') : t('settings.agentStartersOwn')}
        </span>
        <div class="pp-bar-gap"></div>
        <button class="ctrl" disabled={startersBusy || startersEmpty} onclick={clearStarters}>
          {t('settings.agentStartersClear')}
        </button>
        <button class="ctrl ctrl-primary" disabled={startersBusy || !startersDirty} onclick={saveStarters}>
          {startersBusy ? t('settings.saving') : t('settings.agentStartersSave')}
        </button>
      </div>
    </div>
  </div>
{/snippet}

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
          <!-- The same mark, and only here: the gear in the sidebar stays the
               learning queue's alone. A problem is worth finding when you come
               looking and is not worth being pulled out of a chat for. -->
          {#if it.id === 'issues' && cockpit.pendingIssues > 0}
            <span class="nav-count" title={t('settings.issuesWaiting', { count: String(cockpit.pendingIssues) })}>
              {cockpit.pendingIssues}
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
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.firstRunTitle')}</div>
            <div class="d">{t('settings.firstRunDesc')}</div>
          </div>
          <button class="ctrl" onclick={replayFirstRun}>{t('settings.firstRunAction')}</button>
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
                <!-- Green only once the engine has said so. Unknown and not
                     ready look different from each other and neither looks
                     like ready. -->
                <span
                  class="dot" class:green={p.ready === true} class:unknown={p.ready === null}
                  title={p.ready === null
                    ? t('settings.providerChecking')
                    : p.ready ? t('settings.providerReady') : t('settings.providerNotReady')}
                ></span>
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

            {#if account}
              <div class="mset-acct">
                <ProviderAccount {account} />
                {#if account.balance?.hasAmount}
                  <button class="ctrl tiny" disabled={busy === 'account'} onclick={refreshAccount}>
                    <Icon name="refreshCw" size={13} /> {t('settings.refreshBalance')}
                  </button>
                {/if}
              </div>
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

            {#if selectedRow.requiresKey && selectedRow.acceptsKey}
              <div class="mset-field">
                <div class="eyebrow eyebrow-row">
                  <span>{signInMethod ? t('settings.signInOrKey') : t('settings.apiKeyLabel')}</span>
                  <!-- The card asks for a key; every provider hides the page
                       that issues one somewhere different. Drawn only when the
                       catalog knows a page for this row. -->
                  {#if keyPageURL}
                    <button class="keylink" onclick={() => BrowserOpenURL(keyPageURL)}>
                      {t('settings.getKey')}<Icon name="externalLink" size={12} />
                    </button>
                  {/if}
                </div>
                <div class="mset-keyrow">
                  <input
                    class="ctrl key-input" type={showKey ? 'text' : 'password'}
                    placeholder={selectedRow.hasKey
                      ? (selectedRow.keyHint
                          ? t('settings.keySetHintPlaceholder', { hint: selectedRow.keyHint })
                          : t('settings.keySetPlaceholder'))
                      : t('settings.pasteKeyPlaceholder')}
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
                <!-- Only where the list is long enough to be worth searching.
                     Six rows do not need a filter above them. -->
                {#if models.length > 8}
                  <div class="mlist-tools">
                    <input
                      class="ctrl mlist-search" type="search"
                      placeholder={t('settings.filterModels', { n: String(models.length) })}
                      bind:value={modelFilter}
                    />
                    {#if freeCount > 0}
                      <button
                        class="conn-chip" class:on={freeOnly}
                        aria-pressed={freeOnly} onclick={() => (freeOnly = !freeOnly)}
                      >{t('settings.freeOnly')} {freeCount}</button>
                    {/if}
                  </div>
                {/if}
                {#if visibleModels.length === 0}
                  <div class="muted">{t('settings.noModelsMatch')}</div>
                {/if}
                {#each visibleModels as m}
                  <div class="mrow">
                    <span class="mname">{m}</span>
                    <!-- What the row costs, or a dash. Never a zero: on a list
                         this long a zero reads as "free" and the user would
                         act on it, and 84% coverage means the other 16% are
                         genuinely unknown rather than cheap. -->
                    {#if priced[m]?.free}
                      <span class="mprice free">{t('settings.priceFree')}</span>
                    {:else if priced[m]?.priced}
                      <span class="mprice" title={t('settings.pricePerMillion')}>
                        ${priced[m].input} / ${priced[m].output}
                      </span>
                    {:else}
                      <span class="mprice dim">—</span>
                    {/if}
                    <button
                      class="icobtn tiny" title={t('settings.testConnection')} aria-label={t('settings.testConnection')}
                      disabled={connTesting[m]} onclick={() => testConnection(m)}
                    >{#if connTesting[m]}…{:else}<Icon name="plugZap" size={14} />{/if}</button>
                    {#if isActiveProvider && cockpit.model.modelName === m}
                      <span class="badge on">{t('settings.inUse')}</span>
                    {:else}
                      <button class="ctrl" disabled={busy !== ''} onclick={() => useModel(m)}>
                        {busy === m ? t('settings.switching') : t('settings.use')}
                      </button>
                    {/if}
                  </div>
                  {#if connResult[m]}
                    <div class="conn-test" class:ok={connResult[m].startsWith('ok:')}>
                      {#if connResult[m].startsWith('ok:')}
                        <Icon name="check" size={13} /> {t('settings.connOk')}: {connResult[m].slice(3)}
                      {:else}
                        <Icon name="x" size={13} /> {connResult[m].slice(4)}
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
          <!-- Template literal, not concatenation: TOOL_CATEGORIES is a literal
               union, so this resolves to a real message key and a category added
               without its label becomes a compile error. -->
          <span class="group-title">{t(`settings.toolGroup_${g.key}`)}</span>
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

      {@render aiFindCard('settings.aiFindSkillTitle', 'settings.aiFindSkillDesc', 'settings.aiFindSkillPrompt')}

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
          <!-- Keyed by name, not dir: a bundled skill has no folder, so dir is
               empty and would collide the moment a second one ships. Names are
               unique across the merged list by construction — a user folder of
               the same name replaces the bundled entry rather than joining it. -->
          {#each extSkills as s (s.name)}
            <div class="set-row">
              <div class="set-txt">
                <div class="t">{s.name}</div>
                <div class="d">{s.description || '—'}</div>
                <div class="d mono-dim">{s.bundled ? t('settings.skillBundled') : s.dir}</div>
              </div>
              {#if !s.bundled}
                <button class="ctrl ctrl-danger" disabled={skillBusy !== ''} onclick={() => removeSkill(s.name, s.dir)}>
                  {t('settings.remove')}
                </button>
              {/if}
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

    {:else if active === 'team' || active === 'agents'}
      {@const kind = active === 'team' ? 'agent' : 'helper'}
      {#if agentEditing !== null}
        {@render agentEditorPane()}
      {:else}
        {@render profileListPane(kind)}
      {/if}

    {:else if active === 'prompts'}
      <h2>{t('settings.prompts')}</h2>
      <p class="muted set-sub">{t('settings.promptsDesc')}</p>

      {#if editing === null}
        {@render aiFindCard('settings.aiFindPresetTitle', 'settings.aiFindPresetDesc', 'settings.aiFindPresetPrompt')}
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
      <h2>{t('settings.identity')}</h2>
      <p class="muted set-sub">{t('settings.identityDesc')}</p>
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
              <div class="empty">{t('settings.noIdentityFiles')}</div>
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
              class="identity-newfile-input" placeholder={t('settings.newIdentityFile')}
              bind:value={newIdentityName}
              onkeydown={(e) => e.key === 'Enter' && addIdentityFile()}
            />
            <button type="button" class="icobtn tiny" aria-label={t('settings.newIdentityFile')} onclick={addIdentityFile}><Icon name="plus" size={14} /></button>
          </div>
          {#if identity.activeName}
            <textarea
              class="identity-input" placeholder={t('settings.identityPlaceholder')}
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

      <!-- .set-row, like every other switch on this page. This card used to be
           built from .mcp-row/.mcp-row-main, which have no CSS at all — so the
           text sat flush against the card's border and the switch dropped to
           its own line underneath. -->
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.learningEnabled')}</div>
            <div class="d">{t('settings.learningEnabledHint')}</div>
          </div>
          <label class="mswitch">
            <input type="checkbox" checked={learningOn} onchange={toggleLearning} />
            <span></span>
          </label>
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
      <!-- The button used to be a bare child of the card, which has no padding
           of its own — so it sat hard against the left border while the memory
           text above it was inset by 16px. Its own row puts the two on one
           left edge and gives the button the same rule every other card
           footer has. -->
      <div class="settings-card">
        {#each memoryGroups as group (group.scope)}
          <!-- Whose file this block is. Drawn even when there is only one, so
               "ผู้ช่วยหลัก" is stated rather than assumed: once a line can land
               in a desk's or a project's file instead, an unlabelled list is a
               list you cannot act on. -->
          <div class="mem-scope">
            {scopeLabel(group.scope)}
            {#if group.orphan}
              <!-- The folder this file is keyed to moved or was deleted, so no
                   session can ever read it again — a fact only this label
                   states, because on disk the file looks exactly like a live
                   one (§186). A label needs its exits: move the lines to the
                   project the folder became, or let them go. -->
              <span class="mem-orphan">{t('settings.memoryOrphan')}</span>
              <span class="mem-orphan-actions">
                {#if knownProjects.length > 0}
                  <button
                    type="button" class="ctrl tiny"
                    onclick={() => { adoptOpen = adoptOpen === group.scope ? '' : group.scope }}
                  >{t('settings.memoryOrphanMove')}</button>
                {/if}
                <button
                  type="button" class="ctrl tiny mem-forget"
                  onclick={() => forgetScope(group.scope)}
                >{t('settings.memoryOrphanDelete')}</button>
              </span>
            {/if}
          </div>
          {#if group.orphan && adoptOpen === group.scope}
            <div class="mem-adopt">
              {#each knownProjects as p (p.rootPath)}
                <button type="button" class="ctrl tiny" onclick={() => adoptScope(group.scope, p.rootPath)}>{p.name}</button>
              {/each}
            </div>
          {/if}
          <!-- Keyed by index rather than by text: two remembered lines can be
               byte-identical, and the index is also what the save addresses. -->
          {#each group.lines as line, i (i)}
            <div class="mem-row" class:editing={isEditing(group.scope, i)}>
              {#if isEditing(group.scope, i)}
                <!-- svelte-ignore a11y_autofocus -->
                <textarea
                  class="mem-input" rows="2" autofocus
                  bind:value={memoryDraft}
                  onkeydown={(e) => onMemoryKeydown(e, group.scope, i)}
                ></textarea>
                <div class="mem-actions">
                  <button
                    type="button" class="ctrl ctrl-primary"
                    disabled={memorySaving || !memoryDraft.trim()}
                    onclick={() => commitMemory(group.scope, i, memoryDraft)}
                  >{t('settings.learningMemorySave')}</button>
                  <button type="button" class="ctrl" disabled={memorySaving} onclick={cancelMemoryEdit}
                  >{t('settings.learningMemoryCancel')}</button>
                </div>
              {:else}
                <p class="mem-text">{line}</p>
                <div class="mem-actions">
                  <button
                    type="button" class="icobtn tiny tip-l" aria-label={t('settings.learningMemoryEdit')}
                    data-tip={t('settings.learningMemoryEdit')} disabled={memorySaving}
                    onclick={() => startMemoryEdit(group.scope, i)}
                  ><Icon name="pencil" size={13} /></button>
                  <!-- No confirm: the line is one sentence the agent wrote, the
                       file is plain markdown the user owns, and a dialog for
                       every tidy-up is what makes a list nobody tidies. -->
                  <button
                    type="button" class="icobtn tiny tip-l mem-forget" aria-label={t('settings.learningMemoryForget')}
                    data-tip={t('settings.learningMemoryForget')} disabled={memorySaving}
                    onclick={() => commitMemory(group.scope, i, '')}
                  ><Icon name="x" size={13} /></button>
                </div>
              {/if}
            </div>
          {/each}
        {/each}
        {#if memoryScopeError}
          <div class="set-error">{memoryScopeError}</div>
        {/if}
        {#if memoryGroups.length === 0}
          <div class="empty">{t('settings.learningMemoryEmpty')}</div>
        {/if}
        <div class="set-row learn-foot">
          <button type="button" class="ctrl" onclick={() => OpenMemoryFolder()}>
            <Icon name="folderOpen" size={13} /> {t('settings.learningOpenFolder')}
          </button>
        </div>
      </div>

      {#if decidedChanges.length > 0}
        <h3 class="set-h3">{t('settings.learningHistory')}</h3>
        <p class="muted set-sub">{t('settings.learningHistoryHint')}</p>
        <div class="settings-card">
          {#each decidedExpanded ? decidedChanges : decidedChanges.slice(0, DECIDED_PREVIEW) as c (c.id)}
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
          {#if decidedChanges.length > DECIDED_PREVIEW}
            <button type="button" class="learn-more" onclick={() => (decidedExpanded = !decidedExpanded)}>
              <Icon name={decidedExpanded ? 'chevronUp' : 'chevronDown'} size={12} />
              {decidedExpanded
                ? t('settings.learningHistoryLess')
                : t('settings.learningHistoryMore', { n: decidedChanges.length - DECIDED_PREVIEW })}
            </button>
          {/if}
        </div>
      {/if}

    <!-- The problems room. Structure borrowed whole from the queue above
         (.settings-card / .learn-row): the layout was right and is already
         styled, and inventing a second look for a list of rows with two
         buttons would be ornament, not design. What differs is what it says
         and what the buttons do — which is the entire change. -->
    {:else if active === 'issues'}
      <h2>{t('settings.issues')}</h2>
      <p class="muted set-sub">{t('settings.issuesDesc')}</p>
      <!-- Said here rather than only in the privacy policy, because this is the
           page where the button is. The pre-filled body carries the tool's own
           arguments, and those are the user's file paths -- a report is one
           click from publishing "output/20260823/xiaomi-17t-pro/index.html" to
           a public tracker. Nothing is sent until they submit on GitHub, so the
           fix is telling them what they are about to look at, not hiding it. -->
      <p class="muted set-sub">{t('settings.issuesReportNote')}</p>

      {#if issuesError}<div class="mset-error">{issuesError}</div>{/if}

      <!-- On the page somebody opens when something is wrong. Not instead of
           the issue button below: an issue carries the version and the log,
           the group carries the thing you cannot describe well enough to file
           yet. -->
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.community')}</div>
            <div class="d">{t('settings.communityDesc')}</div>
          </div>
          <button class="ctrl" onclick={() => BrowserOpenURL(COMMUNITY_URL)}>{t('settings.communityOpen')}</button>
        </div>
      </div>

      <div class="settings-card">
        {#each systemIssues as c (c.id)}
          <div class="learn-row">
            <div class="learn-main">
              <div class="learn-head">
                <span class="learn-scope">{scopeLabel(c.scope)}</span>
              </div>
              <div class="learn-body">{c.body}</div>
              {#if c.reason}<div class="learn-why">{c.reason}</div>{/if}
            </div>
            <!-- Asking comes first, and is the primary, because it is the only
                 one of the three that answers the question the other two need
                 answered. Reporting a problem that turns out to be this
                 machine's wastes the developer's time and the user's; waving
                 off one that turns out to be a real bug loses it. -->
            <div class="learn-actions">
              <button type="button" class="ctrl ctrl-primary" disabled={issuesBusy === c.id}
                onclick={() => consultIssue(c)}>{t('settings.issuesConsult')}</button>
              <button type="button" class="ctrl" disabled={issuesBusy === c.id}
                onclick={() => reportIssue(c)}>{t('settings.issuesReport')}</button>
              <button type="button" class="ctrl" disabled={issuesBusy === c.id}
                onclick={() => dismissIssue(c.id)}>{t('settings.issuesDismiss')}</button>
            </div>
          </div>
        {/each}
        {#if systemIssues.length === 0}
          <div class="empty">{t('settings.issuesEmpty')}</div>
        {/if}
      </div>

      <!-- Where a decided row went. The rows were never deleted — only invisible,
           which read as destroyed. Same shape as the learning room's record:
           four, then a count. -->
      {#if decidedIssues.length > 0}
        <h3 class="set-h3">{t('settings.issuesHistory')}</h3>
        <p class="muted set-sub">{t('settings.issuesHistoryHint')}</p>
        <div class="settings-card">
          {#each (issuesExpanded ? decidedIssues : decidedIssues.slice(0, DECIDED_PREVIEW)) as c (c.id)}
            <div class="learn-row past">
              <div class="learn-main">
                <div class="learn-head">
                  <span class="learn-scope">{scopeLabel(c.scope)}</span>
                  <span class="learn-op" class:rejected={c.state !== 'reported'}>
                    {c.state === 'reported' ? t('settings.issuesStateReported') : t('settings.issuesStateDismissed')}
                  </span>
                </div>
                <div class="learn-body">{c.body}</div>
              </div>
            </div>
          {/each}
          {#if decidedIssues.length > DECIDED_PREVIEW}
            <button type="button" class="learn-more" onclick={() => (issuesExpanded = !issuesExpanded)}>
              <Icon name={issuesExpanded ? 'chevronUp' : 'chevronDown'} size={12} />
              {issuesExpanded
                ? t('settings.learningHistoryLess')
                : t('settings.learningHistoryMore', { n: decidedIssues.length - DECIDED_PREVIEW })}
            </button>
          {/if}
        </div>
      {/if}
    {:else if active === 'usage'}
      <h2>{t('settings.usage')}</h2>
      <p class="muted set-sub">{t('settings.usageDesc')}</p>

      {#if usageError}<div class="mset-error">{usageError}</div>{/if}

      <!-- Three states, and the middle one used to be missing. UsageStats walks
           the whole history to build the chart, the heatmap and the streak, and
           that is half a second on this machine's own database and longer while
           the engine is writing to it. All of that time the page said "ยังไม่มี
           ข้อมูลการใช้งาน" — a sentence about the data, printed before anybody
           had asked the data anything.
           The placeholder is the page's own grid rather than a spinner, so the
           numbers land in the boxes that were already holding their place and
           nothing jumps. Shapes and pulse are the model pane's (.sk, sk-pulse);
           only the geometry is this layout's. -->
      {#if usagePending}
        <div class="stat-cards usage-sk" aria-busy="true" aria-label={t('settings.loading')}>
          {#each ['wide', 'wide', 'wide', '', '', '', ''] as w, i (i)}
            <div class="stat-card {w}">
              <span class="sk sk-eyebrow"></span>
              <span class="sk sk-figure"></span>
              <span class="sk sk-line short"></span>
            </div>
          {/each}
        </div>
        <div class="settings-card wide-card usage-sk" aria-hidden="true">
          <div class="card-form"><span class="sk sk-eyebrow"></span><span class="sk sk-chart"></span></div>
        </div>
        <div class="settings-card wide-card usage-sk" aria-hidden="true">
          <div class="card-form"><span class="sk sk-eyebrow"></span><span class="sk sk-heat"></span></div>
        </div>
      {:else if usage && usage.totals.calls > 0}
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

          <!-- The counts were always here; what was missing was a price to
               multiply them by, so "why did my balance drain" could only be
               answered in tokens. Three states, and the difference between the
               last two matters: priced, partly priced (say how much of it the
               figure covers, or a total built from half the models reads as
               the bill), and no catalog at all (a dash, never a zero — zero
               reads as "this was free"). -->
          <div class="stat-card wide">
            <div class="eyebrow">{t('settings.usageCost')}</div>
            {#if !tot.pricesFetched || tot.pricedCalls === 0}
              <div class="stat-big dim">—</div>
              <div class="stat-sub">{t('settings.usageCostUnknown')}</div>
            {:else}
              <div class="stat-big"><span class="unit">$</span>{tot.cost.toFixed(2)}</div>
              <div class="stat-sub">
                {t('settings.usageCostEstimate')}
                {#if tot.pricedCalls < tot.calls}
                  · {t('settings.usageCostPartial', {
                    priced: fmtCompact(tot.pricedCalls), total: fmtCompact(tot.calls),
                  })}
                {/if}
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
                  {#each topModels as model (model)}
                    <span><i class="dot s{slotOf(model)}"></i>{model}</span>
                  {/each}
                  {#if allModels.length > 5}
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
        {#if usagePending}
          {#each [0, 1, 2, 3] as i (i)}
            <div class="set-row usage-sk" aria-hidden="true"><span class="sk sk-line"></span></div>
          {/each}
        {:else if usageRows.length === 0}
          <!-- Reached only once the engine has actually answered, which is what
               makes this sentence true when it is printed. -->
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
          {#each usageRows as r (rowKey(r))}
            <div class="set-row usage-row">
              <div class="u-model">
                <i class="dot s{slotOf(r.model)}"></i>{r.model}
                <!-- Who served it, wherever anybody wrote it down. Two rows of
                     one model are two bills, and drawing them as the same name
                     twice with different numbers is a table nobody can read. -->
                {#if r.provider}<span class="u-by">{r.provider}</span>{/if}
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

      {@render aiFindCard('settings.aiFindMCPTitle', 'settings.aiFindMCPDesc', 'settings.aiFindMCPPrompt')}

      <div class="settings-card">
        <div class="card-form">
          <div class="mset-keyrow">
            <div class="eyebrow eyebrow-grow">{t('settings.mcpConfigured')}</div>
            <!-- Beside the list it acts on, not at the bottom of the page under
                 the form it opens. Adding a server is one of the two things you
                 come to this card to do, so it sits with the other one. The
                 form still appears below; openMCPForm scrolls it into view, or
                 the press looks like it did nothing. -->
            <button class="ctrl ctrl-icon" disabled={mcpBusy !== ''} onclick={openMCPForm}>
              <Icon name="plus" size={13} />
              {t('settings.addServer')}
            </button>
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
            <div class="set-row reg-entry" class:mcp-off={s.disabled}>
              <!-- The row is the register entry: the header says who this
                   server is and who it serves, and opening it reveals the
                   switches. A server connected but pointed at nobody is the
                   state worth calling out — it works and reaches no one. -->
              <button
                class="reg-head"
                aria-expanded={mcpOpen === s.name}
                onclick={() => (mcpOpen = mcpOpen === s.name ? '' : s.name)}
              >
                <span class="reg-caret" class:open={mcpOpen === s.name}>›</span>
                <span class="set-txt">
                  <span class="t">
                    <span class="dot" style={statusVar(s.status)}></span> {s.name}
                    <span class="mcp-badge">{s.url ? 'http' : 'stdio'}</span>
                    {#if s.tools > 0}<span class="mcp-badge">{t('settings.mcpToolCount', { n: String(s.tools) })}</span>{/if}
                    {#if !s.disabled}
                      <span class="mcp-badge" class:mcp-badge-warn={mcpServesNobody(s)}>
                        {mcpServesNobody(s)
                          ? t('settings.mcpForNobody')
                          : t('settings.mcpForCount', { n: String(mcpTargetsOf(s).length) })}
                      </span>
                    {/if}
                  </span>
                  <span class="d">{s.url || (s.command ?? []).join(' ')}{s.err ? ' · ' + s.err : ''}</span>
                </span>
              </button>
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

              {#if mcpOpen === s.name}
                <div class="mcp-targets">
                  <div class="d muted mcp-targets-hint">{t('settings.mcpForHint')}</div>
                  {#each ['desk', 'agent'] as kind}
                    {@const rows = mcpTargets.filter((x) => x.kind === kind)}
                    {#if rows.length > 0}
                      <div class="eyebrow mcp-targets-group">
                        {kind === 'desk' ? t('settings.mcpForDesks') : t('settings.mcpForAgents')}
                      </div>
                      {#each rows as target (target.id)}
                        <label class="mcp-target">
                          <input
                            type="checkbox"
                            checked={mcpTargetsOf(s).includes(target.id)}
                            disabled={mcpBusy !== '' || s.disabled}
                            onchange={() => toggleMCPTarget(s, target.id)}
                          />
                          <span>{target.name}</span>
                        </label>
                      {/each}
                    {/if}
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        {/if}
      </div>

      <!-- Nothing at all until it is asked for. The eight controls in here used
           to be laid out permanently under the list, which made a rare act look
           like part of the page (owner, 2026-08-14: *"ทำเป็นปุ่มกด เพิ่ม SERVER
           แล้วค่อยแสดง ดีกว่ามาเรี่ยราดแบบนี้"*). The way in is the button up in
           the list's own header. -->
      {#if mcpFormOpen}
      <div class="settings-card" bind:this={mcpFormEl}>
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
          <!-- Said where the key would otherwise be typed. A safer way to write
               something is worth nothing if it is only in the docs. -->
          <div class="d muted">{t('settings.mcpSecretHint')}</div>

          <!-- Both fields the stored config always had and the form never
               offered. They used to sit behind a "ตัวเลือกเพิ่มเติม" fold, which
               the owner asked to remove on 2026-08-14: a closed disclosure with
               a generic label is one more thing on the page that says nothing
               about itself, and a form with a hidden half is a form people do
               not trust they have finished. The fields are cheap; the lid was
               the expensive part. -->
          <div class="mcp-more">
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
              <label class="pp-field">
                <span class="eyebrow">{t('settings.mcpTools')}</span>
                <textarea class="ctrl mcp-lines" rows="3" placeholder={t('settings.mcpToolsPlaceholder')} bind:value={mcpToolsText}></textarea>
                <span class="d muted">{t('settings.mcpToolsHint')}</span>
              </label>
            </div>
          </div>

          <div class="mset-keyrow">
            <button class="ctrl ctrl-primary" disabled={mcpBusy !== '' || !mcpFormValid} onclick={saveMCP}>
              {mcpBusy === 'save' ? t('settings.saving') : (mcpOriginal ? t('settings.save') : t('settings.add'))}
            </button>
            <!-- Always offered now, where it used to appear only for an edit or
                 a preset. Once the form is something you opened, Cancel is the
                 way to close it again — and a panel you can open and not shut
                 is the thing this whole change was about. -->
            <button class="ctrl" disabled={mcpBusy !== ''} onclick={resetMCPForm}>{t('settings.cancel')}</button>
          </div>
          {#if mcpNeedsKey}
            <!-- Says why the form filled itself in. Without it the preset's
                 button appears to have done nothing. -->
            <div class="d muted">{t('settings.mcpNeedsKey', { name: mcpName })}</div>
          {/if}
          {#if mcpError}<div class="mset-error">{mcpError}</div>{/if}
        </div>
      </div>
      {/if}

      <div class="settings-card">
        <div class="card-form">
          <div class="eyebrow">{t('settings.mcpPresets')}</div>
        </div>
        {#each mcpPresets as p (p.name)}
          {@const wanted = agentsNeeding(p.name)}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{p.name} <span class="mcp-badge">{p.url ? 'http' : 'stdio'}</span></div>
              <div class="d">{p.desc} · {p.url ?? p.command?.join(' ')}</div>
              <!-- Why it is on a shelf at all: what it reaches that Aetox has
                   no tool for. The list was seven names and seven capability
                   lines, which answered "what is this" and never "why am I
                   being shown it". -->
              <div class="d mcp-why">{p.why}</div>
              {#if wanted.length > 0}
                <!-- Read off the agents' own `needs:`, so it cannot disagree
                     with them. This is the line that turns a shelf into an
                     answer: not "here are seven servers" but "this one is the
                     one your research agent has been asking for". -->
                <div class="d mcp-wanted">
                  <Icon name="bot" size={12} />
                  {t('settings.mcpPresetFor')} {wanted.join(', ')}
                </div>
              {/if}
            </div>
            <button class="ctrl" disabled={mcpBusy !== '' || presetTaken(p.name)} onclick={() => addPreset(p)}>
              {mcpBusy === 'preset:' + p.name ? t('settings.adding') : t('settings.add')}
            </button>
          </div>
        {/each}
      </div>
    {:else if active === 'connections'}
      <!-- One register, one page. The sentence under the title has to cover
           both kinds without flattening them: an account needs a key, a machine
           you run needs an address as well, and a user arriving with either one
           should see themselves in it. -->
      <h2>{t('settings.connections')}</h2>
      <p class="muted set-sub">{t('settings.connectionsDesc')}</p>

      <!-- A register, drawn the same way the MCP page draws its own: one line
           per service until you open it. With four services and a token box
           each, cards left open would be a wall — and the thing a returning
           user wants from this page is a glance, not a form. -->
      <div class="settings-card">
        {#each visibleConnections as row (row.id)}
          {@const targets = targetsOf(row)}
          {@const open = connOpen === row.id}
          <div class="set-row reg-entry">
            <button
              class="reg-head"
              aria-expanded={open}
              onclick={() => (connOpen = open ? '' : row.id)}
            >
              <span class="reg-caret" class:open>›</span>
              <span class="set-txt">
                <span class="t">
                  <span class="dot" style={statusVar(row.connected ? 'connected' : '')}></span>
                  {row.label}
                  {#if row.source === 'connection'}
                    <span class="mcp-badge" class:mcp-badge-warn={servesNobody(row)}>
                      {servesNobody(row) ? t('settings.connForNobody') : placementSummary(row)}
                    </span>
                  {/if}
                </span>
                <span class="d">
                  {#if row.source === 'connection'}
                    {t('settings.ghConnectedAs', { login: row.login ?? '' })}
                  {:else if row.source === 'environment'}
                    {t('settings.ghFromEnv')}
                  {:else}
                    {t('settings.ghNotConnected')}
                  {/if}
                </span>
              </span>
            </button>
            <!-- The one action worth reaching without opening the row. A
                 connected service has nothing here: disconnecting is not a
                 thing to do by accident on a list. -->
            {#if row.source !== 'connection' && !open}
              <button class="ctrl" onclick={() => (connOpen = row.id)}>
                {t('settings.ghConnect')}
              </button>
            {/if}

            {#if open}
              <div class="conn-body">
                <div class="d muted">{connBlurb[row.id] ?? ''}</div>

                {#if row.source === 'environment'}
                  <div class="d muted">{t('settings.ghFromEnvHint')}</div>
                {:else if row.env_override}
                  <div class="d muted">{t('settings.ghEnvAlso')}</div>
                {/if}
                {#if connScopes[row.id]}
                  <div class="d mono-dim">
                    {connScopes[row.id].length > 0
                      ? t('settings.ghScopes', { list: connScopes[row.id].join(', ') })
                      : t('settings.ghScopesUnstated')}
                  </div>
                {/if}

                <!-- Placement. Same ids and same list as an MCP server's
                     `for:`, so what a user learns on that page is true here.

                     Unless the connection is locked to one agent (home_agent):
                     an engine is that agent's workstation, and a picker with
                     eight audiences for a thing with one reader is not
                     flexibility — it is eight ways into "connected everywhere,
                     usable nowhere" (2026-08-10). The backend enforces the
                     lock either way; this draws the fact instead of a choice
                     that would be silently corrected. -->
                {#if row.home_agent}
                  <div class="eyebrow conn-eyebrow">{t('settings.connFor')}</div>
                  <div class="d muted">{t('settings.connHomeLocked', { agent: row.home_agent })}</div>
                {:else}
                  <div class="eyebrow conn-eyebrow">{t('settings.connFor')}</div>
                  <div class="conn-targets">
                    {#each mcpTargets as target (target.id)}
                      <!-- The chip carries the desk's NAME; its description is a
                           paragraph and belongs on hover. Both used to be the
                           same string, which put "โต๊ะผู้ช่วย — ทำได้ทุกอย่าง…"
                           inside a chip beside agent names one word long. -->
                      <button
                        class="conn-chip"
                        class:on={targets.includes(target.id)}
                        class:agent={target.kind === 'agent'}
                        disabled={connBusy !== ''}
                        title={target.detail ?? ''}
                        aria-pressed={targets.includes(target.id)}
                        onclick={() => toggleConnectionTarget(row, target.id)}
                      >
                        <!-- A desk and an agent are two different kinds of
                             audience and the row drew them as one list of
                             identical pills, so "assistant, coding, github,
                             research" read as nine of the same thing (owner,
                             19 ส.ค.: "รายชื่อเอเจน เอาให้ชัดสิ ไอค่อนไปไหน").
                             The mark is the difference: a desk wears its own
                             icon, the one it wears in the nav, and an agent
                             wears `bot` — the same glyph every agent already
                             wears in the composer's switcher. -->
                        <Icon name={target.kind === 'agent' ? 'bot' : 'layoutList'} size={12} />
                        {target.name}
                      </button>
                    {/each}
                  </div>
                {/if}

                {#if row.source !== 'connection'}
                  <!-- The address comes first because it is the question the
                       token cannot answer: a service the user runs is at an
                       address only they know, and a key checked against the
                       wrong host fails in a way that reads as a bad key. Not
                       a password field — it is a setting, and hiding it would
                       stop the user spotting their own typo. -->
                  {#if row.needs_base_url}
                    <div class="eyebrow conn-eyebrow">{t('settings.connBaseURLLabel')}</div>
                    <input
                      class="ctrl key-input" type="text" autocomplete="off" spellcheck="false"
                      placeholder={row.base_url_hint ?? ''}
                      value={connBaseURL[row.id] ?? ''}
                      oninput={(e) => (connBaseURL[row.id] = e.currentTarget.value)}
                      onkeydown={(e) => e.key === 'Enter' && connectAccount(row)}
                    />
                    <div class="d muted">{t('settings.connBaseURLHint')}</div>
                    {@render serverControls(row)}
                  {/if}
                  <!-- The service's own words, not GitHub's.
                       These four strings were GitHub's copy hardcoded, so the
                       n8n row asked for a "PERSONAL ACCESS TOKEN" starting
                       `ghp_…` and promised to check it with GitHub. Wrong on
                       every row but one, and wrong in the way that makes a
                       person doubt they are on the right screen. -->
                  <!-- The other half, and it says so. Whether the KEY works is a
                       different question from whether the SERVER is up, and the
                       two were an unlabelled run of fields in one column. -->
                  {#if row.needs_base_url}
                    <div class="conn-part-head standalone">
                      <Icon name="shield" size={13} />
                      <span>{t('settings.connAccountPart')}</span>
                    </div>
                    <div class="d muted">{t('settings.connAccountPartHint')}</div>
                  {/if}
                  <div class="eyebrow conn-eyebrow">
                    {row.needs_base_url ? t('automation.keyLabel') : t('settings.ghTokenLabel')}
                  </div>
                  <div class="mset-keyrow">
                    <!-- type=password: this is a live credential, and a
                         settings page is the one screen people screen-share. -->
                    <input
                      class="ctrl key-input" type="password" autocomplete="off"
                      placeholder={row.needs_base_url ? '' : t('settings.ghTokenPlaceholder')}
                      value={connToken[row.id] ?? ''}
                      oninput={(e) => (connToken[row.id] = e.currentTarget.value)}
                      onkeydown={(e) => e.key === 'Enter' && connectAccount(row)}
                    />
                    <button
                      class="ctrl ctrl-primary"
                      disabled={connBusy !== '' || !connectable(row)}
                      onclick={() => connectAccount(row)}
                    >
                      {connBusy === row.id + ':connect' ? t('settings.ghConnecting') : t('settings.ghConnect')}
                    </button>
                  </div>
                  <div class="d muted">{t('settings.connTokenHint', { name: row.label })}</div>
                {/if}

                <!-- Once connected the address field is gone, and with it the
                     only place the server controls were drawn — but a server
                     you connected yesterday is exactly the one that is down
                     today. So they are here too. -->
                {#if row.needs_base_url && row.source === 'connection'}
                  {@render serverControls(row)}
                {/if}

                <div class="mset-keyrow conn-actions">
                  <div class="d muted eyebrow-grow"></div>
                  {#if row.token_url && row.source !== 'connection'}
                    <button class="ctrl" onclick={() => BrowserOpenURL(row.token_url ?? '')}>
                      {t('settings.connCreateToken', { name: row.label })}
                    </button>
                  {/if}
                  {#if row.connected}
                    <button class="ctrl" disabled={connBusy !== ''} onclick={() => verifyConnection(row)}>
                      {connBusy === row.id + ':verify' ? t('settings.ghVerifying') : t('settings.ghVerify')}
                    </button>
                  {/if}
                  {#if row.source === 'connection'}
                    <button class="ctrl ctrl-danger" disabled={connBusy !== ''} onclick={() => askDisconnect(row)}>
                      {t('settings.ghDisconnect')}
                    </button>
                  {/if}
                </div>

                {#if connError[row.id]}<div class="mset-error">{connError[row.id]}</div>{/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>

    {:else if active === 'account'}
      <h2>{t('settings.account')}</h2>
      <p class="muted set-sub">{t('settings.accountDesc')}</p>

      {#if aetoxError}<div class="mset-error">{aetoxError}</div>{/if}

      <div class="settings-card">
        {#if aetoxAccount?.signed_in}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{aetoxAccount.display}</div>
              {#if aetoxAccount.user?.email && aetoxAccount.user.email !== aetoxAccount.display}
                <div class="d">{aetoxAccount.user.email}</div>
              {/if}
            </div>
            <button class="ctrl" disabled={aetoxBusy} onclick={aetoxCheck}>{t('settings.accountRefresh')}</button>
            <button class="ctrl" disabled={aetoxBusy} onclick={aetoxSignOut}>{t('settings.accountSignOut')}</button>
          </div>
        {:else if aetoxBusy}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{t('settings.accountWaiting')}</div>
              <div class="d">{t('settings.accountWaitingDesc')}</div>
            </div>
            <button class="ctrl" onclick={aetoxAbort}>{t('settings.accountCancel')}</button>
          </div>
        {:else}
          <div class="set-row">
            <div class="set-txt">
              <div class="t">{t('settings.accountSignedOut')}</div>
              <div class="d">{t('settings.accountSignedOutDesc')}</div>
            </div>
            {#each aetoxAccount?.providers ?? [] as door}
              <button class="ctrl" onclick={() => aetoxSignIn(door)}>
                {t('settings.accountWith', { provider: door === 'github' ? 'GitHub' : 'Google' })}
              </button>
            {/each}
          </div>
          <!-- Said on the page rather than discovered after signing in. The
               store this account is for does not exist yet, and a button that
               implies a locked feature would be the lie. -->
          <div class="set-row">
            <div class="set-txt">
              <div class="d">{t('settings.accountUnlocks')}</div>
            </div>
          </div>
        {/if}

        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.accountServer')}</div>
            <div class="d">{t('settings.accountServerDesc')}</div>
            <div class="d">{aetoxAccount?.server ?? ''}</div>
          </div>
        </div>
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
              <!-- Three endings, one action each. canAuto is the VS Code shape
                   split in two (§107): download and verify now, restart when
                   the user says. Scoop installed us, so Scoop upgrades us:
                   Aetox never writes into someone else's package directory.
                   Everything left gets the release page, which is always a
                   correct answer. -->
              <div class="d">
                {updateStatus.canAuto
                  ? t('settings.aboutAutoHint')
                  : updateStatus.hint ? t('settings.aboutRunCommand') : t('settings.aboutDownloadHint')}
              </div>
              {#if !updateStatus.canAuto && updateStatus.hint}
                <code class="about-cmd">{updateStatus.hint}</code>
              {/if}
              {#if updater.error}
                <div class="mset-error">{t('settings.aboutUpdateFailed', { err: updater.error })}</div>
              {/if}
            </div>
            {#if updateStatus.canAuto}
              <!-- The same two acts the card offers, because they are the same
                   two acts. This page is a second view of one state, never a
                   second copy of it (selfUpdate.svelte). -->
              {#if updater.phase === 'ready'}
                <button class="ctrl ctrl-primary" onclick={restartToUpdate}>
                  {t('settings.aboutRestartToUpdate')}
                </button>
              {:else}
                <button class="ctrl" disabled={updater.phase === 'downloading' || updater.phase === 'restarting'} onclick={startDownload}>
                  {updater.phase === 'restarting'
                    ? t('settings.aboutRestarting')
                    : updater.phase === 'downloading'
                      ? (updatePct() >= 0 ? t('settings.aboutDownloadingPct', { pct: String(updatePct()) }) : t('settings.aboutDownloading'))
                      : t('settings.aboutUpdateNow')}
                </button>
              {/if}
            {:else if updateStatus.hint}
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

        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.community')}</div>
            <div class="d">{t('settings.communityDesc')}</div>
          </div>
          <button class="ctrl" onclick={() => BrowserOpenURL(COMMUNITY_URL)}>{t('settings.communityOpen')}</button>
        </div>

        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.aboutReport')}</div>
            <div class="d">{t('settings.aboutReportDesc')}</div>
          </div>
          <button class="ctrl" onclick={() => openIssueForm('problem')}>{t('settings.aboutReportOpen')}</button>
        </div>

        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.aboutFeedback')}</div>
            <div class="d">{t('settings.aboutFeedbackDesc')}</div>
          </div>
          <button class="ctrl" onclick={() => openIssueForm('feedback')}>{t('settings.aboutFeedbackOpen')}</button>
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
        <!-- What a supporter gets, said before the contact row that acts on it.
             The credit is a promise the project can actually keep: a name on a
             page it controls, kept rather than counted once. -->
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.sponsorCredit')}</div>
            <div class="d">{t('settings.sponsorCreditDesc')}</div>
          </div>
        </div>
        <!-- The same GitHub door the problem and feedback rows use. A donation
             is anonymous by construction — PromptPay tells the developer a
             transfer happened, never who to thank — so the supporter has to be
             the one who says. Asking to stay anonymous is offered in the same
             breath, because a name on a page is not everyone's idea of thanks. -->
        <div class="set-row">
          <div class="set-txt">
            <div class="t">{t('settings.sponsorContact')}</div>
            <div class="d">{t('settings.sponsorContactDesc')}</div>
          </div>
          <button class="ctrl" onclick={() => BrowserOpenURL(ISSUES_URL + '/new')}>{t('settings.sponsorContactOpen')}</button>
        </div>
        <!-- Attribution, not decoration: this is the one place in the running app
             that names who wrote it and where it came from. Untranslated on
             purpose — a name, a licence id and a URL read the same in every
             language, and a translated copyright line is a mistranslated one.
             It comes from version.Credit through AppCredit rather than being
             written out here: a literal was a second place naming the licence,
             and on 2026-08-19 it named the old one (§148). -->
        <div class="set-row">
          <div class="set-txt">
            <div class="t">Aetox</div>
            <div class="d">{appCredit}</div>
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
            <span class="group-title">{t(`settings.toolGroup_${g.key}`)}</span>
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
