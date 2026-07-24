<script lang="ts">
  import { onMount } from 'svelte'
  import { t, setLocale, localeNames, i18n, type Locale } from './i18n.svelte'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import { SupportedProviders, RequiresAPIKey, HasAPIKey } from '../../wailsjs/go/main/App'
  import { cockpit, switchProvider, submitAPIKey, switchApprovalMode } from './stores/cockpit.svelte'

  const DONE_KEY = 'aetox.onboarded'

  let visible = $state(false)
  let step = $state(1)
  let busy = $state(false)
  let errorMsg = $state('')

  type ProviderRow = { name: string; requiresKey: boolean; hasKey: boolean }
  let providers = $state<ProviderRow[]>([])
  let selected = $state('')
  let keyDraft = $state('')
  let approval = $state('unsafe-only')

  onMount(async () => {
    if (localStorage.getItem(DONE_KEY)) return
    // An install that already has a working provider key was set up before
    // this wizard existed — never bother it.
    try {
      if (cockpit.model.provider && (await HasAPIKey(cockpit.model.provider))) {
        localStorage.setItem(DONE_KEY, '1')
        return
      }
    } catch {
      /* engine not ready — fall through and show the wizard */
    }
    const names = await SupportedProviders()
    providers = await Promise.all(names.map(async (name) => ({
      name,
      requiresKey: await RequiresAPIKey(name),
      hasKey: await HasAPIKey(name),
    })))
    selected = cockpit.model.provider || providers[0]?.name || ''
    visible = true
  })

  const selectedRow = $derived(providers.find((p) => p.name === selected))

  function finish() {
    localStorage.setItem(DONE_KEY, '1')
    visible = false
  }

  async function saveProviderStep() {
    busy = true
    errorMsg = ''
    try {
      if (selected) {
        if (keyDraft.trim()) await submitAPIKey(selected, keyDraft.trim())
        await switchProvider(selected)
      }
      step = 3
    } catch (err) {
      errorMsg = String(err)
    } finally {
      busy = false
    }
  }

  async function finishWithApproval() {
    busy = true
    errorMsg = ''
    try {
      await switchApprovalMode(approval)
      finish()
    } catch (err) {
      errorMsg = String(err)
      finish() // approval can be changed later in Settings; never trap the user
    } finally {
      busy = false
    }
  }
</script>

{#if visible}
  <div class="onboard-overlay">
    <div class="onboard-card">
      <div class="onboard-brand">Aetox</div>
      <div class="onboard-steps">{step}/3</div>

      {#if step === 1}
        <h2>{t('onboard.welcomeTitle')}</h2>
        <p class="muted">{t('onboard.welcomeDesc')}</p>
        <div class="onboard-field">
          <div class="eyebrow">{t('settings.languageTitle')}</div>
          <select class="ctrl" value={i18n.locale} onchange={(e) => setLocale(e.currentTarget.value as Locale)}>
            {#each Object.entries(localeNames) as [code, name]}<option value={code}>{name}</option>{/each}
          </select>
        </div>
        <div class="onboard-field">
          <div class="eyebrow">{t('settings.themeTitle')}</div>
          <select class="ctrl" value={theme.name} onchange={(e) => applyTheme(e.currentTarget.value as ThemeName)}>
            {#each THEMES as th}<option value={th.value}>{th.label}</option>{/each}
          </select>
        </div>
        <div class="onboard-actions">
          <button class="ctrl" onclick={finish}>{t('onboard.skip')}</button>
          <button class="ctrl primary" onclick={() => (step = 2)}>{t('onboard.next')}</button>
        </div>
      {:else if step === 2}
        <h2>{t('onboard.modelTitle')}</h2>
        <p class="muted">{t('onboard.modelDesc')}</p>
        <div class="onboard-field">
          <div class="eyebrow">{t('settings.providers')}</div>
          <select class="ctrl" bind:value={selected}>
            {#each providers as p}<option value={p.name}>{p.name}{p.hasKey ? ' ✓' : ''}</option>{/each}
          </select>
        </div>
        {#if selectedRow?.requiresKey && !selectedRow?.hasKey}
          <div class="onboard-field">
            <div class="eyebrow">{t('settings.apiKeyLabel')}</div>
            <input class="ctrl" type="password" placeholder={t('onboard.keyPlaceholder')} bind:value={keyDraft} />
          </div>
        {/if}
        {#if errorMsg}<div class="mset-error">{errorMsg}</div>{/if}
        <div class="onboard-actions">
          <button class="ctrl" onclick={finish}>{t('onboard.skip')}</button>
          <button class="ctrl" onclick={() => (step = 1)}>{t('onboard.back')}</button>
          <button class="ctrl primary" disabled={busy} onclick={saveProviderStep}>
            {busy ? t('onboard.saving') : t('onboard.next')}
          </button>
        </div>
      {:else}
        <h2>{t('onboard.approvalTitle')}</h2>
        <p class="muted">{t('onboard.approvalDesc')}</p>
        <div class="onboard-choice-list">
          {#each [
            { value: 'ask', label: t('chat.approvalAsk'), desc: t('onboard.approvalAskDesc') },
            { value: 'unsafe-only', label: t('chat.approvalUnsafeOnly'), desc: t('onboard.approvalUnsafeDesc') },
            { value: 'full-access', label: t('chat.approvalFullAccess'), desc: t('onboard.approvalFullDesc') },
          ] as opt}
            <button class="onboard-choice" class:selected={approval === opt.value} onclick={() => (approval = opt.value)}>
              <div class="t">{opt.label}</div>
              <div class="d">{opt.desc}</div>
            </button>
          {/each}
        </div>
        {#if errorMsg}<div class="mset-error">{errorMsg}</div>{/if}
        <div class="onboard-actions">
          <button class="ctrl" onclick={() => (step = 2)}>{t('onboard.back')}</button>
          <button class="ctrl primary" disabled={busy} onclick={finishWithApproval}>
            {busy ? t('onboard.saving') : t('onboard.start')}
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}
