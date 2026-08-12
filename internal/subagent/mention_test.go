package subagent

import "testing"

// Naming a worker in your own sentence is what removes the paraphrase step, so
// what counts as naming one has to be exact in both directions: it must catch
// the way people actually write, and it must never send a message somewhere
// nobody asked for.
func TestMentionAddressesAWorkerByName(t *testing.T) {
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
		agent, ok := Mention(text)
		if !ok || agent != "doc" {
			t.Errorf("Mention(%q) = %q,%v — want doc", text, agent, ok)
		}
	}

	notAddressed := []string{
		"ปรึกษาเอเจนเอกสารหน่อย", // naming the job is not naming the worker
		"doc ช่วยดูให้หน่อย",     // no @, so it is just a word
		"@docker compose up",      // a longer word that starts the same
		"@document this function", // ditto
		"mail me at mike@doc",     // an address ends a token, it does not start one
		"",                        //
		"@nobody ช่วยหน่อย",       // nobody by that name is on the roster
	}
	for _, text := range notAddressed {
		if agent, ok := Mention(text); ok {
			t.Errorf("Mention(%q) = %q — nobody was addressed", text, agent)
		}
	}
}

// A worker only exists to be addressed if it is on the roster, so a user's own
// file is addressable the day it lands and a broken one never is.
func TestMentionFollowsTheRosterRatherThanAList(t *testing.T) {
	isolate(t)
	writeProfile(t, AgentsDir, "ทีมขาย", "---\ndescription: ดูแลลูกค้า\n---\nคุณดูแลลูกค้า")

	if agent, ok := Mention("@ทีมขาย ช่วยร่างอีเมลให้หน่อย"); !ok || agent != "ทีมขาย" {
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
