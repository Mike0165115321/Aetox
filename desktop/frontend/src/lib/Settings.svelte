<script lang="ts">
  import { onMount } from 'svelte'
  import { theme, toggleTheme } from './theme.svelte'
  import { editorFont, applyEditorFontSize } from './editorFont.svelte'
  import { chatFont, applyChatFontSize } from './chatFont.svelte'
  import { editorTheme, setBuiltinEditorTheme, importThemeFile } from './editorTheme.svelte'
  import { treeFont, applyTreeFontSize } from './treeFont.svelte'
  import { systemZoom, applySystemZoom, SYSTEM_BASE_PX } from './systemFont.svelte'
  import { i18n, t, setLocale, localeNames, type Locale } from './i18n.svelte'
  import {
    SupportedProviders, HasAPIKey, RequiresAPIKey, TerminalShells,
    ListModelsForProvider, ProviderBaseURL,
    ListMCPServers, AddMCPServer, RemoveMCPServer, TestMCPServer,
  } from '../../wailsjs/go/main/App'
  import { cockpit, switchProvider, switchModel, submitAPIKey, switchApprovalMode } from './stores/cockpit.svelte'

  let { onClose }: { onClose: () => void } = $props()

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
  let selected = $state('')
  let baseURL = $state('')
  let models = $state<string[]>([])
  let loadingModels = $state(false)
  let keyDraft = $state('')
  let showKey = $state(false)
  let customModel = $state('')
  let busy = $state('')
  let errorMsg = $state('')

  const selectedRow = $derived(providers.find((p) => p.name === selected))
  const isActiveProvider = $derived(cockpit.model.provider === selected)

  onMount(async () => {
    shells = await TerminalShells()
    if (!shells.some((s) => s.path === defaultShell)) defaultShell = shells[0]?.path ?? ''

    await refreshProviders()
    selectProvider(cockpit.model.provider || providers[0]?.name || '')

    await loadMCP()
  })

  async function refreshProviders() {
    const names = await SupportedProviders()
    providers = await Promise.all(names.map(async (name) => ({
      name,
      requiresKey: await RequiresAPIKey(name),
      hasKey: await HasAPIKey(name),
    })))
  }

  async function selectProvider(name: string) {
    if (!name) return
    selected = name
    errorMsg = ''
    keyDraft = ''
    baseURL = await ProviderBaseURL(name)
    loadingModels = true
    models = []
    try {
      models = await ListModelsForProvider(name)
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

  const saveKey = () => run('key', async () => {
    const key = keyDraft.trim()
    if (!key) return
    await submitAPIKey(selected, key)
    keyDraft = ''
    await refreshProviders()
    await selectProvider(selected)
  })

  // ---------- MCP servers ----------
  type MCPRow = { name: string; command: string[]; status: string; err?: string }
  let mcpServers = $state<MCPRow[]>([])
  let mcpName = $state('')
  let mcpCommand = $state('')
  let mcpBusy = $state('')
  let mcpError = $state('')

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

  const addMCP = () => runMCP('add', async () => {
    const command = mcpCommand.trim().split(/\s+/).filter(Boolean)
    await AddMCPServer(mcpName.trim(), command)
    mcpName = ''
    mcpCommand = ''
    await loadMCP()
  })

  const removeMCP = (name: string) => runMCP('rm:' + name, async () => {
    await RemoveMCPServer(name)
    await loadMCP()
  })

  const testMCP = (name: string) => runMCP('test:' + name, async () => {
    const info = await TestMCPServer(name)
    mcpServers = mcpServers.map((s) => (s.name === name ? info : s))
  })

  function statusColor(status: string): string {
    const c = status === 'connected' ? '#3fb950' : status === 'failed' ? '#f85149' : '#8b949e'
    return `background:${c}`
  }

  // ---------- Nav ----------
  const sections = $derived([
    { group: t('settings.groupPersonal'), items: [
      { id: 'general', label: t('settings.general'), icon: '⚙' },
      { id: 'appearance', label: t('settings.appearance'), icon: '🎨' },
    ]},
    { group: t('settings.groupModels'), items: [
      { id: 'models', label: t('settings.modelSettings'), icon: '🧠' },
    ]},
    { group: t('settings.groupTools'), items: [
      { id: 'mcp', label: t('settings.mcpServers'), icon: '🔌' },
    ]},
  ])

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
          <button class="ctrl" onclick={toggleTheme}>{theme.name === 'dark' ? t('settings.dark') : t('settings.light')}</button>
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
            if (v === 'vs-dark' || v === 'vs') setBuiltinEditorTheme(v)
          }}>
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
          {#each providers as p}
            <button class="mset-prov" class:selected={selected === p.name} onclick={() => selectProvider(p.name)}>
              {p.name}
              <span class="dot" class:green={p.hasKey}></span>
            </button>
          {/each}
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

            <div class="mset-field">
              <div class="eyebrow">{t('settings.baseUrl')}</div>
              <div class="mset-ro">{baseURL || '—'}</div>
            </div>

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
                    {#if isActiveProvider && cockpit.model.modelName === m}
                      <span class="badge on">{t('settings.inUse')}</span>
                    {:else}
                      <button class="ctrl" disabled={busy !== ''} onclick={() => useModel(m)}>
                        {busy === m ? t('settings.switching') : t('settings.use')}
                      </button>
                    {/if}
                  </div>
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
    {:else if active === 'mcp'}
      <h2>{t('settings.mcpServers')}</h2>
      <p class="muted set-sub">{t('settings.mcpDesc')}</p>

      <div class="settings-card">
        {#if mcpServers.length === 0}
          <div class="muted">{t('settings.noMcpServers')}</div>
        {:else}
          {#each mcpServers as s}
            <div class="set-row">
              <div class="set-txt">
                <div class="t"><span class="dot" style={statusColor(s.status)}></span> {s.name}</div>
                <div class="d">{s.command.join(' ')}{s.err ? ' — ' + s.err : ''}</div>
              </div>
              <div style="display:flex; gap:8px">
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => testMCP(s.name)}>
                  {mcpBusy === 'test:' + s.name ? t('settings.testing') : t('settings.test')}
                </button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => removeMCP(s.name)}>{t('settings.remove')}</button>
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <div class="settings-card">
        <div class="eyebrow">{t('settings.addServer')}</div>
        <div class="mset-keyrow">
          <input class="ctrl" placeholder={t('settings.mcpNamePlaceholder')} bind:value={mcpName} />
        </div>
        <div class="mset-keyrow">
          <input
            class="ctrl key-input"
            placeholder={t('settings.mcpCommandPlaceholder')}
            bind:value={mcpCommand}
            onkeydown={(e) => e.key === 'Enter' && mcpName.trim() && mcpCommand.trim() && addMCP()}
          />
          <button class="ctrl" disabled={mcpBusy !== '' || !mcpName.trim() || !mcpCommand.trim()} onclick={addMCP}>
            {mcpBusy === 'add' ? t('settings.adding') : t('settings.add')}
          </button>
        </div>
        {#if mcpError}<div class="mset-error">{mcpError}</div>{/if}
      </div>
    {/if}
    </div>
  </div>
</div>
