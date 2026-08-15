// The one folding transition in the app.
//
// Everything else here animates in CSS, and that is still the rule — but CSS
// cannot animate a *removal*: by the time the rule would apply, Svelte has
// already taken the element out of the document. A delegation finishing is
// exactly that case. The card was there, with its beam and its step list, and
// then it was not, in one frame, with nothing saying where it went (owner,
// 15 ส.ค.: "ตอนเอเจนมันทำงานเสร็จ ผมอยากให้ค่อย ๆ พับลง").
//
// Wrapped rather than used directly at each site for two reasons: the duration
// is one decision, not one per card, and `prefers-reduced-motion` has to be
// honoured — svelte/transition does not consult it, and the CSS half of the app
// already does (style.css).
import { slide } from 'svelte/transition'
import { cubicOut } from 'svelte/easing'

// Read per call rather than once at module load: the setting can change while
// the app is open, and a value captured at startup would need a restart to take
// effect. Guarded for the test environment, where matchMedia does not exist.
function reduced(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

/** Folds an element open and shut by its own height. `transition:fold`. */
export function fold(node: Element, { duration = 240 }: { duration?: number } = {}) {
  return slide(node, { duration: reduced() ? 0 : duration, easing: cubicOut })
}
