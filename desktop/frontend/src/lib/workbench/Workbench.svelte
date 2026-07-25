<script lang="ts">
  import { onMount } from 'svelte'
  import Terminal from '../Terminal.svelte'
  import FileEditor from '../FileEditor.svelte'
  import ReviewPane from './ReviewPane.svelte'
  import FilesPane from './FilesPane.svelte'
  import BrowserPane from './BrowserPane.svelte'
  import ToolsPane from './ToolsPane.svelte'
  import { cockpit } from '../stores/cockpit.svelte'
  import {
    workbench, activateTab, closeTab, removeTab,
    openReview, openFilesTab, openBrowserTab, openTerminalTab, openToolsTab,
    saveWorkbenchSnapshot,
    type WorkbenchTab,
  } from '../stores/workbench.svelte'
  import { TerminalShells, BrowserBack, BrowserForward, BrowserReload, BrowserOpenDevTools } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { t } from '../i18n.svelte'

  const tabIcon: Record<string, string> = { review: '▤', terminal: '⌨', browser: '🌐', files: '⧉', file: '📄', tools: '🛠' }

  // Chrome DevTools' default device presets. CSS viewport sizes — BrowserPane
  // turns one into a real window of that aspect + a matching page zoom.
  const DEVICES = [
    { name: 'Galaxy S8+', w: 360, h: 740 },
    { name: 'iPhone SE', w: 375, h: 667 },
    { name: 'iPhone 12 Pro', w: 390, h: 844 },
    { name: 'Pixel 7', w: 412, h: 915 },
    { name: 'iPhone 14 Pro Max', w: 430, h: 932 },
    { name: 'iPad Mini', w: 768, h: 1024 },
    { name: 'iPad Pro', w: 1024, h: 1366 },
    { name: 'Desktop', w: 1280, h: 800 },
  ]

  let shells = $state<{ name: string; path: string }[]>([])
  let menuOpen = $state(false)
  let urlDraft = $state('')

  const activeTab = $derived(workbench.tabs.find((t) => t.id === workbench.activeId))
  const hasActiveTask = $derived(cockpit.task.steps.some((s) => s.status === 'active'))

  $effect(() => {
    urlDraft = activeTab?.url ?? ''
  })

  // Autosave the layout for the bound session on every tab change (open/close/
  // navigate/activate) — snapshot reads workbench state reactively.
  $effect(() => {
    saveWorkbenchSnapshot()
  })

  onMount(() => {
    TerminalShells().then((s) => (shells = s))
    // The AI agent opens pages on its workbench through this event (browser_open skill).
    return EventsOn('workbench:open-browser', ({ id, url }: { id: string; url: string }) => {
      if (!workbench.tabs.some((t) => t.id === id)) {
        workbench.tabs.push({ id, kind: 'browser', name: t('workbench.newTab'), url })
      }
      workbench.activeId = id
    })
  })

  function openDefaultTerminal() {
    if (shells.length === 0) return
    menuOpen = false
    const saved = localStorage.getItem('defaultShell')
    openTerminalTab(shells.find((s) => s.path === saved) ?? shells[0])
  }

  function pick(fn: () => void) {
    menuOpen = false
    fn()
  }

  /** Device-size preset for the active browser tab; '' = fill the pane. */
  function setViewport(name: string) {
    if (activeTab) activeTab.viewport = DEVICES.find((d) => d.name === name)
  }

  // "google.com" -> https://, "E:\site\index.html" -> file:///, and anything
  // that already carries a scheme (file:, http:, about:) passes through —
  // blindly prepending https:// turns file:/// URLs into a dead https://file/.
  function normalizeUrl(u: string): string {
    if (/^[a-z]:[\\/]/i.test(u)) return 'file:///' + u.replace(/\\/g, '/') // E:\site\index.html (before scheme check: "E:" looks like one)
    if (/^[a-z][a-z0-9+.-]*:\/\//i.test(u) || /^(about|data|mailto|javascript):/i.test(u)) return u
    return 'https://' + u // bare domain or host:port
  }

  function navigate() {
    const u = urlDraft.trim()
    if (!u) return
    const url = normalizeUrl(u)
    let tab = activeTab
    if (!tab || tab.kind !== 'browser') {
      const id = openBrowserTab()
      tab = workbench.tabs.find((x) => x.id === id)
      if (!tab) return
    }
    tab.url = url
    try {
      const p = new URL(url)
      tab.name = p.hostname || decodeURIComponent(p.pathname.split('/').pop() || url)
    } catch {
      tab.name = url
    }
  }

  function browserCmd(fn: (id: string) => Promise<void>) {
    const tab = activeTab
    if (tab?.kind === 'browser' && tab.url) fn(tab.id)
  }

  // Lets a file/browser tab be dragged into the chat composer to attach its
  // content — see Chat.svelte's ondrop, which reads this same MIME type.
  function onTabDragStart(e: DragEvent, tab: WorkbenchTab) {
    if (tab.kind !== 'file' && tab.kind !== 'browser') return
    const ref = tab.kind === 'file' ? tab.path ?? '' : tab.id
    e.dataTransfer?.setData('application/x-aetox-tab', JSON.stringify({ kind: tab.kind, ref, label: tab.name }))
    e.dataTransfer!.effectAllowed = 'copy'
  }

  function closeMenuOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.plus-menu-wrap')) menuOpen = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { menuOpen = false; return }
    if (!e.ctrlKey || e.altKey || e.metaKey) return
    const k = e.key.toLowerCase()
    if (e.shiftKey && k === 'g') { e.preventDefault(); openReview() }
    else if (!e.shiftKey && k === 't') { e.preventDefault(); openBrowserTab() }
    else if (!e.shiftKey && k === 'p') { e.preventDefault(); openFilesTab() }
  }
