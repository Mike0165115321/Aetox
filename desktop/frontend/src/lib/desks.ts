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
  // Workflows, not a clock — which is why the icon is `gitBranch` and not the
  // `timer` it carried while this room still meant "scheduled work". Aetox has
  // no cloud, so a schedule only fires while the machine is awake; the owner
  // withdrew it rather than ship a promise that quietly depends on the laptop
  // never closing (2026-08-09).
  //
  // It was a page for one day, drawing cards for the engines you could connect.
  // That was the wrong shape: connecting an account is register work and the
  // register already does it (ตั้งค่า → การเชื่อมต่อ), so the room was a second
  // place answering a question that already had a home — while the thing the
  // user actually comes here to do, describe an automation and have it built,
  // had nowhere at all.
  //
  // So it is a conversation, and the person on the other side is the automation
  // specialist rather than the assistant (owner, 10 ส.ค.). Not a fourth desk:
  // a desk would need its own manifest, tool list and prompt, all of which the
  // agent already carries — including the one thing a desk could not, which is
  // knowing how n8n's graphs actually work.
  { id: 'auto', kind: 'desk', labelKey: 'desk.auto', blurbKey: 'desk.autoBlurb', icon: 'gitBranch', shell: 'assistant', chair: 'automation' },
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
