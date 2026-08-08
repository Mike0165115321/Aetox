package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// A conversation is the user's, and "theirs" has to mean more than "readable
// inside this app" — the same rule the memory folder already follows (learned:
// nothing the agent learns is allowed to be trapped in a format only we read).
// The transcript was the one thing that still was: living only in aetox.db,
// with the whole machine as its unit of portability.
//
// Two formats, because there are two reasons to take a conversation out:
//
//   - **Markdown** is for reading: paste into a report, hand to a colleague,
//     keep next to the notes. It carries what a person reads — the words —
//     and drops what the app needs (variants, tool parts, row ids).
//   - **JSON** is the copy: everything the messages table holds, versioned,
//     re-importable on any Aetox at full fidelity. This is the one a second
//     machine wants.
//
// What deliberately does not travel: ratings and job rows. They are this
// machine's record of work done here — evidence for the learning layer — and a
// transcript that arrives with pre-filled verdicts would teach the next
// machine lessons nobody on it approved.

// chatExportVersion is the format this build writes, checked on import the
// same way user_version is checked on open: a file from a newer build is
// refused rather than half-read, because a wrong import into someone's history
// is worse than asking them to upgrade.
const chatExportVersion = 1

type chatExport struct {
	AetoxChat int    `json:"aetox_chat"`
	Title     string `json:"title"`
	Mode      string `json:"mode,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Space     string `json:"space,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`

	Messages []chatExportMessage `json:"messages"`
}

// chatExportMessage mirrors one messages row. Variants and Parts ride as raw
// JSON: the store already holds them encoded, and decoding them here only to
// re-encode them there would be two chances to drop a field neither side
// looked at.
type chatExportMessage struct {
	Role      string          `json:"role"`
	Text      string          `json:"text"`
	Time      string          `json:"time,omitempty"`
	Reasoning string          `json:"reasoning,omitempty"`
	ThinkSecs int             `json:"think_secs,omitempty"`
	Variants  json.RawMessage `json:"variants,omitempty"`
	Active    int             `json:"variant_active,omitempty"`
	Parts     json.RawMessage `json:"parts,omitempty"`
}

// exportableSession reads one session out of the store by id alone — no
// project_key filter, because the id arrived from a row the user could see and
// clicked, and "you must focus that project before exporting from it" is a
// hoop with nobody on the other side.
func (a *App) exportableSession(id string) (chatExport, error) {
	var e chatExport
	db, err := a.database()
	if err != nil {
		return e, err
	}
	e.AetoxChat = chatExportVersion
	err = db.QueryRow(
		`SELECT title, mode, agent, space, created_at, updated_at FROM sessions WHERE id = ?`, id).
		Scan(&e.Title, &e.Mode, &e.Agent, &e.Space, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return e, fmt.Errorf("ไม่พบเซสชันนี้")
	}
	rows, err := db.Query(
		`SELECT role, text, time, reasoning, think_secs, variants, variant_active, parts
		   FROM messages WHERE session_id = ? ORDER BY id`, id)
	if err != nil {
		return e, err
	}
	defer rows.Close()
	for rows.Next() {
		var m chatExportMessage
		var variants, parts string
		if rows.Scan(&m.Role, &m.Text, &m.Time, &m.Reasoning, &m.ThinkSecs, &variants, &m.Active, &parts) != nil {
			continue
		}
		m.Variants = rawIfValid(variants)
		m.Parts = rawIfValid(parts)
		e.Messages = append(e.Messages, m)
	}
	if len(e.Messages) == 0 {
		return e, fmt.Errorf("เซสชันนี้ไม่มีข้อความให้ส่งออก")
	}
	return e, nil
}

// rawIfValid passes a stored JSON column through untouched, and drops what
// does not parse — a corrupt blob should cost that blob, not the export.
func rawIfValid(stored string) json.RawMessage {
	if strings.TrimSpace(stored) == "" || !json.Valid([]byte(stored)) {
		return nil
	}
	return json.RawMessage(stored)
}

