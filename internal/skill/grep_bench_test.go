package skill

import (
	"context"
	"testing"
)

// The ignore list is a performance change as much as a correctness one: 10,826
// of this repo's 12,073 files are node_modules, and grep opened and read every
// one of them. Run against the real repo to measure it, not a fixture.
func BenchmarkGrepRealRepo(b *testing.B) {
	s := &grepSkill{root: "../.."}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := s.ExecuteTool(context.Background(), map[string]any{
			"pattern": "func handleToolCall", "glob": "*.go",
		}); err != nil {
			b.Fatal(err)
		}
	}
}
