// Theme state: reactive current theme, persisted to localStorage, defaulting to
// the OS preference on first run. applyTheme() stamps data-theme on <html>, which
// swaps every semantic token in theme.css. THEMES is the registry — add a theme by
// adding a :root[data-theme="x"] block in theme.css and a row here.

export type ThemeName =
  | 'catppuccin-mocha' | 'catppuccin-latte'
  | 'nord' | 'dracula'
  | 'rose-pine' | 'rose-pine-dawn'
  | 'gruvbox-dark' | 'gruvbox-light'
  | 'tokyo-night'

export const THEMES: { value: ThemeName; label: string }[] = [
  { value: 'catppuccin-mocha', label: 'Catppuccin Mocha' },
  { value: 'catppuccin-latte', label: 'Catppuccin Latte' },
  { value: 'nord', label: 'Nord' },
  { value: 'dracula', label: 'Dracula' },
  { value: 'rose-pine', label: 'Rosé Pine' },
  { value: 'rose-pine-dawn', label: 'Rosé Pine Dawn' },
  { value: 'gruvbox-dark', label: 'Gruvbox Dark' },
  { value: 'gruvbox-light', label: 'Gruvbox Light' },
  { value: 'tokyo-night', label: 'Tokyo Night' },
]

// Themes dark enough that the quick topbar toggle should offer the light default next.
export const DARK_FAMILY = new Set<ThemeName>([
  'catppuccin-mocha', 'nord', 'dracula', 'rose-pine', 'gruvbox-dark', 'tokyo-night',
])

const DEFAULT_DARK: ThemeName = 'catppuccin-mocha'
const DEFAULT_LIGHT: ThemeName = 'catppuccin-latte'

const STORAGE_KEY = 'aetox-theme'
const VALID_NAMES = new Set(THEMES.map((t) => t.value))

function preferred(): ThemeName {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved && VALID_NAMES.has(saved as ThemeName)) return saved as ThemeName
  return window.matchMedia('(prefers-color-scheme: light)').matches ? DEFAULT_LIGHT : DEFAULT_DARK
}

export const theme = $state<{ name: ThemeName }>({ name: DEFAULT_DARK })

export function applyTheme(name: ThemeName): void {
  theme.name = name
  document.documentElement.dataset.theme = name
  localStorage.setItem(STORAGE_KEY, name)
}

/** Quick topbar icon toggle — flips between the default dark/light pair, regardless of which named theme is active. */
export function toggleTheme(): void {
  applyTheme(DARK_FAMILY.has(theme.name) ? DEFAULT_LIGHT : DEFAULT_DARK)
}

/** Call once before mount so there is no unthemed flash. */
export function initTheme(): void {
  applyTheme(preferred())
}
