---
name: ci-triage
description: How to find out why a GitHub Actions run failed, and how to tell a real failure from a flake or an infrastructure problem. Use when a check is red, a workflow is stuck, or a run needs re-running.
---

# When a check is red

## Find the actual error, not the first red thing

A failed run reports the *job* that failed; the cause is one line inside one
step of it. Work inward:

1. List the runs for the branch or pull request, and take the failing one.
2. Find which job failed. A matrix fans out — `test (ubuntu, 1.22)` failing
   while `test (macos, 1.22)` passes is already the answer to half the question.
3. Get that job's log and read from the **bottom**. The last lines are the
   runner's own summary; the error is usually just above it.

The trap: the first red line in a log is often not the cause. A compile error
produces a cascade, and the first message is where the compiler noticed rather
than where the problem is. Find the *earliest* line that is an error rather
than a consequence.

**Logs get truncated.** If the log you were handed is cut, say so and say which
part you have. A diagnosis from half a log, presented as a diagnosis, is the
thing this whole skill exists to prevent.

## Which of the four is it

Almost every red check is one of these, and they need different answers:

**A real failure** — the code is wrong, or a test correctly caught something.
The error names a file and a line in the diff. Fix it.

**A flake** — a test that fails on timing, ordering, a port, a shared fixture,
or a clock. Signs: it passes on re-run with no change; the failure mentions a
timeout, a race, a random port, or a date; the same test has failed before on
unrelated pull requests. Re-running is a legitimate move, and re-running
*without saying you did* is not — a flake that nobody names is one that stays.

**Environment, not code** — the runner could not fetch a dependency, a registry
was down, a rate limit was hit, an action version disappeared, the disk filled.
The error is outside the repository's own code. Not the author's problem, and
saying so quickly saves them an hour of looking at their diff.

**Configuration** — a missing secret, a permission the workflow token does not
have, a path that does not exist on this runner. Common right after a change to
the workflow, or on a pull request from a fork, where secrets are deliberately
not available. If it fails only for forks, that is what it is.

## Stuck rather than failed

A queued run that never starts is usually a concurrency group holding it, a
missing runner label nothing matches, or a required approval on a first-time
contributor's pull request. Say which — "waiting for approval" and "no runner
matches `self-hosted-gpu`" look identical from the outside and are hours apart
in what to do about it.

## Re-running

Re-run *failed jobs* rather than the whole run when the passing jobs are
expensive and unaffected. Re-run everything when the workflow file itself
changed, or when you are checking whether something is a flake.

Two re-runs of the same job with no change between them is the limit. A third
is not diagnosis, it is hope, and the answer is in the log you have not finished
reading.

## What to report back

Say the four things the caller cannot see:

- **Which job**, on which matrix leg.
- **The error line itself**, quoted. Not paraphrased — the exact text is what
  they will search for.
- **Which of the four kinds** it is, and why you think so.
- **Whether it is related to this change at all.** A check that was already red
  on the default branch before the pull request existed is the single most
  useful thing to notice, and the easiest to miss: check the branch's own last
  run before blaming the diff.
