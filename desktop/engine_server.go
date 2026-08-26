package main

// The automation agent's own hands on its engine's power switch (owner's call,
// 2026-08-10: "ให้มันเช็คเองว่าพร้อมทำงานไหม เช็คเอง รันเปิดเองได้").
//
// The Settings page grew a "เปิดเซิร์ฟเวอร์" button first (StartConnectionServer),
// and this is the same capability offered to the one agent whose whole job
// stands behind that server — not a second implementation: both doors call the
// same App methods, use the same stored command, and wait the same way.
//
// One tool per vendor, named with the vendor's prefix, and that is what wires
// it into the placement lock for free: `n8n_server_start` is owned by the n8n
// connection (Provider.Tools), so it reaches exactly who the connection
// reaches — the automation agent, when n8n is the engine the user picked, and
// nobody else. A generic `engine_start` would need its own answer to "who gets
// this", and the catalog already has one.
//
// The start command stays a Settings value with a store-once door here: the
// agent may fill an EMPTY field — that is "หาเอง" working as asked, and the
// user reads the result in Settings afterwards — but may not overwrite one the
// user wrote. Changing a stored command is register work, on the page that owns
// it.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/connect"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

type engineServerSkill struct {
	app *App
	// id is the connection id in the connect catalog; the tool name and every
	// message derive from the catalog row so this file cannot drift from it.
	id string
}

func (s *engineServerSkill) Name() string { return s.id + "_server_start" }

func (s *engineServerSkill) Description() string {
	return "เช็คว่าเซิร์ฟเวอร์ " + s.label() + " ตอบไหม ถ้ายังไม่ขึ้นก็เปิดให้ด้วยคำสั่งที่ผู้ใช้บันทึกไว้"
}

func (s *engineServerSkill) label() string {
	if row, ok := connect.StatusOf(s.id); ok {
		return row.Label
	}
	return s.id
}

func (s *engineServerSkill) ToolDefinition() model.ToolDefinition {
	return toolDef(s.Name(),
		"Check whether the user's "+s.label()+" server is answering, and start it if it is not — with the start command saved in Settings → การเชื่อมต่อ, run in a terminal on the desk where the user watches it come up. Waits until the server answers (cold starts run migrations; up to 90s) or clearly fails. Make this your first move when a job needs the engine, before assuming anything about it — and once it answers, `browser` its address so the work happens where the user can see it. Pass `command` ONLY when the tool says no start command is saved: it is stored once and shown in Settings, so find the real one (ask the user, or look for their script) rather than guessing.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Start command to save and use, only when none is saved yet",
				},
			},
		})
}

func (s *engineServerSkill) ExecuteTool(ctx context.Context, args map[string]any) (skill.Output, error) {
	command, _ := args["command"].(string)
	return s.run(ctx, command)
}

func (s *engineServerSkill) Execute(ctx context.Context, input skill.Input) (skill.Output, error) {
	command, _ := input["command"].(string)
	return s.run(ctx, command)
}

