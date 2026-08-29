package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/debuglog"
	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// Stage three of the self-optimize loop: turning a flagged misfire into a
// proposed skill edit (docs/architecture/self-optimize-loop-2026-08-26.md).
//
// The detector (skilltune.go) is deterministic; this is not — drafting an edit
// to prose needs a model to read the skill and the misfires and say what to
// change. That one non-deterministic step sits behind an interface so the
// pipeline around it is testable without an LLM, and behind human approval so a
// draft is only ever a proposal: nothing it produces touches a skill until a
// person clicks yes.

// skillEditDrafter reads a flagged skill and the results that got bad ratings,
// and proposes one edit to its SKILL.md. The App's real implementation calls a
// model; a test supplies a fake.
type skillEditDrafter interface {
	Draft(ctx context.Context, skillName, skillText, evidence string) (op, before, body, reason string, err error)
}

// generateSkillRefinements drafts an edit for each skill the detector flags and
// queues it for approval — skipping any skill that already has a proposal
// waiting, so a skill is not re-drafted and the model not re-called every pass.
func (a *App) generateSkillRefinements(ctx context.Context, drafter skillEditDrafter) {
	if !learningEnabled() || drafter == nil {
		return
	}
	// Through skillMisfires, not detectSkillMisfires: the App reader already
	// opens the database and supplies the live skill set, and this function
	// having its own copy of both meant the two could drift apart on which
	// names count as skills.
	for _, m := range a.skillMisfires() {
		if a.hasPendingSkillProposal(m.skill) {
			continue
		}
		text, err := skill.Body(m.skill)
		if err != nil {
			continue
		}
		op, before, body, reason, err := drafter.Draft(ctx, m.skill, text, a.skillEvidence(m))
		if err != nil {
			debuglog.Msg("skilltune: drafting %s failed: %v", m.skill, err)
			continue
		}
		if strings.TrimSpace(body) == "" {
			continue
		}
		a.proposeSkillEdit(m, op, before, body, reason)
	}
}

// autoTuneSkills lets tests turn the background drafter off: a unit test that
// rates a skill bad three times would otherwise trip maybeTuneSkills into a real
// model call. Set false in the test harness (newJobApp); true in the app.
var autoTuneSkills = true

// maybeTuneSkills is the trigger, called after every turn from recordJobs. It
// does the cheap deterministic detect on this goroutine and, only if a skill is
// actually flagged, spends a model call to draft — in the background, one at a
// time, so a chat turn is never slowed and two turns never draft at once. The
// proposal it produces waits for human approval like every other; nothing here
// changes a skill on its own.
func (a *App) maybeTuneSkills() {
	if !autoTuneSkills || !learningEnabled() || !skillTuneAuto() {
		return
	}
	db, err := a.database()
	if err != nil {
		return
	}
	names := map[string]bool{}
	for _, d := range skill.ListDiscovered(skill.DefaultDiscoveryPaths()) {
		if d.Name != "" {
			names[d.Name] = true
		}
	}
	if len(detectSkillMisfires(db, names, skillMisfireMinBad)) == 0 {
		return
	}
	if !a.skillTuneRunning.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.skillTuneRunning.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		a.generateSkillRefinements(ctx, appDrafter{a})
	}()
}

// RunSkillTuneup is the manual trigger for the settings page: it drafts now,
// whatever the auto switch says (the user asked for it), and reports how many
// new proposals it queued so the page can say something happened. Synchronous —
// the caller awaits one model call — and still only ever queues proposals.
func (a *App) RunSkillTuneup() (int, error) {
	if !learningEnabled() {
		return 0, fmt.Errorf("learning is switched off in settings")
	}
	before := a.countPending(kindSkill)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	a.generateSkillRefinements(ctx, appDrafter{a})
	return a.countPending(kindSkill) - before, nil
}

// hasPendingSkillProposal is the gate that keeps the model from being called for
// a skill whose edit is already waiting on the human.
func (a *App) hasPendingSkillProposal(skillName string) bool {
	db, err := a.database()
	if err != nil {
		return false
	}
	var id int64
	return db.QueryRow(
		`SELECT id FROM pending_changes WHERE kind = ? AND scope = ? AND state = ? LIMIT 1`,
		kindSkill, skillName, statePending).Scan(&id) == nil
}

