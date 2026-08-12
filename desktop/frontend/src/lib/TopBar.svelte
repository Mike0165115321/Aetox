<script lang="ts">
  import { t } from './i18n.svelte'
  import { newSession, switchShell } from './stores/cockpit.svelte'
  import { shell, SHELLS } from './shell.svelte'
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

  // The door switcher (§86). On the wordmark rather than in the room list,
  // because it does not change which room you are in — it changes which
  // building you are standing in, and the two questions must not look like one
  // control. Same place, and the same gesture, as ChatGPT↔Codex.
  let doorOpen = $state(false)
  const current = $derived(SHELLS.find((s) => s.name === shell.name) ?? SHELLS[0])


  function closeOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.brand')) doorOpen = false
  }

  async function pick(name: (typeof SHELLS)[number]['name']) {
    doorOpen = false
    await switchShell(name)
  }
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

<svelte:window onclick={doorOpen ? closeOnOutsideClick : undefined} />

<div class="brand">
  <button
    type="button" class="brand-btn" aria-haspopup="menu" aria-expanded={doorOpen}
    aria-label={t('shell.switch')} onclick={() => (doorOpen = !doorOpen)}
  >
    <Wordmark height={20} />
    <span class="brand-door">{t(current.labelKey)}</span>
    <span class="brand-caret"><Icon name={doorOpen ? 'chevronUp' : 'chevronDown'} size={12} /></span>
  </button>
  {#if doorOpen}
    <div class="door-menu" role="menu">
      {#each SHELLS as s (s.name)}
        <button type="button" class="door-item" class:on={shell.name === s.name} role="menuitem" onclick={() => pick(s.name)}>
          <span class="ic"><Icon name={s.icon} size={15} /></span>
          <span class="txt">
            <span class="t">{t(s.labelKey)}</span>
            <span class="d">{t(s.blurbKey)}</span>
          </span>
          {#if shell.name === s.name}<span class="tick"><Icon name="check" size={13} /></span>{/if}
        </button>
      {/each}
    </div>
  {/if}
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
    <!-- Always, not only while the sidebar is away (owner, 12 ส.ค.). It was
         conditional on the reasoning that the sidebar's header carries this
         otherwise, and that two + buttons on one line is one of them saying
         nothing — true about the pixels, wrong about the act. Starting a new
         chat is the one thing you do from anywhere, and a button that is
         sometimes there is one you have to look for every time: you check the
         corner, find nothing, remember the panel, open the panel. A control at
         a fixed address costs a duplicate; one that moves costs a search on
         every use. -->
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
