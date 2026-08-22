// The three doors (COMPANY.md §2, DECISIONS §86, §158): Aetox ผู้ช่วย,
// Aetox โค้ด and Aetox ทีม.
//
// ทีม holds one room, ห้องทำงาน, and that is the whole reason it exists. The
// roster is NOT behind it — that page went there for an hour on 2026-08-20 and
// the owner sent it back, because walking in to talk to a specialist is
// something you do beside the assistant (§158.9). What the new door is for is
// the new thing, and only the new thing.
//
// A shell is which storefront the window is showing, and it decides one thing
// only: which rooms are drawn. It is *not* a second app — one binary, one
// engine, one DataRoot, one identity — and it is not engine state either, which
// is why it lives here beside the theme rather than in the cockpit: the engine
// never needs to know which door you walked in through. What the engine knows
// is the desk a session was opened at, and that is already `sessions.mode`.
//
// Persisted like the theme, and for the same reason: people use one door for
// days at a time, so reopening the app on another one reads as the app
// forgetting who you are.

import type { IconName } from './icons'
import type { TKey } from './i18n.svelte'

export type ShellName = 'assistant' | 'code' | 'team'

export interface ShellDef {
  name: ShellName
  labelKey: TKey
  blurbKey: TKey
  icon: IconName
  /** The desk a click on this door opens, or '' for a door that opens no
   *  conversation of its own.
   *
   *  ทีม is the first of those (§158.3): you do not talk to the team, you
   *  arrange it and press เริ่ม. Nothing behind it opens a session, so a
   *  "new chat" button there would have to invent someone to be talking to. */
  desk: string
  /** Where a click on this door lands. 'chat' for the doors that open a
   *  conversation; a page id for the one that does not. */
  home: string
  /** Whether the door is on the menu.
   *
   *  ทีม is false, and that is the owner's call on 2026-08-22 looking at the
   *  real thing: *"ไม่แสดงเลยดีกว่านะ แต่โค้ดยังมี"*. ห้องทำงาน is an empty
   *  room, and a door on the menu is a promise that there is something behind
   *  it — §158.9 already refused to make it a `soon` button for the same
   *  reason, from the other end. Not offering the door at all is the version of
   *  that argument that costs the user nothing: there is no button to be
   *  disappointed by.
   *
   *  What it is NOT is a deletion. The door is still a door — routed, filtered,
   *  restored and tested, all of it exercised by doorViews.test.ts through
   *  setShell — so this stays one flag rather than rotting into code nobody
   *  runs. **Shipping ห้องทำงาน is flipping this to true**, and the room it
   *  opens onto is the only thing that has to be built first. */
  offered: boolean
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
//
// ทีม is the exception to the naming rule above and has to be: it has no desk
// to take a name from, so `shell.team` is its own key rather than a room's.
export const SHELLS: ShellDef[] = [
  { name: 'assistant', labelKey: 'desk.assistant', blurbKey: 'shell.assistantBlurb', icon: 'sparkles', desk: 'assistant', home: 'chat', offered: true },
  { name: 'code', labelKey: 'desk.coding', blurbKey: 'shell.codeBlurb', icon: 'fileCode', desk: 'coding', home: 'chat', offered: true },
  { name: 'team', labelKey: 'shell.team', blurbKey: 'shell.teamBlurb', icon: 'layoutList', desk: '', home: 'lines', offered: false },
]

/** The doors the menu draws, in order. */
export function offeredShells(): ShellDef[] {
  return SHELLS.filter((s) => s.offered)
}

const STORAGE_KEY = 'aetox-shell'
// Offered, not merely valid. A door taken off the menu has to stop being
// restorable in the same breath, or the people who were standing behind it when
// it left — this machine included — reopen the app into a door the menu can no
// longer take them out of.
const RESTORABLE_DOORS = new Set(offeredShells().map((s) => s.name))

function preferred(): ShellName {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && RESTORABLE_DOORS.has(saved as ShellName)) return saved as ShellName
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

/** Where a click on a door lands. */
export function homeForShell(name: ShellName): string {
  return (SHELLS.find((s) => s.name === name) ?? SHELLS[0]).home
}

/** Whether a door holds conversations at all. False for ทีม, which opens no
 *  session of its own and hosts no room that does — so its sidebar draws no
 *  history list rather than another door's. */
export function shellHasChats(name: ShellName): boolean {
  return deskForShell(name) !== ''
}

/** Which door a session belongs behind. Legacy sessions ('' — the full desk)
 *  and anything unrecognised answer 'assistant': the storefront is where a
 *  session that predates the split is at home, and where an unknown one is
 *  least surprising (COMPANY.md §2).
 *
 *  ทีม never appears here, and that is not an omission: it holds no desk and no
 *  conversations at all. A chat with an เอเจน is at the specialized desk and
 *  belongs to the storefront, which is where its roster lives (§158.9). */
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
 * at home in the storefront without anyone remembering to add it to a list.
 * */
export function deskFilterFor(name: ShellName): { desks: string[]; exclude: boolean } {
  const workshop = SHELLS.find((s) => s.name === 'code')!.desk
  return name === 'code' ? { desks: [workshop], exclude: false } : { desks: [workshop], exclude: true }
}
