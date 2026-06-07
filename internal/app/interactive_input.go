package app

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"

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
			if selected >= 0 && a.isSlashToken(string(line)) {
				suggestions := a.slashSuggestions(string(line))
				if selected < len(suggestions) {
					line = []rune(suggestions[selected])
				}
			}
			a.console.Print("\n")
			return string(line), nil
		case rawKeyCtrlC, rawKeyEscape:
			return "", io.EOF
		case rawKeyBackspace:
			if len(line) > 0 {
				line = line[:len(line)-1]
			}
			selected = -1
			render()
		case rawKeyArrowUp, rawKeyArrowDown, rawKeyTab:
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
		case rawKeyRune:
			line = append(line, key.r)
			selected = -1
			render()
		}
	}
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

func (a *App) slashSuggestions(input string) []string {
	if !command.IsSlashToken(input) {
		return nil
	}
	return command.SlashSuggestions(input, a.commandSet)
}

func (a *App) isSlashToken(input string) bool {
	return command.IsSlashToken(input)
}

func (a *App) drawLineWithSlashPalette(line string, suggestions []string, selected int) {
	a.console.Print("\r\x1b[2K> ")
	a.console.Print(line)
	a.console.Print("\x1b[J")

	if len(suggestions) == 0 {
		return
	}

	for i, suggestion := range suggestions {
		a.console.Print("\r\n")
		a.console.Print("\x1b[2K")
		if i == selected {
			a.console.Print(" > ")
		} else {
			a.console.Print("   ")
		}
		a.console.Print(suggestion)
	}

	a.console.Printf("\r\x1b[%dA", len(suggestions))
	a.console.Print("\r\x1b[2K> ")
	a.console.Print(line)
}

func isTTYForInput() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
