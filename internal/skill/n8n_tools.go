package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/model"
	"github.com/Mike0165115321/Aetox/internal/automation/n8n"
)

// The n8n tools: read what is on the user's instance, and write to it.
//
// One thing about n8n shapes every description in this file, and it is worth
// stating once here rather than discovering it five tool calls in. n8n's public
// API has no endpoint that lists node types or their parameter schemas — it is
// not hidden behind a flag, it does not exist. The API validates the *envelope*
// of a workflow strictly (unknown top-level fields are a 400) and the *contents*
// of a node not at all: `parameters` is an opaque object, so a node type that
// does not exist on that instance, or a real node with invented parameters, is
// accepted at write time and only fails when the workflow runs.
//
// That is a trap for a model, and the honest response is to say so in the tool
// descriptions rather than to pretend the write succeeded meaningfully. The
// create and update descriptions therefore tell the model to read an existing
// workflow first when it is unsure what a node looks like — the user's own
// instance is the only accurate schema reference available, and it is right
// there.

// n8nUsageHint goes on the write tools because the model reads the description
// of the tool it is about to call, not the file it lives in.
//
// Kept to one sentence on purpose. Every tool offered to the model is paid for
// in the tool block of every request (desktop/tool_budget_test.go), so a
// description earns its length by preventing a wasted turn — and this one does:
// without it the model invents node parameters, the write succeeds, and the
// failure surfaces much later as a broken run nobody connects back to here.
const n8nUsageHint = " n8n does not expose node schemas, so wrong node parameters are accepted here and fail at run time — read an existing workflow that uses a node before writing one."

type n8nListSkill struct{}
type n8nReadSkill struct{}
type n8nCreateSkill struct{}
type n8nUpdateSkill struct{}
type n8nActivateSkill struct{}

// ---------- list ----------

func (*n8nListSkill) Name() string { return "n8n_workflow_list" }

func (*n8nListSkill) Description() string {
	return "ดูรายการ workflow ที่มีอยู่บน n8n ของผู้ใช้"
}

func (s *n8nListSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("n8n_workflow_list",
		"List workflows on the user's n8n: id, name, active or not, node count. Paged — pass a returned next_cursor to continue.",
		map[string]any{
			"limit":  map[string]any{"type": "integer", "description": "1-250, default 50."},
			"cursor": map[string]any{"type": "string", "description": "next_cursor from a previous call."},
		}, nil)
}

func (s *n8nListSkill) Execute(ctx context.Context, input Input) (Output, error) {
	return s.ExecuteTool(ctx, map[string]any{})
}

func (s *n8nListSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	limit := 0
	if n, ok := args["limit"].(float64); ok {
		limit = int(n)
	}
	cursor, _ := args["cursor"].(string)

	items, next, err := n8n.List(ctx, limit, cursor)
	if err != nil {
		return n8nFail("n8n_workflow_list", start, err)
	}
	if len(items) == 0 {
		return newToolOutput("n8n_workflow_list", "n8n_workflow_list",
			"ยังไม่มี workflow บน n8n นี้", start, false, nil), nil
	}
	var b strings.Builder
	for _, w := range items {
		state := "หยุดอยู่"
		if w.Active {
			state = "ทำงานอยู่"
		}
		fmt.Fprintf(&b, "%s  %s  (%s · %d โหนด)\n", w.ID, w.Name, state, w.Nodes)
	}
	if next != "" {
		fmt.Fprintf(&b, "\nnext_cursor: %s", next)
	}
	return newToolOutput("n8n_workflow_list", "n8n_workflow_list",
		strings.TrimRight(b.String(), "\n"), start, false, nil), nil
}

// ---------- read ----------

func (*n8nReadSkill) Name() string { return "n8n_workflow_read" }

func (*n8nReadSkill) Description() string {
	return "อ่าน workflow หนึ่งตัวบน n8n แบบเต็ม รวมทุกโหนดและการเชื่อมต่อ"
}

func (s *n8nReadSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("n8n_workflow_read",
		"Read one workflow whole — every node with its parameters and the wiring — as JSON. Also the only accurate way to learn a node type's real shape before writing one.",
		map[string]any{
			"id": map[string]any{"type": "string", "description": "Workflow id from n8n_workflow_list."},
		}, []string{"id"})
}

func (s *n8nReadSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		start := time.Now()
		err := errors.New("usage: n8n_workflow_read <workflow-id>")
		return newToolOutput("n8n_workflow_read", "n8n_workflow_read", "", start, false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"id": args[0]})
}

func (s *n8nReadSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	id, _ := args["id"].(string)
	raw, err := n8n.Get(ctx, id)
	if err != nil {
		return n8nFail("n8n_workflow_read", start, err)
	}
	// Re-indented rather than passed through flat: the model is about to copy
	// pieces of this, and n8n's own response is one very long line.
	var pretty json.RawMessage
	if json.Unmarshal(raw, &pretty) == nil {
		if formatted, ferr := json.MarshalIndent(pretty, "", "  "); ferr == nil {
			raw = formatted
		}
	}
	return newToolOutput("n8n_workflow_read", "n8n_workflow_read "+id, string(raw), start, false, nil), nil
}

// ---------- create ----------

func (*n8nCreateSkill) Name() string { return "n8n_workflow_create" }

func (*n8nCreateSkill) Description() string {
	return "สร้าง workflow ใหม่บน n8n ของผู้ใช้"
}

