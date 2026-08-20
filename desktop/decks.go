package main

// The bindings behind the slides room: what decks this workspace has, and
// turning one into a file somebody else's program opens.
//
// A deck is an .html file carrying `<section class="slide">`
// (docs/architecture/html-deck-2026-08-19.md). The authoring format is HTML
// because that is what a model writes best and what a browser already renders,
// which leaves exporting as a separate step — and that separation is the whole
// reason this file exists. `slides_write`, which wrote a .pptx directly, was
// retired for it (§149).
//
// Everything here speaks **project-relative paths**, the same vocabulary the
// file host and the workbench panes use. That is not a style choice: the file
// host resolves against SandboxRoot (filehost.go), so a deck addressed any other
// way lists fine and then renders as a blank iframe. Listing something the
// viewer cannot open is worse than not listing it, so the listing is scoped to
// what the viewer can actually show.

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/deck"
	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

// A deck is HTML plus its pictures inline, so it is bigger than a source file
// and much smaller than a video. 40 MB is two dozen embedded screenshots and
// well short of anything that should be scanned on a listing.
const maxDeckBytes = 40 << 20

// Deck is one deck as the room lists it.
type Deck struct {
	// Path is project-relative, which is what SlidesPane and the file host both
	// take. Never absolute: see the file header.
	Path   string `json:"path"`
	Name   string `json:"name"`
	Slides int    `json:"slides"`
	// SessionID is the chat that made it, read off the output folder name, so
	// the room can group by conversation without a table recording it. Empty
	// for a deck sitting loose in output/.
	SessionID string `json:"sessionId,omitempty"`
	Modified  string `json:"modified"` // RFC3339
}

// ListDecks returns every deck under this workspace's output folder, newest
// first.
//
// Scoped to the open workspace rather than to every root the gallery sweeps,
// and that is the file-host constraint above rather than a product decision:
// a deck from another project cannot be rendered here, so offering it would be
// a row that opens onto nothing.
func (a *App) ListDecks() []Deck {
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		// No project open reads as no decks, which is the truth about it. The
		// unfocused workspace has its own SandboxRoot when it is the one open,
		// so this is genuinely "nowhere to look" rather than a missed case.
		return []Deck{}
	}

	out := []Deck{}
	base := filepath.Join(root, outputDir)
	_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //lint:ignore nilerr an unreadable entry is skipped, not fatal
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".html" && ext != ".htm" {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil || info.Size() > maxDeckBytes {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		// The marker is counted rather than matched, so the row can say how
		// many slides are in there without the caller opening it. A page that
		// is not a deck costs one parse and is dropped.
		n := deck.Count(source)
		if n == 0 {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		out = append(out, Deck{
			Path:      filepath.ToSlash(rel),
			Name:      entry.Name(),
			Slides:    n,
			SessionID: sessionOfOutputPath(base, path),
			Modified:  info.ModTime().Format(time.RFC3339),
		})
		return nil
	})

	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out
}

// fileURLForPath turns an absolute OS path into a file:// URL.
//
// The export webview is a browser tab, not the app's own webview, so it cannot
// resolve `/aetox-file/...` — that path is served by the Wails asset handler
// inside the main window (filehost.go). It reaches the deck the way any browser
// reaches a local file. Which works only because the contract requires pictures
// to be embedded: a deck referencing chart.png beside itself would load in the
// pane and print blank boxes.
func fileURLForPath(abs string) string {
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed // C:/x -> /C:/x, so the URL keeps three slashes
	}
	// Each segment escaped on its own, or a Thai filename or a space arrives at
	// the engine as a different path than the one on disk. PathEscape leaves
	// the separators alone, which is why the split is by hand.
	parts := strings.Split(slashed, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "file://" + strings.Join(parts, "/")
}

