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
			items[i].For = clean
			return SaveConnections(items)
		}
	}
	return SaveConnections(append(items, ConnectionConfig{ID: id, For: clean}))
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
