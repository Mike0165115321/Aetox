package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// folderToOffer decides what the card offers, and the card is the whole
// interface: whatever it names is what one click grants.
func TestFolderToOfferNamesTheFolderNotTheFile(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "shared-lib")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "api.go")
	if err := os.WriteFile(file, []byte("package api"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A file offers the folder holding it: granting one file and refusing its
	// siblings is not what anyone means by "let it see the shared library".
	if got := folderToOffer(file); got != dir {
		t.Errorf("folderToOffer(file) = %q, want %q", got, dir)
	}
	// A folder offers itself.
	if got := folderToOffer(dir); got != dir {
		t.Errorf("folderToOffer(dir) = %q, want %q", got, dir)
	}
	// A file that does not exist yet — a write to a folder outside the project —
	// still offers its folder.
	if got := folderToOffer(filepath.Join(dir, "new.go")); got != dir {
		t.Errorf("folderToOffer(missing file) = %q, want %q", got, dir)
	}
}

// A volume root is what filepath.Dir answers for a loose file at the top of a
// drive, and one click would hand over every project on it. The card must not
// be able to make that offer.
func TestFolderToOfferRefusesAVolumeRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	for _, target := range []string{root, filepath.Join(root, "notes.txt")} {
		if got := folderToOffer(target); got != widenSkipped {
			t.Errorf("folderToOffer(%q) = %q, want no offer", target, got)
		}
	}
}

// Relative and empty paths never reach the card: resolveSandboxPath hands over
// an absolute, symlink-resolved target, and anything else means a caller this
// function does not understand.
func TestFolderToOfferRefusesWhatItCannotRead(t *testing.T) {
	for _, target := range []string{"", "   ", "relative/path.go", "../climbing.go"} {
		if got := folderToOffer(target); got != widenSkipped {
			t.Errorf("folderToOffer(%q) = %q, want no offer", target, got)
		}
	}
}

// With no project focused the tools already reach the machine, so there is
// nothing a card could add — and offering to "add a folder to the project" when
// there is no project is nonsense the user would have to decode.
func TestWidenIsSilentWithNoProjectFocused(t *testing.T) {
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
	a.emit = func(string, ...any) {} // a window exists; focus is what decides here
	a.projectFocused = false

	if a.askWorkspaceWiden(a.cur(), filepath.Join(other, "api.go")) {
		t.Error("the door opened with no project focused")
	}
}

// A card needs somebody to click it. Without a window — a headless run, and
// every test that builds an engine with no frontend — the question can never be
// answered, and asking it would park the tool call forever. This is the shape
// that panicked the first time it ran, so it stays pinned.
func TestWidenIsSilentWithNoWindow(t *testing.T) {
	isolateUserDirs(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	other := filepath.Join(base, "shared-lib")
	for _, dir := range []string{root, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := focusedApp(t, root) // no emitter, no ctx: nothing is listening

	if a.askWorkspaceWiden(a.cur(), filepath.Join(other, "api.go")) {
		t.Error("the door opened with no window to ask through")
	}
	// And the tool it guards still refuses, exactly as it did before the door.
	if _, err := listPath(t, a, filepath.Join(other, "api.go")); err == nil {
		t.Error("an outside path was reachable with no window to approve it")
	}
}

// The credential stores are refused before the card is drawn, not after it is
// answered. A question whose yes cannot be honoured teaches the user that the
// questions do not mean anything.
func TestWidenNeverAsksAboutACredentialStore(t *testing.T) {
	isolateUserDirs(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir to place a credential store in")
	}
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	a := focusedApp(t, root)
	a.emit = func(string, ...any) {} // the door is live; the store is what shuts it

	// Nothing answers questions in a test, so a card here would hang rather than
	// quietly pass — which is exactly the failure this asserts against.
	if a.askWorkspaceWiden(a.cur(), filepath.Join(ssh, "id_rsa")) {
		t.Error("the door offered to add a credential store")
	}
	if store := skill.CredentialStoreAt(ssh); store == "" {
		t.Fatal("test setup is wrong: .ssh is not recognised as a credential store")
	}
}

// The two routes a folder arrives by — the panel and the card — must apply one
// set of rules, so recordWorkspaceFolder is the only place they live. If the
// panel refuses a folder, the card has to refuse the same folder.
func TestRecordWorkspaceFolderKeepsThePanelsRules(t *testing.T) {
	isolateUserDirs(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	inside := filepath.Join(root, "sub")
	missing := filepath.Join(base, "not-there")
	for _, dir := range []string{root, inside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := focusedApp(t, root)

	if err := a.recordWorkspaceFolder(inside); err == nil {
		t.Error("a folder already inside the project was accepted onto the list")
	}
	if err := a.recordWorkspaceFolder(missing); err == nil {
		t.Error("a folder that does not exist was accepted onto the list")
	}
	if len(a.workspaceRoots()) != 0 {
		t.Errorf("a refused folder still reached the list: %v", a.workspaceRoots())
	}
}

// A folder added mid-turn has to be persisted like any other, so the next
// session starts with it — the card is a faster route onto the list, not a
// different kind of grant that evaporates.
func TestWidenedFolderIsPersistedForTheNextSession(t *testing.T) {
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

	if err := a.recordWorkspaceFolder(other); err != nil {
		t.Fatalf("recordWorkspaceFolder(%s) = %v", other, err)
	}
	stored := a.storedWorkspaceFolders(root)
	if len(stored) != 1 || stored[0] != other {
		t.Errorf("stored folders = %v, want [%s]", stored, other)
	}
}

// The flag that owes the engine a rebuild is taken once. A second reload after
// the turn would rebuild the engine for nothing, which costs the user a
// re-bootstrap they did not ask for.
func TestWorkspaceDirtyIsTakenOnce(t *testing.T) {
	a := &App{}
	if a.takeWorkspaceDirty() {
		t.Error("a fresh app owed a reload")
	}
	a.markWorkspaceDirty()
	if !a.takeWorkspaceDirty() {
		t.Error("the marked reload was not owed")
	}
	if a.takeWorkspaceDirty() {
		t.Error("the same reload was owed twice")
	}
}
