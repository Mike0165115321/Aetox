package subagent

import "strings"

// Addressing a worker by name, in the middle of an ordinary sentence.
//
// Why this exists: a user who writes "ปรึกษาเอเจนเอกสารหน่อยว่า…" in the main
// chat is asking one specific worker something, and today the only way that
// request reaches it is as a summary the assistant wrote of it. That summary is
// where the damage happens — "ask doc how a good document is put together" left
// as "write a manual about…" and came back as a manual, and a mistyped word
// became a confident guess at a subject nobody had raised. The worker cannot
// catch either, because the summary is the only thing it ever sees.
//
// A rule in the tool's own description ("carry the user's words through") helps
// and is in place, but it asks the model for discipline. This asks for nothing:
// naming the worker removes the step that could mistranslate, the way OpenCode's
// @mention does. There is nothing to get right because there is nothing in
// between.
//
// It is deliberately NOT the chair door (§85, a whole session with one worker).
// That door moves you into their room; this one is a sentence in the room you
// are already in, and the difference matters when the next thing you say is for
// the assistant again.

// Mention finds the worker a message addresses, if it addresses one.
//
// The token must be `@` immediately followed by a known worker's name, and must
// start the message or follow a space — so an email address, a Go doc link or a
// price never reads as an address to somebody. The name is matched against the
// roster rather than a list kept here, because a user's own agent is a file they
// dropped in and it must be addressable the day it appears.
//
// Everything else about the message is left exactly as typed, mention included.
// Stripping the token would be an edit, and the whole argument for this door is
// that nothing edits what the user wrote — a worker reading its own name at the
// front of a sentence understands it the way a person would.
func Mention(text string) (agent string, ok bool) {
	for _, p := range List() {
		if p.Invalid != "" {
			continue // a sick file is not somebody you can talk to
		}
		if mentions(text, p.Name) {
			return p.Name, true
		}
	}
	return "", false
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