func (s *engineServerSkill) run(_ context.Context, command string) (skill.Output, error) {
	start := time.Now()
	out := skill.Output{Name: s.Name(), Command: s.Name()}
	fail := func(msg string, err error) (skill.Output, error) {
		out.Content = msg
		if err != nil {
			out.Stderr = err.Error()
		}
		out.DurationMs = time.Since(start).Milliseconds()
		return out, err
	}

	row, ok := connect.StatusOf(s.id)
	if !ok {
		return fail("ไม่รู้จักการเชื่อมต่อ "+s.id, nil)
	}
	// Answering already is the common case and the cheap one; say where.
	if up, err := s.app.CheckConnectionServer(s.id); err != nil {
		return fail(err.Error(), err)
	} else if up {
		// It answered, so whatever start was in flight is finished — cleared here
		// so a genuine restart later is not refused by a flag left over from the
		// start that worked.
		s.app.clearEngineStarting(s.id)
		out.Success = true
		out.Content = row.Label + " ตอบอยู่แล้วที่ " + row.BaseURL + " — พร้อมทำงาน"
		out.DurationMs = time.Since(start).Milliseconds()
		return out, nil
	}

	if command = strings.TrimSpace(command); command != "" {
		if strings.TrimSpace(row.StartCommand) != "" {
			// Store-once, never overwrite: the field belongs to the user, and a
			// model that could replace it could point the button at anything.
			return fail("มีคำสั่งเปิดเซิร์ฟเวอร์บันทึกไว้แล้ว — ถ้าจะเปลี่ยน ผู้ใช้แก้เองที่ ตั้งค่า → การเชื่อมต่อ", nil)
		}
		if err := connect.SetStartCommand(s.id, command); err != nil {
			return fail("บันทึกคำสั่งไม่สำเร็จ: "+err.Error(), err)
		}
	}

	// The agent starts its engine where the user watches (owner's call,
	// 2026-08-11: "มันก็เปิดเทอมินอลขึ้นมารันเองฝั่งโต๊ะทำงานมัน") — but the server
	// itself runs hidden with its output in a log file, and the desk terminal
	// only TAILS that file. The first cut typed the start command straight into
	// the desk terminal's ConPTY, and the same night's incident is why it does
	// not any more: n8n froze mid-boot inside that plumbing — process alive,
	// port never opened — while the identical command outside it came up in
	// nine seconds. A server's stdout must never depend on a UI pipeline being
	// healthy; a file blocks nobody, and a pane that only reads a file can
	// break nothing but the view (launchLogged, launch_windows.go).
	//
	// One behavioural consequence, accepted on purpose: a server started here
	// lives with the app (§24's job), where the Settings button's detached
	// console outlives it. For the agent's own working session that is the
	// better default — no orphan engine still holding a port tomorrow — and an
	// engine meant to run forever is what the Settings button and a real
	// service install are for.
	saved := strings.TrimSpace(row.StartCommand)
	if command != "" {
		saved = command
	}
	if saved == "" {
		return fail("ยังไม่ได้บอกว่าจะเปิดเซิร์ฟเวอร์นี้ด้วยคำสั่งอะไร", nil)
	}
	logPath := s.serverLogPath()
	// A start already under way is not a reason to start another one.
	//
	// The reachability check above only asks whether the server ANSWERS, and one
	// that is still coming up answers nothing — so a second call walked straight
	// past it and launched a second engine at the same port, over a log the first
	// was writing and a terminal was following. What the user saw was the pane
	// thrashing; underneath were two n8n processes and two tails over one file.
	//
	// Refusing to act is the right answer here precisely because something is
	// already acting: the second caller is told what the first is doing, and
	// where to watch it.
	if since, busy := s.app.engineStarting(s.id); busy {
		return fail(stillStartingMessage(row.Label, row.BaseURL, logPath, since), nil)
	}
	if err := launchLogged(saved, logPath); err != nil {
		return fail("เปิด "+row.Label+" ไม่สำเร็จ: "+err.Error(), err)
	}
	s.app.markEngineStarting(s.id)
	// The front-row seat: a desk terminal following the boot log live. Failing
	// to open one is not failing to start the server — headless runs and a
	// window that is not up yet lose the view, not the engine.
	seated := ""
	if _, err := s.app.openDeskTerminal("", tailCommand(logPath)); err == nil {
		seated = " ผู้ใช้เห็น log มันวิ่งอยู่ในเทอร์มินัลบนโต๊ะ"
	}
	if !waitReachable(row.BaseURL, serverStartPatience) {
		// The log tail rides along on failure, because this sentence is what
		// the model acts on. The night this file was reshaped, the model got
		// only "ยังไม่ตอบใน 90 วินาที" and spent twenty-nine tool calls groping
		// at processes and ports for the reason — which had been printed,
		// readable, in a place nothing handed to it.
		//
		// The path rides along too, and the difference is a snapshot versus an
		// address. A tail is fifteen lines frozen at the moment of giving up; a
		// server that is still coming up changes what it says a second later,
		// and the agent had no way to look again. It asked the user to go and
		// read the terminal instead — a terminal being one-way by design
		// (desk_terminal), while the very file that terminal was following sat
		// there readable the whole time.
		//
		// And the sentence says what "still coming up" looks like, because the
		// same fifteen lines mean two opposite things: "Initializing n8n
		// process" is a server working, and reporting that as a dead end is the
		// one thing the agent's own instructions forbid it to do.
		// FromWorld: the server was asked to start and has not answered yet.
		// That is tonight's machine, not a rule anyone broke, and three cards
		// teaching "avoid whatever hits an n8n that is down" is what the
		// learning floor did with it before.
		out.FromWorld = true
		return fail(unreachableMessage(row.Label, row.BaseURL, logPath), nil)
	}
	out.Success = true
	out.Content = "เปิด " + row.Label + " แล้ว ตอบที่ " + row.BaseURL + " —" + seated + " พร้อมทำงาน"
	out.DurationMs = time.Since(start).Milliseconds()
	return out, nil
}

