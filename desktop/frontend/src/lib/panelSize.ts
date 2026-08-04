// How wide a side panel is allowed to get.
//
// The rule itself is old and still right: the grid is
// `sidebar | handle | main | handle | inspector`, `.main` has a 360px floor,
// and `.app` has overflow-x:auto — so a panel dragged past the space that
// actually exists does not error, it pushes the composer off-screen behind a
// horizontal scrollbar. The cap is what keeps everything on screen.
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
