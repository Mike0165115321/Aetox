package main

import (
	"testing"

	"github.com/Mikedev115/Aetox/internal/subagent"
)

// The face an agent wears on the roster (COMPANY.md §4). Two rules, and the
// second is the one that matters: a profile that names no icon is the normal
// case, not a gap — a roster of blank squares on a fresh install would be the
// feature arriving broken for everyone who never opens the editor.
func TestEveryAgentHasAFaceAndTheProfileCanChooseIt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile subagent.Profile
		want    string
	}{
		// The mark is the file's own choice or the generic one. It was derived
		// from the writer in the agent's tool list until 31 ส.ค., when every
		// agent started holding the same kit — a derivation that answers
		// "document" for all seven says nothing about any of them.
		{"sheet chair, no icon named", subagent.Profile{Name: "sheet"}, "bot"},
		{"nothing it writes says what it is", subagent.Profile{Name: "researcher"}, "bot"},
		// And the file's own choice is the whole of it now.
		{"profile chose one", subagent.Profile{Name: "doc", Icon: "palette"}, "palette"},
	} {
		if got := chairIcon(tc.profile); got != tc.want {
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
