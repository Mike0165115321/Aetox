package main

// Making a deck show everything it has, at the same time, before it is exported.
//
// **Why a deck that scrolled through correctly still printed one slide.** A deck
// built for the screen does not merely reveal content as you arrive — most of
// them un-reveal it as you leave. The real deck that exposed this does:
//
//	slides.forEach((el,i) => el.classList.toggle('visible', i === current))
//
// with `.reveal{opacity:0;transform:translateY(24px)}` underneath. Walking the
// page past every slide fires that code eight times and leaves exactly one slide
// wearing the class, so the export prints one slide with content and seven with
// their static chrome. Watching it happen looks like success right up to the
// moment the file is written.
//
// So scrolling is necessary and not sufficient. The missing half is making the
// revealed state STICK.
//
// **The way that was not taken** is to force the classes: `.reveal`, `.visible`,
// `.in-view`, `.active`, `.fade-in`. It works on decks that happen to use those
// names and silently fails on every other one, which is the same failure wearing
// a longer list.
//
// **The way taken** is to read the answer instead of guessing it. While a slide
// is on screen and revealed, its descendants are pinned to whatever `opacity`
// and `transform` they have computed to *at that moment*, as inline styles.
// Inline `!important` outranks the class the deck is about to remove, so the
// slide keeps the appearance it had when it was current — whatever mechanism
// produced it, and whatever the author named it.

import (
	"encoding/json"
	"fmt"
)

// settleCSS collapses animations and transitions so nothing is mid-flight when
// its final state is read.
//
// Not `animation:none`, which would drop an element that ends where a keyframe
// left it. A duration of 1ms snaps the same animation to its last frame, which
// is the state a reader would have seen a moment later anyway.
const settleCSS = `
*,*::before,*::after{
  animation-duration:1ms!important;animation-delay:0s!important;
  transition-duration:1ms!important;transition-delay:0s!important;
  scroll-behavior:auto!important}
`

// revealScript walks past every slide and pins what it finds there.
//
// The pause is what makes it work at all: the deck's scroll handler and the
// transition it starts both run on later tasks, and a loop that scrolled eight
// times in one go would finish before the first one fired. 160ms is comfortably
// past a transition the stylesheet above has already shortened to nothing.
//
// Only two properties are pinned. They are the two a reveal animates, and
// pinning more would freeze design that was never animated — a slide whose
// layout depends on a class the deck sets for other reasons would come out
// wrong in a way nobody could see the cause of.
const revealScript = `(async()=>{` +
	`const id='__aetox-export-settle';document.getElementById(id)?.remove();` +
	`const s=document.createElement('style');s.id=id;s.textContent=` + "`" + settleCSS + "`" + `;` +
	`document.head.appendChild(s);` +
	`const wait=ms=>new Promise(r=>setTimeout(r,ms));` +
	`const slides=[...document.querySelectorAll('section.slide')];` +
	`let pinned=0;` +
	`for(const el of slides){` +
	`el.scrollIntoView({block:'center',behavior:'auto'});` +
	`await wait(160);` +
	`for(const d of [el,...el.querySelectorAll('*')]){` +
	`const cs=getComputedStyle(d);` +
	`d.style.setProperty('opacity',cs.opacity,'important');` +
	`d.style.setProperty('transform',cs.transform==='none'?'none':cs.transform,'important');` +
	`d.style.setProperty('filter',cs.filter==='none'?'none':cs.filter,'important');` +
	`pinned++}}` +
	`window.scrollTo(0,0);await wait(160);` +
	`return pinned})()`

// revealEverything scrolls the deck past every slide, letting each one's own
// reveal code run, and pins the result so it survives the deck un-revealing it.
//
// Returns how many elements were pinned, which is only useful as a sign of life:
// zero means the walk found no slides at all, and the caller has a better error
// for that than this does.
func revealEverything(call engineCaller) (int, error) {
	raw, err := call("Runtime.evaluate",
		`{"expression":`+mustJSONString(revealScript)+`,"awaitPromise":true,"returnByValue":true}`)
	if err != nil {
		return 0, err
	}
	var answer struct {
		Result struct {
			Value float64 `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return 0, fmt.Errorf("เดินผ่านสไลด์เพื่อให้เนื้อหาโผล่ไม่สำเร็จ: %w", err)
	}
	if answer.ExceptionDetails != nil {
		return 0, fmt.Errorf("เดินผ่านสไลด์เพื่อให้เนื้อหาโผล่ไม่สำเร็จ: %s", answer.ExceptionDetails.Text)
	}
	return int(answer.Result.Value), nil
}