</script>

<svelte:window onclick={menuOpen ? closeMenuOnOutsideClick : undefined} onkeydown={onKeydown} />

<div class="insp-tabs">
  {#each workbench.tabs as tab (tab.id)}
    <button
      class="tab" class:active={workbench.activeId === tab.id} title={tab.name} onclick={() => activateTab(tab.id)}
      draggable={tab.kind === 'file' || tab.kind === 'browser'}
      ondragstart={(e) => onTabDragStart(e, tab)}
    >
      <span class="ic">{tabIcon[tab.kind]}</span>
      <span class="label">{tab.name}</span>
      <span
        class="tab-close" role="button" tabindex="0" aria-label={t('workbench.close', { name: tab.name })}
        onclick={(e) => { e.stopPropagation(); closeTab(tab) }}
        onkeydown={(e) => e.key === 'Enter' && closeTab(tab)}
      >✕</span>
    </button>
  {/each}
  <div class="plus-menu-wrap">
    <button class="icobtn tiny plus-btn" aria-label={t('workbench.addTab')} data-tip={t('workbench.addTab')} onclick={() => (menuOpen = !menuOpen)}>+</button>
    {#if menuOpen}
      <div class="plus-menu">
        <button class="plus-menu-item" onclick={() => pick(openReview)}><span class="ic">▤</span> {t('workbench.reviewTab')} <span class="kbd">Ctrl+Shift+G</span></button>
        <button class="plus-menu-item" disabled={shells.length === 0} onclick={openDefaultTerminal}><span class="ic">⌨</span> {t('workbench.terminalMenu')}</button>
        <button class="plus-menu-item" onclick={() => pick(openBrowserTab)}><span class="ic">🌐</span> {t('workbench.browserMenu')} <span class="kbd">Ctrl+T</span></button>
        <button class="plus-menu-item" onclick={() => pick(openFilesTab)}><span class="ic">⧉</span> {t('workbench.filesTab')} <span class="kbd">Ctrl+P</span></button>
        <button class="plus-menu-item" onclick={() => pick(openToolsTab)}><span class="ic">🛠</span> {t('workbench.toolsTab')}</button>
      </div>
    {/if}
  </div>
</div>
{#if activeTab?.kind === 'browser'}
<div class="insp-addr">
  <button class="icobtn tiny" aria-label={t('workbench.back')} data-tip={t('workbench.back')} onclick={() => browserCmd(BrowserBack)}>←</button>
  <button class="icobtn tiny" aria-label={t('workbench.forward')} data-tip={t('workbench.forward')} onclick={() => browserCmd(BrowserForward)}>→</button>
  <button class="icobtn tiny" aria-label={t('workbench.reload')} data-tip={t('workbench.reload')} onclick={() => browserCmd(BrowserReload)}>↻</button>
  <input
    class="insp-url" placeholder={t('workbench.urlPlaceholder')} bind:value={urlDraft}
    onkeydown={(e) => e.key === 'Enter' && navigate()}
  />
  <button class="icobtn tiny" aria-label={t('workbench.go')} data-tip={t('workbench.go')} onclick={navigate}>↗</button>
  <button class="icobtn tiny tip-r" aria-label={t('workbench.devtools')} data-tip={t('workbench.devtools')} onclick={() => browserCmd(BrowserOpenDevTools)}>🛠</button>
  <!-- A transparent native <select> over the ⋮ glyph. Chromium renders its
       popup as an OS window, so it floats above the tab's own native window —
       a DOM dropdown here is invisible unless the page hides, which reads as
       the page crashing. Looks like a button, behaves like the platform. -->
  <span class="vp-picker tip-r" data-tip={activeTab?.viewport ? `${activeTab.viewport.w}×${activeTab.viewport.h}` : t('workbench.viewportFill')}>
    <span class="icobtn tiny" aria-hidden="true">⋮</span>
    <select
      class="vp-select" aria-label={t('workbench.viewport')} value={activeTab?.viewport?.name ?? ''}
      onchange={(e) => setViewport(e.currentTarget.value)}
    >
      <option value="">{t('workbench.viewportFill')}</option>
      {#each DEVICES as d}
        <option value={d.name}>{d.name} — {d.w}×{d.h}</option>
      {/each}
    </select>
  </span>
</div>
{/if}

<div class="insp-body">
  {#if workbench.tabs.length === 0}
    <div class="insp-start">
      <button class="plus-menu-item" onclick={openReview}><span class="ic">▤</span> {t('workbench.reviewTab')} <span class="kbd">Ctrl+Shift+G</span></button>
      <button class="plus-menu-item" disabled={shells.length === 0} onclick={openDefaultTerminal}><span class="ic">⌨</span> {t('workbench.terminalMenu')}</button>
      <button class="plus-menu-item" onclick={() => openBrowserTab()}><span class="ic">🌐</span> {t('workbench.browserMenu')} <span class="kbd">Ctrl+T</span></button>
      <button class="plus-menu-item" onclick={openFilesTab}><span class="ic">⧉</span> {t('workbench.filesTab')} <span class="kbd">Ctrl+P</span></button>
      <button class="plus-menu-item" onclick={openToolsTab}><span class="ic">🛠</span> {t('workbench.toolsTab')}</button>
    </div>
  {/if}
  {#each workbench.tabs as tab (tab.id)}
    <div class="insp-slot" style="display:{workbench.activeId === tab.id ? 'block' : 'none'}">
      {#if tab.kind === 'review'}
        <ReviewPane />
      {:else if tab.kind === 'terminal'}
        <Terminal sessionId={tab.id} onExit={() => removeTab(tab.id)} />
      {:else if tab.kind === 'files'}
        <FilesPane />
      {:else if tab.kind === 'tools'}
        <ToolsPane />
      {:else if tab.kind === 'file'}
        <FileEditor path={tab.path ?? ''} content={tab.content ?? ''} />
      {:else}
        <BrowserPane tab={tab} active={workbench.activeId === tab.id} menuOpen={menuOpen} />
      {/if}
    </div>
  {/each}
</div>

{#if hasActiveTask}
  <div class="insp-foot">
    <button class="stopbtn">{t('workbench.stopTask')}</button>
  </div>
{/if}
