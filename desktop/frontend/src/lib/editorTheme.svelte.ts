// Monaco's own syntax-highlighting theme. Default ('auto') follows the app's
// named UI theme (theme.svelte.ts) so the editor reskins along with everything
// else; 'vs-dark'/'vs' are plain Monaco built-ins for anyone who wants to opt
// out, and 'imported' is one VS Code theme extension's theme JSON.
//
// VS Code theme JSON -> Monaco IStandaloneThemeData is inherently approximate:
// VS Code's `tokenColors` use TextMate scope selectors (e.g.
// "keyword.control.flow"); Monaco's basic-languages tokenizers are Monarch
// grammars that emit their own, simpler token names (e.g. "keyword"). There is
// no exact mapping between the two systems. scopeToToken below is a best-effort
// prefix match over the common cases (comment/string/number/keyword/type/
// function/variable/tag/attribute/regexp) — good enough to make an imported
// theme look "roughly right", not a pixel-perfect port. `colors` (the UI chrome
// colors — editor.background, editor.foreground, etc.) map through almost
// verbatim, since Monaco reuses VS Code's color id naming directly.

import type { ThemeName } from './theme.svelte'

export type EditorThemeChoice = 'auto' | 'vs-dark' | 'vs' | 'imported'

const CHOICE_KEY = 'aetox-editor-theme-choice'
const JSON_KEY = 'aetox-editor-theme-json'

export const editorTheme = $state<{ choice: EditorThemeChoice; importedName: string | null }>({
  choice: 'auto',
  importedName: null,
})

type MonacoThemeRule = { token: string; foreground?: string; fontStyle?: string }
type MonacoThemeData = { base: 'vs-dark' | 'vs'; inherit: true; rules: MonacoThemeRule[]; colors: Record<string, string> }

const scopeToToken: [RegExp, string][] = [
  [/^comment/, 'comment'],
  [/^string/, 'string'],
  [/^constant\.numeric|^number/, 'number'],
  [/^constant|^support\.constant/, 'constant'],
  [/^keyword|^storage/, 'keyword'],
  [/^entity\.name\.function|^support\.function/, 'function'],
  [/^entity\.name\.type|^entity\.name\.class|^support\.type|^support\.class/, 'type'],
  [/^entity\.name\.tag/, 'tag'],
  [/^entity\.other\.attribute-name/, 'attribute.name'],
  [/^variable/, 'variable'],
  [/^keyword\.operator|^punctuation/, 'delimiter'],
  [/^string\.regexp/, 'regexp'],
  [/^invalid/, 'invalid'],
]

function toMonacoToken(scope: string): string | null {
  for (const [re, token] of scopeToToken) if (re.test(scope)) return token
  return null
}

