package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mikedev115/Aetox/internal/model"
)

// Progressive skill loading: skills_list and skill_view are how the model
// reaches SKILL.md documents. Discovered skills are still registered (the
// user's /skill-name command and collision detection need them) but their
// per-skill tool definitions are no longer sent to the model — see
// Dispatcher.ToolDefinitions.
//
// Why: every discovered skill used to add its own entry to the tool block of
// every request, so a user with 50 skills paid ~50 schemas of context on
// every call whether or not any skill was relevant — the cost grew linearly
// with the library. These two definitions replace all of them at a flat
// price: the model pays for a skill's listing only when it asks, and for a
// body only when it opens one. Same shape as the capability boundary in
// ARCHITECTURE.md §44 — knowing a thing exists is cheap, knowing how it
// works is paid on use.
//
// Both tools scan the discovery paths at call time rather than holding the
// registry: a skill installed mid-session (plugin_install writes to
// ~/.aetox/skills) is listable immediately, before any re-bootstrap.

// ProgressiveTools returns the pair of doors, carrying one extra shelf beside
// the machine's own.
//
// The extra is how a worker's private skills become reachable at all. They are
// registered in that worker's registry, but Dispatcher.ToolDefinitions withholds
// every SourceSkill definition — that is what progressive loading is — so the
// model was never told the names, and these two tools scanned ~/.aetox/skills
// and nowhere else. The knowledge was loaded, executable, and behind no door the
// model could see (found 2026-08-16). Passing the worker's shelf in here is what
// the file that describes the feature always claimed was happening.
//
// nil extra gives the ordinary pair, unchanged.
func ProgressiveTools(extra []DiscoveredSkill) (Skill, Skill) {
	return &skillsListSkill{extra: extra}, &skillViewSkill{extra: extra}
}

// skillsListSkill is L0: one line per installed skill, name + description.
type skillsListSkill struct {
	// paths overrides the scan locations; nil means DefaultDiscoveryPaths(),
	// resolved at call time so tests can point it at a fixture directory.
	paths []string
	// extra is a shelf that is not on any scan path — one worker's own skills,
	// which include documents compiled into the binary and so have no path to
	// scan. Resolved by the caller, because the rule for which copy of a
	// same-named skill wins lives with the worker, not here.
	extra []DiscoveredSkill
}

func (s *skillsListSkill) scanPaths() []string {
	if s.paths != nil {
		return s.paths
	}
	return DefaultDiscoveryPaths()
}

func (s *skillsListSkill) shelf() []DiscoveredSkill {
	return mergeShelf(ListDiscovered(s.scanPaths()), s.extra)
}

// mergeShelf puts the worker's own documents in front of the machine's.
//
// A name held by both is the worker's. That is the direction the whole feature
// points: the shared shelf is what everybody knows, the folder is what this one
// knows instead, and a worker whose `invoice` was overruled by a general
// `invoice` on the shelf would be a specialist with the generalist's answer.
func mergeShelf(base, extra []DiscoveredSkill) []DiscoveredSkill {
	if len(extra) == 0 {
		return base
	}
	out := make([]DiscoveredSkill, 0, len(base)+len(extra))
	own := make(map[string]bool, len(extra))
	for _, d := range extra {
		own[strings.ToLower(d.Name)] = true
		out = append(out, d)
	}
	for _, d := range base {
		if own[strings.ToLower(d.Name)] {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (s *skillsListSkill) Name() string { return "skills_list" }

func (s *skillsListSkill) Description() string {
	return "List the skill documents installed on this machine — task instructions the user has added. " +
		"Returns one line per skill: name — description. Read one with skill_view before doing a task it covers."
}

func (s *skillsListSkill) Execute(ctx context.Context, _ Input) (Output, error) {
	return s.ExecuteTool(ctx, nil)
}

func (s *skillsListSkill) ToolDefinition() model.ToolDefinition {
	schema, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	})
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        s.Name(),
			Description: s.Description(),
			Parameters:  schema,
		},
	}
}

