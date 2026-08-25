package subagent

import (
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/learned"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// A delegate runs with its profile plus what it has learned doing this job
// before — and nothing else. This is where "an agent learns inside its own
// scope" stops being a policy and becomes the shape of the prompt.
func TestADelegateRunsWithItsOwnLearnedMemory(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	if err := learned.Apply("explore", learned.OpAdd, "", "SCOPED-MARKER"); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	if err := learned.Apply(learned.MainScope, learned.OpAdd, "", "MAIN-MARKER"); err != nil {
		t.Fatalf("write main memory: %v", err)
	}

	got := PromptFor(Profile{Name: "explore", Prompt: "you search files"})
	if !strings.Contains(got, "you search files") {
		t.Fatalf("the profile's own instructions were lost:\n%s", got)
	}
	if !strings.Contains(got, "SCOPED-MARKER") {
		t.Errorf("this delegate's memory was not folded in:\n%s", got)
	}
	if strings.Contains(got, "MAIN-MARKER") {
		t.Errorf("the main agent's memory reached a delegate:\n%s", got)
	}
	if strings.Contains(PromptFor(Profile{Name: "plan", Prompt: "p"}), "SCOPED-MARKER") {
		t.Error("one delegate's memory reached another's prompt")
	}
}

// Nothing learned means the prompt is byte-for-byte what it was, so a delegate
// that has learned nothing pays nothing and its prefix cache is undisturbed.
func TestADelegateWithNothingLearnedGetsItsProfileUnchanged(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	p := Profile{Name: "explore", Prompt: "you search files"}
	if got := PromptFor(p); got != p.Prompt {
		t.Errorf("prompt changed with nothing learned:\n%q", got)
	}
}

// The parent's `memory` tool is bound to the main agent's scope. A delegate
// holding it would write into the prompt every other agent pays for — so the
// filtered registry must not carry it, and `task` builds a scoped replacement.
func TestADelegateNeverInheritsTheParentsMemoryTool(t *testing.T) {
	isolate(t)
	parent := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: t.TempDir()})
	if err := parent.Register(&learned.MemoryTool{Scope: learned.MainScope}, skill.SourceBuiltin); err != nil {
		t.Fatalf("register: %v", err)
	}

	// An empty profile allows everything the parent has, which is what makes
	// this the strongest form of the check: memory is dropped even when nothing
	// in the profile asked for it to be.
	child := FilterRegistry(parent, Profile{Name: "explore"}, nil)
	if _, ok := child.Get("memory"); ok {
		t.Error("the parent's memory tool was inherited")
	}
	if _, ok := child.Get("grep"); !ok {
		t.Error("filtering dropped a tool it should have kept")
	}
}

// A profile that refuses memory in its own frontmatter keeps that refusal —
// which is why the tool is dropped in the filter rather than added to
// forcedDenials, where the profile's own answer would never be consulted.
func TestAProfileCanStillRefuseMemory(t *testing.T) {
	if (Profile{Name: "explore", Deny: []string{"memory"}}).AllowsTool("memory") {
		t.Error("a profile that denies memory should not be handed one")
	}
	if !(Profile{Name: "explore"}).AllowsTool("memory") {
		t.Error("a profile that says nothing should be allowed memory")
	}
	if (Profile{Name: "explore", Tools: []string{"grep"}}).AllowsTool("memory") {
		t.Error("an allowlist that omits memory should exclude it")
	}
}
