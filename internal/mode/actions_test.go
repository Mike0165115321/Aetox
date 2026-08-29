package mode

import "testing"

// A `deny:` naming the pack refuses all of it. Somebody who wrote "not the
// browser" meant the browser, not eleven twelfths of it.
func TestDenyingAPackByNameRefusesEveryAction(t *testing.T) {
	m := &Mode{Name: "t", Categories: []string{"web"}, Deny: []string{"browser"}}
	for _, action := range []string{"browser_open", "browser_read", "browser_click"} {
		if m.AllowsAction("browser", action) {
			t.Errorf("AllowsAction(browser, %s) = true, want false — the pack is denied", action)
		}
	}
}

// The bug this fixes rather than a feature it adds: before AllowsAction, a
// manifest could name one action in `deny:` and be ignored, because the only
// question ever asked was about the packed name.
func TestDenyingOneActionRefusesOnlyThatOne(t *testing.T) {
	m := &Mode{Name: "t", Categories: []string{"web"}, Deny: []string{"browser_click"}}
	if m.AllowsAction("browser", "browser_click") {
		t.Error("a denied action came back allowed — this is the silent hole AllowsAction closes")
	}
	if !m.AllowsAction("browser", "browser_read") {
		t.Error("denying one action took the rest of the pack with it")
	}
}

// `tools:` naming the pack grants all of it — the one-word way of saying so,
// and what every manifest written before packing existed meant.
func TestNamingAPackInToolsGrantsEveryAction(t *testing.T) {
	// No category that would let the browser through: the grant has to come
	// from the name alone.
	m := &Mode{Name: "t", Categories: []string{"files"}, Tools: []string{"browser"}}
	for _, action := range []string{"browser_open", "browser_click", "browser_network"} {
		if !m.AllowsAction("browser", action) {
			t.Errorf("AllowsAction(browser, %s) = false, want true — the pack is named in tools", action)
		}
	}
}

// A manifest that names actions gets exactly those. This is the specialized
// desk's shape (`tools: read, write, list, glob`) and the reason packing the
// file tools cannot be allowed to cost it anything.
func TestAManifestThatNamesActionsGetsOnlyThose(t *testing.T) {
	m := &Mode{Name: "t", Categories: []string{"agent"}, Tools: []string{"shell_output"}}
	if !m.AllowsAction("shell", "shell_output") {
		t.Error("a named action was refused")
	}
	if m.AllowsAction("shell", "shell") {
		t.Error("naming one action of shell handed over the whole of it")
	}
}

// The nil desk is the pre-desks full desk and carries everything, actions
// included.
func TestTheNilDeskAllowsEveryAction(t *testing.T) {
	var m *Mode
	if !m.AllowsAction("browser", "browser_click") {
		t.Error("the nil desk refused an action; it has never refused anything")
	}
}

// วางแผน keeps what reads and drops what acts — now inside a pack as well as
// across the block, which is the whole reason the filter exists.
func TestPlanKeepsTheBrowsersReadingHalf(t *testing.T) {
	s := StancePlan
	for _, action := range []string{
		"browser_open", "browser_read", "browser_wait",
		"browser_back", "browser_console", "browser_network",
	} {
		if !s.AllowsAction("browser", action) {
			t.Errorf("วางแผน dropped %s, which only reads", action)
		}
	}
	for _, action := range []string{"browser_click", "browser_type", "browser_tabs", "browser_dialog", "browser_capture"} {
		if s.AllowsAction("browser", action) {
			t.Errorf("วางแผน kept %s, which does not only read", action)
		}
	}
}

// A pack named whole in the list means the whole of it — how "desk" and
// "github" were already written, before there was anything to read them.
func TestPlanReadsAPackNamedWholeAsWhole(t *testing.T) {
	s := StancePlan
	for _, action := range []string{"desk_open", "desk_list", "desk_close"} {
		if !s.AllowsAction("desk", action) {
			t.Errorf("วางแผน dropped %s, but the desk is on the list whole", action)
		}
	}
}

// The two stances at the ends: one takes nothing, one takes everything.
func TestActAndConsultAnswerActionsTheSameWayTheyAnswerTools(t *testing.T) {
	if !StanceAct.AllowsAction("browser", "browser_click") {
		t.Error("ลงมือ refused an action; it refuses nothing")
	}
	if StanceConsult.AllowsAction("browser", "browser_read") {
		t.Error("คู่คิด kept an action; it carries no tools at all")
	}
}

// The gap that was found while wiring this: a plan is built on the shape of a
// project, and repo_map is that shape for about a thousand tokens.
func TestPlanCanSeeTheRepoMap(t *testing.T) {
	if !StancePlan.AllowsTool("repo_map") {
		t.Error("วางแผน cannot read the repo map, which is the cheapest thing a plan can stand on")
	}
}
