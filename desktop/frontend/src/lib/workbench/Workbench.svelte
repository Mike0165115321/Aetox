<script lang="ts">
  import { onMount } from 'svelte'
  import Terminal from '../Terminal.svelte'
  import FileEditor from '../FileEditor.svelte'
  import ReviewPane from './ReviewPane.svelte'
  import FilesPane from './FilesPane.svelte'
  import BrowserPane from './BrowserPane.svelte'
  import { cockpit } from '../stores/cockpit.svelte'
  import {
    workbench, activateTab, closeTab, removeTab,
    openReview, openFilesTab, openBrowserTab, openTerminalTab,
  } from '../stores/workbench.svelte'
  import { TerminalShells, BrowserBack, BrowserForward, BrowserReload } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'

  const tabIcon: Record<string, string> = { review: '▤', terminal: '⌨', browser: '🌐', files: '⧉', file: '📄' }

  let shells = $state<{ name: string; path: string }[]>([])
  let menuOpen = $state(false)
  let urlDraft = $state('')

  const activeTab = $derived(workbench.tabs.find((t) => t.id === workbench.activeId))
  const hasActiveTask = $derived(cockpit.task.steps.some((s) => s.status === 'active'))

  $effect(() => {
    urlDraft = activeTab?.url ?? ''
  })

  onMount(() => {
    TerminalShells().then((s) => (shells = s))
    // The AI agent opens pages on its workbench through this event (browser_open skill).
    return EventsOn('workbench:open-browser', ({ id, url }: { id: string; url: string }) => {
      if (!workbench.tabs.some((t) => t.id === id)) {
        workbench.tabs.push({ id, kind: 'browser', name: 'New tab', url })
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

  function navigate() {
    let u = urlDraft.trim()
    if (!u) return
    if (!/^https?:\/\//i.test(u)) u = 'https://' + u
    let t = activeTab
    if (!t || t.kind !== 'browser') {
      const id = openBrowserTab()
      t = workbench.tabs.find((x) => x.id === id)
      if (!t) return
    }
    t.url = u
    try { t.name = new URL(u).hostname } catch { t.name = u }
  }

  function browserCmd(fn: (id: string) => Promise<void>) {
    const t = activeTab
    if (t?.kind === 'browser' && t.url) fn(t.id)
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
  {#each workbench.tabs as t (t.id)}
    <button class="tab" class:active={workbench.activeId === t.id} title={t.name} onclick={() => activateTab(t.id)}>
      <span class="ic">{tabIcon[t.kind]}</span>
      <span class="label">{t.name}</span>
      <span
        class="tab-close" role="button" tabindex="0" aria-label={`Close ${t.name}`}
        onclick={(e) => { e.stopPropagation(); closeTab(t) }}
        onkeydown={(e) => e.key === 'Enter' && closeTab(t)}
      >✕</span>
    </button>
  {/each}
  <div class="plus-menu-wrap">
    <button class="icobtn tiny plus-btn" aria-label="Add tab" onclick={() => (menuOpen = !menuOpen)}>+</button>
    {#if menuOpen}
      <div class="plus-menu">
        <button class="plus-menu-item" onclick={() => pick(openReview)}><span class="ic">▤</span> Review <span class="kbd">Ctrl+Shift+G</span></button>
        <button class="plus-menu-item" disabled={shells.length === 0} onclick={openDefaultTerminal}><span class="ic">⌨</span> Terminal</button>
        <button class="plus-menu-item" onclick={() => pick(openBrowserTab)}><span class="ic">🌐</span> Browser <span class="kbd">Ctrl+T</span></button>
        <button class="plus-menu-item" onclick={() => pick(openFilesTab)}><span class="ic">⧉</span> Files <span class="kbd">Ctrl+P</span></button>
      </div>
    {/if}
  </div>
  <div class="insp-tabs-icons">
    <span class="icobtn tiny" aria-label="Fullscreen">⤢</span>
    <span class="icobtn tiny" aria-label="Restore">▢</span>
    <span class="icobtn tiny" aria-label="Toggle sidebar">▤</span>
  </div>
</div>
<div class="insp-addr">
  <button class="icobtn tiny" aria-label="Back" onclick={() => browserCmd(BrowserBack)}>←</button>
  <button class="icobtn tiny" aria-label="Forward" onclick={() => browserCmd(BrowserForward)}>→</button>
  <button class="icobtn tiny" aria-label="Reload" onclick={() => browserCmd(BrowserReload)}>↻</button>
  <input
    class="insp-url" placeholder="Enter a URL" bind:value={urlDraft}
    onkeydown={(e) => e.key === 'Enter' && navigate()}
  />
  <button class="icobtn tiny" aria-label="Go" title="ไปที่หน้านี้" onclick={navigate}>↗</button>
  <span class="icobtn tiny">⋮</span>
</div>

<div class="insp-body">
  {#if workbench.tabs.length === 0}
    <div class="insp-blank">
      <span class="ic">🌐</span>
      <div class="insp-blank-title">Start browsing</div>
      <div class="insp-blank-sub">กด + เพื่อเปิดแท็บใหม่</div>
    </div>
  {/if}
  {#each workbench.tabs as t (t.id)}
    <div class="insp-slot" style="display:{workbench.activeId === t.id ? 'block' : 'none'}">
      {#if t.kind === 'review'}
        <ReviewPane />
      {:else if t.kind === 'terminal'}
        <Terminal sessionId={t.id} onExit={() => removeTab(t.id)} />
      {:else if t.kind === 'files'}
        <FilesPane />
      {:else if t.kind === 'file'}
        <FileEditor path={t.path ?? ''} content={t.content ?? ''} />
      {:else}
        <BrowserPane tab={t} active={workbench.activeId === t.id} />
      {/if}
    </div>
  {/each}
</div>

{#if hasActiveTask}
  <div class="insp-foot">
    <button class="stopbtn">⏸ Stop Current Task</button>
  </div>
{/if}