func (s *skillsListSkill) ExecuteTool(_ context.Context, _ map[string]any) (Output, error) {
	start := time.Now()
	discovered := s.shelf()
	if len(discovered) == 0 {
		return newToolOutput(s.Name(), s.Name(), "No skills installed.", start, false, nil), nil
	}
	sort.Slice(discovered, func(i, j int) bool { return discovered[i].Name < discovered[j].Name })
	var b strings.Builder
	for _, d := range discovered {
		b.WriteString(d.Name)
		if d.Description != "" {
			b.WriteString(" — ")
			b.WriteString(d.Description)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nRead one with skill_view {\"name\": \"...\"} before doing a task it covers.")
	return newToolOutput(s.Name(), s.Name(), b.String(), start, false, nil), nil
}

// endMarker closes a skill body so a reader can tell a finished document from a
// clipped one.
//
// A skill that ends on a caveat reads exactly like a skill that was cut off, and
// nothing in the result said which it was. On 2026-08-20 the assistant read
// `aetox-slides` — 15,778 characters, whole, ending on "## What the deck is
// not" — decided it had been truncated, called skill_view a second time (same
// bytes, same conclusion), then spent a glob and a shell hunting the file on
// disk. Three rounds spent, and the deck still written from a generic template.
//
// The second line is the other half of that waste, and it is a fact only this
// package holds: a bundled skill has no Dir (bundled_skills.go), so "go and read
// the file yourself" is not a slow fallback, it is a search that cannot succeed.
// Saying so where the search would start is cheaper than the error it would
// otherwise arrive at.
//
// "with glob or shell" and not "at all": since 2026-08-22 the folder travels
// inside the binary too, so those files do exist — they are reached through
// skill_view and nowhere else. carries is how many were listed just above, so
// the second sentence is only added when there is something to reach.
//
// Counted in characters rather than tokens because characters are what this
// package can count honestly — the number is there to be compared against what
// arrived, not to be budgeted with.
func endMarker(d DiscoveredSkill, carries int) string {
	m := fmt.Sprintf("\n\n[end of %s — this is the whole document, %d characters]", d.Name, len(d.body))
	if d.Dir == "" {
		m += "\n[it ships inside Aetox and has no folder on disk, so there is nothing further to find with glob or shell]"
		if carries > 0 {
			m += "\n[its own files are listed just above; skill_view with a path is the only door to them]"
		}
	}
	return m
}

// notInSkill refuses a path the skill does not carry, and names what it does.
//
// The old text was "the skill body lists what it has". That was true of the
// listing skill_view appends and not of the body, and the difference is the
// whole failure: on 2026-08-23 the aetox-design-system body's own table named
// eight data/*.csv files the binary was not shipping, because a bare `data/`
// line in .gitignore had swallowed every one of them. So the document promised
// a file, the refusal pointed back at the document, and the model asked three
// times for three different rows of the same table. Naming the files that can
// actually be served ends that on the first try, and it is the same move the
// unknown-name branch below already makes with the list of skills.
func notInSkill(d DiscoveredSkill, sub string) error {
	files := supportingFiles(d)
	if len(files) == 0 {
		return fmt.Errorf("%q is not in this skill; it carries no files beside its own document", sub)
	}
	return fmt.Errorf("%q is not in this skill. It carries: %s", sub, strings.Join(files, ", "))
}

// readSkillFile returns one supporting file from inside a skill's folder.
//
// The containment check is the point. `path` is a string the model wrote, and
// it is about to be joined onto a directory — "references/../../../.ssh/id_rsa"
// is the whole attack, and it is a plausible thing for a skill document to
// contain by accident as well as on purpose. Resolved and compared against the
// skill's own directory, so what the gate judges is where the path *lands*,
// never how it was spelled.
//
// Deliberately not routed through resolveSandboxPath: a skill lives in
// ~/.aetox/skills, outside the workspace, so the sandbox would refuse every
// read here. This is the same rule applied to a different root, and it is the
// only place that root is readable from.
func readSkillFile(d DiscoveredSkill, sub string) (string, error) {
	// A skill that ships inside the binary is read out of the binary. Its
	// folder is an FS rooted at the skill, so "outside the skill" is not a
	// judgement this has to make: fs.Sub already refused to hand out a root
	// above it, and cleanPath below refuses a name that climbs.
	if d.files != nil {
		return readEmbeddedSkillFile(d, sub)
	}
	dir := d.Dir
	// No folder and nothing embedded. Without this, filepath.Abs("") returns
	// the process's working directory and the containment check below would
	// then be measuring the wrong root entirely — every path under cwd would
	// pass.
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("this skill ships inside Aetox as one document and has no files beside it")
	}
	base, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read inside this skill")
	}
	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(sub)))
	if err != nil {
		return "", fmt.Errorf("cannot read %q", sub)
	}
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside the skill's own folder", sub)
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", notInSkill(d, sub)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a folder; name a file inside it", sub)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("could not read %q: %w", sub, err)
	}
	if len(data) > maxSkillFileBytes {
		cut := maxSkillFileBytes
		for cut > 0 && !utf8.RuneStart(data[cut]) {
			cut--
		}
		return string(data[:cut]) + fmt.Sprintf("\n\n[cut — file is %d bytes, showed the first %d]", len(data), cut), nil
	}
	return string(data), nil
}

