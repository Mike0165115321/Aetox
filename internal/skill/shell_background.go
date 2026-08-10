package skill

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mike0165115321/Aetox/internal/proc"
)

// Background commands: the third of the three tools a coding agent needs from a
// terminal and the only one Aetox had no answer for at all.
//
// `shell` runs a command and waits, abandoned at 60 seconds (ARCHITECTURE.md
// §50 made that adjustable). That covers a test run and a build. It cannot
// cover the other half of development — a dev server, a watch build, a log
// tail — because those never exit, and "never exits" is not a slow command, it
// is a different shape of work. Before this the model simply could not start
// one: it would hang until the deadline and be killed.
//
// Three tools sharing one runner, the same arrangement §44 uses for `task`:
//
//   - shell with run_in_background:true starts it and returns a handle at once
//   - shell_output redeems the handle for whatever has been printed since the
//     last read
//   - shell_kill ends it
//
// The handle, not the process, is what the model holds. It never sees a pid,
// so it cannot kill something Aetox did not start.

// maxBackgroundShells bounds how many commands can be running unattended. A
// model that starts a hundred servers has misunderstood something, and the
// tenth is where saying so is still cheap.
const maxBackgroundShells = 8

// backgroundOutputCap is per job, and separate from toolOutputByteCap because
// a log tail is precisely the thing that produces megabytes: the buffer keeps
// the newest bytes rather than the oldest, which is the opposite of what a
// finished command wants and the only useful answer for a running one.
const backgroundOutputCap = 256 << 10

type backgroundShells struct {
	mu   sync.Mutex
	seq  int
	jobs map[string]*backgroundJob
}

func newBackgroundShells() *backgroundShells {
	return &backgroundShells{jobs: make(map[string]*backgroundJob)}
}

type backgroundJob struct {
	id      string
	command string
	cancel  context.CancelFunc
	started time.Time

	mu       sync.Mutex
	buf      []byte
	cursor   int // bytes already handed to shell_output
	overflow bool
	done     bool
	exitErr  error
	ended    time.Time
	killed   bool
}

// Write is the job's stdout and stderr. It keeps the tail, not the head: for a
// server or a watcher the interesting line is always the most recent one.
func (j *backgroundJob) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.buf = append(j.buf, p...)
	if len(j.buf) > backgroundOutputCap {
		drop := len(j.buf) - backgroundOutputCap
		j.buf = j.buf[drop:]
		j.overflow = true
		// The cursor indexes the same slice, so it has to slide with it or the
		// next read replays bytes the caller already has.
		if j.cursor -= drop; j.cursor < 0 {
			j.cursor = 0
		}
	}
	return len(p), nil
}

// peek returns what drain would return without consuming it. wait_for has to
// look at the same window repeatedly, and only the read that reports it to the
// model is allowed to mark it read.
func (j *backgroundJob) peek() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return string(j.buf[j.cursor:])
}

// drain returns what has arrived since the last drain and marks it read.
func (j *backgroundJob) drain() (string, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	fresh := string(j.buf[j.cursor:])
	j.cursor = len(j.buf)
	overflowed := j.overflow
	j.overflow = false
	return fresh, overflowed
}

func (j *backgroundJob) status() (done, killed bool, exitErr error, since time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.done {
		return true, j.killed, j.exitErr, j.ended.Sub(j.started)
	}
	return false, false, nil, time.Since(j.started)
}

