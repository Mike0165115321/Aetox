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

import { SYNTAX_THEMES, type SyntaxPalette } from './syntaxTheme'

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

// Built from the shared palette in syntaxTheme.ts rather than a table of its
// own. The chat renders code too, and when these hexes lived here alone the
// only way for it to colour a fenced block was a hardcoded highlight.js
// stylesheet — which is how the four light themes ended up with dark-theme
// token colours on a light surface.
function toMonacoTheme(palette: SyntaxPalette): MonacoThemeData {
  return {
    base: palette.base,
    inherit: true,
    rules: Object.entries(palette.syntax).map(([token, hex]) => ({ token, foreground: hex.replace(/^#/, '') })),
    colors: { ...palette.chrome },
  }
}
/** Registers every named UI theme as a same-named Monaco theme, once. */
export async function defineNamedEditorThemes(): Promise<void> {
  const monaco = await import('monaco-editor')
  for (const [name, palette] of Object.entries(SYNTAX_THEMES)) {
    monaco.editor.defineTheme(name, toMonacoTheme(palette))
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
