<script lang="ts">
  import type { ChatMessage, TaskState, ModelStatus } from './types'
  import TaskTimeline from './TaskTimeline.svelte'
  import { onMount } from 'svelte'
  import {
    SupportedProviders, SupportedThinkLevels,
    ListModelsForProvider, RequiresAPIKey, HasAPIKey,
  } from '../../wailsjs/go/main/App'

  let {
    messages, task, governanceFile, model,
    onSend, onSwitchProvider, onSwitchThinkLevel, onSwitchApprovalMode, onSwitchModel, onSubmitAPIKey,
  }: {
    messages: ChatMessage[]
    task: TaskState
    governanceFile: string
    model: ModelStatus
    onSend: (text: string) => void
    onSwitchProvider: (provider: string) => Promise<void>
    onSwitchThinkLevel: (level: string) => Promise<void>
    onSwitchApprovalMode: (mode: string) => Promise<void>
    onSwitchModel: (modelName: string) => Promise<void>
    onSubmitAPIKey: (provider: string, apiKey: string) => Promise<void>
  } = $props()

  const approvalOptions = [
    { value: 'ask', label: 'Ask' },
    { value: 'unsafe-only', label: 'Unsafe Only' },
    { value: 'full-access', label: 'Full Access' },
  ]

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

  const tabs = ['Chat', 'Aetox.md Map']
  let activeTab = $state('Chat')
  let draft = $state('')

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

<div class="tabs">
  {#each tabs as tab}
    <button class="tab" class:active={activeTab === tab} onclick={() => (activeTab = tab)}>
      {tab}
    </button>
  {/each}
</div>

{#if activeTab === 'Chat'}
  <div class="chat">
    {#each messages as m}
      <div class="msg {m.role === 'user' ? 'user' : 'bot'}">
        <div class="who">{m.role === 'user' ? '🧑' : '🦅'}</div>
        <div>
          <div class="bubble">
            {#if m.role === 'agent'}
              <div class="name"><b>Aetox Agent</b>
                {#if m.tag}<span class="tag think">{m.tag}</span>{/if}
              </div>
            {/if}
            <span class="body-text">{m.text}</span>
            <div class="time">{m.time}</div>
          </div>
        </div>
      </div>
    {/each}

    {#if task.steps.length > 0}
      <TaskTimeline steps={task.steps} elapsed={task.elapsed} />
    {/if}
  </div>

  <div class="composer">
    {#if needsApiKey}
      <div class="api-key-banner">
        <input
          class="ctrl"
          type="password"
          placeholder={`API key for ${model.provider}`}
          bind:value={apiKeyDraft}
          onkeydown={(e) => e.key === 'Enter' && submitApiKey()}
        />
        <button class="ctrl" onclick={submitApiKey}>Save key</button>
      </div>
    {/if}
    <div class="box">
      <textarea
        class="input"
        rows="1"
        placeholder="Type your command or request… (ใช้ / เพื่อดูคำสั่ง)"
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
            placeholder="model id"
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
    <pre style="color:#0f0;font-size:11px;background:#000;padding:8px;margin-top:6px;white-space:pre-wrap;">DEBUG model={JSON.stringify(model)} thinkLevels={JSON.stringify(thinkLevels)} models={JSON.stringify(models)}</pre>
  </div>
{:else}
  <div class="map-view">
    <div class="map-card">
      <div class="map-icon">🗺</div>
      <div class="map-title">{governanceFile} Map</div>
      <p class="map-sub">มุมมองโครงสร้าง governance ของ {governanceFile} — จะเชื่อมกับ Go core ที่ parse ไฟล์จริงในขั้นต่อไป</p>
    </div>
  </div>
{/if}
