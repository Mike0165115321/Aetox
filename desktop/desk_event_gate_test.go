package main

// The desk's one door, enforced (§187.3).
//
// §187's leak was possible because every tool emitted its own workbench:*
// event and the ownership question was asked nowhere. The door (desk_events.go)
// is where it is asked now — and a door beside an open wall is decoration, so
// this test reads the package's own source and fails any workbench:* emission
// that does not go through it. The next desk surface has to answer "whose
// desk" on the day it is written.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryWorkbenchEventLeavesThroughTheDeskDoor(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "desk_events.go" {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, `emitEvent("workbench:`) {
				t.Errorf("%s:%d emits a workbench event directly — route it through deskEvent (desk_events.go), which is where \"whose desk\" gets asked", name, i+1)
			}
		}
	}
}
