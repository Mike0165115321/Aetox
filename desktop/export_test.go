package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedChat writes a two-bubble conversation and returns its id.
func seedChat(t *testing.T, a *App) string {
	t.Helper()
	a.appendTurn(
		SessionMessage{Role: "user", Text: "อ่านสลิปนี้ให้หน่อย", Time: "2026-08-07T10:00:00+07:00"},
		SessionMessage{Role: "agent", Text: "ยอด 1,250 บาท", Time: "2026-08-07T10:00:05+07:00",
			Reasoning: "ดูตัวเลขใต้ QR", ThinkSecs: 4},
	)
	return a.sessionID
}

// The export is the copy, and a copy that comes back different is not a copy.
// Everything the messages table holds survives the trip out and back in.
func TestExportedChatImportsIntact(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	id := seedChat(t, a)
	path := filepath.Join(t.TempDir(), "chat.json")

	if err := a.writeSessionExport(id, "json", path); err != nil {
		t.Fatalf("export: %v", err)
	}
	newID, err := a.importSessionFrom(path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if newID == id {
		t.Fatal("an import must be a new session, not the old row back")
	}

	before, err := a.LoadSession(id)
	if err != nil {
		t.Fatalf("load original: %v", err)
	}
	after, err := a.LoadSession(newID)
	if err != nil {
		t.Fatalf("load import: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("import has %d messages, original %d", len(after), len(before))
	}
	for i := range before {
		b, f := before[i], after[i]
		if f.Role != b.Role || f.Text != b.Text || f.Time != b.Time ||
			f.Reasoning != b.Reasoning || f.ThinkSecs != b.ThinkSecs {
			t.Errorf("message %d changed in transit:\n before %+v\n after  %+v", i, b, f)
		}
	}

	// The verdicts stay home: a rating is this machine's judgement of work done
	// here, and an import that arrived pre-scored would feed the learning layer
	// evidence nobody on this machine gave.
	for _, m := range after {
		if m.Rating != "" && m.Rating != "unknown" {
			t.Errorf("a rating travelled with the export: %q", m.Rating)
		}
	}
}

// The markdown half is for people: the words, in order, and none of the
// bookkeeping.
func TestMarkdownExportCarriesTheConversation(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	id := seedChat(t, a)
	path := filepath.Join(t.TempDir(), "chat.md")

	if err := a.writeSessionExport(id, "markdown", path); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"อ่านสลิปนี้ให้หน่อย", "ยอด 1,250 บาท", "## คุณ", "## Aetox"} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown export is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "ดูตัวเลขใต้ QR") {
		t.Error("the model's reasoning is bookkeeping and should stay out of the readable export")
	}
}

// Same rule as the database: a file from a newer build is refused whole rather
// than half-read into someone's history.
func TestImportRefusesANewerFormat(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "future.json")
	future, _ := json.Marshal(map[string]any{
		"aetox_chat": chatExportVersion + 1,
		"messages":   []map[string]string{{"role": "user", "text": "hi"}},
	})
	if err := os.WriteFile(path, future, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.importSessionFrom(path); err == nil {
		t.Fatal("a newer export format was imported by a build that cannot know what is in it")
	}
}

// Not every .json is a chat. The marker field is what separates "wrong file"
// from a crash inside the insert loop.
func TestImportRefusesForeignJSON(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "notachat.json")
	if err := os.WriteFile(path, []byte(`{"name":"package.json","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.importSessionFrom(path); err == nil {
		t.Fatal("an unrelated JSON file was accepted as a conversation")
	}
}

// SaveDrawing accepts exactly one shape: the PNG data URL the drawing button
// rendered a moment ago. Anything else is not that button.
func TestDecodePNGDataURLIsStrict(t *testing.T) {
	if _, err := decodePNGDataURL("data:image/png;base64,aGVsbG8="); err != nil {
		t.Fatalf("a well-formed PNG data URL was refused: %v", err)
	}
	for _, bad := range []string{
		"", "hello", "data:image/svg+xml;base64,aGVsbG8=",
		"data:image/png;base64,!!!not-base64!!!", "data:image/png;base64,",
	} {
		if _, err := decodePNGDataURL(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
