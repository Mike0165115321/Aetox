package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Connections — the non-secret half of an external account.
//
// The token lives in the credential vault (internal/oauth, encrypted at rest).
// What lives here is only which desks and agents the connection serves, which
// is a setting the user should be able to read, diff and back up. The split is
// the rule oauth.StorePath already states from its own side: a config file gets
// copied into bug reports and synced between machines, so a credential must
// never be in one.
//
// The `for:` vocabulary is deliberately identical to an MCP server's — a desk
// name, or "agent:<name>" — because it answers the identical question: this is
// something bolted onto the outside of the app, and who gets to use it. Two
// spellings of one question is the debt that shows up later as two settings
// pages that disagree.

// ConnectionConfig is one external account's placement.
type ConnectionConfig struct {
	// ID is the provider id — "github". It is not a display name and never
	// carries a token.
	ID string `json:"id"`
	// For lists the desks and agents that may use this connection's tools,
	// exactly as MCPServerConfig.For does. Absent means "never set" and reads
	// as every desk; an explicit empty list means the user switched it off
	// everywhere, and must survive as such.
	For []string `json:"for"`
	// BaseURL is where this service lives, for the services that do not live
	// anywhere in particular. GitHub is one host for everyone and states it as a
	// constant; n8n and Windmill are things the user runs, on a machine only the
	// user knows the address of, so the address is theirs to give.
	//
	// It sits here rather than in the vault because it is not a secret — it is a
	// setting, and a setting the user should be able to read back, diff and
	// correct after a typo without disconnecting anything.
	BaseURL string `json:"baseUrl,omitempty"`
	// StartCommand is how to bring this service up, for the ones the user runs
	// themselves. Empty means Aetox has not been told, and the button that would
	// press it is not drawn.
	//
	// The command is the user's, typed once and remembered — never a path this
	// codebase knows. Aetox cannot guess where somebody installed n8n, and a
	// built-in guess would be wrong for everyone it was not written for. The
	// precedent is an MCP stdio server, which has always been a command in a
	// config file that Aetox launches on request.
	//
	// Stored beside the address rather than in the vault for the same reason the
	// address is: it is a setting, and a setting the user should be able to read
	// back and correct.
	StartCommand string `json:"startCommand,omitempty"`
}

func ConnectionsPath() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "connections.json"), nil
}

