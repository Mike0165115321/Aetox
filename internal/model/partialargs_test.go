package model

import (
	"testing"
	"time"
)

// The point of this helper is to name a tool call while its arguments are
// still arriving — a model writing a 300-line file streams `content` for a
// minute, and until then the UI has nothing to show.
func TestSubjectFromPartialArgs(t *testing.T) {
	cases := []struct {
		name, args, want string
		wantOK           bool
	}{
		{
			name: "path complete, content still streaming",
			args: `{"path": "landing.html", "content": "<!DOCTYPE html>\n<html lang=\"th`,
			want: "landing.html", wantOK: true,
		},
		{
			name: "path itself still arriving reports nothing yet",
			args: `{"path": "landing`, wantOK: false,
		},
		{"empty", ``, "", false},
		{"no nameable key", `{"limit": 10,`, "", false},
		{
			name: "escaped characters are unescaped",
			args: `{"path": "src\lib\a.ts", "content": "x`,
			want: `src\lib\a.ts`, wantOK: true,
		},
		{
			name: "priority order matches the executor's",
			args: `{"url": "https://x.dev", "path": "a.txt"}`,
			want: "a.txt", wantOK: true,
		},
		{"command", `{"command": "go build ./..."}`, "go build ./...", true},
	}
	for _, c := range cases {
		got, ok := SubjectFromPartialArgs(c.args)
		if ok != c.wantOK || got != c.want {
			t.Errorf("%s: SubjectFromPartialArgs(%q) = (%q, %v), want (%q, %v)", c.name, c.args, got, ok, c.want, c.wantOK)
		}
	}
}

// The stream announces as early as possible, then keeps counting: an 800-line
// file is a minute of silence otherwise.
func TestStreamAccumulatorReportsProgress(t *testing.T) {
	type update struct {
		name, subject string
		lines         int
	}
	// Updates are paced to once a second in production; unpace them so the
	// sequencing itself is what this test measures.
	defer func(prev time.Duration) { toolProgressInterval = prev }(toolProgressInterval)
	toolProgressInterval = 0

	var seen []update
	acc := newStreamToolAccumulator(func(_, name, subject string, lines int) {
		seen = append(seen, update{name, subject, lines})
	})
	for _, frag := range []string{`{"path": "a.h`, `tml", "content": "<p>`, `hi</p>
<b>`, `x</b>
"}`} {
		acc.add([]streamToolCallDelta{{Index: 0, Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "write", Arguments: frag}}})
	}
	if len(seen) == 0 {
		t.Fatal("never announced the call")
	}
	// The row opens on the tool name alone — "a.h" is still mid-value, and
	// waiting for the whole path is what used to make a large write silent.
	if seen[0].name != "write" || seen[0].subject != "" {
		t.Errorf("first update = %+v, want write with no subject yet", seen[0])
	}
	// Once the path completes it is reported, and never lost again.
	named := -1
	for i, u := range seen {
		if u.subject == "a.html" && named < 0 {
			named = i
		}
		if named >= 0 && u.subject != "a.html" {
			t.Errorf("update %d dropped the subject: %+v", i, u)
		}
	}
	if named < 0 {
		t.Fatalf("the path never reached the UI: %+v", seen)
	}
	// The count only ever climbs — that is what the UI shows. It repeats once,
	// on the update that exists to deliver the name rather than a new line.
	for i := 1; i < len(seen); i++ {
		if seen[i].lines < seen[i-1].lines {
			t.Errorf("update %d went backwards: %+v after %+v", i, seen[i], seen[i-1])
		}
	}
	if got := acc.finalize()[0].Function.Arguments; got != `{"path": "a.html", "content": "<p>hi</p>
<b>x</b>
"}` {
		t.Errorf("arguments not stitched back correctly: %q", got)
	}
}

// Left at its real interval, a burst of fragments produces one row-opening
// update, not one per line — the throttle is what keeps a large file from
// firing a thousand IPC messages.
func TestStreamAccumulatorPacesUpdates(t *testing.T) {
	freezeClock(t) // the rule is one update per interval, not per 500 loop iterations
	var count int
	acc := newStreamToolAccumulator(func(string, string, string, int) { count++ })
	acc.add([]streamToolCallDelta{{Index: 0, Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "write", Arguments: `{"path": "a.html", "content": "`}}})
	for i := 0; i < 500; i++ {
		acc.add([]streamToolCallDelta{{Index: 0, Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Arguments: `line\n`}}})
	}
	if count != 1 {
		t.Errorf("500 fragments within one interval produced %d updates, want 1", count)
	}
}

