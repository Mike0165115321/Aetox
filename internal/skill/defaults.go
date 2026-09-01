package skill

import (
	"github.com/Mikedev115/Aetox/internal/proc"
	"github.com/Mikedev115/Aetox/internal/stt"
)

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
	// not just write: read/edit/delete/edits fall back to this folder when
	// the literal path resolves to nothing (PlacedPath). Hand it to any new
	// file-consuming skill as well, or that skill will fail to find whatever
	// write just produced.
	OutputSubdir func() string
	// Files is the record of what this app last saw each file as, so a
	// whole-file write can refuse to clobber a change it never saw
	// (filestate.go). Nil is supported and means no guard — the CLI has one
	// writer and nobody typing beside it.
	Files *FileState
	// Vision reports whether the model behind this registry can look at an
	// image. It decides one thing: whether `read` hands back a .png or tells
	// the caller to run image_ocr on it. False is the safe value and the one
	// every caller that does not know should pass (ARCHITECTURE.md §51).
	Vision bool
	// OpenSandbox lifts the sandbox wall for this root: file tools reach
	// anywhere on the machine except the credential stores (see
	// sandbox_open.go). The desktop passes true exactly when no project is
	// focused; false — the default and the only value the CLI ever passes —
	// keeps the closed sandbox unchanged.
	OpenSandbox bool
	// ExtraRoots are folders the user added to a focused project, each with the
	// same rights as SandboxRoot itself. This is how a session reaches a second
	// project — a bug whose cause lives in a shared library, a config that comes
	// from somewhere else — without any mode that reaches everything.
	//
	// Nothing but the user's own list belongs here: no path the app decided was
	// probably fine, no sibling folder inferred from the project's layout. The
	// list IS the permission, so anything added behind the user's back is a
	// permission they never gave.
	ExtraRoots []string
	// AskWorkspace is the door in the wall ExtraRoots is the list for: when a
	// path lands outside the workspace, the host offers to add the folder it
	// lives in rather than ending the work with a refusal the user has to go
	// undo by hand. See WidenFunc for what the answer is and is not allowed to
	// decide.
	//
	// Nil — the CLI, every test, any host with no UI to ask through — keeps the
	// flat refusal exactly as it was.
	AskWorkspace WidenFunc
	// Shell is which shell this workspace speaks: the machine's own, or a WSL
	// distro. A func for the same reason OutputSubdir is one — the user can
	// change it from the picker mid-session, and rebuilding the engine to change
	// which program gets exec'd would be an absurd price.
	//
	// It decides two things that must never disagree: which program runs a
	// command line, and how a path in one is spelled. Both read it from the one
	// record keyed by SandboxRoot (§126), which is why it is recorded here once
	// rather than handed to each tool that cares.
	//
	// Nil means the native shell, which is what the CLI, every test and every
	// caller with no opinion gets, and what the tool did before it was
	// selectable.
	Shell func() proc.Backend
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
	// Recorded on every build, zero values included: re-focusing a project after
	// an unfocused session must close the root again, and a project that just
	// lost a folder must stop reaching it — both in the same call that re-roots
	// the engine, not on the next restart.
	setSandboxPolicy(opts.SandboxRoot, opts.OpenSandbox, opts.ExtraRoots, opts.AskWorkspace)
	// The shell belongs to the same record for the same reason the folder list
	// does: every file tool asks resolveSandboxPath about a path, and a path
	// means a different file depending on which shell wrote it. Recorded on
	// every build, nil included, so a workspace switched back to the native
	// shell stops translating in the same call that switches it.
	setSandboxShell(opts.SandboxRoot, opts.Shell)
	// One registry of background commands, shared by the three tools that see
	// them: shell starts, shell_output reads, shell_kill ends.
	shells := newBackgroundShells()
	defaults := []Skill{
		&helpSkill{registry: registry},
		&echoSkill{},
		&timeSkill{},
		&calcSkill{},
		// One name, three actions: list, glob, grep (search_pack.go). The three
		// types still exist and still do the work - what they stopped being is
		// three entries in the tool block of every desk that can look at files.
		&searchSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir},
		&readSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, vision: opts.Vision, files: opts.Files},
		&gitSkill{root: opts.SandboxRoot},
		&fsSkill{root: opts.SandboxRoot},
		// One name, three actions: run, output, kill (packed.go). The other two
		// types still exist and still do the work — what they stopped being is
		// entries in the tool block of every request that carries a shell.
		// No backend here: which shell it runs in is read from the same record
		// the file tools read, keyed by this root (setSandboxShell above).
		&shellSkill{root: opts.SandboxRoot, shells: shells},
		// One name, five actions: write, edit, append, batch, delete
		// (change_pack.go). The four types still exist and still do the work.
		&changeSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, files: opts.Files},
		&sheetWriteSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, files: opts.Files},
		&docWriteSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, files: opts.Files},
		// `notebook_edit` is deliberately absent (owner's call, 2026-08-19).
		// It is 317 tokens in the block of every desk that carries the files
		// group, paid on every request before the user types, and `tool_runs`
		// records zero calls to it against 2,687 runs since the log began.
		//
		// Its half of notebook.go stays compiled in, and this is a switch
		// rather than a deletion for a reason: `read` renders an .ipynb as
		// numbered cells through loadNotebook/renderNotebook in that same file,
		// so removing the file would take reading notebooks with it — the
		// problem it was written to solve. Notebooks stay readable and become
		// read-only, which is honest: nothing else can edit one (`edit` and
		// `edits` match text the file stores JSON-escaped, and `write`
		// would have to re-emit every base64 output to keep them).
		//
		// Switching it on again is this line plus the files that name a tool
		// rather than describe one, and they are listed here so the switch is
		// read in one place: `defaults_test.go` (the check that this stayed
		// off), the office desk's `chairs:` in internal/mode/modes/
		// specialized.md, and the `tools:` line of each agent under
		// profiles/agents that should hold it. Its category, its size pin, its
		// coverage case and every `deny:` naming it are already written and
		// need nothing.
		// One name, three actions: errors, symbol, map (codebase_pack.go).
		// Open (unfocused) rides in so `map` can refuse to map "the whole
		// machine" — a project map is only honest with a project to stand in
		// (docs/aider-study/EXECUTION.md, ที่อยู่ในบริษัท).
		&codebaseSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, open: opts.OpenSandbox},
		// Not one of them: same language server, but it writes, so it sits on
		// the other side of every gate the pack was drawn along.
		&renameSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, files: opts.Files},
		&pluginInstallSkill{},
		// The automation engines the user connected. Registered unconditionally
		// and gated later by connect.Allows, the same as the github tool: the
		// registry is what this build CAN do, and what a given desk may carry is
		// a different question with a different answer. One name per engine —
		// five actions inside each (packed.go) — which is what §97's "ten engine
		// tools now sit in the registry" cost note deflated to.
		&n8nSkill{},
		&windmillSkill{},
		// One name, three actions: image, video, audio (media_pack.go).
		&mediaReadSkill{root: opts.SandboxRoot, speech: opts.Speech},
		&pdfReadSkill{root: opts.SandboxRoot},
		&webFetchSkill{},
		// The download step of the picture-and-sound recipe, owned by the
		// system (media_fetch.go): web_fetch lists a page's file URLs, this
		// saves one and refuses a body that is not the media it claims.
		&mediaFetchSkill{root: opts.SandboxRoot, outputSubdir: opts.OutputSubdir, files: opts.Files},
		&webSearchSkill{},
		// One name, four actions: search, repo_summary, list_files, read_file
		// (github_pack.go). `plugin_install` above is not one of them — it
		// installs rather than reads, which is a different grant.
		&githubSkill{},
		// Pull requests, five actions (pr_pack.go). Registered unconditionally
		// and gated by connect.Allows like `github`: the registry is what this
		// build CAN do, and whether an account is connected is a different
		// question with a different answer.
		&prSkill{root: opts.SandboxRoot},
		// Progressive skill loading (see progressive.go): these two flat
		// definitions are how the model reaches every SKILL.md, in place of
		// one definition per discovered skill.
		&skillsListSkill{},
		&skillViewSkill{},
	}
	for _, s := range defaults {
		if err := registry.Register(s, SourceBuiltin); err != nil {
			// Two built-ins sharing a name is a programmer error, not a runtime condition.
			panic(err)
		}
	}
}
