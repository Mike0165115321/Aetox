// Point-at-the-page mode, from the app's side. The half that runs inside the
// page is desktop/browser_pick.go.
//
// Two things live here rather than in Workbench.svelte, and for the same
// reason: neither is about drawing a toolbar. The overlay's colours and wording
// are read off the live theme and the live locale and handed to the injected
// script, so the app keeps one palette and one set of strings even though the
// overlay is painted by a script in someone else's document. And the answer is
// turned into the composer chip here, where it can be tested without rendering
// anything.

import {
  BrowserStartPick, BrowserStopPick, BrowserCapturePNG, SaveChatImageData, ReadImageDataURL,
} from '../../../wailsjs/go/main/App'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import { cockpit, nowLabel } from '../stores/cockpit.svelte'
import { t } from '../i18n.svelte'

/** One thing the user pointed at — the shape browser_pick.go sends back. */
export interface PagePick {
  selector: string
  tag: string
  text: string
  html: string
  path: string
  w: number
  h: number
  color: string
  background: string
}

interface PickEvent {
  url: string
  cancelled: boolean
  /** The marks are still on the page, waiting to be photographed. */
  drawn: boolean
  picks: PagePick[] | null
}

/** Pointing at elements, or drawing on the page. */
export type PickMode = 'pick' | 'draw'

/** Which tab is in which mode, null when none. One at a time, always. */
export const pagePick = $state<{ tabId: string | null; mode: PickMode }>({ tabId: null, mode: 'pick' })

let offPick: (() => void) | null = null
let offMeta: (() => void) | null = null

/**
 * What the model reads.
 *
 * The selector alone is a guess; what makes a pick findable in source is the
 * whole set — the class list, the rendered colours, the box, and the ancestors.
 * A rendered `#185fa5` greps straight to the token that produced it.
 */
export function pickChipContent(url: string, picks: PagePick[], drawn = false): string {
  const head = drawn
    ? `Under the marks the user drew on ${url} (the drawing itself is the attached image):`
    : `${picks.length > 1 ? 'Elements' : 'Element'} the user pointed at on ${url}:`
  const blocks = picks.map((p, i) => [
    `[${i + 1}] ${p.selector}`,
    p.text && `    text: ${p.text}`,
    `    box: ${p.w}×${p.h}${p.color ? `  color: ${p.color}` : ''}${p.background ? `  background: ${p.background}` : ''}`,
    p.path && `    within: ${p.path}`,
    p.html && `    html: ${p.html}`,
  ].filter(Boolean).join('\n'))
  return [head, ...blocks].join('\n\n')
}

/** The chip's own line: the one selector, or how many there were. */
export function pickChipLabel(picks: PagePick[]): string {
  if (picks.length === 1) return picks[0].selector
  return t('workbench.pickCount', { n: String(picks.length) })
}

// The overlay is drawn by a script inside the page, so it cannot reach the
// app's stylesheet or its locale — both have to travel with the request.
function overlayOpts(mode: PickMode): string {
  const css = getComputedStyle(document.documentElement)
  const v = (name: string, fallback: string) => css.getPropertyValue(name).trim() || fallback
  // Only the accent travels. The bar carries its own dark palette, because it
  // is drawn on a page whose background nobody knows — see browser_pick.go.
  return JSON.stringify({
    mode,
    accent: v('--accent', '#378add'),
    hint: t(mode === 'draw' ? 'workbench.drawHint' : 'workbench.pickHint'),
    unit: t('workbench.pickUnit'),
    markUnit: t('workbench.drawUnit'),
    doneLabel: t('workbench.drawDone'),
    clearLabel: t('workbench.drawClear'),
    cancelLabel: t('workbench.drawCancel'),
  })
}

// The marks are on the page and nothing else has changed, so this is a still
// life: capture, then take the ink down. Failing to photograph it is not
// failing altogether — the elements under the marks were read before the
// controls came off, and they are worth attaching on their own.
async function attachDrawing(tabId: string): Promise<void> {
  try {
    const dataUrl = await BrowserCapturePNG(tabId)
    const relPath = await SaveChatImageData(dataUrl)
    cockpit.pendingImage = { relPath, dataUrl: await ReadImageDataURL(relPath) }
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
  } finally {
    BrowserStopPick(tabId)
  }
}

function unwire(): void {
  offPick?.()
  offMeta?.()
  offPick = offMeta = null
  pagePick.tabId = null
}

/** Arm a mode on a tab. Pressing the same mode again on the same tab turns it off. */
export async function startPagePick(tabId: string, mode: PickMode = 'pick'): Promise<void> {
  const was = pagePick.tabId === tabId ? pagePick.mode : null
  stopPagePick()
  if (was === mode) return

  pagePick.tabId = tabId
  pagePick.mode = mode
  offPick = EventsOn(`browser:pick:${tabId}`, (e: PickEvent) => {
    unwire()
    const picks = e?.picks ?? []
    // A drawing is worth attaching even when nothing identifiable was under it
    // — the marks are the point, and the picture carries them.
    if (e?.cancelled || (picks.length === 0 && !e?.drawn)) {
      if (e?.drawn) BrowserStopPick(tabId)
      return
    }
    if (picks.length > 0) {
      cockpit.pendingContext = {
        kind: 'pick',
        label: e.drawn ? t('workbench.drawLabel') : pickChipLabel(picks),
        content: pickChipContent(e.url, picks, e.drawn),
      }
    }
    if (e.drawn) attachDrawing(tabId)
  })
  // A navigation takes the overlay with it — the script lived in the document
  // that just went away. Without this the toolbar button stays lit over a page
  // that is not listening to anything.
  offMeta = EventsOn(`browser:meta:${tabId}`, () => unwire())

  try {
    await BrowserStartPick(tabId, overlayOpts(mode))
  } catch {
    // No native window yet (a tab still on its start page), or the tab died.
    // Nothing was armed, so there is nothing to wait for.
    unwire()
  }
}

/** Turn the mode off from the app — the button pressed again, or the tab going away. */
export function stopPagePick(): void {
  const tabId = pagePick.tabId
  unwire()
  if (tabId) BrowserStopPick(tabId)
}
