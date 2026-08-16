//go:build !windows

package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// No answer here, so none is invented: a dock or panel that a desktop keeps for
// itself is asked for through that desktop's own API, and Aetox has no Linux or
// macOS build to ask from yet (ARCHITECTURE.md — 1.0.0 ships Windows first).
// fitToScreen still keeps the window inside the screen, which is the half of
// the problem that does not need this number.
func systemChrome(runtime.Screen) (int, int) { return 0, 0 }
