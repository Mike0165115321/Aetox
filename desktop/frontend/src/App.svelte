<script lang="ts">
  import TopBar from './lib/TopBar.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Chat from './lib/Chat.svelte'
  import FileEditor from './lib/FileEditor.svelte'
  import Settings from './lib/Settings.svelte'
  import Workbench from './lib/workbench/Workbench.svelte'
  import { onMount } from 'svelte'
  import {
    cockpit, sendUserMessage, loadRealState, openFolder,
    switchProvider, switchThinkLevel, switchApprovalMode,
    switchModel, submitAPIKey, setActiveView, closeFile,
  } from './lib/stores/cockpit.svelte'

  function fileLabel(path: string): string {
    return path.split('/').pop() ?? path
  }

  // cockpit starts as emptyCockpitState(); loadRealState() fills project/model in
  // with what the Go engine actually has. tree/sessions/diff/test panels fill in
  // once a real Go-core data source is wired for them too.

  // Panel widths are otherwise unbounded — the .main chat column keeps its own
  // minmax(360px,1fr) floor and .app scrolls horizontally, so neither side panel
  // can squeeze it into the overlap bug from before. Each floor below is just
  // the narrowest that panel's own content survives without clipping.
  const panels = {
    sidebar: { cssVar: '--sidebar-width', storageKey: 'sidebarWidth', min: 200 },
    // wide enough for the workbench's tab row — see workbench/Workbench.svelte's .insp-tabs.
    inspector: { cssVar: '--inspector-width', storageKey: 'inspectorWidth', min: 320 },
  }

  function clampSize(px: number, min: number): number {
    return Math.max(min, px)
  }

  onMount(() => {
    loadRealState()

    for (const { cssVar, storageKey, min } of Object.values(panels)) {
      const stored = localStorage.getItem(storageKey)
      if (stored) {
        const size = clampSize(parseInt(stored, 10), min)
        document.documentElement.style.setProperty(cssVar, `${size}px`)
      }
    }
  })

  let draggingSidebar = $state(false)
  let draggingInspector = $state(false)

  // computeSize turns the pointer position into this panel's size — sidebar
  // anchored to the window's left edge, inspector to its right.
  function startResize(panel: typeof panels.sidebar, computeSize: (e: PointerEvent) => number, setDragging: (v: boolean) => void) {
    return (e: PointerEvent) => {
      setDragging(true)
      e.preventDefault()
      const onMove = (ev: PointerEvent) => {
        const size = clampSize(computeSize(ev), panel.min)
        document.documentElement.style.setProperty(panel.cssVar, `${size}px`)
      }
      const onUp = () => {
        setDragging(false)
        window.removeEventListener('pointermove', onMove)
        window.removeEventListener('pointerup', onUp)
        const size = getComputedStyle(document.documentElement).getPropertyValue(panel.cssVar)
        if (size) localStorage.setItem(panel.storageKey, size.trim())
      }
      window.addEventListener('pointermove', onMove)
      window.addEventListener('pointerup', onUp)
    }
  }

  const startSidebarResize = startResize(panels.sidebar, (e) => e.clientX, (v) => (draggingSidebar = v))
  const startInspectorResize = startResize(panels.inspector, (e) => window.innerWidth - e.clientX, (v) => (draggingInspector = v))
</script>

<div class="app">
  <TopBar
    project={cockpit.project} onOpenFolder={openFolder} onOpenSettings={() => setActiveView('settings')}
  />
  <Sidebar />
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
    <Workbench />
  </aside>
</div>

{#if cockpit.activeView === 'settings'}
  <div class="settings-overlay">
    <Settings onClose={() => setActiveView('chat')} />
  </div>
{/if}
