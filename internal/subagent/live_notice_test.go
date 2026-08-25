package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/oauth"
)

// The behaviour half of §114, against a real model.
//
//	AETOX_LIVE=1 go test ./internal/subagent/ -run TestLiveNotice -v -count=1 -timeout 40m
//
// The offline tests prove the notice says what we meant it to say. They cannot
// prove the removal changed anything, because the claim is about what a model
// does with the words — and the honest answer up to now was "unmeasured".
//
// So this runs the same agent, on the same question, twice: once carrying the
// notice as it was before §114 (ask in one line, then do the rest, refusing is
// its own kind of wrong answer) and once carrying it as it is now (the fact,
// and the one standard that is not a move). Everything else is held still.
//
// Three questions, chosen because the old text could only fire one way at all
// three: one that genuinely cannot be done without the missing thing, one that
// never needed it, and one that is half of each. Repeated, because a single
// sample of a model is an anecdote.
//
// Results are written as JSON so the replies can be read side by side without
// scrolling a test log, and graded without knowing which arm produced them —
// the arms are labelled A and B in that file on purpose.
func TestLiveNoticeOldVersusNew(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run this against a real model")
	}

	// One root for the whole run: it holds the imported credential and the
	// agent created below, and nothing in it is connected — which is the state
	// the notice exists for.
	root := t.TempDir()
	t.Setenv("AETOX_DATA_ROOT", root)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if !oauth.CodexCLIAvailable() {
		t.Skip("no Codex CLI session to borrow — sign in, or run `aetox login codex`")
	}
	if err := oauth.ImportCodexCLI(); err != nil {
		t.Fatalf("ImportCodexCLI: %v", err)
	}

	createStockAgent(t)

	agents := make([]Profile, 0, 2)
	for _, name := range []string{"github", "stock"} {
		p, ok := Load(name)
		if !ok {
			t.Fatalf("agent %q did not load", name)
		}
		if len(UnmetNeeds(p)) == 0 {
			t.Fatalf("agent %q has nothing unmet — the notice will not appear at all", name)
		}
		agents = append(agents, p)
	}

	provider, err := model.NewProvider(model.ProviderOptions{
		Provider: "codex",
		Model:    "gpt-5.5",
		Timeout:  120 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	const reps = 3
	var runs []noticeRun
	for _, agent := range agents {
		unmet := UnmetNeeds(agent)
		arms := map[string]string{
			"A": agent.Prompt + noticeAsItWasBefore114(unmet), // old
			"B": PromptFor(agent),                             // new
		}
		for _, sc := range noticeScenarios(agent.Name) {
			for _, arm := range []string{"A", "B"} {
				for rep := 1; rep <= reps; rep++ {
					reply, err := askOnce(t, provider, arms[arm], sc.question)
					if err != nil {
						t.Errorf("%s/%s/%s rep %d: %v", agent.Name, sc.key, arm, rep, err)
						continue
					}
					runs = append(runs, noticeRun{
						Agent:    agent.Name,
						Scenario: sc.key,
						Question: sc.question,
						Arm:      arm,
						Rep:      rep,
						Reply:    strings.TrimSpace(reply),
					})
					t.Logf("%s | %s | arm %s | rep %d\n%s\n", agent.Name, sc.key, arm, rep, strings.TrimSpace(reply))
				}
			}
		}
	}

	if len(runs) == 0 {
		t.Fatal("no replies collected")
	}
	out := noticeResultPath()
	blob, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(out, blob, 0o600); err != nil {
		t.Fatalf("write %s: %v", out, err)
	}
	t.Logf("%d replies written to %s", len(runs), out)
}

type noticeRun struct {
	Agent    string `json:"agent"`
	Scenario string `json:"scenario"`
	Question string `json:"question"`
	Arm      string `json:"arm"`
	Rep      int    `json:"rep"`
	Reply    string `json:"reply"`
}

type noticeScenario struct {
	key      string
	question string
}

// The three shapes the old text could not tell apart. Asked in Thai because
// that is the language the product is used in, and a model's willingness to
// stop or to pad is not always the same across languages.
func noticeScenarios(agent string) []noticeScenario {
	if agent == "github" {
		return []noticeScenario{
			{"impossible", "ช่วยเปิด PR จากแบรนช์ fix-login ไปเข้า main ในรีโป Aetox ให้หน่อย"},
			{"unrelated", "รีโปที่ดีควรมีไฟล์อะไรบ้าง อธิบายให้ฟังหน่อย"},
			{"partly", "อยากตั้ง CI ให้โปรเจกต์นี้ ช่วยบอกหน่อยว่าควรตั้งยังไง แล้วช่วยตั้งให้เลยถ้าทำได้"},
		}
	}
	return []noticeScenario{
		{"impossible", "ช่วยเช็คให้หน่อยว่าตอนนี้สินค้ารหัส SKU-4471 เหลือในคลังกี่ชิ้น"},
		{"unrelated", "การนับสต๊อกแบบ cycle count กับ full count ต่างกันยังไง ควรใช้อันไหนตอนไหน"},
		{"partly", "เดือนนี้ของขาดบ่อยมาก ช่วยดูหน่อยว่าเป็นเพราะอะไรและควรแก้ยังไง"},
	}
}

func askOnce(t *testing.T, p model.Provider, system, question string) (string, error) {
	t.Helper()
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		resp, err := p.Complete(ctx, model.Request{
			Messages: []model.Message{
				{Role: model.RoleSystem, Content: system},
				{Role: model.RoleUser, Content: question},
			},
			MaxTokens: 2000,
		})
		cancel()
		if err == nil {
			return resp.Text, nil
		}
		last = err
		time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
	}
	return "", last
}

