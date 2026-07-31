<script lang="ts">
  import { cockpit } from '../stores/cockpit.svelte'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import type { IconName } from '../icons'

  const testIcon: Record<string, IconName> = { running: 'loaderCircle', pass: 'check', fail: 'x' }
</script>

<div class="insp-scroll">
  <div class="panel">
    <div class="p-h"><span class="lbl"><span class="arw"><Icon name="chevronDown" size={12} /></span> <span class="eyebrow">{t('reviewPane.filesChanged', { count: cockpit.changedFiles.length })}</span></span></div>
    {#each cockpit.changedFiles as f}
      <div class="file-line"><span class="ic"><Icon name="fileText" size={13} /></span> {f.path} <span class="st">{f.status}</span></div>
    {/each}
  </div>

  <div class="panel">
    <div class="p-h">
      <span class="lbl"><span class="arw"><Icon name="chevronDown" size={12} /></span> <span class="eyebrow">{t('reviewPane.diff')}</span></span>
    </div>
    <div class="diff">
      <div class="fname">{cockpit.diff.file}</div>
      <div class="hunk">{cockpit.diff.hunk}</div>
      <pre>{#each cockpit.diff.lines as l}<div class="dl {l.kind}"><span class="ln">{l.ln}</span><span class="tx">{l.text}</span></div>{/each}</pre>
    </div>
  </div>

  <div class="panel">
    <div class="p-h">
      <span class="lbl"><span class="arw"><Icon name="chevronDown" size={12} /></span> <span class="eyebrow">{t('reviewPane.testResult')}</span></span>
      {#if cockpit.test.cases.some((c) => c.state === 'running')}<span class="spin"><Icon name="rotateCw" size={12} /></span>{/if}
    </div>
    <div class="p-b">
      {#if cockpit.test.cases.length === 0}
        <div class="empty">{t('reviewPane.noTestsRun')}</div>
      {:else}
        <div class="cmd">{cockpit.test.command}</div>
        {#each cockpit.test.cases as tc}
          <div class="trow"><span class="nm">{tc.name}</span><span class="rn {tc.state}"><Icon name={testIcon[tc.state] ?? 'circle'} size={12} /></span></div>
        {/each}
      {/if}
    </div>
  </div>

  <div class="panel">
    <div class="p-h"><span class="lbl"><span class="arw"><Icon name="chevronDown" size={12} /></span> <span class="eyebrow">{t('reviewPane.commandHistory', { count: cockpit.commandHistory.length })}</span></span></div>
    <div class="p-b">
      {#if cockpit.commandHistory.length === 0}
        <div class="empty">{t('reviewPane.noCommands')}</div>
      {:else}
        {#each cockpit.commandHistory as c}<div class="cmd">{c}</div>{/each}
      {/if}
    </div>
  </div>
</div>
