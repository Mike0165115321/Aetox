<script lang="ts">
  import { t } from './i18n.svelte'
  import { newSession } from './stores/cockpit.svelte'
  import Wordmark from './Wordmark.svelte'
  import Icon from './Icon.svelte'

  let {
    inspectorCollapsed, onToggleInspector, sidebarCollapsed, onToggleSidebar,
  }: {
    inspectorCollapsed: boolean
    onToggleInspector: () => void
    sidebarCollapsed: boolean
    onToggleSidebar: () => void
  } = $props()
</script>

<!-- The window frame with the panel's own column drawn inside it: filled while
     that panel is showing, empty once it's collapsed. Reads as "which side,
     and is it there" at a glance, which two near-identical block glyphs did
     not. The right-hand button mirrors the same icon. -->
{#snippet panelIcon(showing: boolean, mirrored: boolean)}
  <svg
    viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"
    fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"
    style={mirrored ? 'transform:scaleX(-1)' : ''}
  >
    <rect x="1.65" y="2.9" width="12.7" height="10.2" rx="2.2" />
    <path d="M6.1 3.4V12.6" />
    {#if showing}
      <rect x="2.4" y="3.65" width="3" height="8.7" rx="1.1" fill="currentColor" stroke="none" opacity=".55" />
    {/if}
  </svg>
{/snippet}

<div class="brand">
  <Wordmark height={20} />
</div>

<div class="topbar">
  <button
    class="icobtn tip-l" aria-label={sidebarCollapsed ? t('topbar.showSidebar') : t('topbar.hideSidebar')}
    data-tip={t('topbar.toggleSidebarTip')} onclick={onToggleSidebar}
  >
    {@render panelIcon(!sidebarCollapsed, false)}
  </button>
  <span class="spacer"></span>

  <!-- tip-r on both: these sit flush against the window's right edge, so a
       centred (or left-anchored) tooltip gets clipped by it. -->
  <div class="winbtns">
    <button
      class="icobtn tip-r" aria-label={t('sidebar.newSession')}
      data-tip="{t('sidebar.newSession')} · Ctrl+N" onclick={newSession}
    ><Icon name="plus" size={15} /></button>
    <button
      class="icobtn tip-r" aria-label={inspectorCollapsed ? t('topbar.showPanel') : t('topbar.hidePanel')}
      data-tip={t('topbar.toggleInspectorTip')} onclick={onToggleInspector}
    >
      {@render panelIcon(!inspectorCollapsed, true)}
    </button>
  </div>
</div>
