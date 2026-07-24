//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

func setUTF8Console() {
	const utf8CP = 65001
	_ = windows.SetConsoleOutputCP(utf8CP)
	_ = windows.SetConsoleCP(utf8CP)
}
