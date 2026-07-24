// Test double for ../../wailsjs/runtime/runtime.
import { vi } from 'vitest'

export const EventsOn = vi.fn(() => () => {})
export const EventsOnce = vi.fn(() => () => {})
export const EventsOff = vi.fn()
export const EventsEmit = vi.fn()
export const BrowserOpenURL = vi.fn()
export const WindowSetTitle = vi.fn()
export const Quit = vi.fn()
export const LogInfo = vi.fn()
export const OnFileDrop = vi.fn()
export const OnFileDropOff = vi.fn()
