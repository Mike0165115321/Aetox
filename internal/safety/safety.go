package safety

import "strings"

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
		if len(a.Effects) > 0 {
			for _, e := range a.Effects {
				if e != EffectReadWorkspace {
					return true
				}
			}
		}
		return a.Risk == RiskHigh
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
