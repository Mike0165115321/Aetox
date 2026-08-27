package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReadSkillExecute(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"hello.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	// `cat -n` shape: the number is right-aligned in six columns, then a tab.
	// Pinned exactly, because edit's description promises the model a prefix it
	// can strip and a change of format here silently breaks that promise.
	if out.Content != "     1\thi there" {
		t.Errorf("Content = %q, want %q", out.Content, "     1\thi there")
	}
	if !out.Success {
		t.Error("Success = false, want true")
	}
}

// A PDF (or any binary) used to come back as Success with the text
// "(binary file)" — a green tick on a read that gave the model nothing, which
// it read as "keep trying" rather than "this cannot be read".
func TestReadSkillFailsOnBinaryInsteadOfReportingSuccess(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), []byte("%PDF-1.7\x00\x01binary"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"doc.pdf"}})
	if err == nil {
		t.Fatal("expected an error for a binary file, got nil")
	}
	if out.Success {
		t.Error("Success = true on a file that could not be read")
	}
}

// The other half of ARCHITECTURE.md §51: the user's attachment reaches a
// sighted model as a picture, and so does a file the model went and found.
func TestReadSkillReturnsAnImageToASightedModel(t *testing.T) {
	root := t.TempDir()
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01}
	if err := os.WriteFile(filepath.Join(root, "shot.png"), png, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root, vision: true}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "shot.png"})
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if len(out.Images) != 1 {
		t.Fatalf("Images = %d, want the picture itself", len(out.Images))
	}
	if out.Images[0].MediaType != "image/png" {
		t.Errorf("MediaType = %q, want image/png", out.Images[0].MediaType)
	}
	if string(out.Images[0].Data) != string(png) {
		t.Error("the bytes handed over are not the bytes on disk")
	}
	// The text still names the file, because the model has to be able to talk
	// about it and edit it later.
	if !strings.Contains(out.Content, "shot.png") {
		t.Errorf("Content = %q, want the path", out.Content)
	}
}

// A blind model must get the refusal that names image_ocr, not an image it
// cannot see and not a silent empty success.
func TestReadSkillRefusesAnImageForABlindModel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shot.png"), []byte{0x89, 'P', 'N', 'G'}, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root} // vision off, the default everywhere

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "shot.png"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "image_ocr") {
		t.Errorf("err = %v, want it to name the tool that can help", err)
	}
	if out.Success || len(out.Images) != 0 {
		t.Error("a refused read must return neither success nor an image")
	}
}

func TestReadSkillMissingFile(t *testing.T) {
	s := &readSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"does-not-exist.txt"}})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestReadSkillRejectsEscape(t *testing.T) {
	s := &readSkill{root: t.TempDir()}
	_, err := s.Execute(context.Background(), Input{"args": []string{"../outside.txt"}})
	if err == nil {
		t.Fatal("expected error escaping sandbox, got nil")
	}
}

func TestReadSkillEmptyFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.txt"), []byte(""), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}
	out, err := s.Execute(context.Background(), Input{"args": []string{"empty.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Content != "(empty file)" {
		t.Errorf("Content = %q, want %q", out.Content, "(empty file)")
	}
}

func TestReadSkillDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}
	_, err := s.Execute(context.Background(), Input{"args": []string{"subdir"}})
	if err == nil {
		t.Fatal("expected error reading a directory, got nil")
	}
}

// The old flat 16KB ceiling hid the tail of any real source file. Paging must
// hand back the requested window and say where to resume.
func TestReadSkillPagesByLine(t *testing.T) {
	root := t.TempDir()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i+1)
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"big.txt"}, "offset": 10, "limit": 3})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	// The numbers are the file's own, not a count from the start of the page:
	// paging that renumbered from 1 would make every citation past page one wrong.
	if !strings.HasPrefix(out.Content, "    10\tline-10\n    11\tline-11\n    12\tline-12") {
		t.Errorf("Content = %q, want to start at line-10 and hold 3 lines", out.Content)
	}
	if !out.Truncated || !strings.Contains(out.Content, "offset=13") {
		t.Errorf("Content = %q, truncated = %v, want a resume hint at offset=13", out.Content, out.Truncated)
	}

	// The last page ends cleanly — no truncation marker, nothing hidden.
	out, err = s.Execute(context.Background(), Input{"args": []string{"big.txt"}, "offset": 98, "limit": 3})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Truncated || !strings.HasSuffix(out.Content, "line-100") {
		t.Errorf("Content = %q, truncated = %v, want the file's tail with no truncation", out.Content, out.Truncated)
	}

	// A file far past the old 16KB cap must still come back whole by default.
	// That was the point of replacing the flat ceiling with paging and it has
	// not changed: what a coding agent cannot survive is a tail that is hidden,
	// not a tail that takes a second call.
	fat := strings.Repeat(strings.Repeat("x", 63)+"\n", 800) // 800 lines, 51KB
	if err := os.WriteFile(filepath.Join(root, "fat.txt"), []byte(fat), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	out, err = s.Execute(context.Background(), Input{"args": []string{"fat.txt"}})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out.Truncated || len(out.Content) < len(fat)-1 {
		t.Errorf("51KB file: len(Content) = %d, truncated = %v, want the whole file", len(out.Content), out.Truncated)
	}
}

