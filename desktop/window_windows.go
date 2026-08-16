//go:build windows

package main

import (
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// user32 itself is declared once for the package, in browser_windows.go.
var procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")

// SPI_GETWORKAREA — the desktop rectangle left over once the taskbar and any
// other appbar has taken its edge.
const spiGetWorkArea = 0x0030

type winRect struct{ Left, Top, Right, Bottom int32 }

// systemChrome is how much of the screen the desktop keeps for itself, in the
// logical units Wails sizes windows in.
//
// Windows reports the work area in physical pixels, and Wails asks for window
// sizes in 96dpi units, so the difference is converted through the ratio the
// screen itself reports between the two. That ratio is the display's scaling —
// 150% on the laptop this was found on — and taking it from the screen rather
// than from a DPI call keeps the number in the same units as everything else
// fitToScreen compares.
//
// SPI_GETWORKAREA answers for the primary display. On a second monitor with a
// taskbar of its own the reserve is therefore an estimate; it is the same
// estimate the primary screen makes exactly, and being a few pixels
// conservative costs a few pixels of window.
func systemChrome(screen runtime.Screen) (int, int) {
	if screen.PhysicalSize.Width <= 0 || screen.PhysicalSize.Height <= 0 {
		return 0, 0
	}
	var work winRect
	ret, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0)
	if ret == 0 {
		return 0, 0
	}
	width := scaleDown(screen.PhysicalSize.Width-int(work.Right-work.Left), screen.Size.Width, screen.PhysicalSize.Width)
	height := scaleDown(screen.PhysicalSize.Height-int(work.Bottom-work.Top), screen.Size.Height, screen.PhysicalSize.Height)
	return width, height
}

// A negative reserve means the work area came back larger than the screen we
// measured against — a second monitor, or a display that changed under us.
// Nothing is kept in that case rather than growing the window past the screen.
func scaleDown(physical, logicalScreen, physicalScreen int) int {
	if physical <= 0 {
		return 0
	}
	return physical * logicalScreen / physicalScreen
}
