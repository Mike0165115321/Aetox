package main

import (
	"context"
	"encoding/json"
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
	"github.com/Mike0165115321/Aetox/internal/turn"
)

// One ordinary chat turn against the real provider, end to end: the system
// prompt the app really builds, the tool batch it really sends, the tool loop
// really executing, files really landing on disk.
//
//	AETOX_LIVE=1 go test ./desktop/ -run TestLiveUnfocusedChat -v -count=1
//
// It exists because every part of the 2026-07-26 root change is only observable
// when a real model drives it: the unfocused root and the session output folder
// have to compose (a doubled "aetox/aetox" or a stranded file shows up here and
// nowhere else), a tool has to find its way back to a file the model asked for
// under a different name, and — the bug that started all of this — the model has
// to answer "where is it on my machine" with a path that exists rather than one
// it assembled out of a folder and a filename.
func TestLiveUnfocusedChatWritesReadsAndReportsWhereItLanded(t *testing.T) {
	key := liveDeepSeekKey(t)

	// t.TempDir() stands in for <home>/aetox — the real unfocused root. The
	// subdir is exactly what App.outputSubdir returns for a chat.
	root := t.TempDir()
	const subdir = "output/20260726-120000.000"
	registry := skill.NewDefaultRegistry(skill.RegistryOptions{
		SandboxRoot:  root,
		OutputSubdir: func() string { return subdir },
	})

	provider, err := model.NewProvider(model.ProviderOptions{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   key,
		Timeout:  5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider: provider,
		Model:    "deepseek-v4-flash",
		// The real thing, not a test string: if the prompt stops telling the
		// model to repeat reported paths, this test is where it shows.
		SystemPrompt: prompt.Build(prompt.SurfaceDesktop, prompt.Scope{Root: root}),
		MaxToolCalls: 12, // nobody is watching this loop; the app's brakes are a human
	})

	var mu sync.Mutex
	var called []string
	exec := turn.NewExecutor(turn.ExecutorOptions{
		Agent:      agent,
		Dispatcher: skill.NewDispatcher(registry),
		// Full access is what an unfocused chat runs with. No Approve func is
		// passed on purpose: if anything in this turn asks for approval the
		// turn fails rather than hanging, which is the honest outcome.
		ApprovalMode: safety.ApprovalFullAccess,
		OnToolAction: func(ev turn.ToolEvent) {
			if ev.Action == "call" {
				mu.Lock()
				called = append(called, ev.Name)
				mu.Unlock()
			}
		},
	})

	ask := "สร้างไฟล์ landing.html เป็นหน้าเว็บง่ายๆ หัวข้อว่า Aetox แล้วอ่านไฟล์กลับมายืนยันว่าเขียนสำเร็จ " +
		"จากนั้นบอกด้วยว่าไฟล์อยู่ที่ไหนในเครื่อง"
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	result, err := exec.Execute(ctx, ask, command.Intent{Raw: ask, Kind: command.KindConversation}, nil, nil, nil)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	mu.Lock()
	tools := append([]string{}, called...)
	mu.Unlock()
	t.Logf("tools called: %s", strings.Join(tools, " → "))
	t.Logf("reply:\n%s", result.Reply)

	if len(tools) == 0 {
		t.Fatalf("no tool ran in a turn that asked for a file to be created; reply was %q", result.Reply)
	}

	// 1. The file landed where the two halves of the path say it should.
	landed := filepath.Join(root, filepath.FromSlash(subdir), "landing.html")
	if _, statErr := os.Stat(landed); statErr != nil {
		t.Fatalf("landing.html is not at %s: %v", landed, statErr)
	}
	// 2. And nowhere else. A doubled prefix is the failure this guards.
	for _, wrong := range []string{
		filepath.Join(root, "landing.html"),
		filepath.Join(root, "aetox", "output", "20260726-120000.000", "landing.html"),
	} {
		if _, statErr := os.Stat(wrong); statErr == nil {
			t.Errorf("a second copy landed at %s", wrong)
		}
	}
	// 3. Reading it back is the round-trip: the model asks for "landing.html",
	//    which exists at no such path, and PlacedPath has to find it anyway.
	if !contains(tools, "read") {
		t.Errorf("the model never read the file back; tools were %v", tools)
	}

	// 4. The answer the user actually reads. The old failure was a path built
	//    from the root and the filename the model typed — a file that is not
	//    there. Assert the wrong path is absent rather than a right one is
	//    present, because there are several correct ways to say it.
	if strings.Contains(result.Reply, filepath.Join(root, "landing.html")) {
		t.Errorf("the reply points at a file that does not exist (root + filename):\n%s", result.Reply)
	}
	if !strings.Contains(result.Reply, "landing.html") {
		t.Errorf("the reply never names the file it created:\n%s", result.Reply)
	}
}

// liveDeepSeekKey reads the key the app itself uses, so a live test runs
// against the same provider the user's desktop does. Skips rather than fails
// when there is no key — CI has none, and a red suite there would say nothing
// about the code.
func liveDeepSeekKey(t *testing.T) string {
	t.Helper()
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live tests")
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "aetox", "model-preference.json"))
	if err != nil {
		t.Skipf("no model-preference.json: %v", err)
	}
	var pref struct {
		ProviderAPIKeys map[string]string `json:"provider_api_keys"`
	}
	if err := json.Unmarshal(data, &pref); err != nil {
		t.Fatalf("preference file unreadable: %v", err)
	}
	key := strings.TrimSpace(pref.ProviderAPIKeys["deepseek"])
	if key == "" {
		t.Skip("no deepseek key configured")
	}
	return key
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
