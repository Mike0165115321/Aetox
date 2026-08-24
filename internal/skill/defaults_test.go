package skill

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/model"
)

func TestNewDefaultRegistryRegistersAllBuiltins(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})

	// `shell` and `github` are packed (packed.go): the names inside them —
	// shell_output, shell_kill, github_search and the rest — are still the
	// vocabulary of permission, but they are not registry entries any more and
	// must not be looked for as any. TestAPackedToolIsOneRegistryEntry pins
	// that from the other side.
	want := []string{
		"help", "echo", "time", "calc", "list", "read",
		"git", "fs", "shell", "write", "sheet_write", "doc_write", "edit", "grep", "glob", "edits", "diagnostics", "symbol", "delete", "plugin_install", "image_ocr", "video_ocr", "pdf_read", "audio_transcribe",
		"web_fetch", "web_search", "github",
		"n8n", "windmill",
		"skills_list", "skill_view",
	}
	for _, name := range want {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("built-in skill %q not registered", name)
		}
		if src, ok := registry.SourceOf(name); !ok || src != SourceBuiltin {
			t.Errorf("SourceOf(%q) = %v, %v, want %v, true", name, src, ok, SourceBuiltin)
		}
	}
	if got := len(registry.Names()); got != len(want) {
		t.Errorf("registry has %d skills, want %d", got, len(want))
	}

	// Compiled in and deliberately not registered (defaults.go, 2026-08-19):
	// zero calls in the log against its 317 tokens on every request. Asserted
	// rather than merely absent from `want`, so switching it back on is a
	// decision somebody makes here rather than something that drifts in.
	if _, ok := registry.Get("notebook_edit"); ok {
		t.Error("notebook_edit is registered again — that is a decision, not a merge: say so in defaults.go and delete this check")
	}
}

func TestRegisterDefaultsNilRegistryIsSafe(t *testing.T) {
	RegisterDefaults(nil, RegistryOptions{}) // must not panic
}

// A tool description that claims a word — "use this whenever the user wants
// slides" — is routing burned into the tool layer, where it beats any weighing
// the system prompt asks for. That is how "สร้างสไลด์" went straight to a
// .pptx over an explicit "อยากได้เป็นไฟล์ HTML" (2026-08-04): the model's
// thinking quoted the description and never considered anything else.
// Descriptions state capability — "when a real .pptx file is the deliverable"
// — and which shape answers the request stays the model's judgment, taught as
// a principle in internal/prompt.
func TestToolDescriptionsStateCapabilityNotWordTriggers(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	for _, name := range registry.Names() {
		s, ok := registry.Get(name)
		if !ok {
			continue
		}
		td, ok := s.(interface{ ToolDefinition() model.ToolDefinition })
		if !ok {
			continue
		}
		desc := strings.ToLower(td.ToolDefinition().Function.Description)
		// "is the deliverable" is the softer second draft of the same trap —
		// "use this when a real .pptx is the deliverable" still tells the model
		// when to pick the tool. The owner's line: capability only, the tool is
		// one option on the shelf.
		for _, trap := range []string{"whenever the user wants", "when the user wants", "whenever the user asks", "when the user asks for", "is the deliverable"} {
			if strings.Contains(desc, trap) {
				t.Errorf("%s claims a word trigger (%q) — describe the capability and leave the routing judgment to the model", name, trap)
			}
		}
	}
}
