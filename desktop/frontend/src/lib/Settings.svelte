<script lang="ts">
  import { onMount } from 'svelte'
  import { theme, toggleTheme } from './theme.svelte'
  import { SupportedProviders, HasAPIKey, RequiresAPIKey } from '../../wailsjs/go/main/App'
  import type { ModelStatus } from './types'

  let { model, onSubmitAPIKey }: {
    model: ModelStatus
    onSubmitAPIKey: (provider: string, apiKey: string) => Promise<void>
  } = $props()

  type ProviderRow = { name: string; requiresKey: boolean; hasKey: boolean }

  let rows = $state<ProviderRow[]>([])
  let drafts = $state<Record<string, string>>({})
  let savingProvider = $state('')

  onMount(refreshRows)

  async function refreshRows() {
    const providers = await SupportedProviders()
    rows = await Promise.all(providers.map(async (name) => ({
      name,
      requiresKey: await RequiresAPIKey(name),
      hasKey: await HasAPIKey(name),
    })))
  }

  async function saveKey(provider: string) {
    const key = (drafts[provider] ?? '').trim()
    if (!key) return
    savingProvider = provider
    try {
      await onSubmitAPIKey(provider, key)
      drafts[provider] = ''
      await refreshRows()
    } finally {
      savingProvider = ''
    }
  }
</script>

<div class="settings">
  <div class="settings-section">
    <h3 class="eyebrow">Appearance</h3>
    <div class="settings-row">
      <span>Theme</span>
      <button class="ctrl" onclick={toggleTheme}>{theme.name === 'dark' ? '🌙 Dark' : '☀ Light'}</button>
    </div>
  </div>

  <div class="settings-section">
    <h3 class="eyebrow">Current model</h3>
    <div class="settings-row">
      <span>Provider / Model</span>
      <span class="muted">{model.provider || '—'} / {model.modelName || '—'}</span>
    </div>
    <div class="settings-row">
      <span>Approval mode</span>
      <span class="muted">{model.approval}</span>
    </div>
  </div>

  <div class="settings-section">
    <h3 class="eyebrow">API keys</h3>
    {#each rows as row}
      <div class="settings-row key-row">
        <span class="key-name">
          {row.name}
          {#if !row.requiresKey}<span class="muted"> (no key needed)</span>
          {:else if row.hasKey}<span class="dot green"></span> set
          {:else}<span class="dot"></span> not set{/if}
        </span>
        {#if row.requiresKey}
          <input
            class="ctrl" type="password" placeholder={row.hasKey ? 'replace key…' : 'API key…'}
            bind:value={drafts[row.name]}
            onkeydown={(e) => e.key === 'Enter' && saveKey(row.name)}
          />
          <button class="ctrl" disabled={savingProvider === row.name} onclick={() => saveKey(row.name)}>
            {savingProvider === row.name ? 'Saving…' : 'Save'}
          </button>
        {/if}
      </div>
    {/each}
  </div>
</div>
