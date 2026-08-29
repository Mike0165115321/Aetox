// The rooms of the company (COMPANY.md §2), as one list.
//
// Two desks, five pages, one of them still to be built — fixed, not a rendering
// of whatever mode files happen to be on disk. A desk manifest is a *capability* file: a user
// who writes a fourth one gets a fourth desk in the engine, not a fourth
// button in the product's navigation. The nav is the product's shape and the
// owner draws it; `ListModes` is read only for the description each button
// shows, so editing a shipped mode changes its own tooltip and nothing else.
//
// `kind` is what a click means, and the three answers are genuinely different:
// a desk opens a session, a page is a view over data with no session of its
// own, and `soon` is a button with the room named and the work deliberately
// deferred (§7, "อย่าจับปลาหลายมือ").

import type { IconName } from './icons'
import type { TKey } from './i18n.svelte'
import type { ShellName } from './shell.svelte'

export type DeskKind = 'desk' | 'page' | 'soon'

export interface NavEntry {
  /** For a desk, the mode name `sessions.mode` stores. For a page, the view id. */
  id: string
  kind: DeskKind
  labelKey: TKey
  /** Fallback blurb, used until ListModes answers (and for the pages, which
   *  have no manifest to describe them). */
  blurbKey: TKey
  icon: IconName
  /** Which door this room is behind (§86). Assigned by reading the star: the
   *  team and everything wired to it — the files it produces, the routines that
   *  hire it — sit with the assistant, because โค้ด is connected to none of
   *  them. */
  shell: ShellName
  /** For a `desk` room whose conversation is with one of the office's agents
   *  rather than with the assistant: the agent's name.
   *
   *  It exists because ระบบออโตเมชั่น is a chat with a specialist and nothing
   *  else — the alternative was a fourth desk with its own mode manifest, its
   *  own tool list and its own prompt, all of which the agent already has. A
   *  room that opens a chair session is one line here; a room that is a desk is
   *  a new entry in a vocabulary the whole app reads. */
  chair?: string
}

export const NAV: NavEntry[] = [
  { id: 'assistant', kind: 'desk', labelKey: 'desk.assistant', blurbKey: 'desk.assistantBlurb', icon: 'sparkles', shell: 'assistant' },
  // A project groups chats and carries a few files into every session held
  // inside it, so the assistant starts each one already knowing the context.
  // It is a folder for conversations, NOT a fence: the assistant keeps the
  // whole machine either way, which is what separates this from the workshop's
  // projects — those root the sandbox, and that is the point of them.
  { id: 'projects', kind: 'page', labelKey: 'desk.projects', blurbKey: 'desk.projectsBlurb', icon: 'folder', shell: 'assistant' },
  // The roster, and the work the team has taken in. It stays behind the
  // storefront, which is where it has been since it was built.
  //
  // It moved out for about an hour on 2026-08-20, when §158 gave the team a
  // door and this page went with it. The owner sent it back the same day, and
  // the reason is the whole point of the room: *"มันยังเอาไว้คุยกับเอเจนโดยตรงได้"*
  // — walking in to talk to a specialist is something you do beside the
  // assistant, not in a building you have to travel to. What §158 was actually
  // asked for was a home for the new thing, and that is all it should have
  // moved.
  //
  // Renamed on the way back: **เอเจนเฉพาะทาง**, because that is what the page is
  // a list of. The view id stays `office`, the way every desk name in the engine
  // stays what it was while the label on its button changed (COMPANY.md §2).
  { id: 'office', kind: 'page', labelKey: 'desk.office', blurbKey: 'desk.officeBlurb', icon: 'bot', shell: 'assistant' },
  // ระบบออโตเมชั่น was a room here until 2026-08-30, and it went for the reason
  // its own comment gave when it arrived: a room that is a second place
  // answering a question that already has a home does not get to stay. Its
  // button called `newChairSession('automation')` — the identical line the
  // roster's own "แชทกับเอเจนนี้" button calls — so the nav was not a room, it
  // was a shortcut to a chair that lives in เอเจนเฉพาะทาง like every other one.
  //
  // What it bought was not reach but announcement: it was the only place the
  // product said out loud that Aetox does automation. That is a real cost and
  // the owner took it knowingly (30 ส.ค.) — if automation earns a nav row for
  // being important, every agent can make the same argument, and then the
  // roster has no reason to exist.
  //
  // `chair` below stays. It is the one line a room needs to be a chat with a
  // specialist, and nothing uses it today.
  { id: 'artifacts', kind: 'page', labelKey: 'desk.artifacts', blurbKey: 'desk.artifactsBlurb', icon: 'package', shell: 'assistant' },
  { id: 'coding', kind: 'desk', labelKey: 'desk.coding', blurbKey: 'desk.codingBlurb', icon: 'fileCode', shell: 'code' },
  // ---- Aetox ทีม (§158) ----
  // The only room behind the third door, and the whole reason that door exists:
  // a ห้องทำงาน is a run written down — the steps a job goes through, and which
  // เอเจน sits at each one.
  //
  // A `page` rather than a `soon`, and the difference is the door. A `soon`
  // button is a name with the room deliberately deferred, which works when it
  // sits among rooms that open; it is the *only* thing behind this door, so
  // leaving it unclickable would mean walking through a door into nothing. The
  // page opens, and it says plainly that the work is not built yet rather than
  // drawing a list it does not have.
  { id: 'lines', kind: 'page', labelKey: 'desk.lines', blurbKey: 'desk.linesBlurb', icon: 'layoutList', shell: 'team' },
]

/** The rooms behind one door, in order. */
export function navFor(shell: ShellName): NavEntry[] {
  return NAV.filter((n) => n.shell === shell)
}

/** The label for a stored session's `mode`, or '' for the sessions that
 *  predate desks — they were held at no desk, and inventing a badge for them
 *  would be claiming to know something the column deliberately does not say. */
export function deskLabelKey(mode: string | undefined): TKey | '' {
  const entry = NAV.find((n) => n.kind === 'desk' && n.id === mode)
  return entry ? entry.labelKey : ''
}
