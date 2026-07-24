<script lang="ts">
  import { onMount } from 'svelte'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import { editorFont, applyEditorFontSize } from './editorFont.svelte'
  import { chatFont, applyChatFontSize } from './chatFont.svelte'
  import { uiFont, applyUiFont, UI_FONTS, type UiFontName } from './uiFont.svelte'
  import { editorTheme, setBuiltinEditorTheme, setAutoEditorTheme, importThemeFile } from './editorTheme.svelte'
  import { treeFont, applyTreeFontSize } from './treeFont.svelte'
  import { systemZoom, applySystemZoom, SYSTEM_BASE_PX } from './systemFont.svelte'
  import { i18n, t, setLocale, localeNames, type Locale } from './i18n.svelte'
  import {
    SupportedProviders, HasAPIKey, RequiresAPIKey, TerminalShells,
    ListModelsForProvider, ProviderBaseURL,
    ListMCPServers, SaveMCPServer, RemoveMCPServer, TestMCPServer, ToggleMCPServer,
    ListExternalSkills, InstallSkillFromGitHub, RemoveExternalSkill, RefreshSkills,
  } from '../../wailsjs/go/main/App'
  import { config } from '../../wailsjs/go/models'
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
    await loadSkills()
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
      const res = await ListModelsForProvider(name)
      models = Array.isArray(res) ? res : []
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
  type MCPRow = {
    name: string; command?: string[]; url?: string
    environment?: Record<string, string>; headers?: Record<string, string>
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
  }

  function editMCP(s: MCPRow) {
    mcpOriginal = s.name
    mcpKind = s.url ? 'http' : 'stdio'
    mcpName = s.name
    mcpCommand = (s.command ?? []).join(' ')
    mcpUrl = s.url ?? ''
    mcpEnvText = mapToLines(s.environment, '=')
    mcpHeadersText = mapToLines(s.headers, ': ')
    mcpError = ''
  }

  const saveMCP = () => runMCP('save', async () => {
    const server = new config.MCPServerConfig({
      name: mcpName.trim(),
      command: mcpKind === 'stdio' ? mcpCommand.trim().split(/\s+/).filter(Boolean) : [],
      url: mcpKind === 'http' ? mcpUrl.trim() : '',
      environment: mcpKind === 'stdio' ? parseLines(mcpEnvText, '=') : {},
      headers: mcpKind === 'http' ? parseLines(mcpHeadersText, ':') : {},
    })
    await SaveMCPServer(mcpOriginal, server)
    resetMCPForm()
    await loadMCP()
  })

  const removeMCP = (name: string) => runMCP('rm:' + name, async () => {
    await RemoveMCPServer(name)
    if (mcpOriginal === name) resetMCPForm()
    await loadMCP()
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
  const mcpPresets: { name: string; desc: string; command?: string[]; url?: string }[] = [
    { name: 'context7', desc: 'Up-to-date library docs', command: ['npx', '-y', '@upstash/context7-mcp'] },
    { name: 'sequential-thinking', desc: 'Step-by-step reasoning scratchpad', command: ['npx', '-y', '@modelcontextprotocol/server-sequential-thinking'] },
    { name: 'memory', desc: 'Knowledge-graph memory', command: ['npx', '-y', '@modelcontextprotocol/server-memory'] },
    { name: 'js-repl', desc: 'Run JavaScript/Node code', command: ['npx', '-y', 'mcp-repl'] },
    { name: 'exa', desc: 'Web search (needs API key header)', url: 'https://mcp.exa.ai/mcp' },
  ]

  const presetTaken = (name: string) => mcpServers.some((s) => s.name.toLowerCase() === name.toLowerCase())

  const addPreset = (p: (typeof mcpPresets)[number]) => runMCP('preset:' + p.name, async () => {
    await SaveMCPServer('', new config.MCPServerConfig({
      name: p.name, command: p.command ?? [], url: p.url ?? '',
    }))
    await loadMCP()
  })

  function statusColor(status: string): string {
    const c = status === 'connected' ? '#3fb950' : status === 'failed' ? '#f85149' : '#8b949e'
    return `background:${c}`
  }

  // ---------- Skills (discovered SKILL.md + plugin install) ----------
  type SkillRow = { name: string; description: string; dir: string }
  let extSkills = $state<SkillRow[]>([])
  let skillBusy = $state('')
  let skillError = $state('')
  let skillInstallUrl = $state('')
  let skillInstallResult = $state('')
  let skillConfirm = $state('') // name pending delete confirmation

  async function loadSkills() {
    extSkills = await ListExternalSkills()
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

  const removeSkill = (name: string) => {
    if (skillConfirm !== name) {
      skillConfirm = name
      return
    }
    skillConfirm = ''
    void runSkill('rm:' + name, async () => {
      await RemoveExternalSkill(name)
      await loadSkills()
    })
  }

  const refreshSkills = () => runSkill('refresh', async () => {
    await RefreshSkills()
    await loadSkills()
  })

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
      { id: 'skills', label: t('settings.skills'), icon: '🧩' },
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
            if (v === 'auto') setAutoEditorTheme()
            else if (v === 'vs-dark' || v === 'vs') setBuiltinEditorTheme(v)
          }}>
            <option value="auto">{t('settings.codeThemeAuto')}</option>
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
    {:else if active === 'skills'}
      <h2>{t('settings.skills')}</h2>
      <p class="muted set-sub">{t('settings.skillsDesc')}</p>

      <div class="settings-card">
        <div class="card-form">
          <div class="mset-keyrow">
            <div class="eyebrow" style="flex:1">{t('settings.skillsInstalled')}</div>
            <button class="ctrl" disabled={skillBusy !== ''} onclick={refreshSkills}>
              {skillBusy === 'refresh' ? t('settings.refreshing') : t('settings.refresh')}
            </button>
          </div>
        </div>
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
              <button class="ctrl" class:danger={skillConfirm === s.name} disabled={skillBusy !== ''} onclick={() => removeSkill(s.name)}>
                {skillConfirm === s.name ? t('settings.confirmRemove') : t('settings.remove')}
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
            <button class="ctrl" disabled={skillBusy !== '' || !skillInstallUrl.trim()} onclick={installSkill}>
              {skillBusy === 'install' ? t('settings.installing') : t('settings.install')}
            </button>
          </div>
          <div class="d muted">{t('settings.skillInstallHint')}</div>
          {#if skillInstallResult}<pre class="skill-result">{skillInstallResult}</pre>{/if}
          {#if skillError}<div class="mset-error">{skillError}</div>{/if}
        </div>
      </div>
    {:else if active === 'mcp'}
      <h2>{t('settings.mcpServers')}</h2>
      <p class="muted set-sub">{t('settings.mcpDesc')}</p>

      <div class="settings-card">
        {#if mcpServers.length > 3}
          <div class="card-form">
            <input class="ctrl" placeholder={t('settings.mcpSearchPlaceholder')} bind:value={mcpQuery} />
          </div>
        {/if}
        {#if mcpServers.length === 0}
          <div class="muted">{t('settings.noMcpServers')}</div>
        {:else}
          {#each mcpFiltered as s (s.name)}
            <div class="set-row" class:mcp-off={s.disabled}>
              <div class="set-txt">
                <div class="t">
                  <span class="dot" style={statusColor(s.status)}></span> {s.name}
                  <span class="mcp-badge">{s.url ? 'http' : 'stdio'}</span>
                  {#if s.tools > 0}<span class="mcp-badge">{t('settings.mcpToolCount', { n: String(s.tools) })}</span>{/if}
                </div>
                <div class="d">{s.url || (s.command ?? []).join(' ')}{s.err ? ' — ' + s.err : ''}</div>
              </div>
              <div style="display:flex; gap:8px; align-items:center">
                <label class="mswitch" title={s.disabled ? t('settings.add') : ''}>
                  <input type="checkbox" checked={!s.disabled} disabled={mcpBusy !== ''} onchange={() => toggleMCP(s)} />
                  <span></span>
                </label>
                <button class="ctrl" disabled={mcpBusy !== '' || s.disabled} onclick={() => testMCP(s.name)}>
                  {mcpBusy === 'test:' + s.name ? t('settings.testing') : t('settings.test')}
                </button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => editMCP(s)}>{t('settings.edit')}</button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => removeMCP(s.name)}>{t('settings.remove')}</button>
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

          <div class="mset-keyrow">
            <button class="ctrl" disabled={mcpBusy !== '' || !mcpFormValid} onclick={saveMCP}>
              {mcpBusy === 'save' ? t('settings.saving') : (mcpOriginal ? t('settings.save') : t('settings.add'))}
            </button>
            {#if mcpOriginal}
              <button class="ctrl" disabled={mcpBusy !== ''} onclick={resetMCPForm}>{t('settings.cancel')}</button>
            {/if}
          </div>
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
              <div class="d">{p.desc} — {p.url ?? p.command?.join(' ')}</div>
            </div>
            <button class="ctrl" disabled={mcpBusy !== '' || presetTaken(p.name)} onclick={() => addPreset(p)}>
              {mcpBusy === 'preset:' + p.name ? t('settings.adding') : t('settings.add')}
            </button>
          </div>
        {/each}
      </div>
    {/if}
    </div>
  </div>
</div>
