package main

import (
	"context"
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

// Delegation end to end against the real provider and the real internet:
//
//	AETOX_LIVE=1 go test ./desktop/ -run TestLiveSubAgent -v -count=1
//
// The offline tests drive the machinery with a scripted provider, which proves
// the wiring but not that a model reaches for it. This one asks a real model to
// hand a job to a sub-agent, and the sub-agent to go and search the web — the
// full §44 path: task returns a handle without blocking, the delegate runs its
// own tool loop in the background with its own registry, its tool events come
// back stamped with the parent call's id, and task_result redeems the handle.
func TestLiveSubAgentSearchesTheWebAndReportsBack(t *testing.T) {
	key := liveDeepSeekKey(t)

	root := t.TempDir()
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

	// Exactly the pair desktop/app.go registers, with the same options.
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
		SystemPrompt: prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: root}),
		MaxToolCalls: 12,
	})
	exec := turn.NewExecutor(turn.ExecutorOptions{
		Agent:        agent,
		Dispatcher:   skill.NewDispatcher(registry),
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: record,
	})

	ask := "ส่งงานให้ซับเอเจน general ด้วยเครื่องมือ task: ให้มันค้นเว็บว่าภาษา Go " +
		"เวอร์ชันเสถียรล่าสุดคือเวอร์ชันอะไร แล้วเก็บผลด้วย task_result และสรุปคำตอบมาให้หน่อย"
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
			t.Logf("  └ sub: %s (parent %s)", e.name, e.parent)
		}
	}
	t.Logf("reply:\n%s", result.Reply)

	var startedTask, collected, searched bool
	for _, e := range seen {
		switch {
		case e.name == "task" && e.parent == "":
			startedTask = true
		case e.name == "task_result" && e.parent == "":
			collected = true
		case e.parent != "" && (e.name == "web_search" || e.name == "web_fetch"):
			searched = true
		}
	}
	if !startedTask {
		t.Fatalf("the model never delegated; tools were %v", seen)
	}
	if !collected {
		t.Error("the model started a sub-agent and never collected it — that work is thrown away at the end of the turn")
	}
	// A sub-agent's events arriving with a parent id is what lets the UI nest
	// them instead of mixing them into the main agent's timeline (§44.5).
	if !searched {
		t.Error("no web_search/web_fetch ran inside the sub-agent — either the delegate never reached the internet, or its events lost the parent stamp")
	}
	if strings.TrimSpace(result.Reply) == "" {
		t.Error("empty reply after a delegated web search")
	}
}

// Two delegates on the real internet at the same time, collected in one call.
//
//	AETOX_LIVE=1 go test ./desktop/ -run TestLiveTwoSubAgents -v -count=1
//
// This is the claim §44.11 makes that a single-delegate test cannot check: N
// started before the first collect run concurrently, so the wall clock is the
// slowest one rather than the sum. It is also where the failures a scripted
// provider cannot produce live — two goroutines hitting the same relay, the same
// runner map and the same usage reporter, with two real models writing tool
// calls at their own pace.
func TestLiveTwoSubAgentsRunAtOnceAndAreCollectedTogether(t *testing.T) {
	key := liveDeepSeekKey(t)

	root := t.TempDir()
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
	type ev struct{ name, parent, agent string }
	var events []ev
	record := func(e turn.ToolEvent) {
		if e.Action != "call" {
			return
		}
		mu.Lock()
		events = append(events, ev{e.Name, e.Parent, e.Agent})
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
		SystemPrompt: prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: root}),
		MaxToolCalls: 14,
	})
	exec := turn.NewExecutor(turn.ExecutorOptions{
		Agent:        agent,
		Dispatcher:   skill.NewDispatcher(registry),
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: record,
	})

	ask := "เปิดซับเอเจน general สองตัวพร้อมกันด้วย task (สั่งตัวแรกให้เสร็จก่อนค่อยสั่งตัวที่สอง ห้ามรอผลระหว่างนั้น): " +
		"ตัวที่หนึ่งให้ค้นเว็บว่าภาษา Go เวอร์ชันเสถียรล่าสุดคืออะไร " +
		"ตัวที่สองให้ค้นเว็บว่า Rust เวอร์ชันเสถียรล่าสุดคืออะไร " +
		"แล้วเก็บผลทั้งสองด้วย task_result ครั้งเดียวโดยใส่ task id ทั้งสองคั่นด้วยจุลภาค " +
		"สุดท้ายสรุปทั้งสองเวอร์ชันมาให้"
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	startedAt := time.Now()
	result, err := exec.Execute(ctx, ask, command.Intent{Raw: ask, Kind: command.KindConversation}, nil, nil, nil)
	elapsed := time.Since(startedAt)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}

	mu.Lock()
	seen := append([]ev{}, events...)
	mu.Unlock()
	for _, e := range seen {
		if e.parent == "" {
			t.Logf("main   : %-12s %s", e.name, e.agent)
		} else {
			t.Logf("  └ sub: %-12s (parent %s)", e.name, e.parent)
		}
	}
	t.Logf("turn took %s", elapsed.Round(time.Second))
	t.Logf("reply:\n%s", result.Reply)

	// Two distinct delegations, each with its own parent id on its own steps.
	parents := map[string]int{}
	var tasksStarted, namedAgents int
	for _, e := range seen {
		if e.parent != "" {
			parents[e.parent]++
			continue
		}
		if e.name == "task" {
			tasksStarted++
			if e.agent != "" {
				namedAgents++
			}
		}
	}
	if tasksStarted < 2 {
		t.Fatalf("only %d delegate(s) started; the turn was %v", tasksStarted, seen)
	}
	if namedAgents < 2 {
		t.Errorf("%d of %d task calls carried the sub-agent name the UI titles the block with", namedAgents, tasksStarted)
	}
	if len(parents) < 2 {
		t.Errorf("delegate steps came back under %d parent id(s), want 2 — the UI would merge two sub-agents into one block: %v", len(parents), parents)
	}
	for id, n := range parents {
		t.Logf("delegate %s ran %d tools", id, n)
	}
	if strings.TrimSpace(result.Reply) == "" {
		t.Fatal("empty reply")
	}
}
