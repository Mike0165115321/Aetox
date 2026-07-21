// Chat message/composer font size — independent of the system UI font scale,
// same reasoning as editorFont.svelte.ts. Only .chat and .composer .input read
// this; the rest of the chrome stays on the locked system scale.

const STORAGE_KEY = 'aetox-chat-font-size'
const DEFAULT_SIZE = 15.5
const MIN_SIZE = 12
const MAX_SIZE = 22

export const chatFont = $state<{ size: number }>({ size: DEFAULT_SIZE })

export function applyChatFontSize(size: number): void {
  const clamped = Math.min(MAX_SIZE, Math.max(MIN_SIZE, size))
  chatFont.size = clamped
  document.documentElement.style.setProperty('--chat-font-size', `${clamped}px`)
  localStorage.setItem(STORAGE_KEY, String(clamped))
}

/** Call once before mount so chat doesn't flash at the fallback size. */
export function initChatFont(): void {
  const saved = parseFloat(localStorage.getItem(STORAGE_KEY) ?? '')
  applyChatFontSize(Number.isFinite(saved) ? saved : DEFAULT_SIZE)
}
