package main

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// The face an agent wears on the roster (COMPANY.md §4). Two rules, and the
// second is the one that matters: a profile that names no icon is the normal
// case, not a gap — a roster of blank squares on a fresh install would be the
// feature arriving broken for everyone who never opens the editor.
func TestEveryAgentHasAFaceAndTheProfileCanChooseIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile subagent.Profile
		tools   []string
		want    string
	}{
		{"slides writer", subagent.Profile{Name: "deck"}, []string{"glob", "slides_write", "read"}, "layoutList"},
		{"doc writer", subagent.Profile{Name: "doc"}, []string{"doc_write"}, "fileText"},
		{"sheet writer", subagent.Profile{Name: "sheet"}, []string{"sheet_write"}, "chartColumn"},
		// Nothing it writes says what it is — the honest answer is the generic
		// mark, and this is exactly the profile whose author will want to pick.
		{"no writer at all", subagent.Profile{Name: "researcher"}, []string{"web_search", "read"}, "bot"},
		// And the file's own choice outranks everything derived.
		{"profile chose one", subagent.Profile{Name: "deck", Icon: "palette"}, []string{"slides_write"}, "palette"},
	} {
		if got := chairIcon(tc.profile, tc.tools); got != tc.want {
			t.Errorf("%s: chairIcon = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The feed shows the line the caller wrote, not the arguments the tool call
// carried — five rows of `{"agent":"doc","description":…}` is the machine's
// copy shown to the person it was never for.
func TestTheFeedShowsTheBriefAndNotTheToolCall(t *testing.T) {
	for _, tc := range []struct{ name, request, want string }{
		{
			"ordinary call",
			`{"agent":"doc","description":"ทำสรุป 5 ข้อ เรื่องนิสัยการทำงานที่ดี","prompt":"สร้างไฟล์ Microsoft Word…"}`,
			"ทำสรุป 5 ข้อ เรื่องนิสัยการทำงานที่ดี",
		},
		{
			// No description written: the brief itself is the next best line,
			// and far better than the object it sits in.
			"description missing",
			`{"agent":"deck","prompt":"สร้างไฟล์ PowerPoint 3 สไลด์"}`,
			"สร้างไฟล์ PowerPoint 3 สไลด์",
		},
		{
			// Stored requests are clamped, so the longest ones arrive cut in
			// half — the rows a feed is most for, and the ones a strict decode
			// would give up on.
			"truncated mid-object",
			`{"agent":"doc","description":"สรุปผลการทดสอบระบบ","prompt":"ทำเอกสาร Word ชื่อ \"สรุ`,
			"สรุปผลการทดสอบระบบ",
		},
		{
			// Every job written before `task` carried arguments.
			"not JSON at all",
			"ทำสไลด์ให้หน่อย",
			"ทำสไลด์ให้หน่อย",
		},
	} {
		if got := briefOf(tc.request); got != tc.want {
			t.Errorf("%s: briefOf = %q, want %q", tc.name, got, tc.want)
		}
	}
	// Never empty: a row with no line says nothing happened at all.
	if got := briefOf(`{"agent":"doc"}`); got == "" {
		t.Error("a call with no brief in it produced an empty line")
	}
}
