package main

// The agent's tabs, plural.
//
// It had exactly one from 2026-08-10 until now, and that rule was really two
// rules wearing one coat:
//
//   - **Ownership.** The agent works tabs it opened; the user's are the user's.
//     This is the one that matters, it is what §127 spent a week getting right,
//     and it is untouched here — the `web-agent-` prefix separates the two at
//     any number, and nothing below can reach a tab the agent did not open.
//   - **Count.** There is one of them. This came from fixing "เปิดใหม่ ๆ รัว ๆ"
//     (tab after tab, each stranded), and it fixed that by removing the ability
//     rather than by managing it.
//
// The agent found the seam itself while being tested, 17 ส.ค.: *"ยังไม่พบความ
// สามารถสร้างแท็บใหม่หรือควบคุมหลายแท็บพร้อมกัน"*. Owner: *"ควรทำเหมือน skill ที่
// ห่อแท็ปแบบนั้น มันจะได้ดูได้"* — and the "ดูได้" half already exists, because a
// tab the agent opens is a real tab in the strip, flagged `mine`, that the user
// can see and close.
//
// What the count rule was protecting is still protected, differently: reuse
// stays the default, so a plain `open` still replaces the current page and a
// session that never asks for a second tab behaves exactly as it did.
//
// ## The hazard, named
//
// refs live in the page, as `data-aetox-ref` attributes textScript stamps on
// during a read. They are therefore per-tab and always were — what changes is
// that the agent can now be on a different tab than the one it read. Selecting
// is a page change like any other, so the answer says so in the words the tool
// description already uses: read, act, read again.

import (
	"fmt"
	"strings"

	"github.com/Mikedev115/Aetox/internal/skill"
)

type browserTabsSkill struct{ app *App }

func (s *browserTabsSkill) run(action, id string) (skill.Output, error) {
	out := skill.Output{Name: "browser_tabs", Command: strings.TrimSpace("browser tabs " + action + " " + id)}
	a := s.app

	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "list":
		out.Content = a.agentTabList()
		out.Success = true
	case "select":
		if err := a.selectAgentTab(id); err != nil {
			out.Content, out.Stderr = err.Error(), err.Error()
			return out, err
		}
		out.Success = true
		out.Content = fmt.Sprintf("ทำงานกับแท็บ %s แล้ว%s refs จากการ read ก่อนหน้าใช้ไม่ได้กับหน้านี้ อ่านใหม่ก่อนคลิก",
			id, a.browserWhere(AgentTabID(id)))
	case "close":
		// No id means this one. The model has just been working a page and
		// wants it shut; making it call `list` first to learn an id the tool
		// already knows is friction that produces a refusal instead of a close
		// — which is exactly what happened the first time the guidance told a
		// model to tidy up after itself (owner's run, 24 ส.ค.).
		//
		// `select` keeps needing an id, and that is not an inconsistency: "go to
		// the tab I am already on" is not a thing anybody asks for.
		if strings.TrimSpace(id) == "" {
			current, curErr := a.agentTab()
			if curErr != nil {
				err := fmt.Errorf("ไม่มีแท็บที่เปิดอยู่ให้ปิด")
				out.Content, out.Stderr = err.Error(), err.Error()
				return out, err
			}
			id = string(current)
			// Named in the row too, or the timeline would show a close with no
			// subject for the one case where the tool worked out the subject.
			out.Command = "browser tabs close " + id
		}
		if err := a.closeAgentTab(id); err != nil {
			out.Content, out.Stderr = err.Error(), err.Error()
			return out, err
		}
		out.Success = true
		out.Content = "ปิดแท็บ " + id + " แล้ว\n" + a.agentTabList()
	default:
		err := fmt.Errorf("browser tabs %q is not one of list, select, close", action)
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}
	out.RawOutput = out.Content
	return out, nil
}

// agentTabs names every tab the agent owns, oldest first, and never one of the
// user's — the prefix check happens once, at open, and this reads the list that
// produced.
func (a *App) agentTabs() []string {
	h := a.browsers
	if h == nil {
		return nil
	}
	h.mu.Lock()
	ids := append([]string(nil), h.agentOrder...)
	h.mu.Unlock()
	// A tab the user closed is gone from the map before the list is pruned in
	// some orderings, and a list is read by a model that will try to select
	// what it names.
	live := ids[:0]
	for _, id := range ids {
		if h.live(id) {
			live = append(live, id)
		}
	}
	return live
}

func (a *App) agentTabList() string {
	ids := a.agentTabs()
	if len(ids) == 0 {
		return "ยังไม่ได้เปิดหน้าไหนเลย ใช้ open"
	}
	current, _ := a.agentTab()
	var b strings.Builder
	b.WriteString("แท็บของคุณ:\n")
	for _, id := range ids {
		mark := "  "
		if AgentTabID(id) == current {
			mark = "* " // the one every other action works
		}
		ref := "(ยังไม่ได้ไปไหน)"
		if t := a.browsers.tab(id); t != nil {
			if named := browserPageRef(t.meta()); named != "" {
				ref = named
			}
		}
		fmt.Fprintf(&b, "%s%s — %s\n", mark, id, ref)
	}
	b.WriteString("* คือแท็บที่ open/read/click/type/capture ทำงานด้วยตอนนี้")
	return b.String()
}

// selectAgentTab makes one of the agent's own tabs the current one, and raises
// it so the user sees the page the agent moved to — the same promise `open`
// makes, for the same reason.
func (a *App) selectAgentTab(id string) error {
	if err := a.mustOwn(id); err != nil {
		return err
	}
	h := a.browsers
	h.mu.Lock()
	h.agentID = id
	h.mu.Unlock()

	var url string
	if t := h.tab(id); t != nil {
		_, url = t.meta()
	}
	a.emitEvent("workbench:open-browser", map[string]string{"id": id, "url": url})
	return nil
}

func (a *App) closeAgentTab(id string) error {
	if err := a.mustOwn(id); err != nil {
		return err
	}
	// closeTab, not BrowserClose: this IS the agent closing its own tab, and
	// telling it afterwards that the user closed the page would be a lie it
	// would then act on.
	a.closeTab(id, closedByAgent)
	return nil
}

// mustOwn is the whole of the ownership rule as the tabs action sees it: an id
// the agent did not open is not refused for being unknown, it is refused for
// being somebody else's. Saying which is the difference between a model that
// tries a different id and one that tries harder.
func (a *App) mustOwn(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("which tab? use list first")
	}
	for _, own := range a.agentTabs() {
		if own == id {
			return nil
		}
	}
	if isAgentTabID(id) {
		return fmt.Errorf("แท็บ %s ปิดไปแล้ว ใช้ list เพื่อดูว่าเหลืออะไร", id)
	}
	return fmt.Errorf("แท็บ %s เป็นของผู้ใช้ ไม่ใช่ของคุณ คุณทำงานได้เฉพาะแท็บที่คุณเปิดเอง (list)", id)
}
