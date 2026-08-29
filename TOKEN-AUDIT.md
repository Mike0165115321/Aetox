# ตรวจโทเค็นที่จ่ายแล้วไม่ได้อะไร — Reading the History We Already Keep

> **Date:** 2026-08-20
> **Status:** **Implemented** 2026-08-20. [cmd/tokenaudit](cmd/tokenaudit/main.go) runs today and every number below came out of it.
> **Scope:** reading `tool_runs` and `token_usage` in `aetox.db`. No schema change, no new recording, no new dependency.
> **Does not revisit:** what a turn costs. That is the usage page ([desktop/usage.go](desktop/usage.go)) and stays there.

## What this practice is called

It is **telemetry** that was already being recorded, read as an **audit**.

The two halves are worth naming apart, because Aetox finished the first one months ago and never started the second:

- **Instrumentation** — writing down what happened while it happens. `token_usage` (schema v1) records one row per model call; `tool_runs` (schema v2) records one row per tool call, with `output_bytes` and `output_sha256`.
- **Audit** — going back over that record with a question the recorder was not built to answer. Nobody logs "this was wasted"; you find waste by asking the log a question it happens to be able to answer.

The specific question here is **token accounting**: not what was spent, but how much of the spend bought nothing. In the wider practice this is the same move as a cache-miss analysis or a dedup report — the useful part is always a comparison the raw counter cannot make on its own.

The reason it belongs in a program and not in a session: a number read once is a fact about a Tuesday. A number that can be read again is a baseline, and a baseline is the only thing that tells you whether a change helped.

## Why the usage page could not answer this

`token_usage` counts tokens **after** they were tokens. It knows a call carried 24,000 prompt tokens; it cannot know that 4,000 of them were the same skill document the model had already read twice, because by then the document is not a document any more — it is a number.

`tool_runs` keeps the sha256 of every output. That single column is the whole audit: identical sha, same session, more than once, means the conversation paid for the same bytes twice.

So the split is not organisational, it is structural. Spend lives in one table and waste lives in the other, and that is why they are two programs rather than one page with more tabs.

## The six sections and how to read them

> Four when written; **CACHE BREAKS** joined 2026-08-28 as ระดับ 1 of the Aider
> adoption plan ([docs/aider-study/EXECUTION.md](docs/aider-study/EXECUTION.md)),
> and **TRIAL** on 2026-08-30 — the seven-day follow-up on that plan's mechanics
> (repo_map, symbol/references, rename, the read >=50KB pass line), baselines
> inlined so one command answers the trial.

### REPEAT WASTE

Byte-identical tool output re-sent into one session. Keyed on `(session_id, tool, output_sha256)` — the same file read in two different sessions is two honest reads and is not counted.

**A high percentage is a tool problem, not a model problem.** A model re-reads a document when it is not sure it got all of it, or when it has forgotten it is already in the conversation. The fix belongs at the tool: return a pointer to what was already sent. `endMarker` in [internal/skill/progressive.go](internal/skill/progressive.go) is the first instance of that fix, added the same day, for exactly this failure in `skill_view`.

Note that `web_fetch` already caches ([web_fetch.go](internal/skill/web_fetch.go)) and still shows up: that cache saves the network round trip, not the context. The bytes land again either way.

### DELEGATION CONTAINMENT

What share of tool output was paid for in a delegate's context rather than the chat's.

This is the number the whole sub-agent mechanism exists to move, and it had never been measured. A byte a delegate produced is paid once and dies with the delegate; a byte the chat produced rides every later round of a conversation the user is still sitting in.

**Read `bytes/call`, not just the total.** A worker with high bytes per call is one whose work genuinely does not belong in a chat window.

### CACHE HEALTH

Fresh input tokens per call, by model. Fresh means `prompt_tokens - cached_prompt_tokens`.

Ranked by fresh-per-call rather than by cache percentage on purpose: a big prompt at 90% can cost more real tokens per round than a small one at 60%, and it is the fresh tokens that are charged in full.

Models whose provider reports no cache accounting at all are excluded. Counting their whole prompt as fresh would rank a provider that merely does not tell us below one that does.

### CACHE BREAKS

