package main

// The agent looking at a page instead of reading it.
//
// `read` hands back the page as text and refs, and that is the right answer
// almost always — it is cheap, it is exact, and it is what a model can act on.
// What it cannot do is answer a question whose answer was never in the text: a
// chart, a canvas, a map, a layout that is wrong. For those the page has to be
// seen, and BrowserCapturePNG has been able to produce the picture since the
// annotation modes shipped (browser_shot.go). It was simply never a tool: the
// only thing that could ask for it was the user drawing on a page.
//
// Two things had to be true before it could become one, and only one of them
// was about pictures.
//
//   - **Which tab.** A photograph of the wrong page is obvious in a way that
//     text of the wrong page is not, and until agentTab existed
//     the browser actions targeted whatever tab was showing. Capture would have
//     been the first action to make that visible, by handing back a picture of
//     whatever the user happened to be looking at.
//   - **Where it lands.** A picture is bytes, and bytes have to go somewhere the
//     user can reach or the capability is a thing the agent saw and nobody else
//     can check.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Mikedev115/Aetox/internal/deck"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

type browserCaptureSkill struct{ app *App }

// Numbered per run rather than per session, which is enough to keep two shots
// in one turn from being one file — the session folder already separates chats.
var browserShotSeq int64

// full asks for the whole document rather than the visible part of it.
//
// Off by default, and that is the right default rather than a timid one: most
// pages fit, a full-page picture of a long one is far more bytes for the same
// answer, and the visible area is what the user is looking at while the agent
// works. It earns its keep on the page that does not fit — a long form, a
// report, a layout whose problem is below the fold.
func (s *browserCaptureSkill) capture(ctx context.Context, full bool) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "browser_capture", Command: "browser capture"}
	a := s.app

	id, err := a.agentTab()
	if err != nil {
		out.Content, out.Stderr = err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	// Raise the tab before photographing it, rather than hoping it is up.
	//
	// A hidden native view is not compositing: BrowserSetVisible(false) is a
	// Win32 ShowWindow(SW_HIDE), and a window in that state produces no frames,
	// so a capture of one comes back as the last thing it drew or as nothing at
	// all. Raising is also the honest half — this tool's whole premise is that
	// the user watches what the agent does, which cannot be true of a photograph
	// taken of something they were never shown.
	//
	// It is the same event `open` uses, and the frontend's handler only makes an
	// existing tab active, so nothing re-navigates and the URL is along for the
	// ride the tab is already on.
	// Nothing of Aetox's own in the photograph.
	//
	// The click ring sits directly over the control it points at, so a capture
	// taken a moment after a click would hand the model a picture of the page
	// with a bright circle drawn across the thing it was looking for — and the
	// model has no way to know the circle is not part of the site.
	//
	// This one BLOCKS until the page says the mark is gone. It used to be queued
	// and left to the 400ms raise below to have landed by, which worked and was
	// not a guarantee: eval hands a script to the page and returns, so the sleep
	// was doing a job it was never told about. The wait is bounded at two
	// seconds and silence is not a failure — see clearPageMarks.
	a.clearPageMarks(id)

	var title, url string
	tab := a.browsers.tab(string(id))
	if tab != nil {
		title, url = tab.meta()
	}
	a.deskEvent("", "open-browser", map[string]string{"id": string(id), "url": url})
	select {
	case <-ctx.Done():
		out.DurationMs = time.Since(start).Milliseconds()
		return out, ctx.Err()
	case <-time.After(400 * time.Millisecond): // the raise has to reach the native view and it has to draw
	}

	// notes are whatever this picture is not. They are collected as they are
	// learnt and spent at the bottom, because every one of them is a thing a
	// caller would otherwise have to infer from a picture that looks perfectly
	// fine: a page cut off at a height it cannot see, a full-page request
	// quietly served by the viewport path, and a deck photographed by a
	// renderer that is not the one that exports it.
	var notes []string
	var dataURL string
	if full {
		var cutAt int
		dataURL, cutAt, err = a.BrowserCaptureFullPNG(ctx, string(id))
		switch {
		case err != nil:
			// Falling back rather than failing, because the visible area is
			// still an answer to most questions. Saying so is not optional: a
			// caller that asked for the whole page and was handed the top of it
			// without being told would report on a page it never saw.
			notes = append(notes, "ถ่ายทั้งหน้าไม่สำเร็จ ("+err.Error()+") ภาพนี้จึงเป็นเฉพาะส่วนที่เห็นบนจอ")
			dataURL, err = a.BrowserCapturePNG(string(id))
		case cutAt > 0:
			notes = append(notes, fmt.Sprintf("หน้านี้ยาวกว่าที่ตัวเรนเดอร์วาดได้ ภาพนี้คือ %d พิกเซลแรกจากบนสุด ส่วนที่เหลือไม่ได้อยู่ในภาพ", cutAt))
		}
	} else {
		dataURL, err = a.BrowserCapturePNG(string(id))
	}
	if err != nil {
		out.Content, out.Stderr = "แคปหน้าเว็บไม่สำเร็จ: "+err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}
	png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, "data:image/png;base64,"))
	if err != nil {
		out.Content, out.Stderr = "ภาพที่ได้อ่านไม่ออก: "+err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	if deckNote := a.deckRendererNote(url); deckNote != "" {
		notes = append(notes, deckNote)
	}

	// Named the way every other browser action names a page. See browserPageRef.
	where := browserPageRef(title, url)
	if where == "" {
		where = "the open page"
	}

	// "Nothing changed" is an answer, and until the tab began remembering its
	// last picture it was the one answer this tool could not give. It handed
	// back the identical image and left the caller to notice for itself, which
	// no caller ever did — see browserTab.lastShot for what that cost.
	//
	// Neither attached nor written a second time, and both halves are the same
	// point: the caller is already holding this exact image, from the capture
	// this one is identical to. Sending it again spends the tokens of a picture
	// to say nothing, and a duplicate under output/<session>/work is a card in
	// the gallery for a photograph nobody took.
	sum := sha256.Sum256(png)
	if tab != nil {
		if at, inARow, same := tab.lastShot(sum); same {
			out.Success = true
			out.DurationMs = time.Since(start).Milliseconds()
			out.Content = fmt.Sprintf("ภาพของ %s เหมือนกับ %s ทุกไบต์ หน้านี้ไม่ได้เปลี่ยนอะไรเลยตั้งแต่แคปครั้งก่อน จึงไม่ได้เก็บไฟล์ซ้ำและไม่ได้ส่งภาพเดิมมาซ้ำ", where, at)
			if inARow > 1 {
				out.Content += fmt.Sprintf("\nเป็นครั้งที่ %d ติดกันแล้วที่ได้ภาพเดิม", inARow)
			}
			out.Content += captureNotes(notes)
			out.RawOutput = out.Content
			return out, nil
		}
	}

	rel, err := a.writeBrowserShot(png)
	if err != nil {
		out.Content, out.Stderr = "เก็บภาพไม่สำเร็จ: "+err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}
	if tab != nil {
		tab.rememberShot(sum, rel)
	}

	out.Success = true
	out.DurationMs = time.Since(start).Milliseconds()
	out.Artifacts = []string{rel}

	// The picture goes to the model only if the model has eyes. A blind one gets
	// the path and the tool it has always had for reading letters out of an
	// image, which is the same trade visionAttachments makes for a user's
	// attachment — one question, one answer, asked in both places by the same
	// function.
	if model.ResolveVision(a.cur().cfg.ModelProvider, a.cur().cfg.ModelName) {
		out.Images = []model.Image{{MediaType: "image/png", Data: png}}
		out.Content = fmt.Sprintf("ภาพของ %s อยู่ด้านล่าง และเก็บไว้ที่ %s", where, rel)
	} else {
		out.Content = fmt.Sprintf("เก็บภาพของ %s ไว้ที่ %s แล้ว ใช้ image_ocr กับไฟล์นี้เพื่ออ่านข้อความในภาพ", where, rel)
	}
	out.Content += captureNotes(notes)
	out.RawOutput = out.Content
	return out, nil
}

