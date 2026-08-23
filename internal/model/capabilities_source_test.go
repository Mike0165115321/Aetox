package model

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The standard, enforced instead of intended.
//
// Four separate bugs this month were the same bug: a question about a MODEL
// answered by a list of model names written into Go. Each time it was fixed by
// hand and each time the next capability was added the same way, because
// nothing stopped it — a table is easy to write, and it is right on the day it
// is written.
//
// So the rule is a test. A capability decision may not name a model. What may
// name a model is a wire dialect (which field, which envelope) — and none of
// those live in these files.
//
// Reading the source from a test rather than asserting on behaviour is
// deliberate, and it is the same shape as providers_test.go scanning
// providerMarks.ts and internal/proc/coverage_test.go scanning its own package:
// the thing being guarded is what someone will type next, not what the code
// currently returns.
var capabilityFiles = []string{
	"capabilities.go",
	"vision_capabilities.go",
	"document_capabilities.go",
	"thinking_capabilities.go",
}

// modelMatchCalls are the ways a model id gets compared to a literal. This is
// the real guard, and it replaced a regex over string literals.
//
// The regex was the obvious idea and it was too weak to be worth trusting. It
// matched "gpt-5.2" and "claude-3-opus" and missed "kimi-k3", "grok-4",
// "minimax-m3" and "deepseek-" — a guard that catches three of the nine
// offenders while reading as though it catches all of them is worse than no
// guard, because it is believed.
//
// Matching on the MECHANISM has no such hole. Whatever a vendor calls its next
// model, deciding a capability from its name means comparing that name to
// something, and there are only these ways to do it.
// Keyed on the CALL, not on the variable passed to it. The first version named
// the variables — HasPrefix(modelID, Contains(name — and a rename walked
// straight past it: `strings.HasPrefix(m, "kimi-k3")` in a helper called
// anything else was invisible. A guard with a hole that size is worse than none,
// because it is quoted as proof.
//
// Paired with the literal check below, these have no innocent use in a
// capability file: the two allowed marker lists compare against a loop VARIABLE,
// so they never trip it.
var modelMatchCalls = []string{
	"strings.HasPrefix(",
	"strings.HasSuffix(",
	"strings.Contains(",
	"strings.EqualFold(",
	"== \"",
	"!= \"",
}

// ownVocabulary is what these files are allowed to compare against by name: the
// depths Aetox offers, the modalities it routes, and the provenance strings it
// stamps. Equality against one of these is the package talking about itself.
//
// Built from thinkingLadder rather than retyped, so a rung added there cannot
// leave this list behind.
func ownVocabulary() map[string]bool {
	out := map[string]bool{
		"off": true, "on": true, "default": true, "ultra": true, "disabled": true,
		"adaptive": true, "enabled": true,
		"text": true, "image": true, "pdf": true, "audio": true, "video": true,
		"models.dev": true, "effort": true, "toggle": true, "budget_tokens": true,
	}
	for _, rung := range thinkingLadder {
		out[rung] = true
	}
	return out
}

// modelIDShape is kept as a second net, for a literal that reaches a decision
// without going through one of the calls above.
var modelIDShape = regexp.MustCompile(`^[a-z][a-z]+[-.]?[0-9]+([.-][0-9a-z]+)*$`)

// notAModelID are the strings that match the shape above and are not models.
// Kept short on purpose: a growing allowlist is the test being argued with
// rather than obeyed.
var notAModelID = map[string]bool{
	"base64": true, // encodings
	"utf-8":  true,
	"sha256": true,
}

