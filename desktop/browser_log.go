package main

// What the page said to itself, and who it called.
//
// `read` answers what a page shows. Nothing answered why a page shows nothing —
// the uncaught TypeError three frames deep, the fetch that came back 401, the
// script the CSP refused to run. The agent saw a blank page and could report
// only that it was blank, which is true and useless, and is exactly the state
// anyone debugging their own app is in when they ask.
//
// **Recorded in the page, not through the engine.** CDP would give a fuller
// answer — every subresource, every browser-generated error — and it would give
// it on Windows only: callEngine is Chrome DevTools Protocol
// (browser_windows.go), and a WebKit host would have to grow its own event
// plumbing before console.log worked at all there. These recorders are ordinary
// JavaScript installed at document creation, the same way dialogScript replaces
// window.alert, so they arrive on every engine the moment that engine can run a
// page. What they cost is stated in the guidance rather than hidden: they see
// what page code does, not what the browser does on its behalf.
//
// The buffer lives in the page too, and is read on demand in one round trip
// rather than streamed. That keeps a chatty page from turning every console.log
// into a message across the bridge, and it makes the lifetime obvious: a new
// document is a new buffer, which is the right answer, because the console of
// the page you left is not evidence about the page you are on.
//
// The page can lie about all of it. So can its text, and the answer is the one
// browser.go's bridge header already gives: what comes off a page is untrusted
// data, and the transport is not where that gets fixed.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// browserLogEntry is one console line or one request. The two shapes share a
// struct because they share a transport and a buffer; every field is optional
// and each kind fills its own.
type browserLogEntry struct {
	Level  string `json:"level,omitempty"`  // console: log|info|warn|error|debug
	Text   string `json:"text,omitempty"`   // console: the message
	Method string `json:"method,omitempty"` // network: GET, POST, ...
	URL    string `json:"url,omitempty"`    // network: the address, secrets redacted
	Status int    `json:"status,omitempty"` // network: 0 means it never answered
	MS     int    `json:"ms,omitempty"`     // network: how long it took
	Error  string `json:"error,omitempty"`  // network: why it failed
	T      int    `json:"t,omitempty"`      // ms since the recorder was installed
}

// browserLogReport is one read of one buffer.
type browserLogReport struct {
	Kind    string
	Entries []browserLogEntry
	Dropped int
	// Armed is whether the recorder was running on this page at all, and it is
	// the field that keeps this tool honest. Without it an empty buffer and a
	// page the recorder never reached give the same answer — "nothing here" —
	// and only one of those is a fact about the page.
	Armed bool
}

// browserLogCap is how many of each kind the page keeps. Old entries are
// dropped rather than new ones refused, because the interesting line on a page
// that has been running a while is almost always a recent one — and the count
// of what fell off is reported, so a full buffer never passes for a complete
// history.
const browserLogCap = 200

