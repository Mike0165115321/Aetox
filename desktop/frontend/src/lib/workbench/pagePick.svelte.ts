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
  DeckPickScript, DeckStopPickScript, DeckCaptureDrawing,
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
  return [head, ...pickBlocks(picks)].join('\n\n')
}

/** One block per pick. Shared, so the deck's chip and the page's chip cannot
 *  come to describe the same node two different ways. */
function pickBlocks(picks: PagePick[]): string[] {
  return picks.map((p, i) => [
    `[${i + 1}] ${p.selector}`,
    p.text && `    text: ${p.text}`,
    `    box: ${p.w}×${p.h}${p.color ? `  color: ${p.color}` : ''}${p.background ? `  background: ${p.background}` : ''}`,
    p.path && `    within: ${p.path}`,
    p.html && `    html: ${p.html}`,
  ].filter(Boolean).join('\n'))
}

/**
 * The same chip for a slide, and the difference is the first line.
 *
 * A page belongs to somebody else, so the most that can be done about the thing
 * pointed at is talk about it, and the head names a URL. A deck is a file on
 * this machine, usually one the agent wrote a moment ago, so the head names the
 * path and the slide instead: that is the difference between 'here is what you
 * are looking at' and 'here is what to change'.
 */
export function deckPickChipContent(path: string, slide: number, picks: PagePick[]): string {
  const what = picks.length > 1 ? 'Elements' : 'Element'
  const head = `${what} the user pointed at on slide ${slide} of the deck at ${path} (an .html file on disk):`
  return [head, ...pickBlocks(picks)].join('\n\n')
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

// ---------- The same thing, aimed at a deck ----------
//
// A browser tab is a native OS window, so its overlay has to be injected by the
// engine and answered through the engine's bridge. A deck is an <iframe> in the
// app's own webview and same-origin with it, so both halves are reachable from
// here: the script goes in as a <script> node, and it answers by calling a
// function this module hangs on the window.
//
// The overlay itself still comes from Go (DeckPickScript). It is three hundred
// lines of pointer handling and selector building, and a copy of it living here
// would be the same feature with two behaviours.

/** Which deck is armed and in which mode. Null when none: one at a time. */
export const deckPick = $state<{ path: string | null; mode: PickMode; capturing: boolean }>({
  path: null,
  mode: 'pick',
  // The picture of a drawing is rendered rather than screen-grabbed, which takes
  // a second or two. The panel reads this so those seconds are not silence.
  capturing: false,
})

/** The window property the injected overlay calls back on. */
const DECK_CALLBACK = '__aetoxDeckPick'

let deckToken = ''

/** Run a script from Go inside a same-origin frame.
 *
 *  A <script> node rather than eval or Function: those are what a Content
 *  Security Policy blocks first, and a deck is a file the user keeps and may
 *  well carry one. The node is taken out straight after, because it has already
 *  run and anything reading the document back would otherwise find it there. */
function runInFrame(doc: Document, source: string): void {
  const el = doc.createElement('script')
  el.textContent = source
  doc.documentElement.appendChild(el)
  el.remove()
}

/**
 * The ink layer, at the slide's own size.
 *
 * The overlay draws on a canvas backed at device pixels (innerWidth x DPR), and
 * what Go composites onto is the slide rendered at its own 1280x720. Handing
 * over the backing store would mean Go resampling a 2x picture down for no
 * reason: the browser is the better resampler and it is already right here.
 *
 * Found by the mark the overlay puts on its own nodes rather than by tag. A deck
 * is free to carry canvases of its own, and photographing one of those would be
 * a drawing nobody made.
 */
function inkDataURL(doc: Document): string | null {
  const view = doc.defaultView
  const canvas = Array.from(doc.documentElement.children).find(
    (el): el is HTMLCanvasElement =>
      el.tagName === 'CANVAS' && !!(el as unknown as { __aetoxOverlay?: number }).__aetoxOverlay,
  )
  if (!canvas || !view) return null
  const w = Math.round(view.innerWidth)
  const h = Math.round(view.innerHeight)
  if (w <= 0 || h <= 0) return null
  if (w === canvas.width && h === canvas.height) return canvas.toDataURL('image/png')
  const flat = doc.createElement('canvas')
  flat.width = w
  flat.height = h
  flat.getContext('2d')?.drawImage(canvas, 0, 0, w, h)
  return flat.toDataURL('image/png')
}

/**
 * Photograph the marks and attach the picture.
 *
 * The browser's version of this asks the engine for a picture of the tab. There
 * is no such call for a frame inside the app's own webview, so the picture is
 * made instead: Go renders the slide the way every export renders it and lays
 * the ink over it (deck_draw.go).
 *
 * Failing to photograph it is not failing altogether. The elements under the
 * marks were read before the controls came off, and the chip carrying them has
 * already landed.
 */
async function attachDeckDrawing(doc: Document, path: string, slide: number): Promise<void> {
  deckPick.capturing = true
  try {
    const ink = inkDataURL(doc)
    if (!ink) return
    const shot = await DeckCaptureDrawing(path, slide, ink)
    const relPath = await SaveChatImageData(shot)
    cockpit.pendingImage = { relPath, dataUrl: await ReadImageDataURL(relPath) }
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
  } finally {
    deckPick.capturing = false
    // The ink was left standing so it could be photographed. Down it comes now,
    // whether or not the picture arrived.
    stopDeckPick(doc)
  }
}

/**
 * Arm a mode on a deck. Pressing the same mode again turns it off; pressing the
 * other one switches, the same as the browser toolbar.
 *
 * `slide` is read when the mode is armed rather than when the answer lands,
 * because the overlay eats every event in the document while it is up: the deck
 * cannot be paged while somebody is pointing at it, so the number cannot go
 * stale between the two moments.
 */
export async function startDeckPick(
  doc: Document,
  path: string,
  slide: number,
  mode: PickMode = 'pick',
): Promise<void> {
  const was = deckPick.path === path ? deckPick.mode : null
  stopDeckPick(doc)
  if (was === mode) return

  const token = crypto.randomUUID()
  deckToken = token
  deckPick.path = path
  deckPick.mode = mode
  const view = window as unknown as Record<string, unknown>
  view[DECK_CALLBACK] = (raw: string) => {
    let e: {
      __aetox?: string; token?: string; cancelled?: boolean; drawn?: boolean; picks?: PagePick[]
    } = {}
    try {
      e = JSON.parse(raw)
    } catch {
      return
    }
    // A deck is a document the user keeps and can carry any script at all. The
    // token is what separates the answer this app asked for from one the file
    // decided to send on its own.
    if (e.__aetox !== 'pick' || e.token !== deckToken) return
    deckToken = ''
    deckPick.path = null
    delete view[DECK_CALLBACK]
    const picks = e.picks ?? []
    // A drawing is worth attaching even when nothing identifiable was under it:
    // the marks are the point, and the picture carries them.
    if (e.cancelled || (picks.length === 0 && !e.drawn)) {
      if (e.drawn) stopDeckPick(doc)
      return
    }
    if (picks.length > 0) {
      cockpit.pendingContext = {
        kind: 'pick',
        label: e.drawn ? t('workbench.drawLabel') : pickChipLabel(picks),
        content: deckPickChipContent(path, slide, picks),
      }
    }
    if (e.drawn) void attachDeckDrawing(doc, path, slide)
  }

  try {
    runInFrame(doc, await DeckPickScript(token, overlayOpts(mode)))
  } catch {
    // The frame went away mid-await, or the script would not run. Nothing is
    // armed, so nothing is waiting for an answer.
    deckToken = ''
    deckPick.path = null
    delete view[DECK_CALLBACK]
  }
}

/** Take it down: the button pressed again, the panel unmounting, a reload. */
export function stopDeckPick(doc: Document | null): void {
  const view = window as unknown as Record<string, unknown>
  // The app-side state goes first and without waiting, because this is called
  // from onDestroy: a teardown that has to finish a round trip is one that
  // sometimes does not finish. From here on the token is empty, so any envelope
  // that still arrives answers nothing.
  deckToken = ''
  deckPick.path = null
  if (!doc) {
    delete view[DECK_CALLBACK]
    return
  }
  // The overlay's own stop() ends by posting a cancelled envelope, so the
  // property has to survive the teardown it is about to trigger: taking it away
  // first would throw a TypeError inside the user's own deck. A sink stands in
  // its place and is removed once the script has run.
  //
  // The identity check is not decoration. startDeckPick calls this before arming,
  // and by the time the script comes back the new round has already installed its
  // own callback — deleting it then would disarm the mode the user just switched on.
  const sink = () => {}
  view[DECK_CALLBACK] = sink
  const clear = () => {
    if (view[DECK_CALLBACK] === sink) delete view[DECK_CALLBACK]
  }
  DeckStopPickScript()
    .then((src) => {
      try {
        runInFrame(doc, src)
      } catch {
        // The document is already gone, which is the outcome this wanted.
      }
      clear()
    })
    .catch(clear)
}
