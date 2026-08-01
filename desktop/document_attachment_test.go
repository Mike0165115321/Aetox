package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// composerDocLine is the exact string cockpit.svelte.ts appends for an attached
// PDF. Written out in full for the same reason composerLine is in
// vision_attachment_test.go: if the composer's wording changes and this does
// not, the document quietly stops reaching the model and nothing else fails.
const composerDocLine = "\n\n[attachment: user-attached file — read it with pdf_read] .aetox-attachments/statement.pdf"

func newDocumentApp(t *testing.T, provider, modelName string, size int) *App {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".aetox-attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	body := append([]byte("%PDF-1.4\n"), make([]byte, size)...)
	if err := os.WriteFile(filepath.Join(dir, "statement.pdf"), body, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return &App{cfg: config.Config{
		SandboxRoot:   root,
		ModelProvider: provider,
		ModelName:     modelName,
	}}
}

func TestDocumentAttachmentsSendsThePDFToAModelThatReads(t *testing.T) {
	app := newDocumentApp(t, "codex", "gpt-5.5", 64)

	text, docs := app.documentAttachments("summarise this" + composerDocLine)

	if len(docs) != 1 {
		t.Fatalf("attached %d documents, want 1", len(docs))
	}
	if docs[0].MediaType != "application/pdf" {
		t.Errorf("media type = %q, want application/pdf", docs[0].MediaType)
	}
	if docs[0].Name != "statement.pdf" {
		t.Errorf("name = %q, want the file's own name", docs[0].Name)
	}
	if len(docs[0].Data) == 0 {
		t.Error("document has no bytes")
	}
	// Rewritten so the model is not told to go extract a file it is holding.
	if strings.Contains(text, "pdf_read") {
		t.Errorf("text = %q, still points at pdf_read", text)
	}
	// The path survives the rewrite: the model has to be able to name the file.
	if !strings.Contains(text, ".aetox-attachments/statement.pdf") {
		t.Errorf("text = %q, lost the path", text)
	}
}

// The fallback is not a lesser path, it is the only path for a model that
// cannot take a file — so nothing about it may change.
func TestDocumentAttachmentsLeavesOtherModelsOnPDFRead(t *testing.T) {
	for _, tc := range []struct{ provider, model string }{
		{"anthropic", "claude-sonnet-5"}, // reads PDFs, but the wire shape is unverified here
		{"openai", "gpt-4o"},
		{"ollama", "llava:13b"},
	} {
		app := newDocumentApp(t, tc.provider, tc.model, 64)
		original := "summarise this" + composerDocLine

		text, docs := app.documentAttachments(original)

		if len(docs) != 0 {
			t.Errorf("%s/%s: attached %d documents; want the pdf_read path", tc.provider, tc.model, len(docs))
		}
		if text != original {
			t.Errorf("%s/%s: text = %q, want it untouched", tc.provider, tc.model, text)
		}
	}
}

// Above the cap the trade inverts: the whole document costs more than the
// truncated extract is worth, so pdf_read keeps the turn.
func TestDocumentAttachmentsLeavesAHugePDFOnPDFRead(t *testing.T) {
	app := newDocumentApp(t, "codex", "gpt-5.5", documentMaxInlineBytes+1)
	original := "summarise this" + composerDocLine

	text, docs := app.documentAttachments(original)

	if len(docs) != 0 {
		t.Fatalf("attached a document over the %d-byte cap", documentMaxInlineBytes)
	}
	if text != original {
		t.Errorf("text = %q, want it left for pdf_read", text)
	}
}

func TestDocumentAttachmentsIgnoresAMessageWithNoAttachment(t *testing.T) {
	app := newDocumentApp(t, "codex", "gpt-5.5", 64)
	plain := "what is the capital of Japan?"

	text, docs := app.documentAttachments(plain)

	if len(docs) != 0 || text != plain {
		t.Errorf("documentAttachments(%q) = %q, %d docs; want it untouched", plain, text, len(docs))
	}
}

func TestDocumentAttachmentsRefusesToEscapeTheSandbox(t *testing.T) {
	app := newDocumentApp(t, "codex", "gpt-5.5", 64)
	escaping := "read" + "\n\n[attachment: user-attached file — read it with pdf_read] ../../etc/passwd.pdf"

	text, docs := app.documentAttachments(escaping)

	if len(docs) != 0 {
		t.Fatalf("attached a document from outside the sandbox")
	}
	if text != escaping {
		t.Errorf("text = %q, want it left for the pdf_read path", text)
	}
}

func TestDocumentAttachmentsSurvivesAMissingFile(t *testing.T) {
	app := newDocumentApp(t, "codex", "gpt-5.5", 64)
	gone := "read" + "\n\n[attachment: user-attached file — read it with pdf_read] .aetox-attachments/nope.pdf"

	text, docs := app.documentAttachments(gone)

	if len(docs) != 0 {
		t.Fatalf("attached %d documents for a file that does not exist", len(docs))
	}
	if text != gone {
		t.Errorf("text = %q, want it unchanged", text)
	}
}

// A .txt attached alongside carries the same marker shape but is not a document
// Aetox uploads — `read` opens it, and always could.
func TestDocumentAttachmentsOnlyTakesPDFs(t *testing.T) {
	app := newDocumentApp(t, "codex", "gpt-5.5", 64)
	root := app.cfg.SandboxRoot
	if err := os.WriteFile(filepath.Join(root, ".aetox-attachments", "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	original := "read" + "\n\n[attachment: user-attached file — read it with pdf_read] .aetox-attachments/notes.txt"

	text, docs := app.documentAttachments(original)

	if len(docs) != 0 {
		t.Fatalf("uploaded a %q as a document", filepath.Ext("notes.txt"))
	}
	if text != original {
		t.Errorf("text = %q, want it unchanged", text)
	}
}
