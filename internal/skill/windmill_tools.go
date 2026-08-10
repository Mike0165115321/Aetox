package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Mike0165115321/Aetox/internal/automation/windmill"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// The Windmill tools. Shaped like the n8n ones next door, and different in the
// two places Windmill genuinely differs — both of which are ways for a model to
// waste a turn if the description does not say so:
//
//   - Everything is scoped to a WORKSPACE, and the segment is the workspace's
//     id, not its display name. Hence a tool whose only job is to hand the model
//     the ids.
//   - A flow's path is its identity and its address, and the server enforces a
//     shape. A refusal there is a bare 400, so the shape is stated in the
//     description and checked before the request leaves.
const windmillPathHint = "A flow's path is also where it lives: `u/<you>/name` is your own space, `f/<folder>/name` a shared folder. At least two segments after the letter, no dots or spaces."

type windmillWorkspacesSkill struct{}
type windmillListSkill struct{}
type windmillReadSkill struct{}
type windmillCreateSkill struct{}
type windmillUpdateSkill struct{}

// ---------- workspaces ----------

func (*windmillWorkspacesSkill) Name() string { return "windmill_workspace_list" }

func (*windmillWorkspacesSkill) Description() string {
	return "ดูรายการ workspace บน Windmill ของผู้ใช้"
}

func (s *windmillWorkspacesSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("windmill_workspace_list",
		"List the user's Windmill workspaces. Every other Windmill tool needs one, and it needs the id — the display name is a different string and using it fails as if the flow did not exist.",
		map[string]any{}, nil)
}

func (s *windmillWorkspacesSkill) Execute(ctx context.Context, input Input) (Output, error) {
	return s.ExecuteTool(ctx, map[string]any{})
}

func (s *windmillWorkspacesSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	spaces, err := windmill.Workspaces(ctx)
	if err != nil {
		return windmillFail("windmill_workspace_list", start, err)
	}
	if len(spaces) == 0 {
		return newToolOutput("windmill_workspace_list", "windmill_workspace_list",
			"token นี้ยังไม่เห็น workspace ไหนเลย", start, false, nil), nil
	}
	var b strings.Builder
	for _, w := range spaces {
		fmt.Fprintf(&b, "%s  (%s)\n", w.ID, w.Name)
	}
	return newToolOutput("windmill_workspace_list", "windmill_workspace_list",
		strings.TrimRight(b.String(), "\n"), start, false, nil), nil
}

// ---------- list ----------

func (*windmillListSkill) Name() string { return "windmill_flow_list" }

func (*windmillListSkill) Description() string {
	return "ดูรายการ flow ใน workspace หนึ่งบน Windmill"
}

func (s *windmillListSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("windmill_flow_list",
		"List the flows in one Windmill workspace.",
		map[string]any{
			"workspace": map[string]any{"type": "string", "description": "Workspace id from windmill_workspace_list."},
		}, []string{"workspace"})
}

func (s *windmillListSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		start := time.Now()
		err := errors.New("usage: windmill_flow_list <workspace-id>")
		return newToolOutput("windmill_flow_list", "windmill_flow_list", "", start, false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"workspace": args[0]})
}

func (s *windmillListSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	ws, _ := args["workspace"].(string)
	flows, err := windmill.ListFlows(ctx, ws)
	if err != nil {
		return windmillFail("windmill_flow_list", start, err)
	}
	if len(flows) == 0 {
		return newToolOutput("windmill_flow_list", "windmill_flow_list",
			"ยังไม่มี flow ใน workspace นี้", start, false, nil), nil
	}
	var b strings.Builder
	for _, f := range flows {
		fmt.Fprintf(&b, "%s", f.Path)
		if f.Summary != "" {
			fmt.Fprintf(&b, "  — %s", f.Summary)
		}
		if f.Archived {
			b.WriteString("  (เก็บเข้าคลังแล้ว)")
		}
		b.WriteString("\n")
	}
	return newToolOutput("windmill_flow_list", "windmill_flow_list "+ws,
		strings.TrimRight(b.String(), "\n"), start, false, nil), nil
}

// ---------- read ----------

func (*windmillReadSkill) Name() string { return "windmill_flow_read" }

func (*windmillReadSkill) Description() string {
	return "อ่าน flow หนึ่งตัวบน Windmill แบบเต็ม"
}

func (s *windmillReadSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("windmill_flow_read",
		"Read one Windmill flow whole — every module and its input_transforms — as JSON. Also the accurate way to learn a step's real shape before writing one.",
		map[string]any{
			"workspace": map[string]any{"type": "string", "description": "Workspace id."},
			"path":      map[string]any{"type": "string", "description": "Flow path, e.g. u/mike/daily."},
		}, []string{"workspace", "path"})
}

