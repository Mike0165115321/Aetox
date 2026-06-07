package plan

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestBuild_ClassifiesSkillAndConversation(t *testing.T) {
	commandSet := BuildCommandSet([]string{"list", "time", "echo", "shell"})

	tests := []struct {
		name     string
		input    string
		wantKind Kind
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "skill command with args",
			input:    "list README.md",
			wantKind: KindSkill,
			wantCmd:  "list",
			wantArgs: []string{"README.md"},
		},
		{
			name:     "meta command treated as conversation",
			input:    "exit",
			wantKind: KindConversation,
			wantCmd:  "exit",
			wantArgs: []string{},
		},
		{
			name:     "chat sentence treated as conversation",
			input:    "สวัสดีครับ มีอะไรให้ช่วยไหม",
			wantKind: KindConversation,
			wantCmd:  "",
		},
		{
			name:     "unknown command treated as conversation",
			input:    "deploy all",
			wantKind: KindConversation,
			wantCmd:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := Build(tt.input, testSplit, commandSet)
			if intent.Kind != tt.wantKind {
				t.Fatalf("kind: want %v got %v", tt.wantKind, intent.Kind)
			}
			if intent.Command != tt.wantCmd {
				t.Fatalf("command: want %q got %q", tt.wantCmd, intent.Command)
			}
			if !reflect.DeepEqual(intent.Args, tt.wantArgs) {
				t.Fatalf("args: want %#v got %#v", tt.wantArgs, intent.Args)
			}
		})
	}
}

func TestBuildCommandSet_NormalizesNames(t *testing.T) {
	got := BuildCommandSet([]string{"List", "TIME", "  shell  ", "", "help"})
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	expect := []string{"help", "list", "shell", "time"}
	if !reflect.DeepEqual(keys, expect) {
		t.Fatalf("normalized command set mismatch: want %#v got %#v", expect, keys)
	}
}

func testSplit(input string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}
