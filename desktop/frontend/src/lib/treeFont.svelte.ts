// File-tree row (name) font size — independent of the system UI font scale,
// same pattern as editorFont.svelte.ts / chatFont.svelte.ts. Only `.row`
// (Sidebar.svelte's tree + workbench/FilesPane.svelte, same shared class)
// reads this; the rest of the sidebar chrome stays on the locked system scale.

const STORAGE_KEY = 'aetox-tree-font-size'
const DEFAULT_SIZE = 13
const MIN_SIZE = 11
const MAX_SIZE = 18

export const treeFont = $state<{ size: number }>({ size: DEFAULT_SIZE })

export function applyTreeFontSize(size: number): void {
  const clamped = Math.min(MAX_SIZE, Math.max(MIN_SIZE, size))
  treeFont.size = clamped
  document.documentElement.style.setProperty('--tree-font-size', `${clamped}px`)
  localStorage.setItem(STORAGE_KEY, String(clamped))
}

/** Call once before mount so the tree doesn't flash at the fallback size. */
export function initTreeFont(): void {
  const saved = parseFloat(localStorage.getItem(STORAGE_KEY) ?? '')
  applyTreeFontSize(Number.isFinite(saved) ? saved : DEFAULT_SIZE)
}
