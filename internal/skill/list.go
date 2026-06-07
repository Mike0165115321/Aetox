package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type listSkill struct {
	root string
}

func (*listSkill) Name() string { return "list" }

func (*listSkill) Description() string {
	return "แสดงรายชื่อไฟล์ใน sandbox root หรือ subpath"
}

func (s *listSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		return newToolOutput("list", "list", "", start, false, fmt.Errorf("list skill unavailable")), fmt.Errorf("list skill unavailable")
	}

	args := stringSlice(input["args"])
	requestPath := "."
	if len(args) > 0 {
		requestPath = strings.Join(args, " ")
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("list", "list "+requestPath, "", start, false, err), err
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return newToolOutput("list", "list "+requestPath, "", start, false, err), err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	output, truncated := limitLines(strings.Join(names, "\n"), defaultToolOutputLineLimit)
	command := "list"
	if requestPath != "" && requestPath != "." {
		command = "list " + requestPath
	}
	return newToolOutput("list", command, output, start, truncated, nil), nil
}

func resolveSandboxPath(root string, requestPath string) (string, error) {
	safeRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "."
	}
	if filepath.IsAbs(requestPath) {
		return "", fmt.Errorf("absolute path is not allowed")
	}

	candidate := filepath.Join(safeRoot, requestPath)
	normalized := filepath.Clean(candidate)
	safeTarget, err := filepath.Abs(normalized)
	if err != nil {
		return "", err
	}

	if safeTarget != safeRoot && !strings.HasPrefix(safeTarget+string(filepath.Separator), safeRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside sandbox root")
	}
	return safeTarget, nil
}
