package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
)

type ShellEntry struct {
	Time       string `json:"time"`
	Command    string `json:"command"`
	WorkDir    string `json:"workdir"`
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

func ShellAuditLogPath() (string, error) {
	dir, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("audit: cannot create audit directory %s: %w", dir, err)
	}
	return filepath.Join(dir, "shell-audit.log"), nil
}

func WriteShell(entry ShellEntry) error {
	path, err := ShellAuditLogPath()
	if err != nil {
		return err
	}

	entry.Command = sanitizeCommand(entry.Command)
	entry.WorkDir = strings.TrimSpace(entry.WorkDir)
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit: cannot marshal shell entry: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: cannot open audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("audit: cannot write audit entry: %w", err)
	}
	return nil
}

func sanitizeCommand(command string) string {
	return strings.TrimSpace(command)
}
