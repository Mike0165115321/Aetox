package skill

import (
	"context"

	"github.com/Mike0165115321/Aetox/internal/stt"
)

// Digester answers a question about a block of text. web_fetch uses it to hand
// back an answer instead of a whole web page: a documentation page is tens of
// thousands of characters and the model usually wants one paragraph of it, so
// without this the tool's own output is the expensive part of the call.
//
// A function, not a model.Provider: internal/skill has no business knowing how
// a completion is made, and a func is what a test can supply in one line. The
// host wires it in RegistryOptions; when it is nil, web_fetch returns the full
// page exactly as it always has.
type Digester func(ctx context.Context, question, text string) (string, error)

// RegistryOptions carries what built-in skills need from the host. Most skills
// only want the sandbox root; a skill that the *user* configures (rather than
// the model) gets its own field here, fed from settings.
// ponytail: one field per configurable skill is fine at this count — turn it
// into a map keyed by skill name if a third or fourth one shows up.
type RegistryOptions struct {
	SandboxRoot string
	// Speech configures audio_transcribe: which engine, which model file.
	// The zero value means the catalog default with an auto-discovered model.
	Speech stt.Options
	// OutputSubdir answers "where should a brand new file go?" as a path
	// relative to SandboxRoot — returning "" writes straight to the root, as
	// before. A func rather than a string because the answer changes per chat
	// session, and re-bootstrapping the whole engine to change one folder name
	// would be an absurd price. See writeSkill.placed.
	//
	// Every skill that consumes an existing file by relative path needs it too,
	// not just write: read/edit/delete/apply_patch fall back to this folder when
	// the literal path resolves to nothing (PlacedPath). Hand it to any new
	// file-consuming skill as well, or that skill will fail to find whatever
	// write just produced.
	OutputSubdir func() string
	// Digest lets web_fetch answer a question about a page instead of
	// returning the page. Nil is a supported configuration — the CLI has no
	// provider handy at registry-build time — and simply means the tool keeps
	// its old behaviour.
	Digest Digester
	// Vision reports whether the model behind this registry can look at an
	// image. It decides one thing: whether `read` hands back a .png or tells
	// the caller to run image_ocr on it. False is the safe value and the one
	// every caller that does not know should pass (ARCHITECTURE.md §51).
	Vision bool
}

func NewDefaultRegistry(opts RegistryOptions) *Registry {
	registry := NewRegistry()
	RegisterDefaults(registry, opts)
	return registry
}

func RegisterDefaults(registry *Registry, opts RegistryOptions) {
	if registry == nil {
		return
	}
	// One registry of background commands, shared by the three tools that see
	// them: shell starts, shell_output reads, shell_kill ends.
	shells := newBackgroundShells()
	defaults := []Skill{
		&helpSkill{registry: registry},
		&echoSkill{},
		&timeSkill{},
		&listSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&readSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, vision: opts.Vision},
		&githubRepoSummarySkill{},
		&gitSkill{root: opts.SandboxRoot},
		&fsSkill{root: opts.SandboxRoot},
		&shellSkill{root: opts.SandboxRoot, shells: shells},
		&shellOutputSkill{shells: shells},
		&shellKillSkill{shells: shells},
		&writeSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&editSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&grepSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&globSkill{root: opts.SandboxRoot},
		&applyPatchSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&notebookEditSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&diagnosticsSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&symbolSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&deleteSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&pluginInstallSkill{},
		&imageOCRSkill{root: opts.SandboxRoot},
		&videoOCRSkill{root: opts.SandboxRoot},
		&pdfReadSkill{root: opts.SandboxRoot},
		&audioTranscribeSkill{root: opts.SandboxRoot, speech: opts.Speech},
		&webFetchSkill{digest: opts.Digest},
		&webSearchSkill{},
		&githubSearchSkill{},
		&githubReadFileSkill{},
		&githubListFilesSkill{},
	}
	for _, s := range defaults {
		if err := registry.Register(s, SourceBuiltin); err != nil {
			// Two built-ins sharing a name is a programmer error, not a runtime condition.
			panic(err)
		}
	}
}
