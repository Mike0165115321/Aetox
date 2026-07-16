// Theme state: reactive current theme, persisted to localStorage, defaulting to
// the OS preference on first run. applyTheme() stamps data-theme on <html>, which
// swaps every semantic token in theme.css.

export type ThemeName = 'dark' | 'light'

const STORAGE_KEY = 'aetox-theme'

function preferred(): ThemeName {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'dark' || saved === 'light') return saved
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

export const theme = $state<{ name: ThemeName }>({ name: 'dark' })

export function applyTheme(name: ThemeName): void {
  theme.name = name
  document.documentElement.dataset.theme = name
  localStorage.setItem(STORAGE_KEY, name)
}

export function toggleTheme(): void {
  applyTheme(theme.name === 'dark' ? 'light' : 'dark')
}

/** Call once before mount so there is no unthemed flash. */
export function initTheme(): void {
  applyTheme(preferred())
}
