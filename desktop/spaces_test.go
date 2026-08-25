package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/prompt"
	"github.com/Mikedev115/Aetox/internal/safety"
)

// A โปรเจกต์ at the storefront door (COMPANY.md §84), from the outside.
//
// What these pin is the one sentence the room promises and the one thing that
// would quietly break it: a project groups chats and carries files, and it
// moves no wall. The day it starts narrowing what the assistant can reach, it
// has become the workshop's project wearing the storefront's name.

func spaceApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	a := seed(&App{ctx: context.Background(), emit: func(string, ...any) {}, dbDir: t.TempDir()}, &conversation{id: newSessionID()})
	t.Cleanup(func() {
		if a.db != nil {
			_ = a.db.Close()
		}
	})
	a.applyConfig(a.cur(), config.Config{
		SandboxRoot:   t.TempDir(),
		ModelProvider: "aetox",
		ModelName:     "aetox-tools:test",
		ApprovalMode:  string(safety.ApprovalFullAccess),
	})
	return a
}

func TestCreatingAProjectMakesTheFolderAndItsContextFolder(t *testing.T) {
	a := spaceApp(t)

	space, err := a.CreateSpace("เปิดร้านกาแฟ")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	root, _ := config.DataRoot()
	want := filepath.Join(root, "project", "เปิดร้านกาแฟ")
	if space.Path != want {
		t.Errorf("project folder is %q, want %q — the owner named this path", space.Path, want)
	}
	// The context folder exists before anything is in it: it is the instruction
	// for where files go, and an instruction that appears only after you have
	// guessed right is not one.
	if info, err := os.Stat(filepath.Join(want, "context")); err != nil || !info.IsDir() {
		t.Errorf("context folder was not created: %v", err)
	}
	// The name the user typed is the name on disk — Thai included — because the
	// whole point of putting this on the filesystem is that they can find it.
	if space.Name != "เปิดร้านกาแฟ" {
		t.Errorf("folder name is %q, want the name the user typed", space.Name)
	}
}

// The disk is the only record that a project exists, so a folder made in
// Explorer is a project and nothing has to be told about it.
func TestAFolderMadeByHandIsAProject(t *testing.T) {
	a := spaceApp(t)
	root, _ := config.DataRoot()
	if err := os.MkdirAll(filepath.Join(root, "project", "งานบ้าน", "context"), 0o755); err != nil {
		t.Fatal(err)
	}

	names := []string{}
	for _, s := range a.Spaces() {
		names = append(names, s.Name)
	}
	if len(names) != 1 || names[0] != "งานบ้าน" {
		t.Errorf("Spaces() = %v, want the folder that is on the disk", names)
	}
}

// A name from the frontend is untrusted input, and the folder it resolves to
// has to stay inside the projects folder.
func TestAProjectNameCannotReachOutOfTheProjectsFolder(t *testing.T) {
	a := spaceApp(t)
	for _, name := range []string{"../escape", "..\\escape", "a/b", `C:\Windows`, "..", ".", "   ", ""} {
		if _, err := a.CreateSpace(name); err == nil {
			t.Errorf("CreateSpace(%q) was accepted", name)
		}
	}
}

// The promise of the room, and the one test that would catch it being broken:
// a chat inside a project reaches exactly what a chat outside one reaches.
func TestAProjectMovesNoWall(t *testing.T) {
	a := spaceApp(t)
	if _, err := a.CreateSpace("แผนธุรกิจ"); err != nil {
		t.Fatal(err)
	}
	before := a.cur().cfg.SandboxRoot
	openBefore := !a.projectFocused

	if _, err := a.NewSessionInSpace("แผนธุรกิจ"); err != nil {
		t.Fatalf("NewSessionInSpace: %v", err)
	}

	if a.cur().cfg.SandboxRoot != before {
		t.Errorf("the sandbox moved to %q — a project is a folder of conversations, not a fence", a.cur().cfg.SandboxRoot)
	}
	if openBefore != !a.projectFocused {
		t.Error("opening a chat inside a project changed whether the sandbox is open")
	}
	if a.cur().space != "แผนธุรกิจ" {
		t.Errorf("the session is in space %q, want แผนธุรกิจ", a.cur().space)
	}
}