// logScript installs the recorders. It goes in at document creation, beside
// dialogScript, so it is in place before the page's first statement — an error
// thrown by a page's opening line is exactly the kind worth having.
func logScript() string {
	return `(function(){
  if(window.__aetox_log)return;
  var T0=Date.now(), CAP=` + fmt.Sprint(browserLogCap) + `;
  var L={console:[],network:[],dropped:{console:0,network:0}};
  window.__aetox_log=L;
  function push(k,item){
    var list=L[k];
    item.t=Date.now()-T0;
    list.push(item);
    if(list.length>CAP){list.shift();L.dropped[k]++;}
  }
  function say(v){
    try{
      if(v===null)return 'null';
      if(typeof v==='undefined')return 'undefined';
      if(typeof v==='string')return v;
      if(v instanceof Error)return (v.name||'Error')+': '+(v.message||'');
      if(typeof v==='object')return JSON.stringify(v);
      return String(v);
    }catch(e){return '[unprintable]';}
  }
  function args(a){
    var parts=[];
    for(var i=0;i<a.length&&i<8;i++)parts.push(say(a[i]));
    return parts.join(' ').slice(0,600);
  }
  var levels=['log','info','warn','error','debug'];
  for(var n=0;n<levels.length;n++){
    (function(k){
      var orig=console[k];
      if(typeof orig!=='function')return;
      console[k]=function(){
        try{push('console',{level:k,text:args(arguments)});}catch(e){}
        return orig.apply(console,arguments);
      };
    })(levels[n]);
  }
  /* The three a page never logs for itself, and the three most worth having. */
  window.addEventListener('error',function(e){
    try{
      /* Chromium's ErrorEvent.message already begins "Uncaught"; WebKit's does
         not. Prepending unconditionally reads "Uncaught Uncaught Error: ..." on
         the engine this ships on today, so the word is added only when it is
         missing. And a location with no file is noise: an error raised from an
         eval has no filename, and "(:10)" says less than nothing. */
      var msg=say(e.message);
      if(msg.indexOf('Uncaught')!==0)msg='Uncaught '+msg;
      var at=e.filename?(' ('+e.filename+':'+(e.lineno||0)+')'):'';
      push('console',{level:'error',text:msg+at});
    }catch(x){}
  },true);
  window.addEventListener('unhandledrejection',function(e){
    try{push('console',{level:'error',text:'Unhandled promise rejection: '+say(e.reason)});}catch(x){}
  });
  window.addEventListener('securitypolicyviolation',function(e){
    try{push('console',{level:'error',text:'CSP blocked '+(e.blockedURI||'')+' ('+(e.violatedDirective||'')+')'});}catch(x){}
  });
  /* A URL is the one thing here that can carry a secret, because plenty of
     services still put one in a query string. Headers and bodies are never
     read at all, which is why there is no rule about them. */
  function safeURL(u){
    try{
      return String(u).replace(/([?&][^=&]*(?:token|key|secret|password|passwd|auth|session|sig|credential)[^=&]*=)[^&#]*/gi,'$1<redacted>').slice(0,300);
    }catch(e){return '';}
  }
  var of=window.fetch;
  if(typeof of==='function'){
    window.fetch=function(input,init){
      var url=(input&&input.url)||input||'';
      var m=(init&&init.method)||(input&&input.method)||'GET';
      var t0=Date.now();
      function done(status,err){
        try{push('network',{method:String(m).toUpperCase(),url:safeURL(url),status:status,ms:Date.now()-t0,error:err||''});}catch(e){}
      }
      try{
        return of.apply(this,arguments).then(function(r){done(r.status,'');return r;},function(e){done(0,say(e));throw e;});
      }catch(e){done(0,say(e));throw e;}
    };
  }
  var xo=XMLHttpRequest.prototype.open, xs=XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open=function(m,u){
    try{this.__aetox={m:m,u:u};}catch(e){}
    return xo.apply(this,arguments);
  };
  XMLHttpRequest.prototype.send=function(){
    var s=this,i=s.__aetox||{},t0=Date.now();
    try{
      s.addEventListener('loadend',function(){
        try{push('network',{method:String(i.m||'GET').toUpperCase(),url:safeURL(i.u||''),status:s.status,ms:Date.now()-t0,error:s.status===0?'no response':''});}catch(e){}
      });
    }catch(e){}
    return xs.apply(this,arguments);
  };
})()`
}

// readLogScript reports one buffer back over the bridge.
func readLogScript(token, kind string) string {
	tok, _ := json.Marshal(token)
	k, _ := json.Marshal(kind)
	return fmt.Sprintf(`(function(){
  var L=window.__aetox_log;
  var kind=%s;
  var entries=[],dropped=0,armed=false;
  if(L){armed=true;entries=L[kind]||[];dropped=(L.dropped&&L.dropped[kind])||0;}
  %s(JSON.stringify({__aetox:"log",token:%s,url:location.href,kind:kind,log:entries,dropped:dropped,armed:armed}));
})()`, string(k), bridgePost, string(tok))
}

