// The parsers: one honest AST where the stdlib provides it, line scans
// everywhere else. Each returns the file's symbols and the reference targets
// its imports point at — a rel file path for scripts, a rel package directory
// for Go — and never an error: a file the scan cannot read is a file with
// nothing on the map, which is what it would earn anyway.
package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type language int

const (
	langNone language = iota
	langGo
	langScript // ts/tsx/js/jsx/mjs/svelte/vue — one scanner, they share the shapes that matter
	langPython
	langMarkdown
	// The rest of the mainstream languages ride one generic line scanner each,
	// configured by a pattern pair (declRules below). Owner's call, 29 ส.ค.:
	// "ทำให้รองรับทุกภาษาไปเลย" — and the honest version of "every language"
	// without surrendering the pure-Go build or the installer's size is every
	// language a declaration-shaped line scan can carry. What a scan misses is
	// a line of the map; what tree-sitter's grammars would cost is megabytes
	// in a binary whose size is a number the README competes on.
	langJVM   // java, kotlin, scala, dart, c# — brace languages with class/interface and modifier-prefixed members
	langRust  // rs — keyword-led declarations, pub is the visibility
	langPHP   // php
	langRuby  // rb — def/class/module, indentation-agnostic
	langSwift // swift
	langC     // c/h/cpp/hpp — types firmly, function definitions by heuristic
)

func languageOf(path string) language {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return langGo
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".svelte", ".vue":
		return langScript
	case ".py":
		return langPython
	case ".md":
		// Markdown is here because the baseline said so: half the biggest
		// whole-file reads in the measured week were .md, and a map that only
		// outlines code would leave the largest files it was built to shrink
		// invisible (BASELINE.md).
		return langMarkdown
	case ".java", ".kt", ".kts", ".scala", ".cs", ".dart":
		return langJVM
	case ".rs":
		return langRust
	case ".php":
		return langPHP
	case ".rb":
		return langRuby
	case ".swift":
		return langSwift
	case ".c", ".h", ".cpp", ".hpp", ".cc", ".hh", ".cxx":
		return langC
	}
	return langNone
}

func parse(lang language, rel string, src []byte, goModule string) ([]Symbol, []string) {
	switch lang {
	case langGo:
		return parseGo(rel, src, goModule)
	case langScript:
		return parseScript(rel, src)
	case langPython:
		return parsePython(rel, src)
	case langMarkdown:
		return parseMarkdown(src), nil
	case langJVM, langRust, langPHP, langRuby, langSwift, langC:
		return scanDeclarations(src, declRules[lang]), nil
	}
	return nil, nil
}

// parseGo uses the real AST — it is free, and Go is the language this map will
// be judged on first. Symbols are top-level funcs and type specs, rendered as
// the source line they start on: the line already IS the signature the way its
// author wrote it, and re-printing from the AST could only differ from it.
// Var/const groups are left out — constants are churn, and what other files
// call is funcs and types.
func parseGo(rel string, src []byte, goModule string) ([]Symbol, []string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
	if f == nil {
		_ = err // a partial AST still lists what parsed; nothing here is fatal
		return nil, nil
	}
	lines := strings.Split(string(src), "\n")
	lineAt := func(pos token.Pos) (int, string, bool) {
		line := fset.Position(pos).Line
		if line < 1 || line > len(lines) {
			return 0, "", false
		}
		return line, signatureLine(lines[line-1]), true
	}
	var symbols []Symbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if line, text, ok := lineAt(d.Pos()); ok {
				symbols = append(symbols, Symbol{Line: line, Text: text, Public: d.Name.IsExported()})
			}
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			// Spec position, not decl position: inside a `type (...)` group the
			// decl line says only "type (" while each spec line is a signature.
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if line, text, ok := lineAt(ts.Pos()); ok {
					if !strings.HasPrefix(text, "type ") {
						text = "type " + text
					}
					symbols = append(symbols, Symbol{Line: line, Text: text, Public: ts.Name.IsExported()})
				}
			}
		}
	}
	var targets []string
	if goModule != "" {
		prefix := goModule + "/"
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if rest, ok := strings.CutPrefix(p, prefix); ok {
				targets = append(targets, rest)
			}
		}
	}
	return symbols, targets
}

