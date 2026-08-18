//go:build windows

package edge

// AETOX PATCH: Chromium.CallDevToolsProtocolMethod — the Chrome DevTools
// Protocol door. See ICoreWebView2CallDevToolsProtocolMethodCompletedHandler.go
// for why this is not upstream.
//
// Aetox needs it for one thing to begin with: `Page.printToPDF`, so a deck
// written as HTML can be exported as a PDF (docs/architecture/html-deck-2026-08-19.md).
// The alternative was binding ICoreWebView2_7 or _16 plus
// ICoreWebView2Environment6 plus ICoreWebView2PrintSettings plus its own
// completed handler — four interfaces for one call, where this is one method and
// one handler and answers every other CDP question afterwards for free.
//
// What it does NOT buy is a way around the compositor. `Page.captureScreenshot`
// goes through the same render path a hidden webview never runs (see
// desktop/browser_capture.go, and WebView2Feedback #1077 and #2983), so this
// door opens PDF and does not open PNG. Printing is different in kind: it is a
// separate pipeline, and Microsoft's own word for PrintToPdf is "silently".
//
// Threading: identical to CapturePreview's, and for the identical reason. Call
// on the webview's thread; read the channel from another goroutine, because the
// answer is delivered by the message pump this thread is running.

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DevToolsResult is one finished (or failed) CDP call. JSON is the protocol's
// own answer object, verbatim and undecoded — this layer moves bytes, it does
// not know what Page.printToPDF returns.
type DevToolsResult struct {
	JSON string
	Err  error
}

// devToolsHandler is one in-flight call. Its own COM object rather than
// Chromium itself, so two calls in flight cannot land on one another's channel.
type devToolsHandler struct {
	method string
	out    chan DevToolsResult
}

func (h *devToolsHandler) QueryInterface(_, _ uintptr) uintptr { return 0 }
func (h *devToolsHandler) AddRef() uintptr                     { return 1 }
func (h *devToolsHandler) Release() uintptr                    { return 1 }

func (h *devToolsHandler) CallDevToolsProtocolMethodCompleted(errorCode uintptr, returnObjectAsJSON *uint16) uintptr {
	defer forgetDevTools(h)

	if errorCode != 0 {
		h.deliver(DevToolsResult{Err: fmt.Errorf("%s failed (hr=%#x)", h.method, errorCode)})
		return 0
	}
	// Copied out here, on the thread that was handed the pointer. The string
	// belongs to the caller for the duration of this callback and not one
	// instruction longer, so reading it from the waiting goroutine would be a
	// race against WebView2 freeing it.
	json := ""
	if returnObjectAsJSON != nil {
		json = windows.UTF16PtrToString(returnObjectAsJSON)
	}
	h.deliver(DevToolsResult{JSON: json})
	return 0
}

// deliver never blocks: the channel is buffered by its maker, and a caller that
// timed out and walked away must not wedge the thread that draws the browser.
func (h *devToolsHandler) deliver(r DevToolsResult) {
	select {
	case h.out <- r:
	default:
	}
}

// The COM object is handed to WebView2 as a bare pointer the Go garbage
// collector cannot see. Without a reference held here, a call that outlives the
// next collection is a pointer into freed memory — and the crash lands in the
// browser process, minutes later, nowhere near this file. Same hazard and same
// answer as capture.go's.
var (
	devToolsLive   = map[*devToolsHandler]struct{}{}
	devToolsLiveMu sync.Mutex
)

func rememberDevTools(h *devToolsHandler) {
	devToolsLiveMu.Lock()
	devToolsLive[h] = struct{}{}
	devToolsLiveMu.Unlock()
}

func forgetDevTools(h *devToolsHandler) {
	devToolsLiveMu.Lock()
	delete(devToolsLive, h)
	devToolsLiveMu.Unlock()
}

// CallDevToolsProtocolMethod runs one CDP method and returns a channel that
// yields the protocol's answer, or an error.
//
// method is "{domain}.{method}" and paramsJSON is that method's parameter
// object. paramsJSON must be valid JSON — "{}" for no parameters, never "" —
// because WebView2 rejects an empty string rather than treating it as an empty
// object.
func (e *Chromium) CallDevToolsProtocolMethod(method, paramsJSON string) <-chan DevToolsResult {
	out := make(chan DevToolsResult, 1)
	if e.webview == nil {
		out <- DevToolsResult{Err: errors.New("no webview to call")}
		return out
	}
	if paramsJSON == "" {
		paramsJSON = "{}"
	}

	u16method, err := windows.UTF16PtrFromString(method)
	if err != nil {
		out <- DevToolsResult{Err: err}
		return out
	}
	u16params, err := windows.UTF16PtrFromString(paramsJSON)
	if err != nil {
		out <- DevToolsResult{Err: err}
		return out
	}

	impl := &devToolsHandler{method: method, out: out}
	rememberDevTools(impl)
	handler := newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(impl)

	hr, _, _ := e.webview.vtbl.CallDevToolsProtocolMethod.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(u16method)),
		uintptr(unsafe.Pointer(u16params)),
		uintptr(unsafe.Pointer(handler)),
	)
	if hr != 0 {
		forgetDevTools(impl)
		out <- DevToolsResult{Err: fmt.Errorf("%s was refused (hr=%#x)", method, hr)}
	}
	return out
}
