// Whose memory a proposal is for, in words.
//
// A scope is a wire value with three shapes — '' for the main assistant,
// `mode:<desk>` for a desk, `project:<name>-<hash>` for one folder on this
// machine, and a bare name for a delegate (internal/learned). Two surfaces show
// it now, the review list in Settings and the card under the answer that
// proposed it, and before this they both printed the raw string: a user
// deciding whether to keep a line was shown "mode:coding" and, once projects
// gained memory, "project:Aetox-1a2b3c4d".
//
// One function rather than one per surface, because the two must agree — the
// same proposal is looked at in both places, and a person who approved
// "โปรเจกต์ Aetox" in the chat should find that same name in Settings.

import { t } from './i18n.svelte'
import { deskLabelKey } from './desks'

/** The 8 hex characters config.ProjectKey appends so two folders named "app"
 *  cannot collide. It is identity, not information: the folder's own name is
 *  what a person recognises, so the suffix is dropped for display only. */
const KEY_HASH = /-[0-9a-f]{8}$/

export function scopeLabel(scope: string): string {
  const s = (scope ?? '').trim()
  if (!s) return t('settings.learningScopeMain')

  const desk = s.startsWith('mode:') ? s.slice(5) : ''
  if (desk) {
    // A desk the product actually draws gets its own name in the user's
    // language; anything else is a desk manifest somebody added, and its own
    // name is the only name it has.
    const key = deskLabelKey(desk)
    return t('settings.learningScopeDesk', { name: key ? t(key) : desk })
  }

  const project = s.startsWith('project:') ? s.slice(8) : ''
  if (project) {
    return t('settings.learningScopeProject', { name: project.replace(KEY_HASH, '') })
  }

  // A delegate. Its name is what the user called it, so it is already the label.
  return s
}
