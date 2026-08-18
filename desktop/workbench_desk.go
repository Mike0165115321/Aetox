package main

// The agent's own reach onto the desk.
//
// Until this file, the whole of that reach was `browser_open`: the agent could
// put a web page on the desk and nothing else. It could write a deck and say so,
// and the user still had to go and click the card themselves — on the surface
// whose entire job is showing what was produced. §87 widened the desk from four
// panes to six and left the agent holding one of them, which made the gap worse
// rather than better.
//
// Three verbs close it, and they are deliberately thin. `desk_open` does not
// know what a pane is — it hands the path to the same opener the file tree and
// the drop handler call, and `fileView()` decides. That is what keeps the desk
// able to learn a new file type without anything here, or in the model's head,
// changing. See docs/architecture/desk-file-panes-2026-08-06.md §6.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
)

// ---------------------------------------------------------------------------
// what is on the desk
// ---------------------------------------------------------------------------

// DeskTab is one tab as the frontend sees it, reported here so the agent can
// read its own desk. The frontend pushes the whole list on every change
// (WorkbenchTabsChanged); nothing on the Go side tries to track it by watching
// the events it sends, which would drift the moment the user closed a tab.
type DeskTab struct {
	Kind string `json:"kind"` // terminal | browser | files | file | decks
	Name string `json:"name"`
	Path string `json:"path,omitempty"` // file tabs
	URL  string `json:"url,omitempty"`  // browser tabs
	// Mine reports that the agent opened this tab rather than the user. It is
	// the whole basis of the redaction below.
	Mine bool `json:"mine"`
}

type deskState struct {
	mu   sync.Mutex
	tabs []DeskTab
}

// WorkbenchTabsChanged is called by the frontend whenever the tab strip changes.
func (a *App) WorkbenchTabsChanged(tabs []DeskTab) {
	a.openTabs.mu.Lock()
	a.openTabs.tabs = tabs
	a.openTabs.mu.Unlock()
}

func (a *App) deskTabs() []DeskTab {
	a.openTabs.mu.Lock()
	defer a.openTabs.mu.Unlock()
	return append([]DeskTab(nil), a.openTabs.tabs...)
}

// ---------------------------------------------------------------------------
// desk_open
// ---------------------------------------------------------------------------

type deskOpenSkill struct{ app *App }

func (*deskOpenSkill) Name() string { return "desk_open" }

func (*deskOpenSkill) Description() string {
	return "วางไฟล์ลงบนโต๊ะทำงานให้ผู้ใช้เห็น"
}

func (*deskOpenSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("desk_open",
		"Put a file on the user's desk, where they can see it. Pass the same path write/doc_write/sheet_write reported, relative to the sandbox root. The desk picks how to show it — a picture as a picture, a video as a player, a PDF as a reader, a spreadsheet as a grid, anything else in the editor — so there is nothing to choose here. Use it whenever you have made or found a file the user should look at, instead of only telling them the filename.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path relative to the sandbox root"},
			},
			"required": []string{"path"},
		})
}

func (s *deskOpenSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	path, _ := args["path"].(string)
	return s.open(path)
}

func (s *deskOpenSkill) Execute(_ context.Context, input skill.Input) (skill.Output, error) {
	path, _ := input["path"].(string)
	return s.open(path)
}

func (s *deskOpenSkill) open(path string) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: "desk_open", Command: "desk_open " + path}
	fail := func(err error) (skill.Output, error) {
		out.Content = "เปิดบนโต๊ะไม่สำเร็จ: " + err.Error()
		out.Stderr = err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return fail(fmt.Errorf("path is required"))
	}
	if s.app.ctx == nil {
		return fail(fmt.Errorf("UI not ready"))
	}
	// Resolved here rather than left to the frontend: a tab that opens onto a
	// path that does not exist shows the user a card reading "file is gone",
	// which reads as the agent having lost the file it just made. The error
	// belongs in the turn, where the model can still do something about it.
	root := strings.TrimSpace(s.app.cfg.SandboxRoot)
	if root == "" {
		return fail(fmt.Errorf("no project open"))
	}
	full, err := safeSandboxPath(root, path)
	if err != nil {
		return fail(err)
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return fail(fmt.Errorf("%s does not exist", path))
	}

	name := filepath.Base(filepath.FromSlash(path))
	s.app.emitEvent("workbench:open-file", map[string]string{"path": path, "name": name})

	out.Success = true
	out.DurationMs = time.Since(start).Milliseconds()
	out.Content = deskOpenedLine(path)
	out.RawOutput = out.Content
	return out, nil
}

