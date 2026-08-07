package safety

import (
	"regexp"
	"strings"
)

type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskHigh
)

type ApprovalMode string

const (
	ApprovalAsk        ApprovalMode = "ask"
	ApprovalUnsafeOnly ApprovalMode = "unsafe-only"
	ApprovalFullAccess ApprovalMode = "full-access"
)

var ValidApprovalModes = map[ApprovalMode]bool{
	ApprovalAsk:        true,
	ApprovalUnsafeOnly: true,
	ApprovalFullAccess: true,
}

func NormalizeApprovalMode(raw string) ApprovalMode {
	mode := ApprovalMode(strings.ToLower(strings.TrimSpace(raw)))
	if ValidApprovalModes[mode] {
		return mode
	}
	return ApprovalAsk
}

// PermissionAction is a user-configured override for a specific tool/pattern,
// taking precedence over the coarse ApprovalMode when it matches. Mirrors
// opencode's per-tool pattern permission model (see
// docs/architecture-reference-opencode.md §4.5).
type PermissionAction string

const (
	PermissionAllow PermissionAction = "allow"
	PermissionAsk   PermissionAction = "ask"
	PermissionDeny  PermissionAction = "deny"
)

func NormalizePermissionAction(raw string) PermissionAction {
	switch PermissionAction(strings.ToLower(strings.TrimSpace(raw))) {
	case PermissionAllow:
		return PermissionAllow
	case PermissionAsk:
		return PermissionAsk
	case PermissionDeny:
		return PermissionDeny
	default:
		return ""
	}
}

// PermissionRule matches a tool call by tool name and an args pattern, both
// glob-style ("*" any sequence, "?" any single char). Pattern "" behaves like
// "*" (matches any args).
type PermissionRule struct {
	Tool    string           `json:"tool"`
	Pattern string           `json:"pattern"`
	Action  PermissionAction `json:"action"`
	// Default marks a rule the app generated rather than the user writing it —
	// today, the "ask before anything from this MCP server" rule that bootstrap
	// attaches per configured server.
	//
	// It exists because those two kinds of rule must lose differently. A rule
	// the user wrote is their decision and outranks everything, mode included.
	// A default is the app's opening position, and it must not outrank the mode
	// the user then went and chose: full access says "รับทุกอย่างโดยไม่ถาม" on
	// the card the user clicks, and an app-generated ask that survives it makes
	// that sentence false (found 2026-08-06 — full access, and every MCP call
	// still opened a dialog).
	//
	// Not persisted: a rule read back from the user's permissions file is by
	// definition the user's, so the zero value is the right one for it.
	Default bool `json:"-"`
}

// PermissionConfig is an ordered list of rules; the last matching rule wins,
// same semantics as opencode's permission object.
type PermissionConfig struct {
	Rules []PermissionRule `json:"rules"`
}

// Resolve returns the action of the last rule matching toolName+args, and
// whether any rule matched at all. Callers should fall back to
// ShouldPrompt/ApprovalMode when matched is false.
//
// ResolveRule is the same answer with the winning rule attached, for the one
// caller that needs to know whether the match was the app's default or the
// user's own decision.
func (c PermissionConfig) Resolve(toolName string, args []string) (action PermissionAction, matched bool) {
	rule, matched := c.ResolveRule(toolName, args)
	return NormalizePermissionAction(string(rule.Action)), matched
}

// ResolveRule returns the last rule matching toolName+args.
func (c PermissionConfig) ResolveRule(toolName string, args []string) (winner PermissionRule, matched bool) {
	tool := strings.ToLower(strings.TrimSpace(toolName))
	joinedArgs := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	for _, rule := range c.Rules {
		normalized := NormalizePermissionAction(string(rule.Action))
		if normalized == "" {
			continue
		}
		if !globMatch(strings.ToLower(strings.TrimSpace(rule.Tool)), tool) {
			continue
		}
		pattern := strings.ToLower(strings.TrimSpace(rule.Pattern))
		if pattern == "" {
			pattern = "*"
		}
		if !globMatch(pattern, joinedArgs) {
			continue
		}
		rule.Action = normalized
		winner, matched = rule, true
	}
	return winner, matched
}

