package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"aetox-cli/internal/command"

	"golang.org/x/term"
)

type rawKeyEvent int

const (
	rawKeyUnknown rawKeyEvent = iota
	rawKeyEnter
	rawKeyBackspace
	rawKeyTab
	rawKeyArrowUp
	rawKeyArrowDown
	rawKeyCtrlC
	rawKeyEscape
	rawKeyRune
)

type rawKey struct {
	kind rawKeyEvent
	r    rune
}

const (
	slashMetaSuggestionColor = "\x1b[38;5;214m"
	slashToolSuggestionColor = "\x1b[38;5;33m"
	slashMetaSwatch          = "\x1b[48;5;214m  \x1b[0m"
	slashToolSwatch          = "\x1b[48;5;33m  \x1b[0m"
)

type slashSuggestion struct {
	Token       string
	Category    string
	Description string
}

func (a *App) readLineInteractive(ctx context.Context) (string, error) {
	if ctx == nil || !isTTYForInput() {
		return a.console.ReadLine()
	}

	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return a.console.ReadLine()
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), state)
	}()

	reader := bufio.NewReader(os.Stdin)
	line := []rune{}
	selected := -1

	render := func() {
		input := string(line)
		suggestions := a.slashSuggestions(input)
		if len(suggestions) == 0 {
			selected = -1
		}
		if selected >= len(suggestions) {
			selected = 0
		}
		a.drawLineWithSlashPalette(input, suggestions, selected)
	}

	render()

	for {
		key, err := awaitRawKey(ctx, reader)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
				return "", io.EOF
			}
			return "", err
		}

		switch key.kind {
		case rawKeyEnter:
			input := string(line)
			if a.shouldClearPaletteOnSubmit(input) {
				suggestionCount := len(a.slashSuggestions(input))
				a.clearSlashPaletteBlock(suggestionCount + 1)
			}
			a.console.Print("\n")
			return input, nil
		case rawKeyCtrlC, rawKeyEscape:
			return "", io.EOF
		case rawKeyBackspace:
			if len(line) > 0 {
				line = line[:len(line)-1]
			}
			selected = -1
			render()
		case rawKeyArrowUp, rawKeyArrowDown:
			input := string(line)
			if !a.isSlashToken(input) {
				continue
			}
			suggestions := a.slashSuggestions(input)
			if len(suggestions) == 0 {
				continue
			}

			if selected == -1 {
				selected = 0
			} else if key.kind == rawKeyArrowUp {
				selected--
				if selected < 0 {
					selected = len(suggestions) - 1
				}
			} else {
				selected++
				if selected >= len(suggestions) {
					selected = 0
				}
			}
			render()
		case rawKeyTab:
			input := string(line)
			if !a.isSlashToken(input) {
				continue
			}

			suggestions := a.slashSuggestions(input)
			if len(suggestions) == 0 {
				continue
			}

			if selected == -1 {
				selected = 0
			}

			selectedSuggestion := suggestions[selected]
			line = []rune(selectedSuggestion.Token)
			if !command.IsMetaSlashCommand(slashCommandNameFromToken(selectedSuggestion.Token)) {
				line = append(line, ' ')
			}
			selected = -1
			render()
		case rawKeyRune:
			line = append(line, key.r)
			selected = -1
			render()
		}
	}
}

func (a *App) shouldClearPaletteOnSubmit(input string) bool {
	if !a.isSlashToken(input) {
		return false
	}
	command := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/")))
	return command == "model"
}

func (a *App) clearSlashPaletteBlock(lines int) {
	if lines <= 0 || !supportsANSI() {
		return
	}
	a.console.Printf("\r\x1b[%dA\x1b[J", lines)
}

func awaitRawKey(ctx context.Context, reader *bufio.Reader) (rawKey, error) {
	result := make(chan rawKey, 1)
	errCh := make(chan error, 1)

	go func() {
		key, err := readRawKey(reader)
		if err != nil {
			errCh <- err
			return
		}
		result <- key
	}()

	select {
	case <-ctx.Done():
		return rawKey{}, context.Canceled
	case err := <-errCh:
		return rawKey{}, err
	case key := <-result:
		return key, nil
	}
}

func readRawKey(reader *bufio.Reader) (rawKey, error) {
	ch, _, err := reader.ReadRune()
	if err != nil {
		return rawKey{}, err
	}

	switch ch {
	case 0xE0:
		next, _, err := reader.ReadRune()
		if err != nil {
			return rawKey{}, err
		}
		switch next {
		case 'H':
			return rawKey{kind: rawKeyArrowUp}, nil
		case 'P':
			return rawKey{kind: rawKeyArrowDown}, nil
		default:
			return rawKey{kind: rawKeyUnknown}, nil
		}
	case 0x00:
		next, _, err := reader.ReadRune()
		if err != nil {
			return rawKey{}, err
		}
		switch next {
		case 'H':
			return rawKey{kind: rawKeyArrowUp}, nil
		case 'P':
			return rawKey{kind: rawKeyArrowDown}, nil
		default:
			return rawKey{kind: rawKeyUnknown}, nil
		}
	case 0x1b:
		next, _, err := reader.ReadRune()
		if err != nil {
			return rawKey{kind: rawKeyUnknown}, err
		}
		if next != '[' && next != 'O' {
			return rawKey{kind: rawKeyEscape}, nil
		}
		next, _, err = reader.ReadRune()
		if err != nil {
			return rawKey{kind: rawKeyUnknown}, err
		}
		switch next {
		case 'A':
			return rawKey{kind: rawKeyArrowUp}, nil
		case 'B':
			return rawKey{kind: rawKeyArrowDown}, nil
		default:
			return rawKey{kind: rawKeyUnknown}, nil
		}
	case '\r', '\n':
		return rawKey{kind: rawKeyEnter}, nil
	case 0x7f, 0x08:
		return rawKey{kind: rawKeyBackspace}, nil
	case '\t':
		return rawKey{kind: rawKeyTab}, nil
	case 0x03:
		return rawKey{kind: rawKeyCtrlC}, nil
	default:
		return rawKey{kind: rawKeyRune, r: ch}, nil
	}
}

