package main

// ไฟบอกสถานะ — the four layers that say the agent is working the browser, and
// the switches that turn each one off.
//
// Owner, 24 ส.ค., on why they are switches at all: *"ไหนๆก็ทำแยกกันแล้วครับจะ
// ได้เพิ่มตัวเลือกให้ผุ้ใช้ด้วย"*. They were built as four independent layers
// because they answer four different questions; once that was true, letting a
// person keep the ones they want cost almost nothing.
//
// **No re-bootstrap.** SetDelegateOff next door calls applyConfig, because what
// the model can DO changes with it and the tools are built at bootstrap. None
// of that is true here: not one of these four reaches the engine, the prompt or
// the tool block. They decide what the window draws. So this is a
// load-modify-save on the preference file, the same shape as SetUserName, and a
// person flipping a switch does not pay for a rebuilt agent.
//
// **The names are the user's, not the repository's.** The first draft called
// one of these "ชิปแท็บ" and the owner read it back: *"ผมอ่านผมไม่รู้นะคืออะไร"*.
// A setting whose name only makes sense to whoever wrote the code is a setting
// nobody can decide about, so every one of these is named for what a person
// SEES.

import (
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
)

// BusyLayer is one switch as the window draws it: a stable id to send back, the
// words to show, and whether it is on right now.
//
// The label and the note travel from Go rather than being looked up in the
// frontend's locale files, for the reason `desk` manifests do the same: the id
// is ours and the words are the product's, and a second table of names in the
// window is a second place for them to drift from what the setting actually
// does.
type BusyLayer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Note  string `json:"note"`
	On    bool   `json:"on"`
}

// The four ids, spelled once. Every caller — the getter, the setter, the
// frontend's checklist — goes through these rather than through a literal.
const (
	busyEdgeGlow  = "edgeGlow"
	busyActionBar = "actionBar"
	busyTabDot    = "tabDot"
	busyPageMarks = "pageMarks"
)

// BusySignal reports the four layers in the order they are shown.
//
// Ordered by how much of the window each one touches — the panel's border, then
// a strip inside it, then a mark on one tab, then the page itself. A list of
// switches that jumps around is one nobody builds a mental model of.
func (a *App) BusySignal() []BusyLayer {
	cfg := a.cur().cfg
	return []BusyLayer{
		{
			ID:    busyEdgeGlow,
			Label: "ขอบแผงเรืองแสง",
			Note:  "ขอบแผงเบราว์เซอร์เรืองแสงตอนเอเจนต์ทำงาน",
			On:    !cfg.BusyEdgeGlowOff,
		},
		{
			ID:    busyActionBar,
			Label: "แถบบอกการกระทำ",
			Note:  "บอกเป็นคำว่ากำลังเปิดหน้า อ่าน กด หรือพิมพ์อะไรอยู่",
			On:    !cfg.BusyActionBarOff,
		},
		{
			ID:    busyTabDot,
			Label: "จุดบนแท็บที่กำลังใช้",
			Note:  "บอกว่ากำลังทำงานอยู่ที่แท็บไหน มีประโยชน์ตอนเปิดหลายแท็บ",
			On:    cfg.BusyTabDot,
		},
		{
			ID:    busyPageMarks,
			Label: "ลูกศรและวงแหวนบนหน้าเว็บ",
			Note:  "ชี้จุดที่กำลังกด และทิศที่กำลังเลื่อน วาดลงบนหน้าเว็บโดยตรง",
			On:    !cfg.BusyPageMarksOff,
		},
	}
}

// SetBusyLayer turns one layer on or off and writes it down.
//
// An unknown id is ignored rather than guessed at, the same rule
// SetDelegateOff's `kind` follows: a typo that fell through to a default would
// flip a switch the caller never named.
//
// The write goes to the preference file AND to the live conversation's config,
// because the window reads the live one and the next launch reads the file.
// Missing either half is a setting that works until you restart, or one that
// only works after you do.
func (a *App) SetBusyLayer(id string, on bool) []BusyLayer {
	id = strings.TrimSpace(id)
	conv := a.cur()
	switch id {
	case busyEdgeGlow:
		conv.cfg.BusyEdgeGlowOff = !on
	case busyActionBar:
		conv.cfg.BusyActionBarOff = !on
	case busyTabDot:
		conv.cfg.BusyTabDot = on
	case busyPageMarks:
		conv.cfg.BusyPageMarksOff = !on
	default:
		return a.BusySignal()
	}
	// Every live chat, not only the one on screen: this is a fact about how the
	// window draws, and a second chat opened later must not come back with the
	// switches somebody turned off an hour ago still on.
	for _, other := range a.convs.all() {
		if other == conv {
			continue
		}
		other.cfg.BusyEdgeGlowOff = conv.cfg.BusyEdgeGlowOff
		other.cfg.BusyActionBarOff = conv.cfg.BusyActionBarOff
		other.cfg.BusyTabDot = conv.cfg.BusyTabDot
		other.cfg.BusyPageMarksOff = conv.cfg.BusyPageMarksOff
	}
	// And the config a NEW chat is born with, or the next one opened would be
	// built from the state before the click (see conversation.cfg).
	a.cfg.BusyEdgeGlowOff = conv.cfg.BusyEdgeGlowOff
	a.cfg.BusyActionBarOff = conv.cfg.BusyActionBarOff
	a.cfg.BusyTabDot = conv.cfg.BusyTabDot
	a.cfg.BusyPageMarksOff = conv.cfg.BusyPageMarksOff
	saveBusySignal(conv.cfg)
	return a.BusySignal()
}

// saveBusySignal is the load-modify-save half. Failures are silent on purpose:
// a switch that could not be written down still took effect on screen, and an
// error banner about a visual preference would be louder than the preference.
func saveBusySignal(cfg config.Config) {
	pref, _, err := config.LoadModelPreference()
	if err != nil {
		return
	}
	pref.BusyEdgeGlowOff = cfg.BusyEdgeGlowOff
	pref.BusyActionBarOff = cfg.BusyActionBarOff
	pref.BusyTabDot = cfg.BusyTabDot
	pref.BusyPageMarksOff = cfg.BusyPageMarksOff
	_ = config.SaveModelPreference(pref)
}
