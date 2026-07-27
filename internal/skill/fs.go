package skill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var errFindLimitReached = errors.New("find result limit reached")

type fsSkill struct {
	root string
}

func (*fsSkill) Name() string { return "fs" }

func (*fsSkill) Description() string {
	return "เครื่องมือจัดการไฟล์แบบอ่านอย่างเดียว: pwd, ls, find, cat"
}

func (s *fsSkill) Execute(_ context.Context, input Input) (Output, error) {
	start := time.Now()
	if s == nil {
		err := errors.New("fs skill unavailable")
		return newToolOutput("fs", "fs", "", start, false, err), err
	}

	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: fs <pwd|ls|find|cat> [args]")
		return newToolOutput("fs", "fs", "", start, false, err), err
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	params := args[1:]

	switch action {
	case "pwd":
		return s.execPwd(start)
	case "ls":
		return s.execLs(start, params)
	case "find":
		return s.execFind(start, params)
	case "cat":
		return s.execCat(start, params)
	default:
		err := errors.New("unsupported fs action: " + action)
		return newToolOutput("fs", "fs "+strings.Join(args, " "), "", start, false, err), err
	}
}

func (s *fsSkill) execPwd(start time.Time) (Output, error) {
	root, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("fs", "fs pwd", "", start, false, err), err
	}
	return newToolOutput("fs", "fs pwd", root, start, false, nil), nil
}

func (s *fsSkill) execLs(start time.Time, params []string) (Output, error) {
	requestPath := "."
	if len(params) > 0 {
		requestPath = strings.Join(params, " ")
	}
	command := "fs ls"
	if requestPath != "" && requestPath != "." {
		command = "fs ls " + requestPath
	}

	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	content, truncated := limitLines(strings.Join(names, "\n"), defaultToolOutputLineLimit)
	return newToolOutput("fs", command, content, start, truncated, nil), nil
}

func (s *fsSkill) execFind(start time.Time, params []string) (Output, error) {
	if len(params) == 0 {
		err := errors.New("usage: fs find <pattern> [path]")
		return newToolOutput("fs", "fs find", "", start, false, err), err
	}

	pattern := strings.TrimSpace(params[0])
	if pattern == "" {
		err := errors.New("find pattern is empty")
		return newToolOutput("fs", "fs find", "", start, false, err), err
	}
	if strings.ContainsAny(pattern, "*?[") {
		err := errors.New("glob-style patterns are not allowed")
		return newToolOutput("fs", "fs find", "", start, false, err), err
	}

	searchPath := "."
	if len(params) > 1 {
		searchPath = strings.TrimSpace(strings.Join(params[1:], " "))
	}

	basePath, err := resolveSandboxPath(s.root, searchPath)
	if err != nil {
		return newToolOutput("fs", "fs find "+pattern+" "+searchPath, "", start, false, err), err
	}
	root, err := resolveSandboxPath(s.root, ".")
	if err != nil {
		return newToolOutput("fs", "fs find "+pattern+" "+searchPath, "", start, false, err), err
	}

	needle := strings.ToLower(pattern)
	results := make([]string, 0)
	maxResults := 200
	maxBytes := 4096

	walkErr := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Same set grep refuses to search: a file search that returns 200
			// hits from node_modules has answered a question nobody asked.
			if name := d.Name(); path != basePath && (strings.HasPrefix(name, ".") || IgnoredDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.Contains(name, needle) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr == nil {
			results = append(results, filepath.ToSlash(rel))
		} else {
			results = append(results, filepath.ToSlash(path))
		}

		if len(results) >= maxResults {
			return errFindLimitReached
		}
		return nil
	})

	if errors.Is(walkErr, errFindLimitReached) {
		walkErr = nil
	}
	if walkErr != nil {
		return newToolOutput("fs", "fs find "+pattern+" "+searchPath, "", start, false, walkErr), walkErr
	}

	sort.Strings(results)
	output := strings.Join(results, "\n")
	if output == "" {
		output = "(no matches)"
	}
	output, truncated := limitLines(output, defaultToolOutputLineLimit)
	if len(results) >= maxResults {
		output += "\n... (max results reached)"
		truncated = true
	}
	if len(output) > maxBytes {
		output = output[:maxBytes] + "\n... (truncated)"
		truncated = true
	}

	return newToolOutput("fs", "fs find "+pattern+" "+searchPath, output, start, truncated, nil), nil
}

func (s *fsSkill) execCat(start time.Time, params []string) (Output, error) {
	if len(params) == 0 {
		err := errors.New("usage: fs cat <path>")
		return newToolOutput("fs", "fs cat", "", start, false, err), err
	}

	requestPath := strings.TrimSpace(strings.Join(params, " "))
	command := "fs cat " + requestPath
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}
	if info.IsDir() {
		err = errors.New("cat target is a directory")
		return newToolOutput("fs", command, "", start, false, err), err
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}
	defer func() {
		_ = file.Close()
	}()

	// Same line-paging as the read tool (read.go) rather than a second byte
	// ceiling: one truncation rule, and the model can page past it here too.
	binary, err := looksBinary(file)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}
	if binary {
		err = errors.New("read target is a binary file — there is no text to read")
		return newToolOutput("fs", command, "", start, false, err), err
	}

	content, next, err := readTextLines(file, 1, readDefaultLines, false)
	if err != nil {
		return newToolOutput("fs", command, "", start, false, err), err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		content = "(no output)"
	}
	if next > 0 {
		content += fmt.Sprintf("\n... (truncated — read %s with offset=%d for more)", requestPath, next)
	}
	return newToolOutput("fs", command, content, start, next > 0, nil), nil
}