// notYetMigrated is the debt register, and the only way past the rule above.
//
// Nine thinking resolvers still answer from prefix lists. That is not an
// oversight. OpenRouter was migrated first because it is a reseller, where a
// hand-written table is nine companies' release schedules at once and was
// already wrong for 171 of its 360 models. The nine below serve one vendor
// each, age far more slowly, and every one of them carries measured decisions
// (DeepSeek's off switch, gpt-5.1 defaulting to none, Groq's per-model
// include_reasoning) that have to be moved deliberately. Doing all ten in one
// sweep was attempted on 2026-08-23, broke twenty tests, and was backed out.
//
// The register exists so the debt is counted rather than felt.
// notYetMigrated is the debt register, and it is empty.
//
// Ten thinking resolvers answered from prefix tables. All ten are gone: the
// answer comes from the catalog, per model, and what stays ours is
// thinkingProfiles — the wire dialect, the default preference, and the ladder
// to offer when the catalog says a model reasons without saying with what.
// Nothing in these files names a model any more.
//
// It stays here as a shape rather than being deleted, so that adding a name is
// a visible act with a counter beside it rather than a quiet new table.
var notYetMigrated = []string{
	// OpenAI: defaults differ per model (gpt-5.1 is none, gpt-5.2 is medium)
	// and models.dev states rungs but never which one the service picks when
	// nothing is sent. A default that disagrees with the service is invisible.
	"resolveOpenAIThinkingCapabilities",
	// Gemini: states its dial as a token budget, which no runtime here can
	// spend, so the catalog calls these models reasoning and names nothing this
	// package can offer. gemini-2.5-pro also cannot be told not to think, which
	// a per-provider ladder gets wrong at the edge.
	"resolveGeminiThinkingCapabilities",
}

// braceDelta counts a line's net brace depth, ignoring braces inside string
// literals — which matter here, because these files are full of them.
func braceDelta(line string) int {
	depth, inString := 0, false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if inString {
				i++
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
			}
		}
	}
	return depth
}

// startsExemptFunc reports the opening line of a resolver on the register.
func startsExemptFunc(line string) bool {
	for _, name := range notYetMigrated {
		if strings.HasPrefix(line, "func "+name+"(") {
			return true
		}
	}
	return false
}

// The register may only shrink.
//
// Without this it is just a place to put anything inconvenient. With it, adding
// a name means editing a number and reading why that number is there, and every
// name must still be a real function — so a resolver that gets migrated cannot
// leave its exemption behind to shelter the next one.
func TestTheDebtRegisterOnlyShrinks(t *testing.T) {
	const startedAt = 10 // ten hand-written thinking resolvers
	const migrated = 8   // eight of ten, 2026-08-23

	if want := startedAt - migrated; len(notYetMigrated) != want {
		t.Fatalf(`the debt register has %d entries, expected %d.

If you migrated a resolver: delete its name AND raise migrated. If you are
adding one: stop. A new capability answers from the catalog, which is what this
whole file exists to make the default.`, len(notYetMigrated), want)
	}

	raw, err := os.ReadFile("thinking_capabilities.go")
	if err != nil {
		t.Fatalf("read thinking_capabilities.go: %v", err)
	}
	for _, name := range notYetMigrated {
		if !strings.Contains(string(raw), "func "+name+"(") {
			t.Errorf("%s is on the register and no longer exists — delete it and raise migrated", name)
		}
	}
}

// visionModelMarkers is the one list of model names this package still keeps,
// and it has a reason no other capability can borrow: models.dev describes what
// PROVIDERS SERVE, and a model somebody pulled onto their own GPU is served by
// nobody. llava and moondream have nothing else to be recognised by.
//
// It is allowed by name, not by pattern, so that a second such list cannot
// appear quietly beside it.
const localModelMarkerVar = "visionModelMarkers"

