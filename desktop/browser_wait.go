package main

// Waiting for the page to catch up, and hearing what it says while you do.
//
// These two arrived together because they are the same defect seen twice: the
// page is doing something and the agent has no way to know. One is the page
// being slow, the other is the page asking a question. Both used to end as a
// timeout and a sentence about the network.
//
// ## wait
//
// `open` waits for a navigation to complete, and for a page that renders on the
// server that is the whole story. For everything built in this decade it is the
// beginning of one: the document arrives, then scripts fetch the actual content
// and put it in. An agent that reads immediately reads an empty shell, and — the
// part that makes it a real defect rather than a slow path — it reads it
// SUCCESSFULLY. There is nothing in the answer to suggest waiting; the page
// genuinely has no results in it yet, so the model concludes there are none.
//
// Waiting is therefore not a convenience. It is the difference between "no
// results" and "not yet", which nothing else in this tool can tell apart.
//
// The condition is text rather than a CSS selector on purpose. A model reliably
// knows what it expects to SEE — a name, a total, the word "Results" — and
// unreliably knows what the page calls its own divs.
//
// ## dialog
//
// `alert()`, `confirm()` and `prompt()` stop a page dead until somebody answers.
// Nobody could: the agent has no hands for a native dialog, so a page that
// raised one hung every later action until its timeout and reported that the
// page was not responding. True, and useless.
//
// The overrides below mean a dialog can no longer block anything. The page gets
// an answer immediately, from a standing policy the agent sets, and what was
// said is recorded so the next answer the agent receives can mention it.
//
// **The default is dismiss**, and that is a safety position rather than a
// convenience one: a `confirm()` sitting in front of a destructive action is the
// commonest kind there is, and answering yes by default would make the browser
// tool quietly agree to things on the user's behalf. Saying yes has to be a
// thing the agent chose, one dialog at a time.
//
// What a dialog says is quoted back and never obeyed. It is text from a page,
// which this subsystem has treated as untrusted since browser-security-2026-07-21.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/statereport"
)

// waitScript polls for text and answers exactly once, either when it turns up or
// when its own deadline passes. The deadline lives in the page as well as in Go
// so a script whose tab is abandoned stops on its own instead of polling for the
// life of the document.
func waitScript(token, needle string, ms int) string {
	tok, _ := json.Marshal(token)
	want, _ := json.Marshal(needle)
	return fmt.Sprintf(`(function(){
  var fired=false,iv=null,deadline=Date.now()+%d;
  function done(ok){
    if(fired)return; fired=true; if(iv)clearInterval(iv);
    %s(JSON.stringify({__aetox:"wait",token:%s,url:location.href,found:ok}));
  }
  function check(){
    var body=document.body?(document.body.innerText||""):"";
    if(body.indexOf(%s)>=0){done(true);return}
    if(Date.now()>deadline)done(false);
  }
  check();
  if(!fired)iv=setInterval(check,200);
})()`, ms, bridgePost, tok, want)
}

// dialogScript replaces the three blocking dialogs. Installed at document
// creation so it is in place before any page script can call one.
//
// window.__aetox_dlg is the standing policy, and living in the page is what
// makes it survive into every frame and every same-document navigation. It is
// reset on each new document, which is the right default: an accept the agent
// granted for one page should not still be granted three pages later.
func dialogScript() string {
	return `(function(){
  if(window.__aetox_dlg)return;
  window.__aetox_dlg={accept:false,text:null};
  function report(kind,msg,answer){
    try{` + bridgePost + `(JSON.stringify({__aetox:"dialog",url:location.href,dialog:kind,message:String(msg==null?"":msg).slice(0,300),answer:answer}));}catch(e){}
  }
  window.alert=function(m){report("alert",m,"ok");};
  window.confirm=function(m){var a=!!window.__aetox_dlg.accept;report("confirm",m,a?"ok":"cancel");return a;};
  window.prompt=function(m,d){
    if(!window.__aetox_dlg.accept){report("prompt",m,"cancel");return null;}
    var v=window.__aetox_dlg.text;
    if(v===null||v===undefined)v=(d===undefined?"":d);
    report("prompt",m,String(v));
    return String(v);
  };
})()`
}

