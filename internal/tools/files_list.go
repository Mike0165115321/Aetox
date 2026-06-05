package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"aetox-cli/internal/contracts"
)

type FileListTool struct {
	sandboxRoot string
}

func NewFileListTool(root string) *FileListTool {
	return &FileListTool{sandboxRoot: root}
}

func (f *FileListTool) Name() string {
	return "files"
}

func (f *FileListTool) Description() string {
	return "Filesystem operations for listing, reading, writing, moving, and deleting"
}

func (f *FileListTool) Actions() []string {
	return []string{"list", "read", "write", "move", "delete"}
}

func (f *FileListTool) Risk(action string, _ map[string]any) contracts.RiskLevel {
	switch action {
	case "read", "list":
		return contracts.RiskLow
	case "write", "move":
		return contracts.RiskMedium
	case "delete":
		return contracts.RiskHigh
	default:
		return contracts.RiskHigh
	}
}

func (f *FileListTool) Execute(ctx context.Context, action string, params map[string]any) (contracts.StepResult, error) {
	select {
	case <-ctx.Done():
		return contracts.StepResult{Status: contracts.StatusFailure, Error: ctx.Err().Error(), Confidence: 0.0}, ctx.Err()
	default:
	}

	switch strings.TrimSpace(strings.ToLower(action)) {
	case "list":
		return f.execList(ctx, params)
	case "read":
		return f.execRead(ctx, params)
	case "write":
		return f.execWrite(ctx, params)
	case "move":
		return f.execMove(ctx, params)
	case "delete":
		return f.execDelete(ctx, params)
	default:
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      "unsupported action: " + action,
			Artifacts:  map[string]string{},
			Confidence: 0.0,
		}, fmt.Errorf("unsupported action: %s", action)
	}
}

func (f *FileListTool) execList(_ context.Context, params map[string]any) (contracts.StepResult, error) {
	path := strParam(params, "path", ".")
	pattern := strParam(params, "pattern", "*")
	recursive := boolParam(params, "recursive", false)

	entries, err := listFiles(f.sandboxRoot, path, pattern, recursive)
	if err != nil {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      err.Error(),
			Artifacts:  map[string]string{},
			Confidence: 0.0,
		}, err
	}

	sort.Strings(entries)
	output := strings.Join(entries, "\n")
	if output == "" {
		output = "No files matched."
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     output,
		Artifacts:  map[string]string{"path": path, "pattern": pattern, "count": fmt.Sprint(len(entries))},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"matched_count": len(entries),
		},
	}, nil
}

func (f *FileListTool) execRead(_ context.Context, params map[string]any) (contracts.StepResult, error) {
	path := strParam(params, "path", "")
	if path == "" {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: "missing path"}, fmt.Errorf("missing path")
	}

	target, err := resolveSandboxPath(f.sandboxRoot, path)
	if err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}
	content := string(data)
	if content == "" {
		content = "(empty file)"
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     content,
		Artifacts:  map[string]string{"path": target, "bytes": fmt.Sprint(len(data))},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"last_read": target,
			"bytes":     len(data),
		},
	}, nil
}

func (f *FileListTool) execWrite(_ context.Context, params map[string]any) (contracts.StepResult, error) {
	path := strParam(params, "path", "")
	if path == "" {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: "missing path"}, fmt.Errorf("missing path")
	}
	content := strParam(params, "content", "")
	if content == "" {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: "missing content"}, fmt.Errorf("missing content")
	}

	target, err := resolveSandboxPath(f.sandboxRoot, path)
	if err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     fmt.Sprintf("wrote %d bytes to %s", len(content), target),
		Artifacts:  map[string]string{"path": target, "bytes": fmt.Sprint(len(content))},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"last_write": target,
			"bytes":      len(content),
		},
	}, nil
}

func (f *FileListTool) execMove(_ context.Context, params map[string]any) (contracts.StepResult, error) {
	source := strParam(params, "source", "")
	target := strParam(params, "target", "")
	if source == "" || target == "" {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      "missing source or target",
			Confidence: 0.0,
		}, fmt.Errorf("missing source or target")
	}

	sourcePath, err := resolveSandboxPath(f.sandboxRoot, source)
	if err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}
	targetPath, err := resolveSandboxPath(f.sandboxRoot, target)
	if err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}

	if err := os.Rename(sourcePath, targetPath); err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     fmt.Sprintf("moved %s -> %s", sourcePath, targetPath),
		Artifacts:  map[string]string{"source": sourcePath, "target": targetPath},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"last_move_source": sourcePath,
			"last_move_target": targetPath,
		},
	}, nil
}

func (f *FileListTool) execDelete(_ context.Context, params map[string]any) (contracts.StepResult, error) {
	path := strParam(params, "path", "")
	if path == "" {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: "missing path"}, fmt.Errorf("missing path")
	}
	target, err := resolveSandboxPath(f.sandboxRoot, path)
	if err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}
	if err := os.Remove(target); err != nil {
		return contracts.StepResult{Status: contracts.StatusFailure, Error: err.Error()}, err
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     fmt.Sprintf("deleted %s", target),
		Artifacts:  map[string]string{"path": target},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"last_delete": target,
		},
	}, nil
}

func listFiles(sandboxRoot, path, pattern string, recursive bool) ([]string, error) {
	target, err := resolveSandboxPath(sandboxRoot, path)
	if err != nil {
		return nil, err
	}

	if !recursive {
		entries, err := os.ReadDir(target)
		if err != nil {
			return nil, err
		}
		var matches []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			ok, err := filepath.Match(pattern, name)
			if err != nil {
				return nil, err
			}
			if ok {
				matches = append(matches, filepath.Join(target, name))
			}
		}
		return matches, nil
	}

	var matches []string
	err = filepath.WalkDir(target, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(p)
		ok, matchErr := filepath.Match(pattern, name)
		if matchErr != nil {
			return matchErr
		}
		if ok {
			matches = append(matches, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func resolveSandboxPath(sandboxRoot, path string) (string, error) {
	base := filepath.Clean(sandboxRoot)
	target := filepath.Clean(path)
	if !filepath.IsAbs(base) {
		absBase, err := filepath.Abs(base)
		if err != nil {
			return "", err
		}
		base = absBase
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, absTarget)
	if err != nil {
		return "", err
	}
	unsafe := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if unsafe {
		return "", fmt.Errorf("path escapes sandbox root: %s", path)
	}
	return absTarget, nil
}

func strParam(params map[string]any, key, fallback string) string {
	raw, ok := params[key]
	if !ok {
		return fallback
	}
	if value, ok := raw.(string); ok {
		return value
	}
	return fallback
}

func boolParam(params map[string]any, key string, fallback bool) bool {
	raw, ok := params[key]
	if !ok {
		return fallback
	}
	if value, ok := raw.(bool); ok {
		return value
	}
	return fallback
}

