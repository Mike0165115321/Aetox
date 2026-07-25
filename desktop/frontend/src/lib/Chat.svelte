<script lang="ts">
  import type { ChatMessage, TaskState, ModelStatus, ToolStep, ContextBreakdown } from './types'
  import TaskTimeline from './TaskTimeline.svelte'
  import Palette from './Palette.svelte'
  import Logo from './Logo.svelte'
  import { onMount } from 'svelte'
  import {
    EnabledProviders, SupportedThinkLevels,
    ListModelsForProvider, RequiresAPIKey, HasAPIKey, PickAttachment,
    GetContextBreakdown,
  } from '../../wailsjs/go/main/App'
  import { t } from './i18n.svelte'
  import { renderMarkdown } from './markdown'
  import { openUrlInWorkbench } from './stores/workbench.svelte'
  import {
    cockpit, attachImageFromPath, clearPendingImage, attachTabContext, clearPendingContext,
    attachFileFromPath, clearPendingFile, fileKind, pushGuideExchange,
    openProject, openFolder, clearProjectFocus, cancelTurn, answerAsk,
  } from './stores/cockpit.svelte'

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
  let reasoningCollapsed = $state(false)
  // Per finished message: which persisted thinking panels are open (collapsed default).
  let expandedReasoning = $state<Record<number, boolean>>({})

  onMount(async () => {
    providers = await EnabledProviders()
  })

  async function refreshProviderDerived(provider: string) {
    const res = await ListModelsForProvider(provider)
    models = Array.isArray(res) ? res : []
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

  async function handleProviderChange(value: string) {
    await onSwitchProvider(value)
  }

  async function handleModelChange(value: string) {
    await onSwitchModel(value)
  }

  async function submitApiKey() {
    if (!apiKeyDraft.trim()) return
    await onSubmitAPIKey(model.provider, apiKeyDraft.trim())
    apiKeyDraft = ''
    await refreshProviderDerived(model.provider)
  }

  let draft = $state('')
  let modelMenuOpen = $state(false)
  let focusMenuOpen = $state(false)
  let ctxMenuOpen = $state(false)
  // Which of the provider/model/think-level pickers inside the model-menu
  // popover is expanded — native <select> can't be forced to open its option
  // list upward (browser-controlled, not stylable), so these render as a
  // small custom dropdown instead, anchored with bottom:100% like the rest
  // of this popover.
  let openDropdown = $state<'provider' | 'model' | 'thinkLevel' | ''>('')

  // Auto-grow the composer upward while typing (the composer is anchored at
  // the bottom, so extra height expands up) — capped, then it scrolls inside.
  // Reacts to every draft change so starter picks and post-send clears resize too.
  let inputEl = $state<HTMLTextAreaElement | null>(null)
  $effect(() => {
    void draft
    const el = inputEl
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 220) + 'px'
  })

  function closeMenusOnOutside(e: MouseEvent) {
    const el = e.target as HTMLElement
    if (modelMenuOpen && !el.closest('.model-pick')) { modelMenuOpen = false; openDropdown = '' }
    if (focusMenuOpen && !el.closest('.focus-pick')) focusMenuOpen = false
    if (ctxMenuOpen && !el.closest('.ctx-pick')) ctxMenuOpen = false
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

  // Guided onboarding: while Aetox runs on its own built-in engine there is no
  // model to answer questions about Aetox, so the answers are canned — and they
  // live in the locale files, which is also why they follow the UI language for
  // free rather than needing a locale plumbed into Go (ARCHITECTURE.md §39).
  const guideTopics = $derived([
    { key: 'skills', q: t('guide.q1'), a: t('guide.a1') },
    { key: 'prompts', q: t('guide.q2'), a: t('guide.a2') },
    { key: 'connect', q: t('guide.q3'), a: t('guide.a3') },
    { key: 'privacy', q: t('guide.q4'), a: t('guide.a4') },
    { key: 'tools', q: t('guide.q5'), a: t('guide.a5') },
    { key: 'who', q: t('guide.q6'), a: t('guide.a6') },
  ])
  let askedGuide = $state<string[]>([])
  const remainingGuide = $derived(guideTopics.filter((g) => !askedGuide.includes(g.key)))
  // Only while running on the built-in engine: a configured model answers for
  // itself, and canned chips under a real reply would be noise.
  const guideOpen = $derived(
    model.provider === 'aetox' && !awaitingReply && messages.length > 0 && remainingGuide.length > 0,
  )

  function askGuide(topic: { key: string; q: string; a: string }) {
    askedGuide = [...askedGuide, topic.key]
    pushGuideExchange(topic.q, topic.a)
  }

  const starters = $derived([
    { icon: '🧭', title: t('chat.starter1Title'), prompt: t('chat.starter1Prompt') },
    { icon: '🛠', title: t('chat.starter2Title'), prompt: t('chat.starter2Prompt') },
    { icon: '🔍', title: t('chat.starter3Title'), prompt: t('chat.starter3Prompt') },
    { icon: '🩹', title: t('chat.starter4Title'), prompt: t('chat.starter4Prompt') },
  ])

  function pickStarter(prompt: string) {
    draft = prompt
  }

  // Pinned auto-scroll (Claude Code/OpenCode behavior): while the user is at
  // the bottom, every new message / stream chunk / reasoning chunk / tool step
  // keeps the view pinned there. Scrolling up unpins so reading is never
  // hijacked; scrolling back down re-pins.
  let chatEl = $state<HTMLDivElement | null>(null)
  let pinnedToBottom = $state(true)
  function onChatScroll() {
    const el = chatEl
    if (!el) return
    pinnedToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80
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

  // Copy an AI reply as plain text. '✓' feedback resets after a moment.
  let copiedText = $state('')
  let copiedTimer: ReturnType<typeof setTimeout> | undefined
  async function copyMessage(text: string) {
    await navigator.clipboard.writeText(text)
    copiedText = text
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => (copiedText = ''), 1500)
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
    if (!e.dataTransfer?.types.includes('application/x-aetox-tab')) return
    e.preventDefault()
    dragOver = true
  }
  async function onComposerDrop(e: DragEvent) {
    const raw = e.dataTransfer?.getData('application/x-aetox-tab')
    dragOver = false
    if (!raw) return
    e.preventDefault()
    const { kind, ref, label } = JSON.parse(raw) as { kind: 'file' | 'browser'; ref: string; label: string }
    await attachTabContext(kind, ref, label)
  }

  // Links in rendered markdown must not navigate the app's own webview away —
  // open them in a workbench browser tab instead.
  function onChatClick(e: MouseEvent) {
    const el = e.target as HTMLElement
    // copy button on a rendered code block ({@html} markup can't carry handlers)
    const copyBtn = el.closest('.code-copy')
    if (copyBtn) {
      const code = copyBtn.closest('.codeblock')?.querySelector('code')
      navigator.clipboard.writeText(code?.textContent ?? '').then(() => {
        copyBtn.textContent = t('chat.copiedCode')
        setTimeout(() => (copyBtn.textContent = t('chat.copyCode')), 1500)
      })
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
    if (e.ctrlKey && !e.altKey && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      palette = palette === 'all' ? '' : 'all'
    }
  }}
/>

{#snippet upSelect(
  id: 'provider' | 'model' | 'thinkLevel',
  options: { value: string; label: string }[],
  current: string,
  onPick: (value: string) => void,
)}
  <div class="updrop">
    <button
      type="button"
      class="ctrl updrop-trigger"
      onclick={(e) => { e.stopPropagation(); openDropdown = openDropdown === id ? '' : id }}
    >
      <span class="t">{options.find((o) => o.value === current)?.label ?? current}</span>
      <span class="caret">{openDropdown === id ? '⌃' : '⌄'}</span>
    </button>
    {#if openDropdown === id}
      <div class="updrop-list">
        {#each options as opt}
          <button
            type="button"
            class="updrop-opt"
            class:selected={opt.value === current}
            onclick={(e) => { e.stopPropagation(); openDropdown = ''; onPick(opt.value) }}
          >{opt.label}</button>
        {/each}
      </div>
    {/if}
  </div>
{/snippet}

{#snippet toolTimeline(steps: ToolStep[], live: boolean)}
  <div class="tool-steps">
    {#each steps as s}
      <div class="tool-step {s.state}">
        {#if s.state === 'run'}
          <span class="glyph spin"></span>
        {:else}
          <span class="glyph">{s.state === 'done' ? '✓' : '✕'}</span>
        {/if}
        <span class="lbl">{s.label}</span>
        {#if s.state === 'run' && live}
          <span class="secs">· {liveSecs(s)}s</span>
        {:else if s.secs}
          <span class="secs">· {s.secs}s</span>
        {/if}
      </div>
    {/each}
  </div>
{/snippet}

  {#if messages.length === 0}
    <div class="empty-state">
      <Logo size={56} />
      <h2>{t('chat.whatToBuild')}</h2>
      <div class="starter-grid">
        {#each starters as s}
          <button class="starter-card" onclick={() => pickStarter(s.prompt)}>
            <span class="ic">{s.icon}</span>
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
      {#each messages as m, i}
        <div class="msg {m.role === 'user' ? 'user' : 'bot'}">
          <div class="bubble">
            {#if m.role === 'agent' && m.tag}
              <div class="name"><span class="tag think">{m.tag}</span></div>
            {/if}
            {#if m.imageDataUrl}
              <img src={m.imageDataUrl} alt="" class="msg-image" />
            {/if}
            {#if m.contextLabel}
              <div class="attach-chip"><span class="ic">📎</span> <span class="attach-name">{m.contextLabel}</span></div>
            {/if}
            {#if m.reasoning}
              <div class="reasoning-panel">
                <button class="reasoning-toggle" onclick={() => (expandedReasoning[i] = !expandedReasoning[i])}>
                  <span class="chev">{expandedReasoning[i] ? '▾' : '▸'}</span>
                  {m.thinkSecs ? t('chat.thoughtFor', { secs: m.thinkSecs }) : t('chat.thoughtDone')}
                </button>
                {#if expandedReasoning[i]}
                  <div class="reasoning-body">{m.reasoning}</div>
                {/if}
              </div>
            {/if}
            {#if m.steps?.length}
              {@render toolTimeline(m.steps, false)}
            {/if}
            <div class="markdown-body">{@html renderMarkdown(m.text)}</div>
            <div class="time">
              {m.time}
              {#if m.role === 'agent' && m.text}
                <button type="button" class="msg-copy" aria-label={t('chat.copy')}
                  onclick={() => copyMessage(m.text)}>
                  {copiedText === m.text ? '✓' : '⧉'}
                </button>
              {/if}
            </div>
          </div>
        </div>
      {/each}

      {#if guideOpen}
        <div class="guide">
          <div class="guide-head">{t('guide.intro')}</div>
          <div class="guide-chips">
            {#each remainingGuide as g (g.key)}
              <button class="guide-chip" onclick={() => askGuide(g)}>{g.q}</button>
            {/each}
          </div>
        </div>
      {/if}

      {#if awaitingReply}
        <div class="msg bot">
          <div class="bubble typing-bubble">
            <div class="typing-row">
              {#if agentStatus && !streamingText}
                <span class="typing-status">{agentStatus}</span>
              {/if}
              <span class="typing-dots"><span></span><span></span><span></span></span>
            </div>
            {#if reasoningText}
              <div class="reasoning-panel">
                <button class="reasoning-toggle" onclick={() => (reasoningCollapsed = !reasoningCollapsed)}>
                  <span class="chev">{reasoningCollapsed ? '▸' : '▾'}</span> {t('chat.thinking')}
                </button>
                {#if !reasoningCollapsed}
                  <div class="reasoning-body">{reasoningText}</div>
                {/if}
              </div>
            {/if}
            {#if cockpit.todos.length > 0}
              <div class="todo-panel">
                {#each cockpit.todos as td}
                  <div class="todo-item {td.status}">
                    <span class="mark">{td.status === 'completed' ? '✓' : td.status === 'in_progress' ? '▸' : '○'}</span>
                    <span class="t">{td.content}</span>
                  </div>
                {/each}
              </div>
            {/if}
            {#if toolSteps.length > 0}
              {@render toolTimeline(toolSteps, true)}
            {/if}
            {#if streamingText}
              <div class="markdown-body">{@html renderMarkdown(streamingText)}</div>
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
                </div>
                <div class="ask-hint">{t('chat.askHint')}</div>
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

  <div class="composer">
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
    {#if cockpit.pendingImage}
      <div class="attach-chip">
        <img src={cockpit.pendingImage.dataUrl} alt="" class="attach-thumb" />
        <span class="attach-name">{cockpit.pendingImage.relPath.split('/').pop()}</span>
        <button class="attach-remove" aria-label={t('chat.removeAttachment')} onclick={clearPendingImage}>✕</button>
      </div>
    {/if}
    {#if cockpit.pendingFile}
      <div class="attach-chip">
        <span class="ic">{cockpit.pendingFile.kind === 'audio' ? '🎧' : cockpit.pendingFile.kind === 'video' ? '🎬' : '📄'}</span>
        <span class="attach-name">{cockpit.pendingFile.label}</span>
        <button class="attach-remove" aria-label={t('chat.removeAttachment')} onclick={clearPendingFile}>✕</button>
      </div>
    {/if}
    {#if cockpit.pendingContext}
      <div class="attach-chip">
        <span class="ic">{cockpit.pendingContext.kind === 'file' ? '📄' : '🌐'}</span>
        <span class="attach-name">{cockpit.pendingContext.label}</span>
        <button class="attach-remove" aria-label={t('chat.removeAttachment')} onclick={clearPendingContext}>✕</button>
      </div>
    {/if}
    <div class="focus-row">
      <div class="focus-pick">
        {#if focusMenuOpen}
          <div class="focus-menu">
            <button type="button" class="focus-item" class:on={!cockpit.project.focused} onclick={() => { focusMenuOpen = false; clearProjectFocus() }}>
              <span class="ic">💬</span> {t('chat.noProject')}
            </button>
            {#if cockpit.projects.length > 0}<div class="menu-sep"></div>{/if}
            {#each cockpit.projects.slice(0, 8) as p (p.key)}
              <button type="button" class="focus-item" class:on={cockpit.project.focused && p.active} onclick={() => { focusMenuOpen = false; openProject(p.path) }}>
                <span class="ic">📁</span><span class="t">{p.name}</span>
              </button>
            {/each}
            <div class="menu-sep"></div>
            <button type="button" class="focus-item" onclick={() => { focusMenuOpen = false; openFolder() }}>
              <span class="ic">📂</span> {t('topbar.openFolder')}…
            </button>
          </div>
        {/if}
        <button type="button" class="focus-chip focus-btn" onclick={() => (focusMenuOpen = !focusMenuOpen)}>
          <span class="ic">{cockpit.project.focused ? '📁' : '💬'}</span>
          {cockpit.project.focused ? cockpit.project.name : t('chat.noProject')}
          <span class="caret">{focusMenuOpen ? '⌃' : '⌄'}</span>
        </button>
      </div>
      {#if cockpit.project.focused && cockpit.project.branch}<span class="focus-chip">⑂ {cockpit.project.branch}</span>{/if}
    </div>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- drag/drop target for a workbench tab; the textarea/buttons inside remain the real interactive elements -->
    <div class="box" class:drag-over={dragOver} ondragover={onComposerDragOver} ondragleave={() => (dragOver = false)} ondrop={onComposerDrop}>
      <textarea
        class="input"
        rows="1"
        placeholder={t('chat.inputPlaceholder')}
        bind:this={inputEl}
        bind:value={draft}
        onkeydown={onKeydown}
      ></textarea>
      <div class="tools">
        <button
          class="icobtn" aria-label={t('chat.attachFile')} data-tip={t('chat.attachFile')}
          onclick={attachViaDialog}
        >+</button>
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
                  <span class="t">{t('chat.contextWindow')}</span>
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
                <div class="mm-row">
                  <span class="lbl">{t('chat.provider')}</span>
                  {@render upSelect('provider', providers.map((p) => ({ value: p, label: p })), model.provider, handleProviderChange)}
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
                {#if thinkLevels.length > 0}
                  <div class="mm-row">
                    <span class="lbl">{t('chat.thinkLevel')}</span>
                    {@render upSelect('thinkLevel', thinkLevels.map((lvl) => ({ value: lvl, label: lvl })), model.thinkLevel, onSwitchThinkLevel)}
                  </div>
                {/if}
              </div>
            {/if}
            <button type="button" class="model-chip" onclick={(e) => { e.stopPropagation(); modelMenuOpen = !modelMenuOpen; if (modelMenuOpen) { refreshThinkLevels(); EnabledProviders().then((p) => (providers = p)) } }}>
              <span class="t">{model.modelName || model.provider}</span>
              {#if model.thinkLevel}<span class="lvl">{model.thinkLevel}</span>{/if}
              <span class="caret">{modelMenuOpen ? '⌃' : '⌄'}</span>
            </button>
          </div>
        {/if}
        {#if awaitingReply}
          <!-- The tool loop is unbounded — this is the user's brake (Ctrl+C of the UI) -->
          <button class="send stop" aria-label="Stop" onclick={cancelTurn}>■</button>
        {:else}
          <button class="send" aria-label="Send" onclick={submit}>➤</button>
        {/if}
      </div>
    </div>
  </div>