// skillEvidence is the misfires a proposal is grounded in — the request that got
// a bad answer, the answer, and how it was scored (👎 or a redo). A person reads
// it to judge whether the edit is warranted; the drafter reads it to know what
// went wrong. Capped: a handful of examples is enough to draft from and to read.
func (a *App) skillEvidence(m skillMisfire) string {
	db, err := a.database()
	if err != nil || len(m.jobIDs) == 0 {
		return ""
	}
	ids := m.jobIDs
	const maxExamples = 5
	if len(ids) > maxExamples {
		ids = ids[:maxExamples]
	}
	var b strings.Builder
	for _, id := range ids {
		var req, ans, src string
		if db.QueryRow(`SELECT request, answer, outcome_source FROM jobs WHERE id = ?`, id).
			Scan(&req, &ans, &src) != nil {
			continue
		}
		if src == "" {
			src = "👎"
		}
		fmt.Fprintf(&b, "- [%s] ถาม: %s\n  ตอบ: %s\n", src, oneLine(req, 220), oneLine(ans, 220))
	}
	return strings.TrimRight(b.String(), "\n")
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// proposeSkillEdit queues one drafted edit as a skill-kind proposal. Mirrors the
// summarizer's proposeSystemIssue: its own INSERT with source "optimizer" — the
// system's self-optimize, kept distinct from "the model decided this mid-task"
// (source "agent") — deduped so the same edit is not queued twice.
func (a *App) proposeSkillEdit(m skillMisfire, op, before, body, reason string) bool {
	db, err := a.database()
	if err != nil {
		return false
	}
	var existing int64
	if db.QueryRow(
		`SELECT id FROM pending_changes WHERE kind = ? AND scope = ? AND op = ? AND before = ? AND body = ? LIMIT 1`,
		kindSkill, m.skill, op, before, body).Scan(&existing) == nil {
		return false
	}
	if strings.TrimSpace(reason) == "" {
		reason = fmt.Sprintf("โดน 👎/ตอบใหม่ %d ครั้ง จาก %d ที่ให้คะแนน", m.bad, m.bad+m.good)
	}
	if _, err := db.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES(?, ?, '', ?, ?, ?, ?, ?, 'optimizer', ?, ?)`,
		kindSkill, m.skill, op, before, body, reason, jobsEvidence(m.jobIDs), statePending,
		time.Now().Format(time.RFC3339)); err != nil {
		debuglog.Msg("skilltune: proposing edit failed: %v", err)
		return false
	}
	a.emitLearningChanged()
	return true
}

// jobsEvidence names the job rows a proposal was drawn from, capped — the same
// shape as summarize.go's evidenceFor, but labelled jobs, not tool_runs, because
// a skill's misfires are scored on the job, not on any one call.
func jobsEvidence(ids []int64) string {
	const maxListed = 20
	listed := ids
	if len(listed) > maxListed {
		listed = listed[:maxListed]
	}
	parts := make([]string, len(listed))
	for i, id := range listed {
		parts[i] = fmt.Sprint(id)
	}
	out := "jobs:" + strings.Join(parts, ",")
	if extra := len(ids) - maxListed; extra > 0 {
		out += fmt.Sprintf(" +%d", extra)
	}
	return out
}

// appDrafter is the real drafter: it asks the user's own current model to
// propose the edit, through the same one-shot Complete path the connection test
// uses. It is the only part of the loop that spends a model call, which is why
// the caller gates it on a skill not already having a proposal waiting.
type appDrafter struct{ app *App }

const skillDraftInstructions = `คุณคือผู้ดูแลสกิลของ Aetox. สกิลด้านล่างถูกผู้ใช้ให้คะแนนแย่ (👎 หรือกด "ตอบใหม่") ซ้ำหลายครั้ง.
อ่านหลักฐานแล้วเรียกเครื่องมือ skill_edit เพื่อเสนอการแก้ SKILL.md เพียง "จุดเดียว" ที่น่าจะลดความพลาด โดยยึดจากหลักฐาน ไม่ใช่การเดา.
แก้ให้น้อยที่สุดที่ได้ผล อย่ารื้อทั้งไฟล์. ถ้าปัญหาไม่ได้อยู่ที่สกิล ให้ body ว่างไว้ (ระบบจะไม่เสนออะไร).`

// skillEditTool is the structured-output channel, learned from how Hermes (and
// Claude Code's synthetic StructuredOutput tool) get a parseable result across
// providers: do not ask the model to "return JSON" — a model that wants to say
// "Looking at the pattern:…" as well will put both in one string. Force a single
// tool call instead, so the prose lands in the text channel and the tool's
// arguments stay pure JSON. One tool + ToolChoice "required" forces it on both
// wire formats (Anthropic maps "required"→any, OpenAI passes it through), so no
// per-provider branch is needed. Application stays deterministic (skill.Apply)
// and gated on human approval — the same split Hermes uses.
var skillEditTool = model.ToolDefinition{
	Type: "function",
	Function: model.ToolFunction{
		Name:        "skill_edit",
		Description: "เสนอการแก้ SKILL.md หนึ่งจุดที่จะลดความพลาดที่โดนให้คะแนนแย่",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"op":{"type":"string","enum":["replace","add"],"description":"replace = แทนข้อความเดิมทั้งท่อน, add = เติมย่อหน้าใหม่ท้ายไฟล์"},
				"before":{"type":"string","description":"ข้อความเดิมทั้งท่อนที่จะแทน ต้องตรงเป๊ะกับใน SKILL.md (เว้นว่างถ้า op=add)"},
				"body":{"type":"string","description":"ข้อความใหม่ (เว้นว่างถ้าปัญหาไม่ได้อยู่ที่สกิล)"},
				"reason":{"type":"string","description":"เหตุผลสั้น ๆ ภาษาไทย"}
			},
			"required":["op","body","reason"]
		}`),
	},
}