// Past readMaxBytes a read is PAGED, never trimmed in silence: page one stops,
// says so, names the line to resume from, and that line really is the next one.
//
// This is the property the old 16KB ceiling failed and the 256KB one bought at
// a price nobody had measured — 27 calls in this machine's history carried half
// of everything `read` has ever returned. A bounded page keeps the promise the
// ceiling broke, as long as this test holds.
func TestReadSkillPagesPastByteCap(t *testing.T) {
	root := t.TempDir()
	const lines = 4000
	body := strings.Repeat(strings.Repeat("y", 63)+"\n", lines) // 256KB, under the line cap
	if err := os.WriteFile(filepath.Join(root, "huge.txt"), []byte(body), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"huge.txt"}, "limit": lines})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Truncated {
		t.Fatalf("len(Content) = %d, truncated = false: a 256KB file must not arrive in one answer", len(out.Content))
	}
	if len(out.Content) > readMaxBytes+readMaxLineLen {
		t.Errorf("len(Content) = %d, want at most one line past the %d byte cap", len(out.Content), readMaxBytes)
	}

	// The marker's offset is the contract. Follow it and the next line is the
	// one page one stopped before — no gap, no repeat.
	marker := "continue with offset="
	i := strings.Index(out.Content, marker)
	if i < 0 {
		t.Fatalf("no resume marker in a truncated read: %q", out.Content[len(out.Content)-80:])
	}
	next := strings.TrimSpace(strings.Trim(out.Content[i+len(marker):], ")\n"))
	resume, err := strconv.Atoi(next)
	if err != nil {
		t.Fatalf("resume offset %q is not a number: %v", next, err)
	}
	if !strings.Contains(out.Content, fmt.Sprintf("%6d\t", resume-1)) {
		t.Errorf("page one does not end at line %d, so the offset skips or repeats a line", resume-1)
	}

	page2, err := s.Execute(context.Background(), Input{"args": []string{"huge.txt"}, "offset": resume, "limit": 1})
	if err != nil {
		t.Fatalf("Execute(page 2): %v", err)
	}
	if !strings.Contains(page2.Content, fmt.Sprintf("%6d\t", resume)) {
		t.Errorf("page two = %q, want it to start at line %d", page2.Content, resume)
	}
}

// A generated file defeats a line cap: 110 lines of a bundle cost 57,437 bytes
// in this machine's history, which is the case readMaxLineLen exists for.
func TestReadSkillClipsGeneratedLines(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("z", readMaxLineLen*3)
	if err := os.WriteFile(filepath.Join(root, "bundle.js"), []byte(long+"\nshort\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	out, err := s.Execute(context.Background(), Input{"args": []string{"bundle.js"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Content) > readMaxLineLen*2 {
		t.Errorf("len(Content) = %d, want the generated line clipped", len(out.Content))
	}
	// Said out loud, because an edit built from a clipped line will not match
	// and the model has to be able to see why.
	if !strings.Contains(out.Content, "not the file's exact text") {
		t.Errorf("Content = %q, want the clip to say it is not the file's text", out.Content)
	}
	// Clipping one line must not cost the next one.
	if !strings.Contains(out.Content, "short") {
		t.Errorf("Content = %q, want the line after the clipped one", out.Content)
	}
}

// JSON hands numbers over as float64; a model that quotes them must work too.
func TestReadSkillExecuteToolOffsetTypes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	for _, offset := range []any{float64(2), "2", 2} {
		out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "a.txt", "offset": offset, "limit": float64(1)})
		if err != nil {
			t.Fatalf("ExecuteTool(offset=%v): unexpected error: %v", offset, err)
		}
		if !strings.HasPrefix(out.Content, "     2\ttwo") {
			t.Errorf("ExecuteTool(offset=%v) = %q, want to start at %q", offset, out.Content, "     2\ttwo")
		}
	}
}

func TestReadSkillExecuteTool(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &readSkill{root: root}

	if _, err := s.ExecuteTool(context.Background(), map[string]any{}); err == nil {
		t.Error("ExecuteTool with no path: expected error, got nil")
	}

	out, err := s.ExecuteTool(context.Background(), map[string]any{"path": "a.txt"})
	if err != nil {
		t.Fatalf("ExecuteTool: unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "content") {
		t.Errorf("Content = %q, want to contain %q", out.Content, "content")
	}
}
