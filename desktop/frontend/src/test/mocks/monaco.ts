// Test double for the monaco-editor package (heavy, worker-based — neither
// loads under jsdom). FileEditor only touches this small surface.
import { vi } from 'vitest'

export const editor = {
  createModel: vi.fn(() => ({ getValue: () => '', dispose: vi.fn() })),
  create: vi.fn(() => ({
    onDidChangeModelContent: vi.fn(),
    addCommand: vi.fn(),
    updateOptions: vi.fn(),
    dispose: vi.fn(),
  })),
  setTheme: vi.fn(),
  defineTheme: vi.fn(),
}
export const KeyMod = { CtrlCmd: 1 }
export const KeyCode = { KeyS: 1 }