function stripHash(hex: unknown): string | undefined {
  return typeof hex === 'string' ? hex.replace(/^#/, '') : undefined
}

/** Best-effort conversion of a VS Code theme extension's theme JSON to a Monaco theme. */
export function vsCodeThemeToMonaco(raw: unknown): { name: string; data: MonacoThemeData } {
  if (typeof raw !== 'object' || raw === null) throw new Error('not a theme JSON object')
  const t = raw as Record<string, unknown>
  const base: 'vs-dark' | 'vs' = t.type === 'light' ? 'vs' : 'vs-dark'
  const name = typeof t.name === 'string' && t.name.trim() ? t.name.trim() : 'imported'

  const rules: MonacoThemeRule[] = []
  if (Array.isArray(t.tokenColors)) {
    for (const entry of t.tokenColors as Record<string, unknown>[]) {
      const settings = entry.settings as Record<string, unknown> | undefined
      const foreground = stripHash(settings?.foreground)
      const fontStyle = typeof settings?.fontStyle === 'string' ? settings.fontStyle : undefined
      if (!foreground && !fontStyle) continue
      const scopes = Array.isArray(entry.scope) ? entry.scope : typeof entry.scope === 'string' ? entry.scope.split(',').map((s) => s.trim()) : []
      for (const scope of scopes) {
        const token = toMonacoToken(scope)
        if (token) rules.push({ token, foreground, fontStyle })
      }
    }
  }

  const colors: Record<string, string> = {}
  if (typeof t.colors === 'object' && t.colors !== null) {
    for (const [key, value] of Object.entries(t.colors as Record<string, unknown>)) {
      if (typeof value === 'string') colors[key] = value
    }
  }

  return { name, data: { base, inherit: true, rules, colors } }
}

async function defineImportedTheme(data: MonacoThemeData): Promise<void> {
  const monaco = await import('monaco-editor')
  monaco.editor.defineTheme('imported', data)
}

export async function importThemeFile(file: File): Promise<void> {
  const text = await file.text()
  const raw = JSON.parse(text)
  const { name, data } = vsCodeThemeToMonaco(raw)
  await defineImportedTheme(data)
  localStorage.setItem(JSON_KEY, text)
  editorTheme.importedName = name
  editorTheme.choice = 'imported'
  localStorage.setItem(CHOICE_KEY, 'imported')
}

export function setBuiltinEditorTheme(choice: 'vs-dark' | 'vs'): void {
  editorTheme.choice = choice
  localStorage.setItem(CHOICE_KEY, choice)
}

export function setAutoEditorTheme(): void {
  editorTheme.choice = 'auto'
  localStorage.setItem(CHOICE_KEY, 'auto')
}

// One row per named UI theme (theme.svelte.ts THEMES), same hex sources — a
// theme here that isn't ported to the app's UI chrome would drift, so the
// naming/roles below intentionally mirror the accent/status tokens in theme.css.
function rules(map: Record<string, string>): MonacoThemeRule[] {
  return Object.entries(map).map(([token, foreground]) => ({ token, foreground: foreground.replace(/^#/, '') }))
}

const NAMED_EDITOR_THEMES: Record<ThemeName, MonacoThemeData> = {
  'catppuccin-mocha': {
    base: 'vs-dark', inherit: true,
    rules: rules({
      comment: '6c7086', string: 'a6e3a1', number: 'fab387', constant: 'fab387',
      keyword: 'cba6f7', function: '89b4fa', type: 'f9e2af', tag: 'f38ba8',
      'attribute.name': 'f9e2af', variable: 'cdd6f4', delimiter: 'bac2de', regexp: 'f5c2e7', invalid: 'f38ba8',
    }),
    colors: {
      'editor.background': '#181825', 'editor.foreground': '#cdd6f4',
      'editor.lineHighlightBackground': '#313244', 'editorCursor.foreground': '#cba6f7',
      'editor.selectionBackground': '#45475a', 'editorLineNumber.foreground': '#585b70',
      'editorLineNumber.activeForeground': '#bac2de',
    },
  },
  'catppuccin-latte': {
    base: 'vs', inherit: true,
    rules: rules({
      comment: '9ca0b0', string: '40a02b', number: 'fe640b', constant: 'fe640b',
      keyword: '8839ef', function: '1e66f5', type: 'df8e1d', tag: 'd20f39',
      'attribute.name': 'df8e1d', variable: '4c4f69', delimiter: '5c5f77', regexp: 'ea76cb', invalid: 'd20f39',
    }),
    colors: {
      'editor.background': '#e6e9ef', 'editor.foreground': '#4c4f69',
      'editor.lineHighlightBackground': '#ccd0da', 'editorCursor.foreground': '#8839ef',
      'editor.selectionBackground': '#bcc0cc', 'editorLineNumber.foreground': '#acb0be',
      'editorLineNumber.activeForeground': '#5c5f77',
    },
  },
  nord: {
    base: 'vs-dark', inherit: true,
    rules: rules({
      comment: '4c566a', string: 'a3be8c', number: 'b48ead', constant: 'b48ead',
      keyword: '81a1c1', function: '88c0d0', type: '8fbcbb', tag: 'bf616a',
      'attribute.name': 'd08770', variable: 'eceff4', delimiter: 'd8dee9', regexp: 'ebcb8b', invalid: 'bf616a',
    }),
    colors: {
      'editor.background': '#3b4252', 'editor.foreground': '#eceff4',
      'editor.lineHighlightBackground': '#434c5e', 'editorCursor.foreground': '#88c0d0',
      'editor.selectionBackground': '#434c5e', 'editorLineNumber.foreground': '#4c566a',
      'editorLineNumber.activeForeground': '#d8dee9',
    },
  },
  dracula: {
    base: 'vs-dark', inherit: true,
    rules: rules({
      comment: '6272a4', string: 'f1fa8c', number: 'bd93f9', constant: 'bd93f9',
      keyword: 'ff79c6', function: '50fa7b', type: '8be9fd', tag: 'ff5555',
      'attribute.name': '50fa7b', variable: 'f8f8f2', delimiter: 'f8f8f2', regexp: 'ff79c6', invalid: 'ff5555',
    }),
    colors: {
      'editor.background': '#282a36', 'editor.foreground': '#f8f8f2',
      'editor.lineHighlightBackground': '#343646', 'editorCursor.foreground': '#bd93f9',
      'editor.selectionBackground': '#44475a', 'editorLineNumber.foreground': '#6272a4',
      'editorLineNumber.activeForeground': '#f8f8f2',
    },
  },
  'rose-pine': {
    base: 'vs-dark', inherit: true,
    rules: rules({
      comment: '6e6a86', string: '8bbd8b', number: 'f6c177', constant: 'f6c177',
      keyword: '9ccfd8', function: 'c4a7e7', type: 'ebbcba', tag: 'eb6f92',
      'attribute.name': 'f6c177', variable: 'e0def4', delimiter: '908caa', regexp: 'eb6f92', invalid: 'eb6f92',
    }),
    colors: {
      'editor.background': '#1f1d2e', 'editor.foreground': '#e0def4',
      'editor.lineHighlightBackground': '#26233a', 'editorCursor.foreground': '#c4a7e7',
      'editor.selectionBackground': '#403d52', 'editorLineNumber.foreground': '#6e6a86',
      'editorLineNumber.activeForeground': '#908caa',
    },
  },
  'rose-pine-dawn': {
    base: 'vs', inherit: true,
    rules: rules({
      comment: '9893a5', string: '4f7a3d', number: 'ea9d34', constant: 'ea9d34',
      keyword: '286983', function: '907aa9', type: 'd7827e', tag: 'b4637a',
      'attribute.name': 'ea9d34', variable: '464261', delimiter: '797593', regexp: 'b4637a', invalid: 'b4637a',
    }),
    colors: {
      'editor.background': '#fffaf3', 'editor.foreground': '#464261',
      'editor.lineHighlightBackground': '#f2e9e1', 'editorCursor.foreground': '#907aa9',
      'editor.selectionBackground': '#dfdad9', 'editorLineNumber.foreground': '#9893a5',
      'editorLineNumber.activeForeground': '#797593',
    },
  },
  'gruvbox-dark': {
    base: 'vs-dark', inherit: true,
    rules: rules({
      comment: '928374', string: 'b8bb26', number: 'd3869b', constant: 'd3869b',
      keyword: 'fb4934', function: '8ec07c', type: 'fabd2f', tag: 'fb4934',
      'attribute.name': 'fabd2f', variable: 'ebdbb2', delimiter: 'ebdbb2', regexp: 'fe8019', invalid: 'fb4934',
    }),
    colors: {
      'editor.background': '#282828', 'editor.foreground': '#ebdbb2',
      'editor.lineHighlightBackground': '#3c3836', 'editorCursor.foreground': '#fe8019',
      'editor.selectionBackground': '#504945', 'editorLineNumber.foreground': '#7c6f64',
      'editorLineNumber.activeForeground': '#bdae93',
    },
  },
  'gruvbox-light': {
    base: 'vs', inherit: true,
    rules: rules({
      comment: '928374', string: '79740e', number: '8f3f71', constant: '8f3f71',
      keyword: '9d0006', function: '427b58', type: 'b57614', tag: '9d0006',
      'attribute.name': 'b57614', variable: '3c3836', delimiter: '3c3836', regexp: 'af3a03', invalid: '9d0006',
    }),
    colors: {
      'editor.background': '#fbf1c7', 'editor.foreground': '#3c3836',
      'editor.lineHighlightBackground': '#ebdbb2', 'editorCursor.foreground': '#af3a03',
      'editor.selectionBackground': '#d5c4a1', 'editorLineNumber.foreground': '#a89984',
      'editorLineNumber.activeForeground': '#665c54',
    },
  },
  'tokyo-night': {
    base: 'vs-dark', inherit: true,
    rules: rules({
      comment: '565f89', string: '9ece6a', number: 'ff9e64', constant: 'ff9e64',
      keyword: '9d7cd8', function: '7aa2f7', type: '2ac3de', tag: 'f7768e',
      'attribute.name': 'bb9af7', variable: 'c0caf5', delimiter: 'a9b1d6', regexp: 'b4f9f8', invalid: 'f7768e',
    }),
    colors: {
      'editor.background': '#1a1b26', 'editor.foreground': '#c0caf5',
      'editor.lineHighlightBackground': '#292e42', 'editorCursor.foreground': '#9d7cd8',
      'editor.selectionBackground': '#33467c', 'editorLineNumber.foreground': '#3b4261',
      'editorLineNumber.activeForeground': '#a9b1d6',
    },
  },
}

/** Registers every named UI theme as a same-named Monaco theme, once. */
export async function defineNamedEditorThemes(): Promise<void> {
  const monaco = await import('monaco-editor')
  for (const [name, data] of Object.entries(NAMED_EDITOR_THEMES)) {
    monaco.editor.defineTheme(name, data)
  }
}

let themesRegistered = false

/** Call once, right before the editor first mounts (FileEditor.svelte's onMount) —
 * NOT at app startup, so Monaco (~5MB) still only loads when a file is actually
 * opened. Registers the 9 named themes and redefines 'imported' if one was saved. */
export async function ensureEditorThemesRegistered(): Promise<void> {
  if (themesRegistered) return
  themesRegistered = true
  await defineNamedEditorThemes()
  const savedJson = localStorage.getItem(JSON_KEY)
  if (savedJson) {
    try {
      const { name, data } = vsCodeThemeToMonaco(JSON.parse(savedJson))
      await defineImportedTheme(data)
      editorTheme.importedName = name
    } catch {
      localStorage.removeItem(JSON_KEY)
      if (editorTheme.choice === 'imported') editorTheme.choice = 'auto'
    }
  }
}

/** Call once before mount: restores the saved choice from localStorage. No Monaco
 * import here — that stays deferred to ensureEditorThemesRegistered(). */
export function initEditorTheme(): void {
  const savedChoice = localStorage.getItem(CHOICE_KEY)
  if (savedChoice === 'auto' || savedChoice === 'vs-dark' || savedChoice === 'vs' || savedChoice === 'imported') {
    editorTheme.choice = savedChoice as EditorThemeChoice
  }
}
