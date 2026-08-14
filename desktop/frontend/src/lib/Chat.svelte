<script lang="ts">
  import type { ChatMessage, TaskState, ModelStatus, ToolStep, TimelineNode, ContextBreakdown } from './types'
  import { groupSteps, isDelegation } from './types'
  import TaskTimeline from './TaskTimeline.svelte'
  import BackgroundWork from './BackgroundWork.svelte'
  import Palette from './Palette.svelte'
  import Logo from './Logo.svelte'
  import { onMount } from 'svelte'
  import { shell } from './shell.svelte'
  import {
    EnabledProviders, SupportedThinkLevels,
    ListModelsForProvider, RequiresAPIKey, HasAPIKey, PickAttachment,
    GetContextBreakdown, GuideTopics, RunChatCommand, ListChairs, ChairStarters, CurrentSessionID,
    Shells, CurrentShell, SetShell, EnginesFor, UseEngine, VerifyConnection,
  } from '../../wailsjs/go/main/App'
  import type { main, connect, subagent } from '../../wailsjs/go/models'
  import { t, i18n } from './i18n.svelte'
  import { isShortcut, shortcutLabel } from './shortcuts'
  import { copyDrawing, saveDrawing } from './drawingExport'
  import { renderMarkdown, renderStreamingMarkdown } from './markdown'
  import { openUrlInWorkbench, openFileTab, setTabDragPayload, TAB_DRAG_MIME } from './stores/workbench.svelte'
  import {
    cockpit, attachImageFromPath, attachImageFromClipboard, clearPendingImage, attachTabContext, clearPendingContext,
    attachFileFromPath, clearPendingFile, fileKind, attachmentPreview,
    openProject, openFolder, clearProjectFocus, cancelTurn, answerAsk, queuedMessages,
    addProjectFolder, removeProjectFolder,
    retryActiveProvider, undoLastTurn, switchApprovalMode,
    startTaskChip, dismissTaskChip,
    retryFailedTurn, regenerateReply, switchVariant, resendEdited, rateReply,
    setActiveView, newChairSession, newSessionAt, openSettingsAt, setStance,
  } from './stores/cockpit.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import Icon from './Icon.svelte'
  import ProviderMark from './ProviderMark.svelte'
  import { ICONS, type IconName } from './icons'
  import { startersFor } from './starters'

  let {
    messages, task, model, awaitingReply, agentStatus, toolSteps, streamingText, reasoningText,
    onSend, onSwitchProvider, onSwitchThinkLevel, onSwitchModel, onSubmitAPIKey,
  }: {
    messages: ChatMessage[]
    task: TaskState
    model: ModelStatus
    awaitingReply: boolean
    agentStatus: string
    toolSteps: ToolStep[]
    streamingText: string
    reasoningText: string
    onSend: (text: string) => void
    onSwitchProvider: (provider: string) => Promise<void>
    onSwitchThinkLevel: (level: string) => Promise<void>
    onSwitchModel: (modelName: string) => Promise<void>
    onSubmitAPIKey: (provider: string, apiKey: string) => Promise<void>
  } = $props()

  let providers = $state<string[]>([])
  let thinkLevels = $state<string[]>([])
  let models = $state<string[]>([])
  let needsApiKey = $state(false)
  let apiKeyDraft = $state('')
  // Thinking, the agent's own tools, and what it delegated are views of the
  // same turn, so they take turns in one slot instead of stacking — opening
  // one closes the others. '' = all collapsed, which is how a finished reply
  // starts. Sub-agents are their own view rather than rows mixed into the tool
  // list: "what did it do itself" and "what did it hand to someone else" are
  // different questions, and one list answers neither. Delegations split once
  // more into 'agents' (เอเจน — a colleague hired for a whole job) and 'subs'
  // (ซับเอเจน — the assistant's own hands), because "ซับเอเจน 1 ตัว" on a
  // turn where the doc agent made the file misnames whose work it was.
  type Panel = 'think' | 'tools' | 'agents' | 'subs' | ''
  let openPanel = $state<Record<number, Panel>>({})
  function togglePanel(i: number, p: Panel) {
    openPanel[i] = openPanel[i] === p ? '' : p
  }
  // The live turn opens on thinking: that is the part still streaming in.
  //
  // Reset at the start of every turn, because this used to be set once for the
  // lifetime of the component: collapsing the panel — or opening the tools view
  // — once left every later turn in that session with nothing on screen at all
  // but the bouncing dots, which is the difference between "working" and
  // "frozen". Only reacts to awaitingReply, so toggling it mid-turn still sticks
  // for that turn.
  let livePanel = $state<Panel>('think')
  $effect(() => {
    if (awaitingReply) livePanel = 'think'
  })

  // A finished tool call is history — it collapses behind a count next to the
  // thinking toggle, same as reasoning does. Only what is still running stays
  // on screen.
  //
  // A delegation is split by the state of its own `task` row, never by its
  // children's: a delegate that has finished three tools and is still on its
  // fourth is one running block, not three finished fragments and a live one.
  // The partition is total: a row is the agent's own only if it has no parent
  // and is not itself a delegation. Everything else — a delegate's step, and a
  // step whose `task` row is missing — belongs to the sub-agent view, so no row
  // can fall through the gap and be counted as the agent's own work.
  const isOwn = (n: TimelineNode) => !n.step.parent && !isDelegation(n)
  const ownSteps = (steps: ToolStep[]) => groupSteps(steps).filter(isOwn).map((n) => n.step)
  const delegated = (steps: ToolStep[]) => groupSteps(steps).filter((n) => !isOwn(n))
  // The two piles a delegation lands in (COMPANY.md §4): a เอเจน takes a whole
  // job and returns a file; a ซับเอเจน is a step of the assistant's own
  // work. The kind rides on the step, stamped by the engine from which home the
  // profile file lives in — the frontend never guesses it from a name. A step
  // with no stamp (an old persisted turn) counts as a helper, which is the pile
  // every delegation was in before the split.
  const isAgentNode = (n: TimelineNode) => n.step.agentKind === 'agent'
  const delegatedAgents = (steps: ToolStep[]) => delegated(steps).filter(isAgentNode)
  const delegatedHelpers = (steps: ToolStep[]) => delegated(steps).filter((n) => !isAgentNode(n))
  // The agent's own steps that are actually tools. Narration and thinking rows
  // ride in the same list and are not — counting them would inflate "used N
  // tools" with sentences.
  //
  // One helper because the count and the button that shows it have to agree.
  // They did not: the button appeared for any own step at all, the summary
  // counted only the tools, and a turn that merely narrated offered "ใช้ 0
  // เครื่องมือ" — a control whose only promise is that there is nothing behind it.
  const ownTools = (steps: ToolStep[]) => ownSteps(steps).filter((s) => !s.kind)

  // This chat's own name, for the breadcrumb above it. Read off the project's
  // list rather than kept here: the title is the store's to know, and a second
  // copy would be the one that goes stale when a chat is renamed.
  const spaceChatTitle = $derived(cockpit.spaceHistory.find((s) => s.active)?.title ?? '')

  const liveDone = $derived(toolSteps.filter((s) => s.state !== 'run'))
  const liveRunning = $derived(toolSteps.filter((s) => s.state === 'run'))
  // The model's latest narration stays on screen while it works — that line is
  // the whole reason notes exist (§59). Older ones collapse into the timeline
  // with everything else.
  const liveNote = $derived([...toolSteps].reverse().find((s) => s.kind === 'note' && !s.parent))
  // Two lists, deliberately: the panel shows every own step, narration
  // included, because that is the model working out loud and it belongs in the
  // timeline. The count beside the button is tools only — and so is what
  // decides whether the button is drawn at all.
  const doneOwn = $derived(ownSteps(toolSteps).filter((s) => s.state !== 'run'))
  const doneOwnTools = $derived(ownTools(toolSteps).filter((s) => s.state !== 'run'))
  const runningOwn = $derived(ownSteps(toolSteps).filter((s) => s.state === 'run'))
  const doneAgents = $derived(delegatedAgents(toolSteps).filter((n) => n.step.state !== 'run'))
  const doneHelpers = $derived(delegatedHelpers(toolSteps).filter((n) => n.step.state !== 'run'))
  // What is still running renders as one list, agents first — each block names
  // its own worker, so the piles only need separating where they are counted.
  const runningSubs = $derived(
    [...delegatedAgents(toolSteps), ...delegatedHelpers(toolSteps)].filter((n) => n.step.state === 'run'),
  )
  // The engine's phase message is a fallback, not the headline. It says
  // "กำลังคิดคำตอบ..." for the whole tool loop — which duplicates the thinking
  // toggle right below it and stops being true the moment a tool starts.
  // Whatever is concretely on screen (reasoning, a running tool, the answer
  // arriving) IS the status; the phrase only fills the gap before any of it.
  const liveStatus = $derived(
    streamingText || reasoningText || liveRunning.length ? '' : agentStatus,
  )
  // One icon table for the two places a file chip shows up: the composer's
  // pending chip and the sent bubble's.
  const fileIcon = (kind?: string): IconName => (kind === 'audio' ? 'headphones' : kind === 'video' ? 'clapperboard' : 'fileText')

  // The agent's own steps and its delegations are counted apart, because "used
  // 6 tools" on a turn where four of them were a sub-agent's says nothing about
  // who did the work. A delegate's steps are counted inside its block, never
  // here.
  function toolsLabel(steps: ToolStep[]): string {
    const own = ownTools(steps)
    const failed = own.filter((s) => s.state === 'err').length
    const base = t('chat.usedTools', { n: own.length })
    return failed ? `${base} · ${t('chat.failedCount', { n: failed })}` : base
  }

  // One label builder for both piles — same shape, different word and count.
  function delegationLabel(nodes: TimelineNode[], key: 'chat.usedAgents' | 'chat.usedSubagents'): string {
    const failed = nodes.filter((n) => n.step.state === 'err').length
    const base = t(key, { n: nodes.length })
    return failed ? `${base} · ${t('chat.failedCount', { n: failed })}` : base
  }

  onMount(async () => {
    providers = await EnabledProviders()
  })

  // At mount, not on first open: whether the chip appears at all depends on the
  // machine having more than one shell, and which one is in use has to be
  // readable without clicking — the point of putting it on this row is that you
  // see it before you approve a command, not after you go looking.
  onMount(refreshShells)

  // The choice is per project, so focusing another one can mean another shell.
  // Without this the chip keeps showing the previous project's answer, which is
  // the one way a label like this can be worse than no label.
  $effect(() => {
    void cockpit.project.focused
    void cockpit.project.name
    refreshShells()
  })

  async function refreshProviderDerived(provider: string) {
    const res = await ListModelsForProvider(provider)
    models = Array.isArray(res) ? res : []
    // Same recovery as Settings: the list loading is proof the endpoint is up.
    if (models.length > 0 && model.warning) await retryActiveProvider()
    needsApiKey = (await RequiresAPIKey(provider)) && !(await HasAPIKey(provider))
  }

  // Model list, API-key requirement, and think levels all depend on the current
  // provider/model — re-derive whenever either changes, from any source (initial
  // async load, a provider switch, or a model switch).
  $effect(() => {
    const provider = model.provider
    if (!provider) return
    refreshProviderDerived(provider)
  })
  // One-shot fetch with no catch used to strand thinkLevels as [] forever if
  // the backend wasn't ready yet (menu row then never appears) — so this is a
  // named refresh, retried every time the model menu opens.
  async function refreshThinkLevels() {
    try {
      thinkLevels = await SupportedThinkLevels()
    } catch {
      /* backend not ready — next menu open retries */
    }
  }
  $effect(() => {
    const provider = model.provider
    const modelName = model.modelName
    if (!provider) return
    refreshThinkLevels()
  })

  // A switch that throws outright (no engine at all — distinct from the
  // fell-back-to-aetox case model.warning covers) used to reject into nothing
  // here, leaving the picker looking like the switch simply didn't take.
  let switchError = $state('')

  async function handleProviderChange(value: string) {
    switchError = ''
    try {
      await onSwitchProvider(value)
    } catch (err) {
      switchError = String(err)
    }
  }

  async function handleModelChange(value: string) {
    switchError = ''
    try {
      await onSwitchModel(value)
    } catch (err) {
      switchError = String(err)
    }
  }

  async function submitApiKey() {
    if (!apiKeyDraft.trim()) return
    await onSubmitAPIKey(model.provider, apiKeyDraft.trim())
    apiKeyDraft = ''
    await refreshProviderDerived(model.provider)
  }

  // What is half-typed survives a reload.
  //
  // It did not: the composer was ordinary component state, so refreshing the
  // window — or the window refreshing itself — threw away a message that had
  // not been sent yet, which is the one piece of text in the app that exists
  // nowhere else. localStorage rather than the engine on purpose: an unsent
  // draft is not part of the conversation, and writing it into the transcript
  // would put words in the user's mouth.
  // Stored with the session it was typed in. A single key restored the same
  // half-written message into every conversation the user opened next, which
  // trades one lost draft for a stray one in someone else's chat — the second
  // is worse, because it looks like something they wrote.
  const DRAFT_KEY = 'aetox-composer-draft'
  let draft = $state('')
  // Reactive: it lands after an await, and the effect below has to re-run when
  // it does. Left as a plain variable, a draft typed in the first moments after
  // the window opens was never stored at all — the effect had already decided
  // there was nowhere to file it.
  // Reactive: it lands after an await, and the effect below has to re-run when
  // it does. Left as a plain variable, a draft typed in the first moments after
  // the window opens was never stored at all — the effect had already decided
  // there was nowhere to file it and nothing told it to look again.
  let draftSession = $state('')

  onMount(async () => {
    let id = ''
    try {
      id = (await CurrentSessionID()) ?? ''
    } catch {
      return // engine not up: an empty composer beats a draft filed under nothing
    }
    draftSession = id
    try {
      const stored = JSON.parse(localStorage.getItem(DRAFT_KEY) ?? '{}')
      // Only into an empty composer: the read finishes after the window is
      // interactive, so a user who has already started typing must not have it
      // replaced by what they wrote last time.
      if (!draft && stored && stored.session === id && typeof stored.text === 'string') draft = stored.text
    } catch {
      // Unreadable or from an older shape — an empty composer is the fallback.
    }
  })

  $effect(() => {
    // Before the session is known there is nothing to file the draft under, and
    // writing it anyway is how it ends up restored into the wrong chat.
    if (!draftSession) return
    try {
      if (draft) localStorage.setItem(DRAFT_KEY, JSON.stringify({ session: draftSession, text: draft }))
      else localStorage.removeItem(DRAFT_KEY)
    } catch {
      // Nothing to do and nothing worth saying: the draft is still on screen.
    }
  })
  let modelMenuOpen = $state(false)
  let focusMenuOpen = $state(false)

  // The who-am-I-talking-to picker (§85). Roster fetched when the menu opens,
  // not held: hiring is dropping a file, and a list read at mount would miss
  // an agent hired while the app was running.
  let agentMenuOpen = $state(false)
  let officeChairs = $state<main.Chair[]>([])
  async function toggleAgentMenu() {
    agentMenuOpen = !agentMenuOpen
    if (agentMenuOpen) {
      try {
        officeChairs = await ListChairs()
      } catch {
        officeChairs = []
      }
    }
  }
  // Which shell the agent's commands run in: this machine's, or a WSL distro.
  //
  // On the composer row rather than on the Settings page because it changes
  // what a command *means* — the same line is a different program to cmd and to
  // bash — and the moment that matters is when you are about to approve one. A
  // setting you have to go looking for is one you find out about by having a
  // command fail.
  //
  // Hidden entirely when the machine offers only its own shell, which is every
  // machine without WSL: a picker with one item is furniture.
  let shellMenuOpen = $state(false)
  let shellOptions = $state<main.ShellOption[]>([])
  let currentShell = $state<main.ShellOption | null>(null)

  async function refreshShells() {
    try {
      const [options, current] = await Promise.all([Shells(), CurrentShell()])
      shellOptions = options
      currentShell = current
    } catch {
      shellOptions = []
      currentShell = null
    }
  }

  async function toggleShellMenu() {
    shellMenuOpen = !shellMenuOpen
    // Re-read on open rather than holding the list from mount: a distro can be
    // installed, renamed or unregistered while the app runs, and a stale list
    // offers a shell that is no longer there.
    if (shellMenuOpen) await refreshShells()
  }

  async function pickShell(setting: string) {
    shellMenuOpen = false
    if (currentShell?.setting === setting) return
    try {
      await SetShell(setting)
    } finally {
      await refreshShells()
    }
  }

  // Which automation engine the specialist is working on.
  //
  // Only drawn in that agent's chat, and only when there is genuinely a choice —
  // a picker offering the one engine you have is furniture, the same rule the
  // shell picker above keeps. It asks the catalog by *family* rather than
  // holding a list of engine ids here, so the day a third engine ships this
  // offers it without being edited.
  //
  // Choosing writes placement (App.UseEngine), which is the same `for:` list the
  // settings register edits and the same gate that decides which tools the agent
  // is handed. So the engine you did not pick is not in the model's tool list at
  // all — stronger than telling it which to prefer, and one truth rather than
  // two.
  const AUTOMATION_AGENT = 'automation'
  let engineMenuOpen = $state(false)
  let engines = $state<connect.Status[]>([])
  // The chip has three states and none of them is "gone":
  //   nothing connected   → ยังไม่ได้เชื่อม, and the menu is a way to Settings
  //   connected, unplaced → ยังไม่ได้เลือก, one click away from working
  //   placed              → the engine's name
  // It disappeared in the first of those twice, which is the state where it was
  // the only thing that could have explained an agent with no tools.
  const activeEngine = $derived(
    engines.find((e) => e.connected && e.for?.some((f) => f === `agent:${AUTOMATION_AGENT}`)) ?? null)
  const anyConnected = $derived(engines.some((e) => e.connected))

  async function refreshEngines() {
    if (cockpit.chair !== AUTOMATION_AGENT) {
      engines = []
      return
    }
    try {
      engines = (await EnginesFor('automation', AUTOMATION_AGENT)) as connect.Status[]
    } catch {
      engines = []
    }
  }

  async function toggleEngineMenu() {
    engineMenuOpen = !engineMenuOpen
    // Re-read on open: an engine can be connected in Settings while this chat
    // is sitting here, and a list held from mount would not know.
    if (engineMenuOpen) await refreshEngines()
  }

  async function pickEngine(id: string) {
    engineMenuOpen = false
    // An engine nobody connected cannot be picked — placing it would hand the
    // agent tools that fail on their first call. The row says so and offers the
    // register instead.
    if (!engines.find((e) => e.id === id)?.connected) {
      openSettingsAt('automation')
      return
    }
    if (activeEngine?.id === id) return
    engineTest = ''
    try {
      await UseEngine('automation', AUTOMATION_AGENT, id)
    } finally {
      await refreshEngines()
    }
  }

  // Does the engine actually answer, right now.
  //
  // Same shape as the model connection test on the settings page — one real
  // request, `ok:` / `err:` in one field — because it answers the same question
  // and a second spelling of a result would be a second thing to read.
  //
  // Worth having here rather than only in Settings: a token that worked when it
  // was pasted can stop working without anything on this screen changing. n8n's
  // keys in particular carry an expiry the user chose at creation, so "it was
  // fine yesterday" is not evidence. The menu is where you are standing when you
  // start to doubt it.
  let engineTest = $state('')
  let engineTesting = $state('')

  async function testEngine(id: string) {
    if (engineTesting) return
    engineTesting = id
    engineTest = ''
    try {
      const account = await VerifyConnection(id)
      engineTest = 'ok:' + (account?.login ?? '')
    } catch (err) {
      engineTest = 'err:' + String(err)
    } finally {
      engineTesting = ''
    }
  }

  // Follows the chat you are in: walking into ระบบออโตเมชั่น has to draw the chip,
  // and walking out of it has to stop.
  $effect(() => {
    void cockpit.chair
    void refreshEngines()
  })

  // Why the last "add folder" was refused, '' when there is nothing to say.
  // Cleared when the menu closes, so a stale refusal never greets the next open.
  let folderError = $state('')
  let ctxMenuOpen = $state(false)
  // The โหมดทำงาน picker (§106). Its list comes from the engine (cockpit.stances)
  // — which stances exist is Go's answer, and a copy here is the one that goes
  // stale — while the words come from the locale.
  let stanceMenuOpen = $state(false)
  // How each stance is drawn, keyed on the engine's id ('' is ลงมือ). One table
  // rather than three lookups, and `as const` on purpose: it makes the glyph a
  // literal the Icon union accepts and the two strings literal locale keys, so
  // adding a stance in Go and forgetting its words here is a compile error
  // rather than a button labelled `stance.plan`.
  //
  // A stance the engine sends that this table does not know falls back to
  // ลงมือ's row: a generic button beats a nameless one, and it is how a stance
  // added on the Go side first shows itself over here.
  const STANCE_VIEW = {
    '': { icon: 'wrench', label: 'stance.act', hint: 'stance.actHint' },
    plan: { icon: 'compass', label: 'stance.plan', hint: 'stance.planHint' },
    consult: { icon: 'brain', label: 'stance.consult', hint: 'stance.consultHint' },
  } as const
  const stanceView = (s: string) => STANCE_VIEW[s as keyof typeof STANCE_VIEW] ?? STANCE_VIEW['']
  // The chip's own row. Derived rather than {@const} in the markup: the chip is
  // no longer inside a block, and {@const} only lives in one.
  const activeStance = $derived(stanceView(cockpit.stance))
  // Which of the provider/model/think-level pickers inside the model-menu
  // popover is expanded — native <select> can't be forced to open its option
  // list upward (browser-controlled, not stylable), so these render as a
  // small custom dropdown instead, anchored with bottom:100% like the rest
  // of this popover.
  let openDropdown = $state<'approval' | 'provider' | 'model' | 'thinkLevel' | ''>('')

  // A model list is whatever the provider currently offers — Anthropic alone
  // returns a dozen names — so a menu that opens scrolled to the top shows the
  // user an arbitrary slice with their current model nowhere in it.
  function revealSelected(list: HTMLElement) {
    const selected = list.querySelector('.updrop-opt.selected')
    if (selected) selected.scrollIntoView({ block: 'nearest' })
  }

  // One glyph per approval mode, shown on the chip itself: which mode you are
  // in has to be readable without opening anything — that is the whole reason
  // it stopped living only in Settings — and a glyph costs no bar width.
  const approvalIcons: Record<string, IconName> = {
    'ask': 'hand', 'unsafe-only': 'shield', 'full-access': 'zap',
  }

  // Auto-grow the composer upward while typing (the composer is anchored at
  // the bottom, so extra height expands up). The ceiling is the stylesheet's
  // max-height alone — a second cap here only ever fought it, and a long
  // pasted prompt deserves the whole window, not 220px.
  // Reacts to every draft change so starter picks and post-send clears resize too.
  let inputEl = $state<HTMLTextAreaElement | null>(null)
  $effect(() => {
    void draft
    const el = inputEl
    if (!el) return
    el.style.height = 'auto'
    el.style.height = el.scrollHeight + 'px'
  })

  function closeMenusOnOutside(e: MouseEvent) {
    const el = e.target as HTMLElement
    if (modelMenuOpen && !el.closest('.model-pick')) { modelMenuOpen = false; openDropdown = '' }
    if (focusMenuOpen && !el.closest('.focus-pick')) { focusMenuOpen = false; folderError = '' }
    if (ctxMenuOpen && !el.closest('.ctx-pick')) ctxMenuOpen = false
    if (stanceMenuOpen && !el.closest('.stance-pick')) stanceMenuOpen = false
    if (openDropdown && !el.closest('.updrop')) openDropdown = ''
    if (palette && !el.closest('.pal-pick')) palette = ''
  }

  // Context meter: how full the model's context window is and what fills it.
  let ctx = $state<ContextBreakdown | null>(null)
  async function refreshContext() {
    try {
      ctx = await GetContextBreakdown()
    } catch {
      ctx = null // engine not ready yet — button hides itself
    }
  }
  // Refresh on mount and after every completed turn (message count settles).
  $effect(() => {
    void messages.length
    if (awaitingReply) return
    refreshContext()
  })
  const ctxPct = $derived(
    ctx && ctx.maxTokens > 0 ? Math.min(100, Math.round((ctx.usedTokens / ctx.maxTokens) * 100)) : 0,
  )
  function slicePct(tokens: number): string {
    if (!ctx || ctx.maxTokens <= 0) return '0%'
    return ((tokens / ctx.maxTokens) * 100).toFixed(1) + '%'
  }
  function fmtTokens(n: number): string {
    return n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n)
  }
  const ctxLabels = $derived<Record<string, string>>({
    system: t('chat.ctx_system'),
    tools: t('chat.ctx_tools'),
    messages: t('chat.ctx_messages'),
    free: t('chat.ctx_free'),
  })

  // Ticks once a second while a turn is in flight, so the running tool step's
  // elapsed counter ("· 12s") advances live.
  let now = $state(Date.now())
  $effect(() => {
    if (!awaitingReply) return
    const id = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(id)
  })
  function liveSecs(s: ToolStep): number {
    return Math.max(0, Math.round((now - s.startedAt) / 1000))
  }

  // Guided onboarding. The questions come from the engine, and picking one
  // SENDS IT as an ordinary message — so the answer streams, persists and
  // scrolls through exactly the same path as every other reply. There is no
  // second code path to keep in sync (ARCHITECTURE.md §42).
  let guideTopics = $state<{ id: string; question: string }[]>([])
  $effect(() => {
    // Re-fetch on language change: the engine returns them already localized.
    void i18n.locale
    GuideTopics().then((t) => (guideTopics = t)).catch(() => {})
  })
  // "Already asked" is read off the transcript rather than kept as state, so it
  // survives a reload for free and cannot drift from what is on screen.
  const remainingGuide = $derived(
    guideTopics.filter((g) => !messages.some((m) => m.role === 'user' && m.text === g.question)),
  )
  // Only while running on the built-in engine: a configured model answers for
  // itself, and canned options under a real reply would be noise.
  const guideOpen = $derived(
    model.provider === 'aetox' && !awaitingReply && messages.length > 0 && remainingGuide.length > 0,
  )

  // The empty chat says the room's own sentence, not one sentence for the whole
  // app (starters.ts). Which room is read off the open session — chair, then
  // project, then desk — so reopening an old chat from history gets the cards
  // of the room it was held in rather than the room the window was last in.
  const roomStarters = $derived(
    startersFor({ desk: cockpit.desk, chair: cockpit.chair, space: cockpit.space }),
  )

  // ...except in a chat with an office agent, where the opening belongs to the
  // agent and lives in its own folder (internal/subagent/starters.go). Asked
  // rather than looked up, because this window does not have the list: a worker
  // the user hired this morning gets its own cards without the app having heard
  // of it, which is the whole reason the file exists.
  //
  // The language goes with the question — the agent keeps one file per language
  // — so switching languages re-asks. A worker with no file of its own answers
  // empty, and the four above stand.
  let chairOpening = $state<subagent.StarterSet | null>(null)
  $effect(() => {
    const name = cockpit.chair
    const locale = i18n.locale
    if (!name) {
      chairOpening = null
      return
    }
    let live = true
    ChairStarters(name, locale)
      .then((set) => { if (live) chairOpening = set })
      .catch(() => { if (live) chairOpening = null })
    return () => { live = false }
  })

  const headline = $derived(chairOpening?.headline || t(roomStarters.headlineKey))
  const starters: { icon: IconName; title: string; prompt: string }[] = $derived(
    chairOpening && chairOpening.cards?.length
      ? chairOpening.cards.map((c) => ({
          // An agent may name any mark from the app's icon set, and may name
          // none. A name this build does not have would draw an empty box, so
          // it is treated as "none" rather than trusted — the file is written by
          // hand, and by someone who cannot see this list.
          icon: (c.icon && c.icon in ICONS ? c.icon : 'bot') as IconName,
          title: c.title,
          prompt: c.prompt,
        }))
      : roomStarters.starters.map((s) => ({ icon: s.icon, title: t(s.titleKey), prompt: t(s.promptKey) })),
  )

  function pickStarter(prompt: string) {
    draft = prompt
  }

  // Pinned auto-scroll (Claude Code/OpenCode behavior): while the user is at
  // the bottom, every new message / stream chunk / reasoning chunk / tool step
  // keeps the view pinned there. Scrolling up unpins so reading is never
  // hijacked; scrolling back down re-pins.
  let chatEl = $state<HTMLDivElement | null>(null)
  let pinnedToBottom = $state(true)
  let lastChatScrollTop = 0
  function onChatScroll() {
    const el = chatEl
    if (!el) return
    const fromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    // Any upward movement unpins, however small. The pin itself only ever
    // scrolls DOWN, so up can only be the user — and it must not be judged by
    // the 80px band below: a touchpad scrolls a few px per event, which never
    // cleared 80px before the next stream chunk snapped the view back to the
    // bottom. That loop read as "can't scroll up while it's replying".
    // The fromBottom guard covers the one non-user way scrollTop can drop:
    // the transcript shrinking (undo, session switch) clamps it — but a clamp
    // lands AT the bottom, and a real upward scroll never does.
    if (el.scrollTop < lastChatScrollTop - 1 && fromBottom > 2) pinnedToBottom = false
    else if (fromBottom < 80) pinnedToBottom = true
    lastChatScrollTop = el.scrollTop
  }
  $effect(() => {
    // every live-updating piece of the transcript re-triggers this
    void messages.length
    void streamingText
    void reasoningText
    void toolSteps.length
    void cockpit.todos.length
    void cockpit.ask
    void awaitingReply
    const el = chatEl
    if (!el || !pinnedToBottom) return
    // after DOM update, not before — otherwise we scroll to the old height
    requestAnimationFrame(() => (el.scrollTop = el.scrollHeight))
  })

  // The answer the user types into the question card itself.
  //
  // Separate from the composer's `draft` on purpose: they are two places you
  // can answer from, and sharing one buffer would mean text half-typed in the
  // composer appeared in the card the moment a question arrived.
  let askDraft = $state('')

  $effect(() => {
    // A new question starts with an empty field — a draft left over from the
    // previous one is an answer to something that is no longer being asked.
    void cockpit.ask?.question
    askDraft = ''
  })

  function submitOwnAnswer(e: Event) {
    e.preventDefault()
    const text = askDraft.trim()
    if (!text) return
    askDraft = ''
    answerAsk(text)
  }

  // Focus lands on the field as the card appears: being asked a question is
  // the one moment where typing is the expected next thing. Clicking an option
  // is unaffected.
  function focusOnMount(el: HTMLInputElement) {
    el.focus()
  }

  // Copy an AI reply as plain text. '✓' feedback resets after a moment.
  let copiedText = $state('')
  let copiedTimer: ReturnType<typeof setTimeout> | undefined
  async function copyMessage(text: string) {
    await navigator.clipboard.writeText(text)
    copiedText = text
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => (copiedText = ''), 1500)
  }

  // Re-running a turn ——————————————————————————————————————————————————————
  //
  // Only the newest exchange can be re-run. Regenerating an older one would
  // invalidate everything said after it, and there is no honest way to draw a
  // transcript whose middle has been replaced.
  const lastIndex = $derived(messages.length - 1)
  const canRerun = $derived(!awaitingReply && !cockpit.ask)
  // Editing the question re-runs the exchange, which only works when there is a
  // recorded exchange to replace. A failed turn was persisted nowhere, so its
  // affordance is Retry, not Edit.
  const canEditLast = $derived(
    canRerun && messages.at(-1)?.role === 'agent' && !messages.at(-1)?.failed,
  )

  // A turn that wrote files is the one case where answering again is dangerous:
  // its tools would run a second time on top of their own output. The revert is
  // not optional — it is what makes the re-run mean what the button says — so
  // the dialog asks whether to proceed at all, not whether to revert.
  let confirmRerun = $state<'regenerate' | 'edit' | ''>('')

  function askRegenerate() {
    if (!canRerun) return
    if (cockpit.undoFiles.length > 0) { confirmRerun = 'regenerate'; return }
    regenerateReply(false)
  }

  // Opens a file the turn produced on the agent's desk — the workbench panel
  // that already exists to the right, where a workbook becomes a grid, a
  // picture becomes a picture, and anything this app cannot render says so and
  // offers the program that can.
  //
  // This used to go straight to the OS. That made handing the file away the
  // default and looking at it the special case, which is backwards for a panel
  // whose whole job is showing finished work — and it meant the answer to
  // "what did I just get?" was a window from another application landing on
  // top of Aetox. Every pane the file can land in carries its own
  // open-externally button, so the way out is one click further in, not gone.
  async function openProducedFile(path: string) {
    await openFileTab(path)
  }

  // Dragging one does the same thing by hand — the workbench's drop target and
  // the composer both read this MIME type (ARCHITECTURE.md §80), so the card
  // can be aimed at either without a second button on a card this small.
  function onFileCardDragStart(e: DragEvent, path: string) {
    setTabDragPayload(e, 'file', path, path.split('/').pop() ?? path)
  }

  // Editing a question the agent already acted on has the same problem, plus a
  // deletion: the answer to the old wording is thrown away, not kept as a variant.
  let editingIndex = $state(-1)
  let editDraft = $state('')
  function startEdit(i: number, text: string) {
    editingIndex = i
    editDraft = text
  }
  function commitEdit() {
    if (!editDraft.trim()) return
    if (cockpit.undoFiles.length > 0) { confirmRerun = 'edit'; return }
    const text = editDraft
    editingIndex = -1
    resendEdited(text, false)
  }
  function confirmRerunNow() {
    const which = confirmRerun
    confirmRerun = ''
    if (which === 'regenerate') { regenerateReply(true); return }
    const text = editDraft
    editingIndex = -1
    resendEdited(text, true)
  }

  function submit() {
    // While the model is blocked on ask_user, typed text is the free-text answer.
    if (cockpit.ask) {
      if (draft.trim()) {
        answerAsk(draft)
        draft = ''
      }
      return
    }
    if (!draft.trim() && !cockpit.pendingImage && !cockpit.pendingContext) return
    onSend(draft)
    draft = ''
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
      return
    }
    // "/" on an empty composer opens the preset list — the placeholder has
    // promised this since before there was anything to show.
    if (e.key === '/' && draft.trim() === '') {
      e.preventDefault()
      palette = 'prompts'
    }
    // "@" opens the roster, wherever in the sentence it is typed. Only at a word
    // boundary, so an email address does not summon a menu.
    if (e.key === '@' && (draft === '' || /\s$/.test(draft))) {
      mentionOpen = true
      if (officeChairs.length === 0) ListChairs().then((c) => (officeChairs = c)).catch(() => {})
    }
    if (e.key === 'Escape' && mentionOpen) mentionOpen = false
  }

  // A delegation's steps are folded away once it has finished, and open while
  // it works. The two states want opposite things from the same rows: a running
  // delegate's steps ARE the evidence it is alive (§105.5), and a finished one's
  // are a record nobody asked to re-read — four of them stacked turned the
  // transcript into a wall ("มันติดกันจนดูยังไงไม่รู้").
  //
  // Keyed on the `task` call's ref so a row keeps its state as the list grows,
  // and holding only what the user has actually toggled: the default is
  // computed per render, so a delegation that finishes while open does not
  // slam shut under the pointer.
  let openSteps = $state<Record<string, boolean>>({})
  const stepsKey = (node: TimelineNode) => node.step.ref ?? node.step.label
  const stepsOpen = (node: TimelineNode) =>
    openSteps[stepsKey(node)] ?? node.step.state === 'run'
  function toggleSteps(node: TimelineNode) {
    openSteps[stepsKey(node)] = !stepsOpen(node)
  }
  // Tool calls only. Narration and thinking ride in the same list and are not
  // tools — counting them would inflate "used N tools" with sentences, the same
  // rule ownTools follows for the agent's own row.
  const toolCount = (node: TimelineNode) => node.children.filter((c) => !c.kind).length

  // Answering a parked delegate goes out as an ordinary chat message — the same
  // door the user's own typing uses, so the engine has one entrance and not two.
  function answerBgTask(id: string, answer: string) {
    onSend(t('chat.bgAnswerPrompt', { id, answer }))
  }

  // Addressing a worker from the composer: `@name` sends the message to that
  // worker as written, instead of to the assistant (subagent.Mention).
  //
  // A menu rather than a thing you have to know: the names are filenames, and
  // the roster grows whenever a file is dropped in, so nobody can be expected to
  // have them memorised. It is not the picker beside it — that one moves you
  // into a worker's room for the rest of the session; this is one sentence,
  // said in the room you are already standing in.
  let mentionOpen = $state(false)
  // What has been typed since the "@" being completed, so the list narrows as
  // you go. Read off the draft rather than tracked separately: backspacing past
  // the "@" has to close the menu, and a counter would have to be told.
  const mentionQuery = $derived.by(() => {
    const at = draft.lastIndexOf('@')
    if (at < 0) return null
    if (at > 0 && !/\s/.test(draft[at - 1])) return null
    const rest = draft.slice(at + 1)
    return /\s/.test(rest) ? null : rest.toLowerCase()
  })
  const mentionMatches = $derived(
    mentionQuery === null ? [] : officeChairs.filter((c) => c.name.toLowerCase().includes(mentionQuery)),
  )
  function insertMention(name: string) {
    const at = draft.lastIndexOf('@')
    draft = draft.slice(0, at) + '@' + name + ' '
    mentionOpen = false
    inputEl?.focus()
  }

  // '' = closed. The two composer buttons and the "/" key set it.
  let palette = $state<'' | 'all' | 'prompts'>('')
  function insertFromPalette(text: string) {
    draft = text
    palette = ''
    inputEl?.focus()
  }

  // One attach button for everything: images keep their thumbnail path, and a
  // clip or document is copied into the sandbox and handed over as a path the
  // tools can open. Splitting this across two buttons was the duplication the
  // owner spotted (ARCHITECTURE.md §38).
  async function attachViaDialog() {
    const path = await PickAttachment()
    if (!path) return
    if (fileKind(path) === 'image') await attachImageFromPath(path)
    else await attachFileFromPath(path)
  }

  // A file/browser tab dragged from the workbench (Workbench.svelte's
  // ondragstart) drops here and is staged as pending context.
  let dragOver = $state(false)
  function onComposerDragOver(e: DragEvent) {
    if (!e.dataTransfer?.types.includes(TAB_DRAG_MIME)) return
    e.preventDefault()
    dragOver = true
  }
  async function onComposerDrop(e: DragEvent) {
    const raw = e.dataTransfer?.getData(TAB_DRAG_MIME)
    dragOver = false
    if (!raw) return
    e.preventDefault()
    const { kind, ref, label } = JSON.parse(raw) as { kind: 'file' | 'browser'; ref: string; label: string }
    await attachTabContext(kind, ref, label)
  }

  // Ctrl+V an image into the composer.
  //
  // Every attach route in this app went through a native picker, which is the
  // one thing a screenshot on the clipboard cannot satisfy — so the most
  // ordinary way there is to show the assistant something did nothing at all,
  // silently. That includes a chart copied out of an answer with the drawing's
  // own คัดลอก button: the copy worked, and there was nowhere to put it.
  //
  // Text paste is left entirely alone — no preventDefault, no interception —
  // so pasting a prompt still behaves like a textarea.
  async function onComposerPaste(e: ClipboardEvent) {
    const item = Array.from(e.clipboardData?.items ?? []).find((i) => i.type.startsWith('image/'))
    if (!item) return
    const file = item.getAsFile()
    if (!file) return
    e.preventDefault()
    await attachImageFromClipboard(file)
  }

  // Runs the command in a shell-tagged code block the user clicked Run on.
  // Output lands in a <pre> appended inside the same block — inserted with
  // textContent, never innerHTML: command output is untrusted text, and this
  // element lives inside {@html} markup that DOMPurify has already left.
  async function runCodeBlock(runBtn: HTMLButtonElement) {
    if (runBtn.dataset.running) return
    const block = runBtn.closest('.codeblock')
    const command = block?.querySelector('code')?.textContent ?? ''
    if (!block || !command.trim()) return
    runBtn.dataset.running = '1'
    runBtn.textContent = t('chat.runningCode')
    let out = block.querySelector<HTMLPreElement>('.code-run-out')
    if (!out) {
      out = document.createElement('pre')
      out.className = 'code-run-out'
      block.appendChild(out)
    }
    try {
      const res = await RunChatCommand(command)
      out.textContent = res.output
      out.classList.toggle('failed', !res.success)
      runBtn.textContent = res.success ? t('chat.ranCode') : t('chat.runFailed')
    } catch (err) {
      out.textContent = String(err)
      out.classList.add('failed')
      runBtn.textContent = t('chat.runFailed')
    } finally {
      delete runBtn.dataset.running
      setTimeout(() => (runBtn.textContent = t('chat.runCode')), 2500)
    }
  }

  // Copy or save the drawing whose button was clicked. The PNG carries the
  // theme's real colours (drawingExport bakes them), so what lands on the
  // clipboard is what the user is looking at. Both paths flash their verdict
  // on the button itself, same as the code-copy button does.
  async function exportDrawing(button: HTMLButtonElement) {
    if (button.dataset.busy) return
    const svg = button.closest('.drawing-box')?.querySelector('svg')
    if (!svg) return
    const isCopy = button.classList.contains('drawing-copy')
    const idle = isCopy ? t('chat.copyDrawing') : t('chat.saveDrawing')
    button.dataset.busy = '1'
    try {
      if (isCopy) {
        await copyDrawing(svg)
        button.textContent = t('chat.copiedCode')
      } else {
        await saveDrawing(svg)
        button.textContent = t('chat.savedDrawing')
      }
    } catch {
      button.textContent = t('chat.drawingExportFailed')
    } finally {
      delete button.dataset.busy
      setTimeout(() => (button.textContent = idle), 1500)
    }
  }

  // Links in rendered markdown must not navigate the app's own webview away —
  // open them in a workbench browser tab instead.
  function onChatClick(e: MouseEvent) {
    const el = e.target as HTMLElement
    // buttons on a rendered code block ({@html} markup can't carry handlers)
    const runBtn = el.closest('.code-run')
    if (runBtn) {
      void runCodeBlock(runBtn as HTMLButtonElement)
      return
    }
    const copyBtn = el.closest('.code-copy')
    if (copyBtn) {
      const code = copyBtn.closest('.codeblock')?.querySelector('code')
      navigator.clipboard.writeText(code?.textContent ?? '').then(() => {
        copyBtn.textContent = t('chat.copiedCode')
        setTimeout(() => (copyBtn.textContent = t('chat.copyCode')), 1500)
      })
      return
    }
    const drawBtn = el.closest('.drawing-copy, .drawing-save')
    if (drawBtn) {
      void exportDrawing(drawBtn as HTMLButtonElement)
      return
    }
    const a = el.closest('a')
    const href = a?.getAttribute('href')
    if (!href || !/^https?:\/\//i.test(href)) return
    e.preventDefault()
    openUrlInWorkbench(href)
  }
</script>

<!-- "/" is the prompt list on its own button; Ctrl+K opens the same component in
     full mode (model, approval, tool counts, shortcuts) — those rows lost their
     button when "+" became the attach control, not their home. -->
<svelte:window
  onclick={modelMenuOpen || focusMenuOpen || palette ? closeMenusOnOutside : undefined}
  onkeydown={(e) => {
    if (isShortcut(e, 'palette')) {
      e.preventDefault()
      palette = palette === 'all' ? '' : 'all'
    }
    // Shift+Tab toggles the leash, as in Claude Code. Deliberately only
    // ask ↔ unsafe-only: full-access means no prompt ever again, which is not
    // something a stray keystroke should be able to turn on. It can still be
    // picked from the menu, and from it this tightens rather than cycles.
    if (e.shiftKey && e.key === 'Tab') {
      e.preventDefault()
      switchApprovalMode(model.approval === 'ask' ? 'unsafe-only' : 'ask')
    }
  }}
/>

<!-- icon/desc are optional per option: approval needs them (picking "full access"
     blind is the one mistake here that costs something), a provider name does
     not, so those callers pass neither and render exactly as before. -->
{#snippet upSelect(
  id: 'approval' | 'provider' | 'model' | 'thinkLevel',
  // `icon` is a name from the shared Icon set; `mark` is a provider's own brand
  // mark. Kept as separate fields rather than one overloaded string — the two
  // draw from different registries, and a provider named like an icon would
  // otherwise silently pick the wrong one.
  options: { value: string; label: string; icon?: string; mark?: string; desc?: string }[],
  current: string,
  onPick: (value: string) => void,
)}
  {@const active = options.find((o) => o.value === current)}
  <div class="updrop">
    <button
      type="button"
      class="ctrl updrop-trigger"
      onclick={(e) => { e.stopPropagation(); openDropdown = openDropdown === id ? '' : id }}
    >
      {#if active?.mark}
        <span class="ic"><ProviderMark name={active.mark} size={13} /></span>
      {:else if active?.icon}
        <span class="ic"><Icon name={active.icon as IconName} size={13} /></span>
      {/if}
      <span class="t">{active?.label ?? current}</span>
      <span class="caret"><Icon name={openDropdown === id ? 'chevronUp' : 'chevronDown'} size={12} /></span>
    </button>
    {#if openDropdown === id}
      <div class="updrop-list" use:revealSelected>
        {#each options as opt}
          <button
            type="button"
            class="updrop-opt"
            class:rich={!!opt.desc}
            class:selected={opt.value === current}
            onclick={(e) => { e.stopPropagation(); openDropdown = ''; onPick(opt.value) }}
          >
            {#if opt.mark}
              <span class="ic"><ProviderMark name={opt.mark} size={13} /></span>
            {:else if opt.icon}
              <span class="ic"><Icon name={opt.icon as IconName} size={13} /></span>
            {/if}
            <span class="t">{opt.label}</span>
            {#if opt.desc}<span class="d">{opt.desc}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/snippet}

{#snippet toolRow(s: ToolStep, live: boolean)}
  {#if s.kind === 'note'}
    <!-- The model's own words for what it is doing, in the position of the
         work it announces (§59). Plain text, not a status row. -->
    <div class="tool-note">{s.label}</div>
  {:else if s.kind === 'thinking'}
    <div class="tool-think"><span class="ic"><Icon name="brain" size={12} /></span> {t('chat.thoughtFor', { secs: s.secs ?? 1 })}</div>
  {:else}
  <div class="tool-step {s.state}">
    {#if s.state === 'run'}
      <span class="glyph spin"></span>
    {:else}
      <span class="glyph"><Icon name={s.state === 'done' ? 'check' : 'x'} size={12} /></span>
    {/if}
    <span class="lbl">{s.label}</span>
    {#if s.state === 'run' && live}
      <span class="secs">· {liveSecs(s)}s</span>
    {:else if s.secs}
      <span class="secs">· {s.secs}s</span>
    {/if}
    {#if s.added || s.removed}
      <!-- While it runs, only the climbing "+N" is real — the removed
           count isn't known until the file is actually written. -->
      <span class="tool-stat">
        <span class="add">+{s.added ?? 0}</span>
        {#if s.state !== 'run'}<span class="del">-{s.removed ?? 0}</span>{/if}
      </span>
    {/if}
    {#if s.error}<span class="tool-err">{s.error}</span>{/if}
  </div>
  {/if}
{/snippet}

{#snippet toolTimeline(steps: ToolStep[], live: boolean)}
  <div class="tool-steps">
    {#each steps as s}
      {@render toolRow(s, live)}
    {/each}
  </div>
{/snippet}

<!-- A delegation is drawn as a block of its own, titled with the sub-agent doing
     the work, with the brief it was given and the steps it ran inside it. The
     events all arrive on one channel, so without this a delegate's work reads as
     the agent's own (§44.5) — and it lives behind its own toggle, because what
     the agent did itself and what it handed to someone else are different
     questions. -->
{#snippet subagentTimeline(nodes: TimelineNode[], live: boolean)}
  <div class="tool-steps">
    {#each nodes as node}
        <!-- The same card the background tray draws (§105.5), and deliberately
             so: a delegation that finished inside its turn and one still
             running afterwards are the same event at two moments, and two
             visual languages for that taught the user they were different
             things. The card lost the argument for a bare left rail the first
             time it was tried — a rail cannot say "alive". -->
        <div class="bgw-card {node.step.state}">
          <div class="bgw-head">
            {#if node.step.state === 'run'}
              <span class="bgw-mark run"><Icon name="loaderCircle" size={15} /></span>
            {:else}
              <span class="bgw-mark {node.step.state === 'done' ? 'ok' : 'fail'}">
                <Icon name={node.step.state === 'done' ? 'check' : 'x'} size={15} />
              </span>
            {/if}
            <!-- Written the way the user would write it. An agent has one
                 address (owner, 12 ส.ค.): you reach doc by typing "@doc", and
                 when the assistant reaches doc on your behalf it has to look
                 like the same act, or the convention you were taught reads as
                 something only you do. A helper keeps its bare name — nobody
                 addresses one; it is the assistant's own hands. -->
            <b class="bgw-agent">{node.step.agent
              ? (isAgentNode(node) ? '@' + node.step.agent : node.step.agent)
              : t(isAgentNode(node) ? 'chat.agent' : 'chat.subagent')}</b>
            {#if node.step.state === 'run'}
              <span class="bgw-badge run">{t('bgw.running')}</span>
            {/if}
            <span class="bgw-meta">
              {#if node.step.state === 'run' && live}{liveSecs(node.step)}s
              {:else if node.step.secs}{node.step.secs}s{/if}
            </span>
          </div>
          <div class="bgw-brief">{node.step.label.replace(/^task\s*/, '')}</div>
          {#if node.step.brief}
            <!-- The brief is the whole reason a delegate did what it did, and it
                 is the one thing the user never otherwise sees: it is written by
                 the main agent, not typed by them. Clamped to a few lines; the
                 full text is the title. -->
            <div class="bgw-longbrief" title={node.step.brief}>{node.step.brief}</div>
          {/if}
          {#if node.children.length}
            <button class="reasoning-toggle bgw-toggle" onclick={() => toggleSteps(node)}>
              <span class="chev"><Icon name={stepsOpen(node) ? 'chevronDown' : 'chevronRight'} size={12} /></span>
              {t('chat.usedTools', { n: toolCount(node) })}
            </button>
            {#if stepsOpen(node)}
              <div class="bgw-steps">
                {#each node.children as child}
                  {@render toolRow(child, live)}
                {/each}
              </div>
            {/if}
          {/if}
          {#if node.step.error}<div class="tool-err">{node.step.error}</div>{/if}
        </div>
    {/each}
  </div>
{/snippet}

  <!-- Where this conversation belongs — above the branch, so it is drawn on an
       empty chat as well as a full one. Inside the branch it was only ever
       drawn once a message existed, which is precisely backwards: a chat opened
       from the office or from a project is empty at the moment the user needs
       telling where they just landed, and without it the screen is the ordinary
       starter page. Reported as "มันพาเด้งมาหน้าหลัก แล้วโปรเจคก็หายไปเลย". -->
  {#if cockpit.chair}
    <div class="chair-strip">
      <Icon name="bot" size={14} />
      <span class="who">{t('chat.talkingTo', { name: cockpit.chair })}</span>
      <button type="button" class="back-office" onclick={() => setActiveView('office')}>{t('chat.backToOffice')}</button>
    </div>
  {:else if cockpit.space}
    <!-- A trail, not a sentence: the project is the level above this chat, so
         naming both in order says where you are and gets you back in one click.
         The chat's own name is dropped until it has one — a chat with no first
         message yet has no title, and "โปรเจกต์ /" trailing into nothing is
         worse than a breadcrumb of one. -->
    <nav class="chair-strip crumb-strip">
      <Icon name="folder" size={13} />
      <button type="button" class="crumb-up" onclick={() => setActiveView('projects')}>{cockpit.space}</button>
      {#if spaceChatTitle}
        <span class="crumb-sep">/</span>
        <span class="crumb-here">{spaceChatTitle}</span>
      {/if}
    </nav>
  {/if}

  {#if messages.length === 0}
    <div class="empty-state">
      <Logo size={56} />
      <h2>{headline}</h2>
      <div class="starter-grid">
        {#each starters as s}
          <button class="starter-card" onclick={() => pickStarter(s.prompt)}>
            <span class="ic"><Icon name={s.icon} size={18} /></span>
            <span class="title">{s.title}</span>
          </button>
        {/each}
      </div>
    </div>
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <!-- delegated click target is the <a> tags rendered inside .markdown-body, already interactive -->
    <div class="chat" bind:this={chatEl} onscroll={onChatScroll} onclick={onChatClick}>
    <div class="chat-inner">
      <!-- A session the engine refused to open. It says why — the folder moved,
           the desk file is gone — and until this existed it said it to nobody:
           the click just did nothing, which reads as a broken row rather than a
           chat that needs something put back. -->
      {#if cockpit.sessionError}
        <div class="session-error">
          <span class="ic"><Icon name="alertTriangle" size={14} /></span>
          <span>{cockpit.sessionError}</span>
        </div>
      {/if}
      {#each messages as m, i}
        <!-- A message sent into a running turn belongs below what has already
             streamed, not above it: it was said at that point, and drawn at the
             top it reads as something the assistant has not answered yet. The
             column is a flex box, so ordering is the whole fix — the array
             stays chronological and the index stays the index. -->
        <div class="msg {m.role === 'user' ? 'user' : 'bot'}" class:during-turn={m.duringTurn}>
          <div class="bubble">
            {#if m.role === 'agent' && m.tag}
              <div class="name"><span class="tag think">{m.tag}</span></div>
            {/if}
            {#if m.imageDataUrl}
              <img src={m.imageDataUrl} alt="" class="msg-image" />
            {/if}
            {#if m.attachLabel}
              <!-- No body: a clip or a PDF was handed over as a path, so there
                   is nothing to show without opening it. -->
              <div class="attach-card">
                <div class="attach-head">
                  <span class="ic"><Icon name={fileIcon(m.attachKind)} size={13} /></span>
                  <span class="attach-name">{m.attachLabel}</span>
                </div>
              </div>
            {/if}
            {#if m.contextLabel}
              <div class="attach-card">
                <div class="attach-head">
                  <span class="ic"><Icon name="paperclip" size={13} /></span>
                  <span class="attach-name">{m.contextLabel}</span>
                </div>
                {#if m.contextPreview}<pre class="attach-body">{m.contextPreview}</pre>{/if}
              </div>
            {/if}
            <!-- Asked in the same terms as the buttons inside it. `m.steps`
                 alone drew the row for a turn whose only steps were narration,
                 and then had nothing to put in it. -->
            {#if m.reasoning || ownTools(m.steps ?? []).length || delegated(m.steps ?? []).length}
              <div class="meta-row">
                {#if m.reasoning}
                  <!-- Each toggle carries the mark of what it opens, so the row
                       is told apart by shape before the count is read. Same
                       icon each concept already uses elsewhere: brain is the
                       live thinking row above, wrench is the workbench tools
                       tab. -->
                  <button class="reasoning-toggle" onclick={() => togglePanel(i, 'think')}>
                    <span class="chev"><Icon name={openPanel[i] === 'think' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="brain" size={12} /></span>
                    {m.thinkSecs ? t('chat.thoughtFor', { secs: m.thinkSecs }) : t('chat.thoughtDone')}
                  </button>
                {/if}
                {#if ownTools(m.steps ?? []).length}
                  <button class="reasoning-toggle" onclick={() => togglePanel(i, 'tools')}>
                    <span class="chev"><Icon name={openPanel[i] === 'tools' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="wrench" size={12} /></span>
                    {toolsLabel(m.steps ?? [])}
                  </button>
                {/if}
                {#if delegatedAgents(m.steps ?? []).length}
                  <button class="reasoning-toggle" onclick={() => togglePanel(i, 'agents')}>
                    <span class="chev"><Icon name={openPanel[i] === 'agents' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="userRound" size={12} /></span>
                    {delegationLabel(delegatedAgents(m.steps ?? []), 'chat.usedAgents')}
                  </button>
                {/if}
                {#if delegatedHelpers(m.steps ?? []).length}
                  <button class="reasoning-toggle" onclick={() => togglePanel(i, 'subs')}>
                    <span class="chev"><Icon name={openPanel[i] === 'subs' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="bot" size={12} /></span>
                    {delegationLabel(delegatedHelpers(m.steps ?? []), 'chat.usedSubagents')}
                  </button>
                {/if}
              </div>
              {#if m.reasoning && openPanel[i] === 'think'}
                <div class="reasoning-body">{m.reasoning}</div>
              {/if}
              {#if m.steps?.length && openPanel[i] === 'tools'}
                {@render toolTimeline(ownSteps(m.steps), false)}
              {/if}
              {#if m.steps?.length && openPanel[i] === 'agents'}
                {@render subagentTimeline(delegatedAgents(m.steps), false)}
              {/if}
              {#if m.steps?.length && openPanel[i] === 'subs'}
                {@render subagentTimeline(delegatedHelpers(m.steps), false)}
              {/if}
            {/if}
            {#if editingIndex === i}
              <!-- The question itself becomes the editor, in place: what is being
                   changed is the message, not a copy of it in a dialog. -->
              <!-- svelte-ignore a11y_autofocus -->
              <textarea
                class="msg-edit"
                autofocus
                bind:value={editDraft}
                onkeydown={(e) => {
                  if (e.key === 'Escape') { editingIndex = -1; return }
                  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); commitEdit() }
                }}
              ></textarea>
              <div class="msg-edit-actions">
                <button type="button" class="ctrl" onclick={() => (editingIndex = -1)}>{t('chat.cancelEdit')}</button>
                <button type="button" class="ctrl edit-send" onclick={commitEdit}>{t('chat.saveEdit')}</button>
              </div>
            {:else}
              <div class="markdown-body">{@html renderMarkdown(m.text)}</div>
            {/if}
            {#if m.producedFiles?.length}
              <!-- The deliverable, offered where it was asked for. Before this
                   the file existed and the answer named it, and getting to it
                   meant opening the file panel and finding it in the tree —
                   four clicks from a product that promises finished work. -->
              <div class="msg-files">
                {#each m.producedFiles as path}
                  <button
                    type="button" class="filecard" title={path}
                    onclick={() => openProducedFile(path)}
                    draggable="true" ondragstart={(e) => onFileCardDragStart(e, path)}
                  >
                    <span class="ic"><Icon name="fileText" size={16} /></span>
                    <span class="fc-name">{path.split('/').pop() ?? path}</span>
                    <span class="fc-open">{t('chat.openFile')}</span>
                  </button>
                {/each}
              </div>
            {/if}
            {#if m.revertedFiles?.length}
              <!-- An answer that had quietly undone six files would be the worse
                   surprise, so the bubble says it did. -->
              <div class="msg-reverted" title={m.revertedFiles.join('\n')}>
                <Icon name="undo2" size={12} /> {t('chat.revertedFiles', { count: String(m.revertedFiles.length) })}
              </div>
            {/if}
            {#if m.error}
              <!-- A re-run that failed. The answer above is still the real one. -->
              <div class="msg-error">{m.error}</div>
            {/if}
            <!-- Actions first, timestamp last (owner, 2026-08-14). What you can
                 do to a reply is what the row is for; when it happened is a
                 footnote to it. -->
            <div class="time">
              {#if m.role === 'agent' && m.text && !m.failed}
                <button type="button" class="msg-copy icobtn tiny" aria-label={t('chat.copy')}
                  data-tip={t('chat.copy')}
                  onclick={() => copyMessage(m.text)}>
                  <Icon name={copiedText === m.text ? 'check' : 'copy'} size={13} />
                </button>
              {/if}
              {#if m.role === 'agent' && m.id && m.text && !m.failed}
                <!-- Whether the answer was any good. Nothing else in the app
                     can say: a tool that did not error is not an answer that
                     helped, and this is the only signal a person can give
                     that means what it says. Two buttons, no scale — a rating
                     nobody actually gave is worse than no rating. -->
                <button
                  type="button" class="msg-copy msg-rate icobtn tiny" class:rated={m.rating === 'good'}
                  aria-label={t('chat.rateGood')} data-tip={t('chat.rateGood')} aria-pressed={m.rating === 'good'}
                  onclick={() => rateReply(m, 'good')}>
                  <Icon name="thumbsUp" size={13} />
                </button>
                <button
                  type="button" class="msg-copy msg-rate icobtn tiny" class:rated={m.rating === 'bad'}
                  aria-label={t('chat.rateBad')} data-tip={t('chat.rateBad')} aria-pressed={m.rating === 'bad'}
                  onclick={() => rateReply(m, 'bad')}>
                  <Icon name="thumbsDown" size={13} />
                </button>
              {/if}
              {#if m.role === 'agent' && i === lastIndex && (m.variants?.length ?? 0) > 1}
                <!-- Which answer of the several this question has had is on
                     screen. Switching also rewrites what the conversation
                     continues from — picking one and being replied to as if you
                     had picked the other is the trap this avoids. -->
                <span class="variant-pick">
                  <button
                    type="button" class="msg-copy icobtn tiny" aria-label={t('chat.prevVariant')} data-tip={t('chat.prevVariant')}
                    disabled={!canRerun || (m.activeVariant ?? 0) <= 0}
                    onclick={() => switchVariant((m.activeVariant ?? 0) - 1)}
                  ><Icon name="arrowLeft" size={13} /></button>
                  <span class="variant-n">{t('chat.variantCount', {
                    n: String((m.activeVariant ?? 0) + 1), total: String(m.variants?.length ?? 1),
                  })}</span>
                  <button
                    type="button" class="msg-copy icobtn tiny" aria-label={t('chat.nextVariant')} data-tip={t('chat.nextVariant')}
                    disabled={!canRerun || (m.activeVariant ?? 0) >= (m.variants?.length ?? 1) - 1}
                    onclick={() => switchVariant((m.activeVariant ?? 0) + 1)}
                  ><Icon name="arrowRight" size={13} /></button>
                </span>
              {/if}
              {#if m.role === 'agent' && m.failed && m.failedText}
                <!-- A turn that never reached the model. The question above is
                     still the question, so only this bubble is replaced. -->
                <button
                  type="button" class="msg-act retry icobtn tiny" disabled={!canRerun}
                  aria-label={t('chat.retry')} data-tip={t('chat.retry')}
                  onclick={() => retryFailedTurn(i)}
                ><Icon name="rotateCw" size={13} /></button>
              {:else if m.role === 'agent' && i === lastIndex && m.text}
                <button
                  type="button" class="msg-act icobtn tiny" disabled={!canRerun}
                  aria-label={t('chat.regenerate')} data-tip={t('chat.regenerate')}
                  onclick={askRegenerate}
                ><Icon name="refreshCw" size={13} /></button>
              {/if}
              {#if m.role === 'user' && i === lastIndex - 1 && canEditLast && editingIndex !== i}
                <button
                  type="button" class="msg-copy icobtn tiny" aria-label={t('chat.editMessage')} data-tip={t('chat.editMessage')}
                  onclick={() => startEdit(i, m.text)}
                ><Icon name="pencil" size={13} /></button>
              {/if}
              <span class="msg-time">{m.time}</span>
            </div>
          </div>
        </div>
      {/each}

      {#if guideOpen}
        <!-- Same card as ask_user: a lettered list Aetox is offering, not chips
             floating loose in the transcript. Owner asked for A/B/C/D and the
             component already existed — reused wholesale, no new CSS. -->
        <!-- guide-card marks this as an offer rather than an answer, so
             scroll targeting can tell them apart -->
        <div class="msg bot guide-card">
          <div class="bubble">
            <div class="ask-panel">
              <div class="ask-q">{t('guide.intro')}</div>
              <div class="ask-opts">
                {#each remainingGuide as g, i (g.id)}
                  <button type="button" class="ask-opt" onclick={() => onSend(g.question)}>
                    <span class="ask-key">{String.fromCharCode(65 + i)}</span>
                    <span class="ask-label">{g.question}</span>
                  </button>
                {/each}
              </div>
            </div>
          </div>
        </div>
      {/if}

      {#each queuedMessages as q, i}
        <div class="msg user queued">
          <div class="bubble">
            {q}
            <button class="queued-drop" aria-label={t('chat.removeQueued')} onclick={() => queuedMessages.splice(i, 1)}><Icon name="x" size={12} /></button>
          </div>
        </div>
      {/each}

      {#if awaitingReply}
        <div class="msg bot">
          <div class="bubble typing-bubble">
            <div class="typing-row">
              {#if liveStatus}
                <span class="typing-status">{liveStatus}</span>
              {/if}
              <span class="typing-dots"><span></span><span></span><span></span></span>
            </div>
            {#if reasoningText || doneOwnTools.length || doneAgents.length || doneHelpers.length}
              <div class="meta-row">
                <!-- The same marks the finished bubble's toggles carry
                     (brain / wrench / userRound / bot). These are the identical
                     rows one moment earlier, and without the marks the row
                     visibly changed shape the instant the turn ended. -->
                {#if reasoningText}
                  <button class="reasoning-toggle" onclick={() => (livePanel = livePanel === 'think' ? '' : 'think')}>
                    <span class="chev"><Icon name={livePanel === 'think' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="brain" size={12} /></span>
                    {t('chat.thinking')}
                  </button>
                {/if}
                {#if doneOwnTools.length}
                  <button class="reasoning-toggle" onclick={() => (livePanel = livePanel === 'tools' ? '' : 'tools')}>
                    <span class="chev"><Icon name={livePanel === 'tools' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="wrench" size={12} /></span>
                    {toolsLabel(liveDone)}
                  </button>
                {/if}
                {#if doneAgents.length}
                  <button class="reasoning-toggle" onclick={() => (livePanel = livePanel === 'agents' ? '' : 'agents')}>
                    <span class="chev"><Icon name={livePanel === 'agents' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="userRound" size={12} /></span>
                    {delegationLabel(doneAgents, 'chat.usedAgents')}
                  </button>
                {/if}
                {#if doneHelpers.length}
                  <button class="reasoning-toggle" onclick={() => (livePanel = livePanel === 'subs' ? '' : 'subs')}>
                    <span class="chev"><Icon name={livePanel === 'subs' ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="ic"><Icon name="bot" size={12} /></span>
                    {delegationLabel(doneHelpers, 'chat.usedSubagents')}
                  </button>
                {/if}
              </div>
              {#if reasoningText && livePanel === 'think'}
                <div class="reasoning-body">{reasoningText}</div>
              {/if}
              {#if doneOwn.length && livePanel === 'tools'}
                {@render toolTimeline(doneOwn, false)}
              {/if}
              {#if doneAgents.length && livePanel === 'agents'}
                {@render subagentTimeline(doneAgents, false)}
              {/if}
              {#if doneHelpers.length && livePanel === 'subs'}
                {@render subagentTimeline(doneHelpers, false)}
              {/if}
            {/if}
            <!-- The checklist used to be drawn here, inside the live block, and
                 that is why it had to be wiped at both ends of every turn: it
                 read as "what is happening now", so last turn's plan reappeared
                 over this turn's question. It is on the strip above the input
                 now, under แผน, where it outlives the turn that wrote it. One
                 place only — drawing it here as well would put the same plan on
                 screen twice and make one of them go stale. -->
            {#if liveNote}
              <div class="tool-note headline">{liveNote.label}</div>
            {/if}
            <!-- What is running stays on screen, delegations first: a sub-agent
                 is the slowest thing in a turn and the one the user most wants
                 to see moving. -->
            {#if runningSubs.length > 0}
              {@render subagentTimeline(runningSubs, true)}
            {/if}
            {#if runningOwn.length > 0}
              {@render toolTimeline(runningOwn, true)}
            {/if}
            {#if streamingText}
              <div class="markdown-body">{@html renderStreamingMarkdown(streamingText)}</div>
            {/if}
            {#if cockpit.ask}
              <div class="ask-panel">
                <div class="ask-q">{cockpit.ask.question}</div>
                <div class="ask-opts">
                  {#each cockpit.ask.options as opt, i}
                    <button type="button" class="ask-opt" onclick={() => answerAsk(opt)}>
                      <span class="ask-key">{String.fromCharCode(65 + i)}</span>
                      <span class="ask-label">{opt}</span>
                    </button>
                  {/each}
                  <!-- The free-text answer is the last row of the same list,
                       not a second widget under it: it is one more way to
                       answer the one question, so it carries the same metrics
                       and the same key slot as the options it sits with.
                       Answering here was always possible — but only through the
                       composer at the far bottom of the window, which is why
                       the card used to spend a line pointing away from itself. -->
                  <form class="ask-opt ask-own" onsubmit={submitOwnAnswer}>
                    <span class="ask-key"><Icon name="pencil" size={12} /></span>
                    <input
                      class="ask-own-input"
                      bind:value={askDraft}
                      placeholder={t('chat.askOwnPlaceholder')}
                      aria-label={t('chat.askOwnPlaceholder')}
                      use:focusOnMount
                    />
                    <button
                      type="submit" class="ask-own-send"
                      disabled={!askDraft.trim()} aria-label={t('chat.askOwnSend')}
                    ><Icon name="sendHorizontal" size={14} /></button>
                  </form>
                </div>
              </div>
            {/if}
          </div>
        </div>
      {/if}

      {#if task.steps.length > 0}
        <TaskTimeline steps={task.steps} elapsed={task.elapsed} />
      {/if}
    </div>
    </div>
  {/if}

  {#if confirmRerun}
    <ConfirmDialog
      title={confirmRerun === 'regenerate' ? t('chat.regenTitle') : t('chat.editTitle')}
      message={confirmRerun === 'regenerate'
        ? t('chat.regenMessage', { count: String(cockpit.undoFiles.length) })
        : t('chat.editMessageBody', { count: String(cockpit.undoFiles.length) })}
      detail={cockpit.undoFiles.join('\n')}
      confirmLabel={confirmRerun === 'regenerate' ? t('chat.regenConfirm') : t('chat.editConfirm')}
      onConfirm={confirmRerunNow}
      onCancel={() => (confirmRerun = '')}
    />
  {/if}

  <div class="composer">
    <!-- Work that outlived the turn that started it (§105). Below the
         transcript rather than inside it: the failure this fixes is work you
         cannot see, and a card that scrolls away with the history is one more
         way not to see it. -->
    <BackgroundWork
      tasks={cockpit.backgroundTasks}
      steps={cockpit.backgroundSteps}
      onAnswer={answerBgTask}
    />
    <!-- Side work the agent flagged (suggest_task): each chip starts its own
         fresh session on click. Lives on the composer, not in the transcript —
         a suggestion is pending input, not part of what was said. -->
    {#if cockpit.taskChips.length > 0}
      <div class="task-chips">
        {#each cockpit.taskChips as chip (chip.id)}
          <div class="task-chip" title={chip.tldr || chip.title}>
            <button class="task-chip-start" onclick={() => startTaskChip(chip)}>
              <Icon name="plus" size={11} />
              <span class="task-chip-title">{chip.title}</span>
            </button>
            <button class="task-chip-drop" aria-label={t('chat.dismissTask')} onclick={() => dismissTaskChip(chip.id)}>
              <Icon name="x" size={11} />
            </button>
          </div>
        {/each}
      </div>
    {/if}
    <!-- Scrolling up unpins the transcript; this is the way back. Sits on the
         composer rather than inside .chat so it can't scroll away with it. -->
    {#if !pinnedToBottom && messages.length > 0}
      <button
        class="scroll-bottom" aria-label={t('chat.scrollToBottom')}
        onclick={() => { if (chatEl) chatEl.scrollTop = chatEl.scrollHeight }}
      ><Icon name="arrowDown" size={14} /></button>
    {/if}
    {#if needsApiKey}
      <div class="api-key-banner">
        <input
          class="ctrl"
          type="password"
          placeholder={t('chat.apiKeyPlaceholder', { provider: model.provider })}
          bind:value={apiKeyDraft}
          onkeydown={(e) => e.key === 'Enter' && submitApiKey()}
        />
        <button class="ctrl" onclick={submitApiKey}>{t('chat.saveKey')}</button>
      </div>
    {/if}
    <div class="focus-row">
      <!-- Only the workshop points at a project. The storefront is the door
           that works on the machine rather than in a folder (§19/§86), so a
           picker there offered a choice that door does not have — and the one
           it offered loudest, "ไม่โฟกัสโปรเจกต์", was the state it is always
           in. switchShell clears the focus on the way in, so this is hidden
           because there is nothing to show, not to hide something real. -->
      {#if shell.name === 'code'}
      <div class="focus-pick">
        {#if focusMenuOpen}
          <div class="focus-menu">
            <!-- Not in the โค้ด window (§86): that door is the workshop, and
                 work there belongs to a project. Offering "no project" there
                 put every coding chat into the unfocused bucket, which has no
                 row in the projects table — and the sidebar groups history by
                 project, so those chats had nowhere to be drawn and read as
                 lost. The storefront still has it: that door never focuses a
                 project at all, which is the whole of its half. -->
            {#if shell.name !== 'code'}
              <button type="button" class="focus-item" class:on={!cockpit.project.focused} onclick={() => { focusMenuOpen = false; clearProjectFocus() }}>
                <span class="ic"><Icon name="messageSquare" size={14} /></span> {t('chat.noProject')}
              </button>
              {#if cockpit.projects.length > 0}<div class="menu-sep"></div>{/if}
            {/if}
            {#each cockpit.projects.slice(0, 8) as p (p.key)}
              <button type="button" class="focus-item" class:on={cockpit.project.focused && p.active} onclick={() => { focusMenuOpen = false; openProject(p.path) }}>
                <span class="ic"><Icon name="folder" size={14} /></span><span class="t">{p.name}</span>
              </button>
            {/each}
            <div class="menu-sep"></div>
            <button type="button" class="focus-item" onclick={() => { focusMenuOpen = false; openFolder() }}>
              <span class="ic"><Icon name="folderOpen" size={14} /></span> {t('topbar.openFolder')}…
            </button>
            <!-- The extra folders live in the focus menu rather than a settings
                 page, because they answer the same question the menu already
                 answers: what can the AI reach right now. Split across two
                 screens, the list stops being the thing the user reads to know. -->
            {#if cockpit.project.focused}
              <div class="menu-sep"></div>
              <div class="folder-head">{t('chat.extraFolders')}</div>
              {#each cockpit.projectFolders as f (f.path)}
                <div class="focus-item folder-row" class:missing={f.missing}>
                  <span class="ic"><Icon name="folder" size={14} /></span>
                  <span class="t" title={f.path}>{f.name}</span>
                  {#if f.missing}<span class="folder-gone">{t('chat.folderMissing')}</span>{/if}
                  <button
                    type="button"
                    class="folder-drop"
                    aria-label={t('chat.removeFolder', { name: f.name })}
                    title={t('chat.removeFolder', { name: f.name })}
                    onclick={() => removeProjectFolder(f.path)}
                  ><Icon name="x" size={12} /></button>
                </div>
              {/each}
              <button type="button" class="focus-item" onclick={async () => { folderError = await addProjectFolder() }}>
                <span class="ic"><Icon name="plus" size={14} /></span> {t('chat.addFolder')}…
              </button>
              <!-- Shown in the menu, not as a toast: the refusal is about the
                   folder the user just picked, and it belongs next to the list
                   they picked it for. -->
              {#if folderError}<div class="folder-error">{folderError}</div>{/if}
              <div class="folder-note">{t('chat.extraFoldersNote')}</div>
            {/if}
          </div>
        {/if}
        <button type="button" class="focus-chip focus-btn" onclick={() => (focusMenuOpen = !focusMenuOpen)}>
          <span class="ic"><Icon name={cockpit.project.focused ? 'folder' : (shell.name === 'code' ? 'folderOpen' : 'messageSquare')} size={13} /></span>
          {cockpit.project.focused
            ? cockpit.project.name
            : (shell.name === 'code' ? t('chat.pickProject') : t('chat.noProject'))}
          <span class="caret"><Icon name={focusMenuOpen ? 'chevronUp' : 'chevronDown'} size={12} /></span>
        </button>
      </div>
      {/if}
      <!-- Who this chat is with, and the way to a different who (§85). Same
           shape as the focus chip beside it: both answer "what am I pointed
           at right now". Picking someone always opens a NEW session — a desk
           or a chair is fixed for a session's life, so the switcher is a door
           to a fresh one, never a dial on this one.

           Not on the coding desk: the star gives โค้ด no path to the office,
           and a desk that can hand work to no one must not wear a button that
           offers to. The code desk talks to exactly one agent, so there is
           nothing to switch. -->
      {#if cockpit.desk !== 'coding'}
      <div class="focus-pick">
        {#if agentMenuOpen}
          <div class="focus-menu">
            <button type="button" class="focus-item" class:on={!cockpit.chair}
              onclick={() => { agentMenuOpen = false; if (cockpit.chair) newSessionAt('assistant') }}>
              <span class="ic"><Icon name="sparkles" size={14} /></span> {t('chat.mainAgent')}
            </button>
            {#if officeChairs.length > 0}<div class="menu-sep"></div>{/if}
            {#each officeChairs as c (c.name)}
              <button type="button" class="focus-item" class:on={cockpit.chair === c.name}
                title={c.description}
                onclick={() => { agentMenuOpen = false; if (cockpit.chair !== c.name) newChairSession(c.name) }}>
                <span class="ic"><Icon name="bot" size={14} /></span><span class="t">{c.name}</span>
              </button>
            {/each}
            <div class="folder-note">{t('chat.agentSwitchNote')}</div>
          </div>
        {/if}
        <button type="button" class="focus-chip focus-btn" onclick={toggleAgentMenu}>
          <span class="ic"><Icon name={cockpit.chair ? 'bot' : 'sparkles'} size={13} /></span>
          {cockpit.chair || t('chat.mainAgent')}
          <span class="caret"><Icon name={agentMenuOpen ? 'chevronUp' : 'chevronDown'} size={12} /></span>
        </button>
      </div>
      {/if}
      {#if cockpit.project.focused && cockpit.project.branch}<span class="focus-chip"><Icon name="gitBranch" size={11} /> {cockpit.project.branch}</span>{/if}
      <!-- Which automation engine the specialist works on.
           Drawn whenever the user has one at all, and NOT only when there are
           two to choose between — which is where the shell picker's rule was
           copied and should not have been. That rule holds when the chip is
           only a control; this one is also the answer to "what will this agent
           touch", and there is a state it is the only cure for.

           A connection reaches an agent only when it is placed on it by name
           (config.ConnectionsForAgent: "an agent is handed things on purpose,
           so silence is not a grant"). So the ordinary case — connect an
           engine, walk into this room — is an agent with no tools at all, and
           with the chip hidden there was nothing on screen that said so or
           could fix it. It reads "ยังไม่ได้เลือก" then, and one click is the
           whole repair. -->
      {#if engines.length > 0}
      <div class="focus-pick">
        {#if engineMenuOpen}
          <div class="focus-menu">
            {#each engines as e (e.id)}
              <div class="focus-row">
                <!-- Name over address: the name is what you are choosing
                     between, the address is how you notice you attached the
                     wrong machine. Both matter, and only one is the label.
                     An engine nobody connected still gets a row — saying
                     "ยังไม่ได้เชื่อม" and leading to the register is the whole
                     job of this menu when nothing is set up. -->
                <!-- One line, and the right edge always carries something: the
                     address when it is attached, "ยังไม่ได้เชื่อม" when it is
                     not. A row with a name on the left and nothing on the right
                     reads as an item floating in a box rather than a list. -->
                <button type="button" class="focus-item" class:on={activeEngine?.id === e.id}
                  onclick={() => pickEngine(e.id)}>
                  <span class="ic"><Icon name={e.connected ? 'gitBranch' : 'plug'} size={14} /></span>
                  <span class="t">{e.label}</span>
                  {#if e.connected}
                    <span class="tail">{(e.base_url ?? '').replace(/^https?:\/\//, '')}</span>
                  {:else}
                    <span class="tail warn">{t('settings.ghNotConnected')}</span>
                  {/if}
                </button>
                <!-- Same mark and same job as the model test on the settings
                     page: one real request to the thing, now. Beside the row
                     rather than inside it, so checking an engine never means
                     switching to it first. -->
                {#if e.connected}
                  <button
                    type="button" class="icobtn tiny"
                    title={t('settings.testConnection')} aria-label={t('settings.testConnection')}
                    disabled={engineTesting !== ''}
                    onclick={() => testEngine(e.id)}
                  >{#if engineTesting === e.id}…{:else}<Icon name="plugZap" size={13} />{/if}</button>
                {/if}
              </div>
            {/each}
            {#if engineTest}
              <div class="conn-test" class:ok={engineTest.startsWith('ok:')}>
                {#if engineTest.startsWith('ok:')}
                  <Icon name="check" size={13} /> {t('chat.engineOk')}{engineTest.slice(3) ? ` · ${engineTest.slice(3)}` : ''}
                {:else}
                  {engineTest.slice(4)}
                {/if}
              </div>
            {/if}
            <div class="menu-sep"></div>
            <!-- The way out to the register. Everything this menu cannot do —
                 connecting an engine, changing its address, disconnecting it —
                 lives there, and a menu that offers a choice without a way to
                 add to it is a dead end for the user who has none. -->
            <button type="button" class="focus-item"
              onclick={() => { engineMenuOpen = false; openSettingsAt('automation') }}>
              <span class="ic"><Icon name="settings" size={14} /></span>
              <span class="t">{t('automation.manage')}</span>
            </button>
            <div class="folder-note">
              {anyConnected ? t('chat.engineNote') : t('chat.engineNoEngineNote')}
            </div>
          </div>
        {/if}
        <!-- Marked whenever there is no engine in play, because that is not a
             neutral state: the agent has no tools and every answer it gives
             will be about what it cannot do. Nothing connected and connected-
             but-unpicked are different sentences with the same remedy — open
             this menu — so both wear the same mark and differ only in words. -->
        <button
          type="button" class="focus-chip focus-btn" class:unset={!activeEngine}
          title={activeEngine
            ? t('chat.engineTitle')
            : (anyConnected ? t('chat.engineNoneHint') : t('chat.engineNoEngineHint'))}
          onclick={toggleEngineMenu}
        >
          <span class="ic"><Icon name={activeEngine ? 'gitBranch' : 'alertTriangle'} size={13} /></span>
          {activeEngine?.label ?? (anyConnected ? t('chat.engineNone') : t('chat.engineNoEngine'))}
          <span class="caret"><Icon name={engineMenuOpen ? 'chevronUp' : 'chevronDown'} size={12} /></span>
        </button>
      </div>
      {/if}
      <!-- Which shell runs what the agent types. Only offered when there is
           something to choose between: on a machine with no WSL this row is
           unchanged from before the picker existed. -->
      {#if shellOptions.length > 1}
      <div class="focus-pick">
        {#if shellMenuOpen}
          <div class="focus-menu">
            {#each shellOptions as option (option.setting)}
              <button type="button" class="focus-item" class:on={option.selected}
                onclick={() => pickShell(option.setting)}>
                <span class="ic"><Icon name="terminal" size={14} /></span><span class="t">{option.label}</span>
              </button>
            {/each}
            <div class="folder-note">{t('chat.shellNote')}</div>
          </div>
        {/if}
        <button type="button" class="focus-chip focus-btn" title={t('chat.shellTitle')} onclick={toggleShellMenu}>
          <span class="ic"><Icon name="terminal" size={13} /></span>
          {currentShell?.label ?? ''}
          <span class="caret"><Icon name={shellMenuOpen ? 'chevronUp' : 'chevronDown'} size={12} /></span>
        </button>
      </div>
      {/if}
      <!-- Visible without opening the menu: a session that can reach outside its
           project must say so on the row that says what it is focused on, or
           the widening is something only the person who set it knows about. -->
      {#if cockpit.project.focused && cockpit.projectFolders.length > 0}
        <button
          type="button"
          class="focus-chip folder-count"
          title={cockpit.projectFolders.map((f) => f.path).join('\n')}
          onclick={() => (focusMenuOpen = !focusMenuOpen)}
        ><Icon name="folder" size={11} /> {t('chat.extraFolderCount', { count: String(cockpit.projectFolders.length) })}</button>
      {/if}
      <!-- Offered only when the last turn actually changed something, so it is
           never a button that does nothing. Disappears once pressed. -->
      {#if cockpit.undoFiles.length > 0}
        <button
          type="button"
          class="focus-chip undo-chip"
          title={cockpit.undoFiles.join('\n')}
          onclick={() => undoLastTurn()}
        ><Icon name="undo2" size={13} /> {t('chat.undoTurn', { count: String(cockpit.undoFiles.length) })}</button>
      {/if}
    </div>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- drag/drop target for a workbench tab; the textarea/buttons inside remain the real interactive elements -->
    <div class="box" class:drag-over={dragOver} ondragover={onComposerDragOver} ondragleave={() => (dragOver = false)} ondrop={onComposerDrop}>
      <!-- Inside the box, above the text, the way every chat app does it: what
           is attached is part of the message being written, not a banner
           floating above the thing you are typing in. -->
      {#if cockpit.pendingImage}
        <div class="attach-card">
          <div class="attach-head">
            <img src={cockpit.pendingImage.dataUrl} alt="" class="attach-thumb" />
            <span class="attach-name">{cockpit.pendingImage.relPath.split('/').pop()}</span>
            <button class="attach-remove" aria-label={t('chat.removeAttachment')} onclick={clearPendingImage}><Icon name="x" size={12} /></button>
          </div>
        </div>
      {/if}
      {#if cockpit.pendingFile}
        <div class="attach-card">
          <div class="attach-head">
            <span class="ic"><Icon name={fileIcon(cockpit.pendingFile.kind)} size={13} /></span>
            <span class="attach-name">{cockpit.pendingFile.label}</span>
            <button class="attach-remove" aria-label={t('chat.removeAttachment')} onclick={clearPendingFile}><Icon name="x" size={12} /></button>
          </div>
        </div>
      {/if}
      {#if cockpit.pendingContext}
        {@const preview = attachmentPreview(cockpit.pendingContext.content)}
        <div class="attach-card">
          <div class="attach-head">
            <span class="ic"><Icon name={cockpit.pendingContext.kind === 'file' ? 'fileText' : 'globe'} size={13} /></span>
            <span class="attach-name">{cockpit.pendingContext.label}</span>
            <button class="attach-remove" aria-label={t('chat.removeAttachment')} onclick={clearPendingContext}><Icon name="x" size={12} /></button>
          </div>
          <!-- The point of the card: what is actually going to the model,
               before it goes. An empty preview (a blank file) leaves the head
               alone rather than drawing an empty box under it. -->
          {#if preview}<pre class="attach-body">{preview}</pre>{/if}
        </div>
      {/if}
      <!-- The roster, while an "@" is being completed. Above the box because the
           composer sits at the bottom of the window and a list under it would
           open off-screen. -->
      {#if mentionOpen && mentionQuery !== null && mentionMatches.length > 0}
        <div class="mention-menu">
          {#each mentionMatches as c (c.name)}
            <button type="button" class="mention-item" title={c.description} onclick={() => insertMention(c.name)}>
              <span class="ic"><Icon name="bot" size={14} /></span>
              <span class="t">@{c.name}</span>
              <span class="d">{c.description}</span>
            </button>
          {/each}
          <!-- The switcher beside the composer lists these same five names, and
               a user reading both has every right to ask what the difference
               is. It answers that question the same way that menu answers its
               own — one line, in the menu, at the moment of choosing. -->
          <div class="folder-note">{t('chat.mentionNote')}</div>
        </div>
      {/if}
      <textarea
        class="input"
        rows="1"
        placeholder={cockpit.chair
          ? t('chat.inputToAgent', { name: cockpit.chair })
          : t('chat.inputPlaceholder', { key: shortcutLabel('palette') })}
        bind:this={inputEl}
        bind:value={draft}
        onkeydown={onKeydown}
        onpaste={onComposerPaste}
      ></textarea>
      <div class="tools">
        <!-- Attach stays leftmost (owner, 2026-08-14). It is the button that
             belongs to the text being written, so it sits against the text;
             everything after it is about how the turn will be run. -->
        <button
          class="icobtn" aria-label={t('chat.attachFile')} data-tip={t('chat.attachFile')}
          onclick={attachViaDialog}
        >+</button>
        <!-- โหมดทำงาน (§106) — how this turn runs, as against what is on the
             desk. On the input row rather than in the chip strip above, which
             is deliberate: every chip up there is a door to a NEW session
             ("never a dial on this one"), and this is the opposite.

             It always says which mode is on — owner's call, 2026-08-14, after
             seeing it drawn the other way. The first build made ลงมือ a bare
             glyph on the reasoning that a default should not demand attention;
             what that produced was a control you have to click to find out what
             it is doing. A dial that decides whether the assistant can touch
             your machine reads its own state out loud, in every position,
             including the ordinary one. -->
        <div class="stance-pick">
          {#if stanceMenuOpen}
            <div class="stance-menu">
              {#each cockpit.stances as s (s)}
                {@const v = stanceView(s)}
                <button
                  type="button" class="stance-item" class:on={cockpit.stance === s}
                  onclick={() => { stanceMenuOpen = false; setStance(s) }}
                >
                  <span class="ic"><Icon name={v.icon} size={14} /></span>
                  <span class="t">
                    <span class="nm">{t(v.label)}</span>
                    <span class="d">{t(v.hint)}</span>
                  </span>
                  {#if cockpit.stance === s}<span class="tick"><Icon name="check" size={13} /></span>{/if}
                </button>
              {/each}
              <div class="folder-note">{t('stance.note')}</div>
            </div>
          {/if}
          <!-- One shape in every position — the name is the control. `.on`
               only changes the colour, so the ordinary mode is legible and the
               ones that withhold tools are unmissable.

               A menu rather than click-to-cycle: cycling means arriving
               somewhere by pressing one time too many, and what this dial
               changes is whether the assistant can touch the machine. -->
          <button
            type="button" class="stance-chip" class:on={!!cockpit.stance}
            title={t('stance.title')} aria-label={t('stance.title')}
            onclick={(e) => { e.stopPropagation(); stanceMenuOpen = !stanceMenuOpen }}
          >
            <Icon name={activeStance.icon} size={13} />
            <span class="nm">{t(activeStance.label)}</span>
            <span class="caret"><Icon name={stanceMenuOpen ? 'chevronUp' : 'chevronDown'} size={11} /></span>
          </button>
        </div>
        <div class="pal-pick">
          {#if palette}
            <Palette
              mode={palette}
              oninsert={insertFromPalette}
              onclose={() => { palette = ''; inputEl?.focus() }}
              onopenmodel={() => { palette = ''; modelMenuOpen = true; refreshThinkLevels() }}
              onswitchthink={(lvl) => onSwitchThinkLevel(lvl)}
            />
          {/if}
          <button
            class="icobtn slash" class:active={palette !== ''}
            aria-label={t('palette.promptsTitle')} data-tip={t('palette.promptsTitle')}
            onclick={(e) => { e.stopPropagation(); palette = palette ? '' : 'prompts' }}
          >/</button>
        </div>
        {#if ctx && ctx.maxTokens > 0}
          <div class="ctx-pick">
            {#if ctxMenuOpen}
              <div class="ctx-menu">
                <div class="ctx-head">
                  <span class="t">{ctx.measured ? t('chat.contextWindow') : t('chat.contextForecast')}</span>
                  <span class="v">{fmtTokens(ctx.usedTokens)} / {fmtTokens(ctx.maxTokens)} ({ctxPct}%)</span>
                </div>
                <div class="ctx-track">
                  {#each ctx.slices.filter((s) => s.key !== 'free' && s.tokens > 0) as s (s.key)}
                    <div class="ctx-seg {s.key}" style="width:{slicePct(s.tokens)}"></div>
                  {/each}
                </div>
                {#each ctx.slices as s (s.key)}
                  <div class="ctx-row">
                    <span class="dot {s.key}"></span>
                    <span class="lbl">{ctxLabels[s.key] ?? s.key}</span>
                    <span class="val">{fmtTokens(s.tokens)}</span>
                    <span class="pct">{slicePct(s.tokens)}</span>
                  </div>
                {/each}
                {#if !ctx.measured}
                  <!-- Nothing has been sent yet. Without saying so, the tool
                       definitions read as tokens already spent rather than as
                       the floor every message starts from. -->
                  <div class="ctx-note">{t('chat.contextNotSent')}</div>
                {:else if ctx.cachedTokens > 0}
                  <!-- Most of a request is the same bytes as the round before
                       (system prompt, tool block), and the provider serves
                       those from cache at a fraction of full price. Without
                       this line the bar presents the whole prompt as paid at
                       full rate every round — which is where "Aetox eats
                       tokens" comes from. -->
                  <div class="ctx-note">{t('chat.contextCached', { cached: fmtTokens(ctx.cachedTokens) })}</div>
                {/if}
              </div>
            {/if}
            <button
              type="button"
              class="icobtn ctx-btn"
              class:active={ctxMenuOpen}
              aria-label={t('chat.contextWindow')}
              data-tip={t('chat.contextWindow')}
              onclick={() => { ctxMenuOpen = !ctxMenuOpen; if (ctxMenuOpen) refreshContext() }}
            >
              <svg viewBox="0 0 20 20" class="ring" aria-hidden="true">
                <circle cx="10" cy="10" r="8" class="bg" />
                <circle cx="10" cy="10" r="8" class="fg" stroke-dasharray="{(ctxPct / 100) * 50.27} 50.27" transform="rotate(-90 10 10)" />
              </svg>
              <span class="ctx-pct">{ctxPct}%</span>
            </button>
          </div>
        {/if}
        {#if model.provider}
          <div class="model-pick">
            {#if modelMenuOpen}
              <div class="model-menu">
                {#if model.warning || switchError}
                  <!-- The picker names a provider the engine never reached; without
                       this the menu showed "lmstudio / —" and looked fine. -->
                  <div class="mm-warn">
                    {#if switchError}
                      <!-- No fallback headline here: a thrown switch means no
                           engine at all, not "running the built-in one". -->
                      <span>{switchError}</span>
                    {:else}
                      <strong>{t('chat.providerFallback')}</strong>
                      <span>{model.warning}</span>
                    {/if}
                  </div>
                {/if}
                <!-- Top row on purpose: the knob changed most often, and the
                     only one on this menu with a safety consequence. -->
                <div class="mm-row">
                  <span class="lbl">{t('palette.approval')}</span>
                  {@render upSelect('approval', [
                    { value: 'ask', label: t('chat.approvalAsk'), icon: approvalIcons['ask'], desc: t('onboard.approvalAskDesc') },
                    { value: 'unsafe-only', label: t('chat.approvalUnsafeOnly'), icon: approvalIcons['unsafe-only'], desc: t('onboard.approvalUnsafeDesc') },
                    { value: 'full-access', label: t('chat.approvalFullAccess'), icon: approvalIcons['full-access'], desc: t('onboard.approvalFullDesc') },
                  ], model.approval, switchApprovalMode)}
                </div>
                <div class="mm-row">
                  <span class="lbl">{t('chat.provider')}</span>
                  {@render upSelect('provider', providers.map((p) => ({ value: p, label: p, mark: p })), model.provider, handleProviderChange)}
                </div>
                <div class="mm-row">
                  <span class="lbl">{t('chat.model')}</span>
                  {#if models && models.length > 0}
                    {@render upSelect('model', models.map((m) => ({ value: m, label: m })), model.modelName, handleModelChange)}
                  {:else}
                    <!-- No discoverable list — read-only; custom model ids are set in Settings -->
                    <span class="mm-static">{model.modelName || '—'}</span>
                  {/if}
                </div>
                <!-- Two, not one: a picker with a single entry is not a choice,
                     it just tells the user there is a setting they cannot move.
                     The command palette has always required two (Palette.svelte);
                     this row asked for one, so a model with exactly one real
                     level — gpt-5-pro, MiniMax M2.x, which cannot stop thinking
                     — drew a dropdown that did nothing when opened. -->
                {#if thinkLevels.length > 1}
                  <div class="mm-row">
                    <span class="lbl">{t('chat.thinkLevel')}</span>
                    {@render upSelect('thinkLevel', thinkLevels.map((lvl) => ({ value: lvl, label: lvl })), model.thinkLevel, onSwitchThinkLevel)}
                  </div>
                {/if}
              </div>
            {/if}
            <!-- Two things on this chip, because two things are worth being
                 wrong about: what Aetox may do without asking, and which brain
                 is answering. The model *name* is neither — it is the longest
                 string on the bar and the one that changes least, so it moves
                 into the chip's title, the menu below, and the Ctrl+K row that
                 names it. The provider mark keeps the second question answered
                 at a glance without spending the width. -->
            <button
              type="button" class="model-chip"
              title={model.modelName || model.provider}
              onclick={(e) => { e.stopPropagation(); modelMenuOpen = !modelMenuOpen; if (modelMenuOpen) { refreshThinkLevels(); EnabledProviders().then((p) => (providers = p)) } }}
            >
              <span class="mode-ic" class:danger={model.approval === 'full-access'}
                    title={t('settings.approvalTitle')}><Icon name={approvalIcons[model.approval] ?? 'hand'} size={14} /></span>
              <span class="pv"><ProviderMark name={model.provider} size={14} /></span>
              <!-- Same test as the menu row below, on purpose. Keyed off
                   model.thinkLevel instead, a model with exactly one real level
                   drew a badge for a setting the menu offers no way to change. -->
              {#if thinkLevels.length > 1 && model.thinkLevel}<span class="lvl">{model.thinkLevel}</span>{/if}
              <span class="caret"><Icon name={modelMenuOpen ? 'chevronUp' : 'chevronDown'} size={12} /></span>
            </button>
          </div>
        {/if}
        {#if awaitingReply}
          <!-- Typing into a running turn is supported — the text is handed to
               the turn in flight (Interject) rather than starting a second one —
               but the only button here was Stop, so the one gesture a person
               makes after typing threw the work away instead of sending. With a
               draft on screen the primary button sends again; the brake stays
               beside it, because the tool loop is unbounded and it is the
               user's only Ctrl+C. -->
          {#if draft.trim()}
            <button class="send stop secondary" aria-label={t('chat.stopTurn')} onclick={cancelTurn}><Icon name="square" size={12} /></button>
            <button class="send" aria-label={t('chat.sendIntoTurn')} title={t('chat.sendIntoTurn')} onclick={submit}>
              <Icon name="sendHorizontal" size={15} />
            </button>
          {:else}
            <button class="send stop" aria-label={t('chat.stopTurn')} onclick={cancelTurn}><Icon name="square" size={13} /></button>
          {/if}
        {:else}
          <button class="send" aria-label="Send" onclick={submit}><Icon name="sendHorizontal" size={15} /></button>
        {/if}
      </div>
    </div>
  </div>
