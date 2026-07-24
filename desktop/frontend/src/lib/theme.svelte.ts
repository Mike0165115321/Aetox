// Theme state: reactive current theme, persisted to localStorage, defaulting to
// the OS preference on first run. applyTheme() stamps data-theme on <html>, which
// swaps every semantic token in theme.css. THEMES is the registry — add a theme by
// adding a :root[data-theme="x"] block in theme.css and a row here.

export type ThemeName =
  | 'aetox'
  | 'catppuccin-mocha' | 'catppuccin-latte'
  | 'nord' | 'dracula'
  | 'rose-pine' | 'rose-pine-dawn'
  | 'gruvbox-dark' | 'gruvbox-light'
  | 'tokyo-night' | 'one-dark'
  | 'everforest-dark' | 'kanagawa-wave'
  | 'solarized-light'

export const THEMES: { value: ThemeName; label: string }[] = [
  { value: 'aetox', label: 'Aetox' },
  { value: 'catppuccin-mocha', label: 'Catppuccin Mocha' },
  { value: 'catppuccin-latte', label: 'Catppuccin Latte' },
  { value: 'nord', label: 'Nord' },
  { value: 'dracula', label: 'Dracula' },
  { value: 'rose-pine', label: 'Rosé Pine' },
  { value: 'rose-pine-dawn', label: 'Rosé Pine Dawn' },
  { value: 'gruvbox-dark', label: 'Gruvbox Dark' },
  { value: 'gruvbox-light', label: 'Gruvbox Light' },
  { value: 'tokyo-night', label: 'Tokyo Night' },
  { value: 'one-dark', label: 'One Dark' },
  { value: 'everforest-dark', label: 'Everforest Dark' },
  { value: 'kanagawa-wave', label: 'Kanagawa Wave' },
  { value: 'solarized-light', label: 'Solarized Light' },
]

const DEFAULT_DARK: ThemeName = 'aetox'

const STORAGE_KEY = 'aetox-theme'
const VALID_NAMES = new Set(THEMES.map((t) => t.value))

function preferred(): ThemeName {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved && VALID_NAMES.has(saved as ThemeName)) return saved as ThemeName
  // Brand default is dark — a light OS preference used to flip a fresh
  // profile to Latte, which reads as "the app is broken" on first open.
  return DEFAULT_DARK
}

export const theme = $state<{ name: ThemeName }>({ name: DEFAULT_DARK })

export function applyTheme(name: ThemeName): void {
  theme.name = name
  document.documentElement.dataset.theme = name
  localStorage.setItem(STORAGE_KEY, name)
}

/** Call once before mount so there is no unthemed flash. */
export function initTheme(): void {
  applyTheme(preferred())
}