func (d appDrafter) Draft(ctx context.Context, skillName, skillText, evidence string) (op, before, body, reason string, err error) {
	p, modelName, err := d.app.oneShotProvider()
	if err != nil {
		return "", "", "", "", err
	}
	req := model.Request{
		Model: modelName,
		Messages: []model.Message{{Role: model.RoleUser, Content: fmt.Sprintf(
			"%s\n\n=== สกิล: %s ===\n%s\n\n=== ผลลัพธ์ที่โดนให้คะแนนแย่ ===\n%s",
			skillDraftInstructions, skillName, skillText, evidence)}},
		Tools:      []model.ToolDefinition{skillEditTool},
		ToolChoice: "required",
		MaxTokens:  1800,
	}
	// One retry: a model that preambled instead of calling the tool the first
	// time almost always calls it cleanly on a second ask (Hermes' parse-then-
	// retry-once recovers ~95% of these).
	op, before, body, reason, err = draftSkillEditOnce(ctx, p, req)
	if err == nil {
		return op, before, body, reason, nil
	}
	return draftSkillEditOnce(ctx, p, req)
}

// draftSkillEditOnce runs one forced-tool call and reads the edit off the tool
// arguments, falling back to a JSON object in the text for a provider that
// answered in prose despite the forcing.
func draftSkillEditOnce(ctx context.Context, p model.Provider, req model.Request) (op, before, body, reason string, err error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return "", "", "", "", err
	}
	var raw string
	if len(resp.ToolCalls) > 0 {
		raw = resp.ToolCalls[0].Function.Arguments
	} else {
		raw = extractJSONObject(resp.Text)
	}
	if raw == "" {
		return "", "", "", "", fmt.Errorf("the drafter returned neither a tool call nor JSON")
	}
	var out struct {
		Op     string `json:"op"`
		Before string `json:"before"`
		Body   string `json:"body"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return "", "", "", "", fmt.Errorf("the drafter's arguments did not parse")
	}
	if out.Op == "" {
		out.Op = "add"
	}
	return out.Op, out.Before, out.Body, out.Reason, nil
}

// oneShotProvider builds a client on the user's current model, the same way
// TestProviderConnection does — endpoint, key and wire format from the live
// config, so the loop drafts with the model the user is actually paying for.
func (a *App) oneShotProvider() (model.Provider, string, error) {
	cfg := a.cur().cfg
	canonical := model.NormalizeProvider(cfg.ModelProvider)
	baseURL := resolveBaseURLForProvider(canonical)
	apiKey := resolveAPIKeyForProvider(canonical)
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		modelName = model.ResolveDefaultModel(canonical, baseURL, apiKey)
	}
	if modelName == "" {
		return nil, "", fmt.Errorf("no model is configured to draft with")
	}
	p, err := model.NewProvider(model.ProviderOptions{
		Provider:   canonical,
		Model:      modelName,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Timeout:    60 * time.Second,
		WireFormat: cfg.ModelWireFormat,
	})
	return p, modelName, err
}

// extractJSONObject pulls the first {...} block out of a model reply, so a model
// that wrapped its JSON in prose or a ```json fence is still read.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
