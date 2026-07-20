<script lang="ts">
  import { cockpit } from '../stores/cockpit.svelte'

  const testGlyph: Record<string, string> = { running: '··', pass: '✓', fail: '✕' }
</script>

<div class="insp-scroll">
  <div class="panel">
    <div class="p-h"><span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Files Changed ({cockpit.changedFiles.length})</span></span></div>
    {#each cockpit.changedFiles as f}
      <div class="file-line"><span class="ic">📄</span> {f.path} <span class="st">{f.status}</span></div>
    {/each}
  </div>

  <div class="panel">
    <div class="p-h">
      <span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Diff</span></span>
    </div>
    <div class="diff">
      <div class="fname">{cockpit.diff.file}</div>
      <div class="hunk">{cockpit.diff.hunk}</div>
      <pre>{#each cockpit.diff.lines as l}<div class="dl {l.kind}"><span class="ln">{l.ln}</span><span class="tx">{l.text}</span></div>{/each}</pre>
    </div>
  </div>

  <div class="panel">
    <div class="p-h">
      <span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Test Result</span></span>
      {#if cockpit.test.cases.some((t) => t.state === 'running')}<span class="spin">↻</span>{/if}
    </div>
    <div class="p-b">
      {#if cockpit.test.cases.length === 0}
        <div class="empty">ยังไม่มีการรันเทสต์</div>
      {:else}
        <div class="cmd">{cockpit.test.command}</div>
        {#each cockpit.test.cases as tc}
          <div class="trow"><span class="nm">{tc.name}</span><span class="rn {tc.state}">{testGlyph[tc.state]}</span></div>
        {/each}
      {/if}
    </div>
  </div>

  <div class="panel">
    <div class="p-h"><span class="lbl"><span class="arw">▾</span> <span class="eyebrow">Command History ({cockpit.commandHistory.length})</span></span></div>
    <div class="p-b">
      {#if cockpit.commandHistory.length === 0}
        <div class="empty">No commands yet</div>
      {:else}
        {#each cockpit.commandHistory as c}<div class="cmd">{c}</div>{/each}
      {/if}
    </div>
  </div>
</div>
