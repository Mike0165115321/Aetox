<script lang="ts">
  import type { ProjectInfo } from './types'
  import { theme, toggleTheme } from './theme.svelte'
  import { t } from './i18n.svelte'

  let {
    project, onOpenFolder, inspectorCollapsed, onToggleInspector, sidebarCollapsed, onToggleSidebar,
  }: {
    project: ProjectInfo
    onOpenFolder: () => void
    inspectorCollapsed: boolean
    onToggleInspector: () => void
    sidebarCollapsed: boolean
    onToggleSidebar: () => void
  } = $props()
</script>

<div class="brand">
  <span class="word">AETOX</span>
</div>

<div class="topbar">
  <button
    class="icobtn tip-l" aria-label={sidebarCollapsed ? t('topbar.showSidebar') : t('topbar.hideSidebar')}
    data-tip={t('topbar.toggleSidebarTip')} onclick={onToggleSidebar}
  >
    {sidebarCollapsed ? '▥' : '▤'}
  </button>
  <button type="button" class="chip" onclick={onOpenFolder}>
    <span class="ic">📁</span> {project.name || t('topbar.openFolder')} <span class="caret">▾</span>
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
      {#if project.governanceLoaded}<span class="dot green"></span> {t('topbar.loaded')}{/if}
    </span>
  {/if}
  <span class="spacer"></span>

  <div class="winbtns">
    <button
      class="icobtn" aria-label={inspectorCollapsed ? t('topbar.showPanel') : t('topbar.hidePanel')}
      data-tip={t('topbar.toggleInspectorTip')} onclick={onToggleInspector}
    >
      {inspectorCollapsed ? '▥' : '▤'}
    </button>
    <button class="icobtn tip-r" aria-label={t('topbar.toggleTheme')} data-tip={t('topbar.toggleThemeTip')} onclick={toggleTheme}>
      {theme.name === 'dark' ? '☀' : '🌙'}
    </button>
  </div>
</div>