// writeFileAtomically writes through a temp file in the same directory and
// renames, the way ooxml.WriteFile does. A half-written export that keeps the
// name of the last good one is worse than no export: it opens, it is wrong, and
// nothing says when it went wrong.
func writeFileAtomically(target string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".aetox-export-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, target); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// sessionOfOutputPath reads the chat id off the first folder under output/.
// A deck loose in output/ predates per-session folders and has none, which is
// answered as "" rather than guessed.
func sessionOfOutputPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// DeckFormat is one row in the export menu.
//
// Ready is here rather than in the pane because whether a format can be written
// is a fact about this binary, not about a button. The menu asks; it does not
// keep its own list. When PrintToPdf lands, one `true` here fills the row in
// without the frontend being touched, and there is never a moment where the two
// lists disagree about what works.
type DeckFormat struct {
	ID    string `json:"id"`
	Ext   string `json:"ext"`
	Ready bool   `json:"ready"`
}

// deckFormats is every format the export menu shows, in menu order.
//
// Ready is what the menu greys a row on, and nothing here may claim it without
// something behind it that writes: a row that looks available and then refuses
// is a lie the user only finds by clicking. TestEveryReadyFormatActuallyWrites
// is what holds that.
//
// The order is the order somebody wants them in: the editable copy, the one that
// looks exactly right, then the pictures for putting a slide in a chat.
var deckFormats = []DeckFormat{
	{ID: "pptx", Ext: ".pptx", Ready: true},
	{ID: "pptx-img", Ext: ".pptx", Ready: true},
	{ID: "pdf", Ext: ".pdf", Ready: true},
	{ID: "png", Ext: ".png", Ready: true},
	{ID: "jpg", Ext: ".jpg", Ready: true},
	{ID: "webp", Ext: ".webp", Ready: true},
}

// DeckFormats is the export menu, straight from the list above.
func (a *App) DeckFormats() []DeckFormat { return deckFormats }

// writableDeckFormat resolves an id to the extension it writes, and refuses
// anything this binary cannot actually produce.
func writableDeckFormat(id string) (string, bool) {
	for _, f := range deckFormats {
		if strings.EqualFold(f.ID, strings.TrimSpace(id)) {
			return f.Ext, f.Ready
		}
	}
	return "", false
}

// ExportDeck writes a deck out in another format and answers with where it
// landed.
//
// The .pptx it produces is deliberately plainer than the HTML it came from:
// title, bullets, one picture, speaker notes, which is exactly what
// ooxml.BuildPPTX has always written. That gap is the trade the move to HTML
// was made to take — the deck people look at is the HTML, and the .pptx is the
// copy somebody can open in PowerPoint and edit. Making the .pptx match the
// HTML pixel for pixel would mean writing a PowerPoint, which is the thing this
// project has now twice decided not to do.
//
// **One caller, and that is the decision (§153).** For a day this had two: the
// export bar and a `deck_export` tool the model could call. The tool was removed
// because the deck itself never needed it — the deck is the HTML, the slides
// pane reads it, and exporting is the separate act of handing a copy to somebody
// else's program. That act has a button, on the screen where the person is
// already looking at the deck; a tool doing the same thing cost 139 tokens in
// every request of the busiest desk to save one click, and was never once
// called.
func (a *App) ExportDeck(relPath, format string) (string, error) {
	root := strings.TrimSpace(a.cur().cfg.SandboxRoot)
	if root == "" {
		return "", fmt.Errorf("no project open")
	}
	ext, ready := writableDeckFormat(format)
	if !ready {
		// The same sentence for "no such format" and "listed but not ready
		// yet", because from here they are the same fact: nothing in this
		// binary writes it. The menu is what tells them apart, before the click.
		return "", fmt.Errorf("ยังส่งออกเป็น %s ไม่ได้", format)
	}
	full, err := safeSandboxPath(root, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", errFileGone
	}
	if info.Size() > maxDeckBytes {
		return "", fmt.Errorf("ไฟล์นี้ใหญ่เกินกว่าจะส่งออก")
	}
	source, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}

	// Into the machine's Downloads folder, under the deck's own name — see
	// freeDownloadPath for why there rather than beside the deck.
	//
	// The deck itself stays where it is. Nothing is moved.
	base := strings.TrimSuffix(filepath.Base(full), filepath.Ext(full))
	if strings.EqualFold(strings.TrimSpace(format), "pptx-img") {
		// Both pptx rows share an extension, so the picture one takes a suffix.
		// Without it the second export silently replaces the first, and the two
		// are not interchangeable: one is editable and plain, the other is exact
		// and frozen.
		base += "-img"
	}
	target, err := a.freeDownloadPath(base, ext)
	if err != nil {
		return "", err
	}

	// The two formats are two different acts, and only one of them reads the
	// deck's structure. `.pptx` reduces the HTML to slides and rebuilds it in
	// OOXML, which is why it is editable and plainer than the original. `.pdf`
	// hands the untouched file to the engine that renders it on screen, which is
	// why it looks exactly like the deck and cannot be edited. Neither is the
	// better one; they answer different questions about the same deck.
	switch id := strings.ToLower(strings.TrimSpace(format)); id {
	case "pdf":
		pdf, err := a.exportDeckPDF(context.Background(), fileURLForPath(full))
		if err != nil {
			return "", err
		}
		if err := writeFileAtomically(target, pdf); err != nil {
			return "", err
		}
	case "png", "jpg", "webp":
		// Pictures are one file per slide, so they land in a folder of their
		// own rather than scattering eight siblings beside the deck. The format
		// is in the folder's name because exporting both must not have the
		// second one overwrite the first.
		shots, err := a.exportDeckImages(context.Background(), fileURLForPath(full), id)
		if err != nil {
			return "", err
		}
		dir := strings.TrimSuffix(target, ext) // <name>.png -> <name>, a folder
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		for i, shot := range shots {
			// Zero-padded, so a ten-slide deck sorts 01..10 in every file
			// browser rather than 1, 10, 2.
			name := filepath.Join(dir, fmt.Sprintf("%02d%s", i+1, ext))
			if err := writeFileAtomically(name, shot); err != nil {
				return "", err
			}
		}
		target = dir
	case "pptx-img":
		// The same pictures the .png export writes, one per slide, each covering
		// its whole slide. It is a .pptx that cannot be edited a word of — and
		// that is the trade, not a shortcoming: what it buys is a deck that
		// looks in PowerPoint exactly like the deck looks here, which the
		// editable one never can without pptx.go learning to lay out a slide.
		shots, err := a.exportDeckImages(context.Background(), fileURLForPath(full), "png")
		if err != nil {
			return "", err
		}
		slides, err := deck.Slides(source, filepath.Dir(full), root)
		if err != nil {
			return "", err
		}
		if len(slides) != len(shots) {
			// The reducer and the renderer both cut on section.slide, so this
			// cannot happen from a well-formed deck — and if it ever does, a
			// deck with the wrong notes on the wrong slide is worse than none.
			return "", fmt.Errorf("จำนวนสไลด์ไม่ตรงกัน (%d ภาพ, %d สไลด์)", len(shots), len(slides))
		}
		picture := make([]ooxml.Slide, len(shots))
		for i, shot := range shots {
			cfg, _, err := image.DecodeConfig(bytes.NewReader(shot))
			if err != nil {
				return "", err
			}
			picture[i] = ooxml.Slide{
				FullBleed: true,
				// Notes survive: they never render, so a picture deck keeps the
				// presenter's script instead of trading it away for fidelity.
				Notes: slides[i].Notes,
				Image: &ooxml.Picture{Ext: "png", Data: shot, WidthPx: cfg.Width, HeightPx: cfg.Height, AltText: slides[i].Title},
			}
		}
		parts, err := ooxml.BuildPPTX(picture)
		if err != nil {
			return "", err
		}
		if err := ooxml.WriteFile(target, parts); err != nil {
			return "", err
		}
	default:
		// deck.Slides is authoritative about what a slide is; the frontend's
		// own check is a routing hint (see internal/deck/deck.go). So a file
		// that reached the room and still has nothing to export says so here,
		// naming what a slide is, rather than writing an empty deck.
		slides, err := deck.Slides(source, filepath.Dir(full), root)
		if err != nil {
			return "", err
		}
		parts, err := ooxml.BuildPPTX(slides)
		if err != nil {
			return "", err
		}
		if err := ooxml.WriteFile(target, parts); err != nil {
			return "", err
		}
	}
	a.rememberExport(target)
	return target, nil
}

