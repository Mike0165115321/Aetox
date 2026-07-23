<script lang="ts">
  import { t } from './i18n.svelte'
  import Wordmark from './Wordmark.svelte'

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
  <Wordmark height={20} />
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
  </div>
</div>
