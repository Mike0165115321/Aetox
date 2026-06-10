package grammar

import (
	"sort"
	"strings"
)

// Kind classifies the type of user input.
type Kind int

const (
	KindConversation Kind = iota
	KindSkill
)

// Intent is a fully parsed representation of one line of user input.
type Intent struct {
	Kind      Kind
	Raw       string
	Command   string
	Args      []string
	Commanded bool
	IsSlash   bool
	IsMeta    bool
}

// SplitFunc tokenises raw input into a command name and its arguments.
type SplitFunc func(input string) (string, []string)

// Grammar holds the complete set of rules for classifying and parsing
// user input. The zero value is usable.
type Grammar struct{}

// New returns a ready-to-use Grammar.
func New() *Grammar { return &Grammar{} }

// ---------------------------------------------------------------------------
// Meta-command catalog
// ---------------------------------------------------------------------------

// slashMetaCommands are commands that are recognised only (or primarily)
// when the user types a leading "/".
var slashMetaCommands = map[string]struct{}{
	"model":    {},
	"approval": {},
	"help":     {},
	"h":        {},
	"exit":     {},
	"quit":     {},
	"bye":      {},
	"logout":   {},
}

var slashMetaCommandDescriptions = map[string]string{
	"model":    "เลือกหรือเปลี่ยนโมเดล/provider",
	"approval": "แสดงหรือเปลี่ยนโหมดอนุมัติ (ถามก่อน/คำสั่งเสี่ยง/รันเต็มที่)",
	"help":     "แสดงรายชื่อ slash command",
	"h":        "คำย่อของ /help",
	"exit":     "ออกจากเซสชันปัจจุบัน",
	"quit":     "ออกจากเซสชันปัจจุบัน",
	"bye":      "ออกจากเซสชันปัจจุบัน",
	"logout":   "ออกจากเซสชันปัจจุบัน",
}

// metaCommands are commands that are recognised without a leading "/".
var metaCommands = map[string]struct{}{
	"exit":   {},
	"quit":   {},
	"bye":    {},
	"logout": {},
}

// colonMetaCommands are commands that start with ":".
var colonMetaCommands = map[string]struct{}{
	":help":  {},
	":clear": {},
	":exit":  {},
	":quit":  {},
}

var slashSuggestionCandidates = []string{
	"model",
	"approval",
	"help",
	"exit",
}

// ---------------------------------------------------------------------------
// Public query helpers
// ---------------------------------------------------------------------------

// SlashSuggestionCandidates returns suggested slash-command names that
// should appear in slash completion UI.
func SlashSuggestionCandidates() []string {
	result := make([]string, len(slashSuggestionCandidates))
	copy(result, slashSuggestionCandidates)
	return result
}

// IsMetaSlashCommand reports whether name is a meta command that is
// recognised when preceded by "/".
func IsMetaSlashCommand(name string) bool {
	_, ok := slashMetaCommands[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// SlashMetaDescription returns the human-readable description for a slash
// meta command, or a generic fallback string.
func SlashMetaDescription(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if desc, ok := slashMetaCommandDescriptions[name]; ok {
		return desc
	}
	return "คำสั่งตั้งค่า"
}

// SlashMetaLegend returns a legend string explaining the command category
// colour scheme.
func SlashMetaLegend() string {
	return "คีย์เดอร์คำสั่ง: [setting] คำสั่งตั้งค่า (ส้ม), [tool] คำสั่งเครื่องมือ (น้ำเงิน)"
}

// ---------------------------------------------------------------------------
// Core parsing
// ---------------------------------------------------------------------------

// Parse transforms a raw line of user input into a structured Intent.
//
//   - input: the raw text the user typed.
//   - split: a tokeniser that returns (firstToken, remainingTokens).
//   - knownCommands: set of skill names that the dispatcher recognises.
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

	// 1. Slash meta command (e.g. /help, /model)
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

	// 2. Plain or colon meta command (exit, :clear, etc.)
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

	// 3. Known skill
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

	// 4. Unknown slash → conversation (will show "unknown command" in app)
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

	// 5. Free text → conversation
	return Intent{
		Kind:      KindConversation,
		Raw:       raw,
		Commanded: true,
		IsSlash:   isSlash,
	}
}

// ParseTokens is the default tokeniser: splits on whitespace and returns
// (firstField, remainingFields).
func ParseTokens(input string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}

// BuildCommandSet normalises a list of skill names into a lookup set used
// by Parse.
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

// IsSlashToken reports whether input looks like a slash token (starts with
// "/" and contains no whitespace).
func IsSlashToken(input string) bool {
	if !strings.HasPrefix(input, "/") {
		return false
	}
	rest := strings.TrimPrefix(input, "/")
	if len(rest) == 0 {
		return false
	}
	return strings.IndexAny(rest, " \t") == -1
}

// SlashSuggestions returns command names that match the prefix typed after
// "/". It combines skill names from commandSet with built-in slash meta
// commands.
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

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func isMetaCommand(name string) bool {
	_, ok := metaCommands[name]
	return ok
}

func isMetaSlashCommand(name string) bool {
	_, ok := slashMetaCommands[name]
	return ok
}

func isColonMetaCommand(name string) bool {
	_, ok := colonMetaCommands[name]
	return ok
}