// globMatch reports whether s matches pattern, where "*" matches any
// (possibly empty) run of characters and "?" matches exactly one.
func globMatch(pattern, s string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

type Effect string

const (
	EffectReadWorkspace         Effect = "read-workspace"
	EffectWriteWorkspace        Effect = "write-workspace"
	EffectDeleteWorkspace       Effect = "delete-workspace"
	EffectMutateGit             Effect = "mutate-git"
	EffectExecuteShell          Effect = "execute-shell"
	EffectUseNetwork            Effect = "use-network"
	EffectTouchOutsideWorkspace Effect = "touch-outside-workspace"
)

type Assessment struct {
	SkillName string
	Risk      RiskLevel
	Effects   []Effect
	Reason    string
}

func ShouldPrompt(mode ApprovalMode, a Assessment) bool {
	switch mode {
	case ApprovalFullAccess:
		// No prompts at all — matching what full access means in the reference
		// implementations (opencode's allow-all, Claude Code's bypassPermissions).
		// Shell used to be carved out and still prompted here, which made the
		// desktop fire a dialog per command; a user who wants shell gated has
		// unsafe-only and ask for exactly that.
		return false
	case ApprovalUnsafeOnly:
		for _, e := range a.Effects {
			switch e {
			case EffectDeleteWorkspace, EffectMutateGit, EffectExecuteShell, EffectTouchOutsideWorkspace:
				return true
			}
		}
		return false
	default:
		if a.Risk == RiskHigh {
			return true
		}
		for _, e := range a.Effects {
			if e != EffectReadWorkspace {
				return true
			}
		}
		return false
	}
}

func AssessCommand(skillName string, args []string) Assessment {
	skillName = strings.ToLower(strings.TrimSpace(skillName))
	if skillName == "" {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskLow,
			Effects:   nil,
			Reason:    "no recognized command",
		}
	}

	// desk_terminal types its argument into a real shell on the user's desk and
	// presses enter. That is `shell` with an audience, so it is assessed as
	// shell and reaches every gate shell reaches. Letting it fall through to the
	// catch-all below would have made it a second, quieter way to run any
	// command — the exact failure the sheet_write note further down describes,
	// with a worse blast radius.
	if skillName == "desk_terminal" {
		if len(args) == 0 {
			// No command — an empty terminal, opened for the user to type into
			// themselves. Nothing runs until they do, and what they then type is
			// theirs, not the agent's. Falling through to the shell branch here
			// would assess it as "shell with an empty command", which is high
			// risk by design and would put a prompt in front of opening a window.
			return Assessment{
				SkillName: "desk_terminal",
				Risk:      RiskLow,
				Reason:    "opens an empty terminal; nothing runs until the user types",
			}
		}
		assessed := AssessCommand("shell", args)
		assessed.SkillName = "desk_terminal"
		return assessed
	}

	if skillName != "shell" {
		if skillName == "git" {
			return assessGitCommand(args)
		}
		if skillName == "fs" {
			return assessFsCommand(args)
		}
		if skillName == "write" {
			return Assessment{
				SkillName: "write",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectWriteWorkspace},
				Reason:    "write can create or overwrite repository files",
			}
		}
		// sheet_write creates files exactly as write does, so it is assessed
		// exactly as write is. Without an entry here it falls through to the
		// catch-all at the bottom of this branch, which answers RiskLow with no
		// effects — and a tool that writes to disk would then skip the approval
		// gate that write, edit and delete all pass through.
		if skillName == "sheet_write" {
			return Assessment{
				SkillName: "sheet_write",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectWriteWorkspace},
				Reason:    "sheet_write can create or overwrite a workbook file",
			}
		}
		if skillName == "slides_write" {
			return Assessment{
				SkillName: "slides_write",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectWriteWorkspace},
				Reason:    "slides_write can create or overwrite a presentation file",
			}
		}
		if skillName == "doc_write" {
			return Assessment{
				SkillName: "doc_write",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectWriteWorkspace},
				Reason:    "doc_write can create or overwrite a document file",
			}
		}
		if skillName == "edit" {
			return Assessment{
				SkillName: "edit",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectWriteWorkspace},
				Reason:    "edit can modify repository files",
			}
		}
		if skillName == "delete" {
			return Assessment{
				SkillName: "delete",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectDeleteWorkspace},
				Reason:    "delete can remove repository files",
			}
		}
		if skillName == "plugin_install" {
			return Assessment{
				SkillName: "plugin_install",
				Risk:      RiskHigh,
				Effects:   []Effect{EffectTouchOutsideWorkspace},
				Reason:    "plugin install can write files outside the repository",
			}
		}
		// All github_* skills are read-only API calls (plugin_install, the
		// one that writes, is handled above this branch).
		if strings.HasPrefix(skillName, "github_") {
			return Assessment{
				SkillName: skillName,
				Risk:      RiskLow,
				Effects:   []Effect{EffectUseNetwork},
				Reason:    "read-only GitHub API request",
			}
		}
		if skillName == "web_fetch" || skillName == "web_search" {
			return Assessment{
				SkillName: skillName,
				Risk:      RiskLow,
				Effects:   []Effect{EffectUseNetwork},
				Reason:    "read-only web request",
			}
		}
		// calc would reach this file's fallback anyway — RiskLow, no effects,
		// no prompt — and that is the right answer for a tool that runs in this
		// process with nothing to reach. It is written down because the right
		// answer arrived at by accident is indistinguishable from a name nobody
		// recognised, which is precisely how a writing tool once skipped the
		// approval prompt (§75).
		if skillName == "calc" {
			return Assessment{
				SkillName: skillName,
				Risk:      RiskLow,
				Effects:   nil,
				Reason:    "runs in-process with no access to files, the network or other programs",
			}
		}
		if skillName == "list" || skillName == "read" || skillName == "grep" || skillName == "time" {
			return Assessment{
				SkillName: skillName,
				Risk:      RiskLow,
				Effects:   []Effect{EffectReadWorkspace},
			}
		}
		return Assessment{
			SkillName: skillName,
			Risk:      RiskLow,
			Effects:   nil,
		}
	}

	if len(args) == 0 {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskHigh,
			Effects:   []Effect{EffectExecuteShell},
			Reason:    "shell with empty command can block or no-op unexpectedly",
		}
	}

	if isShellHighRisk(args[0], args[1:]) {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskHigh,
			Effects:   []Effect{EffectExecuteShell},
			Reason:    "shell action may modify or delete state",
		}
	}

	return Assessment{
		SkillName: skillName,
		Risk:      RiskLow,
		Effects:   []Effect{EffectExecuteShell},
	}
}

