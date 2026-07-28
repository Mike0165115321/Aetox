package model

import (
	"errors"
	"strings"
	"testing"
)

func TestScanSSECollectsDataLines(t *testing.T) {
	stream := strings.Join([]string{
		": a comment",
		"event: response.output_text.delta",
		`data: {"delta":"he"}`,
		"",
		"event: response.output_text.delta",
		`data: {"delta":"llo"}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	var got []string
	err := scanSSE(strings.NewReader(stream), func(data string) (bool, error) {
		got = append(got, data)
		return false, nil
	})
	if err != nil {
		t.Fatalf("scanSSE: %v", err)
	}
	want := []string{`{"delta":"he"}`, `{"delta":"llo"}`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("payloads = %v; want %v (comments, event: lines and [DONE] dropped)", got, want)
	}
}

func TestScanSSEStopsAndPropagatesErrors(t *testing.T) {
	stream := "data: one\n\ndata: two\n\ndata: three\n\n"

	seen := 0
	if err := scanSSE(strings.NewReader(stream), func(string) (bool, error) {
		seen++
		return seen == 2, nil
	}); err != nil {
		t.Fatalf("scanSSE: %v", err)
	}
	if seen != 2 {
		t.Fatalf("handler ran %d times; want it to stop after the second", seen)
	}

	boom := errors.New("boom")
	if err := scanSSE(strings.NewReader(stream), func(string) (bool, error) {
		return false, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("err = %v; want the handler's error", err)
	}
}

// The whole reason this helper exists: the Responses API puts an entire turn —
// every tool call and its arguments — on one line of the final event, and
// bufio.Scanner's 64 KB default kills the stream after the model has already
// done the work.
func TestScanSSEHandlesEventsLargerThan64KB(t *testing.T) {
	huge := strings.Repeat("x", 300*1024)
	var got string
	err := scanSSE(strings.NewReader("data: "+huge+"\n\n"), func(data string) (bool, error) {
		got = data
		return false, nil
	})
	if err != nil {
		t.Fatalf("scanSSE: %v", err)
	}
	if len(got) != len(huge) {
		t.Fatalf("payload length = %d; want %d", len(got), len(huge))
	}
}
