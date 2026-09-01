// What "the first time this app was opened" means, in one place.
//
// Two things read it: the wizard (Onboarding.svelte), which must know whether
// this machine has been through it, and the button in Settings that puts the
// window back into that state so the first run can actually be looked at
// instead of remembered. Splitting those into two lists is how they drift —
// the flag gets cleared, the wizard sees a configured provider and marks
// itself done again, and the button silently does nothing.

import { activeViewStorageKey, SETTINGS_SECTION_KEY } from './stores/cockpit.svelte'

/** Set once the wizard has been finished or skipped. */
export const DONE_KEY = 'aetox.onboarded'

/** One-shot: the next window load shows the wizard whatever else is true —
 * including the has-a-working-key shortcut, which is exactly the condition a
 * developer's own machine is always in and the reason "clear the flag" was
 * never enough to see this screen again. Consumed on read. */
export const REPLAY_KEY = 'aetox.onboarding.replay'

// Every browser-side preference a fresh install does not have yet. Listed by
// name rather than wiped by prefix on purpose: `aetox-composer-draft` holds a
// message the user typed and has not sent, `aetox-workbench:<session>` holds
// the tabs on their desk, and neither is a preference — a reset that ate them
// would be destroying work to test a screen.
//
// Nothing here reaches the engine. Keys, provider choice, approval mode,
// sessions and memory live in Go and on disk (config.Load, the SQLite file),
// and this button is not a factory reset — it replays the first *screen*.
const PREFERENCE_KEYS = [
  DONE_KEY,
  'aetox-theme',
  'aetox-locale',
  'aetox-shell',
  'aetox-ui-font',
  'aetox-type-scale',
  'aetox-system-zoom',
  'aetox-chat-font-size',
  'aetox-editor-font-size',
  'aetox-tree-font-size',
  'aetox-editor-theme-choice',
  'aetox-editor-theme-json',
  // How much of the project the code map draws, and how much of it it names.
  'aetox-repomap-display',
  'sidebarCollapsed',
  'inspectorCollapsed',
  'sidebarWidth',
  'inspectorWidth',
  'defaultShell',
  // The seed the window paints before the engine answers. Left behind, the
  // "first" run opens already knowing which model it is on.
  'lastModelInfo',
]

// Which room the window is standing in survives an F5 on purpose (see
// stores/cockpit's setActiveView), and this reset arrives by way of exactly
// that: the button is in Settings, so without clearing these the "first" run
// would come back with the Settings page open under the welcome card.
//
// Imported rather than respelled: both are cockpit's keys, and a second
// spelling of a storage key is a bug that looks like nothing happening.
const SESSION_KEYS = [activeViewStorageKey, SETTINGS_SECTION_KEY]

/** Clear the remembered UI state and arm the wizard. Returns nothing and
 *  reloads nothing — the caller decides when the window restarts, because a
 *  reload inside a click handler is impossible to test. */
export function armFirstRunReplay(): void {
  try {
    for (const key of PREFERENCE_KEYS) localStorage.removeItem(key)
    localStorage.setItem(REPLAY_KEY, '1')
  } catch {
    /* storage unavailable — nothing was remembered to begin with */
  }
  try {
    for (const key of SESSION_KEYS) sessionStorage.removeItem(key)
  } catch {
    /* same */
  }
}

/** True once, for the load that follows armFirstRunReplay(). Reading it spends
 *  it, so a later reload is an ordinary one. */
export function takeFirstRunReplay(): boolean {
  try {
    const armed = localStorage.getItem(REPLAY_KEY) === '1'
    if (armed) localStorage.removeItem(REPLAY_KEY)
    return armed
  } catch {
    return false
  }
}
