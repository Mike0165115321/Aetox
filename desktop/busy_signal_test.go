package main

// The four switches behind ไฟบอกสถานะ.
//
// What is worth pinning is not that a boolean flips — it is that the shipped
// defaults reach the window unaltered, that flipping one leaves the other three
// alone, and that a switch reaches every chat rather than only the one on
// screen.

import (
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

func layersByID(t *testing.T, list []BusyLayer) map[string]BusyLayer {
	t.Helper()
	out := map[string]BusyLayer{}
	for _, l := range list {
		out[l.ID] = l
	}
	return out
}

// Three ship on, one ships off — and a fresh install must show exactly that
// without anybody having applied a default anywhere.
func TestBusySignalShipsThreeOnAndOneOff(t *testing.T) {
	app := &App{}
	got := layersByID(t, app.BusySignal())
	if len(got) != 4 {
		t.Fatalf("want four layers, got %d", len(got))
	}
	for _, id := range []string{busyEdgeGlow, busyActionBar, busyPageMarks} {
		if !got[id].On {
			t.Errorf("%s ships on and came back off", id)
		}
	}
	if got[busyTabDot].On {
		t.Errorf("%s ships off and came back on", busyTabDot)
	}
}

// Every switch needs words a person can decide from. The one that failed this
// test in the design review was called "ชิปแท็บ".
func TestEveryLayerSaysWhatItIsInWords(t *testing.T) {
	for _, l := range (&App{}).BusySignal() {
		if l.Label == "" {
			t.Errorf("%s has no label", l.ID)
		}
		if l.Note == "" {
			t.Errorf("%s has no note — a switch with no explanation is one nobody can decide about", l.ID)
		}
	}
}

// Flipping one must leave the other three where they were.
func TestFlippingOneLayerLeavesTheRestAlone(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	app := &App{}

	got := layersByID(t, app.SetBusyLayer(busyEdgeGlow, false))
	if got[busyEdgeGlow].On {
		t.Error("the edge glow did not go off")
	}
	if !got[busyActionBar].On || !got[busyPageMarks].On {
		t.Error("turning one off took another with it")
	}
	if got[busyTabDot].On {
		t.Error("turning one off turned the shipped-off one on")
	}

	got = layersByID(t, app.SetBusyLayer(busyTabDot, true))
	if !got[busyTabDot].On {
		t.Error("the tab dot did not come on")
	}
	if got[busyEdgeGlow].On {
		t.Error("the edge glow came back on by itself")
	}
}

// An id nobody has is ignored rather than guessed at — the same rule
// SetDelegateOff's kind follows, and for the same reason: a typo that fell
// through to a default would flip a switch the caller never named.
func TestAnUnknownLayerChangesNothing(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	app := &App{}
	before := layersByID(t, app.BusySignal())

	after := layersByID(t, app.SetBusyLayer("nosuchlayer", false))

	for id, l := range before {
		if after[id].On != l.On {
			t.Errorf("%s moved on an unknown id", id)
		}
	}
}

// A switch is a fact about how the window draws, not about one conversation.
// A chat opened later must not come back with the switches somebody turned off
// an hour ago still on.
func TestASwitchReachesEveryChat(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	app := &App{}
	background := app.cur()
	background.id = "20260824-000000.001"
	app.convs.show(background)

	other := newConversation()
	other.id = "20260824-000000.002"
	app.convs.show(other)

	app.SetBusyLayer(busyEdgeGlow, false)

	if !background.cfg.BusyEdgeGlowOff {
		t.Error("the chat that was not on screen kept the old setting")
	}
	// And the config a brand-new chat is born with.
	if !app.cfg.BusyEdgeGlowOff {
		t.Error("the next chat opened would be born with the old setting")
	}
}

// It is written down, or it is a setting that works until you restart.
func TestASwitchIsWrittenToThePreferenceFile(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	app := &App{}

	app.SetBusyLayer(busyTabDot, true)
	app.SetBusyLayer(busyActionBar, false)

	pref, ok, err := config.LoadModelPreference()
	if err != nil || !ok {
		t.Fatalf("preference not written: ok=%v err=%v", ok, err)
	}
	if !pref.BusyTabDot {
		t.Error("turning the tab dot on was not written down")
	}
	if !pref.BusyActionBarOff {
		t.Error("turning the action bar off was not written down")
	}
	if pref.BusyEdgeGlowOff || pref.BusyPageMarksOff {
		t.Error("a layer nobody touched was written as off")
	}
}
