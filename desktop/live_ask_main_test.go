package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/command"
	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/prompt"
	"github.com/Mike0165115321/Aetox/internal/safety"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
	"github.com/Mike0165115321/Aetox/internal/think"
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// A real model on both sides of the ask: a delegate hits a decision it cannot
// make, asks, waits, and finishes the job with the answer.
//
//	AETOX_LIVE=1 go test ./desktop/ -run TestLiveSubAgentAsks -v -count=1
//
// The offline test drives the same path with a scripted delegate, which proves
// the machinery. This proves the part a script cannot: that the question and the
// answer survive two real models, two system prompts and two tool loops — and
// that the delegate acts on the answer rather than on its own guess. The proof
// is on disk: exactly one of the two config files changes, and it is the one the
// main agent named.
//
// It tests the mechanism, not the model's judgement about when to ask — the
// brief says not to guess, because a flash model left to decide might not.
func TestLiveSubAgentAsksTheMainAgentWhenStuck(t *testing.T) {
	key := liveDeepSeekKey(t)

	root := t.TempDir()
	for name, body := range map[string]string{
		"config.dev.json":  "{\n  \"timeout\": 5\n}\n",
		"config.prod.json": "{\n  \"timeout\": 5\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{SandboxRoot: root})

	provider, err := model.NewProvider(model.ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   key,
		Timeout:  5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	var mu sync.Mutex
	type ev struct{ name, parent string }
	var events []ev
	record := func(e turn.ToolEvent) {
		if e.Action != "call" {
			return
		}
		mu.Lock()
		events = append(events, ev{e.Name, e.Parent})
		mu.Unlock()
	}

	for _, tool := range subagent.NewTaskTools(subagent.TaskOptions{
		Provider:     provider,
		Model:        "deepseek-v4-flash",
		Registry:     registry,
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: record,
		MaxChars:     400_000,
		ThinkLevel:   think.LevelNoThinking,
	}) {
		if regErr := registry.Register(tool, skill.SourceBuiltin); regErr != nil {
			t.Fatalf("register %s: %v", tool.Name(), regErr)
		}
	}

	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     provider,
		Model:        "deepseek-v4-flash",
		SystemPrompt: prompt.Build(prompt.SurfaceDesktop, root),
		MaxToolCalls: 14,
	})
	exec := turn.NewExecutor(turn.ExecutorOptions{
		Agent:        agent,
		Dispatcher:   skill.NewDispatcher(registry),
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: record,
	})

	ask := "ส่งงานให้ซับเอเจน general ด้วย task โดยสั่งมันว่า: " +
		"\"ในโฟลเดอร์นี้มีไฟล์ config อยู่สองไฟล์ ให้แก้ค่า timeout เป็น 60 ในไฟล์เดียวเท่านั้น " +
		"ห้ามเดาเองว่าไฟล์ไหน ถ้าไม่รู้ให้ใช้ ask_main ถามก่อน\" " +
		"ถ้ามันถามกลับมาว่าไฟล์ไหน ให้ตอบด้วย task_answer ว่า config.prod.json " +
		"แล้วเก็บผลด้วย task_result ให้เรียบร้อย"
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	result, err := exec.Execute(ctx, ask, command.Intent{Raw: ask, Kind: command.KindConversation}, nil, nil, nil)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	mu.Lock()
	seen := append([]ev{}, events...)
	mu.Unlock()
	for _, e := range seen {
		if e.parent == "" {
			t.Logf("main   : %s", e.name)
		} else {
			t.Logf("  └ sub: %s", e.name)
		}
	}
	t.Logf("reply:\n%s", result.Reply)

	var askedMain, answered bool
	for _, e := range seen {
		if e.name == "ask_main" && e.parent != "" {
			askedMain = true
		}
		if e.name == "task_answer" && e.parent == "" {
			answered = true
		}
	}
	if !askedMain {
		t.Fatalf("the delegate never asked; tools were %v", seen)
	}
	if !answered {
		t.Fatalf("the main agent never answered — the delegate was left parked")
	}

	// The proof that the answer actually steered the work.
	prod, err := os.ReadFile(filepath.Join(root, "config.prod.json"))
	if err != nil {
		t.Fatalf("read prod config: %v", err)
	}
	dev, err := os.ReadFile(filepath.Join(root, "config.dev.json"))
	if err != nil {
		t.Fatalf("read dev config: %v", err)
	}
	t.Logf("prod: %s", strings.TrimSpace(string(prod)))
	t.Logf("dev : %s", strings.TrimSpace(string(dev)))
	if !strings.Contains(string(prod), "60") {
		t.Errorf("the delegate resumed but did not apply the decision to config.prod.json: %s", prod)
	}
	if strings.Contains(string(dev), "60") {
		t.Errorf("the delegate edited the file it was told not to: %s", dev)
	}
}
