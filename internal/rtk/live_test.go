package rtk

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// The bound has to be real, not decorative. Before WaitDelay, this exact call
// had not returned after five minutes despite a 5-second context: os/exec was
// waiting on a pipe a grandchild still held open.
func TestRewriteRespectsItsOwnTimeout(t *testing.T) {
	if _, err := exec.LookPath("rtk"); err != nil {
		t.Skip("rtk not installed")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Rewrite(context.Background(), "git log")
	}()
	select {
	case <-done:
	case <-time.After(rtkTimeout + waitDelay + 3*time.Second):
		t.Fatalf("Rewrite did not return within %v — the timeout is not being enforced", rtkTimeout+waitDelay)
	}
}

// The measurements the allowlist is built on, checked against the real binary
// so a future rtk that changes behaviour surfaces here instead of as a quiet
// rise in token spend.
func TestLiveRewriteDecisions(t *testing.T) {
	if _, err := exec.LookPath("rtk"); err != nil {
		t.Skip("rtk not installed")
	}
	if _, ok := Rewrite(context.Background(), "go build ./..."); ok {
		t.Error("go build must not be rewritten — measured 0->96 bytes on success, 218->439 on failure")
	}
	if _, ok := Rewrite(context.Background(), "npm install -g typescript"); ok {
		t.Error("npm install must not be rewritten — rtk has no equivalent for it")
	}
	if _, ok := Rewrite(context.Background(), "git log"); !ok {
		t.Error("git log must still be rewritten — measured 185,616 -> 4,514 bytes")
	}
}

// Stop has to reach rtk. It used to run on context.Background(), so a user
// hitting Stop mid-turn could not interrupt it — the turn stayed busy on a
// subprocess nobody could reach.
func TestCancelledContextReturnsImmediately(t *testing.T) {
	if _, err := exec.LookPath("rtk"); err != nil {
		t.Skip("rtk not installed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already stopped, as if the user hit the button first

	start := time.Now()
	if _, ok := Rewrite(ctx, "git log"); ok {
		t.Error("a cancelled context must not produce a rewrite")
	}
	if _, ok := Filter(ctx, "git-status", "on branch main"); ok {
		t.Error("a cancelled context must not produce filtered output")
	}
	// Both should fail out at once, not sit through the 5s ceiling.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v to notice a cancelled context", elapsed)
	}
}
