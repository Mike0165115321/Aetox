package skill

// Installing a skill from a .zip. The other two routes — a GitHub URL and
// dropping a folder in by hand — both assume the skill is already somewhere
// reachable. A zip is what a skill looks like when it arrives by any other
// road: an email, a download, a colleague, a release asset.
//
// The same rule as the GitHub route holds and is the reason this file is not
// three lines of "unzip into the skills folder": a skill is a set — SKILL.md
// plus the scripts, references and templates it sends the model to — so the
// archive is validated whole, then written whole, or not written at all.

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ZipInstallResult is what the settings page reports after an install.
type ZipInstallResult struct {
	Names []string `json:"names"` // skills installed, as their folder names
	Files int      `json:"files"`
	Root  string   `json:"root"`
}

// zipEntry is one archive member, already stripped of the wrapper folder that
// GitHub's "Download ZIP" and most hand-made archives add.
type zipEntry struct {
	rel  string // path relative to the skill root, forward slashes
	file *zip.File
}

// InstallSkillsFromZip reads archivePath and installs every skill it contains
// into targetRoot. Nothing is written until the whole archive has been read and
// found sound.
func InstallSkillsFromZip(archivePath, targetRoot string) (ZipInstallResult, error) {
	var out ZipInstallResult
	if strings.TrimSpace(targetRoot) == "" {
		return out, errors.New("ไม่รู้ว่าจะติดตั้งลงที่ไหน")
	}
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return out, fmt.Errorf("เปิดไฟล์ zip ไม่ได้: %w", err)
	}
	defer r.Close()

	groups, err := groupZipSkills(r.File)
	if err != nil {
		return out, err
	}

	// Written to a staging directory beside the target and moved into place only
	// once every file is on disk. A zip that turns out to be truncated halfway
	// through must not leave a half-skill behind for discovery to pick up.
	staging, err := os.MkdirTemp(filepath.Dir(targetRoot), ".aetox-skill-install-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(staging)

	for _, name := range sortedKeys(groups) {
		stageDir := filepath.Join(staging, name)
		for _, e := range groups[name] {
			if err := writeZipEntry(e, stageDir); err != nil {
				return ZipInstallResult{}, err
			}
			out.Files++
		}
	}

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return ZipInstallResult{}, err
	}
	for _, name := range sortedKeys(groups) {
		dest := filepath.Join(targetRoot, name)
		// Replacing an existing skill is a reinstall, which is the ordinary way
		// to update one. The old copy goes only after the new one is staged and
		// proven, so a failure cannot leave the user with neither.
		if err := os.RemoveAll(dest); err != nil {
			return ZipInstallResult{}, err
		}
		if err := os.Rename(filepath.Join(staging, name), dest); err != nil {
			return ZipInstallResult{}, fmt.Errorf("ย้าย %s เข้าที่ไม่สำเร็จ: %w", name, err)
		}
		out.Names = append(out.Names, name)
	}
	out.Root = targetRoot
	return out, nil
}

