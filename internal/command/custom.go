package command

// Custom slash commands, Claude Code style: a file <DataRoot>/commands/foo.md
// makes "/foo <args>" expand into the file's content, with "$ARGUMENTS"
// replaced by everything after the name (or the args appended when the file
// never mentions $ARGUMENTS). Built-in commands always win — callers must try
// their own grammar first and only then ExpandCustom.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// CustomCommand is one discovered command file, for management UIs.
type CustomCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"` // first non-empty line of the file
	Path        string `json:"path"`
}

// CustomCommandsDir returns <DataRoot>/commands (not created here).
func CustomCommandsDir() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "commands"), nil
}

// ListCustom reports every commands/*.md file. A missing directory is just an
// empty list.
func ListCustom() []CustomCommand {
	dir, err := CustomCommandsDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []CustomCommand
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, CustomCommand{
			Name:        strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
			Description: firstLine(string(raw)),
			Path:        path,
		})
	}
	return out
}

// ExpandCustom checks whether input invokes a custom command and returns the
// expanded prompt. Anything else — no leading slash, unknown name, unreadable
// file — returns the input unchanged with ok=false.
func ExpandCustom(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return input, false
	}
	name, args, _ := strings.Cut(trimmed[1:], " ")
	name = strings.TrimSpace(name)
	args = strings.TrimSpace(args)
	if name == "" || strings.ContainsAny(name, `\/`) {
		return input, false
	}
	dir, err := CustomCommandsDir()
	if err != nil {
		return input, false
	}
	raw, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		return input, false
	}
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return input, false
	}
	if strings.Contains(body, "$ARGUMENTS") {
		return strings.ReplaceAll(body, "$ARGUMENTS", args), true
	}
	if args != "" {
		return body + "\n\n" + args, true
	}
	return body, true
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(strings.TrimLeft(line, "#- ")); line != "" {
			if len([]rune(line)) > 120 {
				return string([]rune(line)[:120]) + "…"
			}
			return line
		}
	}
	return ""
}
