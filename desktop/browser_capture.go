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
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

type browserCaptureSkill struct{ app *App }

// Numbered per run rather than per session, which is enough to keep two shots
// in one turn from being one file — the session folder already separates chats.
var browserShotSeq int64

func (s *browserCaptureSkill) capture(ctx context.Context) (skill.Output, error) {
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
	var title, url string
	if t := a.browsers.tab(string(id)); t != nil {
		title, url = t.meta()
	}
	a.emitEvent("workbench:open-browser", map[string]string{"id": string(id), "url": url})
	select {
	case <-ctx.Done():
		out.DurationMs = time.Since(start).Milliseconds()
		return out, ctx.Err()
	case <-time.After(400 * time.Millisecond): // the raise has to reach the native view and it has to draw
	}

	dataURL, err := a.BrowserCapturePNG(string(id))
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

	rel, err := a.writeBrowserShot(png)
	if err != nil {
		out.Content, out.Stderr = "เก็บภาพไม่สำเร็จ: "+err.Error(), err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	out.Success = true
	out.DurationMs = time.Since(start).Milliseconds()
	out.Artifacts = []string{rel}

	// The picture goes to the model only if the model has eyes. A blind one gets
	// the path and the tool it has always had for reading letters out of an
	// image, which is the same trade visionAttachments makes for a user's
	// attachment — one question, one answer, asked in both places by the same
	// function.
	// Named the way every other browser action names a page. See browserPageRef.
	where := browserPageRef(title, url)
	if where == "" {
		where = "the open page"
	}
	if model.ResolveVision(a.cfg.ModelProvider, a.cfg.ModelName) {
		out.Images = []model.Image{{MediaType: "image/png", Data: png}}
		out.Content = fmt.Sprintf("ภาพของ %s อยู่ด้านล่าง และเก็บไว้ที่ %s", where, rel)
	} else {
		out.Content = fmt.Sprintf("เก็บภาพของ %s ไว้ที่ %s แล้ว ใช้ image_ocr กับไฟล์นี้เพื่ออ่านข้อความในภาพ", where, rel)
	}
	out.RawOutput = out.Content
	return out, nil
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
func (a *App) workFileDir() string {
	session := strings.TrimSpace(a.cur().id)
	if session == "" {
		session = "unsaved" // a chat that has not been saved can still take a picture
	}
	return path.Join("output", session)
}

// writeBrowserShot puts the picture in the work-file folder and answers with the
// sandbox-relative path.
func (a *App) writeBrowserShot(png []byte) (string, error) {
	root := strings.TrimSpace(a.cfg.SandboxRoot)
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
