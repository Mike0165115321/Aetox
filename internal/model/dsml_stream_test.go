package model

import (
	"strings"
	"testing"
)

// collect drives a gate with the given deltas and returns everything delivered.
func collect(t *testing.T, deltas ...string) string {
	t.Helper()
	var got strings.Builder
	gate := newDSMLGate(func(chunk string) error {
		got.WriteString(chunk)
		return nil
	})
	for _, d := range deltas {
		if err := gate.write(d); err != nil {
			t.Fatalf("write(%q): %v", d, err)
		}
	}
	return got.String()
}

func TestGatePassesOrdinaryProseThrough(t *testing.T) {
	// Split the way a provider actually delivers it — a few characters at a time.
	if got := collect(t, "ชาร์จ", "ไปเรื่อยๆ ", "ครับ"); got != "ชาร์จไปเรื่อยๆ ครับ" {
		t.Errorf("delivered %q; want the prose intact", got)
	}
}

// The whole reason this is a gate and not a filter: the marker arrives split
// across deltas, which is the normal case for a tokenized stream.
func TestGateHoldsAMarkerSplitAcrossDeltas(t *testing.T) {
	got := collect(t, "กำลังสร้างไฟล์ให้ครับ\n", "<｜DS", "ML｜invoke name=\"write\">", "<｜DSML｜parameter")
	if got != "กำลังสร้างไฟล์ให้ครับ\n" {
		t.Errorf("delivered %q; want only the prose before the markup", got)
	}
	if strings.Contains(got, "DSML") {
		t.Fatal("leaked markup reached the screen — the whole point of the gate")
	}
}

func TestGateHandlesTheMarkerArrivingWhole(t *testing.T) {
	got := collect(t, "ขอโทษครับ\n<｜DSML｜tool_calls>\n<｜DSML｜invoke name=\"read\">")
	if got != "ขอโทษครับ\n" {
		t.Errorf("delivered %q; want the apology and nothing after it", got)
	}
}

// Once shut it stays shut: everything after an opening marker belongs to the
// block, and the tool loop is about to discard this round's text anyway.
func TestGateStaysShutForTheRestOfTheCall(t *testing.T) {
	got := collect(t, "<｜DSML｜invoke name=\"write\">", "</｜DSML｜invoke>", "and now some prose")
	if got != "" {
		t.Errorf("delivered %q after latching shut; want nothing", got)
	}
}

// The ASCII spelling dsml.go tolerates has to be caught too.
func TestGateCatchesTheASCIISpelling(t *testing.T) {
	if got := collect(t, "before <|DSML|tool_calls> after"); got != "before " {
		t.Errorf("delivered %q; want the ASCII marker gated as well", got)
	}
	// And the mixed spellings the regex's character class allows.
	if got := collect(t, "x<|DSML｜invoke"); got != "x" {
		t.Errorf("delivered %q; want a mixed-pipe marker gated", got)
	}
}

// A '<' is ordinary in prose and extremely common in code answers. Holding one
// back forever, or mistaking it for markup, would break every HTML answer.
func TestGateReleasesOrdinaryAngleBrackets(t *testing.T) {
	cases := []struct{ name, in string }{
		{"html", "<div class=\"x\">hello</div>"},
		{"comparison", "if a < b and c <= d then"},
		{"generic", "List<String> xs = new ArrayList<>()"},
		{"lone bracket mid-text", "a < b"},
		{"word DSML in prose", "the DSML markup is a DeepSeek quirk"},
		{"close tag without an open", "</｜DSML｜parameter>"},
	}
	for _, c := range cases {
		if got := collect(t, c.in); got != c.in {
			t.Errorf("%s: delivered %q; want %q untouched", c.name, got, c.in)
		}
	}
}

// A trailing '<' is a legitimate marker prefix, so it is withheld — but only
// the tail, never the answer in front of it.
func TestGateWithholdsOnlyTheCandidateTail(t *testing.T) {
	if got := collect(t, "the answer is 42 <"); got != "the answer is 42 " {
		t.Errorf("delivered %q; want everything but the candidate tail", got)
	}
	// ...and releases it the moment it proves to be prose.
	if got := collect(t, "the answer is 42 <", "b"); got != "the answer is 42 <b" {
		t.Errorf("delivered %q; want the '<' released once it could not be a marker", got)
	}
}

// A multi-byte rune split across deltas must not be judged half-decoded — the
// fullwidth pipe is three bytes, so this is the marker's own failure mode.
func TestGateSurvivesARuneSplitAcrossDeltas(t *testing.T) {
	marker := "<｜DSML｜invoke"
	// Cut in the middle of the first fullwidth pipe (byte 2 of "<" + 3 bytes).
	head, tail := marker[:3], marker[3:]
	if got := collect(t, "prose "+head, tail); got != "prose " {
		t.Errorf("delivered %q; want the split marker still caught", got)
	}
}

// Every prose byte must arrive exactly once and in order — a gate that
// duplicated or reordered its buffer would corrupt every answer.
func TestGateNeitherDuplicatesNorReordersProse(t *testing.T) {
	full := "Here is <b>bold</b> and a < b comparison, plus List<int>."
	var deltas []string
	for i := 0; i < len(full); i += 3 {
		end := i + 3
		if end > len(full) {
			end = len(full)
		}
		deltas = append(deltas, full[i:end])
	}
	if got := collect(t, deltas...); got != full {
		t.Errorf("delivered %q; want %q byte-for-byte", got, full)
	}
}

func TestGateIsNilSafe(t *testing.T) {
	var gate *dsmlGate
	if gate.handler() != nil {
		t.Error("a nil gate produced a non-nil handler")
	}
	if err := gate.write("anything"); err != nil {
		t.Errorf("write on a nil gate: %v", err)
	}
	// A nil deliver means the caller wants no live content at all.
	if newDSMLGate(nil) != nil {
		t.Error("newDSMLGate(nil) must stay nil so providers see a nil handler")
	}
}

// The gate must agree with the backstop about what counts as markup, or one of
// them is wrong about the same bytes.
func TestGateAgreesWithContainsLeakedDSML(t *testing.T) {
	leaks := []string{
		"<｜DSML｜tool_calls>",
		"<｜DSML｜invoke name=\"write\">",
		"<|DSML|invoke name=\"read\">",
	}
	for _, leak := range leaks {
		if !ContainsLeakedDSML(leak) {
			t.Fatalf("test fixture %q is not a leak by dsml.go's own rule", leak)
		}
		if got := collect(t, "prose "+leak); got != "prose " {
			t.Errorf("gate delivered %q for a leak the backstop recognises", got)
		}
	}
}
