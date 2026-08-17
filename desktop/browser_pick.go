package main

// "ชี้ให้เอเจนดู" — pointing at the page instead of describing it.
//
// The agent can already read a page (browser_read) and act on it. What it could
// not do is hear "ปุ่มสีฟ้ามุมขวาบนอะ" and know which node that is. This is the
// other direction of the same bridge: the user points, and what lands in the
// composer is the node — its selector, its box, the colours it actually renders
// in, and the markup around it.
//
// Everything here is drawn INSIDE the page, and that is not a style choice. A
// browser tab is a real OS window composited above the app's own webview
// (browser.go), so a highlight box drawn in Svelte would be behind the thing it
// is trying to outline. The overlay therefore ships as part of the injected
// script, and its colours and its wording arrive from the frontend as `opts` —
// otherwise the app would own a second palette and a second copy of every
// string, which is the debt this project calls "a second place answering the
// same question".
//
// The security model is the one textScript already established, for the same
// reason: any page can call the bridge with any envelope it likes. A pick is
// accepted only when the sending frame's real origin matches the URL claimed,
// AND the token matches the one this specific BrowserStartPick minted. Without
// the token a page could push a pick nobody asked for into the composer; with
// it, the worst a page can do is lie about its own DOM, which it can do anyway.

import (
	"fmt"
)

// browserPick is one thing the user pointed at.
//
// It carries more than the ref browser_read hands out, because the two answer
// different questions. A ref answers "which node do I click"; a pick answers
// "which node is this in my source" — and the answer to that is rarely the tag
// name. It is the class list, the box, and the colour the user is looking at:
// a rendered #185fa5 greps straight to the token that produced it, where
// `<button>` greps to nothing.
type browserPick struct {
	Selector   string `json:"selector"`
	Tag        string `json:"tag"`
	Text       string `json:"text"`
	HTML       string `json:"html"`
	Path       string `json:"path"`
	W          int    `json:"w"`
	H          int    `json:"h"`
	Color      string `json:"color"`
	Background string `json:"background"`
}

