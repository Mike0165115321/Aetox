package runlang

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTableIsWellFormed guards the one thing that gets a language added wrong:
// the next entry is written by copying a neighbour, and a Script that forgot
// its Ext or its Interpreters would produce a button that runs `"" file` or
// writes a file with no extension.
func TestTableIsWellFormed(t *testing.T) {
	seen := map[string]string{}
	for _, l := range languages {
		if len(l.Tags) == 0 {
			t.Errorf("%v: a language with no tags can never be named by a fence", l)
		}
		for _, tag := range l.Tags {
			if tag != strings.ToLower(tag) {
				t.Errorf("tag %q is not lowercase, so Lookup can never match it", tag)
			}
			if other, dup := seen[tag]; dup {
				t.Errorf("tag %q is claimed by two languages (%s and %s) — "+
					"which one runs it would depend on table order", tag, other, l.Kind)
			}
			seen[tag] = string(l.Kind)
		}

		switch l.Kind {
		case Script:
			if len(l.Interpreters) == 0 {
				t.Errorf("%v is a Script with nothing to run it", l.Tags)
			}
			if l.Ext == "" || l.Ext[0] != '.' {
				t.Errorf("%v has Ext %q, want something like \".py\"", l.Tags, l.Ext)
			}
			for _, i := range l.Interpreters {
				if i.Name == "" {
					t.Errorf("%v has an interpreter with no name", l.Tags)
				}
			}
		case Shell:
			if len(l.Interpreters) != 0 || l.Ext != "" {
				t.Errorf("%v is a Shell, whose text is handed over as written — "+
					"an interpreter or an extension on it would never be used", l.Tags)
			}
		default:
			t.Errorf("%v has kind %q, which is neither of the two ways a block can be read", l.Tags, l.Kind)
		}
	}
}

func TestLookupReadsTheTagTheModelTyped(t *testing.T) {
	for _, tag := range []string{"python", "Python", " PY ", "js", "JavaScript", "go", "bash"} {
		if _, ok := Lookup(tag); !ok {
			t.Errorf("Lookup(%q) found nothing", tag)
		}
	}
	// Neither a command nor a program: a transcript with a `$` in it is not a
	// thing to run, and running markup means nothing.
	for _, tag := range []string{"", "console", "text", "json", "yaml", "html", "css", "sql", "md", "ts"} {
		if _, ok := Lookup(tag); ok {
			t.Errorf("Lookup(%q) offered to run it", tag)
		}
	}
}

func TestCommandLineCarriesTheInterpretersOwnArguments(t *testing.T) {
	py, _ := Lookup("python")
	if got, want := (Interpreter{Name: "python"}).CommandLine("a b.py"), `python "a b.py"`; got != want {
		t.Errorf("CommandLine = %q, want %q", got, want)
	}
	if py.Ext != ".py" {
		t.Errorf("python Ext = %q", py.Ext)
	}

	// Go is the entry that proves Args exist for a reason: `go x.go` is not a
	// command, and a table that only held program names could not say so.
	golang, _ := Lookup("go")
	if len(golang.Interpreters) == 0 {
		t.Fatal("go has no interpreters")
	}
	if got, want := golang.Interpreters[0].CommandLine("blocks/x.go"), `go run "blocks/x.go"`; got != want {
		t.Errorf("CommandLine = %q, want %q", got, want)
	}
}

// TestRunnableOffersShellsAlwaysAndScriptsOnlyWhenPresent is the rule the whole
// package exists for: a button that cannot do anything is never offered.
func TestRunnableOffersShellsAlwaysAndScriptsOnlyWhenPresent(t *testing.T) {
	got := Runnable()

	for _, l := range languages {
		_, present := l.Interpreter()
		want := l.Kind == Shell || present
		for _, tag := range l.Tags {
			kind, offered := got[tag]
			if offered != want {
				t.Errorf("tag %q offered=%v, want %v (kind %s, interpreter present %v)",
					tag, offered, want, l.Kind, present)
			}
			if offered && kind != l.Kind {
				t.Errorf("tag %q reported kind %q, want %q", tag, kind, l.Kind)
			}
		}
	}

	// Shell tags are the ones that can never be missing, so they are also the
	// ones this test can assert about absolutely.
	for _, tag := range []string{"bash", "sh", "powershell"} {
		if got[tag] != Shell {
			t.Errorf("%q is not offered as a shell tag; every machine has a shell", tag)
		}
	}
}

// TestInstalledRejectsAZeroByteProgram is the Microsoft Store stub, reduced to
// what it is. Windows ships python.exe and python3.exe as zero-byte app
// execution aliases on PATH, exec.LookPath finds them, and running one prints
// an advert and exits 9009 — so "is Python installed" answered yes on a
// machine with no Python.
func TestInstalledRejectsAZeroByteProgram(t *testing.T) {
	dir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	stub := filepath.Join(dir, "stubbed"+ext)
	if err := os.WriteFile(stub, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	genuine := filepath.Join(dir, "realish"+ext)
	if err := os.WriteFile(genuine, []byte("MZ not really, but it has a size"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if installed("stubbed") {
		t.Error("a zero-byte program counted as installed — this is the Store stub, and the Run button would be offered for an interpreter that only prints an advert")
	}
	if !installed("realish") {
		t.Error("a real program on PATH did not count as installed")
	}
	if installed("nothing-by-this-name") {
		t.Error("a name that is not on PATH counted as installed")
	}
}
