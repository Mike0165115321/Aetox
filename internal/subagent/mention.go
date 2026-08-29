package subagent

import "strings"

// Addressing a worker by name from the composer.
//
// Why this exists: a user who writes "ปรึกษาเอเจนเอกสารหน่อยว่า…" in the main
// chat is asking one specific worker something, and the only way that request
// used to reach it was as a summary the assistant wrote of it. That summary is
// where the damage happens — "ask doc how a good document is put together" left
// as "write a manual about…" and came back as a manual, and a mistyped word
// became a confident guess at a subject nobody had raised. The worker cannot
// catch either, because the summary is the only thing it ever sees.
//
// Naming the worker removes the step that could mistranslate. There is nothing
// to get right because there is nothing in between.
//
// It is deliberately NOT the chair door (§85, a whole session with one worker).
// That door moves you into their room; this one is a sentence in the room you
// are already in, and the difference matters when the next thing you say is for
// the assistant again.
//
// # Two keys, and one of them is a click
//
// The first build read the address out of the message text alone: any `@name`
// anywhere in it, matched against the roster. On 30 ส.ค. that sent an 8,486-
// character brief — 4 ภาพ carousel, images attached — to `reviewer`, a worker
// with four read-only tools that could not have made a picture if it tried. It
// ran 5 rounds of `search` and was killed at 78.7 seconds, and the user's own
// account of it was that Aetox "เงียบหาย". Nothing was wrong with the model.
// The brief was the user's release-notes draft, and 4,000 characters into it,
// inside backticks, was the sentence `เรียกใช้ได้ด้วย @reviewer`.
//
// A name written *about* a worker and a name written *to* one look identical in
// text, and no amount of care around backticks, quotes or position tells them
// apart — the next paste finds whatever the rule did not think of. So the text
// stopped being the evidence. The owner's rule (30 ส.ค.): *"ต้องกดเรียกจริงๆ
// ไม่ใช่ว่าคีย์เวิร์ดดันไปตรง มันต้อง @ แล้วขึ้นว่าเลือก ไม่ใช่เผลอไปคลิกใส่"*.
//
// So Mention needs both keys turned:
//
//   - **picked** — the name the user chose off the composer's roster menu. This
//     is the act. Nothing that was merely typed or pasted can produce it, which
//     is what makes a paste harmless no matter what it says.
//   - **text** — which must still carry the `@name` token. This is the recall:
//     picking somebody and then deleting the token is changing your mind, and
//     the message goes to the assistant like any other.
//
// # ซับเอเจนเรียกไม่ได้
//
// Only agents can be addressed. Sub-agents are the assistant's own hands and
// exist to help an agent (owner, 30 ส.ค.: *"ซับเอเจนเรียกไม่ได้ เรียกได้แต่
// เอเจน ซับเอเจนมีหน้าที่ช่วยเอเจนครับ"*) — they have no desk, no room to walk
// into, and half of them are shaped for one narrow errand rather than for a job
// handed over a counter. The kind is read off the loaded profile (Desk), which
// is the same answer profile.go gives everywhere else, so this file adds no
// second opinion about who is what.
//
// This also puts the door and the menu in agreement by construction: the
// composer's roster is ListChairs, the agents at the specialized desk, and a
// name that is not one of those is refused here even if a caller sends it.

// Mention reports the worker a message is addressed to, if any.
//
// picked is what the user chose from the composer's roster; text is the message
// as typed. Both have to agree, and picked has to name a healthy agent — see
// the package comment above for why one of them alone is not enough.
func Mention(text, picked string) (agent string, ok bool) {
	picked = strings.TrimSpace(picked)
	if picked == "" {
		return "", false // nobody was chosen, so nobody was addressed
	}
	// Load refuses a name that does not exist and one whose file cannot run, so
	// a stale menu or a hand-written call cannot reach a worker that is not
	// there. Validated here rather than trusted from the window, because this is
	// the boundary the window sits outside of.
	p, found := Load(picked)
	if !found {
		return "", false
	}
	// A desk is what makes somebody a colleague you can talk to. No desk means a
	// helper, and a helper is only ever handed work by an agent.
	if p.Desk == "" {
		return "", false
	}
	if !mentions(text, p.Name) {
		return "", false // the token was taken back out; the choice went with it
	}
	return p.Name, true
}

// mentions reports whether text addresses name with an `@name` token.
//
// Case-insensitive, because the name is a filename and nobody typing a sentence
// is thinking about that. The character after the token has to be a boundary
// too: `@doc` must not match inside `@docker`, which is the kind of near-miss
// that would send a message somewhere the user never named.
func mentions(text, name string) bool {
	lower, at := strings.ToLower(text), "@"+strings.ToLower(name)
	for i := 0; ; {
		found := strings.Index(lower[i:], at)
		if found < 0 {
			return false
		}
		start := i + found
		end := start + len(at)
		beforeOK := start == 0 || isBoundary(rune(lower[start-1]))
		afterOK := end == len(lower) || isBoundary(rune(lower[end]))
		if beforeOK && afterOK {
			return true
		}
		i = end
	}
}

// isBoundary is what may sit against a mention without swallowing it. Letters
// and digits may not — those are a longer word. Everything else can: a space, a
// newline, a comma, a Thai character running straight into it.
func isBoundary(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	case r == '_' || r == '-':
		return false
	default:
		return true
	}
}