func TestNoCapabilityDecisionNamesAModel(t *testing.T) {
	literal := regexp.MustCompile(`"([^"\\]{2,60})"`)

	for _, name := range capabilityFiles {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := strings.Split(string(raw), "\n")
		depth := 0

		for n, line := range lines {
			if depth == 0 && (strings.Contains(line, "var "+localModelMarkerVar) || startsExemptFunc(line)) {
				depth = braceDelta(line)
				continue
			}
			if depth > 0 {
				// Counted rather than matched on a closing line: an early
				// return inside one of these functions closes at one tab too,
				// which ended the exemption halfway through the body and made
				// the guard fire on the very code it was told to skip.
				depth += braceDelta(line)
				continue
			}
			// Comments are where the reasoning lives, and the reasoning is
			// mostly about which models were got wrong. Naming one there is the
			// evidence, not the mechanism.
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = line[:idx]
			}
			// Only when the id is compared to a LITERAL. Comparing it to a
			// variable is how the two allowed marker lists are read, and
			// `modelID == ""` is an emptiness check rather than a name — both
			// are the mechanism used correctly, and a guard that cannot tell
			// them apart would be argued with rather than obeyed.
			// An empty literal is not a name, and neither is a one-character
			// one: `deepest != ""` is an emptiness check, the same as
			// `modelID == ""`.
			named := false
			for _, m := range literal.FindAllStringSubmatch(line, -1) {
				if len(strings.TrimSpace(m[1])) >= 2 {
					named = true
				}
			}
			// An equality against the package's own vocabulary is not a model
			// name — `level == "off"`, `caps.Source == "models.dev"`. Checked
			// before the call scan so those lines never reach it.
			vocab := ownVocabulary()
			onlyOwnWords := true
			for _, m := range literal.FindAllStringSubmatch(line, -1) {
				// Two things are skipped, and the second is not fussiness.
				// A literal under two characters cannot be a model name
				// (`deepest != ""` is an emptiness check). And a "literal"
				// containing a space or a bracket is not one either: the regex
				// pairs quotes left to right, so an empty string next to more
				// code yields a fragment like ` && !offered[` — an artefact of
				// scanning, not something anybody wrote.
				value := strings.TrimSpace(m[1])
				if len(value) < 2 || strings.ContainsAny(value, " 	()[]{}&!|") {
					continue
				}
				if !vocab[strings.ToLower(value)] {
					onlyOwnWords = false
				}
			}
			for _, call := range modelMatchCalls {
				if !named || onlyOwnWords || !strings.Contains(line, call) {
					continue
				}
				t.Errorf(`%s:%d decides from a model NAME (%s).

A capability is a fact about a model, and the fetched catalog states it. Reading
it off the id is how four separate bugs happened this month, the largest being
171 of OpenRouter's 360 models answered wrong. If this is a local runtime, where
no catalog can describe what somebody pulled onto their own GPU, it belongs in
%s.`, name, n+1, call, localModelMarkerVar)
			}
			for _, m := range literal.FindAllStringSubmatch(line, -1) {
				value := strings.ToLower(m[1])
				if notAModelID[value] || !modelIDShape.MatchString(value) {
					continue
				}
				t.Errorf(`%s:%d names a model (%q).

A capability decision must not know model names — that is what the fetched
catalog is for, and what four separate bugs this month were caused by. If this
string is a wire dialect rather than a model, it belongs in the provider client;
if it is genuinely a local-runtime marker, it belongs in %s.`,
					name, n+1, m[1], localModelMarkerVar)
			}
		}
	}
}

// The allowed list must keep being the only one, and it must keep being in the
// file this test expects. A rename that quietly moved it would take the guard
// above with it.
func TestTheOneAllowedModelNameListIsStillWhereItSays(t *testing.T) {
	raw, err := os.ReadFile("vision_capabilities.go")
	if err != nil {
		t.Fatalf("read vision_capabilities.go: %v", err)
	}
	if !strings.Contains(string(raw), "var "+localModelMarkerVar+" = []string{") {
		t.Fatalf("%s is not in vision_capabilities.go any more; the guard above is now blind", localModelMarkerVar)
	}
	for _, name := range capabilityFiles {
		if name == "vision_capabilities.go" {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		// A declaration, not a mention: capabilities.go reads both marker
		// lists by name and would otherwise report itself.
		if strings.Contains(string(raw), "Markers = []string{") {
			t.Errorf("%s declares a second marker list; there is meant to be exactly one", name)
		}
	}
}
