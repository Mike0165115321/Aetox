package app

import (
	"strconv"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// /shell — which shell the agent's commands run in: this machine's own, or a
// WSL distro. The CLI's half of the desktop's composer chip, over the same
// per-project store, so a choice made in one surface is what the other finds.
//
// Bare `/shell` lists and asks, the way `/approval` does, because the thing
// being picked has a name the user has to see spelled correctly — "mikedev" is
// not something to make anyone type from memory. `/shell wsl mikedev` and
// `/shell windows` skip the list for someone who already knows.

func (a *App) handleShellCommand(line string) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 {
		a.pickShell()
		return
	}
	a.applyShell(strings.Join(parts[1:], " "))
}

// shellOptions is the native shell plus every distro, which is the same list
// the desktop picker shows and in the same order.
func (a *App) shellOptions() []proc.Backend {
	options := []proc.Backend{proc.Native()}
	for _, distro := range proc.Distros() {
		options = append(options, proc.WSL(distro))
	}
	return options
}

func (a *App) currentShell() proc.Backend {
	if a.shells == nil {
		return proc.Native()
	}
	return a.shells.For(a.shellRoot)
}

func (a *App) pickShell() {
	options := a.shellOptions()
	current := proc.FormatBackend(a.currentShell())

	a.console.Println("เลือกเชลล์ที่ใช้รันคำสั่ง (ปัจจุบัน: " + proc.Label(a.currentShell()) + "):")
	for i, option := range options {
		mark := " "
		if strings.EqualFold(proc.FormatBackend(option), current) {
			mark = "*"
		}
		a.console.Printf("  %s %d) %s\n", mark, i+1, proc.Label(option))
	}
	if len(options) == 1 {
		// Not a menu at all — say why rather than asking for a choice between
		// one thing. A machine with no WSL is the common case, not a fault.
		a.console.Println("เครื่องนี้มีเชลล์เดียว — ติดตั้ง WSL ถึงจะมีให้เลือก")
		return
	}
	a.console.Printf("เลือก [1-%d]: ", len(options))

	answer, err := a.console.ReadLine()
	if err != nil {
		return
	}
	index, convErr := strconv.Atoi(strings.TrimSpace(answer))
	if convErr != nil || index < 1 || index > len(options) {
		a.console.Println("ยกเลิก")
		return
	}
	a.setShell(options[index-1])
}

// applyShell reads the words a user would actually type. "windows" and "cmd"
// are accepted for the native shell because that is what it is called on the
// machine this runs on, whatever proc calls it internally.
func (a *App) applyShell(arg string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(arg)))
	switch fields[0] {
	case "windows", "native", "cmd", "sh", "เครื่อง", "วินโดว์":
		a.setShell(proc.Native())
	case "wsl", "linux":
		distro := strings.Join(fields[1:], " ")
		if distro == "" {
			// No name given: fine when there is exactly one distro, and a
			// question when there is more than one — picking for the user
			// there would silently run their commands in the wrong place.
			available := proc.Distros()
			switch len(available) {
			case 0:
				a.console.Println("เครื่องนี้ไม่มี WSL distro")
				return
			case 1:
				distro = available[0]
			default:
				a.console.Println("ระบุ distro ด้วย เช่น /shell wsl " + available[0])
				a.pickShell()
				return
			}
		}
		a.setShell(proc.WSL(distro))
	default:
		a.console.Println("ไม่รู้จัก ใช้: /shell windows หรือ /shell wsl <distro>")
		a.pickShell()
	}
}

func (a *App) setShell(backend proc.Backend) {
	if a.shells == nil {
		a.console.Println("เซสชันนี้เปลี่ยนเชลล์ไม่ได้")
		return
	}
	if err := a.shells.Set(a.shellRoot, proc.FormatBackend(backend)); err != nil {
		a.console.Errorf("บันทึกไม่สำเร็จ: %v\n", err)
		return
	}
	// Read back rather than echoing what was asked for: "wsl" without a name
	// resolves to a distro, and the user should see which one they got.
	a.console.Println("คำสั่งถัดไปจะรันด้วย: " + proc.Label(a.currentShell()))
}
