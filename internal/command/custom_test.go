package command

import (
	"os"
	"path/filepath"
	"testing"
)

func setupCommandsDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	dir := filepath.Join(root, "commands")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

func TestExpandCustom(t *testing.T) {
	dir := setupCommandsDir(t)
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte("Review the file $ARGUMENTS carefully."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "standup.md"), []byte("Summarize today's work."), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := ExpandCustom("/review src/main.go")
	if !ok || got != "Review the file src/main.go carefully." {
		t.Fatalf("expand with args = %q, %v", got, ok)
	}

	// No $ARGUMENTS in the body → args appended.
	got, ok = ExpandCustom("/standup ship it")
	if !ok || got != "Summarize today's work.\n\nship it" {
		t.Fatalf("expand append = %q, %v", got, ok)
	}

	// Unknown command and plain text pass through untouched.
	if _, ok := ExpandCustom("/nope"); ok {
		t.Fatal("unknown command must not expand")
	}
	if _, ok := ExpandCustom("hello /review"); ok {
		t.Fatal("non-leading slash must not expand")
	}
	// Path traversal in the name must not read outside the commands dir.
	if _, ok := ExpandCustom("/../secrets"); ok {
		t.Fatal("traversal name must not expand")
	}
}

func TestListCustom(t *testing.T) {
	dir := setupCommandsDir(t)
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte("# Review helper\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	list := ListCustom()
	if len(list) != 1 || list[0].Name != "review" || list[0].Description != "Review helper" {
		t.Fatalf("list = %+v", list)
	}
}
