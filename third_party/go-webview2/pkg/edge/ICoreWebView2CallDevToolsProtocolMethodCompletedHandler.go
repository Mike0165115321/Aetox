//go:build windows

package edge

// AETOX PATCH: upstream declares the CallDevToolsProtocolMethod vtbl slot
// (corewebview2.go) and binds nothing, exactly as it did for GetIsSuccess and
// CapturePreview before them. This is the fourth patch of that same shape, and
// the sibling `pkg/webview2` copy already had both halves.
//
// One thing there is NOT copied, deliberately. That copy declares Invoke's
// second parameter as a Go `string`:
//
//	func ...Invoke(this *..., errorCode uintptr, result string) uintptr
//
// COM passes an LPCWSTR — a pointer to UTF-16 — and a Go string header is two
// words with a different layout entirely, so reading it that way interprets the
// pointer as a length and whatever follows on the stack as a data pointer. It
// happens not to crash there only because nothing in this tree calls it. The
// `edge` package's own ExecuteScript handler has it right (`*uint16`), and that
// is the shape followed here.

type _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type iCoreWebView2CallDevToolsProtocolMethodCompletedHandler struct {
	vtbl *_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl
	impl _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownQueryInterface(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler, refiid, object uintptr) uintptr {
	return this.impl.QueryInterface(refiid, object)
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownAddRef(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.AddRef()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownRelease(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.Release()
}

func iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke(this *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler, errorCode uintptr, returnObjectAsJSON *uint16) uintptr {
	return this.impl.CallDevToolsProtocolMethodCompleted(errorCode, returnObjectAsJSON)
}

type _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl interface {
	_IUnknownImpl
	CallDevToolsProtocolMethodCompleted(errorCode uintptr, returnObjectAsJSON *uint16) uintptr
}

var _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn = _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl{
	_IUnknownVtbl{
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownQueryInterface),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownAddRef),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownRelease),
	},
	NewComProc(iCoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke),
}

func newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(impl _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl) *iCoreWebView2CallDevToolsProtocolMethodCompletedHandler {
	return &iCoreWebView2CallDevToolsProtocolMethodCompletedHandler{
		vtbl: &_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn,
		impl: impl,
	}
}
