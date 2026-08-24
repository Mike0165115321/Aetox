package turn

// The action inside a packed tool, on its way to the UI.
//
// Packing (§99) put a dozen capabilities behind one name, so a UI holding a
// tool event has always been able to see that `browser` was busy and not one
// thing more — which is the whole reason ไฟบอกสถานะ could not say what was
// being done. What is worth pinning is not that a field copies across: it is
// that the call event and the result event read the arguments the SAME way, so
// a UI matching the pair up never sees an action close that it did not see
// open.

import "testing"

func TestPackedActionOfReadsTheActionKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		args string
		want string
	}{
		{"a browser call", `{"action":"click","ref":3}`, "click"},
		{"a delegation", `{"action":"start","agent":"doc"}`, "start"},
		// Models send what they like, and a panel comparing against "click"
		// should not be defeated by a capital letter or a stray space.
		{"shouted and padded", `{"action":"  Click "}`, "click"},
		{"an unpacked tool", `{"path":"internal/skill/edit.go"}`, ""},
		{"arguments that will not parse", `{"action":`, ""},
		{"an action that is not a string", `{"action":7}`, ""},
	} {
		if got := packedActionOf(tc.args); got != tc.want {
			t.Errorf("%s: packedActionOf(%s) = %q, want %q", tc.name, tc.args, got, tc.want)
		}
	}
}

// The pair. A call and its result are matched up by ref in the UI, and the
// action is what the words are chosen from — so a result carrying a different
// action from its own call would close a sentence nobody had started.
func TestCallAndResultAgreeOnTheAction(t *testing.T) {
	var seen []ToolEvent
	e := &Executor{onToolAction: func(ev ToolEvent) { seen = append(seen, ev) }}

	args := `{"action":"scroll","to":"bottom"}`
	e.reportToolCall("call_1", "browser", args)
	e.reportToolResult(ToolEvent{Ref: "call_1", Name: "browser", Act: packedActionOf(args), OK: true})

	if len(seen) != 2 {
		t.Fatalf("want a call and a result, got %d events", len(seen))
	}
	if seen[0].Action != "call" || seen[1].Action != "result" {
		t.Fatalf("got actions %q and %q", seen[0].Action, seen[1].Action)
	}
	for i, ev := range seen {
		if ev.Act != "scroll" {
			t.Errorf("event %d: Act = %q, want scroll", i, ev.Act)
		}
	}
}

// Tab is declared here and filled in by the host, so `turn` must leave it
// alone. If the executor ever started guessing at it, the panel would point at
// a tab chosen by the side that has never heard of a browser.
func TestExecutorLeavesTabToTheHost(t *testing.T) {
	var seen []ToolEvent
	e := &Executor{onToolAction: func(ev ToolEvent) { seen = append(seen, ev) }}
	e.reportToolCall("call_1", "browser", `{"action":"open","url":"https://a.test"}`)
	if len(seen) != 1 {
		t.Fatalf("want one event, got %d", len(seen))
	}
	if seen[0].Tab != "" {
		t.Errorf("Tab = %q, want it left empty for the host to stamp", seen[0].Tab)
	}
	// The subject still travels the way it always did — the new field is beside
	// it, not instead of it.
	if seen[0].Subject != "https://a.test" {
		t.Errorf("Subject = %q, want the URL", seen[0].Subject)
	}
}
