// Which fence tags may carry a Run button on THIS machine.
//
// The renderer used to hold the list itself, which could only ever say what is
// runnable in principle — a `python` block is runnable on a computer with Python
// on it and is a picture of a program on one without, and the renderer has no
// way to tell those apart. So the engine is asked instead (App.RunnableLanguages,
// backed by internal/runlang), once, at startup.
//
// Once and not per render: the answer is what is installed on the machine, which
// does not change while a window is open, and asking across the bridge for every
// fenced block in every message would be a round trip per code block per redraw.
// A machine that gains Python mid-session gets its Run buttons on the next
// launch, which is the same deal every other "what is on this computer" answer
// in the app makes.
//
// A failure leaves the table empty, and empty means no Run buttons anywhere.
// That is the safe direction and the honest one: if the engine cannot say what
// this machine can run, the app must not guess — the cost of guessing wrong is a
// button that does nothing, on the one control in the chat with a real side
// effect.

import { RunnableLanguages } from '../../wailsjs/go/main/App'
import { setRunnableLanguages } from './markdown'

export async function initRunnableLanguages(): Promise<void> {
  try {
    setRunnableLanguages((await RunnableLanguages()) ?? {})
  } catch {
    setRunnableLanguages({})
  }
}
