// How wide a side panel is allowed to get.
//
// The rule itself is old and still right: the grid is
// `sidebar | handle | main | handle | inspector` and `.main` has a 360px floor,
// so a panel dragged past the space that actually exists does not error — the
// grid simply grows wider than the window. The cap is what keeps everything on
// screen.
//
// `.app` is `overflow:clip` now rather than the `overflow-x:auto` this comment
// used to describe, so the overflow no longer arrives as a scrollbar; it
// arrives as content sitting outside the frame, and the only way back is for
// the width to be right. Which is also why this has to be re-applied when the
// WINDOW changes size and not only when the handle is dragged (App.svelte):
// a width that fitted yesterday's window is not a width that fits today's.
//
// It lives here rather than inside App.svelte because it got the arithmetic
// wrong in a way nothing could see: with the sidebar collapsed it still
// subtracted the sidebar's remembered width, and still counted two resize
// handles when one of them was display:none. On a 1417px window that reserved
// 286px for a panel that was not on screen, and the workbench simply stopped
// short with nothing to explain why. A number that silently steals space is
// exactly the kind that needs a test.

/** Layout constants, mirroring style.css's `.app` grid. */
export const MAIN_FLOOR = 360 // .main's minmax(360px, 1fr)
export const HANDLE_PX = 6 // one .resize-handle

export type PanelFit = {
  /** Viewport width the grid has to fit inside (window.innerWidth). */
  viewport: number
  /** This panel's own floor. */
  min: number
  /** The OTHER side panel's width right now — 0 when it is collapsed, because
   *  `.app.sidebar-collapsed` / `.app.inspector-collapsed` give it a 0px
   *  column and hide its handle. */
  otherWidth: number
  /** Whether that other panel is on screen at all; its handle exists only if
   *  it does. */
  otherVisible: boolean
}

/** The widest this panel may be drawn without the grid outgrowing the window. */
export function maxPanelWidth(fit: PanelFit): number {
  // The dragged panel always has its own handle; the other's is there only
  // while that panel is.
  const handles = HANDLE_PX + (fit.otherVisible ? HANDLE_PX : 0)
  return Math.max(fit.min, fit.viewport - fit.otherWidth - MAIN_FLOOR - handles)
}

/** A requested width, held between this panel's floor and that ceiling. */
export function clampPanelWidth(px: number, fit: PanelFit): number {
  return Math.min(Math.max(fit.min, px), maxPanelWidth(fit))
}

/** One side panel, as the window-fit needs to see it. */
export type PanelState = {
  /** What it is drawn at right now. */
  width: number
  /** Its own floor. */
  min: number
  /** Whether it is on screen at all; a collapsed one takes no room and no handle. */
  visible: boolean
}

/**
 * The widths these panels may keep now that the window is this wide, one per
 * panel in the order given.
 *
 * The cap above answers "how far may this drag go", which is asked while a
 * pointer is moving. This answers the other half nothing was asking: the window
 * itself changes size, and a width that fitted yesterday's window is not a
 * width that fits today's. Drag the workbench wide, make the window smaller,
 * and the grid is simply wider than the frame from then on — silently, because
 * `.app` clips rather than scrolls (style.css).
 *
 * Nothing moves while the grid still fits: a resize is not a reason to
 * renegotiate a width the user chose.
 *
 * When it does not fit, the overflow comes out of each panel in proportion to
 * the slack it has above its own floor. NOT one first and the remainder from
 * the other — App.svelte carries a comment about exactly that trap, from the
 * un-collapse path: whichever is asked second is clamped against the width the
 * first is still holding, so one lands on its floor and the other keeps
 * everything. Proportional shrinking has no first.
 */
export function fitPanelsToWindow(viewport: number, panels: PanelState[]): number[] {
  const widths = panels.map((p) => p.width)
  const handles = panels.filter((p) => p.visible).length * HANDLE_PX
  const room = viewport - MAIN_FLOOR - handles
  const shown = panels.filter((p) => p.visible)
  const total = shown.reduce((sum, p) => sum + p.width, 0)
  if (total <= room) return widths

  const slack = panels.map((p) => (p.visible ? Math.max(0, p.width - p.min) : 0))
  const slackTotal = slack.reduce((a, b) => a + b, 0)
  // Every visible panel already sits on its floor and the window is still too
  // small. There is nothing left to give: the panels stay put and the grid
  // overflows, which is honest — the alternative is drawing them below their
  // own minimums, where a panel stops being usable without saying so.
  if (slackTotal === 0) return widths

  const over = total - room
  return panels.map((p, i) =>
    p.visible ? Math.max(p.min, Math.round(p.width - (over * slack[i]) / slackTotal)) : p.width,
  )
}
