---
name: pr-workflow
description: How to open a pull request that gets reviewed, and how to review one properly. Use when opening, describing, reviewing, updating or merging a PR.
---

# Pull requests

## Before you open one

**Read your own diff first.** Not the summary of what you intended — the diff.
Half of everything a reviewer would catch is visible there: a debug print left
in, a file added by accident, a change that touched more than the job asked for.

**Check the house style.** Look at the last handful of merged pull requests in
this repository. They tell you the commit message convention, whether the
project squashes, whether descriptions are prose or a template, and how big a
normal change is here. Match that.

**One pull request, one reason.** A change that needs "and" in its title is two
pull requests, and it will sit unreviewed for a week because nobody has a clear
hour. Splitting it is nearly always faster in wall-clock time than defending it.

## The description

The reviewer's questions, in the order they ask them:

1. **What does this do** — one sentence a non-author understands.
2. **Why** — the bug, the issue number, the thing that was slow. If an issue
   exists, `Closes #123` links and closes it on merge.
3. **What to look at** — where the risk is, and what you are unsure of. A
   reviewer told "the retry logic in `client.go` is the part I'd check" reviews
   better than one handed twelve files and no map.
4. **How it was verified** — the test that now passes, or the steps you ran.
   "Tested locally" is not this.

Leave out the file-by-file narration. The diff already says it, and the
reviewer is reading both.

**Open it as a draft** if the work continues. A draft says "not yet" without
anyone having to ask.

## Reviewing one

Look for these, roughly in this order — the order is what makes a review useful
rather than exhaustive:

- **Does it do what it says?** The commonest defect is a change that works and
  is not the change described.
- **What happens on the unhappy path** — the error not handled, the empty list,
  the nil, the second concurrent call, the request that times out halfway.
- **Anything irreversible or outward-facing** — a migration, a deletion, a
  default that changes, an endpoint that starts returning something new to
  someone who did not ask for it.
- **Secrets, and what is now logged.** A token in a log line is a leak with a
  long tail.
- **Is it tested where it can break**, rather than tested where it is easy.

Then say which comments are blocking and which are not. A review of nine
suggestions with no indication of weight reads as "rewrite it", and that is
usually not what was meant.

Be specific and be about the code. "This will panic when `rows` is empty" is a
review. "This feels off" is a delay.

## Answering a review

Address every comment — a fix, or a reason. A thread left silent is one the
reviewer has to chase, and chasing is what makes review slow enough that people
stop doing it. When you disagree, say why once and let them decide; you are
not the one merging.

Push fixes as new commits rather than force-pushing while the review is live,
so the reviewer can read what changed since they looked. Tidy the history when
it is approved, if the project wants that.

## Merging

**Merging is somebody's decision, and by default it is not yours.** Prepare it,
say what you are about to merge and into which branch, and let the caller say
go. See the line in your own instructions.

When you do merge, follow the repository:

- **Squash** — the common default. One commit per pull request, and the history
  reads as a list of changes. Fix the squash message; the auto-generated one is
  a list of "wip" commits.
- **Merge commit** — keeps every commit, and the branch shape with it. Used
  where the individual commits are meant to be bisectable.
- **Rebase** — linear history, no merge commit, and it rewrites hashes.

Afterwards: delete the branch if the repository does not do it automatically,
check the issue actually closed, and — if this deploys on merge — watch the
first run rather than walking away from it.
