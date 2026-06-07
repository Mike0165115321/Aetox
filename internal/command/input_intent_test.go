package command

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
			name:  "meta command without slash is conversation",
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
			name:  "colon meta command is conversation",
			input: ":clear",
			want: Intent{
				Kind:      KindConversation,
				Raw:       ":clear",
				Command:   ":clear",
				Args:      []string{},
				Commanded: true,
				IsMeta:    true,
				IsSlash:   false,
			},
		},
		{
			name:  "slash meta command is conversation",
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
			name:  "unknown slash is conversation for suggestion path",
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
			name:  "skill command is skill intent",
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
			name:  "free text is conversation with no command",
			input: "สวัสดีครับ",
			want: Intent{
				Kind:      KindConversation,
				Raw:       "สวัสดีครับ",
				Commanded: true,
				Command:   "",
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

func TestSlashSuggestions_UsesCatalog(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list", "time", "shell"})
	got := SlashSuggestions("/s", commandSet)
	want := []string{"/shell"}
	// keep deterministic output for prefix match.
	if len(got) != len(want) {
		t.Fatalf("count: want %d got %d (%#v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("suggestion[%d]: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestParseArgs_ChatAndMessage(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		mode    Mode
		message string
		rawArgs []string
	}{
		{
			name:    "chat normal",
			args:    []string{"chat", "time and list internal"},
			mode:    ModeOnce,
			message: "time and list internal",
			rawArgs: []string{"chat", "time and list internal"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseArgs(tc.args)
			if got.Mode != tc.mode {
				t.Fatalf("mode: want %q got %q", tc.mode, got.Mode)
			}
			if got.Message != tc.message {
				t.Fatalf("message: want %q got %q", tc.message, got.Message)
			}
			if len(got.RawArgs) != len(tc.rawArgs) {
				t.Fatalf("raw args length: want %d got %d", len(tc.rawArgs), len(got.RawArgs))
			}
		})
	}
}

func TestParseArgs_HelpAndVersion(t *testing.T) {
	help := ParseArgs([]string{"help"})
	if help.Mode != ModeHelp {
		t.Fatalf("help mode: want %q got %q", ModeHelp, help.Mode)
	}
	version := ParseArgs([]string{"--version"})
	if version.Mode != ModeVersion {
		t.Fatalf("version mode: want %q got %q", ModeVersion, version.Mode)
	}
}
