<script lang="ts">
  import { theme, toggleTheme } from './theme.svelte'
  import { t } from './i18n.svelte'

  let {
    inspectorCollapsed, onToggleInspector, sidebarCollapsed, onToggleSidebar,
  }: {
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