// renderChatMarkdown is the human half. Only the live answer of each bubble —
// variants and tool parts are the app's bookkeeping; the reader asked for the
// conversation.
func renderChatMarkdown(e chatExport) string {
	var b strings.Builder
	b.WriteString("# " + strings.TrimSpace(e.Title) + "\n\n")
	b.WriteString(fmt.Sprintf("ส่งออกจาก Aetox — %s\n", exportTimeLabel(time.Now().Format(time.RFC3339))))
	for _, m := range e.Messages {
		who := "Aetox"
		if m.Role == "user" {
			who = "คุณ"
		}
		b.WriteString("\n## " + who)
		if label := exportTimeLabel(m.Time); label != "" {
			b.WriteString(" — " + label)
		}
		b.WriteString("\n\n" + strings.TrimSpace(m.Text) + "\n")
	}
	return b.String()
}

// exportTimeLabel renders a stored timestamp for a person. Messages carry
// whatever the frontend sent — RFC3339 on recent builds, a bare clock on the
// oldest rows — so anything unparseable is shown as it is rather than dropped.
func exportTimeLabel(stored string) string {
	if ts, err := time.Parse(time.RFC3339, stored); err == nil {
		return ts.Format("2006-01-02 15:04")
	}
	return strings.TrimSpace(stored)
}

// writeSessionExport renders one session to disk in the named format —
// "markdown" or "json". The pure half of ExportSession, split so a test can
// exercise everything but the dialog.
func (a *App) writeSessionExport(id, format, path string) error {
	e, err := a.exportableSession(id)
	if err != nil {
		return err
	}
	var data []byte
	switch format {
	case "markdown":
		data = []byte(renderChatMarkdown(e))
	case "json":
		if data, err = json.MarshalIndent(e, "", "  "); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown export format %q", format)
	}
	return os.WriteFile(path, data, 0o644)
}

// ExportSession asks where to save and writes one session there. Returns the
// path written, "" when the user closed the dialog — a cancel is a decision,
// not an error to report.
func (a *App) ExportSession(id, format string) (string, error) {
	// Resolved before the dialog opens: a session that cannot be exported
	// should refuse before asking where to put it.
	e, err := a.exportableSession(id)
	if err != nil {
		return "", err
	}
	ext, display := ".json", "Aetox chat (*.json)"
	if format == "markdown" {
		ext, display = ".md", "Markdown (*.md)"
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "ส่งออกบทสนทนา",
		DefaultFilename: exportFilename(e.Title, id) + ext,
		Filters:         []wailsruntime.FileFilter{{DisplayName: display, Pattern: "*" + ext}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, a.writeSessionExport(id, format, path)
}

// exportFilename turns a session title into something a filesystem accepts.
func exportFilename(title, id string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return -1
		}
		return r
	}, strings.TrimSpace(title))
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if r := []rune(cleaned); len(r) > 40 {
		cleaned = string(r[:40])
	}
	if cleaned == "" {
		cleaned = id
	}
	return "aetox-chat-" + cleaned
}

