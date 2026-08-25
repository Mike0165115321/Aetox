// Package aetox exists so this directory can hold one thing: a way to talk to
// Aetox for real, from the repo root, without building or launching anything.
package aetox

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/cognitive"
	"github.com/Mikedev115/Aetox/internal/command"
	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/prompt"
	"github.com/Mikedev115/Aetox/internal/safety"
	"github.com/Mikedev115/Aetox/internal/skill"
	"github.com/Mikedev115/Aetox/internal/subagent"
	"github.com/Mikedev115/Aetox/internal/think"
	"github.com/Mikedev115/Aetox/internal/turn"
)

// TestAskAetox is one real conversation with the real engine — the provider the
// user has configured, the system prompt the app really builds, the whole tool
// set, tools really running against a real folder.
//
// It exists because reading the code you just wrote is not evidence. A tool can
// compile, pass its unit test, and still be unusable from a model's side: the
// description misleads, the error blames the wrong thing, the arguments cannot
// be produced from what the model actually knows. None of that is visible from
// the inside, and all of it shows up here in one turn.
//
//	AETOX_LIVE=1 go test . -run TestAskAetox -v -count=1 -timeout 30m
//	AETOX_LIVE=1 AETOX_ASK="ลอง web_fetch แล้วบอกว่าพังยังไง" go test . -run TestAskAetox -v -count=1 -timeout 30m
//
// Environment:
//
//	AETOX_LIVE=1   required; without it this skips like every other live test
//	AETOX_ASK      what to say. Defaults to the tool survey below.
//	AETOX_ROOT     folder the tools work in. Defaults to a fresh temp dir —
//	               point it at a real project when the question is about one.
//	AETOX_PROVIDER override the provider from model-preference.json (set
//	               AETOX_MODEL too — a model name does not survive the switch)
//	AETOX_MODEL    override the model from model-preference.json
//
// Two reports come back and they are not the same thing. The TOOL LOG is the
// engine's own ToolEvent stream: what really ran, what really failed, in the
// engine's words. AETOX SAYS is the model's account of it. When they disagree,
// the tool log is right and the disagreement is itself the finding.
//
// The test fails only when the harness fails — no provider, no key, a turn that
// errored out, or a conversation where nothing ran at all. A tool that failed is
// the *output*, not a failure: `git status` outside a repo is a correct refusal.
//
// Desktop-only tools (browser_*, ask_user, todo_write, the file editor) need a
// window and are not in this registry. They can only be probed by running the
// app.
func TestAskAetox(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to talk to a real model")
	}

	pref, _, err := config.LoadModelPreference()
	if err != nil {
		t.Skipf("no saved provider to talk to: %v", err)
	}
	// Whatever the app is currently pointed at, unless told otherwise — the
	// preference file is shared, so it can be a small local model that was fine
	// for a chat and is not fine for a forty-call survey.
	provider := strings.TrimSpace(pref.ModelProvider)
	modelName := strings.TrimSpace(pref.ModelName)
	if override := strings.TrimSpace(os.Getenv("AETOX_PROVIDER")); override != "" {
		provider = override
		modelName = "" // a model name from another provider means nothing here
	}
	if override := strings.TrimSpace(os.Getenv("AETOX_MODEL")); override != "" {
		modelName = override
	}
	if provider == "" || modelName == "" {
		t.Skip("no provider/model saved — pick one in the app first")
	}
	key := strings.TrimSpace(pref.ModelAPIKeys[provider])
	if key == "" {
		key = strings.TrimSpace(os.Getenv("AETOX_API_KEY"))
	}
	if key == "" && model.RequiresAPIKey(model.NormalizeProvider(provider)) {
		t.Skipf("no key saved for %s", provider)
	}

	root := strings.TrimSpace(os.Getenv("AETOX_ROOT"))
	if root == "" {
		root = t.TempDir()
	}
	ask := strings.TrimSpace(os.Getenv("AETOX_ASK"))
	if ask == "" {
		ask = defaultToolSurvey
	}
	t.Logf("provider=%s model=%s root=%s", provider, modelName, root)

	p, err := model.NewProvider(model.ProviderOptions{
		Provider: provider,
		Model:    modelName,
		APIKey:   key,
		BaseURL:  pref.ModelBaseURLs[provider],
		Timeout:  10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})
	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     p,
		Model:        modelName,
		SystemPrompt: prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: root}),
		MaxToolCalls: 40, // a survey needs room; nobody is watching this loop
	})
	// Delegation is part of the tool set both front ends register, so a probe
	// without it would be testing a set no user actually has.
	for _, tool := range subagent.NewTaskTools(subagent.TaskOptions{
		Provider:     p,
		Model:        modelName,
		Registry:     registry,
		ApprovalMode: safety.ApprovalFullAccess,
		ThinkLevel:   think.NormalizeLevel(pref.ThinkLevel),
	}) {
		if err := registry.Register(tool, skill.SourceBuiltin); err != nil {
			t.Logf("skipped %s: %v", tool.Name(), err)
		}
	}

	var mu sync.Mutex
	var log []turn.ToolEvent
	exec := turn.NewExecutor(turn.ExecutorOptions{
		Agent:      agent,
		Dispatcher: skill.NewDispatcher(registry),
		// Full access with no Approve func: a probe that stops to ask a human
		// nobody is watching would hang until the timeout instead of reporting.
		ApprovalMode: safety.ApprovalFullAccess,
		TurnOptions:  turn.TurnOptions{ThinkLevel: think.NormalizeLevel(pref.ThinkLevel)},
		OnToolAction: func(ev turn.ToolEvent) {
			mu.Lock()
			log = append(log, ev)
			mu.Unlock()
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	started := time.Now()
	result, execErr := exec.Execute(ctx, ask, command.Intent{Raw: ask, Kind: command.KindConversation}, nil, nil, nil)

	mu.Lock()
	events := append([]turn.ToolEvent{}, log...)
	mu.Unlock()

	t.Logf("\n=== ASKED (%s) ===\n%s", time.Since(started).Round(time.Second), ask)
	t.Logf("\n=== TOOL LOG (the engine's own account) ===\n%s", renderToolLog(events))
	if execErr != nil {
		t.Fatalf("turn failed before it could report anything: %v", execErr)
	}
	t.Logf("\n=== AETOX SAYS ===\n%s", strings.TrimSpace(result.Reply))

	if countCalls(events) == 0 {
		t.Fatalf("nothing ran — the model answered a tool question from memory, or no tool reached it")
	}
}

// defaultToolSurvey asks for the one thing the code cannot tell us: what using
// these tools is actually like. It insists on real calls and on quoting real
// errors, because a model asked "are your tools OK" will happily say yes.
const defaultToolSurvey = `นี่คือการตรวจเครื่องมือ ไม่ใช่การคุยเล่น และนี่คือโฟลเดอร์ทดสอบ ทำอะไรก็ได้ในนี้

ให้ลองใช้เครื่องมือของคุณจริงๆ ให้ครบเท่าที่ทำได้ — เขียนไฟล์ อ่านกลับ แก้ไข ค้นหาด้วย grep/glob list โฟลเดอร์
รันคำสั่งด้วย shell ลอง git ลองดึงเว็บ ลองสั่งงานซับเอเจน และอะไรก็ตามที่คุณมีแต่ผมไม่ได้เอ่ยชื่อ

แล้วรายงานกลับมาเป็นตาราง โดยหนึ่งบรรทัดต่อหนึ่งเครื่องมือ:
  ชื่อเครื่องมือ | เรียกจริงหรือไม่ | ผ่าน/พัง | ข้อความ error จริงที่ได้ (คัดลอกมาตรงๆ) | สิ่งที่คุณคาดว่าจะเกิดแต่ไม่เกิด

กติกา:
- ห้ามเดา เครื่องมือไหนไม่ได้เรียกจริงให้เขียนว่า "ไม่ได้เรียก" อย่าเขียนว่าผ่าน
- error ให้คัดลอกข้อความจริงมา ไม่ต้องสรุปเป็นภาษาสวยๆ
- ถ้าคำอธิบายเครื่องมือทำให้คุณเข้าใจผิด หรืออาร์กิวเมนต์ที่มันขอคุณหาไม่ได้ ให้บอกด้วย — นั่นคือปัญหาแบบที่โค้ดมองไม่เห็น
- ปิดท้ายด้วย 3 อย่างที่ควรแก้เร่งด่วนที่สุด เรียงตามความสำคัญ`

func countCalls(events []turn.ToolEvent) int {
	n := 0
	for _, ev := range events {
		if ev.Action == "call" {
			n++
		}
	}
	return n
}

// renderToolLog prints what really happened, then the failures again on their
// own — the second list is the answer to "which tool has a problem", and it is
// the one worth reading first.
func renderToolLog(events []turn.ToolEvent) string {
	var b strings.Builder
	failures := map[string][]string{}
	for _, ev := range events {
		switch ev.Action {
		case "call":
			fmt.Fprintf(&b, "  → %s\n", ev.Label())
		case "result":
			status := "ok"
			if !ev.OK {
				status = "FAILED"
				failures[ev.Name] = append(failures[ev.Name], strings.TrimSpace(ev.Error))
			}
			fmt.Fprintf(&b, "    %s %s %s\n", status, ev.Label(), strings.TrimSpace(ev.Error))
		case "note":
			fmt.Fprintf(&b, "  ~ %s\n", truncateLine(ev.Text, 160))
		}
	}
	if b.Len() == 0 {
		b.WriteString("  (no tool ran)\n")
	}
	if len(failures) > 0 {
		names := make([]string, 0, len(failures))
		for name := range failures {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString("\n--- tools that failed ---\n")
		for _, name := range names {
			for _, why := range failures[name] {
				fmt.Fprintf(&b, "  %s: %s\n", name, truncateLine(why, 400))
			}
		}
	}
	return b.String()
}

func truncateLine(s string, max int) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