// start launches the command detached from the turn.
//
// context.Background, deliberately: the turn's context is cancelled the moment
// the turn ends, and a dev server that dies with the answer that started it is
// not a background command. What still stops them is the job object every
// child is already in (§24), so nothing here outlives the app.
func (b *backgroundShells) start(backend proc.Backend, workDir, commandLine string) (*backgroundJob, error) {
	b.mu.Lock()
	if len(b.jobs) >= maxBackgroundShells {
		running := 0
		for _, j := range b.jobs {
			if done, _, _, _ := j.status(); !done {
				running++
			}
		}
		if running >= maxBackgroundShells {
			b.mu.Unlock()
			return nil, fmt.Errorf("%d background commands are already running — read or kill one before starting another", running)
		}
		// Only finished jobs are holding the slots; forget the oldest of them
		// rather than refusing to run.
		b.forgetFinishedLocked()
	}
	b.seq++
	id := "bg_" + strconv.Itoa(b.seq)
	b.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := backend.Command(ctx, commandLine, workDir)

	job := &backgroundJob{id: id, command: commandLine, cancel: cancel, started: time.Now()}
	cmd.Stdout = job
	cmd.Stderr = job

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	b.mu.Lock()
	b.jobs[id] = job
	b.mu.Unlock()

	go func() {
		err := cmd.Wait()
		cancel()
		job.mu.Lock()
		job.done, job.exitErr, job.ended = true, err, time.Now()
		job.mu.Unlock()
	}()
	return job, nil
}

// forgetFinishedLocked drops completed jobs whose output has been read. Called
// with b.mu held.
func (b *backgroundShells) forgetFinishedLocked() {
	ids := make([]string, 0, len(b.jobs))
	for id := range b.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		job := b.jobs[id]
		if done, _, _, _ := job.status(); !done {
			continue
		}
		job.mu.Lock()
		unread := job.cursor < len(job.buf)
		job.mu.Unlock()
		if !unread {
			delete(b.jobs, id)
		}
	}
}

func (b *backgroundShells) get(id string) (*backgroundJob, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	job, ok := b.jobs[strings.TrimSpace(id)]
	return job, ok
}

// running lists the ids still going, so a lost handle can be recovered from an
// error message instead of leaving the process unreachable forever.
func (b *backgroundShells) running() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, 0, len(b.jobs))
	for id, job := range b.jobs {
		if done, _, _, _ := job.status(); !done {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// snapshot is every job still remembered, one line each, for shell's `list`
// action. Finished jobs are included as long as they are held: a model that
// lost a handle has usually lost it to a summary, and the job it is looking for
// may well have ended — "ended, output unread" is the answer that lets it
// decide whether to read or to let go.
func (b *backgroundShells) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, 0, len(b.jobs))
	for id := range b.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		job := b.jobs[id]
		done, killed, _, since := job.status()
		state := "running " + since.Round(time.Second).String()
		switch {
		case killed:
			state = "killed after " + since.Round(time.Second).String()
		case done:
			state = "ended after " + since.Round(time.Second).String()
		}
		job.mu.Lock()
		if job.cursor < len(job.buf) {
			state += ", output unread"
		}
		job.mu.Unlock()
		lines = append(lines, fmt.Sprintf("%s  (%s)  %s", id, state, job.command))
	}
	return lines
}

// --- shell_output -----------------------------------------------------------

// Waiting: shell_output can block on a condition instead of reporting at once.
//
// Without it the model's only move while a build runs or a server starts is to
// call shell_output, read "nothing new", and call again — every one of those a
// full model round paid for learning nothing. The fix is not a sleep tool:
// sleep makes the model guess a duration, and both guesses are wrong (too
// short polls anyway, too long sits idle after the work is done). The model
// names the condition — a line of output, or the exit — and the process waits.
//
// Every wait is bounded. wait_timeout_seconds caps it, the command's own exit
// ends it early, and the turn's ctx (the Stop button) cuts through it. The
// turn executor reads wait_timeout_seconds too, so the turn's patience
// stretches to match the wait it was asked for (internal/turn.toolCallDeadline).
const (
	defaultWaitTimeout = 60 * time.Second
	maxWaitTimeout     = 600 * time.Second
	waitPollInterval   = 200 * time.Millisecond
)

// waitForExit is the wait_for value that means "the exit itself", not a
// pattern. A log that literally prints the word "exit" loses the ability to
// wait on that word — documented in the schema, and cheap next to making the
// model juggle a second boolean.
const waitForExit = "exit"

