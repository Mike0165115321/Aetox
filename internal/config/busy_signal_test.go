package config

// The busy signal's four switches, and the one property that matters about
// them: a machine that has never touched one lands on the shipped default
// without any code path having to remember to apply it.

import (
	"encoding/json"
	"testing"
)

// Three ship on, one ships off. Spelled so that the zero value already says
// so — which is what makes a fresh install, a test's blank Config, and a
// preference file that predates the feature all agree.
func TestAZeroConfigShipsTheRightBusyLayers(t *testing.T) {
	var cfg Config
	for _, c := range []struct {
		name string
		on   bool
	}{
		{"edge glow", !cfg.BusyEdgeGlowOff},
		{"action bar", !cfg.BusyActionBarOff},
		{"page marks", !cfg.BusyPageMarksOff},
	} {
		if !c.on {
			t.Errorf("%s ships on and a zero Config has it off", c.name)
		}
	}
	if cfg.BusyTabDot {
		t.Error("the tab dot ships off and a zero Config has it on")
	}
}

// A file written before these existed says nothing about them, and must keep
// taking whatever ships — including on the day a shipped default changes.
func TestAPreferenceFileFromBeforeTheFeatureTakesTheDefaults(t *testing.T) {
	var pref ModelPreference
	if err := json.Unmarshal([]byte(`{"ui_locale":"th"}`), &pref); err != nil {
		t.Fatal(err)
	}
	if pref.BusyEdgeGlowOff || pref.BusyActionBarOff || pref.BusyPageMarksOff {
		t.Error("a silent file turned a shipped-on layer off")
	}
	if pref.BusyTabDot {
		t.Error("a silent file turned a shipped-off layer on")
	}
}

// omitempty is doing real work here: a preference written by somebody who
// never opened these settings must not pin today's defaults into their file.
func TestUntouchedSwitchesAreNotWrittenDown(t *testing.T) {
	raw, err := json.Marshal(ModelPreference{UILocale: "th"})
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"busy_edge_glow_off", "busy_action_bar_off", "busy_tab_dot_on", "busy_page_marks_off",
	} {
		if _, written := back[key]; written {
			t.Errorf("%s was written for a preference nobody set — the default is now frozen in the file", key)
		}
	}
}

// And a real choice survives the round trip, in both directions.
func TestASwitchTheUserFlippedSurvives(t *testing.T) {
	raw, err := json.Marshal(ModelPreference{BusyEdgeGlowOff: true, BusyTabDot: true})
	if err != nil {
		t.Fatal(err)
	}
	var back ModelPreference
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.BusyEdgeGlowOff {
		t.Error("turning the edge glow off did not survive")
	}
	if !back.BusyTabDot {
		t.Error("turning the tab dot on did not survive")
	}
}
