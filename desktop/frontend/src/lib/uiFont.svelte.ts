// App-wide UI font family — selectable from Settings › Appearance, applied by
// overriding the --sans token. 'aetox' (bundled Inter + Noto Sans Thai) is the
// default; 'system' restores the OS stack (Segoe UI / Leelawadee UI).

const STORAGE_KEY = 'aetox-ui-font'

export const UI_FONTS = [
  { value: 'aetox', labelKey: 'settings.uiFont.aetox', stack: '"Inter Variable","Noto Sans Thai Variable","Segoe UI","Leelawadee UI",system-ui,sans-serif' },
  { value: 'system', labelKey: 'settings.uiFont.system', stack: 'system-ui,"Segoe UI","Leelawadee UI",Roboto,"Helvetica Neue",Arial,sans-serif' },
] as const

export type UiFontName = (typeof UI_FONTS)[number]['value']

export const uiFont = $state<{ name: UiFontName }>({ name: 'aetox' })

export function applyUiFont(name: UiFontName): void {
  const font = UI_FONTS.find((f) => f.value === name) ?? UI_FONTS[0]
  uiFont.name = font.value
  document.documentElement.style.setProperty('--sans', font.stack)
  localStorage.setItem(STORAGE_KEY, font.value)
}

/** Call once before mount so the UI doesn't flash in the fallback font. */
export function initUiFont(): void {
  const saved = localStorage.getItem(STORAGE_KEY) as UiFontName | null
  applyUiFont(saved && UI_FONTS.some((f) => f.value === saved) ? saved : 'aetox')
}
