package skill

import (
	"fmt"
	"strings"
)

// One tool call may carry 300 lines of content. Not because 300 is special,
// but because everything above it is a gamble against a number the model
// cannot see.
//
// The round's output ceiling is what actually decides whether a call survives,
// and it ranges from 512 tokens (a nearly full window) to 65,536 (DeepSeek V4)
// across the providers Aetox speaks to — see providerOutputCeiling and
// clampToWindow in internal/cognitive. A model cannot count its own tokens
// against that, and nothing in the wire tells it where the line is. It can
// count lines, which is the entire reason this cap is stated in lines.
//
// 300 against the ceiling met most often (8,192: DeepSeek V3, OpenRouter,
// Groq, and any provider we have no figure for):
//
//	code, ~12 tokens/line   →  ~3,600   room to spare
//	HTML, ~20 tokens/line   →  ~6,000   fits, getting tight
//	Thai prose, ~35/line    → ~10,500   over
//
// So the cap is not a promise that 300 lines always fits. Thai runs about one
// token per two characters (the same measurement clampToWindow is built on),
// and the ceiling is shared with reasoning and whatever the model says before
// the call. What the cap does is keep ordinary work — code and markup, which
// is most of what write is for — inside one call on every provider, and turn
// the remainder into a refusal that arrives before anything is written rather
// than a truncation that arrives after.
//
// Chosen over the tighter numbers considered (100, 250) because the two costs
// are not symmetric. Too low spends an extra round on every large file, for
// ever, and that was already decided against once: deepseekV4OutputTokenMax
// was raised to 64K precisely so a whole file need not be split, since every
// split resends the full context. Too high spends one wasted round when a file
// genuinely overflows, and only then.
const contentLineCap = 300

// contentLines counts what the cap counts. A file ending in a newline is not
// credited with a phantom last line.
func contentLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}

// checkContentLineCap refuses a call whose content is over the cap, and says
// what to do instead.
//
// It guards every door, not just write's: a cap that only watched write would
// be routed around by one enormous edit, which is the same content through a
// different name. The remedy is the same either way, and it is the reason the
// cap is affordable at all — append continues a file without re-sending it.
func checkContentLineCap(field, content string) error {
	lines := contentLines(content)
	if lines <= contentLineCap {
		return nil
	}
	return fmt.Errorf(
		"%s is %d lines, over the %d-line limit for one call. Nothing was written. "+
			"Send the first %d lines or fewer, then continue the file with edit mode=append. "+
			"This is a cap on one call, not on the file: a longer file is written in several.",
		field, lines, contentLineCap, contentLineCap)
}
