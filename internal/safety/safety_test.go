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

func TestPermissionConfigResolve(t *testing.T) {
	cfg := PermissionConfig{Rules: []PermissionRule{
		{Tool: "*", Pattern: "*", Action: PermissionAsk},
		{Tool: "git", Pattern: "status", Action: PermissionAllow},
		{Tool: "shell", Pattern: "rm *", Action: PermissionDeny},
		{Tool: "shell", Pattern: "rm -rf /tmp/*", Action: PermissionAllow},
	}}

	cases := []struct {
		name        string
		tool        string
		args        []string
		wantAction  PermissionAction
		wantMatched bool
	}{
		{"no rules matches catch-all ask", "read", []string{"a.txt"}, PermissionAsk, true},
		{"specific allow overrides catch-all", "git", []string{"status"}, PermissionAllow, true},
		{"git push only matches catch-all", "git", []string{"push"}, PermissionAsk, true},
		{"shell rm matches deny", "shell", []string{"rm", "file.txt"}, PermissionDeny, true},
		{"last matching rule wins over earlier deny", "shell", []string{"rm", "-rf", "/tmp/scratch"}, PermissionAllow, true},
	}
	for _, tc := range cases {
		action, matched := cfg.Resolve(tc.tool, tc.args)
		if matched != tc.wantMatched || action != tc.wantAction {
			t.Errorf("%s: Resolve(%q, %v) = (%q, %v), want (%q, %v)", tc.name, tc.tool, tc.args, action, matched, tc.wantAction, tc.wantMatched)
		}
	}

	if action, matched := (PermissionConfig{}).Resolve("read", nil); matched || action != "" {
		t.Errorf("empty config should never match, got (%q, %v)", action, matched)
	}
}
