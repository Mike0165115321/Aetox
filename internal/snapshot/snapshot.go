// Package snapshot is the undo Aetox never had.
//
// Until now the only thing standing between an agent and a bad edit was the
// approval prompt in front of it — and approval is a judgement made before
// seeing the result. When the result is wrong, the user's own git history is
// the whole safety net, which works exactly as well as they commit.
//
// The design is opencode's, read from its source rather than guessed at, and
// it earns its place by adding nothing:
//
//   - A **shadow git repository** per project, living in Aetox's data root with
//     its work tree pointed at the real project. Snapshots therefore never
//     touch the user's .git, their index, their stash or their reflog. Nothing
//     Aetox does here can show up in `git status`.
//   - A snapshot is a **git tree object**, taken with `write-tree`. Not a copy
//     of the files and not a diff format of our own — git already stores a
//     directory state by content hash, deduplicated, and one command produces
//     it.
//   - `alternates` links the shadow store to the project's real object store,
//     so a file already committed is never stored twice. Only what git does not
//     already have gets written.
//
// The result needs no new storage format, no database, no daemon, and no
// dependency the project does not already have — internal/skill/git.go has been
// shelling out to git since long before this.
package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/proc"
)

// ErrUnavailable is returned when snapshots cannot work here at all: no git on
// the machine, or a project that is not a git repository. Both are ordinary
// conditions, not failures — every caller is expected to carry on without
// undo rather than refuse to work.
var ErrUnavailable = errors.New("snapshots are unavailable for this project")

// Store owns the shadow repository for one project work tree.
type Store struct {
	workTree string
	gitDir   string

	mu    sync.Mutex
	ready bool
}

// New prepares a store for workTree. It does no work until the first Capture,
// because most turns never touch a file and paying git's init cost for them
// would be a startup tax on every session.
func New(workTree string) (*Store, error) {
	workTree = strings.TrimSpace(workTree)
	if workTree == "" {
		return nil, ErrUnavailable
	}
	abs, err := filepath.Abs(workTree)
	if err != nil {
		return nil, ErrUnavailable
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrUnavailable
	}
	root, err := config.DataRoot()
	if err != nil {
		return nil, ErrUnavailable
	}
	// Keyed by a hash of the path, not by the folder name: two projects called
	// "app" in different places are different projects, and a path is not a
	// legal directory name on any OS.
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.ToSlash(abs))))
	return &Store{
		workTree: abs,
		gitDir:   filepath.Join(root, "snapshots", hex.EncodeToString(sum[:8])),
	}, nil
}

// WorkTree is the project this store watches. Exported for callers that need
// to name it — a test writing fixtures, a UI showing what undo would touch.
func (s *Store) WorkTree() string { return s.workTree }

// git runs one plumbing command against the shadow repo.
//
// Every call is explicit about both directories. --git-dir is the shadow store;
// --work-tree is the user's real project, which is what makes a `write-tree`
// here describe their files and a `checkout` here restore them.
func (s *Store) git(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"--git-dir=" + s.gitDir, "--work-tree=" + s.workTree}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = s.workTree
	// The shadow index must never be confused with the project's own, and a
	// stray GIT_INDEX_FILE in the environment would do exactly that.
	cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(s.gitDir, "index"))
	proc.HideConsole(cmd)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, text)
	}
	return text, nil
}

// prepare creates the shadow repository once per store.
func (s *Store) prepare(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return nil
	}
	if err := os.MkdirAll(s.gitDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.gitDir, "HEAD")); err != nil {
		cmd := exec.CommandContext(ctx, "git", "init", "--bare", "--quiet", s.gitDir)
		proc.HideConsole(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git init: %w: %s", err, strings.TrimSpace(string(out)))
		}
		// autocrlf off: a snapshot must restore the bytes that were there, not
		// a line-ending translation of them.
		for _, kv := range [][2]string{
			{"core.autocrlf", "false"},
			{"core.longpaths", "true"},
			{"index.version", "4"},
		} {
			if _, err := s.git(ctx, "config", kv[0], kv[1]); err != nil {
				return err
			}
		}
	}
	if err := s.linkAlternates(); err != nil {
		return err
	}
	s.ready = true
	return nil
}