// waitForText blocks until the page contains needle, or gives up.
func (a *App) waitForText(ctx context.Context, id AgentTabID, needle string, timeout time.Duration) (bool, error) {
	host := a.browsers
	if host == nil {
		return false, fmt.Errorf("no browser")
	}
	t := host.tab(string(id))
	if t == nil {
		return false, fmt.Errorf("no browser tab %q", id)
	}

	token := newMessageToken()
	ch := make(chan bool, 1)
	t.waitMu.Lock()
	t.waitCh, t.waitToken = ch, token
	t.waitMu.Unlock()

	host.onTab(string(id), func(v tabView, _ *browserTab) {
		v.eval(waitScript(token, needle, int(timeout/time.Millisecond)))
	})

	select {
	case found := <-ch:
		return found, nil
	case <-ctx.Done():
		return false, ctx.Err()
	// Slack over the script's own deadline: the page may navigate mid-wait, and
	// a script that went with its document will never answer.
	case <-time.After(timeout + 3*time.Second):
		t.waitMu.Lock()
		t.waitCh, t.waitToken = nil, ""
		t.waitMu.Unlock()
		return false, statereport.New("the page stopped answering while waiting (did it navigate?)")
	}
}

type browserWaitSkill struct{ app *App }

const (
	waitDefault = 10 * time.Second
	waitMax     = 60 * time.Second
)

func (s *browserWaitSkill) wait(ctx context.Context, text string, seconds int) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "browser_wait", Command: "browser wait " + text}
	a := s.app

	text = strings.TrimSpace(text)
	if text == "" {
		err := fmt.Errorf("wait needs the text you expect to appear on the page")
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}
	timeout := waitDefault
	if seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
		if timeout > waitMax {
			timeout = waitMax
		}
	}

	id, err := a.agentTab()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}

	found, err := a.waitForText(ctx, id, text, timeout)
	out.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}
	out.Success = true
	if found {
		out.Content = fmt.Sprintf("เจอ %q แล้วใน %.1f วินาที", text, time.Since(start).Seconds()) + a.dialogNote(id)
	} else {
		// Not an error. The page may simply not be going to say it, and that is
		// a fact about the page rather than a fault in the call — the same
		// distinction §128.4 drew between weather and a defect. What the answer
		// must not do is stay silent about the difference, because "still not
		// there after 10s" and "there are no results" lead somewhere different.
		out.Content = fmt.Sprintf("รอ %.0f วินาทีแล้วยังไม่เจอ %q บนหน้านี้ อาจยังโหลดไม่เสร็จ หรือหน้านี้ไม่มีสิ่งนั้นจริง ๆ อ่านหน้าดูก่อนตัดสิน",
			timeout.Seconds(), text) + a.dialogNote(id)
	}
	out.RawOutput = out.Content
	return out, nil
}

type browserDialogSkill struct{ app *App }

// dialog sets what the page's next alert/confirm/prompt will be answered with,
// and reports anything already said.
func (s *browserDialogSkill) dialog(accept bool, text string) (skill.Output, error) {
	out := skill.Output{Name: "browser_dialog", Command: fmt.Sprintf("browser dialog accept=%v", accept)}
	a := s.app

	id, err := a.agentTab()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		return out, err
	}

	answer, _ := json.Marshal(text)
	js := fmt.Sprintf("window.__aetox_dlg={accept:%v,text:%s}", accept, answer)
	if text == "" {
		js = fmt.Sprintf("window.__aetox_dlg={accept:%v,text:null}", accept)
	}
	a.onTab(string(id), func(v tabView, _ *browserTab) { v.eval(js) })

	out.Success = true
	if accept {
		out.Content = "กล่องข้อความถัดไปบนหน้านี้จะถูกตอบว่าตกลง"
		if text != "" {
			out.Content += fmt.Sprintf(" และ prompt จะกรอกว่า %q", text)
		}
	} else {
		out.Content = "กล่องข้อความถัดไปบนหน้านี้จะถูกตอบว่ายกเลิก"
	}
	out.Content += " ตั้งค่านี้อยู่กับหน้านี้ เปลี่ยนหน้าแล้วกลับเป็นยกเลิกเหมือนเดิม"
	out.Content += a.dialogNote(id)
	out.RawOutput = out.Content
	return out, nil
}
