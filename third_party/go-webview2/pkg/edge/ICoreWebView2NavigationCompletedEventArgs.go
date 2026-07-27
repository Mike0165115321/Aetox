//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type _ICoreWebView2NavigationCompletedEventArgsVtbl struct {
	_IUnknownVtbl
	GetIsSuccess      ComProc
	GetWebErrorStatus ComProc
	GetNavigationId   ComProc
}

type ICoreWebView2NavigationCompletedEventArgs struct {
	vtbl *_ICoreWebView2NavigationCompletedEventArgsVtbl
}

func (i *ICoreWebView2NavigationCompletedEventArgs) AddRef() uint32 {
	ret, _, _ := i.vtbl.AddRef.Call(uintptr(unsafe.Pointer(i)))

	return uint32(ret)
}

func (i *ICoreWebView2NavigationCompletedEventArgs) Release() uint32 {
	ret, _, _ := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))

	return uint32(ret)
}

// AETOX PATCH: upstream declares the vtbl slot but binds no method, so a
// NavigationCompleted handler cannot tell a page that loaded from one that
// failed — ERR_FILE_NOT_FOUND, a dead domain and a real page all raise the
// same event. desktop/browser.go needs the difference so browser_open can
// report the failure instead of a green tick over Chrome's error page.
func (i *ICoreWebView2NavigationCompletedEventArgs) GetIsSuccess() (bool, error) {
	var isSuccess int32 // BOOL: a 4-byte int, not Go's bool
	hr, _, _ := i.vtbl.GetIsSuccess.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&isSuccess)),
	)
	if windows.Handle(hr) != windows.S_OK {
		return false, windows.Errno(hr)
	}
	return isSuccess != 0, nil
}
