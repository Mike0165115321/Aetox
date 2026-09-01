package subagent

import (
	"strings"
	"testing"
)

// A worker's own knowledge has to be named where the worker can see it.
//
// `skills_list` was the index, and it is a call the model has to decide to make
// before it knows whether anything is behind it. Measured 31 ส.ค. on the
// owner's machine: automation opened a skill on 0 of 50 jobs and github on 0 of
// 22, each of them carrying a paragraph in its own AGENT.md telling it to open
// one first. The knowledge was reachable, documented, instructed and unread.
//
// So the index moved into the prompt: names and one-line descriptions, bodies
// still behind `skill_view`. This pins both halves — that it is there, and that
// it is only the index.
func TestAWorkerIsToldWhatItKnows(t *testing.T) {
	isolate(t)

	p, ok := Load("github")
	if !ok {
		t.Fatal("the github agent did not load")
	}
	prompt := PromptFor(p)
	for _, want := range []string{"pr-workflow", "ci-triage", "issue-hygiene", "repo-standards"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the github agent is not told it holds %q", want)
		}
	}

	// The index and not the shelf. A skill's body behind `skill_view` is the
	// whole reason a worker may hold forty documents and pay for none of them;
	// an index that inlined them would be the feature spending what it saves.
	own, _ := OwnSkills("github")
	if len(own) == 0 {
		t.Fatal("the github agent ships no skills, so this test checks nothing")
	}
	// One line per skill and no more. Asked of the index itself rather than of
	// a slice of the prompt: the needs notice is appended after it, and
	// measuring from a marker to the end counted that too.
	index := skillsIndex(p)
	if lines := strings.Count(index, "\n- **"); lines != len(own) {
		t.Errorf("the index has %d entries for %d skills", lines, len(own))
	}
	// Measured 31 ส.ค.: 457 chars for the one-skill agent, 1,043 for this
	// four-skill one, 2,059 for sheet — whose six descriptions are vendored
	// and long. The ceiling is on the shape rather than the content: an
	// index this size is a handful of lines, and anything past it means a
	// body was inlined and the standard's whole saving went with it.
	if len(index) > 3000 {
		t.Errorf("the index is %d chars, so a skill body was inlined rather than named", len(index))
	}
}

// A worker with no skills gets its old prompt back unchanged. The common case
// must not pay for the feature, and prefix caching keys on the leading bytes.
func TestAWorkerWithNoSkillsGetsNoIndex(t *testing.T) {
	isolate(t)

	p, ok := Load("editor")
	if !ok {
		t.Fatal("the editor agent did not load")
	}
	if own, _ := OwnSkills("editor"); len(own) != 0 {
		t.Skip("editor now ships skills, re-point this at a worker that holds none")
	}
	if skillsIndex(p) != "" {
		t.Error("a worker with no skills was charged for an empty index")
	}
	if PromptFor(p) != p.Prompt+needsNotice(UnmetNeeds(p)) {
		t.Error("its prompt is no longer byte for byte what it was")
	}
}