// browserLog reads one of a tab's buffers. Same round-trip shape as
// browserSnapshot, and every part of it is there for the same reason.
func (a *App) browserLog(id, kind string) (browserLogReport, error) {
	host, err := a.browserHostLazy()
	if err != nil {
		return browserLogReport{}, err
	}
	t := host.tab(id)
	if t == nil {
		return browserLogReport{}, fmt.Errorf("no browser tab %q", id)
	}

	token := newMessageToken()
	ch := make(chan browserLogReport, 1)
	t.logMu.Lock()
	t.logCh = ch
	t.logToken = token
	t.logMu.Unlock()

	// Blocks below, so do() must not: see hostBackend.do.
	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(readLogScript(token, kind)) })

	select {
	case rep := <-ch:
		return rep, nil
	case <-time.After(5 * time.Second):
		t.logMu.Lock()
		t.logCh = nil
		t.logToken = ""
		t.logMu.Unlock()
		return browserLogReport{}, fmt.Errorf("page did not respond (still loading?)")
	}
}

// browserLogSkill is both actions; kind decides which buffer.
type browserLogSkill struct {
	app  *App
	kind string // "console" or "network"
}

func (s *browserLogSkill) run(_ context.Context) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "browser_" + s.kind, Command: "browser " + s.kind}

	id, err := s.app.agentTab()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}
	rep, err := s.app.browserLog(string(id), s.kind)
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	var title, url string
	if t := s.app.browsers.tab(string(id)); t != nil {
		title, url = t.meta()
	}
	out.Success = true
	out.Content = formatBrowserLog(browserPageRef(title, url), rep)
	out.RawOutput = out.Content
	out.Truncated = rep.Dropped > 0
	out.DurationMs = time.Since(start).Milliseconds()
	return out, nil
}

// formatBrowserLog is what the model reads. Pure, for the same reason
// formatBrowserRead is: everything that produces it needs a live window, and
// nothing that checks it should.
func formatBrowserLog(where string, rep browserLogReport) string {
	if where == "" {
		where = "the open page"
	}
	var b strings.Builder

	// Said first when it applies, because every other line would otherwise be
	// read as evidence. A page the recorder never reached has an empty buffer
	// and a clean bill of health, and the two are indistinguishable.
	if !rep.Armed {
		fmt.Fprintf(&b, "The recorder is not running on %s, so this is not a report that nothing happened — it is no report at all.\n", where)
		b.WriteString("It is installed when a document is created, so a page that was already open before this build has none. Reload the page and ask again.\n")
		return b.String()
	}

	if len(rep.Entries) == 0 {
		if rep.Kind == "network" {
			fmt.Fprintf(&b, "No fetch or XMLHttpRequest calls from %s since it loaded.\n", where)
			b.WriteString("That is a fact about the page's own code. Images, scripts and stylesheets the browser fetched on its behalf are not counted here and never were.\n")
			return b.String()
		}
		fmt.Fprintf(&b, "Nothing on the console of %s since it loaded — no messages, no uncaught errors, no blocked resources.\n", where)
		return b.String()
	}

	if rep.Kind == "network" {
		fmt.Fprintf(&b, "Requests %s made since it loaded, oldest first:\n", where)
		for _, e := range rep.Entries {
			status := fmt.Sprint(e.Status)
			if e.Status == 0 {
				status = "-"
			}
			fmt.Fprintf(&b, "%s %s %dms %s", e.Method, status, e.MS, e.URL)
			if e.Error != "" {
				fmt.Fprintf(&b, "  (%s)", e.Error)
			}
			b.WriteString("\n")
		}
	} else {
		fmt.Fprintf(&b, "Console of %s since it loaded, oldest first:\n", where)
		for _, e := range rep.Entries {
			fmt.Fprintf(&b, "[%s] %s\n", e.Level, e.Text)
		}
	}

	if rep.Dropped > 0 {
		fmt.Fprintf(&b, "... %d earlier entries were dropped; the page keeps the last %d.\n", rep.Dropped, browserLogCap)
	}
	if rep.Kind == "network" {
		b.WriteString("Only what the page's own code asked for. Anything in a query string that looks like a credential is replaced with <redacted>, and no headers or bodies are read at all.\n")
	}
	return b.String()
}
