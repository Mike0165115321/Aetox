package main

// Why a ref missed, answered from what the tab already knows.
//
// Measured on the owner's own session, 25 ส.ค.: a full read of wikipedia.org
// tagged its elements, a second read with filter="English" tagged three, and
// `type ref 11` came back with "ref มาจาก browser_read ครั้งล่าสุด และหมดอายุ
// ทันทีที่หน้าเปลี่ยน" on a page that had not changed at all. The model read
// again without the filter and the same ref worked. One round spent, and the
// sentence that spent it pointed at the wrong thing.
//
// Every read reassigns refs from 1 and strips the ones before it, so the
// commonest way a ref dies is the NEXT READ — and the recovery for that is
// different from the one the old sentence describes.

import (
	"strings"
	"testing"
)

func TestAFilteredReadIsNamedAsWhyTheRefMissed(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.browsers.tab("web-agent-1").noteRefs(3, "English")

	why := app.browserWhyRefMissed("web-agent-1", 11)

	for _, want := range []string{"English", "3", "filter"} {
		if !strings.Contains(why, want) {
			t.Errorf("the answer does not name %q: %s", want, why)
		}
	}
	if strings.Contains(why, "หน้าเปลี่ยน") {
		t.Errorf("the page had not changed and the answer says it did: %s", why)
	}
}

// The same miss with no filter in play: the read simply tagged fewer than the
// ref being asked for, which is still the numbering and still not the page.
func TestARefPastTheEndOfTheLastReadSaysSo(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.browsers.tab("web-agent-1").noteRefs(8, "")

	why := app.browserWhyRefMissed("web-agent-1", 11)

	if !strings.Contains(why, "8") {
		t.Errorf("the answer does not say how many the last read tagged: %s", why)
	}
	if strings.Contains(why, "หน้าเปลี่ยน") {
		t.Errorf("a ref out of range was blamed on the page changing: %s", why)
	}
}

// A ref INSIDE the last read's range that still matched nothing is the case the
// old sentence was written for, and it keeps it: the numbering is fine, so
// something about the page moved.
func TestARefInsideTheRangeStillBlamesThePage(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.browsers.tab("web-agent-1").noteRefs(20, "")

	why := app.browserWhyRefMissed("web-agent-1", 11)

	if !strings.Contains(why, "หน้าเปลี่ยน") {
		t.Errorf("a ref the last read really did tag should point at the page: %s", why)
	}
}

// Nothing has read the page yet, so there are no refs at all — which is not a
// stale ref and must not be described as one.
func TestARefBeforeAnyReadSaysThereIsNoneYet(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")

	why := app.browserWhyRefMissed("web-agent-1", 1)

	if !strings.Contains(why, "read") {
		t.Errorf("the answer does not send the caller to read first: %s", why)
	}
	if strings.Contains(why, "หน้าเปลี่ยน") {
		t.Errorf("a page nobody has read was blamed for changing: %s", why)
	}
}

// A filter that matched nothing leaves zero refs on a page that may be full of
// them, and "read again without the filter" is the whole of the recovery.
func TestAFilterThatMatchedNothingSaysWhatToDropped(t *testing.T) {
	app := hostWithTabs(t, "web-agent-1", []string{"web-agent-1"}, "web-agent-1")
	app.browsers.tab("web-agent-1").noteRefs(0, "Deutsch")

	why := app.browserWhyRefMissed("web-agent-1", 1)

	if !strings.Contains(why, "Deutsch") {
		t.Errorf("the answer does not name the filter that emptied the list: %s", why)
	}
}

// The guidance is the other half, and it is the half that stops the round being
// spent at all: the rule has to arrive before the failure, not with it.
func TestReadGuidanceSaysARereadRenumbersTheRefs(t *testing.T) {
	s := &browserSkill{}
	for _, action := range []string{"read", "click", "type"} {
		guide := s.Guidance(map[string]any{"action": action})
		if !strings.Contains(guide, "renumber") {
			t.Errorf("%s guidance does not say a later read renumbers the refs: %s", action, guide)
		}
	}
}