// captureNotes is the tail of a capture's answer: each thing the picture is
// not, on a line of its own, or nothing at all.
func captureNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return "\n" + strings.Join(notes, "\n")
}

// deckRendererNote is what a photograph of a deck is not.
//
// A deck is not exported by the tab it is being looked at in. deck_render.go
// opens the file in a window of its own, parked off-screen at exactly
// deckSlideWidthPx by deckSlideHeightPx, and photographs one slide at a time;
// a tab on somebody's monitor is neither that size nor that shape. A layout
// that depends on the window it is in therefore has two answers, and a capture
// only ever shows one of them.
//
// It is a note and not a refusal because the capture is still the right tool
// most of the time — most of a deck does not care how wide the window is. It
// exists because on 28 ส.ค. the complaint was "หน้าแรกพังครับ ตอนกดส่งออก" and
// every verification round that followed was a capture of the on-screen tab,
// so the renderer that had actually broken was never once looked at.
//
// Nothing new is exposed by the read: this is the file the tab in front of the
// user is already displaying.
func (a *App) deckRendererNote(pageURL string) string {
	full := localFileBehind(pageURL)
	if full == "" {
		return ""
	}
	if ext := strings.ToLower(filepath.Ext(full)); ext != ".html" && ext != ".htm" {
		return ""
	}
	// Bounded the way the deck listing bounds itself, and for the same reason:
	// deciding costs a read and an HTML parse, and a deck with its pictures
	// inline is megabytes.
	if info, err := os.Stat(full); err != nil || info.Size() > maxDeckBytes {
		return ""
	}
	source, err := os.ReadFile(full)
	if err != nil || !deck.Is(source) {
		return ""
	}
	return fmt.Sprintf(
		"ไฟล์นี้เป็นเด็ค ภาพนี้คือแท็บบนจอ ไม่ใช่สิ่งที่ตัวส่งออกวาด "+
			"ตัวส่งออกเปิดเด็คในหน้าต่างของมันเอง กว้าง %d สูง %d พอดี แล้วถ่ายทีละสไลด์ "+
			"เลย์เอาต์ที่อิงขนาดหน้าต่างจึงออกมาคนละอย่างได้ระหว่างสองที่นี้",
		deckSlideWidthPx, deckSlideHeightPx)
}

