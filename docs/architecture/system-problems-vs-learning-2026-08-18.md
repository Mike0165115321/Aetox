# ปัญหาของระบบ ไม่ใช่บทเรียน — Two Queues, Not One

> **Date:** 2026-08-18
> **Status:** **Implemented** 2026-08-18, approved as written ([DECISIONS §136](../DECISIONS.md)). Every section below describes shipped code; the two items under "What this does not decide" are still not decided.
> **Scope:** where `summarizeFailures` sends its clusters — [desktop/summarize.go](../../desktop/summarize.go), [desktop/pending.go](../../desktop/pending.go), the `pending_changes` schema in [desktop/db.go](../../desktop/db.go), and the learning room in [Settings.svelte](../../desktop/frontend/src/lib/Settings.svelte).
> **Does not revisit:** §104's three piles, which are settled. This builds the room pile one never got, and stops pile three sleeping in pile two's bed.

## Why this exists

Owner, 18 ส.ค., looking at a card in "รอคุณตัดสิน" proposing to remember that Tesseract is not installed: *"ไปดูตรงนี้มันจะจำข้อผิดพลาดทำไม"* — and then, once the mechanism was traced: *"ควรจะมีแยก สำหรับเรียนรู้และ และปัญหาของระบบสิ"*.

The queue on that machine, all 22 cards it has ever held:

| Writer | Cards | Approved | Rejected | What they say |
|---|---|---|---|---|
| `summarizer` — automatic, after every turn | 17 | 6 | 10 | "เครื่องมือ X เคยล้มซ้ำ ๆ" — all seventeen |
| `memory` tool — the agent, when it notices something | 5 | 3 | 2 | Windows/shell, เครื่องไม่มี Excel, ผู้ใช้เป็นนักพัฒนา, branch convention |

Five of the summarizer's six approvals were `exit status 1/2/7/124/255`, later judged garbage and filtered at the source by `turn.ErrorFromProgram`. One card in seventeen was a real lesson (`#3`, "this command builds a path while it runs") — and when the same cluster came back as `#9`, it was rejected.

**77% of the learning queue is written by a source that has produced one usable lesson out of seventeen.**

The rule this breaks is already written down, in the file that would have to break it. From `openIssueForm`'s header comment in Settings.svelte:

> Deliberately NOT the learning loop's door. That queue is for lessons about this user and this machine; "Aetox is broken" and "I wish Aetox did X" are facts about the product, and the two kinds of report must never share a path.

That is the door a **human** opens. The door the **machine** opens — the summarizer, running unattended after every turn — pours straight into the queue that comment refuses to touch.

## What is actually wrong

`summarizeFailures` has exactly one input: `tool_runs WHERE ok = 0`. A channel fed only by failures cannot emit anything but a problem. Every fix applied to it so far — exit codes (9 ส.ค.), the bare "ไม่สำเร็จ" (12 ส.ค.), state reports (13 ส.ค.), missing programs (18 ส.ค.) — makes it emit **fewer** problems. None of them can make it emit a lesson about the user, because there is no lesson about the user anywhere in its input.

It is wired to a queue whose own copy promises the opposite: *"อนุมัติแล้วจะเข้าไปอยู่ในความจำ และมีผลตั้งแต่แชทถัดไป"*.

## The design

### 1. The destination is a column that already exists

`pending_changes.kind` is `'memory'` on every row ever written. The summarizer's insert becomes `'issue'`.

That buys the whole mechanism unchanged: rows are never deleted, the dedup key is the body text, and a thing dismissed once never comes back. Those three properties were built for the memory queue and are exactly what a problem list needs.

**The body must stay count-free.** It is the dedup key, and the existing comment says why: a body that grew with the cluster would be re-proposed every turn forever. Counts and timestamps go in `reason` and the UI, as they do today.

**The body copy changes.** Today it ends `— เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก`, which is an instruction to the agent. A problem report contains no lesson:

    เครื่องมือ %s ล้มซ้ำด้วยเหตุเดียวกัน: "%s"

### 2. Two cards, two verbs, and they are not the same question

