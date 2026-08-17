package main

// Going back.
//
// This is the smallest action in the browser and the one with the largest
// effect on everything else, because of what it does to a *decision* rather
// than to a capability.
//
// Before it, `open` was destructive: the page you were on was gone, and the
// only way to return was to open its URL again — which is not the same page. A
// form half filled is empty again, a scroll position is lost, a result that
// came from a POST cannot be re-fetched at all, and a list of search results
// may have moved. So the agent had to *predict*, before every open, whether it
// would want the current page later. Owner, 17 ส.ค., on the first attempt to
// solve that by writing a better instruction: *"ไม่ใช่ไปพรอบบอกมัน แต่ทำให้ tool
// ทำให้มันสามารถทำได้"*.
//
// A prediction is the one thing a model cannot be instructed into doing well.
// `back` removes the prediction instead of coaching it: being wrong now costs
// one action rather than a page. That is also why the `newTab` guidance in the
// tool description could get SHORTER when this landed — the tool absorbed what
// the prose was trying to teach.

import (
	"context"
	"fmt"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
)

type browserBackSkill struct{ app *App }

// backWait is short on purpose. A real back is a navigation the engine has
// already cached and it lands in well under a second; the only way to spend the
// whole budget is to have nowhere to go, and telling the agent that quickly
// matters more than surviving one slow revalidation.
const backWait = 6 * time.Second

func (s *browserBackSkill) back(ctx context.Context) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "browser_back", Command: "browser back"}
	a := s.app

	id, err := a.agentTab()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}
	before := a.browserWhere(id)

	tab := a.browsers.tab(string(id))
	if tab == nil {
		err := fmt.Errorf("no browser tab %q", id)
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}
	// Armed before the ask, like every other navigation in this app, so the wait
	// below is this one's and not the last one's.
	tab.armNavigation()
	a.onTab(string(id), func(v tabView, _ *browserTab) { v.eval("history.back()") })

	waitErr := tab.awaitNavigation(ctx, backWait)
	out.DurationMs = time.Since(start).Milliseconds()

	// A tab with nothing behind it does not fail, it simply does not move — and
	// history.back() on such a tab is silent, so the timeout IS the answer
	// rather than an error. Saying "there is nowhere back to" is a fact the
	// agent can act on; "the page did not finish loading" would send it looking
	// for a network problem that does not exist.
	if waitErr != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Success = true
		out.Content = "ย้อนกลับไม่ได้ ไม่มีหน้าก่อนหน้าในแท็บนี้" + before
		out.RawOutput = out.Content
		return out, nil
	}

	out.Success = true
	out.Content = "ย้อนกลับแล้ว" + a.browserWhere(id) + " refs จากการ read ก่อนหน้าใช้ไม่ได้แล้ว อ่านใหม่ก่อนคลิก"
	out.RawOutput = out.Content
	return out, nil
}