// unreachableMessage is what the caller is told when the engine was started and
// has not answered yet. Written once, here, so the test asserts the real
// sentence rather than its own copy of it (same reasoning as deskOpenedLine).
func unreachableMessage(label, baseURL, logPath string) string {
	return "สั่งเปิด " + label + " แล้วแต่ " + baseURL + " ยังไม่ตอบใน 90 วินาที — ท้าย log ของมันบอกว่า:\n" +
		tailOfFile(logPath, 15) +
		"\n\nlog เต็มอยู่ที่ " + logPath + " — อ่านซ้ำเองได้ด้วย `read` ทุกเมื่อ " +
		"ถ้าท้าย log เป็นข้อความว่ากำลังเริ่ม ไม่ใช่ error แปลว่ามันยังขึ้นอยู่ ไม่ใช่ล้ม: " +
		"รอสักครู่แล้ว `read` ไฟล์นี้ซ้ำจนกว่าจะเห็นว่าพร้อม — อย่าถามผู้ใช้ให้ไปดูเทอร์มินัลแทน " +
		"เพราะเทอร์มินัลก็อ่านไฟล์นี้ไฟล์เดียวกัน และมันเป็นหน้าต่างทางเดียว ผู้ใช้ดูได้ คุณอ่านไม่ได้. " +
		"ห้ามเรียกเครื่องมือนี้ซ้ำระหว่างที่มันยังขึ้นอยู่: การเรียกซ้ำจะสั่งเปิดเซิร์ฟเวอร์ตัวที่สองแย่งพอร์ตเดิม " +
		"และล้าง log ทิ้งทั้งไฟล์ใต้ตัวที่กำลังอ่านมันอยู่"
}

// serverLogPath is where this engine's boot log lives — one file per engine,
// truncated on each start, under the same logs folder everything else writes.
func (s *engineServerSkill) serverLogPath() string {
	root, err := config.DataRoot()
	if err != nil {
		// Somewhere is better than nowhere: a boot log in the temp folder is
		// still a boot log the failure message can read back.
		root = os.TempDir()
	}
	dir := filepath.Join(root, "logs")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "engine-"+s.id+".log")
}

// tailOfFile is the last n lines of a file, for a failure message that should
// carry the server's own words. A file that cannot be read answers with why,
// rather than pretending there was no log.
func tailOfFile(path string, n int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "(อ่าน log ไม่ได้: " + err.Error() + ")"
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	tail := strings.TrimSpace(strings.Join(lines, "\n"))
	if tail == "" {
		return "(log ว่างเปล่า — โปรเซสอาจไม่ทันได้พิมพ์อะไรเลย)"
	}
	return tail
}

// ---------------------------------------------------------------------------
// One start at a time, per engine
// ---------------------------------------------------------------------------

// engineStarting reports whether a start for this engine is still under way,
// and how long it has been going.
//
// Kept on the App rather than on the skill: the skill is rebuilt by every
// re-bootstrap, and a flag that forgot on a model switch would let the very
// second launch it exists to prevent through. Expires on its own after the
// patience the starter waits — a start that has not answered by then is not
// under way any more, it is over, and a stuck flag would leave the engine
// unstartable until the app restarted.
func (a *App) engineStarting(id string) (time.Duration, bool) {
	a.engineStartMu.Lock()
	defer a.engineStartMu.Unlock()
	at, ok := a.engineStartedAt[id]
	if !ok {
		return 0, false
	}
	since := time.Since(at)
	if since >= serverStartPatience {
		delete(a.engineStartedAt, id)
		return 0, false
	}
	return since, true
}

func (a *App) markEngineStarting(id string) {
	a.engineStartMu.Lock()
	defer a.engineStartMu.Unlock()
	if a.engineStartedAt == nil {
		a.engineStartedAt = map[string]time.Time{}
	}
	a.engineStartedAt[id] = time.Now()
}

// clearEngineStarting is called the moment the engine answers, so a genuine
// restart later is not refused by a flag left over from the start that worked.
func (a *App) clearEngineStarting(id string) {
	a.engineStartMu.Lock()
	defer a.engineStartMu.Unlock()
	delete(a.engineStartedAt, id)
}

// stillStartingMessage is what a second caller gets while the first start is
// still going. It is not a failure of theirs to fix — it is a wait — so it says
// what is happening, where to read it, and what not to do.
func stillStartingMessage(label, baseURL, logPath string, since time.Duration) string {
	return fmt.Sprintf(
		"%s ถูกสั่งเปิดไปแล้วเมื่อ %d วินาทีที่แล้วและกำลังขึ้นอยู่ ยังไม่ตอบที่ %s\n\n"+
			"อย่าสั่งเปิดซ้ำ — การเรียกซ้ำจะได้เซิร์ฟเวอร์ตัวที่สองแย่งพอร์ตเดิม "+
			"ให้รอแล้วอ่าน log ที่ %s ด้วย `read` จนกว่าจะเห็นว่ามันพร้อม",
		label, int(since.Seconds()), baseURL, logPath)
}
