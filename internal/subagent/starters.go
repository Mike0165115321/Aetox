package subagent

// How a worker opens a conversation, kept in the worker's own folder.
//
// An empty chat shows a question and a few cards that fill the composer when
// clicked. Those used to be a list inside the desktop app, one set per room,
// which meant the one kind of worker the user can actually add — an agent is a
// folder (config.AgentHome) — was the one kind that could never have its own.
// Hiring a bookkeeper gave you four generic cards forever, because giving it
// four real ones meant editing the product (owner's call, 2026-08-10).
//
// So it is a file in the package, beside AGENT.md and skills/: STARTERS.md,
// with a translation beside it as STARTERS.<lang>.md.
//
//	# จะให้ทำเอกสารอะไรดี?
//
//	- ร่างรายงานหรือจดหมาย | ช่วยร่างเอกสารเรื่องนี้ให้หน่อย: | pencil
//	- ตรวจว่าเอกสารนี้ขาดอะไร | ช่วยตรวจเอกสารนี้ว่าขาดอะไรบ้าง: | search
//
// The format is markdown that happens to parse rather than a format with a
// spec: the heading is the question, each list item is a card, and the fields
// are split on `|` — title, the sentence that lands in the composer, and
// optionally the mark it wears. Someone reading the file in any editor sees a
// heading and a list, which is the point; a worker's opening line should not
// need a parser to be legible.
//
// What this package does NOT do is decide anything about the window. It reads
// a file and hands back what was in it. Where the cards are drawn, how many the
// grid fits, and what an empty answer falls back to are the window's business,
// and keeping that line is what lets a user's worker ship cards without the
// window having heard of it.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// Starter is one card: what it says, what it puts in the composer, and the
// mark it wears. Icon is a name from the app's own icon set, same as the
// profile's `icon:` — a name rather than a picture, because the file is a .md
// somebody edits by hand.
type Starter struct {
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Icon   string `json:"icon,omitempty"`
}

// StarterSet is a whole opening: the question, and the cards under it. Both are
// optional — a file with only a heading is a worker that asks its own question
// and leaves the cards to the window, which is a legitimate thing to want.
type StarterSet struct {
	Headline string    `json:"headline,omitempty"`
	Cards    []Starter `json:"cards"`
}

// maxStarters is the ceiling this package will return. Not a layout rule —
// the window owns its grid — but a bound on a file the app did not write, so a
// worker cannot hand back three hundred cards and make the window deal with it.
// Extra lines are dropped from the end, which is the order the author wrote
// them in and therefore the order they would have chosen to lose.
//
// It is a POOL ceiling, not a hand. It was 4 while the window drew every card
// a file held, which made those two numbers the same number and meant a worker
// could only gain a fifth card by giving up one of its four. The window now
// deals four out of whatever it is handed (starters.ts), so the file is free to
// carry more than fits on screen — which is the point of a pool: a worker gets
// deeper without the empty chat getting busier. 24 is six hands, far past what
// anyone will hand-write, and still small enough that a runaway file cannot
// turn into a payload the window has to think about.
const maxStarters = 24

// Starters reads one worker's opening in the requested language.
//
// Two homes and two languages, resolved in that order: **the home decides
// first**. A user who wrote their own STARTERS.md for a worker they took over
// gets their file in every language — falling back to the bundled English of a
// definition they replaced would be answering as an agent that no longer
// exists. Within the winning home, the requested language wins and the base
// file fills in for a language the author never wrote.
//
// Nothing here is an error. A worker with no file, an unreadable file, a file
// of prose with no list in it — all of them mean "no opening of my own", which
// the window already knows how to handle, and none of them is worth refusing to
// open a chat over.
func Starters(name, locale string) StarterSet {
	// "" is the base file. Asked for second, and only when the requested
	// language is not already it.
	languages := []string{locale}
	if config.AgentStartersName(locale) != config.AgentStartersFile {
		languages = append(languages, "")
	}
	for _, read := range []func(string) ([]byte, bool){userStarters(name), bundledStarters(name)} {
		for _, lang := range languages {
			if raw, ok := read(lang); ok {
				return parseStarters(string(raw))
			}
		}
	}
	return StarterSet{Cards: []Starter{}}
}

