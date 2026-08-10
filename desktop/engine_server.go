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
	"strings"
	"time"

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

	// The agent starts its engine on its own desk (owner's call, 2026-08-11:
	// "มันก็เปิดเทอมินอลขึ้นมารันเองฝั่งโต๊ะทำงานมัน") — a terminal pane in the
	// workbench, not the detached console the Settings button opens. Same
	// stored command, same patience, different window, and the difference is
	// the point: the user is sitting in this session watching the agent work,
	// and a server coming up is part of the work. The console the Settings
	// button spawns is right for a click made from a page with no desk; from
	// here it would be a window appearing outside the app for a thing the
	// agent supposedly did in front of you.
	//
	// One behavioural consequence, accepted on purpose: a server in a desk
	// terminal lives with the app, where the detached console outlives it. For
	// the agent's own working session that is the better default — no orphan
	// engine still holding a port tomorrow — and an engine meant to run
	// forever is what the Settings button and a real service install are for.
	saved := strings.TrimSpace(row.StartCommand)
	if command != "" {
		saved = command
	}
	if saved == "" {
		return fail("ยังไม่ได้บอกว่าจะเปิดเซิร์ฟเวอร์นี้ด้วยคำสั่งอะไร", nil)
	}
	shellName, err := s.app.openDeskTerminal(saved)
	if err != nil {
		// No desk to open on — a headless run, or the window not up yet. The
		// Settings door still works from anywhere, so the engine still starts;
		// it is only the seat in the front row that could not be given.
		if err := s.app.StartConnectionServer(s.id); err != nil {
			return fail(err.Error(), err)
		}
		out.Success = true
		out.Content = "เปิด " + row.Label + " แล้ว ตอบที่ " + row.BaseURL + " — พร้อมทำงาน"
		out.DurationMs = time.Since(start).Milliseconds()
		return out, nil
	}
	if !waitReachable(row.BaseURL, serverStartPatience) {
		return fail("สั่งเปิด "+row.Label+" ในเทอร์มินัลบนโต๊ะแล้ว แต่ "+row.BaseURL+" ยังไม่ตอบใน 90 วินาที — อ่านเทอร์มินัลนั้นดูว่ามันติดอะไร", nil)
	}
	out.Success = true
	out.Content = "เปิด " + row.Label + " ในเทอร์มินัล " + shellName + " บนโต๊ะแล้ว ตอบที่ " + row.BaseURL + " — ผู้ใช้เห็นมันรันอยู่ พร้อมทำงาน"
	out.DurationMs = time.Since(start).Milliseconds()
	return out, nil
}
