// One place owns "is there a newer Aetox, and is one being installed right now".
//
// Two doors lead to the same act: the notice that appears by itself when the
// automatic check finds something (update_notify.go emits `update:available`),
// and the button in Settings → About that the user pressed on purpose. Before
// this, About owned that state privately — so a second door meant a second copy
// of "downloading / 42% / restarting / it failed", free to disagree with the
// first. Both now read and drive this.
//
// What this module does NOT decide: which action a channel deserves. Scoop gets
// its command, portable and installer get the one-click button, everything else
// gets the release page — internal/update already makes that call and puts it in
// Status.canAuto / .hint, and re-deriving it here would be the same duplication
// one layer down.

import { ApplyUpdate } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { update } from '../../wailsjs/go/models'

export const updater = $state<{
  /** The newest release the app knows about, or null if it never found one. */
  status: update.Status | null
  /** The user said "later". Holds for this run only — the notice is an offer,
   *  not a to-do item, and re-asking on the next launch is the whole point. */
  dismissed: boolean
  /** Download → verify → swap is running. Stays true after it succeeds: the
   *  window is about to close, and a control that re-arms in its last frame
   *  invites a second press onto an exe that is already being replaced. */
  applying: boolean
  /** 0–100, or -1 when nothing is downloading or the server sent no size. */
  pct: number
  /** The swap is done and the app is closing so the relauncher can take over. */
  restarting: boolean
  /** Empty unless the attempt failed, in which case the old build is still
   *  installed and untouched — which is why the button re-arms. */
  error: string
}>({ status: null, dismissed: false, applying: false, pct: -1, restarting: false, error: '' })

/** True when there is something to show the user right now. */
export function updateOffered(): boolean {
  return !!updater.status?.available && !updater.dismissed
}

/** Called once from App. Returns the unsubscriber Svelte's onMount expects. */
export function listenForUpdates(): () => void {
  const offAvailable = EventsOn('update:available', (st: update.Status) => {
    if (!st?.available) return
    // A newer release than the one already on offer un-dismisses the notice:
    // "later" was said about a different version.
    if (updater.status && updater.status.latest !== st.latest) updater.dismissed = false
    updater.status = st
  })
  const offProgress = EventsOn('update:progress', (p: { done: number; total: number }) => {
    updater.pct = p?.total > 0 ? Math.min(100, Math.round((p.done / p.total) * 100)) : -1
  })
  return () => {
    offAvailable()
    offProgress()
  }
}

export function dismissUpdate(): void {
  updater.dismissed = true
}

/** Download, verify, swap, restart. The Go side closes the window a moment
 *  after this resolves; on failure nothing was replaced and pressing again is
 *  legitimate. */
export async function startUpdate(): Promise<void> {
  if (updater.applying) return
  updater.applying = true
  updater.error = ''
  updater.pct = -1
  try {
    await ApplyUpdate()
    updater.restarting = true
  } catch (err) {
    updater.error = String(err)
    updater.applying = false
  }
}