// LoadConnections reads the placements. A missing file is not an error: it
// means nothing has been placed yet, which is the state every install starts in.
func LoadConnections() ([]ConnectionConfig, error) {
	path, err := ConnectionsPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ConnectionConfig
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func SaveConnections(items []ConnectionConfig) error {
	path, err := ConnectionsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

// SetConnectionTargets replaces one connection's `for:` list, creating the entry
// if this is the first time it has been placed.
//
// The list is stored non-nil even when empty, for the same reason MCP does it:
// "attached nowhere" is a decision, and writing nil would make the next read
// treat it as never-configured and hand the connection back to every desk.
func SetConnectionTargets(id string, targets []string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	clean := make([]string, 0, len(targets))
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !containsFold(clean, t) {
			clean = append(clean, t)
		}
	}
	items, err := LoadConnections()
	if err != nil {
		return err
	}
	for i := range items {
		if strings.EqualFold(items[i].ID, id) {
			// Only the field being set. Placement and address are edited from
			// different rows of the same screen, and a writer that rewrote the
			// whole entry would clear the address every time somebody flipped a
			// desk switch.
			items[i].For = clean
			return SaveConnections(items)
		}
	}
	return SaveConnections(append(items, ConnectionConfig{ID: id, For: clean}))
}

// SetConnectionBaseURL records where a self-hosted service lives. An empty
// string clears it, which is how "I typed the wrong address" is undone without
// disconnecting the account.
func SetConnectionBaseURL(id, baseURL string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	clean := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	items, err := LoadConnections()
	if err != nil {
		return err
	}
	for i := range items {
		if strings.EqualFold(items[i].ID, id) {
			items[i].BaseURL = clean
			return SaveConnections(items)
		}
	}
	// No `For` — an entry created by setting an address has not been placed, and
	// writing an empty list here would read as "switched off everywhere" when
	// the user has not been asked yet.
	return SaveConnections(append(items, ConnectionConfig{ID: id, BaseURL: clean}))
}

// SetConnectionStartCommand records how to bring a self-hosted service up. An
// empty string clears it, which removes the button rather than leaving one that
// runs nothing.
func SetConnectionStartCommand(id, command string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	clean := strings.TrimSpace(command)
	items, err := LoadConnections()
	if err != nil {
		return err
	}
	for i := range items {
		if strings.EqualFold(items[i].ID, id) {
			items[i].StartCommand = clean
			return SaveConnections(items)
		}
	}
	return SaveConnections(append(items, ConnectionConfig{ID: id, StartCommand: clean}))
}

// ConnectionStartCommand reports how to bring a service up, or "".
func ConnectionStartCommand(id string) string {
	items, err := LoadConnections()
	if err != nil {
		return ""
	}
	for _, item := range items {
		if strings.EqualFold(item.ID, strings.TrimSpace(id)) {
			return item.StartCommand
		}
	}
	return ""
}

// ConnectionBaseURL reports where a service was said to live, or "" when it has
// no address stored — which for a service that needs one means "not set up",
// and for the rest means nothing at all.
func ConnectionBaseURL(id string) string {
	items, err := LoadConnections()
	if err != nil {
		return ""
	}
	for _, item := range items {
		if strings.EqualFold(item.ID, strings.TrimSpace(id)) {
			return item.BaseURL
		}
	}
	return ""
}

// ConnectionTargets reports one connection's placement, and whether it has ever
// been placed. The second return is what separates "switched off everywhere"
// from "never configured", which the two callers read in opposite directions.
func ConnectionTargets(id string) ([]string, bool) {
	items, err := LoadConnections()
	if err != nil {
		return nil, false
	}
	for _, item := range items {
		if strings.EqualFold(item.ID, strings.TrimSpace(id)) {
			return item.For, item.For != nil
		}
	}
	return nil, false
}

// ConnectionsForDesk returns the connection ids that desk may use.
//
// An unplaced connection is carried by every desk, which is what keeps an
// upgrade from silently taking working tools away: before this file existed,
// every desk could reach GitHub, and a user who has never opened the new page
// has not asked for that to change.
//
// The empty desk name is the pre-modes full desk and carries everything, the
// same reading MCPServersForDesk gives a nil *mode.Mode.
func ConnectionsForDesk(desk string, known []string) []string {
	desk = strings.TrimSpace(desk)
	return connectionsFor(desk, known, desk == "")
}

// ConnectionsForAgent returns the connections the user pointed at one of the
// team. Unlike a desk, a nameless agent carries nothing: "no desk" is the real
// legacy state, while "no agent" is a caller that failed to say who is asking.
func ConnectionsForAgent(name string, known []string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return connectionsFor(MCPAgentPrefix+name, known, false)
}

// connectionsFor is the one matcher behind both, mirroring mcpServersFor. It is
// given the set of connections that exist so that a stale entry in the file —
// a provider removed from a later build — cannot resurrect itself as a name
// nothing recognises.
func connectionsFor(owner string, known []string, all bool) []string {
	placed := map[string][]string{}
	if items, err := LoadConnections(); err == nil {
		for _, item := range items {
			placed[strings.ToLower(strings.TrimSpace(item.ID))] = item.For
		}
	}
	var out []string
	for _, id := range known {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		targets, configured := placed[strings.ToLower(id)]
		if !configured || targets == nil {
			// Never placed: every desk carries it, and no agent does. An
			// agent is handed things on purpose, so silence is not a grant —
			// which is the same asymmetry ConnectionsForAgent states above.
			if all || !strings.HasPrefix(owner, MCPAgentPrefix) {
				out = append(out, id)
			}
			continue
		}
		if all || containsFold(targets, owner) {
			out = append(out, id)
		}
	}
	return out
}

func containsFold(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}
