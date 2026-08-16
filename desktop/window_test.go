package main

import "testing"

// The laptop the bug was reported from is the first case: 1920x1080 at 150%
// scaling is 1280x720 in the units Wails sizes windows in, and Windows' taskbar
// takes 48 of the height. The window has to come down to fit that, and the
// floor has to come down with it — a 700px minimum on a 672px work area is the
// same window back off the top of the screen.
func TestFitWindow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		availW        int
		availH        int
		width, height int
		shrunk        bool
	}{
		{"vivobook 1080p at 150%", 1280, 672, 1280, 672, true},
		{"height alone is short", 1440, 800, 1440, 800, true},
		{"width alone is narrow", 1366, 900, 1366, 900, true},
		{"room to spare stays put", 1920, 1080, 1440, 900, false},
		{"exactly the asked-for size stays put", 1440, 900, 1440, 900, false},
		{"no work area reported", 0, 0, 1440, 900, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			width, height, shrunk := fitWindow(tc.availW, tc.availH)
			if width != tc.width || height != tc.height || shrunk != tc.shrunk {
				t.Errorf("fitWindow(%d, %d) = %d, %d, %v; want %d, %d, %v",
					tc.availW, tc.availH, width, height, shrunk, tc.width, tc.height, tc.shrunk)
			}
		})
	}
}

// The minimum is lowered to the fitted window and no further: it exists because
// the cockpit's three columns stop making sense under it, so a screen with room
// keeps it whole.
func TestFitWindowLowersFloorOnlyAsFarAsNeeded(t *testing.T) {
	width, height, _ := fitWindow(1280, 672)
	if got := min(windowMinHeight, height); got != 672 {
		t.Errorf("minimum height on a 672px work area = %d; want 672", got)
	}
	if got := min(windowMinWidth, width); got != windowMinWidth {
		t.Errorf("minimum width on a 1280px work area = %d; want %d (untouched)", got, windowMinWidth)
	}
}
