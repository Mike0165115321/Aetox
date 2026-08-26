// Which frontend is allowed to touch native surfaces.
//
// wails dev serves the same frontend to any browser that connects (port
// 34115), and every one of those connections is a FULL frontend: bindings
// work, events arrive. Useful for poking at the DOM — and a trap for anything
// that drives a native surface, because the native browser windows and the
// PTY exist exactly once, inside the real app window. On 2026-08-26 a second
// connected frontend received the open-browser broadcast, mounted its own
// BrowserPane, and reglued the app's native browser window to the second
// window's geometry — ~390px wide, over the chat column, on every page the
// agent opened, until the real window's next reflow won the race back (§191).
//
// The discriminator is client-local on purpose: nothing served over the wire
// can tell the two apart, since both run the same bundle against the same Go.
// What only the real one has is the bridge object its native webview injects —
// window.chrome.webview in WebView2, window.webkit.messageHandlers in WebKit —
// which an external browser never has. In production the question cannot even
// arise: bindings ride that same bridge, so a frontend without it has no Go to
// call. This gate exists for dev, where the websocket bridge answers anybody.
export function isHostWebview(): boolean {
  const w = window as unknown as {
    chrome?: { webview?: unknown }
    webkit?: { messageHandlers?: unknown }
  }
  return !!(w.chrome?.webview || w.webkit?.messageHandlers)
}
