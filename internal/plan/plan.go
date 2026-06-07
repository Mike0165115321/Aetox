package plan

import "aetox-cli/internal/command"

type Kind = command.Kind

const (
	KindConversation = command.KindConversation
	KindSkill        = command.KindSkill
)

type Intent = command.Intent

type SplitFunc = command.SplitFunc

func Build(input string, split SplitFunc, knownCommands map[string]struct{}) Intent {
	return command.Parse(input, split, knownCommands)
}

func BuildCommandSet(names []string) map[string]struct{} {
	return command.BuildCommandSet(names)
}
