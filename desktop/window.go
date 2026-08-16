package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Mike0165115321/Aetox/internal/debuglog"
)

// The window the app asks for, in the logical (96dpi) units Wails sizes windows
// in. main.go hands the same numbers to Wails at creation and fitToScreen
// measures against them afterwards, so the opening size is stated once.
const (
	windowWidth     = 1440
	windowHeight    = 900
	windowMinWidth  = 1100
	windowMinHeight = 700
)

// fitToScreen shrinks the window until it fits the screen it opened on.
//
// Wails creates the window at windowWidth x windowHeight and centres it, and
// neither step asks whether the display is that big. A 1920x1080 laptop at
// Windows' default 150% scaling is 1280x720 in these units, so the window is
// built 180px taller than the screen and then centred on it — which puts the
// title bar, and with it the minimise, maximise and close buttons, above the
// top edge where the pointer cannot reach them (owner, 16 ส.ค., on a VivoBook).
//
// The taskbar has to come out of the measurement, not just the screen. Centring
// is done against the work area, so a window merely as tall as the screen is
// still pushed halfway a taskbar's height off the top. systemChrome is what
// asks the desktop how much it keeps.
//
// The minimums move down with the window. SetSize clamps against them before it
// does anything else, so leaving a 700px floor under a 672px work area would
// put the window straight back where it was. They are only ever lowered to what
// the screen allows: the floor exists because the cockpit's three columns stop
// making sense below it, and nothing here decides it is not needed.
//
// Only ever shrinks. On a monitor with room the window opens at the size the
// app asked for rather than filled to the corners.
//
// ARCHITECTURE.md §117.
func (a *App) fitToScreen() {
	screen, ok := currentScreen(a.ctx)
	if !ok {
		return
	}
	chromeW, chromeH := systemChrome(screen)
	width, height, shrunk := fitWindow(screen.Size.Width-chromeW, screen.Size.Height-chromeH)
	if !shrunk {
		return
	}
	debuglog.Msg("fitToScreen: screen %dx%d, chrome %dx%d, window %dx%d",
		screen.Size.Width, screen.Size.Height, chromeW, chromeH, width, height)
	runtime.WindowSetMinSize(a.ctx, min(windowMinWidth, width), min(windowMinHeight, height))
	runtime.WindowSetSize(a.ctx, width, height)
	// Re-centre: the window was centred at its old size while it was still
	// bigger than the screen, and resizing keeps the top-left corner it had.
	runtime.WindowCenter(a.ctx)
}

// fitWindow is the arithmetic of the note above, kept apart from the runtime
// calls so it can be checked against a screen nobody has to own. availW and
// availH are the work area — screen minus whatever the desktop keeps.
//
// A screen so small that the work area comes back at zero or below is not
// something to divide the window by: the size is left alone and shrunk reports
// false, because a 0x0 window is worse than one whose title bar is off-screen.
func fitWindow(availW, availH int) (width, height int, shrunk bool) {
	if availW <= 0 || availH <= 0 {
		return windowWidth, windowHeight, false
	}
	width = min(windowWidth, availW)
	height = min(windowHeight, availH)
	return width, height, width != windowWidth || height != windowHeight
}

// currentScreen is the display the window opened on. IsCurrent is what the
// window manager says the window is on; the primary display is the fallback for
// a driver that answers neither, and the first screen for one that answers
// nothing — measuring against some screen beats measuring against none, and a
// wrong guess here can only shrink the window.
func currentScreen(ctx context.Context) (runtime.Screen, bool) {
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return runtime.Screen{}, false
	}
	best := screens[0]
	for _, s := range screens {
		if s.IsCurrent {
			best = s
			break
		}
		if s.IsPrimary {
			best = s
		}
	}
	if best.Size.Width <= 0 || best.Size.Height <= 0 {
		return runtime.Screen{}, false
	}
	return best, true
}
