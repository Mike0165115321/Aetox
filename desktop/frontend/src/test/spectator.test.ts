// wails dev hands the full frontend to any browser that connects — bindings
// work, events arrive — but the native surfaces those bindings drive (the
// browser windows, the PTY) exist exactly once, inside the real app window.
// On 2026-08-26 a second connected frontend received the agent's open-browser
// broadcast, mounted its own BrowserPane, and kept regluing the app's native
// window to the second window's geometry: ~390px wide, over the chat column,
// on every page the agent opened (§191).
//
// hostWebview.ts is the gate: only the frontend running inside the app's own
// webview (marked by the bridge object WebView2 injects, window.chrome.webview)
// may steer a native surface. Everything else is a spectator. setup.ts stubs
// the bridge so every OTHER test renders as the real window; these tests
// remove it to be the outsider.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, waitFor } from '@testing-library/svelte'
import BrowserPane from '../lib/workbench/BrowserPane.svelte'
import {
  BrowserOpen, BrowserSetBounds, BrowserSetZoom, BrowserSetVisible,
} from './mocks/wailsApp'
import type { WorkbenchTab } from '../lib/stores/workbench.svelte'

const tab = (): WorkbenchTab =>
  ({ id: 'web-agent-1', kind: 'browser', name: 'GitHub', url: 'https://github.com/', mine: true }) as WorkbenchTab

const savedBridge = (window as any).chrome

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  ;(window as any).chrome = savedBridge
})

describe('a frontend outside the app webview', () => {
  it('never opens or reglues the native window — it is not its to place', async () => {
    delete (window as any).chrome
    const { container } = render(BrowserPane, {
      tab: tab(), active: true, menuOpen: false, dragging: false,
    })

    // The note is what a dev sees instead of a black void that reads as
    // broken rendering — the sentence that opened this investigation.
    await waitFor(() =>
      expect(container.querySelector('.spectator-note')?.textContent)
        .toContain('หน้าต่างนี้เป็นแค่ผู้ชม'))

    expect(vi.mocked(BrowserOpen)).not.toHaveBeenCalled()
    expect(vi.mocked(BrowserSetBounds)).not.toHaveBeenCalled()
    expect(vi.mocked(BrowserSetZoom)).not.toHaveBeenCalled()
    // And it must not hide the window the user is looking at, either — a
    // spectator whose tab strip is on another tab is a fact about the
    // spectator, not about the app.
    expect(vi.mocked(BrowserSetVisible)).not.toHaveBeenCalled()
  })

  // The inverse, so the gate can never quietly flip polarity: the bridge stub
  // from setup.ts is what makes every other browser test the real window.
  it('the real window (bridge present) still opens the page', async () => {
    render(BrowserPane, { tab: tab(), active: true, menuOpen: false, dragging: false })
    await waitFor(() => expect(vi.mocked(BrowserOpen)).toHaveBeenCalled())
  })
})
