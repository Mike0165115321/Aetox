package windmill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Workspace is one of the user's workspaces. ID is what travels in a URL; Name
// is what a person recognises. Keeping both is the whole point — the two are
// different strings and using the wrong one 404s in a way that reads like a
// permission problem.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Flow is one flow as the list reports it.
type Flow struct {
	Path        string `json:"path"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	EditedAt    string `json:"edited_at,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
}

// ErrNotConnected is what every tool gets when there is no token or no address.
var ErrNotConnected = errors.New("ยังไม่ได้เชื่อม Windmill — ไปที่ ตั้งค่า → การเชื่อมต่อ แล้วใส่ที่อยู่เซิร์ฟเวอร์กับ token")

// pathPattern is Windmill's own rule for where a flow may live, enforced on the
// server and checked here first.
//
// Checked on this side because the server's refusal is opaque: a 400 with no
// field named is nothing a model can repair itself from, and the mistakes are
// predictable — a dot, a space, or one segment too few. `[ufg]` is the owner
// kind (user, folder, group) and at least two segments must follow it.
var pathPattern = regexp.MustCompile(`^[ufg](/[\p{L}\p{Nd}_-]+){2,}$`)

// CheckPath reports why a flow path would be refused, or nil.
func CheckPath(path string) error {
	path = strings.TrimSpace(path)
	switch {
	case path == "":
		return errors.New("ต้องระบุ path ของ flow")
	case len(path) > 255:
		return errors.New("path ยาวเกิน 255 ตัวอักษร")
	case !pathPattern.MatchString(path):
		return fmt.Errorf(
			"path %q ผิดรูปแบบ — ต้องขึ้นต้นด้วย u/ (ของตัวเอง), f/ (โฟลเดอร์) หรือ g/ (กลุ่ม) "+
				"แล้วตามด้วยอย่างน้อยสองส่วน เช่น u/mike/ดึงใบเสร็จ · ห้ามมีจุดหรือเว้นวรรค", path)
	}
	return nil
}

