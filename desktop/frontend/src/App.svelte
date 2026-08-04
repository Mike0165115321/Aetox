<script lang="ts">
  import TopBar from './lib/TopBar.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Chat from './lib/Chat.svelte'
  import FileEditor from './lib/FileEditor.svelte'
  import Settings from './lib/Settings.svelte'
  import Onboarding from './lib/Onboarding.svelte'
  import Workbench from './lib/workbench/Workbench.svelte'
  import { onMount } from 'svelte'
  import {
    cockpit, sendUserMessage, loadRealState, openFile,
    switchProvider, switchThinkLevel,
    switchModel, submitAPIKey, setActiveView, restoreActiveView, closeFile, applyAgentStatus, applyToolEvent,
    applyAgentChunk, applyReasoningChunk, attachImageFromPath,
    applyAskUser, applyAskDone, applyTodos, applyMissedInterjections, applyTaskChips,
    applyPendingLearned, refreshPendingLearned,
  } from './lib/stores/cockpit.svelte'
  import { RelativizePath, CloseAllBrowserTabs } from '../wailsjs/go/main/App'
  import { OnFileDrop, OnFileDropOff, EventsOn } from '../wailsjs/runtime/runtime'
  import { workbench, openPathsInWorkbench } from './lib/stores/workbench.svelte'
  import { clampPanelWidth } from './lib/panelSize'
  import Icon from './lib/Icon.svelte'

  function fileLabel(path: string): string {
    return path.split('/').pop() ?? path
  }

  function withinRect(r: DOMRect, x: number, y: number): boolean {
    return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom
  }

  // cockpit starts as emptyCockpitState(); loadRealState() fills project/model in
  // with what the Go engine actually has. tree/sessions/diff/test panels fill in
  // once a real Go-core data source is wired for them too.

  // Each floor below is the narrowest that panel's own content survives
  // without clipping (inspector's 320px fits the workbench tab row — see
  // workbench/Workbench.svelte's .insp-tabs).
  //
  // The max is computed at drag time rather than fixed here: it's whatever's
  // left of window.innerWidth after the OTHER side panel and .main's own 360px
  // grid floor. Dragging a panel wider than that would push the grid's total
  // width past the viewport, which .app's overflow-x:auto turns into a
  // horizontal scrollbar instead of an error — technically nothing breaks, but
  // the composer/chat content scrolls out of view. The arithmetic lives in
  // lib/panelSize.ts, where it can be tested; it used to be wrong here in a way
  // nothing on screen could explain (see that file).
  const panels = {
    sidebar: {
      cssVar: '--sidebar-width', storageKey: 'sidebarWidth', min: 200, defaultPx: 280,
      isCollapsed: () => sidebarCollapsed,
    },
    inspector: {
      cssVar: '--inspector-width', storageKey: 'inspectorWidth', min: 320, defaultPx: 384,
      isCollapsed: () => inspectorCollapsed,
    },
  }

  function currentPx(panel: typeof panels.sidebar): number {
    const raw = getComputedStyle(document.documentElement).getPropertyValue(panel.cssVar).trim()
    const parsed = parseFloat(raw)
    return Number.isFinite(parsed) ? parsed : panel.defaultPx
  }

  function clampSize(px: number, panel: typeof panels.sidebar, otherPanel: typeof panels.sidebar): number {
    // A collapsed panel is a 0px column and its handle is display:none, so the
    // space it would have taken belongs to whoever is being dragged.
    const otherVisible = !otherPanel.isCollapsed()
    return clampPanelWidth(px, {
      viewport: window.innerWidth,
      min: panel.min,
      otherWidth: otherVisible ? currentPx(otherPanel) : 0,
      otherVisible,
    })
  }

  function otherOf(panel: typeof panels.sidebar): typeof panels.sidebar {
    return panel === panels.sidebar ? panels.inspector : panels.sidebar
  }

  // Re-fit a panel to the space that exists right now. Uncollapsing the other
  // one takes its column back from whatever grew into it while it was away —
  // without this, reopening the sidebar next to a workbench that had claimed
  // the whole window is precisely the horizontal scrollbar the cap exists to
  // prevent. Only the CSS var moves; the remembered width in localStorage is
  // the user's preference and is left alone.
  function refit(panel: typeof panels.sidebar): void {
    if (panel.isCollapsed()) return
    const size = clampSize(currentPx(panel), panel, otherOf(panel))
    document.documentElement.style.setProperty(panel.cssVar, `${size}px`)
  }

  onMount(() => {
    restoreActiveView()
    loadRealState()
    // The Go backend outlives a webview reload (a `wails dev` Vite HMR full
    // reload, in particular) — this frontend just loaded, so it owns zero
    // workbench tabs by definition. Any native browser window the backend
    // still has is orphaned from a previous frontend lifetime; nothing would
    // ever reposition or close it otherwise (see CloseAllBrowserTabs).
    CloseAllBrowserTabs()

    const offAgentStatus = EventsOn('agent:status', applyAgentStatus)
    const offAgentTool = EventsOn('agent:tool', applyToolEvent)
    const offAgentChunk = EventsOn('agent:chunk', applyAgentChunk)
    const offAgentReasoning = EventsOn('agent:reasoning', applyReasoningChunk)
    const offAskUser = EventsOn('ask:user', applyAskUser)
    const offAskDone = EventsOn('ask:done', applyAskDone)
    const offTodos = EventsOn('todo:update', applyTodos)
    // A message typed in the moment the turn was already returning: the engine
    // could not fold it in, so it comes back here and goes out as its own turn.
    const offMissed = EventsOn('agent:interjection-missed', applyMissedInterjections)
    const offTaskChips = EventsOn('tasks:changed', applyTaskChips)
    // What the agent wants to remember and cannot until it is allowed to. Also
    // fetched once here, because anything left undecided in a previous session
    // is still undecided and nothing would emit for it.
    const offLearning = EventsOn('learning:changed', applyPendingLearned)
    void refreshPendingLearned()

    for (const panel of Object.values(panels)) {
      const stored = localStorage.getItem(panel.storageKey)
      if (stored) {
        const size = clampSize(parseInt(stored, 10), panel, otherOf(panel))
        document.documentElement.style.setProperty(panel.cssVar, `${size}px`)
      }
    }

    // Drop a file from Explorer anywhere on the window to open it as a tab,
    // same as clicking it in the sidebar tree — lets the user hand the AI a
    // file without hunting for it in the project tree first. Where it lands
    // decides what "open" means, and OnFileDrop gives window coordinates, so
    // we route on those: an image over the chat composer attaches to the
    // message, anything over the workbench opens on the agent's desk, and the
    // rest opens in the main editor.
    const imageExtRe = /\.(png|jpe?g|gif|webp|bmp)$/i
    OnFileDrop(async (x, y, paths) => {
      const composerEl = document.querySelector('.composer')
      const overComposer = !!composerEl && withinRect(composerEl.getBoundingClientRect(), x, y)
      const deskEl = document.querySelector('.inspector')
      // A collapsed panel is display:none, so its rect is empty and nothing
      // can land in it — no separate check needed.
      const overDesk = !!deskEl && withinRect(deskEl.getBoundingClientRect(), x, y)
      if (overDesk) {
        await openPathsInWorkbench(paths)
        return
      }
      for (const path of paths) {
        if (overComposer && imageExtRe.test(path)) {
          await attachImageFromPath(path)
          continue
        }
        try {
          const relPath = await RelativizePath(path)
          await openFile(relPath)
        } catch {
          // Outside the open project, or unreadable — silently skip it.
        }
      }
    }, false)
    return () => {
      OnFileDropOff()
      offAgentStatus()
      offAgentTool()
      offAgentChunk()
      offAgentReasoning()
      offAskUser()
      offAskDone()
      offTodos()
      offMissed()
      offTaskChips()
      offLearning()
    }
  })

  let draggingSidebar = $state(false)
  let draggingInspector = $state(false)
  let inspectorCollapsed = $state(localStorage.getItem('inspectorCollapsed') === 'true')
  let sidebarCollapsed = $state(localStorage.getItem('sidebarCollapsed') === 'true')

  // Closing the last workbench tab should reclaim the inspector panel's
  // width, not leave it reserved and blank — opening a tab should bring it back.
  $effect(() => {
    inspectorCollapsed = workbench.tabs.length === 0
  })

  // Whenever a panel comes back, the OTHER one gives up whatever it took while
  // that panel was away. Which one is which is the whole point: refitting both
  // in a fixed order clamped the returning panel against the width the other
  // had grown into, so reopening the sidebar next to a workbench that had
  // claimed the window pinned the sidebar to its 200px floor and left the
  // workbench holding the difference — the opposite of what this is for.
  //
  // Only an un-collapse needs it. Collapsing frees space; nothing has to move.
  // svelte-ignore state_referenced_locally — the initial value is the point:
  // these hold what the flags were on the previous run, so the effect can tell
  // an un-collapse from a collapse.
  let wasSidebarCollapsed = sidebarCollapsed
  // svelte-ignore state_referenced_locally
  let wasInspectorCollapsed = inspectorCollapsed
  $effect(() => {
    const sc = sidebarCollapsed
    const ic = inspectorCollapsed
    if (wasSidebarCollapsed && !sc) refit(panels.inspector)
    if (wasInspectorCollapsed && !ic) refit(panels.sidebar)
    wasSidebarCollapsed = sc
    wasInspectorCollapsed = ic
  })

  function toggleSidebar() {
    sidebarCollapsed = !sidebarCollapsed
    localStorage.setItem('sidebarCollapsed', String(sidebarCollapsed))
  }

  function toggleInspector() {
    inspectorCollapsed = !inspectorCollapsed
    localStorage.setItem('inspectorCollapsed', String(inspectorCollapsed))
  }

  // computeSize turns the pointer position into this panel's size — sidebar
  // anchored to the window's left edge, inspector to its right.
  //
  // Dragging past the inspector panel's bounds crosses into the native
  // WebView2 browser tab window (a real, separate OS window overlaid by
  // desktop/browser.go — see BrowserSetBounds) rather than staying inside
  // this webview's DOM. Without pointer capture, the OS can deliver the
  // pointerup that ends the drag to THAT window instead of here, so this
  // listener never fires: dragging never stops, and any later mouse movement
  // over the app keeps calling onMove and growing the panel. setPointerCapture
  // makes Chromium keep routing this pointer's events to the handle element
  // regardless of what's visually underneath it, which is the actual fix;
  // pointercancel/blur are just backstops in case capture is lost anyway.
  function startResize(panel: typeof panels.sidebar, computeSize: (e: PointerEvent) => number, setDragging: (v: boolean) => void) {
    const otherPanel = otherOf(panel)
    return (e: PointerEvent) => {
      const handle = e.currentTarget as HTMLElement
      handle.setPointerCapture(e.pointerId)
      setDragging(true)
      e.preventDefault()
      const onMove = (ev: PointerEvent) => {
        const size = clampSize(computeSize(ev), panel, otherPanel)
        document.documentElement.style.setProperty(panel.cssVar, `${size}px`)
      }
      const onEnd = () => {
        setDragging(false)
        try { handle.releasePointerCapture(e.pointerId) } catch { /* already released */ }
        window.removeEventListener('pointermove', onMove)
        window.removeEventListener('pointerup', onEnd)
        window.removeEventListener('pointercancel', onEnd)
        window.removeEventListener('blur', onEnd)
        const size = getComputedStyle(document.documentElement).getPropertyValue(panel.cssVar)
        if (size) localStorage.setItem(panel.storageKey, size.trim())
      }
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onEnd)
      window.addEventListener('pointercancel', onEnd)
      window.addEventListener('blur', onEnd)
    }
  }

  const startSidebarResize = startResize(panels.sidebar, (e) => e.clientX, (v) => (draggingSidebar = v))
  const startInspectorResize = startResize(panels.inspector, (e) => window.innerWidth - e.clientX, (v) => (draggingInspector = v))

  function onKeydown(e: KeyboardEvent) {
    if (e.ctrlKey && e.altKey && e.key.toLowerCase() === 'b') {
      e.preventDefault()
      toggleInspector()
    } else if (e.ctrlKey && e.altKey && e.key.toLowerCase() === 's') {
      e.preventDefault()
      toggleSidebar()
    } else if (e.ctrlKey && !e.altKey && e.key === ',') {
      e.preventDefault()
      setActiveView('settings')
    } else if (e.key === 'Escape' && cockpit.activeView === 'settings') {
      // Ctrl+, opened it; Escape is the other half nobody had. Anything layered
      // over Settings — the confirm dialog, the command palette — stops the key
      // before it reaches window, so this only ever fires on a bare page.
      setActiveView('chat')
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="app" class:inspector-collapsed={inspectorCollapsed} class:sidebar-collapsed={sidebarCollapsed}>
  <TopBar
    inspectorCollapsed={inspectorCollapsed} onToggleInspector={toggleInspector}
    sidebarCollapsed={sidebarCollapsed} onToggleSidebar={toggleSidebar}
  />
  <Sidebar onOpenSettings={() => setActiveView('settings')} />
  <div
    class="resize-handle handle-l" class:dragging={draggingSidebar}
    role="separator" aria-orientation="vertical" aria-label="Resize project panel"
    onpointerdown={startSidebarResize}
  ></div>
  <main class="main">
    {#if cockpit.openFiles.length > 0}
      <div class="tabs">
        <button class="tab" class:active={cockpit.activeView === 'chat'} onclick={() => setActiveView('chat')}>Chat</button>
        {#each cockpit.openFiles as f}
          <button class="tab" class:active={cockpit.activeView === f.path} title={fileLabel(f.path)} onclick={() => setActiveView(f.path)}>
            <span class="label">{fileLabel(f.path)}</span>
            <span
              class="tab-close" role="button" tabindex="0" aria-label={`Close ${fileLabel(f.path)}`}
              onclick={(e) => { e.stopPropagation(); closeFile(f.path) }}
              onkeydown={(e) => e.key === 'Enter' && closeFile(f.path)}
            ><Icon name="x" size={12} /></span>
          </button>
        {/each}
      </div>
    {/if}
    {#if cockpit.activeView === 'chat'}
      <Chat
        messages={cockpit.chat}
        task={cockpit.task}
        model={cockpit.model}
        awaitingReply={cockpit.awaitingReply}
        agentStatus={cockpit.agentStatus}
        toolSteps={cockpit.toolSteps}
        streamingText={cockpit.streamingText}
        reasoningText={cockpit.reasoningText}
        onSend={sendUserMessage}
        onSwitchProvider={switchProvider}
        onSwitchThinkLevel={switchThinkLevel}
        onSwitchModel={switchModel}
        onSubmitAPIKey={submitAPIKey}
      />
    {:else}
      {#each cockpit.openFiles as f (f.path)}
        {#if cockpit.activeView === f.path}
          <FileEditor path={f.path} content={f.content} />
        {/if}
      {/each}
    {/if}
  </main>
  <div
    class="resize-handle handle-r" class:dragging={draggingInspector}
    role="separator" aria-orientation="vertical" aria-label="Resize inspector panel"
    onpointerdown={startInspectorResize}
  ></div>
  <aside class="inspector">
    <Workbench />
  </aside>
</div>

{#if cockpit.activeView === 'settings'}
  <div class="settings-overlay">
    <Settings onClose={() => setActiveView('chat')} />
  </div>
{/if}

<Onboarding />