// The script shapes worth a map line: declared functions and classes,
// arrow/function assignments, and the type-level names other files import.
// Anchored to line starts (with export/async noise allowed) so a callback
// three levels deep does not read as API.
var (
	scriptDefRe   = regexp.MustCompile(`^\s{0,2}(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function|class)\s+[A-Za-z_$][\w$]*`)
	scriptConstRe = regexp.MustCompile(`^\s{0,2}(?:export\s+)?(?:const|let|var)\s+[A-Za-z_$][\w$]*(?:\s*:[^=]{0,60})?\s*=\s*(?:async\s*)?(?:\(|function\b|[A-Za-z_$][\w$]*\s*=>)`)
	scriptTypeRe  = regexp.MustCompile(`^\s{0,2}(?:export\s+)?(?:interface|type|enum)\s+[A-Za-z_$][\w$]*`)
	// Both import forms that carry a RELATIVE path — only those are edges
	// inside the repo; a bare package name is a dependency, not a file.
	scriptImportRe = regexp.MustCompile(`(?:from\s+|require\(|import\()['"](\.\.?/[^'"]+)['"]`)
	// The aliased spelling every Next/Vite/Nuxt project actually writes:
	// `@/components/Card`, `~/lib/db`. Where the alias points is tsconfig's
	// secret, so the path after it resolves the way Python absolutes do — by
	// suffix against the files that exist. The first real frontend this map
	// met wrote nothing BUT these, and drew as sixty dots with no lines.
	scriptAliasImportRe = regexp.MustCompile(`(?:from\s+|require\(|import\()['"][@~]/([^'"]+)['"]`)
)

func parseScript(rel string, src []byte) ([]Symbol, []string) {
	var symbols []Symbol
	var targets []string
	dir := filepath.ToSlash(filepath.Dir(rel))
	for i, line := range strings.Split(string(src), "\n") {
		if scriptDefRe.MatchString(line) || scriptConstRe.MatchString(line) || scriptTypeRe.MatchString(line) {
			public := strings.HasPrefix(strings.TrimSpace(line), "export ")
			symbols = append(symbols, Symbol{Line: i + 1, Text: signatureLine(line), Public: public})
		}
		for _, m := range scriptImportRe.FindAllStringSubmatch(line, -1) {
			if t := resolveScriptImport(dir, m[1]); t != "" {
				targets = append(targets, t)
			}
		}
		for _, m := range scriptAliasImportRe.FindAllStringSubmatch(line, -1) {
			targets = append(targets, filepath.ToSlash(filepath.Clean(m[1])))
		}
	}
	return symbols, targets
}

// resolveScriptImport turns "./x" written in dir into a repo-relative path,
// extensionless the way imports are written. The extension is tried at
// resolve-ref time by matching against files that exist — here it only
// normalizes, because this package has the file list in one place and it is
// not this one.
func resolveScriptImport(dir, imp string) string {
	joined := filepath.ToSlash(filepath.Join(dir, imp))
	if joined == "." || strings.HasPrefix(joined, "../") {
		return "" // escaped the mapped root: not an edge this map can see
	}
	return joined
}

var (
	pythonDefRe = regexp.MustCompile(`^(?:async\s+)?(?:def|class)\s+\w+`)
	// Both spellings of a Python import that can point INSIDE the repo. The
	// captures are the dots-for-levels prefix and the module path; a bare
	// `import requests` has no repo file to be an edge to and resolves to
	// nothing, which is correct rather than lenient.
	pythonImportRe = regexp.MustCompile(`^\s*(?:from\s+(\.*)([\w.]*)\s+import|import\s+(\.*)([\w.]+))`)
	mdHeadingRe    = regexp.MustCompile(`^#{1,4}\s+\S`)
)

func parsePython(rel string, src []byte) ([]Symbol, []string) {
	var symbols []Symbol
	var targets []string
	dir := filepath.ToSlash(filepath.Dir(rel))
	for i, line := range strings.Split(string(src), "\n") {
		// Top level only: an indented def is implementation, and a map that
		// lists every method reads like the file it was supposed to replace.
		if pythonDefRe.MatchString(line) {
			name := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(line), "async "), "def "))
			public := !strings.HasPrefix(strings.TrimPrefix(name, "class "), "_")
			symbols = append(symbols, Symbol{Line: i + 1, Text: signatureLine(line), Public: public})
		}
		if m := pythonImportRe.FindStringSubmatch(line); m != nil {
			dots, module := m[1], m[2]
			if module == "" && m[4] != "" {
				dots, module = m[3], m[4]
			}
			if t := resolvePythonImport(dir, dots, module); t != "" {
				targets = append(targets, t)
			}
		}
	}
	return symbols, targets
}

// resolvePythonImport turns "from app.db.gate import x" (or "from ..util
// import y") written in dir into a repo-relative, extensionless module path.
//
// Relative imports resolve exactly: N dots is N-1 directories up from the
// importer. Absolute ones cannot — Python finds them via sys.path, which this
// package rightly knows nothing about — so the module path is emitted as
// written ("app/db/gate") and resolveTargetFile tries it against every
// ancestor of files that exist, which is the same walk the interpreter's
// package roots amount to in a normal repo layout.
func resolvePythonImport(dir, dots, module string) string {
	modPath := strings.ReplaceAll(module, ".", "/")
	if dots == "" {
		if modPath == "" {
			return ""
		}
		return modPath
	}
	up := dir
	for i := 1; i < len(dots); i++ {
		up = filepath.ToSlash(filepath.Dir(up))
	}
	joined := filepath.ToSlash(filepath.Join(up, modPath))
	if joined == "." || strings.HasPrefix(joined, "../") {
		return ""
	}
	return joined
}

