package turn

// The cap that ate 14.2% of every tool result on the owner's machine.
//
// Found on 2026-08-22 by reading tool_runs: 473 of 3,335 recorded runs were
// over 4,096 characters, and every one of them reached the model cut, with a
// marker that said only "truncated" and never how much. `skill_view aetox`
// delivered 4,096 of 27,803 characters — 15% of a document whose whole job is
// to be read whole. The skill's end marker, added in August precisely so a
// reader could tell a finished document from a clipped one, sat past the cut
// and had never once arrived.
//
// The number itself was not wrong when it was written: memory.defaultMaxChars
// is 128000, and 128000/32 is 4000. It was one thirty-second of the history
// budget. Then the budget started scaling with the model's real window and this
// did not.

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/Mike0165115321/Aetox/internal/model"
)

func TestOutputBackstopScalesWithTheBudgetItIsAFractionOf(t *testing.T) {
	// The case that has to keep working exactly as it did: nothing knows this
	// model's window, so nothing changes. A caller that forgets to pass the
	// budget lands here, which is today's behaviour rather than a surprise.
	if got := OutputBackstop(0); got != defaultOutputBackstop {
		t.Errorf("OutputBackstop(0) = %d, want the floor %d", got, defaultOutputBackstop)
	}
	if got := OutputBackstop(-1); got != defaultOutputBackstop {
		t.Errorf("a negative budget must land on the floor, got %d", got)
	}

	// The old default budget, which is where 4096 came from in the first place.
	// One thirty-second of 128000 is 4000, under the floor, so the floor holds.
	if got := OutputBackstop(128000); got != defaultOutputBackstop {
		t.Errorf("OutputBackstop(128000) = %d, want the floor %d", got, defaultOutputBackstop)
	}

	// A 1M-token model: 4,000,000 characters of budget, so one result may be
	// 125,000. That is the case this whole change exists for.
	if got := OutputBackstop(model.HistoryChars(1_000_000)); got != 125_000 {
		t.Errorf("OutputBackstop for a 1M-token window = %d, want 125000", got)
	}

	// Monotonic: a bigger budget never buys a smaller result.
	prev := 0
	for _, chars := range []int{0, 128_000, 512_000, 4_000_000} {
		got := OutputBackstop(chars)
		if got < prev {
			t.Errorf("budget %d gave %d, less than the %d a smaller budget gave", chars, got, prev)
		}
		prev = got
	}
}

func TestHistoryCharsHasOneHome(t *testing.T) {
	if got := model.HistoryChars(1000); got != 4000 {
		t.Errorf("HistoryChars(1000) = %d, want 4000", got)
	}
	// Unknown window stays unknown rather than becoming zero-sized budget.
	if got := model.HistoryChars(0); got != 0 {
		t.Errorf("HistoryChars(0) = %d, want 0 so callers can tell it apart", got)
	}
}

func TestTrimSaysHowMuchItTook(t *testing.T) {
	long := strings.Repeat("a", 5000)
	got := trimToBackstop(long, 4096)

	// The whole point. "truncated" alone cannot tell a lost sentence from a
	// lost document, and the tools underneath publish continuation contracts
	// that only work if the reader knows a cut happened at all.
	if !strings.Contains(got, "4096") || !strings.Contains(got, "5000") {
		t.Errorf("the marker must carry both numbers, got the tail: %q", got[len(got)-160:])
	}
	if !strings.Contains(got, "ask for the rest") {
		t.Errorf("the marker must warn that the tool's own continuation line may be gone, got: %q", got[len(got)-160:])
	}
}

func TestTrimLeavesAnHonestResultAlone(t *testing.T) {
	short := "all of it"
	if got := trimToBackstop(short, 4096); got != short {
		t.Errorf("a result inside the limit must come back untouched, got %q", got)
	}
	// A zero limit means no backstop was resolved; silently emptying the result
	// would be worse than any cap.
	if got := trimToBackstop(short, 0); got != short {
		t.Errorf("limit 0 must not truncate, got %q", got)
	}
}

func TestTrimNeverSplitsACharacter(t *testing.T) {
	// Thai is three bytes a character, so a byte-indexed cut lands mid-character
	// for two out of every three limits. This package serves Thai first, and the
	// old code sliced bytes.
	thai := strings.Repeat("ก", 100) // 300 bytes
	for limit := 10; limit < 40; limit++ {
		got := trimToBackstop(thai, limit)
		body := got
		if i := strings.Index(got, "\n...(output truncated"); i >= 0 {
			body = got[:i]
		}
		if !utf8.ValidString(body) {
			t.Fatalf("limit %d produced an invalid rune at the cut", limit)
		}
		if len(body) > limit {
			t.Fatalf("limit %d kept %d bytes, more than asked", limit, len(body))
		}
	}
}

// budgetedAgent is toolAwareAgent plus the one method the backstop asks for.
type budgetedAgent struct {
	toolAwareAgent
	chars int
}

func (a *budgetedAgent) HistoryChars() int { return a.chars }

func TestTheExecutorTakesItsBackstopFromTheAgent(t *testing.T) {
	// The wiring, tested end to end, because the first attempt at this passed
	// every unit test and still cut every result at the floor: it sized the
	// backstop off config.ModelContextTokens, an optional override that is
	// almost always zero, so the code read correctly and did nothing.
	big := NewExecutor(ExecutorOptions{Agent: &budgetedAgent{chars: 4_000_000}})
	if big.summaryLimit != 125_000 {
		t.Errorf("executor with a 4M-char agent has limit %d, want 125000", big.summaryLimit)
	}

	// An agent that cannot answer keeps the floor rather than losing its output.
	plain := NewExecutor(ExecutorOptions{Agent: &toolAwareAgent{}})
	if plain.summaryLimit != defaultOutputBackstop {
		t.Errorf("an agent with no budget gave limit %d, want the floor %d", plain.summaryLimit, defaultOutputBackstop)
	}

	// An explicit limit still wins, so a caller with a reason can set one.
	explicit := NewExecutor(ExecutorOptions{Agent: &budgetedAgent{chars: 4_000_000}, SummaryLimit: 999})
	if explicit.summaryLimit != 999 {
		t.Errorf("an explicit SummaryLimit was overridden: %d", explicit.summaryLimit)
	}
}

func TestTheEnvOverrideIsAnEscapeHatchNotADial(t *testing.T) {
	// sync.OnceValue means the real variable is read once per process, so the
	// parsing rule is tested through the same code by resetting the once.
	reset := func() { backstopOverride = sync.OnceValue(readBackstopOverride) }
	defer reset()

	t.Setenv("AETOX_MAX_TOOL_OUTPUT", "50000")
	reset()
	if got := OutputBackstop(4_000_000); got != 50_000 {
		t.Errorf("an explicit override must win over the scaled budget, got %d", got)
	}

	// A typo must not be obeyed. Obeying a zero would silently empty every tool
	// result for the whole session — a far worse answer to a misspelling than
	// carrying on with the default.
	for _, bad := range []string{"0", "-1", "lots", ""} {
		t.Setenv("AETOX_MAX_TOOL_OUTPUT", bad)
		reset()
		if got := OutputBackstop(4_000_000); got != 125_000 {
			t.Errorf("AETOX_MAX_TOOL_OUTPUT=%q should be ignored, got %d", bad, got)
		}
	}
}
