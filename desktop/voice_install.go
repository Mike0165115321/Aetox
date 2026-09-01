package main

// The ติดตั้ง button on ตั้งค่า > เสียง. Before this, a vendor that needed a
// pip package was a red sentence with a command buried mid-prose, and the user
// went to a terminal and came back guessing whether the app had noticed.
//
// The rule that makes the button honest: it runs the catalog's InstallCommand
// and NOTHING else — the same argv the page displays, never a string the
// frontend composed. A webview that can hand this binding an arbitrary command
// is a webview that can run anything; one that can only pick a catalog row is
// not.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/stt"
	"github.com/Mikedev115/Aetox/internal/tts"
)

// InstallVoiceEngine runs one vendor's install command. side names the catalog
// ("stt" or "tts"), id the Descriptor. Output streams out one line at a time
// as "voice:install" events ({side, line}) for the page's live tail; the
// return value is the verdict, as a sentence the page shows verbatim.
func (a *App) InstallVoiceEngine(side, id string) error {
	argv, err := voiceInstallArgv(side, id)
	if err != nil {
		return err
	}
	// Resolve the runner before spawning, so a machine without it gets a
	// sentence about the actual gap — pip missing means no Python, not a
	// cryptic exec error.
	if _, lookErr := exec.LookPath(argv[0]); lookErr != nil {
		switch argv[0] {
		case "pip":
			return fmt.Errorf("ไม่พบ pip ในเครื่อง ต้องติดตั้ง Python ก่อน (python.org) แล้วค่อยกดติดตั้งอีกครั้ง")
		case "scoop":
			return fmt.Errorf("ไม่พบ scoop ในเครื่อง ติดตั้งได้จาก scoop.sh แล้วค่อยกดติดตั้งอีกครั้ง")
		case "brew":
			return fmt.Errorf("ไม่พบ brew ในเครื่อง ติดตั้งได้จาก brew.sh แล้วค่อยกดติดตั้งอีกครั้ง")
		}
		return fmt.Errorf("ไม่พบโปรแกรม %s ในเครื่อง", argv[0])
	}
	// Ten minutes: pip on a cold cache over a slow connection is minutes, not
	// seconds — but nothing the user pressed once may run forever.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	proc.HideConsole(cmd)
	proc.KillOnCancel(cmd)

	// One pipe for both streams: pip narrates on stdout and errors on stderr,
	// and the page's single tail line wants them in arrival order.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	lastLine := ""
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			lastLine = line
			a.emitEvent("voice:install", map[string]string{"side": side, "line": line})
		}
	}()
	runErr := cmd.Run()
	pw.Close()
	<-done
	if runErr != nil {
		// The last line pip printed usually IS the reason; the page shows this
		// verbatim, so it must read as a sentence, not a stack.
		if lastLine != "" {
			return fmt.Errorf("ติดตั้งไม่สำเร็จ: %s", lastLine)
		}
		return fmt.Errorf("ติดตั้งไม่สำเร็จ: %v", runErr)
	}
	return nil
}

// voiceInstallArgv resolves (side, id) to the catalog's own command — the one
// place install argv may come from.
func voiceInstallArgv(side, id string) ([]string, error) {
	var argv []string
	var label string
	switch side {
	case "stt":
		desc, ok := stt.Lookup(id)
		if !ok {
			return nil, fmt.Errorf("ไม่รู้จัก engine ถอดเสียงชื่อ %q", id)
		}
		argv, label = desc.InstallCommand, desc.Label
	case "tts":
		desc, ok := tts.Lookup(id)
		if !ok {
			return nil, fmt.Errorf("ไม่รู้จัก engine เสียงอ่านชื่อ %q", id)
		}
		argv, label = desc.InstallCommand, desc.Label
	default:
		return nil, fmt.Errorf("ไม่รู้จักฝั่งเสียง %q", side)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("%s ไม่มีคำสั่งติดตั้งอัตโนมัติ", label)
	}
	return argv, nil
}
