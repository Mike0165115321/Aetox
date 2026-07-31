<script lang="ts">
  import { onDestroy } from 'svelte'
  import type { WorkbenchTab } from '../stores/workbench.svelte'
  import { cockpit } from '../stores/cockpit.svelte'
  import {
    BrowserOpen, BrowserNavigate, BrowserSetBounds, BrowserSetVisible, BrowserClose, BrowserSetZoom,
  } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  let { tab, active, menuOpen }: { tab: WorkbenchTab; active: boolean; menuOpen: boolean } = $props()

  let host = $state<HTMLDivElement>()
  let opened = $state(false)
  let lastSent = '' // last URL we told the native side to load — breaks the meta-event feedback loop

  // The native WebView2 window is a real OS window: it composites above the
  // app's own webview no matter what the DOM does, so anything the app draws
  // over this pane is invisible until the window hides. That's the settings
  // overlay, and the workbench dropdowns (+ / ⋮) which open downward into it.
  const visible = $derived(active && opened && !menuOpen && cockpit.activeView !== 'settings')

  // Device-size emulation without any emulation trickery: the tab IS a real
  // window, so shrink it to the device's aspect ratio (letterboxed in the pane,
  // never upscaled past 1:1) and zoom the page by that same factor. The page's
  // CSS viewport then measures exactly the device's w×h — real browser zoom, so
  // its media queries fire the way they would on the device. No preset = fill
  // the pane at zoom 1.
  function layout(el: HTMLElement): { rect: [number, number, number, number]; scale: number } {
    const r = el.getBoundingClientRect()
    const s = window.devicePixelRatio
    const vp = tab.viewport
    const scale = vp ? Math.min(1, r.width / vp.w, r.height / vp.h) : 1
    const w = vp ? vp.w * scale : r.width
    const h = vp ? vp.h * scale : r.height
    const rect: [number, number, number, number] = [
      Math.round((r.x + (r.width - w) / 2) * s), Math.round((r.y + (r.height - h) / 2) * s),
      Math.round(w * s), Math.round(h * s),
    ]
    return { rect, scale }
  }

  /** Re-glue the native window to the pane (and re-apply the emulation zoom). */
  function reflow(): void {
    if (!opened || !host) return
    const { rect, scale } = layout(host)
    BrowserSetBounds(tab.id, ...rect)
    BrowserSetZoom(tab.id, scale)
  }

  // Open on first URL; navigate on later URL changes (typed in the address bar).
  $effect(() => {
    const url = tab.url ?? ''
    const el = host
    if (!el || !url || url === lastSent) return
    lastSent = url
    if (!opened) {
      opened = true
      BrowserOpen(tab.id, url, ...layout(el).rect)
    } else {
      BrowserNavigate(tab.id, url)
    }
  })

  // Switching device preset resizes the window and rescales the page.
  $effect(() => {
    tab.viewport
    reflow()
  })

  $effect(() => {
    if (opened) BrowserSetVisible(tab.id, visible)
  })

  // Keep the native window glued to this pane's rect.
  $effect(() => {
    const el = host
    if (!el) return
    const ro = new ResizeObserver(reflow)
    ro.observe(el)
    window.addEventListener('resize', reflow)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', reflow)
    }
  })

  // The page reports its real title/URL after every navigation (including
  // in-page link clicks) — keep the tab and address bar in sync.
  // svelte-ignore state_referenced_locally — tab.id never changes for a mounted pane
  const off = EventsOn(`browser:meta:${tab.id}`, (meta: { title: string; url: string }) => {
    lastSent = meta.url
    tab.url = meta.url
    if (meta.title) tab.name = meta.title.length > 24 ? meta.title.slice(0, 24) + '…' : meta.title
    // Re-glue bounds + z-order after every completed navigation: the app's own
    // WebView2 can composite above the tab's window right after it opens,
    // leaving the page loaded but invisible until something else forces
    // HWND_TOP (see browser.go z-order note).
    if (visible) reflow()
  })

  onDestroy(() => {
    off()
    if (opened) BrowserClose(tab.id)
  })
</script>

{#snippet blank()}
  <div class="insp-blank">
    <span class="ic"><Icon name="globe" size={14} /></span>
    <div class="insp-blank-title">{t('browserPane.startBrowsing')}</div>
    <div class="insp-blank-sub">{t('browserPane.enterUrl')}</div>
  </div>
{/snippet}

<div class="native-host" bind:this={host}>
  {#if tab.viewport}
    <!-- CSS letterboxes this to the same box layout() gives the native window,
         so picking a device preset is visible even before a page is loaded —
         otherwise an empty tab makes the whole menu look dead. -->
    <div class="device-frame" style="--dw:{tab.viewport.w}; --dh:{tab.viewport.h}">
      {#if !tab.url}{@render blank()}{/if}
    </div>
  {:else if !tab.url}
    {@render blank()}
  {/if}
</div>
