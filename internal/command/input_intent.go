package command

import "aetox-cli/internal/grammar"

// Kind = grammar.Kind
type Kind = grammar.Kind

const (
	KindConversation = grammar.KindConversation
	KindSkill        = grammar.KindSkill
)

// Intent = grammar.Intent
type Intent = grammar.Intent

// SplitFunc = grammar.SplitFunc
type SplitFunc = grammar.SplitFunc

// SlashSuggestionCandidates delegates to grammar.
func SlashSuggestionCandidates() []string {
	return grammar.SlashSuggestionCandidates()
}

// IsMetaSlashCommand delegates to grammar.
func IsMetaSlashCommand(name string) bool {
	return grammar.IsMetaSlashCommand(name)
}

// SlashMetaDescription delegates to grammar.
func SlashMetaDescription(name string) string {
	return grammar.SlashMetaDescription(name)
}

// SlashMetaLegend delegates to grammar.
func SlashMetaLegend() string {
	return grammar.SlashMetaLegend()
}

// Parse delegates to grammar.Parse.
func Parse(input string, split SplitFunc, knownCommands map[string]struct{}) Intent {
	return grammar.Parse(input, split, knownCommands)
}

// ParseTokens delegates to grammar.ParseTokens.
func ParseTokens(input string) (string, []string) {
	return grammar.ParseTokens(input)
}

// BuildCommandSet delegates to grammar.BuildCommandSet.
func BuildCommandSet(names []string) map[string]struct{} {
	return grammar.BuildCommandSet(names)
}

// IsSlashToken delegates to grammar.IsSlashToken.
func IsSlashToken(input string) bool {
	return grammar.IsSlashToken(input)
}

// SlashSuggestions delegates to grammar.SlashSuggestions.
func SlashSuggestions(input string, commandSet map[string]struct{}) []string {
	return grammar.SlashSuggestions(input, commandSet)
}
