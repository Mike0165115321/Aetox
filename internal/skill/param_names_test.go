package skill

// What a tool asks for is named after a word the trade already has. `find` and
// `replace` are the two labels on every search-and-replace box ever shipped;
// `limit`/`offset` is how paging has been spelled since SQL, and `read` takes
// the same pair for the same reason; `show` says in plain English what a search
// should hand back, over values grep itself already uses. A name invented here
// would have to be learned before it could be used, and there is no round to
// spare for that.
//
// This test pins the set so it cannot drift — a sixth parameter appearing on
// `edit`, or `show` quietly becoming `mode`. Changing the list is allowed and
// is the point: it should be a line in a diff somebody chose to write.
//
// It sees every tool schema the engine registers. `browser` and `task` are
// registered by the desktop, out of this package's reach.

import (
	"encoding/json"
	"sort"
	"testing"
)

// paramNames returns every property name in a tool's JSON schema, nested ones
// included — the `edits` tool keeps its match strings one level down, inside
// the items of its list.
func paramNames(t *testing.T, name string, raw json.RawMessage) []string {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("%s: schema does not parse: %v", name, err)
	}
	var walk func(node map[string]any) []string
	walk = func(node map[string]any) []string {
		var found []string
		if props, ok := node["properties"].(map[string]any); ok {
			for key, child := range props {
				found = append(found, key)
				if sub, ok := child.(map[string]any); ok {
					found = append(found, walk(sub)...)
				}
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			found = append(found, walk(items)...)
		}
		return found
	}
	names := walk(schema)
	sort.Strings(names)
	return names
}

func TestTheFileAndSearchToolsAskForExactlyTheseNames(t *testing.T) {
	want := map[string][]string{
		// write, edit, append, batch and delete are actions of `change` now
		// (change_pack.go). `path` twice: once per edit inside batch, once as
		// the call-wide default. `mode` is gone - append is an action.
		"change": {"action", "all", "content", "edits", "find", "find", "path", "path", "recursive", "replace", "replace"},
		// grep and glob are actions of `search` now (search_pack.go), so the
		// names the model reads are the pack's - one `path`, one `pattern`, and
		// grep's five own options beside them.
		"search": {"action", "context", "glob", "limit", "multiline", "offset", "path", "pattern", "show", "type"},
		"read":   {"limit", "offset", "path"},
	}

	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})
	dispatcher := NewDispatcher(registry)

	seen := map[string]bool{}
	for _, def := range dispatcher.ToolDefinitions() {
		expected, pinned := want[def.Function.Name]
		if !pinned {
			continue
		}
		seen[def.Function.Name] = true
		got := paramNames(t, def.Function.Name, def.Function.Parameters)
		if len(got) != len(expected) {
			t.Errorf("%s takes %v, pinned as %v", def.Function.Name, got, expected)
			continue
		}
		for i := range got {
			if got[i] != expected[i] {
				t.Errorf("%s takes %v, pinned as %v", def.Function.Name, got, expected)
				break
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("%s is pinned here but no longer registered — delete its line or find out why", name)
		}
	}
}

// One idea, one name. `read` pages and so do two of `search`'s three actions,
// and a reader who learned the pair on one must not have to learn it again on
// the next. Asked of the tools as the model meets them, which is why `search`
// stands here for the grep and glob that used to.
func TestPagingIsSpelledTheSameWayOnEveryToolThatPages(t *testing.T) {
	registry := NewDefaultRegistry(RegistryOptions{SandboxRoot: t.TempDir()})

	for _, name := range []string{"read", "search"} {
		sk, ok := registry.Get(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		tool, ok := sk.(Tool)
		if !ok {
			t.Fatalf("%s is not a tool", name)
		}
		names := paramNames(t, name, tool.ToolDefinition().Function.Parameters)
		var hasLimit, hasOffset bool
		for _, param := range names {
			hasLimit = hasLimit || param == "limit"
			hasOffset = hasOffset || param == "offset"
		}
		if !hasLimit || !hasOffset {
			t.Errorf("%s takes %v, and a tool that pages spells it limit/offset", name, names)
		}
	}
}