// workFileDir is where a file the agent produced **while working** goes, as a
// sandbox-relative path: a screenshot it took to see something, not a document
// somebody asked for by name.
//
// It is a different question from a.outputSubdir(), which answers "where does a
// NEW FILE go" and correctly says "the project itself" when one is focused. A
// deliverable belongs in the project. A byproduct does not, and page-1.png in
// the root of somebody's repository is a change nobody asked for.
//
// Named rather than inlined at the one call site, because the next tool that
// produces a byproduct will need this answer too — and if it is not a named
// concept, that tool copies the reasoning instead of the function, and then
// there are two places answering it.
//
// output/<session> is also the one path ListArtifacts is defined to sweep, so
// anything put here shows up in the gallery under either mode.
//
// Which is right, and was the whole problem until 25 ส.ค.: they showed up
// *level with the deliverable*. Counted on the owner's machine, 46 of 244 files
// in the gallery were browser screenshots — one session had nine in a row, with
// the document somebody actually asked for sitting as the tenth card, indistinct
// from the pages it was written from ("อันไหนเป็นรูปภาพอ่ะครับ รวมมันเป็นการ์ด
// อันเดียวกันได้มั้ยครับ").
//
// So the byproduct gets its own subfolder, and the gallery reads that folder as
// the fact — Artifact.Folder, one card per folder. Nothing new records what a
// file is: the place it was put says it, which is the only kind of record this
// page trusts, because the folder is the half the user can move and rename.
//
// The subfolder is added here rather than at the call site so that the next tool
// with a byproduct inherits it by using this function, which is the reason the
// function has a name at all.
func (a *App) workFileDir() string {
	session := strings.TrimSpace(a.cur().id)
	if session == "" {
		session = "unsaved" // a chat that has not been saved can still take a picture
	}
	return path.Join("output", session, workSubdir)
}

// workSubdir is the one folder name the app creates for its own working files.
// English, like output/ above it: this is a real directory the user will meet in
// Explorer and quote into a shell, and the gallery translates it for the card
// rather than putting Thai in a path.
const workSubdir = "work"

// writeBrowserShot puts the picture in the work-file folder and answers with the
// sandbox-relative path.
func (a *App) writeBrowserShot(png []byte) (string, error) {
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no working folder is set")
	}
	rel := path.Join(a.workFileDir(), fmt.Sprintf("page-%d.png", atomic.AddInt64(&browserShotSeq, 1)))
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, png, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
