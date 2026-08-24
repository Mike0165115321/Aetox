// ไฟบอกสถานะ — which of the four "the agent is working here" layers are on.
//
// Held in one store rather than read per component, because three separate
// pieces of the browser panel draw from it (the panel's border, the action
// strip, the mark on a tab) and a fourth is drawn into the page itself. Four
// readers asking Go the same question four times would be four chances to
// disagree about it.
//
// Read once when the panel first needs it, then kept in step by the setter: the
// switches are only ever changed from the one menu, so there is no second
// writer to watch for.

import { BusySignal, SetBusyLayer } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import type { ToolEvent } from '../types'

export const busy = $state<{ layers: main.BusyLayer[]; loaded: boolean }>({
  layers: [],
  loaded: false,
})

/** Is this layer on right now?
 *
 * True before the first read lands, deliberately — three of the four ship on,
 * and a panel that flickers its border off and back on while it waits for an
 * answer is worse than one that starts as it will end. The one that ships off
 * is named here so it does not get the same benefit of the doubt.
 */
export function layerOn(id: string): boolean {
  const found = busy.layers.find((l) => l.id === id)
  if (found) return found.on
  return !busy.loaded && id !== 'tabDot'
}

/** Read the switches. Called by the browser panel the first time it draws one;
 * repeated calls after the first are cheap and idempotent. */
export async function loadBusySignal(): Promise<void> {
  if (busy.loaded) return
  try {
    busy.layers = (await BusySignal()) ?? []
  } catch {
    // A visual preference that could not be read is not worth an error on
    // screen: layerOn's fallback above is already the shipped answer.
  }
  busy.loaded = true
}

/** Flip one layer. Go hands back the whole set, so the store is replaced rather
 * than patched — one answer about all four, from the side that owns them. */
export async function toggleBusyLayer(id: string, on: boolean): Promise<void> {
  try {
    busy.layers = (await SetBusyLayer(id, on)) ?? busy.layers
    busy.loaded = true
  } catch {
    // Left as it was. A switch that did not take is better than a switch that
    // says it took and did not.
  }
}

// ── What the browser is doing right now ─────────────────────────────────────
//
// The switches above say what the panel MAY draw. This half says whether there
// is anything to draw, and it is driven by tool events and by nothing else.
//
// **Never a timer.** A layer starts on a browser tool `call` and stops on that
// call's `result`. The whole job of this signal is to be believed, and a light
// that keeps running because a countdown has not expired is a light that says
// the agent is working when it is not — which costs more than the light was
// ever worth.

/** A tool event as it arrives on the wire: stamped with the session it happened
 *  in, or bare from an older engine. Spelled here rather than imported from the
 *  chat's store, which would make the browser panel depend on the whole cockpit
 *  to read one field. */
type Stamped = { sessionId?: string; data?: ToolEvent }

/** The name the packed browser tool answers to, matching desktop/browser_tool.go
 *  browserToolName. Matched on rather than on the action, because every browser
 *  action arrives under this one name (§99) and the action is a separate field. */
const BROWSER_TOOL = 'browser'

export const busyWork = $state<{
  /** The tab id the call is working, '' when the engine could not say — which is
   *  the honest state for the first `open` of a session, before any tab exists.
   *  The panel lights itself rather than a chip when this is empty. */
  tab: string
  /** The browser action: open, read, click, type, scroll, capture, … */
  act: string
  /** The one argument worth reading — a URL on `open`, empty on a click. */
  subject: string
  /** A browser call is in flight this instant. Drives every moving part. */
  running: boolean
  /** The browser has been touched at some point in this turn. Drives only
   *  whether the action bar is MOUNTED, and it is a separate fact from
   *  `running` for a physical reason: the bar is a row above the pane, so
   *  mounting it resizes the native browser window. Tied to `running` it would
   *  resize the page twice per call — twenty times in a turn that browses — and
   *  a page that reflows under the agent between its read and its click is a
   *  page whose refs have moved. Once per turn instead. */
  seen: boolean
}>({ tab: '', act: '', subject: '', running: false, seen: false })

// The calls in flight, by the engine's tool-call id. A set rather than a
// boolean because a round can carry more than one browser call, and a single
// flag would be cleared by whichever finished first while the other was still
// running.
const inFlight = new Set<string>()

/** Read one tool event for what it says about the browser.
 *
 * Called beside applyToolEvent rather than from inside it: the chat's timeline
 * and the browser panel are two different readers of the same event, and
 * folding this into the timeline's own function would put a workbench concern
 * inside the store that draws messages.
 */
export function applyBusyEvent(stamped: Stamped | ToolEvent): void {
  const ev = (stamped as Stamped)?.data ?? (stamped as ToolEvent)
  if (!ev || ev.name !== BROWSER_TOOL) return
  // Keyed on the call id, falling back to the action for an engine that sends
  // none — two calls of the same action in one round would then share a key,
  // which loses one of them rather than leaving a light stuck on.
  const key = ev.ref || `act:${ev.act ?? ''}`
  if (ev.action === 'call') {
    inFlight.add(key)
    busyWork.act = ev.act ?? ''
    busyWork.subject = ev.subject ?? ''
    busyWork.tab = ev.tab ?? ''
    busyWork.running = true
    busyWork.seen = true
    return
  }
  if (ev.action !== 'result') return
  inFlight.delete(key)
  busyWork.running = inFlight.size > 0
  // act/subject/tab are left standing. The bar keeps saying what was just
  // done, with its light out — a record of the last action, which is true,
  // rather than a blank strip taking up room for no reason.
}

/** The turn ended. Everything this signal knows was about that turn.
 *
 * This is the other end of the "never a timer" rule and it is not a fallback
 * for a missing result: a stopped turn, a provider that dropped the connection,
 * a tool killed mid-call — none of them owe us a result event, and without this
 * the panel would still be glowing tomorrow morning. It is an event too, just a
 * later one. */
export function clearBusyWork(): void {
  inFlight.clear()
  busyWork.tab = ''
  busyWork.act = ''
  busyWork.subject = ''
  busyWork.running = false
  busyWork.seen = false
}