func parseMarkdown(src []byte) []Symbol {
	var symbols []Symbol
	fenced := false
	for i, line := range strings.Split(string(src), "\n") {
		// A ``` fence flips code mode; a # inside one is a shell comment, not a
		// heading, and mapping it would invent structure the document lacks.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if !fenced && mdHeadingRe.MatchString(line) {
			// The top two levels are a document's table of contents; deeper
			// headings are its prose structure and go first when room runs out.
			public := !strings.HasPrefix(line, "###")
			symbols = append(symbols, Symbol{Line: i + 1, Text: signatureLine(line), Public: public})
		}
	}
	return symbols
}

// declRule drives the generic scanner: which lines are declarations, and
// which of those the language considers public. One rule set per family
// rather than per keyword, because what a MAP needs from Java and what it
// needs from Rust is the same two questions with different spellings.
type declRule struct {
	decl   *regexp.Regexp
	public *regexp.Regexp
}

var declRules = map[language]declRule{
	// Brace languages with modifier-prefixed members. Types always; methods
	// only when a visibility modifier anchors the line, because `if (x) {` and
	// a method signature are otherwise the same shape to a line scan.
	langJVM: {
		decl:   regexp.MustCompile(`^\s{0,4}(?:@\w+\s+)?(?:public|protected|private|internal|open|sealed|abstract|final|static|data|record|export)?\s*(?:static\s+|final\s+|abstract\s+|partial\s+|async\s+)*(?:class|interface|enum|record|object|struct|trait|fun|void|[A-Z][\w<>\[\], ]*)\s+\w+\s*[({<]`),
		public: regexp.MustCompile(`^\s*(?:@\w+\s+)?(?:public|open|export)`),
	},
	langRust: {
		decl:   regexp.MustCompile(`^\s{0,4}(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?(?:fn|struct|enum|trait|impl|mod|type|const)\s+\S`),
		public: regexp.MustCompile(`^\s*pub\b`),
	},
	langPHP: {
		decl:   regexp.MustCompile(`^\s{0,4}(?:abstract\s+|final\s+)?(?:class|interface|trait|enum)\s+\w+|^\s*(?:public\s+|protected\s+|private\s+|static\s+)*function\s+\w+`),
		public: regexp.MustCompile(`^\s*(?:class|interface|trait|enum|public|function)`),
	},
	langRuby: {
		decl:   regexp.MustCompile(`^\s{0,4}(?:def|class|module)\s+\S`),
		public: regexp.MustCompile(`^\s{0,4}(?:class|module)\b|^\s{0,4}def\s+[^_]`),
	},
	langSwift: {
		decl:   regexp.MustCompile(`^\s{0,4}(?:@\w+\s+)?(?:public\s+|private\s+|internal\s+|open\s+|fileprivate\s+)?(?:static\s+|final\s+)?(?:func|class|struct|enum|protocol|extension|actor)\s+\S`),
		public: regexp.MustCompile(`^\s*(?:@\w+\s+)?(?:public|open)`),
	},
	// C has no modifiers to anchor on, so: type keywords firmly, and function
	// DEFINITIONS by the one shape a declaration cannot have — a paren line at
	// column zero that does not end in a semicolon.
	langC: {
		decl:   regexp.MustCompile(`^(?:typedef\s+)?(?:struct|class|enum|union)\s+\w+|^[A-Za-z_][\w:<>*&, ]*\s+\**[A-Za-z_]\w*\([^;]*$`),
		public: regexp.MustCompile(`^`),
	},
}

// scanDeclarations is the whole parser for the declRules languages. It finds
// less than an AST would — that is the trade the pure-Go build buys — and what
// it finds is exactly what the map prints: the line, as written, numbered.
func scanDeclarations(src []byte, rule declRule) []Symbol {
	var symbols []Symbol
	for i, line := range strings.Split(string(src), "\n") {
		if !rule.decl.MatchString(line) {
			continue
		}
		symbols = append(symbols, Symbol{
			Line:   i + 1,
			Text:   signatureLine(line),
			Public: rule.public.MatchString(line),
		})
	}
	return symbols
}

// signatureLine is the one line as its author wrote it, minus the noise a map
// has no room for: leading indent, a trailing brace about to open a body, and
// anything past the char cap.
func signatureLine(line string) string {
	text := strings.TrimSpace(line)
	text = strings.TrimSuffix(text, "{")
	text = strings.TrimSpace(text)
	if len(text) > maxSymbolChars {
		text = text[:maxSymbolChars] + "…"
	}
	return text
}