// pickScript arms point-at-the-page mode on the tab's current document.
//
// opts is a JSON object built by the frontend — accent/panel/text/border
// colours from the live theme, plus the hint wording. It is embedded as a JS
// string literal and JSON.parse'd inside, never interpolated as code, so a
// broken or hostile opts can only fail to parse (the defaults below take over)
// rather than run.
//
// Pointer events drive everything, not mouse events: preventDefault on
// pointerdown is what stops the page selecting text under the drag, and it also
// suppresses the compatibility mousedown/mouseup that would otherwise arrive
// second. Every page-facing event is eaten in the capture phase, so a link
// under the cursor is a target, not a navigation.
func pickScript(token, opts string) string {
	return fmt.Sprintf(`(function(){
  var TOK=%q, O={};
  try{O=JSON.parse(%q)}catch(e){}
  var ACC=O.accent||"#378add";
  var HINT=O.hint||"", UNIT=O.unit||"", MODE=O.mode==="draw"?"draw":"pick";
  if(window.__aetoxPick){window.__aetoxPick.stop(true)}

  var picks=[], down=null, moved=false, strokes=[], stroke=null, done=false;
  var prevCursor=document.documentElement.style.cursor;
  var prevSelect=document.documentElement.style.userSelect;
  document.documentElement.style.cursor="crosshair";
  document.documentElement.style.userSelect="none";

  function mk(css){
    var d=document.createElement("div");
    d.__aetoxOverlay=1;
    d.style.cssText="position:fixed;z-index:2147483600;pointer-events:none;box-sizing:border-box;margin:0;padding:0;"+css;
    document.documentElement.appendChild(d);
    return d;
  }
  var box=mk("border:2px solid "+ACC+";border-radius:4px;display:none");
  var tag=mk("font:11px/1.5 ui-monospace,SFMono-Regular,monospace;color:#fff;background:"+ACC+";padding:2px 7px;border-radius:4px;white-space:nowrap;display:none");
  var band=mk("border:1px dashed "+ACC+";border-radius:4px;display:none");
  /* The bar carries its own colours instead of the app's. It is not drawn in
     the app — it is drawn on somebody else's page, whose background nobody
     knows, and a panel colour borrowed from a light theme is a pale box on a
     white page: invisible exactly where it is needed. Only the accent comes
     from the theme, because the accent is the ink, and the button that ends the
     drawing should be the colour of what was drawn. */
  var BG="#1c1c1e", EDGE="#3a3a3d", FG="#f2f2f2", DIM="#a5a5a8";
  var pill=mk("left:50%%;transform:translateX(-50%%);bottom:16px;background:"+BG+";color:"+FG+";border:1px solid "+EDGE+";border-radius:26px;padding:11px 12px 11px 18px;font:14px/1.4 system-ui,sans-serif;white-space:nowrap;display:flex;gap:12px;align-items:center;box-shadow:0 10px 30px rgba(0,0,0,.6)");
  var says=document.createElement("span");
  says.style.cssText="display:inline-flex;align-items:center;gap:7px;color:"+DIM+";font-size:13px";
  var saysText=document.createElement("span");
  says.appendChild(saysText);
  pill.appendChild(says);

  var NS="http://www.w3.org/2000/svg";
  /* Built node by node rather than through innerHTML: a page may carry a
     Trusted Types policy, and there an innerHTML assignment throws — taking the
     overlay down on exactly the sites most worth marking up. */
  function icon(paths,size){
    var s=document.createElementNS(NS,"svg"),i,p;
    s.setAttribute("viewBox","0 0 24 24");s.setAttribute("width",size);s.setAttribute("height",size);
    s.setAttribute("fill","none");s.setAttribute("stroke","currentColor");
    s.setAttribute("stroke-width","2");s.setAttribute("stroke-linecap","round");s.setAttribute("stroke-linejoin","round");
    s.style.cssText="flex:none";
    for(i=0;i<paths.length;i++){p=document.createElementNS(NS,"path");p.setAttribute("d",paths[i]);s.appendChild(p)}
    return s;
  }
  var PENCIL=["M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z","m15 5 4 4"];
  var CHECK=["M20 6 9 17l-5-5"];
  function rgba(c,a){
    var m=/^#([0-9a-f]{6})$/i.exec(c||""),r,g,b,n;
    if(m){n=parseInt(m[1],16);r=(n>>16)&255;g=(n>>8)&255;b=n&255}
    else{m=/^rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(c||"");if(!m)return c;r=+m[1];g=+m[2];b=+m[3]}
    return "rgba("+r+","+g+","+b+","+a+")";
  }

  /* The ink layer. A canvas rather than SVG paths because what is drawn here is
     never read back as shapes — it is drawn to be photographed, and the photo
     is taken by the engine a moment later with this canvas still on the page. */
  var ink=null,ctx=null,DPR=window.devicePixelRatio||1;
  var doneBtn=null,clearBtn=null,glowed=false;
  if(MODE==="draw"){
    ink=document.createElement("canvas");
    ink.__aetoxOverlay=1;
    ink.width=Math.round(window.innerWidth*DPR);
    ink.height=Math.round(window.innerHeight*DPR);
    ink.style.cssText="position:fixed;left:0;top:0;width:100%%;height:100%%;z-index:2147483599;pointer-events:none;margin:0;padding:0";
    document.documentElement.appendChild(ink);
    ctx=ink.getContext("2d");
    ctx.scale(DPR,DPR);
    ctx.strokeStyle=ACC;ctx.lineWidth=3;ctx.lineCap="round";ctx.lineJoin="round";
    /* The pill is the only way out of a mode with no click target, so in draw
       mode it stops being a caption and becomes controls. */
    says.insertBefore(icon(PENCIL,15),saysText);
    pill.style.pointerEvents="auto";
    pill.appendChild(sep());
    doneBtn=button(O.doneLabel||"OK",CHECK,function(){finish()});
    doneBtn.style.padding="7px 18px";
    doneBtn.style.fontWeight="500";
    clearBtn=button(O.clearLabel||"Clear",null,function(){
      strokes=[];stroke=null;ctx.clearRect(0,0,ink.width,ink.height);say();
    });
    button(O.cancelLabel||"Cancel",null,function(){stop(true)});
  }
  say();

  function sep(){
    var s=document.createElement("span");
    s.style.cssText="width:1px;height:20px;background:"+EDGE+";flex:none";
    return s;
  }
  function button(label,paths,onClick){
    var b=document.createElement("button"),t=document.createElement("span");
    b.__aetoxUI=1;
    b.style.cssText="all:unset;cursor:pointer;display:inline-flex;align-items:center;gap:6px;padding:7px 15px;border-radius:16px;border:1px solid "+EDGE+";color:"+FG+";font:14px/1.4 system-ui,sans-serif;transition:background .15s,box-shadow .15s,opacity .15s";
    if(paths)b.appendChild(icon(paths,15));
    t.textContent=label;b.appendChild(t);
    b.addEventListener("click",function(e){e.stopPropagation();onClick()});
    pill.appendChild(b);
    return b;
  }
  function isUI(n){ while(n){ if(n.__aetoxUI)return true; n=n.parentNode } return false }

  /* The bar reports state rather than repeating instructions: a hint is for
     before you know what to do, and once there is ink on the page the thing
     worth saying is that there is something to send. The button that sends it
     lights up in the colour of the ink at the same moment, so "there is
     something here now" is said once, in two places, rather than left to be
     inferred from a button that looked identical when it did nothing. */
  function paintDone(armed){
    if(!doneBtn)return;
    doneBtn.style.background=armed?ACC:"transparent";
    doneBtn.style.borderColor=armed?ACC:EDGE;
    doneBtn.style.color=armed?"#fff":FG;
    doneBtn.style.opacity=armed?"1":".55";
    doneBtn.style.boxShadow=armed?("0 0 0 4px "+rgba(ACC,".28")+",0 0 18px 2px "+rgba(ACC,".55")):"none";
    if(clearBtn)clearBtn.style.opacity=armed?"1":".55";
    if(!armed){glowed=false;return}
    if(!glowed){
      glowed=true;
      if(doneBtn.animate)doneBtn.animate([{transform:"scale(1)"},{transform:"scale(1.06)"},{transform:"scale(1)"}],{duration:320,easing:"ease-out"});
    }
  }
  function say(){
    if(MODE==="draw"){
      saysText.textContent=strokes.length?strokes.length+" "+(O.markUnit||""):HINT;
      paintDone(strokes.length>0);
      return;
    }
    saysText.textContent=picks.length?HINT+"  ·  "+picks.length+" "+UNIT:HINT;
  }

  function hex(c){
    var m=/^rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(c||"");
    if(!m)return c||"";
    function h(n){var s=(+n).toString(16);return s.length<2?"0"+s:s}
    return "#"+h(m[1])+h(m[2])+h(m[3]);
  }
  function sel(n){
    if(!n||!n.tagName)return "";
    if(n.id)return "#"+n.id;
    var s=n.tagName.toLowerCase();
    var cls=(n.getAttribute("class")||"").split(/\s+/).filter(function(c){return c&&c.length<40}).slice(0,3);
    if(cls.length)s+="."+cls.join(".");
    var p=n.parentElement,same=[],i;
    if(p){
      for(i=0;i<p.children.length;i++)if(p.children[i].tagName===n.tagName)same.push(p.children[i]);
      if(same.length>1)s+=":nth-of-type("+(same.indexOf(n)+1)+")";
    }
    return s;
  }
  function path(n){
    var out=[],p=n.parentElement,i=0;
    while(p&&p.tagName&&p!==document.documentElement&&i<3){out.unshift(sel(p));p=p.parentElement;i++}
    return out.join(" > ");
  }
  function info(n){
    var r=n.getBoundingClientRect(),cs=getComputedStyle(n),html=n.outerHTML||"";
    if(html.length>400)html=html.slice(0,400)+"…";
    return {
      selector:sel(n),tag:n.tagName.toLowerCase(),
      text:(n.innerText||n.value||n.getAttribute("aria-label")||"").trim().replace(/\s+/g," ").slice(0,120),
      html:html,path:path(n),
      w:Math.round(r.width),h:Math.round(r.height),
      color:hex(cs.color),background:hex(cs.backgroundColor)
    };
  }
  function paint(n){
    if(!n){box.style.display="none";tag.style.display="none";return}
    var r=n.getBoundingClientRect();
    box.style.display="block";box.style.left=r.left+"px";box.style.top=r.top+"px";
    box.style.width=r.width+"px";box.style.height=r.height+"px";
    tag.style.display="block";
    tag.textContent=sel(n)+" · "+Math.round(r.width)+"×"+Math.round(r.height);
    var ty=r.top-22; if(ty<2)ty=r.bottom+4;
    tag.style.left=Math.max(2,r.left)+"px";tag.style.top=ty+"px";
  }
  function at(e){
    var n=document.elementFromPoint(e.clientX,e.clientY);
    while(n&&n.__aetoxOverlay)n=n.parentElement;
    if(!n||n===document.documentElement||n===document.body)return null;
    return n;
  }
  /* A dragged box takes what it FULLY encloses, outermost first: the point of
     circling three cards is the three cards, not the ninety nodes inside them. */
  function inRegion(x0,y0,x1,y1){
    var all=document.querySelectorAll("body *"),cand=[],touch=[],out=[],i,n,r;
    for(i=0;i<all.length&&cand.length<400;i++){
      n=all[i];
      if(n.__aetoxOverlay)continue;
      r=n.getBoundingClientRect();
      if(r.width<=0||r.height<=0)continue;
      if(r.right<x0||r.left>x1||r.bottom<y0||r.top>y1)continue;
      /* Overlapping is remembered separately: a box drawn inside one big
         element encloses nothing, and coming back with nothing is how a drag
         across the middle of a page looked like a feature that did not work. */
      if(touch.length<400)touch.push(n);
      if(r.left<x0-1||r.top<y0-1||r.right>x1+1||r.bottom>y1+1)continue;
      cand.push(n);
    }
    if(cand.length===0)cand=touch;
    for(i=0;i<cand.length&&out.length<12;i++){
      n=cand[i];
      var nested=false,p=n.parentElement;
      while(p){if(cand.indexOf(p)>=0){nested=true;break}p=p.parentElement}
      if(!nested)out.push(n);
    }
    return out;
  }

  /* The topmost real element under each mark. Sampled along the strokes rather
     than derived from their bounding box: what "I drew on this" means is what
     was under the pen, and a box round a curve is mostly the things it missed. */
  function underInk(){
    var out=[],seen=[],s,i,j,n;
    for(i=0;i<strokes.length;i++){
      s=strokes[i];
      for(j=0;j<s.length&&out.length<6;j+=4){
        n=document.elementFromPoint(s[j].x,s[j].y);
        while(n&&n.__aetoxOverlay)n=n.parentElement;
        if(!n||n===document.documentElement||n===document.body)continue;
        if(seen.indexOf(n)>=0)continue;
        seen.push(n);out.push(info(n));
      }
    }
    return out;
  }

  function eat(e){
    if(isUI(e.target))return;
    e.preventDefault();e.stopPropagation();if(e.stopImmediatePropagation)e.stopImmediatePropagation();
  }
  function onMove(e){
    if(MODE==="draw"){
      if(!stroke)return;
      eat(e);
      var last=stroke[stroke.length-1];
      stroke.push({x:e.clientX,y:e.clientY});
      ctx.beginPath();ctx.moveTo(last.x,last.y);ctx.lineTo(e.clientX,e.clientY);ctx.stroke();
      return;
    }
    if(down){
      var dx=e.clientX-down.x,dy=e.clientY-down.y;
      if(!moved&&(Math.abs(dx)>6||Math.abs(dy)>6))moved=true;
      if(moved){
        box.style.display="none";tag.style.display="none";band.style.display="block";
        band.style.left=Math.min(down.x,e.clientX)+"px";band.style.top=Math.min(down.y,e.clientY)+"px";
        band.style.width=Math.abs(dx)+"px";band.style.height=Math.abs(dy)+"px";
      }
      return;
    }
    paint(at(e));
  }
  function onDown(e){
    if(isUI(e.target))return;
    eat(e);
    if(MODE==="draw"){stroke=[{x:e.clientX,y:e.clientY}];return}
    down={x:e.clientX,y:e.clientY};moved=false;
  }
  function onUp(e){
    if(isUI(e.target))return;
    eat(e);
    if(MODE==="draw"){
      if(stroke&&stroke.length>1)strokes.push(stroke);
      stroke=null;
      say();
      return;
    }
    var wasDrag=moved,d=down,i;
    down=null;moved=false;band.style.display="none";
    if(wasDrag&&d){
      var els=inRegion(Math.min(d.x,e.clientX),Math.min(d.y,e.clientY),Math.max(d.x,e.clientX),Math.max(d.y,e.clientY));
      for(i=0;i<els.length;i++)picks.push(info(els[i]));
    }else{
      var n=at(e);
      if(n)picks.push(info(n));
    }
    if(e.shiftKey&&picks.length){say();paint(at(e));return}
    stop(false);
  }
  function onKey(e){
    if(e.key==="Escape"){eat(e);stop(true)}
    else if(e.key==="Enter"){eat(e);MODE==="draw"?finish():stop(picks.length===0)}
  }

  var WIRED=[["pointermove",onMove],["pointerdown",onDown],["pointerup",onUp],["keydown",onKey],
             ["click",eat],["dblclick",eat],["auxclick",eat],["contextmenu",eat],
             ["mousedown",eat],["mouseup",eat],["dragstart",eat],["submit",eat]];
  for(var w=0;w<WIRED.length;w++)document.addEventListener(WIRED[w][0],WIRED[w][1],true);

  function teardown(keepInk){
    for(var w=0;w<WIRED.length;w++)document.removeEventListener(WIRED[w][0],WIRED[w][1],true);
    var kill=[box,tag,band,pill];
    if(ink&&!keepInk)kill.push(ink);
    for(var k=0;k<kill.length;k++)if(kill[k].parentNode)kill[k].parentNode.removeChild(kill[k]);
    document.documentElement.style.cursor=prevCursor;
    document.documentElement.style.userSelect=prevSelect;
  }
  function post(cancelled,drawn){
    %s(JSON.stringify({__aetox:"pick",token:TOK,url:location.href,cancelled:!!cancelled,drawn:!!drawn,picks:cancelled?[]:picks}));
  }
  function stop(cancelled){
    /* After a finish only the ink is left standing, waiting to be photographed.
       Stopping then means taking it down, and saying nothing: the answer has
       already been sent, and a second one would attach the same marks twice. */
    if(done){
      if(ink&&ink.parentNode)ink.parentNode.removeChild(ink);
      window.__aetoxPick=null;
      return;
    }
    teardown(false);
    window.__aetoxPick=null;
    post(!!cancelled,false);
  }
  /* Drawing ends by leaving the marks up. The picture is taken from outside,
     against this page, a moment from now — so the controls go and the ink
     stays, and the app takes the ink down once it has what it came for. */
  function finish(){
    if(MODE!=="draw"){stop(picks.length===0);return}
    if(stroke&&stroke.length>1)strokes.push(stroke);
    stroke=null;
    picks=underInk();
    done=true;
    teardown(true);
    post(false,true);
  }
  window.__aetoxPick={stop:stop};
})()`, token, opts, bridgePost)
}

