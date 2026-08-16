// Every chord the app answers to, in one table.
//
// Two things used to be spread across four components and two locale files: the
// test that decides whether a keystroke counts, and the string that tells the
// user what to press. Neither survived the split — Palette advertised
// "Ctrl+Shift+G" for a workbench tab that has been Ctrl+P for months, and every
// handler wrote its own slightly different modifier check. Both now come from
// the same row, so a chord that changes changes everywhere or not at all.
//
// `code` before `key` is the whole point. `key` is the *character the layout
// produces*: on a Thai keyboard the N key reports 'ื', B reports 'ิ', ',' reports
// 'ม'. Matching on it means every shortcut in the app is dead for anyone not
// typing US-English at that moment — which is most of the time, for the person
// who writes this app. `code` is the physical key and never moves. `key` stays
// as a second chance so remapped layouts (Dvorak, Colemak) still reach the
// letter where their user thinks it lives.

export type ShortcutId =
  | 'newSession'
  | 'palette'
  | 'settings'
  | 'toggleSidebar'
  | 'toggleInspector'
  | 'browserTab'
  | 'filesTab'
  | 'pickElement'
  | 'drawOnPage'

type Chord = {
  ctrl?: boolean
  alt?: boolean
  shift?: boolean
  /** Physical key, layout-independent (KeyboardEvent.code). */
  code: string
  /** Latin character the key carries on a US layout, lowercase. */
  key: string
  /** How the key reads in a tooltip. */
  display: string
}

const CHORDS: Record<ShortcutId, Chord> = {
  newSession: { ctrl: true, code: 'KeyN', key: 'n', display: 'N' },
  palette: { ctrl: true, code: 'KeyK', key: 'k', display: 'K' },
  settings: { ctrl: true, code: 'Comma', key: ',', display: ',' },
  toggleSidebar: { ctrl: true, alt: true, code: 'KeyS', key: 's', display: 'S' },
  toggleInspector: { ctrl: true, alt: true, code: 'KeyB', key: 'b', display: 'B' },
  browserTab: { ctrl: true, code: 'KeyT', key: 't', display: 'T' },
  filesTab: { ctrl: true, code: 'KeyP', key: 'p', display: 'P' },
  // Only ever pressed with a browser tab in front, so it shares the S the
  // sidebar uses under a different modifier pair rather than reaching for a
  // letter that means nothing.
  pickElement: { ctrl: true, shift: true, code: 'KeyS', key: 's', display: 'S' },
  drawOnPage: { ctrl: true, shift: true, code: 'KeyD', key: 'd', display: 'D' },
}

/**
 * Does this keystroke mean `id`?
 *
 * Modifiers match exactly — Ctrl+Shift+N is not Ctrl+N. A chord that fires on
 * "Ctrl held, whatever else" is one that fires by accident, and the accident is
 * always someone reaching for a different shortcut.
 */
export function isShortcut(e: KeyboardEvent, id: ShortcutId): boolean {
  const c = CHORDS[id]
  if (e.metaKey) return false
  if (e.ctrlKey !== !!c.ctrl || e.altKey !== !!c.alt || e.shiftKey !== !!c.shift) return false
  return e.code === c.code || e.key.toLowerCase() === c.key
}

/** "Ctrl+Alt+B" — derived from the same row that matches it, never typed twice. */
export function shortcutLabel(id: ShortcutId): string {
  const c = CHORDS[id]
  const parts: string[] = []
  if (c.ctrl) parts.push('Ctrl')
  if (c.alt) parts.push('Alt')
  if (c.shift) parts.push('Shift')
  parts.push(c.display)
  return parts.join('+')
}
