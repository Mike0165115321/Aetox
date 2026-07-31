<script lang="ts">
  // One dialog for every action that cannot be undone. Before this, three pages
  // used a click-twice-to-arm pattern and three others deleted on the first
  // click — so the user learned "it asks first" from one page and lost a
  // configured MCP server to the next.
  //
  // Deliberately not a native <dialog>: this app renders in WebView2 inside a
  // frameless window, and ::backdrop plus the top-layer promotion fight the
  // app's own overlay stack (Palette, Onboarding). A plain fixed overlay
  // behaves the same in every theme and is what the two existing overlays do.
  import { t } from './i18n.svelte'

  let {
    title,
    message,
    detail = '',
    confirmLabel,
    onConfirm,
    onCancel,
  }: {
    title: string
    message: string
    /** The exact thing being destroyed — a server name, a file path. Rendered
     *  monospace and unwrapped, so the user checks it, not the sentence. */
    detail?: string
    confirmLabel: string
    onConfirm: () => void
    onCancel: () => void
  } = $props()

  let cancelEl = $state<HTMLButtonElement | null>(null)
  const opener = typeof document !== 'undefined' ? document.activeElement : null

  // Focus lands on Cancel, never on the destructive button. A dialog that
  // appears under the user's finger must not turn a stray Enter into a delete.
  $effect(() => {
    cancelEl?.focus()
    return () => {
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
    }
  })

  // Escape closes; Tab is trapped between the two buttons so focus can never
  // reach the page behind the overlay while it is up.
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onCancel()
      return
    }
    if (e.key !== 'Tab') return
    const focusable = Array.from(
      e.currentTarget instanceof HTMLElement
        ? e.currentTarget.querySelectorAll<HTMLElement>('button')
        : [],
    )
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    const active = document.activeElement
    if (e.shiftKey && active === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && active === last) {
      e.preventDefault()
      first.focus()
    }
  }
</script>

<!-- Escape is handled on the container rather than on window: focus is trapped
     inside it, so the key always lands here first, and stopping it there keeps
     one Escape from also closing Settings behind the dialog. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="confirm-overlay"
  role="alertdialog"
  tabindex="-1"
  aria-modal="true"
  aria-labelledby="confirm-title"
  aria-describedby="confirm-message"
  onkeydown={onKeydown}
>
  <button class="confirm-backdrop" aria-label={t('settings.cancel')} onclick={onCancel}></button>
  <div class="confirm-card">
    <h3 id="confirm-title" class="confirm-title">{title}</h3>
    <p id="confirm-message" class="confirm-message">{message}</p>
    {#if detail}
      <div class="confirm-detail">{detail}</div>
    {/if}
    <div class="confirm-actions">
      <button class="ctrl confirm-cancel" bind:this={cancelEl} onclick={onCancel}>
        {t('settings.cancel')}
      </button>
      <button class="ctrl ctrl-danger confirm-go" onclick={onConfirm}>{confirmLabel}</button>
    </div>
  </div>
</div>
