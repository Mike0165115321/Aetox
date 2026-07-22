package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTestDataRoot isolates a test to its own directory via AETOX_DATA_ROOT
// (internal/config.DataRoot's override) — the same mechanism
// desktop/wails-dev.bat uses for dev, dogfooded here for test isolation
// instead of the audit package needing its own separate isolation trick.
func setTestDataRoot(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", dir)
}

func readAuditEntries(t *testing.T, path string) []ShellEntry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read audit log %s: %v", path, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	entries := make([]ShellEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry ShellEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSONL line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestWriteShell_WritesJSONLEntry(t *testing.T) {
	setTestDataRoot(t, t.TempDir())

	entry := ShellEntry{
		Time:       "2026-06-09T14:00:00+07:00",
		Command:    "echo hello",
		WorkDir:    "/tmp/test",
		Success:    true,
		DurationMs: 42,
	}

	if err := WriteShell(entry); err != nil {
		t.Fatalf("WriteShell() unexpected error: %v", err)
	}

	path, err := ShellAuditLogPath()
	if err != nil {
		t.Fatalf("ShellAuditLogPath() error: %v", err)
	}

	entries := readAuditEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Command != "echo hello" {
		t.Fatalf("command: want %q got %q", "echo hello", entries[0].Command)
	}
	if entries[0].Success != true {
		t.Fatalf("success: want true got %v", entries[0].Success)
	}
	if entries[0].DurationMs != 42 {
		t.Fatalf("duration: want 42 got %d", entries[0].DurationMs)
	}
}

func TestWriteShell_CreatesDirectoryWhenMissing(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "aetox-data")
	setTestDataRoot(t, dataRoot)

	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("expected data root to not exist yet")
	}

	entry := ShellEntry{
		Command: "echo test",
		WorkDir: "/tmp",
		Success: true,
	}

	if err := WriteShell(entry); err != nil {
		t.Fatalf("WriteShell() unexpected error: %v", err)
	}

	info, err := os.Stat(dataRoot)
	if err != nil {
		t.Fatalf("data root not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("data root is not a directory")
	}
}

func TestWriteShell_RecordsFailedCommand(t *testing.T) {
	setTestDataRoot(t, t.TempDir())

	entry := ShellEntry{
		Command:    "rm -rf /nonexistent",
		WorkDir:    "/tmp",
		Success:    false,
		DurationMs: 3,
		Error:      "exit status 1",
	}

	if err := WriteShell(entry); err != nil {
		t.Fatalf("WriteShell() unexpected error: %v", err)
	}

	path, err := ShellAuditLogPath()
	if err != nil {
		t.Fatalf("ShellAuditLogPath() error: %v", err)
	}

	entries := readAuditEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Success != false {
		t.Fatalf("success: want false got %v", entries[0].Success)
	}
	if entries[0].Error != "exit status 1" {
		t.Fatalf("error: want %q got %q", "exit status 1", entries[0].Error)
	}
}

func TestWriteShell_AppendsMultipleEntries(t *testing.T) {
	setTestDataRoot(t, t.TempDir())

	first := ShellEntry{Command: "echo one", WorkDir: "/tmp", Success: true}
	second := ShellEntry{Command: "echo two", WorkDir: "/tmp", Success: true}

	if err := WriteShell(first); err != nil {
		t.Fatalf("first WriteShell() error: %v", err)
	}
	if err := WriteShell(second); err != nil {
		t.Fatalf("second WriteShell() error: %v", err)
	}

	path, err := ShellAuditLogPath()
	if err != nil {
		t.Fatalf("ShellAuditLogPath() error: %v", err)
	}

	entries := readAuditEntries(t, path)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Command != "echo one" {
		t.Fatalf("first command: want %q got %q", "echo one", entries[0].Command)
	}
	if entries[1].Command != "echo two" {
		t.Fatalf("second command: want %q got %q", "echo two", entries[1].Command)
	}
}

func TestShellAuditLogPath_UsesDataRoot(t *testing.T) {
	dataRoot := t.TempDir()
	setTestDataRoot(t, dataRoot)

	path, err := ShellAuditLogPath()
	if err != nil {
		t.Fatalf("ShellAuditLogPath() error: %v", err)
	}

	expected := filepath.Join(dataRoot, "shell-audit.log")
	if path != expected {
		t.Fatalf("path: want %q got %q", expected, path)
	}
}

func TestWriteShell_AutoTimeField(t *testing.T) {
	setTestDataRoot(t, t.TempDir())

	entry := ShellEntry{
		Command: "echo auto-time",
		WorkDir: "/tmp",
		Success: true,
	}
	if entry.Time != "" {
		t.Fatalf("expected empty time before write")
	}

	if err := WriteShell(entry); err != nil {
		t.Fatalf("WriteShell() error: %v", err)
	}

	path, err := ShellAuditLogPath()
	if err != nil {
		t.Fatalf("ShellAuditLogPath() error: %v", err)
	}

	entries := readAuditEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Time == "" {
		t.Fatalf("expected time to be auto-filled")
	}
}

func TestSanitizeCommand_ReturnsTrimmed(t *testing.T) {
	result := sanitizeCommand("  echo hello  ")
	if result != "echo hello" {
		t.Fatalf("expected %q got %q", "echo hello", result)
	}
}
