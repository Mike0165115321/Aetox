<script lang="ts">
  import TopBar from './lib/TopBar.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Chat from './lib/Chat.svelte'
  import FileEditor from './lib/FileEditor.svelte'
  import Settings from './lib/Settings.svelte'
  import Workbench from './lib/workbench/Workbench.svelte'
  import { onMount } from 'svelte'
  import {
    cockpit, sendUserMessage, loadRealState, openFolder, openFile,
    switchProvider, switchThinkLevel, switchApprovalMode,
    switchModel, submitAPIKey, setActiveView, closeFile, applyAgentStatus, applyToolEvent,
    attachImageFromPath,
  } from './lib/stores/cockpit.svelte'
  import { RelativizePath } from '../wailsjs/go/main/App'
  import { OnFileDrop, OnFileDropOff, EventsOn } from '../wailsjs/runtime/runtime'
  import { workbench } from './lib/stores/workbench.svelte'

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
  // The max is computed at drag time (see clampSize) rather than fixed here:
  // it's whatever's left of window.innerWidth after the OTHER side panel's
  // current width and .main's own 360px grid floor (2 handles at 6px each).
  // Dragging a panel wider than that would push the grid's total width past
  // the viewport, which .app's overflow-x:auto turns into a horizontal
  // scrollbar instead of an error — technically nothing breaks, but the
  // composer/chat content scrolls out of view, which reads as "the panel
  // grows without limit." Capping it here keeps everything on-screen without
  // reintroducing the old bug this was written to avoid (main getting
  // squeezed below its 360px floor) — main's grid floor is untouched, this
  // only stops the OTHER two columns from claiming space main needs.
  const mainFloor = 360
  const handleWidths = 12 // two 6px resize handles
  const panels = {
    sidebar: { cssVar: '--sidebar-width', storageKey: 'sidebarWidth', min: 200, defaultPx: 280 },
    inspector: { cssVar: '--inspector-width', storageKey: 'inspectorWidth', min: 320, defaultPx: 384 },
  }

  function currentPx(panel: typeof panels.sidebar): number {
    const raw = getComputedStyle(document.documentElement).getPropertyValue(panel.cssVar).trim()
    const parsed = parseFloat(raw)
    return Number.isFinite(parsed) ? parsed : panel.defaultPx
  }

  function clampSize(px: number, panel: typeof panels.sidebar, otherPanel: typeof panels.sidebar): number {
    const max = Math.max(panel.min, window.innerWidth - currentPx(otherPanel) - mainFloor - handleWidths)
    return Math.min(Math.max(panel.min, px), max)
  }

  function otherOf(panel: typeof panels.sidebar): typeof panels.sidebar {
    return panel === panels.sidebar ? panels.inspector : panels.sidebar
  }

  onMount(() => {
    loadRealState()

    const offAgentStatus = EventsOn('agent:status', applyAgentStatus)
    const offAgentTool = EventsOn('agent:tool', applyToolEvent)

    for (const panel of Object.values(panels)) {
      const stored = localStorage.getItem(panel.storageKey)
      if (stored) {
        const size = clampSize(parseInt(stored, 10), panel, otherOf(panel))
        document.documentElement.style.setProperty(panel.cssVar, `${size}px`)
      }
    }

    // Drop a file from Explorer anywhere on the window to open it as a tab,
    // same as clicking it in the sidebar tree — lets the user hand the AI a
    // file without hunting for it in the project tree first. An image
    // dropped specifically over the chat composer attaches to the message
    // instead — OnFileDrop gives window coordinates, so we route on those.
    const imageExtRe = /\.(png|jpe?g|gif|webp|bmp)$/i
    OnFileDrop(async (x, y, paths) => {
      const composerEl = document.querySelector('.composer')
      const overComposer = !!composerEl && withinRect(composerEl.getBoundingClientRect(), x, y)
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
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="app" class:inspector-collapsed={inspectorCollapsed} class:sidebar-collapsed={sidebarCollapsed}>
  <TopBar
    project={cockpit.project} onOpenFolder={openFolder}
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
            >✕</span>
          </button>
        {/each}
      </div>
    {/if}
    {#if cockpit.activeView === 'chat'}
      <Chat
        messages={cockpit.chat}
        task={cockpit.task}
        model={cockpit.model}
        project={cockpit.project}
        awaitingReply={cockpit.awaitingReply}
        agentStatus={cockpit.agentStatus}
        toolSteps={cockpit.toolSteps}
        onSend={sendUserMessage}
        onSwitchProvider={switchProvider}
        onSwitchThinkLevel={switchThinkLevel}
        onSwitchApprovalMode={switchApprovalMode}
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
    <Workbench onToggleInspector={toggleInspector} />
  </aside>
</div>

{#if cockpit.activeView === 'settings'}
  <div class="settings-overlay">
    <Settings onClose={() => setActiveView('chat')} />
  </div>
{/if}
