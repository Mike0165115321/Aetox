<script lang="ts">
  import { onDestroy } from 'svelte'
  import { recordVisit, type WorkbenchTab } from '../stores/workbench.svelte'
  import { cockpit, isOverlayView } from '../stores/cockpit.svelte'
  import {
    BrowserOpen, BrowserNavigate, BrowserSetBounds, BrowserSetVisible, BrowserSetZoom,
  } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { isHostWebview } from '../hostWebview'
  import { t } from '../i18n.svelte'
  import BrowserStart from './BrowserStart.svelte'

  let { tab, active, menuOpen, dragging }: { tab: WorkbenchTab; active: boolean; menuOpen: boolean; dragging: boolean } = $props()

  // The native window this pane glues itself to exists ONCE, inside the real
  // app window. wails dev hands this same component to every browser that
  // connects, and each copy believes the window is its to place — so a second
  // connected frontend kept regluing the app's window to ITS geometry (~390px,
  // over the chat column) every time the agent opened a page (§191). A
  // frontend that is not the app's own webview watches; it does not steer.
  const spectator = !isHostWebview()

  let host = $state<HTMLDivElement>()
  let opened = $state(false)
  let lastSent = '' // last URL we told the native side to load — breaks the meta-event feedback loop

  /** Pane pixels reserved around the native window, in CSS px. See layout(). */
  const PANE_FRAME = 3

  // The native WebView2 window is a real OS window: it composites above the
  // app's own webview no matter what the DOM does, so anything the app draws
  // over this pane is invisible until the window hides. That is every
  // full-window room, and the workbench dropdowns (+ / ⋮) which open downward
  // into it.
  //
  // It used to name `settings` and nothing else, from the days when settings
  // was the only room drawn over the app. Three more arrived and none of them
  // was added here, so a loaded page floated on top of ทีมเอเจน, ผลงาน and
  // โปรเจกต์ — the failure is invisible to whoever adds the room, because their
  // room works and it is somebody else's window that is wrong.
  // isOverlayView derives the set from the one list, so the next room is
  // covered by having been added at all.
  //
  // `dragging` is the same problem one step further: while this window is up it
  // also *swallows* the drag, since the pointer is over another window, so the
  // workbench's drop target could never see a file dropped on a tab with a page
  // open. It stands down for the length of the drag (Workbench.svelte).
  const visible = $derived(active && opened && !menuOpen && !dragging && !isOverlayView(cockpit.activeView))

  // Device-size emulation without any emulation trickery: the tab IS a real
  // window, so shrink it to the device's aspect ratio (letterboxed in the pane,
  // never upscaled past 1:1) and zoom the page by that same factor. The page's
  // CSS viewport then measures exactly the device's w×h — real browser zoom, so
  // its media queries fire the way they would on the device. No preset = fill
  // the pane at zoom 1.
  function layout(el: HTMLElement): { rect: [number, number, number, number]; scale: number } {
    const box = el.getBoundingClientRect()
    // A few pixels of the pane kept back from the native window, all the way
    // round. ไฟบอกสถานะ's border light is drawn by the app, and the app
    // draws BEHIND this window: flush to the pane, the comet would run its lap
    // hidden under the page on three sides out of four. This is the strip it
    // runs in (§174).
    //
    // Held back always, not only while the agent works. Insetting on demand
    // would resize the native window twice per browser call, and every resize
    // is a real page reflow under an agent that is in the middle of reading it.
    // A constant frame costs three pixels and moves nothing, ever.
    const r = {
      x: box.x + PANE_FRAME, y: box.y + PANE_FRAME,
      width: Math.max(0, box.width - PANE_FRAME * 2),
      height: Math.max(0, box.height - PANE_FRAME * 2),
    }
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
    if (spectator || !opened || !host) return
    const { rect, scale } = layout(host)
    BrowserSetBounds(tab.id, ...rect)
    BrowserSetZoom(tab.id, scale)
  }

  // Open on first URL; navigate on later URL changes (typed in the address bar).
  // A spectator never opens or steers: `opened` stays false there, which is
  // what keeps every other native call in this file naturally inert.
  $effect(() => {
    const url = tab.url ?? ''
    const el = host
    if (spectator || !el || !url || url === lastSent) return
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
    if (spectator || !el) return
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
    // Every navigation that lands, whoever caused it — this is the only place
    // that sees a link clicked inside a page.
    recordVisit(meta.url, meta.title)
    // Re-glue bounds + z-order after every completed navigation: the app's own
    // WebView2 can composite above the tab's window right after it opens,
    // leaving the page loaded but invisible until something else forces
    // HWND_TOP (see browser.go z-order note).
    if (visible) reflow()
  })

  // Unmounting is not closing, and this used to treat it as if it were.
  //
  // A pane goes away for reasons that have nothing to do with anybody's
  // intent — most often because the ENGINE closed the tab and told us, which
  // takes the chip off the strip and takes this pane with it. Closing from
  // here then asked Go to close what Go had just closed, and Go read that
  // second call as the user pressing ×. The agent was told a person had shut
  // its page, six seconds after it opened one, three times in forty seconds
  // (docs/architecture/browser-tab-lifetime-2026-08-25.md).
  //
  // Every close now says who it is at the place it happens: the × goes through
  // the store's closeTab, the agent's own close goes through its tool, and a
  // window orphaned by a reload — the one case this hook was really covering,
  // and the one case where it does not run at all — is swept by
  // CloseAllBrowserTabs on the next mount.
  onDestroy(() => {
    off()
  })
</script>

<div class="native-host" bind:this={host}>
  {#if spectator && tab.url}
    <!-- Without this the pane is a black void, and a black void where a page
         should be reads as "rendering broke" - the exact sentence that opened
         the investigation this note came out of. -->
    <div class="spectator-note">{t('workbench.spectator')}</div>
  {:else if tab.viewport}
    <!-- CSS letterboxes this to the same box layout() gives the native window,
         so picking a device preset is visible even before a page is loaded —
         otherwise an empty tab makes the whole menu look dead. -->
    <div class="device-frame" style="--dw:{tab.viewport.w}; --dh:{tab.viewport.h}">
      {#if !tab.url}<BrowserStart {tab} />{/if}
    </div>
  {:else if !tab.url}
    <BrowserStart {tab} />
  {/if}
</div>