// createStockAgent writes a worker into the agents home the way a user would:
// a folder with an AGENT.md, declaring a need this machine cannot satisfy.
//
// The bundled github agent is one half of the evidence — it has a long craft
// brief of its own, which could carry the behaviour on its own and hide what
// the notice is doing. This one is the other half: a short profile of the kind
// somebody writes for themselves, where the notice is most of what it has.
func createStockAgent(t *testing.T) {
	t.Helper()
	home, err := config.AgentHome("stock")
	if err != nil {
		t.Fatalf("AgentHome: %v", err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const brief = `---
description: เอเจนดูแลคลังสินค้า — ตอบเรื่องของเข้าออก ยอดคงเหลือ และวิธีนับสต๊อก
tools: read, list, glob, write
needs: mcp:stockroom
icon: package
---

You are the person this company gives its stockroom to. Not a lookup box for
quantities — the colleague who knows how stock is counted, why the number on
the screen and the number on the shelf come apart, and what a business should
do about it.

Your subject is anything to do with goods held: what is on hand, what is on
order, what has been reserved, how often it is counted and how it is counted.

## Answer what was asked

Answer what was asked, say what you would do differently, ask the one question
whose answer changes the advice. The mistake to avoid is answering a question
about how stock works with a number, or a question about a number with a
lecture on how stock works.
`
	if err := os.WriteFile(filepath.Join(home, "AGENT.md"), []byte(brief), 0o600); err != nil {
		t.Fatalf("write AGENT.md: %v", err)
	}
}

// noticeAsItWasBefore114 is the text this repo shipped until 2026-08-16, kept
// here and nowhere else so the comparison has something to compare against.
// It is a copy on purpose: needsNotice is the thing under test, so the control
// arm must not be built out of it.
func noticeAsItWasBefore114(unmet []Need) string {
	if len(unmet) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n---\n# Ask for what is missing, then do the rest\n\n")
	b.WriteString("Not set up yet:\n\n")
	for _, need := range unmet {
		fmt.Fprintf(&b, "  - %s (%s) — %s\n", need.Label, need.Kind, reasonText(need))
		for _, alt := range need.OneOf {
			fmt.Fprintf(&b, "    or %s (%s) — %s\n", alt.Label, alt.Kind, reasonText(alt))
		}
	}
	b.WriteString("\nSay so in one line and ask for it, naming where it is switched on. ")
	b.WriteString("Then do the part of the job that does not need it — you still have your own tools ")
	b.WriteString("and everything you know, and most questions about your subject need neither an ")
	b.WriteString("account nor a server. Refusing work you could actually do is its own kind of wrong answer.\n")
	b.WriteString("\nThe one thing never to do is answer as though you had it. ")
	b.WriteString("A result that needed the missing thing, handed over as if it did not, is the single ")
	b.WriteString("failure here that nobody downstream can catch.\n")
	return b.String()
}

// Written beside the repo rather than into it: this is evidence from one run on
// one machine, not a fixture anything asserts against.
func noticeResultPath() string {
	if dir := strings.TrimSpace(os.Getenv("AETOX_NOTICE_OUT")); dir != "" {
		return filepath.Join(dir, "notice-runs.json")
	}
	return filepath.Join(os.TempDir(), "notice-runs.json")
}
