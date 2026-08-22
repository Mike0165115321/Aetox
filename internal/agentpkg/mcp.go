package agentpkg

// The servers a package brings, on both sides of the road.
//
// Reading: mcp.json inside a package is an array in mcp-servers.json's own
// schema, so a server that already works can be lifted from one file to the
// other by copy-paste and nothing has to be translated. Two fields are not the
// package's to write, and are dropped on the way in rather than being trusted
// and checked later:
//
//   - `for:` — a package declares, it never grants. The installer writes
//     ["agent:<name>"], which is also what makes the server agent-only and so
//     deferred until that agent is actually spoken to.
//   - `source:` — provenance is the installer's record of what it did, not a
//     claim a file gets to make about itself.
//
// Writing: the exporter turns the servers currently placed on an agent back
// into that array, with every secret-looking value replaced by an ${ask:...}
// the buyer answers at install.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/Mike0165115321/Aetox/internal/config"
)

// ReadDeclaredMCP reads a package's mcp.json out of its folder.
//
// An absent file is the normal state — most workers bring no servers — and is
// not an error. A malformed one is: the buyer is about to be shown what this
// package will add to their machine, and a list that was half-understood is
// worse than no install.
func ReadDeclaredMCP(pkg fs.FS) ([]config.MCPServerConfig, error) {
	raw, err := fs.ReadFile(pkg, config.AgentMCPFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var servers []config.MCPServerConfig
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("%s ของแพ็กเกจนี้อ่านไม่ออก: %w", config.AgentMCPFile, err)
	}
	out := make([]config.MCPServerConfig, 0, len(servers))
	for _, s := range servers {
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" {
			return nil, fmt.Errorf("%s มีเซิร์ฟเวอร์ที่ไม่มีชื่อ", config.AgentMCPFile)
		}
		// Not the package's to write. Dropped rather than rejected: a seller who
		// exported by hand and left a `for:` in has made a harmless mistake, and
		// refusing the whole install over it would teach nothing.
		s.For = nil
		s.Source = ""
		out = append(out, s)
	}
	return out, nil
}

// PlacedServers returns the servers the user has pointed at this agent, in full.
//
// The placement lives on the server (`for: agent:<name>`), so this is two reads
// and a filter rather than anything the agent's folder knows. It is what the
// exporter packs and what the settings page would show.
func PlacedServers(name string) ([]config.MCPServerConfig, error) {
	placed := config.MCPServersForAgent(name)
	if len(placed) == 0 {
		return nil, nil
	}
	all, err := config.LoadMCPServers()
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(placed))
	for _, n := range placed {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var out []config.MCPServerConfig
	for _, s := range all {
		if want[strings.ToLower(strings.TrimSpace(s.Name))] {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// declareMCP turns live servers into what a package ships, and reports every
// value it refused to carry.
//
// Disabled is cleared on purpose. A seller who had the server switched off is
// describing their own machine on the day they exported; a package that arrives
// switched off is a package that does not work and says nothing about why.
func declareMCP(servers []config.MCPServerConfig) ([]config.MCPServerConfig, []AskField) {
	out := make([]config.MCPServerConfig, 0, len(servers))
	var asked []AskField
	for _, s := range servers {
		s.For = nil
		s.Source = ""
		s.Disabled = false
		s.Environment, asked = redact(s.Name, s.Environment, false, asked)
		s.Headers, asked = redact(s.Name, s.Headers, true, asked)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, asked
}

// redact replaces the values a buyer must bring themselves. A value that is
// already a placeholder is left as it is, so exporting a package that was
// installed from one does not ask the same question twice under a new name.
func redact(server string, values map[string]string, header bool, asked []AskField) (map[string]string, []AskField) {
	if len(values) == 0 {
		return values, asked
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(values))
	for _, k := range keys {
		v := values[k]
		if name, label, ok := ParseAsk(v); ok {
			out[k] = v
			asked = append(asked, AskField{Server: server, Key: k, Header: header, Name: name, Label: label})
			continue
		}
		if !LooksSecret(k) || strings.TrimSpace(v) == "" {
			out[k] = v
			continue
		}
		out[k] = Ask(k, "")
		asked = append(asked, AskField{Server: server, Key: k, Header: header, Name: k})
	}
	return out, asked
}

// marshalServers writes the declaration the way mcp-servers.json is written —
// same indent, trailing newline — so a seller who opens the file inside the zip
// reads something they already recognise.
func marshalServers(servers []config.MCPServerConfig) ([]byte, error) {
	raw, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, "\n"...), nil
}
