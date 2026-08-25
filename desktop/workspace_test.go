package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// listPath runs the real `list` tool out of the app's live registry, which is
// the only way to test the thing that matters: not that resolveSandboxPath has
// a branch for added folders, but that the engine the user is actually talking
// to was built with them.
func listPath(t *testing.T, a *App, path string) (skill.Output, error) {
	t.Helper()
	s, ok := a.cur().registry.Get("list")
	if !ok {
		t.Fatal("no list skill in the registry")
	}
	tool, ok := s.(interface {
		ExecuteTool(context.Context, map[string]any) (skill.Output, error)
	})
	if !ok {
		t.Fatal("list skill lost ExecuteTool")
	}
	return tool.ExecuteTool(context.Background(), map[string]any{"path": path})
}

func focusedApp(t *testing.T, root string) *App {
	t.Helper()
	a := seed(&App{dbDir: t.TempDir(), projectFocused: true}, newConversation())
	closeDBOnCleanup(t, a)
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   root,
		ModelProvider: "aetox",
		ModelName:     "aetox-grid",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a
}

// The whole point, end to end: a focused project cannot see the folder next
// door, the user adds it, the running session can — and taking it off the list
// takes it back, in the same call, without a restart.
func TestAddedFolderWidensTheRunningSessionAndRemovalNarrowsIt(t *testing.T) {
	isolateUserDirs(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "shared-lib")
	for _, dir := range []string{root, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(other, "client.go"), []byte("package client"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := focusedApp(t, root)
	if _, err := listPath(t, a, other); err == nil {
		t.Fatal("a focused project reached a folder nobody added")
	}

	if _, err := a.addWorkspaceFolder(other); err != nil {
		t.Fatalf("adding a folder failed: %v", err)
	}
	out, err := listPath(t, a, other)
	if err != nil || !strings.Contains(out.Content, "client.go") {
		t.Fatalf("added folder is not reachable: err=%v content=%q", err, out.Content)
	}

	if _, err := a.RemoveWorkspaceFolder(other); err != nil {
		t.Fatalf("removing a folder failed: %v", err)
	}
	if _, err := listPath(t, a, other); err == nil {
		t.Fatal("folder stayed reachable after it was removed from the list")
	}
}

// The model is told what it can reach from the same list that lets it reach —
// a folder the tools accept but the prompt never mentions is a capability the
// model will not use, which reads to the user as the feature not working.
func TestAddedFolderIsNamedInTheSystemPrompt(t *testing.T) {
	isolateUserDirs(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "shared-lib")
	for _, dir := range []string{root, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := focusedApp(t, root)
	if _, err := a.addWorkspaceFolder(other); err != nil {
		t.Fatalf("adding a folder failed: %v", err)
	}
	messages := a.cur().agent.ContextMessages()
	if len(messages) == 0 {
		t.Fatal("agent has no context messages, so it has no system prompt")
	}
	if prompt := messages[0].Content; !strings.Contains(prompt, other) {
		t.Errorf("system prompt does not name the added folder %q:\n%s", other, prompt)
	}
}

// Every refusal is one the user finds out about while choosing, not later as a
// tool error on one file inside a folder the panel says they added.
func TestAddWorkspaceFolderRefusesWhatItCannotHonour(t *testing.T) {
	home := isolateUserDirs(t)
	root := filepath.Join(home, "project")
	sub := filepath.Join(root, "internal")
	ssh := filepath.Join(home, ".ssh")
	for _, dir := range []string{root, sub, ssh} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := focusedApp(t, root)

	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"a credential store", ssh, "ที่เก็บกุญแจ"},
		{"a folder already inside the project", sub, "อยู่ในโปรเจกต์อยู่แล้ว"},
		{"a path that is not a folder", filepath.Join(home, "nope"), "ไม่ใช่โฟลเดอร์"},
	}
	for _, tc := range cases {
		_, err := a.addWorkspaceFolder(tc.dir)
		if err == nil {
			t.Errorf("%s was accepted onto the list", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: refusal does not say why (%q)", tc.name, err)
		}
	}
	if got := a.WorkspaceFolders(); len(got) != 0 {
		t.Errorf("a refused folder still landed on the list: %+v", got)
	}
}

// With no project focused the tools already reach the machine, so the list has
// nothing to add — and offering it anyway would suggest the mode is narrower
// than it is.
func TestAddWorkspaceFolderIsRefusedWithNoProjectFocused(t *testing.T) {
	isolateUserDirs(t)
	a := seed(&App{dbDir: t.TempDir()}, newConversation()) // zero value: unfocused, the startup state
	closeDBOnCleanup(t, a)
	if _, err := a.AddWorkspaceFolder(); err == nil {
		t.Fatal("adding a folder was allowed with no project focused")
	}
}

// "This project's bug comes from that library" is a fact about one project. A
// folder added to one must not follow the user into the next.
func TestWorkspaceFoldersAreStoredPerProject(t *testing.T) {
	a := seed(&App{dbDir: t.TempDir()}, newConversation())
	closeDBOnCleanup(t, a)
	projectA := filepath.Join(t.TempDir(), "a")
	projectB := filepath.Join(t.TempDir(), "b")
	shared := filepath.Join(t.TempDir(), "shared")

	if err := a.storeWorkspaceFolder(projectA, shared); err != nil {
		t.Fatalf("store: %v", err)
	}
	if got := a.storedWorkspaceFolders(projectA); len(got) != 1 || got[0] != shared {
		t.Errorf("project A lost its folder: %v", got)
	}
	if got := a.storedWorkspaceFolders(projectB); len(got) != 0 {
		t.Errorf("project B inherited a folder it never had: %v", got)
	}
}

// Leaving a project leaves its folders behind. Carrying them into unfocused
// mode would be harmless (that mode reaches everything) right up until someone
// focuses the next project from there and the list is still populated.
func TestClearingFocusDropsTheFolderList(t *testing.T) {
	isolateUserDirs(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "shared-lib")
	for _, dir := range []string{root, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := focusedApp(t, root)
	if _, err := a.addWorkspaceFolder(other); err != nil {
		t.Fatalf("adding a folder failed: %v", err)
	}

	a.focusNone()
	if got := a.WorkspaceFolders(); len(got) != 0 {
		t.Errorf("folder list survived leaving the project: %+v", got)
	}
	// Still on disk, so focusing the project again brings it back.
	if got := a.storedWorkspaceFolders(root); len(got) != 1 {
		t.Errorf("the stored list should outlive the session: %v", got)
	}
}
