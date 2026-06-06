package plan

import "strings"

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
}

type SplitFunc func(input string) (string, []string)

func Build(input string, split SplitFunc, knownCommands map[string]struct{}) Intent {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Intent{Kind: KindConversation, Raw: raw}
	}

	name, args := split(raw)
	if name == "" || len(name) == 0 {
		return Intent{Kind: KindConversation, Raw: raw}
	}

	name = strings.ToLower(strings.TrimSpace(name))
	if isMetaCommand(name) {
		return Intent{Kind: KindConversation, Raw: raw}
	}

	if _, ok := knownCommands[name]; ok {
		return Intent{
			Kind:      KindSkill,
			Raw:       raw,
			Command:   name,
			Args:      args,
			Commanded: true,
		}
	}

	return Intent{Kind: KindConversation, Raw: raw}
}

func isMetaCommand(name string) bool {
	switch name {
	case "exit", "quit", ":help", ":clear", "bye", ":exit", ":quit", "logout":
		return true
	default:
		return false
	}
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
