<script lang="ts">
  import { onDestroy } from 'svelte'
  import type { WorkbenchTab } from '../stores/workbench.svelte'
  import { cockpit } from '../stores/cockpit.svelte'
  import {
    BrowserOpen, BrowserNavigate, BrowserSetBounds, BrowserSetVisible, BrowserClose,
  } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { t } from '../i18n.svelte'

  let { tab, active }: { tab: WorkbenchTab; active: boolean } = $props()

  let host = $state<HTMLDivElement>()
  let opened = $state(false)
  let lastSent = '' // last URL we told the native side to load — breaks the meta-event feedback loop

  // The native WebView2 window floats above the whole UI, so it must hide
  // whenever its workbench tab is inactive or the settings overlay is open.
  const visible = $derived(active && opened && cockpit.activeView !== 'settings')

  function physRect(el: HTMLElement): [number, number, number, number] {
    const r = el.getBoundingClientRect()
    const s = window.devicePixelRatio
    return [Math.round(r.x * s), Math.round(r.y * s), Math.round(r.width * s), Math.round(r.height * s)]
  }

  // Open on first URL; navigate on later URL changes (typed in the address bar).
  $effect(() => {
    const url = tab.url ?? ''
    const el = host
    if (!el || !url || url === lastSent) return
    lastSent = url
    if (!opened) {
      opened = true
      BrowserOpen(tab.id, url, ...physRect(el))
    } else {
      BrowserNavigate(tab.id, url)
    }
  })

  $effect(() => {
    if (opened) BrowserSetVisible(tab.id, visible)
  })

  // Keep the native window glued to this pane's rect.
  $effect(() => {
    const el = host
    if (!el) return
    const update = () => {
      if (opened) BrowserSetBounds(tab.id, ...physRect(el))
    }
    const ro = new ResizeObserver(update)
    ro.observe(el)
    window.addEventListener('resize', update)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', update)
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
    if (host && visible) BrowserSetBounds(tab.id, ...physRect(host))
  })

  onDestroy(() => {
    off()
    if (opened) BrowserClose(tab.id)
  })
</script>

<div class="native-host" bind:this={host}>
  {#if !tab.url}
    <div class="insp-blank">
      <span class="ic">🌐</span>
      <div class="insp-blank-title">{t('browserPane.startBrowsing')}</div>
      <div class="insp-blank-sub">{t('browserPane.enterUrl')}</div>
    </div>
  {/if}
</div>
