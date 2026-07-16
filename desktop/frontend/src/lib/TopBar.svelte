<script lang="ts">
  import type { ProjectInfo, ModelStatus } from './types'
  import { theme, toggleTheme } from './theme.svelte'

  let { project, model }: { project: ProjectInfo; model: ModelStatus } = $props()
</script>

<div class="brand">
  <span class="glyph">🦅</span>
  <span class="word">AETOX</span>
</div>

<div class="topbar">
  <span class="chip"><span class="ic">📁</span> {project.name} <span class="caret">▾</span></span>
  <span class="chip branch"><span class="ic">⑂</span> {project.branch}</span>
  {#if project.extraBranches > 0}
    <span class="chip badge-count">+{project.extraBranches}</span>
  {/if}
  <span class="chip">
    <span class="ic">📄</span> {project.governanceFile}
    {#if project.governanceLoaded}<span class="dot green"></span> Loaded{/if}
  </span>
  <span class="spacer"></span>

  <div class="stat">
    <span class="k">Model</span>
    <span class="v">
      {model.provider}
      <span class="pill low">{model.thinkLevel}</span>
      <span class="pill jow">{model.speed}</span>
    </span>
  </div>
  <div class="stat">
    <span class="k">Context</span>
    <span class="v">{model.contextPct}% · <span class="muted">{model.contextUsed} / {model.contextMax}</span></span>
    <span class="meter"><i style="width:{model.contextPct}%"></i></span>
  </div>
  <div class="stat">
    <span class="k">Approval</span>
    <span class="v"><span class="dot green"></span> {model.approval} <span class="caret">▾</span></span>
  </div>

  <div class="winbtns">
    <button class="icobtn" aria-label="Toggle theme" onclick={toggleTheme}>
      {theme.name === 'dark' ? '☀' : '🌙'}
    </button>
    <span class="icobtn">⚙</span>
    <span class="icobtn">—</span>
    <span class="icobtn">▢</span>
    <span class="icobtn close">✕</span>
  </div>
</div>
