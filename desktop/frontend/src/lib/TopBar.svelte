<script lang="ts">
  import type { ProjectInfo } from './types'
  import { theme, toggleTheme } from './theme.svelte'

  let {
    project, onOpenFolder,
  }: {
    project: ProjectInfo
    onOpenFolder: () => void
  } = $props()
</script>

<div class="brand">
  <span class="glyph">🦅</span>
  <span class="word">AETOX</span>
</div>

<div class="topbar">
  <button type="button" class="chip" onclick={onOpenFolder}>
    <span class="ic">📁</span> {project.name || 'Open Folder'} <span class="caret">▾</span>
  </button>
  {#if project.branch}
    <span class="chip branch"><span class="ic">⑂</span> {project.branch}</span>
  {/if}
  {#if project.extraBranches > 0}
    <span class="chip badge-count">+{project.extraBranches}</span>
  {/if}
  {#if project.name}
    <span class="chip">
      <span class="ic">📄</span> {project.governanceFile}
      {#if project.governanceLoaded}<span class="dot green"></span> Loaded{/if}
    </span>
  {/if}
  <span class="spacer"></span>

  <div class="winbtns">
    <button class="icobtn" aria-label="Toggle theme" onclick={toggleTheme}>
      {theme.name === 'dark' ? '☀' : '🌙'}
    </button>
  </div>
</div>