// groupZipSkills finds the skills in an archive and returns each one's files.
//
// Accepts the shapes a zip actually arrives in: SKILL.md at the top, SKILL.md
// inside a single wrapper folder (what GitHub's Download ZIP produces), or a
// folder per skill. The wrapper is stripped so a repo zip installs under the
// skill's own name rather than "my-skill-main".
func groupZipSkills(files []*zip.File) (map[string][]zipEntry, error) {
	type candidate struct{ prefix string }
	var members []*zip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		members = append(members, f)
	}
	if len(members) == 0 {
		return nil, errors.New("ไฟล์ zip ว่างเปล่า")
	}

	// A single top-level folder is a wrapper, not a skill boundary.
	trimmed := make(map[*zip.File]string, len(members))
	root := commonZipRoot(members)
	for _, f := range members {
		trimmed[f] = strings.TrimPrefix(cleanZipName(f.Name), root)
	}

	// Zip-slip, checked over the whole archive before anything else is decided.
	// It has to come first: a `../../../evil.txt` also matches the dotfile
	// filter below, so leaving it to that would drop the entry quietly and
	// report a clean install of an archive that tried to write outside the
	// skills folder. An archive carrying one is not a mistake to route around.
	for _, f := range members {
		if hasParentSegment(cleanZipName(f.Name)) {
			return nil, fmt.Errorf("ไฟล์ zip มีเส้นทางที่ออกนอกโฟลเดอร์ติดตั้ง: %s", f.Name)
		}
	}

	// Same rule as the GitHub route: the folder that directly contains a
	// SKILL.md is the skill, at whatever depth — a zip of a repository whose
	// skills live under skills/<name>/ is the ordinary case, not an exotic one.
	var skillDirs []candidate
	seenDir := map[string]bool{}
	for _, f := range members {
		rel := trimmed[f]
		dir, base := path.Split(rel)
		if base != skillFileName {
			continue
		}
		dir = strings.TrimSuffix(dir, "/")
		if seenDir[dir] || (dir != "" && !isSkillLocation(dir)) {
			continue
		}
		seenDir[dir] = true
		skillDirs = append(skillDirs, candidate{prefix: dir})
	}
	if len(skillDirs) == 0 {
		return nil, fmt.Errorf("ไม่พบ %s ในไฟล์ zip, ต้องมีโฟลเดอร์ที่มี %s อยู่ข้างใน", skillFileName, skillFileName)
	}
	// A SKILL.md at the top means the archive is one skill, so a nested one is
	// its material rather than a sibling.
	for _, c := range skillDirs {
		if c.prefix == "" {
			skillDirs = []candidate{{prefix: ""}}
			break
		}
	}

	// Named by the folder, so skills/shadcn/ installs as "shadcn". Collisions
	// fall back to the full path rather than one skill overwriting another.
	basenames := map[string]int{}
	for _, c := range skillDirs {
		if c.prefix != "" {
			basenames[path.Base(c.prefix)]++
		}
	}

	groups := map[string][]zipEntry{}
	for _, c := range skillDirs {
		name := sanitizeSkillName(strings.TrimSuffix(root, "/"))
		if c.prefix != "" {
			base := path.Base(c.prefix)
			if basenames[base] > 1 {
				base = strings.ReplaceAll(c.prefix, "/", "-")
			}
			name = sanitizeSkillName(base)
		}
		prefix := c.prefix
		if prefix != "" {
			prefix += "/"
		}
		for _, f := range members {
			rel := trimmed[f]
			if prefix != "" && !strings.HasPrefix(rel, prefix) {
				continue
			}
			rel = strings.TrimPrefix(rel, prefix)
			if rel == "" || strings.HasPrefix(rel, ".") || strings.Contains(rel, "/.") {
				continue
			}
			if _, err := normalizeManifestRelativePath(rel); err != nil {
				// Zip-slip: an entry named ../../something. Refuse the archive
				// rather than the entry — a zip carrying one is not a mistake.
				return nil, fmt.Errorf("ไฟล์ zip มีเส้นทางที่ไม่ปลอดภัย: %s", f.Name)
			}
			groups[name] = append(groups[name], zipEntry{rel: rel, file: f})
		}
	}
	return groups, nil
}

// commonZipRoot returns the single top-level folder every member shares, with a
// trailing slash — or "" when the archive has no such wrapper.
func commonZipRoot(members []*zip.File) string {
	var root string
	for i, f := range members {
		name := cleanZipName(f.Name)
		slash := strings.Index(name, "/")
		if slash < 0 {
			return ""
		}
		head := name[:slash+1]
		if i == 0 {
			root = head
			continue
		}
		if head != root {
			return ""
		}
	}
	return root
}

// cleanZipName normalises separators. Zips written on Windows can carry
// backslashes even though the format says forward.
func cleanZipName(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, `\`, "/"), "./")
}

// hasParentSegment reports a ".." path segment, or an absolute path — the two
// ways an archive member reaches outside the directory it is extracted into.
// Segment-wise, so a file legitimately named "..config" is not caught.
func hasParentSegment(name string) bool {
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return true
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func writeZipEntry(e zipEntry, stageDir string) error {
	target := filepath.Join(stageDir, filepath.FromSlash(e.rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := e.file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()
	// Unbounded on purpose. The archive is a local file the user picked, and how
	// much of their own disk to spend on it is their call — same reasoning as
	// the absent install ceiling in github_tools.go.
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func sortedKeys(m map[string][]zipEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
