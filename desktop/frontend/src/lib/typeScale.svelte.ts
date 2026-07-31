// Text-only size scale. Multiplies every --fs-* step in styles/type.css by one
// factor, written to the root element so the whole app follows.
//
// Deliberately separate from systemZoom (systemFont.svelte.ts), which scales
// the entire rendered UI — text, padding, icon boxes — as one unit. The two
// answer different complaints: "the letters are too small to read" wants this,
// "everything is too small on this monitor" wants zoom. They compose, so the
// px readout the zoom control shows has to account for this factor too.

// `as const` so labelKey stays a literal and t() type-checks it as a real
// message key — same shape as UI_FONTS in uiFont.svelte.ts.
export const TYPE_SCALES = [
  { value: 'compact', labelKey: 'settings.typeScaleCompact', scale: 0.92 },
  { value: 'default', labelKey: 'settings.typeScaleDefault', scale: 1 },
  { value: 'comfortable', labelKey: 'settings.typeScaleComfortable', scale: 1.08 },
  { value: 'large', labelKey: 'settings.typeScaleLarge', scale: 1.18 },
] as const

export type TypeScaleName = (typeof TYPE_SCALES)[number]['value']

const STORAGE_KEY = 'aetox-type-scale'
const DEFAULT_NAME: TypeScaleName = 'default'
const BY_NAME = new Map(TYPE_SCALES.map((s) => [s.value, s]))

export const typeScale = $state<{ name: TypeScaleName; scale: number }>({
  name: DEFAULT_NAME,
  scale: 1,
})

export function applyTypeScale(name: TypeScaleName): void {
  const step = BY_NAME.get(name) ?? BY_NAME.get(DEFAULT_NAME)!
  typeScale.name = step.value
  typeScale.scale = step.scale
  document.documentElement.style.setProperty('--fs-scale', String(step.scale))
  try {
    localStorage.setItem(STORAGE_KEY, step.value)
  } catch {
    // storage unavailable — the scale still applies for this run
  }
}

/** Call once before mount so text doesn't reflow one frame after paint. */
export function initTypeScale(): void {
  let saved: string | null = null
  try {
    saved = localStorage.getItem(STORAGE_KEY)
  } catch {
    // storage unavailable — fall through to the default
  }
  applyTypeScale(BY_NAME.has(saved as TypeScaleName) ? (saved as TypeScaleName) : DEFAULT_NAME)
}
