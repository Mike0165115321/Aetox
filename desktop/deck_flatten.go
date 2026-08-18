package main

// Laying a deck out flat before exporting it.
//
// A deck built for the screen may stack its slides on top of one another and
// show one at a time — `display:none`, `position:absolute; inset:0`, a
// `transform` carousel. That is a good way to build a deck and a fatal one to
// print, because the print pipeline lays out what is in flow and seven of the
// eight are not. The same fact breaks the picture exports less visibly: every
// slide measures to the same rectangle, and eight captures of one place come
// back looking like eight copies of slide one.
//
// **Every override here is conditional, and that is the whole design.** The
// first version of this file was a blanket stylesheet — `display:block
// !important` on every slide, and the rest. It fixed stacked decks and quietly
// broke every deck that was already fine: the real one that exposed it lays its
// slides out normally with `display:flex; justify-content:center`, and being
// forced to `block` dropped the vertical centring out of all eight. A deck that
// does not need flattening must come out of here untouched, so each property is
// read first and set only where it is actually wrong.

import (
	"encoding/json"
	"fmt"
)

// flattenScript puts stacked slides back into flow and reports where they
// landed. It touches `section.slide` and the ancestors that can trap it, and
// nothing else — the marker is the one thing every deck agrees on.
const flattenScript = `(()=>{` +
	`const slides=[...document.querySelectorAll('section.slide')];` +
	`if(!slides.length)return JSON.stringify({docHeight:0,rects:[]});` +

	// The scrollbar has to go before anything is measured, and it is not
	// cosmetic. A deck eight slides tall has a vertical scrollbar, which takes
	// ~15px off the viewport width; the slide then measures 1265 wide instead
	// of 1280, that number becomes the paper size, and printing at the narrower
	// width re-wraps every line so slides grow past one page and spill. Eight
	// slides came out as twelve pages, and the whole difference was one
	// scrollbar. Measured from a real export: MediaBox 948.96pt = 13.18in,
	// against the 13.333in the deck was designed at.
	`const bar=document.createElement('style');` +
	`bar.textContent='html{scrollbar-width:none!important}::-webkit-scrollbar{display:none!important}';` +
	`document.head.appendChild(bar);` +

	// A scrolling ancestor is the trap that looks like success: slides laid out
	// down an inner container measure to different positions, so nothing seems
	// wrong, while the DOCUMENT stays one viewport tall and printing clips
	// everything past the first screen. Unlocked before the slides, because the
	// slide measurements below are only meaningful once the window is the
	// thing that scrolls.
	`const up=new Set();` +
	`for(const el of slides)for(let p=el.parentElement;p;p=p.parentElement)up.add(p);` +
	`for(const p of up){const cs=getComputedStyle(p);` +
	`if(cs.overflow!=='visible')p.style.setProperty('overflow','visible','important');` +
	`if(cs.position==='fixed'||cs.position==='sticky')p.style.setProperty('position','static','important');` +
	`if(cs.height!=='auto')p.style.setProperty('height','auto','important');` +
	`if(cs.maxHeight!=='none')p.style.setProperty('max-height','none','important');}` +

	// Only what is actually wrong. `display` is never forced to block — a slide
	// laid out as flex is laid out the way its author meant, and the only thing
	// that needs changing is a slide that is not displayed at all.
	`for(const el of slides){const cs=getComputedStyle(el);` +
	`if(cs.display==='none')el.style.setProperty('display','block','important');` +
	`if(cs.position!=='static'&&cs.position!=='relative'){` +
	`el.style.setProperty('position','relative','important');` +
	`for(const side of ['top','right','bottom','left'])el.style.setProperty(side,'auto','important');}` +
	`if(cs.visibility!=='visible')el.style.setProperty('visibility','visible','important');` +
	`if(parseFloat(cs.opacity)===0)el.style.setProperty('opacity','1','important');` +
	`if(cs.float!=='none')el.style.setProperty('float','none','important');` +
	`el.style.setProperty('break-after','page','important');` +
	`el.style.setProperty('page-break-after','always','important');` +
	`el.style.setProperty('break-inside','avoid','important');}` +
	// The last slide must not push an empty page after it.
	`const last=slides[slides.length-1];` +
	`last.style.setProperty('break-after','auto','important');` +
	`last.style.setProperty('page-break-after','auto','important');` +

	// Every slide pinned to the first one's box, so the paper and the artwork
	// are the same rectangle and one slide is exactly one page. The contract
	// fixes a slide AS a box (1280x720), so pinning is what that means rather
	// than a liberty taken with it — and without it `min-height:100vh` re-reads
	// against the page in print and a slide can quietly become 1.05 pages.
	`const box=slides[0].getBoundingClientRect();` +
	`for(const el of slides){` +
	`el.style.setProperty('box-sizing','border-box','important');` +
	`el.style.setProperty('width',box.width+'px','important');` +
	`el.style.setProperty('height',box.height+'px','important');` +
	`el.style.setProperty('min-height','0','important');` +
	`el.style.setProperty('max-height',box.height+'px','important');` +
	`el.style.setProperty('overflow','hidden','important');}` +

	`window.scrollTo(0,0);` +
	`return JSON.stringify({` +
	`docHeight:document.documentElement.scrollHeight,` +
	`rects:slides.map(el=>{const r=el.getBoundingClientRect();` +
	`return {x:r.left+window.scrollX,y:r.top+window.scrollY,w:r.width,h:r.height}})})})()`

