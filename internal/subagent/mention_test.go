package subagent

import "testing"

// Addressing a worker takes two keys: the name picked off the composer's
// roster, and the `@name` token still standing in the message. What each one is
// for is easiest to see in what it refuses on its own.
func TestMentionNeedsBothTheChoiceAndTheToken(t *testing.T) {
	isolate(t)

	addressed := []string{
		"@doc ปรึกษาหน่อยว่าเอกสารที่ดีควรเป็นยังไง", // at the front
		"ช่วยถาม @doc หน่อย",            // mid-sentence
		"ถาม @doc ว่าไง",                // Thai running against it
		"@DOC what makes a good report", // a filename is not case
		"เขียนสรุปให้หน่อย\n@doc",       // after a newline
		"ping @doc, please",             // punctuation right after
	}
	for _, text := range addressed {
		agent, ok := Mention(text, "doc")
		if !ok || agent != "doc" {
			t.Errorf("Mention(%q, doc) = %q,%v — want doc", text, agent, ok)
		}
	}

	notAddressed := []struct{ text, picked string }{
		// Nothing was picked. This is every message anybody types or pastes, and
		// it is the whole of the fix: the 8,486-character brief that went to
		// `reviewer` on 30 ส.ค. is the first line here.
		{"เรียกใช้ได้ด้วย `@reviewer`\n\nแนวคิดง่าย ๆ คือ", ""},
		{"@doc ปรึกษาหน่อย", ""},
		{"เขียนเอกสารที่มีคำว่า @doc อยู่ในนั้น", ""},
		// Picked, then taken back out. Deleting the token is changing your mind.
		{"เปลี่ยนใจแล้ว ถามเฉย ๆ", "doc"},
		// The old near-misses, which still have to hold with a choice behind them:
		// a longer word is a different word.
		{"@docker compose up", "doc"},
		{"@document this function", "doc"},
		{"mail me at mike@doc", "doc"},
		// Nobody by that name is on the roster, whatever the window sent.
		{"@nobody ช่วยหน่อย", "nobody"},
		{"", ""},
	}
	for _, c := range notAddressed {
		if agent, ok := Mention(c.text, c.picked); ok {
			t.Errorf("Mention(%q, %q) = %q — nobody was addressed", c.text, c.picked, agent)
		}
	}
}

// ซับเอเจนเรียกไม่ได้ (owner, 30 ส.ค.). A helper is the assistant's own hands and
// takes work from an agent, never over the counter — so no amount of picking and
// typing reaches one. Checked against the bundled set rather than one name,
// because the rule is about the kind and a new helper must inherit it.
func TestMentionRefusesEverySubAgent(t *testing.T) {
	isolate(t)

	helpers := Delegates()
	if len(helpers) == 0 {
		t.Fatal("no sub-agents bundled; this test would pass by having nothing to refuse")
	}
	for _, p := range helpers {
		if agent, ok := Mention("@"+p.Name+" ช่วยดูให้หน่อย", p.Name); ok {
			t.Errorf("Mention addressed the sub-agent %q as %q — helpers are not somebody you talk to", p.Name, agent)
		}
	}
}

// A worker only exists to be addressed if it is on the roster, so a user's own
// file is addressable the day it lands and a broken one never is.
func TestMentionFollowsTheRosterRatherThanAList(t *testing.T) {
	isolate(t)
	writeProfile(t, AgentsDir, "ทีมขาย", "---\ndescription: ดูแลลูกค้า\n---\nคุณดูแลลูกค้า")

	if agent, ok := Mention("@ทีมขาย ช่วยร่างอีเมลให้หน่อย", "ทีมขาย"); !ok || agent != "ทีมขาย" {
		t.Errorf("Mention of a user's own agent = %q,%v — want ทีมขาย", agent, ok)
	}
}

// The receipt is written for a model that delegates too eagerly. A user who
// named the worker did not delegate anything, so it comes off their answer —
// and only it: a worker whose own last line is bracketed keeps it.
func TestTheReceiptComesOffAUserAddressedAnswer(t *testing.T) {
	cases := []struct{ in, want string }{
		{"เอกสารเรียบร้อยครับ\n[task doc: 4 tool calls, 12.0s]", "เอกสารเรียบร้อยครับ"},
		{"answer\n[task doc: 1 tool calls, 2.0s] NOTE: that was one tool call", "answer"},
		{"answer with no receipt at all", "answer with no receipt at all"},
		{"a line\n[not a receipt]", "a line\n[not a receipt]"},
		{"[task doc: 1 tool calls, 2.0s]", "[task doc: 1 tool calls, 2.0s]"}, // nothing above it to keep
	}
	for _, c := range cases {
		if got := withoutReceipt(c.in); got != c.want {
			t.Errorf("withoutReceipt(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