// readEmbeddedSkillFile is readSkillFile's other half: the same read against a
// skill that lives inside the binary.
//
// Shorter than the disk one, and the difference is not carelessness. fsys is
// rooted at the skill by fs.Sub, so there is no parent to reach — an fs.FS
// cannot express one. What is left is the spelling: `path.Clean` folds a "../"
// into a name that leaves the root, and fs.ValidPath is what rejects it, which
// is why the clean happens before the check rather than after.
func readEmbeddedSkillFile(d DiscoveredSkill, sub string) (string, error) {
	fsys := d.files
	name := path.Clean(strings.TrimPrefix(filepath.ToSlash(sub), "./"))
	if name == "" || name == "." || !fs.ValidPath(name) {
		return "", fmt.Errorf("%q is outside the skill's own folder", sub)
	}
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return "", notInSkill(d, sub)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a folder; name a file inside it", sub)
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("could not read %q: %w", sub, err)
	}
	if len(data) > maxSkillFileBytes {
		cut := maxSkillFileBytes
		for cut > 0 && !utf8.RuneStart(data[cut]) {
			cut--
		}
		return string(data[:cut]) + fmt.Sprintf("\n\n[cut — file is %d bytes, showed the first %d]", len(data), cut), nil
	}
	return string(data), nil
}

// skillViewSkill is L1: the full body of one skill, fetched by name.
type skillViewSkill struct {
	paths []string
	extra []DiscoveredSkill // see skillsListSkill.extra
}

func (s *skillViewSkill) scanPaths() []string {
	if s.paths != nil {
		return s.paths
	}
	return DefaultDiscoveryPaths()
}

func (s *skillViewSkill) shelf() []DiscoveredSkill {
	found, _ := scanSkills(s.scanPaths())
	return mergeShelf(found, s.extra)
}

func (s *skillViewSkill) Name() string { return "skill_view" }

func (s *skillViewSkill) Description() string {
	return "Read one installed skill document by name (as listed by skills_list) and follow its instructions."
}

// maxSkillFileBytes caps one L2 read. A skill may legitimately ship a large
// data file; sending all of it costs the same context whether the model needed
// the whole thing or the first page, and the cut is reported so a truncated
// read is never mistaken for a short file.
const maxSkillFileBytes = 32 << 10