type shellOutputSkill struct{ shells *backgroundShells }

func (*shellOutputSkill) Name() string { return "shell_output" }

func (*shellOutputSkill) Description() string {
	return "อ่านผลลัพธ์ใหม่จากคำสั่งที่รันอยู่เบื้องหลัง"
}

// No ToolDefinition, and none for shellKillSkill below. Both are actions on
// `shell` now (packed.go), so their parameters and their sentences live in that
// one schema — a second definition here would be a description nobody sends,
// free to drift from the one the model actually reads. `shell_output` remains
// the name every gate judges an output read by; what stopped existing is the
// entry in the tool block, not the name.
//
// They are still their own types because they are still their own work, and
// because the background-shell tests drive them directly.

func (s *shellOutputSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: shell_output <shell_id>")
		return newToolOutput("shell_output", "shell_output", "", time.Now(), false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"shell_id": args[0]})
}

func (s *shellOutputSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	id, _ := args["shell_id"].(string)
	id = strings.TrimSpace(id)
	command := "shell_output " + id
	if id == "" {
		err := errors.New("shell_id is required")
		return newToolOutput("shell_output", "shell_output", "", start, false, err), err
	}
	job, ok := s.shells.get(id)
	if !ok {
		err := fmt.Errorf("no background command %q — %s", id, describeRunning(s.shells))
		return newToolOutput("shell_output", command, "", start, false, err), err
	}

	waitNote := ""
	if target, _ := args["wait_for"].(string); strings.TrimSpace(target) != "" {
		note, err := waitOnJob(ctx, job, strings.TrimSpace(target), waitTimeout(args))
		if err != nil {
			return newToolOutput("shell_output", command, "", start, false, err), err
		}
		waitNote = note
	}

	fresh, overflowed := job.drain()
	if pattern, _ := args["filter"].(string); strings.TrimSpace(pattern) != "" {
		filtered, err := filterLines(fresh, pattern)
		if err != nil {
			return newToolOutput("shell_output", command, "", start, false, err), err
		}
		fresh = filtered
	}

	done, killed, exitErr, elapsed := job.status()
	header := fmt.Sprintf("[%s] %s — running for %s", id, job.command, elapsed.Round(time.Second))
	switch {
	case killed:
		header = fmt.Sprintf("[%s] %s — killed after %s", id, job.command, elapsed.Round(time.Second))
	case done && exitErr != nil:
		header = fmt.Sprintf("[%s] %s — exited with %v after %s", id, job.command, exitErr, elapsed.Round(time.Second))
	case done:
		header = fmt.Sprintf("[%s] %s — finished in %s", id, job.command, elapsed.Round(time.Second))
	}
	if waitNote != "" {
		header += "\n" + waitNote
	}
	if overflowed {
		header += "\n(earlier output was dropped — this command prints faster than it is being read)"
	}

	body := strings.TrimRight(fresh, "\r\n")
	if body == "" {
		body = "(nothing new since the last read)"
	}
	body, truncated := limitLines(body, defaultToolOutputLineLimit)
	// Not an error even when the command failed: shell_output succeeded at its
	// own job, which is reporting. The header carries the command's fate.
	return newToolOutput("shell_output", command, header+"\n\n"+body, start, truncated, nil), nil
}