// exportsDir is where an export lands: the machine's Downloads folder.
//
// Falls back to the home folder, then to the project, rather than failing — a
// machine with no Downloads is unusual but the export is still worth having, and
// the answer names wherever it actually went.
func (a *App) exportsDir() (string, error) {
	// Test seam, and it earns its keep: without it every run of the export
	// tests would drop files into the developer's real Downloads folder.
	if override := strings.TrimSpace(a.exportsRoot); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("หาโฟลเดอร์ของผู้ใช้ไม่เจอ")
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads, nil
	}
	return home, nil
}

// freeDownloadPath is <Downloads>/<base><ext>, with a number appended if that
// name is taken.
//
// Overwriting would be the wrong default here in a way it is not inside the
// session folder: Downloads is shared with every other program, the file there
// may be one the user already sent to somebody, and an export that quietly
// replaced it would destroy something this app never made. Windows numbers
// duplicate downloads the same way.
func (a *App) freeDownloadPath(base, ext string) (string, error) {
	dir, err := a.exportsDir()
	if err != nil {
		return "", err
	}
	base = sanitiseFileName(base)
	candidate := filepath.Join(dir, base+ext)
	for n := 2; n < 1000; n++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, n, ext))
	}
	return "", fmt.Errorf("มีไฟล์ชื่อนี้อยู่แล้วเป็นร้อยไฟล์ ลองเปลี่ยนชื่อเด็คดู")
}

