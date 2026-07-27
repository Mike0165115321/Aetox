//go:build !windows

package main

import (
	"errors"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
)

// The stand-in host for platforms whose real one is not written yet. It keeps
// the app whole rather than keeping it honest about one feature: everything
// else in desktop/ — chat, sessions, MCP, skills, the file tabs, the terminal —
// works on Linux and macOS the moment this file exists, because the only thing
// that stopped the package compiling was browser.go's Win32 imports.
//
// browser_linux.go (WebKitGTK) and browser_darwin.go (WKWebView) replace this
// one platform at a time; each takes its own GOOS out of this file's build tag.
// See PLATFORM-SUPPORT.md for where that stands.

// bridgePost is how a page hands a message back to Go on this engine. Both
// WebKits use the same script-message API, so this is already the right value
// for phases 3a and 3b — the injected scripts in browser.go need no further
// per-platform work. The handler name must match the one those hosts register
// with WebKitUserContentManager / WKUserContentController.
const bridgePost = "window.webkit.messageHandlers.aetox.postMessage"

var errNoBrowserHost = errors.New(
	"the in-app browser tab is Windows-only for now — everything else in the workbench works here")

type stubHost struct{ warned bool }

func newHostBackend() hostBackend { return &stubHost{} }

func (h *stubHost) start() error {
	if !h.warned {
		h.warned = true
		debuglog.Msg("browser: no native host on this platform yet; browser tabs are disabled")
	}
	return errNoBrowserHost
}

// do and openTab are unreachable in practice — browserHostLazy returns start's
// error and every binding gives up before reaching them — but a backend that
// silently did nothing would be worse than one that says so in the log if that
// ever stops being true.
func (h *stubHost) do(fn func()) {
	debuglog.Msg("browser: do() called with no native host; dropping the command")
}

func (h *stubHost) openTab(id, url string, x, y, w, hgt int, cb tabCallbacks) tabView {
	debuglog.Msg("browser: openTab(%s) called with no native host", id)
	return nil
}
