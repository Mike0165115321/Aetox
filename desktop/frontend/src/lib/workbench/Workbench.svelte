<script lang="ts">
  import { onMount } from 'svelte'
  import Terminal from '../Terminal.svelte'
  import FileEditor from '../FileEditor.svelte'
  import FilesPane from './FilesPane.svelte'
  import BrowserPane from './BrowserPane.svelte'
  import ExternalFilePane from './ExternalFilePane.svelte'
  import SheetPane from './SheetPane.svelte'
  import ImagePane from './ImagePane.svelte'
  import MediaPane from './MediaPane.svelte'
  import PdfPane from './PdfPane.svelte'
  import SlidesPane from './SlidesPane.svelte'
  import DeckRoom from './DeckRoom.svelte'
  import GitPane from './GitPane.svelte'
  import { fileURL } from '../fileUrl'
  import { cockpit } from '../stores/cockpit.svelte'
  import {
    workbench, activateTab, closeTab, removeTab,
    openFilesTab, openBrowserTab, openTerminalTab, openDecksTab, openGitTab, openFileTab, closeAgentFileTab,
    browserTabClosedByEngine, reportDeskTabs,
    openUrlInWorkbench, saveWorkbenchSnapshot, resolveAddressBarInput, labelForUrl,
    setTabDragPayload, TAB_DRAG_MIME,
    type WorkbenchTab,
  } from '../stores/workbench.svelte'
  import { busy, busyWork, layerOn, loadBusySignal, toggleBusyLayer } from '../stores/busySignal.svelte'
  import { TerminalShells, BrowserBack, BrowserForward, BrowserReload, BrowserOpenDevTools } from '../../../wailsjs/go/main/App'
  import { pagePick, startPagePick, stopPagePick, type PickMode } from './pagePick.svelte'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { t, type TKey } from '../i18n.svelte'
  import { isShortcut, shortcutLabel } from '../shortcuts'
  import Icon from '../Icon.svelte'
  import type { IconName } from '../icons'

  const tabIcon: Record<string, IconName> = { terminal: 'keyboard', browser: 'globe', files: 'copy', file: 'fileText', decks: 'layoutList' }

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
  // The busy-signal checklist, opened from the browser toolbar. Its own flag
  // rather than sharing menuOpen: two menus that close each other by accident
  // is the bug that costs a click every single time.
  let busyOpen = $state(false)
  let urlDraft = $state('')

  const activeTab = $derived(workbench.tabs.find((t) => t.id === workbench.activeId))
  const hasActiveTask = $derived(cockpit.task.steps.some((s) => s.status === 'active'))

  // ── ไฟบอกสถานะ (§174) ──────────────────────────────────────────────
  //
  // Three of the four layers are drawn here; the fourth is drawn inside the
  // page itself, because it has to be (desktop/browser_marks.go).
  //
  // Each is a switch AND a fact, never one of them. The switch says what the
  // panel may draw, the fact says whether there is anything to draw, and a
  // layer that lit on the switch alone would be a light that means "you left
  // this on" rather than "the agent is working".
  //
  // The three read different facts on purpose. The border and the tab mark
  // follow `running`, which is one call in flight and nothing else. The action
  // bar follows `seen` — the browser has been touched at some point this turn
  // — because mounting it resizes the native page underneath, and doing that
  // twice per call would reflow a page in the middle of being read (see
  // busyWork.seen).
  const busyGlow = $derived(layerOn('edgeGlow') && busyWork.running)
  const busyBar = $derived(layerOn('actionBar') && busyWork.seen)
  const busyDot = $derived(layerOn('tabDot') && busyWork.running)

  // The browser actions the bar has words for. Anything else lands on `other`,
  // which is a real sentence rather than a blank: an action added to the
  // browser tool and not to the dictionary should read as work being done, not
  // as the signal having broken.
  const BUSY_ACTS = new Set(['open', 'read', 'click', 'type', 'scroll', 'capture', 'tabs', 'wait', 'back', 'dialog', 'console', 'network'])

  /** What the bar says: the action in words, with the thing it is being done to.
   *
   *  Two forms per action rather than one phrase with the tense bolted on, so
   *  each language writes its own sentence — Thai puts the tense at both ends
   *  ("กำลังกด X" / "กด X แล้ว") and English changes the verb.
   *
   *  The collapse at the end is what lets one phrase serve an action that names
   *  something and one that does not: `click` has no subject, so "{subject}"
   *  resolves to nothing and the space it left goes with it. */
  const busyText = $derived.by(() => {
    const act = BUSY_ACTS.has(busyWork.act) ? busyWork.act : 'other'
    const key = `workbench.busy${busyWork.running ? 'Run' : 'End'}.${act}` as TKey
    return t(key, { subject: busyWork.subject }).replace(/\s+/g, ' ').trim()
  })

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
    // Read once, here rather than when the checklist opens. The panel draws
    // from these on the first browser call of the session, which is long before
    // anybody has a reason to open the menu — and layerOn's shipped-default
    // fallback is meant to cover the milliseconds of a round trip, not a user
    // who turned a layer off yesterday.
    void loadBusySignal()
    // The ways the agent reaches this desk. Each mirrors a door the user
    // already has — a page, a file, a shell, and the × on a tab — so nothing
    // here can do something to the desk that a click could not have.
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
        void openFileTab(path, name, true)
      }),
      // desk close — only ever a tab the agent opened itself; the store checks
      // that again against the live array rather than trusting the mirror.
      EventsOn('workbench:close-file', ({ path }: { path: string }) => {
        closeAgentFileTab(path)
      }),
      // The browser's half of the same pair, which it never had. Any close on
      // the Go side lands here — the agent closing its own tab, and the orphan
      // sweep after a reload — because a chip whose native view is gone is a
      // black rectangle nothing can repair.
      EventsOn('workbench:close-browser', ({ id }: { id: string }) => {
        browserTabClosedByEngine(id)
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

  async function navigate() {
    const u = urlDraft.trim()
    if (!u) return
    // Async because Go decides whether this is a place or a search — one line
    // of code and one round trip, in exchange for the address bar behaving the
    // way every other address bar does. See resolveAddressBarInput.
    const url = await resolveAddressBarInput(u)
    if (!url) return
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

  // Same gate as browserCmd: a tab still on its start page has no native
  // window to inject anything into.
  function togglePick(mode: PickMode) {
    const tab = activeTab
    if (tab?.kind === 'browser' && tab.url) startPagePick(tab.id, mode)
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
    if (url && looksLikeUrl(url)) openUrlInWorkbench(await resolveAddressBarInput(url))
  }

  function closeMenuOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.plus-menu-wrap')) menuOpen = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { menuOpen = false; if (pagePick.tabId) stopPagePick(); return }
    if (isShortcut(e, 'browserTab')) { e.preventDefault(); openBrowserTab() }
    else if (isShortcut(e, 'filesTab')) { e.preventDefault(); openFilesTab() }
    // Only reaches here while the app's own webview has focus. Once the page
    // has it, the chord is the page's to see — which is why the injected
    // overlay listens for Escape itself rather than trusting this handler.
    else if (isShortcut(e, 'pickElement')) { e.preventDefault(); togglePick('pick') }
    else if (isShortcut(e, 'drawOnPage')) { e.preventDefault(); togglePick('draw') }
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
<div class="wb" class:busy-glow={busyGlow} ondragover={onDeskDragOver} ondragleave={onDeskDragLeave} ondrop={onDeskDrop}>
  <div class="insp-tabs">
    <!-- The hairline half of จุดบนแท็บที่กำลังใช้: a lit line along the
         strip's own border, saying the row above it is where to look. Absolute
         so it lies ON the border rather than pushing it down — a strip that
         grew a pixel when the agent started working would move every tab, and
         resize the native browser window underneath. -->
    {#if busyDot}<span class="busy-strip-line" aria-hidden="true"></span>{/if}
    {#each workbench.tabs as tab (tab.id)}
      <button
        class="tab" class:active={workbench.activeId === tab.id} title={tab.name} onclick={() => activateTab(tab.id)}
        draggable={canDrag(tab)}
        ondragstart={(e) => onTabDragStart(e, tab)}
      >
        <span class="ic"><Icon name={tabIcon[tab.kind] ?? 'fileText'} size={13} /></span>
        <span class="label">{tab.name}</span>
        <!-- Breathing on the one tab the agent is working. tab.id is the id the
             engine minted (web-agent-N) and busyWork.tab is that same id come
             back on the tool event, so this is an identity check and not a
             guess. Empty tab means the engine could not say which — no dot
             anywhere, and the border light carries the message alone. -->
        {#if busyDot && busyWork.tab && tab.id === busyWork.tab}
          <span class="busy-tab-dot" aria-hidden="true"></span>
        {/if}
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
          <button class="plus-menu-item" onclick={() => pick(openBrowserTab)}><span class="ic"><Icon name="globe" size={14} /></span> {t('workbench.browserMenu')} <span class="kbd">{shortcutLabel('browserTab')}</span></button>
          <button class="plus-menu-item" onclick={() => pick(openFilesTab)}><span class="ic"><Icon name="copy" size={14} /></span> {t('workbench.filesTab')} <span class="kbd">{shortcutLabel('filesTab')}</span></button>
          <button class="plus-menu-item" onclick={() => pick(openDecksTab)}><span class="ic"><Icon name="layoutList" size={14} /></span> {t('workbench.decksTab')}</button>
          <!-- โค้ด desk only: a working tree is what that desk is held inside,
               and the storefront has no project to report on (§161.4). -->
          {#if cockpit.desk === 'coding'}
            <button class="plus-menu-item" onclick={() => pick(openGitTab)}><span class="ic"><Icon name="gitBranch" size={14} /></span> {t('workbench.gitTab')}</button>
          {/if}
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
    <span class="insp-sep" aria-hidden="true"></span>
    <!-- What the panel SHOWS you about the agent working, as against the two
         controls after it, which do something TO the page. It sits first
         because it is the only one here that changes nothing except how the
         panel reports itself. -->
    <span class="busy-wrap">
      <button
        class="icobtn tiny" class:active={busyOpen}
        aria-label={t('workbench.busySignal')} data-tip={t('workbench.busySignal')}
        aria-expanded={busyOpen} aria-haspopup="true"
        onclick={() => { busyOpen = !busyOpen; if (busyOpen) void loadBusySignal() }}
      ><Icon name="sparkles" size={14} /></button>
      {#if busyOpen}
        <div class="busy-menu" role="dialog" aria-label={t('workbench.busySignal')}>
          <p class="busy-head">{t('workbench.busySignalHint')}</p>
          {#each busy.layers as layer (layer.id)}
            <button
              class="busy-item" class:on={layer.on}
              role="menuitemcheckbox" aria-checked={layer.on}
              onclick={() => void toggleBusyLayer(layer.id, !layer.on)}
            >
              <span class="tick">{#if layer.on}<Icon name="check" size={12} />{/if}</span>
              <span class="txt"><b>{layer.label}</b><span>{layer.note}</span></span>
            </button>
          {/each}
        </div>
      {/if}
    </span>
    <!-- Pointing at the page is a way of talking to the agent, not a way of
         inspecting the page — the rule that separates it from the two controls
         after it, which are the user's own magnifying glass. -->
    <span class="insp-sep" aria-hidden="true"></span>
    <button
      class="icobtn tiny" class:active={pagePick.tabId === activeTab?.id && pagePick.mode === 'pick'}
      aria-label={t('workbench.pick')} data-tip={`${t('workbench.pick')} · ${shortcutLabel('pickElement')}`}
      onclick={() => togglePick('pick')}
    ><Icon name="pointer" size={14} /></button>
    <button
      class="icobtn tiny" class:active={pagePick.tabId === activeTab?.id && pagePick.mode === 'draw'}
      aria-label={t('workbench.draw')} data-tip={`${t('workbench.draw')} · ${shortcutLabel('drawOnPage')}`}
      onclick={() => togglePick('draw')}
    ><Icon name="pencil" size={14} /></button>
    <span class="insp-sep" aria-hidden="true"></span>
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
  <!-- แถบบอกการกระทำ: what is being done, and to what, in words.
       Words only. The first draft had a small spinner turning inside it and the
       owner took it straight back out — *"ฝากเอาอนิเมชั่นออกเลย ไม่จำเป็น"*.
       The border light already says "still going" and says it better; a second
       thing turning next to the sentence is decoration competing with the one
       part of this panel that has to be read.
       So the dot is a dot: lit while a call is in flight, out between them. -->
  {#if busyBar}
    <div class="busy-bar" class:live={busyWork.running} role="status" aria-live="polite">
      <span class="busy-bar-dot" aria-hidden="true"></span>
      <span class="busy-bar-txt">{busyText}</span>
    </div>
  {/if}
  {/if}

  <div class="insp-body">
    {#if workbench.tabs.length === 0}
      <div class="insp-start">
        <button class="plus-menu-item" disabled={shells.length === 0} onclick={openDefaultTerminal}><span class="ic"><Icon name="keyboard" size={14} /></span> {t('workbench.terminalMenu')}</button>
        <button class="plus-menu-item" onclick={() => openBrowserTab()}><span class="ic"><Icon name="globe" size={14} /></span> {t('workbench.browserMenu')} <span class="kbd">{shortcutLabel('browserTab')}</span></button>
        <button class="plus-menu-item" onclick={openFilesTab}><span class="ic"><Icon name="copy" size={14} /></span> {t('workbench.filesTab')} <span class="kbd">{shortcutLabel('filesTab')}</span></button>
        <button class="plus-menu-item" onclick={openDecksTab}><span class="ic"><Icon name="layoutList" size={14} /></span> {t('workbench.decksTab')}</button>
        {#if cockpit.desk === 'coding'}
          <button class="plus-menu-item" onclick={openGitTab}><span class="ic"><Icon name="gitBranch" size={14} /></span> {t('workbench.gitTab')}</button>
        {/if}
      </div>
    {/if}
    {#each workbench.tabs as tab (tab.id)}
      <!-- A terminal's slot must never scroll (.term-host): xterm scrolls its
           own scrollback, and a slot scrollbar is not just redundant — it is
           the fuel of a resize feedback loop. The bar appearing steals ~15px
           of width, the pane's ResizeObserver refits, the PTY resize makes
           ConPTY replay its whole screen, the replay nudges content height,
           the bar leaves, and around again — every lap smearing a copy of the
           screen into the pane. An engine boot under that loop painted
           hundreds of half-shifted duplicate lines while the log file it
           tailed stayed byte-for-byte clean (2026-08-12, twice in one
           morning). -->
      <div class="insp-slot" class:term-host={tab.kind === 'terminal'} style="display:{workbench.activeId === tab.id ? 'block' : 'none'}">
        {#if tab.kind === 'terminal'}
          <Terminal sessionId={tab.id} onExit={() => removeTab(tab.id)} />
        {:else if tab.kind === 'files'}
          <FilesPane />
        {:else if tab.kind === 'decks'}
          <DeckRoom />
        {:else if tab.kind === 'git'}
          <GitPane />
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
            {:else if tab.deck && tab.content !== undefined}
              <SlidesPane path={tab.path ?? ''} name={tab.name} content={tab.content} active={workbench.activeId === tab.id} />
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
