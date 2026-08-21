package skill

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/command"
)

// The three extension skills — `aetox-skills`, `aetox-mcp`, `aetox-prompts` —
// are what the "ให้ผู้ช่วยหาให้" buttons on the settings pages send the model to
// read. Each one exists to answer a question the settings page cannot: not
// "where is the field" but "should this be added at all, and what happens if it
// is".
//
// They fail the way any document does: silently, when the code moves under
// them. What is pinned below is only the handful of sentences that are copies of
// a decision living in code — a gate the model will walk into, or a rule its
// input will be validated against. Nothing about tone, structure or advice is
// pinned; that is the author's, and freezing it would be guarding a sentence
// rather than a behaviour (see bundled_slides_test.go for the same line drawn).

func TestTheExtensionSkillsAreBundledAndParse(t *testing.T) {
	for _, name := range []string{"aetox-skills", "aetox-mcp", "aetox-prompts"} {
		s := bundledDoc(t, name)
		if strings.TrimSpace(s.Description) == "" {
			t.Errorf("%s: no description — skills_list prints it, and a skill with no line there is one nobody opens", name)
		}
		if !s.Bundled {
			t.Errorf("%s: Bundled is false; the delete and reveal surfaces key on it", name)
		}
		if len(strings.TrimSpace(s.body)) < 500 {
			t.Errorf("%s: body is %d bytes; that is not a document", name, len(s.body))
		}
	}
}

// The skill-hunting document sends the model down two roads that are gated, and
// both gates are in this package.
//
// `plugin_install` is the only way in that the model itself can drive; the shelf
// is closed to every file tool. A document that forgot the second fact would
// have the assistant try to `write` a SKILL.md, be refused, and relay the
// refusal to the user as Aetox being broken — which is the exact failure
// recorded against skillShelf on 2026-08-20.
func TestTheSkillsSkillNamesBothTheDoorAndTheWall(t *testing.T) {
	body := bundledDoc(t, "aetox-skills").body

	for _, fact := range []string{
		"plugin_install", // the door the model has
		"skills_list",    // and how it confirms the install landed
		skillShelf,       // the wall: ~/.aetox is refused to every file tool
	} {
		if !strings.Contains(body, fact) {
			t.Errorf("the aetox-skills document never names %q", fact)
		}
	}

	// The injection warning is not decoration: a SKILL.md fetched off the
	// internet is a stranger's text that will be handed to the model as
	// instructions in every future session. If this paragraph is ever edited
	// away, the feature ships a remote-instruction channel with a button on it.
	if !strings.Contains(body, "data, not instructions") {
		t.Error("the aetox-skills document lost the paragraph saying fetched skill text is data — " +
			"that button installs a stranger's instructions on the user's machine")
	}
}

// The MCP document's whole shape depends on one fact: the model cannot add a
// server, because the config file is refused to every file tool. It is refused
// by NAME (ownSecretFiles), so this is checked against the list rather than
// against a copy of it.
func TestTheMCPSkillNamesTheConfigItCannotWrite(t *testing.T) {
	body := bundledDoc(t, "aetox-mcp").body

	const configFile = "mcp-servers.json"
	var refused bool
	for _, name := range ownSecretFiles {
		if name == configFile {
			refused = true
		}
	}
	if !refused {
		t.Fatalf("%s is no longer in ownSecretFiles — the aetox-mcp document tells the model it cannot "+
			"write the file, and that is now false", configFile)
	}
	if !strings.Contains(body, configFile) {
		t.Errorf("the aetox-mcp document never names %q, so it cannot explain why adding a server is the user's job", configFile)
	}

	// Static headers only (internal/mcp/client.go). Recommending an OAuth server
	// produces a settings form asking for a token the user has no way to obtain
	// — the failure the curated shelf's own rule was written against.
	for _, fact := range []string{"OAuth", "${env:"} {
		if !strings.Contains(body, fact) {
			t.Errorf("the aetox-mcp document never mentions %q", fact)
		}
	}
}

// The preset document tells the model what a name may be, and that rule is
// enforced by ValidPresetName. A document that promised more than the validator
// allows produces a refusal the user watches happen for no stated reason.
func TestThePromptsSkillMatchesThePresetValidator(t *testing.T) {
	body := bundledDoc(t, "aetox-prompts").body

	if !strings.Contains(body, "$ARGUMENTS") {
		t.Error("the aetox-prompts document never names $ARGUMENTS — the one piece of syntax a preset has")
	}
	// The length limit, as the document writes it. Checked by asking the
	// validator rather than by reading the constant, so a change to either side
	// lands here.
	if err := command.ValidPresetName(strings.Repeat("a", 40)); err != nil {
		t.Errorf("a 40-character name is refused now (%v) — the aetox-prompts document says it is allowed", err)
	}
	if err := command.ValidPresetName(strings.Repeat("a", 41)); err == nil {
		t.Error("a 41-character name is accepted now — the aetox-prompts document says 40 is the limit")
	}
	if !strings.Contains(body, "40") {
		t.Error("the aetox-prompts document never states the 40-character limit")
	}
	// A space is the one a user actually tries, because a preset name reads like
	// a title until you try to type it after a slash.
	if err := command.ValidPresetName("two words"); err == nil {
		t.Error("a name with a space is accepted now — the aetox-prompts document says it is not")
	}
	if !strings.Contains(body, "no spaces") {
		t.Error("the aetox-prompts document never says a name cannot contain a space")
	}
}
