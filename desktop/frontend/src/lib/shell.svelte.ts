// The two doors (COMPANY.md §2, DECISIONS §86): Aetox ผู้ช่วย and Aetox โค้ด.
//
// A shell is which storefront the window is showing, and it decides one thing
// only: which of the five rooms are drawn. It is *not* a second app — one
// binary, one engine, one DataRoot, one identity — and it is not engine state
// either, which is why it lives here beside the theme rather than in the
// cockpit: the engine never needs to know which door you walked in through.
// What the engine knows is the desk a session was opened at, and that is
// already `sessions.mode`.
//
// Persisted like the theme, and for the same reason: people use one door for
// days at a time, so reopening the app on the other one reads as the app
// forgetting who you are.

import type { IconName } from './icons'
import type { TKey } from './i18n.svelte'

export type ShellName = 'assistant' | 'code'

export interface ShellDef {
  name: ShellName
  labelKey: TKey
  blurbKey: TKey
  icon: IconName
  /** The desk a click on this door opens. */
  desk: string
}

// A door takes its *name* from the desk behind it — same key the room button
// uses, because it is the same word and two spellings of one name is how they
// drift apart.
//
// Its *line* is not the desk's description, and the difference is not
// pedantry. A manifest description answers "what can this desk do", is read in
// Settings beside a tool list, and is as long as being accurate requires. A
// door sign answers "which half of the product is this" for someone holding a
// mouse, and every word past about five costs more than it earns. Binding the
// menu to the manifest was tried and read as a wall of text; these are three
// words each, on purpose, and they are not a duplicate of anything.
export const SHELLS: ShellDef[] = [
  { name: 'assistant', labelKey: 'desk.assistant', blurbKey: 'shell.assistantBlurb', icon: 'sparkles', desk: 'assistant' },
  { name: 'code', labelKey: 'desk.coding', blurbKey: 'shell.codeBlurb', icon: 'fileCode', desk: 'coding' },
]

const STORAGE_KEY = 'aetox-shell'
const VALID = new Set(SHELLS.map((s) => s.name))

function preferred(): ShellName {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && VALID.has(saved as ShellName)) return saved as ShellName
  } catch {
    /* storage unavailable — the default door is the honest fallback */
  }
  // The storefront, not the workshop: a first run should open on the door that
  // needs no explanation.
  return 'assistant'
}

export const shell = $state<{ name: ShellName }>({ name: 'assistant' })

/** Read the remembered door. Call once before mount, like initTheme. */
export function initShell(): void {
  shell.name = preferred()
}

/** Remember the door. Opening a session at the right desk is the caller's job
 *  (stores/cockpit.svelte's switchShell) — this module knows nothing about
 *  sessions, which is what keeps it a UI preference rather than a second
 *  source of truth about what the engine is doing. */
export function setShell(name: ShellName): void {
  shell.name = name
  try {
    localStorage.setItem(STORAGE_KEY, name)
  } catch {
    /* not remembering is survivable; failing to switch is not */
  }
}

/** The desk behind a door — where "a new chat" lands for someone standing at
 *  that door, whichever of its rooms they happen to be in. The inverse of
 *  shellForDesk, and read off the same table so the pair cannot disagree. */
export function deskForShell(name: ShellName): string {
  return (SHELLS.find((s) => s.name === name) ?? SHELLS[0]).desk
}

/** Which door a desk belongs behind. Legacy sessions ('' — the full desk) and
 *  anything unrecognised answer 'assistant': the storefront is where a session
 *  that predates the split is at home, and where an unknown one is least
 *  surprising (COMPANY.md §2). */
export function shellForDesk(desk: string | undefined): ShellName {
  return desk === 'coding' ? 'code' : 'assistant'
}

/** The same rule as a database filter: which desks a door's history covers.
 *
 * Sent to the engine (ListSessionsForDoor) so the scoping happens in SQL, where
 * LIMIT is applied after WHERE. Filtering a fetched page here instead meant 200
 * rows were taken across both doors and half thrown away, so a run of coding
 * sessions could hand the storefront an empty list while its history sat in the
 * table — a defect that only shows up once the app has been used enough.
 *
 * The storefront asks for everything *except* the workshop's desk rather than
 * naming its own. That is what keeps shellForDesk's promise on this side too: a
 * desk added later, a user-written one, or a legacy session held at no desk is
 * at home in the storefront without anyone remembering to add it to a list. */
export function deskFilterFor(name: ShellName): { desks: string[]; exclude: boolean } {
  const workshop = SHELLS.find((s) => s.name === 'code')!.desk
  return name === 'code' ? { desks: [workshop], exclude: false } : { desks: [workshop], exclude: true }
}