// SaveStarters writes one worker's opening for one language, in the same
// markdown Starters reads.
//
// The settings page's door. Editing the file by hand stays exactly as valid —
// that is what the format is for — and this is the same act through a form, so
// what it writes has to be a file a person would have been happy to write:
// a heading, a blank line, and a list. Nothing here is a marker the parser
// needs and a human does not.
//
// An empty set removes the user's file rather than writing an empty one. That
// is the honest inverse: with no file the worker falls back to the shipped
// opening, or to the four cards the window draws for any colleague — which is
// what "I do not want my own opening" means, and is not the same as "my opening
// is blank".
//
// Only the user's home is ever written. A bundled worker's opening is edited by
// writing your own beside it, the same shape as AGENT.md, so pressing Save on a
// shipped agent gives you a copy to change rather than a fight with the binary.
func SaveStarters(name, locale string, set StarterSet) error {
	path, err := config.AgentStartersPath(name, locale)
	if err != nil {
		return err
	}
	body, err := serializeStarters(set)
	if err != nil {
		return err
	}
	if body == "" {
		// Already absent is the state being asked for, so a missing file is
		// success rather than something to report.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// serializeStarters is parseStarters' inverse, and refuses what it cannot
// round-trip.
//
// `|` is the field separator, so one inside a title or a prompt would come back
// as a different card than the one that was saved — the icon field filled with
// half a sentence. Refused by name rather than escaped: an escape would put a
// backslash into a file whose whole point is that it reads as ordinary
// markdown, and refusing tells the user something they can act on.
func serializeStarters(set StarterSet) (string, error) {
	headline := strings.TrimSpace(set.Headline)
	var lines []string
	for _, c := range set.Cards {
		title := strings.TrimSpace(c.Title)
		// The space inviteCompletion adds after a trailing colon is put back on
		// read and must not be written: it is trailing whitespace, which every
		// editor strips and this parser trims anyway.
		prompt := strings.TrimSpace(c.Prompt)
		icon := strings.TrimSpace(c.Icon)
		if title == "" || prompt == "" {
			continue // a half-filled row is a row the user has not finished, not an error
		}
		if strings.ContainsAny(title+prompt+icon, "|\n") {
			return "", fmt.Errorf("การ์ด %q มีเครื่องหมาย | หรือขึ้นบรรทัดใหม่ ซึ่งใช้ในไฟล์นี้ไม่ได้", title)
		}
		if len(lines) >= maxStarters {
			break
		}
		if icon != "" {
			lines = append(lines, "- "+title+" | "+prompt+" | "+icon)
		} else {
			lines = append(lines, "- "+title+" | "+prompt)
		}
	}
	if headline == "" && len(lines) == 0 {
		return "", nil
	}
	var b strings.Builder
	if headline != "" {
		b.WriteString("# " + headline + "\n")
		if len(lines) > 0 {
			b.WriteString("\n")
		}
	}
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	return b.String(), nil
}

// userStarters reads from <DataRoot>/agents/<name>/. An invalid name simply
// never finds anything — AgentStartersPath is the guard, and it is the same one
// every other path into a home goes through.
func userStarters(name string) func(string) ([]byte, bool) {
	return func(locale string) ([]byte, bool) {
		path, err := config.AgentStartersPath(name, locale)
		if err != nil {
			return nil, false
		}
		raw, err := os.ReadFile(path)
		return raw, err == nil
	}
}

// bundledStarters reads from the shipped package of the same name. Embedded
// paths are always slash-separated, whatever the host OS does.
func bundledStarters(name string) func(string) ([]byte, bool) {
	return func(locale string) ([]byte, bool) {
		if !validName(name) {
			return nil, false
		}
		raw, err := bundledProfiles.ReadFile(bundledAgentDir + "/" + name + "/" + config.AgentStartersName(locale))
		return raw, err == nil
	}
}

// parseStarters reads the file. Every line is one of four things — a heading, a
// card, a comment, or nothing — and anything it cannot make a card out of is
// skipped rather than reported: this is a file a person hand-edits, and a typo
// on line three must cost line three and not the whole opening.
func parseStarters(raw string) StarterSet {
	set := StarterSet{Cards: []Starter{}}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "<!--"):
			continue
		case strings.HasPrefix(line, "#"):
			// The first heading is the question. Later ones are the author
			// organising their own file and are not a second question.
			if set.Headline == "" {
				set.Headline = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
			continue
		}
		if len(set.Cards) >= maxStarters {
			continue
		}
		if card, ok := parseStarter(line); ok {
			set.Cards = append(set.Cards, card)
		}
	}
	return set
}

// parseStarter reads one line into a card, or says it is not one.
//
// A line with no `|` is prose — a note the author left themselves — and not a
// card with a missing half. Guessing that the whole line was both the title and
// the prompt would put a paragraph on a button.
func parseStarter(line string) (Starter, bool) {
	for _, bullet := range []string{"- ", "* ", "+ "} {
		if after, found := strings.CutPrefix(line, bullet); found {
			line = strings.TrimSpace(after)
			break
		}
	}
	parts := strings.SplitN(line, "|", 3)
	if len(parts) < 2 {
		return Starter{}, false
	}
	card := Starter{
		Title:  strings.TrimSpace(parts[0]),
		Prompt: strings.TrimSpace(parts[1]),
	}
	if len(parts) == 3 {
		card.Icon = strings.TrimSpace(parts[2])
	}
	if card.Title == "" || card.Prompt == "" {
		return Starter{}, false
	}
	card.Prompt = inviteCompletion(card.Prompt)
	return card, true
}

// inviteCompletion puts back the space after a trailing colon.
//
// A prompt ending in ":" is the deliberate half-sentence — the card fills the
// composer and the user types the rest — and the space between the colon and
// the cursor is part of it. It cannot be written in the file: it is trailing
// whitespace, which editors strip, git flags, and this parser trims. So the
// colon is the author's way of saying "this one is unfinished", and the space
// is put back here rather than being something they have to defend.
func inviteCompletion(prompt string) string {
	if strings.HasSuffix(prompt, ":") || strings.HasSuffix(prompt, "：") {
		return prompt + " "
	}
	return prompt
}