// sanitiseFileName keeps a deck's own name while dropping what Windows will not
// accept in one. A deck may be called anything; a file may not.
func sanitiseFileName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 0x20 {
			return '-'
		}
		return r
	}, name)
	cleaned = strings.Trim(strings.TrimSpace(cleaned), ".")
	if cleaned == "" {
		return "deck"
	}
	return cleaned
}

// rememberExport records a file this session wrote, so OpenExport can open it.
//
// An export lands in Downloads, outside the project, which every other file
// binding here refuses by design (safeSandboxPath). Widening one of those to
// "anywhere" to serve one button would hand the frontend a way to open any path
// on the machine. A set of the paths this app actually wrote is the narrow
// version of the same permission: the only thing that can be opened is a file
// the user just asked to be made.
func (a *App) rememberExport(path string) {
	a.exportMu.Lock()
	defer a.exportMu.Unlock()
	if a.exported == nil {
		a.exported = map[string]bool{}
	}
	a.exported[path] = true
}

// OpenExport opens a file this session exported, with whatever the OS uses.
func (a *App) OpenExport(path string) error {
	a.exportMu.Lock()
	known := a.exported[path]
	a.exportMu.Unlock()
	if !known {
		// Not "permission denied": from here it is the truth. Nothing else has
		// ever been offered to open, so a path that is not in the set is not a
		// path this button ever produced.
		return fmt.Errorf("ไฟล์นี้ไม่ได้มาจากการส่งออกในรอบนี้")
	}
	if _, err := os.Stat(path); err != nil {
		return errFileGone
	}
	// Same door the file card uses, so an exported file opens exactly the way
	// every other produced file in this app does.
	return a.revealInFileManager(path)
}
