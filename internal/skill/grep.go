package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
)

var errGrepLimitReached = errors.New("grep result limit reached")

type grepSkill struct {
	root string
}

func (*grepSkill) Name() string { return "grep" }

func (*grepSkill) Description() string {
	return "Search file contents under sandbox root with a regular expression"
}

func (*grepSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Go regular expression to search for. Prefix with (?i) for case-insensitive.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Relative file or directory to search (default: whole sandbox)",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "grep",
			Description: "Search file contents by regex; returns path:line:text matches.",
			Parameters:  payload,
		},
	}
}

func (s *grepSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("grep skill unavailable")
		return newToolOutput("grep", "grep", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		err := errors.New("usage: grep <pattern> [path]")
		return newToolOutput("grep", "grep", "", start, false, err), err
	}

	pattern := args[0]
	searchPath := "."
	if len(args) > 1 {
		searchPath = strings.TrimSpace(strings.Join(args[1:], " "))
	}
	command := "grep " + pattern
	if searchPath != "." {
		command += " " + searchPath
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}

	basePath, err := resolveSandboxPath(s.root, searchPath)
	if err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}
	root, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("grep", command, "", start, false, err), err
	}

	const (
		maxResults   = 200
		maxFileBytes = 1 << 20
		maxLineLen   = 200
	)
	results := make([]string, 0)

	walkErr := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// ponytail: skips all dot-dirs (.git, .cache, ...), allowlist if a dot-dir ever matters
			if name := d.Name(); strings.HasPrefix(name, ".") && path != basePath {
				return filepath.SkipDir
			}
			return nil
		}

		file, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxFileBytes))
		_ = file.Close()
		if readErr != nil || bytes.Contains(data, []byte{0}) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		display := filepath.ToSlash(path)
		if relErr == nil {
			display = filepath.ToSlash(rel)
		}

		for i, line := range strings.Split(string(data), "\n") {
			if !re.MatchString(line) {
				continue
			}
			line = strings.TrimRight(line, "\r")
			if len(line) > maxLineLen {
				line = line[:maxLineLen] + "..."
			}
			results = append(results, display+":"+strconv.Itoa(i+1)+":"+line)
			if len(results) >= maxResults {
				return errGrepLimitReached
			}
		}
		return nil
	})
	if errors.Is(walkErr, errGrepLimitReached) {
		walkErr = nil
	}
	if walkErr != nil {
		return newToolOutput("grep", command, "", start, false, walkErr), walkErr
	}

	output := strings.Join(results, "\n")
	if output == "" {
		output = "(no matches)"
	}
	output, truncated := limitLines(output, defaultToolOutputLineLimit)
	if len(results) >= maxResults {
		output += "\n... (max results reached)"
		truncated = true
	}

	return newToolOutput("grep", command, output, start, truncated, nil), nil
}

func (s *grepSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		err := errors.New("pattern is required")
		return newToolOutput("grep", "grep", "", time.Now(), false, err), err
	}
	callArgs := []string{pattern}
	if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
		callArgs = append(callArgs, strings.TrimSpace(path))
	}
	return s.Execute(ctx, Input{"args": callArgs})
}