func (s *windmillReadSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) < 2 {
		start := time.Now()
		err := errors.New("usage: windmill_flow_read <workspace-id> <path>")
		return newToolOutput("windmill_flow_read", "windmill_flow_read", "", start, false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"workspace": args[0], "path": args[1]})
}

func (s *windmillReadSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	ws, _ := args["workspace"].(string)
	path, _ := args["path"].(string)
	raw, err := windmill.GetFlow(ctx, ws, path)
	if err != nil {
		return windmillFail("windmill_flow_read", start, err)
	}
	var pretty json.RawMessage
	if json.Unmarshal(raw, &pretty) == nil {
		if formatted, ferr := json.MarshalIndent(pretty, "", "  "); ferr == nil {
			raw = formatted
		}
	}
	return newToolOutput("windmill_flow_read", "windmill_flow_read "+path, string(raw), start, false, nil), nil
}

// ---------- create ----------

func (*windmillCreateSkill) Name() string { return "windmill_flow_create" }

func (*windmillCreateSkill) Description() string {
	return "สร้าง flow ใหม่บน Windmill"
}

func (s *windmillCreateSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("windmill_flow_create",
		"Create a Windmill flow. `value.modules` is the list of steps; each step needs a stable `id`, because that is how later steps refer to its results. "+windmillPathHint,
		map[string]any{
			"workspace": map[string]any{"type": "string", "description": "Workspace id."},
			"path":      map[string]any{"type": "string", "description": "Where it lives, e.g. u/mike/daily."},
			"summary":   map[string]any{"type": "string", "description": "One line the user will see in the list."},
			"value":     map[string]any{"type": "object", "description": "The flow itself: {\"modules\": [...]}. {\"modules\":[]} makes an empty one."},
			"schema":    map[string]any{"type": "object", "description": "JSON Schema of the flow's own inputs. Without it the run form is empty."},
		}, []string{"workspace", "path"})
}

func (s *windmillCreateSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	err := errors.New("windmill_flow_create ต้องเรียกแบบ tool call พร้อมโครงสร้าง flow")
	return newToolOutput("windmill_flow_create", "windmill_flow_create", "", start, false, err), err
}

func (s *windmillCreateSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	ws, _ := args["workspace"].(string)
	path, err := windmill.CreateFlow(ctx, ws, args)
	if err != nil {
		return windmillFail("windmill_flow_create", start, err)
	}
	return newToolOutput("windmill_flow_create", "windmill_flow_create",
		"สร้างแล้วที่ "+path, start, false, nil), nil
}

// ---------- update ----------

func (*windmillUpdateSkill) Name() string { return "windmill_flow_update" }

func (*windmillUpdateSkill) Description() string {
	return "แก้ flow ที่มีอยู่บน Windmill (แทนที่ทั้งตัว)"
}

func (s *windmillUpdateSkill) ToolDefinition() model.ToolDefinition {
	return toolDef("windmill_flow_update",
		"Replace a Windmill flow. FULL REPLACE, not a patch: read it first, change what you mean to, send it all back. Deploys immediately — there is no staging step, so a colleague with an unsaved draft at that path can be surprised. "+windmillPathHint,
		map[string]any{
			"workspace": map[string]any{"type": "string", "description": "Workspace id."},
			"path":      map[string]any{"type": "string", "description": "The flow to replace."},
			"summary":   map[string]any{"type": "string"},
			"value":     map[string]any{"type": "object", "description": "EVERY module it should have afterwards."},
			"schema":    map[string]any{"type": "object"},
		}, []string{"workspace", "path"})
}

func (s *windmillUpdateSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	err := errors.New("windmill_flow_update ต้องเรียกแบบ tool call พร้อมโครงสร้าง flow")
	return newToolOutput("windmill_flow_update", "windmill_flow_update", "", start, false, err), err
}

func (s *windmillUpdateSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	ws, _ := args["workspace"].(string)
	path, _ := args["path"].(string)
	saved, err := windmill.UpdateFlow(ctx, ws, path, args)
	if err != nil {
		return windmillFail("windmill_flow_update", start, err)
	}
	return newToolOutput("windmill_flow_update", "windmill_flow_update "+path,
		"แก้แล้วที่ "+saved, start, false, nil), nil
}

// windmillFail is the one failure shape, keeping Windmill's own sentence where
// it had one — a 400 that names the offending field is what lets the model
// repair its own flow rather than guess.
func windmillFail(name string, start time.Time, err error) (Output, error) {
	return newToolOutput(name, name, "", start, false, err), err
}

