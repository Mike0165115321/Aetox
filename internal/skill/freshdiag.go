package skill

// The self-check after a change (owner approved 30 ส.ค., the shape is
// opencode's): when edit/edits/write touches a file a language server speaks,
// that file's fresh ERRORS ride back inside the tool result itself — the
// model sees the break in the same breath as the change that made it, instead
// of three turns later when something else trips over it.
//
// Measured reason this is automatic rather than taught: `diagnostics` existed
// for weeks as a tool the model could call after editing, and was called 9
// times against thousands of edits. A behaviour the loop needs on every edit
// cannot live in the model's discretion — it lives in the tool, and costs the
// prompt nothing.
//
// Errors only. Warnings and hints are opinions the user may not share, and a
// result that appends four style nags to every honest edit teaches the model
// to stop reading the appendix at all. Silence means "nothing broken", the
// same grammar the git badge and the clean file use.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mikedev115/Aetox/internal/lsp"
)

// autoDiagTimeout is deliberately tighter than the diagnostics tool's own 20s:
// this runs uninvited inside somebody's edit, and an edit that stalls is worse
// than a check that quietly gave up. The shared client keeps servers warm, so
// after the first touch of a session the answer is usually immediate.
const autoDiagTimeout = 8 * time.Second

// maxAutoDiagLines caps the appendix. Ten errors say "this file is broken" as
// well as forty do, at a quarter of the context.
const maxAutoDiagLines = 10

// workspaceMarkers are the files whose presence makes a folder something a
// language server can honestly index. A bare directory holding a .go file is
// not a workspace — diagnosing it starts a server that has nothing to stand
// on, holds the folder open, and (measured, 30 ส.ค.) turned every edit test's
// TempDir cleanup into "file in use". Real projects have one of these; the
// scratch folders that must not spawn servers do not.
var workspaceMarkers = []string{
	".git", "go.mod", "package.json", "tsconfig.json", "pyproject.toml",
	"requirements.txt", "Cargo.toml", "composer.json", "Gemfile",
}

var workspaceCache sync.Map // root -> bool

func looksLikeWorkspace(root string) bool {
	if v, ok := workspaceCache.Load(root); ok {
		return v.(bool)
	}
	is := false
	for _, m := range workspaceMarkers {
		if _, err := os.Stat(filepath.Join(root, m)); err == nil {
			is = true
			break
		}
	}
	workspaceCache.Store(root, is)
	return is
}

// warmedRoots marks workspaces whose server has been ASKED to start. The
// first change in a session skips its own appendix and warms the server in
// the background instead: gopls spends seconds indexing on first contact, and
// an edit that stalls behind that reads as the tool hanging. From the second
// change on, the server is hot and the check costs milliseconds.
var warmedRoots sync.Map // root+ext -> bool

// appendFreshDiagnostics decorates a successful file-change Output with the
// file's current errors, when a language server for it is ALREADY installed —
// this path never triggers an install (lsp.Installed vs Available, see there).
// On any failure it returns the output untouched: the edit succeeded, and this
// appendix must never turn that into anything else.
func appendFreshDiagnostics(ctx context.Context, root, path string, out Output) Output {
	if !out.Success || root == "" || path == "" {
		return out
	}
	if !lsp.Configured(path) || !lsp.Installed(path) || !looksLikeWorkspace(root) {
		return out
	}
	warmKey := root + "|" + strings.ToLower(filepath.Ext(path))
	if _, warmed := warmedRoots.LoadOrStore(warmKey, true); !warmed {
		go func() {
			warmCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_, _ = lsp.Shared(root).Diagnose(warmCtx, path, 60*time.Second)
		}()
		return out
	}
	diagCtx, cancel := context.WithTimeout(ctx, autoDiagTimeout)
	defer cancel()
	diags, err := lsp.Shared(root).Diagnose(diagCtx, path, autoDiagTimeout)
	if err != nil {
		return out
	}
	var errs []lsp.Diagnostic
	for _, d := range diags {
		if d.Severity == "error" {
			errs = append(errs, d)
		}
	}
	if len(errs) == 0 {
		return out
	}
	var b strings.Builder
	b.WriteString(out.Content)
	fmt.Fprintf(&b, "\n\n[%d error(s) in %s after the change]\n", len(errs), path)
	for i, d := range errs {
		if i == maxAutoDiagLines {
			fmt.Fprintf(&b, "... and %d more\n", len(errs)-i)
			break
		}
		b.WriteString(d.String() + "\n")
	}
	out.Content = strings.TrimRight(b.String(), "\n")
	out.Problems = len(errs)
	return out
}