// deskOpenedLine is written once, here, so the round-trip test asserts the real
// sentence rather than its own copy of the format (same reasoning as
// browserOpenedLine).
func deskOpenedLine(path string) string {
	return fmt.Sprintf("วางไฟล์ %s บนโต๊ะแล้ว ผู้ใช้เห็นอยู่ตอนนี้", path)
}

// ---------------------------------------------------------------------------
// desk_terminal
// ---------------------------------------------------------------------------

type deskTerminalSkill struct{ app *App }

func (*deskTerminalSkill) Name() string { return "desk_terminal" }

func (*deskTerminalSkill) Description() string {
	return "เปิดเทอร์มินัลบนโต๊ะ แล้วพิมพ์คำสั่งให้ผู้ใช้เห็นสด ๆ"
}

func (*deskTerminalSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("desk_terminal",
		"Open a terminal on the user's desk and optionally type a command into it. This is for the user to WATCH — the output streams into a real terminal in front of them, live. It is not how you read a command's output: `shell` is, and it is faster and gives you the text. Reach for this when being seen matters — a build or a server the user should watch run, or a session they will keep typing into themselves afterwards.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Command to type and run. Omit to open an empty terminal for the user."},
			},
		})
}

func (s *deskTerminalSkill) ExecuteTool(_ context.Context, args map[string]any) (skill.Output, error) {
	cmd, _ := args["command"].(string)
	return s.run(cmd)
}

func (s *deskTerminalSkill) Execute(_ context.Context, input skill.Input) (skill.Output, error) {
	cmd, _ := input["command"].(string)
	return s.run(cmd)
}

// openDeskTerminal is the one way a terminal appears on the desk from the Go
// side: start the shell, tell the frontend to mount a pane on it, and type the
// command in if there is one. Extracted from the tool below on 2026-08-11,
// when the engine starter (engine_server.go) became its second caller — an
// engine starting in a window the user watches is this exact act, and a copy
// of these lines over there would be two ways to open the same terminal.
func (a *App) openDeskTerminal(command string) (shellName string, err error) {
	if a.ctx == nil {
		return "", fmt.Errorf("UI not ready")
	}
	shells := a.TerminalShells()
	if len(shells) == 0 {
		return "", fmt.Errorf("no shell found on this machine")
	}
	// The first profile, not the user's saved default: that preference lives in
	// the frontend's localStorage and the Go side cannot see it. Worth knowing
	// rather than guessing at — if it starts mattering, the frontend has to send
	// it over, and this comment is where to start.
	sh := shells[0]

	id, err := a.TerminalStart(sh.Path, 80, 24)
	if err != nil {
		return "", err
	}
	// The session exists before the tab does, which is the reverse of the
	// browser's flow (there the frontend creates the window and Go polls for
	// it). It is simpler this way round: the id is already real, so the frontend
	// only has to mount a pane onto it, and there is nothing to wait for.
	a.emitEvent("workbench:open-terminal", map[string]string{"id": id, "name": sh.Name})

	if command != "" {
		// A newline is what makes it run. Given to the PTY exactly as a keypress
		// would arrive, because that is what this is: the agent typing.
		if err := a.TerminalWrite(id, command+"\r"); err != nil {
			return "", err
		}
	}
	return sh.Name, nil
}

