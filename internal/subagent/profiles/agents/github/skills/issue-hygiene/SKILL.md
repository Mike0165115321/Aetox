---
name: issue-hygiene
description: How to write an issue somebody can act on, how to triage a pile of them, and the label vocabulary that stays useful. Use when filing, triaging, labelling, closing or linking issues.
---

# Issues

## Writing one

An issue is a request for someone's time. Earn it in the first three lines.

**Title**: what is wrong, not that something is wrong. "Export writes an empty
file when the sheet has one row" — searchable, and a maintainer can guess the
cause from it. "Bug in export" costs everyone a round trip.

**Body**, for a bug — four things and nothing else is required:

1. What you did (the smallest version that still does it)
2. What happened
3. What you expected
4. Version, OS, and how it was installed

Attach the error text as text, not a screenshot of text — it is what someone
will paste into a search. A screenshot is right for a layout problem and wrong
for a stack trace.

**Body**, for a feature — the problem before the solution. "I need a `--json`
flag" hides the actual need, and there may be a better answer than the flag.
Say what you were trying to do and where you got stuck.

**Search before filing.** Include closed issues: the answer is often in one
closed six months ago, and a duplicate costs a maintainer the same time as a
new report.

## Triaging a pile

Answer three questions per issue, in this order, and move on:

1. **Is it a duplicate?** Link to the original and close. Say which one — a
   close with no link reads as a dismissal.
2. **Is it actionable?** If it lacks the reproduction, ask for exactly the
   missing piece and label it as waiting on the reporter. Do not ask for
   everything again; ask for the one thing.
3. **Is it a bug, a feature, or a question?** A question answered is closed. A
   question that reveals the docs were wrong is a documentation issue, and that
   relabelling is where most of the value in triage actually is.

Then it needs one label from each of two axes and no more: **what kind** (bug /
feature / docs / question) and **what it takes** (good first issue / needs
design / blocked). Everything else — priority ladders, area labels, versions —
is worth adding only when a real person is using it to decide what to do next.
A label vocabulary nobody filters by is a taxonomy nobody maintains.

## Closing

Close with a reason and a link: the commit that fixed it, the pull request, the
duplicate, or a sentence about why it is out of scope. A close with no words is
read as being ignored, and the person will file it again.

`Closes #123` in a pull request description closes it on merge and links the
two, which is better than closing by hand — the link survives and tells the next
person where the fix is.

**Not planned** is a real and kind outcome. An issue left open for two years
because closing it felt rude is not kinder than saying so; it just wastes the
attention of everyone who scans the list afterwards.

## Linking to the work

- Reference an issue from a commit or pull request by number and it appears in
  the issue's timeline.
- A tracking issue that lists sub-issues is worth it for work that spans a
  month, and a milestone is the right tool for "which of these is in 1.4".
- Keep the discussion in the issue, and the review on the pull request. When
  they split, the decision ends up in whichever one the next person did not
  open.
