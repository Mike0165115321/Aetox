// One place owns "is there a newer Aetox, and where is it up to".
//
// Three doors lead to the same act: the notice that appears by itself when the
// automatic check finds something (update_notify.go emits `update:available`),
// the button in Settings → About that the user pressed on purpose, and the
// version row in the profile menu — which is where somebody who never opens a
// settings page still finds out a new Aetox exists. Before this, About owned
// that state privately — so a second door meant a second copy of "downloading /
// 42% / ready / it failed", free to disagree with the first. All three now read
// and drive this: the running version, the answer to "is there a newer one",
// and how far the download got.
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

import {
  StageUpdate, RestartToUpdate, StagedUpdate, CheckForUpdate, AppVersion,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { update } from '../../wailsjs/go/models'

/** idle → downloading → ready → (restarting). `error` can follow either act. */
export type UpdatePhase = 'idle' | 'downloading' | 'ready' | 'restarting' | 'error'

export const updater = $state<{
  /** The version this build is, straight from Go. Empty until asked — the menu
   *  shows a dash rather than guessing, because a version number the app made
   *  up is worse than one it has not fetched yet. */
  current: string
  /** The newest release the app knows about, or null if it never found one. */
  status: update.Status | null
  /** True when the standing offer arrived on its own (the daily check's
   *  `update:available`), false when the user went and asked. Only the first
   *  kind earns the card that covers the window: a check the user pressed is
   *  answered where they pressed it, and a card on top of that answer would be
   *  the same news twice. */
  announced: boolean
  /** A check is in flight. Its own flag rather than a phase, because checking
   *  and downloading can be true of the same minute — the phases below are
   *  about bytes, this is about a question. */
  checking: boolean
  /** Why the last check did not complete, or "". Separate from `error`: a
   *  check that could not reach GitHub says nothing about a download, and
   *  folding the two would let one overwrite the other's sentence. */
  checkError: string
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
}>({
  current: '', status: null, announced: false, checking: false, checkError: '',
  dismissed: false, phase: 'idle', done: 0, total: 0, staged: '', error: '',
})

/** True when there is an offer to put in front of the user right now. */
export function updateOffered(): boolean {
  return !!updater.status?.available && !updater.dismissed
}

/** True when the offer arrived unasked and so still has to be carried to the
 *  user by the card. A door that shows the offer itself uses `updateOffered`. */
export function updateAnnounced(): boolean {
  return updateOffered() && updater.announced
}

/** 0–100, or -1 when nothing is downloading or the size is unknown. */
export function updatePct(): number {
  return updater.total > 0 ? Math.min(100, Math.round((updater.done / updater.total) * 100)) : -1
}

/** Which version this build is, for every door that names it. One Go call per
 *  run — the number cannot change while the process lives — and safe to call
 *  from anywhere, since the second caller finds it already answered. */
export function loadCurrentVersion(): void {
  if (updater.current) return
  void AppVersion().then((v) => { updater.current = v }).catch(() => {
    /* backend not up yet: the door shows a dash rather than a wrong number */
  })
}

/** Called once from App. Returns the unsubscriber Svelte's onMount expects. */
export function listenForUpdates(): () => void {
  loadCurrentVersion()

  const offAvailable = EventsOn('update:available', (st: update.Status) => {
    if (!st?.available) return
    updater.announced = true
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

/** Ask GitHub now, because the user asked. Both doors that carry a button call
 *  this — Settings → About and the profile menu — so the answer they show is
 *  one answer, not two that can disagree by a release.
 *
 *  A failed check does NOT wipe a status that was already found: "could not
 *  reach GitHub just now" is not evidence that the release announced an hour
 *  ago stopped existing, and erasing it would take a real offer off the screen
 *  because the wifi dropped. */
export async function checkNow(): Promise<void> {
  if (updater.checking) return
  updater.checking = true
  updater.checkError = ''
  try {
    const st = await CheckForUpdate()
    // A different release than the one already waved off is news again — the
    // same rule the announcement follows, for the same reason.
    if (updater.status && updater.status.latest !== st.latest) updater.dismissed = false
    updater.status = st
    // Asked for, so answered where it was asked. See `announced`.
    updater.announced = false
    // Status.current is deliberately NOT copied into updater.current. It is
    // the same Go constant AppVersion returns, and a fact with two writers is
    // a fact that can be written twice differently — which is the whole reason
    // this module exists.
  } catch (err) {
    // Offline, rate-limited, a proxy eating the response: say so and change
    // nothing else. A failed check is not a broken app.
    updater.checkError = String(err)
  } finally {
    updater.checking = false
  }
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
