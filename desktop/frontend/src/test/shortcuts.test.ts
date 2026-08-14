import { describe, it, expect } from 'vitest'
import { isShortcut, shortcutLabel, type ShortcutId } from '../lib/shortcuts'

function press(init: Partial<KeyboardEventInit> & { code: string; key: string }): KeyboardEvent {
  return new KeyboardEvent('keydown', { ctrlKey: false, altKey: false, shiftKey: false, ...init })
}

// What Windows reports for each physical key while the Thai (Kedmanee) layout
// is active. This is the whole bug: `key` carries the layout's character, so
// every shortcut in the app matched on a letter the keyboard never sends.
const THAI: Record<string, string> = {
  KeyN: 'ื', KeyB: 'ิ', KeyS: 'ห', KeyK: 'า', KeyT: 'ะ', KeyP: 'ย', Comma: 'ม',
}

const ALL: { id: ShortcutId; code: string; alt?: boolean }[] = [
  { id: 'newSession', code: 'KeyN' },
  { id: 'palette', code: 'KeyK' },
  { id: 'settings', code: 'Comma' },
  { id: 'toggleSidebar', code: 'KeyS', alt: true },
  { id: 'toggleInspector', code: 'KeyB', alt: true },
  { id: 'browserTab', code: 'KeyT' },
  { id: 'filesTab', code: 'KeyP' },
]

describe('keyboard shortcuts', () => {
  it('fires on a Thai layout, where the key reports a Thai character', () => {
    for (const { id, code, alt } of ALL) {
      const e = press({ code, key: THAI[code], ctrlKey: true, altKey: !!alt })
      expect(isShortcut(e, id), `${id} on Thai layout`).toBe(true)
    }
  })

  it('still fires on a US layout', () => {
    for (const { id, code, alt } of ALL) {
      const key = code === 'Comma' ? ',' : code.slice(3).toLowerCase()
      expect(isShortcut(press({ code, key, ctrlKey: true, altKey: !!alt }), id), id).toBe(true)
    }
  })

  it('fires on a remapped layout that moved the letter elsewhere', () => {
    // Dvorak: the N a Dvorak typist presses sits on the physical L key. `code`
    // misses it; `key` is the second chance that catches it.
    expect(isShortcut(press({ code: 'KeyL', key: 'n', ctrlKey: true }), 'newSession')).toBe(true)
  })

  it('matches modifiers exactly — a near miss is not a hit', () => {
    expect(isShortcut(press({ code: 'KeyN', key: 'n' }), 'newSession')).toBe(false)
    expect(isShortcut(press({ code: 'KeyN', key: 'n', ctrlKey: true, shiftKey: true }), 'newSession')).toBe(false)
    expect(isShortcut(press({ code: 'KeyN', key: 'n', ctrlKey: true, altKey: true }), 'newSession')).toBe(false)
    expect(isShortcut(press({ code: 'KeyN', key: 'n', metaKey: true, ctrlKey: true }), 'newSession')).toBe(false)
    // Ctrl+S alone is the file editor's save; only Ctrl+Alt+S is the sidebar.
    expect(isShortcut(press({ code: 'KeyS', key: 's', ctrlKey: true }), 'toggleSidebar')).toBe(false)
  })

  it('does not answer to a different key', () => {
    expect(isShortcut(press({ code: 'KeyT', key: 't', ctrlKey: true }), 'filesTab')).toBe(false)
  })

  it('labels read the way the chord is pressed', () => {
    expect(shortcutLabel('newSession')).toBe('Ctrl+N')
    expect(shortcutLabel('settings')).toBe('Ctrl+,')
    expect(shortcutLabel('toggleInspector')).toBe('Ctrl+Alt+B')
    expect(shortcutLabel('filesTab')).toBe('Ctrl+P')
  })

  // The reason this table exists. Palette advertised "Ctrl+Shift+G" for a tab
  // that answers to Ctrl+P, because the hint was typed in a second place.
  it('every label comes from the same row that matches the keystroke', () => {
    for (const { id, code, alt } of ALL) {
      const label = shortcutLabel(id)
      expect(label.startsWith('Ctrl+')).toBe(true)
      expect(label.includes('Alt')).toBe(!!alt)
      expect(isShortcut(press({ code, key: THAI[code], ctrlKey: true, altKey: !!alt }), id)).toBe(true)
    }
  })
})
