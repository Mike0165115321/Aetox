//go:build windows

package edge

// AETOX PATCH: upstream declares the CapturePreview vtbl slot (corewebview2.go)
// but binds no method and ships no completion handler for it, so the `edge`
// package cannot take a picture of the page at all. The sibling `pkg/webview2`
// copy has both; this is the same pair, written in this package's idiom.
//
// Aetox needs it because a picture is the only honest answer to "look at THIS
// part of the page" — an annotation drawn over a region has to be a region of
// something. See desktop/browser_shot.go.

import (
	"unsafe"
)

type _ICoreWebView2CapturePreviewCompletedHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type iCoreWebView2CapturePreviewCompletedHandler struct {
	vtbl *_ICoreWebView2CapturePreviewCompletedHandlerVtbl
	impl _ICoreWebView2CapturePreviewCompletedHandlerImpl
}

func (i *iCoreWebView2CapturePreviewCompletedHandler) AddRef() uint32 {
	ret, _, _ := i.vtbl.AddRef.Call(uintptr(unsafe.Pointer(i)))

	return uint32(ret)
}

func (i *iCoreWebView2CapturePreviewCompletedHandler) Release() uint32 {
	ret, _, _ := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))

	return uint32(ret)
}

func _ICoreWebView2CapturePreviewCompletedHandlerIUnknownQueryInterface(this *iCoreWebView2CapturePreviewCompletedHandler, refiid, object uintptr) uintptr {
	return this.impl.QueryInterface(refiid, object)
}

func _ICoreWebView2CapturePreviewCompletedHandlerIUnknownAddRef(this *iCoreWebView2CapturePreviewCompletedHandler) uintptr {
	return this.impl.AddRef()
}

func _ICoreWebView2CapturePreviewCompletedHandlerIUnknownRelease(this *iCoreWebView2CapturePreviewCompletedHandler) uintptr {
	return this.impl.Release()
}

func iCoreWebView2CapturePreviewCompletedHandlerInvoke(this *iCoreWebView2CapturePreviewCompletedHandler, errorCode uintptr) uintptr {
	return this.impl.CapturePreviewCompleted(errorCode)
}

type _ICoreWebView2CapturePreviewCompletedHandlerImpl interface {
	_IUnknownImpl
	CapturePreviewCompleted(errorCode uintptr) uintptr
}

var _ICoreWebView2CapturePreviewCompletedHandlerFn = _ICoreWebView2CapturePreviewCompletedHandlerVtbl{
	_IUnknownVtbl{
		NewComProc(_ICoreWebView2CapturePreviewCompletedHandlerIUnknownQueryInterface),
		NewComProc(_ICoreWebView2CapturePreviewCompletedHandlerIUnknownAddRef),
		NewComProc(_ICoreWebView2CapturePreviewCompletedHandlerIUnknownRelease),
	},
	NewComProc(iCoreWebView2CapturePreviewCompletedHandlerInvoke),
}

func newICoreWebView2CapturePreviewCompletedHandler(impl _ICoreWebView2CapturePreviewCompletedHandlerImpl) *iCoreWebView2CapturePreviewCompletedHandler {
	return &iCoreWebView2CapturePreviewCompletedHandler{
		vtbl: &_ICoreWebView2CapturePreviewCompletedHandlerFn,
		impl: impl,
	}
}
