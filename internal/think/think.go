package think

import (
	"fmt"
	"strings"
)

type Level string

const (
	LevelLow       Level = "low"
	LevelMedium    Level = "medium"
	LevelHigh      Level = "high"
	LevelNoThinking Level = "off-think"
)

type Profile struct {
	Requested  Level
	Resolved   Level
	Native     bool
	Downgraded bool
}

func ParseLevel(raw string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(LevelLow):
		return LevelLow, nil
	case string(LevelMedium):
		return LevelMedium, nil
	case string(LevelHigh):
		return LevelHigh, nil
	case string(LevelNoThinking):
		return LevelNoThinking, nil
	default:
		return "", fmt.Errorf("invalid think level %q (expected low|medium|high|off-think)", raw)
	}
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