// A tool cannot ask to be used and a folder cannot announce itself: without
// this layer the assistant is in a project it has never been told about, which
// is the same failure as a drawing nobody asked for (§88).
func TestTheAssistantIsToldWhichProjectItIsIn(t *testing.T) {
	root := t.TempDir()
	scope := prompt.Scope{Root: root, Space: prompt.Space{
		Name:        "เปิดร้านกาแฟ",
		ContextPath: filepath.Join(root, "context"),
		Files:       []string{"สูตรกาแฟ.md", "ต้นทุน.xlsx"},
	}}

	got := prompt.Build(prompt.SurfaceDesktop, scope)

	for _, want := range []string{"เปิดร้านกาแฟ", "สูตรกาแฟ.md", "ต้นทุน.xlsx", filepath.Join(root, "context")} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not name %q", want)
		}
	}
	// Named, never pasted: the file list costs a line, the contents would cost
	// the user's whole context window on every turn forever.
	if !strings.Contains(got, "not included") {
		t.Error("the prompt does not say the files are named rather than included")
	}
	// And it must not read as a permission. The sandbox sentence above it is
	// the one that grants; this one only says what the conversation is about.
	if !strings.Contains(got, "does not narrow what you can reach") {
		t.Error("the prompt does not say the project narrows nothing")
	}
}

// An empty context folder is a different fact from no folder, and the more
// useful one: it is where the user's next file goes.
func TestAnEmptyProjectStillSaysWhereItsFilesGo(t *testing.T) {
	root := t.TempDir()
	got := prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: root, Space: prompt.Space{
		Name: "งานใหม่", ContextPath: filepath.Join(root, "context"),
	}})

	if !strings.Contains(got, "งานใหม่") || !strings.Contains(got, "empty so far") {
		t.Errorf("an empty project does not say so:\n%s", got)
	}
}

// A chat held outside every project must read exactly as it did before this
// feature existed — most chats are that chat.
func TestAChatOutsideEveryProjectSaysNothingAboutProjects(t *testing.T) {
	got := prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: t.TempDir()})

	if strings.Contains(got, "is being held inside the user's project") {
		t.Error("a chat in no project is told it is in one")
	}
}

