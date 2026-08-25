package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Mikedev115/Aetox/internal/statereport"
)

// Workflow is one workflow as the list and read calls report it. Deliberately
// shallow: the graph itself is handed to the model as raw JSON, because nothing
// in Go needs to understand an n8n node and a Go struct that half-understood one
// would silently drop whatever it had not been taught.
type Workflow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Nodes     int    `json:"nodes"`
}

// writableFields is the allowlist every write body is built from.
//
// This is the single most important line in the package. n8n validates request
// bodies against its OpenAPI schema with `additionalProperties: false`, and it
// marks id, active, createdAt, updatedAt, isArchived, versionId, triggerCount,
// tags and meta as read-only — sending any of them is a 400, not a shrug. Which
// means the obvious implementation is the broken one: you cannot GET a workflow,
// change a node and PUT it back.
//
// So writes are assembled by copying only these keys out of whatever the model
// produced. An allowlist rather than a denylist because the read-only set grows
// with n8n's versions, and a denylist written today would start failing on an
// instance upgraded next month.
var writableFields = []string{
	"name", "nodes", "connections", "settings",
	// Optional, accepted, and worth passing through when the model sets them.
	"staticData", "pinData",
}

// ErrNotConnected is what every tool gets when there is no key or no address,
// so the sentence the user reads is written once. A state report: it describes
// this machine's configuration, not anything the caller should do differently.
var ErrNotConnected = statereport.New("ยังไม่ได้เชื่อม n8n — ไปที่ ตั้งค่า → การเชื่อมต่อ แล้วใส่ที่อยู่เซิร์ฟเวอร์กับ API key")

// List reports the workflows on the instance, newest page first.
//
// Pagination is cursor-based with no total count, so this takes one page and
// says how to ask for the next rather than looping: a model that wants
// everything can ask again, and an instance with 4,000 workflows must not be
// pulled into one tool result.
func List(ctx context.Context, limit int, cursor string) ([]Workflow, string, error) {
	if limit <= 0 || limit > 250 {
		limit = 50
	}
	query := url.Values{}
	query.Set("limit", fmt.Sprint(limit))
	// Pinned data is sample payloads a user pinned while building; it can be
	// megabytes and none of it belongs in a list.
	query.Set("excludePinnedData", "true")
	if strings.TrimSpace(cursor) != "" {
		query.Set("cursor", cursor)
	}

	var payload struct {
		Data []struct {
			ID        string           `json:"id"`
			Name      string           `json:"name"`
			Active    bool             `json:"active"`
			UpdatedAt string           `json:"updatedAt"`
			Nodes     []json.RawMessage `json:"nodes"`
		} `json:"data"`
		NextCursor string `json:"nextCursor"`
	}
	if err := call(ctx, http.MethodGet, "/workflows?"+query.Encode(), nil, &payload); err != nil {
		return nil, "", err
	}
	out := make([]Workflow, 0, len(payload.Data))
	for _, w := range payload.Data {
		out = append(out, Workflow{
			ID: w.ID, Name: w.Name, Active: w.Active,
			UpdatedAt: w.UpdatedAt, Nodes: len(w.Nodes),
		})
	}
	return out, payload.NextCursor, nil
}

// Get returns one workflow whole, as raw JSON.
//
// Raw because the caller is a language model reading a graph, and re-encoding
// it through a Go struct could only lose things — a node type this build has
// never heard of is exactly what a user's own instance is full of.
func Get(ctx context.Context, id string) (json.RawMessage, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("ต้องระบุ id ของ workflow")
	}
	var raw json.RawMessage
	if err := call(ctx, http.MethodGet, "/workflows/"+url.PathEscape(id), nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// Create makes a new workflow from a graph the model wrote.
//
// It cannot be created already running: `active` is read-only on create, so
// switching a workflow on is always a second call. That is stated here rather
// than hidden behind a convenience flag, because "I made it and it is live" and
// "I made it and it is sitting there" are different enough to be worth the
// caller saying which it meant.
func Create(ctx context.Context, graph map[string]any) (Workflow, error) {
	body, err := writeBody(graph)
	if err != nil {
		return Workflow{}, err
	}
	var created struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := call(ctx, http.MethodPost, "/workflows", body, &created); err != nil {
		return Workflow{}, err
	}
	return Workflow{ID: created.ID, Name: created.Name, Active: created.Active}, nil
}

// Update replaces a workflow.
//
// There is no PATCH. Leaving `nodes` out does not mean "leave the nodes alone",
// it means the request is rejected; sending three of five nodes means the other
// two are gone. Callers are expected to Get, change, and send the whole thing —
// and writeBody is what makes that round trip legal at all.
func Update(ctx context.Context, id string, graph map[string]any) (Workflow, error) {
	if strings.TrimSpace(id) == "" {
		return Workflow{}, errors.New("ต้องระบุ id ของ workflow")
	}
	body, err := writeBody(graph)
	if err != nil {
		return Workflow{}, err
	}
	var updated struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := call(ctx, http.MethodPut, "/workflows/"+url.PathEscape(id), body, &updated); err != nil {
		return Workflow{}, err
	}
	return Workflow{ID: updated.ID, Name: updated.Name, Active: updated.Active}, nil
}

// SetActive switches a workflow on or off.
//
// Failure here is usually about the workflow rather than the request — one with
// no trigger node cannot be activated at all — so the server's own sentence is
// passed through rather than replaced with a guess.
func SetActive(ctx context.Context, id string, active bool) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("ต้องระบุ id ของ workflow")
	}
	verb := "deactivate"
	if active {
		verb = "activate"
	}
	return call(ctx, http.MethodPost, "/workflows/"+url.PathEscape(id)+"/"+verb, []byte("{}"), nil)
}