// stopPickScript ends a mode the user turned off from the toolbar. Written to
// be harmless on a page that never had it (a navigation drops the whole thing),
// because the frontend cannot know which of those two it is looking at.
const stopPickScript = `window.__aetoxPick&&window.__aetoxPick.stop(true)`

// armPick mints the token this round of picking will be answered with, and
// hands it to the caller to embed in the script.
func (t *browserTab) armPick() string {
	token := newMessageToken()
	t.pickMu.Lock()
	t.pickToken = token
	t.pickMu.Unlock()
	return token
}

// claimPick consumes the pending pick token if it is the one offered. False
// means this message answers no request of ours — a stale round, or a page
// posting a pick nobody asked for.
func (t *browserTab) claimPick(token string) bool {
	t.pickMu.Lock()
	defer t.pickMu.Unlock()
	if token == "" || token != t.pickToken {
		return false
	}
	t.pickToken = ""
	return true
}

// BrowserStartPick turns on point-at-the-page mode for a tab. opts is the
// frontend's JSON: the theme colours and the hint wording the in-page overlay
// draws itself with (see pickScript).
//
// The answer does not come back here — it arrives as a browser:pick:<id> event
// once the user has actually pointed at something, which may be never.
func (a *App) BrowserStartPick(id, opts string) error {
	host, err := a.browserHostLazy()
	if err != nil {
		return err
	}
	t := host.tab(id)
	if t == nil {
		return fmt.Errorf("no browser tab %q", id)
	}
	token := t.armPick()
	host.onTab(id, func(v tabView, _ *browserTab) { v.eval(pickScript(token, opts)) })
	return nil
}

// BrowserStopPick turns the mode off from the app side — the toolbar button
// pressed again, the tab closing, a navigation landing under a live mode.
func (a *App) BrowserStopPick(id string) {
	a.onTab(id, func(v tabView, t *browserTab) {
		t.pickMu.Lock()
		t.pickToken = ""
		t.pickMu.Unlock()
		v.eval(stopPickScript)
	})
}
