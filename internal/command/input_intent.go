package command

import (
	"sort"
	"strings"
)

type Kind int

const (
	KindConversation Kind = iota
	KindSkill
)

type Intent struct {
	Kind      Kind
	Raw       string
	Command   string
	Args      []string
	Commanded bool
	IsSlash   bool
	IsMeta    bool
}

type SplitFunc func(input string) (string, []string)

var slashMetaCommands = map[string]struct{}{
	"model":  {},
	"help":   {},
	"h":      {},
	"exit":   {},
	"quit":   {},
	"bye":    {},
	"logout": {},
}

var metaCommands = map[string]struct{}{
	"exit":   {},
	"quit":   {},
	"bye":    {},
	"logout": {},
}

var colonMetaCommands = map[string]struct{}{
	":help":  {},
	":clear": {},
	":exit":  {},
	":quit":  {},
}

var slashSuggestionCandidates = []string{
	"model",
	"help",
	"h",
	"exit",
	"quit",
	"bye",
	"logout",
}

func Parse(input string, split SplitFunc, knownCommands map[string]struct{}) Intent {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Intent{Kind: KindConversation, Raw: raw}
	}

	parsed := raw
	isSlash := strings.HasPrefix(parsed, "/")
	if isSlash {
		parsed = strings.TrimSpace(strings.TrimPrefix(parsed, "/"))
	}

	commandName, args := split(parsed)
	if commandName == "" {
		return Intent{
			Kind:    KindConversation,
			Raw:     raw,
			IsSlash: isSlash,
		}
	}

	commandName = strings.ToLower(strings.TrimSpace(commandName))
	if isSlash && isMetaSlashCommand(commandName) {
		return Intent{
			Kind:      KindConversation,
			Raw:       raw,
			Command:   commandName,
			Commanded: true,
			IsSlash:   true,
			IsMeta:    true,
		}
	}
	if (isMetaCommand(commandName) && len(args) == 0) || (isColonMetaCommand(commandName) && len(args) == 0) {
		return Intent{
			Kind:      KindConversation,
			Raw:       raw,
			Command:   commandName,
			Args:      args,
			Commanded: true,
			IsSlash:   isSlash,
			IsMeta:    true,
		}
	}

	if _, ok := knownCommands[commandName]; ok {
		return Intent{
			Kind:      KindSkill,
			Raw:       raw,
			Command:   commandName,
			Args:      args,
			Commanded: true,
			IsSlash:   isSlash,
		}
	}

	if isSlash {
		return Intent{
			Kind:      KindConversation,
			Raw:       raw,
			Command:   commandName,
			Args:      args,
			Commanded: true,
			IsSlash:   true,
		}
	}

	return Intent{
		Kind:      KindConversation,
		Raw:       raw,
		Commanded: true,
		IsSlash:   isSlash,
	}
}

func ParseTokens(input string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}

func BuildCommandSet(names []string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func IsSlashToken(input string) bool {
	if !strings.HasPrefix(input, "/") {
		return false
	}
	rest := strings.TrimPrefix(input, "/")
	return strings.IndexAny(rest, " \t") == -1
}

func SlashSuggestions(input string, commandSet map[string]struct{}) []string {
	if !IsSlashToken(input) {
		return nil
	}

	candidates := map[string]struct{}{}
	for name := range commandSet {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		candidates[name] = struct{}{}
	}
	for _, name := range slashSuggestionCandidates {
		candidates[name] = struct{}{}
	}

	rawToken := strings.TrimPrefix(input, "/")
	match := strings.ToLower(strings.TrimSpace(rawToken))
	suggestions := make([]string, 0, len(candidates))
	for name := range candidates {
		if strings.HasPrefix(name, match) {
			suggestions = append(suggestions, "/"+name)
		}
	}
	sort.Strings(suggestions)
	return suggestions
}

func isMetaCommand(name string) bool {
	_, isMeta := metaCommands[name]
	return isMeta
}

func isMetaSlashCommand(name string) bool {
	_, isMeta := slashMetaCommands[name]
	return isMeta
}

func isColonMetaCommand(name string) bool {
	_, isMeta := colonMetaCommands[name]
	return isMeta
}