// The UI recognizes a tool call by its label, so the streaming path and the
// completed path must produce byte-identical subjects. They used to differ:
// only the completed one truncated, so any path over 60 characters drew two
// rows for one call.
func TestStreamingAndCompletedSubjectsAgree(t *testing.T) {
	long := "internal/skill/very/deeply/nested/directory/structure/for/testing/edit.go"
	thai := "เอกสาร/รายงานประจำปีที่มีชื่อยาวมากจริงๆ/สรุปผลการดำเนินงานฉบับสมบูรณ์/ไฟล์.md"
	for _, path := range []string{"a.html", long, thai} {
		streamed, ok := SubjectFromPartialArgs(`{"path": "` + path + `", "content": "x`)
		if !ok {
			t.Fatalf("streaming path did not resolve %q", path)
		}
		completed := SubjectFromArgs(map[string]any{"path": path})
		if streamed != completed {
			t.Errorf("labels drift for %q:\n  streaming: %q\n  completed: %q", path, streamed, completed)
		}
	}
}

// Aetox speaks to several providers, and only the OpenAI-compatible one
// reports streamed progress. Everything else — Anthropic, Ollama, the built-in
// noop, and any provider that does not stream at all — leaves the hook nil.
// Nil must be ordinary, not a crash: this is the path most providers take.
func TestStreamAccumulatorWithoutAHookIsInert(t *testing.T) {
	acc := newStreamToolAccumulator(nil)
	frag := func(s string) []streamToolCallDelta {
		return []streamToolCallDelta{{Index: 0, Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "write", Arguments: s}}}
	}
	acc.add(frag(`{"path": "a.html", "content": "`))
	acc.add(frag(`hi\n"}`))

	calls := acc.finalize()
	if len(calls) != 1 || calls[0].Function.Arguments != `{"path": "a.html", "content": "hi\n"}` {
		t.Fatalf("a nil hook changed what the accumulator produces: %+v", calls)
	}
}

// A provider may send the name only in a later delta than the arguments
// (OpenCode hit this in the wild — anomalyco/opencode#24137). The call must
// still be announced once the name shows up, not dropped.
func TestStreamAccumulatorHandlesNameArrivingLate(t *testing.T) {
	defer func(prev time.Duration) { toolProgressInterval = prev }(toolProgressInterval)
	toolProgressInterval = 0

	var names []string
	acc := newStreamToolAccumulator(func(_, name, _ string, _ int) { names = append(names, name) })
	mk := func(name, args string) []streamToolCallDelta {
		return []streamToolCallDelta{{Index: 0, Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: args}}}
	}
	acc.add(mk("", `{"path": "a.html",`)) // arguments first, no name yet
	if len(names) != 0 {
		t.Fatalf("announced a call with no name: %v", names)
	}
	acc.add(mk("write", ` "content": "x`)) // name arrives late
	if len(names) != 1 || names[0] != "write" {
		t.Errorf("late name was not announced: %v", names)
	}
}

// Argument order is the model's choice, not the schema's. When "content" comes
// before "path", waiting for the subject before saying anything meant a
// 400-line file went by with nothing on screen and the row appeared only once
// the call was already complete — the "it freezes while writing" this whole
// path exists to prevent.
func TestStreamAccumulatorCountsBeforeThePathArrives(t *testing.T) {
	defer func(prev time.Duration) { toolProgressInterval = prev }(toolProgressInterval)
	toolProgressInterval = 0

	type update struct {
		id, subject string
		lines       int
	}
	var seen []update
	acc := newStreamToolAccumulator(func(id, _, subject string, lines int) {
		seen = append(seen, update{id, subject, lines})
	})
	frag := func(id, name, args string) []streamToolCallDelta {
		return []streamToolCallDelta{{Index: 0, ID: id, Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: name, Arguments: args}}}
	}

	// id and name ride the first delta, as OpenAI-style streams send them.
	acc.add(frag("call_1", "write", `{"content": "<!doctype html>\n`))
	acc.add(frag("", "", `<html>\n`))
	acc.add(frag("", "", `<body>\n`))

	if len(seen) != 3 {
		t.Fatalf("3 fragments of content produced %d updates, want 3 — a silent write is the bug: %+v", len(seen), seen)
	}
	for i, u := range seen {
		if u.id != "call_1" {
			t.Errorf("update %d carries id %q, want call_1 — the UI keys the row on it", i, u.id)
		}
		if u.subject != "" {
			t.Errorf("update %d reported subject %q before the path was sent", i, u.subject)
		}
		if want := i + 2; u.lines != want {
			t.Errorf("update %d counted %d lines, want %d", i, u.lines, want)
		}
	}

	acc.add(frag("", "", `</body>\n</html>", "path": "landing.html"}`))
	last := seen[len(seen)-1]
	if last.subject != "landing.html" {
		t.Errorf("the path never reached the row: %+v", last)
	}
	if last.id != "call_1" {
		t.Errorf("the naming update lost the id: %+v", last)
	}
}
