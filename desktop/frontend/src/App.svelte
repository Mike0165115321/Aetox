<script lang="ts">
  import TopBar from './lib/TopBar.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Chat from './lib/Chat.svelte'
  import FileEditor from './lib/FileEditor.svelte'
  import Settings from './lib/Settings.svelte'
  import Office from './lib/Office.svelte'
  import Workroom from './lib/Workroom.svelte'
  import Artifacts from './lib/Artifacts.svelte'
  import Projects from './lib/Projects.svelte'
  import Onboarding from './lib/Onboarding.svelte'
  import Updater from './lib/Updater.svelte'
  import CapabilityProgress from './lib/CapabilityProgress.svelte'
  import { listenCapabilities } from './lib/capabilities.svelte'
  import Workbench from './lib/workbench/Workbench.svelte'
  import { onMount } from 'svelte'
  import {
    cockpit, sendUserMessage, loadRealState, openFile,
    switchProvider, switchThinkLevel,
    switchModel, submitAPIKey, setActiveView, restoreActiveView, closeFile, applyAgentStatus, applyToolEvent,
    applyAgentChunk, applyReasoningChunk, attachImageFromPath,
    applyAskUser, applyAskDone, applyTodos, applyMissedInterjections, applyTaskChips, applyUsageRound,
    applyPendingLearned, refreshPendingLearned, refreshPendingIssues, applyAgentDone, isOverlayView, closeOverlay,
    refreshProjectFolders, refreshOpenFiles,
  } from './lib/stores/cockpit.svelte'
  import { shell, shellHasChats } from './lib/shell.svelte'
  import { applyBusyEvent, clearBusyWork } from './lib/stores/busySignal.svelte'
  import { RelativizePath, CloseAllBrowserTabs } from '../wailsjs/go/main/App'
  import { OnFileDrop, OnFileDropOff, EventsOn } from '../wailsjs/runtime/runtime'
  import { workbench, openPathsInWorkbench, filesChangedOnDisk } from './lib/stores/workbench.svelte'
  import { listenForUpdates } from './lib/selfUpdate.svelte'
  import { clampPanelWidth, fitPanelsToWindow } from './lib/panelSize'
  import { isShortcut } from './lib/shortcuts'
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

  let appEl = $state<HTMLDivElement | null>(null)

  /** How wide the grid actually is, in the units the grid is laid out in.
   *
   *  NOT window.innerWidth, and the difference is a real 44px on this machine.
   *  The UI zoom control writes `zoom` to <body> (systemFont.svelte.ts), so the
   *  shell's box measures innerWidth ÷ zoom CSS pixels while innerWidth itself
   *  does not change at all. Every width this file hands the grid is a CSS
   *  pixel, so reserving space against innerWidth reserves space that is not
   *  there: at zoom 1.03 the panels were allowed to be 44px too wide and the
   *  grid hung that far off the right edge of the window, taking the workbench
   *  toolbar's last button with it (owner, 20 ส.ค., three rounds of looking in
   *  the wrong place).
   *
   *  Reported by the element rather than computed from the zoom factor, because
   *  the element is the thing being fitted and it can answer for itself —
   *  scrollbars, borders and any future frame around it included.
   */
  function gridWidth(): number {
    return appEl?.clientWidth || window.innerWidth
  }

  function clampSize(px: number, panel: typeof panels.sidebar, otherPanel: typeof panels.sidebar): number {
    // A collapsed panel is a 0px column and its handle is display:none, so the
    // space it would have taken belongs to whoever is being dragged.
    const otherVisible = !otherPanel.isCollapsed()
    return clampPanelWidth(px, {
      viewport: gridWidth(),
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

  // The window is the third thing that changes this arithmetic, and the only
  // one nothing was watching: the cap ran on a drag and on load, never on a
  // resize. So a workbench dragged wide on a maximised window stayed that wide
  // when the window came back down, and the grid was wider than the frame from
  // then on with nothing on screen to say so — the room a deck's scrollIntoView
  // then found to shove the whole shell sideways (SlidesPane.center,
  // .app in style.css).
  //
  // Both panels through one function rather than refit() twice: refitting in a
  // fixed order pins whichever is asked second against the width the first is
  // still holding. See fitPanelsToWindow.
  // Watched on the SHELL, not on the window. The window fires `resize` when the
  // window changes; it says nothing when the UI zoom control changes how many
  // CSS pixels that window is worth (systemFont.svelte.ts), and that is exactly
  // the moment the widths stop fitting. The element that has to fit is the one
  // that can be asked.
  //
  // The window listener stays as well: it costs nothing and it is the one that
  // fires first when a window is dragged between monitors of different scaling.
  $effect(() => {
    if (!appEl || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => refitToWindow())
    ro.observe(appEl)
    return () => ro.disconnect()
  })

  function refitToWindow(): void {
    const list = Object.values(panels)
    const state = list.map((p) => ({ width: currentPx(p), min: p.min, visible: !p.isCollapsed() }))
    fitPanelsToWindow(gridWidth(), state).forEach((px, i) => {
      if (!state[i].visible || px === state[i].width) return
      document.documentElement.style.setProperty(list[i].cssVar, `${px}px`)
    })
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
    // The same stream, read for a different question. applyToolEvent builds
    // the chat's timeline; this asks only whether the browser is working and
    // on which tab, which is what ไฟบอกสถานะ draws (§174). A second
    // listener rather than a call inside the first, so a workbench concern
    // stays out of the store that draws messages.
    const offBusyTool = EventsOn('agent:tool', applyBusyEvent)
    const offAgentChunk = EventsOn('agent:chunk', applyAgentChunk)
    // The ending for a turn this window has no promise for — a webview reload
    // killed the SendMessage promise, the engine kept working, and this event
    // is how the finished answer still reaches the screen.
    const offAgentDone = EventsOn('agent:done', applyAgentDone)
    // The far end of the busy signal. A browser call normally puts its own
    // light out when its result arrives, but a turn that was stopped, or that
    // died with the provider, owes nobody a result — and the panel would keep
    // glowing for a page nothing is looking at.
    const offBusyDone = EventsOn('agent:done', clearBusyWork)
    const offAgentReasoning = EventsOn('agent:reasoning', applyReasoningChunk)
    const offAskUser = EventsOn('ask:user', applyAskUser)
    const offAskDone = EventsOn('ask:done', applyAskDone)
    const offTodos = EventsOn('todo:update', applyTodos)
    // A message typed in the moment the turn was already returning: the engine
    // could not fold it in, so it comes back here and goes out as its own turn.
    const offMissed = EventsOn('agent:interjection-missed', applyMissedInterjections)
    const offTaskChips = EventsOn('tasks:changed', applyTaskChips)
    // What the turn is costing, round by round, from the same reporter that
    // has always written it to the usage table (desktop/usage.go). The table
    // answered the question a day later; this answers it while there is still
    // something to do about it.
    const offUsage = EventsOn('usage:round', applyUsageRound)
    // A folder the user let in from the card the agent raised mid-turn. The
    // panel is the one place that says what this session can reach, so it has
    // to learn about a folder that arrived without anybody opening it.
    const offWorkspace = EventsOn('workspace:changed', () => { void refreshProjectFolders() })
    // The agent just wrote a file. Both surfaces that draw one re-read it, so
    // somebody reading a document while a turn edits it sees the edit instead
    // of the version it replaced (owner, 24 ส.ค.).
    const offFiles = EventsOn('workbench:files-changed', (ev: { data?: string[] }) => {
      const paths = ev?.data ?? []
      void filesChangedOnDisk(paths)
      void refreshOpenFiles(paths)
    })
    const offCapabilities = listenCapabilities()
    // What the agent wants to remember and cannot until it is allowed to, and
    // what keeps failing and might be worth telling the developer about.
    // One event, two queues (docs/architecture/system-problems-vs-learning-2026-08-18.md):
    // the payload is the lessons count, and the problems room asks for its own
    // when the signal arrives. Both fetched once here too, because anything
    // left undecided in a previous session is still undecided.
    const offLearning = EventsOn('learning:changed', (count: number) => {
      applyPendingLearned(count)
      void refreshPendingIssues()
    })
    void refreshPendingLearned()
    void refreshPendingIssues()
    // "A newer Aetox exists." Wired here rather than in Settings because the
    // window is where the user is — the check runs on its own now
    // (update_notify.go), and an answer that only lands on a page nobody has
    // open is the same as no answer.
    const offUpdate = listenForUpdates()

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
      offBusyTool()
      offAgentChunk()
      offAgentDone()
      offBusyDone()
      offAgentReasoning()
      offAskUser()
      offAskDone()
      offTodos()
      offMissed()
      offTaskChips()
      offUsage()
      offWorkspace()
      offFiles()
      offCapabilities()
      offLearning()
      offUpdate()
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
    if (isShortcut(e, 'toggleInspector')) {
      e.preventDefault()
      toggleInspector()
    } else if (isShortcut(e, 'toggleSidebar')) {
      e.preventDefault()
      toggleSidebar()
    } else if (isShortcut(e, 'settings')) {
      e.preventDefault()
      setActiveView('settings')
    // isOverlayView, not a list spelled out here: the same set decides which
    // rooms Escape closes and when the native browser window has to hide behind
    // one (BrowserPane). Two copies drift the day a room is added, and the copy
    // that forgets is the one whose failure lands somewhere else entirely.
    } else if (e.key === 'Escape' && isOverlayView(cockpit.activeView)) {
      // Ctrl+, opened it; Escape is the other half nobody had. Anything layered
      // over Settings — the confirm dialog, the command palette — stops the key
      // before it reaches window, so this only ever fires on a bare page. The
      // office and the gallery close the same way, because a page you can only
      // leave with the mouse is one people get stuck on.
      //
      // closeOverlay, not setActiveView('chat'): behind ทีม there is no chat to
      // land on, and this used to hand that door another door's conversation.
      closeOverlay()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} onresize={refitToWindow} />

<!-- `resizing` turns the column transition off for the length of a drag. The
     panels ease open and shut on a click, but while the pointer is holding an
     edge the pointer IS the animation, and easing toward the cursor turns a
     direct manipulation into a control that lags behind the hand. -->
<div
  class="app"
  bind:this={appEl}
  class:inspector-collapsed={inspectorCollapsed}
  class:sidebar-collapsed={sidebarCollapsed}
  class:resizing={draggingSidebar || draggingInspector}
>
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
    <!-- The tab strip switches between a conversation and the files opened
         from it, so it belongs to a door that holds conversations. Behind ทีม
         neither destination exists, and a run of coding work left the strip
         standing over ห้องทำงาน with a "Chat" tab that led to another door's
         session. -->
    {#if cockpit.openFiles.length > 0 && shellHasChats(shell.name)}
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
    {#if cockpit.activeView === 'lines'}
      <!-- The ทีม door's home, drawn IN the layout rather than over it. Every
           other page here is somewhere you visit from a conversation and
           return to; this one is where you stand when that door is open, so an
           overlay would have covered the only two controls that lead anywhere
           — the topbar's door menu and the sidebar's rooms row, which §158.9
           says is the door itself. -->
      <Workroom />
    {:else if cockpit.activeView === 'chat'}
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

<!-- The rooms that are not a chat. Each one is a full-window view over the
     app, which is the seam Settings has always used — the layout underneath is
     untouched, and closing one puts you back exactly where you were. -->
{#if cockpit.activeView === 'settings'}
  <div class="settings-overlay">
    <Settings onClose={closeOverlay} />
  </div>
{:else if cockpit.activeView === 'office'}
  <div class="settings-overlay">
    <Office onClose={closeOverlay} />
  </div>
{:else if cockpit.activeView === 'artifacts'}
  <div class="settings-overlay">
    <Artifacts onClose={closeOverlay} />
  </div>
{:else if cockpit.activeView === 'projects'}
  <div class="settings-overlay">
    <Projects onClose={closeOverlay} />
  </div>
{/if}

<Onboarding />
<!-- Outside every view, because the offer belongs to the app and not to a page:
     whichever room the user is standing in, that is where the notice has to
     find them. Renders nothing until there is something to say. -->
<Updater />
<CapabilityProgress />
