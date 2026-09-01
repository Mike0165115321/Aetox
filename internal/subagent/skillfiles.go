package subagent

// Taking a folder out of a worker's own shelf and putting it somewhere it can
// be worked on.
//
// Every other way an agent reaches its skills is read-only prose: `skills_list`
// indexes them, `skill_view` hands back a body, and the model copies what it
// needs by typing it out. That works for markup and stops working the moment a
// skill ships bytes nobody can type — the video library's scene folders carry
// mp3 and png beside their HTML, and a model asked to reproduce those is a
// model asked to invent them.
//
// So this is a copy, not a second discovery mechanism. It resolves through the
// same two addresses OwnSkills does, in the same order — what the user wrote
// wins over what shipped — and it refuses anything outside the named skill.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
)

// CopySkillDir copies one folder out of an agent's skill into dest.
//
// `sub` is slash-separated and relative to the skill's own root — "motion/
// structured-grid", never a path that climbs out of it. dest must not exist:
// this writes a new folder rather than merging into one, because merging is how
// a half-finished project quietly inherits files from a different scene.
//
// The answer is the number of files written, so a caller can say what it did
// rather than that it did something.
func CopySkillDir(agent, skillName, sub, dest string) (int, error) {
	clean, err := cleanSkillSub(sub)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(dest); err == nil {
		return 0, fmt.Errorf("%s มีอยู่แล้ว", filepath.Base(dest))
	}

	// The user's own copy first, the shipped one second — OwnSkills' order, and
	// for its reason: editing a bundled worker means copying it out, and the
	// copy has to be the one that runs afterwards.
	if home, err := config.AgentSkillsPath(agent); err == nil {
		src := filepath.Join(home, skillName, filepath.FromSlash(clean))
		if info, err := os.Stat(src); err == nil && info.IsDir() {
			return copyOSDir(src, dest)
		}
	}
	root := bundledSkillsDir(agent) + "/" + skillName + "/" + clean
	if _, err := fs.Stat(bundledProfiles, root); err != nil {
		return 0, fmt.Errorf("ไม่พบ %s ในสกิล %s ของเอเจน %s", sub, skillName, agent)
	}
	return copyEmbeddedDir(root, dest)
}

// cleanSkillSub refuses anything that is not a plain path inside the skill.
//
// A caller passes a name the model chose, so "../../.." is not a hypothetical.
// path.Clean collapses the tricks and then the result is checked rather than
// trusted: absolute, empty, or still climbing means no.
func cleanSkillSub(sub string) (string, error) {
	clean := path.Clean(strings.TrimSpace(strings.ReplaceAll(sub, "\\", "/")))
	if clean == "" || clean == "." || clean == "/" {
		return "", errors.New("ต้องบอกว่าจะเอาโฟลเดอร์ไหน")
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%q อยู่นอกสกิล", sub)
	}
	return clean, nil
}

// ReadSkillFile hands back one file from a worker's shelf, resolved through the
// same two addresses as CopySkillDir.
//
// Its own function rather than a mode of the copy, because the two answer
// different questions — "put this where I can work on it" and "what does this
// say" — and a caller that wanted one and got the other would find out by
// writing a folder somebody has to delete.
func ReadSkillFile(agent, skillName, sub string) ([]byte, error) {
	clean, err := cleanSkillSub(sub)
	if err != nil {
		return nil, err
	}
	if home, err := config.AgentSkillsPath(agent); err == nil {
		src := filepath.Join(home, skillName, filepath.FromSlash(clean))
		if info, err := os.Stat(src); err == nil && !info.IsDir() {
			return os.ReadFile(src)
		}
	}
	data, err := fs.ReadFile(bundledProfiles, bundledSkillsDir(agent)+"/"+skillName+"/"+clean)
	if err != nil {
		return nil, fmt.Errorf("ไม่พบ %s ในสกิล %s ของเอเจน %s", sub, skillName, agent)
	}
	return data, nil
}

// ListSkillDir names what is on one shelf of a worker's skill, without reading
// any of it.
//
// It exists for the sentence a caller writes after refusing something: a model
// that asked for a scene the library does not have is told what the library
// does have, in the same answer, rather than being sent to open a document it
// may not be carrying. Folders and files both, with the extension left on,
// because that is how the caller has to spell them back.
//
// Both addresses again, merged rather than shadowed: a user who added one scene
// of their own has a shelf that is the shipped one plus theirs, and a listing
// that showed only the folder it found first would hide whichever half lost.
func ListSkillDir(agent, skillName, sub string) ([]string, error) {
	clean, err := cleanSkillSub(sub)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	if home, err := config.AgentSkillsPath(agent); err == nil {
		entries, err := os.ReadDir(filepath.Join(home, skillName, filepath.FromSlash(clean)))
		if err == nil {
			for _, e := range entries {
				add(e.Name())
			}
		}
	}
	entries, err := fs.ReadDir(bundledProfiles, bundledSkillsDir(agent)+"/"+skillName+"/"+clean)
	if err == nil {
		for _, e := range entries {
			add(e.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("ไม่พบ %s ในสกิล %s ของเอเจน %s", sub, skillName, agent)
	}
	sort.Strings(names)
	return names, nil
}

func copyEmbeddedDir(root, dest string) (int, error) {
	written := 0
	err := fs.WalkDir(bundledProfiles, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		target := dest
		if rel != "" {
			target = filepath.Join(dest, filepath.FromSlash(rel))
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(bundledProfiles, p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		written++
		return nil
	})
	return written, err
}

func copyOSDir(src, dest string) (int, error) {
	written := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// Streamed rather than read whole: a scene folder carries audio, and
		// the largest one in the library is already half a megabyte.
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		written++
		return nil
	})
	return written, err
}