// writeBody builds a legal request body out of whatever the model produced.
//
// Three rules, each of which n8n enforces with a 400 rather than a shrug:
// only allowlisted keys survive; `settings` is required even when empty; and
// `nodes` and `connections` must be present on every write, because a write is
// always a whole replacement.
func writeBody(graph map[string]any) ([]byte, error) {
	if graph == nil {
		return nil, errors.New("ไม่มีข้อมูล workflow ให้ส่ง")
	}
	out := map[string]any{}
	for _, key := range writableFields {
		if v, ok := graph[key]; ok && v != nil {
			out[key] = v
		}
	}
	if name, _ := out["name"].(string); strings.TrimSpace(name) == "" {
		return nil, errors.New("workflow ต้องมีชื่อ (name)")
	}
	// Defaults for the three the schema requires. An empty workflow is a legal
	// one — it is what "create it, then I will fill it in" looks like — so the
	// missing pieces are supplied rather than refused.
	if _, ok := out["nodes"]; !ok {
		out["nodes"] = []any{}
	}
	if _, ok := out["connections"]; !ok {
		out["connections"] = map[string]any{}
	}
	if _, ok := out["settings"]; !ok {
		out["settings"] = map[string]any{}
	}
	return json.Marshal(out)
}

// call is the one place a request to n8n is built, sent and read.
//
// Every tool goes through it so that the auth header, the not-connected check,
// and the translation of a status code into a sentence a person can act on
// exist once. A 400 in particular is passed through with the server's own
// message: n8n says exactly which field it objected to, and that sentence is
// the difference between the model fixing its graph and the model guessing.
func call(ctx context.Context, method, path string, body []byte, into any) error {
	base := APIBase()
	key := Token()
	if base == "" || key == "" {
		return ErrNotConnected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set(authHeader, key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return statereport.Newf("ติดต่อ n8n ไม่ได้: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp)
	}
	return decodeJSON(resp, into)
}

// statusError turns a refusal into something the reader can act on, keeping the
// server's own words wherever they are more specific than ours.
//
// Statuses split along the statereport line: 401/403 describe a credential's
// state and the residual (mostly 5xx) a server's — true or false regardless of
// what the caller sends next. 404/400/409 answer the request itself — a wrong
// id, a malformed graph, a name collision — which the model can and should do
// something about, so those stay readable as lessons.
func statusError(resp *http.Response) error {
	var detail struct {
		Message string `json:"message"`
	}
	_ = decodeJSON(resp, &detail)
	said := strings.TrimSpace(detail.Message)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return statereport.New("n8n ปฏิเสธ API key — อาจหมดอายุแล้ว ไปสร้างใหม่ที่ Settings → n8n API")
	case http.StatusForbidden:
		return statereport.New("คีย์นี้สิทธิ์ไม่พอสำหรับคำสั่งนี้")
	case http.StatusNotFound:
		return errors.New("ไม่พบสิ่งที่ขอบน n8n — id ผิด หรือถูกลบไปแล้ว")
	case http.StatusBadRequest:
		// The useful one. n8n names the offending field, and that sentence is
		// what lets the model repair its own graph instead of guessing.
		if said != "" {
			return fmt.Errorf("n8n ไม่รับข้อมูลนี้: %s", said)
		}
		return errors.New("n8n ไม่รับข้อมูลนี้ (400)")
	case http.StatusConflict:
		if said != "" {
			return fmt.Errorf("n8n ปฏิเสธเพราะชนกับของเดิม: %s", said)
		}
		return errors.New("n8n ปฏิเสธเพราะชนกับของเดิม (409)")
	}
	if said != "" {
		return statereport.Newf("n8n ตอบกลับ %d: %s", resp.StatusCode, said)
	}
	return statereport.Newf("n8n ตอบกลับสถานะ %d", resp.StatusCode)
}