// waitOnJob blocks until the job's unread output matches target, the job
// exits, or the timeout passes — whichever is first. It never consumes output;
// the drain that follows it does. The returned note names what ended the wait,
// because "here is the output" and "I gave up waiting for it" must not read
// the same. Only a bad pattern or a canceled turn is an error: a timeout is a
// truthful report, and truthful reporting is this tool's whole job.
func waitOnJob(ctx context.Context, job *backgroundJob, target string, timeout time.Duration) (string, error) {
	var re *regexp.Regexp
	if target != waitForExit {
		var err error
		if re, err = regexp.Compile(target); err != nil {
			return "", fmt.Errorf("wait_for %q is not a valid regular expression: %w", target, err)
		}
	}
	began := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(waitPollInterval)
	defer tick.Stop()
	for {
		if re != nil && re.MatchString(job.peek()) {
			return fmt.Sprintf("(wait_for /%s/ matched after %s)", target, time.Since(began).Round(time.Second)), nil
		}
		if done, _, _, _ := job.status(); done {
			// Output can land between the peek above and the exit check, and a
			// line printed on the way out is a match, not a missed one. The job
			// is done, so this second look is the final word.
			if re != nil && !re.MatchString(job.peek()) {
				return fmt.Sprintf("(/%s/ never appeared — the command ended first)", target), nil
			}
			if re != nil {
				return fmt.Sprintf("(wait_for /%s/ matched after %s)", target, time.Since(began).Round(time.Second)), nil
			}
			return "", nil // the header already reports the exit
		}
		select {
		case <-ctx.Done():
			// The Stop button. The command keeps running — only the wait dies.
			return "", ctx.Err()
		case <-deadline.C:
			what := "the command to finish"
			if re != nil {
				what = "/" + target + "/"
			}
			return fmt.Sprintf("(waited %s for %s — not yet; the command is still running and was NOT stopped)",
				timeout.Round(time.Second), what), nil
		case <-tick.C:
		}
	}
}

// waitTimeout mirrors shell's timeout_seconds handling: absent, unparseable or
// non-positive falls back to the default, and the ceiling holds. JSON numbers
// decode to float64, so an integer schema still arrives as one.
func waitTimeout(args map[string]any) time.Duration {
	var seconds float64
	switch v := args["wait_timeout_seconds"].(type) {
	case float64:
		seconds = v
	case int:
		seconds = float64(v)
	default:
		return defaultWaitTimeout
	}
	if seconds <= 0 {
		return defaultWaitTimeout
	}
	if d := time.Duration(seconds) * time.Second; d < maxWaitTimeout {
		return d
	}
	return maxWaitTimeout
}

// --- shell_kill -------------------------------------------------------------

type shellKillSkill struct{ shells *backgroundShells }

func (*shellKillSkill) Name() string { return "shell_kill" }

func (*shellKillSkill) Description() string {
	return "หยุดคำสั่งที่รันอยู่เบื้องหลัง"
}

func (s *shellKillSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) == 0 {
		err := errors.New("usage: shell_kill <shell_id>")
		return newToolOutput("shell_kill", "shell_kill", "", time.Now(), false, err), err
	}
	return s.ExecuteTool(ctx, map[string]any{"shell_id": args[0]})
}

func (s *shellKillSkill) ExecuteTool(_ context.Context, args map[string]any) (Output, error) {
	start := time.Now()
	id, _ := args["shell_id"].(string)
	id = strings.TrimSpace(id)
	command := "shell_kill " + id
	if id == "" {
		err := errors.New("shell_id is required")
		return newToolOutput("shell_kill", "shell_kill", "", start, false, err), err
	}
	job, ok := s.shells.get(id)
	if !ok {
		err := fmt.Errorf("no background command %q — %s", id, describeRunning(s.shells))
		return newToolOutput("shell_kill", command, "", start, false, err), err
	}
	if done, _, _, elapsed := job.status(); done {
		return newToolOutput("shell_kill", command,
			fmt.Sprintf("[%s] had already finished, after %s", id, elapsed.Round(time.Second)), start, false, nil), nil
	}
	job.mu.Lock()
	job.killed = true
	job.mu.Unlock()
	// cancel is what proc.KillOnCancel watches, so this takes the whole process
	// tree — npm and the node it spawned, not just the shell in front of them.
	job.cancel()
	return newToolOutput("shell_kill", command, "["+id+"] killed: "+job.command, start, false, nil), nil
}

func describeRunning(shells *backgroundShells) string {
	ids := shells.running()
	if len(ids) == 0 {
		return "nothing is running in the background"
	}
	return "currently running: " + strings.Join(ids, ", ")
}
