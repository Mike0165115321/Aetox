package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// composerLine is the exact string cockpit.svelte.ts appends for an attached
// image. Written out in full rather than built from a helper: if the composer's
// wording changes and this does not, the picture stops reaching the model and
// nothing else fails — so this literal is the contract, and this test is what
// notices when one side moves.
const composerLine = "\n\n[attachment: user-attached image — read it with image_ocr] .aetox-attachments/shot.png"

func newVisionApp(t *testing.T, provider, modelName string) (*App, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".aetox-attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A real PNG header, so mime.TypeByExtension and any later sniffing agree.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), png, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return &App{cfg: config.Config{
		SandboxRoot:   root,
		ModelProvider: provider,
		ModelName:     modelName,
	}}, root
}

func TestVisionAttachmentsSendsThePictureToASightedModel(t *testing.T) {
	app, _ := newVisionApp(t, "openai", "gpt-4o")

	text, images := app.visionAttachments("why is this broken?" + composerLine)

	if len(images) != 1 {
		t.Fatalf("want one image attached, got %d", len(images))
	}
	if images[0].MediaType != "image/png" || len(images[0].Data) == 0 {
		t.Errorf("image = %+v, want a png with bytes", model.Image{MediaType: images[0].MediaType})
	}
	if strings.Contains(text, "image_ocr") {
		t.Errorf("text still tells the model to OCR a picture it is holding: %q", text)
	}
	// The path stays: the model needs the file's name to talk about it or edit it.
	if !strings.Contains(text, ".aetox-attachments/shot.png") {
		t.Errorf("text = %q, want the path kept", text)
	}
	if !strings.Contains(text, "why is this broken?") {
		t.Errorf("text = %q, want the question kept", text)
	}
}

// The fallback, and the reason image_ocr is not being deleted: a model with no
// vision must get exactly what it got before this feature existed.
func TestVisionAttachmentsLeavesABlindModelOnTheOCRPath(t *testing.T) {
	app, _ := newVisionApp(t, "deepseek", "deepseek-v4")
	original := "why is this broken?" + composerLine

	text, images := app.visionAttachments(original)

	if len(images) != 0 {
		t.Errorf("attached %d images to a model that cannot see", len(images))
	}
	if text != original {
		t.Errorf("text was rewritten for a blind model\n got: %q\nwant: %q", text, original)
	}
}

func TestVisionAttachmentsIgnoresAMessageWithNoAttachment(t *testing.T) {
	app, _ := newVisionApp(t, "openai", "gpt-4o")
	const plain = "just a question"

	text, images := app.visionAttachments(plain)

	if text != plain || images != nil {
		t.Errorf("visionAttachments(%q) = (%q, %v), want it untouched", plain, text, images)
	}
}

// A path that escapes the sandbox must not be read. Falling back to the OCR
// line rather than erroring is deliberate: image_ocr resolves paths through the
// same guard and will refuse it in terms the model already understands.
func TestVisionAttachmentsRefusesToEscapeTheSandbox(t *testing.T) {
	app, _ := newVisionApp(t, "openai", "gpt-4o")
	escaping := "look" + "\n\n[attachment: user-attached image — read it with image_ocr] ../../secrets.png"

	text, images := app.visionAttachments(escaping)

	if len(images) != 0 {
		t.Fatal("read an image from outside the sandbox root")
	}
	if text != escaping {
		t.Errorf("text = %q, want it left for the OCR path", text)
	}
}

// A missing file is the same story: no crash, no empty image, just the path
// that was already there.
func TestVisionAttachmentsSurvivesAMissingFile(t *testing.T) {
	app, _ := newVisionApp(t, "openai", "gpt-4o")
	gone := "look" + "\n\n[attachment: user-attached image — read it with image_ocr] .aetox-attachments/nope.png"

	text, images := app.visionAttachments(gone)

	if len(images) != 0 {
		t.Fatalf("attached %d images for a file that does not exist", len(images))
	}
	if text != gone {
		t.Errorf("text = %q, want it unchanged", text)
	}
}
