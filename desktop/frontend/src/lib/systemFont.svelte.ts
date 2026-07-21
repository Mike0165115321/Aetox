// Whole-system scale (nav, buttons, topbar, settings, icons, paddings —
// everything except the file editor and chat, which keep their own
// independent size on top of this — see editorFont.svelte.ts / chatFont.svelte.ts).
//
// Was deliberately locked with no UI control; reopened after repeated
// back-and-forth tuning requests made self-service the better tradeoff.
//
// Implemented as CSS `zoom` on <body> rather than a font-size var: style.css
// uses fixed px everywhere (not em/rem), so a font-size var would only
// cascade to the handful of elements that don't set their own size — it
// would miss almost everything. `zoom` scales the whole rendered subtree
// (text AND icon/padding boxes) as one unit, which is also why fixed-size
// icon glyphs don't overflow their boxes at this setting the way per-rule
// font-size scaling did. Aetox only ships on WebView2 (Chromium), so the
// non-standard-but-Chromium-supported `zoom` property is safe here.
const STORAGE_KEY = 'aetox-system-zoom'
const DEFAULT_ZOOM = 1
const MIN_ZOOM = 0.8
const MAX_ZOOM = 1.3

/** body's actual font-size in style.css — the px zoom=1 represents. Settings
 * shows/edits this as a real px number rather than an abstract percentage. */
export const SYSTEM_BASE_PX = 15.5

export const systemZoom = $state<{ value: number }>({ value: DEFAULT_ZOOM })

export function applySystemZoom(value: number): void {
  const clamped = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value))
  systemZoom.value = clamped
  document.body.style.zoom = String(clamped)
  localStorage.setItem(STORAGE_KEY, String(clamped))
}

/** Call once before mount so the UI doesn't flash at the fallback zoom. */
export function initSystemZoom(): void {
  const saved = parseFloat(localStorage.getItem(STORAGE_KEY) ?? '')
  applySystemZoom(Number.isFinite(saved) ? saved : DEFAULT_ZOOM)
}
