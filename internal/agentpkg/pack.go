// Package agentpkg turns one worker's folder into something a stranger can
// install, and back.
//
// The standard it implements is
// docs/architecture/agent-package-standard-2026-08-08.md, v2. Three laws hold
// everything here up:
//
//  1. The folder is the whole worker. Anything that is this agent's identity
//     travels with it — its brief, its skills, its opening cards, the servers
//     it brings.
//  2. A package declares; it never grants. mcp.json and `needs:` say what is
//     wanted. `for:` on the server and on the connection stays the only thing
//     that gives, and the human at the install screen is the only one who
//     writes it.
//  3. One question has one answer on disk. mcp-servers.json remains the truth
//     about which servers this machine has; a package's mcp.json is an
//     instruction read once, at install.
//
// Export comes before install, and not because it is easier: it is the test of
// the standard. Anything that fails to travel is something still coupled to the
// app rather than to the worker.
//
// This package imports config and nothing else of ours, deliberately. The
// embedded copies of the shipped workers live in internal/subagent, which
// imports internal/skill, which must be able to stay ignorant of both — so what
// a caller hands in is an fs.FS and the question of where it came from is
// theirs.
package agentpkg

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// Options is one export.
type Options struct {
	// Name is the worker's local name — its folder. It is not the package's id
	// across machines (`package:` in the frontmatter is), and a buyer renaming
	// it on the way in is expected rather than tolerated.
	Name string
	// Sources are the folders that make up this worker, most-owned first: the
	// user's own home, then whatever shipped inside the binary under the same
	// name. First to hold a path wins, which is the rule the profile resolver
	// and OwnSkills already use — editing a shipped worker means shadowing it,
	// so an export of a half-edited one must ship the edits and the parts that
	// were never touched, as one thing.
	Sources []fs.FS
	// Servers are the tool servers currently placed on this agent, in full.
	// They are turned into the package's mcp.json, minus every secret.
	Servers []config.MCPServerConfig
}

// Result is what an export did, in the terms the seller needs to check it.
type Result struct {
	Path    string     `json:"path"`
	Name    string     `json:"name"`
	Files   int        `json:"files"`
	Servers []string   `json:"servers"`
	Asked   []AskField `json:"asked"`
	// Left names what was deliberately not packed, so that "where is my
	// MEMORY.md" has an answer on the screen that did it rather than in a
	// document nobody reads.
	Left []string `json:"left"`
}

// skipRoot are the files that never travel, by name at the package root.
//
// MEMORY.md is the one that matters, and it is not a checkbox. It is what this
// worker learned doing the seller's jobs — client names, document numbers,
// paths on somebody else's disk — and on the buyer's side it would be a
// stranger's recollections wearing the agent's face. What is sold is a
// capability, never an experience.
//
// mcp.json is skipped because it is regenerated: the truth about which servers
// this worker uses is the placement in mcp-servers.json, and a stale copy left
// in the folder by a previous install must not win over it.
var skipRoot = map[string]bool{
	strings.ToLower(config.AgentMemoryFile): true,
	strings.ToLower(config.AgentMCPFile):    true,
}

// Export writes the package at dest and reports what went into it.
func Export(dest string, opt Options) (Result, error) {
	var res Result
	name := strings.TrimSpace(opt.Name)
	if name == "" {
		return res, errors.New("ไม่รู้ว่าจะส่งออกเอเจนตัวไหน")
	}
	if strings.TrimSpace(dest) == "" {
		return res, errors.New("ไม่รู้ว่าจะเขียนไฟล์ไว้ที่ไหน")
	}
	res.Name = name
	res.Path = dest

	picked, left, err := collect(opt.Sources)
	if err != nil {
		return res, err
	}
	if _, ok := picked[config.AgentDefinitionFile]; !ok {
		return res, fmt.Errorf("โฟลเดอร์ของ %s ไม่มี %s จึงยังไม่ใช่แพ็กเกจเอเจน", name, config.AgentDefinitionFile)
	}
	res.Left = left

	declared, asked := declareMCP(opt.Servers)
	res.Asked = asked
	for _, s := range declared {
		res.Servers = append(res.Servers, s.Name)
	}

	rels := make([]string, 0, len(picked))
	for rel := range picked {
		rels = append(rels, rel)
	}
	// Sorted, not walked-in-order. A package that is byte-identical whenever
	// its contents are identical is what lets a publisher sign one later and
	// lets a buyer tell two downloads apart — and it costs one sort now against
	// a format change then.
	sort.Strings(rels)

	tmp := dest + ".partial"
	if err := writeArchive(tmp, rels, picked, declared); err != nil {
		os.Remove(tmp)
		return res, err
	}
	// Windows will not rename onto an existing file. Removing first is the
	// same gesture the user made by choosing that filename in the save dialog.
	os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return res, err
	}
	res.Files = len(rels)
	if len(declared) > 0 {
		res.Files++ // the generated mcp.json
	}
	return res, nil
}

// collect resolves the overlay: every file each source holds, first source
// winning a path that two of them have.
func collect(sources []fs.FS) (map[string]fs.FS, []string, error) {
	picked := map[string]fs.FS{}
	var left []string
	seenLeft := map[string]bool{}
	for _, src := range sources {
		if src == nil {
			continue
		}
		err := fs.WalkDir(src, ".", func(rel string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			base := path.Base(rel)
			// Dotted names are the machine's, not the worker's: .git, .DS_Store,
			// an editor's swap folder. None of them are what was sold.
			if strings.HasPrefix(base, ".") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(base, "Thumbs.db") {
				return nil
			}
			if !strings.Contains(rel, "/") && skipRoot[strings.ToLower(base)] {
				if !seenLeft[base] {
					seenLeft[base] = true
					left = append(left, base)
				}
				return nil
			}
			if _, taken := picked[rel]; !taken {
				picked[rel] = src
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Strings(left)
	return picked, left, nil
}

// writeArchive writes the zip: the collected files at the root, then the
// generated mcp.json. The root is the package — no wrapper folder — because
// "the archive's root holds AGENT.md" is what the installer looks for, and an
// exporter that produced a shape its own installer has to strip would be
// describing two formats.
func writeArchive(dest string, rels []string, picked map[string]fs.FS, declared []config.MCPServerConfig) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, rel := range rels {
		if err := copyInto(zw, picked[rel], rel); err != nil {
			zw.Close()
			return err
		}
	}
	if len(declared) > 0 {
		raw, err := marshalServers(declared)
		if err != nil {
			zw.Close()
			return err
		}
		w, err := zw.Create(config.AgentMCPFile)
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(raw); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Sync()
}

func copyInto(zw *zip.Writer, src fs.FS, rel string) error {
	in, err := src.Open(rel)
	if err != nil {
		return err
	}
	defer in.Close()
	// zip.Create rather than CreateHeader: no modification times go in, which
	// is the other half of the byte-identical promise sort() starts.
	w, err := zw.Create(rel)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}
