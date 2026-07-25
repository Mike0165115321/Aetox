package model

import (
	"fmt"
	"strings"
	"testing"
)

// The accumulator sees every streamed fragment of a tool call, and both helpers
// it calls scan the arguments accumulated *so far* — which grow with the file
// being written. That is quadratic by construction, so it is worth knowing what
// it actually costs before shipping it on the hot path.
func BenchmarkStreamAccumulatorLargeWrite(b *testing.B) {
	// An 800-line HTML file, arriving the way a model streams one.
	var frags []string
	frags = append(frags, `{"path": "landing.html", "content": "`)
	for i := 0; i < 800; i++ {
		frags = append(frags, fmt.Sprintf(`  <div class=\"row-%d\">line of content here</div>\n`, i))
	}
	frags = append(frags, `"}`)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		acc := newStreamToolAccumulator(func(string, string, int) {})
		for _, f := range frags {
			acc.add([]streamToolCallDelta{{Index: 0, Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "write", Arguments: f}}})
		}
	}
}

func BenchmarkSubjectFromPartialArgs(b *testing.B) {
	args := `{"path": "internal/skill/edit.go", "content": "` + strings.Repeat(`x\n`, 5000)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		SubjectFromPartialArgs(args)
	}
}