type flattenReport struct {
	DocHeight float64     `json:"docHeight"`
	Rects     []slideRect `json:"rects"`
}

// flattenForExport lays the deck out flat and returns one rectangle per slide,
// in document order.
func flattenForExport(call engineCaller) ([]slideRect, error) {
	raw, err := call("Runtime.evaluate", `{"expression":`+mustJSONString(flattenScript)+`,"returnByValue":true}`)
	if err != nil {
		return nil, err
	}
	var answer struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return nil, fmt.Errorf("จัดหน้าเด็คเพื่อส่งออกไม่สำเร็จ: %w", err)
	}
	if answer.ExceptionDetails != nil {
		return nil, fmt.Errorf("จัดหน้าเด็คเพื่อส่งออกไม่สำเร็จ: %s", answer.ExceptionDetails.Text)
	}
	var report flattenReport
	if err := json.Unmarshal([]byte(answer.Result.Value), &report); err != nil {
		return nil, fmt.Errorf("ตำแหน่งสไลด์ที่ได้กลับมาอ่านไม่ออก: %w", err)
	}
	if len(report.Rects) == 0 {
		return nil, fmt.Errorf(`ไม่พบสไลด์ในไฟล์นี้ สไลด์คือ <section class="slide">`)
	}
	if err := checkFlattened(report); err != nil {
		return nil, err
	}
	return report.Rects, nil
}

// checkFlattened refuses a deck that is still folded up.
//
// Two ways it can be, and the second is the one that fooled this code for a
// whole round of fixes. Slides sharing a position is the obvious one. The subtle
// one is a document shorter than the slides it contains: they measure to
// different places, so every position looks right, and the page they sit on is
// one screen tall — printing clips the rest and reports success.
//
// Without this the failure is silent and confident: a one-page PDF, or eight
// pictures of slide one. Both look exactly like the export working.
func checkFlattened(r flattenReport) error {
	if len(r.Rects) < 2 {
		return nil
	}
	for i := 1; i < len(r.Rects); i++ {
		if r.Rects[i].Y == r.Rects[i-1].Y && r.Rects[i].X == r.Rects[i-1].X {
			return fmt.Errorf("สไลด์ %d กับ %d ยังซ้อนอยู่ที่เดียวกันหลังจัดหน้าแล้ว "+
				"เด็คนี้น่าจะซ่อนสไลด์ด้วยวิธีที่ตัวส่งออกยังแกะไม่ออก", i, i+1)
		}
	}
	// A tenth of slack, because a deck may legitimately end a little short of
	// the sum of its slides — overlapping margins, a last slide that is not
	// full height. An order of magnitude short is a trapped document.
	var wanted float64
	for _, rect := range r.Rects {
		wanted += rect.H
	}
	if r.DocHeight < wanted*0.9 {
		return fmt.Errorf("หน้าเอกสารสูง %.0fpx แต่สไลด์ทั้งหมดรวมกัน %.0fpx "+
			"แปลว่ายังมีตัวครอบที่ตัดสไลด์ที่เหลือทิ้ง — ส่งออกไปก็จะได้ไม่ครบ",
			r.DocHeight, wanted)
	}
	return nil
}
