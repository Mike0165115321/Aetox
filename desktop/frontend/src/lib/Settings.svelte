<script lang="ts">
  import { onMount } from 'svelte'
  import { theme, toggleTheme } from './theme.svelte'
  import {
    SupportedProviders, HasAPIKey, RequiresAPIKey, TerminalShells,
    ListModelsForProvider, ProviderBaseURL,
    ListMCPServers, AddMCPServer, RemoveMCPServer, TestMCPServer,
  } from '../../wailsjs/go/main/App'
  import { cockpit, switchProvider, switchModel, submitAPIKey } from './stores/cockpit.svelte'

  let { onClose }: { onClose: () => void } = $props()

  // ---------- General: default shell ----------
  let shells = $state<{ name: string; path: string }[]>([])
  let defaultShell = $state(localStorage.getItem('defaultShell') ?? '')

  function saveDefaultShell() {
    localStorage.setItem('defaultShell', defaultShell)
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
  const sections = [
    { group: 'ส่วนบุคคล', items: [
      { id: 'general', label: 'ทั่วไป', icon: '⚙' },
      { id: 'appearance', label: 'รูปลักษณ์', icon: '🎨' },
    ]},
    { group: 'โมเดล AI', items: [
      { id: 'models', label: 'Model settings', icon: '🧠' },
    ]},
    { group: 'เครื่องมือ', items: [
      { id: 'mcp', label: 'MCP servers', icon: '🔌' },
    ]},
  ]

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
    <button class="settings-back" onclick={onClose}>← กลับไปที่แอป</button>
    <input class="settings-search" placeholder="ค้นหาการตั้งค่า…" bind:value={query} />
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
    {#if active === 'general'}
      <h2>ทั่วไป</h2>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">เชลล์สำหรับเทอร์มินัลในตัว</div>
            <div class="d">เลือกเชลล์ที่จะเปิดเมื่อสร้างแท็บเทอร์มินัลใหม่</div>
          </div>
          {#if shells.length === 0}
            <span class="muted">ไม่พบ shell ในเครื่อง</span>
          {:else}
            <select class="ctrl" bind:value={defaultShell} onchange={saveDefaultShell}>
              {#each shells as s}
                <option value={s.path}>{s.name}</option>
              {/each}
            </select>
          {/if}
        </div>
      </div>
    {:else if active === 'appearance'}
      <h2>รูปลักษณ์</h2>
      <div class="settings-card">
        <div class="set-row">
          <div class="set-txt">
            <div class="t">ธีม</div>
            <div class="d">สลับโหมดมืด/สว่างของแอป</div>
          </div>
          <button class="ctrl" onclick={toggleTheme}>{theme.name === 'dark' ? '🌙 Dark' : '☀ Light'}</button>
        </div>
      </div>
    {:else if active === 'models'}
      <h2>Model settings</h2>
      <p class="muted set-sub">จัดการผู้ให้บริการโมเดล เลือก provider แล้วตั้งคีย์/เลือกโมเดลที่จะใช้ในแชทได้ทันที</p>

      <div class="settings-card mset">
        <aside class="mset-side">
          <div class="settings-group-label eyebrow">Providers</div>
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
                <span class="badge on">Active</span>
              {:else}
                <button class="ctrl" disabled={busy !== ''} onclick={useProvider}>
                  {busy === 'provider' ? 'กำลังสลับ…' : 'ใช้ provider นี้'}
                </button>
              {/if}
            </div>

            <div class="mset-field">
              <div class="eyebrow">Base URL</div>
              <div class="mset-ro">{baseURL || '—'}</div>
            </div>

            {#if selectedRow.requiresKey}
              <div class="mset-field">
                <div class="eyebrow">API key</div>
                <div class="mset-keyrow">
                  <input
                    class="ctrl key-input" type={showKey ? 'text' : 'password'}
                    placeholder={selectedRow.hasKey ? 'ตั้งค่าแล้ว — วางคีย์ใหม่เพื่อแทนที่' : 'วาง API key…'}
                    bind:value={keyDraft}
                    onkeydown={(e) => e.key === 'Enter' && saveKey()}
                  />
                  <button class="icobtn tiny" aria-label="Show key" onclick={() => (showKey = !showKey)}>👁</button>
                  <button class="ctrl" disabled={busy === 'key' || !keyDraft.trim()} onclick={saveKey}>
                    {busy === 'key' ? 'Saving…' : 'Save'}
                  </button>
                </div>
              </div>
            {/if}

            <div class="mset-field">
              <div class="eyebrow">Model list</div>
              {#if loadingModels}
                <div class="muted">กำลังโหลดรายชื่อโมเดล…</div>
              {:else if models.length === 0}
                <div class="muted">ไม่พบรายชื่อโมเดล — ใส่ model id เองด้านล่างได้</div>
              {:else}
                {#each models as m}
                  <div class="mrow">
                    <span class="mname">{m}</span>
                    {#if isActiveProvider && cockpit.model.modelName === m}
                      <span class="badge on">ใช้อยู่</span>
                    {:else}
                      <button class="ctrl" disabled={busy !== ''} onclick={() => useModel(m)}>
                        {busy === m ? 'กำลังสลับ…' : 'ใช้'}
                      </button>
                    {/if}
                  </div>
                {/each}
              {/if}
              <div class="mset-keyrow">
                <input
                  class="ctrl key-input" placeholder="model id อื่นๆ เช่น gpt-4o…"
                  bind:value={customModel}
                  onkeydown={(e) => e.key === 'Enter' && customModel.trim() && useModel(customModel.trim())}
                />
                <button class="ctrl" disabled={busy !== '' || !customModel.trim()} onclick={() => useModel(customModel.trim())}>ใช้</button>
              </div>
            </div>

            {#if errorMsg}
              <div class="mset-error">{errorMsg}</div>
            {/if}
          {/if}
        </div>
      </div>
    {:else if active === 'mcp'}
      <h2>MCP servers</h2>
      <p class="muted set-sub">เชื่อมต่อ MCP server ภายนอก (stdio) เพื่อเพิ่มเครื่องมือให้ผู้ช่วย — เครื่องมือจาก MCP จะถามยืนยันก่อนรันเสมอ</p>

      <div class="settings-card">
        {#if mcpServers.length === 0}
          <div class="muted">ยังไม่มี MCP server — เพิ่มด้านล่าง</div>
        {:else}
          {#each mcpServers as s}
            <div class="set-row">
              <div class="set-txt">
                <div class="t"><span class="dot" style={statusColor(s.status)}></span> {s.name}</div>
                <div class="d">{s.command.join(' ')}{s.err ? ' — ' + s.err : ''}</div>
              </div>
              <div style="display:flex; gap:8px">
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => testMCP(s.name)}>
                  {mcpBusy === 'test:' + s.name ? 'กำลังทดสอบ…' : 'ทดสอบ'}
                </button>
                <button class="ctrl" disabled={mcpBusy !== ''} onclick={() => removeMCP(s.name)}>ลบ</button>
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <div class="settings-card">
        <div class="eyebrow">เพิ่ม server</div>
        <div class="mset-keyrow">
          <input class="ctrl" placeholder="ชื่อ เช่น filesystem" bind:value={mcpName} />
        </div>
        <div class="mset-keyrow">
          <input
            class="ctrl key-input"
            placeholder="คำสั่ง เช่น npx -y @modelcontextprotocol/server-filesystem /path"
            bind:value={mcpCommand}
            onkeydown={(e) => e.key === 'Enter' && mcpName.trim() && mcpCommand.trim() && addMCP()}
          />
          <button class="ctrl" disabled={mcpBusy !== '' || !mcpName.trim() || !mcpCommand.trim()} onclick={addMCP}>
            {mcpBusy === 'add' ? 'กำลังเพิ่ม…' : 'เพิ่ม'}
          </button>
        </div>
        {#if mcpError}<div class="mset-error">{mcpError}</div>{/if}
      </div>
    {/if}
  </div>
</div>
