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
		{"full-access high-risk shell skips", ApprovalFullAccess, highShell, false},
		{"full-access low-risk shell skips", ApprovalFullAccess, lowShell, false},
		{"full-access write skips", ApprovalFullAccess, writeFile, false},
		{"full-access read skips", ApprovalFullAccess, readOnly, false},
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
		{"edit", "edit", []string{"a.txt"}, RiskHigh},
		{"delete", "delete", []string{"a.txt"}, RiskHigh},
		{"grep", "grep", []string{"foo"}, RiskLow},
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

// audio_transcribe reads a file the user pointed at and shells out to local
// binaries to do it — exactly what video_ocr does, so it must land in the same
// tier and never grow an approval prompt video_ocr doesn't have.
func TestAssessCommandMediaToolsMatchVideoOCR(t *testing.T) {
	reference := AssessCommand("video_ocr", []string{"clip.mp4"})
	for _, name := range []string{"audio_transcribe", "image_ocr"} {
		got := AssessCommand(name, []string{"clip.mp4"})
		if got.Risk != reference.Risk || len(got.Effects) != len(reference.Effects) {
			t.Errorf("AssessCommand(%q) = risk %v effects %v, want the video_ocr tier: risk %v effects %v",
				name, got.Risk, got.Effects, reference.Risk, reference.Effects)
		}
		for _, mode := range []ApprovalMode{ApprovalFullAccess, ApprovalUnsafeOnly, ApprovalMode("")} {
			if ShouldPrompt(mode, got) != ShouldPrompt(mode, reference) {
				t.Errorf("%s prompts differently from video_ocr under mode %q", name, mode)
			}
		}
	}
}

// calc asks for no permission, and the reason has to be recorded rather than
// inherited from the fallback every unrecognised name lands in: a tool that
// runs in this process and can reach no file, socket or program is not asking
// for a right. A named assessment is also the only way this stays true — the
// day calc grows a way out, this test is where it should stop being silent.
func TestCalcIsAssessedAsAskingForNothing(t *testing.T) {
	got := AssessCommand("calc", []string{"1234 * 5678"})

	if got.Risk != RiskLow || len(got.Effects) != 0 {
		t.Errorf("AssessCommand(calc) = risk %v effects %v, want low risk and no effects", got.Risk, got.Effects)
	}
	if got.Reason == "" {
		t.Error("calc fell through to the unrecognised-name fallback — the answer is right by accident, not on purpose")
	}
	for _, mode := range []ApprovalMode{ApprovalFullAccess, ApprovalUnsafeOnly, ApprovalAsk, ApprovalMode("")} {
		if ShouldPrompt(mode, got) {
			t.Errorf("calc prompts for approval under mode %q", mode)
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
