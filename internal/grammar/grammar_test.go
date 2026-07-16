package grammar

import (
	"reflect"
	"testing"
)

func TestParse_ConversationAndSlash(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list", "git", "shell"})

	tests := []struct {
		name  string
		input string
		want  Intent
	}{
		{
			name:  "empty input is conversation",
			input: "",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "",
				Commanded: false,
			},
		},
		{
			name:  "whitespace only is conversation",
			input: "   ",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "",
				Commanded: false,
			},
		},
		// Slash commands
		{
			name:  "slash help is meta",
			input: "/help",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/help",
				Command:   "help",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash h is meta",
			input: "/h",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/h",
				Command:   "h",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash model is meta",
			input: "/model",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/model",
				Command:   "model",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash exit is meta",
			input: "/exit",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/exit",
				Command:   "exit",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash quit is meta",
			input: "/quit",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/quit",
				Command:   "quit",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash bye is meta",
			input: "/bye",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/bye",
				Command:   "bye",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash logout is meta",
			input: "/logout",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/logout",
				Command:   "logout",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash with upper case is normalized",
			input: "/HELP",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/HELP",
				Command:   "help",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "slash with mixed case is normalized",
			input: "/Help",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/Help",
				Command:   "help",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},

		// Plain meta commands
		{
			name:  "meta command exit without slash",
			input: "exit",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "exit",
				Command:   "exit",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "meta command quit without slash",
			input: "quit",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "quit",
				Command:   "quit",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "meta command bye without slash",
			input: "bye",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "bye",
				Command:   "bye",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "meta command logout without slash",
			input: "logout",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "logout",
				Command:   "logout",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "meta command with trailing spaces is still meta",
			input: "exit  ",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "exit",
				Command:   "exit",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},

		// Colon meta commands
		{
			name:  "colon clear is meta",
			input: ":clear",
			want: Intent{
				Kind:      KindConversation,
				Raw:       ":clear",
				Command:   ":clear",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "colon help is meta",
			input: ":help",
			want: Intent{
				Kind:      KindConversation,
				Raw:       ":help",
				Command:   ":help",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "colon exit is meta",
			input: ":exit",
			want: Intent{
				Kind:      KindConversation,
				Raw:       ":exit",
				Command:   ":exit",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "colon quit is meta",
			input: ":quit",
			want: Intent{
				Kind:      KindConversation,
				Raw:       ":quit",
				Command:   ":quit",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},

		// Help variants
		{
			name:  "help command with slash is meta",
			input: "/help",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/help",
				Command:   "help",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "h shortcut with slash is meta",
			input: "/h",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/h",
				Command:   "h",
				Commanded: true,
				IsSlash:   true,
				IsMeta:    true,
			},
		},
		{
			name:  "colon help is meta",
			input: ":help",
			want: Intent{
				Kind:      KindConversation,
				Raw:       ":help",
				Command:   ":help",
				Commanded: true,
				Args:      []string{},
				IsMeta:    true,
				IsSlash:   false,
			},
		},

		// Skill name matching
		{
			name:  "git status is skill intent",
			input: "git status",
			want: Intent{
				Kind:      KindSkill,
				Raw:       "git status",
				Command:   "git",
				Args:      []string{"status"},
				Commanded: true,
				IsSlash:   false,
			},
		},
		{
			name:  "list is skill intent",
			input: "list",
			want: Intent{
				Kind:      KindSkill,
				Raw:       "list",
				Command:   "list",
				Args:      []string{},
				Commanded: true,
				IsSlash:   false,
			},
		},
		{
			name:  "list with path is skill intent",
			input: "list /tmp",
			want: Intent{
				Kind:      KindSkill,
				Raw:       "list /tmp",
				Command:   "list",
				Args:      []string{"/tmp"},
				Commanded: true,
				IsSlash:   false,
			},
		},
		{
			name:  "shell command is skill intent",
			input: "shell echo hello",
			want: Intent{
				Kind:      KindSkill,
				Raw:       "shell echo hello",
				Command:   "shell",
				Args:      []string{"echo", "hello"},
				Commanded: true,
				IsSlash:   false,
			},
		},
		{
			name:  "skill with slash is skill intent",
			input: "/git status",
			want: Intent{
				Kind:      KindSkill,
				Raw:       "/git status",
				Command:   "git",
				Args:      []string{"status"},
				Commanded: true,
				IsSlash:   true,
			},
		},

		// Unknown command fallback
		{
			name:  "unknown slash without skill match is conversation",
			input: "/foo bar",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/foo bar",
				Command:   "foo",
				Args:      []string{"bar"},
				Commanded: true,
				IsSlash:   true,
				IsMeta:    false,
			},
		},
		{
			name:  "unknown slash with no args is conversation",
			input: "/xyz",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/xyz",
				Command:   "xyz",
				Args:      []string{},
				Commanded: true,
				IsSlash:   true,
				IsMeta:    false,
			},
		},

		// Natural language passthrough
		{
			name:  "Thai free text is conversation",
			input: "สวัสดีครับ",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "สวัสดีครับ",
				Commanded: true,
				Command:   "",
			},
		},
		{
			name:  "English free text is conversation",
			input: "hello world",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "hello world",
				Commanded: true,
				Command:   "",
			},
		},
		{
			name:  "question is conversation",
			input: "what is the time",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "what is the time",
				Commanded: true,
				Command:   "",
			},
		},
		{
			name:  "sentence with known command word as non-first token",
			input: "please list all files",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "please list all files",
				Commanded: true,
				Command:   "",
			},
		},

		// Edge cases
		{
			name:  "specious is not shell (no known command detected)",
			input: "specious",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "specious",
				Commanded: true,
				Command:   "",
			},
		},
		{
			name:  "slash with only slash is conversation with isSlash",
			input: "/",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "/",
				Commanded: false,
				IsSlash:   true,
			},
		},
		{
			name:  "slash slash is unknown slash conversation",
			input: "//",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "//",
				Command:   "/",
				Args:      []string{},
				Commanded: true,
				IsSlash:   true,
				IsMeta:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input, ParseTokens, commandSet)
			if got.Kind != tt.want.Kind {
				t.Fatalf("kind: want %v got %v", tt.want.Kind, got.Kind)
			}
			if got.Raw != tt.want.Raw {
				t.Fatalf("raw: want %q got %q", tt.want.Raw, got.Raw)
			}
			if got.Command != tt.want.Command {
				t.Fatalf("command: want %q got %q", tt.want.Command, got.Command)
			}
			if got.IsSlash != tt.want.IsSlash {
				t.Fatalf("slash: want %v got %v", tt.want.IsSlash, got.IsSlash)
			}
			if got.IsMeta != tt.want.IsMeta {
				t.Fatalf("meta: want %v got %v", tt.want.IsMeta, got.IsMeta)
			}
			if got.Commanded != tt.want.Commanded {
				t.Fatalf("commanded: want %v got %v", tt.want.Commanded, got.Commanded)
			}
			if !reflect.DeepEqual(got.Args, tt.want.Args) {
				t.Fatalf("args: want %#v got %#v", tt.want.Args, got.Args)
			}
		})
	}
}

