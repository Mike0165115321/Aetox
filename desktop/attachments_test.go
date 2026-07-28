package main

// Attachment lifetime: one folder per session, removed with the session, and
// what no session owns is swept on root change. Before per-session folders,
// every chat's attachments piled up in one shared folder forever — a later
// chat could list and read documents attached to any earlier one (found
// 2026-07-28, when a session surfaced another session's financial PDFs).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSourceFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSaveChatAttachmentLandsInSessionFolder(t *testing.T) {
	root := t.TempDir()
	a := newTestApp(t, root)

	rel, err := a.SaveChatFile(writeSourceFile(t, "doc.pdf"))
	if err != nil {
		t.Fatalf("SaveChatFile: %v", err)
	}
	wantPrefix := attachmentsDir + "/" + a.sessionID + "/"
	if !strings.HasPrefix(rel, wantPrefix) {
		t.Errorf("attachment path = %q, want it under %q — outside its session folder it outlives the chat", rel, wantPrefix)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Errorf("returned path does not exist on disk: %v", err)
	}
}

func TestSaveChatAttachmentRequiresASession(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	a.sessionID = ""
	if _, err := a.SaveChatFile(writeSourceFile(t, "doc.pdf")); err == nil {
		t.Fatal("no session must fail loudly — a flat file would be swept as legacy")
	}
}

func TestDeleteSessionRemovesItsAttachments(t *testing.T) {
	root := t.TempDir()
	a := newTestApp(t, root)
	a.appendTurn(
		SessionMessage{Role: "user", Text: "with attachment", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "ok", Time: "10:00"},
	)
	if _, err := a.SaveChatFile(writeSourceFile(t, "doc.pdf")); err != nil {
		t.Fatalf("SaveChatFile: %v", err)
	}
	id := a.sessionID
	dir := filepath.Join(root, attachmentsDir, id)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("attachment folder missing before delete: %v", err)
	}

	if err := a.DeleteSession(id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("attachments must go with their session, stat err = %v", err)
	}
}

func TestSweepAttachments(t *testing.T) {
	root := t.TempDir()
	a := newTestApp(t, root)
	a.appendTurn(
		SessionMessage{Role: "user", Text: "owner", Time: "10:00"},
		SessionMessage{Role: "agent", Text: "ok", Time: "10:00"},
	)
	dir := filepath.Join(root, attachmentsDir)

	owned := filepath.Join(dir, a.sessionID) // has a sessions row → kept
	fresh := filepath.Join(dir, "20991231-000000.000")
	orphan := filepath.Join(dir, "20250101-000000.000")
	for _, d := range []string{owned, fresh, orphan} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// No row, but too young to judge: could be another window's chat that has
	// not sent its first message yet.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	flat := filepath.Join(dir, "1785163088460-3.pdf") // pre-per-session pile
	if err := os.WriteFile(flat, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	a.sweepAttachments(root)

	if _, err := os.Stat(flat); !os.IsNotExist(err) {
		t.Error("legacy flat file survived the sweep — that pile is the leak")
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Error("old folder with no session row survived the sweep")
	}
	if _, err := os.Stat(owned); err != nil {
		t.Error("folder of a live session must survive the sweep")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("young row-less folder must survive — it may belong to a chat that has not saved yet")
	}
}