func (a *App) slashSuggestions(input string) []slashSuggestion {
	if !command.IsSlashToken(input) {
		return nil
	}
	rest := strings.TrimPrefix(strings.TrimSpace(input), "/")
	rest = strings.ToLower(strings.TrimSpace(rest))

	descriptions := a.skillDescriptions()

	candidates := map[string]struct{}{}
	for name := range a.commandSet {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		candidates[name] = struct{}{}
	}
	for _, name := range command.SlashSuggestionCandidates() {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		candidates[name] = struct{}{}
	}

	names := make([]string, 0, len(candidates))
	for name := range candidates {
		if strings.HasPrefix(name, rest) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	result := make([]slashSuggestion, 0, len(names))
	for _, name := range names {
		if command.IsMetaSlashCommand(name) {
			result = append(result, slashSuggestion{
				Token:       "/" + name,
				Category:    "setting",
				Description: command.SlashMetaDescription(name),
			})
			continue
		}

		desc, ok := descriptions[name]
		if !ok || desc == "" {
			desc = "คำสั่งเครื่องมือ"
		}
		result = append(result, slashSuggestion{
			Token:       "/" + name,
			Category:    "tool",
			Description: desc,
		})
	}

	return result
}

func (a *App) isSlashToken(input string) bool {
	return command.IsSlashToken(input)
}

func (a *App) drawLineWithSlashPalette(line string, suggestions []slashSuggestion, selected int) {
	a.console.Print("\r\x1b[2K> ")
	a.console.Print(line)
	a.console.Print("\x1b[J")

	if len(suggestions) == 0 {
		return
	}

	a.console.Print("\r\n")
	a.console.Print("\x1b[2K")
	a.console.Print("Legend: ")

	ansiOk := supportsANSI()
	settingSwatch := "🟧"
	toolSwatch := "🔵"
	if ansiOk {
		settingSwatch = slashMetaSwatch + " " + ansiReset
		toolSwatch = slashToolSwatch + " " + ansiReset
	}

	a.console.Print(settingSwatch)
	a.console.Print(" " + slashMetaSuggestionColor + "[setting]" + ansiReset + " = คำสั่งตั้งค่า (ส้ม), ")
	a.console.Print(toolSwatch)
	a.console.Print(" " + slashToolSuggestionColor + "[tool]" + ansiReset + " = คำสั่งเครื่องมือ (น้ำเงิน)")

	for i, suggestion := range suggestions {
		a.console.Print("\r\n")
		a.console.Print("\x1b[2K")
		categorySwatch := toolSwatch
		tokenColor := ""
		resetColor := ""
		if i == selected {
			a.console.Print(" > ")
			if ansiOk {
				tokenColor = "\x1b[1;97;104m" // Bold white text on light blue background
				resetColor = ansiReset
			}
		} else {
			a.console.Print("   ")
			if suggestion.Category == "setting" {
				categorySwatch = settingSwatch
				if ansiOk {
					tokenColor = slashMetaSuggestionColor
					resetColor = ansiReset
				}
			} else if ansiOk {
				tokenColor = slashToolSuggestionColor
				resetColor = ansiReset
			}
		}

		a.console.Print(fmt.Sprintf("%s %s%-12s [%-7s] %s%s",
			categorySwatch,
			tokenColor,
			suggestion.Token,
			suggestion.Category,
			suggestion.Description,
			resetColor,
		))
	}

	a.console.Printf("\r\x1b[%dA", len(suggestions)+1)
	a.console.Print("\r\x1b[2K> ")
	a.console.Print(line)
}

func supportsANSI() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	stdout := os.Stdout
	if stdout == nil {
		return false
	}
	stat, err := stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func (a *App) skillDescriptions() map[string]string {
	result := map[string]string{}
	snapshotSource, ok := a.skillDispatcher.(describeSkills)
	if !ok {
		return result
	}

	snapshot := snapshotSource.Snapshot()
	for name, s := range snapshot {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		description := "tool"
		if s != nil {
			description = strings.TrimSpace(s.Description())
			if description == "" {
				description = "tool"
			}
		}
		result[name] = description
	}
	return result
}

func slashCommandNameFromToken(token string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "/"))
}

func isTTYForInput() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
