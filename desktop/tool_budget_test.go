package main

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/oauth"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// The tool block is the one cost nobody sees and everybody pays. Every schema
// in it is sent on every request, before the user has typed a word — so it is
// subtracted from the context window of every model Aetox runs on, and paid
// again on every turn by every provider without prompt caching (which is most
// local runtimes, i.e. exactly the cheap-model case this project is built for).
//
// It grows the way this kind of thing always grows: one tool at a time, each
// obviously worth its own 200 tokens, none of them ever weighed against the
// total. This test is the weighing. It is not a limit on what Aetox may do —
// it is a limit on what Aetox may carry *everywhere*, which is a different
// question and the one that was never being asked.
//
// When it fails, the fix is a decision, not a bigger number: does this belong
// in front of every model on every request, or behind `task`, a profile, or a
// skill? Raising the ceiling is a legitimate answer — but it has to be
// deliberate, which is the whole point.
const (
	// Measured 2026-08-04: 44 tools, 35,215 bytes ≈ 8,803 tokens.
	//
	// Raised to 9,600 on 2026-08-07 for `calc`, and the argument is on the
	// record because that is what this test asks for. It is ~130 tokens on
	// every request, and it was weighed against the two cheaper answers and
	// beat both: putting it behind `task` means a sum costs a delegated agent,
	// and leaving it to `shell` means arithmetic costs the right to touch the
	// user's machine and depends on what they happen to have installed. What
	// makes it worth carrying everywhere rather than in a profile is that
	// nothing else in the block fails invisibly — a wrong number arrives in the
	// same confident sentence as a right one, on any kind of task, so there is
	// no profile it could sit in that would be the right one.
	//
	// Its description was cut to capability alone first (occasions belong in
	// internal/prompt), which is the order the fix should always take.
	//
	// Raised to 9,900 on 2026-08-08 for `doc_write`'s `lineitems` block, and
	// the argument is on the record because that is what this test asks for.
	//
	// What it buys is the one thing a language model cannot be trusted with on
	// a priced document: the arithmetic. A `lineitems` block sends the lines and
	// the tax rate and gets the amounts, the subtotal, the VAT and the total
	// worked out in Go — so an invoice's figures are calculated rather than
	// recalled. A total that was worked out in a model's head arrives looking
	// exactly like a correct one and is found weeks later by an accountant.
	//
	// The two cheaper answers were both tried first and neither reaches. It
	// cannot move behind a skill: a skill is prose, and this is the shape of the
	// tool call itself. It cannot move behind `task`: it already is: `doc_write`
	// sits in the `chairs:` list of the specialized desk, so the assistant never
	// carries it and only the document agent does. Its description was cut to
	// capability alone first, in the order above.
	//
	// Worth a separate decision, not taken here: this measures the whole
	// registry, and the three office writers in it (doc_write, sheet_write,
	// slides_write ≈ 1,000 tokens) are chairs-only. No assistant request has
	// ever paid for them, so the number this test defends is larger than the
	// block any real conversation sends. Narrowing the measurement to a desk
	// would make it truer and would also make it easier to pass, which is why
	// it is left alone until somebody decides that on purpose.
	// Twice raised and then stopped, on 2026-08-10, because the number had
	// started measuring the wrong thing. n8n put five tools into the registry
	// and Windmill five more, and none of the ten is sent to anybody who has not
	// connected that engine — connect.Allows withholds a connection's tools
	// until there is an account to use them with. Raising the ceiling a third
	// time would have recorded growth in a number no request has ever paid.
	//
	// So this now measures the block a FRESH INSTALL sends: the registry with
	// the connection gate applied and nothing attached, which is what every user
	// pays on their first message and most users pay forever. Adding an
	// eleventh connection tool no longer moves it, and that is correct rather
	// than lenient — what a connection costs the people who turn it on is
	// guarded separately, per service, by the test below.
	//
	// This is deliberately NOT the "narrow it to a desk" change the note above
	// reserves for the owner. A desk is a preference about which room to
	// measure; the connection gate is a fact about what is sent.
	//
	// One thing that DOES move it is hiring: the automation agent added no tool
	// and cost ~58 tokens, because `task` names the team it can delegate to
	// inside its own description. That is the right design — a model cannot
	// delegate to a name it has never been told — and it is not free. Five more
	// colleagues is another ~300 tokens on every message. When that bites, make
	// `task` name the team more briefly rather than stop hiring.
	// Raised from 10100 to 10400 on 2026-08-17, and this one is a debt marker
	// rather than a budget.
	//
	// The browser gained `wait`, `back` and `dialog` that day (§131) and went
	// from 470 to ~766 tokens. Every previous raise in this file was argued for
	// on the merits of what it bought; this one was not argued at all. Owner,
	// 17 ส.ค., mid-implementation: *"เรื่องโทเค็น ไม่ต้องห่วง เดะค่อยคิด
	// สถาปัตยกรรมใหม่มาจัดการมัน"* — the ceiling stops being the mechanism, so
	// there is no honest number to argue for until the replacement exists.
	//
	// What that means for anyone reading this before then: the block IS over its
	// old ceiling, deliberately, and this constant is no longer evidence that
	// the cost is under control. Do not treat a passing test here as permission
	// to add more. The mechanism that will be evidence has not been designed
	// yet, and `task` at ~1,568 tokens — three times the browser, for one
	// mechanism — is where it should start looking.
	maxToolBlockTokens = 10400
	// The count is full again at 48: the next tool everybody carries has to
	// displace one, and that is the intended reading of this number rather than
	// a nuisance. A connection's tools are not counted here — see above.
	maxToolCount = 48
)

