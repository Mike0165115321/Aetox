/** The block that follows the work down a tool timeline.
 *
 * A turn is a column of calls where exactly one of them is usually live, and
 * the block marking it used to appear on the new row the instant the old one
 * finished. Two blocks, one frame apart, in a column: it read as a flicker
 * rather than as the work moving on. This is one element that TRAVELS instead —
 * so the handover is the animation, and no separate transition had to be
 * invented for it (owner, 25 ส.ค.).
 *
 * An action rather than component state because the thing it needs is a
 * measurement, not a value: rows are not a fixed height (narration and thinking
 * rows sit between them, and a label can wrap), so the only honest source for
 * where the live row is, is the live row. It re-measures on any change inside
 * the list and on a resize, both coalesced into one frame.
 *
 * It bows out when it cannot be right, and only then. Two calls running at once
 * is an ordinary state — providers do issue parallel tool calls — and one bar
 * cannot be in two places, so `glide-on` comes off and the per-row block in
 * style.css takes over. Having no live row at all is NOT such a case: the class
 * stays on across a gap, because a row arriving is dressed by whatever the
 * class says at that instant.
 *
 * The two are drawn to the same metrics, so the swap is invisible. The same
 * fallback is what draws a sub-agent's steps inside its card, which this action
 * never touches.
 */
export function toolGlide(node: HTMLElement) {
  const bar = document.createElement('div')
  bar.className = 'tool-glide'
  // Appended, not prepended: `.tool-step:not(:first-child)` draws the rail
  // between rows, and a foreign FIRST child would make the first real row think
  // it had something above it. Absolutely positioned, so last in the DOM costs
  // nothing visually.
  node.append(bar)

  let frame = 0
  // Whether the bar is on screen right now. It is the difference between a
  // handover and an arrival, and those are not the same move — see place().
  let shown = false
  const place = () => {
    frame = 0
    const running = node.querySelectorAll<HTMLElement>('.tool-step.run')
    // Off only for the one case the bar genuinely cannot serve: two rows live
    // at once. With NONE live it stays ON, which looks like a class describing
    // a state that is not true and is in fact the whole fix (owner, 26 ส.ค.:
    // "ตอนมันรัน Tool ใหม่มีสีขาวๆ แว๊บมา"). A row is born wearing whatever this
    // class says at the instant it is inserted, and a gap between two calls is
    // the ordinary state — the model narrates between them. With the class off
    // across that gap, the next row arrives wearing the per-row block at full
    // strength, the following frame takes it away again, and the bar then ramps
    // the same block back up over --beam-in. Full, gone, fading in: that is the
    // flash, and it fired on every new row after a pause.
    node.classList.toggle('glide-on', running.length <= 1)
    const row = running[0]
    if (running.length !== 1 || !row) {
      bar.style.opacity = '0'
      shown = false
      return
    }
    // The 1px is the block's vertical breath, and it matches the -1px inset the
    // per-row version carries. Two numbers that must agree; they are both here
    // and in one rule in style.css.
    const y = row.offsetTop - 1
    const h = row.offsetHeight + 2
    if (shown) {
      // A handover: one live row to the next, and the travel between them IS
      // the animation this file exists for.
      bar.style.transform = `translateY(${y}px)`
      bar.style.height = `${h}px`
      return
    }
    // An arrival, which is a different act. The bar is parked wherever it was
    // last needed — a row that finished several seconds and a paragraph of
    // narration ago — so transitioning from there would slide a lit rectangle
    // down through that narration, and on a turn's first call would grow it out
    // of nothing at the top of the list. Neither is a handover; there is
    // nothing to hand over from. Put it where it belongs with the transition
    // suppressed and let the fade carry the arrival on its own.
    bar.style.transition = 'none'
    bar.style.transform = `translateY(${y}px)`
    bar.style.height = `${h}px`
    void bar.offsetHeight // read forces the layout, so the line below starts from here
    bar.style.transition = ''
    bar.style.opacity = '1'
    shown = true
  }
  const schedule = () => { if (!frame) frame = requestAnimationFrame(place) }

  // Attributes filtered to `class` on purpose: `place` writes bar.style, and a
  // watcher that also fired on style changes would answer its own writes.
  const rows = new MutationObserver(schedule)
  rows.observe(node, { childList: true, subtree: true, attributes: true, attributeFilter: ['class'] })
  const size = new ResizeObserver(schedule)
  size.observe(node)
  // The first measurement is synchronous, unlike every one after it: a timeline
  // can mount with a call already running (reopening a chat mid-turn), and a
  // frame of the per-row block before the bar takes over is a visible flinch on
  // exactly the row the user came back to look at.
  place()

  return {
    destroy() {
      rows.disconnect()
      size.disconnect()
      if (frame) cancelAnimationFrame(frame)
      bar.remove()
    },
  }
}
