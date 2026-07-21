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

func ApprovalModeFromLegacy(autoApprove bool) ApprovalMode {
	if autoApprove {
		return ApprovalFullAccess
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
}

// PermissionConfig is an ordered list of rules; the last matching rule wins,
// same semantics as opencode's permission object.
type PermissionConfig struct {
	Rules []PermissionRule `json:"rules"`
}

// Resolve returns the action of the last rule matching toolName+args, and
// whether any rule matched at all. Callers should fall back to
// ShouldPrompt/ApprovalMode when matched is false.
func (c PermissionConfig) Resolve(toolName string, args []string) (action PermissionAction, matched bool) {
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
		action, matched = normalized, true
	}
	return action, matched
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
		if skillName == "github_repo_summary" {
			return Assessment{
				SkillName: "github_repo_summary",
				Risk:      RiskLow,
				Effects:   []Effect{EffectUseNetwork},
				Reason:    "read-only network request for repository summary",
			}
		}
		if skillName == "list" || skillName == "read" || skillName == "time" {
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
