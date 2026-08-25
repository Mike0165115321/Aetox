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
 * It bows out when it cannot be right. Two calls running at once is an ordinary
 * state — providers do issue parallel tool calls — and one bar cannot be in two
 * places, so `glide-on` comes off and the per-row block in style.css takes over.
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
  const place = () => {
    frame = 0
    const running = node.querySelectorAll<HTMLElement>('.tool-step.run')
    node.classList.toggle('glide-on', running.length === 1)
    const row = running[0]
    if (running.length !== 1 || !row) {
      bar.style.opacity = '0'
      return
    }
    // The 1px is the block's vertical breath, and it matches the -1px inset the
    // per-row version carries. Two numbers that must agree; they are both here
    // and in one rule in style.css.
    bar.style.opacity = '1'
    bar.style.transform = `translateY(${row.offsetTop - 1}px)`
    bar.style.height = `${row.offsetHeight + 2}px`
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
