package turn

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/rtk"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// These tests exercise the actual integration seam (ARCHITECTURE.md §13):
// modelToolReceipt calling into internal/rtk, not internal/rtk's own logic
// (already covered by internal/rtk/rtk_test.go). Skipped when rtk isn't
// installed — same pattern as internal/rtk's own live tests.

func TestModelToolReceipt_GitStatusIsFilteredWhenRTKAvailable(t *testing.T) {
	if !rtk.Available() {
		t.Skip("rtk not installed on PATH")
	}
	e := NewExecutor(ExecutorOptions{})
	raw := "On branch main\nYour branch is up to date with 'origin/main'.\n\nnothing to commit, working tree clean\n"

	receipt := e.modelToolReceipt(
		"git",
		map[string]any{"args": []any{"status"}},
		skill.Output{Success: true, RawOutput: raw, Command: "git status"},
		nil,
	)

	var decoded map[string]any
	if err := json.Unmarshal([]byte(receipt), &decoded); err != nil {
		t.Fatalf("receipt is not valid JSON: %v\nreceipt: %s", err, receipt)
	}
	output, _ := decoded["output"].(string)
	if output == "" {
		t.Fatal("receipt output is empty")
	}
	if output == strings.TrimSpace(raw) {
		t.Errorf("expected rtk to filter git-status output, but it passed through unchanged: %q", output)
	}
}

func TestModelToolReceipt_UnmappedToolPassesThroughUnfiltered(t *testing.T) {
	// "read" has no rtk mapping (ARCHITECTURE.md §13.4) — must never be
	// touched, regardless of whether rtk is installed on this machine.
	e := NewExecutor(ExecutorOptions{})
	raw := "package main\n\nfunc main() {}\n"

	receipt := e.modelToolReceipt(
		"read",
		map[string]any{"path": "main.go"},
		skill.Output{Success: true, RawOutput: raw, Command: "read main.go"},
		nil,
	)

	var decoded map[string]any
	if err := json.Unmarshal([]byte(receipt), &decoded); err != nil {
		t.Fatalf("receipt is not valid JSON: %v\nreceipt: %s", err, receipt)
	}
	output, _ := decoded["output"].(string)
	if output != strings.TrimSpace(raw) {
		t.Errorf("expected unmapped tool output unchanged, got %q, want %q", output, strings.TrimSpace(raw))
	}
}
