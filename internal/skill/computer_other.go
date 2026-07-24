//go:build !windows

package skill

import (
	"context"
	"errors"
)

var errComputerUnsupported = errors.New("สกิล computer รองรับเฉพาะ Windows ตอนนี้ (เดสก์ท็อป Aetox เป็น Windows-only)")

func computerScreenInfo() (int, int, int, int, error) { return 0, 0, 0, 0, errComputerUnsupported }

func computerMouseMove(int, int) error { return errComputerUnsupported }

func computerClick(int, int, string) error { return errComputerUnsupported }

func computerType(string) error { return errComputerUnsupported }

func computerKey(string) error { return errComputerUnsupported }

func computerScreenshot(context.Context, string) error { return errComputerUnsupported }