func isShellHighRisk(cmd string, rest []string) bool {
	token := strings.ToLower(strings.TrimSpace(cmd))
	if token == "" {
		return true
	}

	switch token {
	case "rm", "del", "erase", "rmdir", "rd", "mv", "move", "rename", "format", "mkfs",
		"shred", "sdelete", "takeown", "icacls", "attrib", "cacls", "chown", "chmod", "cd",
		"shutdown", "reboot", "halt", "poweroff", "kill", "taskkill":
		return true
	}

	for _, arg := range rest {
		norm := strings.ToLower(strings.TrimSpace(arg))
		if norm == "-rf" || norm == "-rm" || strings.HasPrefix(norm, "/s") || strings.HasPrefix(norm, "/q") {
			return true
		}
	}

	for _, marker := range []string{"--recursive", "-rf", "/s", "/q", "-f", "--force"} {
		for _, value := range rest {
			if strings.EqualFold(strings.TrimSpace(value), marker) {
				return true
			}
		}
	}

	return false
}

func assessGitCommand(args []string) Assessment {
	if len(args) == 0 {
		return Assessment{
			SkillName: "git",
			Risk:      RiskHigh,
			Effects:   []Effect{EffectMutateGit},
			Reason:    "missing git action",
		}
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "status", "log", "branch", "diff", "show":
		return Assessment{
			SkillName: "git",
			Risk:      RiskLow,
			Effects:   []Effect{EffectReadWorkspace},
		}
	case "fetch":
		return Assessment{
			SkillName: "git",
			Risk:      RiskHigh,
			Effects:   []Effect{EffectMutateGit, EffectUseNetwork},
			Reason:    "fetch may change local git state and should be confirmed",
		}
	case "add", "commit", "restore", "reset", "rebase", "clean", "switch", "checkout", "merge", "push", "pull", "mv", "move", "rm", "stash", "tag":
		return Assessment{
			SkillName: "git",
			Risk:      RiskHigh,
			Effects:   []Effect{EffectMutateGit},
			Reason:    "git action may change repository state",
		}
	default:
		return Assessment{
			SkillName: "git",
			Risk:      RiskHigh,
			Effects:   []Effect{EffectMutateGit},
			Reason:    "unsupported or potentially destructive git action",
		}
	}
}

func assessFsCommand(args []string) Assessment {
	if len(args) == 0 {
		return Assessment{
			SkillName: "fs",
			Risk:      RiskHigh,
			Effects:   []Effect{EffectWriteWorkspace},
			Reason:    "missing fs action",
		}
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "pwd", "ls", "find", "cat":
		return Assessment{
			SkillName: "fs",
			Risk:      RiskLow,
			Effects:   []Effect{EffectReadWorkspace},
		}
	default:
		return Assessment{
			SkillName: "fs",
			Risk:      RiskHigh,
			Effects:   []Effect{EffectWriteWorkspace},
			Reason:    "unsupported fs action",
		}
	}
}