func (s *n8nCreateSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("n8n_workflow_create",
		"Create a workflow on the user's n8n. Created switched off — use n8n_workflow_activate to start it."+n8nUsageHint,
		map[string]any{
			"name":  map[string]any{"type": "string"},
			"nodes": map[string]any{"type": "array", "description": "Node objects; [] for a blank workflow.", "items": map[string]any{"type": "object"}},
			// The doubled array is the single most common way an AI-written
			// workflow validates and then renders as disconnected nodes, so the
			// shape is spelled out here rather than left to be inferred.
			"connections": map[string]any{"type": "object", "description": "Keyed by node NAME: {\"A\":{\"main\":[[{\"node\":\"B\",\"type\":\"main\",\"index\":0}]]}}. Outer array = output port, inner = fan-out."},
			"settings":    map[string]any{"type": "object", "description": "{} is fine."},
		}, []string{"name"})
}

func (s *n8nCreateSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	err := errors.New("n8n_workflow_create ต้องเรียกแบบ tool call พร้อมโครงสร้าง workflow")
	return newToolOutput("n8n_workflow_create", "n8n_workflow_create", "", start, false, err), err
}

func (s *n8nCreateSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	created, err := n8n.Create(ctx, args)
	if err != nil {
		return n8nFail("n8n_workflow_create", start, err)
	}
	msg := fmt.Sprintf("สร้างแล้ว: %s (id %s) — ยังไม่เปิดใช้งาน เรียก n8n_workflow_activate เพื่อเริ่ม", created.Name, created.ID)
	return newToolOutput("n8n_workflow_create", "n8n_workflow_create", msg, start, false, nil), nil
}

// ---------- update ----------

func (*n8nUpdateSkill) Name() string { return "n8n_workflow_update" }

func (*n8nUpdateSkill) Description() string {
	return "แก้ workflow ที่มีอยู่บน n8n (แทนที่ทั้งตัว)"
}

func (s *n8nUpdateSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("n8n_workflow_update",
		"FULL REPLACE of a workflow, not a patch: read it first, change what you mean to, send it all back. Three of five nodes deletes the other two. Does not change active state."+n8nUsageHint,
		map[string]any{
			"id":          map[string]any{"type": "string"},
			"name":        map[string]any{"type": "string"},
			"nodes":       map[string]any{"type": "array", "description": "EVERY node it should have afterwards.", "items": map[string]any{"type": "object"}},
			"connections": map[string]any{"type": "object", "description": "The complete wiring, keyed by node name."},
			"settings":    map[string]any{"type": "object"},
		}, []string{"id", "name"})
}

func (s *n8nUpdateSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	err := errors.New("n8n_workflow_update ต้องเรียกแบบ tool call พร้อมโครงสร้าง workflow")
	return newToolOutput("n8n_workflow_update", "n8n_workflow_update", "", start, false, err), err
}

func (s *n8nUpdateSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	id, _ := args["id"].(string)
	updated, err := n8n.Update(ctx, id, args)
	if err != nil {
		return n8nFail("n8n_workflow_update", start, err)
	}
	msg := fmt.Sprintf("แก้แล้ว: %s (id %s)", updated.Name, updated.ID)
	return newToolOutput("n8n_workflow_update", "n8n_workflow_update "+id, msg, start, false, nil), nil
}

// ---------- activate ----------

func (*n8nActivateSkill) Name() string { return "n8n_workflow_activate" }

func (*n8nActivateSkill) Description() string {
	return "เปิดหรือปิดการทำงานของ workflow บน n8n"
}

func (s *n8nActivateSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("n8n_workflow_activate",
		"Switch a workflow on or off. Activation means \"run by itself from now on\", so it needs a webhook, schedule or polling trigger. A Manual Trigger is NOT one of those: a workflow built to be tested from the editor cannot be activated, does not need to be, and is finished without this call — do not make it, and if you did, the refusal is not a fault in the workflow.",
		map[string]any{
			"id":     map[string]any{"type": "string"},
			"active": map[string]any{"type": "boolean", "description": "Default true."},
		}, []string{"id"})
}

func (s *n8nActivateSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		start := time.Now()
		err := errors.New("usage: n8n_workflow_activate <workflow-id>")
		return newToolOutput("n8n_workflow_activate", "n8n_workflow_activate", "", start, false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"id": args[0], "active": true})
}

func (s *n8nActivateSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	id, _ := args["id"].(string)
	active := true
	if v, ok := args["active"].(bool); ok {
		active = v
	}
	if err := n8n.SetActive(ctx, id, active); err != nil {
		return n8nFail("n8n_workflow_activate", start, err)
	}
	state := "เปิดใช้งานแล้ว"
	if !active {
		state = "ปิดแล้ว"
	}
	return newToolOutput("n8n_workflow_activate", "n8n_workflow_activate "+id,
		fmt.Sprintf("%s: %s", id, state), start, false, nil), nil
}

// ---------- shared ----------

// toolDef builds the JSON schema every tool in this file declares, so the five
// of them cannot drift on `additionalProperties` — which n8n's own API taught us
// to care about.
func toolDef(name, description string, properties map[string]any, required []string) model.ToolDefinition {
	if required == nil {
		required = []string{}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  payload,
		},
	}
}

// n8nFail is the one failure shape. The error text is n8n's own wherever n8n
// said something specific — a 400 naming the field it rejected is what lets the
// model repair its graph instead of guessing at it.
func n8nFail(name string, start time.Time, err error) (Output, error) {
	return newToolOutput(name, name, "", start, false, err), err
}
