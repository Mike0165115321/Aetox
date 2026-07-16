<script lang="ts">
  import type { ChangedFile, DiffView, TestRun } from './types'

  let {
    changedFiles, diff, test, commandHistory,
  }: {
    changedFiles: ChangedFile[]
    diff: DiffView
    test: TestRun
    commandHistory: string[]
  } = $props()

  const tabs = ['Inspector', 'Terminal', 'Logs', 'Audit']
  let activeTab = $state('Inspector')

  const placeholders: Record<string, string> = {
    Terminal: 'ยังไม่มี output — เทอร์มินัลจะสตรีมจาก Go core',
    Logs: 'ยังไม่มี log ในเซสชันนี้',
    Audit: 'ยังไม่มีรายการ audit',
  }
</script>

<div class="insp-tabs">
  {#each tabs as tab}
    <button class="tab" class:active={activeTab === tab} onclick={() => (activeTab = tab)}>
      {tab}
    </button>
  {/each}
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
      <div class="p-h"><span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Test Result</span></span><span class="spin">↻</span></div>
      <div class="p-b">
        <div class="cmd">{test.command}</div>
        <div class="running-label">Running…</div>
        {#each test.cases as t}
          <div class="trow"><span class="nm">{t.name}</span><span class="rn">··</span></div>
        {/each}
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

<div class="insp-foot">
  <button class="stopbtn">⏸ Stop Current Task</button>
</div>
