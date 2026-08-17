// The three colours that stand for a theme in a picker: its ground, the panel
// that sits on it, and the accent that marks what is live.
//
// Read out of the stylesheet rather than written down again. theme.css is the
// only place a theme's palette is decided (see theme.svelte.ts), and a second
// table of hexes here would be a copy that drifts the first time a theme is
// re-tuned — the picker would then show colours the app does not use, which is
// worse than no picker at all.
//
// How: stamp each theme onto <html> in turn and ask the browser what the tokens
// resolved to. All of it happens inside one synchronous call, so no frame is
// painted mid-loop and nothing flashes; the original theme is put back before
// returning. Cheap enough to run once at mount for all fourteen.

import { THEMES } from './theme.svelte'

export type Swatch = { bg: string; panel: string; accent: string }

export function readThemeSwatches(): Record<string, Swatch> {
  const out: Record<string, Swatch> = {}
  const root = document.documentElement
  const was = root.dataset.theme
  const style = getComputedStyle(root)
  const token = (name: string) => style.getPropertyValue(name).trim()
  for (const th of THEMES) {
    root.dataset.theme = th.value
    out[th.value] = {
      bg: token('--surface-app'),
      panel: token('--surface-raised'),
      accent: token('--accent'),
    }
  }
  if (was === undefined) delete root.dataset.theme
  else root.dataset.theme = was
  return out
}
