// File-content font size — independent of the system UI font scale. Read
// directly (not via CSS var) by FileEditor.svelte's Monaco instance, since
// Monaco renders via its own canvas/DOM hybrid and doesn't observe CSS custom
// properties — its `fontSize` option is set/updated straight from editorFont.size.

const STORAGE_KEY = 'aetox-editor-font-size'
const DEFAULT_SIZE = 14
const MIN_SIZE = 10
const MAX_SIZE = 24

export const editorFont = $state<{ size: number }>({ size: DEFAULT_SIZE })

export function applyEditorFontSize(size: number): void {
  const clamped = Math.min(MAX_SIZE, Math.max(MIN_SIZE, size))
  editorFont.size = clamped
  localStorage.setItem(STORAGE_KEY, String(clamped))
}

/** Call once before mount so the editor doesn't flash at the fallback size. */
export function initEditorFont(): void {
  const saved = parseFloat(localStorage.getItem(STORAGE_KEY) ?? '')
  applyEditorFontSize(Number.isFinite(saved) ? saved : DEFAULT_SIZE)
}
