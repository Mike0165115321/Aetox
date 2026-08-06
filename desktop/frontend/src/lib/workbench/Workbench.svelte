<script lang="ts">
  import { onMount } from 'svelte'
  import Terminal from '../Terminal.svelte'
  import FileEditor from '../FileEditor.svelte'
  import FilesPane from './FilesPane.svelte'
  import BrowserPane from './BrowserPane.svelte'
  import ToolsPane from './ToolsPane.svelte'
  import ExternalFilePane from './ExternalFilePane.svelte'
  import SheetPane from './SheetPane.svelte'
  import ImagePane from './ImagePane.svelte'
  import MediaPane from './MediaPane.svelte'
  import PdfPane from './PdfPane.svelte'
  import { fileURL } from '../fileUrl'
  import { cockpit } from '../stores/cockpit.svelte'
  import {
    workbench, activateTab, closeTab, removeTab,
    openFilesTab, openBrowserTab, openTerminalTab, openToolsTab, openFileTab, reportDeskTabs,
    openUrlInWorkbench, saveWorkbenchSnapshot, normalizeUrl, labelForUrl,
    setTabDragPayload, TAB_DRAG_MIME,
    type WorkbenchTab,
  } from '../stores/workbench.svelte'
  import { TerminalShells, BrowserBack, BrowserForward, BrowserReload, BrowserOpenDevTools } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import type { IconName } from '../icons'

  const tabIcon: Record<string, IconName> = { terminal: 'keyboard', browser: 'globe', files: 'copy', file: 'fileText', tools: 'wrench' }

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
    // Same trigger, same reason: the agent's view of the desk has to change
    // when the desk does, including when the user is the one who changed it.
    reportDeskTabs()
  })

  onMount(() => {
    TerminalShells().then((s) => (shells = s))
    // The three ways the agent reaches this desk. Each mirrors a door the user
    // already has — a page, a file, a shell — so nothing here can put something
    // on the desk that a click could not have.
    const offs = [
      // browser_open
      EventsOn('workbench:open-browser', ({ id, url }: { id: string; url: string }) => {
        if (!workbench.tabs.some((t) => t.id === id)) {
          workbench.tabs.push({ id, kind: 'browser', name: t('workbench.newTab'), url, mine: true })
        }
        workbench.activeId = id
      }),
      // desk_open — straight into the same opener the tree and the drop use, so
      // the routing table stays the only thing that decides which pane draws it.
      EventsOn('workbench:open-file', ({ path, name }: { path: string; name: string }) => {
        void openFileTab(path, name)
      }),
      // desk_terminal — the session already exists on the Go side (unlike the
      // browser, where the frontend creates the window), so this only mounts a
      // pane onto an id that is already live.
      EventsOn('workbench:open-terminal', ({ id, name }: { id: string; name: string }) => {
        if (!workbench.tabs.some((tab) => tab.id === id)) {
          workbench.tabs.push({ id, kind: 'terminal', name, mine: true })
        }
        workbench.activeId = id
      }),
    ]
    return () => offs.forEach((off) => off())
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
    tab.name = labelForUrl(url)
  }

  function browserCmd(fn: (id: string) => Promise<void>) {
    const tab = activeTab
    if (tab?.kind === 'browser' && tab.url) fn(tab.id)
  }

  // Lets a file/browser tab be dragged into the chat composer to attach its
  // content — see Chat.svelte's ondrop, which reads this same MIME type.
  //
  // A browser tab still on its start page is deliberately not draggable: it has
  // no native window and therefore no text, and offering the drag anyway ended
  // in `no browser tab "web-2"` after the user had already let go.
  // `!!tab.path` is not belt-and-braces: a drop that failed outright also opens
  // a file tab, and that one has no path (openDropError). Dragging it staged an
  // attachment with an empty path, which then went into the transcript as
  // "[attachment: … read it with read] " with nothing after it — a wasted turn
  // and a stored line that re-renders broken on every reopen.
  const canDrag = (tab: WorkbenchTab) =>
    (tab.kind === 'file' && !!tab.path) || (tab.kind === 'browser' && !!tab.url)
  function onTabDragStart(e: DragEvent, tab: WorkbenchTab) {
    if (!canDrag(tab)) return
    const ref = tab.kind === 'file' ? tab.path ?? '' : tab.id
    setTabDragPayload(e, tab.kind as 'file' | 'browser', ref, tab.name)
  }

  // ---- the desk as a drop target ----
  // Whatever the user is already holding lands here and opens: a file the agent
  // just made, dragged off its card in the chat; a page dragged off a real
  // browser's tab strip; anything at all out of Explorer.
  //
  // The window-level watch is not decoration. A browser tab's page is a real OS
  // window composited over this pane (see BrowserPane), so while one is up the
  // DOM underneath cannot see a drag at all — the pointer is over another
  // window. Noticing the drag anywhere in the app and lowering that window for
  // its duration is what makes this pane reachable; without it, dropping onto a
  // desk with a page open would silently do nothing.
  let dragging = $state(false) // something droppable is in flight over the app
  let overDesk = $state(false) // ...and it is over this panel right now
  let dragIdle: ReturnType<typeof setTimeout> | undefined

  function droppable(dt: DataTransfer | null): boolean {
    if (!dt) return false
    return dt.types.includes('Files') || dt.types.includes('text/uri-list') || dt.types.includes(TAB_DRAG_MIME)
  }

  function onWindowDragOver(e: DragEvent) {
    if (!droppable(e.dataTransfer)) return
    dragging = true
    // A drag that started in Explorer and wanders back out of the window fires
    // no dragend here — it was never ours to end. So the end of one is inferred
    // from dragover falling quiet (Chromium repeats it while the pointer is
    // inside, ~every 350ms even when still).
    clearTimeout(dragIdle)
    dragIdle = setTimeout(endDrag, 800)
  }

  function endDrag() {
    clearTimeout(dragIdle)
    dragging = false
    overDesk = false
  }

  function onDeskDragOver(e: DragEvent) {
    if (!droppable(e.dataTransfer)) return
    e.preventDefault() // the one thing that makes a drop land at all
    e.dataTransfer!.dropEffect = 'copy'
    overDesk = true
  }

  function onDeskDragLeave(e: DragEvent) {
    // Crossing between children fires leave/enter pairs; only a leave that
    // actually exits the panel counts.
    if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null)) overDesk = false
  }

  // Text dragged out of a page is a selection, not an address. Only something
  // that already reads as a URL gets opened as one — https://foo, E:\a\b.html,
  // or a bare host — so dropping three highlighted words doesn't navigate.
  function looksLikeUrl(s: string): boolean {
    if (/\s/.test(s)) return false
    return /^[a-z][a-z0-9+.-]*:/i.test(s) || /^[a-z]:[\\/]/i.test(s) || /^[\w-]+(\.[\w-]+)+(:\d+)?([/?#].*)?$/.test(s)
  }

  async function onDeskDrop(e: DragEvent) {
    const dt = e.dataTransfer
    endDrag()
    if (!dt) return
    // A dropped OS file carries no readable path in the DOM. Wails resolves
    // those natively and App.svelte routes them here by drop coordinates
    // (OnFileDrop) — this handler must not swallow the event first.
    if (dt.types.includes('Files')) return
    e.preventDefault()

    const card = dt.getData(TAB_DRAG_MIME)
    if (card) {
      try {
        const { kind, ref, label } = JSON.parse(card) as { kind: 'file' | 'browser'; ref: string; label: string }
        if (kind === 'file') await openFileTab(ref, label)
        else activateTab(ref) // a browser tab dragged inside its own desk: just focus it
      } catch {
        // Not our payload after all — nothing to open.
      }
      return
    }

    const url = (dt.getData('text/uri-list') || dt.getData('text/plain') || '')
      .split('\n')
      .map((l) => l.trim())
      .find((l) => l && !l.startsWith('#')) // uri-list comments
    if (url && looksLikeUrl(url)) openUrlInWorkbench(normalizeUrl(url))
  }

  function closeMenuOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.plus-menu-wrap')) menuOpen = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { menuOpen = false; return }
    if (!e.ctrlKey || e.altKey || e.metaKey) return
    const k = e.key.toLowerCase()
    if (!e.shiftKey && k === 't') { e.preventDefault(); openBrowserTab() }
    else if (!e.shiftKey && k === 'p') { e.preventDefault(); openFilesTab() }
  }
</script>

<svelte:window
  onclick={menuOpen ? closeMenuOnOutsideClick : undefined} onkeydown={onKeydown}
  ondragover={onWindowDragOver} ondrop={endDrag} ondragend={endDrag}
/>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- Drop target for the whole panel; the tabs and panes inside stay the real
     interactive elements. Keyboard users reach every one of these by the +
     menu, Ctrl+T / Ctrl+P, and the chat's file cards. -->
<div class="wb" ondragover={onDeskDragOver} ondragleave={onDeskDragLeave} ondrop={onDeskDrop}>
  <div class="insp-tabs">
    {#each workbench.tabs as tab (tab.id)}
      <button
        class="tab" class:active={workbench.activeId === tab.id} title={tab.name} onclick={() => activateTab(tab.id)}
        draggable={canDrag(tab)}
        ondragstart={(e) => onTabDragStart(e, tab)}
      >
        <span class="ic"><Icon name={tabIcon[tab.kind] ?? 'fileText'} size={13} /></span>
        <span class="label">{tab.name}</span>
        <span
          class="tab-close" role="button" tabindex="0" aria-label={t('workbench.close', { name: tab.name })}
          onclick={(e) => { e.stopPropagation(); closeTab(tab) }}
          onkeydown={(e) => e.key === 'Enter' && closeTab(tab)}
        ><Icon name="x" size={12} /></span>
      </button>
    {/each}
    <div class="plus-menu-wrap">
      <button class="icobtn tiny plus-btn" aria-label={t('workbench.addTab')} data-tip={t('workbench.addTab')} onclick={() => (menuOpen = !menuOpen)}><Icon name="plus" size={14} /></button>
      {#if menuOpen}
        <div class="plus-menu">
          <button class="plus-menu-item" disabled={shells.length === 0} onclick={openDefaultTerminal}><span class="ic"><Icon name="keyboard" size={14} /></span> {t('workbench.terminalMenu')}</button>
          <button class="plus-menu-item" onclick={() => pick(openBrowserTab)}><span class="ic"><Icon name="globe" size={14} /></span> {t('workbench.browserMenu')} <span class="kbd">Ctrl+T</span></button>
          <button class="plus-menu-item" onclick={() => pick(openFilesTab)}><span class="ic"><Icon name="copy" size={14} /></span> {t('workbench.filesTab')} <span class="kbd">Ctrl+P</span></button>
          <button class="plus-menu-item" onclick={() => pick(openToolsTab)}><span class="ic"><Icon name="wrench" size={14} /></span> {t('workbench.toolsTab')}</button>
        </div>
      {/if}
    </div>
  </div>
  {#if activeTab?.kind === 'browser'}
  <div class="insp-addr">
    <button class="icobtn tiny" aria-label={t('workbench.back')} data-tip={t('workbench.back')} onclick={() => browserCmd(BrowserBack)}><Icon name="arrowLeft" size={14} /></button>
    <button class="icobtn tiny" aria-label={t('workbench.forward')} data-tip={t('workbench.forward')} onclick={() => browserCmd(BrowserForward)}><Icon name="arrowRight" size={14} /></button>
    <button class="icobtn tiny" aria-label={t('workbench.reload')} data-tip={t('workbench.reload')} onclick={() => browserCmd(BrowserReload)}><Icon name="rotateCw" size={14} /></button>
    <input
      class="insp-url" placeholder={t('workbench.urlPlaceholder')} bind:value={urlDraft}
      onkeydown={(e) => e.key === 'Enter' && navigate()}
    />
    <button class="icobtn tiny" aria-label={t('workbench.go')} data-tip={t('workbench.go')} onclick={navigate}><Icon name="externalLink" size={14} /></button>
    <button class="icobtn tiny tip-r" aria-label={t('workbench.devtools')} data-tip={t('workbench.devtools')} onclick={() => browserCmd(BrowserOpenDevTools)}><Icon name="wrench" size={14} /></button>
    <!-- A transparent native <select> over the ⋮ glyph. Chromium renders its
         popup as an OS window, so it floats above the tab's own native window —
         a DOM dropdown here is invisible unless the page hides, which reads as
         the page crashing. Looks like a button, behaves like the platform. -->
    <span class="vp-picker tip-r" data-tip={activeTab?.viewport ? `${activeTab.viewport.w}×${activeTab.viewport.h}` : t('workbench.viewportFill')}>
      <span class="icobtn tiny" aria-hidden="true"><Icon name="ellipsisVertical" size={14} /></span>
      <select
        class="vp-select" aria-label={t('workbench.viewport')} value={activeTab?.viewport?.name ?? ''}
        onchange={(e) => setViewport(e.currentTarget.value)}
      >
        <option value="">{t('workbench.viewportFill')}</option>
        {#each DEVICES as d}
          <option value={d.name}>{d.name} ({d.w}×{d.h})</option>
        {/each}
      </select>
    </span>
  </div>
  {/if}

  <div class="insp-body">
    {#if workbench.tabs.length === 0}
      <div class="insp-start">
        <button class="plus-menu-item" disabled={shells.length === 0} onclick={openDefaultTerminal}><span class="ic"><Icon name="keyboard" size={14} /></span> {t('workbench.terminalMenu')}</button>
        <button class="plus-menu-item" onclick={() => openBrowserTab()}><span class="ic"><Icon name="globe" size={14} /></span> {t('workbench.browserMenu')} <span class="kbd">Ctrl+T</span></button>
        <button class="plus-menu-item" onclick={openFilesTab}><span class="ic"><Icon name="copy" size={14} /></span> {t('workbench.filesTab')} <span class="kbd">Ctrl+P</span></button>
        <button class="plus-menu-item" onclick={openToolsTab}><span class="ic"><Icon name="wrench" size={14} /></span> {t('workbench.toolsTab')}</button>
      </div>
    {/if}
    {#each workbench.tabs as tab (tab.id)}
      <div class="insp-slot" style="display:{workbench.activeId === tab.id ? 'block' : 'none'}">
        {#if tab.kind === 'terminal'}
          <Terminal sessionId={tab.id} onExit={() => removeTab(tab.id)} />
        {:else if tab.kind === 'files'}
          <FilesPane />
        {:else if tab.kind === 'tools'}
          <ToolsPane />
        {:else if tab.kind === 'file'}
          <!-- Keyed on rev so a re-read actually lands on screen: FileEditor
               copies `content` into its own state once and this pane never
               unmounts on its own, so without the key a file the agent had
               rewritten would keep showing the previous turn's bytes. Nothing
               renders until the first read resolves, rather than flashing an
               empty editor at every open. -->
          {#key tab.rev}
            {#if tab.view === 'image'}
              <ImagePane src={fileURL(tab.path ?? '')} name={tab.name} path={tab.path ?? ''} />
            {:else if tab.view === 'video' || tab.view === 'audio'}
              <MediaPane path={tab.path ?? ''} name={tab.name} kind={tab.view} />
            {:else if tab.view === 'pdf'}
              <PdfPane path={tab.path ?? ''} name={tab.name} />
            {:else if tab.sheet}
              <SheetPane path={tab.path ?? ''} preview={tab.sheet} />
            {:else if tab.unreadable}
              <ExternalFilePane path={tab.path ?? ''} reason={tab.unreadable} name={tab.name} />
            {:else if tab.content !== undefined}
              <FileEditor path={tab.path ?? ''} content={tab.content} />
            {/if}
          {/key}
        {:else}
          <BrowserPane tab={tab} active={workbench.activeId === tab.id} menuOpen={menuOpen} dragging={dragging} />
        {/if}
      </div>
    {/each}
  </div>

  {#if hasActiveTask}
    <div class="insp-foot">
      <button class="stopbtn"><Icon name="square" size={14} /> {t('workbench.stopTask')}</button>
    </div>
  {/if}

  {#if dragging}
    <!-- Up from the moment something is picked up anywhere in the app, not just
         once it is over the panel: the point is to answer "can I put this here?"
         while the user is still deciding where to aim. -->
    <div class="wb-drop" class:over={overDesk}>
      <div class="wb-drop-card">
        <Icon name="plus" size={20} />
        <span class="wb-drop-title">{t('workbench.dropHere')}</span>
        <span class="wb-drop-sub">{t('workbench.dropHint')}</span>
      </div>
    </div>
  {/if}
</div>

<style>
  /* .inspector is the flex column; this fills it and gives the overlay below
     something to be absolute against. */
  .wb { flex: 1; min-height: 0; display: flex; flex-direction: column; position: relative; }

  .wb-drop {
    position: absolute;
    inset: 6px;
    z-index: 5;
    pointer-events: none; /* the drag events belong to .wb underneath */
    display: flex;
    align-items: center;
    justify-content: center;
    border: 2px dashed var(--border-default);
    border-radius: var(--r-md);
    background: color-mix(in srgb, var(--surface-sunken) 82%, transparent);
    opacity: 0.75;
    transition: opacity 120ms ease, border-color 120ms ease;
  }
  .wb-drop.over { opacity: 1; border-color: var(--accent); }

  .wb-drop-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    text-align: center;
    padding: 0 24px;
    color: var(--text-secondary);
  }
  .wb-drop.over .wb-drop-card { color: var(--text-primary); }
  .wb-drop-title { font-size: var(--fs-md); }
  .wb-drop-sub { font-size: var(--fs-xs); color: var(--text-dim); max-width: 30ch; }
</style>
