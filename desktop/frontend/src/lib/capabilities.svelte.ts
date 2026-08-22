// What a capability download is doing right now, for the one card that says so.
//
// Modelled on selfUpdate.svelte.ts and drawn with the same card, because it is
// the same kind of event: a large download the user started, which must not
// interrupt them and must not finish in silence either. The first-run screen
// hands off to this deliberately — pressing "install" there lands you in the
// app, and the last screen of a setup is the worst possible place to watch a
// progress bar (docs/architecture/capability-install-2026-08-21.md).

import { EventsOn } from '../../wailsjs/runtime/runtime'
import { InstallCapabilities } from '../../wailsjs/go/main/App'

type Phase = 'idle' | 'installing' | 'done' | 'error'

export const capabilities = $state({
  phase: 'idle' as Phase,
  /** Which download of how many. Components, not capabilities: "speech" is two
   *  files, and a bar that sat at 1/1 through both would look stuck. */
  index: 0,
  of: 0,
  /** 0-100, or -1 when the server sent no Content-Length. */
  percent: -1,
  error: '',
  /** The capabilities the user asked for, kept so a failure can offer to
   *  resume rather than only apologise. */
  requested: [] as string[],
})

/** Called by whoever starts an install, before it starts, so a retry knows what
 *  to retry. Kept here rather than read back from Go: the request is the user's
 *  intent, and the engine only ever hears the parts still missing. */
export function noteCapabilityRequest(keys: string[]): void {
  capabilities.requested = [...keys]
  capabilities.phase = 'installing'
  capabilities.index = 0
  capabilities.of = keys.length
  capabilities.percent = -1
  capabilities.error = ''
}

/** Try the same set again. Anything that finished stays finished, so this
 *  resumes rather than starting over. */
export async function retryCapabilities(): Promise<void> {
  const keys = capabilities.requested
  if (keys.length === 0) return
  noteCapabilityRequest(keys)
  try {
    await InstallCapabilities(keys)
  } catch (err) {
    capabilities.phase = 'error'
    capabilities.error = String(err)
  }
}

export function dismissCapabilities(): void {
  capabilities.phase = 'idle'
  capabilities.error = ''
}

/** Subscribe to the engine's two events. Returns an unsubscribe, the shape
 *  App.svelte's other listeners already use. */
export function listenCapabilities(): () => void {
  const offProgress = EventsOn(
    'capabilities:progress',
    (p: { id: string; index: number; of: number; percent: number }) => {
      capabilities.phase = 'installing'
      capabilities.index = p.index
      capabilities.of = p.of
      capabilities.percent = p.percent
    },
  )
  const offDone = EventsOn('capabilities:done', (d: { ok: boolean; error?: string }) => {
    if (d?.ok) {
      capabilities.phase = 'done'
      capabilities.percent = 100
      capabilities.error = ''
      return
    }
    capabilities.phase = 'error'
    capabilities.error = d?.error ?? ''
  })
  return () => {
    offProgress()
    offDone()
  }
}
