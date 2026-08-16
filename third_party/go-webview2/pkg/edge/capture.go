//go:build windows

package edge

// AETOX PATCH: Chromium.CapturePreview — a PNG of what the tab is currently
// showing. See ICoreWebView2CapturePreviewCompletedHandler.go for why this is
// not upstream.
//
// The picture goes into a COM stream over an HGLOBAL rather than a file: the
// caller wants bytes, and a temp file would be a path to clean up on a machine
// that may lose power between the write and the delete. The bytes are read back
// off the HGLOBAL directly (GetHGlobalFromStream + GlobalLock) instead of
// through IStream::Seek/Read, because this package's IStream binding covers
// only the two ISequentialStream slots — reading the memory the stream is
// already backed by needs no further vtbl archaeology.
//
// Threading: WebView2 is STA, so this must be called on the thread that owns
// the webview, and the completion handler is invoked on that same thread's
// message pump. It therefore cannot block waiting for its own callback — the
// pump is what would deliver it. Callers get a channel and wait elsewhere.

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	captureOle32               = windows.NewLazySystemDLL("ole32.dll")
	procCreateStreamOnHGlobal  = captureOle32.NewProc("CreateStreamOnHGlobal")
	procGetHGlobalFromStream   = captureOle32.NewProc("GetHGlobalFromStream")
	captureKernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalLockForCapture   = captureKernel32.NewProc("GlobalLock")
	procGlobalUnlockForCapture = captureKernel32.NewProc("GlobalUnlock")
	procGlobalSizeForCapture   = captureKernel32.NewProc("GlobalSize")
)

const capturePreviewFormatPNG = 0 // COREWEBVIEW2_CAPTURE_PREVIEW_IMAGE_FORMAT_PNG

// captureHandler is one in-flight CapturePreview. It is its own COM object
// rather than Chromium itself so that two captures cannot land on one
// another's channel.
//
// Everything it does runs on the webview's thread, because that is where the
// message pump invokes it — which is the point: the stream is read and released
// in the apartment that created it, rather than from whatever goroutine
// happened to be waiting.
type captureHandler struct {
	stream uintptr
	out    chan CaptureResult
}

func (h *captureHandler) QueryInterface(_, _ uintptr) uintptr { return 0 }
func (h *captureHandler) AddRef() uintptr                     { return 1 }
func (h *captureHandler) Release() uintptr                    { return 1 }

func (h *captureHandler) CapturePreviewCompleted(errorCode uintptr) uintptr {
	defer forgetCapture(h)
	defer releaseCOM(h.stream)

	if errorCode != 0 {
		h.deliver(CaptureResult{Err: fmt.Errorf("the page could not be captured (hr=%#x)", errorCode)})
		return 0
	}
	png, err := streamBytes(h.stream)
	h.deliver(CaptureResult{PNG: png, Err: err})
	return 0
}

// deliver never blocks: the channel is buffered by its maker, and a caller that
// timed out and walked away must not wedge the thread that draws the browser.
func (h *captureHandler) deliver(r CaptureResult) {
	select {
	case h.out <- r:
	default:
	}
}

// The COM object is handed to WebView2 as a bare pointer, which the Go garbage
// collector cannot see. Without a reference held here, a capture that takes
// longer than the next collection is a pointer into freed memory — and the
// crash lands in the browser process, minutes later, nowhere near this file.
var (
	captureLive   = map[*captureHandler]struct{}{}
	captureLiveMu sync.Mutex
)

func rememberCapture(h *captureHandler) {
	captureLiveMu.Lock()
	captureLive[h] = struct{}{}
	captureLiveMu.Unlock()
}

func forgetCapture(h *captureHandler) {
	captureLiveMu.Lock()
	delete(captureLive, h)
	captureLiveMu.Unlock()
}

// CapturePreview asks the webview for a PNG of its visible viewport and returns
// a channel that yields the bytes, or an error.
//
// Call it on the webview's own thread. Read the channel from another goroutine:
// the answer arrives via the message pump this thread is running.
func (e *Chromium) CapturePreview() <-chan CaptureResult {
	out := make(chan CaptureResult, 1)
	if e.webview == nil {
		out <- CaptureResult{Err: errors.New("no webview to capture")}
		return out
	}

	var stream uintptr
	// (nil, deleteOnRelease=TRUE) — COM allocates and owns the memory.
	hr, _, _ := procCreateStreamOnHGlobal.Call(0, 1, uintptr(unsafe.Pointer(&stream)))
	if hr != 0 || stream == 0 {
		out <- CaptureResult{Err: fmt.Errorf("could not allocate a capture stream (hr=%#x)", hr)}
		return out
	}

	impl := &captureHandler{stream: stream, out: out}
	rememberCapture(impl)
	handler := newICoreWebView2CapturePreviewCompletedHandler(impl)

	hr, _, _ = e.webview.vtbl.CapturePreview.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(capturePreviewFormatPNG),
		stream,
		uintptr(unsafe.Pointer(handler)),
	)
	if hr != 0 {
		forgetCapture(impl)
		releaseCOM(stream)
		out <- CaptureResult{Err: fmt.Errorf("CapturePreview refused the request (hr=%#x)", hr)}
	}
	return out
}

// CaptureResult is one finished (or failed) capture.
type CaptureResult struct {
	PNG []byte
	Err error
}

// streamBytes copies what the stream wrote out of the global memory backing it.
func streamBytes(stream uintptr) ([]byte, error) {
	var hglobal uintptr
	hr, _, _ := procGetHGlobalFromStream.Call(stream, uintptr(unsafe.Pointer(&hglobal)))
	if hr != 0 || hglobal == 0 {
		return nil, fmt.Errorf("the capture stream had no memory behind it (hr=%#x)", hr)
	}
	size, _, _ := procGlobalSizeForCapture.Call(hglobal)
	if size == 0 {
		return nil, errors.New("the capture came back empty")
	}
	ptr, _, _ := procGlobalLockForCapture.Call(hglobal)
	if ptr == 0 {
		return nil, errors.New("the capture memory could not be read")
	}
	defer procGlobalUnlockForCapture.Call(hglobal)

	// Copied, not referenced: the moment the stream is released this memory is
	// gone, and a []byte pointing into it would be a use-after-free with a
	// picture on the other end of it.
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size))
	return out, nil
}

// releaseCOM calls IUnknown::Release (vtbl slot 2) on a raw interface pointer.
func releaseCOM(iface uintptr) {
	if iface == 0 {
		return
	}
	vtbl := *(**[3]uintptr)(unsafe.Pointer(iface))
	ComProc(vtbl[2]).Call(iface)
}
