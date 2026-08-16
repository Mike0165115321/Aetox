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

## A second, unrelated patch

`pkg/edge/ICoreWebView2NavigationCompletedEventArgs.go` binds `GetIsSuccess`.
Upstream declares the vtbl slot but no method, so every `NavigationCompleted`
looked identical whether the page loaded or Chrome rendered its own error page
— `browser_open` reported success over `ERR_FILE_NOT_FOUND`. The sibling
`pkg/webview2` copy of this interface already binds it; this is the same
binding, in the `edge` package's own idiom.

## A third patch: taking a picture of the page

`pkg/edge/capture.go` and `pkg/edge/ICoreWebView2CapturePreviewCompletedHandler.go`
add `Chromium.CapturePreview`. Same shape as the second patch — upstream
declares the vtbl slot and binds nothing, and the sibling `pkg/webview2` copy
already has both halves.

Aetox needs it for annotation (`desktop/browser_shot.go`): a mark drawn on a
page has to be a mark on *something*, and the only honest carrier of "ตรงบริเวณ
นี้" is the rendering itself. It is the engine's own capture rather than a
screen grab of the tab's window, so nothing floating above the window can end up
in the picture.

Two things there are deliberate and easy to undo by accident:

- **The bytes are read off the HGLOBAL** (`GetHGlobalFromStream` + `GlobalLock`),
  not through `IStream::Seek`/`Read`. This package's `IStream` binding covers
  only the two `ISequentialStream` slots, and reading the memory the stream is
  already backed by needs no further vtbl work.
- **`CapturePreview` returns a channel, and the caller must read it off the
  webview thread.** The completion handler is invoked by that thread's message
  pump, so waiting for it there is waiting for the thing that would deliver it.

## Upgrading go-webview2

Re-copy the module, then re-apply the four `AETOX PATCH` blocks, the
`GetIsSuccess` binding, and the two capture files above. Keep the version in
this note and the root `go.mod` require in sync.
