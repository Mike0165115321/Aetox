// One place owns "is there a newer Aetox, and where is it up to".
//
// Two doors lead to the same act: the notice that appears by itself when the
// automatic check finds something (update_notify.go emits `update:available`),
// and the button in Settings → About that the user pressed on purpose. Before
// this, About owned that state privately — so a second door meant a second copy
// of "downloading / 42% / ready / it failed", free to disagree with the first.
// Both now read and drive this.
//
// The phases are two acts, not one, because they cost the user different
// things (internal/update's Stage/Restart): downloading is bandwidth and can
// happen while they work; restarting costs them whatever they were in the
// middle of. So `ready` is a real resting state — the new build is already on
// disk, and the app will be it whenever they next start it, whether they press
// the button or just close the window tonight.
//
// What this module does NOT decide: which action a channel deserves. Scoop gets
// its command, portable and installer get the one-click button, everything else
// gets the release page — internal/update already makes that call and puts it in
// Status.canAuto / .hint, and re-deriving it here would be the same duplication
// one layer down.

import { StageUpdate, RestartToUpdate, StagedUpdate } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { update } from '../../wailsjs/go/models'

/** idle → downloading → ready → (restarting). `error` can follow either act. */
export type UpdatePhase = 'idle' | 'downloading' | 'ready' | 'restarting' | 'error'

export const updater = $state<{
  /** The newest release the app knows about, or null if it never found one. */
  status: update.Status | null
  /** The user said "later". Holds for this run only — the notice is an offer,
   *  not a to-do item, and re-asking on the next launch is the whole point.
   *  Dismissing a *ready* update costs nothing: it is already on disk. */
  dismissed: boolean
  phase: UpdatePhase
  /** Bytes, as the download actually knows them. total is 0 when the server
   *  never said how big the file is. */
  done: number
  total: number
  /** The version waiting on disk once phase is 'ready'. */
  staged: string
  /** Empty unless something failed, in which case the build the user is
   *  running is still installed and untouched. */
  error: string
}>({ status: null, dismissed: false, phase: 'idle', done: 0, total: 0, staged: '', error: '' })

/** True when there is an offer to put in front of the user right now. */
export function updateOffered(): boolean {
  return !!updater.status?.available && !updater.dismissed
}

/** 0–100, or -1 when nothing is downloading or the size is unknown. */
export function updatePct(): number {
  return updater.total > 0 ? Math.min(100, Math.round((updater.done / updater.total) * 100)) : -1
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
    updater.done = p?.done ?? 0
    updater.total = p?.total ?? 0
  })
  // A webview reload (Vite HMR, or a crash of the frontend alone) leaves the Go
  // side holding a staged update this fresh page knows nothing about. Without
  // this the card would vanish and the user would be offered the same download
  // a second time — for a build already sitting on their disk.
  void StagedUpdate().then((version) => {
    if (version && updater.phase === 'idle') {
      updater.staged = version
      updater.phase = 'ready'
    }
  }).catch(() => {
    /* nothing staged, or the binding is unavailable in a test: idle is right */
  })
  return () => {
    offAvailable()
    offProgress()
  }
}

export function dismissUpdate(): void {
  updater.dismissed = true
}

/** Download and verify. Changes nothing the user can see when it finishes —
 *  the app is one restart away, and the restart is their call. */
export async function startDownload(): Promise<void> {
  if (updater.phase === 'downloading' || updater.phase === 'restarting') return
  updater.phase = 'downloading'
  updater.error = ''
  updater.done = 0
  updater.total = 0
  try {
    await StageUpdate()
    updater.staged = updater.status?.latest ?? ''
    updater.phase = 'ready'
  } catch (err) {
    updater.error = String(err)
    updater.phase = 'error'
  }
}

/** Close this build and come back as the new one. The Go side quits the app a
 *  moment after this resolves, so `restarting` never has to be undone. */
export async function restartToUpdate(): Promise<void> {
  if (updater.phase === 'restarting') return
  updater.phase = 'restarting'
  updater.error = ''
  try {
    await RestartToUpdate()
  } catch (err) {
    // A turn is running, most likely. Nothing was lost — the new build is
    // still staged on disk — so this goes back to offering the restart, never
    // back to offering a download that has already happened.
    updater.error = String(err)
    updater.phase = 'ready'
  }
}