func (s *deskTerminalSkill) run(command string) (skill.Output, error) {
	start := time.Now()
	command = strings.TrimSpace(command)
	out := skill.Output{Name: "desk_terminal", Command: "desk_terminal " + command}
	fail := func(err error) (skill.Output, error) {
		out.Content = "เปิดเทอร์มินัลไม่สำเร็จ: " + err.Error()
		out.Stderr = err.Error()
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	name, err := s.app.openDeskTerminal(command)
	if err != nil {
		return fail(err)
	}
	sh := ShellProfile{Name: name}

	out.Success = true
	out.DurationMs = time.Since(start).Milliseconds()
	if command == "" {
		out.Content = fmt.Sprintf("เปิดเทอร์มินัล %s บนโต๊ะแล้ว", sh.Name)
	} else {
		// Says plainly that the output is not coming back here, so a model
		// waiting for it stops waiting and uses shell instead next time.
		out.Content = fmt.Sprintf("เปิดเทอร์มินัล %s บนโต๊ะแล้วพิมพ์ `%s` ให้ ผู้ใช้กำลังเห็นผลสด ๆ — ผลลัพธ์ไม่ได้ส่งกลับมาที่นี่ ถ้าต้องอ่านผลเองให้ใช้ shell", sh.Name, command)
	}
	out.RawOutput = out.Content
	return out, nil
}

// ---------------------------------------------------------------------------
// desk_list
// ---------------------------------------------------------------------------

type deskListSkill struct{ app *App }

func (*deskListSkill) Name() string { return "desk_list" }

func (*deskListSkill) Description() string {
	return "ดูว่าตอนนี้บนโต๊ะมีอะไรเปิดอยู่บ้าง"
}

func (*deskListSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("desk_list",
		"List what is currently open on the user's desk. Use it before opening something, to avoid putting up a file that is already there, and to tell whether the user is looking at something else right now.",
		map[string]any{"type": "object", "properties": map[string]any{}})
}

func (s *deskListSkill) ExecuteTool(_ context.Context, _ map[string]any) (skill.Output, error) {
	return s.list()
}

func (s *deskListSkill) Execute(_ context.Context, _ skill.Input) (skill.Output, error) {
	return s.list()
}

func (s *deskListSkill) list() (skill.Output, error) {
	start := time.Now()
	lines := describeDesk(s.app.deskTabs())
	content := "โต๊ะว่าง ยังไม่มีอะไรเปิดอยู่"
	if len(lines) > 0 {
		content = "บนโต๊ะตอนนี้:\n" + strings.Join(lines, "\n")
	}
	return skill.Output{
		Name:       "desk_list",
		Command:    "desk_list",
		Success:    true,
		Content:    content,
		RawOutput:  content,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// describeDesk renders the tab list for the agent, with one redaction.
//
// §81 decided that the workbench's browsing history stays in localStorage
// rather than the tool_runs table, because that table is the agent's own
// searchable memory and putting the user's personal browsing in it "is a far
// bigger decision than a start-page list gets to make". A desk_list that
// returned page titles and URLs would hand over exactly that, through a
// different door.
//
// So: a page the agent opened reports fully — it already knows, it put it
// there. A page the user opened reports that it exists and nothing else.
func describeDesk(tabs []DeskTab) []string {
	lines := make([]string, 0, len(tabs))
	for _, t := range tabs {
		switch {
		case t.Kind == "browser" && !t.Mine:
			lines = append(lines, "- แท็บเบราว์เซอร์ที่ผู้ใช้เปิดเอง (ไม่เปิดเผยที่อยู่)")
		case t.Kind == "browser":
			lines = append(lines, fmt.Sprintf("- เบราว์เซอร์: %s (%s)", t.Name, t.URL))
		case t.Kind == "file":
			lines = append(lines, fmt.Sprintf("- ไฟล์: %s", t.Path))
		case t.Kind == "terminal":
			lines = append(lines, fmt.Sprintf("- เทอร์มินัล: %s", t.Name))
		default:
			lines = append(lines, fmt.Sprintf("- %s", t.Name))
		}
	}
	return lines
}
