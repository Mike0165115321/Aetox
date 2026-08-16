// Package runlang owns one question and answers it in one place: which fenced
// code blocks can this machine run, and with what.
//
// It exists because that answer used to live in two files that agreed only by
// somebody remembering to edit both. A hardcoded set in the chat's markdown
// renderer decided whether the Run button appeared; a table in the desktop
// package decided whether anything happened when it was clicked. Neither asked
// the machine, so the button was offered for interpreters that were not
// installed, and the click came back with a Microsoft Store advert instead of
// the answer the user wanted.
//
// So the table lives here, the machine is asked here, and the chat asks this
// package rather than carrying its own copy. Adding a language is one entry in
// `languages` and nothing else.
package runlang

import (
	"slices"
	"strings"
)

// Kind is how a block's text is to be treated, and there are exactly two ways.
type Kind string

const (
	// Shell means the text IS a command: it goes to the machine's shell as
	// written. Every machine has a shell, so a shell tag needs no detection —
	// which of the two kinds a tag is decides whether it can be missing at all.
	Shell Kind = "shell"

	// Script means the text is a FILE: it is written out and run through an
	// interpreter, and an interpreter is a thing a machine can be without.
	Script Kind = "script"
)

// Interpreter is one program that can run a Script language's file. Args are
// the fixed arguments that come before the file name, because `go run x.go`
// needs one where `python x.py` needs none.
type Interpreter struct {
	Name string
	Args []string
}

// Language is one row of the table: the fence tags that name it, how its text
// is treated, and — for a Script — what can run it.
//
// Interpreters are candidates in preference order, not alternatives of equal
// standing: the first one this machine actually has wins.
type Language struct {
	Tags         []string
	Kind         Kind
	Interpreters []Interpreter
	Ext          string
}

// languages is the whole table. A language earns a place here by meeting three
// things at once, and every rejection below failed one of them:
//
//  1. One command runs one file. No build step, no project scaffolding, no
//     generated artifact left in somebody's folder.
//  2. The block is a program. Running it is what the user meant by clicking
//     Run — which is why `json`, `yaml`, `html`, `css`, `sql` and `md` are not
//     here and never will be. Running markup means nothing, and `html` in
//     particular would mean "open a browser", a different act that deserves a
//     different button rather than a quiet second meaning for this one.
//  3. Failure is honest. A tag whose interpreter this machine lacks simply
//     does not get a button (see Runnable), so the table can carry a language
//     nobody here has installed without lying to anyone.
//
// Kept out on purpose:
//
// TypeScript. `ts` reads like the obvious next entry and is not one: node
// 22.16 on this machine cannot run a .ts file (measured — SyntaxError on the
// first type annotation), and tsx, ts-node and deno are all absent. It gets an
// entry the day a runtime here can run it, and not before, because a tag in
// this table that cannot run is exactly the bug the package was written to end.
//
// Java and C#. `java Foo.java` needs a JDK (this machine has an Oracle
// java8path shim) and `dotnet run` needs a project file. Both fail rule 1.
var languages = []Language{
	{
		Tags: []string{"bash", "sh", "shell", "zsh", "powershell", "ps1", "cmd", "bat"},
		Kind: Shell,
	},
	{
		Tags: []string{"python", "py"},
		Kind: Script,
		// `python3` last, and it is not a style choice: on Windows it is
		// usually the Store stub, and `python` is the name a real Windows
		// install answers to. installed() is what stops the stub either way.
		Interpreters: []Interpreter{{Name: "python"}, {Name: "py"}, {Name: "python3"}},
		Ext:          ".py",
	},
	{
		Tags:         []string{"javascript", "js", "node"},
		Kind:         Script,
		Interpreters: []Interpreter{{Name: "node"}},
		// .js rather than .mjs, so the file inherits the module kind of the
		// project it is written into — a snippet run inside somebody's package
		// should behave the way that package's own files do.
		Ext: ".js",
	},
	{
		// mjs is the tag a model writes when it means ESM regardless of what
		// the surrounding project says, and the extension is how node is told.
		Tags:         []string{"mjs"},
		Kind:         Script,
		Interpreters: []Interpreter{{Name: "node"}},
		Ext:          ".mjs",
	},
	{
		Tags: []string{"go"},
		Kind: Script,
		// `go run` takes a file, leaves no binary behind, and — checked
		// 2026-08-16 — the dot-directory these files are written into is
		// invisible to `go build ./...` and `go vet ./...`, so a block parked
		// there for the length of one run cannot break the user's own build.
		Interpreters: []Interpreter{{Name: "go", Args: []string{"run"}}},
		Ext:          ".go",
	},
}

// Lookup returns the language a fence tag names. The tag is lowered because a
// fence is whatever the model typed, and ```Python is the same language as
// ```python.
func Lookup(tag string) (Language, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return Language{}, false
	}
	for _, l := range languages {
		if slices.Contains(l.Tags, tag) {
			return l, true
		}
	}
	return Language{}, false
}

// CommandLine is the command that runs file through i.
//
// The file is quoted and the interpreter is not: the interpreter is a name
// from the table above, and the file is a path that can contain a space.
func (i Interpreter) CommandLine(file string) string {
	parts := make([]string, 0, len(i.Args)+2)
	parts = append(parts, i.Name)
	parts = append(parts, i.Args...)
	parts = append(parts, `"`+file+`"`)
	return strings.Join(parts, " ")
}

// Names lists an interpreter candidate set the way a person would read it,
// for the message a user sees when none of them is installed.
func (l Language) Names() []string {
	names := make([]string, 0, len(l.Interpreters))
	for _, i := range l.Interpreters {
		names = append(names, i.Name)
	}
	return names
}
