package safety

import "strings"

type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskHigh
)

type Assessment struct {
	SkillName string
	Risk      RiskLevel
	Reason    string
}

func AssessCommand(skillName string, args []string) Assessment {
	skillName = strings.ToLower(strings.TrimSpace(skillName))
	if skillName == "" {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskLow,
			Reason:    "no recognized command",
		}
	}

	if skillName != "shell" {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskLow,
		}
	}

	if len(args) == 0 {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskHigh,
			Reason:    "shell with empty command can block or no-op unexpectedly",
		}
	}

	if isShellHighRisk(args[0], args[1:]) {
		return Assessment{
			SkillName: skillName,
			Risk:      RiskHigh,
			Reason:    "shell action may modify or delete state",
		}
	}

	return Assessment{
		SkillName: skillName,
		Risk:      RiskLow,
	}
}

func isShellHighRisk(cmd string, rest []string) bool {
	token := strings.ToLower(strings.TrimSpace(cmd))
	if token == "" {
		return true
	}

	switch token {
	case "rm", "del", "erase", "rmdir", "rd", "mv", "move", "rename", "format", "mkfs",
		"shred", "sdelete", "takeown", "icacls", "attrib", "cacls", "chown", "chmod",
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