func TestTheToolBlockStaysWithinItsBudget(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	// The gate applied with every connection held by the desk, on a machine
	// where none of them has an account: exactly what a fresh install sends.
	// Held rather than omitted so that the only thing withholding a tool here is
	// the missing account, which is the state being measured.
	held := connect.IDs()
	defs := skill.NewDispatcher(a.registry).ToolDefinitions()
	type row struct {
		name  string
		bytes int
	}
	rows := make([]row, 0, len(defs))
	total := 0
	for _, d := range defs {
		if !connect.Allows(d.Function.Name, held) {
			continue
		}
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("%s: schema does not marshal: %v", d.Function.Name, err)
		}
		rows = append(rows, row{d.Function.Name, len(b)})
		total += len(b)
	}
	// 4 bytes/token is the rough rate for the English JSON these schemas are.
	// Approximate on purpose: the budget is about order of magnitude, and a
	// real tokenizer here would make the test depend on which model is loaded.
	tokens := total / 4

	sort.Slice(rows, func(i, j int) bool { return rows[i].bytes > rows[j].bytes })
	if len(rows) > maxToolCount || tokens > maxToolBlockTokens {
		t.Errorf("tool block is %d tools / ~%d tokens, budget is %d / %d",
			len(rows), tokens, maxToolCount, maxToolBlockTokens)
		t.Log("largest first — this is the list to argue with:")
		for i, r := range rows {
			if i == 12 {
				t.Logf("  … and %d more", len(rows)-i)
				break
			}
			t.Logf("  %5d B  %4d tok  %s", r.bytes, r.bytes/4, r.name)
		}
	}
}

// What a connection costs everybody who turns it on.
//
// The budget above measures a machine with nothing connected, which is the
// honest default — connect.Allows withholds a connection's tools until there is
// an account to use them with, so an unconnected user pays nothing for n8n. But
// that also means the test above cannot see the price of switching it on, and a
// cost that only appears on the user's machine is a cost nobody reviews.
//
// So this measures the same block with n8n attached and states the difference
// out loud. It fails only if a connection's tools grow past a fifth of the whole
// budget: one service should not be able to quietly become the largest thing in
// every request the assistant sends.
func TestConnectingAnEngineDoesNotDoubleTheToolBlock(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)

	// Measured through the connection gate, not off the raw registry. The
	// registry is what this build CAN do and never changes when an account is
	// attached; what a request actually carries is the registry with
	// connect.Allows applied, and that is the number a user pays.
	held := connect.IDs()
	measure := func() (int, int) {
		a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
		t.Cleanup(func() {
			if a.db != nil {
				_ = a.db.Close()
			}
		})
		a.applyConfig(config.Config{
			SandboxRoot:   t.TempDir(),
			ModelProvider: "aetox",
			ModelName:     "aetox-tools:test",
			ApprovalMode:  string(safety.ApprovalFullAccess),
		})
		count, total := 0, 0
		for _, d := range skill.NewDispatcher(a.registry).ToolDefinitions() {
			if !connect.Allows(d.Function.Name, held) {
				continue
			}
			b, _ := json.Marshal(d)
			count++
			total += len(b)
		}
		return count, total / 4
	}

	beforeCount, beforeTokens := measure()

	// Attached the way the app attaches one: an address in the config and a key
	// in the vault. Nothing is sent anywhere — CurrentStatus reads storage.
	if err := config.SetConnectionBaseURL("n8n", "http://localhost:5678"); err != nil {
		t.Fatalf("SetConnectionBaseURL: %v", err)
	}
	if err := oauth.Set("n8n", oauth.Credential{Type: "api", Key: "test-key", Account: "localhost:5678"}); err != nil {
		t.Fatalf("oauth.Set: %v", err)
	}

	afterCount, afterTokens := measure()

	addedCount := afterCount - beforeCount
	addedTokens := afterTokens - beforeTokens
	t.Logf("n8n off: %d tools / ~%d tok   ·   n8n on: %d tools / ~%d tok   ·   +%d tools / +%d tok",
		beforeCount, beforeTokens, afterCount, afterTokens, addedCount, addedTokens)

	if addedCount == 0 {
		t.Fatal("connecting n8n added no tools — the gate is withholding them from a connected account")
	}
	if limit := maxToolBlockTokens / 5; addedTokens > limit {
		t.Errorf("connecting n8n adds ~%d tokens to every request, over the %d this test allows one service — trim the descriptions", addedTokens, limit)
	}
}

// Every tool the assistant can be handed has to say what it is for, or the
// surfaces that group them quietly drop it into a catch-all and the page stops
// being a complete answer.
//
// This lives here rather than in internal/skill because only the desktop can
// see the whole registry at once — builtins, the workbench tools, and the
// delegation trio are registered from three different places, and a table
// checked against only one of them would look complete while missing a third.
func TestEveryToolHasACategory(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := &App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir(), sessionID: newSessionID()}
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})

	var missing []string
	for _, name := range a.registry.Names() {
		if src, ok := a.registry.SourceOf(name); ok && src == skill.SourceSkill {
			continue // a SKILL.md document is not a tool and is listed elsewhere
		}
		if !skill.HasCategory(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("these tools are not in internal/skill/category.go, so they fall into the catch-all: %v", missing)
	}
}
