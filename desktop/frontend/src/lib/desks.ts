// The rooms of the company (COMPANY.md §2), as one list.
//
// Two desks, one office, three pages — fixed, not a rendering of whatever mode
// files happen to be on disk. A desk manifest is a *capability* file: a user
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
   *  office and everything wired to it — the files it produces, the routines
   *  that hire it — sit with the assistant, because โค้ด is connected to none
   *  of them. */
  shell: ShellName
}

export const NAV: NavEntry[] = [
  { id: 'assistant', kind: 'desk', labelKey: 'desk.assistant', blurbKey: 'desk.assistantBlurb', icon: 'sparkles', shell: 'assistant' },
  // A project groups chats and carries a few files into every session held
  // inside it, so the assistant starts each one already knowing the context.
  // It is a folder for conversations, NOT a fence: the assistant keeps the
  // whole machine either way, which is what separates this from the workshop's
  // projects — those root the sandbox, and that is the point of them.
  { id: 'projects', kind: 'soon', labelKey: 'desk.projects', blurbKey: 'desk.projectsBlurb', icon: 'folder', shell: 'assistant' },
  { id: 'office', kind: 'page', labelKey: 'desk.office', blurbKey: 'desk.officeBlurb', icon: 'bot', shell: 'assistant' },
  { id: 'auto', kind: 'soon', labelKey: 'desk.auto', blurbKey: 'desk.autoBlurb', icon: 'timer', shell: 'assistant' },
  { id: 'artifacts', kind: 'page', labelKey: 'desk.artifacts', blurbKey: 'desk.artifactsBlurb', icon: 'package', shell: 'assistant' },
  { id: 'coding', kind: 'desk', labelKey: 'desk.coding', blurbKey: 'desk.codingBlurb', icon: 'fileCode', shell: 'code' },
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
