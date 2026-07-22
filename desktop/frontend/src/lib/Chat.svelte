<script lang="ts">
  import type { ChatMessage, TaskState, ModelStatus, ToolStep } from './types'
  import TaskTimeline from './TaskTimeline.svelte'
  import { onMount } from 'svelte'
  import {
    SupportedProviders, SupportedThinkLevels,
    ListModelsForProvider, RequiresAPIKey, HasAPIKey, PickAttachmentImage,
  } from '../../wailsjs/go/main/App'
  import { t } from './i18n.svelte'
  import { renderMarkdown } from './markdown'
  import { openUrlInWorkbench } from './stores/workbench.svelte'
  import { cockpit, attachImageFromPath, clearPendingImage, cancelTurn } from './stores/cockpit.svelte'

  let {
    messages, task, model, awaitingReply, agentStatus, toolSteps, streamingText,
    onSend, onSwitchProvider, onSwitchThinkLevel, onSwitchModel, onSubmitAPIKey,
  }: {
    messages: ChatMessage[]
    task: TaskState
    model: ModelStatus
    awaitingReply: boolean
    agentStatus: string
    toolSteps: ToolStep[]
    streamingText: string
    onSend: (text: string) => void
    onSwitchProvider: (provider: string) => Promise<void>
    onSwitchThinkLevel: (level: string) => Promise<void>
    onSwitchModel: (modelName: string) => Promise<void>
    onSubmitAPIKey: (provider: string, apiKey: string) => Promise<void>
  } = $props()

  let providers = $state<string[]>([])
  let thinkLevels = $state<string[]>([])
  let models = $state<string[]>([])
  let showCustomModel = $state(false)
  let customModel = $state('')
  let needsApiKey = $state(false)
  let apiKeyDraft = $state('')

  onMount(async () => {
    providers = await SupportedProviders()
  })

  async function refreshProviderDerived(provider: string) {
    models = await ListModelsForProvider(provider)
    needsApiKey = (await RequiresAPIKey(provider)) && !(await HasAPIKey(provider))
  }

  // Model list, API-key requirement, and think levels all depend on the current
  // provider/model — re-derive whenever either changes, from any source (initial
  // async load, a provider switch, or a model switch).
  $effect(() => {
    const provider = model.provider
    if (!provider) return
    showCustomModel = false
    refreshProviderDerived(provider)
  })
  $effect(() => {
    const provider = model.provider
    const modelName = model.modelName
    if (!provider) return
    SupportedThinkLevels().then((levels) => (thinkLevels = levels))
  })

  async function handleProviderChange(e: Event) {
    await onSwitchProvider((e.target as HTMLSelectElement).value)
  }

  async function handleModelChange(e: Event) {
    const value = (e.target as HTMLSelectElement).value
    if (value === '__custom__') {
      showCustomModel = true
      return
    }
    showCustomModel = false
    await onSwitchModel(value)
  }

  async function submitCustomModel() {
    if (!customModel.trim()) return
    await onSwitchModel(customModel.trim())
    customModel = ''
  }

  async function submitApiKey() {
    if (!apiKeyDraft.trim()) return
    await onSubmitAPIKey(model.provider, apiKeyDraft.trim())
    apiKeyDraft = ''
    await refreshProviderDerived(model.provider)
  }

  let draft = $state('')

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

  const starters = $derived([
    { icon: '🧭', title: t('chat.starter1Title'), prompt: t('chat.starter1Prompt') },
    { icon: '🛠', title: t('chat.starter2Title'), prompt: t('chat.starter2Prompt') },
    { icon: '🔍', title: t('chat.starter3Title'), prompt: t('chat.starter3Prompt') },
    { icon: '🩹', title: t('chat.starter4Title'), prompt: t('chat.starter4Prompt') },
  ])

  function pickStarter(prompt: string) {
    draft = prompt
  }

  function submit() {
    if (!draft.trim() && !cockpit.pendingImage) return
    onSend(draft)
    draft = ''
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  async function attachViaDialog() {
    const path = await PickAttachmentImage()
    if (path) await attachImageFromPath(path)
  }

  // Links in rendered markdown must not navigate the app's own webview away —
  // open them in a workbench browser tab instead.
  function onChatClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest('a')
    const href = a?.getAttribute('href')
    if (!href || !/^https?:\/\//i.test(href)) return
    e.preventDefault()
    openUrlInWorkbench(href)
  }
</script>

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
      <span class="empty-glyph word">Aetox</span>
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
    <div class="chat" onclick={onChatClick}>
    <div class="chat-inner">
      {#each messages as m}
        <div class="msg {m.role === 'user' ? 'user' : 'bot'}">
          <div class="bubble">
            {#if m.role === 'agent'}
              <div class="name"><b>{t('chat.agentName')}</b>
                {#if m.tag}<span class="tag think">{m.tag}</span>{/if}
              </div>
            {/if}
            {#if m.imageDataUrl}
              <img src={m.imageDataUrl} alt="" class="msg-image" />
            {/if}
            {#if m.steps?.length}
              {@render toolTimeline(m.steps, false)}
            {/if}
            <div class="markdown-body">{@html renderMarkdown(m.text)}</div>
            <div class="time">{m.time}</div>
          </div>
        </div>
      {/each}

      {#if awaitingReply}
        <div class="msg bot">
          <div class="bubble typing-bubble">
            <div class="typing-row">
              {#if agentStatus && !streamingText}
                <span class="typing-status">{agentStatus}</span>
              {/if}
              <span class="typing-dots"><span></span><span></span><span></span></span>
              <button class="stop-btn" onclick={cancelTurn} title={t('chat.stopTurn')}>■ {t('chat.stopTurn')}</button>
            </div>
            {#if toolSteps.length > 0}
              {@render toolTimeline(toolSteps, true)}
            {/if}
            {#if streamingText}
              <div class="markdown-body">{@html renderMarkdown(streamingText)}</div>
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
    <div class="box">
      <textarea
        class="input"
        rows="1"
        placeholder={t('chat.inputPlaceholder')}
        bind:value={draft}
        onkeydown={onKeydown}
      ></textarea>
      <div class="tools">
        <button class="icobtn" aria-label={t('chat.attachImage')} data-tip={t('chat.attachImage')} onclick={attachViaDialog}>📎</button>
        <select class="ctrl" value={model.provider} onchange={handleProviderChange}>
          {#each providers as p}<option value={p}>{p}</option>{/each}
        </select>
        <select class="ctrl" value={model.modelName} onchange={handleModelChange}>
          {#each models as m}<option value={m}>{m}</option>{/each}
          <option value="__custom__">Custom…</option>
        </select>
        {#if showCustomModel || models.length === 0}
          <input
            class="ctrl"
            type="text"
            placeholder={t('chat.modelIdPlaceholder')}
            value={customModel || model.modelName}
            oninput={(e) => (customModel = (e.target as HTMLInputElement).value)}
            onkeydown={(e) => e.key === 'Enter' && submitCustomModel()}
          />
        {/if}
        {#if thinkLevels.length > 0}
          <select class="ctrl" value={model.thinkLevel} onchange={(e) => onSwitchThinkLevel((e.target as HTMLSelectElement).value)}>
            {#each thinkLevels as lvl}<option value={lvl}>{lvl}</option>{/each}
          </select>
        {/if}
        <button class="send" aria-label="Send" onclick={submit}>➤</button>
      </div>
    </div>
  </div>
