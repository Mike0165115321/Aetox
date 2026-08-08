package connect

import (
	"context"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
}

// The catalog is the register. If an entry is malformed the settings page draws
// a row nobody can use, so the shape is checked rather than assumed.
func TestEveryCatalogEntryIsUsable(t *testing.T) {
	for _, p := range catalog {
		if p.ID == "" || p.Label == "" {
			t.Fatalf("entry %+v has no id or label", p)
		}
		if p.connect == nil || p.verify == nil || p.status == nil || p.disconnect == nil {
			t.Fatalf("%s does not implement all four verbs", p.ID)
		}
		if len(p.Tools) == 0 {
			t.Fatalf("%s contributes no tools — nothing would change when it is switched off", p.ID)
		}
		if p.Kind == KindToken && p.TokenURL == "" {
			t.Fatalf("%s asks for a pasted token but says nowhere to mint one", p.ID)
		}
	}
}

// A tool that belongs to no connection is not this package's business. Gating
// it here would take away tools nobody attached to an account.
func TestToolsOutsideTheCatalogAreAlwaysAllowed(t *testing.T) {
	for _, tool := range []string{"read", "shell", "web_search", ""} {
		if !Allows(tool, nil) {
			t.Fatalf("%q was gated, but belongs to no connection", tool)
		}
	}
}

func TestConnectionToolsFollowTheDesksPlacement(t *testing.T) {
	if !Allows("github_search", []string{"github"}) {
		t.Fatal("a desk holding github cannot see github_search")
	}
	if Allows("github_search", []string{"gmail"}) {
		t.Fatal("a desk holding another connection can see github_search")
	}
	if Allows("github_search", nil) {
		t.Fatal("a desk holding nothing can see github_search")
	}
}

// The install tool reaches GitHub like the rest, so a desk with the connection
// off must not keep exactly one tool that still goes there.
func TestPluginInstallIsGatedWithTheRestOfGitHub(t *testing.T) {
	if Allows("plugin_install", nil) {
		t.Fatal("plugin_install survived a desk that does not hold github")
	}
	owner, ok := ProviderOfTool("plugin_install")
	if !ok || owner != "github" {
		t.Fatalf("ProviderOfTool(plugin_install) = %q, %v; want github", owner, ok)
	}
}

func TestProviderOfToolIsCaseInsensitive(t *testing.T) {
	if owner, ok := ProviderOfTool("  GitHub_Search "); !ok || owner != "github" {
		t.Fatalf("owner = %q, %v; want github", owner, ok)
	}
	if _, ok := ProviderOfTool("read"); ok {
		t.Fatal("read was claimed by a connection")
	}
}

// Absent placement means every desk — an install that never opened the page has
// not asked for its tools to go away.
func TestUnplacedConnectionReportsItselfAsUnconfigured(t *testing.T) {
	isolate(t)

	rows := List()
	if len(rows) != len(catalog) {
		t.Fatalf("List returned %d rows, want %d", len(rows), len(catalog))
	}
	for _, row := range rows {
		if row.Configured {
			t.Fatalf("%s reports a placement with an empty config", row.ID)
		}
		if row.For == nil {
			t.Fatalf("%s sent a nil list to the UI, which renders as absent rather than empty", row.ID)
		}
		if row.Connected {
			t.Fatalf("%s reports connected with an empty vault", row.ID)
		}
	}
}

func TestSetTargetsIsReportedBackByList(t *testing.T) {
	isolate(t)

	if err := SetTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetTargets: %v", err)
	}
	row, ok := StatusOf("github")
	if !ok {
		t.Fatal("github vanished from the catalog")
	}
	if !row.Configured || len(row.For) != 1 || row.For[0] != "coding" {
		t.Fatalf("row = %+v, want a placement of exactly [coding]", row)
	}
	// And the desks agree with the page.
	if !Allows("github_search", config.ConnectionsForDesk("coding", IDs())) {
		t.Fatal("the coding desk cannot see a tool the page says it holds")
	}
	if Allows("github_search", config.ConnectionsForDesk("assistant", IDs())) {
		t.Fatal("the assistant desk sees a tool the page says it does not hold")
	}
}

// Reconnecting is about the token, not about where the account belongs. Making
// the user choose desks again would treat one decision as two.
func TestDisconnectKeepsThePlacement(t *testing.T) {
	isolate(t)

	if err := SetTargets("github", []string{"coding"}); err != nil {
		t.Fatalf("SetTargets: %v", err)
	}
	if err := Disconnect("github"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	row, _ := StatusOf("github")
	if !row.Configured || len(row.For) != 1 {
		t.Fatalf("placement after disconnect = %+v, want it kept", row.For)
	}
}

func TestUnknownConnectionIsRefusedByEveryVerb(t *testing.T) {
	isolate(t)

	if _, err := Connect(context.Background(), "myspace", "token", nil); err == nil {
		t.Fatal("Connect accepted an id the catalog does not have")
	}
	if err := SetTargets("myspace", []string{"coding"}); err == nil {
		t.Fatal("SetTargets accepted an unknown id")
	}
	if err := Disconnect("myspace"); err == nil {
		t.Fatal("Disconnect accepted an unknown id")
	}
	if _, ok := StatusOf("myspace"); ok {
		t.Fatal("StatusOf answered for an unknown id")
	}
}

// A connect that fails must not leave the placement written for an account that
// was never attached — but the reverse order is worse, so the rule is that a
// failed connect stores nothing the user can mistake for success. The placement
// alone is inert: no token, so no reach.
func TestFailedConnectStoresNoAccount(t *testing.T) {
	isolate(t)

	// Points at a token the real GitHub will never accept; the call fails at
	// the network or at the 401, and either way nothing is attached.
	if _, err := Connect(context.Background(), "github", "definitely-not-a-token", []string{"coding"}); err == nil {
		t.Skip("network reachable and GitHub accepted a junk token — nothing to assert")
	}
	row, _ := StatusOf("github")
	if row.Connected {
		t.Fatal("a failed connect left an account attached")
	}
}
