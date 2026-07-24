package skill

import (
	"strconv"
	"sync"
	"testing"
)

// Registry must tolerate MCP tools being registered from a background
// goroutine while turns read it live (see desktop applyConfig). Run with
// -race when cgo is available; even without it, this catches a missing lock
// via map-concurrent-write panics.
func TestRegistryConcurrentRegisterAndRead(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	// Writers: register distinct tools concurrently.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = r.Register(&stubSkill{name: "tool-" + strconv.Itoa(n) + "-" + strconv.Itoa(j)}, SourceMCP)
			}
		}(i)
	}

	// Readers: hammer the live-read paths the dispatcher uses.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				for _, n := range r.Names() {
					r.Get(n)
					r.SourceOf(n)
				}
				r.Snapshot()
			}
		}()
	}

	wg.Wait()
	if got := len(r.Names()); got != 8*50 {
		t.Fatalf("registered %d tools, want %d", got, 8*50)
	}
}