// supportingFiles lists what else lives in a skill's folder, so the model can
// find a reference it was never told about.
//
// L1 without this is a dead end whenever a skill is too long for one file:
// SKILL.md can name its own references, but nothing makes it, and a skill
// written by the agent itself (which splits work into files precisely because
// it grew past one) would be unreachable past its first page. Directories are
// walked, so references/a.md shows as references/a.md rather than as a folder
// the model has to guess the contents of.
func supportingFiles(d DiscoveredSkill) []string {
	if d.files != nil {
		return trimListing(embeddedSupportingFiles(d.files))
	}
	dir := d.Dir
	// Same reason as readSkillFile: filepath.Clean("") is ".", and walking that
	// would list the working directory as if it were a bundled skill's contents.
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	var out []string
	root := filepath.Clean(dir)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "SKILL.md" {
			return nil
		}
		// Slash-separated regardless of platform: this string goes back to the
		// model as something to pass to `path`, and it is joined here, so one
		// spelling everywhere is one less thing for it to get wrong.
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return trimListing(out)
}

// embeddedSupportingFiles is the same walk over a skill that ships inside the
// binary. fs.WalkDir names everything relative to the root already, so the
// paths it produces are the ones `path` takes, with no conversion to get wrong.
func embeddedSupportingFiles(fsys fs.FS) []string {
	var out []string
	_ = fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || name == skillFileName {
			return nil
		}
		out = append(out, name)
		return nil
	})
	return out
}

// trimListing sorts a file listing and caps it, so both walks answer with the
// same shape and the cap is stated once.
func trimListing(out []string) []string {
	sort.Strings(out)
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func (s *skillViewSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := map[string]any{}
	if input != nil {
		if raw, ok := input["args"].(string); ok {
			args["name"] = strings.TrimSpace(raw)
		}
	}
	return s.ExecuteTool(ctx, args)
}

func (s *skillViewSkill) ToolDefinition() model.ToolDefinition {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name exactly as skills_list reported it",
			},
			// L2. Named files are opened only when the model decides it needs
			// them, which is the whole point of the three levels: a skill can be
			// as long as it needs to be without any of that length reaching the
			// context of a session that did not use it.
			// No example path here, deliberately. The first version ended
			// "e.g. references/formats.md" and a model followed that shape
			// instead of the listing it had just been handed — asking for
			// references/consumer-props.md when the skill's own file was
			// consumer-props.md, flat. An example in a description is not an
			// illustration, it is an instruction, and it outranks data that
			// arrives later in the conversation. Name where the truth lives
			// instead.
			"path": map[string]any{
				"type":        "string",
				"description": "Optional: one of the files listed at the end of the skill body, spelled exactly as it appears there",
			},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	})
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        s.Name(),
			Description: s.Description(),
			Parameters:  schema,
		},
	}
}

func (s *skillViewSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		err := fmt.Errorf("name is required — call skills_list to see what is installed")
		return newToolOutput(s.Name(), s.Name(), err.Error(), start, false, err), err
	}
	sub, _ := args["path"].(string)
	sub = strings.TrimSpace(sub)

	discovered := s.shelf()
	for _, d := range discovered {
		if !strings.EqualFold(d.Name, name) {
			continue
		}
		if sub != "" {
			content, err := readSkillFile(d, sub)
			if err != nil {
				return newToolOutput(s.Name(), s.Name()+" "+d.Name, err.Error(), start, false, err), err
			}
			return newToolOutput(s.Name(), s.Name()+" "+d.Name+"/"+sub, content, start, false, nil), nil
		}
		body := d.body
		files := supportingFiles(d)
		if len(files) > 0 {
			body += "\n\nFiles in this skill — read one with skill_view {\"name\": \"" + d.Name +
				"\", \"path\": \"…\"}:\n- " + strings.Join(files, "\n- ") + "\n"
		}
		body += endMarker(d, len(files))
		return newToolOutput(s.Name(), s.Name()+" "+d.Name, body, start, false, nil), nil
	}

	// Name the alternatives in the error: the model asked for something that
	// is not there, and the cheapest recovery is handing it the real list now
	// instead of making it burn a round on skills_list.
	names := make([]string, 0, len(discovered))
	for _, d := range discovered {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	var err error
	if len(names) == 0 {
		err = fmt.Errorf("skill %q not found — no skills are installed", name)
	} else {
		err = fmt.Errorf("skill %q not found — installed skills: %s", name, strings.Join(names, ", "))
	}
	return newToolOutput(s.Name(), s.Name(), err.Error(), start, false, err), err
}
