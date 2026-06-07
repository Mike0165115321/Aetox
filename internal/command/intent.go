package command

import (
	"strings"
)

type Mode string

const (
	ModeHelp        Mode = "help"
	ModeVersion     Mode = "version"
	ModeInteractive Mode = "interactive"
	ModeOnce        Mode = "once"
)

type ParsedIntent struct {
	Mode    Mode
	Message string
	RawArgs []string
}

func ParseArgs(args []string) ParsedIntent {
	if len(args) == 0 {
		return ParsedIntent{
			Mode:    ModeInteractive,
			RawArgs: []string{},
		}
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "help", "-h", "--help":
		return ParsedIntent{
			Mode:    ModeHelp,
			RawArgs: args,
		}
	case "version", "--version", "-v":
		return ParsedIntent{
			Mode:    ModeVersion,
			RawArgs: args,
		}
	case "chat":
		if len(args) == 1 {
			return ParsedIntent{
				Mode:    ModeInteractive,
				RawArgs: args,
			}
		}
		return ParsedIntent{
			Mode:    ModeOnce,
			Message: strings.TrimSpace(strings.Join(args[1:], " ")),
			RawArgs: args,
		}
	default:
		return ParsedIntent{
			Mode:    ModeOnce,
			Message: strings.TrimSpace(strings.Join(args, " ")),
			RawArgs: args,
		}
	}
}