// linkAlternates points the shadow store at the project's own objects, so a
// file that is already committed is never stored a second time. Best effort:
// without it snapshots still work, they just cost more disk.
func (s *Store) linkAlternates() error {
	projectObjects := filepath.Join(s.workTree, ".git", "objects")
	if info, err := os.Stat(projectObjects); err != nil || !info.IsDir() {
		return nil // a work tree without .git/objects (a submodule, a worktree file)
	}
	infoDir := filepath.Join(s.gitDir, "objects", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return nil
	}
	return os.WriteFile(filepath.Join(infoDir, "alternates"), []byte(projectObjects+"\n"), 0o644)
}

// Capture records the current state of the work tree and returns its id — a git
// tree SHA. Two captures of an unchanged tree return the same id, which is what
// makes "did this turn change anything" answerable by string comparison.
func (s *Store) Capture(ctx context.Context) (string, error) {
	if err := s.prepare(ctx); err != nil {
		return "", err
	}
	// --all stages deletions as well as edits, so a snapshot describes the tree
	// as it is rather than as it was plus additions. .gitignore is honoured by
	// git itself: build output and node_modules are not part of a snapshot for
	// the same reason they are not part of a commit.
	if _, err := s.git(ctx, "add", "--all", "."); err != nil {
		return "", err
	}
	id, err := s.git(ctx, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(id), nil
}

// Changed lists the files that differ between two snapshots, as paths relative
// to the work tree. This is what a user is shown before deciding to undo.
func (s *Store) Changed(ctx context.Context, from, to string) ([]string, error) {
	if err := s.prepare(ctx); err != nil {
		return nil, err
	}
	out, err := s.git(ctx, "diff", "--name-only", from, to)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	return strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n"), nil
}

// Restore puts the named files back to how they were in the snapshot, and
// returns what it actually touched.
//
// Per file, not per tree, and that is the whole point: undoing one bad edit
// should not throw away the four good ones made beside it. A file that did not
// exist in the snapshot is deleted, because "restore to before" has to include
// "before it existed" or a created file can never be undone.
//
// Passing no files restores every file that changed between the snapshot and
// now.
func (s *Store) Restore(ctx context.Context, id string, files []string) ([]string, error) {
	if err := s.prepare(ctx); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		current, err := s.Capture(ctx)
		if err != nil {
			return nil, err
		}
		if files, err = s.Changed(ctx, id, current); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, nil
	}

	inSnapshot, err := s.filesIn(ctx, id)
	if err != nil {
		return nil, err
	}
	restored := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(filepath.ToSlash(file))
		if file == "" {
			continue
		}
		// Containment is checked here rather than trusted from the caller: this
		// writes to and deletes from the user's real project.
		full := filepath.Join(s.workTree, filepath.FromSlash(file))
		if rel, relErr := filepath.Rel(s.workTree, full); relErr != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if !inSnapshot[file] {
			// It did not exist then. Undoing its creation means removing it.
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return restored, err
			}
			restored = append(restored, file)
			continue
		}
		if _, err := s.git(ctx, "checkout", id, "--", file); err != nil {
			return restored, err
		}
		restored = append(restored, file)
	}
	return restored, nil
}

// filesIn is the snapshot's own file list, used to tell "changed" from
// "created" — the two need opposite treatment and git does not distinguish them
// in a diff name list.
func (s *Store) filesIn(ctx context.Context, id string) (map[string]bool, error) {
	out, err := s.git(ctx, "ls-tree", "-r", "--name-only", id)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			set[line] = true
		}
	}
	return set, nil
}
