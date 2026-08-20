import { ICONS, type IconName } from './icons'

// The mark a worker wears, and the one rule about what it falls back to.
//
// A profile may name its own icon (`icon:` in its file, offered from the
// picker in Settings). Most do not, and what they got instead was `bot`,
// hardcoded at every draw site — which handed an เอเจน with no icon of its own
// the ซับเอเจน mark, on the เอเจน page, next to the sidebar item that wears
// `userRound` for exactly that distinction.
//
// So the fallback is derived from the kind, from the two marks the app already
// owns: `userRound` is เอเจน (the team page, the "used N agents" toggle, and
// the identity page carries a comment about not taking it because "the เอเจน
// page below owns that") and `bot` is ซับเอเจน (its settings page, its toggle).
// Nothing here invents a third.
//
// isAgent is asked of the caller rather than derived from a `desk:` field,
// because the pages that draw these already know the answer from ListChairs and
// a second derivation is a second answer.
//
// A name this build does not have is treated as "none" rather than trusted: a
// profile is written by hand, by somebody who cannot see this list, and an
// unknown name would draw an empty square.
export function workerFace(icon: string | undefined, isAgent: boolean): IconName {
  if (icon && icon in ICONS) return icon as IconName
  return isAgent ? 'userRound' : 'bot'
}
