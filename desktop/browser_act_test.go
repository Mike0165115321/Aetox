package main

// Two bugs found by reading tool_runs on 2026-08-22, after a full pass of the
// browser tools reported 8/8 and clicks had nonetheless done nothing.
//
// The log said it plainly: args {"action":"click","ref":"1"} and output
// "คลิก ref 0 แล้ว", ok=1, twelve times across this machine's history. The model
// quoted the number, the desktop's own intArg handled int and float64 but not
// string, and clickScript's `if(!el)return` swallowed the miss. So the tool
// reported a successful click on an element that does not exist.
//
// The type coercion is the smaller half. The bigger one is that a tool which
// reports success for work it did not do turns every bug upstream of it into a
// loop: the model read the page, saw nothing had changed, clicked again,
// reopened the page, clicked again — six rounds, because the sentence that
// would have ended it was one nobody was saying.

import (
	"strings"
	"testing"
)

func TestQuotedArgumentsAreNotSilentlyZero(t *testing.T) {
	// The exact shape out of tool_runs. This is the regression.
	if got := intArg("1"); got != 1 {
		t.Errorf(`intArg("1") = %d, want 1 — a model that quotes the number still means the number`, got)
	}
	for _, c := range []struct {
		in   any
		want int
	}{{1, 1}, {float64(2), 2}, {"3", 3}, {" 4 ", 4}, {"", 0}, {"not a number", 0}, {nil, 0}} {
		if got := intArg(c.in); got != c.want {
			t.Errorf("intArg(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
	for _, c := range []struct {
		in   any
		want bool
	}{{true, true}, {"true", true}, {"TRUE", true}, {false, false}, {"false", false}, {"", false}, {nil, false}} {
		if got := boolArg(c.in); got != c.want {
			t.Errorf("boolArg(%#v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestActionsReportBeforeTheyAct(t *testing.T) {
	// Order is the assertion. A click can navigate, and a navigation tears down
	// the document that would have sent the report — so a report written after
	// the click is one that never arrives for exactly the clicks that matter.
	click := clickScript("tok", 3)
	report, act := strings.Index(click, "aetoxReport("), strings.Index(click, "el.click()")
	if report < 0 || act < 0 || report > act {
		t.Errorf("clickScript must report before it clicks, got:\n%s", click)
	}

	typed := typeScript("tok", 3, "x", false)
	report, act = strings.Index(typed, "aetoxReport("), strings.Index(typed, "el.focus()")
	if report < 0 || act < 0 || report > act {
		t.Errorf("typeScript must report before it types, got:\n%s", typed)
	}
}

func TestActionsReportEvenWhenTheRefMatchesNothing(t *testing.T) {
	// The whole bug in one assertion: the report has to come out ABOVE the
	// early return, or a ref that matches nothing produces silence, and silence
	// is what the old code turned into "clicked".
	for name, js := range map[string]string{"click": clickScript("tok", 9), "type": typeScript("tok", 9, "x", false)} {
		report, bail := strings.Index(js, "aetoxReport("), strings.Index(js, "if(!el)return;")
		if report < 0 || bail < 0 || report > bail {
			t.Errorf("%sScript must report before giving up on a missing ref, got:\n%s", name, js)
		}
		if !strings.Contains(js, "found:!!el") {
			t.Errorf("%sScript must say whether the ref matched, got:\n%s", name, js)
		}
	}
}

func TestActLabelNamesWhatWasHit(t *testing.T) {
	got := browserActLabel(2, browserActResult{Found: true, Ref: 2, Tag: "button", Label: "ปุ่มในเฟรม"}, true)
	// "คลิก ref 2 แล้ว" cannot be checked from outside the page. The tag and the
	// label are what let a caller see the action landed on what it meant.
	if !strings.Contains(got, "button") || !strings.Contains(got, "ปุ่มในเฟรม") {
		t.Errorf("a landed action must name the element, got: %s", got)
	}

	quiet := browserActLabel(2, browserActResult{}, false)
	if !strings.Contains(quiet, "ไม่ได้ยืนยัน") {
		t.Errorf("an unconfirmed action must not read like a confirmed one, got: %s", quiet)
	}
}
