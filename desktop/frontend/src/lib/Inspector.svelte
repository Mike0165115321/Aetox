<script lang="ts">
  import type { ChangedFile, DiffView, TestRun, TaskState } from './types'

  let {
    changedFiles, diff, test, commandHistory, task,
  }: {
    changedFiles: ChangedFile[]
    diff: DiffView
    test: TestRun
    commandHistory: string[]
    task: TaskState
  } = $props()

  const testGlyph: Record<string, string> = { running: '··', pass: '✓', fail: '✕' }
  const hasActiveTask = $derived(task.steps.some((s) => s.status === 'active'))

  const tabs = ['Inspector', 'Logs', 'Audit']
  const tabIcons: Record<string, string> = { Inspector: '🔍', Logs: '📜', Audit: '🛡' }
  let activeTab = $state('Inspector')
  let menuOpen = $state(false)

  const placeholders: Record<string, string> = {
    Logs: 'ยังไม่มี log ในเซสชันนี้',
    Audit: 'ยังไม่มีรายการ audit',
  }

  function pickTab(tab: string) {
    activeTab = tab
    menuOpen = false
  }

  function closeMenuOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.plus-menu-wrap')) menuOpen = false
  }
</script>

<svelte:window onclick={menuOpen ? closeMenuOnOutsideClick : undefined} onkeydown={(e) => e.key === 'Escape' && (menuOpen = false)} />

<div class="insp-tabs">
  {#each tabs as tab}
    <button class="tab" class:active={activeTab === tab} onclick={() => (activeTab = tab)}>
      {tab}
    </button>
  {/each}
  <div class="plus-menu-wrap">
    <button class="icobtn tiny plus-btn" aria-label="Add panel" onclick={() => (menuOpen = !menuOpen)}>+</button>
    {#if menuOpen}
      <div class="plus-menu">
        {#each tabs as tab}
          <button class="plus-menu-item" onclick={() => pickTab(tab)}>
            <span class="ic">{tabIcons[tab]}</span> {tab}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>

{#if activeTab === 'Inspector'}
  <div class="insp-scroll">
    <div class="panel">
      <div class="p-h"><span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Files Changed ({changedFiles.length})</span></span></div>
      {#each changedFiles as f}
        <div class="file-line"><span class="ic">📄</span> {f.path} <span class="st">{f.status}</span></div>
      {/each}
    </div>

    <div class="panel">
      <div class="p-h">
        <span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Diff</span></span>
        <span><span class="icobtn tiny">⧉</span><span class="icobtn tiny">⤢</span></span>
      </div>
      <div class="diff">
        <div class="fname">{diff.file}</div>
        <div class="hunk">{diff.hunk}</div>
        <pre>{#each diff.lines as l}<div class="dl {l.kind}"><span class="ln">{l.ln}</span><span class="tx">{l.text}</span></div>{/each}</pre>
      </div>
    </div>

    <div class="panel">
      <div class="p-h">
        <span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Test Result</span></span>
        {#if test.cases.some((t) => t.state === 'running')}<span class="spin">↻</span>{/if}
      </div>
      <div class="p-b">
        {#if test.cases.length === 0}
          <div class="empty">ยังไม่มีการรันเทสต์</div>
        {:else}
          <div class="cmd">{test.command}</div>
          {#each test.cases as t}
            <div class="trow"><span class="nm">{t.name}</span><span class="rn {t.state}">{testGlyph[t.state]}</span></div>
          {/each}
        {/if}
      </div>
    </div>

    <div class="panel">
      <div class="p-h"><span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Command History ({commandHistory.length})</span></span></div>
      <div class="p-b">
        {#if commandHistory.length === 0}
          <div class="empty">No commands yet</div>
        {:else}
          {#each commandHistory as c}<div class="cmd">{c}</div>{/each}
        {/if}
      </div>
    </div>
  </div>
{:else}
  <div class="insp-scroll">
    <div class="empty tab-empty">{placeholders[activeTab]}</div>
  </div>
{/if}

{#if hasActiveTask}
  <div class="insp-foot">
    <button class="stopbtn">⏸ Stop Current Task</button>
  </div>
{/if}
