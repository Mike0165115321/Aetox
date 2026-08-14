// Test double for ../../wailsjs/runtime/runtime.
import { vi } from 'vitest'

// Variadic on purpose, for the same reason the App bindings are (wailsApp.ts):
// a zero-arg mock types `mock.calls[0]` as an empty tuple, so a test that
// reaches for the handler it registered — `mock.calls.find(c => c[0] ===
// 'update:available')[1]`, the only way to drive a Go-side event from a test —
// fails to type-check even though it passes at runtime.
export const EventsOn = vi.fn((..._args: any[]) => () => {})
export const EventsOnce = vi.fn((..._args: any[]) => () => {})
export const EventsOff = vi.fn()
export const EventsEmit = vi.fn()
export const BrowserOpenURL = vi.fn()
export const WindowSetTitle = vi.fn()
export const Quit = vi.fn()
export const LogInfo = vi.fn()
export const OnFileDrop = vi.fn()
export const OnFileDropOff = vi.fn()