The moments CACHE HEALTH's averages are made of: calls where `cached_prompt_tokens` fell at least 2,000 tokens AND 5% below the previous call in the same session. CACHE HEALTH says a model averaged 77%; this says whether that is a steady dynamic tail (fix the prompt layout) or a good cache repeatedly broken (fix whatever moves).

Each break is classified as far as `token_usage` can honestly see: **model switch** (the previous row ran a different model — the break was bought knowingly), **idle gap** (same model, >5 minutes quiet — the shortest provider TTL in play; this is "expired", not "invalidated"), and **prefix changed** — everything else, meaning something in the sent prefix moved mid-conversation. WHAT moved needs a prefix hash per call, an app change deliberately deferred until this SQL-only view proves insufficient.

First run (2026-08-28, 7 days): 173 breaks, 4.69M tokens dropped — `qwen3.7-plus` alone lost 2.49M to "prefix changed" across 96 breaks, which reads that model's 77% cache as breakage, not as an incompressible tail.

### OUTPUT VOLUME

Bytes each tool put into a context. The plain answer to "what fills a conversation", which is reliably not what it feels like.

## Baseline, 2026-08-20

Recorded so a later run has something to be compared against. All time, on the owner's machine, 3,275 model calls and 2,753 tool calls.

**Repeat waste — 10.1% of all tool output bytes** (465,813 of 4,613,934). By tool, where any repeat exists:

| tool | bytes | repeated | % |
|---|---|---|---|
| browser | 718,739 | 176,010 | 24.5% |
| read | 1,003,980 | 125,255 | 12.5% |
| github_read_file | 221,274 | 99,316 | **44.9%** |
| skill_view | 257,802 | 34,047 | 13.2% |
| grep | 471,721 | 31,185 | 6.6% |

**Delegation containment — 39.9%** of tool output kept out of the chat (1,595,709 of 4,000,410 bytes over the last seven days). `research` is the densest worker at 18,807 bytes per call against the chat's own 1,997, which is the mechanism doing exactly what it was built for.

**Cache health:**

| model | calls | prompt | fresh | cache% | fresh/call |
|---|---|---|---|---|---|
| gpt-5.6-luna | 1,101 | 24,606,780 | **13,201,468** | 46.4% | **11,990** |
| deepseek-v4-flash | 1,637 | 35,904,356 | 1,847,652 | 94.9% | 1,128 |
| gpt-5.6-terra | 197 | 3,864,197 | 666,245 | 82.8% | 3,381 |
| deepseek-v4-pro | 273 | 16,045,127 | 337,223 | 97.9% | 1,235 |
| gpt-5.4-mini | 43 | 366,523 | 202,427 | 44.8% | 4,707 |

**One model carried 80% of every fresh input token ever spent, from 34% of the calls.** Not because it called more tools — because its cache hit 46% where another hit 95%. There is no change to the tool loop, the prompt, or the sub-agents that reaches a number that size.

This was checked for a cause on our side and there is not one: nothing in [internal/prompt](internal/prompt) varies per request, so the cacheable prefix is stable. It is the provider's caching behaviour.

## What this deliberately does not do

- **No pricing.** The usage page prices calls and owns that. A second cost total built from a different table is two answers to one question.
- **No writing.** Opened `mode=ro`. A tool that measures history must not be able to change it.
- **Not shipped.** [build.ps1](build.ps1) builds `./cmd/aetox` only. This is a tool for reading this machine's own history, like `cmd/relsign` is a tool for signing a release.
- **No cross-session repeat detection.** Reading the same file in two sessions is two real reads. Widening the key to catch them would turn honest work into a waste statistic.

## What it found that we then acted on

Nothing in the loop. The two changes made the same day — raising `maxParallelToolCalls` from four to five, and telling the `explore` helper to batch its searches — buy **wall clock, not tokens**: on 2026-08-20 the session's prompt was 96.4% cached, so the round trips they remove were already the cheap part. That is worth knowing before the next round of loop tuning, and it is why this document exists rather than a fifth optimisation.

The two things worth acting on are both above the loop: the model a session runs on, and tools that hand back bytes the conversation already holds.
