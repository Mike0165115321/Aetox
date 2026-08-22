package agentpkg

// A package cannot carry a secret, and this file is the whole of how it says so.
//
// An agent that brings a tool server usually brings one that needs a token. The
// seller has one; the buyer needs their own. So a value in a package's mcp.json
// is either an ordinary setting, written out literally, or a **request**:
//
//	"env": { "NOTION_TOKEN": "${ask:NOTION_TOKEN|Notion integration token}" }
//
// which is a field on the install screen and nothing else. The exporter puts
// these in place of whatever the seller typed, and the installer refuses a
// package whose secret-looking value is *not* one of them — a package that
// ships a working key is shipping the seller's account, and the buyer would
// have no way to know it was not their own.
//
// Where the buyer's answer is written is deliberately not new: it goes where
// MCP env already lives (owner's call, 2026-08-20). The install screen replaces
// a hand-edit of mcp-servers.json and must not quietly change the posture in
// either direction. Moving MCP env into the credential vault is real work and
// is its own piece; connections.go states the rule from the other side.

import "strings"

const (
	askOpen  = "${ask:"
	askClose = "}"
	// askLabelSep separates the field name from what to call it on screen.
	askLabelSep = "|"
)

// AskField is one value the buyer has to supply before a server can run.
type AskField struct {
	// Server is the server entry this belongs to, by name.
	Server string `json:"server"`
	// Key is where the answer lands — the env variable, or the header.
	Key string `json:"key"`
	// Header distinguishes the two, because a remote server's Authorization
	// header and a local one's env var are the same question asked of
	// different transports.
	Header bool `json:"header"`
	// Name is the token's own id, which is what two servers needing the same
	// token share. Defaults to Key.
	Name string `json:"name"`
	// Label is what the install screen calls it. Empty means the screen falls
	// back to Name — a package that did not bother to write a label is not an
	// invalid package.
	Label string `json:"label"`
}

// Ask builds the placeholder a package writes.
func Ask(name, label string) string {
	if label == "" {
		return askOpen + name + askClose
	}
	return askOpen + name + askLabelSep + label + askClose
}

// ParseAsk reads a placeholder back. The whole value must be the placeholder:
// a token glued into a longer string ("Bearer ${ask:X}") is refused rather than
// half-understood, because the half that is understood would be written into a
// config file as if it were finished.
func ParseAsk(value string) (name, label string, ok bool) {
	v := strings.TrimSpace(value)
	if !strings.HasPrefix(v, askOpen) || !strings.HasSuffix(v, askClose) {
		return "", "", false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(v, askOpen), askClose)
	if strings.Contains(inner, askOpen) {
		return "", "", false
	}
	name, label, _ = strings.Cut(inner, askLabelSep)
	name = strings.TrimSpace(name)
	label = strings.TrimSpace(label)
	if name == "" {
		return "", "", false
	}
	return name, label, true
}

// secretWords are what a key is called when it holds something the buyer must
// bring themselves.
//
// A guess, and it is allowed to be one, because both of its mistakes are
// survivable in the right direction: a false positive asks the buyer to type a
// value they already had, and a false negative is caught by the installer's
// refusal only if it also looks secret — so the list is deliberately generous.
// It is not a security boundary. The boundary is that the exporter never
// carries a value it was not told to carry, and the install screen shows every
// value that is being written.
var secretWords = []string{
	"token", "secret", "password", "passwd", "apikey", "api_key",
	"auth", "credential", "private", "session", "cookie", "key",
}

// LooksSecret reports whether a value under this key should be asked for rather
// than shipped.
func LooksSecret(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	for _, w := range secretWords {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}
