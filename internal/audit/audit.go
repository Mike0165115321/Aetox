package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/debuglog"
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

	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("audit: cannot write audit entry: %w", err)
	}
	// Closed here rather than deferred, and its error is the return value: a
	// write reports success as soon as the bytes are handed over, and the flush
	// that actually puts them on disk can still fail — full disk, a network
	// path. Of everything in this repo, an audit line that silently did not
	// land is the worst one to be wrong about.
	if err := f.Close(); err != nil {
		return fmt.Errorf("audit: cannot flush audit entry: %w", err)
	}
	return nil
}

// sanitizeCommand is the last thing between a shell command and a file that
// outlives the session. A command line is one of the likeliest places a
// credential appears in the clear — `curl -H "Authorization: Bearer …"`, a
// token passed as a flag, a key echoed into a config — and this log is append
// only and never rotated, so anything that lands here stays.
//
// The registry is debuglog's because it is already the one place secrets are
// registered (config.LoadCredentials, config.Load). A second list kept here
// would drift, and the drift would stay invisible until someone read the file
// it failed on.
func sanitizeCommand(command string) string {
	return debuglog.Scrub(strings.TrimSpace(command))
}