// Workspaces lists the workspaces this token can see.
//
// The first call any flow tool needs, because every other path is scoped to one
// and the segment is the workspace's id rather than its name.
func Workspaces(ctx context.Context) ([]Workspace, error) {
	var out []Workspace
	if err := call(ctx, http.MethodGet, "/workspaces/list", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFlows reports the flows in one workspace.
func ListFlows(ctx context.Context, workspace string) ([]Flow, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, errors.New("ต้องระบุ workspace")
	}
	var out []Flow
	if err := call(ctx, http.MethodGet, workspacePath(workspace, "/flows/list"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFlow returns one flow whole, as raw JSON — the caller is a model reading a
// graph, and re-encoding it through a Go struct could only lose things.
func GetFlow(ctx context.Context, workspace, path string) (json.RawMessage, error) {
	if err := CheckPath(path); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := call(ctx, http.MethodGet, workspacePath(workspace, "/flows/get/"+path), nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// CreateFlow makes a new flow and returns the path it was stored at.
func CreateFlow(ctx context.Context, workspace string, flow map[string]any) (string, error) {
	body, err := writeBody(flow)
	if err != nil {
		return "", err
	}
	// The response is the path as text/plain, not JSON. Decoding it would fail
	// on a call that actually succeeded.
	return callText(ctx, http.MethodPost, workspacePath(workspace, "/flows/create"), body)
}

// UpdateFlow replaces a flow.
//
// POST, not PUT — Windmill spells this one differently from every other update
// in the API. And the body carries `path`, `summary` and `value` every time:
// older versions take the same required shape as create, so sending all three
// is what makes one client work against both.
func UpdateFlow(ctx context.Context, workspace, path string, flow map[string]any) (string, error) {
	if err := CheckPath(path); err != nil {
		return "", err
	}
	if flow == nil {
		flow = map[string]any{}
	}
	// The path in the URL says which flow; the path in the body says where it
	// should live afterwards. They are the same unless the caller is moving it,
	// and a body missing it is refused by the older shape.
	if _, ok := flow["path"]; !ok {
		flow["path"] = path
	}
	body, err := writeBody(flow)
	if err != nil {
		return "", err
	}
	return callText(ctx, http.MethodPost, workspacePath(workspace, "/flows/update/"+path), body)
}

// writeBody builds a legal flow body: path, summary and value, with the two
// that Windmill requires supplied when the caller left them out.
func writeBody(flow map[string]any) ([]byte, error) {
	if flow == nil {
		return nil, errors.New("ไม่มีข้อมูล flow ให้ส่ง")
	}
	path, _ := flow["path"].(string)
	if err := CheckPath(path); err != nil {
		return nil, err
	}
	out := map[string]any{"path": strings.TrimSpace(path)}
	// Required by the shape older versions still use, and harmless on newer
	// ones. An empty summary is a real value; a missing one is a 400.
	out["summary"], _ = flow["summary"].(string)
	if v, ok := flow["value"]; ok && v != nil {
		out["value"] = v
	} else {
		// An empty flow is a legal one — it is what "make it, I will fill it in"
		// looks like.
		out["value"] = map[string]any{"modules": []any{}}
	}
	for _, optional := range []string{"description", "schema"} {
		if v, ok := flow[optional]; ok && v != nil {
			out[optional] = v
		}
	}
	return json.Marshal(out)
}

// workspacePath joins the workspace segment and the rest.
//
// Built as a string on purpose. A flow path is multi-segment and the route that
// receives it is a greedy wildcard, so the slashes inside it must arrive as
// slashes — running this through url.URL would percent-encode them and every
// request would 404.
func workspacePath(workspace, rest string) string {
	return "/w/" + strings.TrimSpace(workspace) + rest
}

var httpClientRW = httpClient

// call sends a request and decodes a JSON response.
func call(ctx context.Context, method, path string, body []byte, into any) error {
	resp, err := send(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("อ่านคำตอบจาก Windmill ไม่สำเร็จ: %w", err)
	}
	if into == nil {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("คำตอบจาก Windmill ไม่ใช่ JSON ที่อ่านได้: %w", err)
	}
	return nil
}

// callText sends a request whose successful response is plain text — which is
// what create and update return, and the reason they cannot share `call`.
func callText(ctx context.Context, method, path string, body []byte) (string, error) {
	resp, err := send(ctx, method, path, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", statusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("อ่านคำตอบจาก Windmill ไม่สำเร็จ: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func send(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	base := APIBase()
	token := Token()
	if base == "" || token == "" {
		return nil, ErrNotConnected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClientRW.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ติดต่อ Windmill ไม่ได้: %w", err)
	}
	return resp, nil
}

// statusError turns a refusal into something the reader can act on, keeping
// Windmill's own words where they are more specific than ours.
func statusError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	said := strings.TrimSpace(string(raw))
	// Windmill answers errors as text as often as JSON, so the body is taken as
	// written and only unwrapped when it turns out to be an object.
	var wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &wrapped) == nil {
		if m := strings.TrimSpace(wrapped.Error.Message); m != "" {
			said = m
		} else if m := strings.TrimSpace(wrapped.Message); m != "" {
			said = m
		}
	}
	if len(said) > 400 {
		said = said[:400] + "…"
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errors.New("Windmill ปฏิเสธ token — ผิดหรือถูกเพิกถอน")
	case http.StatusForbidden:
		// The read_only box and folder permissions land here, and Windmill's own
		// sentence is the only thing that says which.
		if said != "" {
			return fmt.Errorf("Windmill ไม่อนุญาต: %s (ถ้าตอนสร้าง token ติ๊ก read_only ไว้ ต้องสร้างใหม่)", said)
		}
		return errors.New("token นี้สิทธิ์ไม่พอ — ถ้าตอนสร้างติ๊ก read_only ไว้ ต้องสร้างใหม่")
	case http.StatusNotFound:
		return errors.New("ไม่พบสิ่งที่ขอบน Windmill — path หรือ workspace ผิด (workspace ต้องใช้ id ไม่ใช่ชื่อที่แสดง)")
	case http.StatusBadRequest:
		if said != "" {
			return fmt.Errorf("Windmill ไม่รับข้อมูลนี้: %s", said)
		}
		return errors.New("Windmill ไม่รับข้อมูลนี้ (400)")
	}
	if said != "" {
		return fmt.Errorf("Windmill ตอบกลับ %d: %s", resp.StatusCode, said)
	}
	return fmt.Errorf("Windmill ตอบกลับสถานะ %d", resp.StatusCode)
}
