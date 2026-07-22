<script lang="ts">
  import type { ChatMessage, TaskState, ModelStatus, ProjectInfo } from './types'
  import TaskTimeline from './TaskTimeline.svelte'
  import { onMount } from 'svelte'
  import {
    SupportedProviders, SupportedThinkLevels,
    ListModelsForProvider, RequiresAPIKey, HasAPIKey,
  } from '../../wailsjs/go/main/App'
  import { t } from './i18n.svelte'
  import { renderMarkdown } from './markdown'

  let {
    messages, task, model, project,
    onSend, onSwitchProvider, onSwitchThinkLevel, onSwitchApprovalMode, onSwitchModel, onSubmitAPIKey,
  }: {
    messages: ChatMessage[]
    task: TaskState
    model: ModelStatus
    project: ProjectInfo
    onSend: (text: string) => void
    onSwitchProvider: (provider: string) => Promise<void>
    onSwitchThinkLevel: (level: string) => Promise<void>
    onSwitchApprovalMode: (mode: string) => Promise<void>
    onSwitchModel: (modelName: string) => Promise<void>
    onSubmitAPIKey: (provider: string, apiKey: string) => Promise<void>
  } = $props()

  const approvalOptions = $derived([
    { value: 'ask', label: t('chat.approvalAsk') },
    { value: 'unsafe-only', label: t('chat.approvalUnsafeOnly') },
    { value: 'full-access', label: t('chat.approvalFullAccess') },
  ])

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
    if (!draft.trim()) return
    onSend(draft)
    draft = ''
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }
</script>

  {#if messages.length === 0}
    <div class="empty-state">
      <span class="empty-glyph">🦅</span>
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
    <div class="chat">
      {#each messages as m}
        <div class="msg {m.role === 'user' ? 'user' : 'bot'}">
          <div class="who">{m.role === 'user' ? '🧑' : '🦅'}</div>
          <div>
            <div class="bubble">
              {#if m.role === 'agent'}
                <div class="name"><b>{t('chat.agentName')}</b>
                  {#if m.tag}<span class="tag think">{m.tag}</span>{/if}
                </div>
              {/if}
              <div class="markdown-body">{@html renderMarkdown(m.text)}</div>
              <div class="time">{m.time}</div>
            </div>
          </div>
        </div>
      {/each}

      {#if task.steps.length > 0}
        <TaskTimeline steps={task.steps} elapsed={task.elapsed} />
      {/if}
    </div>
  {/if}

  <div class="composer">
    {#if project.name}
      <div class="project-chips">
        <span class="chip"><span class="ic">📁</span> {project.name}</span>
        {#if project.branch}<span class="chip branch"><span class="ic">⑂</span> {project.branch}</span>{/if}
      </div>
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
    <div class="box">
      <textarea
        class="input"
        rows="1"
        placeholder={t('chat.inputPlaceholder')}
        bind:value={draft}
        onkeydown={onKeydown}
      ></textarea>
      <div class="tools">
        <select class="ctrl" value={model.approval} onchange={(e) => onSwitchApprovalMode((e.target as HTMLSelectElement).value)}>
          {#each approvalOptions as opt}<option value={opt.value}>{opt.label}</option>{/each}
        </select>
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
            bind:value={customModel}
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