// ---------- the packed tool ----------

// windmillSkill is this engine's one name in the block — same day, same reason
// and same shape as n8n's above (packed.go, DECISIONS §99).
type windmillSkill struct {
	actions []string
}

func (*windmillSkill) Name() string { return "windmill" }

func (*windmillSkill) Description() string {
	return "จัดการ flow บน Windmill ของผู้ใช้ — ดู workspace ดูรายการ อ่าน สร้าง และแก้"
}

func (s *windmillSkill) allowedActions() []string {
	p := packs["windmill"]
	if s == nil || len(s.actions) == 0 {
		return append([]string(nil), p.actions...)
	}
	return s.actions
}

func (s *windmillSkill) Actions() []string { return packs["windmill"].permissions() }

func (s *windmillSkill) Narrow(named []string) Skill {
	narrowed := *s
	narrowed.actions = packs["windmill"].narrow(named)
	return &narrowed
}

func (s *windmillSkill) ToolDefinition() model.ToolDefinition {
	allowed := s.allowedActions()

	lines := map[string]string{
		"workspaces": "`workspaces` — the user's workspaces and their IDs. Every other action is scoped to a workspace, and it takes the id — the display name is a different string and using it fails as if the flow did not exist.",
		"list":       "`list` (workspace) — the flows in one workspace.",
		"read":       "`read` (workspace, path) — one flow whole, every module and its input_transforms, as JSON. Also the accurate way to learn a step's real shape before writing one.",
		"create":     "`create` (workspace, path, summary?, value?, schema?) — a new flow. value.modules is the list of steps; each step needs a stable id, because that is how later steps refer to its results.",
		"update":     "`update` (workspace, path, summary?, value?, schema?) — FULL REPLACE, not a patch: read it first, change what you mean to, send it all back. Deploys immediately — there is no staging step.",
	}
	var actions strings.Builder
	for _, a := range allowed {
		actions.WriteString(lines[a] + "\n")
	}

	return toolDef("windmill",
		"Work the flows on the user's Windmill. Actions:\n"+actions.String()+"\n"+windmillPathHint,
		map[string]any{
			"action":    map[string]any{"type": "string", "enum": allowed, "description": "What to do"},
			"workspace": map[string]any{"type": "string", "description": "Workspace id from `workspaces` — every action but workspaces needs it"},
			"path":      map[string]any{"type": "string", "description": "action=read/create/update: the flow's path, e.g. u/mike/daily"},
			"summary":   map[string]any{"type": "string", "description": "action=create/update: one line the user will see in the list"},
			"value":     map[string]any{"type": "object", "description": "action=create/update: the flow itself: {\"modules\": [...]}. On update: EVERY module it should have afterwards."},
			"schema":    map[string]any{"type": "object", "description": "action=create/update: JSON Schema of the flow's own inputs. Without it the run form is empty."},
		}, []string{"action"})
}

func (s *windmillSkill) Execute(ctx context.Context, input Input) (Output, error) {
	start := time.Now()
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := fmt.Errorf("usage: windmill <%s> ...", strings.Join(s.allowedActions(), "|"))
		return newToolOutput("windmill", "windmill", "", start, false, err), err
	}
	inner, err := s.innerFor(strings.ToLower(strings.TrimSpace(args[0])))
	if err != nil {
		return newToolOutput("windmill", "windmill", "", start, false, err), err
	}
	return inner.Execute(ctx, Input{"args": args[1:]})
}

func (s *windmillSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	raw, _ := args["action"].(string)
	inner, err := s.innerFor(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return newToolOutput("windmill", "windmill", "", start, false, err), err
	}
	return inner.ExecuteTool(ctx, args)
}

func (s *windmillSkill) innerFor(action string) (Tool, error) {
	p := packs["windmill"]
	if _, known := p.names[action]; !known {
		return nil, fmt.Errorf("unknown windmill action %q — this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	if !slices.Contains(s.allowedActions(), action) {
		return nil, fmt.Errorf("windmill %s is not available here — this session may use: %s",
			action, strings.Join(s.allowedActions(), ", "))
	}
	switch action {
	case "workspaces":
		return &windmillWorkspacesSkill{}, nil
	case "list":
		return &windmillListSkill{}, nil
	case "read":
		return &windmillReadSkill{}, nil
	case "create":
		return &windmillCreateSkill{}, nil
	case "update":
		return &windmillUpdateSkill{}, nil
	}
	return nil, fmt.Errorf("windmill action %q has no implementation", action)
}
