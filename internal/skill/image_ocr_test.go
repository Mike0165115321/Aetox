package skill

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageOCRSkillRejectsEscape(t *testing.T) {
	s := &imageOCRSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../outside.png"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestImageOCRSkillUsageError(t *testing.T) {
	s := &imageOCRSkill{root: t.TempDir()}
	if _, err := s.Execute(context.Background(), Input{"args": []string{}}); err == nil {
		t.Fatal("expected usage error for missing path, got nil")
	}
	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected usage error for missing path arg, got nil")
	}
}

// Exercises the real "tesseract not installed" path when the binary isn't on
// PATH (true for most machines, including CI) — asserts a clear, actionable
// error rather than a raw exec.ErrNotFound.
func TestImageOCRSkillMissingBinaryGivesActionableError(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err == nil {
		t.Skip("tesseract is installed on this machine — not exercising the missing-binary path")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "img.png"), []byte("not a real png, just needs to exist"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &imageOCRSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"img.png"}})
	if err == nil {
		t.Fatal("expected error when tesseract is not installed, got nil")
	}
	if out.Success {
		t.Error("Success = true, want false")
	}
	if !strings.Contains(err.Error(), "ติดตั้ง") {
		t.Errorf("error should tell the user to install Tesseract, got %q", err.Error())
	}
}

func TestMissingTesseractErrorNeverEmpty(t *testing.T) {
	// Exercises whichever OS branch this test happens to run on — the point
	// is just that every branch returns a real, non-empty message.
	if err := missingTesseractError(); err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Errorf("missingTesseractError() = %v, want a non-empty actionable message", err)
	}
}

func TestCommandExistsFalseForBogusName(t *testing.T) {
	if commandExists("this-command-definitely-does-not-exist-aetox-test") {
		t.Error("commandExists returned true for a name that can't possibly be on PATH")
	}
}
