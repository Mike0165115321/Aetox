# go-webview2 — local fork (Aetox patch)

Vendored copy of `github.com/wailsapp/go-webview2 v1.0.22`, wired via a
`replace` in the root `go.mod`. Same pattern as `third_party/conpty`.

## Why

Upstream `pkg/edge/chromium.go`'s error path is:

```go
func (e *Chromium) errorCallback(err error) {
	e.globalErrorCallback(err)
	os.Exit(1)          // always, even after SetErrorCallback
}
```

Aetox embeds one WebView2 **per browser tab** (`desktop/browser.go`). Any
single tab hitting a transient WebView2 failure — `ERROR_INVALID_STATE`
(0x8007139F) from RivaTuner/RTSS DLL injection, a GPU-driver hiccup, low
memory — routed through `errorCallback` and `os.Exit(1)`'d the **entire app**.
`SetErrorCallback` looked like it prevented this but only swapped the inner
callback; the `os.Exit` fired regardless.

## The patch (search `AETOX PATCH` in pkg/edge/chromium.go)

1. `SetErrorCallback` sets `customErrorCallback = true`.
2. `errorCallback` skips `os.Exit(1)` when a custom callback is installed —
   that callback owns recovery (Aetox logs it and lets the one tab fail).
   The default handler (used by the wails main window, which never calls
   SetErrorCallback) keeps exiting, so main-window behavior is unchanged.
3. `CreateCoreWebView2ControllerCompleted` early-returns on failure instead of
   nil-dereferencing `controller` (upstream relied on the now-removed exit),
   sets `inited` to unblock `Embed`'s message loop, and flags `embedFailed`.
4. `Embed` returns `false` on `embedFailed`, so `desktop/browser.go` destroys
   the orphan child window instead of navigating a nil webview.

## Upgrading go-webview2

Re-copy the module, then re-apply the four `AETOX PATCH` blocks. Keep the
version in this note and the root `go.mod` require in sync.
