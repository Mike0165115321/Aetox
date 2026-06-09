package think

import (
	"fmt"
	"regexp"
	"strings"
)

type Level string

const (
	LevelNone       Level = "none"
	LevelMinimal    Level = "minimal"
	LevelLow        Level = "low"
	LevelMedium     Level = "medium"
	LevelHigh       Level = "high"
	LevelXHigh      Level = "xhigh"
	LevelMax        Level = "max"
	LevelDefault    Level = "default"
	LevelNoThinking Level = "off"
)

var levelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

type Profile struct {
	Requested  Level
	Resolved   Level
	Native     bool
	Downgraded bool
}

func ParseLevel(raw string) (Level, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return "", fmt.Errorf("invalid think level %q", raw)
	}
	if !levelPattern.MatchString(normalized) {
		return "", fmt.Errorf("invalid think level %q", raw)
	}
	return Level(normalized), nil
}

func NormalizeLevel(raw string) Level {
	level, err := ParseLevel(raw)
	if err != nil {
		return LevelMedium
	}
	return level
}

func Resolve(level Level, nativeSupported bool) Profile {
	resolved := NormalizeLevel(string(level))
	if resolved == LevelNoThinking {
		return Profile{
			Requested:  resolved,
			Resolved:   resolved,
			Native:     false,
			Downgraded: false,
		}
	}
	return Profile{
		Requested:  resolved,
		Resolved:   resolved,
		Native:     nativeSupported,
		Downgraded: !nativeSupported,
	}
}

func (p Profile) ReasoningEffort() string {
	if !p.Native {
		return ""
	}
	return string(p.Resolved)
}

func (p Profile) StatusLabel() string {
	if p.Requested == LevelNoThinking {
		return fmt.Sprintf("%s (disabled)", p.Resolved)
	}
	if p.Native {
		return fmt.Sprintf("%s (native)", p.Resolved)
	}
	return fmt.Sprintf("%s (provider default fallback)", p.Resolved)
}