// Deleting the folder deletes the project — but not the conversations held in
// it. Reopening one lands outside every project instead of refusing, because
// the folder held context to read, never rights to use.
func TestAChatSurvivesItsProjectFolderBeingDeleted(t *testing.T) {
	a := spaceApp(t)
	space, err := a.CreateSpace("ชั่วคราว")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSessionInSpace("ชั่วคราว"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(space.Path); err != nil {
		t.Fatal(err)
	}

	if got := a.resolvedSpace("ชั่วคราว"); got != "" {
		t.Errorf("a deleted project still resolves to %q", got)
	}
	if len(a.Spaces()) != 0 {
		t.Error("a deleted folder is still listed as a project")
	}
}

// A file added twice must not silently replace the one already there. The user
// asked to add a file; losing the previous one is not something they asked for,
// and it is unrecoverable.
func TestAddingTheSameFileTwiceKeepsBoth(t *testing.T) {
	a := spaceApp(t)
	space, err := a.CreateSpace("รายงาน")
	if err != nil {
		t.Fatal(err)
	}
	contextDir := filepath.Join(space.Path, "context")

	source := filepath.Join(t.TempDir(), "สรุป.md")
	if err := os.WriteFile(source, []byte("รอบแรก"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyIntoContext(source, contextDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("รอบสอง"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyIntoContext(source, contextDir); err != nil {
		t.Fatal(err)
	}

	files := a.describeSpace(space.Path, space.Name, 0).ContextFiles
	if len(files) != 2 {
		t.Fatalf("context holds %v, want both copies", files)
	}
	first, _ := os.ReadFile(filepath.Join(contextDir, "สรุป.md"))
	if string(first) != "รอบแรก" {
		t.Errorf("the first file was overwritten: %q", first)
	}
	// Set membership, not position: the list is sorted for the page, and a
	// space sorts before a dot, so the numbered copy leads.
	if !slices.Contains(files, "สรุป (2).md") {
		t.Errorf("context holds %v, want a numbered second copy beside the first", files)
	}
}

// The filename arrives from the frontend, so the delete has to resolve inside
// the project's own context folder — the frontend is not the gate.
func TestRemovingContextCannotReachOutOfTheContextFolder(t *testing.T) {
	a := spaceApp(t)
	space, err := a.CreateSpace("ป้องกัน")
	if err != nil {
		t.Fatal(err)
	}
	// A file one level up, inside the project but outside its context folder.
	outside := filepath.Join(space.Path, "อย่าลบ.md")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"../อย่าลบ.md", "..\\อย่าลบ.md", filepath.Join("..", "..", "อย่าลบ.md")} {
		if _, err := a.RemoveSpaceContext("ป้องกัน", name); err == nil {
			t.Errorf("RemoveSpaceContext(%q) was accepted", name)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("a file outside the context folder was deleted")
	}
}

func TestRemovingContextDeletesTheFileAndAnswersWithWhatIsLeft(t *testing.T) {
	a := spaceApp(t)
	space, err := a.CreateSpace("ลบได้")
	if err != nil {
		t.Fatal(err)
	}
	contextDir := filepath.Join(space.Path, "context")
	for _, name := range []string{"เก็บไว้.md", "เอาออก.md"} {
		if err := os.WriteFile(filepath.Join(contextDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	left, err := a.RemoveSpaceContext("ลบได้", "เอาออก.md")
	if err != nil {
		t.Fatalf("RemoveSpaceContext: %v", err)
	}
	if len(left) != 1 || left[0] != "เก็บไว้.md" {
		t.Errorf("what is left is %v, want only เก็บไว้.md", left)
	}
}

// The prompt is built from the folder on every bootstrap, so a file dropped in
// is known to the next session without restarting anything. If this ever needs
// a cache, the cache is what will be wrong.
func TestAFileAddedNowIsInTheNextPromptBuild(t *testing.T) {
	a := spaceApp(t)
	space, err := a.CreateSpace("บริบทสด")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSessionInSpace("บริบทสด"); err != nil {
		t.Fatal(err)
	}
	if got := a.spaceContextForPrompt(); len(got.Files) != 0 {
		t.Fatalf("a new project already reports %v", got.Files)
	}

	if err := os.WriteFile(filepath.Join(space.Path, "context", "โจทย์.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := a.spaceContextForPrompt()
	if len(got.Files) != 1 || got.Files[0] != "โจทย์.md" {
		t.Errorf("the prompt would be built with %v, want the file that is on the disk now", got.Files)
	}
}

// A project must not follow the user out of the room. Reported from the app:
// a chat started inside one, then a plain new chat from the sidebar, and the
// second chat was still filed under the project — told about its files and
// recorded on its row, though nobody opened it there.
func TestAPlainNewChatIsInNoProject(t *testing.T) {
	a := spaceApp(t)
	if _, err := a.CreateSpace("งานร้าน"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSessionInSpace("งานร้าน"); err != nil {
		t.Fatal(err)
	}
	if a.cur().space != "งานร้าน" {
		t.Fatalf("the project chat is in space %q", a.cur().space)
	}

	if _, err := a.NewSessionAt("assistant"); err != nil {
		t.Fatal(err)
	}

	if a.cur().space != "" {
		t.Errorf("a fresh chat is still in project %q — the room followed the user out of it", a.cur().space)
	}
	if got := a.spaceContextForPrompt(); got.Path != "" || len(got.Files) != 0 {
		t.Errorf("a fresh chat still carries the project's context: %+v", got)
	}
}

// A project's chats belong to that project's list and to nothing else. Mixed
// into the sidebar they read as a chat that is in two places, which is the
// thing the room was built to stop — reported as "ประวัติแชทที่คุยกันผ่าน
// โปรเจคไม่ควรไปโผล่ปนกันแบบนี้".
//
// Search is the deliberate exception, and the other half of this test: someone
// typing a word is looking for a conversation, not for a tidy list, so the row
// comes back carrying the name of the project it lives in.
func TestAProjectsChatsStayOutOfTheGeneralHistory(t *testing.T) {
	a := spaceApp(t)
	if _, err := a.CreateSpace("ร้านกาแฟ"); err != nil {
		t.Fatal(err)
	}

	if _, err := a.NewSessionInSpace("ร้านกาแฟ"); err != nil {
		t.Fatal(err)
	}
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "สูตรกาแฟเย็น"},
		SessionMessage{Role: "assistant", Text: "ได้ครับ"},
	)
	if _, err := a.NewSessionAt("assistant"); err != nil {
		t.Fatal(err)
	}
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "สรุปข่าววันนี้"},
		SessionMessage{Role: "assistant", Text: "ครับ"},
	)

	for _, list := range []struct {
		name string
		rows []SessionMeta
	}{
		{"ListSessions", a.ListSessions()},
		{"ListSessionsAt(assistant)", a.ListSessionsAt("assistant")},
		{"ListSessionsForDoor", a.ListSessionsForDoor(DeskFilter{})},
	} {
		if len(list.rows) != 1 {
			t.Errorf("%s returned %d chats, want only the one held outside the project", list.name, len(list.rows))
		}
		for _, row := range list.rows {
			if row.Title == "สูตรกาแฟเย็น" {
				t.Errorf("%s lists the project's chat", list.name)
			}
		}
	}

	// The project's own list has it, and only it.
	inSpace := a.SessionsInSpace("ร้านกาแฟ")
	if len(inSpace) != 1 || inSpace[0].Title != "สูตรกาแฟเย็น" {
		t.Errorf("the project lists %v, want its own chat", inSpace)
	}

	// And search still finds it, saying where it lives.
	hits := a.SearchSessionsForDoor("กาแฟ", DeskFilter{})
	if len(hits) != 1 {
		t.Fatalf("search returned %d hits, want the project's chat", len(hits))
	}
	if hits[0].Space != "ร้านกาแฟ" {
		t.Errorf("the search result does not say which project it is in: %+v", hits[0])
	}
}

// The pair the user came for — a script and what it produced — has to land in
// one place. A script writes where its own text says, so the prompt is the only
// thing that can keep the two together.
//
// This used to require the literal "$PSScriptRoot", which pinned a prompt built
// once for every session to one shell's spelling of the idea. It was wrong for
// half of them: a project whose commands run in a distro has no $PSScriptRoot,
// and this machine has one of each.
//
// Naming the shell is not this file's job, and doing it here would be a second
// source for a fact that already has a better one. The shell tool's description
// is assembled from the backend that will actually run the command — "Commands
// are run by bash (WSL: mikedev) on this machine, so write them in that shell's
// syntax" — so the model is told which language it is writing in by the thing
// that knows, and how that language names a script's own directory follows.
// What belongs here is the instruction that does not vary: where the output has
// to end up.
func TestTheAssistantIsToldWhereAScriptsOutputGoes(t *testing.T) {
	got := prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: t.TempDir(), Open: true})

	// The failure, the fix, and the way out.
	for _, want := range []string{"hardcoded", "write beside itself", "take the output path as an argument"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not say where a script's own output belongs (%q missing)", want)
		}
	}
	// And no shell's own word for it. Whichever one were written here would be
	// the wrong instruction for every session running the other (§99: the
	// prompt teaches what generalizes, and never keeps a case list).
	for _, idiom := range []string{"$PSScriptRoot", "PowerShell", "$0", "__file__", "dirname"} {
		if strings.Contains(got, idiom) {
			t.Errorf("the prompt names %q — one shell's answer, in a prompt built for all of them", idiom)
		}
	}
}

func TestTwoProjectsCannotShareAName(t *testing.T) {
	a := spaceApp(t)
	if _, err := a.CreateSpace("ซ้ำ"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateSpace("ซ้ำ"); err == nil {
		t.Error("the second project with the same name was created, silently over the first")
	}
}

// The room's own list: chats are grouped by the project they were held in, and
// a chat held in no project is in nobody's group.
func TestChatsAreListedUnderTheProjectTheyWereHeldIn(t *testing.T) {
	a := spaceApp(t)
	if _, err := a.CreateSpace("กลุ่มเอ"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSessionInSpace("กลุ่มเอ"); err != nil {
		t.Fatal(err)
	}
	a.appendTurn(a.cur(),
		SessionMessage{Role: "user", Text: "สวัสดี"},
		SessionMessage{Role: "assistant", Text: "ครับ"},
	)

	inSpace := a.SessionsInSpace("กลุ่มเอ")
	if len(inSpace) != 1 {
		t.Fatalf("the project lists %d chats, want 1", len(inSpace))
	}
	if got := a.SessionsInSpace("กลุ่มที่ไม่มีอยู่"); len(got) != 0 {
		t.Errorf("a project that does not exist lists %d chats", len(got))
	}
}
