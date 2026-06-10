package debuglog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	writer io.Writer
	indent int
	logDir string
)

func Init(baseDir string) {
	if writer != nil {
		return
	}
	dir := filepath.Join(baseDir, "logs")
	os.MkdirAll(dir, 0755)
	logDir = dir

	name := "aetox-" + time.Now().Format("20060102-150405") + ".log"
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	writer = f
	timestamp("=== AETOX DEBUG LOG ===")
	timestamp("file: " + path)
}

func Enable(filepath string) error {
	if writer != nil {
		_ = Disable()
	}
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	writer = f
	timestamp("=== AETOX DEBUG LOG ===")
	timestamp("file: " + filepath)
	return nil
}

func Disable() error {
	if writer == nil {
		return nil
	}
	var err error
	if closer, ok := writer.(io.Closer); ok {
		err = closer.Close()
	}
	writer = nil
	return err
}

func IsEnabled() bool {
	return writer != nil
}

func LogDir() string {
	return logDir
}

func Msg(format string, args ...any) {
	if writer == nil {
		return
	}
	prefix := strings.Repeat("  ", indent)
	line := prefix + fmt.Sprintf(format, args...)
	timestamp(line)
}

func Block(title string) func() {
	if writer == nil {
		return func() {}
	}
	timestamp(strings.Repeat("  ", indent) + "=== " + title + " ===")
	indent++
	return func() {
		if indent > 0 {
			indent--
		}
		timestamp(strings.Repeat("  ", indent) + "--- " + title + " ---")
	}
}

func Info(label, value string) {
	Msg("%-20s = %s", label+":", value)
}

func timestamp(msg string) {
	fmt.Fprintf(writer, "[%s] %s\n", time.Now().Format("15:04:05.000"), msg)
}
