package think

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		raw     string
		want    Level
		wantErr bool
	}{
		{raw: "low", want: LevelLow},
		{raw: "medium", want: LevelMedium},
		{raw: "HIGH", want: LevelHigh},
		{raw: " auto ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseLevel(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("want %q got %q", tt.want, got)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	native := Resolve(LevelHigh, true)
	if native.Resolved != LevelHigh {
		t.Fatalf("expected high, got %q", native.Resolved)
	}
	if !native.Native || native.Downgraded {
		t.Fatalf("expected native profile, got %+v", native)
	}
	if native.ReasoningEffort() != "high" {
		t.Fatalf("unexpected reasoning effort: %q", native.ReasoningEffort())
	}

	fallback := Resolve(LevelLow, false)
	if fallback.Resolved != LevelLow {
		t.Fatalf("expected low, got %q", fallback.Resolved)
	}
	if fallback.Native || !fallback.Downgraded {
		t.Fatalf("expected downgraded profile, got %+v", fallback)
	}
	if fallback.ReasoningEffort() != "" {
		t.Fatalf("expected no reasoning effort, got %q", fallback.ReasoningEffort())
	}
	if fallback.StatusLabel() != "low (provider default fallback)" {
		t.Fatalf("unexpected status label: %q", fallback.StatusLabel())
	}
}