func TestParse_EmptyCommandSet(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Intent
	}{
		{
			name:  "skill name maps to skill",
			input: "list",
			want: Intent{
				Kind:      KindSkill,
				Raw:       "list",
				Command:   "list",
				Args:      []string{},
				Commanded: true,
				IsSlash:   false,
			},
		},
		{
			name:  "unknown skill name in empty set is conversation",
			input: "foo",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "foo",
				Commanded: true,
				Command:   "",
			},
		},
	}

	commandSet := BuildCommandSet([]string{"list"})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input, ParseTokens, commandSet)
			if got.Kind != tt.want.Kind {
				t.Fatalf("kind: want %v got %v", tt.want.Kind, got.Kind)
			}
			if got.Command != tt.want.Command {
				t.Fatalf("command: want %q got %q", tt.want.Command, got.Command)
			}
		})
	}
}

func TestParse_NilCommandSet(t *testing.T) {
	// nil command set should not panic
	got := Parse("list", ParseTokens, nil)
	if got.Kind != KindConversation {
		t.Fatalf("expected conversation for nil command set, got %v", got.Kind)
	}
	if got.Command != "" {
		t.Fatalf("expected empty command, got %q", got.Command)
	}
}

func TestSlashSuggestions_UsesCatalog(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list", "time", "shell"})
	got := SlashSuggestions("/s", commandSet)
	want := []string{"/shell"}
	if len(got) != len(want) {
		t.Fatalf("count: want %d got %d (%#v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("suggestion[%d]: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestSlashSuggestions_NoMatch(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list", "time", "shell"})
	got := SlashSuggestions("/z", commandSet)
	if len(got) != 0 {
		t.Fatalf("expected no suggestions for /z, got %#v", got)
	}
}

func TestSlashSuggestions_NotSlashToken(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list"})
	got := SlashSuggestions("list", commandSet)
	if got != nil {
		t.Fatalf("expected nil for non-slash input, got %#v", got)
	}
}

func TestSlashSuggestions_IncludesBuiltins(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list"})
	got := SlashSuggestions("/m", commandSet)
	// should include /model from built-in candidates
	if len(got) != 1 || got[0] != "/model" {
		t.Fatalf("expected [/model], got %#v", got)
	}
}

func TestParseTokens(t *testing.T) {
	tests := []struct {
		input    string
		wantCmd  string
		wantArgs []string
	}{
		{"", "", nil},
		{"   ", "", nil},
		{"foo", "foo", []string{}},
		{"foo bar", "foo", []string{"bar"}},
		{"foo bar baz", "foo", []string{"bar", "baz"}},
		{"  foo   bar  ", "foo", []string{"bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, args := ParseTokens(tt.input)
			if cmd != tt.wantCmd {
				t.Fatalf("cmd: want %q got %q", tt.wantCmd, cmd)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("args: want %#v got %#v", tt.wantArgs, args)
			}
		})
	}
}

func TestBuildCommandSet(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  map[string]struct{}
	}{
		{
			name:  "normalizes to lowercase",
			input: []string{"List", "GIT", "Shell"},
			want: map[string]struct{}{
				"list":  {},
				"git":   {},
				"shell": {},
			},
		},
		{
			name:  "trims whitespace",
			input: []string{"  list ", " git"},
			want: map[string]struct{}{
				"list": {},
				"git":  {},
			},
		},
		{
			name:  "skips empty strings",
			input: []string{"list", "", "  "},
			want:  map[string]struct{}{"list": {}},
		},
		{
			name:  "empty input",
			input: []string{},
			want:  map[string]struct{}{},
		},
		{
			name:  "nil input",
			input: nil,
			want:  map[string]struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCommandSet(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("want %#v got %#v", tt.want, got)
			}
		})
	}
}

func TestIsSlashToken(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/help", true},
		{"/model", true},
		{"/", false},
		{"help", false},
		{"/help me", false},
		{" /help", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsSlashToken(tt.input)
			if got != tt.want {
				t.Fatalf("IsSlashToken(%q): want %v got %v", tt.input, tt.want, got)
			}
		})
	}
}

func TestIsMetaSlashCommand(t *testing.T) {
	if !IsMetaSlashCommand("model") {
		t.Fatal("expected model to be meta slash command")
	}
	if !IsMetaSlashCommand("help") {
		t.Fatal("expected help to be meta slash command")
	}
	if IsMetaSlashCommand("unknown") {
		t.Fatal("expected unknown not to be meta slash command")
	}
}

func TestSlashMetaDescription(t *testing.T) {
	desc := SlashMetaDescription("model")
	if desc == "" {
		t.Fatal("expected non-empty description for model")
	}
	desc = SlashMetaDescription("unknown")
	if desc == "" {
		t.Fatal("expected non-empty fallback description for unknown")
	}
}

func TestSlashSuggestionCandidates(t *testing.T) {
	candidates := SlashSuggestionCandidates()
	if len(candidates) == 0 {
		t.Fatal("expected non-empty slash suggestion candidates")
	}
}

func TestSlashMetaLegend(t *testing.T) {
	legend := SlashMetaLegend()
	if legend == "" {
		t.Fatal("expected non-empty legend")
	}
}

func TestNew(t *testing.T) {
	g := New()
	if g == nil {
		t.Fatal("expected non-nil Grammar")
	}
}
