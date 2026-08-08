package config

import (
	"os"
	"testing"
)

// known is the set of connections the build ships. The matcher is given it so a
// stale id left in the file cannot come back as something nothing recognises.
var known = []string{"github", "gmail"}

func connectionsRoot(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
}

// Before this file existed every desk could reach GitHub. An install that has
// never opened the new page has not asked for that to change.
func TestUnplacedConnectionIsCarriedByEveryDesk(t *testing.T) {
	connectionsRoot(t)

	for _, desk := range []string{"coding", "assistant", "specialized"} {
		got := ConnectionsForDesk(desk, known)
		if len(got) != 2 {
			t.Fatalf("desk %q carries %v, want both", desk, got)
		}
	}
	// The pre-modes full desk carries everything, same as MCPServersForDesk.
	if got := ConnectionsForDesk("", known); len(got) != 2 {
		t.Fatalf("full desk carries %v, want both", got)
	}
}

// An agent is handed things on purpose. Silence is not a grant.
func TestUnplacedConnectionReachesNoAgent(t *testing.T) {
	connectionsRoot(t)

	if got := ConnectionsForAgent("researcher", known); len(got) != 0 {
		t.Fatalf("agent carries %v with nothing placed, want none", got)
	}
}

func TestPlacementDecidesWhichDesksCarryIt(t *testing.T) {
	connectionsRoot(t)

	if err := SetConnectionTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}

	if got := ConnectionsForDesk("coding", known); !contains(got, "github") {
		t.Fatalf("coding carries %v, want github", got)
	}
	if got := ConnectionsForDesk("assistant", known); contains(got, "github") {
		t.Fatalf("assistant carries %v, want github left off", got)
	}
	// gmail was never placed, so it is still on every desk — placing one
	// connection must not quietly place the others.
	if got := ConnectionsForDesk("assistant", known); !contains(got, "gmail") {
		t.Fatalf("assistant carries %v, want gmail untouched", got)
	}
}

// "Switched off everywhere" is a decision the user made, and it has to survive
// a reload. If it were stored as nil it would read back as never-configured and
// the connection would reappear on every desk.
func TestSwitchedOffEverywhereSurvives(t *testing.T) {
	connectionsRoot(t)

	if err := SetConnectionTargets("github", nil); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}
	targets, configured := ConnectionTargets("github")
	if !configured {
		t.Fatal("an explicit empty placement read back as never configured")
	}
	if len(targets) != 0 {
		t.Fatalf("targets = %v, want empty", targets)
	}
	if got := ConnectionsForDesk("coding", known); contains(got, "github") {
		t.Fatalf("coding carries %v after being switched off everywhere", got)
	}

	// On disk it must be [] rather than null, or a hand-edit of the file has
	// the same ambiguity.
	path, _ := ConnectionsPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read connections.json: %v", err)
	}
	if want := `"for": []`; !containsSub(string(raw), want) {
		t.Fatalf("file = %s, want it to contain %s", raw, want)
	}
}

func TestAgentPlacementUsesTheSameVocabularyAsMCP(t *testing.T) {
	connectionsRoot(t)

	if err := SetConnectionTargets("github", []string{"coding", MCPAgentPrefix + "researcher"}); err != nil {
		t.Fatalf("SetConnectionTargets: %v", err)
	}
	if got := ConnectionsForAgent("researcher", known); !contains(got, "github") {
		t.Fatalf("agent carries %v, want github", got)
	}
	if got := ConnectionsForAgent("someone-else", known); contains(got, "github") {
		t.Fatalf("an unnamed agent carries %v, want none", got)
	}
}

func TestPlacingTwiceReplacesRatherThanAppends(t *testing.T) {
	connectionsRoot(t)

	_ = SetConnectionTargets("github", []string{"coding"})
	_ = SetConnectionTargets("github", []string{"assistant"})

	items, err := LoadConnections()
	if err != nil {
		t.Fatalf("LoadConnections: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("stored %d entries, want one per connection", len(items))
	}
	if got := ConnectionsForDesk("coding", known); contains(got, "github") {
		t.Fatalf("coding still carries %v after being replaced", got)
	}
}

// Duplicates in the list a settings page sends are the page's problem, not the
// file's.
func TestTargetsAreDeduplicated(t *testing.T) {
	connectionsRoot(t)

	_ = SetConnectionTargets("github", []string{"coding", "Coding", " coding ", ""})
	targets, _ := ConnectionTargets("github")
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want one", targets)
	}
}

// A provider dropped from a later build leaves its row behind. It must not come
// back as an id nothing in this binary recognises.
func TestStaleEntryForAnUnknownConnectionIsIgnored(t *testing.T) {
	connectionsRoot(t)

	_ = SetConnectionTargets("retired-service", []string{"coding"})
	if got := ConnectionsForDesk("coding", known); contains(got, "retired-service") {
		t.Fatalf("coding carries %v, want the unknown id left out", got)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func containsSub(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