// importSessionFrom reads an exported JSON file into a brand-new session in
// the current project, and returns the new id. The pure half of ImportSession.
//
// The desk and chair come along verbatim, unchecked. If this machine has no
// such desk, opening the import will refuse with the same message a deleted
// desk gets on the machine it was deleted on (LoadSession: putting the file
// back is the way in) — one rule about missing stations, not two.
//
// The space does not come along: it names a project folder on the exporting
// machine (sessions.space), and stamping it here would file the chat inside a
// project this machine may not have. The import lands where the user is
// standing, which is where they chose to put it.
func (a *App) importSessionFrom(path string) (string, error) {
	db, err := a.database()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var e chatExport
	if err := json.Unmarshal(data, &e); err != nil || e.AetoxChat == 0 {
		return "", fmt.Errorf("ไฟล์นี้ไม่ใช่บทสนทนาที่ส่งออกจาก Aetox")
	}
	if e.AetoxChat > chatExportVersion {
		return "", fmt.Errorf(
			"ไฟล์นี้ส่งออกจาก Aetox รุ่นใหม่กว่า (รูปแบบ %d, รุ่นนี้รู้จักถึง %d) — อัปเดตแอปก่อนนำเข้า",
			e.AetoxChat, chatExportVersion)
	}
	var messages []chatExportMessage
	for _, m := range e.Messages {
		if m.Role == "user" || m.Role == "agent" {
			messages = append(messages, m)
		}
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("ไฟล์นี้ไม่มีข้อความให้นำเข้า")
	}

	title := strings.TrimSpace(e.Title)
	if title == "" {
		for _, m := range messages {
			if m.Role == "user" {
				title = sessionTitleFrom(m.Text)
				break
			}
		}
	}
	now := time.Now().Format(time.RFC3339)
	created := strings.TrimSpace(e.CreatedAt)
	if created == "" {
		created = now
	}

	id, err := unusedSessionID(db)
	if err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	// updated_at is now, not the file's: an import the user just did belongs at
	// the top of the list they are looking at, not sorted into last month.
	if _, err := tx.Exec(`
		INSERT INTO sessions(id, project_key, title, created_at, updated_at, mode, agent, space)
		VALUES(?,?,?,?,?,?,?,?)`,
		id, projectKey(a.cfg.SandboxRoot), title, created, now, e.Mode, e.Agent, a.space); err != nil {
		return "", err
	}
	for _, m := range messages {
		if _, err := tx.Exec(
			`INSERT INTO messages(session_id, role, text, time, reasoning, think_secs, variants, variant_active, parts)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			id, m.Role, m.Text, m.Time, m.Reasoning, m.ThinkSecs,
			string(m.Variants), m.Active, string(m.Parts)); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// unusedSessionID mints an id no stored session holds. Two imports in the same
// millisecond — or an import in the same millisecond as a new chat — would
// otherwise collide on the timestamp id and fail the INSERT.
func unusedSessionID(db *sql.DB) (string, error) {
	base := newSessionID()
	id := base
	for n := 2; ; n++ {
		var one int
		err := db.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, id).Scan(&one)
		if err == sql.ErrNoRows {
			return id, nil
		}
		if err != nil {
			return "", err
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// SaveDrawing writes a chart the model drew in a reply to a PNG file the user
// picks. The frontend rasterizes the SVG with the theme's actual colours baked
// in (a raw .svg coloured by var(--text-secondary) is invisible everywhere
// outside this app) and hands the finished image here as a data URL — this
// side only asks where and writes bytes. Returns the path, "" on cancel.
func (a *App) SaveDrawing(dataURL string) (string, error) {
	data, err := decodePNGDataURL(dataURL)
	if err != nil {
		return "", err
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "บันทึกภาพ",
		DefaultFilename: "aetox-drawing.png",
		Filters:         []wailsruntime.FileFilter{{DisplayName: "PNG (*.png)", Pattern: "*.png"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// decodePNGDataURL is the checked half of SaveDrawing: only a PNG data URL,
// because the bytes come from a canvas this app rendered a moment ago — any
// other shape means the caller is not the drawing button.
func decodePNGDataURL(dataURL string) ([]byte, error) {
	raw, ok := strings.CutPrefix(strings.TrimSpace(dataURL), "data:image/png;base64,")
	if !ok {
		return nil, fmt.Errorf("not a PNG data URL")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("the image data does not decode")
	}
	return data, nil
}

// ImportSession asks for an exported .json file and brings it in as a new
// session in the current project. Returns the new session's id, "" when the
// user closed the dialog.
func (a *App) ImportSession() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "นำเข้าบทสนทนา",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Aetox chat (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return a.importSessionFrom(path)
}