| | memory card | issue card |
|---|---|---|
| asks | จำไว้ไหม | แจ้งนักพัฒนาไหม |
| yes | **อนุมัติ** → writes a line into a .md file | **แจ้งปัญหานี้** → opens a prefilled GitHub form; the app writes nothing and sends nothing |
| no | **ไม่เอา** → `rejected` | **ไม่เป็นไร** → `rejected` |
| after yes | `approved` | `reported` — a new state value, because "approved" would be a lie: nothing was applied |

### 3. The page

A new Settings room next to `learning`, same nav shape (`id`, `label`, `icon`, `terms`). Markup borrowed wholesale from the learning room's `.settings-card` / `.learn-row` / `.learn-actions` — the structure is right and already styled; nothing new is invented to look at.

Copy (th; en and zh follow):

| key | th |
|---|---|
| `settings.issues` | ปัญหาของระบบ |
| `settings.issuesDesc` | สิ่งที่ล้มซ้ำ ๆ ด้วยเหตุเดียวกัน หน้านี้ไม่ใช่ความจำ ไม่มีอะไรถูกจดจากตรงนี้ |
| `settings.issuesCount` | เกิด {n} ครั้ง ล่าสุด {when} |
| `settings.issuesReport` | แจ้งปัญหานี้ |
| `settings.issuesDismiss` | ไม่เป็นไร |
| `settings.issuesReported` | แจ้งไปแล้ว |
| `settings.issuesEmpty` | ยังไม่มีอะไรล้มซ้ำ |

### 4. The report button is the door that already exists

`openIssueForm('problem')` builds the URL with version, OS, and `RecentDebugLog`, then hands it to the user's browser. The card's button calls the **same function** with the cluster attached to the body — not a second URL builder, not a second privacy story. The last reader before anything leaves the machine stays the person it belongs to.

### 5. The migration is not optional

`UPDATE pending_changes SET kind = 'issue' WHERE source = 'summarizer'`, as a numbered migration alongside the others in db.go.

Without it the dedup key changes shape and all sixteen already-decided summarizer rows resurface on the new page on day one, asking again about outages from 12 ส.ค. With it, the owner's existing decisions carry over and the page opens with exactly one item: `#22`.

## The seams

- **The badge.** `PendingLearnedCount` counts every pending row. Unchanged, it would count problems as things waiting to be remembered — the badge would keep lying, just in a new way. It needs `AND kind = 'memory'`, and so does `ListPendingChanges`.
- **`ApprovePendingChange` needs no new case.** Its `switch c.Kind` already returns *"this build does not know how to apply a %q change"* for anything but `memory`. An issue row can never be written into a memory file by accident. Leave it exactly as it is.
- **The learning switch.** "ให้ Aetox เรียนรู้จากงานที่ทำ" gates `summarizeFailures` today. If problems are not learning, turning learning off should arguably not blind the problems page. **Owner's call** — the honest reading is that clustering failures is not learning, so the page keeps working; the counter-argument is that one switch the user already understands beats two.
- **Pile three has no home and gets none here.** `edit: find text not found` three times is neither an Aetox bug nor a fact about the user — it is the agent being clumsy, and layer 3 (prompt scoring) is where it belongs, unbuilt. Until then it lands on the problems page, where the user judges whether it is worth reporting. A filter guessing on their behalf would be the fourth instance of the mistake this whole document is about.

## What this does not decide

- **What fills the learning queue instead.** The queue goes quiet, fed only by the agent's own `memory` tool (3 of 5 approved — the good half). The sources agreed on 13 ส.ค. — user corrections mid-task, preferences said in passing, fail→success pairs, recurring requests as STARTERS.md cards — remain design, not code.
- **Failures that cannot be classified at all.** A tool reporting through its `Output` instead of returning an error reaches `tool_runs` with `error_kind = ''` no matter what its author knows, because `classifyToolError` reads the error *value* ([DECISIONS §104](../DECISIONS.md)). `n8n_server_start`'s 90-second timeout and `task`'s "MCP not connected yet" are both weather and both unmarkable. They would land on the problems page — which is less wrong than landing in memory, but still not right.
