package safety

import "testing"

func TestShouldPrompt(t *testing.T) {
	readOnly := Assessment{Risk: RiskLow, Effects: []Effect{EffectReadWorkspace}}
	lowShell := Assessment{Risk: RiskLow, Effects: []Effect{EffectExecuteShell}}
	highShell := Assessment{Risk: RiskHigh, Effects: []Effect{EffectExecuteShell}}
	writeFile := Assessment{Risk: RiskHigh, Effects: []Effect{EffectWriteWorkspace}}
	network := Assessment{Risk: RiskLow, Effects: []Effect{EffectUseNetwork}}

	cases := []struct {
		name string
		mode ApprovalMode
		a    Assessment
		want bool
	}{
		{"ask read-only", ApprovalAsk, readOnly, false},
		{"ask low-risk shell still prompts", ApprovalAsk, lowShell, true},
		{"ask high-risk shell", ApprovalAsk, highShell, true},
		{"ask write", ApprovalAsk, writeFile, true},
		{"ask network", ApprovalAsk, network, true},
		{"unsafe-only shell prompts", ApprovalUnsafeOnly, lowShell, true},
		{"unsafe-only write skips", ApprovalUnsafeOnly, writeFile, false},
		{"unsafe-only read skips", ApprovalUnsafeOnly, readOnly, false},
		{"full-access never prompts", ApprovalFullAccess, highShell, false},
		{"full-access write skips", ApprovalFullAccess, writeFile, false},
	}
	for _, tc := range cases {
		if got := ShouldPrompt(tc.mode, tc.a); got != tc.want {
			t.Errorf("%s: ShouldPrompt(%q) = %v, want %v", tc.name, tc.mode, got, tc.want)
		}
	}
}

func TestAssessCommand(t *testing.T) {
	cases := []struct {
		name     string
		skill    string
		args     []string
		wantRisk RiskLevel
	}{
		{"shell benign", "shell", []string{"echo", "hi"}, RiskLow},
		{"shell rm", "shell", []string{"rm", "file.txt"}, RiskHigh},
		{"shell chained delete flags", "shell", []string{"echo", "hi", "&&", "del", "/s", "/q", "*"}, RiskHigh},
		{"shell force flag", "shell", []string{"git", "push", "--force"}, RiskHigh},
		{"shell empty", "shell", nil, RiskHigh},
		{"git status", "git", []string{"status"}, RiskLow},
		{"git push", "git", []string{"push"}, RiskHigh},
		{"git unknown action", "git", []string{"frobnicate"}, RiskHigh},
		{"fs read", "fs", []string{"cat", "a.txt"}, RiskLow},
		{"fs unknown", "fs", []string{"chmod"}, RiskHigh},
		{"write", "write", []string{"a.txt", "x"}, RiskHigh},
		{"delete", "delete", []string{"a.txt"}, RiskHigh},
		{"plugin_install", "plugin_install", []string{"https://github.com/a/b"}, RiskHigh},
		{"read", "read", []string{"a.txt"}, RiskLow},
	}
	for _, tc := range cases {
		got := AssessCommand(tc.skill, tc.args)
		if got.Risk != tc.wantRisk {
			t.Errorf("%s: AssessCommand(%q, %v).Risk = %v, want %v", tc.name, tc.skill, tc.args, got.Risk, tc.wantRisk)
		}
	}
}

func TestNormalizeApprovalMode(t *testing.T) {
	if got := NormalizeApprovalMode(" Full-Access "); got != ApprovalFullAccess {
		t.Errorf("NormalizeApprovalMode trims/lowers: got %q", got)
	}
	if got := NormalizeApprovalMode("bogus"); got != ApprovalAsk {
		t.Errorf("invalid mode should fall back to ask: got %q", got)
	}
}
