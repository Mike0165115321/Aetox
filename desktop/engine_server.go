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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/config"
	"github.com/Mike0165115321/Aetox/internal/connect"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/skill"
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
	if err := launchLogged(saved, logPath); err != nil {
		return fail("เปิด "+row.Label+" ไม่สำเร็จ: "+err.Error(), err)
	}
	// The front-row seat: a desk terminal following the boot log live. Failing
	// to open one is not failing to start the server — headless runs and a
	// window that is not up yet lose the view, not the engine.
	seated := ""
	if _, err := s.app.openDeskTerminal(tailCommand(logPath)); err == nil {
		seated = " ผู้ใช้เห็น log มันวิ่งอยู่ในเทอร์มินัลบนโต๊ะ"
	}
	if !waitReachable(row.BaseURL, serverStartPatience) {
		// The log tail rides along on failure, because this sentence is what
		// the model acts on. The night this file was reshaped, the model got
		// only "ยังไม่ตอบใน 90 วินาที" and spent twenty-nine tool calls groping
		// at processes and ports for the reason — which had been printed,
		// readable, in a place nothing handed to it.
		return fail("สั่งเปิด "+row.Label+" แล้วแต่ "+row.BaseURL+" ยังไม่ตอบใน 90 วินาที — ท้าย log ของมันบอกว่า:\n"+tailOfFile(logPath, 15), nil)
	}
	out.Success = true
	out.Content = "เปิด " + row.Label + " แล้ว ตอบที่ " + row.BaseURL + " —" + seated + " พร้อมทำงาน"
	out.DurationMs = time.Since(start).Milliseconds()
	return out, nil
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
