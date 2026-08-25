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

/** Opens an attachment from its top edge and shuts it the same way.
 * `transition:unroll`.
 *
 * The height IS the reveal: the card clips its own contents, so the picture and
 * the name arrive at full size and full sharpness from the first frame. That is
 * the whole reason this is not a scale — a card scaled up from 55% renders its
 * 11px filename at fractional sizes on the way, and the eye reads that as a
 * blurry picture rather than as movement (owner rejected exactly that, 25 ส.ค.,
 * and picked this one: "ม่านเปิดลง").
 *
 * What `slide` could not do is the composer's `gap:10px`. A gap belongs to the
 * container and appears the instant the child exists, so the whole box jumped
 * 10px before the card had drawn a single pixel — the jolt was still there no
 * matter how gently the card itself arrived. The negative margin cancels it and
 * unwinds with the height, so the space and the card are one movement rather
 * than two events a frame apart.
 */
export function unroll(
  node: Element,
  // 560ms is the owner's, picked at 2x on the live comparison and not a typo:
  // this is the slowest movement in the app on purpose. Every other transition
  // here reports something the app did, and gets out of the way; this one is
  // the app acknowledging something the user did, and it is the only frame in
  // which they can check that the right picture went in.
  { duration = 560, gap = 10 }: { duration?: number; gap?: number } = {},
) {
  // Measured before the transition starts, which for an outro is while the card
  // is still at full size — the one moment its real height can be read.
  const height = (node as HTMLElement).offsetHeight
  return {
    duration: reduced() ? 0 : duration,
    easing: cubicOut,
    css: (t: number) => `overflow:hidden; height:${t * height}px; margin-bottom:${(t - 1) * gap}px;`,
  }
}
