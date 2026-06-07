package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type writeSkill struct {
	root string
}

func (*writeSkill) Name() string { return "write" }

func (*writeSkill) Description() string {
	return "create/overwrite a file in sandbox root"
}

func (s *writeSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("write skill unavailable")
		return newToolOutput("write", "write", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) < 2 {
		err := errors.New("usage: write <path> <content>")
		return newToolOutput("write", "write", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(args[0])
	content := strings.Join(args[1:], " ")
	if requestPath == "" {
		err := errors.New("usage: write <path> <content>")
		return newToolOutput("write", "write "+strings.TrimSpace(strings.Join(args, " ")), "", start, false, err), err
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return newToolOutput("write", "write "+requestPath, "", start, false, err), err
	}

	output := "write done: " + filepath.ToSlash(targetPath)
	return newToolOutput("write", "write "+requestPath, output, start, false, nil), nil
}

func ensureWriteDir(targetPath string) error {
	dir := filepath.Dir(targetPath)
	if dir == "." {
		return nil
	}
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return errors.New("parent path is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}
