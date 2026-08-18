# Aetox — The Decision Log (§10–§84)

> Moved verbatim from [ARCHITECTURE.md](../ARCHITECTURE.md) on 2026-08-05 to keep the hub readable. **The section numbers are permanent addresses** — code comments cite "ARCHITECTURE.md §NN" in hundreds of places, so nothing here is ever renumbered, shipped sections are never edited (append an amendment instead), and new decisions continue the numbering at the bottom of this file, each also getting its one-line row in the hub's index.
>
> The hub ([ARCHITECTURE.md](../ARCHITECTURE.md)) keeps the system maps and the index; the product picture is [COMPANY.md](../COMPANY.md). Internal "§NN" references throughout resolve within this file.

## 10. Decision — Agent Orchestrator Layer (Proposed, approved 2026-07-21)

Discussed with the project owner: CLI, Desktop, and future UI front ends currently each embed the engine as a Go library in their own process (§2, §4) — there is no gateway process and no shared live agent/session state across front ends. The owner asked whether a background layer for multi-agent orchestration should exist, and whether front ends should share one running agent.

**Decision:** keep the embedded-library model (no new standalone gateway process — rejected as premature; no front end today needs to observe another front end's live session). Add a new orchestrator responsible for multi-agent lifecycle, built so it can be wrapped by local RPC later without a rewrite if that need becomes concrete.

```mermaid
flowchart TB
    subgraph today["Today — proposed, not built"]
        CLIp["CLI process"] --> Orch1["orchestrator\n(in-process)"]
        Deskp["Desktop process"] --> Orch2["orchestrator\n(in-process)"]
        Orch1 --> A1["cognitive.Agent ×N"]
        Orch2 --> A2["cognitive.Agent ×N"]
    end
```

- **What it is:** a new package, [internal/orchestrator/orchestrator.go](../internal/orchestrator/orchestrator.go) (moves to `engine/orchestrator` once the module split migrates), that spawns/tracks/stops multiple `cognitive.Agent` instances per process via `Spawn`/`Get`/`Stop`/`List`, replacing the current one-`Agent`-per-front-end model. Distinct from `internal/app` (§6.1), which wires exactly one agent today. Not yet wired into `cmd/aetox` or `desktop/app.go` — those still construct a single `cognitive.Agent` directly via `bootstrapFromConfig`.
- **Interface constraint (the point of the decision):** operates on agent **IDs and a serializable `Info` snapshot** (`ID`, `Model`, `CreatedAt`), not Go closures or channels owned by a specific front end — so a future local-RPC wrapper (front end as thin client, Desktop or a dedicated process as host) can sit on top without redesigning the orchestrator itself.
- **Explicitly deferred, not built:** any IPC/RPC transport, any "Desktop as host for CLI" wiring, any shared-state protocol, and wiring into the existing front ends. Build only when a front end has a concrete requirement to observe or drive another front end's session, or to run more than one agent concurrently.
- **Status:** `Direct` (package exists, `go test ./internal/orchestrator/...` passes) but **unused** — no caller yet. Not to be confused with existing `internal/cognitive.Agent` (single-agent, already built and wired) or the roadmap "MAIN + sub-agent" description in `AETOX.md`, which this scaffolds toward but does not yet implement (no sub-agent spawning logic, no profile/tool-filtering, no MAIN-vs-sub-agent role distinction).

**Naming — settled, don't rename without checking this first:**

| Rejected name | Why it was rejected |
|---|---|
| `gateway` | Implies a network/process boundary (something external connects *into*) — there is none yet (§10 decision explicitly defers IPC/RPC). If a local-RPC layer is ever built in front of this package, `gateway` belongs to *that* layer, not to the in-process tracker. |
| `router` | Already claimed in `README.md`'s architecture diagram — "Multi-Provider Orchestration: **Router** \| Comparator \| Consensus" means routing a request to which *provider/model*, an unrelated concept. Reusing it for agent lifecycle tracking would collide with that already-planned component. |

`orchestrator` was kept because (a) it doesn't collide with any name already used in `README.md`/`AETOX.md`, and (b) `AETOX.md`'s own "Multi-Agent Layer" description ("MAIN → spawn sub-agent") already implies exactly this responsibility — lifecycle, not routing or transport.

---

## 11. Decision — Prompt/Context Layer (Proposed, being settled section-by-section, 2026-07-22)

Discussed with the project owner, who asked whether it is time to build the layer that manages system prompts and per-project context files (`AETOX.md`), and requested the design be settled here in ARCHITECTURE.md piece-by-piece rather than approved wholesale. Findings that motivated it (all `Direct`):

- The system prompt is a hardcoded string duplicated in **three places**: `cmd/aetox/main.go:buildSystemPrompt`, `desktop/app.go:buildSystemPrompt` (95% identical, one sentence differs), and a fallback in `internal/cognitive/agent.go:NewAgent`.
- **The desktop `GovernanceLoaded` badge lies.** `desktop/app.go:projectStatus` only `os.Stat`s `Aetox.md` — no code anywhere reads its contents into the prompt. A user who writes rules in `Aetox.md` sees "loaded ✓" while the model never sees a byte of it. The CLI ignores the file entirely.
- There is no user-level (cross-project) instruction file at all.
- Adjacent parts that are **fine and out of scope**: model management (`config.ModelPreference`, already centralized, CLI/desktop share one preference file) and conversation memory/truncation (`internal/memory.Context`).

**Where it sits:** one new engine-side package, `internal/prompt` (moves to `engine/prompt` when the §4 module split migrates). Both front ends call it from their existing bootstrap paths (`cmd/aetox/main.go`, `desktop/app.go:bootstrapFromConfig`), deleting both `buildSystemPrompt` copies. In the 5-layer reader's map this feeds layer 2 (what the model sees) but is a leaf package — no new dependencies, no new process, no front-end code beyond the call site.

**Assembly order (settled 2026-07-22):** four layers concatenated, most-specific last, because models weight later context higher on conflict — so project rules beat personal rules:

| # | Layer | Source | Notes |
|---|---|---|---|
| 1 | Identity | hardcoded in Go, `surface` param (`"cli"`/`"desktop"`) | the only per-surface difference today is one sentence |
| 2 | Environment | sandbox root (+ existing don't-leak-path rule) | |
| 3 | User global | every `*.md` file in `<UserConfigDir>/aetox/identity/` | new capability; cross-project personal rules — a directory, not one file, see 2026-07-23 addendum below |
| 4 | Project | `<root>/AETOX.md`, fallback `<root>/AGENTS.md` | wins on conflict |

Missing files are skipped silently; per-file size cap (~16KB) to keep the prompt bounded. Content format is free markdown — no schema, the model reads it as-is.

**Project file naming (settled 2026-07-22):** `AETOX.md` uppercase — matches this repo's own root file. Falls back to `AGENTS.md` because the ecosystem is converging on it (OpenCode, Codex, Gemini CLI), so any repo that already has one works with Aetox without creating a new file. The desktop badge should switch from stat-ing the file to reporting what the prompt layer actually loaded, making it honest.

**Reload timing (settled 2026-07-22, owner: "ตอนรีสตาร์ท หรือตอนเริ่ม ... หลายๆที่ทำก็แบบนั้น"):** **Option A, bootstrap-only.** The file is read where the agent is created — app start, project switch, model/provider switch. Editing `AETOX.md` mid-session has no effect until one of those happens. Matches convention elsewhere; zero new mechanism (no context-reset seam, no per-turn stat cost). An upgrade path to per-turn mtime-checking (previously sketched as "Option B") is still possible later without an API change, if it's ever needed — not built now (YAGNI).

**Explicitly deferred, not part of this layer:** per-turn dynamic context (git status / open files injected every turn, OpenCode-style), and sub-agent profile files (`.aetox/agents/*.md`) — the latter waits until the orchestrator (§10) has a real caller; per the source-reading note on agents, a profile is just model override + prompt override + permission ruleset (+ `steps`), so it can be layered on top of this package later without redesign.

**Related cleanup (done 2026-07-22):** the tool loop is now unbounded engine-wide (OpenCode-style; brakes = approval layer + ctx cancel — CLI Ctrl+C, desktop Stop button → `App.CancelTurn`). The CLI's leftover `defaultAgentMaxToolCalls = 16` was removed — it was only applied at startup and silently lost on `/model` switch anyway. Details: `docs/architecture/model-control-layer-2026-07-22.md` §3.

**Status:** `Approved & done 2026-07-22.` [internal/prompt](../internal/prompt/README.md) built and wired into both front ends: both copies of `buildSystemPrompt` (`cmd/aetox/main.go`, `desktop/app.go`) deleted, both now call `prompt.Build`/`prompt.BuildWithReport`. `desktop/app.go`'s `projectStatus` no longer stats a hardcoded `Aetox.md` — it calls `prompt.ProjectContextFile`, so the `GovernanceLoaded` badge now reports the same file the prompt layer actually loaded (the "badge lies" gap this section opened with is closed). New: `config.UserGlobalContextPath()` (same `<UserConfigDir>/aetox/*` pattern as `PreferencePath`/`PermissionsPath`). Tests: `internal/prompt/prompt_test.go` (layering, fallback priority, size cap). **Incidental fix while wiring the CLI call site:** the CLI's leftover `defaultAgentMaxToolCalls = 16` (only applied at startup, silently lost on `/model` switch) was removed in the same pass as the "related cleanup" note above — see `docs/architecture/model-control-layer-2026-07-22.md` §3.

**Addendum 2026-07-23 — user global layer became a directory, not one file (owner: "ตัวตน เอไอ หมายถึง พวกไฟล์ root ที่ติดตัว เอไอ ไปตลอดอ่ะ ไฟล์ context.md ไฟล์ สกิล ไฟล์ พวกนี้อ่ะ").** The single `AETOX.md` blob from row 3 above didn't match the owner's mental model — they picture the "AI Identity" layer as a small set of always-attached root files (`context.md`, a skills note, etc.), not one text box. Changed: `config.IdentityDir()` (`<UserConfigDir>/aetox/identity/`) replaces `UserGlobalContextPath` as the layer 3 source; `prompt.BuildWithReport` now folds in every `*.md` file found there (sorted by filename, same per-file 16KB cap), each as its own `Personal instructions — <name>` block, most-specific-last ordering unaffected since this whole layer still sits before the project layer. `Loaded.UserGlobalPath` (single string) became `Loaded.UserGlobalPaths` (`[]string`) — not consumed by any caller outside `internal/prompt` yet, so this was a safe rename. `desktop/app.go` gained `ListIdentityFiles`/`ReadIdentityFile`/`SaveIdentityFile`/`DeleteIdentityFile` (replacing the short-lived single-file `ReadGlobalInstructions`/`SaveGlobalInstructions` pair from the same day), with a one-time migration: the old `AETOX.md` at `DataRoot` is moved into `identity/context.md` the first time the identity directory is touched. `UserGlobalContextPath` itself is kept only as that migration's source path, not for new code. Desktop sidebar's "ตัวตน AI" section is now a small file list (open/create/delete) over a single-file editor — see `desktop/frontend/src/lib/identity.svelte.ts`.

**Addendum 2026-07-24 — identity-layer conventions (settled in discussion with owner):**

- **Suggested identity files (convention, not schema):** `identity.md` (who the AI is), `thinking.md` (thinking discipline), `context.md` (about the user), `skills.md` (always-on notes). Surfaced as one-click templates in the desktop sidebar (`identityTemplates` in `identity.svelte.ts`) shown for any not-yet-created name. The engine still treats every `*.md` in the identity dir identically — **zero magic filenames**; the differentiation is UI nudge only.
- **`thinking.md` is discipline, not steps — and goes to every model.** Owner initially proposed skipping it for models without native reasoning; inverted after discussion: non-reasoning models benefit *most* from prose thinking discipline (that's what CoT prompting is), while step-by-step "think like this" instructions are what provider guidance (DeepSeek R1, OpenAI o-series) warns can degrade native-reasoning models. So the template is values-style (evidence-first, don't guess, say when unsure), safe for all models. Upgrade path if a specific model measurably degrades: per-file frontmatter condition (e.g. `when: reasoning|no-reasoning`), declared in the file — never hardcoded filenames in Go.
- **Guiding rule for what goes where:** habit/style → prompt (identity file); hard prohibition → code (`internal/safety` permission layer, prose can't enforce); thinking *effort* → `think.Level` (already a real API knob via `model.ReasoningConfig`, not prose).
- **Project fallback chain extended:** `AETOX.md` → `AGENTS.md` → `CLAUDE.md` (Claude Code compat — same drop-in motivation as `AGENTS.md`, mirrors OpenCode reading Claude Code's files).
- **Agent profile files (`agents/*.md`) remain deferred** per this section's original deferral — owner confirmed they are a separate topic and will never be merged into `AETOX.md`; format to be settled when `internal/orchestrator` (§10) gets a real caller.

---

## 12. Decision — Per-Module Documentation, Hub-and-Spoke (Proposed 2026-07-22)

Owner proposal: each module owns its documentation inside its own folder (complex modules first), ARCHITECTURE.md stays the central overview/hub, and every module doc links back here (and the hub's §4 module map links out). This section is the survey of how many modules that actually means, so it can be approved with real numbers instead of "น่าจะเยอะ".

**Survey (2026-07-22, non-test LOC):** 19 modules total — 17 `internal/` packages + `desktop/` + `cmd/aetox`. Tiered by what documentation they actually warrant:

| Tier | Rule | Modules (LOC) | Action |
|---|---|---|---|
| **1 — complex, gets a `README.md` in its folder** | >1000 LOC or many files or a real subsystem | `internal/model` (3852, 10 files), `internal/skill` (3391, 18 files), `internal/turn` (3367), `desktop/` (3345 Go + Svelte frontend), `internal/app` (1440), `internal/grammar` (1032 — **single file, zero docs today**), `cmd/aetox` (1010) | **7 READMEs to write.** `desktop/README.md` exists but is Wails template boilerplate — replace, don't append. |
| **2 — medium, hub row + package doc comment** | 300–800 LOC, already covered elsewhere or too simple for a file | `internal/provider` (771), `internal/mcp` (681), `internal/config` (606), `internal/cognitive` (517 — already covered by `docs/architecture/model-control-layer-2026-07-22.md` + ADR 0002), `internal/safety` (481 — same doc), `internal/command` (298) | No new files until one grows. |
| **3 — small, doc comment at top of file only** | <300 LOC, single concern | `internal/audit` (283), `internal/memory` (194), `internal/think` (166), `internal/orchestrator` (131 — §10 covers the design), `internal/plan` (106), `internal/debuglog` (100) | Nothing to do beyond existing comments. |

**Conventions (proposed):**

- A module `README.md` answers: what this package is, its key seams (the 2–3 types/functions everything else hangs off), links to its dated deep-dive docs, and one link back to `ARCHITECTURE.md` — a map, not a novel. Aim under ~80 lines each.
- **Dated deep-dive docs stay where they are** (`docs/architecture/*-2026-*.md`, `docs/adr/`) — they are session records/history, not living module docs. Module READMEs link to them; nothing gets moved, so existing links and git history stay intact.
- The hub embeds the index: §4's module map gains a **Docs** column linking each Tier-1 README (done when the READMEs land, not before — no dead links).
- New rule going forward: a change that meaningfully alters a Tier-1 module updates that module's README in the same commit.

**Suggested write order (follows active work, not size):** `internal/app` + `cmd/aetox` + `desktop/` first (the prompt layer §11 touches exactly these three), then `internal/turn`/`internal/skill`/`internal/model`, `internal/grammar` last.

**Addendum (2026-07-22):** `internal/prompt` was added after this survey (§11). By LOC alone (~110 lines) it would be Tier 3 (doc comment only), but it got a README anyway — a new module that implements an approved Decision section earns one regardless of size, so the design doesn't only live in ARCHITECTURE.md. Tier boundaries above are for *existing, undocumented* modules; a freshly-built module tied to a Decision section is a standing exception, not a tier-4.

**Status:** `Approved & done 2026-07-22.` All 7 Tier-1 READMEs written ([internal/app](../internal/app/README.md), [cmd/aetox](../cmd/aetox/README.md), [internal/turn](../internal/turn/README.md), [internal/skill](../internal/skill/README.md), [internal/model](../internal/model/README.md), [internal/grammar](../internal/grammar/README.md), [desktop](../desktop/README.md) — boilerplate replaced); §4 map carries the Docs column; the update-docs-with-changes rule is recorded in §9. The 5-layer reader's map stays as the top-level reading aid — module READMEs are the finer breakdown beneath it.

---

## 13. Decision — RTK Integration (Proposed, being settled section-by-section, 2026-07-22)

Owner asked to integrate `rtk` (Rust Token Killer — the owner's own CLI output-filtering tool, already used as a Claude Code hook) into Aetox itself. This section also closes a documentation gap the owner flagged in the same conversation: the model-call path (what exactly gets sent to/from the provider each turn) was explained in chat but never written down — closing it here because RTK's insertion point *is* that exact seam.

### 13.1 The call path this plugs into (closes the doc gap)

**Moved to its permanent home:** [docs/architecture/model-control-layer-2026-07-22.md §6](../docs/architecture/model-control-layer-2026-07-22.md) ("The exact call path — what a 'call' to the provider actually contains") — full diagram, `model.Request` field breakdown, and why `memory.Context.enforceLimits` is the only *other* token-budget mechanism. §7 of that same doc explains, with evidence, why the RTK hook is **not** inside `Agent.buildRequest` (a question re-raised and settled in this same 2026-07-22 session): by the time `buildRequest` runs, tool output is already a flat, un-typed message string — the tool name/args needed to pick an `rtk pipe -f <filter>` only exist earlier, at `modelToolReceipt`. A second hook at `buildRequest` would be redundant (re-filtering already-filtered text) and risk corrupting already-correct content by guessing wrong.

### 13.2 What RTK is (evidence from direct inspection, not the owner's description alone)

- Installed at `~/.cargo/bin/rtk`, v0.34.3 — a real, already-installed Rust binary, not vendored into this repo. `Direct`.
- Its own description: "a high-performance CLI proxy designed to filter and summarize system outputs before they reach your LLM context." Subcommands overlap directly with existing Aetox skills: `rtk git {status,diff,log,show}` ↔ `internal/skill/git.go`, `rtk grep` ↔ a grep-style tool, `rtk read --level {minimal,aggressive}` ↔ `internal/skill/read.go`, `rtk ls`/`rtk tree`/`rtk find` ↔ `internal/skill/list.go`/`fs.go`.
- Demonstrated on this repo (`Direct`, ran live): `rtk git status` on a clean tree returned `clean — nothing to commit` (one line) instead of porcelain output.
- Demonstrated risk (`Direct`, ran live): `rtk read internal/prompt/prompt.go --level minimal` **silently dropped every doc comment**, including the package comment and the one on `type Surface`. For a tool whose output the agent might use to *edit* a file afterward, or whose commentary explains a non-obvious constraint, this is a correctness risk, not just a cosmetic change.
- Today has no library/API mode found in `--help` output — integration means shelling out to the `rtk` binary per call, same as Aetox's skills already shell out to `git`/native commands.
- No hook currently installed for this machine's `rtk` (`rtk config` reports "No hook installed"); Aetox integrating it would be independent of whatever Claude Code hook setup exists on this machine.

### 13.3 Corrected design — the actual hook point (found only after tracing the real data path)

The owner's own framing ("สอดตอนที่ระบบสร้าง call ก่อนส่งให้ provider" — insert where the system builds the call, before sending to the provider) pointed at `internal/cognitive/agent.go`'s tool-loop as the first guess. Tracing the actual data (`Direct`, not assumed) showed that's the wrong seam: by the time `cognitive.Agent.RespondWithTools` has a tool's output, it's *already* the JSON-wrapped receipt string from `turn.executeToolCallWithOutcome` → `modelToolReceipt` (`internal/turn/executor.go:661`), which bundles `{"tool", "status", "output", "stderr", ...}`. Piping a JSON envelope through `rtk pipe -f git-status` would just fail to parse. **The real seam is one line earlier, inside `modelToolReceipt` itself** — where `result := output.RawOutput` is still a plain string, before it's wrapped into JSON and before the existing secret-redaction pass (`sanitizeAndTrimOutput`, `internal/turn/result.go:158` — redacts api key/token/password patterns, enforces `summaryLimit`).

Also discovered live (`rtk pipe -f <bogus-name>` lists its own valid filters in the error): `rtk pipe -f <filter>` needs a **named, format-aware filter** — `cargo-test, pytest, go-test, go-build, tsc, vitest, grep, rg, find, fd, git-log, git-diff, git-status, log, mypy, ruff-check, ruff-format, prettier` — it is not a blind text compactor. This confirms the §13.4 read.go concern independently: there is no filter name for "arbitrary file content," so file reads were never a fit for this mechanism at all (they'd need the separate `rtk read --level` mechanism, not attempted here).

### 13.4 Settled design

- **Hook location:** `internal/turn/executor.go:modelToolReceipt`, immediately after `result` is computed from `output.RawOutput`/`output.Content`, **before** `sanitizeAndTrimOutput` — so RTK sees genuine unredacted command output (needed to match its filter formats) but the existing secret-redaction/length-cap safety net still runs unconditionally afterward on whatever comes out, RTK-filtered or not.
- **New package:** [internal/rtk](../internal/rtk/README.md) — `Available()` (cached `exec.LookPath` check), `FilterForTool(name, args)` (tool → filter-name mapping), `Filter(filter, content)` (runs `rtk pipe -f <filter>`, 5s timeout, any failure returns the original content untouched).
- **Optional, never required.** No installer bundling (unlike tesseract's precedent in `docs/architecture/tesseract-ocr-bundling-2026-07-22.md`) — a token-cost optimization doesn't earn download-on-first-use the way an OCR capability did.
- **Approval untouched.** `safety.AssessCommand`/`turn.resolveApproval` see the real command (`git status`) always — RTK only touches the result string, after execution, never what's presented for approval.
- **v1 scope, decided from evidence rather than re-asked:** `git` (status→`git-status`, diff/show→`git-diff`, log→`git-log`) and `shell`. `read.go` excluded (§13.3's mechanism mismatch, plus the demonstrated comment-stripping risk). Every other tool (`write`/`delete`/`list`/`fs`/`echo`/`time`/`help`/`input`/`output`/`github_repo_summary`/`plugin_install`/`image_ocr`) has no mapping and passes through exactly as before.
- **Failure behavior:** any error (missing binary, unknown filter, subprocess failure, empty output) falls back to the original content silently — never surfaced as a tool error, since RTK is strictly an optimization layer, not a capability.

**Status:** `Approved & done 2026-07-22`, refined `2026-07-23` (below). Full repo `go build`/`go vet`/`go test ./...` green throughout. List/fs and a wider filter set are natural v2 candidates once v1 usage confirms real savings — not built now (YAGNI).

### 13.5 Refinement (2026-07-23) — `shell` switched from a guessed prefix-list to `Rewrite`, matching rtk's own OpenCode plugin

Owner's own instruction: check how OpenCode's actual RTK integration is configured before expanding ours, and "เอาแค่คำสั่งที่จำเป็น...อย่ายัดทุกอย่างเข้า" (take only what's necessary, don't stuff everything in). Ran `rtk init -g --opencode --dry-run -v` (`Direct`, live) — it previews the exact plugin rtk would install for OpenCode:

```ts
export const RtkOpenCodePlugin: Plugin = async ({ $ }) => {
  // ...checks `which rtk`...
  return {
    "tool.execute.before": async (input, output) => {
      // ...only for tool === "bash"/"shell"...
      const result = await $`rtk rewrite ${command}`.quiet().nothrow()
      // ...substitutes args.command with the rewritten result if different...
    },
  }
}
```

The real plugin is a **single hook, single delegated call** (`rtk rewrite`) — no hand-maintained command-to-filter table at all. Aetox's v1 `shell` case (a hardcoded `strings.Contains`/`HasPrefix` guess-list: `go test`, `go build`, `grep`, `rg`, `find`, `fd`) was strictly inferior: incomplete (misses anything not on the list, e.g. `cat`→`rtk read`, confirmed live), and would silently drift out of date as rtk's own registry grows.

**Change made:** `shell.go` now calls `rtk.Rewrite(commandLine)` directly (after approval, before `exec.Command`) — if rtk has an equivalent, Aetox runs *that* instead of the original command line; same underlying side effects (rtk actually runs the real command), just captured pre-compacted. `FilterForTool`'s `shell` case was deleted entirely (dead code once `Rewrite` covers it — keeping both would have double-processed shell output). `git` was deliberately **left unchanged** — it already parses its exact subcommand (`internal/skill/git.go`'s `allowedGitReadActions`), so reconstructing a command string just to ask `Rewrite` would be strictly more roundabout than the existing direct name→filter mapping; not "everything," only where it actually simplifies something.

**Bug found and fixed during this change:** `rtk rewrite`'s own `--help` claims "exits 0 and prints the rewritten command if supported" — a live check showed a *successful* rewrite (`git status` → `rtk git status`) exiting **3**, not 0. `internal/rtk.Rewrite` originally used `exec.Cmd.Output()` (which errors on any non-zero exit, discarding stdout) and so treated every successful call as a failure. Fixed by capturing stdout unconditionally and judging success by content, not exit code — same resilient pattern `Filter` already used correctly. Caught by `TestRewriteRealBinary`, a live integration test against the real installed binary (same pattern as `TestFilterRealBinary`) — this is exactly the kind of gap a mocked test would have missed.

Updated: [internal/rtk/README.md](../internal/rtk/README.md) (seam table split by which mechanism each tool uses and why), `rtk_test.go` (shell prefix-list tests replaced with `Rewrite` tests, including the live one that caught the exit-code bug). No change needed to `internal/turn/executor.go` beyond what §13.4 already describes — `FilterForTool` shrinking to git-only doesn't change its call site.

**Test coverage gap closed (2026-07-23):** `internal/rtk`'s own 9 tests cover the package's logic in isolation, but neither integration seam had a direct test — added two: `internal/turn/rtk_hook_test.go` (`modelToolReceipt` calling into `rtk.FilterForTool`/`Filter` for real, plus a regression check that an unmapped tool like `read` passes through byte-for-byte unfiltered) and a new case in `internal/skill/shell_test.go` (`shellSkill.Execute` actually substitutes the rtk-rewritten command — asserted by the *output shape* difference: rtk's compact `git status` reads `clean — nothing to commit`, plain git's doesn't, so passing proves substitution happened, not just that `Rewrite()` works standalone — and confirms the recorded `Command` field stays the original, never the rtk one). All three tests skip gracefully when `rtk` isn't installed, same pattern as `internal/rtk`'s own live tests. Full repo `go build`/`go vet`/`go test ./...` green.

### 13.6 Runtime auto-install (2026-07-23) — RTK downloads itself once if missing, no installer changes

Owner's direction: Aetox is meant to become a program other people download and run, so should `rtk` be part of the installer? Asked, then explicitly chose **runtime fallback instead of touching the NSIS installer** — see the three options put to the owner (installer bundling / runtime fallback / defer) and the choice made.

**Evidence gathered before building anything (`Direct`, all live):**
- `rtk` on this machine was installed via `cargo install --git https://github.com/rtk-ai/rtk` — a real, specific, identifiable public repo, not the owner's private/undisclosed tool.
- `gh api repos/rtk-ai/rtk` confirms: public, **72,590 stars**, license **Apache 2.0**, not archived — permissively redistributable, no licensing blocker for bundling or auto-downloading it.
- Its latest release (`v0.43.0`) publishes a portable, checksummed asset per platform: `rtk-x86_64-pc-windows-msvc.zip`, `rtk-{x86_64,aarch64}-apple-darwin.tar.gz`, `rtk-x86_64-unknown-linux-musl.tar.gz`, `rtk-aarch64-unknown-linux-gnu.tar.gz` — each with a `sha256:` digest **published directly in the GitHub release API response** (no separately-computed/pinned hash needed, unlike the Tesseract installer's case where the digest field was `null`).
- Downloaded and inspected both archive shapes directly: the Windows zip contains a single bare `rtk.exe` at its root (no subfolder); the Linux tar.gz contains a single bare `rtk` file, already executable. Both confirmed by actually unzipping/untarring them, not assumed from convention.

**Design, mirroring the Tesseract precedent's judgment (`docs/architecture/tesseract-ocr-bundling-2026-07-22.md` §3 — macOS's Homebrew auto-install, "no elevation needed, so one automatic attempt is safe") but applicable on every OS here** since rtk ships a portable archive with no installer wizard to script around, unlike Tesseract's Windows story which needed a real install-time hook:

- New file [internal/rtk/install.go](../internal/rtk/install.go): `resolve()` (package-private, `sync.Once`-cached) tries, in order: (1) PATH, (2) a previously-downloaded copy at `<UserConfigDir>/aetox/bin/rtk[.exe]`, (3) one download-verify-extract attempt from the GitHub release matching `GOOS`/`GOARCH`. `Available()` now calls this instead of a bare `exec.LookPath`; `Rewrite`/`Filter` invoke the resolved path instead of a literal `"rtk"` string, so a downloaded-but-not-on-PATH binary actually gets used.
- Any failure at any step (offline, unsupported OS/arch, checksum mismatch, GitHub rate-limited) leaves `Available()` returning `false` — identical fallback behavior to rtk never having been installed at all. This is strictly additive to §13's original "optional, never required" principle, not a change to it.
- **Explicitly not done:** no change to `project.nsi`/the NSIS installer. This is the owner's deliberate scope boundary for this round — an install-time mechanism remains a live option later if the runtime fallback proves insufficient (e.g. if GitHub API rate-limiting or offline-install scenarios turn out to matter in practice), same "revisit later if needed" posture the Tesseract doc already takes for macOS/Linux packaging.

**Testing:** `internal/rtk/install_test.go` — fast, deterministic, no-network tests for the pure logic (`assetNameFor` across all 5 supported platforms + one unsupported, `parseSha256Digest`, zip/tar.gz extraction against synthetic in-memory archives) plus light real-network checks against the actual GitHub API using the small `checksums.txt` asset (838 bytes) rather than the multi-MB platform binaries, so the routine `go test ./...` sweep stays fast and skips cleanly if offline. One additional test, `TestTryAutoInstallEndToEnd`, actually downloads the full real binary, extracts it, and runs `--version` to confirm the pieces compose correctly end to end — gated behind `AETOX_TEST_RTK_E2E=1` (skipped by default) since a full multi-MB download on every test run would be disproportionate; run manually once during this work and confirmed passing.

**Status:** `Approved & done 2026-07-23.` Full repo `go build`/`go vet`/`go test ./...` green (including the gated end-to-end check, run manually and passing).

---

## 14. Decision — Unified Data Root (2026-07-23) — cleaning up where Aetox writes its own data

Owner, continuing straight from §13.6: "ทำระบบของเราให้คลีนเลยไม่ไปสร้างเครสมั่วๆ อีกได้ไหมครับ... ข้อมูลใน WebView เราจะฝังในระบบของเราเลยดีสุดปลอดภัยสุดครับ" — make the system clean, stop scattering files, and WebView2 data specifically should be embedded in Aetox's own design rather than left to an external library's default. Also raised as a general principle: things Aetox designs itself and doesn't need to share with other tools should live inside Aetox's own structure, not wherever an OS/library convention happens to put them.

### 14.1 Evidence — re-surveyed every OS-directory resolution point (`Direct`, re-grepped after §13's rtk work)

| Location found | Consistent with the others? |
|---|---|
| `internal/config`'s 5 path functions (preferences, permissions, MCP servers, `.env`, `AETOX.md`) | ✅ all `<UserConfigDir>/aetox/*`, but each one duplicated the same 6-line OS-fallback block inline |
| `desktop/db.go` (session SQLite) | ✅ same folder, but its own separate copy of the same fallback logic |
| `internal/rtk/install.go` (downloaded `rtk` binary, built in §13.6) | ✅ already correct, reused `UserGlobalContextPath`'s directory |
| `internal/audit/audit.go` (shell command audit log) | ❌ used `os.UserHomeDir()` + `.aetox` — a **second, different "Aetox home"** (`~/.aetox`) that had nothing to do with the `%AppData%\aetox` folder everything else uses. Flagged in an earlier session pass, left pending owner approval — approved and fixed in this pass. |
| `desktop/main.go`/`browser.go` (WebView2 profiles) | ❌ left empty by default → Wails/go-webview2's own silent convention (`%AppData%\<exe-name>`), which is *why* `aetox-desktop.exe`, `aetox-desktop-dev.exe`, and `aetox-browser` were three unrelated folder names for what's conceptually one thing — not a location Aetox itself ever decided on |
| `internal/skill/discovery.go` + `github_tools.go` (`~/.agents/skills`, `~/.claude/skills`) | **Deliberately not unified** — see §14.3 |

### 14.2 Design — one function, everything else derives from it

`config.DataRoot()` (new, `internal/config/config.go`): resolves an `AETOX_DATA_ROOT` env var override first, else `<UserConfigDir>/aetox` (the production default, unchanged). Every one of the ✅ and ❌ rows above except the intentional exception now calls this one function instead of repeating the OS-fallback dance or inventing its own convention:

- `PreferencePath`/`PermissionsPath`/`MCPServersPath`/`EnvFilePath`/`UserGlobalContextPath` — refactored onto `DataRoot()`, deleting 5 copies of identical fallback logic (`LegacyPreferencePath` deliberately left untouched — it represents a fixed historical location for one-time migration, not a place to redirect).
- `desktop/db.go` — same, keeping its existing `dbDir` test-seam override (a *different*, test-only mechanism from `AETOX_DATA_ROOT`, both still work together).
- `internal/rtk/install.go`'s `privateBinaryPath` — simplified to call `DataRoot()` directly instead of borrowing another function's directory.
- `internal/audit/audit.go` — **fixed**: `ShellAuditLogPath` now uses `DataRoot()`, closing the `~/.aetox` vs `%AppData%\aetox` inconsistency flagged earlier.
- `desktop/main.go`'s `webviewUserDataDir` — **now always** returns `<DataRoot()>/webview/<name>`, explicitly, every time (empty return is only a last-resort fallback if `DataRoot()` itself errors) — never Wails'/go-webview2's silent exe-name-based default again, for dev *or* production. `browser.go` already called this same helper, so it picked up the fix automatically.
- `desktop/wails-dev.bat` — the dev-only override env var is renamed from the narrower `AETOX_WEBVIEW_DATA_DIR` (introduced in §13's disk-cleanup pass, webview-only) to the general `AETOX_DATA_ROOT`, now covering *all* of Aetox's data during dev, not just WebView2. `desktop/.gitignore`'s `.webview2-data` entry renamed to `.aetox-data` to match.

This reconciles the two asks that were briefly in tension: "keep dev data off C:" (§13, still true — `wails-dev.bat` still redirects everything to a project-local folder) and "WebView data should be embedded in our own design" (this section — the *shape* of where it lives is now something Aetox explicitly decides, `<DataRoot>/webview/*`, not a location any external library chose for it; the drive it happens to sit on during dev is a separate, already-solved concern).

### 14.3 Explicit exception — skill discovery/`plugin_install` stay pointed at shared, external locations

`~/.agents/skills` and `~/.claude/skills` (`internal/skill/discovery.go`) are deliberately **not** brought under `DataRoot()`. These are ecosystem-wide conventions — the same paths OpenCode and Claude Code use — and the entire value of scanning them is that *other tools'* skills/plugins get auto-discovered, and anything `plugin_install` writes into `~/.agents/skills` becomes visible to those other tools too. "Don't share what doesn't need to be shared" (the owner's own stated principle) cuts the other way here: this is a location that specifically *needs* to stay shared to keep working. Unifying it under Aetox's own private folder would quietly break interop for no benefit.

### 14.4 Test-isolation bug found and fixed along the way

`internal/audit/audit_test.go` isolated itself by overriding `HOME`/`USERPROFILE` — which redirected the *old* `os.UserHomeDir()`-based path, but `DataRoot()` resolves via `os.UserConfigDir()` (`%AppData%` on Windows), which those env vars don't affect. Running the suite after the refactor made this obvious immediately: tests started reading/writing the real `%AppData%\aetox\shell-audit.log` on this machine, accumulating real entries across runs (`want 2 got 5`-style failures). Fixed by isolating via `AETOX_DATA_ROOT` instead — dogfooding the same override mechanism `wails-dev.bat` uses, rather than the audit package inventing its own isolation trick.

Also found while fixing it: `internal/skill/shell_test.go`'s tests call `shellSkill.Execute`, which unconditionally calls `audit.WriteShell` — and had **never** isolated the audit log at all, on any prior session. Every `go test ./...` run before this fix was silently appending real entries to the real, machine-wide audit log. Fixed the same way (`isolateAuditLog` helper, `AETOX_DATA_ROOT`). The real log file this had already polluted was deleted by hand as part of this cleanup.

### 14.5 Deferred, not designed now — easier installation (npm or otherwise)

Owner mentioned wanting Aetox installable via `npm` or similar "someday," explicitly framed as a future direction, not a current ask. Noted here so it isn't lost, not designed: Aetox is a Go+Wails binary, not a Node package, so "npm install" would mean a thin wrapper package (the pattern several Go/Rust CLI tools use — an npm package whose `postinstall` fetches the real platform binary, similar in spirit to how `internal/rtk/install.go` already fetches `rtk`'s binary). Revisit when actual distribution work starts; not blocking anything today.

**Status:** `Approved & done 2026-07-23.` Full repo `go build`/`go vet`/`go test ./...` green.

---

## 15. Decision — Coding Loop Tools: `edit` + `grep` (2026-07-23)

Owner brought external feedback proposing three workstreams (browser mastery, total tool command, full coding capability). Checked against the actual code before building anything:

### 15.1 Evidence — most of the feedback was already built or already decided

| Proposed | Reality |
|---|---|
| "Tool chaining" | Already exists — `cognitive.Agent.RespondWithTools` is the model-driven multi-tool loop (§10/§3 of the model-control doc). |
| "Self-healing execution" | Not a feature — `turn.modelToolReceipt` already returns `stderr`/`success` to the model; retrying is the model reading the receipt. No code needed. |
| "Max-retry guard against infinite loops" | Already decided — `AgentConfig.MaxToolCalls` is an opt-in cap; the deliberate default is unbounded with permission gate + ctx cancel as the brakes (comment at `cognitive/agent.go:71`). Not re-litigated. |
| "Coding loop: Explore→Read→Edit→Verify" | Read (`read`), Verify (`shell`) existed. **Explore and Edit were the real gaps** — no content search, and `write` (whole-file overwrite) was the only mutation tool. |
| "Browser control" | Skeleton already exists (`desktop/workbench.go` `browser_open/read/click/type`, `SourceExternal`). Deeper design deferred until the coding loop is proven. |

### 15.2 Design — two new built-ins, nothing else

- **`edit`** (`internal/skill/edit.go`) — exact search & replace: model sends `path`/`old_string`/`new_string`; Go verifies `old_string` matches **exactly once** (0 → "not found, re-read the file"; >1 → "add surrounding lines"), then replaces. **Rejected the feedback's suggestion of LLM-generated unified diffs + `go-diff`**: models drift on line numbers, and uniqueness-checked literal replace is both safer and dependency-free (`strings.Count` + `strings.Replace`). `ExecuteTool` deliberately bypasses `stringSlice` (it trims/drops empties, which corrupts whitespace-significant match strings). Classified `RiskHigh`/`EffectWriteWorkspace` in `safety.go` — critical because *unknown* skill names default to `RiskLow`/no-effects there, which would have made `edit` an unprompted file writer.
- **`grep`** (`internal/skill/grep.go`) — stdlib `regexp` + `filepath.WalkDir` content search returning `path:line:text`, capped (200 results, 1 MB/file, skips dot-dirs and binaries). **Rejected shelling out to ripgrep**: not guaranteed installed, and the stdlib walk is ~100 lines with no dependency. `RiskLow`/`EffectReadWorkspace`.

Both registered in `defaults.go`, path-arg extraction added to `turn.toolCallToArgs` so permission patterns (`{Tool: "edit", Pattern: "docs/*"}`) can match. Tests follow the existing per-skill pattern; the full suite also caught that `stringSlice` trimming issue before it shipped.

**Status:** `Done 2026-07-23.` `go build`/`go test ./...` green. Next in this thread when owner resumes it: browser-control design (§15.1 last row) as its own Decision section.

---

## 16. Decision — Dead-Code Sweep (2026-07-23)

Ran `golang.org/x/tools/cmd/deadcode ./...` (trustworthy here: Wails-bound desktop methods register as reachable). 47 findings, triaged three ways:

**Deleted (~430 lines):** CLI-era and module-split leftovers — `model/provider_catalog.go` 4 delegate funcs (note the trap: dead `ModelChoicesWithEndpointAndAPIKEY` vs live `...APIKey`), `DefaultThinkingLevel` + `BuildCapabilityCatalog`/`ModelCapability`, `provider.CapabilitiesFor`/`EnvKeys`, duplicated `SlashMetaLegend`/`SlashSuggestions` in both `command` and `grammar` (+ `grammar.New`), `turn/infer.go`'s regex-inference helpers superseded by the model-driven loop (`inferToolFromConversation`, `parseWriteIntent`, `parseListPath`, `applyExtensionHint`, `splitIntentParts`), `turn.executeToolCall`, `app.go`'s `dispatchBySkill`/`showSkillPalette`/`printAvailableSkills`, `config.SavePermissions` (its round-trip test rewritten to cover the live `LoadPermissions` directly), `safety.ApprovalModeFromLegacy`, `debuglog.IsEnabled`/`LogDir`. Tests referencing deleted funcs removed with them; two tests that actually pinned live code were kept and repointed instead of deleted.

**Deliberately kept despite being flagged:** `internal/orchestrator` (§10's unwired layer — the future task/subagent tool) and `rtk.Available` (live test-infrastructure: skip-guard in `shell_test.go`/`rtk_hook_test.go`; deadcode doesn't count test binaries).

**Pending manual delete (session tooling can't remove files):** `internal/plan/` (whole package, zero callers), `internal/model/openrouter.go` + `_test.go` (duplicate of the live OpenRouter spec in `internal/provider`), `internal/turn/record.go` (aspirational audit-record type, never constructed anywhere).

**Status:** `Done 2026-07-23` except the four files above. `go build`/`go vet`/`go test ./...` green; `deadcode` re-run clean apart from the kept/pending items.

---

## 17. Decision — Kill the Regex Intent Layer (Proposed 2026-07-23)

**Trigger (real failure, 2026-07-23):** user asked `ornith:9b` "คุณทำอะไรได้อีก เอาเนื้อหาในเว็บมา ทำเป็นไฟล์ html ให้ผมได้ไหม". Phase 1's regex (`internal/turn/infer.go`, `reWriteIntent`) matched the bare verb "ทำ", hijacked the turn into a direct `write` with a garbage path, and the model never saw the message. The per-regex patch shipped that day treats one symptom; the architecture invites the next one.

**The debt, mapped (audit 2026-07-23):** `turn.Executor.Execute` runs *four* phases, three of which second-guess the model with NL keyword regexes:
- Phase 1 (`executor.go:178`, `shouldExecuteInferredBeforeAgent` :255): regex candidates for write/read/delete/github/plugin_install execute **before the model is ever called**.
- Phase 2 (:190): the real model-driven `RespondWithTools` loop — but if the model doesn't call a tool, or the tool fails, it **silently re-enters the regex loop** (:196-209).
- Phase 3 (:213): regex-only path for non-tool agents.
- `infer.go` is 746 lines / 22 NL regexes (Thai + English), the only NL-inference file left in the repo (§16's sweep already deleted its dead half).
- Compounding it: `ollama.SupportsToolCalling()` is hardcoded `true` (`internal/model/ollama.go:56`) — never checked against the actual model, so weak local models "pass" the Phase 2 gate, fail to tool-call, and land in the regex layer anyway.

**Reference points:** Claude Code does zero NL inference — pre-model parsing is explicit syntax only (`/slash`, `!bash`, `@file`); everything else goes to the model with tool definitions and the model decides every call. A weak model's failure mode is a plain text answer, never a hijacked tool. OpenCode is the same shape: a per-model capability catalog declares `tool_call` true/false; non-tool models simply get no tools (chat-only). Neither has a keyword layer, in any language.

**Decision:**
1. **Delete the layer.** `internal/turn/infer.go` entirely; executor Phases 1 and 3; both silent regex fallbacks inside Phase 2; `shouldExecuteInferredBeforeAgent` / `shouldUseInferredToolPath` / `executeInferredToolCandidatesLoop` / `executeInferredTool` and the inferred-path test suites (executor_test.go is 1,387 lines mostly pinning this layer). Net ≈ −2,000 lines.
2. **Pipeline becomes CC-shaped:** `grammar.Parse` (kept — explicit slash/meta/known-command tokens, the analog of CC's slash commands) → explicit `KindSkill` command → `executeSkillTurn` direct dispatch (kept — analog of `!`) → everything else: model tool loop when the provider truly supports tools, else plain streaming chat. The model's answer is final; nothing re-guesses it.
3. **Make Ollama capability honest:** replace the hardcoded `true` with a bootstrap probe of Ollama's `/api/show` capabilities list (`"tools"`, Ollama ≥ 0.4), plus a runtime backstop — on the API's "does not support tools" error, mark the session chat-only. Surface the result in `ModelInfo` so the desktop UI can show "tools unavailable for this model" instead of pretending.
4. **Accepted regression:** non-tool-capable local models lose the "สร้างไฟล์ x.txt" magic and become chat-only — same behavior as OpenCode, and honest. The fix for a weak model is a tool-capable model, not a regex.

**Status:** `Done 2026-07-23.` Owner approved same day, with one amendment to point 3: **no preemptive capability gating** — tools are served to every model normally, no size/capability prejudgment (owner's call: "ระบบเราจะเสิร์ฟให้โมเดลตามปกติ... แต่เผื่อทางไว้ก็ดี"). The `/api/show` probe was dropped; only the runtime backstop shipped — `ollama.Complete` retries a tools request as plain chat when Ollama itself answers "does not support tools" (`internal/model/ollama.go`, pinned by `TestOllamaComplete_RetriesWithoutToolsWhenModelRejectsThem`). Demolition tally: `infer.go` (−746), `record.go` (−36, §16's pending item), `executor.go` 876→620, `executor_test.go` 1387→560 — net ≈ −2,050 lines. New §17 regression pin: `TestExecute_ConversationTextNeverTriggersToolsDirectly` (the exact Thai sentence that triggered this decision). `go build`/`go test ./...` green.

---

## 18. Decision — Browser Embedding: Never Find Your Own Window by Title (2026-07-24)

**Trigger (real failure, 2026-07-24):** every browser tab rendered blank — `file://` pages AND https — surviving app restarts, with zero errors anywhere. §6.6's z-order fix was live and correct, but no tab webview process was even being created.

**Evidence (instrumented `open()`/`start()`, read from `debuglog`):** `CreateWindowExW FAILED: Access is denied.` on every tab, and the one-line diagnosis: `parent hwnd=… parentPid=1464 selfPid=9148` — `browserHost.start()` located the main window via `FindWindowW(nil, "Aetox Desktop")`, i.e. **by title**, and matched a *foreign process's* window carrying the same text (an `explorer.exe` taskbar-thumbnail host; a dev-URL browser tab titled by our own `<title>` can collide the same way). A cross-process parent makes `WS_CHILD` creation fail with `ERROR_ACCESS_DENIED` — silently, because `open()` swallowed every failure path.

**Fix applied ([desktop/browser.go](../desktop/browser.go)):**
1. `findOwnMainWindow()` — enumerate top-level windows, match by `GetWindowThreadProcessId == os.Getpid()` + visible. Title-based lookup is banned in this subsystem.
2. Every native failure path now logs to `debuglog` with tab id — the silent `return`s are what turned a one-line bug into a half-night hunt.
3. Visibility hardened while in there: `SetWindowPos(HWND_TOP)` moved into `NavigationCompletedCallback` natively (§6.6's meta-event-driven re-glue depended on page JS surviving the origin check — never true for `file://`, whose URLs have no host; `sameOrigin` now special-cases file↔file, which also restores title/URL sync for local pages).
4. Address-bar `normalizeUrl` (frontend): drive paths → `file:///`, schemes pass through — blind `https://` prefixing had produced dead `https://file:///…` URLs.

**Full write-up:** [docs/architecture/native-browser-embedding-2026-07-24.md](../docs/architecture/native-browser-embedding-2026-07-24.md) — architecture, the complete failure catalog (7 entries), and the macOS (WKWebView subview) / Linux (WebKitGTK widget) port blueprint; both platforms structurally avoid the window-parenting and z-order classes entirely. Also settles §7's open question in passing: `desktop/browser.go` has no build tag — desktop is Windows-only today by construction, and the port blueprint is the plan for changing that.

**Status:** `Done 2026-07-24.` Owner verified tabs load (file:// + web). `go build`/`go test ./desktop` green (file↔file origin cases added to `TestSameOrigin`).

---

## 19. Decision — Desktop: No-Project-Focus Default, Session-Bound Workbench, Non-Nil Binding Contract (2026-07-24)

Three same-day desktop decisions, recorded together; all owner-driven, all shipped.

### 19.1 No-project-focus is the startup default

**Trigger:** the composer chip showed "desktop" — the engine silently adopted whatever cwd it was launched from (`config.go`'s `RootPath == "" → os.Getwd()`; `wails dev` runs from `desktop/`). Owner: never bind to the launch folder; focus must be an explicit choice (Claude Desktop's เลือกโปรเจกต์ picker as the reference), and unfocused chat must keep **full tool access** — "เหมือนเปิด Claude/Codex มาคุยดื้อๆ แต่ยังรันอะไรในเครื่องเราได้."

**Design ([desktop/app.go](../desktop/app.go), [desktop/sessions.go](../desktop/sessions.go)):** `App.projectFocused` flag; startup calls `focusNone()` — engine re-roots at the user's **home dir** (tools all work, rooted neutrally), no `touchProject`, so launch cwd never pollutes recent projects. `ClearProjectFocus` binding + a focus picker on the composer chip (no-focus / recent projects / open folder). Unfocused sessions live under the home-dir `projectKey` bucket — no schema change — and `LoadSessionAnyProject` special-cases that bucket back into unfocused mode instead of erroring "project not found". `ProjectTree`/`GitChangedFiles` return empty when unfocused (never eagerly walk a home dir). `ProjectStatus.Focused` drives the UI.

**Amendment 2026-07-26 — the unfocused root is `<home>/aetox`, not the home directory.** Owner, reading the system prompt aloud: *"Current working sandbox root is: C:\Users\Gigabyte. บรรทัดนี้เหี้ยมาก … แล้ว sandbox คืออันนี้จริงๆหรอ"*. It was. Containment is "anything under the root" ([resolveSandboxPath](../internal/skill/list.go)), so unfocused the sandbox held `.ssh`, `.aws`, `.gitconfig`, `AppData` with its browser token stores, Documents — readable by `read`/`grep`/`glob` on any turn and writable with no prompt, since `focusNone` sets full-access in the same call.

What makes this a reversal rather than a correction of a mistake: **the original call was sound for the tools that existed on 2026-07-24.** What changed is the loop's shape — `web_fetch`, `web_search` and `browser_read` now sit in it, so a page can carry an instruction in and the same loop can carry an answer back out. Broad read access stops being a blast radius and becomes an exfiltration path the moment untrusted text and network egress share a loop.

- **`focusNone` roots at `<home>/aetox`** ([desktop/app.go](../desktop/app.go)), created on demand so `list .` works on a fresh install.
- **`outputSubdir` drops its `aetox/` prefix** in the same commit — it is now `output/<session>`, joined onto the new root. The absolute destination is unchanged (`<home>/aetox/output/<session>`), so **nothing on disk moved and no migration runs**: the new root is exactly the parent of the folder writes already landed in. Owner caught this one: *"อย่าลืมนะครับ เรามี aetox\output ด้วยหนา"*. The two halves are asserted together in `desktop/app_test.go` — alone, either change doubles the folder name or strands every artifact.
- **`LoadSessionAnyProject` accepts both bucket keys** ([desktop/sessions.go](../desktop/sessions.go)): the new root's, and the home dir's for every chat held before this. Dropping the old key would answer real, still-present history with "โฟลเดอร์อาจถูกย้ายหรือลบไปแล้ว".
- **Reaching outside is now a deliberate act**, and both routes already existed: open the folder as a project, or attach the file (`saveChatAttachment` copies it under the root, so it works whatever the root is). What is genuinely lost is *"ไปส่องโฟลเดอร์ Downloads ให้หน่อย"* without focusing anything first — which is the ask that should require a click.
- **Not done, deliberately:** deny-rules for `.ssh`/`.aws`/… while staying rooted at home. That is a list to maintain forever, and it fails on the first entry nobody thought of.

**Amendment 2026-08-04 — unfocused mode roams the machine again (OpenSandbox).** Owner, after watching an unfocused chat answer "หาไฟล์ PDF ในเครื่องนี้" with *ทำไม่ได้*: unfocused should mean the machine is the workspace — "มันควรไปไหนมาไหนก็ได้ในเครื่องเรา … ถอดข้อจำกัดก็พอ ผมแค่ระบุว่าเวลาสร้างไฟล์ให้สร้างในเอาท์พุต". The 07-26 wall made the mode safe by making it useless for exactly the errands it exists for.

- **`skill.RegistryOptions.OpenSandbox`** ([internal/skill/sandbox_open.go](../internal/skill/sandbox_open.go)): openness is recorded per root by `RegisterDefaults` on every registry build, and `resolveSandboxPath` — still the single wall every file tool answers through — accepts absolute and climbing paths for an open root. The desktop passes `!projectFocused` ([desktop/app.go](../desktop/app.go) `applyConfig`); a focused project and the whole CLI stay closed, unchanged.
- **The 07-26 threat gets a narrower answer instead of a wall:** a short credential-store denylist (`.ssh`, `.aws`, gh/git creds, browser profile stores, `~/.aetox` — Aetox's own keys) is refused inside `resolveSandboxPath` itself, open mode or not. The 07-26 bullet above rejected a denylist *as the only fence around a home-rooted sandbox*; here it locks the cabinets while the mode's real containment is that writes still land in `output/<session>` and a project focus restores the full wall.
- **The prompt tells the same story as the tools** ([internal/prompt/prompt.go](../internal/prompt/prompt.go) `environment(open)`): the refusal in the trigger screenshot was the *model's belief*, not the sandbox — it had been told "absolute paths are rejected" and kept saying so. Open mode says: any path works, credential stores are off-limits, bare filenames land in the output folder.
- **Accepted, eyes open:** unfocused still runs full-access, so an injected page can now order reads of anything outside the credential list without a prompt. The owner's call is that on a personal machine the errand value outweighs it; the cabinets that turn that read into a stolen key are the part that stays shut.

**Amendment 2026-08-04b — a focused project can be given more folders (ExtraRoots).** Owner: *"จริงๆผมอยากให้มันเลือกข้ามโปรเจคได้ แบบบางทีปัญหาในโปรเจคมาจากที่อื่น ให้ AI ไปตรวจสอบได้ แต่โหมดโฟกัสโปรเจคพื้นฐานก็คือทำงานในโฟลเดอร์โปรเจคที่กำหนด"* — and then the rule that shaped the design: *"สิทธิ์ควรกำหนดที่เดียว คือผ่านที่ผู้ใช้เลือก ที่เหลือไม่ต้องแอบมีเบื้องหลัง … มันจะกลายเป็นหนี้ในระบบในอนาคต สำหรับคนใช้"*.

Until this, the only way to read a second project was to unfocus — trading one folder for the whole machine to look at one directory. Claude Code answers the same need with a working directory plus `--add-dir`/`/add-dir`/`permissions.additionalDirectories`; this is that shape, minus the three places to configure it.

- **One gate, one list.** `sandboxPolicy` ([internal/skill/sandbox_open.go](../internal/skill/sandbox_open.go)) holds a root's whole reach — `open`, plus the user's `extra` folders — and `resolveSandboxPath` is still the only thing that reads it. Set by `RegisterDefaults` on every registry build, zero values included, so removing a folder narrows the session in the same reload.
- **Extra folders carry the project's rights exactly** — read and write, no prompt. A quieter second tier ("look but don't touch") would be a permission the user never agreed to and cannot see in the list, which is the debt the owner's rule is about. The list is the permission: a folder that should not be written to does not go on it.
- **Nothing else may widen it.** No folder inferred from imports, no sibling adopted because it looked related, no temporary grant. `RegistryOptions.ExtraRoots` is fed only from `project_folders` (schema v6, keyed per project), which is fed only by the picker.
- **Absolute paths stopped being refused on sight.** That was a spelling rule wearing a permission rule's clothes, and it cannot survive a second workspace folder — naming that folder in full is the only way to reach it. The gate now judges where a path lands, never how it was written; an absolute path outside the workspace is refused exactly as before ([internal/skill/sandbox_test.go](../internal/skill/sandbox_test.go)).
- **The prompt is built from the same list** ([internal/prompt](../internal/prompt) `Scope`): added folders are named in full — the one machine-specific path worth sending, because an added folder has no other name — along with the fact that they carry the project's rights. A folder the tools accept but the prompt never mentions is a capability the model will not use.
- **The credential cabinet does not answer to the list** — adding `~` does not hand over `~/.ssh`, and the picker refuses a credential-store folder outright rather than accepting it and failing file by file.
- **UI lives in the focus menu, not a settings page** ([desktop/frontend/src/lib/Chat.svelte](../desktop/frontend/src/lib/Chat.svelte)): it answers the same question that menu already answers — what can the AI reach right now — and a chip on the focus row shows the count without opening it, so a widened session says so where the user already looks.
- **Not covered:** the desktop's own file APIs (`safeSandboxPath`: the file tree, `ReadFile`, drag-drop) stay project-only. That is the user acting directly rather than the agent, so it is a UI feature, not a hole — but the tree will not show an added folder.

**Amendment 2026-08-04c — `shell` answers to the gate too.** Owner, on being told the shell was the one tool still walking out of a focused project: *"ปิดให้หมดสิครับ shell สำคัญมาก"*. It had only ever set `cmd.Dir` and handed the line to the OS, so `type D:\Other\file` worked — and with the desktop on full-access, without a prompt. The wall the UI describes was real for read/write/grep and imaginary for the tool that can do what all three do.

**Design ([internal/skill/shell_sandbox.go](../internal/skill/shell_sandbox.go)):** every path a command names goes through `resolveSandboxPath` — the same function, not a second copy of its rules — before the command runs. Tokens are split quote-aware and per command segment; `~`, `%VAR%`, `$VAR` and `$env:VAR` are expanded first; `--out=` and cmd's `/f:` prefixes are stripped; a glob is reduced to its literal directory. A path found outside refuses the command with the message that names the remedy. `git` was routed through the same gate (`guardArgs`) in the same change, retiring its reliance on a private denylist that blocked `-C` by name and let `--output=` through.

- **Two things it deliberately does not contain, stated so they are not mistaken for oversights.** *Program position:* a command's first token names code to run, and `go` through PATH reaches a binary outside the workspace exactly as surely as its absolute path does — checking the spelling would forbid the spelling and permit the act. *What a program does once started:* a script inside the workspace can open any file the user can. Both need an OS boundary, not a better parser.
- **A construct it cannot read stops the command** rather than passing through: `$(…)`, backticks, `${…}`, `-EncodedCommand`, `Invoke-Expression`, and a variable in path position that is not one of the folder-naming names. This is where it parts from OpenCode, whose shell tokenizer is the better-engineered version of the idea (real tree-sitter grammars for bash and PowerShell, provider paths, cygpath) but which does not gate on what it finds: read against the source (the source-reading note on permissions (kept off this repo), commit `76ced54`), `tool/bash.ts` matches the **raw command string** against the permission rules and the path scan is an advisory warning only. Its containment is the prompt — default `ask` when no rule matches — and the filesystem is not walled at all. *(Corrected 2026-08-15: this bullet previously said OpenCode "checks paths only for a known verb list", which overstates it. The verb list produces a warning, not a refusal.)*
- **Paths are also matched by shape anywhere in the line**, not just as tokens: `python -c "open('C:/Users/me/.ssh/id_rsa')"` is one token to any tokenizer and a path to the OS. Reading inside those strings properly would need an interpreter per language; recognising the shape costs a scan and closes the family (node -e, sed, `powershell -Command`).
- **Containment, not isolation, and the difference is the point.** It reads the command as written and cannot follow a path that only exists at runtime. Claude Code draws the same line and ships both halves — permission rules on the command string, plus an OS sandbox (Seatbelt, bubblewrap) that holds "regardless of what the model chose to run". The OS half does not exist on native Windows, so this is the half that can be built today; what it cannot check, it refuses.
- **The prompt says so** ([internal/prompt](../internal/prompt)): shell obeys the same rule, and reaching for shell after another tool refused a path gets the same answer. Without that the model spends a turn discovering it.

**Amendment 2026-08-15 — the wall gets a door (`AskWorkspace`).** Owner, on being told that Aetox refuses cross-project paths where the competition does not: *"Claude code ทำงานข้ามโปรเจคได้ครับ"*. He is right, and the framing that called our refusal a strength was wrong. Claude Code's project is a starting point, not a fence — cross-project work is ordinary daily work there, and OpenCode has no filesystem wall at all. Being stricter than both **at the exact thing they are useful at** is not safety, it is narrowness.

What was actually expensive was not the refusal but the asymmetry: the agent knew the path it needed, and the user had to go find the same folder by hand — project menu, OS dialog, a path already written in the transcript — and then ask a second time. The app knew the answer and made the person fetch it.

- **The door is one branch in one function.** `sandboxPolicy` grows an optional `ask` ([internal/skill/sandbox_open.go](../internal/skill/sandbox_open.go)); when `resolveSandboxPath` is about to refuse a path for being outside the workspace, it offers the folder to the host and re-checks. Every file tool and `shell` reach it because they already answer through that one function — nothing per-tool was written.
- **The answer is a hint; the list is the permission.** Whatever the host returns, the verdict is re-read from the policy: a host that says yes without putting a folder on the list gets the refusal it would have got anyway ([sandbox_widen_test.go](../internal/skill/sandbox_widen_test.go)). The door still leads *through* the list — it stops making the user walk around to reach it.
- **No "allow once".** A right that is not on the list is a right the panel has stopped describing, which is the 08-04b rule (*"สิทธิ์ควรกำหนดที่เดียว"*) unchanged. A yes writes the folder to `project_folders` like any other, so it survives the session and can be removed in the panel.
- **The credential cabinet is unaffected**, because it is enforced *after* this point on the same resolved path. That is deliberately the ordering OpenCode uses to keep a saved "always allow" from overriding a configured deny — read from `permission.ts:147-162`, where `denied()` is evaluated against config rules before saved-allow rules are appended.
- **`project_folders` was already OpenCode's saved-allow shape**: per project, allow-only, persisted. The difference is the unit — a folder rather than a raw command string — which is the half a person can actually reason about on a card.
- **A folder approved mid-turn reaches the running call without rebuilding the engine** (`skill.SetWorkspaceFolders`, the one writer that is not `RegisterDefaults`), and the engine picks up the same list at `endTurn`. Rebuilding under a turn in flight would discard the work the user is waiting on.
- **No window, no card.** `a.emit == nil && a.ctx == nil` — headless runs and the whole CLI keep the flat refusal, and `prompt.Scope.CanAsk` is fed from the same field, so the model is never told to expect a door this session has not got. Caught by a nil-context panic on the first run; pinned by `TestWidenIsSilentWithNoWindow`.
- **A volume root is never offered.** It is what `filepath.Dir` answers for a loose file at the top of a drive, and one click would hand over every project on it.
- **Not done:** an OS sandbox. Claude Code does not have one on Windows either, so copying it buys nothing on the platform 1.0.0 ships to; and the hard refusal for unreadable tokens stays, because a card that cannot say which path it is granting is not a question.

**The prompt line went with it.** [internal/prompt](../internal/prompt) no longer states the sandbox root at all: it is machine-specific, carries the user's account name, was sent to whichever provider is configured on every request, and the next sentence told the model not to repeat it. It bought nothing — every file tool rejects an absolute path, so the root could not be used in a tool call — and its one real use, answering *"ไฟล์อยู่ที่ไหนในเครื่อง"*, is now served by `write`'s receipt, which names the on-disk path when placement moved the file. What replaces it is the rule whose absence caused the wrong answer in the first place: **repeat the path a tool reported, never assemble one**. The cost of the trade is that the model can no longer name the project's absolute path unprompted; the UI's project badge already shows it.

### 19.2 Workbench (right pane) state is per-session

**Trigger:** owner: "แต่ละเซสชันต้องผูกกับฝั่งขวาที่เปิดไว้ด้วย" — switching sessions must bring back what was open.

**Design ([workbench.svelte.ts](../desktop/frontend/src/lib/stores/workbench.svelte.ts)):** layout snapshots (browser URLs, file paths, singleton panes, active tab) persist to localStorage keyed `aetox-workbench:<sessionId>` — the Go session store never learns about UI layout. Two entry paths: `switchWorkbenchSession` (explicit switch: save old, restore new) and `adoptWorkbenchSession` (passive id change: first sighting restores, a mid-chat re-key migrates the open tabs). **Terminals are excluded** — a dead PTY restored as an empty shell is more confusing than a closed tab.

### 19.3 Bindings never return nil slices

**Trigger (real failure):** the model-picker button "didn't work" — a Go `nil` slice serialized to JSON `null`, and the menu's render crashed on `thinkLevels.length` mid-open. Same class hit `ListModelsForProvider` and `ListIdentityFiles` (sidebar startup error).

**Rule:** any Wails binding returning a slice returns `[]T{}`, never nil. Fixed in `SupportedThinkLevels`/`ListModelsForProvider`/`ListIdentityFiles`; treat this as the standing contract for every future binding (frontend code is allowed to assume arrays).

**Status:** `Done 2026-07-24.` All three verified live (screenshots + `go test ./desktop` + `svelte-check` green). **Open item from the same session:** DeepSeek V4's DSML tool-call markup leaking into chat as raw text — backstop parser written ([internal/model/dsml.go](../internal/model/dsml.go), grammar from DeepSeek-V4-Flash's encoding README) but not yet wired into `openai_compatible.go`'s `Complete()`; §17's backstop-not-gate convention applies. A latent sibling bug is noted for the same file: `StreamComplete` silently drops structured tool-call deltas (currently harmless — no streaming path passes tools).

---

## 20. Decision — Tool-Loop Hardening, Context Truth, Summarizing Compaction (2026-07-24)

One session, three engine layers fixed; OpenCode/Claude Code are the confirmed reference implementations for all of it (owner: "ใช้ OpenCode / Claude Code นี่แหละครับ เป็นแนวทางในการพัฒนาระบบโค้ดดิ้งและคอร์หลัก").

### 20.1 Tool-loop doom loops from truncated tool calls (`7017324`)

**Trigger (real failure):** desktop chat looped forever writing a landing page — `write` failed 7 times with `open C:\Users\...\:` before accidentally succeeding.

**Root cause chain (from `debuglog`):** the tool loop hardcoded `max_tokens: 4096` → long write payloads got cut mid-JSON (`finish_reason: "length"`, never checked) → the salvage parser (`salvageWriteArgs`) had an index bug that turned *every* truncated write into path `":"` → the model was told "path is wrong", so it retried new paths forever (the loop is deliberately uncapped, §17-style; the brake was missing). OpenCode hit the identical class — sst/opencode#18108.

**Fixes ([internal/cognitive/agent.go](../internal/cognitive/agent.go), [internal/model](../internal/model), [internal/turn/executor.go](../internal/turn/executor.go)):** per-provider explicit `max_tokens` (OpenCode's 32k ceiling, clamped where an API 400s: deepseek-v4* 32000, V3-era 8192, openai 16384, anthropic/gemini/zai 32000, others 8192; Ollama now maps `MaxTokens → options.num_predict`); `Response.FinishReason` surfaced from all providers (`length`/`max_tokens`/`done_reason`), and a truncated tool call is **not executed** — the model gets a truthful "arguments truncated, shorten or split" receipt; `salvageWriteArgs` deleted (half-written files reported as ✓ are worse than loud failure); doom-loop brake à la OpenCode PR #3445 (identical consecutive call: warn at 3, hard-stop at 5, reset on any different call). §19's open DSML item closed the same day (`3c9c7e1`): the backstop parser is wired into `Complete()`.

### 20.2 Context truth: per-model windows, no message cap (`fa3bb61`)

**Trigger:** the new context meter showed 32k for deepseek-v4-flash — a 1M-token model. The 32k was `memory.Context`'s internal 128k-char buffer, unrelated to the model.

**Fix:** [internal/model/context_window.go](../internal/model/context_window.go) — curated per-model windows (thinking_capabilities.go pattern; openrouter resolves `vendor/model` through the vendor; unknown → 0, caller falls back); the agent's char budget scales to the window (`tokens×4`) in CLI and desktop; `MaxTurns=80` deleted outright — nobody set it, the token/char budget is the only brake, same as the references. User override (`ModelContextTokens`) still wins everywhere.

### 20.3 Summarizing compaction (`65fea12`)

**Trigger:** with real windows in place, the remaining honesty gap: at budget the engine dropped whole old turns verbatim — long coding sessions forgot early decisions.

**Design ([internal/memory/context.go](../internal/memory/context.go), [internal/cognitive/agent.go](../internal/cognitive/agent.go)):** before each turn, if usage > 80% of budget, the model summarizes everything between the system prompt and the last ~6 messages into one dense message (goals, decisions, files touched, unresolved tasks, user's language) which replaces the span. The boundary always lands on a user turn (assistant+tool blocks never split); failure/empty summary is non-fatal (turn proceeds, char trim still guards); the fresh user message is added *after* compaction so it is never summarized. Known ceilings, deliberate: no mid-tool-loop compaction, and the chars/4 estimate over-counts Thai (3 bytes/char) so Thai sessions compact ~3× earlier — revisit only if it hurts.

**Verified live (2026-07-24):** real DeepSeek run — a secret fact stated in turn 1, four padding turns to 8,881/9,000 chars, compaction produced a 1,401-char summary replacing the original turns, and the model recalled the secret *from the summary*. Unit coverage: truncation/doom-loop/finish-reason (cognitive, model, turn), compaction trigger/boundary/failure (memory, cognitive).

---

## 21. Decision — Research Tool Suite: web_search, web_fetch, GitHub read, images end-to-end (2026-07-24)

**Trigger:** owner use case — "ค้นเว็บหาข้อมูล (มือถือ ฯลฯ) พร้อมรูปภาพ". Audit verdict: the browser suite could limp through search-by-hand, but image URLs never reached the model, and multi-page research meant driving visible tabs one at a time.

**Shipped (each its own commit):**
- `browser_read` now surfaces page images (rendered ≥64px, deduped, max 20, URL+alt) with a hint to embed via `![alt](url)` ([desktop/browser.go](../desktop/browser.go) textScript, [desktop/workbench.go](../desktop/workbench.go)).
- [internal/skill/web_fetch.go](../internal/skill/web_fetch.go) — headless page reading: `x/net/html` walk (already a dependency), scripts/styles stripped, links + images collected as absolute URLs, 40k text cap, http(s) only, output flagged as untrusted data (prompt-injection first line).
- [internal/skill/web_search.go](../internal/skill/web_search.go) — DuckDuckGo HTML endpoint, no API key; `uddg` redirect decoding; engine-level, so CLI gets it too. Known ceiling: DDG markup drift degrades to "(no results)", selectors live in one place.
- [internal/skill/github_tools.go](../internal/skill/github_tools.go) — `github_search` / `github_list_files` / `github_read_file` on the existing `githubRepoClient`; `GITHUB_TOKEN`/`GH_TOKEN` honored on every GitHub request (rate limit 60→5000/h, private repos); path traversal rejected before any request; all read-only network in `safety` (github_* prefix), `plugin_install` keeps its stricter assessment.
- Composer attachments now carry `[attachment: <type>]` source tags (user image vs dragged file-tab vs dragged browser-tab) — the model can no longer confuse provenance ([cockpit.svelte.ts](../desktop/frontend/src/lib/stores/cockpit.svelte.ts)).
- Chat rendering: `.markdown-body img` rules (single image capped 240px; multiple images in one paragraph flow as a wrapping gallery at ~⅓ bubble width); fenced code got the standard AI-chat chrome — language label + copy button + highlight.js (delegated clicks; DOMPurify still sanitizes).
- [internal/model/noop.go](../internal/model/noop.go) — noop is the UI test harness: picker models `aetox-image:test` / `aetox-think:test` / `aetox-markdown:test` (plus `img1/img5/imgbig/imgbroken` keyword cases) exercise every rendering path with no API key; `aetox-think:test` streams reasoning through `onReasoningChunk` before the answer. Later joined by `aetox-tools:test` (tool loop, todo panel, ask_user cards) and `aetox-subagent:test` (delegation end to end — §44.13).

**Status:** `Done 2026-07-24.` Full suite green; rendering verified live by owner (noop screenshots). Not yet built, deliberately: web_search fallback engine, per-domain fetch rate limiting, browser screenshots (WebView2 `CapturePreview`).

---

## 22. Decision — Capability Extension: video_ocr + computer, and the empty-reply nudge (2026-07-24, `computer` removed 2026-07-25)

**Trigger:** owner positioning — "โมเดลที่อ่านภาพ/วิดีโอไม่ได้ มาใช้ผ่านเราแล้วทำได้ เพราะสถาปัตยกรรมขยายศักยภาพให้โมเดล" — plus a live failure: ornith:9b returned an empty reply after a skill payload and the UI showed a bare `(empty response)`.

**Shipped:**
- Empty-reply recovery ([internal/cognitive/agent.go](../internal/cognitive/agent.go)): a blank model reply triggers ONE nudge. In the tool loop the nudge stays **inside the loop with tools intact** — a model missing a capability is told to reach for a tool that covers it, never to refuse; wording is tools-first. Only a second blank reply falls back to the fixed bilingual "เกินขีดจำกัดของโมเดลปัจจุบัน" line. Streaming path returns `streamed=false` on recovery so the UI renders the reply. Language handling is model-mirrored (reply in the user's language), no i18n table.
- [internal/skill/video_ocr.go](../internal/skill/video_ocr.go) — ffmpeg samples a frame every N seconds (default 5, cap 120 frames), each frame through the existing `runTesseract`, output as `[m:ss] text` with consecutive duplicates collapsed. Live test synthesizes a two-scene video and asserts text, order, timestamps, dedupe; Thai OCR verified exact on clean rendered text.
- `internal/skill/computer.go` (+ `_windows`/`_other`) — real mouse/keyboard/screen via Win32 SendInput, screenshot through PowerShell `CopyFromScreen`, RiskHigh in safety so every action prompted. **Removed 2026-07-25, see below.**
- Browser form-filling completed (owner: "คลิกในเบราว์เซอร์แม่นยำ กรอกแทนได้หมด"): `browser_type` now handles `<select>` (option matched by value/label via the native setter — previously the else-branch would have destroyed the options with textContent) and takes `enter=true` (synthetic keydown, then `requestSubmit` only if the page didn't preventDefault — untrusted key events never trigger implicit submission); `browser_read` lists each select's `[options: ...]`. JS logic proven by executing the generated script against a stub DOM (select match, no-clobber on miss, no double-submit). RTK-for-web idea assessed and rejected: rtk's filters are per-CLI-format ([internal/rtk/rtk.go](../internal/rtk/rtk.go) pipeFilters), web compression already lives in web_fetch.

**Status:** `Done 2026-07-24.` Live-verified on the dev box: cursor round-trip, real-screen screenshot → OCR read actual VS Code content. Not built, deliberately: scene-detection sampling and audio transcription for video_ocr (`ponytail:` comments mark the ceilings).

**Amendment 2026-07-25 — `computer` removed.** In real use the tool did not work from the desktop app (owner: "computer ใช้งานไม่ได้จริง"): calls like `screen_info` and `key win+d` came back failed while every other tool in the same turn succeeded. Deleted rather than debugged — the capability it sold (act on the real screen) is covered inside the app by `browser_*` for web work and by `shell` for the machine, and a tool that fails in front of the user is worse than one that was never offered. Gone: the five `computer*.go` files, its `RegisterDefaults` entry, its `safety.Assess` branch, and its `toolCallToArgs` case. Docs that sold it ([README.md](../README.md), [docs/index.html](../docs/index.html), [PLATFORM-SUPPORT.md](../PLATFORM-SUPPORT.md)) now lead with `browser_*` for the "hands" half of the story. If desktop control comes back, it comes back with a live end-to-end test on a real desktop, not just unit tests over the input-buffer builders — that was the gap that let a broken tool ship.

---

## 23. Decision — Windows Distribution: GitHub Releases → winget → Scoop; npm rejected (2026-07-24)

**Trigger:** owner — "เตรียมทำให้ติดตั้งได้ ผ่าน npm หรืออะไร เสนอมา อันไหนพร้อมและสะดวก วินโดวส์ก่อน".

**Channel ranking:**
1. **GitHub Releases + NSIS installer** — the foundation every other channel points at. The installer ([desktop/build/windows/installer/project.nsi](../desktop/build/windows/installer/project.nsi)) was already excellent: WebView2 runtime + Tesseract silently installed with pinned SHA256 + Thai traineddata. What was missing was purely the pipeline.
2. **winget** — widest reach (built into Win 10/11); needs one published release first, then `wingetcreate new <installer-url>` generates the manifest PR. Blocked on the LICENSE decision (winget requires a license field).
3. **Scoop** — zero-gatekeeper channel; manifest lives in this repo ([scoop/aetox.json](../scoop/aetox.json), `checkver: github` + autoupdate), users run `scoop install https://raw.githubusercontent.com/Mike0165115321/Aetox/main/scoop/aetox.json`.
4. **npm — rejected:** wrong audience and wrong shape for a desktop app (per-platform binary wrapper packages, no Start-Menu integration). Revisit only if the CLI ever targets JS-ecosystem devs specifically.

**Shipped:**
- [.github/workflows/release.yml](../.github/workflows/release.yml) — tag `v*` → windows-latest builds `wails build -nsis` (wails pinned to go.mod's version, NSIS via choco), portable zip, `checksums.txt`, attaches all three to a **draft** release (owner publishes manually). The CLI exe was dropped from the release in §30.
- [desktop/wails.json](../desktop/wails.json) `info` block (product name/version/copyright); CI re-stamps `productVersion` from the tag, local builds use the committed value.
- Proven locally end-to-end 2026-07-24: NSIS via `scoop install nsis`, `wails build -nsis` → `aetox-desktop-amd64-installer.exe` (12.4 MB) with correct version metadata (ProductVersion 0.4.0).
- [docs/index.html](../docs/index.html) — Thai landing page for GitHub Pages (Settings → Pages → main, `/docs`): download CTA on the stable `releases/latest/download/...` URL, scoop one-liner, capability-extension pitch with a terminal mock of the blind-model loop, honest SmartScreen note. Single self-contained file, no build step, one accent color, gridgeist-reviewed.

**Release checklist (owner):**
1. Bump `info.productVersion` in [desktop/wails.json](../desktop/wails.json) — that is the shipped version. `appVersion` in [cmd/aetox/main.go](../cmd/aetox/main.go) is the unshipped CLI's own string (§30); keep it in step with the product or leave it, nothing user-facing reads it.
2. `git tag v0.4.0 && git push origin v0.4.0` → CI drafts the release → review + publish.
3. First release only: copy the portable zip's SHA256 from `checksums.txt` into [scoop/aetox.json](../scoop/aetox.json) `hash` (autoupdate maintains it afterwards).
4. When LICENSE lands: `wingetcreate new <installer asset URL>` → PR to microsoft/winget-pkgs.

~~**Open decision (owner):** LICENSE file~~ — **closed 2026-07-25 (§28):** MIT, matching what README already claimed; `scoop/aetox.json` updated from `TBD`. The winget step above is now unblocked. *(Relicensed to Apache-2.0 in §60; releases up to v0.7.1 remain MIT.)*

**Not built, deliberately:** code signing (unsigned exe = SmartScreen "unknown publisher" warning on first run — Azure Trusted Signing ~$10/mo when distribution volume justifies it), macOS/Linux packaging (desktop is Win32-only today, §22).

---

## 24. Decision — Settings Parity Roadmap + Process-Tree Lifetime via Job Object (2026-07-24)

**Trigger:** owner — "เรามาทำให้มันพร้อมจริงๆกันดีกว่า ... ผมจะทำทั้งหมดครับ เอาให้เข้ากับบริบทเรา" หลังเทียบ Settings sidebar ของ ZCode กับของจริงในโค้ด (ผลสำรวจอยู่ใน `SETTINGS-PARITY-PLAN.md` ซึ่งเป็นเอกสารแผนของ decision นี้).

**Decisions:**
1. **ไม่ก็อป sidebar ของ ZCode 1:1** — เอาเฉพาะหัวข้อที่มีของจริงให้ต่อยอดหรือมีความต้องการจริง เรียงถูก→แพง (Skills/Plugins → Onboarding → Usage stats → Commands → Code preview → Subagents) ทุก phase จบแล้ว ship ได้เป็น commit แยก.
2. **Skills + Plugins = หน้าเดียว** — สองหัวข้อของ ZCode คือกลไกเดียวกันของเรา (โฟลเดอร์ `SKILL.md`; `plugin_install` ก็เขียนลง `~/.agents/skills/` อยู่แล้ว) การรวมยังปิด half-finished loop ของ `plugin_install` (ติดตั้งแล้ว re-discover ทันที ไม่ต้อง restart).
3. **Indexing page ตัดทิ้ง** — FTS5 (§ desktop/db.go) เป็น implementation detail ไม่มี knob ให้ผู้ใช้; ZCode มีหน้านี้เพราะทำ repo-wide RAG indexing ซึ่งเราไม่ได้ทำ. เพิ่มเมื่อทำ code indexing จริงเท่านั้น.
4. **Subagents ต้องผ่าน design ก่อนเขียนโค้ด** — `internal/orchestrator` (§10) คือ scaffold ที่รอเรื่องนี้; จะได้ Decision section ของตัวเองก่อน implement (walking skeleton: `task` tool, depth 1) — ไม่แตะ ensemble/routing ของ ADR 0002 ในรอบนี้.
5. **Child-process lifetime แก้ที่ root ด้วย Windows Job Object** — พบ orphan `node.exe`/`cmd.exe` (MCP servers) ค้างหลังปิดแอปจริงในเครื่อง owner คืนนี้. แทนที่จะไล่ทำ process-group cleanup รายทาง (upgrade path เดิมที่มาร์ก `ponytail:` ใน `internal/mcp/client.go`), ตอน boot ใส่ process ตัวเองเข้า Job Object ที่ตั้ง `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` — ลูกหลานทุกตัวจากทุกเส้น spawn (MCP, ConPTY shell, git, rtk, ffmpeg) อยู่ใน job โดยอัตโนมัติและถูกฆ่าทั้งต้นไม้เมื่อ process หลักตาย ไม่ว่าจะปิดดีๆ หรือโดน force-kill. จุดเดียวจบใน [internal/proc](../internal/proc) (แพ็กเกจเดียวกับ `HideConsole` ของ §fix `917b550`) เรียกจาก main ของทั้ง desktop และ CLI; no-op นอก Windows.

---

## 25. Decision — Subagents: the `task` tool (Proposed 2026-07-24, awaiting owner approval)

**Trigger:** `SETTINGS-PARITY-PLAN.md` Phase 6 — the last parity item, and the first real caller for the `internal/orchestrator` scaffold (§10) that has sat unused since it was built.

**Proposed design (walking skeleton, nothing more):**
1. **One new built-in tool `task`** — schema `{description: string, prompt: string}`. When the MAIN agent calls it, the executor spawns a sub-agent via `orchestrator.Spawn` with the same provider/model, a **fresh context** (no conversation history — the prompt must carry everything, same rule as every task-tool system), and the same skill registry **minus `task` itself** — depth 1 is enforced structurally, not by a counter.
2. **Lifecycle:** the sub-agent runs its own bounded tool loop; ctx cancellation propagates (Stop button kills the whole chain); its final text returns as the tool output; the orchestrator entry is removed on completion. Token usage flows through the same `SetUsageReporter` hook (§ Phase 3), so Usage stats absorb subagent spend automatically.
3. **Safety:** the sub-agent inherits the session's `ApprovalMode` and permission rules unchanged — approval prompts surface to the same UI. No silent privilege widening.
4. **UI:** `task` shows up as a normal tool step in the chat timeline (label = description). No new panel in v1.
5. **Settings:** single knob — enable/disable the `task` tool (default off until proven). Lives on the existing General page, not a new section.

**Explicitly out of scope (this decision):** ADR 0002's ensemble/routing/consensus, per-task model override (phase 2 if wanted), parallel fan-out, persistence of sub-agent transcripts, and any cross-process orchestration.

**Status: Approved 2026-07-26, folded into §44.** Read §44, not this section, before writing code — it is the same decision at implementation resolution, and where the two disagree §44 wins because it was written against the code. Note the one thing §44 adds that this section never had: **a primary-agent picker was built on top of this plan and then cut** (§44.0) — the main agent is the assistant, and only the sub-agent half of Phase 6 survives.

---

## 26. Decision — Browser Tab Errors Must Never Crash the App (2026-07-24)

**Trigger:** owner hit a live crash — `browser tab error: The group or resource is not in the correct state to perform the requested operation` (`ERROR_INVALID_STATE`, 0x8007139F) that took the whole app down ("เบราว์เซอร์ชอบมีปัญหา มันพาแอปเด้งหลุด กลัวคนอื่นจะมีปัญหาด้วย").

**Root cause:** `github.com/wailsapp/go-webview2`'s `Chromium.errorCallback` always calls `os.Exit(1)` after the error callback. `desktop/browser.go` installs a `SetErrorCallback` to log-and-continue, but that only swaps the inner callback — the `os.Exit(1)` fires regardless. Aetox embeds one WebView2 per browser tab, so any single tab's transient failure (RTSS DLL injection — the same RivaTuner that crashed the app's own webview in the install session, GPU driver, low memory) crashed the entire process. `os.Exit` can't be recovered in-process (skips defers, defeats recover).

**Decision — vendor + patch, matching the `conpty` precedent (§ `917b550`), owner-approved:** fork go-webview2 into [third_party/go-webview2](../third_party/go-webview2) via a `go.mod replace`. Four surgical `AETOX PATCH` edits (documented in [third_party/go-webview2/AETOX-PATCH.md](../third_party/go-webview2/AETOX-PATCH.md)): a caller-installed `SetErrorCallback` now owns the exit decision (default handler still exits, so the **wails main window is unaffected** — it never sets a custom callback); the controller-failure path early-returns instead of nil-dereferencing, unblocks `Embed`'s message loop, and makes `Embed` report `false` so `browser.go` tears down the orphan child window. Net effect: a browser tab that fails to embed just logs and disappears; the app lives.

**Rejected alternative:** per-tab WebView2 user-data folders (would remove *this* `ERROR_INVALID_STATE` trigger but not the class — other machines hit other transient webview errors that all route through the same `os.Exit`). The vendor+patch is trigger-independent.

**Not done:** upgrading past v1.0.22 (re-apply the four patch blocks when doing so); process-isolating the browser host (heavier, unnecessary now that errors are non-fatal).

---

## 27. Decision — Engine Parity Batch + Interactive UI Tools (2026-07-25)

One session, three fronts; OpenCode/Claude Code remain the reference implementations (per §20's owner directive). Everything below is unit-tested; the suite is green across `go test ./...` + vitest.

### 27.1 Engine parity fixes (OpenCode/Claude Code alignment audit)

An audit of model selection / tool loop / long-run behavior against the references found five deviations; all fixed in [internal/cognitive/agent.go](../internal/cognitive/agent.go), [internal/turn](../internal/turn), [desktop/app.go](../desktop/app.go):

1. **Ephemeral tool summaries** — `summarizeToolExecution` used to route through `Respond`, which wrote the summary prompt (kilobytes of tool output, as a fake user message) into conversation history forever. New `Agent.RespondEphemeral` completes over the context without writing anything — the references never let meta-work pollute the transcript.
2. **No duplicate user message** — the tool loop's first-call-failure fallback re-added the user message via `Respond(msg)`; extracted `respondFromContext` responds over history as-is.
3. **Per-provider output ceiling for plain conversation** — `Respond`/`RespondStream` were flat-capped at 768 tokens (truncating long answers for non-tool-calling models); now they use `toolLoopMaxTokens()` like everything else. A ceiling costs nothing until used.
4. **Mid-loop compaction** — §20.3's known ceiling closed: `compactIfNeeded` now runs every tool-loop round (OpenCode checks per step). `SplitForCompaction`'s user-turn boundary guarantees the in-flight turn's tool results are never summarized away.
5. **Model switch keeps tool history** — `applyConfig` now replays the *outgoing agent's real context* (tool calls, results, compaction summaries) into the fresh agent instead of the text-only transcript; falls back to transcript when no live agent exists. Project-switch paths are safe: every `reload` caller immediately `startNewSession`/`LoadSession`s, which resets context.

**Slow-tool guard (owner-requested):** a single chokepoint in [internal/turn/executor.go](../internal/turn/executor.go) `executeTool` runs every model-driven tool call under a 60s deadline, so one slow call cannot hang the turn (the 270s `grep "(?i)aetox" .` case). `noDeadlineTools` (`ask_user`, `task`, `task_result`) are exempt: waiting *is* their work.

**Parked, not abandoned (2026-07-29, owner-requested):** the deadline used to *cancel* the call and tell the model "abnormally slow … retry with a narrower scope". A 90-second transcription was therefore thrown away at 60s and started again from zero — twice the work, still no answer, and the user watching two identical rows in the timeline. Now the call is left running and the model reads *"STILL RUNNING … call the same tool with the same arguments to look in on it"*. `beginCall` keys pending calls by tool name + arguments, so the check-up finds the running call instead of starting a second one, and collects its real result the moment it lands; `forget` drops the entry on delivery so the next identical call runs fresh. Same shape as the two pairs already in the engine — shell's `run_in_background`/`shell_output`, and `task`/`task_result` — rather than a third mechanism. `maxToolExecutionTimeout` (10m) is the ceiling for a tool that will never finish; the turn's ctx (Stop) is still the brake. `shell`'s `timeout_seconds` therefore means "report back after", not "kill after". *ponytail:* a parked call the model never checks back on keeps its map entry for the session.

**Approval mode is live (2026-07-29):** `Executor.approvalMode` is an `atomic.Pointer` with `SetApprovalMode`, and [desktop/app.go](../desktop/app.go) `SwitchApprovalMode` now calls it instead of rebuilding the engine through `applyConfig`. It used to be fixed at construction, so the rebuild left the *running* turn on the old executor: the user clicked "full access" **because** a prompt was on screen, and that prompt — and every one after it in the same turn — kept asking. Known gap: `subagent.TaskOptions.ApprovalMode` is still a bootstrap-time snapshot, so a delegate started after a live switch runs on the mode the engine booted with.

### 27.2 Wire-format latent bugs (the 401 restart bug)

`c49ec3b`'s wire-format support had two endpoint-resolution bugs that only surfaced when the app finally restarted (preferences normalize the catalog-default BaseURL to `""` on save):

- [internal/model/anthropic.go](../internal/model/anthropic.go): empty BaseURL defaulted to `DefaultBaseURL("anthropic")` — the real api.anthropic.com — ignoring `cfg.Provider`. A restarted app sent the DeepSeek key to Anthropic → `401 invalid x-api-key`. Now defaults to the *provider's own* endpoint first.
- [internal/model/factory.go](../internal/model/factory.go): switching wire formats kept a stale other-format BaseURL (OpenAI requests aimed at `/anthropic/v1` and vice versa). The default-format URL now swaps to the alt endpoint on an alt-format switch and back; only a user-customized URL survives.

**Model discovery through the alt endpoint** ([internal/model/provider_catalog.go](../internal/model/provider_catalog.go)): Anthropic-format providers have no `/models` endpoint, so DeepSeek's model list was always empty. Discovery now routes through the OpenAI-compatible `AltBaseURL` when the primary runtime can't discover — chat stays on the Anthropic format (clean streamed tool calls), listing uses the alt endpoint, same account/key. Verified live against api.deepseek.com.

**Dev loop:** [desktop/wails.json](../desktop/wails.json) gained `reloaddirs: ../internal,../cmd` — backend edits outside `desktop/` now trigger the rebuild they always should have (the missing piece behind several "แก้แล้วหน้าจอเหมือนเดิม" sessions).

### 27.3 Interactive UI tools: `ask_user` + `todo_write` (desktop-side skills)

Two new skills in [desktop/ask_user.go](../desktop/ask_user.go), registered with the workbench tools (same pattern as `browser_*`) because they talk to the UI over Wails events:

- **`ask_user`** — the Claude Code AskUserQuestion pattern: the model blocks mid-turn on a question + 2–4 options; the chat renders stacked A/B/C/D option cards; the composer doubles as free-text answer input while pending. One question in flight at a time (second concurrent ask fails loudly); turn cancel unblocks; exempt from the slow-tool deadline.
- **`todo_write`** — the Claude Code TodoWrite pattern: the model maintains a task checklist (full-list replace per call; ○/▸/✓ statuses) rendered live in the turn bubble. `ponytail:` frontend-state only — persist with the session if surviving reloads ever matters.

**UI test model** ([internal/model/noop.go](../internal/model/noop.go)): `aetox-tools:test` added to the aetox picker — a stateless scripted turn (round derived from tool results in the transcript): todo_write → ask_user (really blocks) → todo_write all-completed → final text echoing the user's pick. `aetox-think:test` now streams ~6 numbered sections of long reasoning to exercise the unbounded panel.

### 27.4 Chat UX (owner-driven)

- **Thinking persists** — reasoning chunks are accumulated in `SendMessage` and saved on the session turn (`SessionMessage.Reasoning` + `ThinkSecs`, first→last chunk); finished messages show a collapsed "คิดเป็นเวลา Xs" toggle. Live panel unchanged; the 220px CSS max-height wall is gone (thinking renders at natural height).
- **Pinned auto-scroll** — while at the bottom, every live update (chunks, reasoning, tool steps, todos, ask panel) keeps the view pinned; scrolling up unpins, scrolling back re-pins. Reference behavior.
- **Stop button** — `CancelTurn` existed backend-side but *nothing in the UI called it*; the send button now becomes a red ■ during a turn.
- Copy button on AI replies; provider connection test (`TestProviderConnection`: a real 1-token completion through the same client chat uses — endpoint+key+wire format in one shot); chat model menu is picker-only (custom model ids live in Settings); sidebar projects fold their chat history by default.

**Not yet built (proposed, awaiting owner):** edit-previous-message + resend (needs history truncation), queued mid-turn send, retry button, `aetox-error:test`/`aetox-doomloop:test` UI test models.

---

## 28. Decision — External Review Response: Sandbox, Process and Read-Path Hardening (2026-07-25)

**Trigger:** an outside code review of the public repo (owner relayed it verbatim). Seven findings, all accepted, all fixed in one batch. Recorded here because three of them change security or tool-contract behavior, not just internals.

**1. `shell` output buffer had no ceiling** ([internal/skill/shell.go](../internal/skill/shell.go)). `limitLines` trims *after* the process exits, so between spawn and the 60s slow-tool deadline (§27.1) a runaway producer (`yes`, a looping log tail) grew a plain `bytes.Buffer` without bound — gigabytes of RAM inside a desktop app. Replaced with `cappedWriter`: first 1 MiB kept, remainder dropped, `dropped` folded into the existing `Truncated` flag so the model is still told output was cut. No mutex — `os/exec` collapses Stdout and Stderr to one pipe and one copy goroutine when they hold the same interface value, which is how `Execute` wires them.

**2. Cancelling a command orphaned its children** ([internal/proc/tree_windows.go](../internal/proc/tree_windows.go), [tree_other.go](../internal/proc/tree_other.go)). `exec.CommandContext` kills only the process it spawned, so Stop-during-`npm install` left npm, node and their children running. §24's Job Object was the *app-exit* answer and is still correct there — it just fires far too late for a user who pressed Stop. New `proc.KillOnCancel(cmd)`: Windows shells out to `taskkill /T /F` (no Win32 kill-process-group primitive exists short of a Job Object per command); Unix sets `Setpgid` and signals `-pid`. Both set `WaitDelay`, without which `Run` blocks past the kill on an output pipe a surviving grandchild still holds open. Wired into `shell` only — the other `exec` sites are short-lived and were not the reported problem.

**3. Sandbox containment was lexical, so symlinks escaped it** ([internal/skill/list.go](../internal/skill/list.go)). `resolveSandboxPath` cleaned the path and compared prefixes; a symlink inside the root pointing at `C:\Users` or `/etc` passed untouched, and the existing tests only covered `../`. Now both sides of the comparison run through `evalExistingSymlinks` — `EvalSymlinks` on the deepest *existing* prefix, since `write`/`edit` legitimately name a leaf that does not exist yet. The **returned** path stays lexical so callers and their output still show the path the user asked for. Same fix closes the reviewer's second-order bug: `withinRoot` lower-cases on Windows, because rejecting `C:\Work` under root `c:\work` on a case-insensitive filesystem is a false positive, not safety.

**4. `read` capped at 16KB — a correctness bug for a coding agent** ([internal/skill/read.go](../internal/skill/read.go)). An 800-line Go file exceeded it, so the model reasoned about code it had never seen and the truncation notice gave it no way to continue. Replaced with line paging à la Claude Code/opencode (§20's reference rule): `offset` (1-based) + `limit` (default 2000 lines, 256KB hard ceiling), and the truncation marker now names the exact offset to resume from. `bufio.Reader.ReadString` not `Scanner` — a minified bundle blows past Scanner's 64KB token limit. `fs cat` carried an identical 16KB ceiling and now shares the same `readTextLines`/`looksBinary` helpers: one truncation rule in the codebase, not two.

**5. No CI ran the tests** ([.github/workflows/ci.yml](../.github/workflows/ci.yml)). 71 test files and nothing executing them outside the owner's machine. `windows-latest` because that is what Aetox ships on. The frontend build step is mandatory, not decorative: `desktop/main.go` embeds `frontend/dist`, which is gitignored, so `go vet ./...` fails on a fresh checkout without it. Runs `go vet` + `go test ./...` + the vitest suite.

**6. The `engine/`/`providers/`/`cli/` module scaffold is deleted.** Four days of `go.mod`-only directories with zero source made first-time readers hunt for where the code lives, and §8's "migration drift risk" had no timeline behind it. Deleted along with `go.work`/`go.work.sum` — with the scaffold gone the workspace listed one module, i.e. nothing, and §7's open question about committing `go.work.sum` dissolves with it. The *reason* the split was proposed (`internal/model` importing `internal/provider`, §6.3) is unchanged and still recorded in [docs/architecture/module-split-2026-07-21.md](../docs/architecture/module-split-2026-07-21.md); re-scaffold when the migration actually starts.

**7. API keys are plaintext JSON at `0600`** — accepted as-is for v0.4, but it now appears in [README.md](../README.md)'s security table with the OS-keychain move (Windows DPAPI / macOS Keychain) named as the next step. The product pitch leads with "your data is yours"; the gap between that and the storage belongs in public docs, not only in a reviewer's notes.

**Also added:** [LICENSE](../LICENSE) (MIT). README already claimed MIT and `scoop/aetox.json` said `TBD` — without the file, "no lock-in" was legally untrue, since no license means all rights reserved. *(Superseded by §60: Apache-2.0 from v0.7.2 on.)*

### 28.1 What CI found in its first hour (the fixes the review did not ask for)

Standing up CI immediately paid for itself — three defects the review never saw, two of them pre-existing and one of them serious:

- **Every quoted shell command was corrupted on Windows** ([internal/proc/shell_windows.go](../internal/proc/shell_windows.go)). `exec.Command("cmd", "/C", line)` makes os/exec escape embedded quotes for the C runtime's argv convention, which **cmd.exe does not follow** — so `echo "hello world"` came back as `\"hello world\"`, and `git commit -m "msg"`, `python -c "..."`, `grep "a b"` were all silently mangled. Nothing caught it because every test used unquoted commands; it surfaced only when the new process-tree test tried to run a quoted path. Fixed by building the invocation through `SysProcAttr.CmdLine` as `cmd /S /C "<line>"` — `/S` is cmd's documented "strip the outer quotes, take the rest literally" rule. Unix is unaffected (execve takes an argv array) and now shares one `proc.ShellCommand` seam instead of an inline `runtime.GOOS` branch in [shell.go](../internal/skill/shell.go). Regression test: `TestShellSkillPreservesQuotedArguments`.
- **`go test` downloaded a third-party binary from the internet** ([internal/rtk/install.go](../internal/rtk/install.go)). §13.6's lazy auto-install fired inside the test suite on a clean CI runner — GitHub API call plus a multi-megabyte download nobody asked for, and network-flaky by construction. `resolve()` now returns empty under `testing.Testing()`; tests that need rtk skip.
- **`TestShellSkillRewritesToRTKWhenAvailable` guarded on the wrong thing** — `rtk.Available()` (binary resolvable) does not imply `rtk.Rewrite` has an equivalent for a given command, and `Rewrite` returning `ok=false` is a normal outcome, not a failure. It only ever passed because the owner's machine has rtk with a `git status` filter. Now guards on the rewrite itself.

- **The CLI did not build on macOS or Linux at all** — `internal/model/context_windows.go` holds model *context-window* token limits and has nothing to do with the OS, but Go reads a `_windows.go` suffix as an implicit build constraint, so every non-Windows build failed on `undefined: model.ContextWindowTokens`. Renamed to `context_window.go`. A cross-compile step now runs in CI, which also gives `internal/proc`'s `!windows` files (the process-group kill above) their first compile check — until now they had never been built by anything. This also answers §7's standing "is Aetox Windows-only by design?" question in the least flattering way: it was Windows-only by filename accident, not by decision.

**Verification beyond unit tests** (owner asked, and the answer was not "yes" until this was done):

- The process-tree fix has a real behavioral test, not a mocked one: `TestShellSkillCancelKillsGrandchild` runs the test binary itself as a grandchild through the shell, writing a heartbeat file every 150ms, then cancels and asserts the heartbeat stops. Checked for teeth by disabling `proc.KillOnCancel` — the test fails, `Execute` hangs on the output pipe the survivor holds, and the orphan locks its temp dir. That is precisely the reported bug, reproduced and then closed.
- **Measured, not assumed:** `resolveSandboxPath` costs **981µs/call** with symlink resolution vs **1.8µs** lexical (Windows; Defender scans every component open) — a 530× relative regression that is ~2ms per tool call in absolute terms, because it runs at most twice per call and never inside `grep`/`fs find`'s `WalkDir` (verified by reading both walk bodies). Left unoptimized with a `ponytail:` note naming the cache-the-root upgrade path.
- End-to-end through the real CLI binary (`aetox chat` + the built-in `aetox-tools:test` model), not just package tests: the `read` rewrite returns real file content through the actual dispatcher.

**Not done:** `proc.KillOnCancel` on the MCP/ffmpeg/tesseract `exec` sites (§24's Job Object still covers those at exit); per-command Job Objects instead of `taskkill` (needs a suspended-start to close the assignment race — real, but not worth it before someone hits it); the keychain migration itself.

**Addendum 2026-08-18 — the MCP line above is closed (§141).** `noctx` flagged the spawn, and the `ponytail:` note sitting on it had already described the leak: a server that forks leaves grandchildren the SDK's shutdown ladder never signals. `internal/mcp` and `internal/lsp` now build their commands with a client-owned cancellable context and `proc.KillOnCancel`, and `Close` sweeps with the new `proc.KillTree` after the protocol's own polite shutdown has run — cancelling after `Wait` returns is dead code, because os/exec has retired the goroutine that would call `cmd.Cancel`.

What "Job Object still covers those at exit" understated: it covers them **only** at exit, and only on Windows. Every mid-life teardown leaked — Settings' Test button, adding or removing a server, the `CloseShared` on a project switch — and off Windows there is no job object to be the backstop. The ffmpeg and tesseract sites named above stay as they were, deliberately: they are bounded by their own completion and already carry a context.

`TestEverySpawnCanBeStopped` is what keeps it closed. `TestEveryExecSiteHidesTheConsole` had been guarding every `exec` site for a console window since the day one flashed, and nothing was guarding the same sites for teardown — which is why the two that needed it were the two nobody was told about.

---

## 29. Decision — Windows First, Recorded Not Pursued; and the Sibling-Bug Sweep (2026-07-25)

**Trigger:** owner, after §28's cross-platform findings — *"ผมยังไม่อยากรีบโดดไป linux หรือแม็ค แต่ถ้าโครงสร้างมันพร้อม ก็แจ้งในเอกสารไว้ ... เอาวินโด้ให้ทำงานจนเสถียรได้ก่อนดีกว่า"* plus *"เจออะไรอีก อันไหนไม่กระทบมากแก้ได้แก้เลยแต่สำรวจดีๆ"*.

**Decision 1 — Windows is the platform; portability is a record, not a roadmap item.** [PLATFORM-SUPPORT.md](../PLATFORM-SUPPORT.md) states exactly what runs where and what each further step costs, and is explicitly *not* a plan in flight. This also closes §7's standing "is Aetox Windows-only by design?" question with evidence, in two parts: the CLI and engine were Windows-only **by accident** (a filename, fixed in §28.1) and now cross-compile under CI guard; `desktop/` is Windows-only **by construction** — it imports ConPTY and go-webview2, and `browser.go` calls Win32 with no build tag. Nothing beyond the cross-compile check was added for Unix; the `!windows` files still have never been executed anywhere.

**Decision 2 — chase the bug class, not the reported instance.** §28's review named the unbounded buffer in `shell`. Grepping every sibling that buffers a child process's output found the same defect elsewhere, and the sweep of every file tool against the real repository (not a synthetic temp dir) found three more:

1. **`git` had the identical unbounded buffer** ([internal/skill/git.go](../internal/skill/git.go)) — `git log` with no range, `git diff` on a large change, `git show` of a commit that added a binary. `cappedWriter` moved out of `shell.go` into [output.go](../internal/skill/output.go) next to `limitLines` and both tools now share it. `executeCommand` also reports whether the cap was hit, because a capped `git show` of a binary is a single very long line that `limitLines` would otherwise call complete.
2. **Every `git diff` on Windows was prefixed with CRLF warnings** — stdout and stderr were merged into one buffer, so `warning: LF will be replaced by CRLF` appeared once per touched file, glued to the front of the diff the model reads. On a large change that noise can push the real content past the 220-line limit. stderr is now separate and only surfaces when the command fails, which is the only time it carries the information.
3. **`edit` loaded any file entirely into memory** ([edit.go](../internal/skill/edit.go)) — `data`, the string conversion, the `Replace` result and the write-back are four live copies, so a few hundred MB of generated log was enough to end the process. A 16 MiB stat guard refuses early with a message telling the model what to do instead. No source file, lockfile or config comes near it.
4. **`write` and `delete` echoed the resolved absolute path** while `edit` echoed the relative one. Made consistent — the absolute form is token noise that also nudges the model toward repeating the sandbox root back at the user, against `internal/prompt`'s environment rule.

Checked and deliberately left alone: `image_ocr` (tesseract stdout is one image's text), `video_ocr` (ffmpeg runs at `-loglevel error`), `rtk` (bounded by the input it filters), `computer` (a PowerShell result). Also verified the project's own "every `exec.Cmd` passes through `HideConsole`" invariant (§`917b550`) still holds at every one of the eleven spawn sites.

**Measured, not assumed:** `read` costs ~0.65ms and shell ~39ms per call end-to-end on Windows (200-iteration benchmarks). Sandbox path resolution dominates the file tools at ~0.6–1ms — the cost of §28's symlink containment — which is noise beside a model round trip, so the cache-the-root upgrade path stays a `ponytail:` note rather than code. Shell is dominated by cmd.exe process creation, unchanged by anything here.

**Verified by sweeping the real repo, not fixtures:** read/list/grep/fs/git against the actual working tree and write/edit/delete against scratch — sandbox escape refused, binary detection correct, paging correct at an offset deep in a large file, git output clean.

---

## 30. Decision — The CLI Is Not a Product: Unship and Unadvertise, Keep the Code (2026-07-25)

**Trigger:** owner — *"CLI ไม่เคยเทสเลย ลบออกไปก่อนได้ไหม ตัวติดตั้ง โฆษณาด้วย ที่ผ่านมาโฟกัสแค่ตัวเดสท็อปอยู่"*.

**The problem was honesty, not code.** `cmd/aetox` compiles, has tests, and passes CI — but it has never been exercised as a product, while [README.md](../README.md) listed it as a shipped capability twice and the entire "เริ่มต้นใช้" section taught CLI flags as *the* way to start Aetox. The release attached `aetox-cli-windows-amd64.exe` to every GitHub Release. Users were being handed, and pointed at, something nobody had ever used end to end.

**Decision — stop shipping and stop advertising it; keep the source.** Owner picked this over deleting `cmd/aetox` outright, and it is the right call: the code costs nothing to keep, CI keeps it compiling and tested so it does not rot, and "ก่อน" (for now) stays literally true — bringing it back is a docs change, not a rebuild.

**Changed:**
- [.github/workflows/release.yml](../.github/workflows/release.yml) — the `Build CLI` step, the checksum entry and the release asset are gone; a release is now installer + portable zip + `checksums.txt`. Header comment states why, so the next person does not "restore" it.
- [README.md](../README.md) — the two feature-table rows claiming a shipped CLI are gone, and **"เริ่มต้นใช้" was rewritten around what actually ships**: installer link, scoop one-liner, portable zip, the SmartScreen note, and `wails build` for building it yourself. The old section was pure CLI flags, i.e. install instructions for a product that no longer exists. The `Flags` table went with it. `cmd/aetox` stays in the project tree, labelled as not-a-product.
- The closing "วันนี้คือ CLI ไม่กี่พันบรรทัด" line was cut whole (owner's call) rather than reworded — the paragraph reads fine without it.
- §23 above: shipped-assets list and release checklist corrected; the LICENSE open decision marked closed by §28.

**Explicitly unchanged:** `cmd/aetox`, `internal/app`'s terminal presentation, and `build.ps1`. §6.1's finding — that the GUI links CLI presentation code it never calls — is untouched and still open; unshipping does not make it worse, and deleting the CLI would have forced that cleanup in the same breath.

**What this costs:** `appVersion` in `cmd/aetox/main.go` is now the only version string nothing user-facing reads. Left in place rather than deleted, for the same reason as the rest of the CLI.

---

## 31. Decision — `audio_transcribe`: the Last Sense, and Why the Model File Is Not Bundled (2026-07-25)

**Trigger:** owner — a clip that is only speech, with nothing written on screen, returns an empty `video_ocr`. Aetox could read images and read text off video frames but could not *hear*.

**Shipped:** [internal/skill/audio_transcribe.go](../internal/skill/audio_transcribe.go) — ffmpeg strips a 16kHz mono WAV (`-vn -ar 16000 -ac 1 -c:a pcm_s16le`, one command that covers audio files and video files alike), whisper.cpp transcribes it locally, segments come back as `[m:ss] text`, byte-identical in shape to `video_ocr` so both can run over one clip and read as a single transcript. Language is `-l auto`; consecutive repeats collapse (whisper loops phrases over silence) and `limitLines` caps the output like every other tool.

**Local binary, never a cloud API.** Owner's constraint, and the only one consistent with the product: uploading a user's audio to transcribe it would contradict the single promise Aetox makes. Same shape as tesseract and ffmpeg — an external binary reached through `exec`, no new Go dependency.

**The ggml model is not bundled and not auto-downloaded.** base is ~142MB against a ~12MB installer that [docs/index.html](../docs/index.html) advertises by size — bundling would inflate the download twelvefold for a capability most sessions never use. Silently fetching it on first use was rejected too: spending 142MB of someone's bandwidth without asking is not a decision a tool gets to make. A missing model returns instructions instead — what to fetch, how big, and the exact path to drop it in (`<DataRoot>/models/`, the §14 data root). Any `ggml-*.bin` already present is accepted, so a user who chose tiny or small is not nagged for base.

**Safety: no new branch, on purpose.** `video_ocr` and `image_ocr` have no case in `AssessCommand` — they fall to the read-only default (`RiskLow`, no effects, never prompts). Adding a branch for `audio_transcribe` would have put it in a *different* tier than the tools it belongs with, so the change to [internal/safety/safety.go](../internal/safety/safety.go) is deliberately zero lines. A test in [safety_test.go](../internal/safety/safety_test.go) pins all three to the same assessment and the same `ShouldPrompt` answer in every approval mode, so a future edit cannot silently split them.

**Verification without the 142MB download.** The interesting failure mode is wrong whisper flags or a mis-parsed output format — neither of which unit tests over pure functions can catch, and the live test skips on any machine without whisper.cpp installed (including the dev box). So `TestMain` doubles as a stub whisper.cpp via the `os/exec` helper-process idiom: the test re-execs the test binary in the binary's place, and the stub **fails the test** unless it was handed an existing model, an existing converted `.wav`, `-l auto` and `-np`. ffmpeg conversion, argument construction, output parsing and temp-dir cleanup are all exercised for real on every machine that has ffmpeg. Proven to fail: flipping `-l auto` to `-l th` in the production path turns the test red.

**Status:** `Done 2026-07-25.` Full suite green. Not verified: whisper.cpp's own accuracy on Thai speech, and that the real binary's stdout matches the stubbed format — both need the binary + model on a real machine, which is a user decision, not a build step.

---

## 32. Decision — Assets: What a Tool Needs on Disk, Chosen and Fetched by the User (Proposed 2026-07-25)

**Trigger:** owner, immediately after §31 shipped — *"สกิลนี้ต้องเพิ่มทางเลือกแล้วครับ เขาจะเลือกโมเดลอะไรยังไง ... สกิลอื่นพร้อมให้โมเดลเรียกใช้ อันนี้มันต้องเป็นมากกว่านั้นแล้ว ... อนาคตเผื่อโมเดลอะไรใหม่ๆ มา ระบบต้องรองรับและขยายต่อได้ เราจะไม่ฝังโมเดลในระบบ เพราะมันจะหนักไฟล์ดาวน์โหลด"*.

**The distinction that forces a new concept.** Every skill in [internal/skill](../internal/skill) has exactly one audience: the model. `grep`, `shell`, `video_ocr` — the model calls it, it runs, done; no human prepares anything first. `audio_transcribe` has **two** audiences: the model calls it, but a *human* must first choose which ggml model to run (30MB and fast but poor at Thai, 141MB balanced, 466MB best and slow) and get it onto the disk. Aetox has no place for that second audience at all. §31's answer was an error message telling the user to go download a file themselves — which is a capitulation, not a design.

**It was never one tool's problem.** Three tools already depend on something Aetox does not ship, and all three answer it differently: `tesseract` (NSIS installs it silently at Aetox install time), `ffmpeg` (an error with a `winget install` line), `whisper.cpp` + its model (an error with a URL and a path). None of it is visible anywhere in the app, so "why doesn't this work" is answered only after the user has already tried and failed. Assets is where those three converge.

**Naming (owner picked, from three candidates):** **Asset** in code — `internal/asset`, `skill.Provisioner` — and **"ไฟล์ที่ต้องมี"** in the UI. Chosen over "Capability Pack" (nicer in a UI, but couples 1 pack to 1 tool when tesseract already serves two tools) and "Runtime / Local Model" (clearest for Ollama/LM Studio users, but "runtime" already means the WebView2 runtime in this project's installer vocabulary). Asset covers both model weights and binaries, which is what the catalog actually holds.

**Shape — the skill declares, something else provisions:**

```go
// internal/skill
type Provisioner interface{ Requirements() []Requirement }

type Requirement struct {
    ID      string   // "whisper-model"
    Kind    string   // "model" | "binary"
    Options []Option // what the user may pick between
    Active  string   // Option.ID currently on disk, "" if none
}

type Option struct {
    ID, Label   string
    Bytes       int64  // real size, shown before the user commits to it
    URL, SHA256 string // empty URL = not fetchable, instructions only (winget, brew)
    Note        string // "เร็ว แต่ไทยมั่ว" — why you would pick this one
}
```

A skill never downloads anything and never learns how. `internal/asset` owns the catalog, the download, the checksum check and placement under the §14 data root; the UI enumerates the registry for skills implementing `Provisioner` and renders one generic panel. The payoff is exactly what the owner asked for: **a new model is a new line in `Options`** — no UI change, no new code path — and a new asset-hungry tool is one method.

**Only the user downloads (owner picked).** No `asset_install` tool, not even one gated behind approval. Three reasons: spending 141MB of someone's bandwidth is a human decision that needs a progress bar and a cancel button, neither of which exists inside a tool call; a model handed an install tool will "helpfully" fetch on every failure, which is precisely what §31 refused to do; and the failure path already works — an error that names what is missing and where to go. A *read-only* "what is installed" tool may earn its place later; the install path itself never becomes a tool call.

**Never bundled — the constraint that holds all of this up.** The installer stays ~12MB, the number [docs/index.html](../docs/index.html) advertises. Sizes measured by HEAD request 2026-07-25, no download: `ggml-tiny-q5_1.bin` 30.7MB · `ggml-tiny.bin` 74.1MB · `ggml-base-q5_1.bin` 56.9MB · `ggml-base.bin` 141.1MB. Bundling base means a 12× larger download for a capability most sessions never touch; the quantized variants exist precisely so the catalog can offer a cheap first step.

**Checksum discipline is not new here.** [desktop/build/windows/installer/project.nsi](../desktop/build/windows/installer/project.nsi) already downloads Tesseract with a pinned SHA256; `internal/asset` inherits that rule rather than inventing one — a fetched asset that fails its hash is deleted, not used.

**First slice, proposed:** catalog + verified download + the whisper entries + `Requirements()` on `audioTranscribeSkill` + one settings panel, hung off the existing settings surface (`SETTINGS-PARITY-PLAN.md`). `tesseract` and `ffmpeg` join as instruction-only entries (`URL` empty) so the panel tells the whole truth from day one instead of showing a single lonely row.

**Status:** `Proposed 2026-07-25 — shape approved by owner (name + user-only downloads), not built.` Open: whether `Active` is stored in preferences or inferred from what is on disk (§31 currently infers — any `ggml-*.bin` wins), and what the panel does when two models are present.

---

## 33. Decision — `internal/stt`: One Language for Every Speech Engine (2026-07-25)

**Trigger:** owner — *"สถาปัตยกรรมเราต้องรับโมเดลอ่านเสียงได้หลายรูปแบบ ... ไม่ว่าจะใช้โมเดลจากที่ไหน สถาปัตยกรรมอะไร ควรจะถูกแปรเป็นภาษาเดียวกันเพื่อทำงาน ... เราพร้อมแค่ไหนครับ"* — with the follow-up that the skills settings page has to become somewhere a user can actually configure this.

**Readiness, answered with evidence before anything was written:**

| What it needs | State when asked |
|---|---|
| A proven translate-many-into-one pattern | ✅ [internal/model/factory.go](../internal/model/factory.go) already collapses 14 providers into 4 runtimes behind one `model.Provider`. The pattern was not hypothetical — it shipped. |
| Precedent for "same endpoint, different wire format" | ✅ `ModelWireFormat` in [internal/config](../internal/config/config.go). |
| A way to pass user configuration into a skill | ❌ **Zero.** `RegistryOptions` had exactly one field, `SandboxRoot`. Nothing could be configured per skill, by anyone, ever. This was the real blocker — not the engine work. |
| `audio_transcribe` accepting more than one engine | ❌ §31 hardcoded whisper.cpp: binary names, flags and output format all inline in the skill. |
| A settings surface to hang it on | 🟡 ~70%. [Settings.svelte](../desktop/frontend/src/lib/Settings.svelte) already builds its nav from an array and has a "skills" section — but that section only lists, installs and removes *external* skills. No built-in skill has ever been configurable. |

**Decision — copy the pattern that already works.** [internal/stt](../internal/stt) is `internal/model` in miniature: a `catalog` of `Descriptor`s (id, label, binary candidates, model glob, install hint), a `New(Options)` that switches on the descriptor and returns one `Engine` interface, and callers that never name a concrete engine. Adding faster-whisper, Vosk or sherpa-onnx is a catalog entry plus a constructor — no caller changes, no UI changes.

**The "one language" is `stt.Segment`** — `StartMs`, `EndMs`, `Text`. Every engine's output is translated into that, and nothing above the package knows that whisper prints `[HH:MM:SS.mmm --> ...]` on stdout while another engine will emit JSON. `audio_transcribe` shrank to what is genuinely its own job: sandboxing the path, producing a 16kHz mono WAV with ffmpeg, and formatting `[m:ss]` to match `video_ocr`. Milliseconds rather than a formatted string, because `[m:ss]` is one rendering choice, not the data.

**`RegistryOptions` gained `Speech stt.Options`** — the first configuration ever to reach a built-in skill from outside. That is the seam the settings UI plugs into; without it every UI design was impossible regardless of how it looked. `ponytail:` one field per configurable skill is fine at this count, becomes a map at three or four.

**Two model stores, never mixed** ([internal/stt/stores.go](../internal/stt/stores.go)) — owner: *"ตำแหน่งโมเดลของระบบเราด้วย แบบแยกมา จะได้ไม่สับสน"*. **Managed** is `<DataRoot>/models`, the single directory Aetox downloads into and is allowed to delete from. **External** is everything else — Ollama, LM Studio, the HuggingFace cache, or any folder the user points at — discovered automatically (`KnownExternalStores`, env-var aware: `OLLAMA_MODELS`, `HF_HOME`) and **read-only to Aetox forever**. A user with 40GB of models already downloaded should not download again; Aetox still does not get to own someone else's directory. `InstalledModel.Managed` carries that distinction all the way to the UI, so a delete button can only ever appear on a file Aetox put there.

**Verified live, with real speech, on 2026-07-25.** whisper.cpp 1.9.1 installed via scoop, `ggml-base.bin` (141.1MB) downloaded into the managed store, and Windows SAPI used to synthesize a sentence. The full tool returned:

```
[0:00] Hello. This is Edak's testing the audio transcribe tool. The architecture gives every model a pair of ears.
```

Which closes §31's open item: the real binary's stdout format is exactly what the parser expects, and the flags are right. (base mishears the invented word "Aetox" as "Edak's" — a model-quality fact, not a bug.) It also showed whisper writes `load_backend:` / `read_audio_data:` chatter alongside the segments even under `-np`; the parser was already immune, since it ignores every line that does not start with `[`.

**Test structure follows the split.** The engine's real command line is pinned in `internal/stt` by a helper-process stub that fails unless it is handed an existing model, an existing `.wav`, `-l auto` and `-np`. The skill's own tests use a fake `stt.Engine` and assert what the skill owns — sandbox refusal, WAV conversion of an `.mp4`, `[m:ss]` formatting, temp cleanup. Neither needs a model downloaded.

**Status:** `Done 2026-07-25.` Full suite green; live-verified above. Next, proposed not built: the settings UI for this (owner: *"UI หน้าสกิลน่าจะต้องออกแบบใหม่ให้มันว้าว เผื่อสกิลอื่นอาจจะมีเคสแบบนี้ในอนาคต"*) and the §32 Asset download path that would make the model arrive from inside the app instead of a manual download.

---

## 34. Decision — A nil Go Slice Is a Frontend Crash: Fix It at the Boundary (2026-07-25)

**Trigger:** owner, with a screenshot — *"ทำไมกดไม่ได้อ่ะครับ"*. Clicking **Settings → สกิล** highlighted the nav item but the right-hand panel kept showing the previous section. Nothing looked broken; the page just looked dead.

**It was not a UI bug.** The chain, confirmed end to end on the owner's machine:

1. `~/.aetox/skills` does not exist (no external skills ever installed).
2. `scanSkills` declares `var found []DiscoveredSkill` and never appends — idiomatic Go, returns **nil**.
3. `ListExternalSkills` passed that straight through, and a nil slice marshals to JSON **`null`**, not `[]`.
4. `onMount` does `extSkills = await ListExternalSkills()` → the `$state<SkillRow[]>` now holds `null`.
5. Nothing renders it yet, so nothing fails — until the skills panel is opened, where `{#if extSkills.length === 0}` throws `TypeError`.
6. Svelte 5 aborts that update **mid-flush**. The nav button had already been updated in the same flush and keeps its `active` class; the panel never re-renders.

Which is why it reads as "the button doesn't work" rather than "something crashed" — the only visible evidence is a highlight that leads nowhere.

**Decision — convert at the last Go frame, not in `internal/`.** Returning nil for "nothing" is correct Go and every Go caller handles it; `internal/skill` and `internal/command` are not wrong. What cannot survive nil is the JSON boundary, so [desktop/jsonslice.go](../desktop/jsonslice.go) adds `jsonSlice[T]` and the rule is: **a binding that hands a slice to the frontend returns it through `jsonSlice`.** Rejected: patching the six frontend load sites with `?? []`, which hides the next nil instead of fixing it.

**Swept the whole class, not the reported instance.** All 22 slice-returning `App` methods were checked. Exactly two were broken — `ListExternalSkills` and `ListCustomCommands` — and both are thin pass-throughs to an `internal/` function, added after the pattern was already understood elsewhere: `ListIdentityFiles` carries the comment *"non-nil so the frontend gets [] not null"* and every binding in `sessions.go` starts from `out := []T{}`. The knowledge existed in the codebase; nothing enforced it. So **Settings → คำสั่ง was one click away from the identical dead page** for any user with no custom commands.

**Enforcement is a test, not a convention.** [desktop/binding_slices_test.go](../desktop/binding_slices_test.go) reflects over the bindings a test can safely call, in the environment that produces the bug — empty data root, empty home, an empty non-repo sandbox — and fails on any nil slice with the fix named in the message. It found both offenders before either fix was written, and it is what stops the third one.

**Status:** `Done 2026-07-25.` Full suite green; desktop rebuilt so the owner can confirm the page opens.

---

## 35. Decision — Prompt Presets: Ship the Examples, Not an Empty Folder (2026-07-25)

**Trigger, in three steps, all in one sitting:**
1. §34's fix made Settings → คำสั่ง open for the first time. It showed *"ยังไม่มีคำสั่ง"* — an empty page.
2. Owner: *"ผมว่าเอาออกดีกว่า ผมเองยังไม่รู้เลยว่ามันเอาไว้ทำอะไรยังไง"*. The removal was written and staged.
3. Before it was committed: *"คิดไปคิดมาแม่งดีนะ เป็นเหมือนคลังเก็บพรอมต์คำสั่งดีๆ ใช่ไหมครับ"* — reverted with `git restore`, nothing lost.

**The lesson is the diagnosis.** The feature was never the problem. A working feature that shows an empty list, under a name that describes its mechanism ("คำสั่ง" / commands) rather than its value, is indistinguishable from a broken one — the owner who *built it* could not tell what it was for. It nearly got deleted for that alone.

> **Thai name changed again 2026-07-25 → "ชุดคำสั่ง"** (owner). Only the user-facing Thai word moved; English stays *Prompt presets*, and the code and folder keep `preset` / `prompts/`. The sections below keep the name they were written with — they are a record, not a spec.

**Renamed to what it is: "พรอมต์สำเร็จรูป" / Prompt presets.** Mechanism-names describe the implementation; this one now describes what the user gets. The rename goes all the way down rather than stopping at the label — `CustomCommand`→`Preset`, `ExpandCustom`→`ExpandPreset`, `ListCustomCommands`→`ListPromptPresets`, `<DataRoot>/commands/`→`<DataRoot>/prompts/` — so the next reader never finds product and code disagreeing. Nobody had a file in the old folder, so there is nothing to migrate.

**Bundled presets, compiled in.** [internal/command/presets/](../internal/command/presets) ships five `.md` files via `//go:embed`, so the page is useful on a fresh install with no folder created and no file written. Rejected: seeding the folder on first run — updates would never reach existing users, and a user who deletes a preset would watch the app recreate it, which is the app fighting its owner.

**A user file shadows a bundled one of the same name.** Editing a preset is copying it out and changing it; the app never argues. Aetox's own slash grammar still wins over both.

**The five, written from researched structure rather than invented.** Every source on prompt quality converges on the same five parts — role and goal, real context, hard constraints, an output format, and an instruction to verify — so each preset carries all five, and each one ends by consuming `$ARGUMENTS`:

| `/landing` | one-file landing page: scroll-reveal animation, `prefers-reduced-motion` honored, **no invented testimonials or metrics** |
| `/review` | code review biased to real defects: every finding needs file+line, a concrete failure scenario, and a fix — no style comments |
| `/debug` | root cause over symptom: reproduce first, grep every sibling caller, smallest fix at the shared frame, one test proven to fail |
| `/clip` | `audio_transcribe` + `video_ocr` over one file, read together — the §33 architecture as a one-word command |
| `/explain` | a map of unfamiliar code: entry point, data flow, the 20 lines that matter, the traps |

**Tests pin the promise, not the prose.** Every bundled preset must expand, must consume `$ARGUMENTS` (a preset that ignores its input is a snippet, not a command), must carry a description the settings page can show, and must work with no user folder present. Shadowing is asserted to replace rather than duplicate.

**Status:** `Done 2026-07-25.` Go and frontend suites green; desktop rebuilt.

---

## 36. Decision — Composer Palette: `+` and `/`, Split by What Enter Does (2026-07-25)

**Trigger:** owner, with a screenshot of Claude Code's action palette — *"ฟีเจอร์เริ่มเยอะละ เพิ่มปุ่มแบบนี้ดีไหม ... เพราะมันมีคำสั่ง มีสกิล"* — then a second screenshot of a `+` and `/` button pair: *"ทำเป็นปุ่มประมาณนี้นะครับ"*.

**What the survey found before anything was designed:**

| | State |
|---|---|
| Composer controls | 📎 attach · context-window ring · model chip · send. That is all. |
| Keyboard shortcuts | **Six already exist** — `Ctrl+,` `Ctrl+Alt+B` `Ctrl+Alt+S` `Ctrl+N` `Ctrl+T` `Ctrl+Shift+G` — spread across four components and **listed nowhere in the app**. |
| The `/` menu | Did not exist. The placeholder said *"(ใช้ / เพื่อดูคำสั่ง)"* anyway, and had since before there was anything to show. |

That last row is why this was not a nice-to-have. §35 had just shipped prompt presets invoked with `/`, into an app that advertised a `/` menu it did not have. The discovery surface was the missing half of the feature, not decoration.

**Decision — one component, two entry points, split by what Enter does.** Claude Code merges everything into one palette; Aetox cannot, because here `/` means *"write this into my message"* (presets expand engine-side, [internal/command](../internal/command)) while `+` means *"do this to the app"*. A list where Enter sometimes types and sometimes executes is a list nobody trusts, so [Palette.svelte](../desktop/frontend/src/lib/Palette.svelte) renders both from the same code and the same rows, and the mode decides which rows exist and what picking one does. Typing `/` in an empty composer opens the same list in prompts mode — the placeholder's promise, finally true.

**What the rows carry, and why:**
- **Current value on the right** — model, approval mode and think level live in three different places today; the palette is the first surface that shows all three at once, so it doubles as a status readout rather than only a launcher.
- **Shortcut on the right** — the only place the app has ever told anyone those six shortcuts exist.
- **A read-only tools row** — `ToolCounts()` counts the live registry by `Source` (builtin / skill / MCP) instead of a written-down number, so it cannot rot the way [internal/skill/README.md](../internal/skill/README.md)'s "15 built-ins" did (§31 commit). It answers "what can the agent do right now" at the exact moment the user is about to ask for something, which is where the architecture story belongs.
- **Cycling, not submenus**, for approval and think level: the value is on the row, three modes is short, and one click showing the next value beats opening anything.

**Grows by itself.** Presets come from `ListPromptPresets()`, counts from the registry. A new preset, skill or MCP server appears without the palette being touched.

**Another §34 offender found on the way.** `ListSkills()` returned a bare `nil` when the engine was not ready yet — same nil-slice-to-JSON-null crash, in a binding the tools panel already used. Fixed and added to `binding_slices_test.go`'s list, which is now the third defect that test has caught.

**Status:** `Done 2026-07-25.` Go and frontend suites green (21 frontend tests, 5 new for the palette: prompts-mode isolation, insert-with-trailing-space, live counts, static rows not behaving as buttons, filtering, Escape). Desktop rebuilt. Not built: `Ctrl+K` as a keyboard twin of `+` — three lines on the same component whenever it is wanted.

---

## 37. Decision — Preset Gallery: Cards, Covers, and Editable Where the Reference Sites Are Not (2026-07-25)

**Trigger:** owner — *"ผมอยากให้แบ่งเป็นบล็อคๆ ... ตอนเพิ่มพรอมต์ก็ให้ใส่รูปภาพได้ด้วย คนจะได้เห็นว่านี่คือพรอมต์สำเร็จรูป เหมือนเว็บที่พวกเขาแจกพรอมต์สำหรับแลนดิ้งเพจ ... กดมาหน้าพรอมต์สำเร็จรูป ต้องแสดงพรอมต์และแก้ไขได้"*, plus a request to go find the site he was thinking of.

**The reference sites, read rather than guessed:** [jiro.build](https://jiro.build/) is a card grid — cover thumbnail, title, category tag, copy button, click through to a detail page. [websiteprompts.com](https://websiteprompts.com/) is cards with tags and a truncated preview, no images. **Both are copy-only: neither lets you edit the prompt.** That is the gap worth taking, and it is free for us — our presets are already files the user owns (§35), so editing is the natural behavior, not a feature to bolt on.

**Cards, not rows.** A preset is chosen by looking at it, so [Settings.svelte](../desktop/frontend/src/lib/Settings.svelte)'s section is a `minmax(220px, 1fr)` grid: cover, `/name`, badge, two-line description. Clicking one opens it — name, cover, and the full prompt in a textarea, saved back to disk.

**Covers are optional and none ship.** A preset with no image gets a generated cover: hue derived from its own name, so the grid reads as a gallery on a fresh install with **zero bytes added to the installer** — the same constraint §32 set for models applies to decoration. A user cover is a real file next to the prompt (`<name>.png` beside `<name>.md`), inlined as a data URI, capped at 4MB so a camera original cannot stall the settings page.

**Editing a bundled preset writes an override, never the binary.** Saving `/landing` creates `<DataRoot>/prompts/landing.md`, which shadows the compiled one; deleting it restores what shipped. The UI says so on the editor rather than disabling the fields — the round trip is safe, so let people try it. Bundled presets have no delete button because there is nothing of theirs to delete.

**The name is a trust boundary, not a label.** It becomes a filename and a `/command`, so `ValidPresetName` rejects empty, `.`/`..`, path separators, Windows-reserved characters, whitespace and anything over 40 runes — tested against all of them, because this is the one field where user input turns into a path.

**Three more presets, sourced from the same reading.** Both sites split "whole page" from "one section", so the shipped set now covers both: `/hero` and `/pricing` are section-level, `/waitlist` is a whole page with a single job. Eight bundled presets, still under 30KB compiled in.

**Amendment, same day — real covers and a textarea that says something.** Owner on seeing it live: *"ผมอยากได้รูปจริงด้วยอ่ะครับ"* and *"เนื้อพรอมต์ ทำไมโล่งแบบนั้น ... ควรจะมีอะไรชี้ให้กดหน่อย มันเป็นเรื่องของ UX UI"*. Both were fair:

- **Generated covers are "no image, handled gracefully", not artwork.** All eight bundled presets now ship a hand-drawn SVG of *what they produce* — a wireframe for `/landing`, three tiers with the middle one lifted for `/pricing`, a waveform feeding `[m:ss]` lines for `/clip`, a symptom arrow tracing back to a root cause and out to sibling callers for `/debug`. Vector, **15.2KB for all eight**, installer unchanged at 12.52MB. A test asserts *every* bundled preset has one, because a grid where some cards have art and others have a coloured rectangle reads as broken rather than minimal. A user cover still wins over the shipped one.
- **A blank 300px box is a question with no hint.** A new preset now opens on the five-part skeleton every good prompt shares, so the work is editing rather than starting; the textarea keeps a placeholder for when it is cleared; and `$ARGUMENTS` — the one token a preset cannot work without and nobody remembers how to spell — gets a button that inserts it at the caret.

**Status:** `Done 2026-07-25.` Go suite green (7 preset tests: name validation, folder-created-on-save, override-then-delete restores the bundled original, cover round trip, cover not mistaken for a prompt, every bundled preset ships a cover, user cover wins); frontend 23 tests green (gallery renders shipped covers as images and falls back to the generated one, clicking a card opens its full text with the override note and a locked name, a new preset opens on the skeleton). Desktop rebuilt.

---

## 38. Decision — `+` Is Attach, `/` Is Prompts, Ctrl+K Is Everything Else (2026-07-25)

**Trigger:** owner, with a crop of the composer showing `+ / 📎` — *"บทบาท มันซ้ำกันครับ ลบออกสักตัว ... เอา + มาเป็นสำหรับโยนไฟล์ หรืออัปโหลด เหมือนที่ปุ่มอันนี้ทำอยู่"*, then on being asked: *"+ แนบไฟล์อัปโหลดไฟล์ เหมือน 📎 ไง และเอา 📎 ออกเพราะมันซ้ำกัน อัปโหลดวิดีโอหรืออะไร ... / เอาสี่เหลี่ยมมาครอบเหมือน Claude Code ก็ดี จะได้เห็นภาพ"*.

He was right, and §36 had caused it: `+` opened a palette whose Context group's first row was *"แนบรูป…"*, so `+` and 📎 were two doors to the same action sitting next to each other.

**Asked before acting, on his instruction** (*"ถามผมก่อนนะถ้าไม่มั่นใจ"*). What was certain — 📎 goes, `+` becomes attach — was never the question; where the palette's *other* rows (model, approval, tool counts, shortcuts) should live was, and guessing would have quietly deleted a surface.

**Result: three roles, no overlap.** `+` attaches. `/` lists prompt presets, in a bordered square so it reads as a key you press rather than a slash you typed — Claude Code's affordance, and what makes the button findable at all. `Ctrl+K` opens the same palette component in full mode, so nothing shipped in §36 was lost, only unbuttoned.

**Attaching is no longer image-only.** `SaveChatImage` already copied into `<sandbox>/.aetox-attachments/` and returned a relative path any sandboxed tool can open — the mechanism was general, only the door was narrow. It now backs `SaveChatFile` too: **streamed with `io.Copy` instead of `os.ReadFile`**, because the whole point of attaching a 1GB clip is that it is a clip, and a 2GB cap that loads into memory first is not a cap, it is a crash. A half-written copy is removed rather than left behind.

**A file is handed over as a path, never inlined.** An image already worked this way; a clip has no other option. The staged attachment names the tool that opens it — audio → `audio_transcribe`, video → *both* `audio_transcribe` and `video_ocr` (speech and screen, §33's pair), anything else → `read`. Naming the wrong tool costs a wasted turn; naming none makes the model guess.

**Status:** `Done 2026-07-25.` Go green; frontend 28 tests (5 new: extension classification, a clip staged as a path with a label, the hint naming `audio_transcribe` for audio and both tools for video, no fenced content in the message, removable before send). Desktop rebuilt.

---

## 39. Decision — The Guide Lives in the Locale Files, Not in the Engine (2026-07-25)

**Trigger:** owner, after settling that *"สกิลคือสิ่งที่โมเดลเรียกเองได้ ส่วนพรอมต์คือคำสั่งสำเร็จว่าเราจะเรียกตอนไหน"* — *"ตอนกด Aetox อ่ะครับ เวลามันตอบให้มันยื่นคำถามมาด้วย ทีละประเด็นเป็นไกด์ ... คำตอบสำเร็จรูปของ Aetox ต้องเปลี่ยนตามภาษานะครับ เช็คดีๆ ... เปลี่ยนชื่อด้วย Aetox 0.0.1 บ้าอะไร"*.

**The constraint decided the architecture.** A guide about Aetox is needed exactly when Aetox has **no model configured** — that is the whole point of the built-in engine. So the answers must be canned. And they must follow the UI language. But **nothing tells Go which language the UI is in**: `config.Config` has no locale field and no binding sets one. Plumbing one through `BootstrapOptions` → `ProviderOptions` → `NoopProvider` would be ~40 lines and a re-bootstrap on every language switch.

**So the guide lives in the frontend locale files.** Question and answer are both `t()` keys, which means language switching is not a feature that had to be built — it is the absence of a problem. `pushGuideExchange` appends the pair straight to the transcript; no model is involved, because there is no model to involve and nothing a model could add about software it has never seen.

**Chips appear only on the built-in engine** (`model.provider === 'aetox'`), one topic at a time, each disappearing once asked. A configured model answers for itself, and canned chips under a real reply would be noise.

**Six topics**, ordered by what a first-run user actually hits: skills vs presets (the distinction that prompted this), how to use presets, how to connect a real model, data safety, what the tools do, who built it and why. The first answer states the real difference — **who invokes it**, not "one calls tools" — because §35/§36's own code proves `markdownSkill.Execute` returns its body and calls nothing ([discovery.go:29](../internal/skill/discovery.go#L29)).

**Renamed `Aetox0.0.1:0b` → `aetox-review`.** Owner's call and overdue: the id was a fake version number in a family whose other members are `aetox-image:test`, `aetox-think:test`, `aetox-tools:test`. Renamed at the catalog and every test that pinned it.

**Found and reported, not silently fixed:** the onboarding greeting itself still cannot switch language, for the reason above — it is engine-side. Rather than leave English users with one trailing line, both halves are now complete on their own. Full switching needs the locale plumbed into Go, which is a separate decision to make deliberately rather than smuggle in here.

**Amendment — lettered cards, and the disappearing act.** Owner, with a screenshot of the `ask_user` panel: *"ควรจะแสดงแบบนี้ไม่ใช่หรอครับ ที่ผมบอก ABCD อ่ะ"*. His earlier *"แบบ A ..."* had meant **option A**, not "for example". The guide now renders in the same `.ask-panel` component the `ask_user` tool uses — lettered A/B/C/D rows inside an agent bubble, **reusing the existing markup and CSS wholesale**; the pill-chip styles written for it were deleted rather than kept alongside.

Correcting a claim made in the §40 summary: guide messages already in the transcript do **not** re-render when the language changes. `pushGuideExchange` stores the resolved string, and `ChatMessage.text` is a plain `string` — only the un-clicked options are `$derived` on `t()`. Retroactive translation was considered and rejected: a transcript records what was said, and no chat app rewrites its own history.

Checking that claim surfaced a real defect: `pushGuideExchange` only pushed into the in-memory array, while the transcript is persisted exclusively inside `SendMessage` ([app.go](../desktop/app.go)) — so **every guide exchange vanished on reload**. `AppendGuideTurn` now records it through the same `appendTurn` a real turn uses. It reads as part of the conversation, so it has to survive like part of the conversation; a silent disappearance is the one thing an onboarding guide must never do.

**Status:** `Done 2026-07-25.` Go green; frontend 33 tests (5: the exchange lands without a model, it is persisted, every topic has a real question and answer in **both** languages with missing-key detection, switching locale switches the answer text and not just labels, and the skills-vs-presets answer actually names who invokes each). Desktop rebuilt.

---

## 40. Decision — The Locale Reaches Exactly One Provider (2026-07-25)

**Trigger:** owner pushing back on §39's own excuse — *"คือตอนแรก มันยากมากใช่ไหมที่จะทำให้โมเดลมันเปลี่ยนตามภาษา ... ผูกแค่ aetox ก็พอ ผมกลัวเป็นหนี้ทางระบบมากเลย"*.

**The pushback was right and the estimate was wrong.** §39 claimed ~40 lines plus a re-bootstrap on every language switch. Counted properly: `BootstrapOptions{}` is built in **two** places, `NewNoopProvider` is called in **one** production place ([factory.go](../internal/model/factory.go)), and `applyConfig` **already** re-bootstraps on every settings change — so the "extra" machinery was machinery that already existed. Real cost: ~15 lines.

**Three options were weighed against debt, not line count:**

| | Approach | Debt taken on |
|---|---|---|
| **A** ✅ | Locale in the preference file → `BootstrapOptions` → `ProviderOptions` → `NoopProvider` | Almost none — the file already stores what the user chose, and language is what the user chose |
| B | The engine returns a sentinel (`[[aetox:onboarding]]`), the frontend renders it | **Real** — an internal token lands in the user's SQLite transcript forever; changing it later breaks old sessions |
| C | Do nothing; keep the greeting bilingual | None, but an English user reads a Thai paragraph first |

B looks shorter and is the trap: it puts an internal identifier inside user data, which is exactly the kind of debt that shows up on the day you want to change something.

**Why A is not layer pollution.** The owner's *"ผูกแค่ aetox ก็พอ"* is what makes it clean: `NoopProvider` **is not a model — it is an onboarding screen wearing a Provider interface**, the thing a user with nothing configured is talking to. Giving it the UI language is not "the engine doing i18n", it is a screen speaking to its user. Every real provider ignores `Locale` entirely, and a test asserts that.

**`SetUILocale` adds no new path.** It writes to `a.cfg` and calls `applyConfig`, which already persists preferences and re-bootstraps; it no-ops when the locale is unchanged, so switching Settings around never re-bootstraps for nothing. The frontend pushes the locale on every start as well as on change, so the two copies self-heal if they ever drift.

**Division of labour, now explicit:** prose the *UI* owns lives in the locale files (§39's guide chips — switching language re-renders them instantly, including history). Prose the *engine* emits — one greeting — takes the locale as data. Nothing else in the engine gets to have a language.

**Status:** `Done 2026-07-25.` Go green including 3 new tests: the greeting follows the locale with Thai as the fallback for empty/unknown, the locale survives the real path (`BootstrapProvider` → factory → provider) rather than only a direct field set, and a real provider never resolves to the built-in one. Frontend 32 green. Desktop rebuilt.

---

## 41. Decision — Every Built-in Model Speaks the User's Language, and an Instant Answer Scrolls to Its Top (2026-07-25)

**Trigger:** owner, three things at once — *"ตอนกดแล้ว มันดันไม่เด้งไปหน้าที่ AI มันตอบ ... เวลา AI ตอบควรพาไปโฟกัสที่ AI ตอบ"*, *"aetox อื่นๆ ก็ทำภาษาอังกฤษด้วยครับ"*, and *"aetox-review เปลี่ยนเป็น aetox-grid ดีกว่ามั้ง"*.

**The audit he asked for, run before answering:** counting Thai-bearing lines per canned surface in [noop.go](../internal/model/noop.go) showed **1 of 5 built-in models was language-ready** — the default (§40) — while `aetox-image:test` (11 lines), `aetox-think:test` (7), `aetox-tools:test` (5) and `aetox-markdown:test` (4) were Thai-only.

Three options were put up: translate all four, leave them and document them as dev instruments, or drop them from the model picker so a user never meets them. **Owner chose translation**, and that is the right call for a reason the "they're only test models" argument misses: they are listed in `RecommendedModels`, so they are *in the picker a real user chooses from*. Anything a user can pick is a surface, and a surface that answers in the wrong language is broken regardless of why it exists.

**Implementation is one seam, not four.** `NoopProvider.english()` and `pick(th, en)` — every canned string in the file now routes through them, including the `aetox-tools:test` **tool arguments**, since its todo items and `ask_user` options reach the user through panels rather than message text. A test walks the whole family: each model must answer with no Thai under `locale=en`, the tools model's tool arguments included, and Thai must remain the default for all of them.

**Renamed `aetox-review` → `aetox-grid`** (owner's call).

**The scroll bug was a wrong instinct, not a missing feature.** The chat already auto-scrolls, and the auto-scroll was working: `scrollTop = scrollHeight` on every transcript change while pinned to the bottom. That is correct for a *streamed* reply — following the bottom is following the text as it arrives. It is exactly wrong for an answer that appears **whole**: the guide answers are long, so landing at the bottom lands *past* them, on the options card underneath.

So `askGuide` takes the wheel: it drops the pin, then puts the **top** of the new reply at the top of the view — where reading starts. The guide card carries a `guide-card` class purely so the scroll target can tell an offer from an answer. Streaming behavior is untouched.

**Amendment — the guide is a menu, so it stops pretending to be a chat bubble.** Owner: *"ทำไมมันไม่แยกเป็น 2 แถวอ่ะครับ เช็คการแสดงผลดีๆด้วยนะครับ เผื่อขนาดเล็กๆด้วย"*. Six options stacked in a 72%-wide bubble is a tall thin column. `.guide-card` now takes the full reading width and lays its options out with `repeat(auto-fit, minmax(min(260px, 100%), 1fr))` — two columns while each can hold 260px, one the moment it cannot, no media query.

The `min(260px, 100%)` is the point, and it is the small-size check he asked for: a bare `minmax(260px, 1fr)` has a **hard** floor, so in a container narrower than 260px the track refuses to shrink and overflows instead of wrapping. The same guard was applied to the preset gallery's grid, and `max-width:100%` to the preset cover, which was a fixed 230px inside a wrapping flex. Every other fixed width added in this session is a 14–48px icon or badge — audited, no overflow risk.

Only the guide gets the two-column treatment: `ask_user`'s own panel keeps its single column, because those options are an answer being chosen, not a menu being browsed. The modifier hangs off `.guide-card`, so the shared component is untouched.

**Status:** `Done 2026-07-25.` Go green including the family-wide language test; frontend 37 green (4 new: the guide renders A–F on the built-in engine, stays hidden for a real provider, stays hidden mid-reply, and keeps the markup the grid is written against — jsdom applies no external CSS, so that last one pins the contract rather than the pixels). Desktop rebuilt.

---

## 42. Decision — The Guide Stops Being a Second Path (2026-07-25)

**Trigger:** owner, watching a guide answer appear — *"เวลากดอ่ะครับมันตู้มเดียวเลย ทำไมไม่เป็นแพทเทิร์นเดียวกันอ่ะ ผมกลัวจะมีหนี้ทางระบบมากถ้าสร้างแบบนี้แยกกันเยอะ"*.

**He was reading the symptom and diagnosing the cause correctly.** The answer arrived in one lump because §39 had built the guide as a **parallel path**: `pushGuideExchange` pushed two messages into the store directly, `AppendGuideTurn` persisted them separately, and neither touched the engine. Everything a normal reply gets for free had to be re-implemented or gone without — streaming was gone, persistence was a second Go binding, and scrolling needed its own special case because the normal auto-scroll assumed streaming.

**§40 had already removed the reason for the split.** §39 put the answers in the frontend locale files because Go could not know the UI language. §40 gave the built-in provider the locale. The workaround outlived its constraint by two sections.

**So the guide became a message.** [internal/model/noop.go](../internal/model/noop.go) owns the questions and answers in both languages; `GuideTopics(locale)` hands the UI a list; clicking one calls `onSend(question)` — **the same function the composer calls**. From there the answer streams word by word, is persisted by the same `appendTurn` a real turn uses, and scrolls by following the bottom like every other reply.

**Deleted, not moved:** `pushGuideExchange`, the `AppendGuideTurn` binding, its mock, the guide-specific scroll override (`pinnedToBottom` juggling plus a `scrollIntoView`), and 12 locale keys × 2 languages. "Already asked" is no longer state either — it is read off the transcript, so it survives a reload for free and cannot drift from what is on screen.

**One thing the collapse fixed on its own:** the guide question now matches *before* the model-specific test scripts, so clicking an option on `aetox-tools:test` answers the question instead of being hijacked by the tool-loop script. On the old path that bug could not even be expressed — which is the argument against parallel paths in one line.

**Status:** `Done 2026-07-25.` Go green with a new test walking every guide question in both languages: each must resolve (not fall through to onboarding), answer in the right language, still match after a language switch, and not be intercepted by the tools script. Frontend 33 green. Net effect on the tree: **365 lines deleted against 368 added**, and the added ones are almost entirely the answer text moving house.

---

## 43. Decision — A Fresh Install Shows Its Model Name (2026-07-25)

**Trigger:** owner — *"ผมให้ค่าเริ่มต้นไง ตอนเปิดมาครั้งแรกหลังติดตั้งอ่ะ ให้เป็น aetox grid"*.

**Probed rather than assumed.** A throwaway test against `resolveConfig` with no preference file reported `provider="aetox" model=""` — **the model name was blank on a fresh install**, so the composer chip fell back to showing the provider. The catalog has had `FallbackModel: "aetox-grid"` all along; it just never reached a first run.

**The cause was one deliberate exclusion** ([desktop/app.go](../desktop/app.go)): `if cfg.ModelName == "" && !strings.EqualFold(cfg.ModelProvider, "aetox")`. It dates from §27, when every aetox model was a test fixture and putting a made-up name in the picker would have been noise. That trade has since inverted — `aetox-grid` answers the guide (§42) and is what a fresh install actually runs on — so the exclusion is gone and aetox gets its catalog default like every other provider.

**Checked before removing, not after:** the noop provider switches on substrings (`image`/`think`/`markdown`/`tools`), and `aetox-grid` matches none of them, so the behavior on a fresh install is unchanged — only the name it reports.

**Pinned by a test, because a default that nobody looks at is a default that drifts.** `TestFreshInstallDefaultsToTheGuideModel` runs `resolveConfig` against an empty data root and asserts both provider and model name.

**Footnote on naming:** the model was briefly renamed to `aetox-guide` on a misread of this instruction, then reverted within the same turn. It stays `aetox-grid`.

**Status:** `Done 2026-07-25.` Go green, frontend 33 green, desktop rebuilt.

---

## 44. Decision — Sub-agents: the `task` Tool and Its Profiles (2026-07-26)

**Trigger:** `SETTINGS-PARITY-PLAN.md` Phase 6 — the last parity item, and the first real caller for the `internal/orchestrator` scaffold (§10) that has sat unused since it was built. Owner's framing: *"ซับเอเจน ที่คอยช่วยงานซ้ำหรืองานที่เอเจนเมนไม่จำเป็นต้องเสียเวลารันลูปเอง เวลา hit สูง เงินจะได้ไม่โดนผลาญ … มันต้องแสดง tool ด้วย ตัวเมนจะได้รู้ว่ามันทำอะไรอยู่"*.

### 44.0 The main agent is not chosen from a list — and the road to knowing that

This section spent a day being about two things and is now about one. The record matters more than the code that got deleted, because the same mistake is easy to make again.

**What happened.** Asked for "เอเจน 3 ตัว" (personal assistant, coding, planning) plus sub-agents, this document specified both: an agent layer the user picks from, and a sub-agent layer the agent delegates to. Both were built and shipped green — profiles, a preference, a picker in Settings, an agent badge on the composer chip. Then the owner looked at it in the app and cut the first half: *"ผมว่าตัดเลยดีกว่า เมนหลักไม่มีเอเจน เมนหลักคือตัวหลักจริงๆ มันมีผลเชิง UX ต้องถามก่อนว่าเราสร้างมันมาเพื่ออะไร ผมอยากให้มันเป็นผู้ช่วยส่วนตัวอัจฉริยะ การจะมาแบ่งเอเจนทำให้มันแตกไปหลายบุคลิก"*.

**Why he is right, in the terms this document should have used from the start:**

1. **There is already a layer that answers "who is the AI".** The identity directory (§11 — `identity.md`, `thinking.md`, `context.md`) rides into every project and every session. An agent profile carrying its own role prompt is a **second mechanism for the same question**, and the two would eventually contradict each other with no rule for which wins. That is the same class of debt the owner caught twice in one day inside this very feature (a `mode:` key that could disagree with its folder; two layers sharing one list).
2. **`build` and `plan` were never two personalities** — the source-reading note on agents (kept off this repo) says so in its own conclusion: a "mode" is a bundle of (permission ruleset + prompt), not a branch in the engine. But *calling* it an agent and putting it in a dropdown turns it into "who are you talking to", which is precisely the fragmentation the owner felt. Claude Code ships the same capability as **plan mode** (shift+tab) on one assistant; Codex ships no agent switch at all. Only OpenCode models it as a selectable agent — and following that one detail of the reference implementation cost more in product coherence than it bought.
3. **The requirement was never primary switching.** Phase 6 asked for sub-agents. The agent layer was scope this document added on its own.

**Settled: the main agent is the assistant.** One identity, configured by the identity files, never selected from a list. `internal/prompt` has no role-override layer and will not get one. Profiles exist for exactly one purpose — describing who the assistant *delegates to*.

**Kept as a possible future, deliberately not built:** "อ่านได้ แก้ไม่ได้" is genuinely useful during a large refactor. If it returns it returns as a **mode** — one toggle next to approval/think, the same assistant with its hands tied — reusing `Profile.DenyRules` + `FilterRegistry`, which already exist. Not an agent to pick.

**The deleted work is in `6f7ad46`**, committed before the cut on purpose: the reasoning above is worth more than the code, but the code is one `git show` away if a read-only mode ever wants it.

### 44.1 The survey: almost everything needed is already built and unused

| Needed | State (all `Direct`) |
|---|---|
| Multi-agent lifecycle | [internal/orchestrator](../internal/orchestrator/orchestrator.go) — `Spawn/Get/Stop/List`, **zero callers** since §10 |
| What a profile even is | the source-reading note on agents (kept off this repo) §1 — profile = prompt + model override + permission ruleset |
| Restricting what a delegate may touch | [safety.go:80](../internal/safety/safety.go#L80) `PermissionConfig.Resolve` — glob over tool + args, already allow/deny/ask |
| Showing tools live | [app.go:91](../desktop/app.go#L91) `recordToolAction` → `agent:tool` event, already a struct not a string (§27) |
| Per-sub-agent loop cap | `cognitive.AgentConfig.MaxToolCalls` exists; the main agent deliberately runs unbounded ([agent.go:191](../internal/cognitive/agent.go#L191)) |
| Absorbing sub-agent token spend | `SetUsageReporter`, wired at [app.go:1210](../desktop/app.go#L1210) |
| Ship examples not an empty folder | §35's `//go:embed` + user-file-shadows-bundled rule |
| Frontmatter `.md` parsing | [discovery.go](../internal/skill/discovery.go)'s parser, now exported as `skill.ParseFrontmatter` |

### 44.2 Profile format — one package, `internal/subagent`

```go
type Profile struct {
	Name        string   // file basename; also how `task` selects it
	Description string   // shown in the settings row
	Model       string   // "" = whatever model is selected; never forces a provider switch
	Tools       []string // "" = everything in the registry; non-empty = only these
	Deny        []string // → safety.PermissionRule{Tool: x, Action: Deny}
	Steps       int      // → AgentConfig.MaxToolCalls; 0 = 24
	Prompt      string   // the markdown body: the brief, not steps
}
```

Frontmatter keys `description/model/tools/deny/steps`, body = the brief. Rather than a second parser, [discovery.go](../internal/skill/discovery.go)'s became exported and generic — `skill.ParseFrontmatter(raw) (map[string]string, body string, err error)` — and its one existing call site reads `name`/`description` off the map. Comma-separated lists; still not YAML, same reasoning as the original. `name:` is **not read**: the filename is the name, because a key that can disagree with the file it lives in is the split §35 was written about.

**`Tools` is a real allowlist even though opencode has none, and the reason is cost, not safety.** A denied tool is still *sent* to the model on every round of the loop — deny only blocks execution. Cutting `explore` from ~25 tool definitions to 4 is a per-round saving on the delegate that gets spawned most often, which is the whole point the owner raised. Safety stays where it already is (`Deny` + the permission layer); this field is a token budget.

**Storage, per §35's precedent:** `profiles/*.md` compiled in via `//go:embed`; a user file at `<DataRoot>/subagents/<name>.md` shadows a bundled one of the same name; nothing is written on first run.

### 44.3 The two that ship

| Profile | Tools | Why it exists |
|---|---|---|
| `explore` | `grep`, `glob`, `list`, `read` | File-search specialist. The cheapest possible win: searching is what floods the main context worst |
| `general` | all minus the forced denials | The "งานซ้ำ" worker — can actually edit and run things, under a step cap |

Forced for every sub-agent regardless of profile: `task` (depth 1, structural — see 44.5), `help` (its registry pointer is the parent's), `ask_user` and `todo_write` (no human is attached to a sub-agent's loop; `ask_user` would block until the per-tool deadline, and a delegate writing the user's todo list is noise — same force-deny opencode applies).

### 44.4 `task` — where it lives, and why not in `internal/skill`

**It cannot be a built-in in [internal/skill](../internal/skill/defaults.go): `turn` imports `skill`, and `task` needs `turn` + `cognitive`.** It goes in `internal/subagent` (same package as the profiles — no reason for two), registered at bootstrap as `SourceExternal`, which is exactly how the desktop already injects `ask_user`/`todo_write`/browser tools (§27.3).

Schema `{description, prompt, agent}` — `agent` defaults to `explore`, enum = `subagent.List()`.

One call does:

1. **Build the child registry** — `FilterRegistry` over the parent's `Snapshot()` ([skill.go:135](../internal/skill/skill.go#L135)), keeping only what the profile allows. Depth 1 is then structural: `task` is simply not in the child's registry, so no counter can be got wrong.
2. **Build the child agent** — `cognitive.NewAgent` with the profile's brief, a **fresh context** (no history — the `prompt` argument must carry everything), `MaxToolCalls` from `Steps`, and `SetUsageReporter` pointed at the same hook as the main agent so Usage stats absorb the spend with no new plumbing.
3. **Run the full loop through the real executor** — `turn.NewExecutor` over the child registry, then `Execute(ctx, prompt, command.Intent{Raw: prompt, Kind: command.KindConversation}, nil, nil, nil)`. **The explicit `Intent` is load-bearing:** [executor.go:663](../internal/turn/executor.go#L663) returns a caller-supplied intent untouched, and without one, a prompt that happens to begin with a tool name ("read every test file and…") would be parsed as an explicit skill command and dispatched as a single tool call instead of a conversation.
4. **Permissions** — the session's deny rules plus the profile's, per the study's point 4: a sub-agent inherits its parent's prohibitions but never inherits its parent's permissions.
5. **Return** — `Result.Reply` (the final text, nothing else) plus one line of receipt: `[task explore: 7 tools, 12.4s]`.

### 44.5 Three gaps in existing files, fixed before `task` can work at all

1. **The 60-second per-tool deadline would kill every sub-agent.** [executor.go:35](../internal/turn/executor.go#L35) abandons any tool still running after 60s, and the only exemption is `ask_user` via `interactiveTools` ([executor.go:39](../internal/turn/executor.go#L39)). A sub-agent runs for minutes by design. Rename to `noDeadlineTools`, add `task`, and rewrite the comment to state both reasons (waiting on a human; running a whole nested loop) — the brake for both is ctx cancel, i.e. the Stop button, which already propagates.
2. **`ToolEvent` has no way to say who made the call.** Every event goes down one `agent:tool` channel ([app.go:94](../desktop/app.go#L94)), so a sub-agent's tools would land in the timeline indistinguishable from the main agent's. Add `Parent string` to [ToolEvent](../internal/turn/executor.go#L142); the child's `OnToolAction` stamps it with the `task` call's own id; the frontend nests those rows under the `task` row. `recordToolAction`'s Command History skips stamped events, or it becomes a flat dump of somebody else's work.
3. **Nothing hands a tool its own call id.** `Dispatcher.ExecuteTool(ctx, name, args)` doesn't carry one, and gap 2 needs it. `turn` exports `WithCallID`/`CallID` and stamps ctx at [executor.go:405](../internal/turn/executor.go#L405), immediately before `executeToolCallWithOutcome`. Two small functions, and the same seam serves any future depth accounting.

### 44.6 Correcting the premise: sub-agents do not save money by existing

The owner's goal — *"เวลา hit สูง เงินจะได้ไม่โดนผลาญ"* — is right about the mechanism but narrower than it sounds, and recording it here keeps a later reader from expecting savings the design cannot deliver.

**What actually saves:** a large tool result (a 200-line grep, a whole file) never enters the main context, so it is not re-sent on every subsequent round of the main loop. That resend is the real cost of a long loop, and it grows in a way a single call price does not.

**What costs more:** a fresh context pays the system prompt over again, and the child re-sends its own tool definitions every round of its own loop. Delegating a small task is strictly more expensive than doing it inline.

So the three things that make this a saving rather than a spend are all in the design above, and none of them is the `task` tool itself: `Steps` capping a loop nobody is watching, `Tools` shrinking what gets re-sent per round, and **the main agent seeing only the final text plus a one-line receipt**. That last one is the owner's *"มันต้องแสดง tool ด้วย"* split in two: the **UI** shows every sub-agent tool call live (free — it travels over the event channel, not the context window), while the **model** gets the summary. Feeding the tool log back into the main context would undo the entire saving in the name of showing it.

### 44.6.1 What stops the model delegating everything

Owner, right after it started working — *"งานเล็กๆน้อยๆที่เมนทำได้ ไม่ใช่โยนไปซับหมดนะ"*. Correct, and the first cut had only one weak line of prose against it. Two guards now, and it is worth being precise that **neither is enforcement**:

1. **The tool description states the rule and the reason.** WHEN TO USE: work that would otherwise pour into the conversation — hunting through many files for something you cannot name yet, the same mechanical change across many places. WHEN NOT TO: anything you can already name; one read, one grep, one edit, a handful of known paths. With the cost argument attached, because a rule with a reason survives a model's paraphrase where a bare prohibition does not: *a delegate pays for a second system prompt and its own tool list on every round, so a small job is strictly more expensive delegated*.

2. **The receipt judges the delegation afterwards.** A delegate that made ≤1 tool call comes back with `NOTE: that was one tool call — small enough to have done here … Do work this size yourself.` appended to its receipt, which the parent model reads mid-turn.

**Why after and not before.** How big a job turns out to be is not knowable from its brief: "find every caller of Resolve" is one grep in a small repo and forty reads in a large one. A pre-flight heuristic (brief length, paths named) would refuse real work *and* wave through pointless work, which is the worst of both. Measuring the delegation that actually happened costs one line and cannot be wrong about what it observed — and a model that reads "you could have done that here" stops doing it for the rest of the conversation.

**Deliberately not built:** a hard refusal. `task` never declines on size, because the guess would be wrong in both directions and a refused delegation is a turn the user waits through for nothing. The threshold is tool calls rather than seconds — one slow grep is still one call, and wall-clock says more about the disk than the work.

**Still unmeasured:** whether a real paid model actually respects either guard. The built-in provider always makes exactly one call, so the tests pin the *mechanism*, not the model's judgement. That answer needs `--live` or a real session, and it is the honest open item on this whole feature.

### 44.11 Delegation does not block the turn

Owner, once it worked — *"งานไหนคิดว่าควรจะส่งต่อให้ซับเอเจนให้มันพิจารณาถูกแล้วครับ แต่ตอนส่งงานไปให้ซับเอเจนมันจะต้องไม่เสียเวลารอนะ มันต้องไปทำอย่างอื่นได้ระหว่างรอ ไม่ใช่ให้มันโยนงานนะแต่มันต้องรู้ว่าต้องทำอะไร"*. The first cut was foreground and blocking (which §44.7 had listed as fine for a walking skeleton); that is now wrong.

**The shape, and why it is not "make the loop async".** The tool loop is synchronous by design — one round, then the next. Rather than reopen that, delegation splits into two tools:

| | |
|---|---|
| `task` | starts a delegate and **returns immediately** with a handle (`task_1`) |
| `task_result` | redeems a handle; waits only if that delegate has not finished |

The model stays in charge of what happens in between, which is exactly the distinction the owner drew: not *throwing work away*, but knowing what to do while it runs. Nothing in `cognitive` or `turn` changed to allow it — a background goroutine plus a handle was enough, because the loop was never the thing that had to wait.

**What falls out for free:**

- **Parallelism.** N delegates started before the first collect run at once, so wall clock is the slowest rather than the sum. This is §44.9's fan-out arriving as a property of the tool pair rather than as a second mechanism — and it needed no change to `internal/subagent` or to a child's loop, exactly as the study predicted.
- **Batch collection.** `task_result` takes several ids, so collecting three costs one round trip, not three.
- ~~**Turn-bounded lifetime.** A delegate's context descends from the turn's, so Stop cancels every outstanding one and none can outlive the reply it was meant to serve. No reaper, no leak.~~ **Superseded by §105 (2026-08-13).** This was a deadline, not a lifetime rule: an uncollected delegate's whole run was destroyed at the moment of the reply. A delegate's life is the session's now, and Stop — not the turn ending — is what ends one early.

**A cap, because a model in a loop is a real failure mode:** four delegates in flight per turn (`maxConcurrent`). Past that, `task` refuses with a message that says how to make room. This is the concurrency question §44.9 left open, answered with a desktop-sized number rather than a setting nobody would know how to tune.

**One real bug this introduced, found by reasoning rather than by the race detector** (`-race` needs cgo, which needs a C compiler. [verify.sh](../verify.sh) carries a `race` stage but **it skips unless `go env CC` is on PATH, and on the owner's machine it is not** — `go env CC` reports `gcc`, `command -v gcc` finds nothing, so the stage has been printing `skip` rather than running. An earlier version of this paragraph claimed the gap was closed on 2026-07-27; it was not, and the claim is corrected here rather than deleted because believing it is what let §74's audit read the standing CI failure as a race for six days. The only place `-race` has ever actually run on this repo is the `unix (ubuntu-latest)` CI job — see §75): tool events now arrive from a delegate's goroutine, and `App.toolHistory` was an unguarded slice written by what used to be the only turn goroutine. Two writers is now normal, so it takes a mutex — and the same fix applies to any App field a delegate's callbacks touch. `recordTokenUsage` was already safe (it only writes through `database/sql`, which is concurrency-safe).

**Known ceilings, deliberate:**

- **An uncollected delegate's work is thrown away** when the turn ends. Both tool descriptions say so; nothing pushes a finished result at the model, because that would mean synthesizing a message into a turn that may already have ended. Revisit only if a model demonstrably forgets.
- **`task_result` waits with no cap of its own.** The turn's ctx (Stop) is the brake, same as the tool loop itself. A model that collects too early waits — which is the cost of its own ordering, and the description tells it to work first.
- **The single-delegation case pays one extra round trip** versus the old blocking call. Accepted: the alternative is two modes of one tool, and the model can always collect immediately when it has nothing else to do.

### 44.12 Repeated work is one delegate looping, not one delegate per item

Owner, thinking past the feature — *"งานซ้ำอ่ะ ไม่จำเป็นต้องทำเอเจนแยก ให้มันลูปเลย เราทำให้ซับเอเจนทำหน้าที่นั้นได้เลยนี่หว่า"*. Right, and it is worth being explicit about **why nothing had to be built for it**: a delegate already runs a full tool loop of its own, so "do this to twelve files" is one brief with twelve items in it. The looping is the sub-agent, not a layer above it.

What that made necessary was a *guard*, because §44.11 had just made the expensive version possible: twelve delegates with one item each, each paying for its own fresh context — multiplying exactly the cost delegation exists to avoid. So:

- **`task`'s description states it as a rule:** *REPEATED WORK IS ONE JOB — hand the whole list to ONE sub-agent and let it loop; twelve items is one task with twelve items in its prompt, never twelve tasks.* With the cost reason attached, and the exception named (start several only when the jobs are genuinely unrelated).
- **`general` is now written as the looper it always was.** Its brief says a list is one job, work through it one after another in the same run, verify as you go, carry on past a failed item with the failure named, and — if it runs out of room — say exactly where it stopped so the work can be resumed rather than repeated. Its cap went from the default 24 to **48**, because a loop over a list needs more rounds than a search does.

**Two real bugs this exposed, both about a loop that ends without the delegate choosing to.** `cognitive`'s tool loop has two such endings — the `MaxToolCalls` ceiling and the doom-loop guard — and both are returned as ordinary *replies* rather than errors, because the user has to see them. For a delegate that is wrong twice over: the parent model got `agent tool loop reached maximum iterations` (an internal sentence) or `หยุดการทำงาน: …ลองสั่งใหม่หรือปรับคำสั่งดูครับ` (Thai prose addressed to a human), **and both came back marked successful**.

Fixed at both ends. `cognitive` exports the two sentinels (`ToolLoopExhausted`, `DoomLoopStopPrefix`) so a caller can recognise them **without matching prose** — the §27 lesson, where the frontend once decided success by matching a Thai word. `task` translates each into the next action: a step-cap ending says *split the work into smaller batches or raise `steps:`*, a doom-loop ending says *the brief was too vague, say concretely what to look at*. Both are failed results, so the parent cannot mistake either for an answer.

### 44.13 A sub-agent that gets stuck asks the main agent — `ask_main` / `task_answer` (2026-07-27)

Owner: *"ผมอยากให้มันเวลามีปัญหา หรือต้องตัดสินใจให้มันถามเมน"*. Until now a delegate could only guess or stop: `ask_user` is force-denied to it (no human watches its loop), `task` is not in its registry, and its whole vocabulary back to the parent was one final string.

**The shape is forced by the loop, not chosen.** The main agent's tool loop is synchronous — the only place a message can enter its head is as the result of a tool it called itself. A delegate therefore cannot interrupt; it can only be *found waiting*. So:

1. The delegate calls **`ask_main`**, which **parks its goroutine inside the tool call**.
2. The next **`task_result`** finds a question instead of an answer, and says which tool unsticks it.
3. **`task_answer`** supplies the reply; it becomes the return value of the parked call.
4. The delegate carries on **in the same loop**, with every file it read and every dead end it ruled out still in context. A second `task_result` collects the finished work.

**Parking rather than returning is the whole value.** A delegate that ended its run to ask would have to be re-briefed and started fresh — the work paid for twice: ten files read, then read again because the answer to *"which folder?"* arrived after they were forgotten. Nothing needed storing to make this work; the child's context stays alive because the child is literally still inside a tool call.

**The deadlock this had to avoid** is the obvious one and it is why `collect` was restructured: the delegate waits on the parent, so a parent that waited on the delegate would leave both parked until the user pressed Stop. `runner.collect` checks for an outstanding question **before** selecting on `done`, and returns the same question every time it is asked until it is answered — collecting a stuck delegate never blocks, however many times a confused model collects it.

**Distribution of the tool:** `ask_main` is injected into each child's registry inside the goroutine (it is bound to one delegation, so it cannot exist before it), **regardless of the profile's `tools:` allowlist** — asking touches nothing on the machine, and a profile that cannot ask is exactly the state this replaces. `task_answer` joins `task`/`task_result` in `forcedDenials`, for the same structural reason: a delegate must not answer a question meant for the main agent.

Both bundled profiles were rewritten — `general`'s brief said *"Do not ask for clarification, because nobody is watching to answer"*, which this makes false, and `explore` claimed a four-tool inventory it no longer has. Both now name the one case worth asking about (the brief means two different things / a problem it did not anticipate) and rule out checking in.

**Scripted-provider support:** the delegate script in [internal/model/noop.go](../internal/model/noop.go) asks first when the brief contains `askmain`, so the park-and-resume path is exercised in `verify.sh` with no key — opt-in per brief, the same way the image scenarios are opt-in per keyword, so every other test keeps its one-round delegate.

**The bench that was missing: `aetox-subagent:test`.** Scripting the ask behind a keyword made it *testable* but not *checkable by hand* — §45 says the built-in models double as the manual surface, and delegation had no entry in the picker at all, so nobody could watch the nested timeline without an API key. It is now the sixth built-in model ([internal/provider/catalog.go](../internal/provider/catalog.go)): pick it, send anything, and it drives all four rounds itself — `task` → `task_result` (question) → `task_answer` → `task_result` (finished work) — while the delegate runs `ask_main` → `list` underneath. Both sides run on it, since a delegate inherits the session's provider; which script a request gets is decided by whether it was offered `task`, which a delegate structurally never is. The parent script reads the handle back out of the tool result the way a real model would, rather than being handed it.

**Status:** `Done 2026-07-27.` Offline tests cover the three ways it breaks differently (collect-does-not-block, answer-reaches-the-same-run, refusing what is not waiting), plus one that runs the bench model through the real executor and asserts the four rounds in order — a bench that quietly stops working takes the manual check with it. Live-verified on deepseek-v4-flash: two config files, the delegate told not to guess, and exactly the one the main agent named came back edited.

### 44.14 Delegation reaches the CLI, and one over-narrowing reverted (2026-07-27)

Owner: *"เริ่มเลย สร้าง aetox สำหรับเทสซับเอเจนให้ผมด้วยนะ"*. Building a binary to test §44 with turned up the reason there wasn't one: **`task` was registered only in `desktop/app.go`**, so `aetox.exe` — the fastest surface to try anything on — could not delegate at all. Three things followed.

**1. The CLI registers the same three tools.** [cmd/aetox/main.go](../cmd/aetox/main.go) now does what `bootstrapFromConfig` does, with one difference that is the interesting part: it registers **after** `app.NewApp` so it can pass `Approve: aetoxApp.ConfirmApproval`. A delegate runs its own `turn.Executor`, and `approveOrDeny` reads a **nil `Approve` as approved** — so the desktop's `ApprovalFullAccess` was hiding a hole rather than choosing one. On the CLI a delegate now asks through the same y/N prompt the main agent does, and the user's approval mode means the same thing inside a delegation as outside it. That is a partial answer to the question §44.9 left open about approvals from children; the remaining half (N children prompting at once) is still open.

**2. The built-in model can drive the parent half.** §45 says whole-path tests run on `aetox-tools:test`, and it could not reach `task`: the CLI has no `todo_write`/`ask_user`, so its main agent landed on `noopDelegateToolsReply` — the *delegate* script — and never delegated. [noop.go](../internal/model/noop.go) gains `noopDelegationReply`: whoever holds `task` is a parent (delegates are force-denied all three halves), and a brief containing `subagent` gets `task` → `task_result` → report. Opt-in by keyword like the `askmain` script, so every other tools-test stays one round. The handle is **parsed** out of what `task` returned rather than assumed to be `task_1`, or a two-delegate turn would collect the wrong one.

Verified by running the binary, not only by test:

```
aetox --model-name "aetox-tools:test" --root <sandbox> "ส่งงานให้ subagent general ไปดูหน่อยว่ามีไฟล์อะไรบ้าง"
→ task(general) → delegate runs list → task_result → "[task general: 1 tool calls, 0.0s] NOTE: that was one tool call…"
```

The §44.12 receipt firing on a one-call delegate in a real run is the nudge working as designed.

**3. An allowlist for `general` was built and reverted — worth recording because the revert is the decision.** `general` inherits the whole registry (~25 tool definitions re-sent on every one of its 48 rounds), so a `tools:` line trimming it to find/change/verify looked free. It was not: it silently took `web_search` from a delegate, breaking the live test that asks one to research something, within minutes of being written. A profile named **general** that has to be told about each new tool goes stale the moment one is added — the default has to be *in*.

What shipped instead is `deny: plugin_install, delete` — the two with a reason that is not a token count, both being about **nobody watching the loop**: one changes what Aetox itself can do, the other is one-shot and irreversible. `shell` stays, so this is not a wall; it removes the two a model would otherwise reach for *by name*. `Deny` does both jobs (it trims the handed list *and* reaches `PermissionConfig`), which is why it is the right knob here and `Tools` is not. The token saving stays unclaimed because it was never measured.

**Known ceiling, now written down where it lives.** `maxConcurrent` counts delegates, not writers: four `general` delegates editing one file would interleave, and nothing in [runner.go](../internal/subagent/runner.go) stops them. The only guard is prose — `task`'s description and `general`'s brief both say a list goes to **one** delegate (§44.12) — which matches how delegation is meant to be used, so it holds until it doesn't. If two delegates ever genuinely need to write at once, the fix is a git worktree each, not a lock.

**Tests added since, across the dimensions the earlier ones left alone:**

- **The child's tool *loop*, not just its first call.** Every earlier test had the delegate run one `list`. A brief containing `toolchain` now walks `list` → `glob` → `grep`, and the test asserts all three ran **inside** the delegate (parent-stamped), that the receipt counted all three so the one-call nudge stays off real work, and that what reached the parent is the delegate's digest with **no raw tool envelope in it** (§44.6). The script reports byte counts rather than echoing results, which is what makes that last check able to fail.
- **Deny at both layers.** `general`'s two denials have to be gone from the child registry *and* present as `PermissionDeny` rules, so a discovered skill registering under the same name later cannot walk back in.
- **An oversize brief is refused, not truncated.** `memory.Context` trims the *last* message from its tail, so a brief bigger than the child's budget used to arrive cut off at an arbitrary point — and the delegate would work from half of it, confidently, with nothing telling it the rest existed. `task` now compares `len(brief) + len(profile.Prompt)` against `MaxChars` and refuses with all three numbers so the parent can split the job. The ceiling that bites first in practice is different and lives upstream: the parent has to *generate* the brief as tool-call arguments, so it cannot exceed one response's output budget (`cognitive.toolLoopMaxTokens` — 32K tokens, 64K on DeepSeek V4, 8K on the safe default).

**Status:** `Done 2026-07-27.` `go test ./...` green; CLI path verified by running the binary against the built-in model.

### 44.14 A delegation looks like a delegation — and three bugs that only harder tests found (2026-07-27)

Owner, on the timeline: *"UI ตอนซับเอเจนทำงานอ่ะ ควรจะมี 1 ซับเอเจน และชื่อซับเอเจนตัวนั้นด้วยดิ … tool ที่ซับเอเจนรัน ควรจะไปแสดงในซับเอเจนนั้นๆ ถ้าแสดงพรอมที่เมนสั่งซับเอเจนด้วยยิ่งดี"*. The timeline said **"ใช้ 6 เครื่องมือ"** for a turn where four of them were somebody else's work.

- **`ToolEvent` gained `Agent` and `Brief`**, set only on the `task` call that opens a delegation ([internal/turn/executor.go](../internal/turn/executor.go)). `turn` names one specific tool to fill them, which it otherwise never does: the alternative is a UI reading a delegate's identity out of English prose in a tool result. Nothing dispatches on the name — a mismatch costs a label, not a turn.
- **`description` joined `ArgSubjectKeys`**, last, so a `task` row reads as the job it was given. It is `task`'s own *"few words naming the job"* — written for exactly this line.
- **The timeline renders a delegation as a block** ([types.ts](../desktop/frontend/src/lib/types.ts) `groupSteps`/`isDelegation`, [Chat.svelte](../desktop/frontend/src/lib/Chat.svelte)): the sub-agent's name, the job, its own tool count, the brief the main agent wrote (clamped, full text on hover), and its steps inside. The collapsed line counts the two separately — **"ใช้ 3 เครื่องมือ · ซับเอเจน 1 ตัว"**. An orphaned child, whose `task` row is not in the list, stays visible at the top level: a row in the wrong place beats work that vanished.

**Sub-agents are their own panel, not rows in the tool list** (owner: *"ทำไมซับเอเจนกับใช้เครื่องมือดันมาอยู่เป็นก้อนเดียวกันอ่ะ ควรจะแยก เหมือนความคิดกับเครื่องมือดิ"*). Three toggles sharing one slot — **ความคิด · เครื่องมือ · ซับเอเจน** — because *what did it do itself* and *what did it hand to someone else* are different questions and one list answers neither. The split is total: a row counts as the agent's own only if it has no parent and is not itself a delegation, so nothing falls through and gets counted as the agent's work. A delegation is placed by the state of its own `task` row, never its children's — a delegate three tools in and still working is one running block, not three finished fragments and a live one.

**The bench delegate now does a real job**, since a bench exists to be watched: `aetox-subagent:test` delegates to `general` rather than `explore` and the delegate asks, lists, writes a summary file, reads it back and greps it — five rows under one block, ending in an artifact on disk. Each step is conditional on the tool actually being offered, so the same script degrades on a narrower profile instead of calling something it was never handed.

**The brief is the part worth arguing for.** It is the only thing on screen the user did not write and cannot otherwise see — the main agent wrote it, and it is the whole reason the delegate did what it did. Without it a delegation is a black box that happens to have a name.

**Then: "เทสหนักกว่านี้ได้ไหมครับ" — three real bugs, one per new test.**

1. **A question outlived its delegate.** Stop frees a parked goroutine but cannot un-ask its question, and `collect` checked for a pending question *first* — so after Stop the parent was told "it is waiting for a decision" about a delegate that was already dead, forever, and answering it failed. Fixed at both ends: `ask` clears the slot when its context dies, and `collect` treats finished-or-cancelled as outranking any question. The task's own context is what makes that exact instead of a race against the goroutine waking up.
2. **A row could never learn its name.** Argument order is the model's choice, and when it puts `content` first the `path` arrives in the very last fragment — inside the pacing window the previous update just opened. Pacing swallowed the one update that would have named the row, and it stayed unnamed for the rest of the turn. Caught live on a 9KB write. `toolProgressTracker.flush` now runs at `finalize`, forcing the window open once when the arguments are complete.
3. **A pacing test that measured the machine, not the rule.** `TestToolProgressTrackerPaces` fed 500 fragments and hoped they finished inside a real 200ms window; on a loaded box they did not, and the suite went red for no reason. `nowFunc` is now indirected, the test holds the clock still, and it checks the boundary in both directions — which the racing version could not.

**What "harder" bought, beyond those:** three delegates parked at once and answered in reverse order, each asserting it resumed on *its own* answer and saw no one else's; Stop while parked, with a goroutine count taken before and after; two concurrent delegations in the UI asserting no cross-contamination between blocks; and live, two delegates on the real internet at once collected in one `task_result` — 18s wall clock for a 4-tool and a 2-tool delegate, which is the concurrency claim §44.11 makes, measured rather than asserted.

**Then, 2026-08-18: one delegate hired, two cards drawn** (owner, on a screenshot of `explore` working with a nameless **ซับเอเจน** running beside it: *"ส่วนแสดงว่าซับเอเจนกำลังทำงานอ่ะ ซ้ำไหม"*). The second card was the agent's own `task collect` — the call it makes to sit and wait for the delegate it started 27 seconds earlier. `tool_runs` had one `action=start` in that window and no second one; the two cards' clocks (1m01s and 34s) differed by exactly the gap between the start and the collect.

The engine was never confused. `delegationOf` has excluded every non-`start` action since delegation was packed under one name (§99), and its comment names this exact card. But the answer had nowhere to travel, so [types.ts](../desktop/frontend/src/lib/types.ts) `isDelegation` re-derived it from the row's label — where a collect and a start both read `task` — and the second place answering the question is the one that won. §116's rule, in the small.

`ToolEvent.Delegation` and `ToolPart.Delegation` now carry it, as `*bool`: **nil is a third answer, not a quiet no.** A row is born from the streaming announcement, which fires while the model is still writing the arguments — and the action is *in* the arguments, so at that moment nobody can say. nil keeps the old label guess, which is right about every delegation and wrong only about the actions that are not one; once the executor speaks, its answer is final. Written down as well as emitted, or a reopened session would draw the card the live turn had just stopped drawing. Tests: three actions on the Go side asserting both the event and the stored part, and a render test that fails with 2 cards when the flag is ignored.

### 44.7 Out of scope for the walking skeleton

~~Parallel fan-out~~ (arrived with §44.11 — N delegates in flight, collected in one call), background tasks returning as a later synthetic message, persisting sub-agent transcripts, per-call model override beyond what the profile names, anything from ADR 0002 (ensemble/routing/consensus), cross-process orchestration.

**Reversed on the way (2026-07-26):** this section originally said `task` would ship **disabled by default** behind a settings toggle. It ships **on**, with no toggle. The owner asked for it working (*"เอาให้มันทำงานได้นะครับ"*), and the caution the toggle stood for is already carried by three things that are actually in the code — `Steps` capping a loop nobody watches, `Tools` shrinking what is re-sent per round, and the parent seeing only a summary. A toggle nobody knows exists protects nobody; if a kill switch is ever wanted it is a preference field and one line at the registration site.

### 44.8 Build order

1. ~~Decision~~ ✅ this section.
2. ~~`internal/subagent`~~ ✅ **done 2026-07-26.** [profile.go](../internal/subagent/profile.go) + [store.go](../internal/subagent/store.go) + [profiles/](../internal/subagent/profiles) (2 files, `//go:embed`), `List`/`Load`/`Dir`, `ReadRaw`/`Save`/`Delete`/`SetModel`, `AllowsTool`/`DenyRules`/`MaxToolCalls`/`FilterRegistry`, user shadowing, path-traversal guard on the name (it arrives from a model-written tool call). `skill.ParseFrontmatter` extracted. Tests: the bundled two, `explore` read-only and unable to recurse, `general` inheriting tools but not `task`, deny rules reaching `PermissionConfig`, step caps, shadow-replaces-not-duplicates, `SetModel` editing exactly one line, `FilterRegistry` never handing back the parent registry, traversal rejection.
3. ~~Settings → ซับเอเจน page~~ ✅ **done 2026-07-26.** [desktop/subagents.go](../desktop/subagents.go) bindings (list · read · save · delete · pin model · open folder) and one card in Settings per §44.10, each row carrying its whole configuration as badges. 4 frontend tests.
4. ~~44.5's three gaps, then `task` itself~~ ✅ **done 2026-07-26.** All three gaps closed: `interactiveTools` → `noDeadlineTools` + `task` (with `turn.HasNoDeadline` exported so the dependency can be asserted without sleeping 60s), `ToolEvent.Parent`, and [internal/turn/callid.go](../internal/turn/callid.go) (`WithCallID`/`CallID`, stamped one line before `executeToolCallWithOutcome`). The tool is [internal/subagent/task.go](../internal/subagent/task.go), registered as `SourceBuiltin` in `bootstrapFromConfig` with the live provider/registry/permissions so a re-bootstrap replaces it. 9 tests on Aetox's own model (§45): a delegate does real work and only its text + receipt comes back, its events are stamped and the parent's are not, `task`/`write`/`shell`/`help`/`ask_user`/`todo_write` are all absent from a child registry while `general` keeps `write`, bad input returns a *failed result* the model can read rather than an error, no `agent` defaults to `explore`, the schema names the profiles, a cancelled turn stops the delegate, and the deadline exemption holds. `recordToolAction` skips stamped events so Command History stays the agent's own log.
5. ~~Frontend nesting~~ ✅ **done 2026-07-26.** `ToolStep.parent` carries the stamp; a delegate's row renders indented with a `↳` mark; `applyToolEvent` matches rows **within their own scope**, because two delegates (or a delegate and the main agent) running `grep` at the same moment would otherwise claim each other's row. 2 frontend tests. **Enable toggle: not built** — see 44.7.

**Two bugs the tests caught, both real:**
- **A cancelled delegate reported success.** `exec.Execute` can return `err == nil` carrying the empty-reply fallback text after a Stop, and the first cut of `task` only checked `err`. Now `ctx.Err()` is checked first and independently: a stopped delegation can never come back as a successful one.
- **Aetox's own model called tools it was never handed.** `aetox-tools:test` scripted `todo_write`/`ask_user` unconditionally, but every delegate is force-denied both — so the built-in model could not drive the delegation path at all, which is exactly what §45 requires it to do. It now scripts from the tool list it was actually given (`noopDelegateToolsReply`), the way a real model does.

### 44.9 Parallel sub-agents — studied before building, per the owner

Owner: *"เมนยังสร้างเอเจนมารันขนานกันได้ในงานซ้ำ จุดนี้อยากให้ศึกษาจากคนอื่นก่อนลงมือทำ เห็นว่าเจ้าอื่นๆก็ทำได้หนิ"*. Read from his own library at `E:\MikeData\OpenSource-Study` (`14-Multi-Agent-Enterprise/`):

| Source | The pattern | What it implies for Aetox |
|---|---|---|
| **LangGraph** (`02-Team-Orchestration/LangGraph.md`) | Supervisor→worker; `Topic` does fan-out → fan-in (map-reduce) inside one superstep; nodes that don't depend on each other run together | The shape to copy: one `task` call that takes N briefs, spawns N children, waits, returns N results in order |
| **CrewAI** | Role-based crew + explicit process (sequential vs hierarchical) | Roles are our profiles already; the "process" is the parent's choice, not new machinery |
| **deer-flow** (ByteDance) | SuperAgent harness + sub-agents | Confirms the harness lives with the parent, not in each child |
| **OpenCode** (study §2) | `task` is foreground+blocking; background mode exists but sits behind an experimental flag | Foreground first is the reference behavior too, not a shortcut |

**What every one of them has in common, and it is the load-bearing observation:** the orchestrator and the workers are distinct things, and parallelism is the *orchestrator's* concern — never something a worker knows about. So parallel fan-out is an addition to the `task` tool's own signature (`prompts: []string` → N children → N results) plus a concurrency cap, and it changes **nothing** in `internal/subagent` or in a child's loop.

**Not designed yet, on purpose** — the questions it has to answer first, all of which need the serial version running to answer honestly: what a sensible concurrency cap is on one desktop machine, whether N children each holding a fresh context blows the provider's rate limit before it blows the wallet, how the UI shows N live timelines without becoming noise, and whether approval prompts from N children at once are usable at all (they are all `ApprovalFullAccess` on the desktop today, which hides the problem rather than solving it).

**Status: settled 2026-07-26 for §44.0–44.8; steps 4–5 pending; 44.9 studied, not designed.** `./verify.sh` green: vet · build · `go test ./...` · 70 frontend tests · svelte-check 0 errors · vite build.

### 44.10 The settings page, read off ZCode's rather than invented

Owner supplied a screenshot of ZCode's **Subagents** page as the reference, the same way §37 was given jiro.build. What it does, and what Aetox takes:

| ZCode does | Aetox |
|---|---|
| Two groups on one page — *User subagents* / *Built-in subagents* | **Take the grouping idea, drop the split** — one list, with built-in-vs-yours as a row badge, because a user file shadowing a bundled one is the *same* entry, not a second one |
| Row badges: model · `All tools` / `7 tools` | **Take verbatim** — those are `Profile.Model` and `len(Tools)`, computed from the profile the way `ToolCounts()` counts the live registry (§36), never written down. Plus the step cap, which is the thing that decides what a delegate costs |
| The file's absolute path under each row, in mono | **Take.** It is the cheapest way to say "this is a file you own" |
| *"Built-in profiles are runtime defaults and cannot be edited here"* | **Reject — this is the gap worth taking, and it is free.** Bundled profiles here are shadowable by a same-named user file (§35), so Edit on a built-in row opens the real text and saving writes your own copy. Same move as §37: editable where the reference is not |
| `Inherit default ▾` model dropdown | **Take**, implemented as the shadow: picking a model writes `<DataRoot>/subagents/<name>.md` with the bundled body and one changed `model:` line. No second override store |
| Per-row enable/disable toggle | **Defer.** With two profiles the single `task` on/off switch (§44.7) is the same control at less machinery |
| Search box + `All` filter dropdown | **Skip.** Two rows |
| Row shows no permission info | **Add what they lack:** a profile's denials are visible on its row, or a user will not understand why a delegate refuses to edit |

**Sidebar naming:** the page is `ซับเอเจน`, sitting next to สกิล / ชุดคำสั่ง / MCP, with its own *Open folder* button. Noted for the record: ZCode's sidebar also carries *Indexing*, which `SETTINGS-PARITY-PLAN.md` deliberately dropped (no user-facing knob, and Aetox does not RAG-index a repo) — seeing it in their UI does not reopen that.

**One thing the page must never grow:** a picker for the main agent. See §44.0.

---

## 45. Decision — System Tests Run on Aetox's Own Model (2026-07-26)

**Trigger:** owner, on being shown a sub-agent rehearsal driven by a hand-written fake provider — *"เวลาเทสอ่ะ เทสผ่านโมเดล Aetox นะครับทั้งตัวเมนและซับ"* … *"ตั้งค่าเลยเวลาเทสระบบให้ใช้ Aetox เป็นโมเดลสำหรับเทสยาวๆเลย เพราะเราให้มันผ่านช่องเดียวกันอยู่ละ"*.

**Settled:** any test that exercises a *whole path* — a turn, a tool loop, a delegation — runs on the built-in `aetox` provider ([internal/model/noop.go](../internal/model/noop.go)), model `aetox-tools:test` when tools are involved. Not a fake provider written for the test.

**Why it is the better default, and it is not just convenience:**

- **Same channel.** The built-in provider is a real `model.Provider` going through `cognitive.Agent` → `turn.Executor` → the registry, exactly like DeepSeek would. A per-test fake is a second implementation of the thing under test, and it passes even when the real path is broken.
- **No key, no cost, no network.** Which is what makes it usable in `verify.sh` on every run rather than behind `--live`.
- **Deterministic and stateless.** Its scripts derive the next round from the transcript, so they survive a re-bootstrap mid-run and never flake.
- **It doubles as the manual test surface.** `aetox-tools:test` is what a developer switches to in the app to exercise the tool UI by hand (§27.3, §42) — the same model, so a green test and a hand-check are looking at the same behavior.

**What this obliges the provider to do:** stay capable of driving whatever path is under test. It earned a second tool script the same day for exactly that reason — `noopDelegateToolsReply`, taken when the request's tool list lacks `todo_write`/`ask_user`, which is the case for **any** trimmed registry: a sub-agent (they are force-denied to every delegate) and equally the engine-only registry in a Go test, where those two live desktop-side. It runs one read-only `list` and reports the result. Scripting a tool the caller never offered would have tested nothing, because the call dies at dispatch — the same way a real model's would.

**Fakes are still right for one thing:** a test about a provider *edge case* — a truncated tool call, a leaked DSML block, a 401 — needs a provider that can produce that exact wire condition on demand. Those stay hand-written and stay small. The rule is about whole-path tests, not about never writing a stub.

**Where the fixture lives:** `model.NewNoopProvider("aetox-tools:test")`, no options needed. See `internal/subagent/spawn_demo_test.go` (since removed) for the pattern: build a registry, build a `cognitive.Agent` on the built-in provider, drive it through `turn.NewExecutor`, assert on the tool events and the final text.

**Status:** `Settled 2026-07-26.` Recorded in [TEST-REPORT.md](../TEST-REPORT.md) as the convention new tests follow.

---

## 46. Decision — Foreign Coding CLIs Are Consultants, Never the Engine (Deferred 2026-07-27)

**Trigger:** owner asked whether Claude Code / Codex / OpenCode could run through Aetox, so that someone paying for another subscription could try it — then parked the whole thread in favour of the automation direction.

**Settled, so it is not re-argued:** a foreign coding CLI may **never** occupy the `model.Provider` seam. They are agents, not models — one in that slot bypasses the registry, `safety`, rtk (§13), `task` (§44) and compaction (§20.3), leaving Claude Code in an Aetox window; and subscription auth inside a winget/Scoop-distributed product (§23) risks the *user's* account. The allowed shape is a **user-file sub-agent profile used as a consultant** (read-only, one `claude -p` call, `tools: shell`), never bundled and never an implementer. It does not solve "try Aetox without paying" — that is first-run onboarding over the free catalog entries, a separate decision.

**Full reasoning, the rejected alternatives, the profile sketch and its costs:** [docs/architecture/foreign-coding-clis-2026-07-27.md](../docs/architecture/foreign-coding-clis-2026-07-27.md).

**Status:** `Deferred 2026-07-27.` Nothing built; the profile written during the discussion was deleted so an unused delegate could not sit in `task`'s enum spending the owner's quota.

---

## 47. Decision — Typing While Aetox Works Goes Into the Turn, Not a Queue (2026-07-27)

Owner, for the second time: *"เวลาพิมพ์อะไรลงไป มันต้องส่งต่อได้ทันทีดิ ไม่ใช่แบบต้องรอให้มันทำงานเสร็จก่อนถึงจะส่งได้ ผมเคยบอกไปแล้วรอบนึง แต่ตอนนั้นไม่ได้ทำ"*. The first pass built the composer half only: a message typed under a running turn was parked in `queuedMessages` and fired as a **fresh turn** once the engine went idle. The composer stayed usable, which is what got checked — but the message still waited, which is what was asked about.

**Why it was a queue.** The engine holds one conversation and the tool loop is synchronous, so a second `SendMessage` into a live turn races the first and is lost. Parking it was the correct fix for *that* bug and the wrong answer to the request.

**What it is now.** The loop already has the only seam this needs — it comes back to the top on every round:

1. **`Agent.Interject(text)`** ([internal/cognitive/agent.go](../internal/cognitive/agent.go)) appends to a mutex-guarded buffer. It is called from the UI's goroutine while the loop sits inside a provider call on another.
2. **The loop drains it at the top of every round**, before the next request is built, so the model reads it alongside the tool results it just got rather than after the turn.
3. **An interjection keeps a finishing turn alive.** If the model returned text with no tool calls — it had decided to stop — the drain runs anyway, and a non-empty buffer makes the loop `continue` instead of return. Without this the common case is the lost one: users type while the answer is being written.
4. **`DrainInterjections` gives the host what was left.** A message can still land in the gap between the loop's last drain and the reply arriving; `App.SendMessage` takes it and emits `agent:interjection-missed`, and the composer's old queue — now only a straggler net — sends it as its own turn.

**No new message plumbing.** Consecutive `RoleUser` messages are already merged by the providers that require alternating roles ([convertMessagesToAnthropic](../internal/model/anthropic.go)), so an interjection following a tool result needs nothing special.

**It arrives marked, and the mark does not decide anything.** The first cut appended the raw text, which made it identical to a message typed before the turn began — read as a fresh instruction, *"ใส่สีน้ำเงินด้วยนะ"* is a reason to abandon a half-written file. The second cut over-corrected and hard-coded one answer ("treat it as an addition and carry on"), which is the §17 mistake: pre-judging on the model's behalf.

What `interjectionNote` supplies is the one fact the model cannot infer — **this arrived while you were working** — plus the choices, and then gets out of the way. The owner's description of what makes the feature good is exactly this judgement: *"โคตรเจ๋ง โมเดลฉลาดเลือกด้วยนะว่าที่บอกกลางคันนั้นคืออะไร จะทำตอนนี้เลย เช่นบอกปรับสี หรือไว้ทำตอนเสร็จงานใหญ่ที่ทำอยู่ก่อน"*. So the note names three dispositions — **small enough to fold in now** (a colour, a name, a correction), **a change to the job in hand** (adjust course), **separate or larger** (finish the current work first, then do it, and say so) — with one hard rule: only drop what it is doing if the message plainly says to stop.

The straggler path deliberately carries **no** note: by the time it is re-sent the turn really has ended, so it genuinely is a new request.

**Known ceiling: a message typed while a sub-agent is running waits for it.** `task_result` blocks inside a tool call, so the loop has not reached the top where the drain lives — the interjection lands the moment the delegate is collected, not while it is still working. Consistent with the design (the parent's loop is synchronous and §44.11 is built on that), and the parent is told to do other work rather than collect immediately, which shortens the wait. Revisit only if delegations get long enough that this is felt.

**What the user sees:** the bubble appears the moment they press Enter, and the answer arrives inside the turn that was already running — not as a second turn after the first one finishes. `sendUserMessage` therefore pushes the bubble and clears attachments **before** the `awaitingReply` branch, and returns without touching `toolSteps` / `streamingText`, which belong to the turn still in flight.

**Not done:** the CLI. `RunInteractive` blocks on readline, so there is no moment at which a second line can be typed — a different problem (input multiplexing) with a different answer.

**One bug the tests found, and it was pointing the wrong way.** The loop checks `ctx.Err()` *before* it drains, so a cancelled turn returned with the message still buffered — `SendMessage` then handed it back as a straggler and the composer sent **the very thing the user had just cancelled** as a fresh turn. `App.CancelTurn` now discards the buffer before cancelling. Stop meaning stop includes what was typed under it; the composer already cleared its own queue, and this was the half that had no owner.

**Status:** `Done 2026-07-27.` Coverage, by the way each part fails differently:

| Dimension | Test |
|---|---|
| Lands on the next round, **after** the tool result | `TestInterjectionReachesTheModelOnTheNextRound` |
| Keeps a finishing turn alive, earlier answer still in context | `TestInterjectionKeepsAFinishingTurnAlive` |
| Several at once: all delivered, in order, blanks dropped, never twice | `TestInterjectionsArriveTogetherInOrder` |
| Cancelled turn hands it back **unmarked**, fit to re-send | `TestACancelledTurnLeavesTheInterjectionForTheHost` |
| Quiet path allocates nothing; nil agent does not panic | `TestDrainInterjectionsIsEmptyWhenNobodyTyped` |
| All three dispositions survive an edit to the note | `TestTheMidWorkNoteLeavesTheChoiceToTheModel` |
| Binding hands text over, drops blanks, errors with no engine | `TestInterjectHandsTheTextToTheRunningAgent` |
| **Stop discards what was typed under it** | `TestCancelTurnDropsWhatWasTypedUnderIt` |
| Composer: immediate handover, no second `SendMessage`, bubble on send | frontend `queuedMessages.test.ts` |
| Straggler sends as its own turn with no duplicate bubble | same |
| An attachment attached mid-turn rides the interjection, not the next message | same |

---

## 48. Decision — §29 Reversed: the Linux/macOS Port Is Now the Work, Desktop First (2026-07-27)

**Trigger:** owner — *"ผมจะทำให้มันรองรับ mac และ ลีนุ๊กครับ"*, then *"ผมจะทำเดสท็อปก่อนครับ เท่านั้น"*, then *"เราจะสร้าง เวอร์ชั่น 0.7.0 ให้มันรองรับกัน"*. This supersedes §29 Decision 1 ("Windows is the platform; portability is a record, not a roadmap item"). [PLATFORM-SUPPORT.md](../PLATFORM-SUPPORT.md) changes role from record to live plan.

**Decision 1 — the port is phased, and CI comes before any port code.** Wails cannot cross-compile a GUI app, and the owner's Windows machine has no C compiler (`verify.sh` skips the `race` stage and says so). Together that means *nothing about this port is provable on the development machine*. The order is therefore: **0** CI matrix → **1** `terminal.go` split → **2** `browser.go` split with a stub host off Windows → **3a** Linux WebKitGTK host → **3b** macOS WKWebView host → **4** packaging. Phases 0–2 touch no GTK and no Objective-C but leave the app building, running and green on all three platforms minus the browser tab — so an abandoned or deferred phase 3 still leaves shippable work rather than an unmergeable branch.

**Decision 2 — the exported binding surface is identical on every platform; only implementations get build tags.** `desktop/frontend/wailsjs/go/main/App.d.ts` is committed and is generated from `App`'s exported methods. Tagging `BrowserOpen`/`TerminalStart` out of a non-Windows build would regenerate that file without them and break `BrowserPane.svelte`'s imports at `vite build` time, not at runtime. Every `App.Browser*`/`App.Terminal*` method therefore exists everywhere; the platform seam sits one level below, at the `hostBackend`/`tabView`/`ptySession` interfaces. This is the constraint that rules out splitting `desktop/` into per-OS directories: two `package main`s means two binding sets.

**Decision 3 — `do()` is asynchronous on every platform.** On Windows the browser host owns a dedicated STA thread and `do()` posts to it. GTK and Cocoa require the webview on the *main* thread, so off Windows `do()` becomes `g_idle_add`/`dispatch_async` — and must never become `dispatch_sync`. `browserSnapshot` ([desktop/browser.go](../desktop/browser.go)) calls `do()` and then blocks up to 5s waiting on a channel; a synchronous `do()` called from the main thread deadlocks the read path the agent uses for `browser_read`. Unit tests cannot see this.

**Decision 4 — Linux ships as `.deb`/`.rpm`/tarball, never AppImage.** [BENCHMARK.md](../BENCHMARK.md) §4's headline is 33 MB, 4–35× smaller than every competitor, and the stated reason is that Aetox uses the OS's own webview instead of bundling Chromium. That reason holds on macOS (WKWebView is always present) and holds on Linux **only if WebKitGTK stays a declared dependency**. An AppImage bundling it lands near 150 MB and deletes the product's headline number. Flatpak keeps the size but its sandbox fights what Aetox is — an agent that runs shells, git and MCP servers across the user's machine.

**Measured on real Linux (Docker `golang:1.25`, 2026-07-27), not cross-compiled:**

| Check | Result |
|---|---|
| `go vet ./cmd/... ./internal/...` | clean |
| `go test` — 23 packages | all `ok` — first execution of this code on Linux, ever |
| `go test -race` — 23 packages | all `ok`, zero races — the check `verify.sh` has never once been able to run |
| `TestShellSkillCancelKillsGrandchild` | **PASS (2.55s)** — `tree_other.go`'s `Setpgid` + `kill(-pgid)` proven against a real grandchild process, not merely compiled |
| `libwebkit2gtk-4.0-dev` on Ubuntu 24.04 | **does not exist** — 4.1 only, so the Linux job pins `libwebkit2gtk-4.1-dev` and phase 3a needs `-tags webkit2_41` |

The §29 line "the `!windows` files still have never been executed anywhere" is now closed with evidence rather than removed.

**Decision 5 (phase 1, measured) — off Windows a terminal tab is bounded by its *session*, not its process group.** `proc.KillOnCancel`'s negative-pid kill is the obvious thing to reuse when a terminal tab closes, and it is wrong here. Measured on a real Linux kernel, under bash *and* dash, with job control on and off: a backgrounded job always lands in a process group of its own while staying in the shell's session, so `kill(-shellPid)` reaches the shell and nothing it put in the background — `npm run dev &` outlives the tab and keeps its port. Nothing catches it later either, because `proc.KillTreeOnExit` is a no-op without job objects. `unixPTY.Close` therefore sweeps the session (`pkill -KILL -s <pid>`; `pty.StartWithSize` sets `Setsid`, so the session holds that tab and nothing else, and a session leader's id equals its pid on both Linux and macOS). `TestCloseKillsBackgroundedGrandchild` holds the line.

The blast radius was checked rather than assumed: `KillOnCancel`'s own callers ([internal/skill/shell.go](../internal/skill/shell.go), [internal/lsp/install.go](../internal/lsp/install.go)) run `sh -c` with no tty, which means no job control, which means background jobs *do* stay in the shell's process group — measured, and the kill reaches them. `tree_other.go` is correct for what calls it. The distinction to carry forward: process group is enough for a non-interactive `sh -c`, and never enough for an interactive shell on a pty.

**Decision 6 (phase 2, measured) — a test that isolates the config dir by setting `APPDATA`/`LOCALAPPDATA` isolates nothing off Windows.** Running `desktop/` on Linux for the first time turned four tests red, and none of them was about the browser. `os.UserConfigDir` reads `APPDATA` only on Windows; it reads `XDG_CONFIG_HOME` on Linux and `$HOME/Library/Application Support` on macOS. Nine tests across `desktop/` and `internal/config` were therefore reading and writing the developer's real `~/.config/aetox` on any non-Windows machine — which is how `TestLoadMCPServersMissingFileReturnsNil` began failing the moment another package's tests ran first and left a file behind. One `isolateUserDirs(t)` helper per package now sets all five variables, so the same test isolates on all three platforms. Related and same cause: `TestProjectKeyStableAndDistinct` asserted a `filepath.Base` result against a hardcoded `C:\projects\app`, which is one undivided name off Windows — it had been passing for the wrong reason.

The general shape is worth keeping: **an environment variable that names a Windows path is a portability assumption, and a test is where it hides longest**, because a test that silently touches the real user directory still passes on the machine that wrote it.

**Open cost, found while writing this and not yet paid — phase 3 needs a third vendored patch.** [docs/architecture/native-browser-embedding-2026-07-24.md](../docs/architecture/native-browser-embedding-2026-07-24.md) porting rule 1 is *"never locate your own window/view by title or any ambient global — hold a direct handle from the toolkit"*, written out of that session's failure catalog. Wails v2.13.0 exports no such handle: there is no `NativeWindowHandle`/`GetNativeHandle` anywhere in the module, and `pkg/runtime/window.go` stops at geometry. Following the rule means patching Wails in `third_party/` alongside conpty and go-webview2. `findOwnMainWindow()` in `browser.go` already violates the rule on Windows (it enumerates toplevels and matches by PID) — that is pre-existing debt the port should retire, not copy.

---

## 49. Decision — The Agent Can Finally Run What It Writes: `shell` and `git` Reach the Model (2026-07-28)

**The gap, and how long it hid.** [ADR 0001](../docs/adr/0001-native-tool-calling-foundation.md) set a phase-1 rule — "`shell` is not exposed as an automatic model tool", available "only through explicit user command paths" — and a phase-3 exit: "keep `shell` manual until narrower tools prove sufficient." The narrower tools shipped: `read`/`grep` (§15), `glob`/`apply_patch`/`diagnostics` (§21, §27). The exit condition was met and never checked, so for the whole life of the desktop product the agent could edit code and **never once run it** — no `go test`, no build, no linter, no package install, no `git diff` of its own work. Explore→Read→Edit→**Verify** was missing its last step, and the loop's own name says so.

Two things made this worse than an unfinished phase:

1. **The docs had already moved on.** §22's amendment justified deleting `computer` by saying the machine half was "covered … by `shell`", and §44 justified a sub-agent deny-list with "`shell` stays, so this is not a wall". Both describe a world where the model has the tool. Neither was true.
2. **The UI claimed it.** After `0ccee0e` split tools from skills, `ListTools` ([desktop/mcp.go](../desktop/mcp.go)) returns every registry entry whose `Source` is not `SourceSkill` — which includes `shell` and `git`, since they are `SourceBuiltin`. The model's list is built from a *different* filter: `ToolDefinitions()` ([internal/skill/dispatcher.go](../internal/skill/dispatcher.go)) keeps only entries that satisfy the `Tool` interface. Settings therefore listed a capability under "Tools" that was never sent. **`Source` says where a thing came from; `Tool` says whether the model can call it. They are not the same question, and the Settings page can only answer the first.**

**The change.** `ToolDefinition()` + `ExecuteTool()` on [shell.go](../internal/skill/shell.go) and [git.go](../internal/skill/git.go) — the two methods that were missing, nothing else about either tool touched. `shell` takes one `command` string; `git` takes an `action` enum plus `args`, with the enum built from `allowedGitReadActions` rather than restated, so the schema cannot drift from the allowlist it is supposed to mirror (sorted, because an unsorted enum reshuffles the tool payload every turn and misses the provider's prefix cache — the same reason `Registry.Names` sorts).

**What did not change is the point.** Everything that guards a shell command was already built and already in the path: `safety.AssessCommand`'s `EffectExecuteShell` branch with `isShellHighRisk` on the command word, `PermissionConfig.Resolve`'s per-tool patterns, the approval prompt, the audit log, `cappedWriter`'s 1 MiB ceiling (§28), `proc.KillOnCancel`'s process-tree kill (§28), rtk rewriting (§13.5), and the sandbox root. Under the default `ask` mode a human still approves every call. This decision hands the model a door that was already fitted with every lock — it had simply never been shown the door.

**The one non-obvious wiring detail.** `toolCallToArgs` ([internal/turn/executor.go](../internal/turn/executor.go)) translates a tool call into the `[]string` that `safety` reads, and it had no case for either tool. Without one, `AssessCommand("shell", nil)` takes the `len(args) == 0` branch and returns **RiskHigh, "shell with empty command"** for every call alike — `go test` and `rm -rf` rendered identically in the prompt, which is how a user learns to click through the one prompt that matters. So `shell` now passes `strings.Fields(command)` (tokenized, because `isShellHighRisk` reads the command word and its flags separately) and `git` passes `action` followed by its arguments — the same shape the text path produces, so one permission rule matches whether a human typed the command or the model called the tool.

**The pin that will catch the next regression.** `cliOnlySkills` in [desktop/tool_coverage_test.go](../desktop/tool_coverage_test.go) is now `{echo, fs, help}`, and both tools got a real case in the coverage test: `shell` runs `echo` and its output is checked, `git` runs `status --short` against a sandbox the fixtures now `git init`. That test failing is what would have caught this years earlier — it pins the hidden set, and the hidden set was simply never questioned once written down.

**Recorded, not fixed, because they are separate work:** a tool is abandoned after 60 seconds (`toolExecutionTimeout`), which a full suite on a large repo will exceed, and there is no background execution at all — no dev server, no watch mode, no long build. `shell`'s description tells the model both limits rather than letting it discover them as failures.

---

## 50. Decision — Four Parameters the Standard Tools Have and Ours Did Not (2026-07-28)

§49 asked whether the model has a tool at all. This asks whether the tools it has are as good as the ones it is used to. Every model Aetox drives has been trained against Claude Code's and opencode's tool schemas, so a missing parameter is not only a missing capability — it is a capability the model *expects* and works around badly. All four below were found by dumping Aetox's real tool definitions and diffing them against those two, parameter by parameter, rather than by reading our own source and deciding it looked complete.

**1. `edit` gained `replace_all`.** Renaming a symbol at ten call sites in one file previously meant ten `apply_patch` entries, each carrying enough surrounding context to be unique, or a whole-file `write`. The uniqueness guard stays the default and stays right — a model that meant one call site and matched eight has made a mistake worth stopping — so the error now names `replace_all` as the way through instead of only demanding more context. Line counts multiply by the number of occurrences replaced, because the timeline's "+9 −0" should describe the change that happened.

**2. `read` numbers its lines.** Both references hand the model `cat -n` output; Aetox handed it bare text, so the model could cite a location only by quoting the code back, and "which line is that" cost a second call to `grep`. Numbering is `%6d\t`, the file's own numbers rather than a count from the top of the page — paging that renumbered from 1 would make every citation past page one wrong. **The prefix is not in the file**, which makes it a hazard for the two exact-match tools: `read`, `edit` and `apply_patch` all now say so in their descriptions, and `read_test.go` pins the exact format, because a change to it silently breaks a promise those descriptions make. `fs cat` opts out via a new argument to `readTextLines` — it prints to a human who asked for the file, not to a model that has to cite it.

**3. `grep` gained `output_mode`, `head_limit` and `offset`.** "Which files mention this" is the commonest question asked of a code search, and answering it with every matching line costs one to two orders of magnitude more tokens than answering it with a list of paths — `files_with_matches` and `count` answer it directly. The paging pair fixes a genuine dead end: the 200-match cap had no way past it, so a search that hit the ceiling left the model inventing a narrower pattern, and the marker now names the offset to resume from. `head_limit` may only tighten the cap, never raise it. In the file modes the cap counts *files*, so a repo-wide search stops walking once it has the page it was asked for.

**4. `shell` gained `description` and `timeout_seconds`.** `description` is what the user reads on the timeline row — "run the speech tests" rather than a wrapped command line — which required `description` to outrank `command` in `model.ArgSubjectKeys`; `task` has no `command`, so nothing else moved. `timeout_seconds` is the first per-call override of the 60-second guard, and it lives in `internal/turn` because that is where the deadline is enforced, not in the skill: `toolCallDeadline` reads it for `shell` alone (nothing else knows how long its own work takes), clamps to ten minutes, and falls back to the default on anything absent, unparseable, zero or negative — a bad timeout is not worth refusing to run over. The 60-second default was already too short for this repo's own suite, which §49 recorded and this pays.

**Still open, and deliberately not bundled here:** background execution (`run_in_background` plus a way to read and kill a running command), images reaching the model at all rather than through OCR, and checkpoint/undo. Each is a design, not a parameter.

---

## 51. Decision — Models With Eyes Get the Picture; OCR Becomes the Fallback It Was Always Meant to Be (2026-07-28)

**What was wrong, and why it took this long to see.** §22 and §31 built four "senses" tools on a premise that was true when they were written: *most models have no vision at all* ([image_ocr.go](../internal/skill/image_ocr.go) says so in its own comment). `image_ocr` turns a screenshot into the letters inside it. For a blind model that is the whole picture. For every model shipped since, it is a picture thrown away — a UI bug screenshot, a diagram, a chart, a design mock all arrive as a handful of stray words. The owner named the trap exactly: the design solved *"make a model that cannot see understand the meaning"* so thoroughly that nobody went back and asked whether the models that **can** see were being served at all. They were not: `Message.Content` was a `string`, so no image could reach any provider by any route.

**The fix is not a second model.** Asking a separate vision model to describe the picture costs an extra round trip, an extra key, and — the part that actually kills it — that model has none of the conversation. It can answer "what is in this image", never "why does this layout look wrong given what we just changed".

**`Message.Images []Image`, `json:"-"`.** Raw bytes plus a media type, deliberately not base64: the three wire formats disagree about the envelope, and holding the encoded form would mean decoding it back to re-wrap it. The `json:"-"` is the load-bearing part — each adapter owns its own shape, and an adapter that has not been taught about images sends the text alone rather than an invalid body.

- **OpenAI-compatible** takes content parts, so `[]Message` could no longer be marshalled straight into the payload. `openAIRequestMessage` mirrors `Message` field for field, tag for tag **and in the same order**, differing only in `content` being `any`. Go emits keys in field order, so a message with no image serializes to exactly the bytes it did before — pinned by a test that marshals both and compares, because the conversation prefix is what providers cache and a reordered key would miss that cache on every turn for every user.
- **Anthropic** already built `[]anthropicContentBlock`, so this is one new block type plus a nested `source` object. One trap: an image with no caption must **not** carry an empty text block — the API rejects it.
- **Ollama** wants bare base64 in a sibling `images` field, no media type, no `data:` prefix. Getting that wrong is an image silently ignored, which is why it has its own test.

**`ResolveVision(provider, model)` decides, and unknown means blind.** Modelled on `ResolveThinkingCapabilities`: matched on substrings of the model id rather than a table of exact ids, because ids churn weekly (dated snapshots, `:free`, `:nitro`, `:q4_K_M`). Text-only markers win over family markers, so a family with one sighted member and one text-only member does not call both sighted. An unrecognized model is treated as blind on purpose — that costs a working OCR path, while the other way costs an image silently dropped and an answer invented from the caption, with nothing in the transcript to say so.

**Threading, and the one rule that matters.** `TurnOptions.Images` carries them, set per `ExecuteWithImages` call and never on the executor, so an attachment cannot leak into the next question. In `cognitive`, `addUserTurn` attaches them to the message that **opens** the turn and to nothing else: every later `RoleUser` entry in the same loop — an interjection, the empty-reply nudge, the DSML nudge — is Aetox talking to the model, and re-attaching there would resend the image every round. `memory.Context.AddMessage` rebuilds messages field by field and had to be taught the new one, or the image would vanish between the composer and the provider with no visible symptom.

**Where the decision is made.** `App.visionAttachments` ([desktop/app.go](../desktop/app.go)) reads the marker line the composer already appends, and either loads the bytes and rewrites that line — a model holding the picture should not also be told to go OCR it — or changes nothing at all. Sandbox escape, an unreadable file and a non-image type all fall back to leaving the line alone rather than erroring: `image_ocr` resolves paths through the same guard and will refuse them in terms the model already understands. The frontend is untouched, and the transcript keeps what the composer actually sent.

**Not done, and next:** the model asking to look at an image it found itself. Anthropic allows image blocks inside `tool_result`, so `read` could return one directly; the OpenAI-compatible APIs do not, and the workaround — a synthetic user message carrying the image after the tool result — is tool-loop plumbing, not an adapter change. Separate decision, separate work.

---

## 52. Decision — The Sweep Behind §50: One Real Defect and Four Dead Ends (2026-07-28)

§50 compared tool *schemas* against Claude Code's and opencode's. This pass compared tool *behaviour*, by driving every remaining tool through a throwaway test and reading what came back instead of reading the source and deciding it looked right. That distinction found something the schema diff could not.

**`write` had been corrupting every file it wrote.** [write.go](../internal/skill/write.go) passed the content through `stringSlice`, which trims each element and drops the empty ones. Driven for real, that is:

```
in="package main\n\nfunc main() {}\n"  → out="package main\n\nfunc main() {}"
in="  indented first line\nsecond\n"   → out="indented first line\nsecond"
in="\n\nleading blank lines\n"         → out="leading blank lines"
content:""                             → error "usage: write <path> <content>"
```

Every file written by the agent lost its trailing newline; any file whose first line was indented — YAML, a Python block, a continued expression — lost that indentation; an empty file could not be created at all. The fix is to take the raw `[]string`, which is **what [edit.go](../internal/skill/edit.go) already does, with a comment naming this exact hazard**. The lesson is not "write was wrong"; it is that a hazard documented at one call site does not fix the sibling call site, and only running the tool would have shown it. `TestWriteSkillWritesExactBytes` now compares bytes, because "close enough" is precisely what hid this.

**Three dead ends, all the same shape: a cap with no way past it.**

- **`glob`** stopped at 300 paths and said "narrow the pattern". It now takes `head_limit`/`offset` and names the offset to resume from, the same as `grep` gained in §50. Sorting happens before paging, so page two continues page one's newest-first order rather than reordering it. A `maxScan` ceiling bounds the walk, and the caveat that past it the ordering is over what the walk reached rather than the whole tree is marked rather than hidden.
- **`list`** returned bare names, so `sub` and `sub.txt` were the same kind of thing on the page and the only way to tell was to call `list` again and see whether it errored. Directories now end in `/`.
- **`diagnostics`** took one file, so "is anything I just changed broken" meant one call per file — and in practice meant asking about one and assuming the rest. It now accepts a folder, walks it with the same exclusions `grep` and `glob` use, and reports how many files it checked and how many it skipped. Bounded at 40 files because each is a round trip and the turn abandons a tool at 60 seconds; a partial answer that says how far it got beats a complete one that never arrives.

**`web_fetch` gained a cache and a `prompt`.** The cache is 15 minutes, matching Claude Code's, because research is a walk back and forth over the same few pages and the second walk should cost nothing — counted at the test server, since "it returned the same text" is also true of a cache that never worked. `prompt` is the larger one: a documentation page is tens of thousands of characters and the model usually wants one paragraph, and every character of the rest stays in context for the rest of the session. The seam is `RegistryOptions.Digest`, a **function** rather than a `model.Provider` — `internal/skill` has no business knowing how a completion is made, and a func is what a test supplies in one line. Nil is a supported configuration (the CLI builds its registry with no provider in hand) and means the tool behaves exactly as it always did. So does a digester that errors, and so does a page short enough that the round trip would cost more than the page: **every failure returns the full page**, because the question was only ever an optimization and the page is already in hand.

**Left alone, with reasons:** `delete` still refuses directories (`shell` covers it now, under approval); `apply_patch` still cannot create files (`write` does); `web_search` is still a DuckDuckGo HTML scrape, which owes nobody an API key but will break the day their markup changes — worth knowing, not worth pre-emptively rewriting; MCP is still tools-only, no resources or prompts or OAuth, which is where the official Go SDK and every server we have met agree the value is.

---

## 53. Decision — The Three That Were Designs, Not Parameters (2026-07-28)

§50 and §52 closed everything that was a missing argument or a wrong line of code. What was left were three capabilities with no home in the existing shapes at all. Each gets its own seam here.

### 53.1 Background commands — `run_in_background`, `shell_output`, `shell_kill`

`shell` runs a command and waits. That covers a test run and a build, and it cannot cover the other half of development — a dev server, a watch build, a log tail — because those never exit, and **never exiting is not a slow command, it is a different shape of work**. Before this the model could not start one at all: it would hang until the deadline and be killed with nothing to show.

Three tools over one runner, the arrangement §44 already uses for `task`. The model holds a **handle**, never a pid, so it cannot reach a process Aetox did not start. Four decisions worth keeping:

- **`context.Background()`, not the turn's context.** A background command whose whole point is to outlive the answer that started it cannot be tied to that answer's lifetime. What still stops them is §24's job object, which every child is already inside — so nothing outlives the app, which is the property that actually matters.
- **The buffer keeps the tail, not the head.** `cappedWriter` keeps the first 1 MiB, which is right for a command that finishes and exactly wrong for one that is still printing: for a server the interesting line is always the most recent. The read cursor slides with the buffer, or the next read replays bytes the caller already had.
- **Output is consumed.** Each `shell_output` returns only what is new. A model polling a chatty server otherwise pays for every line once per poll.
- **A killed job keeps its unread output.** The handle outlives the process on purpose: the reason it was killed is usually in the last few lines it printed.

### 53.2 `read` opens an image — the other half of §51

§51 got a *user's* attachment to a sighted model. This gets a file the model found itself: `read` on a `.png` returns the picture rather than the refusal that points at `image_ocr`. `RegistryOptions.Vision` decides, resolved by the same `model.ResolveVision` the attachment path uses, and false everywhere it is unknown — a blind model still gets exactly the refusal it always got, naming exactly the tool that can help it.

**The plumbing decision:** a picture cannot ride inside the tool result. Anthropic allows an image block in `tool_result`, the OpenAI-compatible APIs do not, and Ollama has no `tool_result` shape at all. Rather than three code paths, the tool loop appends **one follow-up user message** carrying the image, which works on all three. It costs one extra message in history and it says which call it belongs to — a user message the user did not send is otherwise indistinguishable from one they did. `skill.Output.Images` is how the tool says so, and the tool callback's signature grew a `[]model.Image` to carry it out of `internal/turn`.

### 53.3 Undo — `internal/snapshot`

The only thing between an agent and a bad edit was the approval prompt, and approval is a judgement made *before* seeing the result. When the result is wrong, the user's own git history was the entire safety net — which works exactly as well as they commit.

The design is opencode's, read from its source (the source-reading note on snapshots (kept off this repo)) rather than invented, and it earns its place by **adding nothing**: no storage format, no database, no daemon, no new dependency. A **shadow git repository** per project lives in Aetox's data root with its work tree pointed at the real project; a snapshot is a **git tree object** from `write-tree`; `alternates` links the shadow store to the project's own objects so a committed file is never stored twice.

What that buys, and what each is pinned by a test for:

- **The user's repository never notices.** No commits, no staged changes, no stash, nothing in `git status`. This is the property that makes it safe to run on every turn, so it is asserted directly rather than assumed.
- **`.gitignore` is obeyed**, because it is already git's answer to "what is not the user's code" and a snapshot has no business disagreeing.
- **Restore is per file.** Undoing one bad edit must not throw away the four good ones made beside it.
- **"Restore to before" includes "before it existed"** — a file the agent created is deleted, or a created file could never be undone.
- **Undo is one turn deep.** The question a user asks is "undo what it just did", asked immediately; an undo stack invites the far more dangerous "undo the last six" long after the reasons are forgotten. Pressing it twice is a no-op, because the restore is itself the state now.

Unavailable is an ordinary condition, not a failure: no git, or a folder that is not a repository, means no undo and no fuss — `App.snapshots` is nil and every caller is written for nil.

**Not done:** the button. `UndoLastTurn` and `PendingUndo` are bound and tested; putting them in front of the user is frontend work, and [Chat.svelte](../desktop/frontend/src/lib/Chat.svelte) currently carries uncommitted changes of the owner's that this should not collide with.

---

## 54. Decision — Clearing the Rest of the Parity List (2026-07-28)

The tail of §52's sweep, done rather than recorded. Each of these was small on its own; the reason to do them together is that a coding agent is judged on the one tool that was missing, not the twenty that were there.

**`notebook_edit`, and `read` renders notebooks.** A `.ipynb` is JSON with the code buried inside it — `"source": ["def f():\n", "    return 1\n"]` — plus every output the cell ever produced, including base64 images running to tens of thousands of characters. That broke both halves of the normal path: `read` handed the model raw JSON, so looking at five lines of Python cost an enormous amount of context on a picture it could not see; and `edit` matches an exact string, which in the file is JSON-escaped, so a match either failed or corrupted the notebook. So `read` renders cells with their numbers and summarises outputs (a traceback is often the answer, a base64 PNG never is), and `notebook_edit` changes a cell **through the JSON**. Everything not named — metadata, kernelspec, nbformat version, other cells' outputs — is written back untouched, and source is written as a list of lines the way Jupyter writes it, or a one-cell change becomes a diff covering the whole notebook. Replacing a cell clears **that cell's** outputs, because a recorded result describing code that is no longer there is how a model reads a stale traceback as current.

**`delete` takes a folder, but only when asked.** `recursive` is required and refused by name otherwise: "delete" of a path that turned out to be a directory is the one mistake at this layer with nothing smaller to get wrong. The sandbox root is refused outright — it is the project.

**`web_search` gained `allowed_domains`/`blocked_domains`.** Suffix matching, so `go.dev` catches `pkg.go.dev`, with a boundary check so it does not also catch `notgo.dev`. "The engine found nothing" and "your own filter removed everything" are reported differently, because they call for opposite next moves.

**`todo_write` gained `activeForm`** — the task worded as what is happening now, which is what the row should read while a step runs. Optional, so a model that omits it leaves the UI showing what it showed before.

**The staleness question, answered by what was already there.** Claude Code enforces read-before-edit with a ledger. Aetox does not need one: `edit` requires the text to appear exactly once, so a file that moved under the model either no longer matches (refused) or now matches twice (refused). The failure mode a ledger prevents — an edit landing somewhere the model never looked — cannot happen here, and a test now says so rather than leaving it a happy accident.

**Plan mode is a sub-agent profile, not a mode.** the source-reading note on agents (kept off this repo) recorded the finding that opencode's `plan` agent is structurally identical to `build` — same loop, same everything — differing only in its permission set. Aetox already had the whole mechanism (§44): [profiles/plan.md](../internal/subagent/profiles/subagents/plan.md) inherits every reading tool, because a plan built without `diagnostics`, `git` or the web is a worse plan, and denies every writing one. `Deny` rather than a `tools:` allowlist on purpose — the allowlist is a token filter, `Deny` is the gate that reaches `PermissionConfig`, so a discovered skill by the same name cannot walk through. The brief fixes the answer's shape (what is there now / what to change / what could go wrong / what you are unsure of) because a planner that free-forms produces prose nobody can act on.

---

## 55. Decision — MCP Resources, `symbol`, and Two Things Deliberately Not Built (2026-07-28)

**`symbol` (§54's LSP gap).** The language server was already running for `diagnostics` and was only ever asked one question. "What is this and where does it come from" is the other one, and without it the model answers by searching — grep the name, open what matched, guess which definition is in scope. That works, it is expensive, and it is subtly wrong in any codebase with two types of the same name, which is most of them once vendored code and test doubles are counted.

It takes a **name, not a line and column**: a model reads code, not coordinates, and asking it to count characters invites an off-by-one that describes the token next door with total confidence. The name is matched as a whole word, so looking up `Get` does not land inside `Getter`.

This needed real request/response correlation in `internal/lsp`, which the client did not have — diagnostics arrive as notifications, and the one request it made (`initialize`) was confirmed by *the server staying alive*, not by matching an id. `readLoop` now routes replies to whoever asked, and the id is registered **before** the message is sent, because a fast server can answer before the sending goroutine gets back to the map. Both parsers accept every shape the protocol has carried for hover contents and definition locations; servers disagree and all of them are legal.

**MCP resources.** A server exposes two different kinds of thing. Tools are verbs. A **resource** is a noun — a document, a record, a config the server is the authority on, addressed by URI. Bridging only the verbs left every server that is a *source of data* looking empty.

Two tools per server, not one per resource: a server can expose thousands, they change while Aetox is running, and a tool definition per resource would be serialized into every request. Listing is a call, not a schema. And the pair is registered **only when the server actually has resources** — a tool that always answers "none" is still paid for in the tool block of every single turn, which is the whole cost of getting this wrong.

**Not built, with reasons.**

- **MCP prompts stay at the client layer.** `Client.Prompts`/`GetPrompt` exist and compile; they are not bridged as model tools. A prompt template is a workflow a *human* picks, which is what the composer's `/` palette is for (§36) — handing one to the model as a tool offers it a canned instruction it has no way to judge. The plumbing is there for whenever the palette wiring happens.
- **Plugin hooks are not being built.** the source-reading note on plugin hooks (kept off this repo) put them next after MCP, and that ordering has aged out: `plugin_install` is still the half-finished loader of §6.5, so a `tool.execute.before/after` system today would be an extension point with nothing on the other end of it. The same study's own finding applies — *"always grep for the call site, not just the type, before citing a feature as real"* — and building the type first is how you end up citing your own scaffolding. MCP already covers third-party capability, which is what the hooks were wanted for.
- **The undo button, again.** Same blocker as §53: it needs `wails generate` to regenerate the bindings **and** an edit to [Chat.svelte](../desktop/frontend/src/lib/Chat.svelte), which carries the owner's uncommitted work. `UndoLastTurn` and `PendingUndo` are bound and tested on the Go side.

---

## 56. Decision — Vision Proven Against Real Providers, Not Just Against Its Own Schema (2026-07-28)

§51's unit tests prove the JSON has the right shape. They cannot prove a provider agrees with that shape, and every one of the three spells an image differently — which is exactly the kind of claim that passes review and fails in front of a user. Run against a local Ollama serving `qwen3-vl:8b`:

| path | code under test | result |
|---|---|---|
| Ollama native | `convertMessagesToOllama` → `images: [base64]` sibling field | **"red green blue"** |
| OpenAI-compatible | `convertMessagesToOpenAI` → `content` parts with `image_url` | **"Red Green Blue"** |

The fixture is three coloured bars and **contains no text at all**, so `image_ocr` on it returns nothing. A correct answer can only come from a model that actually saw the picture — which is the difference this whole change exists to make.

The second row matters more than the first: that adapter serves almost the entire catalog (OpenAI, DeepSeek, OpenRouter, Groq, LM Studio), and Ollama's own `/v1` endpoint exercises it without a paid key. `web_fetch`'s digester was proven the same way against a real DeepSeek model — it returned "the option is **MaxAttempts**, default **5**" from a page padded with 24,000 characters of filler, which is an answer rather than a summary.

**Still unproven: Anthropic's `source` block.** No key on this machine. The shape is unit-tested and the empty-text-block trap is covered, but it has not met a real server, and this section says so rather than letting the two green rows imply three.

**What stopped this being run at all**, and is worth keeping: the owner refused the first attempt, because a machine already running Ollama could have had a second model pulled into VRAM on top of the first. The concern was right in general and wrong here — `ResolveVision` reads the name of the *currently selected* model and nothing anywhere constructs a second provider for images — but the reason it was worth asking is that "there is a spare vision model on this box, let's use it" was **my** reasoning, not the product's, and I had not checked what else the machine was doing. Verified by grep afterwards rather than asserted: the only three `NewProvider` call sites are bootstrap, its `aetox` fallback, and the settings connection test.

---

## 57. Decision — Hooks Are a Shell Command, Not a Plugin Runtime; and Undo Gets a Button (2026-07-28)

**§55 said plugin hooks were not being built. That was the right call about the wrong question.**

opencode's shape is a plugin runtime: a JS module registers `tool.execute.before`/`after` and the host calls it. Aetox has nowhere to run one — `plugin_install` is still the half-finished loader of §6.5 — so those callback points would have been an extension point with nothing on the other end. That reasoning holds and is not revisited here.

What it missed is that **nobody actually wants a plugin runtime**. They want: refuse a command by my own rules, run my formatter after a write, tell me when the agent changes something. Claude Code answers that with a shell command in a settings file and no plugin system at all, and once the question is asked that way Aetox already had every piece.

The pattern was in the codebase three times over, hardcoded: **rtk rewriting a command before it runs** (§13.5), **safety refusing one**, and **rtk compacting the output after** (§13.4). `internal/hook` is that idea made configurable without a Go build — `hooks.json` beside `permissions.json`, one answering "may this run" and the other "what else should run".

Six decisions worth keeping:

- **`PreToolUse` fires *after* approval, not before.** A hook is a rule the user wrote; firing their formatter and their notifier for a call they are about to refuse is work that never happens.
- **Non-blocking is the default.** A hook is usually a formatter or a notifier, and one that silently starts refusing work because it exited 1 is worse than no hook at all. `blocking: true` is opt-in, per hook.
- **A blocked call returns as a normal tool result carrying the hook's own stdout**, not as an error. The model then reads *why* and does something else, rather than calling into the same wall again — the same reasoning as §50's rule about the abandoned-tool message.
- **`PostToolUse` fires on failure too**, because "tell me when a command fails" is the same hook point as "run my formatter after a write". It **cannot block**: the tool has already run, and a hook that pretended otherwise would be lying to the model about what it is reading.
- **The call reaches the hook two ways at once** — JSON on stdin for a real script, `AETOX_TOOL`/`AETOX_EVENT`/`AETOX_TOOL_ARGS` for a one-line shell guard. One channel would have made half the useful hooks awkward to write.
- **Ten seconds, not configurable.** A hook sits between the model and its tool, so a slow one is felt on every single call. Anything needing longer is a background job, not a hook.

**And the button.** `internal/snapshot` had worked since §53.3 with no way for a user to reach it, which §53 and §55 both recorded as outstanding. The chip sits in the composer's focus row beside the project and branch, appears **only when the last turn actually changed a file** — so it is never a button that does nothing — and lists those files on hover. Pressing it posts the result into the transcript rather than a toast: undoing is a real event in the session, and a message you can scroll back to is the only record of it that survives.

**One thing this closes that was never on the list.** Formatter-on-save was a Low row in the architecture reference note (kept off this repo)'s gap table, and it is now closed without a single line of formatter code in the product: `PostToolUse` with `matcher: "write"` runs whatever the user already uses.

---

## 58. Decision — v0.7.0, and What the Numbers Say After a Day of Building (2026-07-28)

Version bumped in the four places that carry it: [cmd/aetox/main.go](../cmd/aetox/main.go), [desktop/wails.json](../desktop/wails.json), [README.md](../README.md)'s status heading, and [docs/index.html](../docs/index.html)'s badge. Download links needed no change — they point at `releases/latest/`, which is why they were written that way. [scoop/aetox.json](../scoop/aetox.json) has its version and URL bumped but **its `hash` is still v0.6.0's**: that hash is of the released zip, which does not exist until CI publishes the tag, so it stays a post-release step exactly as it was for v0.6.0.

**`bench.ps1` gained `-Engine`.** The four existing modes measure what a user feels *once* — install size, launch time, idle RAM, a long soak. Nothing measured what they feel *on every message*, which happens thousands of times more often. The new mode reports the tool payload each request carries, the cost of assembling it, the benchmarks already in the repo, build time, and how long the whole suite takes. **No competitors in that table on purpose**: the internals of Claude Code or opencode cannot be measured from outside, so a comparison would be a guess. It measures us against ourselves across versions.

Measured on the v0.7.0 build:

| | |
|---|---|
| `aetox.exe` · installer | 33.1 MB · 12.8 MB |
| tools the model is sent | **27 built-in** (30 registry entries) |
| payload per request | **18.1 KB** — was 9.9 KB at 20 tools |
| assemble + serialize | **0.12 ms/turn** — was 0.13 ms at 20 tools |
| grep the whole repo | 45.5 ms |
| resolve one sandbox path | 728 µs — every file tool pays it |
| `wails build` · `go test ./...` | 43.3 s · 38.5 s, 26 packages green |

Two of those are worth reading twice. **The payload nearly doubled and the time to build it went down**, so the cost of nine new tools is paid in tokens, not in latency. And **38.5 s is a ceiling report, not trivia**: the agent runs `go test ./...` itself now, and §50 set the per-tool deadline at 60 s — 21.5 s of headroom before a full suite needs `timeout_seconds`.

**A bug in the measuring instrument, found by measuring.** `-Start` decided "is the app already running" with `Get-PidSet`, which does **not** apply the `CmdMatch` filter `Measure-Procs` does. `msedgewebview2.exe` is the process name every WebView2 app on the machine shares — Windows itself included — so it counted other apps as Aetox and skipped the run entirely. On any machine with a WebView2 app open, which is most of them, **`-Start` could never measure Aetox**. It reported "12 processes already running" while `-Snapshot` saw none.

[BENCHMARK.md](../BENCHMARK.md) §5 had already diagnosed this exact class of bug for `-Snapshot` and `-Soak`, and stated that `-Start` was immune "because it compares process lists before and after". That sentence is half right, which is the dangerous kind: the mode counts processes in **two** places, and only the measurement half had a baseline. The doc is corrected rather than deleted — the wrong version is the more useful record.

**What did not go on the website.** The post-fix startup re-measure (1.96 s cold, 0.53 s warm, 276 MB) ran 3 rounds with a 25 s settle, which breaks BENCHMARK.md's own rules 4 and 5. It stays in BENCHMARK.md as a **no-regression check** — launch time unchanged at 0.53 s despite nine more tools — and explicitly not as a website number. The README keeps the numbers that passed the full methodology, and its test counts are re-measured: **854 Go tests across 26 packages, 92 frontend**.

---

## 59. Decision — The Loop Works Out Loud: Narration and Thinking Interleaved into the Timeline (2026-07-28)

**What the user saw and asked for.** Watching Claude Code work — "Thought for 60s → tools → thought for 2s → tools", with a sentence of the model's own words between batches — reads as *working while thinking*. Aetox's turn looked like *silence, then an answer*, even though its loop ([internal/cognitive/agent.go](../internal/cognitive/agent.go) `RespondWithTools`) is the same OpenCode-style run-until-done loop, streams reasoning every round, and even records the per-round assistant text into context.

**Measured before building** (the bundle lesson, applied): across 42 debug logs of real usage, **28% of 363 tool-call rounds already carried narration text** — "Let me try a different approach to get system info." — that went into context and nowhere else. The other 72% were genuinely silent rounds. So the feature surfaces work the model already does, and a one-line prompt nudge raises the rate; nothing here waits on a future model.

**The mechanism.** Two additions, both on existing seams:

- `TurnOptions.OnRound` ([internal/turn/executor.go](../internal/turn/executor.go)) — the loop reports each completed round's text and whether it was final. Carried in the options struct **specifically so the `Agent` interface and every fake implementing it stay untouched**. The interjection-continued round reports too: its answer text was previously invisible everywhere.
- Two new `ToolEvent` kinds on the one timeline channel the UIs already consume: `"note"` (the round's narration, `Text`) and `"thinking"` (the round's reasoning-stream duration, `Secs`, cut per round by `thinkSegments` wrapping the reasoning handler). Events arrive in causal order — thinking, the note it produced, then the calls the note announces. The final round's text is never a note: the reply bubble owns it.

The frontend renders notes as plain sentences in the timeline (`kind` on `ToolStep`), keeps the **latest** note on screen while the turn runs, and excludes both kinds from the "used N tools" count. Sub-agent rounds flow through the same relay with `Parent` stamped, so a delegate narrates inside its own block. CLI and any listener that ignores unknown actions are unaffected.

**Prompt half:** one sentence in [internal/prompt/prompt.go](../internal/prompt/prompt.go) (`narration()`) asking for a short line in the user's language before tool batches. Deliberately one sentence — narration is output tokens on every round.

**Not built, on purpose:** persisting the interleaved timeline across session reloads. Tool rows are already live-only today and nobody has missed them; if that changes, the events are already structured and the persistence is a phase of its own.

---

## 60. Decision — MIT → Apache-2.0, and the Brand Is Carved Out of the Licence (2026-07-28)

**The question.** Could someone clone the GitHub repo, rebrand it, and pass it off as their own? Under MIT: yes, nearly all of it — fork, modify, rename, close the source, sell it. §28 chose MIT to make "no lock-in" legally true, and it did. What it never did was say anything about the *name*, and a survey of the repo found no notice of any kind: no `NOTICE`, no trademark line in [README.md](../README.md), and a binary that printed `aetox version 0.7.1` and nothing about who wrote it.

**What changed.** Four things, only the first of which is irreversible:

- **[LICENSE](../LICENSE) is now the verbatim Apache-2.0 text** (202 lines, fetched from apache.org, SHA256 `cfc7749b…`, unmodified — the appendix template stays as-is, per convention). Apache buys three things MIT does not have: an explicit trademark carve-out (§6), a patent grant, and §4(b)/(d) — a redistributor must state what they changed and must propagate `NOTICE`.
- **[NOTICE](../NOTICE)** names the author, the repo, and reserves "Aetox" and the logo as trademarks. This file is the load-bearing one: §4(d) makes carrying it a *condition of the licence*, so a rebranded fork that strips the name is in breach and loses its grant — that is a copyright takedown, available today, with nothing registered anywhere.
- **The running program states its origin.** `aetox --version` prints a credit line (`versionCredit`, [cmd/aetox/main.go](../cmd/aetox/main.go)), the desktop Settings → About panel carries the same line ([desktop/frontend/src/lib/Settings.svelte](../desktop/frontend/src/lib/Settings.svelte)), and [desktop/wails.json](../desktop/wails.json)'s copyright field — which Wails bakes into the Windows exe version resource — now carries the legal name instead of a GitHub handle. None of this *stops* a rebrand; all of it makes stripping the attribution a deliberate act rather than a side effect, which is the difference that matters if it is ever argued.
- **[README.md](../README.md) and `scoop/aetox.json`** say Apache-2.0, and the README gained a licence-and-brand section: fork freely, ship under your own name.

**Why Apache and not something stronger.** AGPL would force a SaaS rebrander to publish their source, and BSL would forbid reselling outright — both were considered and both cost more than they return here: many companies ban AGPL by policy, and BSL is not open source at all, which contradicts the project's own pitch. Apache is the strongest option that keeps "use it for anything" true while still reserving the name.

**Two facts that constrain any future move.** v0.7.1 and everything before it shipped under MIT and stay MIT **permanently** — anyone may fork that tag and continue from it; this change binds only what comes after. And the relicence needed nobody's permission solely because the copyright is one person's. **That ends with the first merged outside PR**, after which a relicence needs every contributor's agreement — so a CLA or DCO has to land before contributions are accepted, or this door closes.

**Not done, on purpose:** no per-file Apache header (the appendix recommends one; `LICENSE` + `NOTICE` already carry the terms, and 200-odd files of boilerplate is noise for a repo one person reads). No trademark registration — that is money and 12–18 months at DIP, and it is the only thing that would enable a *name-based* takedown, so it stays the open item if the name ever becomes worth defending. No CLA yet: there are no outside contributors to bind, but see above for when that stops being true.

---

## 61. Decision — Sign In With the Plan You Already Pay For (2026-07-29)

**The question.** Every provider Aetox supported wanted the same thing from the user: paste an API key. That excludes the largest group of people who already pay for a model — a Claude Pro subscriber, a ChatGPT subscriber, someone whose employer bought Copilot — because none of those plans issue a key at all. Competing agents (opencode, hermes-agent, plandex, crush) all solved this, and the survey of how is in the commit that added [internal/oauth](../internal/oauth): four working flows, four different protocols.

**What shipped.** A sign-in path beside the key path, never instead of it.

| Provider | Flow | Terms | Runtime |
|---|---|---|---|
| OpenRouter | PKCE → local redirect listener | **Published** by OpenRouter, no client id needed; mints a real API key the user owns | existing `openai-compatible` |
| GitHub Copilot | Device code | Editor plugin's OAuth client | existing `openai-compatible` |
| Qwen | Device code + PKCE | qwen-code CLI's OAuth client | existing `openai-compatible` |
| Claude Pro/Max | PKCE + pasted code | Claude Code's OAuth client | existing `anthropic` |

Three of the four reach models through runtimes that already existed. That is the reason this order was chosen over the more obvious "Claude first": the expensive part of a provider is its wire format, and picking sign-ins that reuse a wire format meant four providers for roughly one provider's work.

**The one seam that mattered.** An OAuth token expires — a Copilot one after ~25 minutes — and providers are constructed once and reused for a whole session. Resolving credentials at construction, which is what `APIKey string` implies, would hand every long session a dead token. So both runtimes gained `TokenSource func(context.Context) (string, error)`, consulted **per request**, with refresh and write-back behind it ([internal/oauth/token.go](../internal/oauth/token.go)). A nil `TokenSource` is the old path byte for byte, which is why no existing provider changed behavior and no call site needed editing: [model.NewProvider](../internal/model/factory.go) resolves the sign-in itself.

**Three things the Claude flow requires that nothing else does.** `Authorization: Bearer` **instead of** `x-api-key` (sending both is a 401), `anthropic-beta: oauth-2025-04-20`, and a system prompt that opens with `You are Claude Code, Anthropic's official CLI for Claude.` The last one is a condition of the credential rather than a prompt choice — and §63 later found the check is stricter than "opens with": the line must be a system **block of its own, byte-for-byte** ([anthropic.go](../internal/model/anthropic.go) `anthropicSystemField`), with Aetox's prompt as the block after it. It is *not* sent on the API-key path, which is what `TestAnthropicAPIKeyPathUnchanged` exists to hold.

**Risk is a field, not a footnote.** `oauth.Method.Risk` is `open` or `restricted`, and the restricted note renders next to the button in Settings before the user commits. Three of the four flows work by presenting another product's OAuth client: the provider can switch that off without notice, and a consumer plan's terms may not permit driving it from Aetox. That is the user's account and therefore the user's call — but it is not a call anyone can make from a button that says nothing.

**Credentials do not live in `model-preference.json`.** That file is pasted into bug reports and synced between machines; a refresh token in it is a standing account takeover. Sign-ins go to `oauth.json`, 0600, write-then-rename ([internal/oauth/store.go](../internal/oauth/store.go)).

**What the live test caught that unit tests could not.** `AETOX_LIVE=1 go test ./internal/oauth/` opens each flow against the real authorization server. It immediately failed on Qwen with `invalid character '<'` — the endpoint answers Go's default `Go-http-client/1.1` User-Agent with an HTML block page while serving curl fine. One header fixed it. No amount of mocking would have found that, and the symptom on a user's machine would have been a sign-in button that fails with a parse error.

**Not done yet, and why.** **ChatGPT/Codex** is the one flow that cannot reuse an existing runtime: `chatgpt.com/backend-api/codex` speaks only the Responses API over SSE, so it needs a fourth runtime with its own tool-call mapping — a bigger piece than all four above combined, and unverifiable without a subscription to test against. **Gemini Code Assist** is the same shape (`cloudcode-pa.googleapis.com`, its own wire format). Both stay on the list; neither is half-shipped here, because a stored credential for a provider that cannot answer is worse than no button at all.

---

## 62. Decision — The Two Wire Formats §61 Deferred, and What Only a Live Call Could Tell Us (2026-07-29)

**What §61 left open.** ChatGPT and Gemini were the two sign-ins that could not ride an existing runtime. Both are now built: [internal/model/responses.go](../internal/model/responses.go) (`codex` — a ChatGPT plan via `chatgpt.com/backend-api/codex`) and `internal/model/codeassist.go` (deleted with the restricted sign-ins) (`code-assist` — a personal Google account's free tier via `cloudcode-pa.googleapis.com`). Aetox now speaks five wire formats.

**Neither is a variation on something we had.**

| | Responses (ChatGPT) | Code Assist (Gemini) |
|---|---|---|
| System prompt | `instructions`, a top-level string | `systemInstruction` with role `"user"` — there is no system role |
| History | typed items: `message`, `function_call`, `function_call_output` | `contents[]` of `parts[]`, alternating user/model |
| Tool result | `function_call_output` matched by `call_id` | a **user** turn holding `functionResponse`, matched by function **name** |
| Streaming | ~12 named SSE event types | one shape repeated, everything nested under `response` |
| Reasoning | `response.reasoning_summary_text.delta` | a `text` part carrying `thought: true` |

That last Gemini row is the trap: thoughts and answer arrive in the *same* `text` field, and only the flag separates them. Read it wrong and the model's private notes render as its reply.

**Four things only a real call could have told us,** each of which had already passed unit tests:

1. **`gpt-5.1-codex` does not exist for a ChatGPT account** — 400, by name. There are no fixed model ids on this path any more; the list is per-account and per-plan. Aetox now asks (`DiscoverResponsesModels`, `GET /models?client_version=…`, which returns `slug` rather than `id`) and the catalog list is only a fallback.
2. **`max_output_tokens` is rejected outright.** A subscription endpoint does not take the knobs a per-token API sells. `Request.MaxTokens` and `Temperature` are simply not expressible here — stated in the code rather than silently dropped.
3. **Qwen's edge blocks Go's default User-Agent** with an HTML page, surfacing as `invalid character '<'` on an endpoint curl talks to happily. One header. (§61's flows, found by the same live test.)
4. **Google's refresh response contains no `refresh_token`.** Writing the response through as-is logs the user out on the first refresh — the old one must be carried forward deliberately.

**What is proven, and what is not.** [live_subscription_test.go](../internal/model/live_subscription_test.go) (`AETOX_LIVE=1`) adopts the session the matching official CLI already holds on this machine, into an isolated store, and runs the whole agent loop:

- **Code Assist: green, end to end.** Answered "Tokyo is the capital of Japan", called `get_weather({"city":"Bangkok"})`, accepted the result back and answered from it. Usage accounting included, with `thoughtsTokenCount` folded into completion tokens — omitting it under-reports every reasoning turn.
- **Codex: proven up to metering.** The account used for testing is on the free plan and its Codex quota is spent (`usage_limit_reached`, resets in 3 days). A 429 arrives only *after* the backend has accepted the credentials, headers, model id and body, so the wire format is verified as far as this account can verify it — and no further. That is stated here rather than rounded up to "works".

The live test skips rather than fails on a spent plan (a green suite must not depend on how much of someone's month is left) and paces itself past the Gemini free tier's ~1-request-per-minute limiter.

**Adopting an existing session.** `aetox login codex --import` / `--import` for `code-assist`, and a button beside each sign-in in Settings, read the credential the official CLI already wrote on this machine. Explicit action only: reading another tool's credential file is something a user asks for, never something Aetox discovers on its own. hermes-agent's Claude support is *only* this, and it requires Claude Code to be installed; here it is the shortcut, not the mechanism.

**Naming, deliberately.** `codex` is not an alias of `openai` and `code-assist` is not an alias of `gemini`. The `chatgpt` alias stays pointed at the API-key OpenAI provider — moving it would silently repoint saved preferences at a different account and a different bill. Two tests hold that line.

**One shared piece fell out of it.** [sse.go](../internal/model/sse.go): `bufio.Scanner`'s 64 KB line cap is fine for the older formats but not for Responses, which puts an entire turn — every tool call and its arguments — on the single line of the final event. A turn that wrote a large file would have died with "token too long" *after* the model had already done the work.

---

## 63. Decision — The Claude Contract, As the Server Actually Enforces It (2026-07-29)

Every item here passed unit tests and failed (or lied) against the real endpoint; all were settled by live calls (`AETOX_LIVE=1`, [live_subscription_test.go](../internal/model/live_subscription_test.go)).

**Thinking depth is `output_config.effort`, not `budget_tokens`.** Current Claude models take exactly two knobs: `thinking` is adaptive-or-off, and depth is `output_config.effort` (`low`→`max`). The old `{"type":"enabled","budget_tokens":N}` is a 400 on every current model, so a think level maps to an effort string, never a token count ([anthropic.go](../internal/model/anthropic.go) `convertThinkingToAnthropic`). The Settings dial for Claude is now the real six-step ladder (`thinking_capabilities.go`, default `high` — `xhigh` is what Claude Code uses but quietly spends more of someone's plan, so it is one click away instead of the default). Fable-class models think unconditionally and reject an explicit disable; "off" there means sending no thinking field at all.

**Reasoning arrives empty unless asked for.** `thinking.display` defaults to `omitted` — blocks stream with empty text, so the thinking panel sat blank on a model visibly thinking. `display: "summarized"` fixed it; verified live (six effort levels returned 0 chars until it was set).

**Sampling is gone.** Temperature is rejected alongside thinking on current models, and Aetox sets one on every call — so it is dropped for provider `anthropic` only; DeepSeek's `/anthropic` endpoint still honors it and keeps it.

**The OAuth system check is exact, per block — and its failure mode is a counterfeit 429.** The user's every turn died with `rate_limit_error` while a bare probe passed. Live bisection (system / tools / stream / max_tokens, one at a time) isolated the system prompt, then the encoding: the same content joined as one string (`prefix\n\nAetox prompt`) is refused; split as `[prefix][Aetox prompt]` blocks it is accepted. The endpoint compares the **first system block byte-for-byte** against the Claude Code line and disguises the rejection as a rate limit with body `{"message":"Error"}` and **no rate-limit headers** — the tell that separates it from the real thing. `anthropicSystemField` now emits the block pair on the subscription path (string unchanged for API keys and wire-format borrowers), and the live effort-ladder test carries a real system prompt so this contract stays held.

**A real 429 names its reset in a dialect we didn't read.** A Pro/Max sign-in reports limits as `anthropic-ratelimit-unified-*` with **Unix-seconds** resets, not the documented RFC3339 per-key headers — so the one honest thing a 429 error message can say ("try again in 3m") was unavailable. `rateLimitWindow` reads both dialects. Same live probe also surfaced what the plan headers actually carry: 5h/7d utilization, an `allowed_warning` band, and an overage flag.

**One panic on the same path.** `retryAfter(resp, -1)` — a caller using -1 as "no attempt, just the window" — hit `1 << -1` and killed the whole turn with `negative shift amount`. The function no longer trusts its input, and the caller that wanted the window without an attempt got its own `providerRetryAfter` ([httpclient.go](../internal/model/httpclient.go)).

**Three things the first look at the finished UI found, all the same shape: a written-down answer outranking the one the system would give.**

- **The desktop had its own provider allowlist.** `desktopProviders` in [app.go](../desktop/app.go) is hand-maintained, so four working sign-in providers were invisible in the picker while every test passed. Now guarded by [providers_test.go](../desktop/providers_test.go): anything with a sign-in must be listed, and nothing listed may be a name the engine does not know.
- **`ResolveDefaultModel` returned the catalog's model name before asking anything.** A name written months ago outranked the list the provider was serving right now. The order is inverted: local probe → live list → catalog name as the last resort. `RecommendedModels` is deleted for every provider that can be asked, and `DiscoverAnthropicModels` (paginated `GET /v1/models`, working with either an API key or a Pro/Max sign-in) fills the one gap that was left. Only two written lists survive: `aetox`, whose models are ours to define, and `code-assist` — its `:fetchAvailableModels` answers **403 PERMISSION_DENIED** on the free tier, verified live, so there is no list to ask for.
- **A working provider was reported broken.** Settings' "test connection" pings with a tiny `max_tokens`, and every runtime treated an empty completion as a failure — so a model that spent that budget before emitting a visible token came back as `anthropic response has empty text`. One shared guard (`errEmptyCompletion`, [types.go](../internal/model/types.go)) now consults the stop reason at all five call sites: truncation is not failure. The Anthropic stream had to start tracking `stop_reason` to answer it, which it never did before.

---

## 64. Decision — The Restricted Sign-Ins Come Back Out (v0.8.1, 2026-07-31)

**What changed.** The Claude Pro/Max, ChatGPT (codex) and GitHub Copilot sign-ins that §61/§62 shipped are removed. §61 named the risk honestly — those three flows work by presenting another product's OAuth client against a consumer plan, the provider can switch that off without notice, and the plan's terms may not permit it — and put the call in the user's hands with a warning next to the button. v0.8.1 reverses that: the warning does not neutralize the standing risk to the user's account, and a sign-in whose safest outcome is "the provider hasn't noticed yet" is not a feature Aetox should hold open. This is the one risk in the product that lands on the *user's* account rather than on Aetox, so it goes first and alone in a release.

**What stays.** Every sign-in whose terms are the provider's own to publish: OpenRouter (published PKCE flow that mints a key the user owns), Qwen (qwen-code's device flow against the user's free quota), and Gemini Code Assist (a personal Google account's free tier). Anthropic remains a provider — API key only, exactly as before §61. The `codex` and `github-copilot` providers are gone entirely, because neither has any credential path other than the removed flows; with them went the Responses runtime ([responses.go](../internal/model/responses.go), codex was its only rider) and the per-block system-prompt encoding §63 documented (subscription-path only, so the API-key path is byte-for-byte what it always sent).

**Stored credentials are dropped, not just orphaned.** An `oauth.json` written by 0.8.0 may still hold refresh tokens for the three removed providers. The store filters them on load ([store.go](../internal/oauth/store.go) `removedProviders`), so they are never read, never refreshed, never sent — and the next write purges them from disk. `TestRemovedProviderCredentialsAreDropped` holds that line.

**What §61 got right survives.** The `TokenSource`-per-request seam, the risk field on every method, the 0600 write-then-rename store, and the live sign-in smoke tests all remain — they serve the three flows that stay.

> **Superseded within the same release.** The two restricted sign-ins this section kept — Qwen and Gemini Code Assist — are removed by §65 and §66, which ship in v0.8.1 alongside it. Read §64 as the decision that started the sweep, not as a description of what v0.8.1 ships.
>
> **Partly reversed by §69.** ChatGPT (`codex`) is back, with the Responses runtime, and the rule this section stated is narrowed there: Aetox states a borrowed-client risk rather than removing the button, and holds the line only against a sign-in with a known enforcement record. Claude Pro/Max and Copilot stay removed.

---

## 65. Decision — Qwen's Sign-In Goes Too (v0.8.1, 2026-08-01)

**What changed.** The Qwen device-code sign-in §61 shipped and §64 kept is removed. §64 kept it on the reasoning that its terms were "the provider's own to publish" — but it was in the same class as the three §64 deleted: it presented the qwen-code CLI's public client id (`f0304373b74a44d2b584a3fb70ca9e56`) against a consumer account, which is `RiskRestricted` by Aetox's own definition. The plainer reason is that it did not work in practice, so what was left was a button carrying a standing account risk and no working path behind it.

**Qwen the provider stays; only the sign-in goes.** Unlike `codex` and `github-copilot` in §64, `qwen` has a credential path that never depended on OAuth: `DASHSCOPE_API_KEY` / `QWEN_API_KEY` against DashScope's OpenAI-compatible endpoint. It keeps its catalog entry and its place in the desktop picker — moved out of the "signed into" group into the API-key group ([app.go](../desktop/app.go) `desktopProviders`). Its base URL is a fixed catalog entry again rather than one the login handed back, so `resource_url` and the `qwenEndpoint` normalizer went with the flow.

**Stored credentials are dropped, not just orphaned.** `qwen` joins `removedProviders` in [store.go](../internal/oauth/store.go): a refresh token an 0.8.0 `oauth.json` still holds is never read, never refreshed, never sent, and the next write purges it. `TestRemovedProviderCredentialsAreDropped` covers it, and a new `TestRemovedProvidersHaveNoSignIn` closes the other half — a removed provider may not still carry a `Method`, a refresher, or a `Start()` case.

**The device-code machinery stays, unridden.** [device.go](../internal/oauth/device.go) had exactly two riders — Copilot (§64) and Qwen (§65) — and now has none. It stays because it is RFC 8628 verbatim, tested on its own rather than through a provider, and it is the implementation half of the `device` kind that `Method` and the Settings UI still promise. Its test seeds a synthetic method instead of a shipped one, which is the honest version of that coverage.

---

## 66. Decision — Code Assist Goes; One Sign-In Left (v0.8.1, 2026-08-01)

**What changed.** The Gemini Code Assist sign-in is removed, and with it the `code-assist` provider and the Code Assist runtime. `OpenRouter is the only sign-in Aetox has.` This closes the arc §61 opened: what survives is one flow, the one whose terms the provider publishes for third-party apps to use.

**Why, and this one is not a judgement call.** Two facts, both from Google:

1. Google announced detection for "policy-violating use cases (e.g. using Gemini CLI oAuth with third-party software)" in [gemini-cli discussion #22970](https://github.com/google-gemini/gemini-cli/discussions/22970), effective 2026-03-25. The flow Aetox shipped is that use case by name.
2. [Google's transition post](https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/) retired Gemini CLI and the Code Assist IDE extensions for the free and consumer-paid tiers on **2026-06-18**, in favour of Antigravity CLI. The free personal-account tier is exactly what `code-assist` targeted.

So the button was pointed at a policy Google enforces against, serving a tier that stopped answering six weeks before this release. The §64 test — does the risk land on the user's account rather than on Aetox — answers yes, and unlike §64 there is not even a working feature on the other side of it.

**The provider goes with the sign-in, exactly as `codex` did.** `code-assist` had `envKeys: nil` and no API-key path: OAuth was its only credential, so removing the sign-in leaves a provider nobody can authenticate. Deleted with it: `internal/model/codeassist.go` (deleted with the restricted sign-ins) (the `RuntimeCodeAssist` wire format — `code-assist` was its only rider, same as `codex` and the Responses runtime), the `RuntimeCodeAssist` constant, the catalog entry and its five aliases, and `codeAssistThinkingCapabilities`. **The `gemini` provider is untouched** — that is the public API-key endpoint at `ai.google.dev`, a different host, different credentials, and a bill instead of a plan. §62's "`code-assist` is not an alias of `gemini`" is what makes removing one and keeping the other a clean cut.

**Stored credentials are dropped, not just orphaned.** `code-assist` joins `removedProviders` alongside `qwen` in [store.go](../internal/oauth/store.go). A Google refresh token an 0.8.0 `oauth.json` still holds is never read, never refreshed, never sent, and the next write purges it — which matters more here than anywhere else in this file, because that token is a live Google account credential.

**Session import is now an empty registry.** `aetox login <provider> --import` and `ImportableSignIns`/`ImportSignIn` existed to adopt the credential another product's CLI had already written — first Codex, then only Gemini CLI. With both gone there is nothing to adopt, so the CLI flag is removed and the desktop bindings return nothing rather than pretending. Reading another tool's credential file was always the shortcut for a borrowed-client sign-in; with no borrowed clients left, the shortcut has no destination.

**Two UI branches now outlive their producers, on purpose.** The `device` kind (§65) and the `RiskRestricted` warning (this section) are drawn by Settings but produced by no shipped `Method`. Both keep their tests on a synthetic method — [Settings.test.ts](../desktop/frontend/src/test/Settings.test.ts) `seedDeviceSignIn` / `seedRestrictedSignIn` — rather than losing coverage with the provider that used to supply one. The risk field in particular is the thing §61 got right and must survive: the next sign-in that needs a warning should not have to reinvent it.

**Considered and rejected: Antigravity.** Antigravity CLI is Google's replacement for the Gemini CLI this section removes, it still serves consumer accounts, and third-party OAuth plugins for it already exist and work ([opencode-antigravity-auth](https://github.com/NoeFabris/opencode-antigravity-auth) reaches `gemini-3-pro` and `claude-opus-4-5-thinking` on a user's Google plan through a `localhost:36742` loopback that `loopback.go` could serve unchanged). It is rejected anyway, and it is the clearest rejection in this file. The [Antigravity Additional Terms](https://antigravity.google/terms) name this exact case — third-party software reaching the service, with an OAuth-plugin example spelled out in parentheses — as a breach, and give suspension or termination as the consequence. Google has already acted on it: [discussion #20632](https://github.com/google-gemini/gemini-cli/discussions/20632) documents the bans, a recertification form to get reinstated, and a **permanent ban on a second violation**. That Antigravity still works is what makes it worse than Code Assist, not better — Code Assist stopped answering, so nobody could lose an account to it; Antigravity would serve happily until the day the account goes, and what goes is the user's whole Google account rather than a quota. This is the same question §64 answered, asked once more with the enforcement now visible, and the answer does not change.

> **Narrowed by §69.** "OpenRouter is the only sign-in Aetox has" held for one release. ChatGPT is back beside it, and the rule this section applied is restated there: the two removals *here* were made on facts (an announced enforcement, a dead tier), and those facts are what survive — not the wider rule §64 wrote. Everything below about Antigravity stands unchanged; it is now the worked example of where the line sits.

**Where a new sign-in would come from.** The published, register-your-own-client kind, not the borrowed kind — that is now the only kind Aetox accepts. Hugging Face is the closest fit: [public OAuth apps with no client secret](https://huggingface.co/docs/hub/en/oauth), authorization-code + PKCE *and* RFC 8628 device code, an `inference-api` scope that makes inference calls on the user's behalf, and an [OpenAI-compatible router](https://huggingface.co/docs/inference-providers/index) at `https://router.huggingface.co/v1` — `RuntimeOpenAICompatible` unchanged, `loopback.go` unchanged, and `device.go` ridden again. The honest caveat is the free allowance: $0.10/month, $2 on PRO. `Proposed, pending approval.`

---

## 67. Decision — The Store Learns to Version Itself, and Starts Recording the Work (v0.8.2, 2026-08-01)

**The question.** The plan after 1.0.0 is a workflow layer that learns from what has already been done — spot a repeated job, summarize it into a rule ("receipt jobs where OCR came back under three lines: ask first"), then let each sub-agent tune its own prompt against measured outcomes. Every layer of that reads from a record of past work. Aetox had no such record. It remembered the *conversation* — `appendTurn` writes exactly two rows per turn, the user's text and the agent's final text — and nothing about the work in between. `App.toolHistory` kept the last 50 event *labels*, in RAM, gone on exit. Tool arguments and outputs were captured nowhere at all: `ToolEvent` carries one `Subject` because it is drawn on screen, and everything else was discarded at the point the tool returned. This section is the base for that layer, and nothing above it.

**Two hooks, not one widened hook.** `ToolEvent` is not extended into a record. It crosses the Wails bridge on every call and is rendered; a record is written to disk and read by a machine later. Widening it would put kilobytes of tool output through the UI on every call to serve a consumer the UI does not have. So `turn.ToolRun` is a second, separate shape emitted from a second, separate hook (`ExecutorOptions.OnToolRun`), carrying what the timeline throws away: the raw arguments JSON exactly as the model sent it (a malformed call is itself worth having recorded), the tool's own output before the receipt is built for the model, the outcome, and the duration. Nil records nothing, which is what the CLI passes.

**Captured at the executor, not at the database.** The capture point is inside the tool-exec callback in [executor.go](../internal/turn/executor.go), beside where `ToolEvent` is built — the last place the whole call still exists before it collapses to a `Subject` and a receipt. `RawOutput` is preferred over `Content` for the same reason `modelToolReceipt` prefers it: a record that disagrees with what the model was shown would be worse than no record.

**A delegate's calls are kept, and the panel still hides them.** `recordToolAction` drops every event with a `Parent` — deliberately, because the command-history panel is "what the main agent did" (§44.5). The store keeps them, stamped by the same relay that stamps `ToolEvent.Parent` ([task.go](../internal/subagent/task.go)) plus the profile name, because *which sub-agent is bad at what* is unanswerable without them. Two consumers, opposite needs, one producer.

**Stored short, measured whole.** Some tools return a file. A `read` of a large source file or a `web_fetch` of a long page would put more into `aetox.db` in one row than the application weighs, and "33 MB, local-first" is a promise about the user's disk too. So arguments are capped at 4 KB and output at 8 KB, with the *true* byte length stored beside them and a SHA-256 over the whole output whenever it was cut — the hash is what tells two runs that truncate to the same prefix apart, which a stored prefix cannot do on its own. The cut lands on a rune boundary: Thai is three bytes a character, so a naive cut would store invalid UTF-8 nearly every time. What the summarizing layer actually reads ("OCR returned under three lines", "this fetch came back a login page") is decidable from the head, so the cap costs it nothing it was going to use.

**`PRAGMA user_version`, because the next change is not an ADD COLUMN.** The store migrated by `addedColumns`, which can only ever `ALTER TABLE … ADD COLUMN`. `tool_runs` is the first change that exceeds it, and there was no way to ask which schema a user's file was on. Migrations are now an append-only list keyed on SQLite's own `user_version` — no table of our own, no join, no bootstrap ordering problem, and the version is written *inside* the same transaction as the step, so a step that fails halfway leaves the marker behind rather than claiming work it did not do. Version 1 is the pre-versioning schema verbatim: all `CREATE ... IF NOT EXISTS` plus `applyAddedColumns`, hence a no-op over every database already in the wild, which sits at `user_version` 0 with its tables already present.

**A database from a newer build is refused, not used.** Install 0.8.2, roll back to 0.8.1, and the old build meets a schema it does not know. It now declines to touch it and says which version it found. The app still starts — `database()` returning an error already means "no history" to every caller — so the cost is a history panel that is empty until the user upgrades, rather than a wrong write into the only copy of their chat history.

**Deleting a conversation deletes its runs.** `DeleteSession` now clears `tool_runs` for the session in the same transaction. Those rows hold file contents and page text, not labels; leaving them behind would be the opposite of what the delete button promises, and of the "everything remembered is viewable and deletable" line the learning design is being built under.

**Not done here, on purpose.** Nothing reads `tool_runs` yet — no UI, no summarizing, no prompt tuning. This is layer 1 of three ("remember"), and the value of shipping it alone is that layers 2 and 3 need a dataset that already exists rather than one that starts accumulating the day they are written.

---

## 68. Decision — The Settings Surface Gets a Design System, and Three Pages Get Their Bugs Back (v0.8.2, 2026-08-01)

**The question.** Settings had grown to eleven pages and 2,300 lines with good information architecture and no visual system underneath it. An audit found the split cleanly: the grouping, the Tools/Skills separation, the sign-in-versus-key split were all right and stayed untouched; everything below them — type, buttons, icons, and what happens when you press a destructive one — was not shippable. This section is that layer, plus the three page-level bugs the sweep turned up.

**Nothing may be destroyed on the first click.** Six actions deleted user data. Three of them (Skills, Prompts, Sub-agents) armed on a first click and deleted on a second; three (MCP server, provider, identity file) simply deleted. So the page taught "it asks first" on one screen and took a configured MCP server on the next. All six, plus leaving an editor with unsaved work, now go through one `ConfirmDialog`. Focus lands on Cancel, never on the destructive button — a dialog that appears under a finger must not turn a stray Enter into a delete — and each message states what is lost rather than asking "are you sure": removing the running provider says the engine will move back to `aetox`, because that side effect is the reason the confirm is worth reading.

**Tokens, and what they are allowed to cover.** `style.css` carried 23 distinct `font-size` values and 14 `border-radius` values, half-pixel steps included, each chosen one edit at a time. They are now nine and seven, in [styles/type.css](../desktop/frontend/src/styles/type.css), multiplied by one `--fs-scale` the user sets. Kept in px rather than rem for the reason `systemFont.svelte.ts` already documents — this stylesheet sizes everything absolutely, so a root font-size would cascade to almost nothing. Three declarations are deliberately excluded: the chat, editor and file-tree sizes are their own dials on the same page, and folding them in would make one setting move two ways at once. Spacing is *not* tokenised: a gap is a layout decision, not a type one.

**A weight this theme cannot express.** The button hierarchy shipped with four roles, the fourth being a borderless "ghost" for Cancel and Back. It was wrong, and measurement is what said so: `--border-subtle` reads 1.21:1 against `--surface-app` and even `--border-strong` only reaches 1.70:1, against a 3:1 floor for a control's boundary. On these surfaces a button with no fill is not quiet, it is invisible — the sub-agent editor's Back button, the only way out of that page, read as a line of body text. The role was removed rather than tuned. Three roles, and the lesson recorded here rather than rediscovered: *quiet* is not a weight this palette has.

**Emoji were never icons.** 56 distinct glyphs across 168 sites drew a different set on every machine — some colour, some monochrome, none on a text baseline, none able to take the ink of the row they sit in. They are now one inlined Lucide set ([icons.ts](../desktop/frontend/src/lib/icons.ts), 58 icons, 9 KB, ISC). Provider rows additionally carry their own brand mark from one source ([providerMarks.ts](../desktop/frontend/src/lib/providerMarks.ts), @lobehub/icons) rather than a mix, so fifteen marks share an optical weight; a provider with none falls back to a lettered tile rather than a gap. Measured after: every nav icon 16×16, stroke 2, vertical offset from its row's centre 0.0px — three things emoji could not do at all.

**One syntax palette, two surfaces.** The file editor already reskinned with the app theme. The chat did not: `markdown.ts` imported `highlight.js/styles/github-dark-dimmed.css`, so a fenced code block wore one fixed palette under all fourteen themes — on the four light ones, ten of fourteen token colours measured under 3:1 against the code surface. The palettes moved out of `editorTheme.svelte.ts` into [syntaxTheme.ts](../desktop/frontend/src/lib/syntaxTheme.ts), `applyTheme` stamps them as `--syn-*`, and `style.css` maps highlight.js's classes onto those. Monaco builds its themes from the same table. Kept in TypeScript rather than in theme.css because Monaco needs the values as data, not as computed style. What remains under 3:1 is the upstream themes' own design — Nord and Catppuccin Latte recede their comments deliberately — and raising it would stop them being the themes they are named after.

**Three page bugs, each the same shape: the UI knew something and did not say it.**

- **The Skills page named the wrong folder.** Two of the three places it printed a path said `~/.agents/skills`, which belongs to opencode and which Aetox never scans; only the card header had `~/.aetox/skills` right. Anyone following the instructions dropped files where nothing was looking. The page now reads the path from `skill.DefaultSkillsDir` through a binding — a path the UI can state on its own authority is a path that can drift from the one being scanned. It also gained the folder button the prompts and sub-agent pages already had, and surfaces the scan errors `ListDiscovered` had always dropped, so a broken SKILL.md stops being indistinguishable from an unwatched folder.

- **Every folder button in the app did nothing.** `proc.HideConsole` sets `HideWindow` and `CREATE_NO_WINDOW` so a background console child does not flash — but `explorer.exe` is a GUI program whose window is the entire point, and those flags suppressed it. Three copies of the same launch switch had each inherited the bug; they are one function now. `TestEveryExecSiteHidesTheConsole` still holds the rule and was not weakened: a GUI launch opts out with a marker comment in the source, so the exception is reviewable where it is used rather than hidden on a skip list.

- **Install only worked for repositories written for Aetox.** `plugin_install` required `aetox-plugin.json`, which approximately no repository has. It now falls back to reading the repository's own layout, and the rule is the honest one: *the folder that directly contains a SKILL.md is the skill*, at any depth. An earlier version of this fallback searched only the root and one level down and reported "no skill found" on a repository that plainly had two under `skills/` — the most common published layout there is. What disqualifies a folder is its kind, not its depth: `node_modules`, `vendor`, build output and dot-directories hold somebody else's code. A `.zip` route was added beside it for skills that arrive by any other road, with the same rule, zip-slip refused whole, and a staging directory so a truncated archive cannot leave half a skill behind.

**No ceiling on an install, and that is a product decision.** Both install routes briefly capped a skill at a file count and a byte total, on the theory that a repository containing a SKILL.md might be a monorepo. It was removed on the owner's call, and the reasoning is worth keeping: nothing is uploaded, nothing is hosted, and no cost lands anywhere but the user's own disk for something they asked for by URL. Rationing it was Aetox deciding how much of their machine they may spend, and a refusal would only have sent them to download it by hand. What stays is not policy: the set installs whole or not at all, paths that escape the install folder are refused, and a listing GitHub truncated is refused because completeness cannot be established from it.

**A truncation that wore a syntax error's clothes.** The first version of the repository listing reused `doJSONRequest`, whose 1 MB read limit was sized for a manifest. A real repository's recursive tree came to 1.6 MB, and `io.ReadAll(io.LimitReader(...))` returns the truncated bytes with a *nil* error — so it surfaced as `parse repository listing: unexpected end of JSON input`, a size problem with the real number nowhere on screen. Limits are now per kind of response, and `readBounded` reads one byte past the limit so overflow is detectable and named. The same 1 MB cap was silently corrupting any downloaded skill file over that size.

**How this was verified, and what that does not cover.** Contrast ratios, computed geometry and token coverage were measured in a browser against the real stylesheets rather than asserted — which is how the ghost button, the wrapped Back button and a `.tp-body` class collision were caught. That harness is not Wails, and the release was walked through `wails dev` before the version moved.

---

## 69. Decision — ChatGPT Comes Back, and the Rule Behind §64 Is Narrowed (2026-08-01)

**What changed.** The ChatGPT (`codex`) sign-in §64 removed is restored, on the owner's call. With it come [internal/oauth/codex.go](../internal/oauth/codex.go), the Responses runtime ([internal/model/responses.go](../internal/model/responses.go)) that had `codex` as its only rider, the `RuntimeResponses` constant, the catalog entry and its four aliases, `responsesThinkingCapabilities`, `oauth.Headers`, `startLoopbackAs`, and the `--import` path. Nothing else §64/§65/§66 removed comes back: Claude Pro/Max, Copilot, Qwen's sign-in and Code Assist stay out.

**Why this one and not the others.** §64 removed three flows on one rule — *a borrowed OAuth client against a consumer plan is a standing risk to the user's account, so Aetox will not hold the door open* — and §65/§66 extended it to two more. That rule is too wide, and §66 is what shows it: the two sign-ins it removed were removed for **facts**, not for the rule. Code Assist was a policy Google had announced detection for, against a tier that had stopped answering. Qwen did not complete a sign-in at all. Neither needed the rule. ChatGPT has neither fact against it: the flow works, and no enforcement against it has been observed or announced. What §64 had on it was a hypothetical, and the reversal is that a hypothetical is the user's to weigh, not Aetox's to pre-empt.

**So the rule narrows to this.** Aetox states the risk and lets the user decide; it does not decide for them by removing the button. That is what `RiskRestricted` was built for in §61, and §64 is better read as *deleting a feature to avoid writing a warning*. What Aetox will still not ship is a sign-in with a **known** enforcement record against it — §66's Antigravity rejection stands unchanged and is the live example of where this line now sits.

> **The warning this section kept was dropped by §70,** which is the same argument carried one step further: a caveat with no evidence behind it is not information. §69's reversal of the *removal* stands; only the warning changed.

**The credential is read again, not resurrected.** `codex` leaves `removedProviders` in [store.go](../internal/oauth/store.go), so an `oauth.json` that survived §64 without being written since is read rather than dropped. Nothing is recovered that was purged: §64's filter deleted those entries from disk on the next save, and this change cannot undo that — a user whose store was rewritten in 0.8.1 signs in again, or imports the Codex CLI session. `TestRemovedProviderCredentialsAreDropped` now carries `codex` as its control: the same file, the same load, four providers dropped and this one kept.

**Session import returns with one entry.** `aetox login codex --import` and `ImportableSignIns`/`ImportSignIn` adopt the session the Codex CLI already wrote at `$CODEX_HOME/auth.json`, so someone signed into that CLI does not authorize the same account twice. Still opt-in by name, never automatic — §66's reasoning on that has not changed, only its emptiness has.

**What the account id costs, and where it is checked.** The ChatGPT backend routes on an account id that exists nowhere in the token response body — only in a namespaced claim inside the `id_token`, which a refresh response often omits entirely. `codexAccountFromIDToken` reads it without verifying the signature (the token arrived over TLS from the issuer we asked, and this is a routing id, not an authorization decision), `refreshCodex` carries the previous one forward when a refresh drops it, and `oauth.Headers` is what puts it on the wire. All three were untested after §64 deleted the live test; `TestCodexAccountFromIDToken` and `TestHeadersOnlyForCodex` now pin them, and the live round trip against the real backend is restored in [live_subscription_test.go](../internal/model/live_subscription_test.go) behind `AETOX_LIVE=1`.

**The restricted-risk UI branch has a producer again.** §66 recorded that branch as drawn but unproduced, tested on a synthetic method. It keeps the synthetic seed — what the test checks is that the branch draws for *any* method carrying the flag, and naming ChatGPT there would tie it to one provider's copy. The `device` kind is still unridden; §65's note on it stands. *(§70 removed the producer again — the branch is back to being drawn but unproduced.)*

---

## 70. Decision — The Warning Comes Off Too, and `Risk` Starts Meaning Evidence (2026-08-01)

**What changed.** ChatGPT's sign-in is `RiskOpen`. The `Note` loses its last sentence — "OpenAI's consumer terms may not allow driving your plan from another app — your account, your call" — and keeps what the flow actually needs: which client it presents, whose quota the turns come out of, and that port 1455 must be free. Settings no longer draws the restricted branch for it, and the CLI now prints the `Note` for **every** method instead of only the restricted ones, because with the caveat gone what is left is information a user wants *before* the browser opens.

**Why, on the owner's call.** §69 already argued that §64 removed a feature to avoid writing a warning. Kept to its own logic, that argument does not stop at the removal: the warning §69 wrote was speculation — *may not permit* — about a flow that has worked continuously, against a provider that has taken no action anyone has observed. §64's mistake and §69's warning share a root, which is treating the *shape* of a sign-in as a finding about it.

**So `Risk` is redefined, and this is the durable part.** `RiskRestricted` now means Aetox knows a **specific** reason this sign-in could cost the user their account — an enforcement the provider has announced or acted on. `RiskOpen` is everything else. The bar is evidence, not shape. The practical argument is the stronger one: a warning that fires on every borrowed-client flow is one users learn to click past, which costs exactly the case the warning exists for. §66's two removals survive this redefinition untouched, because both were made on evidence and not on shape — an announced detection policy, a retired tier — and Antigravity is still the worked example of a `RiskRestricted` that would never ship at all.

**What is not claimed.** That OpenAI permits third-party apps to present the Codex CLI's client id. Nobody has established that, and this section does not assert it — the change rests on the absence of enforcement, not on the presence of permission. If that changes, the finding lands in `RiskRestricted` and the flow goes back out under §66's rule, which is why the level and its UI branch are kept alive on a synthetic test rather than deleted.

**No shipped method is `RiskRestricted` now.** The Settings warning branch and `seedRestrictedSignIn` join `device.go` (§65) as machinery that outlives its producer on purpose. Given what the level now means, an empty `RiskRestricted` is the state to expect in a healthy release, not a sign the vocabulary is dead.

---

## 71. Decision — Context Gets Cheaper Four Ways, and the Store Gets Its First Reader (2026-08-02)

Five changes shipped together, all pointed at the same claim: a cheap model with a small window is enough. Four make context cheaper; the fifth makes the §67 store answer questions. None of the techniques is novel — prompt caching, tiered document loading and full-text recall are all well-trodden — and each was taken only where it already fit a decision this document had already made, never because it exists elsewhere.

**Anthropic requests now carry cache breakpoints.** The usage reader has parsed `cache_read_input_tokens` since the beginning — but Anthropic caches *nothing* without an explicit `cache_control` marker, so every Claude call re-evaluated the full prompt at full price while the panel dutifully reported 0 cached. [buildAnthropicRequest](../internal/model/anthropic.go) now marks the three stable prefixes — last tool definition, system prompt, end of history — and from the second call of a session on, the prefix is a 90%-discount read. Only for Anthropic itself: DeepSeek and the other wire-format borrowers cache automatically and never documented the field, so they keep the plain-string system shape — same split, same reason, as `anthropicRejectsSampling`. The system field marshals as a block array when cached, a string otherwise (`anthropicSystem`). This is also why §14's sorted tool names and the frozen-at-bootstrap prompt (§11) now pay off in money rather than only in local prompt-eval time.

**Discovered skills leave the tool block.** Every SKILL.md under `~/.aetox/skills` used to register as its own tool definition, sent with every request — fifty installed skills meant fifty schemas of context, relevant or not, and the cost grew linearly with the library. The model now reaches skills through two flat built-ins ([progressive.go](../internal/skill/progressive.go)): `skills_list` (names and descriptions) and `skill_view` (one body, by name). [Dispatcher.ToolDefinitions](../internal/skill/dispatcher.go) withholds `SourceSkill` definitions; the skills stay *registered*, because the user's `/skill-name` command, the Settings page, and collision detection all still resolve through the registry. Knowing a skill exists is cheap; knowing how it works is paid on use — the same shape as the capability boundary (§44). Both tools scan the discovery paths at call time rather than holding the registry, so a skill installed mid-session is listable before any re-bootstrap. The wrong-name error from `skill_view` carries the real list, so the model recovers in zero extra rounds.

**The prompt teaches batch work.** Nothing used to stop the model from renaming 200 files as 200 shell calls — 200 rounds of schemas and results, which a small-context model does not survive and a paid one re-reads the whole conversation for. `batchWork()` in [prompt.go](../internal/prompt/prompt.go) says: same operation over many items → one script, one round, spot-check after; per-item judgment stays per-call. Zero-cost change, aimed squarely at the models Aetox targets — a 10-line loop is far more reliably written than 200 turns are coherently sustained.

**`tool_runs` becomes searchable, and the model can search it.** §67 shipped the record with no reader on purpose. The first reader turns out not to be the learning layer but recall: schema version 3 adds `tool_runs_fts` (trigram, over the stored clamped columns — the clamp bounds the index exactly as it bounds the table), backfilled in the migration because rows written at v2 would otherwise be the one permanently unsearchable stretch of history. On top of both indexes sits `session_search` ([session_search.go](../desktop/session_search.go)), a workbench tool: the user says "เหมือนคราวที่แล้ว" and the model searches past chat *and* past tool work by content — Thai substrings match, trigram needs three characters and the tool says so rather than silently matching nothing — for the price of a database lookup. Hits name the session, the date, and whether they are from the open session. Desktop-only by nature; the history database belongs to the desktop app. `DeleteSession`'s promise extends through the delete trigger: removed work leaves the index in the same transaction.

**Compaction was already here.** The plan's fifth item — summarize old turns instead of dropping them — turned out to be shipped: `compactIfNeeded` ([agent.go](../internal/cognitive/agent.go)) summarizes at 80% of the char budget, re-checked every tool-loop round, with the verbatim trim in `enforceLimits` kept as the hard floor when summarization fails. What this section changes is one stale comment in [app.go](../desktop/app.go) that still promised it as future work. Noted here because compaction and the cache breakpoints interact: a compaction rewrites the prefix and costs one cache miss by design, rare and priced in.

**Weighed and declined, on purpose.** Messaging-platform front ends, cloud terminal backends, a bundled model subscription, desktop control — all were on the table in the same pass and none was taken: each either fails the one-sentence test ("does it help an ordinary person finish repetitive work on their own machine faster?") or contradicts the local-data line. Ensemble reasoning stays out until ADR 0002 is actually decided, because its cost is multiplied per answer and that argues against the cheap-model claim this section is built on.

---

## 72. Decision — The Chat Learns Two Gestures: Run This, and Do That Later (2026-08-02)

Two owner-requested features, both about what a suggestion in the chat can become without the user retyping anything.

**Run on shell-tagged code blocks.** A fenced block tagged `bash`/`sh`/`shell`/`zsh`/`powershell`/`ps1`/`cmd`/`bat` gets a Run button beside Copy ([markdown.ts](../desktop/frontend/src/lib/markdown.ts)); output lands in a `<pre>` appended inside the same block, via `textContent` — command output is untrusted text and the surrounding markup already passed DOMPurify once. Only those eight tags: `console` and `text` are for *showing* output, `python` and friends are source files, and running either would be a wrong guess with a real side effect. The click is the consent — the command is fully visible in the block, exactly as if the user retyped it into the Workbench terminal, so there is no approval dialog on top. What the click does not bypass is the machinery: `RunChatCommand` ([run_block.go](../desktop/run_block.go)) resolves the `shell` skill from the live registry and calls `ExecuteTool`, never a bare `exec.Cmd` — sandbox working directory, background-shells registry, RTK rewrite (§13), and the shell audit log all apply, so there is exactly one way a command runs in Aetox whether the model called it or the user clicked it.

**Suggested-task chips.** A new workbench tool, `suggest_task` ([task_chips.go](../desktop/task_chips.go)): mid-task, the agent flags side work it noticed but was not asked to do — title, tldr, and a prompt that must stand alone. A chip appears on the composer; clicking it consumes the chip, opens a fresh session, and sends the prompt verbatim ([cockpit.svelte.ts](../desktop/frontend/src/lib/stores/cockpit.svelte.ts) `startTaskChip`); the ✕ declines it. The turn that flagged it never stops — the tool's receipt explicitly redirects the model back to its current work, and the test pins that sentence, because a model that starts doing the flagged task itself defeats the entire feature. Two guards on the input: title+prompt required, and a prompt under 40 characters is rejected with "name the files, the finding, and what to do" — a thin prompt would start a future session that has no idea what it is about, and the retry costs one round now instead of a wasted session later. Chips live in memory for the app run, not in `aetox.db`: a chip is a suggestion, not a record, and persisting declined suggestions forever would make the store a nag list.

**The event seam grew one guard.** `emitTaskChips` does not use `emitEvent`: that helper assumes either the test seam or a live Wails ctx, and this event also fires from the tool-coverage harness where neither exists. A chip added with nobody listening is still added — rendered on next mount from `ListTaskChips`.

**Where this is going.** The chip's producer today is the model noticing things mid-turn. The intended v2 producer is the store: `tool_runs` (§67) plus its FTS index (§71) can eventually notice "this user has done this same job three times" and suggest the automation itself — the chip UI, start-fresh-session flow, and standalone-prompt contract built here are that feature's delivery mechanism, built first because they are useful on their own.

---

## 73. Decision — Aetox Learns Its Own Version, and Asks Whoever Installed It to Upgrade It (2026-08-02)

Owner asked for update detection. Before any of it could be built, the app had to be able to answer a question it could not: **which version am I?**

**The number lived in five places and nowhere the app could read.** `cmd/aetox/main.go` had a const — in the CLI that §30 says is not shipped. `desktop/wails.json` had `productVersion`, which Wails bakes into the exe's Windows version resource, i.e. into a place Go cannot read back. README's status heading, `docs/index.html`'s badge and `scoop/aetox.json` had three more copies. Bumping five by hand worked only because nothing ever read them; the moment the app starts *checking* for updates, a missed bump stops being a typo and becomes the app lying to the user in the one place they went to find out. So: [internal/version](../internal/version/version.go) is now the single constant, and [version_test.go](../internal/version/version_test.go) asserts the four non-Go copies agree with it. It rides `go test ./...`, which CI already runs on all three platforms — a stale copy fails the build instead of shipping. The tag is the only copy that lives outside the repo, so [release.yml](../.github/workflows/release.yml) checks that too, before anything is built. Deliberately a plain const and not an `-ldflags` stamp: `wails dev` and `go run` are how this app is actually run all day, and a stamp leaves both reporting empty — the update check would be dark exactly where it is being developed.

**Then the real decision: never upgrade ourselves.** The obvious design is to download the new exe and overwrite our own binary — [minio/selfupdate](https://github.com/minio/selfupdate), or what the Wails v3 tutorial shows (Wails v2 has no updater; [wails#1178](https://github.com/wailsapp/wails/issues/1178) is still open). It needs a signing key and a signed feed to be safe, it needs elevation because our NSIS build lands under `$PROGRAMFILES64` with `RequestExecutionLevel admin`, and on macOS replacing your own bundle without notarization breaks Gatekeeper. That is three infrastructure problems bought to solve one convenience.

opencode answers it differently and better: ask *how you were installed*, then hand the upgrade back to whoever installed it — `scoop install`, `brew upgrade`, `npm i -g` — and never touch your own files. That is how it supports eight install methods without owning an updater. [internal/update/channel.go](../internal/update/channel.go) copies that design. `Detect()` reads `os.Executable()` (and its symlink target, since Scoop is a symlink farm) and classifies: `/scoop/apps/` → Scoop, under Program Files or `%LOCALAPPDATA%\Programs` → installer, any other Windows path → portable, everything else → unknown. Scoop gets `scoop update aetox` and nothing else — the About page deliberately does **not** also offer that user a download button, because inviting them to unpack a zip over a Scoop-managed install is the exact mess this design exists to avoid. Every other channel gets the release page, which is never the wrong answer.

**The test for adding a channel is not "does this platform exist" but "can we beat 'open the releases page' here".** Homebrew, AppImage and dpkg are named in the comments as the cases to add when §48's port lands, and are deliberately not guessed at now: untested detection that prints the wrong upgrade command is worse than the fallback it replaced. `detectFrom` takes the executable path, the symlink resolver, the OS and the environment all as parameters, so its table runs on the Linux and macOS runners too — the paths it classifies are Windows-shaped *strings*, and nothing about them needs Windows to check. That injection is what makes the Scoop shim case testable at all: what actually runs is `scoop\shims\aetox.exe`, which only reveals itself as Scoop once the symlink is resolved, and no test can conjure that farm on Windows without privileges.

**The check itself fails open, and is off in tests.** [update.go](../internal/update/update.go) hits `releases/latest`, which excludes drafts and prereleases — release.yml publishes every release as a draft, so an unpublished build can never notify anybody by accident. An ETag is cached in `DataRoot()/update-check.json`, making the repeat call a bodiless 304; that is what keeps this inside GitHub's 60-requests-per-hour budget for unauthenticated callers. Offline, rate-limited, proxied: error returned, nothing else changes, and the About page says "nothing is broken" rather than printing a Go error at the user. `AETOX_DISABLE_UPDATE_CHECK` switches it off entirely (opencode's `OPENCODE_DISABLE_AUTOUPDATE`, same two reasons: privacy, and CI). And `Check` refuses to use the real API from a test at all — `internal/rtk/install.go` learned that lesson when a clean CI runner downloaded a third-party binary mid-suite.

Version comparison is numeric, never string: `0.8.10` is newer than `0.8.9` and a string compare gets that exactly backwards. Anything unparseable returns false — a check that shows no banner is much cheaper than one that nags about a release that does not exist.

**Settings → About** ([Settings.svelte](../desktop/frontend/src/lib/Settings.svelte)) shows the installed version, the detected channel, and a manual check button. Four outcomes, four different sentences — up to date, newer release, could-not-check, switched-off — because "you turned this off" is a choice the user made and must not render as a failure.

**Explicitly not built, so it is not re-argued:** no self-replacing binary and no sidecar updater (signing infrastructure, elevation, notarization — see above); no update server of our own, since GitHub Releases already is the source of truth; and no silent background download, which would contradict the "nothing is fetched without asking" promise `scoop/aetox.json`'s own notes field makes to every Scoop user.

**Still open (owner-approved, next):** the automatic once-per-24h check with a Settings toggle, and — for the installer channel only — downloading the new installer, verifying it against the release's `checksums.txt`, and running it. The installer already taskkills the running app, so that path needs no new process handling; it needs the toggle and the download-verify step, and it is the one channel where Aetox can do better than a link.

---

## 74. Decision — A Turn Is a Sequence, Not a String (2026-08-03)

Owner reported a small thing: on DeepSeek the *thinking* streamed live but the *answer* landed in one silent jump, long enough that the app read as hung. Chasing it ended at the shape of a turn.

**Why the answer was withheld.** `completeToolLoop` ([agent.go](../internal/cognitive/agent.go)) passed a hardcoded `nil` content handler to `StreamComplete` and a real one for reasoning — so thinking streamed and prose did not. That was not DeepSeek-specific and not a bug in the ordinary sense: every shipped provider hardcodes `SupportsToolCalling() → true` and the desktop always registers ~30 builtin skills, so `agentCanUseTools` at [executor.go](../internal/turn/executor.go) is never false there and `RespondStream` — the only path that streams reply text — is structurally unreachable in a real desktop build. The one configuration that *did* stream the answer was the keyless demo provider; streaming stopped the moment a user added an API key. The stated reason for the `nil` was leaked DSML tool-call markup (§17's backstop only lifts out *complete* blocks), and that reason is real but narrow: DeepSeek's catalog entry defaults to `RuntimeAnthropic`, where tool calls arrive as their own content blocks and a leak cannot occur at all.

**The real blocker was the string.** A tool loop writes text every round, and only the last round's text was "the answer". Everything else had to be classified and, if streamed, taken back: narration in front of a tool call, a round nudged away for leaking markup, a round that came back empty, a finished reply demoted to narration because the user typed under it (`Interject`, §59). An audit of the loop found **fifteen** distinct places that had to answer "is this text the reply?" — a question no provider protocol asks, because Anthropic and OpenAI both stream a turn as *ordered blocks* and the whole sequence is the assistant's turn.

**So the turn keeps its shape.** [turn/part.go](../internal/turn/part.go) adds `TurnPart` — `text`, `thinking` (its duration), `tool` — and the executor assembles the sequence as it happens, from callbacks it already had: `OnRound` for prose and thinking segments, its own tool callback for the work. It was throwing that order away and keeping the last sentence. `Result.Parts` now carries it; `Reply` survives beside it as the concatenation of the text parts, so the model's context, the store, session titles, FTS and sub-agents read exactly what they read before and none of them learn what a part is.

Two of the fifteen compensation points disappear outright, and they are precisely the two the string model invented: narration is a part rather than something to retract, and an interjected-over answer is a non-final part rather than a deletion. Three remain and are honest — a round that leaked markup, one that came back empty, one that failed — because that text never entered the turn at all. Reducing compensation from five call sites to three is the measurable form of the argument.

**The sequence is persisted** (schema 5, `messages.parts`). This is what it was actually worth: a tool call had never been written anywhere a message could reach, so the toggles under a reopened answer came back empty. The same migration finally stores `reasoning` and `think_secs`, which had been fields on `SessionMessage` since thinking panels shipped and were written to nothing — every reload silently dropped them.

**Streaming, with the gate the old design needed.** [dsml_stream.go](../internal/model/dsml_stream.go) sits between the provider's content deltas and the screen and withholds any tail that could still become an opening marker, latching shut on a complete one. It buffers rather than filters because a marker arrives split across deltas as the *normal* case — `<｜DS` in one frame and `ML｜invoke` in the next — and a per-chunk check would pass both halves through. Seven runes of lookahead; `<div>`, `a < b` and `List<String>` all flow. `RespondStream` was gated too: that path has no DSML backstop at all and tells its caller the text is already on screen, so a leak there would have been permanent rather than merely early.

The live preview is explicitly **not** a delivery. The reply still arrives exactly once, and one event carries a `replace` flag so a fragment, an erase and the finished answer ride the same ordered channel — which makes a doubled answer structurally impossible rather than merely unlikely.

**The sequence is a record, not a layout.** Drawing it inline in the bubble was built, shipped to the owner, and rejected within the hour: the same thinking segment appeared twice (once as the toggle, once between paragraphs) and a tool row every few sentences stopped the answer being the thing the eye lands on. The bubble is back to one answer with a row of collapsed toggles above it, and `stepsFromParts` ([cockpit.svelte.ts](../desktop/frontend/src/lib/stores/cockpit.svelte.ts)) folds the stored sequence into those existing rows. The model underneath was the point; the layout was a judgement call and it lost.

**Asking the same question again.** Retry a failed turn, answer again keeping the previous answer switchable, flip between answers, edit the question and resend ([regenerate.go](../desktop/regenerate.go)). Four features, one problem: put the conversation back the way it was, then run it again. That is engine work because a turn lives in three places that do not fail together — the bubble list, `messages`, and `memory.Context` — and a turn that *errors* reaches the model's memory and neither of the other two, since the user message is added to context before the provider is called. Every entry point rebuilds the context from the transcript, the only record written after the fact. Variants are stored as JSON beside the live answer (schema 4) rather than as extra rows, so `text` stays the live one.

Answering again when the turn wrote files is the one genuinely dangerous case — its tools would run a second time on top of their own output — so the UI confirms, reverts first, and the bubble names the files that went back. There is deliberately no one-click "answer again and keep the files": that is the footgun, and leaving it out is cheaper than explaining it.

**Also found and fixed while in here.** A failed tool loop returned `handled=false`, which is what the guard clauses return for "does not apply" — so `execute()` fell through and re-sent the turn as a second, non-tool completion; one DNS failure cost three provider calls and put the question in the model's memory twice. `retryTransport` ([httpclient.go](../internal/model/httpclient.go)) retried statuses but not transport failures, so a two-second Wi-Fi blink threw away a whole turn. `livePanel` was set once for the component's lifetime, so collapsing the thinking panel once left every later turn in that session showing nothing but bouncing dots.

**Still open.** The live bubble still renders a single string while a turn runs and redraws as the collapsed layout when it ends — a visible seam. A delegate's own steps are not in the sequence (only the `task` call is), though `tool_runs` has stored them with `parent_ref` since §67, so joining them back is a feature, not a migration. And `Reply` is still the primary string rather than a derived accessor; removing it is a cleanup pass across every caller, deliberately not bundled with a release.

---

## 75. Decision — The Agent Learns to Hand Back a File Excel Opens (2026-08-03)

The landing page promises **finished work**, and everything the agent could return was text. For the people this is built for a `.md` file is not finished work — a file that opens in Excel is. The site's own flagship example ends at a table ("a pile of slips becomes a table you can use") and the best the tools could produce was a CSV: no header, no column widths, and Thai that Excel mis-decodes often enough that the first thing the user sees is garbage. `write` is `os.WriteFile(path, []byte(content))` ([write.go](../internal/skill/write.go)); nothing in the repo could emit a binary at all.

**The library question was settled by measurement, not preference.** `.xlsx`, `.pptx` and `.docx` are all ZIPs of XML (OOXML/ECMA-376). The two Go libraries covering all three — unioffice and gooxml — are AGPLv3, which Aetox cannot take on top of Apache-2.0 without making the whole project AGPL; that gate is closed for the same reason Windmill was rejected as a base. The one permissive option, excelize (BSD-3), covers xlsx only. `OFFICE-EXPORT-PLAN.md` (plan doc, since removed) §4 set the deciding number in advance — go with a hand-written builder if excelize costs more than ~3 MB — and it costs **5.5 MB on a 15.8 MB CLI binary**, measured by adding it and building. The hand-written package costs **117 KB**, 47× less, for the same phase-1 capability. Deciding on the number rather than the argument mattered, because the argument alone was already one-sided: excelize's advantages are formulas, charts and rich styling, all of which the plan's own §8 rules out of scope.

**One container, three formats.** [internal/ooxml](../internal/ooxml) is the ZIP-of-XML writer plus the xlsx parts on top of it; `.pptx` and `.docx` are further part sets on the same container, which is the whole reason the shared piece was built first — pptx needs a hand-written builder regardless of what happens to xlsx, so building it once buys all three and no new dependency ever.

**What the model is not asked to know.** `sheet_write` takes `{path, sheets: [{name, columns, rows}]}` as ordinary JSON and nothing about OOXML — the same division of labour as `image_ocr` against a model with no vision (§51). A small local model that can only type still returns a working spreadsheet, because the part it cannot do is not asked of it.

The details that decide whether the file is worth anything are all in the architecture, not the prompt: a number sent as a JSON number becomes a numeric cell so `SUM` works (text that looks like money is what makes an export worthless for the accounting job it was made for); an ISO date becomes Excel's day-serial plus a number format, not a string; `"0012"` stays text, because coercing it destroys an invoice number; `03/08/2026` stays text too, because it is 3 August here and 8 March in the US and a silent wrong guess is worse than no guess. Control characters — which arrive routinely from OCR of a scanned slip — are stripped rather than escaped, since one is enough for Excel to declare the whole workbook unreadable. Thai vowels and tone marks are excluded from column-width measurement: they stack on the consonant, and counting them makes every Thai column half again too wide.

**It is gated like `write`, because it writes.** `AssessCommand` ([safety.go](../internal/safety/safety.go)) answers `RiskLow` with no effects for any skill name it does not recognise, so a file-producing tool without an explicit entry would have skipped the approval prompt that `write`, `edit` and `delete` all pass through. It shares `write`'s sandbox root and session output folder through `placedWrite`, extracted so the two cannot drift.

**What CI can and cannot prove.** There is no Excel on a build machine, so the tests assert what is checkable — the package unzips, every part is well-formed XML, the parts the spec requires are present, content types and relationships resolve in both directions, numbers are numeric cells and dates are styled serials — and the file is opened by hand once per phase. Phase 1 is xlsx; `.pptx` (title, bullets, image, speaker notes) and `.docx` follow on the same container.

---

## 76. Decision — The Workbench Loses the Review Panel, and Gains a Way Out to the Real Program (2026-08-03)

Two changes to the right-hand workbench, both from the same question: what is that surface actually for, now that the agent finishes work rather than only edits code?

**Review is gone.** It offered four sections. Two of them — the diff and the test run — read `cockpit.diff` and `cockpit.test`, which were initialised empty in [types.ts](../desktop/frontend/src/lib/types.ts) and **written by nothing, ever**: they had shown an empty box since the day they shipped. The two that worked, files-changed and command-history, duplicated what the file tree already says — `ProjectTree` folds `GitChangedFiles` in per node, which is where the M/U badges in the Files panel come from. So the panel was half dead and half redundant, and what remained was a code-review surface in a product whose promise is finished work for people who do not read diffs. Removing it also removed four store fields and one redundant git round-trip per project load. `App.CommandHistory` is now called by nothing and is the obvious next thing to cut.

The restore path skips a `review` tab in a layout saved before this rather than failing on it — an old session's other tabs still have to come back.

**A file tab now has a way out.** §75 gave the agent a tool that writes `.xlsx`, and `ReadFile` correctly refuses a ZIP — so clicking the workbook the agent had just announced opened an editor containing the words "binary file cannot be previewed". `App.OpenFileExternally` hands the file to whatever program the OS opens it with, through the same `openInFileManager` the three reveal buttons already share, and [ExternalFilePane.svelte](../desktop/frontend/src/lib/workbench/ExternalFilePane.svelte) offers that instead of an editor full of an error.

Deliberately not a preview. Rendering a spreadsheet in here would mean writing an OOXML *reader* — out of scope by OFFICE-EXPORT-PLAN.md §8 — to build a worse Excel than the one already installed on the machine the app promises to work on. The same pane covers `.pptx` and `.docx` when they land.

**But the file should not have to be looked for at all.** Reaching it still meant opening the file panel and finding it in the tree: four clicks and a guess about where to look, from a product that had just said the work was done. The question "where does a produced file appear" was answered *in the chat, under the answer* — the workbench is where you go to look at something, and a deliverable should arrive instead.

`skill.Output.Artifacts` carries it, and the field is modelled on `Images` one line above it: the tool hands back the thing rather than a sentence about the thing. It rides the existing `ToolEvent` feed to the UI, so no new channel and no new panel — `cockpit.turnFiles` accumulates during a turn and `turnArtifacts()` freezes it onto the reply, exactly as it already did for tool steps and reasoning. The chat draws a card per file with a button that calls `OpenFileExternally`.

Only tools whose *whole output is a file* set it. `write` and `edit` deliberately do not: a coding turn touches a dozen source files, and a card for each would bury the one that matters under noise it created itself. Like `steps`, it is not persisted — a reloaded session shows the answer without the cards, and the files are still in the file panel.

---

## 77. Decision — The Deck Writer, and Why It Ignores the Format's Own Layout System (2026-08-04)

Phase 2 of OFFICE-EXPORT-PLAN.md. `slides_write` produces a real `.pptx` on the same container §75 built, and the whole of phase 2 cost **308 KB** on top of phase 1 — 443 KB for both, against a 3 MB budget. Most of that is not the deck code: it is `image/png`, `image/jpeg` and `image/gif` being linked in to measure an embedded picture.

**Explicit shapes, not placeholders.** The idiomatic way to build a slide is to inherit position, size and text style from a layout, the layout from a master. It is also the fragile way: the inheritance chain is where PowerPoint makes its own decisions about what an incomplete file means, and getting one link wrong yields either text somewhere unintended or the repair prompt — neither of which is diagnosable from a machine with no PowerPoint on it. Every shape here carries its own geometry in EMU, and the master and layout exist only because the format refuses a presentation without them. The one deliberate exception is the notes placeholder: the presenter's pane finds its text by placeholder type, so a plain text box there would store the note and show nothing.

**Thai was the whole risk, and it is a two-part fix.** OFFICE-EXPORT-PLAN.md §7 named it first: Thai is a complex script, and with no `cs` typeface on a run PowerPoint chooses a complex-script font itself. The one it chooses routinely has different metrics from the Latin face, which is what leaves vowels and tone marks floating beside their consonants instead of over them. Every run names `+mn-cs`, and theme1.xml carries `<a:font script="Thai" typeface="Leelawadee UI"/>` — the format's own mechanism for "use this face for this script" — so the reference resolves to something chosen rather than to whatever is convenient. Naming the typeface without the theme entry, or the theme entry without the run attribute, fixes nothing.

**Four features, on purpose.** Title, bullets, one picture, speaker notes. That is where a hand-written OOXML writer stops being a file format and starts being a second PowerPoint; charts, animation and custom masters are ruled out by the plan's §8 and stay out.

**Failing loudly on a bad picture is a feature.** A deck that silently dropped the chart it was asked for is worse than one that was never written, because the user sends it before noticing. The image is decoded to validate the format and to measure it — the aspect ratio comes from the file, since a guessed one is a stretched screenshot, and a chart squashed 20% is misleading rather than merely ugly. The image path is sandbox-checked as carefully as the destination: it is the second way out, and the easier one to forget, since a picture read from anywhere on disk would be embedded into a file the user then sends to somebody.

**A deck with no notes ships no notes master.** An empty `notesMasterIdLst` pointing at a part that is not there is exactly the dangling link that makes PowerPoint offer to repair the file, so the part appears only when some slide actually has notes.

`.docx` is phase 3 and the cheapest of the three: `word/document.xml` plus `word/styles.xml` on a container that now has two formats' worth of proof behind it.

---

## 78. Decision — The Document Writer Closes the Set (2026-08-04)

Phase 3 of OFFICE-EXPORT-PLAN.md, and the cheapest of the three exactly as the plan predicted: a document is a flat run of blocks, with none of xlsx's cell typing and none of pptx's coordinate geometry. `doc_write` produces a real `.docx` — headings, paragraphs, bullet and numbered lists, tables — on the container §75 built. All three phases together cost **515 KB** against the 3 MB budget, and `go.mod` has not moved a line since the work started.

**The Thai fix is a third variant, not a third copy — and this one was nearly got wrong.** Word keeps a *separate font, size and language for complex scripts*, and Thai reads that set: `w:cs`, `w:szCs`, `w:lang w:bidi`. Bolding a heading with `w:b` alone puts regular-weight Thai beside bold Latin, and the complex-script twins sit *interleaved* in the schema sequence (`bCs` right after `b`, `szCs` right after `sz`) rather than appended — so the Thai fix and the ordering hazard land in the same line of code.

The part that would have shipped broken: setting only `w:cs` is correct for Word and **wrong for Google Docs**, which has no complex-script model at all and reads `w:ascii`/`w:hAnsi`. Two of the three target readers disagree about which attribute to read, so the only setting that satisfies both is all four font slots naming one family. And unlike pptx, the answer here is to ship **no theme at all**: `w:cstheme` does not fall back to `w:cs`, it replaces it.

**This was found by research, not by testing.** Before writing the generator, six parallel agents were pointed at six dimensions of docx correctness — part manifest, Thai, styles, lists, tables, and what specifically makes a reader refuse a file — and their findings were merged into one checklist and then run against the implementation. It caught nine defects. Three mattered: the font-slot bug above; every list block sharing one `w:abstractNum`, which makes a second numbered list count 3, 4, 5 instead of restarting, because numbering state lives on the abstract definition rather than on the reference; and a table with no explicit `w:tblCellMar` or dxa `w:tblW`, whose text touches the borders and whose column widths are merely advisory. The remaining six were smaller and equally invisible from a passing test suite. The lesson is the one this file already records for OCR and for vision: the details that break a format are the ones you do not know to look for, and a machine with no Word on it will not tell you.

One Thai detail is specific to documents: line spacing is `auto`, never `exact`. Thai stacks a tone mark above a vowel above the consonant, and an exact line height clips the top of that stack — which looks fine on screen and is wrong on paper.

**Ordering is the format's main trap.** Every complex type in WordprocessingML is a *sequence*, so `w:spacing` before `w:pStyle` inside a `w:pPr` is not untidy, it is a document Word offers to repair. `w:sectPr` must be the last child of `w:body`. Every `w:tc` must end with a `w:p` — a cell holding only its `w:tcPr` is the single most common way a hand-built table is declared unreadable. Two tables in a row merge into one unless a paragraph separates them, and a body that ends with a table before `sectPr` is malformed rather than merely unusual; both are handled by emitting an empty paragraph, and both are pinned by tests.

**A heading is a heading because of `w:outlineLvl`.** Without it Word renders large bold text that never reaches the navigation pane or a table of contents — right on screen, wrong in every way that matters. And the bullet is a plain U+2022 in the document's own font rather than Word's usual F0B7 in Symbol: Symbol is a legacy symbol-encoded face whose character is only a bullet while that exact font is applied, and Google Docs and LibreOffice routinely draw it as a box.

**One new test reaches back across all three formats.** A part that is in the package, has a content type, and has no relationship pointing at it is *silently ignored* — the file opens clean and the feature is simply absent, with nothing anywhere saying why. `TestEveryPartIsReachableFromARelationship` now asserts the reverse direction for `.xlsx`, `.pptx` and `.docx` together. It passes today; it exists because the failure it guards against is undetectable by reading the output.

**What CI still cannot prove** is unchanged from §75: there is no Word, PowerPoint or Excel on a build machine. The tests assert that the package unzips, that every part is well-formed XML, that the required parts exist, that content types and relationships resolve in both directions, and that the specific traps above are absent. Worth stating plainly about the research too — every rendering measurement in it came from Word on Windows; the Google Docs and LibreOffice claims are reasoning from files on disk, not measurement.

**Closed out by hand on 2026-08-04.** The owner opened a generated `.xlsx`, `.pptx` and `.docx` on their own machine and all three passed — no repair prompt, Thai intact. That is the only evidence CI can never produce, and it is what makes these three writers shippable rather than merely green. Still unverified, and worth doing before anyone relies on it: **Google Docs / Sheets / Slides and LibreOffice**. Those two are exactly where the research had no measurements either, and several of the docx choices — the four matched font slots, the U+2022 bullet, shipping an explicit `ListParagraph` definition — were made *for* Google Docs specifically and have never been seen working there.

---

## 79. Decision — A Workbook Gets a Grid; a Deck and a Document Do Not (2026-08-04)

§76 said the answer to "I cannot render this file" was a button that hands it to the program that can, and argued against an in-app preview: rendering a spreadsheet would mean writing an OOXML *reader*, ruled out by OFFICE-EXPORT-PLAN.md §8, to build a worse Excel than the one already installed. That argument was too wide. It is right about an *editor* and wrong about a *glance* — and the owner asking to see the file in the right-hand pane is a reasonable thing to want after the agent says the work is done.

**So the position narrows rather than reverses.** `.xlsx` opens as a read-only grid. `.pptx` and `.docx` keep going to the open-externally card, and that is not a "later" — a slide preview is a rendering engine and a document preview is a layout engine, which is exactly the line §76 drew. A spreadsheet is different in kind: it *is* a table, and a table is what a pane can draw without pretending to be anything.

The read-only part is load-bearing. Nothing round-trips, every value crosses the boundary as a display string already formatted the way Excel shows it, and the open-in-Excel button lives in the preview's own header rather than somewhere else. It answers "what did I just get?" and stops.

**It reads more than we write, on purpose.** Aetox's writer uses inline strings; every workbook produced anywhere else uses a shared-string table, and a preview that worked only on Aetox's own files would be the more confusing half-feature. So `ReadXLSX` handles shared strings and their rich-text runs, both the reserved and the custom number formats (a date in xlsx is a day count plus a format — without the format lookup the preview shows `46237` where Excel shows a date, which is the writer's own bug in the other direction), formula cells by their cached value rather than their expression, and sparse rows by cell reference so a missing cell does not slide the rest of the row left. Quoted literals in a format code are skipped when deciding whether it is a date, or a Thai currency format carrying a month abbreviation in quotes would turn a column of amounts into dates.

**The limits are stated rather than silent.** 500 rows, 64 columns, 20 sheets, 8 MB — a preview is a glance, and the whole thing is marshalled to JSON across an IPC boundary. A truncated sheet says so and gives the real row count, because a prefix displayed as if it were the whole file is the one failure here that misleads instead of merely annoying. A workbook the reader cannot make sense of falls through to the card rather than becoming a dead end: it is still a file Excel can open.

---

## 80. Decision — The Workbench Is a Desk, So Things Can Be Put Down On It (2026-08-04)

The right-hand panel could only ever be filled from inside the app: the `+` menu, `Ctrl+T`/`Ctrl+P`, a link clicked in chat, or the agent opening a page itself. The owner's words for it were "โต๊ะทำงานของ AI" — the AI's desk — and the obvious thing to do with a desk was the one thing it refused. Dragging a file onto it did nothing, or worse, opened it in the *main* editor on the other side of the window, because the only file-drop route was window-wide (`App.svelte`'s `OnFileDrop`) and it knew nothing about where the drop landed.

**Where a drop lands now decides what it means.** `OnFileDrop` gives window coordinates, which is already how an image dropped on the composer became an attachment; the panel's rect is the second test. Over the desk, files open as tabs there. A collapsed panel is `display:none`, so its rect is empty and nothing can land in it — no separate check.

**The native browser window has to stand down for the length of a drag.** A browser tab's page is a real OS window composited over that pane (§ browser.go, `BrowserSetBounds`), and it does not merely cover the DOM — it *swallows the drag*, because the pointer is over a different window. A drop target that worked on every tab except the browser would be the worse half-feature, so `Workbench.svelte` watches for a droppable drag anywhere in the app and `BrowserPane` hides its window while one is in flight, the same standing-down it already does for the settings overlay and the `+` menu. The end of the drag is inferred from `dragover` going quiet: a drag that began in Explorer and wanders back out fires no `dragend` here, because it was never ours to end.

**A file from outside the project is copied in rather than refused.** Everything below the desk reads through the sandbox, so without the copy the honest answer to a file dropped from `Downloads` would be "path is outside project root" — a rule about this program's internals, not an answer about the file. It takes the same route the paperclip button takes (`SaveChatFile`, session-scoped, dies with the chat), which has the second effect that matters more: the agent can now read what was put on its desk. A file already inside the project opens where it lies and is not copied. The copy's generated name is an implementation detail, so the tab carries the name the user dropped.

**Images get a pane; §79's line holds everywhere else.** §76 said "hand it to the program that opens it" and §79 narrowed that for spreadsheets. An image narrows it again for the same reason and no further: the webview *is* an image renderer, so there is nothing to reimplement, and answering someone's own screenshot with "binary file cannot be previewed" is the failure §76 was written about. Decks and documents still go to the card. `ReadImageDataURL` grew the size cap it never had, since a data: URL costs about 4/3 its file size as a string in the DOM and the desk opens whatever is dropped on it.

**A drop that fails still opens a tab.** Too large, no project open, unreadable — the desk says so. Silence would read as the panel ignoring the drop, which is the exact complaint this whole surface exists to answer.

**The chat's file cards are draggable, and that is the case this was built for.** The file the agent just made is already on screen as a card; dragging it to the desk opens it there. The card, a workbench tab, and a page dragged off a real browser all travel as the same `application/x-aetox-tab` payload the composer already accepted. (Clicking one went to the OS when this was written; §81 turned that around.)

---

## 81. Decision — The Desk Is the Default Destination, and a New Tab Shows Where the Agent Has Been (2026-08-04)

Two changes with one argument behind them. §80 made the workbench a place things can be put down; this makes it the place things land by default, and gives its emptiest surface something true to say.

**Clicking a file the agent produced now opens it on the desk, not in another application.** §76 was right that Aetox has no business being a worse Excel, and §79 narrowed that to "a glance is not an editor" — but the click still went straight to `OpenFileExternally`, which made handing the file away the default and looking at it the special case. For a panel whose entire job is showing finished work that is backwards, and the practical result was that the answer to "what did I just get?" was a window from another program landing on top of Aetox. The owner's framing settled it: *"ไฟล์ที่ AI สร้างเสร็จ มันต้องเปิดด้วยโต๊ะทำงานของ AI เป็นค่าเริ่มต้นสิ ถ้าจะเปิดด้วยแอปภายนอก ก็ทำเป็นปุ่มแยกในนั้นเอา."* The way out is not gone, it is one click further in — every pane a file can land in carries its own open-externally button, which is now a requirement of adding a pane rather than a courtesy. `ImagePane` gained one for exactly that reason.

**The error line under the card is gone with it, and that is a repair.** `openFileTab` cannot fail into nothing: a file it cannot read opens a tab that says why and offers the program that can. A red sentence under a card was a worse version of a card that already existed.

**A new browser tab is not a blank slate.** It said "เริ่มท่องเว็บ / พิมพ์ URL เพื่อเปิดหน้าเว็บ" — two lines casting the panel as the user's browser and leaving them to supply everything, on a surface §80 had just called the AI's desk. It now shows the pages Aetox has opened itself, newest first, one click from loading in that same tab.

**The list is a read of something the app has always written and never once read back.** Every `browser_open` is already a `tool_runs` row. `RecentAgentPages` is that read: no new table, no migration, no localStorage, nothing written by this feature at all. It is machine-wide rather than per-project or per-session, because a page the agent opened is not project data, and scoping it through sessions would hide the row the list exists for.

**It parses our own sentence, deliberately.** `browser_open` writes `เปิดแล้ว: <title> (<url>)` into `RawOutput`, which is what the *model* reads — turning that into JSON for the convenience of a panel would change model-facing text to save a function. The format and its parser share one constant and sit in the same file, and a round-trip test builds the string the way the skill does and feeds it to the parser, so drifting them apart fails a test rather than silently emptying the list. A row that will not parse falls back to the URL in its arguments: a format change costs the title, never the list.

**Nothing on it can be a dead end.** A `file://` row whose file has since been deleted is dropped rather than shown (session output folders age out), the argument fallback refuses a sandbox-relative path because `https://out/report.html` is exactly the dead click this page forbids, and a failed `browser_open` is not somewhere the agent has been. A database that will not open renders the same sentence as an empty history, because "there is nothing to show you" is the honest content of both.

**What was refused, and why it matters more than what was built.** No search box — the address bar is 46px above, and a second input is the two-doors bug §38 deleted; a web-search default would also ship keystrokes off the machine on every new tab. No local-port scan: it answers a developer's question on a surface whose test is ordinary repetitive work, and it is the only candidate that would have added a promise to the README's privacy table. No speed dial, favicons or thumbnails — each is an outbound request per tile the instant a tab opens.

**The user's own browsing was refused too, and that lasted about an hour.** The argument was that the history only belongs in this product if it is the agent's. The owner opened the finished pane, saw three rows that were all files Aetox had generated, and asked the obvious question — *"ทำไมมันแสดงแค่ไฟล์ที่เคยเปิดล่ะครับ ทำไมไม่แสดงเบราว์เซอร์ที่เคยไปหรือเข้าด้วย."* A list of "pages opened here" that silently omits the ones you opened yourself is not restraint, it is a list that is wrong. So both sources now feed one list, deduplicated by URL, newest record winning.

**But it is not written where the agent's opens are, and that distinction is the whole of what survived the reversal.** `tool_runs` feeds `tool_runs_fts`, which `session_search` reads as the agent's own memory — putting the user's personal browsing there would quietly make it agent-readable, which is a far bigger decision than a start-page list gets to make. The user's navigations live in `localStorage` under `aetox-browser-history`, capped at 200, in the UI layer where the workbench's own layout already lives. `file://` pages are deliberately *not* recorded there: the frontend cannot check that a file still exists, and the Go side already covers those with that check. The no-dead-end rule outranks completeness.

**A related cap was quietly stealing space this whole time.** The side panels are capped so the grid cannot outgrow the window, but the arithmetic subtracted a *collapsed* panel's remembered width and counted its hidden resize handle — so with the sidebar closed, a 1417px window offered the workbench 765px instead of 1051px, and dragging simply stopped with nothing on screen to explain why. The rule was right and the numbers were wrong, which is the kind of bug that cannot be seen, only measured; the arithmetic now lives in `lib/panelSize.ts` with a test that pins both figures. Uncollapsing a panel re-fits the other one, since the space it reclaims has to come from somewhere.

**The page never grows a button that leaves.** "Show all" expands the same list in place. A start page for a browser tab that offered to open a different kind of tab would be answering a question nobody asked there.

---

## 82. Decision — The Floor Under Learning: a Score, a Place to Write, a Door, and a Scope (2026-08-04)

Owner: *"วันนี้เราจะทำฐานตรงนี้ให้เสร็จ แล้วต่อยอด กับระบบเรียนรู้ที่จะสร้างสกิลเองและปรับปรุงพรอมป์ของตัวเองได้"* — finish the floor, then build the thing that writes its own skills and improves its own prompts on top of it. What follows is only the floor. Nothing here learns anything; it is the four things that have to exist first, and the reason each of them cannot be added later.

**The gap was not memory, it was measurement.** §67 gave the store `tool_runs` — every call, its arguments, its output — and §71 gave that a reader. What none of it could answer is whether any of the work was any good. `tool_runs.ok` means "the tool did not return an error", which a confidently wrong OCR passes as easily as a correct one. A system that cannot tell a good run from a bad one cannot improve itself; it can only change itself, and call the change improvement.

**`jobs` (schema v7) is one row per unit of work with a result somebody could judge** — a chat turn, or one `task` handed to a delegate. Three of its columns carry the design. `tool_seq` renders the work as an ordered shape (`read>image_ocr>sheet_write`), which makes "this job has happened five times" a `GROUP BY` rather than a model call — the difference between repeat detection that can run on every turn and one that costs a request. `outcome` is the score, with `outcome_source` beside it so a verdict a person gave is never confused with one inferred from behaviour. `agent` is the scope. Delegate rows are *reconstructed* from `tool_runs` at turn end rather than captured separately: the `task` call's own arguments are the brief, its output is the result, and its ref stamps the child's calls — one recording path, so there are not two that can disagree about what a job is.

**Scoring a turn had to cost the user nothing, so two signals feed it.** The thumbs under a reply are the deliberate one, and pressing the lit thumb withdraws the verdict, because a rating you cannot take back is one people stop giving. The free one is "answer again" — a user pressing that has already said the answer missed, and it is the only negative signal that arrives without anyone doing something they would not have done anyway. It never overwrites a thumb, and the regenerated attempt is a *second* row against the same bubble rather than a replacement, because "this shape of work needed two tries" is exactly what a later pass should be able to see. An edited question deletes its job rows instead of marking them bad: the user asked for the wrong thing, and recording that as agent failure would teach the learning layer to avoid whatever it happened to do right.

**Nothing about this is backfillable, which is why it shipped before anything that reads it.** Someone three months into using Aetox cannot go back and rate two hundred answers.

**`internal/learned` is the place to write, and it is a folder of markdown.** The store would hold it more cheaply, and that is the argument against: a memory only Aetox can read is one the user cannot take anywhere. `MEMORY.md` for the main agent, `agents/<name>.md` per delegate, under `<DataRoot>/memory` — separate from `identity/` so that "delete everything it thinks it learned" is offerable without also deleting what the user wrote. The file explains itself in a header that is stripped before the prompt sees it, because a bare list of assertions reads as configuration to obey rather than as something that was concluded. Entries are matched by substring, never by line number — the agent has the text in its prompt, and a coordinate goes stale the moment anything above it moves. There is a byte cap, and hitting it is an error the agent is told about rather than a silent drop: consolidating two facts into one is work it can do, and a write that appears to succeed and did not is the worse outcome.

**Approved memory reaches the model at the next session, not this one.** Written to disk immediately, folded in at bootstrap. The lag is the feature: `internal/prompt` is built once per session precisely so the provider's prefix cache survives, and a system prompt that mutated mid-conversation would spend more on cache misses than the memory saves.

**`pending_changes` is the door, and its rows are never deleted.** Everything an agent proposes to learn lands there and does nothing until a human approves it. Approving sets a state and a timestamp rather than removing the row, so for any line in a memory file there is a record of when it was proposed, what reasoning was offered, and what it replaced. That record is the whole difference between an agent that shows its work and one that quietly rewrites itself. Rejections are kept for the same reason and one more: a later pass needs to know what has already been turned down. An apply that fails leaves the proposal pending — a row marked approved whose change never landed would be a lie in the one place built to be trusted.

**A queue nobody is told about is one that never gets emptied.** The door only means anything if the user knows there is something at it, and the review page is four clicks deep. So `learning:changed` marks the gear in the sidebar footer with a dot — not a count, because what matters there is that there is anything at all — and the count itself sits on the settings row that opens the list. It is fetched once at mount as well as pushed, since anything left undecided in an earlier session is still undecided and nothing would emit for it. Deliberately not a chip in the conversation: `suggest_task` chips are work the user might want done now, and mixing "start this job" with "allow this belief" would make both read as noise.

**Scope is what stops this becoming a single growing brain.** Every job row, every proposal and every memory file carries whose it is. `internal/prompt` reads only the main scope; a delegate's memory is folded into that delegate's prompt by `internal/subagent` and nowhere else. The enforcement is structural rather than instructed: `FilterRegistry` drops the parent's `memory` tool, and `task` constructs a replacement bound to the profile's own name, so there is no scope a child could name but its own. A profile can still refuse memory in its frontmatter, which is why the drop lives in the filter and not in `forcedDenials` — that list answers first and would take the choice away.

This is §44's capability boundary applied to knowledge, and it is the axis Aetox cannot trade. A single agent that accumulates everything it has ever learned pays for all of it on every request, forever; the main agent's context here stays flat no matter how much its delegates work out. An agent that has learned nothing produces byte-for-byte the prompt it produced before this existed — the common case does not pay for the feature.

**`skill_view` gained L2 in the same change, because it is a precondition rather than a nicety.** L0/L1 (§71) let the model list skills and open one. A skill written from real completed work splits into files precisely because it outgrew one, so without a way to open `references/…` the skills this floor exists to eventually produce would be unreachable past their first page. L1 now lists what else the skill carries — the model cannot open a file nobody told it about — and the path is judged by where it lands, not how it was spelled.

**One line is not negotiable and belongs here rather than in a comment:** the model never edits `internal/prompt/prompt.go`. The identity is compiled into the binary. What an agent may propose changes to is markdown — the user's identity files, its own memory, a sub-agent profile — which is what keeps "the agent improves its own prompt" from meaning "the agent edits who it is."

**It shipped without a bench, and that was the actual defect.** §45 settles two things: a whole-path test runs on Aetox's own model, and that model doubles as the surface a person checks by hand. The first pass of this work satisfied neither — every test built its rows by hand and called `recordJobs`/`proposeLearned` directly, so all of them would have passed with `memory` never registered and `recordJobs` never called from a turn at all. `aetox-tools:test` now answers a brief containing `memory` with one round that proposes something, and the desktop tests drive a real `SendMessage` through the executor and assert on what the store holds afterwards. The delegate half is the same keyword passed through a delegation, because "a delegate's proposal is filed under the delegate" is the claim the flat context rests on and it looks identical from the outside when it is wrong. It was wrong once, in the bench itself — the parent took the memory branch before delegating — and the ordering that fixes it is now the thing the comment explains.

**A keyword rather than a seventh model.** `aetox-render:test` also replaced `aetox-image:test` and `aetox-markdown:test`, which were one renderer benched twice; both names still resolve, for the reason the provider catalog keeps its aliases. The picker is a bench, and a bench nobody reads to the bottom of is not one.

**And the bench immediately earned its keep.** The first hand-check on `aetox-render:test` came back as a bubble holding `12` — one token out of the middle of a table. `TurnOptions.OnContent` documents the contract in as many words: *a preview is not a delivery; the reply arrives exactly once, through `onChunk`* — and the conversation path had been handing `onChunk` the token stream. The desktop's handler replaces the bubble with whatever it is given, because on every other path that is the finished answer, so each word became the whole answer for 40ms and the last one stayed. The path is reached only by a model that cannot call tools, so no real provider had ever taken it: **this was invisible except through Aetox's own models, which is the case §45 says they exist for.** Streaming now goes to the preview channel and `onChunk` delivers once, matching the tool loop. Two tests pin it — one per path, asserting a single delivery that equals the returned reply and still has its line structure. Reverting the fix turns "exactly one delivery" into 108.

**A precondition nobody had hit before:** no desktop test had ever driven a live turn, because five emitters on the turn path called `wailsruntime.EventsEmit` directly. That call is `log.Fatal` on any context but Wails' own — a hard exit, not a failure a test can report — so a test that tried died without saying why. They now go through `emitEvent`, the seam that already existed for exactly this and that three other emitters were already using; it holds the nil-context guard each of them used to carry separately. The whole learning path is testable because of that one-line door, and so is every turn-level thing anyone wants to test next.

**What was deliberately not built.** No curator: nothing yet reads `jobs` looking for repeats worth turning into a skill or failures worth turning into a rule. No prompt versioning or scoring of sub-agent profiles. No auto-approval, at any confidence. Those are the layers this floor is for, and each of them is only honest once there is a score to point at.

## 83. Decision — Three Modes: One Assistant, Three Desks (2026-08-05)

Owner: *"แนวคิดผมคือ จะแยกเลย โหมดผู้ช่วย … โค้ดดิ้งแน่นอนว่าฐานเราแน่นสุด … โหมด 3 คือ โหมดงานเฉพาะทาง"* — and the sentence that names why: *"ข้อดีของการแยกโหมดคือ tool ไม่ปนกัน จนคอนแท็กบวม … ถ้าพรอมป์สั่งเอไอ ก็จะมีประสิทธิภาพมากขึ้นเพราะเรารู้ทิศทางอยู่แล้ว"*. The settling line for the whole design: **ติดตั้งหนึ่งที่ · เรียนรู้ต่อโหมด · ทำงานต่อ session** — installed once, learned per mode, worked per session.

**The pile is the problem, and grouping only made it readable.** Every session today gets the identical 41 tools — a measured ~8,800 tokens of definitions on every request — whether the work is a slide deck, a refactor, or a reminder. [internal/skill/category.go](../internal/skill/category.go) grouped the pile by capability so a person could read it; a mode is the next step, where a *session* carries only the groups its work needs. This is not the "tool door" that was considered and rejected when the 8,800 tokens were measured: that design had the model discover tools mid-turn, hiding capability from the user. Here the user picks the mode, visibly, before the session exists — §19's rule in one more place: the list is the permission, and what the agent can do is always something the user chose from a list they can see.

**This is the door §44.0 left open, walked through — not walked back.** §44.0 records the owner building three selectable *agents* (personal assistant, coding, planning), looking at them in the app, and cutting them: an agent picker answers "who are you talking to", and a second answer to that question fragments the one assistant into personalities. That reasoning stands untouched. But the same section kept a future on purpose: if the capability returns, *"it returns as a **mode** — the same assistant with its hands tied … Not an agent to pick."* A mode answers "what is on the desk", never "who is sitting at it": one assistant, one identity layer (§11), whatever the desk. Three desks ship: **ผู้ช่วย** (assistant — long-term memory, documents, web, media; no code tools), **โค้ดดิ้ง** (coding — files, shell, code; no deck writers), **งานเฉพาะทาง** (specialized — the Office writers and every sense, read-only files; no shell). A mode bundles four things and nothing else: which tool categories are on the desk, which MCP servers are attached, what direction the base prompt adds, and which memory scope is folded in. The moment a mode manifest grows a role prompt that says who the assistant *is*, it has become the thing §44.0 deleted, and the answer is no.

**Three stores, three lifetimes.** Everything installable has exactly one home — the compiled registry, `mcp-servers.json`, `skills/`, `prompts/` — and a mode is a *view* over those homes, never a second copy. Splitting the install stores per mode was considered and dropped: it is "I installed the server, why can't the other mode see it", plus two config files that can disagree. What *does* split per mode is knowledge (below), and what splits per session is work — history, tool runs, attachments, `output/<session>` — which §19 and §67 already separated. The reason session is the wrong boundary for knowledge is the assistant mode's own definition: memory that dies with the session is not "จำระยะยาว".

**A mode manifest is a markdown file in the §35 two-source shape.** `internal/mode` reads bundled `modes/*.md` (compiled in, so a fresh install has three modes with nothing to download) shadowed by `<DataRoot>/modes/*.md` (so editing a shipped mode is copying it out, and a fourth mode is a file, not a feature request). The frontmatter dialect is the sub-agent profile's: `categories:` names groups from category.go, `tools:`/`deny:` handle the exceptions categories cannot express (specialized mode gets `read` without the rest of the files group), `mcp:` names attached servers, and the body is the direction prompt — the thing the owner's second sentence asked for, possible only because the mode knows the work's direction before the first message.

**MCP attachment is the one default-closed field, and the category fallback must never decide it.** `CategoryOf` answers `agent` for any name it does not know — deliberately, so every tool lands in some group on the Settings page. If that fallback also decided mode membership, every mode carrying `agent` (all three) would carry every MCP server ever installed, and the 8,800-token problem returns through the side door. So an MCP tool is judged by its *server's* presence in `mcp:`, decided by the host from `Registry.SourceOf`, and an absent `mcp:` field means **no servers** — unlike `categories:`/`tools:`, where absent means everything, matching the sub-agent precedent. The asymmetry is the point: built-ins are the product and are safety-gated individually; MCP servers are user-added per purpose, arrive in bulk, and are exactly what a mode exists to keep off the other desks. Server processes stay shared (one connection however many sessions), only *visibility* is per-mode.

**Skills stay shared, and nothing needed to change.** A SKILL.md costs nothing until the model calls `skills_list` — it is knowledge, not capability. Filtering skills per mode would be machinery without a measured problem; the filter point (`skills_list`) exists if one ever appears.

**Memory gains a mode scope beside the ones §82 built.** `memory/MEMORY.md` remains the cross-mode truths (who the user is, what this machine is like); `memory/modes/<mode>.md` is what working in that mode taught the agent, folded only into that mode's prompt, with its own byte cap. The per-scope machinery — write through `pending_changes`, read at bootstrap, cap-and-refuse — exists already; this is one more scope value, not a new mechanism. What coding mode learns about this repo never costs the assistant desk a token, which is §44's boundary applied along a second axis.

**A session is born in a mode and stays in it.** `sessions.mode` (schema v8) is written at creation and never updated; `''` is every session that predates modes and means the full desk — an existing install upgrades to identical behaviour. No mid-session switch: the entire value of a mode is that the context *never contained* the other desks' tools, and a switch would un-decide that retroactively. Changing mode is opening a new session, which costs what it should cost: nothing.

**The dispatcher moves from boot to session-open.** `NewDispatcher` already snapshots the registry at construction — the seam was built for this without knowing it. One registry keeps being assembled at bootstrap exactly as today; what changes is that each session constructs its dispatcher through the mode's filter (categories/tools/deny for built-ins and workbench tools, server membership for MCP). Sub-agents live under the mode's ceiling: a profile's `tools:` intersects with the mode's set, because a delegate that can reach what its parent cannot makes the mode a façade.

**A mode is not a permission tier.** The safety gate, approval rules and §66's single permission list are untouched and identical in every mode. A mode changes what is *on the desk*; it grants nothing, and nothing about picking a narrower mode may ever be read as consenting to weaker gates.

**Order of work, and what is deliberately not built yet.** Sequence: schema v8 → `internal/mode` → per-session dispatcher + prompt direction → MCP visibility → memory mode scope → the UI (a mode picker on new-session and mode filters on the existing lists — thin by then, which is the test the backend was done). Not built until their own decisions: the assistant desk's scheduler (the `jobs` table is §82's learning floor, not a cron), the specialized desk's preview surfaces beyond what the workbench has, anything video, and per-mode skill filtering.

## 84. Decision — The Company: Five Buttons, One Face, a Star With One Center (2026-08-05)

Owner: *"โหมดเฉพาะทางเหมือนเป็นเอเจนที่นั่งเก้าอี้สำหรับทำงานนั้น … มองภาพใหญ่เป็นบริษัทนึงเลย … พนักงานหรือเอเจนจะเพิ่มขึ้นได้ แต่ผู้ช่วยหรือตัวแชทหลักคือตัวหลัก"* — and, on sequencing: *"หน้าออโต้ ทำโครงพร้อม แต่มองข้ามไปก่อน อย่าจับปลาหลายมือ เดะจะตายก่อน"*. **The canonical product picture now lives at [COMPANY.md](../COMPANY.md), at the root, in the owner's language** — this section records what changed against §83 and why, for the engineering log; when the two disagree, COMPANY.md is the product and this file is the implementation.

**The specialized desk became an office, and its manifest became a ceiling.** §83 shipped `specialized` as one desk. The owner's company picture splits it into a *room* and its *staff*: a chair is one sub-agent profile (the machinery §44 already runs), hiring is dropping one more `.md`, and every chair's tools intersect with the specialized desk's manifest — so a chair that writes `tools: shell` into itself simply does not get shell. Growth is structural, not disciplined. The UI surface for this button is a roster plus a received-work feed read from `jobs` (`agent` + `parent_ref`, §82) — no new state, no inbox.

**Navigation is five buttons, and they are not five modes.** Two desks (ผู้ช่วย — machine-scoped, no project; โค้ด — project-bound, the current full workbench), one office (above), two pages (ออโต้ — deferred; ผลงาน — a view over `output/<session>` where the disk is the truth and there is deliberately no index table to desync). Session history filters by desk via `sessions.mode` (v8) — the column earns its keep before the picker even exists.

**Dispatch is a star with one center, and the ceiling rule gains its one carve-out.** ผู้ช่วย and ออโต้ may hand work to the office; the three left-side surfaces never talk to each other; the office calls no one. The principle that makes the topology non-arbitrary: work may cross desks only when it compresses to a single brief and returns as a file — chat, code and schedules are *contexts*, and shipping work between contexts means shipping the context, which is the bloat modes exist to kill. This adds the one exception to §83's "sub-agents live under the mode's ceiling": a cross-desk dispatch runs on the *target* desk's manifest, in its own context, and only the result crosses back. The ceiling exists to stop tools leaking into the caller's context; a file handed over the counter is not a leak. The safety gate stays identical everywhere — §83's "a mode is not a permission tier" is unamended and unamendable.

**Multi-step work is a pipeline through the center, and it already exists.** The owner's target picture — an accounting chair's numbers becoming a document chair's report — is two `task` calls in one main-agent turn, the second brief composed from the first result. **The baton is a file path, not the content**, so the main's context never carries the intermediate material; and the composer of each brief is the only agent that knows the user's actual intent, which is the argument that makes center-mediation better than a chair-to-chair edge rather than merely safer (a leaf that briefs another leaf is writing briefs for a goal it was never told). Chair-to-chair calling therefore stays absent — it is not a deferred feature, it is the thing the star forbids — and `forcedDenials` already enforces it. A pipeline used on a schedule is what the deferred ออโต้ page will store as a recipe: same conveyor, same door, only the starter changes from the user to a clock.

**ออโต้ is deferred, and deferring it costs nothing by construction.** The only structure it needs — the dispatch door — is the same one ผู้ช่วย uses from day one, so the scheduler arrives later as a new caller of an existing door, not a rework. Its real open question (what approval means when nobody is watching — pre-granted rights only, skip-and-report, never prompt into the void) gets its own decision when it is actually being built. The native-vs-Windmill question defers with it; the positioning bias (33 MB, no Docker) is on record in COMPANY.md's sequencing note.

**What §84 does not reopen:** no manifest, mode or chair, ever says who the assistant *is* (§44.0); no mid-session mode switch; MCP stays default-closed per desk. One face, however many chairs.

### 84.1 — Built (2026-08-05): the back half, and the seams it needed

The engine now runs at a desk. Six things changed, and each one is the *existing* mechanism given one more input rather than a new mechanism beside it.

**The dispatcher took a filter, and the registry stayed whole.** `skill.NewDispatcherFor(registry, ToolFilter)` is the seam; `mode.Carries` is what the desktop passes it. Filtering at read time rather than copying the registry is what keeps §71's live registration working — an MCP server that connects mid-session still appears, on the desks that named it. The alternative (a filtered copy per session, the shape `subagent.FilterRegistry` uses) would have frozen each session's tool list at the moment it opened, and the bug would have looked like "my server only works if I start a new chat".

**Desk membership is decided in one function, because the MCP trap is real.** `Carries` branches on `skill.Source`: MCP is judged by its server (read off the tool name's own prefix, `mcp.ToolPrefix`), a discovered skill is always kept (knowledge, not capability), everything else goes through `AllowsTool`. §83 predicted the failure and this is where it is prevented — `CategoryOf` answers `agent` for unknown names, so one forgetful branch would attach every installed server to all three desks.

**Changing desks re-bootstraps the engine.** Everything a desk decides — the dispatcher's filter, the prompt's direction and memory scope, the ceiling over sub-agents — is built once from one value, so `App.desk` is that value and `switchDesk` rebuilds around it, exactly as a project switch already did. The considered alternative was hot-swapping the pieces in place, which needs a mutable "current desk" that a running delegate could read halfway through. A session is born at a desk and never moves; making the engine work the same way costs one rebuild on a switch the user makes deliberately.

**`dispatch:` is a manifest field, so the star's topology is data.** ผู้ช่วย declares `specialized`; nothing else declares anything. `task` reads it twice: chairs a desk cannot reach are not listed in its tool schema, and naming one anyway is refused with a sentence the model can act on. When ออโต้ arrives it adds the same line to its own manifest and walks through the door that already exists — which is what "deferring it costs nothing by construction" meant.

**The ceiling is an intersection, computed per job.** `FilterRegistry(parent, profile, ceiling)`: the caller's desk for an ordinary delegate, the chair's own desk for a cross-desk dispatch. Three chairs ship (`deck`, `doc`, `sheet`), each one file, and a chair that writes `tools: shell` into itself is listed without one — the office roster shows the tools a chair *gets*, not the tools it asked for, because the ceiling has to be visible exactly where someone looks to check it.

**Memory gained its third namespace and the tool gained one optional word.** `memory/modes/<desk>.md` beside `MEMORY.md` and `agents/<name>.md`, through the same `pending_changes` door. A session at a desk can write to either scope — `where: this-desk` — and the parameter exists only when there is a desk, so every other session sends the tool block it always sent.

**The line that was not crossed:** nothing about a narrower desk changes the safety gate, the approval rules or the permission list. A desk is what is on it, never what is allowed.

**Pinned by tests rather than by intent.** [desktop/desk_test.go](../desktop/desk_test.go) runs all of it through a real engine: a legacy session's tool set and its prompt *byte for byte*; each desk's actual tool block; a chair capped by the office ceiling; a server reaching only the desk that named it; the coding desk refused a dispatch; and an assistant-desk session handing a job to a chair, getting a .docx back in its own output folder, with the row recorded under the chair. The last one runs on `aetox-tools:test` (§45), so the same path can be checked by hand with no API key.

### 84.2 — The five buttons arrive, and the surface stays additive (2026-08-05)

The nav is a row at the top of the existing sidebar and the two new pages are `.settings-overlay` views — the seam Settings has used since it was built. Nothing in the chat layout moved: the current screen *was* the coding desk already, and what it got is a name.

**The nav list is fixed, not a rendering of the modes folder.** A user who writes a fourth manifest gets a fourth desk in the engine — `NewSessionAt` takes any name — and no fourth button. A mode file is a capability; the five rooms are the product's shape, and the owner draws that. `ListModes` is read for one thing: the blurb on each desk's button, so editing a shipped mode changes what the button says about itself and nothing else has to be told.

**Re-clicking the desk you are already at does nothing to the conversation.** It is the difference between a nav bar and a trapdoor: changing desks opens a session (which costs nothing — every turn is already persisted before the click), but standing still must not. `openDesk` is that one branch.

**History filters to the desk you are at, and the way back to everything is one click.** `SessionMeta` gained `mode` on every list, including the cross-project ones, so the sidebar labels each row and filters without a second query. Sessions from before desks hold `''`, belong to no desk, and appear only in the combined list — which is why that list has to stay reachable rather than being replaced by the filter.

**ผลงาน owns deletion, and only it.** `DeleteArtifact` is path-checked against the folders the gallery actually swept, `OpenArtifact` is its read-only twin (`OpenFileExternally` could not serve: it takes a path relative to the *open project's* root, and an artifact is absolute and routinely belongs to another project). The card's delete arms on the first click and fires on the second, the same gesture the session list uses.

**What is still open:** ออโต้'s scheduler and its unattended-approval policy, the ประตูส่งไม้ door (assistant → code, one-way, COMPANY.md §3 marks it unbuilt), and per-desk skill filtering — which still has no measured problem to solve.

**Amended the same day, after the owner's second pass (all recorded in COMPANY.md, the copy that counts):** (1) The assistant desk widened to its honest definition — *everything on the machine except the developer's tools*: `categories: agent, web, media, deliverables, files, shell`. The owner's target user includes someone touching a computer for the first time, and an assistant that can see a broken setup but not touch it contradicts the product's one-line positioning; safety lives in the approval gate, not in missing tools — which makes plain-language approval prompts a first-class product surface, noted as future UX work. (2) Deleting a session never deletes its `output/<session>` files — the ผลงาน page owns artifact deletion, and a pinning test guards the current (already correct) behaviour. (3) The office button is named **ทีมเอเจน** — UI label only; the desk stays `specialized` in code and data. (4) The star gains a second door, different in kind from the first: **จ้างงาน** (dispatch to the office: brief in, wait, file back) and **ส่งไม้** (assistant → code: compose the plan from the conversation, open a code session seeded with it, let go — one-way, no return path, the user supervises and approves at the code desk where the diff and terminal live). Live relay between desks stays forbidden; a future "your delegated job finished" notification can close the loop without ever opening a conversation back. The three left-side surfaces still hold no conversations with each other — the sentence changed from "never talk" to "never *converse*; a baton may cross, words may not."

---


## 85. Decision — Chairs Are Full Agents: Direct Chat in the Office (2026-08-05)

Owner: *"ในทีมเอเจนอ่ะครับ ควรจะเข้าไปแชทกับมันโดยตรงได้ เหมือนแชทกับตัวหลักอ่ะ แต่อันนั้นผูกกับของเฉพาะทาง … มันคือการถอดซับเอเจนออกมาเป็นเอเจนเต็มตัวนั่นแหละ ชื่อซับเอเจนในระบบตอนแรกน่าจะต้องถอดออกละ"* — the office's chairs stop being reachable only through `task`: a user can walk into the office and talk to one directly.

**The word "sub-agent" retires from every user-facing surface.** A chair is an agent, full stop — it has a job description, its own memory, its own record of work, and now its own conversations. "Sub-agent" survives only as code vocabulary (`internal/subagent`, the `task` internals), the same split as desk ids: the user's word and the schema's word are allowed to differ, and renaming a Go package mid-flight buys nothing that a locale string doesn't.

**This does not reopen §44.0, and the difference is worth stating precisely.** What §44.0 deleted was a picker that changed who the *main chat* is — one door, many personalities. A chair chat is a *different room*: the user leaves บ้าน, walks into the office, and talks to a named employee about that employee's specialty. บ้าน remains the company's one face; nothing in it gained a selector. The company metaphor carries the whole distinction: talking to the deck specialist at their desk is normal office life, the receptionist becoming a different person mid-sentence is not.

**A session gains its second coordinate: `sessions.agent` (schema v9), `''` default.** A chair session is `mode='specialized'` + `agent='<chair>'`. Everything else reuses the delegate machinery unchanged: the engine mounts `FilterRegistry(registry, profile, office ceiling)` — so a chair that asks for `shell` still does not get it in person — plus the chair-scoped `memory` tool, and the prompt direction is `PromptFor(profile)`: the chair's own prompt plus what it has learned doing this job (§82's fold, now serving conversations too). The safety gate is identical; approvals land in the same UI because this is the ordinary interactive loop, just behind a narrower desk.

**What a chair still cannot do, even face to face:** call `task` (a leaf stays a leaf, even when spoken to), reach the main's memory scope, or see any conversation but its own. `ask_user`/`todo_write` stay off too — they are forced denials shared with the delegate path, and carving an interactive exception would fork one invariant into two; deliberately deferred rather than quietly weakened. Direct-chat turns write `jobs` rows with `agent=<chair>` and no `parent_ref` — the roster's activity counts them, the received-work feed (which filters on `parent_ref`) correctly does not.

**Reopening a chair session whose profile file is gone refuses**, exactly as a desk whose manifest is gone refuses (§83): the transcript is intact, but reproducing what that session could do without the file would mean widening it by guesswork.

**The office page becomes a gallery** — agent cards (name, description, real tool list after the ceiling, activity), each opening a direct chat; creating an agent is writing one more profile file, which the gallery is the front door to. The reference the owner pointed at is the agent-gallery pattern; ours differs in one load-bearing way: the cards are not templates to instantiate, they are the actual staff, with history.

## 86. Decision — One Company, Two Doors: Aetox ผู้ช่วย and Aetox โค้ด (2026-08-05)

Owner: *"เราจะแยก Aetox โค้ด และ Aetox ผู้ช่วย … เพราะยังไงงานโค้ดก็ต้องมีทีมเอเจนแยกอยู่ละ การทำแบบนี้น่าจะดีในระยะยาว"* — and the visibility rule stated as the settled fact it now is: *"โค้ดมองไม่เห็นทีมเอเจนและผลงานแล้ว เพราะแยกส่วนกันแล้ว"*. The reference pattern is ChatGPT ↔ Codex: one account, two products, a switcher on the wordmark.

**This is the UI catching up with the star, not a pivot.** §84 isolated โค้ด by rule — it dispatches to no one, no one dispatches to it, and the only bridge is the one-way plan handoff. §83's `sessions.mode` already separates the data; the office machinery is per-desk by construction (`subagent.Chairs(desk)`). A product this separated in its rules, rendered inside one shared shell, produced exactly the misfit the owner kept pointing at: office buttons on a coding screen, an agent switcher on a desk the star forbids from reaching anyone. The five rooms stay; they get sorted behind two doors.

**The doors.** **Aetox ผู้ช่วย** (the storefront): บ้าน, ทีมเอเจน, ออโต้ (deferred), ผลงาน — the four rooms that live around the office and its files. **Aetox โค้ด** (the workshop): projects, sessions at the coding desk, the full workbench/terminal/files surface — today's whole IDE screen, unmoved; later its own team page, which costs one call to the mechanism that already exists. ผลงาน and ออโต้ sit behind the assistant door because both are wired to the office (deliverable files; routines that hire it), and the star never wired either to โค้ด — the sorting is a *reading* of the topology, not a new decision.

**One binary, one engine, one store — the switch is a shell swap, never a second app.** DataRoot, providers, identity, memory, permissions: one of each, exactly as §83's "installed once" layer demands. The switcher lives on the wordmark, remembers the last door used, and swapping costs what changing `activeView` costs. Anything that forks the engine or the data to make the split "cleaner" has misread this section.

**"One face" refines rather than breaks: one face *per door*.** §44.0's rule was never "one window"; it was that the person you talk to is not selected from a list of personalities. That holds unchanged inside each door — in ผู้ช่วย you talk to the assistant (and may walk into an office room, §85); in โค้ด you talk to the code agent. What the company gains is what any company with a storefront and a workshop has: two entrances, each honest about what is behind it — and two audiences (first-time computer users; developers) that no longer have to see each other's furniture.

**Visibility rules, stated once:** the code shell renders no ทีมเอเจน, no ผลงาน, no ออโต้, and no agent switcher (that chip was removed from the coding desk in this decision's first landed slice); the assistant shell renders no workbench, no terminal, no project tree. Sessions filter by desk through the columns v8/v9 already carry — the split needs no migration at all, which is the measure of how long the backend has been waiting for it.

**Deliberately not built yet:** the code door's own team page (the mechanism is ready — `Chairs(desk)` has been per-desk since it was written — but no profile declares `desk: coding`, and a room is not worth opening before anyone can sit in it), and any cross-shell navigation beyond ประตูส่งไม้ — when the assistant hands a plan to โค้ด (§84), the user walks through the front of the workshop like everyone else.

**The workshop's nav stays one room, on purpose.** Asked whether to fill it out like the reference product's row (New chat · Pull requests · Scheduled · Plugins), the owner's answer was to wait — *"ผมไม่อยากด่วนเพิ่มจนของไม่มีประสิทธิภาพ … เอาหน้าผู้ช่วยปกติก่อน"* — and each of those four fails a different test anyway, worth writing down so the question is not re-opened as though it were untried. *New chat* is already the column's first button and Ctrl+N; a second door to it is two controls for one act. *Scheduled* **is** ออโต้, which hires the office and therefore sits behind the storefront by the star rather than by preference. *Plugins* already has one home in Settings (MCP + skills), and a second entrance to one thing is exactly the drift §83's single-install layer exists to prevent. *Pull requests* is the only one that names a real absence rather than a duplicate — the `github_*` tools read, they do not review — which makes it a feature with its own decision to earn, never a button added because a competitor has one.

## 87. Decision — Files Travel as Addresses, and the Desk's Menu Does Not Grow (2026-08-06)

Owner, asking the question this section answers: *"เช็คทีครับ ตอนนี้มันมีไฟล์แบบไหนบ้างที่ระบบเราแสดงไม่ได้ … ผมอยากให้มันแสดงไฟล์ได้ทุกไฟล์"* — and, once the shape was clear, the rule that outlives the work: *"ถ้าหากอันไหนมันต้องสร้างเพิ่ม ต้องเพิ่มจากตรงนี้ก่อนด้วย เพราะมันคือโต๊ะทำงาน"*. The full routing table lives in [docs/architecture/desk-file-panes-2026-08-06.md](architecture/desk-file-panes-2026-08-06.md); this section records what was decided, not what the rows say.

**The audit found one cause behind four symptoms.** PDFs, video, audio and SVG all ended at the same "cannot preview" card, and it was tempting to read that as four missing features. It was one missing transport: every file the desk could show had to cross the Wails binding as a JavaScript value — text as a string, a workbook as JSON, a picture as a base64 data: URL. That works while a file is small and stops working exactly when it is not, which is why the image pane needed a 20 MB ceiling and why a video could never appear at all. A `ReadVideoDataURL` next to `ReadImageDataURL` would have been the fourth wrong answer to the same question.

**So the decision is a URL, not a reader.** [`filehost.go`](../desktop/filehost.go) is Wails middleware claiming `/aetox-file/`, resolving through the same `safeSandboxPath` every binding uses, and answering with `http.ServeContent` — which brings Range support, and therefore scrubbing a video without downloading it, out of the standard library rather than out of hand-written code. Bytes stream from an `*os.File` and never enter the webview whole. **This adds a transport, not a right**: nothing outside the project root is reachable, and the tests that matter assert exactly that (`../secret.txt`, `%252e%252e`, and — the one that would take the whole window down — that any path *not* under the prefix is passed through untouched).

**A ceiling now belongs only where bytes must enter JavaScript whole.** Image, video, audio and PDF have none, and the image pane's 20 MB limit was deleted rather than raised: it was never protecting the file, only the data: URL that no longer exists. `.xlsx` keeps a byte cap because its preview is marshalled to JSON, though the row cap is the real guard. The text path's 1 MB refusal survives this section unchanged and is named in the reference doc as the wrong *shape* rather than the wrong number — the house already knows the right answer (`ReadXLSX` cuts and reports `Truncated`; the agent's own `read` returns an offset to continue from), and the desk is the last place still saying "no" where everything else says "here is some of it". Fixing that requires a read-only editor in the same change, because `WriteFile` replaces the whole file and saving a truncated buffer would destroy what was never shown.

**The `+` menu did not grow, and the rule for when it may is now written down.** Its four entries — เทอร์มินัล, เบราว์เซอร์, ไฟล์, เครื่องมือ — are *sources that start from empty*, not file types; not one of them opens a file, and file tabs are born at three other doors (tree row, drop, chat card). A "PDF" entry would have had to answer its own click by asking which PDF, which is what ไฟล์ already does. What earns a fifth entry is a surface that begins with nothing chosen; a file type earns a row in the routing table and nothing else. The whole of this work touched no tab kind, no icon map and no menu — only the `if`-chain inside a file tab, which is the measure of whether a change of this kind is aimed correctly.

**What is still the card, and why that is not a defeat.** `.docx`/`.pptx` go there without a read attempt so the reason shown is the real one rather than whichever gate fired first (§79's lesson, kept). `.doc`/`.xls`/`.ppt` remain unread — with one correction to an earlier claim that lumped them together: `.xls` is BIFF8 and genuinely reachable, and would land in the existing `SheetPane` unchanged, while `.doc` and `.ppt` address their text through a piece table and a record tree with no usable Go reader. If Office formats are ever answered wholesale, the route is a converter already on the user's machine (`soffice --headless --convert-to pdf`) landing in the PDF pane this section built — one mechanism for all six, and consistent with the promise that the work happens on the owner's machine. Driving an installed Microsoft Office through COM is the tempting version of that and is rejected: Microsoft does not support unattended automation, and a hung invisible Word is a worse outcome than a card.

**The rule that outlives the code.** The desk is where the user finds out what the agent actually produced. It is small — four entries, four routing questions — which is precisely why an addition changes its shape and why the shape can still be checked in one reading. So anything new that will appear on it is written into the reference doc *before* it is built: which door it comes from, which pane draws it, what it cannot do. A row with no code behind it is a plan and should say so; code with no row is the error. The reverse order is how a fifth menu entry, a second router, or a second reason for the same refusal gets in — each defensible alone, and wrong against the whole.

## 88. Decision — A Drawing Is Confined to Its Own Box (2026-08-06)

Owner, on a chart that came back with a donut and then a screenful of nothing: *"ผลลัพท์อันนี้ปกติดีนะครับ"*, then *"เช็คดีๆนะครับคิดเผื่อด้วย … เพราะ เอไอมันสร้างออกมาได้ตลอดเวลา"*. The last clause is the whole reason this section exists: a drawing is not a feature that ships once, it is output the model produces again tomorrow, so the question is never "why did *this* one break" but "what will it do next time".

**The blank space was not a bug in the drawing, it was a hole the renderer cut in it.** The legend was laid out in `<foreignObject>` — the obvious way to put wrapped text in a picture — and DOMPurify has that element in both `svgDisallowed` and `FORBID_CONTENTS`, so the element *and its subtree* are removed. The `viewBox` still reserved the space, so what reached the screen was a correct drawing with its bottom half deleted and no error anywhere. Probing the rest of the surface the same way found the same silence in four more places: `<use>` (the shape it points at is simply not drawn), `<animate>` (removed, while `animateTransform` survives), a `<style>` inside an inline `<svg>` (**not** scoped — it is a document stylesheet, so `.row{display:none}` aimed at a legend hides the sidebar), and ids (`url(#g)` resolves to the first match *in the page*, and `g` is what a model names a gradient — so the second drawing in a conversation wore the first one's colours). Markdown owns the text around all of it: a blank line inside a drawing ends the HTML block, and the four-space indent anyone pretty-prints XML with turns the rest of the shapes into a code block beside the picture.

**The split: what the model can be told, and what it cannot.** It can be told what the surface is — [`drawing()`](../internal/prompt/prompt.go) now says *sanitizer, not a browser*, names the three tags that vanish, and asks for one block at the left margin with no blank line in it. It cannot be told to stop colliding with ids it will never see, or to scope a stylesheet whose reach it has no way to observe, so those two are repaired in [`markdown.ts`](../desktop/frontend/src/lib/markdown.ts): every drawing that carries a `<style>` or an `id` gets a fingerprint of itself, its selectors are prefixed with `[data-drawing="…"]`, its ids are namespaced, and `@import` — a stylesheet fetching from the network on the strength of model output — is dropped with the other non-nesting at-rules. A drawing with neither, which is most of them, comes out byte-for-byte as it went in.

**And markdown no longer gets to read a picture as prose.** A complete `<svg>` in the left margin is lifted out before `marked` sees it and put back after — the same treatment a fenced block gets, for the same reason. Indented ones are left alone on purpose: they belong to the list item they are under, and lifting them would land the picture below the list. What is *not* fixed, and is written here so it is a decision rather than an oversight: a remote `<image href="https://…">` still loads, which is a working beacon out of a machine whose whole premise is that the work stays on it — closing it means deciding what a legitimate remote image in an answer is, and that is a policy, not a patch.

## 89. Decision — A Number Is Worked Out, Not Recalled (2026-08-07)

Owner: *"ผมว่าจะเพิ่มให้ aetox สามารถเลือกเขียนโค้ดเพื่อรันสคริปอะไรเองได้ กรณีที่ต้องคำนวณอะไรให้ผู้ใช้ ดีกว่าปล่อยโมเดลคิดสุ่ม"*.

**Why not `write` + `shell`, which could already do this.** Three reasons, and none of them is that it would not work. It depends on what the user happens to have installed, so the answer to "3.7% ของ 128,400" would rest on whether Python is on this machine. It spends the shell's right to touch the machine on a multiplication, which teaches the approval gate to mean less than it does. And it costs three round trips and a leftover file per sum. So the interpreter is compiled in: [goja](https://github.com/dop251/goja) is ECMAScript in pure Go, and `calc` ([calc.go](../internal/skill/calc.go)) runs a short script in a fresh runtime with a 5s clock and a 256MB heap watch. **The clock was set by measurement, not by taste**: the interpreter runs ~7M loop iterations a second, so the first draft's one second bought a million rows of arithmetic and refused everything past it — ten million additions, a sort of two hundred thousand numbers, a Monte Carlo of any size. A ceiling belongs where a real calculation stops, not where a small one does. **It asks for no permission, and the reason matters**: `require`, `process`, `fetch`, `fs` are not blocked in it — they were never there. A tool that cannot see a file, open a socket or start a process is not asking for a right, and putting it behind the same gate as shell would say the gate is decoration (§ the one-gate rule). The script the model ran is what the user reads in the timeline, which is the whole difference between a computed number and a remembered one: a wrong answer becomes a line of arithmetic somebody can point at.

**The tool alone would have shipped and never fired.** This is the second time in one day: `drawing()` exists because inline SVG had always rendered and nothing said so, so nothing ever drew. A tool cannot ask to be used. [`computing()`](../internal/prompt/prompt.go) is the layer that decides whether the capability happens at all. What makes it worth its own layer rather than a line: a wrong sum is the only failure in the whole tool block that never announces itself, arriving in the same confident sentence as a right one.

**Where the line sits, and why it is not "any arithmetic".** Owner: *"ถ้าเป็นงานง่ายๆสั้นๆ ผมว่ามันคิดเองก็จบ โมเดลสมัยนี้คิดคำนวณฉลาดมาก แต่ถ้าเป็นงานหนักๆยาวๆ ผมให้มันสร้างสคริปมาคิดแทน"*. The first draft of this layer sent every percentage through the tool, which is ceremony — a round trip to prove 20% of 500 — so short arithmetic is now explicitly the model's own. But "long" had to be written as something **countable**: digits carried, steps that feed the one after them, a repetition down a list, a figure someone will act on. Never as "when it is hard". Difficulty is the one thing a model cannot judge about its own arithmetic — 47 × 93 and 4.7 × 9.3 feel identical from the inside, and so does getting one of them wrong — so a threshold made of feelings selects for the calculations that already looked easy, which is exactly the set the silent mistakes live in.

**Two guards fired during the work, and both were right to.** The tool-block budget ([tool_budget_test.go](../desktop/tool_budget_test.go)) failed at ~9,561 tokens against a 9,500 ceiling — so the description was cut to capability alone first (occasions belong in the prompt, where the weighing is), and only then was the ceiling raised to 9,600 with the argument recorded next to it. The count is now full at 48: the next tool has to displace one. The second guard was the category table: `calc` runs a script, so `code` is where it reads like it belongs — and `code` is the developer group that the assistant and specialized desks refuse on purpose, which would have left the desk whose whole job is answering a person's question as the one desk that cannot count. It is filed under `agent`, beside `time` and `memory`, and [a test says every desk can work a number out](../internal/mode/mode_test.go).

**Two things learned by writing fifteen scripts against it rather than reasoning about it.** `console.log` is muscle memory — a bare goja has no console at all, so without an alias the first attempt at half the scripts ever written here would die on a ReferenceError and buy nothing but a house rule. And a script whose last line is `({total: x})` — the ordinary way to hand back an object — is not a new statement: the line above ends in `)`, JavaScript inserts no semicolon, and the parentheses become a call. It fails as "Cannot access a variable before initialization", pointing at a variable that is fine and never mentioning the line break. Two of those fifteen scripts hit it. So `return` is accepted (the script is re-run wrapped in a function when the engine calls the return illegal), and the shape is detected in the source and **explained in words rather than repaired silently** — a script rewritten behind the author's back is a script nobody can reason about.

**What is deliberately still `shell`.** Numbers that live in a file, more of them than anyone would type, or work that needs a real library. `calc` holds nothing between calls and reaches nothing; the moment a calculation needs the machine, it is the machine's gate that should be asked, not a second one built into a calculator.

## 90. Decision — A Project Is a Folder of Conversations, Not a Fence (2026-08-07)

Owner, pointing at the ห้อง that had been sitting there disabled since §84: *"เห็นตรงนี้ไหมครับ เราจะทำให้มันพร้อม สร้างโฟลเดอร์โปรเจคไว้ที่ aetox"*, and on where it lives: *"เก็บโฟลเดอร์จริงเลย แต่ตำแหน่ง aetox/project ครับ ข้างในก็แยกย่อยจะได้หาง่าย … ในนั้นจะมีโฟลเดอร์แยกเก็บบริบทในโปรเจคนั้นๆ"*.

**The room was already described, and the description was the specification.** §84 named it, drew the button, and said what it would be — chats grouped, files that ride into every session, *"เป็นโฟลเดอร์ของบทสนทนา ไม่ใช่รั้ว"*. Nothing about the shape was decided in this session; what was decided was where it lives (`<DataRoot>/project/<ชื่อ>/context/`, beside `modes/` and `agents/`, because everything the user is meant to open and back up by hand lives as files under DataRoot) and what carries into the prompt. That is what a room drawn early buys: the work arrives as construction, not as design.

**The disk is the only record that a project exists.** There is no table of projects and deliberately never will be — the folder is the half the user moves, renames and deletes without telling us, so an index would show projects that are gone and hide ones that are there. This is ผลงาน's rule (§84) applied a second time. A folder made in Explorer is therefore a project, tested as such. The database holds exactly one new thing, `sessions.space` (migration v10): which project a conversation was *held in*, which is a fact about the conversation and belongs on its row. Deleting a project folder deletes the project and leaves the chats — reopening one lands outside every project rather than refusing, because the folder held context to read, never rights to use.

**The word had to be two words in code, and that is written down rather than left to be discovered.** The workshop door already has a โปรเจกต์: a folder that the sandbox is rooted in, and rooting it is the entire point of that one. This one is its opposite — it moves no wall. Two meanings cannot share an identifier, so the storefront's is `space` in code (`desktop/spaces.go`, `sessions.space`) and the workshop's keeps `project_key`. Both are now rows in COMPANY.md §8's glossary, because an untranslated second word is exactly how the drift this project keeps refusing gets in. The test that guards the meaning is not about naming at all: `TestAProjectMovesNoWall` asserts that opening a chat inside a project leaves the sandbox exactly where it was. The day that fails, the storefront's project has become the workshop's wearing its name.

**Context files are named to the assistant, never pasted.** [`workingIn()`](../internal/prompt/prompt.go) tells it which project the conversation is in, where that project keeps its material, and the file names in it — then says to read the ones a question needs and not to read them all at the start "to be thorough". Naming costs a line and buys the only thing the assistant was missing, which is knowing they exist; `read` and `grep` were already in its hands. Pasting the contents would spend the whole context window on files most turns never open, on every turn, forever — the version of this feature that quietly makes the assistant worse at everything else. An empty context folder still says so, because "there is nothing in it yet" is a more useful fact than silence, and it is the sentence that tells the user's next file where to go.

**Two things this deliberately does not do yet, so the gap is a decision and not an omission.** Existing chats cannot be moved into a project — the column records where a conversation *was held*, and back-filling it would be inventing a fact about it (the same reasoning that refused to backfill `mode` in §83). And a project cannot be renamed or deleted from the page: the folder is the truth, so the file manager is where those happen, and a rename orphans that project's chats rather than following them. If renaming is wanted later it is a real decision — it means either a stable id (a second identity for something the disk already names) or rewriting rows, and neither should be reached for by accident.

## 91. Decision — An Agent Is a Colleague With a Job, Not a Tool With a Name (2026-08-08)

Owner: *"เอเจนในระบบผมคือ เหมือนผู้เชี่ยวชาญเฉพาะด้าน เราสามารถพูดคุยถามตอบอะไรกับมันได้ปกติ ไม่ใช่เครื่องมือแบบ ส่งเข้าแล้ว รอรับอย่างเดียวแต่ต้องคุยได้ ทำงานได้ แต่มันก็มีบทบาทของตัวเองชัดเจนเหมือนพนักงานคนนึง … ไม่งั้นผมจะสร้างหน้าแชทกับเอเจนโดยตรงมาทำไม"*

§85 built the door — a user can walk into the office and talk to an agent directly. This states what is on the other side of it, because the door was built and the definition was left implicit, and everything written afterwards drifted back toward the transaction.

**The standard, in one sentence: an Aetox agent is a specialist colleague with a job you could put on a business card — you can talk with them, ask them things, disagree with them, and also hand them work.** Not a function with a friendly name.

Three clauses, and each of them was being broken by the bundled three on the day this was written.

**1. The remit is a profession, not an output format.** `doc` was described as "รับ brief แล้วคืนไฟล์ .docx" — which is a description of `doc_write`, not of a person. The proof that this is backwards: extending `doc_write` with a computed-totals block on 2026-08-08 silently enlarged what `doc` was for, without anybody deciding that issuing tax invoices was its job. The remit has to come first and the tools serve it, or the agent's identity is whatever its tool happens to be able to emit this month. So: `doc` is documents of record, `sheet` is data that must compute, `deck` is presenting to a room — and which file comes out is a consequence.

**2. Conversation is a first-class arrival, not a degraded delegation.** Every bundled profile opened with *"The brief you were given is everything you know — there is no conversation behind it"*, and `PromptFor(profile)` is the same prompt the §85 chat runs. So an agent being spoken to face to face was being told, in its own instructions, that nobody was there — and told to reply in one or two lines because "every line you write is paid for by whoever is waiting". It answered questions with files, because that is what we asked it to do. Each profile now names both arrivals and what changes between them: in a conversation you ask, argue, explain and may produce nothing; over the counter the brief is all there is and the file is the whole reply.

**3. The reply is the asking.** `ask_user` stays a forced denial even face to face (§85), and that is correct — but it left no stated way for an agent to ask anything, which reads as "do not ask". In a direct chat the reply *is* the question: the user is right there. Written into each profile rather than into the engine, because it is a fact about the situation and not a capability to add.

**What this does not change.** A chair still cannot call `task`, still cannot reach the main assistant's memory, still sees only its own conversation, and still measures against the office ceiling. Being a colleague is about what an agent *is for*, not about widening what it may touch — and the two are separate on purpose, because "treat them as people" is exactly the argument that would otherwise be used to hand one a shell.

**Where this binds.** Every profile in `internal/subagent/profiles/agents/`, and every profile a user writes: the `description` names a person's job, and the body works in both arrivals. A profile that reads as a tool description is a bug in the profile, and the office page — cards, a chat door on each, and a job feed — is the shape of the product that this standard is the content of.

## 92. Decision — The Code Door Is a Focus, Not a Fence; and the Automation Room Borrows Somebody Else's Clock (2026-08-09)

Three answers given the same day, after a pass over the direction documents found each of them being answered differently in different places. Recorded together because they were decided together, not because they are one idea.

**1. The assistant does what the user asks, software included. The desks are separated in the system, not in the assistant's behaviour.** Owner: *"ผู้ช่วยทำได้ครับ ไม่จำเป็นต้องปฏิเสธ โหมดโค้ดแค่ทำมาเพื่อโฟกัสโปรเจกต์เท่านั้น"* and, when the first fix still left a redirect in place: *"ทำตามพรอมป์ที่ผู้ใช้สั่งเลยครับ ไม่ได้แยกขาดในมุมของพฤติกรรม AI แค่เราทำระบบให้แยกก็พอ … ให้มันเป็นผู้ช่วยในคอมพิวเตอร์ที่ทำอะไรก็ได้ในคอม … โค้ดก็โฟกัสสร้างระบบเต็มรูปแบบเฉย ๆ"*

`COMPANY.md` §2 has said since 5 August that the assistant's work includes *สร้างระบบ สร้างเว็บไซต์*, while `internal/mode/modes/assistant.md` told it, in its own instructions, that *"building or changing software is not [this desk's work]: say so and point at a coding session"*. Both were shipped, so the product's answer depended on which one you read — and only one of them the user ever meets.

The prompt was the wrong one. Missing tools were being read as a missing permission: the desk has no `diagnostics`, no language server and no repository browsing, and that was generalised into a refusal to write a page or a script — which the desk has the files and the shell to do, for a user who by definition cannot do it themselves. The rule this restates is the one already written at §6 clause 2 and never repealed: **a desk changes what is on it, never what is allowed.** A refusal derived from a tool list is that rule being broken from the inside.

The first attempt at fixing this replaced the refusal with a recommendation — *say a coding session is the better place for it* — and the owner rejected that too, which is the part worth recording. A recommendation to go elsewhere is still work handed back. The user in front of this desk may be someone who has never opened a terminal; telling them their request belongs at another desk is, from where they are sitting, the same event as being told no. **The prompt now carries no redirect at all**: the tools it lacks are named as tools, and the instruction is to do the work with what is there and say plainly when something was worked around.

Where the separation lives instead is the system: two doors, two tool manifests, a project that is a fence on one side and a folder of conversations on the other. The coding desk is a place to build a full system with everything a developer needs open at once — its value is focus, and focus is a service to whoever chooses it, not a rule imposed on whoever doesn't. `COMPANY.md` §2, `docs/DOOR-ASSISTANT.md` §1 and `docs/DOOR-CODE.md` §0 now say this in the same words.

**2. Rooms marked `kind: 'soon'` stay drawn in 1.0.** `ROADMAP.md` carried *"a button that does nothing in 1.0 reads as unfinished — hide it"* against `COMPANY.md` §2's *"empty rooms are drawn on purpose … a button that appears later reads as a new feature; a button that was there from the start reads as a map being completed"*. The roadmap's own header rule settles it — the owning document wins and the roadmap is the one that gets fixed — so the line is withdrawn and the 1.0 criterion now says in writing that a `soon` room is not a placeholder. The shape of the product is a promise being kept in public, and `kind: 'soon'` is the type in the code that says so.

**3. The automation room connects an external engine. We do not write a scheduler.** Owner: *"จะทำเป็นทางเลือกให้เชื่อม n8n หรือ Windmill ครับ เชื่อมภายนอก ถึงจะทำได้"*

Three documents held three answers: `COMPANY.md` §7 planned a scheduler for later, the roadmap's deferred table said n8n makes one unnecessary, and the roadmap's debt list said to hide the room entirely. The answer is the middle one, stated as the room's definition rather than as a shortcut: **they are the clock, Aetox is the hands.** n8n and Windmill already have cron, webhooks and triggers that we would spend a release approximating, and the point of the product is the hands.

Two consequences worth writing down. **More than one engine from the first line of code** — an integration shaped around n8n alone makes the second one a rewrite, and the owner named two on the day of the decision. And **the room genuinely does nothing until something is connected**, which is not a temporary state to apologise for: it is what the room is. The trigger arriving from outside walks through the hiring door built in §84.1, so nothing is reworked — but *who an unattended job asks for approval* is untouched by any of this and still needs its own decision before the room opens.

## 93. Decision — A Prompt Layer That Names a Tool Must Be Able to Ask Whether the Tool Is Here (2026-08-09)

Owner: *"พรอมป์อะไรที่มันเกินจำเป็น หรือบางอย่างที่โมเดลรู้อยู่แล้วก็ไม่จำเป็นต้องระบุ ไปตรงๆนะ"*

An audit of what `internal/prompt` actually sends found the two desks sharing 9,395 characters of identical engine text — 70% of the assistant's prompt and 86% of the coding desk's. The desk-specific part was 18% and 5%. That ratio is fine on its own; what it hid is that the shared 86% was written before desks existed and never learned they do.

**Two instructions were being sent to desks that could not follow them.** `fileEditing` told every desk to call `diagnostics` after changing a source file — a `code` tool, so the assistant desk, whose declared audience is someone who has never opened a terminal, was sent after a tool it was never given, on every code edit. `longform` told every desk to hand deliverable work to *the agent whose craft it is* — but the coding desk declares no `dispatch:`, and `subagent.available` filters those agents out of the list its `task` tool advertises, so the sentence described a move with nobody on the other end.

Both are the same defect: **a layer that names a tool cannot ask whether the tool is here.** `Desk` carried a name and a direction, which is enough to say *what this session is* and nothing at all about *what this session holds*. So it now carries two more things — the manifest's own `AllowsTool` as a closure, and one bool for whether the desk may dispatch. A closure rather than a copied `[]string` because `internal/prompt` must not import `internal/mode` (§11 — a package this low reading one that reads `skill` runs the dependency backwards), and a copied list is a second answer to a question the manifest already answers. The zero `Desk` still carries everything and may dispatch to anyone, which is what a pre-desks session was.

**The rule this generalises to, written into the package README:** a layer that names a tool takes `Desk` and asks first. The cost of getting it wrong is not a style problem — it is a round the user pays for, ending in the assistant reporting a limit that does not exist.

**And the second half of the audit: say the rule, not the argument for it.** The engine layers had grown essays justifying instructions the model does not need justified — that an edit is cheaper than a rewrite, that a tool call costs a round, that it is allowed to do 20% of 500 in its head. What a model cannot know from the tool list stays: that `apply_patch` is atomic, that grep-with-context usually beats reading the file, what `calc` cannot reach, where the line between short and long arithmetic sits. Trimming that back took ~450 characters off every request at both desks, and the deduplication of the document rule — stated three times in `assistant.md` and once more in `longform`, four statements of one rule — took another ~600 off the assistant's. Nothing Aetox-specific was cut.

**Where the rationale goes instead:** the comment above the function. Comments are not sent; prompt text is paid for on every request, forever. That is the whole of the accounting, and it is why this package has always been written with more explanation above the strings than inside them.

## 94. Decision — A Desk Owns Its Own Team, and a Team's Ceiling Is Its Desk's (2026-08-09)

Owner, choosing between a per-desk home and a `desk:` field in the profile: *"เอาตาม 1 ครับ แยกแบบนั้นดีสุด"*

Three sentences in `COMPANY.md` were written when every agent that existed sat at one desk, and they were written as absolutes. Giving the code door its own team — planned since §84, listed as missing in `DOOR-CODE.md` §2 and §6 — contradicts all three unless it is said plainly which part of them was the rule and which part was the circumstance.

### 94.1 An agent declares its desk by where it lives, and nowhere else

`<DataRoot>/agents/<desk>/<name>/` — the same folder-is-the-agent shape decided on 2026-08-06, with one level above it.

This keeps the rule v0.9.3 established and did not repeal: **no profile writes `desk:`**, because the field can only ever be written wrong, and wrong is silent — a file at a desk that renders no team page still parses, still validates, still appears in `List()`, and vanishes from every door a user can walk through. `applyHomeRules` learns a second home; it does not learn a new field. `internal/subagent/profile_test.go`'s assertion that no bundled agent writes `desk:` stays true and stays where it is. That the field-free rule survives intact is the reason this shape won over writing `desk: coding` into a file.

**A name still has exactly one owner, across every desk.** `jobs.agent`, `sessions.agent` and the `learned` scope are all keyed by the bare name, so two agents called `reviewer` at two desks would silently share one memory file and one work history. `resolve()` therefore reports a name held in two desk folders as a `Conflict`, exactly as it already does for a name held in both the agent and sub-agent homes. The desk folder says where an agent works. It never says who it is.

The cost, stated so nobody meets it as a surprise: `config.AgentHome(name)` stops being a join and becomes a lookup, because a name no longer determines a path. Five call sites, and `MigrateAgentHomes` already exists to move what is on disk today into `agents/specialized/`.

### 94.2 An agent's ceiling is its own desk's manifest — so an agent at the code desk holds a shell

§84 says the `specialized` manifest is the ceiling over *every* agent, that no agent has a shell or code tools, and that this is safety by structure rather than by discipline. The mechanism stays exactly as it is — every agent still intersects with a ceiling it cannot widen from inside its own file. What changes is which ceiling: **the one belonging to the desk the agent sits at.**

An agent at the coding desk therefore holds `shell`, `git` and `diagnostics`, because the coding desk does. This is the first time an agent holds a shell, and "an agent has no shell" stops being true of the system and becomes true of a desk.

Why that is not the hole it sounds like: the ceiling was never what made a shell safe. §83 has said since it landed that **a desk changes what is on it, never what is allowed** — the safety gate and the approval rules are identical at every desk and for every agent, and §92 struck down a refusal that had been derived from a tool list precisely because missing tools were being read as a missing permission. A coding-desk agent running a command is gated exactly as the coding desk itself is: same assessment, same approval card, same audit row. What the ceiling actually buys is that a document writer cannot reach code tools it has no business holding — and that is preserved, because the document writer is still at `specialized`.

### 94.3 Hiring inside one desk is not crossing a desk

§84 says the code desk hires nobody and nobody hires it, with no exceptions offered. That rule is about the **star**: work crossing from one desk's context into another's, which is the thing that carries context, cost and blame across a boundary nobody can see.

A coding-desk agent is hired by the coding desk and hands its result back to the coding desk. Nothing crosses. The absolute was written when the only agents in existence were at `specialized`, so "hires nobody" and "hires nobody at another desk" described the same world and did not have to be told apart. They do now:

- **Across desks:** the code desk still hires no one and is hired by no one. The star keeps one center, and that center is `specialized`.
- **Within a desk:** a desk may hire the agents that live at it, at depth 1, and a leaf is still a leaf — a coding-desk agent hires nobody in turn.

### What this does not decide

Which agents sit at the code desk (`reviewer` / `explore` / `tester` have been talked about; none is chosen here), and the team **loop** — architect, approved design, parallel build, assemble, disband — which still needs plan mode back from §44 first. This decision is the room and the rule for who may sit in it, not the people and not the choreography.

## 95. Decision — Waiting Is a Condition, Not a Sleep (2026-08-10)

Owner: *"ระหว่างที่รอระบบภายนอกทำงาน … ควรจะมีระบบรอเหมือน Claude Code แบบหยุดเรียกโมเดลสักแปบ รอมันโหลดเสร็จ"*

The background-shell tools left one round-trip on the table. `shell_output` returns at once, always, and its own description told the model what that costs: *"nothing yet is not a failure, call again."* Every one of those calls-again is a full model round paid for learning nothing — a three-minute build could burn ten turns saying "still going." The work Aetox exists for — builds, installs, servers coming up on this machine — is exactly the work where waiting happens most.

**The shape rejected first: a sleep tool.** `sleep(seconds)` forces the model to guess a duration, and both guesses are wrong — too short polls anyway, too long sits idle after the work already finished. The model knows *what* it is waiting for, never *how long*. So the model names the condition and the process does the waiting: `shell_output` gains `wait_for` (a regex against new output, or the sentinel `"exit"`) and `wait_timeout_seconds` (default 60, ceiling 600), and blocks until match, exit, or timeout — whichever is first.

**A field, not a tool.** A new tool is ~300–500 tokens on every request forever (§ the 8,800-token audit that already ruled "trim descriptions, don't add doors"); two fields on an existing tool are a fraction of that, and the job registry, the drain mechanism and the kill path all already exist. The wait peeks without consuming — only the read that reports to the model marks output read.

**The turn's patience follows the wait.** `toolCallDeadline` had one exception, `shell`, with the rationale *"the tool itself is the only thing that knows how long its work is about to take."* A waiting `shell_output` is the second tool that qualifies, by the same sentence: a wait the turn cuts short at 60s degrades back into the polling loop it replaces, parked behind a "STILL RUNNING" the model must poll. So the executor reads `wait_timeout_seconds` too — but only when `wait_for` is present. A plain read returns at once and buys nothing by naming a number.

**Every wait has three exits and all are bounded.** The match, the command's own exit (checked once more after done, because a line printed on the way out is a match, not a miss), and the timeout — which is a report, not an error: the header says the command is still running and was not stopped. The fourth thing that cuts through is the Stop button — ctx cancellation ends the wait, and *only* the wait: the command survives, because v0.9.4's promise that the stop button stops things must not grow a new thing it fails to stop, and a dev server that dies because the user cancelled a status read is a different bug wearing the first one's fix.

### What this does not decide

The other half of Claude Code's shape — the finished job *waking the model up* without being asked. That means an event starts a model call no user typed, which changes what a turn is, and it is not bought here. A blocking wait covers most of the pain without touching the turn model; the wake-up waits for its own decision. The desktop's background-tasks side panel (running jobs, elapsed, a stop button per row — the owner pointed at Claude Code's) is spun off as its own task: it is a UI over a registry that already answers every question the panel asks.

## 96. Decision — Do Not Ask the Model to Match What It Cannot See, and Do Not Answer a Failure With "Look Again" (2026-08-09)

Owner: *"เครื่องมือ edit เคยล้มซ้ำ ๆ ด้วยเหตุเดียวกัน … เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก"*

`edit` compared `old_string` to the file byte for byte, and returned *"old_string not found in file; re-read the file and match the text exactly"* when it did not. Both halves of that were wrong on the reference platform, and together they made a loop.

**The match.** Windows checkouts have `core.autocrlf` on and this repository ships no `.gitattributes`, so every text file on the machine Aetox is developed and shipped on ends its lines `\r\n`. `read` returns those bytes as they are — correctly; it is a file reader. But a `\r` is invisible in a way a tab or a trailing space is not: nothing in the rendering of the file distinguishes a CRLF line from an LF one. So the model joins the lines it saw with `\n`, `strings.Count` returns zero, and the edit is refused. **Every multi-line edit, every time.** Single-line `old_string` values contain no newline and were never affected, which is exactly why this survived: it presented as an unreliable model rather than as a deterministic bug, and the deterministic bug is what it was. A test now pins it.

**The advice.** Told to re-read, the model re-read, saw the same characters, composed the same `\n`, and failed identically — the same error for the same reason, which is what the owner had been watching. Worse, re-reading is what the system prompt spends a paragraph telling it not to do (*"Do not read a large file end to end just to change one line in it"*, §93's own layer). The tool was contradicting the prompt while asking for the one recovery that could not work.

**Both fixed in [internal/skill/lineendings.go](../internal/skill/lineendings.go), and neither is a leniency.**

*Match on what the caller can observe.* The literal string is tried first and always wins when it matches, so a caller that did send the file's line endings is never second-guessed; only then are the LF and CRLF forms tried. A line ending is the file's business, not the caller's — if `old_string` identifies exactly one place once that is set aside, that is the place it meant. The uniqueness rule is untouched: two matches are still two matches and still refused.

*Write back in the file's own convention.* This half is not optional, and getting it wrong would be worse than the original bug: replacement text arrives with whatever newlines the model typed, and dropping an LF line into a CRLF file leaves a mixed file that shows up in everybody's diff and that nobody typed. So `new_string` is converted to the file's dominant ending on every edit, not only on the fallback path.

*Say what is wrong, from the file you are holding.* The tool has the content in hand at the moment it fails, and the failure is almost never "I misremembered the text" — it is one invisible difference. So the error now names it: a line-number prefix carried over from `read`, indentation that is tabs where the caller wrote spaces, a first line that matches with a later line that does not, or text that is genuinely absent. That turns a whole re-read into a one-line correction. Kept short on purpose: every character is a tool result paid for on the turn that already failed.

**`apply_patch` carried the identical bug and a heavier bill** — a patch that cannot apply in full writes nothing, so one invisible `\r` threw away every edit in the batch. Same fix, same file.

**`write` carried the second half of it.** The prompt sends `write` here for the "replacing nearly all of an existing file" case, and it wrote the caller's bytes verbatim — so on a CRLF checkout, replacing one function rewrote every line of the file in git's eyes. The model meant to change a function; the diff said it touched the whole file. Overwriting an existing file now keeps that file's line endings. **A file that does not exist yet is left exactly as typed** — there is no convention to honour, and choosing one there would be the tool overreaching, which is why the check is on the outgoing file rather than on the platform.

**The rule this generalises to, written into [internal/skill/README.md](../internal/skill/README.md):** never require the model to match something it cannot see, and never answer a failure with "go and look again" when the tool can answer it. A tool that can diagnose and hands the diagnosis back instead is spending the user's round to avoid its own.

---

## 97. Decision — A Second Engine Is Another Dialect, Not Another Agent; and a Server May Be Taken in Part (2026-08-10)

Owner: *"ผมกลัวที่ มันจะไม่ครบอ่ะดิ ถ้ามันจะเยอะขนาดนั้น ทำเอเจนสำหรับ n8n โดยเฉพาะ ไปเลยดีกว่าไหม แต่ willmill แม่งดีว่ะ"*

Windmill arrived, and with it the question §92.3 had deferred: one automation agent that speaks two dialects, or one agent per product. The pull toward splitting was the tool count — two engines' tools in one context looked like too many.

**Splitting was refused, and the number that refused it is already in the repository.** [desktop/tool_budget_test.go](../desktop/tool_budget_test.go) records that hiring an agent costs ~58 tokens **on every request from every user**, because `task` names the team inside its own description. Two vendor agents is ~116 tokens forever, paid by people who have never automated anything. The tools, meanwhile, do not add up the way they appear to: `connect.Allows` withholds an engine's tools until that engine is attached, so a user who runs one engine never carries the other's. **The cost of splitting is global and permanent; the cost of carrying both dialects is local and only paid by the user who chose both.** The identity prose would also have been copied, and two copies drift — which is the same failure §91 named when it refused to make an agent out of a file extension.

**So the shape stays: one agent, dialect sections, one skill per engine.** `TestTheAutomationAgentBelongsToNoVendor` already pinned that the identity names no product; it now has a second dialect beside the first, which is exactly the change that test was written to make cheap.

### 97.1 Windmill comes in as a connection like n8n, and not through its own MCP server

Windmill ships an MCP server in the product — Community Edition included, `createFlow` and `updateFlow` among its tools, streamable HTTP with a header, which [internal/mcp/client.go](../internal/mcp/client.go) already speaks. It was the cheaper road and it was recommended: a working write path for no client at all.

**It was not taken, and the reason is what a second mechanism would have cost everywhere else.** An engine reached through somebody else's server is an engine whose calls arrive by a different route than n8n's: a different permission rule, a different shape in `tool_runs`, a different Settings page to switch it on, and a different sentence in the agent's prompt explaining which is which. The user does not have "an automation engine connected" — they have one of two kinds of thing, and every surface downstream has to know which. That is the seam §92.3 was written to prevent, arriving one layer lower than expected.

So [internal/automation/windmill](../internal/automation/windmill) is a client like [internal/automation/n8n](../internal/automation/n8n), Windmill is a `connection:` in the same catalog with the same `Family: FamilyAutomation`, and both engines are switched on in the same place. The MCP route stays available and is now a fallback with a written reason rather than a road not looked at: if Windmill's API moves somewhere the client cannot follow, the server is still there.

What this cost, recorded because the budget test asks for it: ten engine tools now sit in the registry instead of five.

### 97.2 A `needs:` entry may name alternatives, and the nearest one is the one reported

`needs: connection:n8n | connection:windmill` — met when **any** alternative is met. It spans kinds as readily as it spans ids (`connection:x | mcp:y` is valid), which is what keeps 97.1 reversible. Without this the automation agent's declaration was false for every Windmill user: the roster told them they were missing n8n, which they had never wanted, and a readiness warning that is false teaches people to ignore the roster.

`|` inside the entry rather than a second frontmatter key, because the alternation belongs to the requirement and not to the agent: an agent may want "either of these" for one thing and "definitely this" for another, and a key cannot say which of its entries go together.

When none is met, **one** need is reported, not a list — two lines read as two missing things and the agent asks the user to set up both, when the second may be one they deliberately did not choose. The one reported is the one nearest to working (`closeness` in [internal/subagent/needs.go](../internal/subagent/needs.go)), so an account already connected and one toggle away beats a product that is not installed. Reporting the first-written would routinely send the user to do the expensive thing.

### 97.3 A server may be taken in part — as an allowlist, never as a denial list

n8n-mcp is two products in one box: 7 tools answer what n8n's own API cannot (it publishes no node schemas), and 16 write workflows, which Aetox already does through its own tools, its own permission rules and its own `tool_runs` log. Taking the whole server puts a second way to write in front of the model, and whichever it happened to pick would decide whether the user's record of what was done to their instance had the entry.

`deny:` on the profile could already have cut those 16 today. **It is the wrong instrument**, and the reason is the failure mode rather than the effect: a denial list is written against somebody else's catalogue, so the day that server ships a seventeenth write tool it arrives switched on and nothing says so. `Tools` on the server config ([internal/mcp/client.go](../internal/mcp/client.go)) is the allowlist form — new tools stay out until somebody adds them on purpose.

Two details that are the whole point of it being safe:

- **Both spellings match.** The server calls it `get_node`; Aetox shows it as `n8n-docs_get_node`. Both are names the user can read off a screen, and accepting only one would mean a list copied from the more natural place silently selected nothing. Which forced the list to resolve against the server's whole catalogue rather than one tool at a time: a server whose own names already begin with its server name makes the two spellings **collide** — on a server named `n8n`, `n8n_validate_workflow` is the bridged name of n8n-mcp's docs tool `validate_workflow` and the raw name of its workflow-writing `n8n_validate_workflow`. An ambiguous entry is refused by name, because guessing would sometimes admit a write tool to a list written to exclude every one of them, silently, through the mechanism built to prevent exactly that.
- **A list that matches nothing is an error, not an empty server.** A typo and an upstream rename look identical from here, and both leave an agent with no tools and nothing on screen saying why. It fails loudly instead — the same principle `needs:` was built on.

Empty means take everything, so every server configured today is unchanged.

**It is on the Settings form, under More options, and not only in the file.** The `for:` field's own note says why: a rule the user cannot see, edit or switch off is not a switch. That put one more thing on the save path to get right — the form must be able to *clear* a list, so an empty box has to mean empty, while a caller that omits the field entirely must leave the stored one alone. Absent and empty are different answers here, and `TestTheFormOwnsTheAllowlistAndSilenceDoesNot` pins both directions.

### What this does not decide

Whether to adopt n8n-mcp's 7 documentation tools at all. That is now a reversible decision rather than an architectural one, which is why it was left until after the allowlist existed. The counter-argument on the record: its node schemas are a snapshot of the n8n version it was built against, not of the user's instance, so **read-the-real-workflow stays the rule** — the tools would reduce guessing, not remove it.

---

## 99. Decision — Packing: One Name in the Tool Block, Several Rights Inside It (2026-08-10)

Owner: *"browser จริง ๆ มันควรจะแพ็ครวมกัน... หากเพิ่มจะได้ไม่ต้องเสียเวลามาไล่เปิดทีละตัว ๆ"* — and, when the objection was put that four names are four rights: *"รวมแล้วแยกย่อยข้างใน"*.

`browser` was four tools describing one object. Every capability added to that object cost another entry in the tool block of **every request that carried it** — a bill paid per turn, forever, by users who never open a browser. Packed, a new capability costs an action instead.

The objection worth recording, because it is what the design answers: **four names are four rights.** A profile's `tools:` grants by name, so collapsing them would have made "may look at a page" and "may act on one" the same permission — a distinction this product cannot lose, because its rule is that rights come only from a list the user can see (§44, §94). The answer was one tool on the outside and the original names still the vocabulary of permission on the inside.

`shell` (3 tools), `github` (4), and then the two engines — `n8n` (5) and `windmill` (5) — follow, and the second one is where the idea had to be made general rather than copied. The engines are the sharpest case of the argument: they are the tools about to grow (§100's proof loop wants run and executions on n8n), and packed, that growth costs description lines instead of tool-block entries.

### 99.1 The rule: outside is the packed name, inside is the name the act always had

> Outside — what the model calls, what a hook matches, what the timeline shows — is the packed name. Inside — what the approval gate judges, and what the turn measures its patience against — is still the old per-action name, because **the act has not changed**.

The second half is not tidiness; it is the difference between working and broken, and it is why [internal/skill/packed.go](../internal/skill/packed.go) exists instead of four lines in each tool:

- [internal/safety/safety.go](../internal/safety/safety.go) keys on the literal string `"shell"` and answers `RiskHigh` to a call it can find no command in. Pack the three shell tools naively and **reading a background command's output becomes a high-risk act the user is asked to approve** — which is how you train someone to click through the prompt that matters.
- [internal/turn/executor.go](../internal/turn/executor.go) stretches a turn's deadline only for a `shell_output` that was asked to `wait_for` something (§53.1, §95). Under the packed name it would read `timeout_seconds` off a call that has none and **cut the wait off at sixty seconds**, degrading it back into the polling loop §95 removed.

Neither failure announces itself. `Unpack` is the one function both readers now go through, and `TestTheGatesBelowTheBlockStillSeeThePerActionName` is what stops the next refactor from "cleaning up" a leftover name that six other things depend on.

### 99.2 A packed tool cannot ask who is calling it, so it is told

`browser` lived in `desktop` and for its first day read the open session's profile directly (`a.chairProfile()`) — it could, being where a profile is visible. `shell` and `github` live in `internal/skill`, which **must never import `desktop`** — so the answer arrives from outside: a `Packed` tool declares its per-action names, and `subagent.FilterRegistry` hands back a narrowed copy built from the profile's own `tools:` and `deny:` lines.

A copy, not a mutation: the registry is shared by every session in the process, and narrowing in place would let one agent's `tools:` line quietly take an action away from everyone else.

Once the general mechanism existed, `browser` moved onto it the same day — its private profile-reading gate was retired before it could become a second mechanism answering the same question. All four packs now declare their vocabulary in the one table (`packed.go`) and are narrowed by the one function.

Three sentences a `tools:` line can say, all of them pinned by `TestAToolsLineNarrowsAPackedToolToTheActionsItNames`:

- **naming the tool** asks for all of it — which is what every manifest written before the packing says, and it has to keep meaning that;
- **naming only an action** (`tools: shell_output`) asks for that action and nothing else — which is *also* what a manifest written before the packing said, in the days when it was a whole tool name;
- **naming nothing** is not "nothing allowed". Reading silence as a denial hands an agent a tool that refuses every call, which is the same fault as handing it a tool it does not have, only harder to see from outside.

`deny:` reaches inside the pack, and a worker left with no actions at all is handed nothing rather than a tool that says no to everything — the state the 2026-08-10 incident actually shipped, where every screen said equipped and the toolbox was empty.

### 99.3 What is deliberately not in a pack

`plugin_install` reaches GitHub through the same connection and the same gate as the four `github` actions, and is not one of them. The other four hand back something to read; this one puts code on the user's machine. **A tool that installs is not a depth of looking**, and folding it in would have made "may read a repository" and "may install from one" the same grant.

`desk_terminal` is not a `shell` action for the same kind of reason: it runs a command in a terminal the *user* is watching, has its own branch in `safety.Assess` (an empty terminal is not a shell with an empty command), and is a different object rather than a fourth way to run something.

### What this cost, and what it bought

Measured by [desktop/tool_budget_test.go](../desktop/tool_budget_test.go): **45 tools / ~9,764 tok → 40 tools / ~9,728 tok** with no engine connected, and connecting an engine now adds **+2 tools / ~802 tok** where it added +6 / ~941. The token saving is modest because a packed description carries every action's line; the entry saving is the point, and so is what happens next — the executions work §100 will bring to `n8n` now costs an action rather than three more names in everyone's tool block.

The per-action names — `shell_output`, `shell_kill`, the four `github_*` readers, the five `n8n_workflow_*` calls and the five `windmill_*` calls — are no longer registry entries and can no longer be called by the model directly (`shell <cmd>`, `github search <q>`, `n8n list` keep working as door codes, routed through the pack). They remain, everywhere it matters: `categories:`, a profile's `tools:` and `deny:`, `connect.Allows`, the permission rules, and the sentence in the approval prompt.

---

## 101. Decision — A Cache That Outlives What It Describes Will Lie; Ask the Thing Itself (2026-08-12)

Owner, having watched all five agents refuse in a row: *"ทำไมไม่ได้สักตัวเลยครับ"*

Ten dispatches, ten refusals, none of them longer than six milliseconds — `doc`, `deck`, `sheet`, `github`, `automation`, twice each. Each one came back saying the MCP server they work with *"[had] not finished connecting yet — start this job again in a moment"*, and starting again produced the same answer for the same reason, forever.

The server had connected. `mcpMgr.Register` finished in 12.5 seconds and logged no error at all. The tools were enumerated and then dropped on the floor, and nothing anywhere said so.

### 101.1 What was actually wrong

[internal/mcp.Manager](../internal/mcp/manager.go) is built once and kept — the comment at the top of the file says so, and that is right: connections are cached there, so a re-bootstrap reuses a live session instead of respawning every server. Its `attach` carried a `done[server]` flag with that same lifetime, to stop a second dispatch re-registering names the registry already held.

**A `*skill.Registry` does not have that lifetime.** Every bootstrap builds a new one. So the flag was answering a question about the registry with a memory belonging to the manager, and the moment there were two registries the memory was about the wrong one.

There are always two. [App.startup](../desktop/app.go) calls `focusNone()` and then `openAtRememberedDesk()`, and each re-bootstraps the engine — two registries, nineteen milliseconds apart, two background registrations against the one manager. The first goroutine to return set the flag and filled **the registry that had already been superseded**; the live one asked, was told `done`, and got `nil`. No error, because from `attach`'s point of view nothing had gone wrong.

The result on this machine: **no MCP tool was ever reachable in a desktop session** — not just for the agents, for the assistant too — and a mid-session model switch would have lost them the same way for the same reason.

### 101.2 The rule

> A cache whose lifetime is longer than the thing it describes will eventually describe something else. Where the thing itself can be asked, ask it.

`attach` now skips a tool **this registry** already holds and registers everything else, and `RegisterFor` decides whether a server is present by looking for its tools in the registry it was handed ([`holds`](../internal/mcp/manager.go)) rather than by consulting a flag. It is the same question `subagent.missingAgentServers` asks before starting an agent, asked the same way — so the guard and the thing it guards can no longer disagree. "Which tools does this registry have" was being answered in two places and nobody had noticed it was a second place; the fix is the usual one, which is to delete the copy rather than to keep it honest.

`TestRegisterFillsEveryRegistryItIsGiven` runs the two registrations concurrently the way startup does, and `TestRegisterForFillsARegistryBuiltLater` covers the deferred path after a re-bootstrap. Both fail against the old flag.

### 101.3 The two things that made it expensive to find

**A guard that returns "nothing to do" and "nothing happened, and that is a problem" as the same value.** `attach` returned `nil` errors for a successful skip and for a silent drop. The log showed a registration that took 12.5 seconds and reported nothing, which reads exactly like success. The fix removes the ambiguity by removing the state — but the shape is worth naming, because it is available anywhere a fast path returns early.

**Advice a user cannot act on is worse than no advice.** "Start this job again in a moment" was written for a real case: a dispatch in the seconds after launch, before the connect lands. It was handed out for a permanent condition too, and it sent the owner round the same loop twice. A retry instruction is only honest when the thing being waited for can still arrive; `EnsureServers` could not rescue this one either, because [`RegisterFor`](../internal/mcp/manager.go) only handles `Deferred` servers and this one was carried by desks. **Left standing, and named here so the next person meets it as a known edge rather than a discovery:** a server that has genuinely *failed* to connect is still reported to the model as one that has not finished.

### What this cost

Nothing measurable — one flag removed and one lookup added, on a path that runs once per server per bootstrap. What it bought is that MCP works at all in the desktop app.

## 102. Decision — A Feedback Loop Cannot Be Deduplicated Away; Cut the Path, Not the Repeats (2026-08-12)

Owner, twice in one morning: *"ปัญหาหนักมากครับเทอมินอลทำงานผิดปกติ"* — and after the first fix shipped — *"เทอมินอลยังพังยุไม่หาย"*.

The desk terminal, through an n8n boot, painted hundreds of half-shifted duplicate lines: `See:` arriving as `SSee:`, `SeSee:`, `SeeSee:`, and the PowerShell banner twenty times down the pane.

The first thing worth doing was the thing that turned out to matter most: **read the log file the pane was tailing.** It was clean — every byte, every boot marker, one n8n and not two. Nothing upstream was corrupt. The corruption was being *drawn*, not written, and that single fact ruled out the engine, the launcher and the log plumbing before any of them were touched.

### 102.1 What ConPTY actually does on resize

A probe against ConPTY directly, counting how often the shell banner reappeared in the raw stream:

| Phase | Banner appearances |
|---|---|
| Boot, no resizes | 1 |
| Ten resizes to **the same dimensions** | 11 |
| Ten more, oscillating 99↔100 | 21 |

**`ResizePseudoConsole` re-emits the entire visible screen on every call, unchanged dimensions included** — about 300 bytes a time. A resize is not idempotent, and nothing in its signature says so.

That justified a dedupe: `TerminalResize` now remembers the dimensions the PTY has actually received and drops a call that would deliver the same pair again. It is correct and it stays.

**It did not fix the smear.**

### 102.2 Why the first fix could not have worked

`.insp-slot` — the box the terminal is drawn in — carried `overflow-y:auto`. That closes a circuit:

output overflows the slot by a pixel → **a scrollbar appears and takes ~15px of width** → the pane is genuinely narrower → `fit()` computes a genuinely different column count → the PTY is told, correctly, about a size it has never heard → ConPTY repaints the whole screen → the repaint changes content height → the scrollbar leaves → the width comes back → around again, for as long as output keeps arriving.

Every size in that loop is new. A dedupe keyed on *"have I sent this one before"* passes every single lap, because the answer is honestly no each time. The guard was never weak; it was aimed at a different bug that happened to share a face.

> A loop that manufactures genuinely new values cannot be closed by suppressing repeats. Cut the path that feeds it. Deduplication upstream of a feedback loop suppresses the symptom in the quiet case and nothing at all in the loud one — which is exactly backwards, because the loud case is the bug.

The terminal's slot no longer scrolls (`.insp-slot.term-host`). xterm owns its own scrollback, so the bar was never load-bearing — it was only fuel.

### 102.3 Where the dedupe is allowed to live

An earlier attempt put it in the pane, skipping the call when `fit()` produced the same size. That shipped and was reverted the same hour: a shell wraps its lines at the width it was last told, so a width it never hears about leaves the pane rendering one wrap column while the shell uses another — scrambled, then unresponsive.

The two are not the same guard. **The pane knows what it measured; only the Go side knows what was delivered.** A size may be skipped when the PTY has already received exactly that size — never merely because the pane computed it twice. So the frontend reports every fit, and the filter sits with the one party holding the answer.

### 102.4 The same face, three times

Two other bugs wore this face the same morning, and both are recorded in the commits rather than here because neither changed a rule: a second `n8n_server_start` walking past a reachability check that a still-booting server cannot pass, and `launchLogged` opening its log `O_TRUNC` underneath a live `Get-Content -Wait` follower, whose byte offset then sat past EOF and never recovered.

Three unrelated causes, one symptom, and a user reasonably reporting all three as *"the terminal is broken"*.

### What this cost

Three lines of CSS, a five-line guard in Go, and most of a morning — nearly all of it spent on the fix that was correct and insufficient. The cheap lesson is about ConPTY. The expensive one is that *the artifact you are reading may not be the artifact that is broken*: the log was clean from the first minute and said so plainly, and every minute after that was spent above it.

## 103. Decision — A Step That Cannot Mistranslate Is a Step That Does Not Run (2026-08-12)

Owner, 12 ส.ค., after watching a question come back as a manual: one address, for both of us.

Until now there was exactly one route to a worker: the assistant decided to call `task` and wrote the brief itself. So the worker never saw the user's sentence — it saw a paraphrase of it. The failure that made this concrete: *"ask doc how a good document is put together"* reached `doc` as *"write a manual about how a good document is put together"*, and `doc` — correctly, given what it was handed — wrote the manual. `doc`'s own rule against answering a question with a file could not save it, because by then there was no question left in the brief.

### 103.1 The address is the same act, performed by either party

`@doc <request>` in the composer routes the message to `doc` **verbatim**, before the model runs at all. The `@doc` token is not even stripped: what the worker reads is what the user typed.

This is deliberately *not* a second delegation path. It reuses the `task` registration the session already holds — same profile, same registry cut, same step ceiling, same prompt, same `ask_main`. `Dispatcher` is an interface for exactly that reason: the desktop borrows the existing machinery rather than growing a parallel one that would drift.

And it is announced in both directions. `agentChoice` now tells the model that *"the user addresses these same workers by writing @name"*, so a `@doc` sitting in the transcript reads to the assistant as the same act it performs itself, not as stray text.

> Removing a translation step beats improving it. A paraphrase that is 95% faithful still fails one request in twenty, and the failure is invisible — it comes back as a confident answer to a question nobody asked.

### 103.2 What the schema was quietly deciding

The `agent` parameter's description used to end *"…and returns a finished file"*. Because the caller was told the return type before writing the brief, every brief had to be phrased as an order to produce a file. The mistranslation above was not a lapse; it was the schema working as written.

The clause is **removed rather than reworded**. A specialist has its own instructions and its own tools and already knows what its work looks like; stating the return type in the shared schema is a second answer to a question that already has one — and the second answer wins, because it is the one the caller reads first. `TestTheAgentParameterDoesNotDecideWhatComesBack` pins it. `doc`'s own description changed the same way: from a list of artifact types to a description of the job.

### 103.3 A question asked is a run still open

A worker that stops on `ask_main` holds everything it has already read. So the answer must reach *that run*, not a new one: `Reply.Pending` carries the task id, the desk remembers it, and the user's next message — unless it addresses somebody else — is delivered as an answer rather than as a fresh job. One message where starting over costs the whole run.

Two smaller consequences worth having written down. The receipt line (`[task doc: N tool calls, Xs]`) is stripped from a user-addressed answer: it is an argument aimed at a model that over-delegates, and the user delegated nothing. And the assistant's history gets a two-message stub holding a 300-rune gist of the answer, not the answer itself — it needs to know the exchange happened, not to carry its payload.

**Left standing and named here rather than discovered later:** the stored view of an addressed turn has no steps under it. `Parts` comes back empty because the parent executor that assembles them is the piece being skipped, and `finishAddressed` synthesises a single `task` card in its place. Live, the timeline is complete; reopened, it is the answer without the work beneath it.

### What this cost

A parser, an interface, and two fields on the desk. What it removes is a whole class of failure that could only ever be mitigated: every request that survived being rewritten by a party who was not asking it.

## 104. Decision — Three Kinds of Bad News, Three Different Doors (2026-08-13)

Owner, 13 ส.ค., looking at three approval cards that were all the same outage: *"เราแยกดีมั้ย … ข้อผิดพลาดของระบบ ทำเป็นสำหรับกดเพื่อส่งให้นักพัฒนา … แยกกับ learning loop ที่เรียนรู้นิสัย ความชอบ"* — and the answer that settled it: the split is right, but there are three piles, not two.

The queue that morning held three cards drawn from one event — an n8n at `localhost:5678` that was not running. Two browser cards (the same unreachable server, split by error phrasing) and one n8n card ("ปฏิเสธ API key"), each proposing to carve *tonight's outage* into permanent memory as "เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก". The summarizer's own comment had already named this hole and predicted the failure; this is the day it happened on screen.

### 104.1 The three piles

**Aetox is broken** → the developer, as a GitHub issue *the user submits*. A new row on the About page opens `…/Aetox/issues/new` in the system browser with the two facts nobody should have to hunt for (version + channel, OS) prefilled into the body. The app sends nothing itself: the user reads the whole message on GitHub's form and submits it under their own account. The last reader before anything leaves the machine is the person it belongs to — that one sentence is the entire privacy design, and it is why this button needs no token, no consent screen, and no redaction pass.

**The world is not ready** — a server down, a key expired, a network out → *neither door*. Not an issue (the developer would drown in "somebody's n8n was off on Tuesday"), not a lesson (it will be false tomorrow morning). Today these are simply kept out of the learning queue; a session-scoped "known down right now, stop retrying" memory is the natural next step and is deliberately **not** built here.

**The agent broke a rule and the error says what to do instead** → the learning loop, unchanged.

### 104.2 The mechanism is the third instance of the same fix

This is the third time the summarizer broke on the same shape — *text in the error column is not always a lesson* — after `exit status 1` (§ the summarizer's 2026-08-09 note) and the bare "ไม่สำเร็จ" (2026-08-12). All three fixed in the same place, upstream, by recording what only the author knows:

- `internal/statereport` — a leaf package (n8n cannot import `turn`: turn→skill→n8n would cycle) whose whole API is `New/Newf/Mark/Is`. The line an author draws, written into its doc: mark it when what it reports would be true or false **regardless of how the caller behaves**; leave it when the remedy is a change in the caller's own next action. Unmarked stays the conservative default and still reaches the user as a card.
- `turn.ErrorFromWorld = "state"` — `classifyToolError` asks the error, never the text; the author's explicit mark outranks the `exec.ExitError` inference.
- The summarizer's `error_kind = ''` filter then drops these unread — the same gate that already excludes exit codes, no new query logic at all.

Marked at birth: browser navigation's two timeout/unreachable sentences, and n8n's connectivity, 401/403, and residual-status errors. n8n's 404/400/409 stay unmarked on purpose — a wrong id or a rejected graph is the caller's to fix, which is exactly what a lesson is.

Missed at birth, marked 2026-08-18: every *"this machine does not have X"* message in the tool layer — `image_ocr`'s Tesseract, `pdf_read`'s poppler, `video_ocr`'s ffmpeg, `internal/stt`'s whisper binary and model file, `git`'s own absence from PATH. The rule above was right and the door was open; nobody walked the OCR family through it, so three missing-Tesseract runs on a machine whose installer step had been skipped drafted card #22 — "เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก", a permanent rule against OCR written from a missing install. Worth naming as the cost of putting the knowledge at the author: a rule holds only where an author applied it, and nothing fails loudly at a site that forgot. `internal/skill/missing_program_test.go` and `internal/stt/missing_program_test.go` now assert the mark at the source, which is the only place that knows.

Still unmarkable by construction: a tool that reports failure through its `Output` instead of returning an error. `classifyToolError` reads the error *value*, and `executor.go` falls back to `failureReason(output)` when there is none — so `n8n_server_start`'s "ยังไม่ตอบใน 90 วินาที" (3 runs, 11–12 ส.ค.) and `task`'s "the MCP server(s) … have not finished connecting yet" (2 runs, 16 ส.ค.) reach `tool_runs` with `error_kind = ''` no matter what their authors know. Both are weather. Closing it means giving `Output` a kind of its own; not decided here.

### What this does not decide

What the learning loop should learn *next* — corrections the user makes mid-task, preferences said in passing, fail→success pairs, recurring requests as starter cards — was agreed in direction the same day but is design, not code, and none of it is built here. The session-scoped "known down" memory for pile two likewise.

## 105. Decision — Work Given to Somebody Else Does Not End When the Sentence Does (2026-08-13)

Owner, looking at Claude Code's background-tasks tray — *"Aetox ควรจะมีฟีเจอร์นี้นะครับ"* — and then, once the mechanism behind it was clear: *"ผมก็อยากได้แบบนั้นแหละ"*.

The tray is not the feature. What makes that panel worth having is that the work inside it is still running while the conversation moves on; a panel over work that dies with the reply would be an empty box every time you looked at it. So the first thing to change is not the UI.

### 105.1 The rule that had to go, and the one already in the house

§44.11 gave a delegate the turn's own context and called the consequence a benefit: *"Turn-bounded lifetime … no reaper, no leak."* Read again with a year of use behind it, that is not a lifetime rule — it is a deadline nobody agreed to. A model that started a delegate and answered before collecting it destroyed the whole run at the moment of the reply: minutes of real reading, thrown away for the crime of finishing a sentence. The user watching the timeline could see the work happening and had no way to keep it.

The counter-argument was already written down in this repo, in `internal/skill/shell_background.go`, for the same question asked about commands:

> *context.Background, deliberately: the turn's context is cancelled the moment the turn ends, and a dev server that dies with the answer that started it is not a background command.*

Neither is an agent. The two mechanisms are the same shape down to the details — three tools over one register (`shell`/`shell_output`/`shell_kill` against `task`/`task_result`/`task_answer`), a handle rather than a process, a cap on how many may be unattended. They disagreed on exactly one thing, and there was never a reason for it: a shell could be backgrounded and an agent could not.

**So a delegate's life is the session's.** The turn that started it ending is not an event it hears about.

### 105.2 Stop is a statement about the work, not about the turn

Which leaves one thing that must still end a delegate early, and it is the user. A Stop button that leaves an agent editing files has lied.

That fact — "the user pressed Stop" — is not observable from inside `internal/subagent`: cancellation there cannot tell an ordinary turn ending from a brake being pulled, and both used to arrive as the same cancelled context. So the register is now the **host's** to hold: `subagent.NewDelegations()`, handed in through `TaskOptions`, returned on `bootstrap.Result`, and `StopAll()` called from `App.CancelTurn` — deliberately *outside* its idle check, because work started in an earlier turn is exactly the work still running when there is no turn to cancel. A re-bootstrap stops the old register's delegates for the same reason: an engine nobody can collect from must not keep spending tokens.

A host with no Stop button (the CLI, whose Ctrl+C takes the whole process; every test) passes nothing and gets a private register, so the shape stays the same everywhere.

### 105.3 What a question means now

A delegate parked on `ask_main` used to be freed by the turn ending, which is the same destruction as above wearing a politer face: the delegate is holding everything it worked out, and the answer it needs is one sentence. It now waits across turns, and the next turn can answer it and collect the finished work. The goroutine-leak test that guarded the old behaviour still guards the new one — it just presses Stop instead of ending a turn.

### 105.4 The frontend consequence, which is not cosmetic

`toolSteps` is cleared at both ends of every turn. A surviving delegate keeps emitting, so its rows would land in the **next** turn's live block and be drawn as work the user's new question caused — the same shape as the stale-checklist bug already fixed in `runLiveTurn`, and worse, because these rows are real work rather than a leftover.

Routed by asking which delegation a row belongs to: every event from inside a sub-agent carries the spawning `task` call's ref as `parent` (the executor stamps `Ref` and `WithCallID` from the same `call.ID`), so a parent whose row is not in this turn's list means the delegation started in an earlier one. Those go to `cockpit.backgroundSteps`, which is session-scoped — the same span the work now has.

### What this does not decide

**The tray itself.** `backgroundSteps` keeps surviving work out of the wrong place; it does not yet put it in the right one. When the panel is built it should read the register in Go rather than reconstruct one from events, because the register is authoritative and a stream can be missed on reload — and because the frontend cannot honestly tell a running delegation from a finished one anyway: a `task` row completes the instant the call returns, which is the moment the work *starts*.

### 105.5 The tray, and why the first one was wrong (2026-08-14)

Built the next day on exactly that argument. `Delegations.Snapshot()` is the read, `desktop/background_tasks.go` the binding.

The **first** cut was a one-line status strip on the composer, and the owner killed it within a minute of running the build: *"ถ้ามันทำงาน ควรจะไม่นิ่งแบบนี้"*. It was not inaccurate — it was useless, and the difference is the whole lesson. **A counter that changes every few seconds cannot answer "is this alive"**; the eye has no way to tell a slow number from a stopped one. What can answer it is a list of filenames scrolling past. So the second cut is a card whose centre of gravity is its last three steps, with the counters demoted to the corner:

- **State** comes from the register — running / waiting / done — because only it knows. **Steps** come from the live event feed, because the register does not keep them and nothing else can make the card look alive. Two sources, joined on the delegation's id.
- That join needed a new field. `ToolEvent.Parent` is the provider's tool-call id and `task_1` is the register's — two namespaces for one delegation, so a UI holding both could not tell they described the same thing. `ToolEvent.Task` closes it.
- **A finished card is a receipt, not a control.** Owner, same session: *"ทำไมต้องรอผมเก็บผลอ่ะครับ มันควรจะรู้ตัวเองสิ"*. Right — the poll already sees `done`, and that is the moment to act on it, so it sends a `[ระบบ]` turn telling the model to collect and report. The user presses nothing. A question (`waiting`) is deliberately **not** auto-answered: it needs the human, and that is the one thing the card asks for.
- Everything the card sends — the answer box included — goes out as an **ordinary chat message**, the same door the user's own typing uses. One entrance to the engine, not two.
- **No progress bar.** There is no total to divide by (nobody knows how many files a brief will touch), and a bar drawn from a guess is a lie told in a shape people trust.

Two smaller things that were not free: a delegate parked on `ask_main` that nobody has collected has its question sitting in a channel no reader may peek at, so the runner tracks `parked` alongside `pending` — "stuck, needs you" has to show *before* anyone thinks to collect; and a finished row leaves the card only when `collect` redeems it, because collecting is the one door out and nothing else can know it happened.

### 105.6 One card, wherever the work is

The tray shipped beside the *old* delegation block in the turn timeline, and the owner saw the split immediately: a delegation that finished inside its turn drew a bare left rail, one that outlived its turn drew a card. *"บอกให้เอาแบบใหม่แทนเลย"*.

Right, and the reason is not tidiness. **They are the same event at two moments** — the only difference is whether the reply happened to go out first — so two visual languages taught the user they were two different things, and the fast case (which is most of them) got the language that cannot show life.

What was *not* done is delete the old block. It is the turn's record: snapshotted into the message, still there when the session is reopened months later, and the card cannot replace it because the register is gone by then. So the block keeps its data and loses its looks — one card, two lives:

- **live** (running, in-turn or background) — spinner, ticking clock, steps arriving; state from the register.
- **history** (finished, or a reopened session) — the identical card, still, rendered from the stored `message.steps`.

The tool rows underneath are now literally the same markup (`.tool-step`) in both places rather than two things that looked alike, so a change to a step row cannot drift between them. The old `.subagent*` vocabulary is gone.

One trap this created, caught by the tests: `.bgw-card` now matches both a turn's delegation and a tray card, so a leftover background task in a test fixture counts as one of the turn's own. The fixtures clear it. Worth remembering the next time something reaches for that selector.

**And then the card had to fold.** Unifying the look meant every finished delegation drew its full step list, and a turn with four of them was a wall — *"มันติดกันจนดูยังไงไม่รู้"*. The rule that came out of it is the same one the card was built on, read in the other direction: **a running delegate's steps are the evidence it is alive, and a finished one's are a record nobody asked to re-read.** Same rows, opposite defaults — open while working, folded behind `ใช้ N เครื่องมือ` once done. The state is keyed on the `task` call's ref and holds only what the user actually toggled, so the default is recomputed per render and a delegation that finishes while you have it open does not slam shut under the pointer.

Its count is tool calls only. Narration and thinking ride in the same children list and are not tools — the same trap `ownTools` was written to avoid, which would otherwise have offered "used 11 tools" for a delegate that ran six and talked five times.

**Per-delegate model and tokens.** Still aggregated with no attribution, and `recordTokenUsage` still stamps the session's model name onto a delegate that was pinned to another one. A separate fix, and a pre-existing one.

**Telling the model, unprompted, that background work finished.** It is reachable — the transcript carries the id and `task_result` now works across turns — but nothing volunteers it. Matching `shell_background`, which does not volunteer either.

## 106. Decision — What Is on the Desk and How the Turn Runs Are Two Axes; โหมด Belongs to the One That Switches (2026-08-14)

Owner: *"ผมว่าเราพัฒนาโหมดการทำงานดีไหม แบบ มุ่งสู่เป้าหมาย หรืออื่นๆงี้อ่ะ โหมดวางแผน"*

**This one is recorded before the build, not after it.** Every decision above is written from the far side of a shipped change, which is why each ends in measured numbers. This is the design as agreed in conversation on 2026-08-14, written down first so the build has something it can be held to. The closing section says what it is expected to cost, and must be replaced with what it did.

### 106.1 Two axes, and why they were about to become one

`internal/mode` answers **what is on the desk**: which capability groups a session carries, which servers it attaches, what direction the base prompt gains (§83). It is fixed for a session's life, on purpose — the whole value of a desk is a context that never held another desk's tools (COMPANY.md §6.3).

What the owner asked for is a different question with the same shape: **how does this turn run.** Should the assistant stop at a plan, or carry on to a finish? Should it ask, or decide? Should it reach for anything at all, or just think out loud?

Nothing in the codebase answered that, and the temptation was to answer it with a fourth desk. That fails immediately: a desk cannot switch mid-session, and *switching* is the entire point — "this next message should be planned, not executed" is a thought you have at the keyboard, three messages into a conversation, not one you have when you open a window.

> **โต๊ะ answers what is on the desk and is fixed for the session. โหมดทำงาน answers how this turn runs and switches at any moment.**

The rule that keeps the second from breaking the first:

> **A โหมดทำงาน may only subtract from the desk. It can never add.**

That is what makes mid-session switching safe against §6.3. The reason a desk is frozen is contamination — a context must never carry another desk's tools — and removing tools cannot contaminate anything. Returning to ลงมือ restores only what this desk already had.

### 106.2 The word — and the three meanings it already had

The owner's call, after the collision was put to him: **the new axis takes the word โหมด, as โหมดทำงาน.**

That is only clean because the word was never doing one job. Counted on 2026-08-14, `โหมด` appears in three surfaces with three meanings:

- **COMPANY.md** — 14 occurrences, all meaning โต๊ะ, in a document whose §8 is a list of permitted words.
- **[th.ts:662](../desktop/frontend/src/lib/locales/th.ts)** — `'palette.approval': 'โหมดอนุมัติ'`. On the actual screen, today, โหมด means the **permission tier**.
- **Code** — `internal/mode`, `sessions.mode`, `memory/modes/`, all meaning โต๊ะ.

So the choice does not add a meaning; it settles which one wins and retires the others from the surfaces they were never supposed to be on:

| surface | before | after |
|---|---|---|
| screen | โหมด = ชั้นอนุมัติ | **โหมดทำงาน** = this axis · approval becomes **ระดับการอนุมัติ** |
| docs | โหมด = โต๊ะ | **โต๊ะ** only (14 edits in COMPANY.md, 1 in README.th.md) |
| code | `mode` = โต๊ะ | unchanged |

Code identifiers do not follow the screen. That precedent is already set and already written down: `Chairs()` survived the retirement of เก้าอี้ (COMPANY.md §4), for the same reason — an identifier is a different layer from a label, and renaming across the two on every vocabulary decision is how a codebase acquires churn instead of clarity.

**The objection worth recording, because the design has to answer it:** `โหมดทำงาน` and `ระดับการอนุมัติ` will sit in the same app, and if the second had kept the name `โหมดอนุมัติ` the two would have read as siblings — which teaches, in the vocabulary itself, exactly the thing COMPANY.md §6.2 forbids: that a working mode is a kind of permission. Renaming the approval control is not tidying. It is the one edit that keeps the rule legible, and it costs a single locale string.

### 106.3 Four positions, three of them interesting

| โหมดทำงาน | what the engine actually does |
|---|---|
| **ลงมือ** (default) | nothing is changed — today's behaviour |
| **วางแผน** | every writing, editing and running tool is withheld from the dispatcher; the turn ends in one plan and an approve control |
| **มุ่งเป้า** | ~~the goal is pinned in context; a finish condition and a ceiling (steps / tokens) are set; `ask_user` is suppressed unless genuinely blocked; the closing report says what was done, not what will be~~ — **dropped 2026-08-14, see §106.10** |
| **คู่คิด** | no tool definitions are sent at all |

Three of these are interesting and one is the way back. **ลงมือ must occupy a position on the switch** — a control you can enter but not leave is not a switch.

Two of them earn their place on the one test that matters here:

> **A โหมดทำงาน must change what the engine does. If it only changes how the assistant talks, it is a preset, not a mode** — and it does not pass the feature filter (COMPANY.md §1.2).

**คู่คิด** is measurable: sending no tool definitions saves the floor that [tool_budget_test.go](../desktop/tool_budget_test.go) pins at ~9,700 tokens per request, and it removes the failure where a request for an opinion produces six tool calls.

**มุ่งเป้า** *(designed here, declined at §106.10 — kept because the reasoning is what the decision rests on)* is the one that fails if written loosely. Without a written goal and a ceiling it is not a mode, it is the instruction "be more agentic", which is prose. All four parts are required. And what it changes is whether the assistant pauses to ask **the user** — a conversational choice, not a permission one. It does not touch `safety.ApprovalMode`, the gate, or any approval card. That dial belongs to the user and stays where it is.

**วางแผน turns out to be a door that is already on the map.** COMPANY.md §8 lists ประตูส่งไม้ (ผู้ช่วย → โค้ด) as *"ยังไม่สร้าง"*, and §3 describes it as: the assistant works the plan out in conversation, then opens a coding session with the plan already on the desk. That is วางแผน seen from the other side — it produces the one artifact that door needs. Build order follows from this: **คู่คิด** first (smallest, and it proves mid-session switching), then **วางแผน** (which lands the missing door), then **มุ่งเป้า** *(which was never reached — dropped 2026-08-14, §106.10)*.

### 106.4 Where it is enforced — the same one gate

Not a second filter. [`mode.Carries`](../internal/mode/mode.go) is the single place a registered tool is judged against the desk, and a โหมดทำงาน intersects there: `desk.Carries(name, src) && stance.Allows(name, src)`. A second gate would be a second answer to "is this tool available", and the subtract-only rule means the intersection can never widen anything.

The direction text folds in through [`prompt.BuildForDesk`](../internal/prompt/prompt.go), **after** the desk's own direction. That ordering is deliberate and the file already reasons about it: models weight later context more heavily, so position is the only mechanism there is for saying which of two instructions wins.

Persistence is one column. The session already carries three coordinates — `mode` (v8), `agent` (v9), `space` (v10) — and this is the fourth, at migration **v13**. The transcript stores the โหมดทำงาน each turn ran under, because a mode that switches mid-conversation makes the history unreadable otherwise: scrolling back to a turn that refused to write a file, with nothing on screen saying why, is a bug report waiting to be filed against a working feature.

### 106.5 It binds to the session, not to the agent

Asked directly — *"เราจะผูกโหมด กับเอเจนดีไหม เอเจนแต่ละตัวก็มีโหมดของมัน"* — and the answer is no, because an agent is in two situations and only one of them has anyone listening.

An agent **hired through a brief** ends at the counter: brief in, file out, nobody watching. คู่คิด would leave it unable to produce the file it was hired for; วางแผน would return a plan to a counter with no one at it, breaking the door's contract. What fits is the door's own terms, which need no stance to express them: the brief is the goal and the file is the finish condition, which is what ประตูจ้างงาน already *means*. So a hired agent runs at ลงมือ and takes no stance from its caller, fixed by the dispatch path rather than read from a setting.

*(This paragraph originally said มุ่งเป้า was the one that fit. It was right about the shape and wrong to need a mode for it — the door had always said the same thing. §106.10.)*

An agent **you walk in and talk to** (§85) is a session with a person in it, and gets every position.

The codebase had already drawn this exact line, one axis earlier. [`AttendedRegistry`](../internal/subagent/store.go) exists to add `ask_user` back for a direct chat, and its comment states the principle:

> *"…adds the single thing that depends on **who is listening** rather than on **what the work is**"*

A โหมดทำงาน is the second such thing, so it goes where the first one went. And because a direct agent chat is an ordinary session (`mode='specialized' + agent='<name>'`, [db.go:398](../desktop/db.go)), binding to the session gives agents their modes for free — no field, no precedence rule between an agent's declared mode and the user's chosen one, no explanation of why the field is ignored when the agent is hired.

The instinct behind the question is right and already has a home. *"This employee works this way"* is the body of its `AGENT.md`, which is where the direction of the work belongs. The test for which of the two a thing is:

> **A โหมดทำงาน is something that switches mid-conversation. If it never switches, it is the agent's prompt, not a mode.**

### 106.6 The control goes on the input row, not in the chip strip

The chips above the composer — project, agent, engine — look like the obvious home and are the wrong one. [Chat.svelte](../desktop/frontend/src/lib/Chat.svelte) states that strip's rule in its own comment: *"a desk or a chair is fixed for a session's life, so the switcher is a door to a fresh one, **never a dial on this one**"*. Every chip there starts a new session. A โหมดทำงาน is the opposite — it is a dial on this one — and placing it there would seat it directly beside โต๊ะ and เอเจน, which is the confusion this whole decision exists to prevent.

It belongs in `.tools`, on the input row, **immediately left of the model picker**: that row is already where the per-turn dials live (model, thinking level, context, palette), the two controls read together as one sentence — which brain, which mode — and the decision is made with hands on the keyboard.

Its appearance is not cosmetic, because a stance decides whether the assistant can touch the machine at all, and **nobody may be in one by accident**.

The first build drew ลงมือ as a bare glyph, on the reasoning that a default should not demand attention. **Overruled by the owner on sight, 2026-08-14, and rightly**: what that produced was a control you have to click to find out what it is doing. So the chip carries its name in every position, and `.on` changes only the colour — the ordinary mode stays legible, the ones that withhold tools are unmissable.

A menu, not a click-to-cycle: cycling means arriving somewhere by pressing one time too many, and what this dial changes is whether the assistant can touch the machine. Something like that is reached by choosing.

**Where it sits took three tries and the record is the useful part.** "Immediately left of the model picker" was written above and read on screen as the far right, which is not what the owner had pictured; moving it to the left end then displaced the attach button, which belongs against the text it attaches to. The layout that settled: **attach and the mode chip on the left** (the two things you set before typing), **the context meter and the model picker on the right** (the two readouts about the turn). Prose describing a layout by what it is *next to* is ambiguous in exactly the way a screenshot is not.

No per-desk guard. The neighbouring chips carry `{#if cockpit.desk !== 'coding'}`; this one appears at both doors, and วางแผน is arguably worth most at the coding desk.

### 106.7 What this is deliberately not

- **Not a permission tier.** COMPANY.md §6.2 is untouched: the gate and the approval rules are identical in every mode. A mode narrows what is on the desk, never what is allowed. `safety.ApprovalMode` remains the user's separate dial — which is why it is being renamed out of the word.
- **Not an identity.** A mode manifest that begins describing who the assistant *is* has become the thing §44.0 deleted. It says what the work is.
- **Not a matrix.** Modes are global and few. Not per-desk, not per-agent, not configurable at v1 — four modes across three desks is twelve cells nobody can hold in mind, and the same argument that keeps them off agents keeps them off desks.

### 106.8 คู่คิด, built (2026-08-14)

The first stance ships with the framework under it, which is why it was chosen first: it is the smallest one that proves the mechanism end to end.

**Measured on the assistant desk, no MCP connected:**

| | tool block | system prompt | total |
|---|---|---|---|
| ลงมือ | 34 tools / ~8,023 tok | 11,440 B / ~2,860 tok | **~10,883 tok** |
| คู่คิด | 0 tools / ~0 tok | 6,248 B / ~1,562 tok | **~1,562 tok** |

**~9,321 tokens saved per request — 86%.** A third of the saving is not the tool block at all: it is `prompt.Desk.ToolLess` skipping seven layers that were only ever instructions for using tools (capability, fileEditing, batchWork, computing, longform, narration, clarify). A session carrying nothing was being handed four thousand characters on how to call things, which is the failure `Desk.Carries` was added to stop arriving through the door `Carries` cannot watch — half those layers name no tool it could be asked about.

`drawing` and `panel` deliberately survive: they describe how the *answer* is rendered, and a diagram is something a conversation produces.

**Where it is enforced, and the second place that had to learn it.** `bootstrap.withStance` wraps `Desk.Carries` in an AND — never replaces it — so a stance has no way to hand back a tool the desk withheld; `TestAStanceNeverWidensTheDesk` puts the office desk into every stance and checks it never acquires `shell`. The dispatcher intersects the two axes at the one seam (`NewDispatcherFor`), and both halves are read at request time.

`App.deskTools` was the trap. It is a *second* place answering "what does this session carry" — the tools panel's copy — and it had `AttendedRegistry`'s own warning written above it about exactly this drift, one axis earlier. Left alone, คู่คิด would have emptied the model's tool block while the panel went on listing thirty-four tools. `TestTheToolsPanelAgreesWithTheModel` is what stops it happening again.

**Switching mid-session needed no new machinery.** `applyConfig` already carries the outgoing agent's context into the new engine — written for model switches, and exactly as true here — so `SetStance` sets one field and re-bootstraps. Prompt and dispatcher come out of the same value, so the two cannot disagree about what the session holds, and `TestSwitchingStanceKeepsTheConversation` asserts what was said before the switch is still there after it.

**Three refusals worth recording.** A stance switch goes through `guardSessionSwitch`, the same door-check a session switch uses — not because this is a session switch, but because it rebuilds the agent under a turn that would then finish on a context it did not start with. A stance name this build does not implement becomes ลงมือ rather than an error, in both `NormalizeStance` and `SetStance`: a session left in วางแผน and reopened after a downgrade must come back carrying everything, never silently carrying nothing. And `startNewSession` resets to ลงมือ *and re-bootstraps*, because `NewSession()` does not go through `setStation` and would otherwise have reset the field while the running engine kept the old stance's dispatcher.

### 106.9 วางแผน, built the same day

Look at everything, change nothing. Every reading tool stays — `read`, `grep`, `glob`, `web_fetch`, `pdf_read`, `diagnostics`, the four `github` readers, the automation *list/read* calls — and every tool that writes, runs or hands work to somebody who will is withheld.

**An allow-list, not a deny-list**, and the direction is the whole point: a tool added next month is withheld here until somebody decides it reads. A deny-list would hand a new writing tool to the one mode whose entire promise is that it changes nothing, and it would do it silently. `TestPlanWithholdsAToolNobodyHasClassified` pins the direction.

Three costs, all deliberate, all named rather than discovered later:

- **Packs go whole.** `browser` (open/read/click/type) and `shell` (run/output/kill/list) are single tools with mixed actions, so both go, taking two reads with them — a plan cannot click through a page and uses `web_fetch` instead. `skill.Packed` plus the narrowing `subagent.FilterRegistry` already does for a profile could split them; a stance narrows at the same granularity a desk does, and making it the first thing to narrow finer is a change to make on purpose. `github` is the pack that survives whole, because all four of its actions read.
- **MCP goes entirely.** `jira_search` and `jira_create_issue` are the same kind of string to this package. An allow-list cannot cover names it has never seen, and a guess would be a mode that promises to change nothing while calling something that does. The honest fix is a declaration from the server side, which is a decision of its own.
- **`memory` is withheld** though it only proposes. A mode that changes nothing should not leave something behind to be decided.

**The prompt had to learn the same thing, and this is where a real bug was.** `longform` opens with "write it to a .md file yourself with write" and `fileEditing` and `batchWork` are about tools วางแผน does not have — no desk had ever withheld those, so nothing gated them. Each is now gated on the tool it is about, through `Desk.Carries`, and gated on *the tool* rather than on a stance name so a later stance withholding the same thing is covered by the line already there.

**And a trap worth recording, because the comment warning against it was written by the same hand that then fell into it.** `withStance` set `ToolLess` with `!s.AllowsTool("")` — a probe — under a comment explaining that "does this session carry anything at all" cannot be answered by a name-at-a-time filter. It worked while คู่คิด was the only narrowing stance. วางแผน answers from an allow-list, `""` is not in it, and วางแผน came out marked toolless: it lost the prompt layers it needs and read as a broken คู่คิด. `Stance.CarriesNothing()` now answers the question directly. **A probe is a derivation that happens to agree with the truth today.**

**Still owed by §106:** the transcript divider (§106.4) — now genuinely needed, because วางแผน refuses to write while looking exactly like a desk that could; the keyboard shortcut; and ประตูส่งไม้ itself, which now has the plan it was waiting for but not yet the door.

### 106.10 มุ่งเป้า, decided against (2026-08-14)

Owner, after the design was laid out: *"ไม่ทำดีกว่าตัดสินใจละไม่คุ้มหรอก"*

The axis ships with three positions, and the fourth is not deferred. It was considered in full and declined, which is a different thing from being next on a list.

**The owner found the objection himself, before the design did.** Asked what the mode would do, he described it and then said: *"โดยปกติมันก็ทำแบบนั้น"* — normally it does that anyway. That sentence is §106.3's own test applied by the person who would pay for the build. A stance has to change what the engine does; "it tries harder for longer" is a paragraph of prose, and prose belongs in a prompt.

There was one real mechanism buried in it, and it is worth writing down because it is the thing that would have to be true if this is ever reopened. **Today a turn ends when the model decides it is done.** มุ่งเป้า would have moved that decision: the turn ends when the goal is met or a ceiling is hit. That is a genuine engine change, and it needs four parts, none of them optional: a goal pinned so a thirty-step run cannot drift off it, a finish condition that can be checked rather than felt, a checker that is not the model that just declared victory, and a hard ceiling on steps and tokens.

Four parts, and a shape unlike the two that shipped. คู่คิด and วางแผน are filters: a few lines in `Carries`, subtract-only, cheap to reason about and cheap to run. This one filters nothing and adds a control loop in the turn executor, with a verification call every round. Same axis from where the user sits, an entirely different piece of machinery underneath, and a cost that multiplies rather than adds.

**And the pain it was reaching for is not actually a stance problem.** A long unattended run is unpleasant today because of approval cards, forty of them, and the fix for that is the user's own ระดับการอนุมัติ dial. It is emphatically *not* a stance quietly skipping the gate: that would break COMPANY.md §6.2, which is the line this whole axis was built to respect.

Recorded rather than dropped silently, so the next person who has this idea reads what it costs before they start.

### 106.11 วางแผน had no output shape, and that is why it did not look like planning (2026-08-14)

Owner, after turning the dial and getting prose back: *"ทำไมตอนวางแผนมันไม่วางแผนอ่ะครับ"*

The stance shipped as a filter plus a paragraph of direction, and the paragraph said what to **mention** — go and look first, name the actual files, say what you are unsure of — while never saying what to **produce**. So the turn came back as prose, and nothing distinguished it from an answer given in ลงมือ.

**Nothing else in the prompt was going to fill that in, and it is worth being precise about why.** No layer in `internal/prompt` has ever specified the shape of an answer. The nearest thing, `longform`, is about where a long answer *lands* — "write it to a .md file yourself with `write`" — and it names *a plan* among its examples of exactly that. วางแผน does not even receive that layer, because §106.9 gated it on `write` and วางแผน has no `write`. Correct on the tool half, and it was never carrying a shape to lose.

So this is not a gap in all three stances. **ลงมือ** withholds nothing and needs no shape of its own. **คู่คิด** loses four layers to `ToolLess`, but its own direction replaces them with three concrete demands — the answer, the confidence without having checked, what would settle it. วางแผน is the only stance whose entire output is one document, so it is the only one that had to say what the document is.

**The shape already existed in this repository, one package away.** `profiles/subagents/plan.md` has pinned four headings since it was written: *What is there now / What to change / What could go wrong / What you are unsure of*. It was unreachable from the stance twice over — `internal/subagent` imports `internal/mode`, so the dependency cannot run the other way, and the stance withholds `task` anyway, so the profile that knows the shape is precisely what วางแผน is not allowed to call.

**One shape, two glosses, and a test for the seam.** `mode.planShape` is now the single list and `PlanHeadings()` reports it. The wording *under* each heading still differs between the two on purpose: the sub-agent is a coding helper and can say "the file and line you read it in", while a stance runs at every desk and cannot assume the work has files at all. What must not differ is the headings — and a Go list cannot reach inside a markdown file, so `TestThePlanProfileKeepsTheSharedPlanShape` reads the embedded profile and checks it still asks for every one, the same way the release checklist pins six files with a test rather than with a habit.

The escape hatch is stated as the shape's own justification rather than as an exception to it: a small job gets single-line headings, not a dropped shape, **because being able to read every plan the same way is what the dial was turned for**.

### 106.12 The plan as a drawn object (2026-08-14)

Owner, with a reference screenshot from another tool: *"ผมอยากได้เป็นกล่องประมาณนี้อ่ะครับ"*

The headings had to come first — there is nothing to draw a card around until a plan reliably has a shape — and with §106.11 in place the card is the second half.

**The contract is a fenced block tagged `plan`, because the renderer already had that seam.** `markdown.ts` intercepts a fence on its language and builds chrome around it; that is how a code block gets its header bar, its copy button and its Run button. A plan is the same move with a different box, so the card cost a branch rather than a parser. The alternative — detecting the four headings in any reply that happens to have them — was rejected twice over: it would put the shape in a second place, and it would wrap any message quoting a plan in a card.

**Where the instruction lives is the decision worth recording.** The wrapper is in `internal/prompt.planCard`, gated on `surface == SurfaceDesktop` *and* on the new `Desk.Planning`, next to `drawing()` and `panel()` — the two layers that were already gated on what the surface can render. The stance is not asked, because a stance cannot know: the same วางแผน runs in a terminal, where a fence is punctuation the user reads. So the shape is policy and travels everywhere; the box is rendering and stays where something can draw it. `Desk.Planning` is a bool rather than a stance name for the reason `Carries` is a function rather than a desk name — this package still does not import `internal/mode`.

**Three things the implementation had to get right, each of which fails silently:**

- **A fence inside the plan closes the plan.** The card then holds the first third of the answer with the rest spilled underneath as loose prose, with no error anywhere. `planCard` states it, and points at inline backticks as what to use for a filename instead.
- **The card's own icon is not a drawing.** `confine()` frames every top-level `<svg>` with copy and save buttons; without a guard the compass in the header got framed as a 14px drawing with its own toolbar. It is marked `data-chrome` where it is built rather than guessed at by size.
- **Copy has to hand back the markdown.** Read off the rendered card it would return the plan with every heading and bullet flattened away, so the source rides along on `data-plan`.

**Not collapsed behind a toggle**, which is where this deliberately parts company with the tool that inspired it. There a plan is an artifact beside the answer; here the whole turn was spent producing it, so the card *is* the reply, and folding it away would fold away the message. For the same reason it is a card inside the assistant's message rather than a bubble of its own — a sentence before the plan still reads before it.

Verified in both a dark and a light theme against the real stylesheets. It reads because `.msg.bot .bubble` has no background of its own (§ "The reply is not a card"): the plan on `--surface-raised` is the only raised thing in the reply.

**Still owed:** saving a plan to a file. Copy is one binding away from Save, and the drawing's `SaveDrawing` shows the shape of it, but a plan wants `.md` rather than a PNG and that is a decision rather than a rename.

### What this cost, and what it bought

**Cost:** one migration (v13), one intersection at an existing gate, one prompt layer, one control, one transcript divider — and the vocabulary sweep, **done on 2026-08-14 ahead of the build** so that no window exists where two documents answer the same question differently.

The sweep was estimated at 16 points in three files and came to roughly forty across eighteen, which is worth recording because the miss has a cause: `grep โหมด COMPANY.md` finds the *document's* uses and says nothing about the product's. The word was also in the CLI (`internal/app`, `internal/grammar`), in the onboarding text (`internal/model/noop.go`), in the `description:` line of all three desk manifests — which is screen text, rendered on the picker card — in four user-facing error strings, and in a frontend test asserting the old label. **Counting a vocabulary change by grepping the document that defines the vocabulary is counting the one place that cannot be wrong.**

What was deliberately left alone: shipped release notes and `BENCHMARK.md` (records of what was said at the time, not current text), the source-reading notes (another product's word), and two verbatim owner quotes — the §4 one, which already carries a note saying its words are dated, and the one in `shellSwitch.test.ts`. The rule from §4 holds: a quote is evidence, and evidence is not edited to match today's vocabulary.

Switching mid-session breaks the prompt cache once per switch; that is accepted, because it happens only on an explicit press.

**Buy:** คู่คิด removes ~9,321 tokens per request from conversations that never needed them (measured, §106.8). วางแผน delivers ประตูส่งไม้, which has been on the map unbuilt since §86. And the word โหมด stops meaning three things.

## 107. Decision — An Update Nobody Is Told About Is an Update Nobody Gets (2026-08-14)

Owner, 14 ส.ค., with a screenshot of VS Code's little "Updating Antigravity…" window attached: *"อยากให้มีประมาณนี้อ่ะครับ แบบกดอัปเดตได้เลยผ่านเน็ตไม่ใช่ต้องมากดติดตั้งใหม่ตลอด"*.

The awkward part of the answer: the app could already do it. `internal/update.Apply` has downloaded the release, checked its ed25519 signature, verified the hash against the signed `checksums.txt`, renamed the running exe aside and restarted into the new one since v0.9.4 — and the About page has had a one-click button for it just as long. The owner of the project asking to stop reinstalling by hand is the whole finding. The feature was complete and unreachable.

`desktop/app.go` said so in a comment, without noticing it was a bug report: *"Explicitly, from the button in Settings → About — nothing calls it on a timer yet."* Nothing did. The only way to learn a release existed was to suspect one and go looking.

### 107.1 The check runs itself, and stays quiet

`desktop/update_notify.go`: once, 20 seconds after startup, then daily for as long as the window is open. Three rules keep a question nobody asked from becoming a nuisance:

- **Silent unless there is something newer.** A failed automatic check emits nothing at all — offline, rate-limited, proxy in the way, all of it goes to `debuglog` and none of it to the window. A red banner about GitHub being unreachable, over an app the user is in the middle of using, is an answer to a question they never posed, and it makes a working app look broken. The explicit button in Settings still reports its failures; that one *was* asked for, and silence there reads as a dead click.
- **It downloads nothing.** The notice offers, the user decides, bytes move on the click. An app that quietly pulls tens of megabytes over someone's tethered connection has made a decision that was not its to make.
- **`AETOX_DISABLE_UPDATE_CHECK` switches it off before the first sleep**, not just at the request — an opted-out user should not have a goroutine ticking on their behalf either.

The startup check is deliberately unconditional rather than skipped when the cache says "asked recently": the previous run may have found a release the user closed the app on, and the ETag makes the repeat cost a 304 with no body.

### 107.2 The event carries the whole answer, not a boolean

`update:available` ships the entire `update.Status`. Which action a channel deserves — the download button for portable and installer, `scoop update aetox` for Scoop (that directory is not ours to write into), the release page for everything else — is a decision `internal/update` already makes and already has tests for. A frontend that re-derived it from `channel` would be a second place answering one question, free to drift the first time a channel is added.

### 107.3 Downloading and restarting are two acts, and only one of them is the user's to schedule

The first cut of this was one button that downloaded, verified, swapped and restarted — and it was wrong for a reason the owner spotted immediately, holding up Zed's updater as the shape he wanted: *"เนี้ยเหมือนตัวนี้อ่ะครับ"*. Zed downloads, then says **"v3.7.6 is ready"** with **Later** and **Restart to update**, and waits.

The two halves cost the user completely different things. Downloading costs bandwidth and can happen while they keep working. Restarting costs them whatever they were in the middle of. An act that did both spent the second without ever asking — it took the one decision that was actually theirs and made it a consequence of the one that wasn't.

So `internal/update.Apply` became `Stage` + `Staged.Restart`:

- **Stage** goes as far as it safely can while the window stays open. For the portable channel that is *everything*, swap included — `swapPortable` no longer relaunches, and after it the exe on disk is the new build while the process keeps running from the renamed `.old` file, which Windows is perfectly happy to do. For the installer channel it is the download and the verification only; an installer needs the app closed.
- **Restart** is the sentence the user times. It relaunches (portable) or hands the verified installer over (installer), and only it is gated on `turnBusy` — bytes coming down interrupt nothing, so `StageUpdate` deliberately is not on that list.

The consequence worth saying out loud on the card: **ไฟล์ลงเครื่องแล้ว — กดรีสตาร์ทตอนนี้ หรือปิดแอปตามปกติแล้วเปิดใหม่ทีหลังก็ได้เหมือนกัน**. "Later" here is not a postponed install. The install already happened; only the restart is outstanding, and closing the app tonight collects it. Without that sentence, "Later" reads as "not yet", and the user keeps a dialog on their conscience for nothing.

`Staged` lives on `App`, not in the package: it is one running app's state. `StagedUpdate()` exists so a webview reload — a Vite HMR cycle, or the frontend alone dying — does not lose it and offer the same 50 MB a second time for a build already on disk.

### 107.4 Two doors, one act, one state

The card and the About button start the same update, so "downloading / 53.2MB of 53.4MB / ready / here is why it failed" cannot live inside either page. About used to own it privately; that was fine while it was the only door and wrong the moment it stopped being. It moved to `frontend/src/lib/selfUpdate.svelte.ts`, which both read and both drive — the same rule as everywhere else in this repo: a second place answering the same question is หนี้ในระบบ.

That module is named `selfUpdate` and not `updater` for a reason worth writing down: on Windows's case-insensitive filesystem, `import … from './updater.svelte'` resolves to `Updater.svelte` — the component — and hands back a module with none of the expected exports. It fails at runtime as `undefined`, not at resolution, so the error surfaces nowhere near its cause. A `.svelte.ts` store must never differ from a `.svelte` component by case alone.

### 107.5 A card, not a dialog — and what it says

Nothing here interrupts the app, so nothing here takes the screen. The first cut put a scrim over the window during the download; with the act split, that scrim was claiming an urgency the feature does not have. It is a corner card through all four phases, and the × hides what it is *currently* saying rather than the card forever — a download that finished or failed behind a closed card is still news the user has to get, so any change of phase (or a newer release than the one waved off) brings it back.

Three details it inherits from the shape rather than from taste:

- **Bytes, not a percentage**, when the size is known: `53.2MB / 53.4MB` says how much is left in the unit the user's connection is in. When the server sends no `Content-Length` the bar moves without claiming a number and no size is printed at all — a fake 50% is not the humble option.
- **The release date beside the version.** "v0.9.7" alone says nothing about whether this is an hour old or a month old, which is most of what someone deciding about a restart wants to know. `Status.PublishedAt` was added for exactly this line.
- **On failure, "เวอร์ชันที่ใช้อยู่ยังอยู่ครบและใช้งานได้ตามปกติ".** This is the case the signature and the checksums exist for, and a refused download is the chain working, not the app breaking. `swapPortable` puts the old exe back if the second rename fails, so the promise is true. A restart refused mid-turn returns to **ready**, never to a fresh download: the build is still staged, and offering to fetch it again would be a lie about where things stand.

### What this does not decide

Update-on-quit (installing during shutdown so no one waits at all) is not built — with the swap already done by Stage, the remaining gap is just the relaunch, so this is now a small step rather than a design. Neither is a release-notes view on the card; the link to the release page is the whole of "what changed" for now. macOS and Linux still have no auto path at all — `canAuto` is `false` off Windows, and closing that is part of the 1.0.0 three-platform gate, not this change.

## 108. Decision — A Provider Row Must Not Promise What the Endpoint Never Sends (2026-08-14)

Owner, 14 ส.ค., testing every provider in turn: *"ฝากเทสทุกโพไวเดอร์ทีครับ"* — then, on the Gemini card: *"ยังไม่รู้ลิมิต คุยสักครั้งแล้วจะขึ้น แก้ได้ไหม"*.

Three separate findings came out of one afternoon of live testing. They share a root: **the catalog recorded intentions, and the UI read them as facts.**

### 108.1 Six providers were finished and invisible

`desktopProviders` in `desktop/app.go` is an allowlist, and `desktop/providers_test.go` already carried the confession in data — six entries reading `"never added; no reason on record"`. Groq, Mistral, Kimi, MiniMax, Together and Perplexity each had a runtime, a base URL, an env key and a brand mark, and no way to reach any of them from the desktop.

Each was probed against its live endpoint with a deliberately invalid key. All six answered the auth wall: base URL, path and body shape correct, only a real key missing. That is the same standard §-Codex was held to ("proven up to metering") and it is enough to show a row, because the next thing that happens is the user pasting their own key.

**Cohere stays off.** It got the same 401, and the 401 proves less there: the catalog points at Cohere's native API rather than the OpenAI-compatible surface it is declared to speak, so the gateway rejecting an invalid key says nothing about whether the path underneath is the right one. A row that takes a key and fails on first use is worse than an absent row. It keeps its reason in `notOnTheDesktop` and waits for someone with a real key.

### 108.2 Gemini states no rate-limit window, and the card promised one

`quotaSource: QuotaOpenAIStd` was set on the theory that an OpenAI-compatible host probably sends the `x-ratelimit-*` family. Gemini does not. A real key against the live endpoint returns 200 with thirteen headers and not one of them is a rate-limit field.

The UI read that theory as a fact and printed *"ยังไม่รู้ลิมิต คุยสักครั้งแล้วจะขึ้น"* — a promise conditioned on chatting, which is exactly the thing that could never make it true. Gemini is now `QuotaNone`, measured, with the measurement and its date in the entry and a test pinning it.

The sentence itself was also wrong in general, not just for Gemini: every other `QuotaOpenAIStd` provider inherits the same optimism, and none of them are verified. It now says the limit appears after a turn **if the provider states one** — which is the honest shape of the claim, and costs nothing on the providers that do.

### 108.3 The card asked for a key and left finding it as homework

Every provider hides its key page somewhere different — `aistudio.google.com/apikey`, `console.groq.com/keys`, a tab query string deep inside Alibaba's console. The card asked for the key and said nothing about where it lives.

`apiKeyURL` joins the catalog entry, next to the base URL and the env keys, because it is a fact about the provider of exactly the same kind. Putting the table in TypeScript instead would have been a second place answering the same question, free to drift. `APIKeyURL` returns empty for a provider that takes no pasted key, so a sign-in row and a local runtime draw no link rather than a dead one, and a test walks the whole catalog to make sure no row asks for a key it cannot point at.

### What this does not decide

Whether the other eleven `QuotaOpenAIStd` providers actually send those headers is still unmeasured — the dummy-key probe returns 401 before rate-limit accounting on most hosts, so only a real key can answer it, one provider at a time. The reworded sentence is what makes that acceptable to ship: it no longer claims anything that a silent provider would disprove.

---

## 109. Decision — 1.0.0 Is the Windows Release, and the Bar Moved Rather Than Was Met (2026-08-15)

Owner, 15 ส.ค., cutting the build: *"มันคือ 1.0.0 ที่ผมภูมิใจ"*.

Four files said the same thing in four places — `README.md`, `README.th.md`, `ROADMAP.md`, `HANDOFF.md` — **1.0.0 ships all three platforms or it is not 1.0.0.** Windows shipped alone. So one of the two had to give, and the honest version of giving is to say which.

**The bar moved. It was not met.** That sentence is the whole entry. A criterion quietly reworded to fit what happened is worse than no criterion, because the next one is then also negotiable and nobody will say so out loud. Commit `b04e5f5` had already refused to do it by hand-wave — *"ถ้าเจ้าของตัดสินว่าวินโดวส์ออก 1.0.0 ได้ก่อน มันคือการแก้ Decision ที่ต้องบันทึก ไม่ใช่การแก้ถ้อยคำสี่ไฟล์"* — and this is that record.

### 109.1 Why holding the number back bought nothing

The three-platform rule was written to protect a promise: *"1.0.0 แปลว่าผมสัญญาว่าจะไม่ทำของคุณพัง"*, with a half-sentence added on 9 ส.ค.: *"และมันทำงานได้ไม่ว่าคุณอยู่บนอะไร"*.

The first half is true on Windows today. The second half is blocked on two things that a version number has no effect on: a browser pane that is still a stub on Linux and macOS, and `internal/atrest`, where DPAPI is Windows-only — which means secrets sit in plaintext on the other two, and that is a security blocker, not a packaging one. Neither arrives a day sooner because the Windows build is called 0.9.10.

So the promise splits along the line it was always two promises: **the number is stability, the launch is reach.** 1.0.0 goes out on Windows. The big launch — the one aimed at r/LocalLLaMA and Hacker News, whose audience is on Mac and Linux — still waits for all three, and `ROADMAP.md` now says that explicitly instead of using the version as a proxy for it.

### 109.2 What is not being claimed

**Three-platform support is not cancelled, downgraded, or made optional.** The criteria moved into a "รุ่นถัดไป" block in `ROADMAP.md` with the at-rest encryption item still marked as the blocker it is. `PLATFORM-SUPPORT.md` is untouched and remains the one place the port status is true.

**And the version number does not certify the numbers.** `BENCHMARK.md`'s own rule is that unmeasured figures may not appear in `README.md` or `docs/index.html`; its last full run was 2026-07-27 on v0.9.2, when the app was 33 MB. It is 41.9 MB now. Shipping 1.0.0 does not tick "ตัวเลขที่เผยแพร่ทุกตัวตรงกับของจริง" — that box is still open on the day of the build, and the roadmap now says so in writing rather than leaving an unticked box next to a released version for a reader to interpret.

## 110. Decision — A Sub-Agent's Loop Cap Is Removed, and Its Clock Is Told the Truth (2026-08-15)

Owner, 15 ส.ค., looking at a `@research` card in the transcript: *"ปลดล็อคเพดานทุกซับเอเจนเลยครับ เป็นค่าเริ่มต้นได้เลย แบบไม่ต้องมีเพดาน ผมสั่ง"* — and, in the same breath, *"เวลามันหยุดนิ่งอ่ะ ขณะที่ซับเอเจนยังรัน tool อยู่ มันควรจะเดินต่อ จะได้รู้ว่าซับเอเจนตัวนี้ใช้เวลาไปนานแค่ไหน"*.

Two things on one card, and they are the same card for a reason: it was showing a green tick and `8s` over a worker that was on its twenty-seventh tool call, under a ceiling that would have cut it off before it finished.

### 110.1 The ceiling was protecting a number, not the user

`defaultSteps` was 24, written down in §44.7 as one of the three things that make delegation cheaper rather than pricier: the main agent runs unbounded because the brakes are a human (the approval layer and Stop), and a delegate has no human attached, so it got a number instead.

What the number actually bought, once real jobs ran through it, was the opposite. A delegate that reached the ceiling did not stop spending — it stopped **mid-job**, and handed the parent a failure saying to split the work and start again. The same work, paid for twice, plus the wait. And the ceiling could only ever be guessed: how many rounds a job takes is not knowable from the brief, which is exactly the thing §44.7 assumed away.

**The brakes that end a runaway loop were never the counter, and all of them are still here:** the doom-loop guard stops a delegate repeating the same call with no progress; Stop reaches every delegate through `Delegations.StopAll`; and every call a delegate makes passes the same permission rules and the same approval card its parent's calls do. Removing the count removes a guess, not a guard.

So `defaultSteps = StepsUnlimited`, the eight shipped profiles lost their `steps:` lines, and `steps: 40` still means exactly 40 for anyone who wants one. The keyword survives the change because it is what a reader of the file sees — a profile that says `unlimited` keeps saying what it meant on a machine whose default might be different.

**Correction 2026-08-16 — the change went further than the instruction, in two places.** Owner, reading this section back: *"AI นั่นแหละมันปรับเยอะเกิน ผมแค่ให้ตั้ง ไม่มีจำกัด เป็นค่าเริ่มต้นเฉยๆ แท้ๆ ตอนแรก"*.

- **A test forbade what this section says is allowed.** `TestBundledProfilesAreUsable` was inverted from "every sub-agent is capped" to `MaxToolCalls() > 0 → error` — no shipped worker may carry a ceiling, ever. That is not the default changing, it is a ceiling becoming illegal, and it contradicts the sentence directly above it. **Removed**: the test now checks the mechanism is honest (`steps: N` runs at N, no `steps:` runs uncapped) and leaves the number to whoever owns the worker. The eight numbers stay off — 110.1's reasoning applies to them as much as to the default — but the door is open, which is what this section always claimed.
- **The step-cap test was left half-updated and started hanging instead of failing.** `TestALoopThatEndsItselfIsAnActionableFailure/step_cap_exhausted` drove `explore`, whose cap this change removed, with a provider that never repeats a call. `TestGeneralIsTheLooper` one function above was updated for §110; this one was not, so the loop it exists to bound became genuinely unbounded and `go test ./...` could not finish from the 1.0.0 commit onward — 25 minutes before the suite timeout, with no failure printed. Fixed by running a profile that asks for `steps: 3` (an agent home, the one place a person can still write one) plus a 30s deadline, so the next regression fails in seconds with a sentence rather than looking like a slow test.

**And one claim in 110.1 is narrower than it reads.** "The doom-loop guard stops a delegate repeating the same call with no progress" is true, and it is the *only* remaining bound — it fires on **repetition**, so a delegate whose calls vary (a grep with a different pattern each round) has nothing stopping it but a human pressing Stop. That is the shape the hanging test simulated for 25 minutes. Left open rather than patched: re-introducing a step count is what this section rejected, so if it is ever closed it should be on *no progress* or a wall-clock/token budget, not on a counter under a new name.

**The settings page had to change with it, and that is the part that was nearly missed.** "จำกัดรอบทำงาน" read *"เว้นว่างไว้ = ใช้ค่าเริ่มต้น 24"* over files that say nothing about steps, and every agent row wore a `จำกัด 24 รอบ` badge that no longer described anything. A default changed in Go and left standing in the UI is not a changed default; it is two answers to one question.

### 110.2 The clock: `task` finishes when the work *starts*

The delegation card in the transcript drew its state and its elapsed time from the `task` tool call's own row. `task` returns the instant the worker is spawned (`begin` hands back a handle and the parent goes off to do other work — §44.11), so that row closes a second or two in. By the row's own account **every delegation is finished from birth**, which is why the card sat there with a tick and a frozen `8s` while 27 tool rows piled up underneath it.

This was already known and already solved once: the background tray reads the engine's register precisely because "the event stream shows every delegation as finished from birth". The tray and the in-chat card are deliberately the same card (§105.5) — so having them tell two different stories was the bug.

The fix is the same join, made in the same place:

- **`TaskInfo.ElapsedMs`** — the delegation's real duration, taken from the output the runner already stored, set only once it has stopped. While it runs there is no total to report; the clock is still going and the card counts from `Started`.
- **The row learns its delegation's id from the first step its worker runs.** Only the events from *inside* a delegate carry it — the `task` call completes before the register has a handle to give — and `parent` is the provider's call id, a different namespace.
- **`cardState`/`cardSecs`** ask the register; the row is the fallback for a turn read back from the database, where it is all there is.

**The first cut of this did not work, and the reason is worth keeping.** Reading the register is only half a fix — somebody has to *fetch* it. The poll was armed by a background event or by the turn ending, which is correct for a tray that only ever showed work outliving its turn, and useless for a card drawn while the turn is still open: for the whole time a delegate worked inside its turn the list was empty, the card fell back to the row it was written to ignore, and the clock froze in exactly the same place. The poll now also starts the moment a delegate's first step arrives.

**And that produced the duplicate, which is the second thing worth keeping.** Once the register is polled during a turn, the tray draws a row for a delegation whose card is already on screen — *"แล้วจะแสดงซ้อนกันทำไมอ่ะ กุบอกให้เพิ่มแค่เวลาไง"*. The first filter checked the live step list only, and missed the commonest case there is: the parent is told to start a delegate and go do other work (§44.11), so by the time it collects, the card lives in the *previous* turn's message and the live list has been cleared. The tray now hides any delegation the transcript is drawing, wherever it is drawing it — except one parked on a question, because the tray row is the only one with somewhere to type the answer. **One delegation, one card.** The tray was the answer to a transcript card that could not say "alive"; now that it can, a second copy is not redundancy, it is two clocks to reconcile.

Three things came free with it, all of them the same bug wearing different clothes: a working delegate now stays in the live area instead of collapsing behind "เอเจน 1 ตัว" the moment it started, its steps stay open (they are the evidence it is alive), and the ticker arms while background work is running rather than only while a turn is in flight — a delegate outlives its turn, so the clock used to stop at exactly the moment the user starts watching it.

### What this does not decide

**It does not make delegation unbounded in cost.** `maxConcurrent` still caps delegates at four, depth is still 1, and the brief is still refused if it will not fit the child's context (§44.12). What was removed is a guess about length, not a limit on breadth.

**And it does not give the card a clock for a reloaded session.** A persisted `task` part keeps the spawn's duration, and the register is per-session; once the app restarts, an old delegation's card shows what the row recorded. Fixing that means writing the delegation's real duration into the part when it is collected, and no one has needed it yet.

## 111. Decision — The Windows Shell Is PowerShell, Because the Shell Is Chosen for the Model (2026-08-15)

Owner, 15 ส.ค., reading a timeline of red shell cards: *"ทำไม shell ชอบมีปัญหาครับ"* — then, once the cards separated into probes that were supposed to fail and one round genuinely lost to dialect: *"เราจะบังคับเป็นคำสั่งของพาเวอร์เชลเลยดีไหม ไม่ใช่ CMD แล้ว"*, and on the plan: *"เห็นด้วยครับ ทำตามแนวทางนั้นเลย"*.

**cmd.exe was never chosen; it was inherited.** "The Windows shell" had always meant cmd, so cmd is what `proc.ShellCommand` ran — but of the shells a model can be told to write for, cmd is the one its training has seen least. Naming the interpreter in the tool description (the rule shell.go already had) softened that and could not beat it: the first command of a session was still written from habit, and habit is POSIX or PowerShell. The conclusion is the same one OpenCode reached independently — its Windows order is pwsh, then powershell, then Git Bash, with cmd.exe a last resort, and it refuses `fish` and `nu` even as the user's login shell — **the shell behind an agent's shell tool is chosen for the model that writes into it, not for the machine's tradition.**

So `Native()` on Windows now resolves **pwsh (7+) first, Windows PowerShell 5.1 as the fallback that every Windows ships with**, and cmd.exe is unreachable. WSL backends are untouched, and the shell picker's vocabulary (`native` / `wsl:<distro>`) did not change meaning.

### 111.1 The name carries the dialect, and 5.1 gets one Note

§-old doctrine in shell.go read "the name, and nothing else — a table of equivalents would be a case list." That survives with one amendment: the backend may now hand the description a single **Note**, and the only one that exists is 5.1's — *no `&&`/`||`, chain as `cmd1; if ($?) { cmd2 }`* — because it is the one habit the name "PowerShell" actively invites and the shell then refuses. A fact the name lies about is not a case list. pwsh's Note is empty: there the name really is the whole instruction.

### 111.2 Three traps, each measured before the code believed it

**Exit codes.** `-Command` collapses a native command's exit code to 1 on both shells (`cmd /c exit 5` → 1, measured), and the obvious fix — appending `; exit $LASTEXITCODE` — turns a failed *cmdlet* into exit 0, because no native command ran and `exit $null` is exit 0. That is a failure reported as success, the one direction a wrapper must never be wrong in. The epilogue in shell_windows.go reads `$?` first and only then prefers the native code: git's 128 arrives as 128, a failed `Get-Item` as 1, and `cmd /c exit 5; echo after` as 0, which is exactly bash's `$?` semantics. It is joined with a newline so a trailing `# comment` cannot swallow it.

**Encoding.** Owner, mid-change: *"กรองเป็น UTF-8 ในตัวเลยได้ไหม"*. Spawned the way Aetox spawns shells — no console, output piped — **both** 5.1 and pwsh decode native output with the OEM codepage, so Thai text arrived as CP874 mojibake (measured byte-for-byte). A prologue sets `[Console]::OutputEncoding` and `$OutputEncoding` to UTF-8 inside a try/catch that can degrade but never fail the user's command.

**Encoding, the half the prologue cannot reach (2026-08-16).** The prologue only fixes the direction PowerShell owns — how it *decodes* what a child already wrote. The child picks its own output encoding, and a console-less Python picks the ANSI codepage (874 here), so an agent's calculus script printing `∞` died with `UnicodeEncodeError` and reached it as a bare `exit status 1`: it read that as "my maths is wrong" and rewrote correct work to get away from it (found in `shell-audit.log`, three runs, 16 ส.ค. 21:06–21:07). The quieter half is worse — CP874 *can* encode Thai, so a script printing Thai exits 0 with `���ͺ������` in the output, and mojibake that succeeds is a thing no agent can catch. `ShellCommand` now exports `PYTHONUTF8=1`, and only Python needs telling: node, go and rust already write UTF-8 on Windows, so this stays one variable rather than the start of a per-language table. Skipped whole when `PYTHONUTF8` or `PYTHONIOENCODING` is already set, because a default arriving later must not overwrite a setting the user made.

**The kill race.** `taskkill /T` reads the process tree once, then kills; a child born between that snapshot and its parent's death survives as an orphan. With cmd the window was academic. PowerShell spends whole seconds starting, so a Stop pressed mid-startup reliably orphaned the very command being stopped — found as a test's temp dir held open for 60 seconds by an orphaned ping. `KillOnCancel` now walks the tree itself (Toolhelp snapshot, root killed first) and walks it **twice**: an orphan's `ParentProcessID` still names its dead parent, so the second pass finds exactly the children the first one raced with — the thing taskkill cannot do.

### 111.3 What moves with the shell, said out loud

**User hooks now run in PowerShell.** By §-design a hook runs in the same backend as the shell tool (one setting, bootstrap.go), so a hook line written in cmd dialect breaks. That is the documented coupling doing its job, not a regression to paper over: the fix for such a hook is the same one the model made, write it in PowerShell.

**The risk vocabulary grew.** safety.go's high-risk list now recognises the PowerShell spellings of the acts it already stopped — `Remove-Item`, `Move-Item`, `Rename-Item`, `Clear-Content`, `Stop-Process`, `-Recurse`, `-Force` and the short aliases — because a model told its shell is PowerShell writes `Remove-Item` where it wrote `del`, and the gate has to recognise the act in the dialect it told the model to use.

**One dialect cost is accepted, not hidden:** PowerShell splits an unquoted dotted flag at the dot (`-test.run` arrives as `-test`), so such flags must be quoted — models writing for PowerShell already do, and the one place our own tests did not, they now do. And §32's measured "shell ~39ms per call, dominated by cmd.exe process creation" is stale in the direction of more: PowerShell processes cost more to start, and the number stays unmeasured until the next benchmark run rather than guessed here.

## 112. Decision — An Answer That Is Still Arriving Has to Look Like It, and Be Updated Rather Than Rebuilt (2026-08-15)

Owner, 15 ส.ค., watching a plan render: *"ตอนวาดแผน อนิเมชั่นพังครับ เพราะมันสตรีมอ่ะ ถ้าเป็นตอนอื่นก็น่าจะไม่ต่างกัน เช็คดี ๆ ครับ"*. The instinct was right and it was bigger than the animation.

### 112.1 The signal: a block that is still being written says so

A plan, a drawing, and a long fenced file all get their edge, their heading and their คัดลอก button from the first frame — before a third of the content exists. They read as finished, and a user who copies then gets a third of a plan. This is the same failure the delegation card had (§110.2), so it gets the same answer: the running beam, on the block still being produced.

Which block that is, is not guessed. A drawing is unfinished while its `<svg>` has no `</svg>`; a fence is open while the count of fence lines is odd. Only the **last** match is marked — three code blocks in one answer must not all pulse because a fourth is arriving — and a finished message renders through a path that does none of this, so nothing can be left glowing by a turn that ended.

### 112.2 The clock cannot live on the thing that keeps being replaced

A streamed reply was drawn with `{@html}`, which assigns `innerHTML`. That **destroys every element and builds new ones, on every token.** An animation declared on the ring restarts with the element carrying it, so the light did not travel — it twitched in place, sixty times a second. No duration or easing fixes that: the element is new, so its clock is new.

The beam's own fix is structural. The phase is animated on an ancestor the stream does not touch (`.markdown-body`, and the delegation card), the registered property inherits, and a ring built this frame reads a phase already in progress. `:has(.live)` keeps the clock off every finished message in the transcript.

### 112.3 But the rebuild was the real bug, and it was not only about a beam

The beam could be worked around. Everything else inside a streamed answer could not:

- **An animation the model wrote itself** — newly possible, since `@keyframes` now survives the drawing scoper — restarted every token and never got past its first frame.
- **A text selection** inside the reply was dropped mid-drag.
- Anything with state of its own (an open `<details>`, a loaded `<img>`) was rebuilt from scratch.

So the streamed answer is now **morphed**: the new markup is reconciled against the old node by node, same kind of node in the same place is kept and updated, the tail is trimmed or extended. Deliberately not a virtual DOM — no keys, no move detection — because the input is one document growing at its end, not a list being reordered. The guarantee it owes is narrow: *an element that is still the same element next frame is the same element in the DOM.*

**Half a fix would have looked like a whole one.** A drawing's scope was a fingerprint of its entire markup, so while it streamed, every token renamed its ids and its animation — and a `<style>` rewritten each frame restarts the animations it names, morph or no morph. The scope is now taken from the opening tag plus the drawing's position: settled before the first shape is drawn, and still distinct between two drawings that open identically. Both halves are needed, and the test that pins them compares **node identity**, not markup, because that is what actually broke.

### What this does not decide

**The finished bubble still uses `{@html}`.** It renders once and has nothing to keep; morphing it would be machinery for no property.

**It is not a general renderer.** Two nodes that swap places are replaced rather than moved, which is correct output and lost identity. Markdown re-parsing does not reorder blocks as text is appended to the end, so the case is theoretical until something makes it real.

---

## 113. Decision — A Window Is Named by the Length It States, Not by the Slot It Arrived In (2026-08-16)

Owner, 16 ส.ค., holding a screenshot of his own quota row: *"อันนี้ของจริงหรือปลอมกันว่ะเนี้ย"*. The row read **"5 ชั่วโมงนี้ · เหลือ 6% · รีเซ็ตอีก 26 วัน"**, and under it **"สัปดาห์นี้ · เหลือ 100%"**. The first reading back called the numbers real and the pairing a bug. The owner's second message was the correct one: *"มันปลอมครับไปเช็คดี ๆ Codex ไม่มีลิมิตรายสัปดาห์นานแล้วครับ"*.

Both halves of that row came from somewhere, and only one of them came from OpenAI.

### 113.1 The name was a constant wearing a reading's clothes

`readCodexQuotas` opened with a table: `{"primary", "5h"}, {"secondary", "week"}`. The percentage was read off the wire. The **name printed beside it was written in this repo**, from what the ChatGPT plans looked like the day the parser was written. When those plans changed the rows kept their old captions, and no test could fail, because the tests asserted the constant against itself.

The doc comment three functions up had already said what should have happened — *"It comes from the provider's own dialect rather than a constant, because the windows genuinely differ"* — with the reasoning spelled out in full. It described a version of the code that was never written. A comment that argues against the function below it is worse than no comment: it is the reviewer's own objection, pre-answered.

This is §108's root one layer further down. There it was *"the catalog recorded intentions, and the UI read them as facts."* Here the **parser** recorded them, which is harder to catch, because a parser is where facts are supposed to enter the program.

### 113.2 The field that could have caught it was being spent on something else

The backend states `x-codex-<slot>-window-minutes`: the window's own length, the one field that says what the row is measuring. It was being read as *how long until the window refills* — the `else` branch of the reset lookup, under a comment claiming *"two spellings seen in the wild for the same fact"*. They are not one fact. A window's length and its time remaining are equal exactly once, at the instant it resets.

So the evidence needed to notice the drift arrived on every single turn and was spent misfiled. The name now comes from that length (`windowName`, ±10%: minute, hour, 5h, day, week, month), and anything else — including a backend that states no length at all — is `other`, drawn as *ลิมิตช่วงนี้*. Vaguer than "this week", and incapable of being wrong.

### 113.3 An impossible pair is published by halves

"Five hours" and "26 days" cannot both describe one window, and from inside the parser there is no way to tell which half went stale. So neither is guessed at: the percentage is kept, the reset is dropped, and the row draws the shape it already draws for a provider that sent no reset at all.

That dropped pair is the one event in this family worth a `debuglog` line. The headers are undocumented, their values are recorded nowhere, and the only evidence available while this was being diagnosed was a photograph of the row they produced. Lengths and durations only — nothing in `x-codex-*` identifies an account.

### 113.4 The same guess, twice more, and one of them stays

- **Anthropic's unified window** was labelled `"week"`. That family states a remaining count, a limit and a reset instant, and never a length, so the label was a guess about a plan rather than a reading of anything. It is `plan` now — *ลิมิตของแผน* — which is what the headers can actually support.
- **The OpenAI-compatible dialect** calls every window `"minute"`. The minute is OpenAI's own, inherited by the eleven hosts that copied the header names, several of which meter by the day. Nothing available here can *confirm* a minute. Exactly one thing can **disprove** it: a window still refilling more than ninety seconds out was never a minute long, and that case now says `other`. The rest keep the assumption they have always had, unproven, exactly as §108 left it. This pass narrows a claim; it does not get to invent a measurement.

### What this does not decide

**Whether the weekly row should exist at all.** If the backend still sends `x-codex-secondary-*`, Aetox still draws that row, now under whatever name its stated length earns. Reporting what the endpoint states is the rule, and a row deleted because we decided the provider is wrong about its own limits is the same mistake pointed the other way. If OpenAI has stopped sending the family, the row disappears on its own — `TestCodexQuotasVanishWhenHeadersDo` has pinned that since the feature shipped.

**What the live headers actually say.** Still unmeasured. The values have never been recorded, and this pass adds one log line for one case rather than logging the family on every turn.

---

## 114. Decision — An Agent Is Told What Is True, Not What to Do About It (2026-08-16)

Owner, 16 ส.ค., setting the direction: *"เอเจนในมุมของผม คือ ตัวตนนึง ที่คิดตัดสินใจได้ ตามหน้าที่ของตน ไม่ใช่เครื่องมือสร้างของที่ทำอย่างอื่นไม่ได้"* — and, on whether the system should let an agent negotiate how it wants work handed to it: *"พวกนี้คือพฤติกรรมเชิงผลลัพธ์ไม่ใช่พรอมป์ตที่ควรจะไปเขียนบอกมัน แต่ถ้าเราไม่กำหนด มันจะตัดสินใจเองและนั่นคือเอเจนที่ดูเป็นพนักงานมากขึ้น"*.

Leaving the call undetermined is not an omission. It is the feature.

### 114.1 The layer the audit got wrong, and why it is recorded here

The first sweep for this debt reported the six bundled `STARTERS.md` files — 96 cards, 98KB of imperative text against 38KB of `AGENT.md` — as the bulk of it, on the reasoning that a card's prompt half restates the profile as an order and, arriving as the first message, outranks it.

The owner refused the finding before any of it was acted on: *"การ์ดพวกนั้นเป็นหนี้จริงหรอ มันอยู่คนละชั้นไม่ใช่หรอ"*. He was right, and the mechanism settles it: `pickStarter` sets `draft` and nothing else, so a card's text lands in the composer where the user reads it, edits it, and sends it — or does not. Starters never reach `PromptFor`. **A card saying "ถามฉันก่อนให้ครบ" is the user asking, and an agent that does as it is asked is the behaviour we want, not the debt.** The two layers are the identity we write and the request the user sends, and only the first is ours.

This is written down because the finding was wrong in a way that reads convincing — a size comparison, a real quotation, a plausible override story — and the same argument will assemble itself again.

### 114.2 The one place that was legislating

`needsNotice` folds into every agent's prompt through `PromptFor`, which makes it the identity layer without ever looking like it. It carried three sentences of choreography: ask in one line naming where the thing is switched on, then do the part that does not need it, and never refuse work you could actually do.

Its own history is the argument against it. The first version said *"say what is missing, then stop"* and turned a specialist into a door nobody could get through. The second was written as the cure and was the same medicine — a rule that happened to point the better way. Neither version tried stating the fact and leaving the move to the agent.

So it now carries what an agent has no way of working out for itself: which need is unmet, why, and where the user switches it on — that last part already lived in `reasonText`, so dropping the instruction lost no information. What survives is one line that is a standard rather than a move: a result that needed the missing thing, handed over as if it did not, is the failure nobody downstream can catch. Same kind of thing as the research worker's *"never report what you did not read"*.

`TestTheNoticeCarriesTheFactAndNotTheMove` asserts the fact arrives, the standard survives, and — the half that matters — that `then stop`, `ask for it`, `does not need it` and `Refusing work` are all absent. The rule regrew once already by being rewritten as its own opposite; the guard is against both directions.

### 114.3 What the measurement showed, and what it did not

The claim "the removal changes what an agent does" was unmeasured when the change was made, so it was measured afterwards: the same agent, same question, same model, carrying the old notice and the new one (`live_notice_test.go`, kept as a live test rather than a fixture). The run hit the account's quota after 8 of 36 calls, leaving exactly one fully paired cell — the bundled github agent asked for something it genuinely could not do, three replies per arm.

**The behaviour changed.** All three new-notice replies handed the user a runnable `gh pr create` and closed on a single named blocker; none of the three old-notice replies offered any route that did not run back through the assistant. Three correlated markers moving together across a 3–0 split is a real difference, not a draw of the dice.

**Which one is better is unestablished, and this section exists to stop the record implying otherwise.** A blind four-lens panel scored it 3–1 for the new text, and its own sceptic dismantled the tally: there is one observable — the new text's replies carry an executable command — and the four lenses each scored that same observable, one of them with the sign reversed, because a concrete command necessarily contains concrete invented strings. One vote, not four, and its sign turns on whether you prefer action or restraint from an assistant that cannot act. With three samples on one question, it does not turn on the data.

Two things the run surfaced are worth more than the verdict. The new text's replies invent a PR title and body inside the command they hand over — milder in kind than what the old text produced in its worst sample, but pointed at the one standard §114 kept. And on the one 3–0 that went the other way, both arms carried the same two fix locations in `reasonText` and the new arm relayed only one of them: it was not missing the fact, it decided to be brief and dropped something true. That is not a defect of the change; it is what the change is, seen once, in the small.

### What this does not decide

**Nothing in the six `AGENT.md` files changes.** They are identity and craft, and they already grant the call: doc's *"Discuss a document without producing one when discussing is what was wanted; produce one the moment it is"* is the shape the rest of the system was fighting.

**`clarify()` stays as it is.** It already teaches a principle rather than cases, under the owner's constraint of 2026-08-04, and delegates receive it through `BuildPrompt`. The shelf for standing was never empty — it was being overwritten further down.

**Whether the bundled starter cards should be shorter.** They are not debt, so this is a product question about what a user meets on an empty chat, and it is open.

---

## 115. Decision — A Proposal Is Decided Where It Was Made (2026-08-16)

Owner, 16 ส.ค., holding a screenshot of another tool's chat with a one-line **"Saved a memory ›"** sitting between two paragraphs of the answer: *"ผมชอบรูปแบบนี้ Aetox ทำได้ไหม"*. Offered the cheap placement and the faithful one, he took the cheap one: *"เห็นด้วยครับ A ก็ดี"*.

What the screenshot is about is not where the row sits. It is that the thing happened where a person can see it.

### 115.1 The one tool whose work does not happen when it runs

`memory` queues a row in `pending_changes` and waits for a human (§82). Every other tool in the list has done its work by the time its row goes grey, so the timeline treats this one the same way and gets it exactly backwards: the call drew a row reading `memory` and nothing else — the tool's arguments are `op`/`text`/`old`/`why`, and `model.ArgSubjectKeys` names none of them, so the row had no subject to show — inside a panel collapsed behind *ใช้ N เครื่องมือ*. The sentence it wanted to remember, the reasoning it offered for it, and the yes or no it was waiting on all lived on the Settings page behind a badge.

The approval door was built so nothing changes the agent without a person allowing it. A door nobody is standing at does not hold that promise; it just never opens.

### 115.2 The card carries an id, not a copy

`skill.Output.ProposalID` rides out to the UI the way `Artifacts` does, through `ToolEvent` for the live turn and `ToolPart` for the written-down one, and the card in the chat reads its row back through `PendingChangeByID` — in whatever state it is in, which is why that binding is not restricted to pending rows.

The alternative was to carry the sentence itself in the transcript, and it fails the moment the two disagree. A transcript is frozen and the queue keeps moving: a proposal approved from Settings that evening would still be asking in a session reopened next week, and clicking it would produce *"this was already decided"*. Reading the queue by id, a decided row comes back saying which way it went. Only the id is written down, inside the existing parts column, so nothing migrates and older turns simply have none.

### 115.3 It asks; the tool it was copied from announces

"Saved a memory" is true where it was copied from. Here it would be a lie: at that moment nothing is saved, and whether anything ever is belongs to the user. So the card asks — *ขอจำเรื่องนี้ไว้* — carries the line and the reasoning, and offers อนุมัติ/ไม่เอา on the spot. Approving says *มีผลตั้งแต่แชทถัดไป ไม่ใช่แชทนี้*, because the opposite is what every reader assumes: memory is read at session start, so the conversation you approved it in is the one conversation it does not reach.

### What this does not decide

**Where the card sits.** Under the answer, in the row of things a turn hands back, beside the file cards — not inline at the point in the prose where the call happened, which is what the screenshot actually shows. Drawing the sequence inline was tried and read worse (the note at `stepsFromParts`): the same thinking segment appeared twice and the answer stopped being what your eye lands on. That rejection was about drawing *everything* inline, so the narrower version — a whitelist of parts worth drawing in the flow — is still open, and it is the same mechanism the mid-turn chart bug needs. One change, when it is done, not two.

**Whether a proposal should reach the running session.** It should not, and does not.

**The summarizer's own proposals.** Those enter the same queue from `summarize.go` rather than from a tool call in a conversation, so they have no answer to sit under and are still decided in Settings. That is the right place for them: they belong to no single turn.

---

## 116. Decision — Learning Crosses Projects; Deciding Does Not (2026-08-16)

Owner, 16 ส.ค., cutting through an answer of mine that had folded two different things into one: *"เราต้องแยกสิ ความจำ การเรียนรู้ เรามีอยู่แล้วไม่ใช่หรอเรียนรู้ข้ามโปรเจกต์อ่ะ ที่ผมอยากให้เพิ่มตอนแรก คือ จำการตัดสินใจตอนทำโปรเจกต์ไง ไม่งั้นผมจะพุ่งเป้ามาที่ตัวทำงานฝั่งโค้ดทำไม"*.

Four layers, and the third one did not exist:

| | written by | crosses projects |
|---|---|---|
| การเรียนรู้ — `MEMORY.md`, `modes/<desk>.md` | the agent, user-approved | yes, deliberately |
| **the decisions a project settled** | **the agent, user-approved** | **no** |
| project rules — `AETOX.md` | the user, by hand, in the repo | no |
| a delegate's memory | that delegate, user-approved | no |

### 116.1 Crossing is the feature, and that is why the missing row is not it

The question that opened this was whether a fact learned in one project bleeding into another is a bug. It is not: *"เครื่องนี้ไม่มี Excel"*, *"เชลล์ที่นี่คือ PowerShell"*, *"เจ้าของชอบคำตอบสั้น"* are true wherever the user is sitting, and a system that made them stop at a folder boundary would be a system with no memory at all. §82's design is right and does not change.

What has no home is the other kind entirely — *we settled on X here, and this is why*. The desk cannot hold it: โต๊ะโค้ด is the same desk in every repository, so a decision kept there arrives as advice in the next one. And `AETOX.md` is not it either, though the first answer to this claimed it was. That file is what the user tells the agent; this is what the work settled and the user approved. **Different author, different question** — this repository proves it by hand, keeping `docs/DECISIONS.md` beside its own `AETOX.md`.

### 116.2 What the tools we are measured against actually do

Asked to go and look rather than reason from the design, and the answer inverted the assumption the design was resting on.

- **Claude Code**, on the owner's own machine: the agent-written memory is stored **per project**, at `~/.claude/projects/<encoded-path>/memory/`. Two projects on this machine, two separate stores, and no global one at all — no `~/.claude/memory`, and not even a `~/.claude/CLAUDE.md`. Its cross-project layer is a different mechanism (a file the user writes) that this installation has never created.
- **opencode** has no agent-written memory at all. It has rules files — `AGENTS.md` in the project, `~/.config/opencode/AGENTS.md` globally, both written by a person — and everything self-writing is a third-party plugin.

So Aetox was the **mirror image** of the tool it is measured against: everything the agent writes was global, and the project side was human-only. Not a smaller version of the same design — the other half of it.

### 116.3 Under the data root, not in the repository

Both places were defensible and the owner chose the data root, which is also Claude Code's choice. Writing into somebody's repository means git noise nobody asked for, a file that needs the sandbox's write right to exist, and nothing at all for a project folder that is not a repository. What the team should see goes in `AETOX.md`, which is theirs and is written on purpose.

The key is `config.ProjectKey` — readable base name plus a hash of the path, the same key the store already files that project's history under. It moved out of `desktop/sessions.go` for that reason: two implementations of "what is this project called" would let a project's history and its memory disagree, silently, and the memory would simply never be read again. The readable half is flattened for the filename (`safeKey`), because folder names here have spaces in them and `validScope` refuses those — unsanitized, *"My App"* would be a project that could never have a memory and never say so.

### 116.4 Where it sits in the prompt is the precedence

Between the desk's memory and the project's rules. A desk is the same desk everywhere, so what one project settled must not outrank it there; and what the user wrote in `AETOX.md` outranks anything the agent concluded about the same code. Order is the whole mechanism, as it has been since §82 — no sentence in the prompt claims a ranking that could drift from it.

An unfocused session reads none of it. It is rooted at the machine, and a memory keyed to that folder would be one junk drawer every roaming session shared.

### What this does not decide

**Whether the agent should ever write to `AETOX.md` itself.** It should not, today. Proposing a line for the team's file is a different act from remembering, and it would need the repo's write right and a diff the user reads. The card can say "this belongs to the team" long before the agent can act on it.

**How a project's memory gets consolidated when it fills.** Each scope has its own 8KB ceiling, which is the point — a project's decisions never tax another project — but a long-running repository will reach it, and the answer today is the same refusal the other scopes give: merge or drop a line first.

**The summarizer.** It still proposes into the shared scope only. It reads `tool_runs`, which knows the session and therefore the project, so aiming its proposals is possible and unbuilt.

## 117. Decision — A Window Opens Inside the Screen It Was Given, Not the One It Assumed (2026-08-16)

Owner, 16 ส.ค., with a photograph of Aetox on a VivoBook: *"มันเปิดมาแล้ว หน้าต่างไม่พอดีจอ เห็นข้างบนมั้ย ไม่เห็นปุ่ม กา ขยาย หรือ ทับลงเลย เพราะอันนั้น เราใช้ของวินโด้ใช่ไหมล่ะ แต่ตัวโปรแกรมมันไม่รู้ไง"*.

The last sentence is the whole diagnosis. The title bar is Windows' — the minimise, maximise and close buttons belong to the OS, not to the app — and the app had never told itself anything about the screen those buttons have to land on.

### 117.1 Two constants and a centring, none of which asked

`desktop/main.go` opened the window at 1440x900 with a floor of 1100x700, and Wails centres it during `Run`. Three steps, no measurement between them.

A 1920x1080 laptop at Windows' default 150% scaling is **1280x720** in the units Wails sizes windows in, and its taskbar takes 48 of the height. So the window was built 180px taller than the display and then centred on it, which puts its top edge — and the OS's buttons — at **y = -114**. Nothing in the app was wrong on its own terms; it simply stated a size as a fact about the world.

### 117.2 The taskbar has to be in the measurement, not just the screen

Centring is done against the work area, so a window merely as tall as the *screen* is still pushed half a taskbar off the top. Fitting to `Screen.Size` alone would have moved the failure from 114px to 24px and called it fixed.

`SystemParametersInfoW(SPI_GETWORKAREA)` is what the desktop answers with, in physical pixels; the screen reports its own logical and physical size, and their ratio is the display's scaling. Converting through that ratio rather than through a DPI call keeps the number in the same units as everything else in the comparison. On a second monitor with a taskbar of its own it is an estimate, and a conservative one — it can only cost a few pixels of window.

### 117.3 Only ever shrinks, and the floor comes down with it

Two rules, both of them about not overreaching:

- **Never grows.** A monitor with room opens the window at the size the app asked for, not filled to the corners. The fix is for a screen that is too small, and it should be invisible everywhere else.
- **The minimum moves with the window, and no further.** `SetSize` clamps against the floor before it does anything else, so a 700px minimum under a 672px work area puts the window straight back where it was. The floor still exists for the reason it always did — the cockpit's three columns stop making sense below it — so it is lowered only as far as the screen forces, and a 1280px-wide display leaves the 1100px width floor untouched.

`fitToScreen` runs first thing in `App.startup`, which is after the window is created and centred and before the webview has content to show, so the correction happens where nobody can watch it move.

### What this does not decide

**Remembering where the user put the window.** Every launch still opens centred at a computed size; a window someone dragged to a second monitor comes back to the first. That is a separate feature and this is not a substitute for it.

**Anything about non-Windows.** `systemChrome` returns nothing there and says so — a dock or panel is asked for through that desktop's own API, and there is no build to ask from yet (§109). Fitting inside the screen still works; only the taskbar reserve is missing.

## 118. Decision — Mathematics Is Drawn, Because It Was Never Prose (2026-08-16)

Owner, 16 ส.ค., same session: *"ตอนเขียนโจทย์คณิตศาสตร์ แม่งพัง"*, with a screenshot of an answer about integration.

A model asked about an integral writes LaTeX, because that is what mathematics is written in. Nothing on this surface knew that, so markdown read the equation as prose and printed what was left of it.

### 118.1 It failed twice, and the second failure hid the first

`\[` is a backslash-escaped bracket to markdown, so a display equation arrived as a bare `[` on a line of its own. `x^2` and `a_1` handed their `^` and `_` to the emphasis parser. Whatever survived both was printed as source. The screenshot is exactly that: `\int_0^2 x^2\,dx` as text, between two orphaned brackets.

So a renderer bolted on after markdown would have been given mangled input to typeset. Equations are tokenized **before** markdown reads anything — a marked extension runs ahead of the built-in tokenizers, which is what keeps the escape rule and the emphasis rule off the LaTeX inside — and KaTeX draws them. The same seam the fenced-code and plan cards already use.

### 118.2 Four delimiters, because the surface does not get to choose

`\[...\]`, `$$...$$`, `\(...\)` and `$...$` all render. Models write all four and which one arrives is not a decision this end makes.

The single `$` is the one that needs guarding: it is also how money is written, and *"ราคา $5 และ $10"* is a sentence, not an equation named "5 และ ". Three conditions settle it — an equation does not open with a space, does not close with one, and its closing `$` is not the start of another price — and a `$` cannot appear inside a match, so a run of prices can never be joined into one.

The display pair are inline tokenizers as well as block ones, because a block tokenizer is only ever offered the start of a block, and `\[x\]` written mid-sentence happens.

### 118.3 What a refusal produces

KaTeX refuses what it cannot read, and the answer to that is the source, not a red error. A refusal means one of two things and both end the same way: the text between the delimiters was never mathematics, or it used a command KaTeX does not carry — and the reader is better served by what the model wrote than by an error message about it. Streaming is the other reason: half an equation is a syntax error for the few frames before its closing brace lands, and nothing should flash red on its way to being correct.

`strict` is off and `trust` stays off. They sound alike and are opposites: `strict` is LaTeX pedantry, and on it refuses a Thai character inside `\text{}`, which is most of what the equations in this app say. `trust` is the safety one — it is what would let `\href` and `\includegraphics` out of the equation and into the document.

### 118.4 Three collisions with what was already there

- **KaTeX draws in SVG.** A square root's overbar, a stretched brace, an arrow over a vector. `confine()` frames a drawing with คัดลอก and บันทึก buttons, so without an exemption a radical sign gets its own toolbar — and renumbers the real drawings around it.
- **DOMPurify half-removes the `<annotation>`.** KaTeX writes the equation twice: as the spans that are drawn, and as MathML for anything reading the page aloud, ending in an annotation holding the original LaTeX. The sanitizer deletes that tag and keeps its text, which is the worst of both — raw `\boxed{\text{...}}` loose inside the `<math>` element, out of sight but read aloud and picked up by a copy. It is dropped before the sanitizer sees it; the MathML itself stays, because it is the whole reason a screen reader can follow an equation whose visible half is `aria-hidden`.
- **KaTeX_Main has no Thai in it.** `\text{อนุพันธ์ = ดูความชัน}` is what an answer here actually contains. Chromium falls back on its own, to Windows' default face rather than the one the app is set in, so the app's stack is named after KaTeX's: Latin and digits keep KaTeX's metrics, Thai lands in Noto Sans Thai with the rest of the answer.

### 118.5 The prompt is told, in the layer that owns the question

`surfaceLayer` (§114 — the agent is told what is true, not what to do about it) already answers "where does what I write end up", for both surfaces. Mathematics joins the sentence, and the delimiters are named: a model that cannot tell whether they will be drawn has two ways to hedge, and both are worse than the equation — spelling the integral out in words, or reaching for unicode superscripts that run out at the first fraction. The terminal half gains the matching negative.

### 118.6 An equation is a thing you take away, and it needed the air to be one

Seen running, two things were wrong that reading the markup could not have shown (owner, 16 ส.ค.: *"มันชิดไปไหมอ่ะ ผมอยากให้มัน ห่างอีกนิด ... และ ทำให้มันคัดลอกได้หน่อยครับ"*).

**The spacing was the prose rhythm, and that is not what a display equation is.** The margin was `.7em`, which collapses against the `.85em` under a paragraph and leaves 13px — the same gap as the line above it *inside* a paragraph. So an equation set on its own line did not read as being on its own line; it read as a wrapped line that happened to be centred.

**And it could not be taken anywhere.** A code block has คัดลอก and a drawing has คัดลอก and บันทึก; the one thing on the surface a user is most likely to want in another program had neither.

Both were answered at once, and by the box rather than by a number. A display equation is drawn as a fenced code block is drawn — same border, same radius, same surface, same header bar with a label on the left and a hover-revealed คัดลอก on the right, same margins (owner, 16 ส.ค.: *"แสดงเหมือนตอนเจนโค้ดได้ไหม"*). It is the same kind of thing: notation rather than prose, written in a language of its own, read as a unit rather than a line at a time, and wanted somewhere else as often as it is wanted here. The first attempt was a bare centred equation with the button floating over the notation, which is what a control with nothing to hang off looks like.

Every value is `.codeblock`'s, taken deliberately rather than tuned to match: two boxes meant to read as the same kind of object must not be assembled from two sets of numbers that drift apart on the next change. The label says `latex` for the same reason a code block's says `python` — it is the language of the thing, and it is exactly what คัดลอก hands over. Untranslated, like the language tag, because it is a name and not a word.

The one place they are not the same: the notation stays centred inside the box where code is left-aligned, because that is how an equation is set everywhere it is set.

Three decisions inside that one:

- **It copies the LaTeX, not the rendering.** The source rides on `data-tex`, the way the plan card carries its markdown on `data-plan`. Reading it back off the DOM instead returns KaTeX's layout text — every fraction flattened, every limit and exponent run onto one line — which pastes into another tool as something that has to be retyped. The source pastes as the equation.
- **A `<span>`, not a `<div>`.** An equation written mid-paragraph renders inside a `<p>`, and a `<div>` there ends the paragraph early and drops the rest of the sentence out of it.
- **Inline maths gets none of it.** A box around the `x` in the middle of a sentence is not a box, it is an interruption.

### What this does not decide

**Chemistry, music, and anything else `mhchem`-shaped.** KaTeX ships extensions for some of it and none are loaded. The failure is the visible one — source instead of notation — not a silent wrong rendering.

**900KB of fonts nobody opens.** KaTeX ships each of its twenty faces three times, and its stylesheet names all three, so the build emitted sixty files. The engine is WebView2, which is Chromium, which has read woff2 since 2014. The stylesheet is rewritten at build time to name only woff2 — 292KB emitted instead of 1.2MB — because Vite emits an asset when something references it, so the reference is the thing that had to go.

---

## 119. Decision — The User Points, and What Travels Is the Element (2026-08-16)

**Status:** Implemented 2026-08-16 · **Files:** `desktop/browser_pick.go`, `desktop/browser.go`, `desktop/frontend/src/lib/workbench/pagePick.svelte.ts`, `desktop/frontend/src/lib/workbench/Workbench.svelte`

### 119.1 The sentence that could not be said

The agent could already read a page and act on one (§48, `browser_read`/`browser_click`). What it could not do was hear *"ปุ่มสีฟ้ามุมขวาบนอะ"* and know which node that was. Every question about a rendered thing had to be translated into prose first, and the translation is the part that loses: a colour and a corner are not an address, and the model's only recourse was to read the whole page back and guess.

So the direction of the bridge is reversed. The user points at the page, and what lands in the composer is the element — not a description of it.

### 119.2 The overlay is drawn inside the page, and that is not a style choice

A browser tab is a real OS window composited above the app's own webview. A highlight box drawn in Svelte would sit *behind* the thing it is trying to outline. So the whole overlay — the outline, the selector badge, the hint pill — ships inside the injected script and is painted in someone else's document.

Which creates the debt this project names most often: a second palette and a second copy of every string, living where nobody looking at `style.css` or `locales/th.ts` would ever find them. The answer is that neither is allowed to live there. The frontend reads the live theme off `getComputedStyle(document.documentElement)` and the wording out of the locale, and hands both to Go as `opts`, which embeds it as a JS string literal for the page to `JSON.parse`. Go never parses it and never composes it. The overlay is drawn by a stranger, from our paint.

### 119.3 A pick is not a ref

`browser_read` hands out refs, and a ref answers "which node do I click". A pick answers "which node is this **in my source**", and the answer to that is almost never the tag name. It is the class list, the box, the ancestors, and above all the colours the element actually renders in: a rendered `#185fa5` greps straight to the token that produced it, where `<button>` greps to nothing.

Three consequences the shape had to carry:

- **Drag takes a region, not a picture.** Circling three cards yields the three cards — every element the box *fully encloses*, outermost first, capped at twelve. Without the outermost rule the answer to "these three" is ninety nodes.
- **Shift keeps the mode open.** "สามอันนี้ต้องเรียงให้เท่ากัน" is a sentence about a set, and a mode that ends on the first click cannot express a set.
- **The chip is a `pendingContext`, not a new mechanism.** It says `kind: 'pick'` so the attachment line can say what it is, and everything else — the card, the preview, the re-send on edit — was already built.

### 119.4 Origin is checked before the token is claimed, and the order is the whole point

The pick rides the same bridge every page can call, so it carries the same two proofs a `text` message does: the sending frame's real origin must match the URL claimed, and the token must be the one *this* `BrowserStartPick` minted. Without the token, any page could push an element into the user's composer unasked.

The order is not incidental. Claim first and a message from the wrong origin *consumes* the token the real page is still going to answer with — so any frame on the page could end a pick the user is halfway through, silently, by being wrong. Origin first, then claim.

### 119.5 The pencil, added the same day, because pointing at a node is not pointing at a place

Shipped an hour later, on being used (owner, 16 ส.ค.: *"มันชี้ได้แล้วครับ แต่มันวาดไม่ได้อ่ะ เผื่อผู้จะพยายามพูดถึงบริเวรล่ะครับ"*). Every affordance above answers "**which element**". None of them answers "**this bit here**" — the gap between two cards, a margin that is wrong, an area with nothing in it. A region can be described by what it contains only while it contains something.

So the answer had to become a picture, and that took the third patch to the vendored WebView2 (`CapturePreview`, see its AETOX-PATCH.md). The alternative — a screen grab of the tab's window — was rejected for one reason: it photographs whatever floats above the window, so it is a lie about the page exactly when something overlaps it.

The order is the part worth writing down, because every obvious arrangement is wrong:

1. The user draws. The ink is a `<canvas>` **inside the page**, for the same reason the highlight is (§119.2).
2. On finish the script removes **its own controls and keeps the ink**, then answers. What is on screen at that moment is the page as the user marked it, and nothing else.
3. The app photographs it from outside — a still life, since nothing is animating.
4. Only then is the ink taken down.

Tear the ink down with the controls in step 2 and step 3 photographs a page with nothing on it. That is why finishing does not go through the same path as cancelling, and why the mode stays half-alive between the answer and the picture.

Step 3 has a rule of its own, learned by shipping it wrong (`hr=0x802a000c`, on the first real drawing). WebView2 is apartment-threaded, so the capture must be **asked for on the thread that owns the webview and awaited off it** — and both halves are load-bearing. The first version queued the ask onto that thread and then, inside the queued command, handed the actual COM call to a goroutine; the engine refused it, and the refusal reads like the engine saying no rather than like a bug in the call site. Hence the channel-of-a-channel in `BrowserCapturePNG`: the queued command hands back *where* the answer will arrive, and the waiting happens on the caller's goroutine, because the pump that delivers the answer is the very thing the webview thread is running.

The same rule shapes the completion handler: it reads the stream and releases it **inside the callback**, on the thread that created it, rather than on whichever goroutine was waiting. And the handler is held in a package-level map for its lifetime — it is a Go object handed to WebView2 as a bare pointer, invisible to the collector, and a capture that outlives one collection would otherwise be a pointer into freed memory, crashing in the browser process minutes later and nowhere near the file that caused it.

The drawing also carries **what was under the pen** — sampled with `elementFromPoint` along each stroke, not derived from a bounding box, because a box around a circle is mostly the things the circle avoided. The picture is the point; the elements ride along.

A failed capture attaches the elements anyway. A drawing that could not be photographed is still a question about something.

### 119.6 The bar owns its colours, and says what is true rather than what to do

Seen running, the controls were there and nobody could find them (owner, 16 ส.ค.: *"ยังไม่เด่นพอ หรือมองยากไป"*). Three options were drawn; the largest won — 14px, a leading icon, a heavier shadow. But the size was never the real fault, and two things underneath it were.

**The bar was wearing the app's clothes on somebody else's page.** Its background, text and border came from the theme, which is correct for anything drawn in the app and wrong for the one thing that is not: this bar floats over a document whose background nobody knows. Under a light theme it was a pale box on a white page — perfectly styled and invisible. It now carries its own dark palette outright. Only the accent still travels, because the accent is the ink, and the button that ends a drawing should be the colour of what was drawn.

**And it repeated an instruction instead of reporting a state.** A hint is for before you know what to do; once there is ink on the page the thing worth saying is that there is something to send. So the label stops instructing and starts counting, and at that same moment the ปุ่มเสร็จ fills with the ink colour and takes a halo (owner's own idea, 16 ส.ค.: *"ตอนวาดแล้ว ให้ ปุ่มเสร็จแล้วมันเรืองแสงด้วย"*). Before that it is quiet and half-faded, because pressing it would send nothing. One fact, said once, in two places.

The icons are built with `createElementNS`, not `innerHTML`. A page carrying a Trusted Types policy throws on an `innerHTML` assignment — and those are exactly the pages worth marking up.

### 119.7 What this does not decide

**Letting the agent ask by pointing.** `ask_user` plus these modes is a question answered with a click rather than a sentence, and the plumbing for both halves now exists. It is not wired.

**The agent seeing pages on its own.** `BrowserCapturePNG` is the capability the vision path has been missing — a page that is a canvas, or one whose layout is wrong, is invisible to `browser_read`. It is a user-driven binding today and no tool action.

**Cross-origin iframes.** The script reaches the top frame only. Pointing inside an embedded frame highlights the frame; drawing over one draws over it, and the elements sampled underneath stop at its boundary.

## 120. Decision — A Run Button Is Offered Because the Machine Can, Not Because the Renderer Remembers (2026-08-16)

Owner, 16 ส.ค., after finding out the button was on Python and shells and nothing else: *"คิดว่าภาษาอะไรอยากให้รันผ่านหน้าแชทได้ครับ"*, then *"หากในเครื่องไม่เจอภาษานั้น ๆ ไม่ต้องแสดงปุ่มรันนะ"*. The second sentence is the decision; the list is a consequence of it.

### 120.1 The answer lived in two places and neither of them was the machine

`runnableShells` and `runnableScripts` in markdown.ts decided whether the button was drawn. `scriptLangs` in run_script.go decided whether anything happened when it was clicked. Adding a language meant editing both, in two languages, and the only thing keeping them in step was somebody remembering — the same debt as any second place that answers one question.

Worse, neither of them asked the machine. A `python` block is a program on a computer with Python on it and a picture of a program on one without, and both files said "runnable" either way.

**And the machine's answer was being read wrong even when it was asked.** `exec.LookPath("python")` returns a path on a Windows with no Python at all: a clean install ships Microsoft Store *app execution aliases* for `python.exe` and `python3.exe` on PATH, and they are zero-byte reparse points that print an advert for the Store and exit 9009. Measured 2026-08-16: `os.Stat` reports size 0 and an irregular mode for both, where every real interpreter is a regular file with a size. So the honest message run_script.go had ready — *"python is not installed, or not on PATH"* — was nearly unreachable, and what the user got instead was an advert. The test is `IsRegular() && Size() > 0`, which names no Windows path and checks no GOOS on purpose: a zero-byte program is not a program anywhere.

### 120.2 One module owns the question

`internal/runlang` holds the table, asks the machine, and is asked by both ends. `App.RunnableLanguages()` answers `tag → kind` once at startup; the renderer draws a button where the answer has one and nothing where it does not; `RunChatScript` looks the same table up to find what runs the file. Adding a language is one entry and nothing else.

Asked once rather than per render, because what is installed does not change while a window is open and the alternative is a bridge round trip per code block per redraw. A machine that gains Python mid-session gets its buttons on the next launch, which is the deal every other "what is on this computer" answer in the app already makes. An engine that cannot answer leaves the table empty, and empty means no buttons — the safe direction for the one control in the chat with a real side effect.

### 120.3 What earns an entry

Three rules, and every rejection failed one:

1. **One command runs one file.** No build step, no project scaffolding, no artifact left in somebody's folder.
2. **The block is a program.** `json`, `yaml`, `html`, `css`, `sql` and `md` are not here and never will be; running markup means nothing. `html` would mean "open a browser", which is a different act and deserves its own button rather than a quiet second meaning for this one.
3. **Failure is honest.** A tag whose interpreter is missing simply gets no button, so the table can carry a language nobody here has installed without lying.

Added: **JavaScript** (`js`, `javascript`, `node`) — node writes UTF-8 on Windows whatever the codepage says, where Python had to be told (§111.2), and its exit codes arrive intact. `.js` rather than `.mjs` so a snippet inherits the module kind of the project it is written into; `mjs` is its own entry for when the model means ESM regardless. **Go** (`go`) — the entry that proves an interpreter can need arguments of its own, because `go x.go` is not a command and `go run x.go` is. Checked live: `go run` on a single file works with no `go.mod` anywhere, and the dot-directory the block is written into is invisible to `go build ./...` and `go vet ./...`, so a file parked there for one run cannot break the user's own build.

Kept out: **TypeScript**, which reads like the obvious next entry and is not one — node 22.16 on this machine cannot run a `.ts` file (measured: SyntaxError on the first annotation) and tsx, ts-node and deno are all absent. It gets an entry the day a runtime here can run it, because a tag in this table that cannot run is the exact bug the module was written to end. **Java and C#**, which fail rule 1.

---

## 121. Decision — An Answer the User Typed Over Is Still an Answer (2026-08-16)

Owner, 16 ส.ค., with a screenshot of a reply whose `##`, `**` and `>` were showing as source in muted 11px text: *"ตอนทักกลางคันมันเหมือนที่ เอไอ ตอบมาก่อนหน้ามันพังหมดเลย ไปเช็ค"*. Nothing was broken in the record. Everything was broken in the reading of it.

### 121.1 One channel, two things on it

Typing under a running turn goes straight into that turn (§44 Interject). When the model has already written its answer and called no tools, `cognitive.Agent` drains the interjection and keeps the turn alive rather than making the user wait out a reply they have moved past — and hands the answer over as a **non-final** round, which was the whole extent of what it said about it.

One step downstream that is indistinguishable from narration. `turn.Executor`'s `OnRound` treats every non-final round the same way: record the text, erase the live preview, emit `ToolEvent{Action: "note"}`. A note is §59's row for *"reading the config first"* — one line, drawn as `.tool-note` at `--fs-xs` in `--text-muted` with no markdown, and after the turn ends it is a text part that is not the last one, which is not the bubble's body and so lands behind the "ใช้ N เครื่องมือ" toggle.

So a finished reply lost its markdown, its size, its colour and its place, in one step, at the exact moment the user was reading it. The three defects were one defect: **the only fact that could tell the two apart was known in one place and carried out of it by nothing.**

### 121.2 The round that knows says so

`RoundEvent.Demoted` is set where the interjection is drained, because a round with no tool calls is a round the model meant to stop on, and that is knowable there and nowhere after. The executor branches on it: `partList.addAnswer` writes a `TurnPart{Kind: PartText, Demoted: true}`, and the live hand-over is `ToolEvent{Action: "said"}` rather than a note.

The flag rides on the stored part too, so a session reopened tomorrow can still tell an answer from narration — `stepsFromParts` reads it back into a `said` step. A new JSON field inside the existing `parts` column, so no migration; older rows have no flag, which is what they had anyway.

`addAnswer` never merges, in either direction. `addText` folds consecutive prose together because two text parts in a row mean nothing happened between them — but something did happen here: the user cut in. Two answers glued into one paragraph run would be a third way of saying the reply was never finished.

### 121.3 Prose is drawn as prose

A `said` row is not a row. It is excluded from `ownSteps`, so it can never appear inside the tool timeline or be counted as a tool, and it is drawn as `.markdown-body` in the bubble itself — above the answer that ended the turn while it is running, and still above it afterwards. The only thing added is a seam (`.said-block`), so two replies in one bubble do not read as one long one.

The rule this settles, for anything that later wants to re-place something the user has already read: **moving where a thing belongs must not change what it is.** The interjection changed the answer's place in the conversation. It had no business changing its typography.

---

## 122. Decision — A Popover Is Measured Against the Pane It Opens In, Not the Screen (2026-08-17)

**Status:** Implemented 2026-08-17 · **Files:** `desktop/frontend/src/style.css`, `desktop/frontend/src/test/composerNarrow.test.ts`

### 122.1 Laid out perfectly, half of it outside the window

Reported as one screenshot of the model picker with its left column sheared off (owner, 17 ส.ค.: *"การแสดงผลในพื้นที่น้อย"*). `ระดับความคิด` was `ความคิด`, `การอนุมัติ` was `รอบอนุมัติ`, and the words that were missing were the ones naming what each control does.

Nothing was wrong with the menu. `.model-menu` carried `min-width:280px` and grew leftward from a chip on the right, which is correct in every window it was ever designed in. The workbench then took most of the width, and the chat column left behind was narrower than 280px — so the menu grew out of the pane and the pane clipped it.

`.ctx-menu` had the same 270px and the same fate. Whichever had been fixed alone, the other was the next screenshot.

### 122.2 The composer is the only thing that knows

A menu cannot bound itself: it is positioned against a chip a few pixels wide, and `100%` there means the chip. `100vw` is the screen, and the screen is not the pane — that is exactly the assumption that broke. The one element that knows how narrow it has become is the composer box, so it is declared a container (`container-type:inline-size`) and the menus are measured in `cqi` against it.

`min-width` becomes `min(280px, 100cqi)`: 280 is what the rows *want*, not what they get. Past that it is whatever the composer has.

Inline-size only, deliberately. Block-axis containment would stop the composer growing with the prompt, which is the one thing that box is supposed to do.

### 122.3 The length was right and the origin was wrong

That fix shipped and the labels were still cut off — by less, and in the same place (owner, 17 ส.ค.: a second screenshot, *"ไปคิดออกแบบมาดีๆ"*).

A bound is measured from somewhere. The menu grows leftward from the **chip**, and the chip is not at the box's right edge — the send button is. So a menu allowed a full container width *from there* overflows the left edge by exactly the width of the send button, which is what the second screenshot showed and what the first one showed too, minus 42 pixels of it.

The arithmetic answer is `calc(100cqi - 42px)`, and it is the wrong kind of answer: it encodes the send button's width in the menu's stylesheet, and it is wrong again the day anything else lands beside the chip. So the anchor moved instead. The menus hang off the composer box itself — `.model-pick` and `.ctx-pick` are deliberately not positioned, and both menus offset by `--composer-pad-x`, a variable the box publishes so its padding is not spelled twice.

Now the bound cannot be wrong, because the thing being bounded and the thing bounding it are the same box. **A fix by structure survives the next control; a fix by arithmetic is a bug with a delay on it.**

The same correction applies one level in: `.updrop-list` hangs off a 180px trigger and was allowed 240px, so a long model id pushed it out through the menu's left edge — a popover overflowing a popover that was overflowing the pane. It is bounded to the menu now.

### 122.4 Side by side for as long as it fits

The first attempt also stacked every row below 360px, and the owner's answer to seeing it was to prefer the unstacked one (*"แบบนี้อ่ะดีแล้วครับ"*). Correctly so: a label beside the thing it names is the shape that reads, and 360px was nowhere near the width at which it stops fitting — the menu was still comfortable at 250.

So the threshold dropped to 220px, and the label learned to trim itself: `flex:none` had made it a hard floor under the row, so a long label widened the menu rather than shortening itself. Stacking is now what happens when there is genuinely no room, not what happens when a guess said so.

### 122.5 The same fault, one row up

The chips above the input — project, branch, shell — ran off the right edge in the same pane, the terminal one reading `Windo` against the boundary. A row of separate facts, and a fact half off the edge is not a shorter fact, it is a missing one.

They wrap now, and a single chip longer than the whole row trims itself, since wrapping has nothing left to offer it. Scoped as `.composer > .focus-row`, because `.focus-row` is *also* the name of a row inside the focus menu — two components sharing a class name, where the second definition silently overrides the first's `gap`. Left un-scoped, wrapping the composer's chips would have quietly rearranged a menu nobody was looking at.

### 122.6 A container query, not a media query

Every threshold here is asked of the composer, never of the window. The trigger is the pane, and the pane's width has nothing to do with the screen's — three panes and a sidebar share it, and the workbench can take most of it while the window stays maximised. A media query would fire on a machine and not on a layout, which is the same mistake as `100vw` in a different spelling.

### 122.7 Guarded where the failure lives

The test reads `style.css` off disk and asserts the bounds, for the same reason `themeContrast.test.ts` does: vitest stubs CSS imports to `""`, and a rule checked against an empty stylesheet passes. It checks both menus in one `it.each`, because the bug was never about one of them.

## 123. Decision — A Command Line Is Read by One Shell, and `--` Was a Second One (2026-08-17)

Owner, 17 ส.ค., asking for a check rather than reporting a fault: *"เช็ค รันคำสั่ง Linux เช่น `bash`, `ls`, `grep`, `find`, `sed`, `awk` หน่อย กรณีทำงานบน WSL ทำงานได้ดีหรือยัง"* — and on the finding, *"เริ่มเลยครับ"*.

Most of it worked, and the part that did not had been failing quietly since the backend shipped. `proc.WSL` invoked `wsl.exe … -- bash -lc <line>`, and the comment beside it said the line reached bash byte for byte. It did not. **`--` hands the rest to the distro's default shell**, which reads it as a command line of its own and expands every `$…` before bash is started. `-e` (`--exec`) starts the program with the argv given, which is what the call always meant.

The measurement that settles it uses no shell of ours at all: `-- /bin/echo 'A=$X B=$HOME'` prints `A= B=/home/mikedev`. Single quotes do not stop it, because by the time anything of ours reads the line the `$` is already gone.

### 123.1 What a missing `$` looked like from the outside

Nothing looked like an error, which is why a live test existed for quoting, the working directory, the login profile and cancellation, and none of them caught this:

- `awk '{print $1}'` arrived as `awk '{print }'` — every column instead of the first, no warning. An agent reading a field out of `grep | awk` got the wrong answer and had no way to know.
- `awk '{sum+=$2}'` arrived as `{sum+=}`, a syntax error the model would read as its own.
- `false; echo rc=$?` printed `rc=0`, and `set -o pipefail` reported a failed pipeline as `0`. §111.2 already states the rule for PowerShell's exit codes — a failure reported as success is the one direction a wrapper must never be wrong in — and this was that same fault arriving through the other backend.
- Anything the line set itself read back empty: a `for` loop's variable, a `VAR=x` prefix.

So the diagnosis is not "WSL is broken". It is that **one command line was being read by two shells**, and only the second one was ours. The fix is to stop the first read, not to escape around it: `\$` survives the outer shell, and a backend that asks every caller to double-escape is a backend that will be wrong the first time someone forgets.

### 123.2 Guarded where nothing else could see it

`wsl_live_test.go` now asserts that `$1`, `$?`, a self-assigned variable and a literal `$5` all survive the trip, plus the `/bin/echo` probe that proves it with no bash of ours in the way — so a failure cannot be mistaken for something bash did. Measured after the change: `--cd`, quoting with runs of spaces in it, UTF-8 Thai, pipes, `sed -i`, `find -exec` and cancellation-reaches-the-distro are all unchanged. The only thing removed was the extra shell.

### 123.3 A guess must produce evidence; a stated path need not

The same check turned up a second fault, separate enough to have been offered as its own decision (owner, same day, on the options: *"เห็นด้วย"*). The guard's embedded-path scan reads a `/…` inside quoted text as an absolute path — which is exactly what closes `python3 -c "open('/etc/passwd')"`, a token to any tokenizer and a path to the OS — and it therefore also refused `sed 's/^/n=/'`, `sed 's/ /_/g'`, `sed -E 's/(a)-(b)/\2\1/'` and every heredoc that writes a shebang. It failed closed, so it cost work rather than containment, but a wall that stops ordinary work is still a wall.

**The line now drawn: a path the command states is checked as written; a path this file guesses from shape must produce evidence before it may refuse.** Both halves matter. Every token the shell would pass as an argument is contained exactly as before — nothing about that path changed. Only the *inference* — a bare `/` found inside quoted text, which is also how POSIX spells a regex, a field separator and a substitution — now has to answer for itself.

The evidence is the cheapest fact that separates the two: whether the first segment names something that is there. `/etc`, `/home`, `/mnt` and `/tmp` exist on every distro, so every real escape keeps its refusal; `/n=`, `/_` and `/\2\1` exist nowhere. The lookup happens only when a guess is about to refuse a command, so a command that passes costs nothing at all.

**Failing closed is asserted, not assumed.** A filesystem this process cannot reach — a distro that is not installed, a name that never resolves — answers "keep the refusal", because *"I could not look" must never read as "it is not there"*. That is a test of its own (`TestShellUnderWSLKeepsTheRefusalWhenItCannotLook`), and it is the assertion that would catch someone later "simplifying" the reachability probe away.

Two more rules, each for a reason already in this file. A lone `/` **found by shape** is a delimiter, not the root of the filesystem. And a `/` straight after `#!` is an interpreter — program position, the one thing this guard has never contained (refusing the spelling would forbid the spelling and permit the act), which is why writing a script with a shebang is no longer refused for naming `/usr/bin/env`.

### 123.4 Where the clearing stops, and why there

Owner, on the residuals: *"เช็คต่อทีถ้าจะเคลียร์ต่อล่ะครับ"*. One of the two cleared; the other turned out to be the visible corner of a larger family, and the reason it stays is worth more than the family is.

**Cleared: a `/` after a glob or regex metacharacter.** `^`, `$`, `*`, `+`, `?`, `(`, `)`, `[`, `]`, `{`, `}` are how POSIX writes a pattern, and an absolute path cannot begin after one — so `grep -E '^/usr'` and `grep -E 'a$|^/etc'` are no longer read as naming /usr and /etc. Nothing is let out: reaching /etc/passwd needs that literal text, and putting a metacharacter directly in front of it makes it a *different, relative* path (`*/etc/passwd`), not a hidden absolute one. The two constructs where such a character genuinely can precede a path, `${…}` and `$(…)`, are stopped by `unreadable()` before this scan ever runs. `\` is excluded from the set on purpose — to a POSIX shell `\/` is an escaped slash, and `cat \/etc/passwd` really does open the file.

**Not cleared: a pattern written as a whole token.** `awk '/needle/ {print}'`, `sed -n '/^beta/p'`, `sed '/^#/d'`, `tr '/' '-'`, `cut -d '/' -f2` — measured, all refused, and all innocent. They are refused by the *token* half of §123.3, which is the half that was deliberately left alone: a token the shell hands to a program is checked as written.

Softening it means letting a token through on the same missing-first-segment evidence a guess answers to, and that reopens one specific thing: **`mkdir -p /x; cp secret.txt /x/y` in a single command line**. At the moment the guard reads that line, `/x` does not exist, so evidence would clear both tokens — and then the line runs and stages the workspace's contents outside it. Written as two commands it is still refused, because by the second one `/x` exists. It is the same-line version that no amount of parsing gets right, because the fact it depends on is created by the command being judged.

So the trade is a handful of one-liners, each with a way round it, against a staging area outside the workspace that one line can create and fill. The one-liners lose. What works today, and is now held by a test so it keeps working: `sed 's|/|-|g'` (any delimiter but `/`), `awk '{if ($0 ~ /needle/) print}'` and `awk '{gsub(/needle/,"x")}'` (the pattern stops being the token's first character), `grep '^beta'`, `cut -d/ -f2` (glued to its flag). The refusals are pinned by a test of their own, so the day someone clears them it is a decision and not a drift.

**And one that no rule reaches:** `awk '$1 ~ /^var/'` is refused as "builds a path while it runs", because `$1` at the head of a token is a variable to every shell and a field only to awk. That is §-old policy about unknown variables doing exactly what it says; `awk '{print $1}'` and `awk '$1 > 5 {print}'` are unaffected, since neither puts a `/` after the variable.

---

## 124. Decision — A Screen Nobody Who Builds This Can See Will Rot, and It Did (2026-08-17)

The first-run screen is the one page in Aetox with no returning users, and the one page nobody who works on Aetox ever sees. Its own logic guarantees that: it closes itself when the active provider already has credentials, which is the permanent state of every machine that builds it. Clearing the `aetox.onboarded` flag is not enough either — the has-a-key shortcut fires on the very next load and marks it done again. So it had drifted, unwatched, into a 560px dialog with two operating-system dropdowns in it, one of them seventeen providers deep and sorted alphabetically.

The owner asked what a new install actually meets, and then rebuilt it across five rounds, rejecting each version with the reason attached. What follows is what those reasons turned into.

### 124.1 The switch that makes an invisible screen visible

Settings → ทั่วไป now carries **ดูหน้าจอเปิดโปรแกรมครั้งแรก**. It arms a one-shot flag that *outranks every shortcut* — that is the whole design, and the reason "just clear the flag" was never a fix. It forgets the remembered look (theme, language, door, fonts, panel widths, the model seed) and the room the window was standing in, so the replay does not open on the Settings page it was pressed from. It does not touch keys, chat history, engine configuration, an unsent draft, or the tabs on the desk: this is a replay of a *screen*, not a factory reset.

Generalised in [DESIGN.md](../DESIGN.md) §3: **any state a developer machine cannot reach on its own gets a switch that reaches it, and the switch beats the shortcuts.** Empty database, expired token, refused folder, exhausted quota — all the same shape.

### 124.2 Three questions wearing one label

`HasAPIKey` answers true for a provider that *has a key*, for one that *needs no key at all*, and for one that is *signed in*. The onboarding list printed one label off that one boolean, so every row read "มีคีย์แล้ว" — including Ollama on a machine where it was never installed, and including Codex, which has no key and never will.

The same conflation was a real bug three places deep. `RequiresAPIKey && !HasAPIKey` drew a password field for Codex under the composer and inside the wizard, and in the CLI's provider menu it did worse: the loop refuses an empty line, so picking Codex demanded, forever, a credential that does not exist. Codex is a ChatGPT subscription reached at chatgpt.com; the only key anyone could paste belongs to api.openai.com — different host, different bill, guaranteed 401. `AcceptsAPIKey` had existed in the catalog since the provider was added and only the Settings page was asking it.

The rule: **a boolean that is true for more than one reason cannot be a label.** Ask how many ways it can be true; if more than one, split it before showing it. Here that is three — needs credentials, takes a pasted key, has one now.

### 124.3 Ask before you list

Seventeen providers alphabetically forces a first-time user to translate "I pay for ChatGPT every month" into "which row is that" before they can move at all. The screen now asks one question in the words of what the user already holds — a plan they pay for, a key from someone else, or a model on their own machine — and only then opens the wall of marks for the answer they gave. An answer with nothing behind it on that machine is not drawn: an empty category is a dead end wearing the clothes of a choice.

Skipping was removed on the owner's instruction (*"ห้ามข้ามทุกขั้นตอน"*), with Back kept on every screen. That is a live trade and it is written down as one: **removing the escape obliges you to name the door that is always openable.** Today that door is "install a local runtime". The day that stops being true, this decision has to be reopened rather than quietly tolerated.

### 124.4 What the ground is made of, and what motion is for

A radial glow borrowed from another product's welcome screen was tried and thrown out the moment it stood in Aetox (*"เอาลำแสงตรงกลางออก"*). What replaced it is the brand mark Aetox already draws behind an empty chat — and it shares that CSS rule by selector rather than by a copied set of values, because the two screens are three seconds apart on a new install and the day they disagree is the day the setup stops looking like the app it is setting up. **Borrow the structure; never borrow the ornament.**

Motion answers two questions and no others: did the press register, and what is happening right now. A slower cinematic fade-in was offered and turned down against the one the app already had. The timings are now tokens in `styles/type.css` (`--dur-press`, `--dur-arrive`, `--dur-settle`, `--dur-hold-success`, `--dur-hold-done`, `--dur-ground`), and the two places a script owns the timing read them through `lib/motion.ts` instead of restating the number. DESIGN.md names the tokens and never the milliseconds, for the reason this file gives everywhere else: a second place answering one question is a debt.

### 124.5 What the tests could not have caught

Three of this round's faults were invisible to the suite and visible in one look at the running app: a row that said "signed in" twice, a card grid that hung to the left because two cards sat in the first two of four fixed columns, and a content column pinned to the left of a 1900px window. They were fixed and then *measured* — 124px of space on the left, 124px on the right — rather than pronounced better.

So the suite now also pins what must **not** be there: the first screen may have exactly two buttons, the connect screen's only link is Back, and the sign-in path may never grow a password field. A negative assertion is the one that survives the next person's good intentions.

## 125. Decision — Two Languages Written as a Number, Not as a List (2026-08-17)

The owner asked a sizing question — how hard would a third language be? — and the answer separated into two halves that did not resemble each other.

The window was ready. `i18n.svelte.ts` says so in its own header: one `locales/<code>.ts` file plus one line. Every screen that offers the language, the first-run picker included, iterates `localeNames` rather than naming Thai and English, so none of them would need touching. The engine was not ready, and neither was the rest of the small print.

### 125.1 The shape that forbade a third

`internal/model/noop.go` is the one part of the engine that talks to a user in their own words (§40), and it held every one of those words as two positional arguments: `pick(th, en)`, 37 times, plus four `if english()` branches that returned a whole reply per language. Nothing there was wrong for two languages. It was wrong about how many there could be — "exactly two" was written into the signature between the strings and the reader, so a third could not be written at all until all 37 sites had been edited first. That is the definition of debt the owner named: the cost of the next language was being paid by a change that had nothing to do with translating anything.

It is now `phrase`, a map keyed by language code, read through one `inLocale` (whose resolution order §125.5 revised the same day it was written). Adding Japanese is adding `"ja"` keys to the phrases someone translated, and nothing to the ones nobody did. `english() bool` is gone, which is the point: a boolean can only ever answer for two.

The four branches went with it. A branch is the thing that has to be *found* again for the third language; a map entry is a place that is already open. `noopLongReasoning` now renders both languages and throws one away, which is a real waste and the right trade — it is a bench model's thinking script, and uniformity is worth more here than the microseconds.

### 125.2 The same debt, one line long

`index.html` shipped `lang="th"` and never changed it, so the document told the browser Thai no matter what the user picked — the answer that reaches hyphenation, spellcheck, and a screen reader choosing a voice. `setLocale` now sets `documentElement.lang`. And `initLocale` read the saved preference as `saved === 'en' ? 'en' : 'th'`, which is the same "two" written a third way; it now asks whether the saved code is a language the app has.

`i18n.svelte.ts` itself listed the languages three times over — a union type, a names record, a dictionary record — three chances to add a language the compiler accepts and the picker never shows. They are one `locales` object now, with `Locale` derived from its keys, so the header's promise of "one line here" is finally true.

### 125.3 What a third language still costs, honestly

Not zero, and worth stating before anyone starts:

- **~1,160 strings** in a new `locales/<code>.ts`. Typed against `th`'s keys, so a missing one is a compile error, not a silent fallback. This is the work; the rest is minutes.
- **The engine's canned copy** — one onboarding reply and six guide answers. Product copy, so it has to be translated by someone who means it. The bench scripts can stay Thai and fall back.
- **Six `STARTERS.<lang>.md`**, one per shipped worker. Already generic: `config.AgentStartersName` builds the name and guards it against escaping the folder.
- **OCR and speech are a different question entirely.** Tesseract is bundled with tha+eng data and whisper is set to Thai+English; a UI in Japanese does not mean Aetox can read a Japanese screenshot. Adding a UI language and adding a sense are not the same purchase, and saying so in the release notes is part of the work. Nothing in the app currently claims otherwise — the guide answer already says "ไทย+อังกฤษ" in both languages, and the checked claim is what makes this a note rather than a fix.

`TestAThirdLanguageIsDataNotCode` pins the half that is fixed. It asks a `phrase` for a language nothing in the package has been translated into yet, which is exactly the question that could not be asked before.

### 125.4 The two the sizing pass found and this change closed

The identity templates were the worst of them and the least visible. `identity.svelte.ts` held four Thai string literals while every other user-facing string in the window lived in the locale files — so the one screen that ignored the language choice was the screen where a user writes down **who they are**. They are `identity.tpl*` keys now, and `identityTemplates` became a function because `t` has to be read at call time. The filenames did not move and must not: `identity.md`, `thinking.md`, `context.md`, `skills.md` are addresses `internal/prompt` folds into every prompt, and translating one would quietly open a second file instead of the first. `identityTemplates.test.ts` pins both halves — the bodies follow the locale, the names never do.

The installer was one line. It carried `MUI_LANGUAGE "English"` and nothing else, so the Thai-first app introduced itself in English. English stays first, because the first table inserted is the fallback for a machine matching none of them, and Thai follows — NSIS then picks by the machine's UI language with no dialog to click through. It is wrapped in `!if /FileExists` on the `.nlf`, because the installer is built **only** by the release workflow: a missing language file would not fail a PR, it would fail a release. An English installer on one machine is a small loss; an installer that does not build is the whole release.

### 125.5 Eight languages chosen, and the two rules that had to change first

The owner picked the set the same day: **จีนตัวย่อ, สเปน, โปรตุเกส-บราซิล, เวียดนาม, รัสเซีย, ญี่ปุ่น, เยอรมัน, ฝรั่งเศส** — ten with Thai and English. Measured before agreeing, because two of the numbers were guesses until they were not:

| | |
|---|---|
| แรม ต่อภาษา | **540 KB** measured (V8 heap, the real locale files) — of which ~107 KB is the text and the rest is V8's own overhead |
| ฟอนต์ จีน | **4.31 MB** (`@fontsource-variable/noto-sans-sc`, 101 subsets) |
| ฟอนต์ ญี่ปุ่น | **~5.0 MB** (`@fontsource-variable/noto-sans-jp`) |
| ฟอนต์ สเปน/โปรตุเกส/เวียดนาม/รัสเซีย/เยอรมัน/ฝรั่งเศส | **0** — Inter already ships latin-ext, cyrillic and vietnamese subsets |
| `chi_sim.traineddata` (OCR) | 42.3 MB, downloaded at install, **not** in the installer |

An earlier estimate in this conversation put a CJK font at "tens of MB". That was the *static* package (`@fontsource/noto-sans-sc`, 74.5 MB); the variable one is seventeen times smaller. Corrected here because the wrong number would have killed Chinese on a false premise.

Two rules had to change before a single word could be translated, and both were only wrong once there were more than two languages:

**A region subtag is not a flavour.** `inLocale` cut every tag at the first dash, so `pt-BR` resolved as `pt` — which is exactly how you hand a Brazilian the European translation and never hear about it. It now tries the whole tag first, then the language half, and the lookup is case-insensitive on both sides because `pt-BR` and `pt-br` get written about equally often and a key that misses on capitalisation would fall through to a real translation, just the wrong one.

**Thai was the fallback because it was the only other language.** That made it right by accident. A Spanish user meeting a key nobody had translated would have got Thai — a script they cannot sound out — while the complete English sat unused. Both sides now fall through to English, with one carve-out: an *empty* locale still means Thai, because empty is not "a language we lack", it is a fresh install that has not been asked yet.

**And the completeness rule got a second tier.** `Record<keyof typeof th, string>` — missing key, compile error — is correct for the two languages the owner writes and reads. Held over eight he cannot read, it means every new button in Aetox breaks the build until eight translations arrive, which does not produce eight translations. It produces a placeholder pasted in to turn the build green, and a placeholder is a lie that type-checks. Contributed locales are typed `Partial` and fall through to English instead.

What replaces the type check is `localeGuard.test.ts`, which asserts everything checkable **without reading the language**: no key the app does not have, nothing blank, and every `{placeholder}` preserved. That last one is the reason the file exists — a translation that drops `{count}` renders "You have  items" to a real user forever, and no test that only counted keys would ever see it.

---

## 126. Decision — A Search That Never Happened Is Not an Empty Result (2026-08-17)

Owner, 17 ส.ค., with a screenshot: *"เจอบั๊คอีกแล้ววว"*. On the diagnosis and the three-part fix: *"เริ่มเลยครับ"*.

On screen, the agent told the user it could not reach WSL `mikedev` or their D: drive, and offered them a `grep` to run by hand. Fifteen minutes earlier it had copied a project out of that distro, started a server in it and read the pages back. `tool_runs` holds what actually happened, and it is four calls:

| tool | asked for | answered |
|---|---|---|
| `grep` | `/mnt/d/Project/AI-Robot-Guide-` | **ok** · `(no matches)` |
| `glob` | `/mnt/d/Project/AI-Robot-Guide-` | **ok** · `(no files matched)` |
| `list` | `/mnt/d/Project` | failed · `open C:\Users\ASUS\aetox\mnt\d\Project` |
| `shell` | `wsl -d mikedev -- bash -lc 'find …'` | failed · `wsl: command not found` |

The third row is the confession. `/mnt/d/Project` has no drive letter, so `filepath.IsAbs` says it is relative, so it was joined onto the sandbox root. Only `list` said so out loud; the two searches reported a clean, successful, empty result.

### 126.1 The half that has nothing to do with WSL

**A tool that could not look must never answer as though it looked and found nothing.** `grep` and `glob` walked a directory that was not there, opened no files, matched nothing, and returned the same sentence they use for a folder they read every byte of. The two are not one answer, because only one of them is evidence.

What it cost is the reason this is the first half and not a footnote. The question in front of the agent was whether that project held an admin password. It was handed "no", and it passed "no" to the user — about a folder it had never opened. §111.2 and §123.1 already say a failure reported as success is the one direction a wrapper must never be wrong in; this is that rule arriving through a *search* instead of an exit code, and it was wrong on any machine, WSL or not, for any mistyped path.

`searchBaseExists` is now the gate, and it names both spellings — what was asked for and where it resolved to — because "no such folder" says the search failed while "`/mnt/d/Project` resolved to `C:\Users\ASUS\aetox\mnt\d\Project`" says why, which is the difference between the model fixing the path and the model concluding the project is empty.

One case keeps the gentle answer, and the split is deliberate: `glob` derives a walk root from the *pattern's* literal prefix, and a prefix that does not exist is not a claim about the disk. A directory the caller **named** is. Explicit path, error; guessed prefix, no match.

### 126.2 One workspace, two spellings

The shell had been translating since the backend shipped: `gateFor(backend)` puts every path a command names through `HostPath` before containment is decided. The file tools never learned. So one session held a shell that speaks `/mnt/d/...` and a `read`/`grep`/`glob`/`list` that speak `D:\...`, and nothing in between.

Translation now happens in the one place containment is decided — `resolveSandboxPath`, through `hostSpelling` — rather than in each of ~20 tools, which is the same reasoning that put the workspace policy in a map keyed by root instead of in every struct. The shell is recorded beside that policy on every registry build, nil included, so a workspace switched back to the native shell stops translating in the same call.

**Translating is not widening.** The same wall, checked against the file the caller meant instead of against a mangled path that was refused by accident. A guest path outside the project is refused; `/etc/passwd` from a focused Windows project is refused; a native session's reading of a leading `/` is byte-for-byte what it was.

Only paths that could be guest-spelled are translated, and that is a cost rule as much as a correct one: `HostPath` resolves the distro's mount root by reading `\\wsl.localhost\<distro>\etc\wsl.conf`, which **starts the distro** if it is stopped. Relative and Windows paths already mean the same thing in both worlds and are what nearly every call carries, so the common case must not pay a distro boot to be told it was fine.

### 126.3 A second home on the same machine

The credential denylist is home-relative, and until file tools could hold a guest path there was only ever one home to be relative to. Now `/home/mike/.ssh/id_rsa` resolves to `\\wsl.localhost\<distro>\home\mike\.ssh\id_rsa` — a real file on this machine, holding exactly what the list exists to refuse, sitting nowhere near the Windows profile the check compares against.

That path was always spellable; what §126.2 changed is that it became the *natural* way to write it, which is the difference between a hole nobody walks into and one on the main road. So the list is now applied to the distro's homes as well, read off the path (`/home/<user>`, `/root`) rather than asked of the distro: asking costs a boot on a check that runs on every resolved path, and answers for the default user when the file may belong to another. Being wrong here can only refuse a `.ssh` that is not a `.ssh`.

### 126.4 The habit a shell's name cannot correct

`Backend.Note` exists for exactly one thing (§111): the constraint a shell's *name* does not carry, which for Windows PowerShell 5.1 is the missing `&&`. WSL's note was empty, on the reasoning that "bash" is instruction enough. That is true of the syntax and false of the standpoint.

A model told its commands run in `bash (WSL: mikedev)` still believes it is on the Windows side looking in, so it writes the line that would be right from there — `wsl -d mikedev -- bash -lc …` — into a shell already standing inside mikedev. `wsl` is not a Linux program. It died with `command not found`, and the agent read that as *this machine has no WSL* and told the user so.

The note now says where the command starts, and says the same thing about paths, because every file tool answers with the Windows spelling of a file and the prompt rightly tells the model to repeat the path a tool gave it. Two sentences, one fact, and not a syntax table: what generalizes is **you are already there**, and everything a model would otherwise do to get there follows from it.

### 126.5 One fact, one home

Owner, on the fix as it stood: *"การทำให้ WSL กับวินโด้สวิชไปมาได้แบบนี้จะทำให้เกิดหนี้ในระบบไหมอ่ะ"*.

The answer was yes, in one specific place, and it was introduced by §126.2 rather than by the switching. `shell` held its own `backend func() proc.Backend`, handed to it at construction; the file tools read the workspace's record. Two homes for one fact, set from the same option one line apart in `RegisterDefaults` — which is the exact shape that let the shell and the file tools disagree about what a path meant in the first place. A fix that reproduces the bug's structure one layer up is not finished.

`shell` now looks its backend up by root like everything else (`sandboxShellFor`). The lookup is still per use, so the picker still takes effect on the next command rather than the next restart. The field is gone.

The category objection is worth recording because it is the reason this looked wrong before it looked right: a sandbox *policy* is about permission, and "which shell" is not a permission. But the record is not a permission list, it is what a root's session **is** — which folders it may touch, and which language it speaks — and the file's own header already gives the reason it exists at all: ~20 skills hold a root, and threading session state through every struct is worse than one map keyed by it.

### 126.6 The return trip

§126.2 translated everything crossing *into* the workspace. Nothing translated what the tools hand *back*, and their answers are Windows paths.

That is not symmetric politeness — it is the same failure aimed the other way. The prompt tells the model to repeat the path a tool reported rather than assemble one, which is right and is the rule that stops it composing a root with a filename and sending the user to a file that is not there. In a WSL session it also means the one receipt that names an absolute path hands the model `D:\…` to type into a shell that has never heard of drive letters.

`GuestPath` had existed since the backend shipped, with a comment claiming it was "what tells the model where it is standing", and **no caller outside its own tests**. It is on the `Backend` interface now, identity on native, and the receipt uses it.

The receipt names **both** spellings, and that is the whole decision. This clause has two readers who need different answers: the user asks where the file is on their machine and opens the Windows path in Explorer — which is why the clause exists at all — while the model's next move is often to use the file in a shell where that path opens nothing. Saying which is which costs one clause on a receipt that only appears when placement moved a file. Picking one reader to be wrong for costs a turn, every time.

Four tools appended that clause, in four identical copies. A receipt that has to be right about two filesystems is not the thing to keep four copies of; it is `onDiskNote` now, and a native session gets back the string it always had.

### 126.7 An absence reported at the size it actually is

§126.4 stops the model reaching for `wsl` from inside the distro. It does not stop the *reading* that turned one dead command into a claim about the machine, and that reading is the more expensive half: `wsl: command not found` is a true sentence about one program, and the agent published it as *there is no WSL here and no way to reach your D: drive*.

Nothing in the answer said who had spoken, so the largest possible reading of the absence was also the only one available. A failed command now names the shell it failed in — `exit status 127 (in bash (WSL: mikedev))` — on the path that already failed, at a cost of one clause.

This is §126.1's rule with the subject changed. There, a search that could not look must not answer as though it looked; here, a program that was not found must not be reported as an environment that is not there. Both are an absence being rounded up.

### 126.8 The cut that was measured, and not made

Owner, once the seam was visible: *"ถ้าเราตัด WSL ออกไปมันจะง่ายขึ้นกว่าเดิมเลยไหม"*.

Simpler, and measured rather than argued: ~2,100 lines of Go across eleven files, plus the composer chip, its test and its strings. `Backend` itself is most of that — the interface exists only because there are two of them, and one shell is a build-tagged package function again.

Two things kept it. The first is that **the tax is smaller than the deletion suggests**: of the five pieces of work this decision covers, two survive the cut. §126.1 is a lie any machine tells about any mistyped path, and consolidating four copies of one receipt was duplication regardless. `gateFor`'s POSIX split stays too, because Linux and macOS builds exist and the guard still has to know how a line tokenizes.

The second is the audit log from four hours before the question. Between 12:55 and 13:03 that day the agent rsynced a project out of `/mnt/d`, started `uvicorn` on port 8014 from a venv, checked it with `ss` and `curl`, and opened the page. Without the backend the files are still readable through `\\wsl.localhost` and **nothing runs**: no python, no docker, no pytest. The capability is not speculative; it is what the owner was doing when the bug was found.

The cheap middle was considered and does not fit: deriving the shell from where the project lives — which `DefaultBackendFor` already does for projects under `\\wsl.localhost\` — would delete the picker, the stored setting and the mid-session switch, and with them everything §126.5 had to untangle. It fails on the case in hand. `D:\Project\AI-Robot-Guide-` is a Windows path whose toolchain is in `mikedev`; files on one side and tools on the other is the shape of the work, and nothing in a file's location says so.

What stays unpaid is recorded in §126.9.

### 126.9 Not done: the switch leaves no mark in the conversation

A shell can change while a conversation is running. The chip changes, the tool description changes, and **the transcript does not** — so the model's own earlier turns, written correctly for the old shell, remain the strongest evidence in front of it. That is why `wsl -d mikedev --` was written at 13:09 by an agent that had been fluent in bash since 12:54.

§126.4 competes with that history on every request, which is worth more than a line that scrolls away and is summarized out. It does not correct it. The honest fix is a mark in the transcript at the moment it happens, and there is no honest place to put one: `messages` has two roles, `user` and `agent`, and the frontend renders anything that is not `user` as the assistant speaking. A notice in a bot bubble is the app putting words in the agent's mouth.

Cheaper than a third role, and recorded as the alternative: forbid the mid-session switch outright. It saves no code and removes the class.

## 127. Decision — The Agent's Tab Is the One It Opened, Not the One in Front (2026-08-17)

**Status:** Implemented 2026-08-17 · **Files:** `desktop/browser.go`, `desktop/workbench.go`, `desktop/browser_tool.go`, `desktop/browser_capture.go`, `internal/skill/packed.go`

Owner, 17 ส.ค., asking for the capability §119.7 had left unwired: *"ผมอยากพัฒนาอ่ะ แบบ เอเจน แคปรูปหน้าเว็บเองได้ ดูได้ หรือ เก็บไว้ที่ไหนสักที่ให้เราได้"*. The picture was the ask. What the ask uncovered was that the agent did not reliably know which page it was on.

### 127.1 One field was answering two questions

`browserHost.lastID` means "the tab in front": `BrowserSetVisible` rewrites it every time a tab is raised, **including when the user clicks between their own**. `browser_read`, `browser_click` and `browser_type` all resolved their target through it.

So a user glancing at their own page mid-turn moved what the agent's next action landed on. Reading a page it never opened is the mild version; the real one is that `click` pressed buttons and `type` filled fields on it. The rule that rights come only from a list the user can see does not survive the agent acting on a document nobody granted it.

`agentBrowserTabID` looked like the guard against this and was not. It read the same `lastID` and accepted it only when the id carried the `web-agent-` prefix, which quietly means *the agent's tab, but only while it happens to be the one on screen*. The failure was symmetrical and both halves were bad: with the user's tab in front it answered "" while the agent's tab sat there alive, so `open` minted a **second** agent tab and stranded the first — the tab-after-tab behaviour that reuse was introduced on 2026-08-10 to end.

The host now keeps `agentID` next to `lastID`. Two questions, two fields, neither pretending to be the other: the frontend asks which tab is showing, and nothing the agent does may. The `web-agent-` prefix is tested in exactly one place, at the moment a tab is created, and everything downstream reads `agentID`.

It is never cleared on close. A tab the user closed is gone from `tabs`, and the liveness check reads that as no tab — which is also how `open` learns to mint a fresh one, so one mechanism covers both.

Owner on what this buys, and it is the better sentence: *"เราเปลี่ยนกระทันหัน แต่มันก็รู้ว่าแท็ปไหนคืองานไหน"*.

### 127.2 The refusal had to change with it

*"No browser tab open in the workbench"* was true while the target was whatever was showing. Once the target is the agent's own tab, the user can have a screenful of them and the sentence is a lie. It now says the agent has not opened a page, and says whose the others are.

### 127.3 Reading a page the user opened is not a smaller version of this

The tempting middle position is to let `read` see any tab while `click`/`type`/`capture` stay on the agent's own — a viewing right and an acting right, which sounds like the permission split this product already makes.

It does not work here, because **refs come from a read and are spent on a click**. Point those two at different documents and a ref resolves against a page it was never numbered for. That is not a safer arrangement than the bug; it is a different one. So the rule is single: the agent works its own tab. A page the user wants looked at, the agent opens for itself.

### 127.4 Only then, the picture

`BrowserCapturePNG` has existed since the annotation modes (§119) and was reachable only by the user drawing on a page. Two things made it not-yet-a-tool, and only one of them was about pictures.

The first was §127.1: a photograph of the wrong page is obvious in a way that text of the wrong page is not, so capture would have been the action that finally made the tab bug visible, in the worst possible way — by handing back a picture of whatever the user was looking at.

The second is that **the tab is raised before it is shot, rather than hoped to be up**. A hidden native view is a Win32 `ShowWindow(SW_HIDE)` and composites no frames, so a capture of one is the last thing it drew or nothing at all. Raising is also the honest half: this tool's premise is that the user watches what the agent does, and that cannot be true of a photograph taken of something they were never shown. It reuses the event `open` already sends, whose handler only makes an existing tab active — so nothing re-navigates.

`capture` is its own permission (`browser_capture`), not part of `browser_read`. "May read this page" and "may see it" are different rights: a picture carries everything the text left out.

### 127.5 Where a picture lands, and why not where new files land

Shots go to `output/<session>` under the working root **always**, deliberately not through `a.outputSubdir()`. Those agree with no project focused and disagree with one, where `outputSubdir` is `""` — meaning a new file lands in the project itself. That is right for a file the user asked for by name and wrong for this one: a screenshot is a byproduct of the agent looking at something, and `page-1.png` in the root of somebody's repository is a change nobody asked for. `output/<session>` is also the one path `ListArtifacts` is defined to sweep, so the shot appears in the gallery under either mode.

The model is handed the image only when it has eyes (`model.ResolveVision`, the same question `visionAttachments` asks of a user's attachment). A blind one gets the path and `image_ocr`, which is the path it has had since §22.

### 127.6 A camera with no rule for it photographs everything

The action's description leads with the condition, not the capability: use it **only when `read` cannot answer, because the answer was never in the text** — a chart, a canvas, a map, a rendered document, a layout you suspect is wrong. Read first, photograph second. A model told it can take pictures and not told when will take one of every page it opens, and each one costs more than the read it duplicated.

For the same reason `click` and `type` now name the page they landed on. Owner: *"อันที่ฉันเปิด มันก็ต้องรุ้ด้วยนะ ว่าปัจจุบันเปิดอะไรอยู่อีก"*. Steering one tab of your own is half an answer; knowing what is in it is the other half, and a click can move the page. Without it the model either remembers across turns or spends a whole `read` asking where it is.

### 127.7 What this does not decide

**Whether a raised-then-shot tab needs a longer settle than 400ms.** The wait is a guess until it is watched on a real window; it is a number to tune, not a design.

**Full-page capture.** The engine gives the viewport. Scroll-and-stitch would lie about sticky headers and lazy loading, and the pages that need a picture at all are usually one screen.

**Going back to an older tab.** The agent has "the last one I opened" and no way to name a tab and return to it. Nothing has wanted that yet.

### 127.8 The rule the capture path stated, and the one place that broke it

Found by running §127 rather than reading it: the agent opened `example.com` in one second, then wikipedia, google and httpbin each failed after exactly twenty. First page fine, every page after it dead.

`browser_shot.go`'s header states the rule for this engine — a COM call made from a goroutine instead of the webview's thread *is refused outright, not delayed*. Every call into a view in `browser.go` goes through `withTab`/`do` accordingly: navigate, bounds, zoom, visibility, and all three script paths. The reuse branch of `workbenchOpenBrowser` called `tab.view.navigate` directly, and it was the only one.

So on a reused tab nothing navigated. The refusal went to the engine's own error callback, no `NavigationCompleted` ever fired, and `awaitNavigation` spent its full twenty seconds before reporting a page that had never been asked for — a sentence that reads like a slow site and was a call the engine had thrown away.

It has been this way since reuse shipped on 2026-08-10, and it is not what §127.1 changed: that line is untouched by it. What §127.1 did was make reuse fire reliably, which turned an intermittent failure into every-time — the ordinary way a correctness fix pays for itself before its own feature is even switched on.

### 127.9 Three bugs, one root, and a proposal

§127.1, §127.8 and the tab-after-tab behaviour of 2026-08-10 are not three mistakes. Each broke a rule that was correct, written down, and enforced by nothing but a comment somebody had to have already read.

Owner, 17 ส.ค., after watching the third one: *"ร่างสถาปัตยกรรมที่มันควรจะเป็นดีไหม"*. Drafted as [docs/architecture/browser-subsystem-2026-08-17.md](architecture/browser-subsystem-2026-08-17.md) — **proposed, nothing built**. Its thesis is that all three rules can move from prose into types: the webview reachable only from inside the host-thread callback (so `tab.view.navigate` stops compiling), the agent's tab a distinct type from any tab (so the user's cannot be passed where the agent's is meant), and one resolver for "is this an address or a search" with the policy left to the caller — the address bar searching, `open` refusing and naming `web_search`.

## 128. Decision — A Webview You Can Only Reach From the Thread That Owns It (2026-08-17)

**Status:** Implemented 2026-08-17 · **Files:** `desktop/browser.go`, `desktop/browser_pick.go`, `desktop/browser_shot.go`, `desktop/workbench.go` · **Design:** [docs/architecture/browser-subsystem-2026-08-17.md](architecture/browser-subsystem-2026-08-17.md) Defect 1

Owner, 17 ส.ค., after the third bug in two days: *"ร่างสถาปัตยกรรมที่มันควรจะเป็นดีไหม"*, then *"เริ่มเลย"*.

### 128.1 The door was already right; there was no wall beside it

`browserHost.withTab` has always queued onto the thread that owns webviews, and sixteen call sites used it. `browserTab.view` was an ordinary field on an ordinary struct, so anything holding a `*browserTab` could reach the engine without it — and one call site did (§127.8), for a week, invisibly, because the engine's answer to a wrong-thread call is a refusal delivered to a callback nobody reads and a page that never finishes loading.

Adding a comment would have been the third comment about this rule.

So the field is gone. Views live in `browserHost.views`, which nothing outside `browser.go` reads, and `onTab` hands one out only from inside `backend.do`:

```go
func (h *browserHost) onTab(id string, fn func(tabView, *browserTab)) {
	h.backend.do(func() {
		h.mu.Lock()
		v, t := h.views[id], h.tabs[id]
		h.mu.Unlock()
		if v != nil && t != nil { fn(v, t) }
	})
}
```

The measure of whether this worked is not that the tests pass. It is that `tab.view.navigate(url)` — the exact line that cost §127.8 — does not compile.

### 128.2 What it does not claim

A caller can still copy the `tabView` out of the callback and use it later, and no Go type will stop that. This does not prevent a decision; it prevents an accident, and an accident is what happened. Nobody chose to call an engine from the wrong thread — they reached for the nearest field, and the nearest field was the wrong one.

### 128.3 Two smaller truths that fell out

`tab == nil || tab.view == nil` was the liveness check at three call sites, and it is now `host.live(id)`. That reads better because it is more honest: a tab with no view was never a tab, it was an `openTab` that failed and got stored anyway.

And `BrowserClose` now takes the view out of the map before destroying it, rather than deleting the tab and destroying through a pointer the map no longer knows about. Same effect today; one fewer way for a later reader to be surprised.

### 128.4 What the browser told nobody

The owner found this one by asking the right question rather than reading the code, 17 ส.ค.: *"เบาเซอร์ อาจจะพังเพราะมันไม่รายงานอะไรกับเอเจน เลยก็ได้ เอเจนเลยไม่รู้"*.

`SetErrorCallback` wrote every engine complaint to stderr and to `debuglog`, and `tabCallbacks` had no field to carry it any further. Two writes, no reader. So the tool above could only ever answer with the timeout it observed — `page did not finish loading` — which is a `statereport`, and a statereport means *weather*: slow tonight, down tonight, nothing to correct.

The agent read that correctly. It concluded the network was unreliable and said so to the user, three times, while WebView2 had been answering `This method can only be called from the thread that created the object` on every attempt. **The agent was not wrong; it was accurate about a sentence that was wrong.** No amount of reasoning gets from "the page did not load" to "this program is calling COM from the wrong apartment".

So `onEngineError` now rides on `tabCallbacks`, `browserTab` keeps the last complaint, `armNavigation` clears it so it always belongs to the navigation being waited on, and `awaitNavigation`'s timeout splits into two branches that are different **kinds** of thing:

- the engine refused our call → a plain `fmt.Errorf` naming what it said. A defect in this program.
- nothing from the engine → `statereport` as before. Weather.

Filing the first as the second is precisely how it survived a week of being logged every twenty seconds.

The general rule, which is the part worth carrying past the browser: **anything an outside system says about a request WE made has to be able to reach the caller.** A log serves the person who already knows to investigate, and not knowing there was anything to investigate was the whole of the problem.

### 128.5 Four ways to say which page, and the one that nearly took the panel with it

Auditing 128's own diff turned up debt it had just added. `open` named a page `Title (url)`, `read` wrote a markdown header, and `click`, `type` and `capture` each invented a spelling as they were written. One fact, four renderings — in the file that already keeps `browserOpenedPrefix` as a shared constant *precisely* so `open`'s sentence and `parseBrowserOpened` cannot drift. The lesson was written down here already; it simply had not been applied to the sentences added after it.

`browserPageRef` is now the one place a page gets named, and `open`, `click`, `type` and `capture` all go through it. **`read` is a deliberate exception** and keeps its two-line header: it is not a sentence referring to a page, it is the top of a document the model reads as a document, and folding it in would make it worse to read in order to make it easier to count.

The interesting part is what the fix nearly broke. A page with no title had been written `เปิดแล้ว:  (url)` — a sentence with a hole in it — and `browserPageRef` sensibly returns the bare address instead. But `parseBrowserOpened` required a closing parenthesis, so for one commit every titleless page the agent visited would have vanished from the "pages the agent has been" panel, silently and forever. `TestParseBrowserOpened`'s `empty title` case caught it in the same minute.

That test exists because this exact contract broke once before. It paid for itself twice.

### 128.6 What the neighbours do, and the one thing worth taking

Asked whether there was open-source work to learn from. Two families, and only one of them shares our problem.

**Agents driving a browser** — Playwright MCP, browser-use, Stagehand — drive an *external* browser over CDP. Playwright MCP's design is accessibility-tree snapshots with per-element refs and no vision, exposed as `browser_navigate`/`browser_click`/`browser_type`/`browser_snapshot`. That is what Aetox already does, arrived at independently and cited here since §48. Little to take.

**Embedding a webview in your own window** — CEF, wry/Tauri, Electron — is where all of this week's bugs live, and where the first family has nothing to say, because they never embed anything. wry's answer to thread affinity is that webviews only exist on the main thread and off-thread work goes through an event-loop proxy, with platform handles reachable only inside a `with_webview` closure. That is `onTab`, independently, in Rust — which is the reassuring part: §128.1 is the standard answer to this problem, not an invention of ours.

CEF has the one thing worth taking. Alongside `CefPostTask` (our `do`) and `CefCurrentlyOn` sits `CEF_REQUIRE_UI_THREAD()`, a `DCHECK(CefCurrentlyOn(TID_UI))` at the top of every function with a thread it must be on. It covers exactly the gap §128.2 admits: a compiler cannot see a `tabView` copied out of a callback and used later.

So `win32Tab.requireHostThread` now guards every method. It does **not** panic — a crash in a shipping desktop app over a browser tab is not a trade worth making, and a log line is what §127.8 proved nobody reads. It reports through `onEngineError`, the channel §128.4 built for this, so a wrong-thread call names itself in our own words, in the tool result, *before* WebView2's more cryptic refusal arrives. A wall for the reach, a tripwire for the stash.

### 128.7 And the measurement, since a number beats an opinion

Playwright MCP publishes roughly 200–400 tokens per accessibility snapshot. `browser_read` caps at 60,000 characters, which would be about 15,000 tokens, and nobody had ever checked what it actually costs.

Measured against the `tool_runs` history on the owner's machine — the log this app has always written and §81 already reads back:

| tool | n | median | p90 | max |
|---|---|---|---|---|
| `browser_read` | 15 | ~211 tok | ~841 tok | ~952 tok |
| `browser_open` | 33 | ~22 tok | ~32 tok | ~45 tok |

The median sits inside Playwright MCP's published band and the worst case is a fifteenth of the cap. **There is no problem here**, which is worth writing down precisely because it was worth suspecting.

The honest caveat: n=15, over example.com, a localhost page and MDN. A heavy application — a mail client, a dashboard — is not represented, and the tail is therefore unmeasured. The median says this is not currently costing anything; it does not say the cap is set right.

## 129. Decision — The Agent's Tab Has a Type, and an Address Bar Has Two Jobs (2026-08-17)

**Status:** Implemented 2026-08-17 · **Files:** `desktop/address.go`, `desktop/workbench.go`, `desktop/browser_capture.go`, `desktop/frontend/src/lib/stores/workbench.svelte.ts`, `desktop/frontend/src/lib/workbench/Workbench.svelte` · **Design:** [browser-subsystem-2026-08-17.md](architecture/browser-subsystem-2026-08-17.md) Defects 2 and 3

Owner, 17 ส.ค.: *"เอาให้เสร็จเลย ผมรอดูผลลัพท์"*.

### 129.1 AgentTabID, and two functions becoming one

Splitting the host's `lastID` from `agentID` (§127.1) fixed the bug and left the shape that produced it: every tab was a `string`, so every tab was substitutable for every other one, and `BrowserClickRef(id string, ref int)` would take the user's tab as happily as the agent's.

`AgentTabID` is now a distinct type whose only constructor is `agentTab()`. An agent path cannot be handed the user's tab without an explicit `AgentTabID(...)` conversion — one greppable token a reviewer can ask about, instead of an ordinary argument nobody looks at twice.

It deliberately stops short of the plumbing underneath. `browserSnapshot` and `BrowserClickRef` keep plain ids because they genuinely serve both callers: "use this page as context" is the user pointing at their own tab, through the same function. What is protected is the decision of *which* tab, which is where the bug was, and the `string(id)` conversions all sit one line under the only call that can produce an `AgentTabID`.

The same change retired a piece of debt §127 had just created. `agentBrowserTabID` answered `""` and `agentBrowserTabTarget` answered an error, for one question, differing only in how they said no — the same shape as the `lastID`/`agentID` confusion they were both written to fix. `open` now reads the error as "there is nothing to steer, mint one"; everyone else shows it to the model.

### 129.2 An address bar searches; `open` refuses

`normalizeUrl` in TypeScript and `normalizeWorkbenchURL` in Go answered the same question with two sets of rules, and both ended by stamping `https://` onto whatever was left. So **ยูทูป** became `https://ยูทูป`, punycoded to `xn--o3cit6gb`, refused by DNS. Owner: *"อนนี้มันไม่ฉลาดพอจะค้นหรอ"* — it was never about intelligence; the address bar had one job where every browser's has two.

`resolveAddress` in `address.go` is the one classifier, and the doubt falls toward *search*: a mistaken search shows a results page somebody can act on, a mistaken navigation shows `ERR_NAME_NOT_RESOLVED` and a punycode hostname.

**The policy stays with the callers, and they differ on purpose:**

- **The address bar searches.** Someone who types a word into an address bar wants to search.
- **`open` refuses and names `web_search`.** The agent already has a search tool. An `open` that quietly searched would teach it that `open` is a search box — it would keep reaching for the wrong tool and keep getting away with it.

That is not a second place answering one question, and the test for the difference is direct: changing how text is classified must change both callers, while changing what the address bar does with a search must change neither the agent nor `address.go`.

Google, as one named constant, on the ground that outweighs the rest for a Thai-first product: Thai queries come back materially better. Surfacing it as a setting is a frontend job and deliberately not in the same change as the plumbing.

### 129.3 A byproduct is not a deliverable

`writeBrowserShot` bypassed `a.outputSubdir()` and was right to — but "right, with a paragraph of reasoning at one call site" is how a second place gets born, because the next tool to produce a byproduct copies the reasoning rather than the function.

So the concept has a name. `workFileDir()` answers "where does a file the agent made *while working* go", which is a different question from `outputSubdir()`'s "where does a new file go". A deliverable belongs in the project when one is focused. A screenshot taken to see something does not, and `page-1.png` in the root of somebody's repository is a change nobody asked for.

### 129.4 What is left

The `400ms` settle before a capture is still a guess, and stays one until a picture has been taken on a real window. Punycode in tab labels still shows as `xn--`: decoding it is fifty lines of RFC 3492 for a tab title, and was not worth smuggling into this change.

## 130. Decision — One Rule Was Two, and Only One of Them Was Load-Bearing (2026-08-17)

**Status:** Implemented 2026-08-17 · **Files:** `desktop/browser_tabs.go`, `desktop/browser.go`, `desktop/browser_tool.go`, `desktop/workbench.go`, `internal/skill/packed.go`

The agent found this itself, being tested: *"ยังไม่พบความสามารถสร้างแท็บใหม่ ... เปิดหลายแท็บพร้อมกัน ... สลับระหว่างแท็บ"*. Owner: *"ไม่ควรผูกหน้าอะไรกับเอเจนดิ หรือควรทำเหมือน skill ที่ห่อแท็ป แบบนั้นมันจะได้ดูได้"*.

### 130.1 The two rules

"The agent has one tab" was two rules wearing one coat, and separating them is the whole decision:

- **Ownership** — the agent works tabs it opened; the user's are the user's. This is what §127 spent a week getting right, and it is untouched. The `web-agent-` prefix separates the two at any number.
- **Count** — there is one of them. This came from fixing the tab-after-tab strandedness of 2026-08-10, and it fixed that by *removing the ability* rather than by managing it.

Only the first was ever load-bearing. `agentID` being singular was an implementation detail that had been reading as a principle.

So `browserHost` now carries `agentOrder` beside `agentID`: "which are mine" and "which of mine am I working" are different questions, the same way `lastID` and `agentID` are. **Reuse stays the default** — a plain `open` still replaces the current page, and a session that never asks for a second tab behaves exactly as it did, because strandedness was the real thing the count rule was protecting against.

### 130.2 The hazard, named rather than engineered around

refs are `data-aetox-ref` attributes textScript stamps into the page. They were always per-tab; what changes is that the agent can now be somewhere else than where it read. `select` is a page change like any other, and the description says so in the words it already used: read, act, read again. Playwright MCP publishes the same warning about its own tab indexes, which is some comfort that there is no cleverer answer waiting.

### 130.3 `tabs` is a right of its own

`browser_tabs` joins `browser_capture` in the permission vocabulary rather than riding on `browser_open`. It is the only action that can make a page **disappear** from under the user, and a profile granting "may look at pages" should not be granting "may close them" by implication.

The refusal for a tab the agent does not own says *whose* it is, not that it is unknown. "Unknown tab" makes a model try another id; "that one is the user's" makes it stop.

### 130.4 What it cost, measured

The tool block was at 9,941 of its 10,100 tokens before this. `tabs` plus `open`'s `newTab` came to +111 after trimming `browser`'s own prose to pay for part of it — the tool that grows pays for its growth, rather than borrowing from a neighbour that did not.

```
before   9,941 / 10,100   (browser ~470)
after   10,052 / 10,100   (browser ~581)   headroom 48
```

**Forty-eight tokens is not room for anything.** The next addition anywhere in the block has to buy space first, and the obvious seller is `task` at ~1,568 tokens — three times `browser` for one mechanism. Not touched here, because trimming another tool's description is that tool's decision and nobody asked for it.

### 130.5 What this deliberately does not do

Let the agent read or drive the **user's** tabs. §127.3 has the mechanical reason (a ref read from one page, spent on another, resolves against a document it was never numbered for), and there is a better one: the gesture for handing the agent a page already exists — dragging a browser tab into the composer attaches its content. That is a **give**, not a reach, and it needs no new mechanism.

## 131. Decision — Three Tools That Remove a Dead End, Not Three That Add a Feature (2026-08-17)

**Status:** Implemented 2026-08-17 · **Files:** `desktop/browser_back.go`, `desktop/browser_wait.go`, `desktop/browser.go`, `desktop/browser_windows.go`, `desktop/browser_tool.go`, `internal/skill/packed.go`

Owner set the problem: *"มันควรมี tool อะไรบ้างที่จะทำให้มันควบคุมเบาเซอร์ได้เต็ม 100% เหมือนคน"*, with the rule attached — *"เรากำลังคิดโจทย์ทำ tool ไม่ใช่พรอบสอนให้มันเป็นเครื่องทำตามสั่ง"*.

### 131.1 The test is not "can it do what a person does"

Half of what a person does at a browser compensates for having only eyes and a mouse. Scrolling exists because a person sees one screenful; `read` returns the page. Copying "everything a human can do" would build the workarounds along with the capabilities.

The test used instead was **no dead ends**: no situation where the page does something and the agent cannot continue and cannot tell why. That reordered the list immediately, and put three things at the top that are not features at all — they are places the agent currently falls in a hole.

### 131.2 back, and why it is the smallest and most important

`open` was destructive. The page you were on was gone, and the only way back was to open its URL again — which is not the same page: a half-filled form is empty, a scroll is lost, a POST result cannot be re-fetched, and a results list may have moved. So before every `open` the agent had to **predict** whether it would want the current page later.

The first attempt at this was a better sentence in the tool description, telling the model when to prefer `newTab`. The owner rejected the whole approach — *"ไม่ใช่ไปพรอบบอกมัน แต่ทำให้ tool ทำให้มันสามารถทำได้"* — and was right: **a prediction is the one thing a model cannot be instructed into doing well.**

`back` removes the prediction rather than coaching it. Being wrong costs one action instead of a page. The evidence that this is the better kind of fix is that the description got *shorter* when it landed: the tool absorbed what the prose had been trying to teach.

A tab with nothing behind it does not fail. `history.back()` is silent there, so the timeout **is** the answer, and it says "there is nowhere back to" — a fact to act on — rather than "the page did not finish loading", which would send the agent hunting a network problem that does not exist.

### 131.3 wait, and the failure that does not look like one

`open` waits for a navigation. For a server-rendered page that is the whole story; for anything built this decade it is the beginning of one, because the document arrives and then scripts fetch the actual content.

An agent that reads immediately reads an empty shell — **successfully**. Nothing in the answer suggests waiting. The page genuinely has no results in it yet, so the model concludes there are none, and reports that to the user. This is the same shape as §128.4: a true sentence producing a false belief.

`wait` is the only thing in the tool that can tell **"not yet" from "not there"**, and that is the line the description leads with.

The condition is text, not a CSS selector, because a model reliably knows what it expects to *see* and unreliably knows what a page calls its own divs. And a wait that finds nothing is not an error — it is a report, and it says both possibilities out loud rather than picking one.

### 131.4 dialog, and a default that is a safety position

`alert()`, `confirm()` and `prompt()` stop a page until somebody answers, and nobody could: the agent has no hands for a native dialog. A page that raised one hung every later action until its timeout and reported that the page was not responding. True, and useless.

The three are replaced at document creation, so a dialog can no longer block anything. The page gets an answer immediately from a standing policy, and what was said is recorded for the next answer the agent receives.

**The default is dismiss.** A `confirm()` in front of a destructive action is the commonest kind there is, and answering yes by default would make the browser quietly agree to things on the user's behalf. Saying yes has to be something the agent chose, one page at a time. `browser_dialog` is therefore its own right in the permission vocabulary — "may read pages" must never imply "may agree to the deletion".

What a dialog says is quoted back and never obeyed: it is text from a page, which this subsystem has treated as untrusted since [browser-security-2026-07-21](architecture/browser-security-2026-07-21.md).

### 131.5 The one refused: `eval`

Arbitrary JavaScript subsumes every action above, in one line, for almost no tokens. It is the lazy answer that looks like 100%.

It is refused because rights in this product come from a list the user can see, and **`eval` is a single right containing every other right invisibly.** A user granting it cannot know what they granted; an audit line reading "ran JS" says nothing. `click`, `type`, `wait` and `dialog` can each be described, judged and refused individually. That is the entire reason there are several of them instead of one.

### 131.6 What it cost, and the honesty problem in that number

The block went from ~10,024 to ~10,237 tokens, over its 10,100 ceiling, and the ceiling was raised to 10,400.

Every previous raise in `tool_budget_test.go` was argued on the merits of what it bought. **This one was not argued at all.** Owner, mid-implementation: *"เรื่องโทเค็น ไม่ต้องห่วง เดะค่อยคิดสถาปัตยกรรมใหม่มาจัดการมัน"*. That is a legitimate call — the ceiling stops being the mechanism — but until the replacement exists there is no honest number to argue for, so the constant is now a debt marker rather than a budget, and says so where it is defined.

A passing budget test is no longer evidence the cost is under control. When the replacement is designed, `task` at ~1,568 tokens — three times the browser, for one mechanism — is where it should start looking.

### 131.7 Still not built, in the order they were ranked

`upload` (a file input cannot be filled any other way, and it is what "put this receipt into the system" needs), downloads landing somewhere named (`workFileDir` already exists for it), `scroll` (both for lazy-loaded content and so `capture` can see past the fold), `key` for Escape and Tab, `hover`, iframes, and the page's own console errors — which are the §128.4 argument one more time: the page is complaining and nothing carries it out.

## 132. Decision — A Tool Block Carries What Exists, Not What to Think (2026-08-18)

**Status:** Standard adopted, mechanism implemented, migration not started · **Files:** `internal/skill/guidance.go`, `internal/skill/dispatcher.go`, `internal/skill/block_standard_test.go`, `internal/skill/README.md` · **Working document:** [Existence, Signature, Judgment](https://claude.ai/code/artifact/c6bb1af6-4ed5-4122-acc5-624dc152f721)

Owner, 18 ส.ค., after §131 pushed the block over its ceiling with no argument for it: *"เรามาออกแบบสถาปัตยกรรม การจัดการ tool กันดีกว่าไหม"*, then on the result: *"เอาแบบนี้เป็นมาตรฐานเลยไหม เผื่ออนาคตเพิ่ม tool อื่นๆ"*.

### 132.1 Measured first, because the obvious design was wrong

From `tool_runs` on a real machine — 68 sessions, 1,883 calls:

- a session uses **5 distinct tools** at the median (p75 8, p90 15) while the block ships **38**
- **no tool is used by even half of sessions.** The most-used, `list`, reaches 45%

The plan going in was to tier by frequency: ship the popular ones, load the rest. The third number killed it. Shipping the top 20 of 38 still leaves **55% of sessions** needing something else, and the top 10 leaves 85%. There is no core set to find, because sessions differ by *subject*, not by intensity.

### 132.2 The cut that works is lifetime

A tool definition carries three separable things, and they were bundled only because nothing had asked them to be separate:

| Layer | Needed | Weight on `browser` |
|---|---|---|
| **Existence** — that the capability is here | always | ~10 tok (1%) |
| **Signature** — what to pass | when calling | ~70 tok (9%) |
| **Judgment** — when to reach for it, hazards, cost | **once** | ~686 tok (**90%**) |

The heaviest layer by an order of magnitude is the one needed a single time, and it was re-sent with every message for the life of the conversation. All of it is prose written in good faith — §131 added most of the browser's — and none of it was wrong. What was wrong is when it travelled.

### 132.3 The honest counter-argument, recorded because it nearly wins

The block sits at the head of the request and is prompt-cached, so the real price per message is well below what 10,237 tokens suggests. **On money alone this work barely pays.**

It is worth doing because a cache returns the price and not the **space**. Those tokens still occupy the context window, competing with the files, pages and history the actual work needs. Every conversation that gets summarised because it filled up paid for that summary partly in the descriptions of 33 tools it never called.

### 132.4 Delivery costs no round trip, which is the part that is ours

Guidance rides on the **first result of the session** — a call that was going to happen anyway — rather than being fetched. Anthropic's own `defer_loading` / tool-search and the MCP progressive-loading patterns spend a lookup to pull a schema in; this spends nothing. It teaches on a failed first call too, because a call that went wrong is exactly when the judgment was missing.

Aetox could not have used the provider's mechanism regardless: `defer_loading` is an Anthropic API feature and this app is not single-provider. The layering has to live in `internal/skill`.

The trade, stated plainly: the **first** call of a session is made without judgment. Signature has to carry it, which is why the standard puts parameter names in the block and prose out of it, and why a wrong first call gets the guidance back as its error — a loop that closes in one round trip rather than one that never closes.

### 132.5 It changes delivery, not grant

The rule this had to answer is the one about hidden doors: rights come only from a list the user can see. **Nothing here touches which tools exist for a caller.** Every capability stays in the block, by name, always; a tool the user did not grant is still absent, and a connection without an account is still absent. What moved is where the JSON schema and the prose sit. Bytes, not permission.

And the project had already decided this once. The skill system is built on *a skill's body is never in your context until you ask* — which is why a skill is the right home for knowledge and a prompt layer is not. This is that rule, applied to tools.

### 132.6 A ratchet, because a standard that starts red gets deleted

`block_standard_test.go` enforces 80 tokens for a plain tool and 65 + 10 per action for a packed one. Thirty-odd tools are over it today, so a plain ceiling would have failed on the day it was written.

Instead: a tool **not** on the exemption list must fit — that is every future tool, and it is where the standard bites — and a tool on the list may shrink but never grow. The list is a to-do list that empties itself, and the diff is the progress bar.

The first draft of that list was written from estimate rather than measurement, and every number in it was wrong. The ratchet caught it on its first run, which is a fair advertisement for the mechanism.

### 132.7 Not done

The migration itself: no tool has moved its prose into `Guidance()` yet. `task` (1,568) and `browser` (766) are the two largest and live in the desktop package, pinned separately by `desktop/tool_budget_test.go`.

Two open risks are recorded in the working document and neither is answered. **Guidance sits in the message stream, not the cached head, so a conversation that gets summarised can lose it** — leaving the model with signature and no judgment, silently; re-teaching after a compaction needs a design. And the **first-call quality** question should be settled with numbers rather than opinion: `tool_runs` already has an `ok` column, so the baseline exists to be measured before the migration and compared after.

## 133. Decision — Unknown Is an Answer, and the Meter Has to Be Able to Say It (2026-08-18)

**Status:** Shipped · **Files:** `internal/model/context_window.go`, `internal/model/context_window_test.go`, `desktop/app.go`, `desktop/usage.go`, `desktop/app_test.go`, `desktop/frontend/src/lib/Chat.svelte`, `desktop/frontend/src/test/contextMeter.test.ts`, three locale files

Owner, 18 ส.ค., on the composer's context meter reading `11.4k / 32.0k (36%)`: *"อันนี้มันวัดจริงมั้ยเนี้ย"*, then on the answer: *"ผมว่ามันคือหนี้ในระบบ"*.

### 133.1 The numerator was honest, the denominator was invented

The meter's two halves came from different places and only one of them was a measurement.

`32.0k` was not the model's window. It was `memory.defaultMaxChars` (128,000) divided by four, reached because `App.contextWindowTokens` had a third fallback: when the catalog and the curated tables both answered 0, it read **the agent's own char budget** and converted it. Those are two different facts wearing one label. The budget is how much history Aetox keeps before it summarizes; the window is how much the model will accept. Nothing connects them.

Codex hit that fallback because it has no entry of its own anywhere: models.dev files these models under `openai` (0 rows begin with `codex/`), and `ContextWindowTokens` had no `case "codex"`. So every Codex user was shown a flat 32,000-token window.

### 133.2 The install's own history disproved the number it was drawing

From `token_usage` on the owner's machine, rows where the provider column reads `codex`:

| | tokens |
|---|---|
| largest round Codex accepted | **43,434** |
| rounds above the displayed 32,000 window | **9** |
| same model, `openai/gpt-5.6-luna` in the cached catalog | **1,050,000** |

One round at 52,609 was served with 50,944 of it from cache — the most expensive-looking request in the history was among the cheapest actually billed.

### 133.3 The UI could not show the state it was in, which is why it lasted

`ctxPct` was `Math.min(100, …)` and `slicePct` was not clamped at all. So a round past the stated window drew a comfortably full ring, a header reading the self-refuting `52.6k / 32.0k (100%)`, a `messages` segment 128% wide painting over the rows beneath it, and `พื้นที่ว่าง 0`. Nine rounds looked normal. The clamp was protecting the layout from a number that should have been sending someone to look.

### 133.4 The shorter fix was the wrong one

The one-line version is `codex → openai` in `modelsDevProvider`. It works and it is not safe: `ModelCatalog.For` is also what **prices** a model, so filing Codex under OpenAI would inherit OpenAI's per-token rates onto a flat monthly plan — the exact bill `token_usage.provider` was added to prevent (db.go, migration 15).

So the alias is scoped to the one question it answers, by recursion inside `ContextWindowTokens`, the way `openrouter` already resolves through its vendor. **One provider, two facts, and only one of them travels.**

### 133.5 What changed

- `case "codex"` resolves through `openai`: catalog first (1,050,000), curated table as the floor (400,000 for `gpt-5.1-codex`, which models.dev does not carry).
- The char-budget fallback is **deleted**. Unknown returns 0 and stays 0.
- `GetContextBreakdown` omits the `free` slice entirely when the window is unknown. "0 free" is a claim about a number nobody has, and it is the more alarming of the two available inventions.
- The meter drops the denominator rather than the meter: it reports the size of the request, and says the percentages are shares of that request rather than of a window.
- `ctxPct` no longer clamps; only the ring's arc does. A round past the window says so in words.
- `PriceModels` asks `ContextWindowTokens` for the context column instead of reading the catalog row beside the price, so the picker and the composer cannot answer "how big is this model" differently. They did, in opposite directions: blank in the picker, fabricated in the composer.

### 133.6 The bill was not only cosmetic

`bootstrap.ContextChars` feeds on the same lookup, so the 0 came back the other way too: the engine really was summarizing Codex sessions at 102,400 bytes. The fix moves that budget to 4,200,000. **A wrong number on screen and a real budget 33× too small, both from one missing case.**

### 133.7 Third time for this shape

§113 was a rate-limit window's *length* read as its *time remaining*. §108 was the catalog recording intentions and the UI reading them as facts. This is the same family: two facts with one name, and a caller that would rather answer wrongly than answer "unknown". The rule worth keeping is the one already written above `ModelCatalog.For` and now enforced one layer up — **callers must render unknown as unknown, and never as zero or as anything else they happen to have in hand.**

### 133.8 The budget is a guess, so the provider gets a vote

`ContextChars` converts a token window into a byte budget at a flat 4 bytes per token. That was harmless while the budget was pinned at 128,000; at 4,200,000 it is load-bearing. Measured against this install's own sessions, including tool output and not just chat text:

| session | bytes | peak prompt | **bytes/token** |
|---|---|---|---|
| 18 ส.ค. 01:30 | 20,882 | 17,445 | **3.45** |
| 16 ส.ค. 22:00 | 56,032 | 25,559 | **3.96** |
| 17 ส.ค. 21:53 | 53,359 | 21,177 | **5.46** |

So ×4 is a fair centre and a poor guarantee. The spread is ±25% and it tracks the mix of Thai prose against English tool output, which is a property of the task, not of the model. At 3.45 the 4,200,000-byte budget is 1.22M tokens against a 1,050,000 window: **a budget reading comfortably-under is already past what the model will take.**

Retuning the constant cannot fix that. A value low enough to be safe at 3.45 throws away a quarter of the window in every session that is not the worst case, and the worst case is not knowable in advance.

**So the constant stops being the guarantee.** `model.IsContextLengthError` recognizes the refusal in the vendors' own words (there is no typed error to switch on: every provider folds the 400's prose into a plain `fmt.Errorf`), and `Agent.compactNow` summarizes on the spot and retries the round, twice at most. Rate limits, spent balances and output truncation are deliberately excluded — three different failures, three different fixes, and summarizing helps none of them.

What this replaces is nothing at all. Before it, **no part of the engine reacted to a context-length rejection**: the turn died, the history that caused it stayed, and every retry the user typed by hand failed identically.

Also collapsed: `cmd/aetox/main.go` held its own `resolveContextChars`, byte-for-byte the same arithmetic, despite `ContextChars` carrying a doc comment saying it exists *because* the logic had been written twice. The CLI now calls the shared one.

### 133.9 Not done

The estimate in the meter is still `chars/4` and still unmeasured: about **20% high** for the tool block (11.4k predicted against 9,460 counted on a real first round), and low for Thai. `token_usage` already holds a real count for every round ever sent, so the ratio can be learned per model instead of assumed — which would make both the meter's forecast and `ContextChars` right by measurement rather than by luck. The retry above makes that a performance question rather than a correctness one, which is why it is not urgent.

---

## 134. Decision — Stop Means Stop, Reading Is Not Switching, and a Round Is Not a Queue (2026-08-18)

Three complaints from one session, all three about the same thing wearing different clothes: the window throwing away work the engine still had.

### 134.1 Stop was deleting the turn it stopped

Pressing Stop mid-work left one line — *"หยุดการทำงานแล้ว"* — over an empty chat. Every tool row, every line of narration between them, the thinking clock: gone from the screen the instant the turn ended.

It was lost twice over, in two different layers, and the first reading of this bug found only the second one. **The window threw the work away, and the engine had already thrown it away first.**

The engine half is the one that outlives a restart. `executeAgentToolLoop` ended every error path with `return Result{}, true, err` — so the sequence it had spent the whole turn assembling, each tool call and each line of narration between them, went out with the error. `appendFailedTurn` faithfully stored what it was handed, which was nothing: the row existed, it said *stopped*, and it held no record of the work. Reopening the conversation showed a question with nothing under it. The test that was supposed to cover this (`TestAFailedTurnIsWrittenDownWithItsReason`) passed throughout, because it exercises the conversation path, which returns its partial reply on error — the tool path, the one every real turn takes, was never asked.

The window half is the three things that had to line up, and did:

- `App.runTurn` returns the partial reply beside the error, and **Wails discards a bound method's return value whenever the error is non-nil**. So the window never receives what Go built.
- `turnEndedBubble` therefore fell back to `cockpit.streamingText` — which `discardAnswerPreview` empties at the end of **every round that ends in a tool call**. That is precisely the moment anybody presses Stop, so the fallback was empty by construction, not by accident.
- The bubble never took `turnArtifacts()` — the snapshot the successful path has always taken — and `runLiveTurn`'s `finally` cleared `toolSteps` one line later.

The fix is the same sentence at both layers: **a turn that was stopped did not do less work than one that finished; it did the same work and then someone ended it.** The executor returns `parts.all()` with the error, so the row is written with its sequence and a reopened chat draws the timeline of a stopped turn exactly as it draws a finished one. The bubble takes `turnArtifacts()` on both endings, so the screen keeps it without waiting for a reload.

Two details the second layer decided. `Result.Reply` on the error path is **the last text part, never the loop's `reply`** — that variable holds a cancelled tool's own output, which on a Stop is the string `context canceled`, and a bubble reading *"context canceled"* above *"หยุดการทำงานแล้ว"* is the app blaming itself for obeying. And the round that was *in flight* when the wall came is still not in the parts, because its prose never completed a round: the window's live preview is the only copy of that half-sentence, which is exactly why the frontend keeps its own and the engine keeps the rest.

The old test covered the case that almost never happens (Stop pressed mid-sentence, preview full) and not the one that always does (Stop pressed between rounds, preview empty). Three new ones pin the layers: the bubble keeps the timeline, the executor returns its parts with the error, and the row round-trips them through SQLite.

### 134.2 "One turn, one chat" was one word too wide

The gate (§ `guardSessionSwitch`) refused every door out of a running turn's chat with one sentence. The reasoning was sound for what it was protecting: the engine holds one agent context, and *opening* a session rewrites it — `LoadSession` restores the desk, the chair, the project and the history the running turn is thinking with.

But "open" was doing two jobs. **Reading a conversation touches none of that.** `SessionTranscript` reads rows and returns; it was written for the window that reloads mid-turn and needs its own transcript back, and it has never had a guard because it has never needed one.

So the door splits: reading is allowed, writing is not. While another chat is on screen, `cockpit.peek.live` holds the working chat's messages and every write the turn makes goes through `liveChat()` — the failure this replaces (an answer landing in whichever conversation the user had opened) cannot happen, because the turn no longer writes to "whatever is on screen". The live block is not drawn over a chat it does not belong to; the composer is disabled and says why; **Stop stays reachable**, because the turn it ends is running whether or not it is being watched.

The doors that genuinely re-root the engine — new session, new desk, another project — still refuse, and `cockpit.turnBusy` lost its now-false tail (*"แล้วค่อยสลับแชท"*).

### 134.3 A round carrying six reads was six rounds long

The model can ask for several tools in one round; every provider supports it and the DSML nudge already tells it to. The loop then ran them **strictly one after another**, so six 200 ms reads cost 1.2 s of wall clock for work with no order in it.

Consecutive parallel-safe calls now run together, capped at four, with results entering the context **in call order however they finished** — a provider handed its `tool_calls` back out of order is one that will eventually reject the request.

The load-bearing decision is the direction of the list. The obvious spelling — *run whatever `safety.AssessCommand` calls harmless* — is unusable: **that function's catch-all answers `RiskLow` with no effects for every name it does not recognize**, which is every MCP tool and every skill added after it was written. A deny-list would have parallelized a payment API the day somebody connected one. So an allow-list of this repo's own read tools, and everything else — shell, write, edit, git, task, all of MCP — keeps the sequential path. A write and the read after it are the one pair whose order *is* the answer, so a group also stops at any call that is not in the list rather than gathering the safe ones from across the round.

The media tools (`image_ocr`, `video_ocr`, `audio_transcribe`) are read-only and deliberately absent: they run ffmpeg and whisper, and four at once is a machine that stops answering, not a faster turn.

### 134.4 Not done, and named as not done

**One agent context per session.** All three of the above are the same shape — one engine, one turn, one live state, and a window that has to pretend otherwise — and the honest fix for the third complaint the owner actually made (*"สั่งงานหลายเซสชั่นก็ต้องได้"*) is per-session engines: a context each, live state keyed by session, every `agent:*` event stamped with the session it belongs to, and a decision about the things that really are shared (the workspace root, the shell backend, `ask_user`'s single channel, the delegate register). What shipped here is the read-only half, which removes the wall from the case that hurts daily. The other half is a piece of work, not a patch, and it is not pretended otherwise.

---

## 135. Decision — The Window Was Answering a Question Only the Engine Knows (2026-08-18)

Two things landed on the same day and they are one thing seen twice: a document writer that could not put a picture in a document, and a window that told the user a document had been deleted while it sat on their disk.

### 135.1 A picture is not the deck's idea

`slides_write` could embed a picture from the day it shipped. `doc_write` never could — `internal/ooxml/docx.go` did not contain the word *image* — so a twenty-screenshot appendix came back as twenty captions with square brackets where the figures should be, and ten minutes of the user inserting them by hand. The agent's own answer to being asked was correct and read as an excuse: *"เครื่องมือชุดที่ผมมีในเครื่องนี้สร้างรูปแทรกในไฟล์ไม่ได้เลย"*.

The picture, the byte cap, the "is this really a png" check and the aspect-ratio rule were all decided once, in the deck writer, and every one of them is a rule about **pictures** rather than about slides. So `SlideImage` became `ooxml.Picture` and the loader became `skill.loadPicture`, before either grew a second copy.

Three things in the docx side are load-bearing and none is visible in the result. `wp:inline` has a child *sequence*, so a reordered child is "unreadable content" rather than a differently-placed picture. `wp:docPr/@id` must be unique and non-zero — Word tolerates a repeat, Google Docs drops every picture after the first duplicate, which is how a twenty-figure appendix arrives holding one figure and no error anywhere. And `xmlns:r` goes on the document root, because `r:embed` is an attribute and an attribute cannot declare its own namespace.

A caption is not a small italic paragraph under a picture; it is Word's own `Caption` style, so Insert > Table of Figures finds it and so every caption in a document changes together. 10pt rather than Word's 9pt, because Thai stacks a tone mark above a vowel above the consonant and 9pt loses the top of that stack on paper.

**What the block standard did to this change, and why that is the point.** §132 landed the day before and pinned `doc_write` at 525 tokens with "it may shrink, not grow". Adding the picture put it at 576 and the ratchet refused. The fix was the one §132 prescribes: judgment out of the schema and into `Guidance()`, which is sent once. The tool ended at **509 tokens with a capability it did not have before**. A budget that forces a feature to pay for itself out of the block's own prose is doing exactly what it was built to do, on the first change that tested it.

### 135.2 "I was not allowed to look" is not "it is not there"

The same session, an hour later. The agent wrote a document to `D:\Mike\...\ภาคผนวก_ภาพหน้าจอ.docx` — legitimately: no project was focused, so `OpenSandbox` was on and the machine was the workspace. The window then drew a card saying **ไฟล์นี้ไม่อยู่แล้ว ถูกลบหรือย้ายไปหลังจากที่สร้างเสร็จ** and removed the button that would have opened it.

The desktop had its own copy of the containment rule, and the copy was worse in the way a second copy always is — it answered a question the original had already solved:

```
filepath.Join(`C:\Users\ASUS\aetox`, `D:\Mike\...\ภาคผนวก.docx`)
  ->  `C:\Users\ASUS\aetox\D:\Mike\...\ภาคผนวก.docx`   err = nil
```

An absolute path appended to the root. A path that cannot exist, that **passes the prefix check** because it genuinely is under the root, and that `os.Stat` then fails to find. Ten call sites in `desktop/` shared it. The engine's own `resolveSandboxPath` has never had this bug and its comment names the failure exactly — *"gets joined onto the root, and the tool answers about a folder nobody asked for"* — which is the clearest possible statement that the second copy should never have existed.

So `safeSandboxPath` now delegates to `skill.WorkspacePath`, the same gate every file tool answers through, with one difference and it is the whole reason there is a second entry point: **the window asks quietly.** A tool refused mid-turn may offer to add the folder and carry on; a pane working out whether to draw a button must not put a permission dialog on screen for a question nobody asked out loud. Two consequences come free: the window's reach is now the engine's reach, so it can no longer disown a file the agent just wrote, and the credential stores are refused on this path too — which the desktop's copy never checked at all.

**The worse half was the lie, and it was written down.** `FileStillThere` returned a bool, and its own comment defended folding permission into absence: *"A path outside the sandbox, or no project open, answers false — the same as missing, because in both cases there is nothing here to open."* That is a claim about the disk assembled out of a fact about permissions, and it is §133's family exactly: a caller that would rather answer wrongly than answer "unknown". Here it was worse than a wrong label, because the answer **removed the control that would have disproved it**.

It now answers three things. `there` when it looked and found; `gone` only when it looked and the file was not there; `unknown` when it never got to look — no project, outside the workspace, or a file the OS will not talk about, which includes the ordinary case of Word holding the document open. Only `gone` may take the offer away. `unknown` keeps the button, and the click answers with the truth.

The regression test is the user's own case, and it fails against the old code on the line that matters.
### 135.3 The window could read a Word file and the document agent could not

`read` refused a `.docx` with *"read target is a binary file — there is no text to read"*. True about the bytes, useless about the file: a Word document is nothing but text, and the agent being refused was the one whose profile promises to review drafts somebody sends it. Meanwhile the preview card in the same application had been pulling the words out of `.docx` and `.pptx` for weeks — the extractor was never wrong, it was one layer away from the only caller that needed it badly.

**No new tool.** A `docx_read` would cost every request in the block forever for a capacity most sessions never use, and would leave `read` still refusing — so the model's first attempt fails and the recovery depends on it noticing a sibling it was not thinking about. `read` is already the answer to "what is in this file". An Office file is a file. The block grew by the six words its description needed to stop lying.

**Structure, not prose, and that is what makes the next piece possible.** A document comes back numbered, each block named by the style the file itself carries:

```
รายงาน.docx — 18 block(s), 6 picture(s)
[3] Heading2      ภาคผนวก ก ภาพหน้าจอ
[4] image phone.png
[5] Caption       ภาพที่ ก-2 หน้าจอมือถือ
[13] table 3x2
```

The number is the point: it is how the next instruction names the thing to change. The style id is reported as the file spells it — `a3`, `a5`, whatever Thai Word generated — rather than normalised into something friendlier, because an agent about to change something has to name it the way the file does. It also answers a question nobody could ask before: a draft whose every heading reads `paragraph` has no structure at all however it looks on screen, and its author cannot generate a table of contents no matter what they try.

**A token walk, not a struct unmarshal**, because the only documents worth reading are ones Aetox did not write. WordprocessingML nests the same element names at several depths and every writer emits a different subset; a walk that tracks depth steps over what it does not recognise. Tested against four real documents on the owner's machine — 118 blocks, 24,000 characters, `w:proofErr`, `mc:AlternateContent`, auto-generated style ids — none of which this package would ever produce.

**What that testing found, which no synthetic fixture would have.** One of those documents reported five pictures and one block. It was not a parser bug: the file genuinely is a single paragraph carrying five inline drawings, which is what pasting screenshots in a row without pressing Enter produces. Reported as "an image", an agent asked to caption the figures writes one caption and believes it has finished. A block now says how many drawings it holds and names the ones that name themselves.

And the preview card now calls the same reader. Two walks over one format is two answers to "what does this file say", and the one that drifts is always the one nobody is looking at — 70 lines of the desktop's own extractor deleted rather than left to rot beside its replacement.

---

## 136. Decision — Two Queues, Because a Problem Is Not a Lesson (2026-08-18)

Owner, looking at "รอคุณตัดสิน" offering to remember that Tesseract is not installed: *"ไปดูตรงนี้มันจะจำข้อผิดพลาดทำไม"* — and, once the mechanism was traced and one leak patched: *"ไม่ได้บอกให้แก้ปัญหา แต่ มันชอบเอาปัญหาไปใส่ในช่อง เรียนรู้ … ควรจะมีแยก สำหรับเรียนรู้และ และปัญหาของระบบสิ"*.

The queue on that machine had held 22 cards in its life. Seventeen were written by the summarizer and every one of them said "เครื่องมือ X เคยล้มซ้ำ ๆ"; five of its six approvals were `exit status` clusters later judged garbage, and exactly one card in seventeen was a usable lesson. The other five came from the agent's own `memory` tool — Windows/shell, no Excel, the user is a developer, branch convention — and the owner approved three.

**77% of the learning queue was written by a source that cannot produce a lesson.** `summarizeFailures` reads `tool_runs WHERE ok = 0` and nothing else, so every filter added over the year — exit codes (§104), "ไม่สำเร็จ", state reports, missing programs — could only make it emit *fewer* problems. None could make it emit something about the user.

The rule was already written down, in the file that would have had to break it: `openIssueForm`'s comment says the product's problems and the user's lessons *"must never share a path"*. That governed the door a **human** opens. The door the **machine** opens, running unattended after every turn, poured into the queue that comment refuses to touch.

### 136.1 What moved

`pending_changes.kind` was `'memory'` on every row ever written; the summarizer's rows are now `'issue'`. That keeps the three properties this table already had and a problem list needs anyway — rows are never deleted, the body text is the dedup key, a thing turned down once never returns — while `ListPendingChanges`, `ListDecidedChanges` and `PendingLearnedCount` all gained the kind filter that makes the gear's badge true again.

Two cards, two verbs, and they are not the same question. **อนุมัติ** applies a change; **แจ้งปัญหานี้** applies nothing at all — it opens GitHub's own form, prefilled with the cluster, its count and the `tool_runs` rows behind it, in the user's browser, and `stateReported` records only that the problem was carried out the door. `ApprovePendingChange`'s `default:` branch was left exactly as it was: it is what guarantees a failure cluster can never be written into a memory file.

The body lost its tail. It used to end "— เลี่ยงรูปแบบที่ชนเงื่อนไขนี้ตั้งแต่ครั้งแรก", an instruction to the agent, which is what a lesson sounds like; nothing on a problem card teaches anyone anything. Migration 16 rewrites the old rows to the new wording *and* moves their kind, because the dedup key is (kind, scope, body) — left behind, sixteen decisions the owner already made would have re-opened on the new page on day one.

### 136.2 What this does not fix

The learning queue now goes quiet, fed only by the agent's own `memory` tool. That is the intended outcome and not a loss: it is the half with three approvals in five. The sources agreed in direction on 13 ส.ค. — corrections made mid-task, preferences said in passing, fail→success pairs, recurring requests as starter cards — remain design, not code.

And a tool that reports failure through its `Output` instead of returning an error still reaches `tool_runs` with `error_kind = ''` whatever its author knows, because `classifyToolError` reads the error *value*. `n8n_server_start`'s 90-second timeout and `task`'s "MCP not connected yet" are both weather and both unmarkable. They land on the problems page now, which is less wrong than landing in memory, and still not right.

Full design: [system-problems-vs-learning-2026-08-18.md](architecture/system-problems-vs-learning-2026-08-18.md).

---

## 137. Decision — Every Read Gets a Bound, and the Bound Lives in the Query (2026-08-18)

Asked while the per-session engine work was in flight: what happens when the database passes 1 GB. The answer is that file size is not the thing that slows SQLite down — rows read is — and the schema's indexes already cover every hot path. So the risk is not the size, it is the one read in this app that has no ceiling.

**The ceiling that exists**: the sidebar list is `LIMIT 200`, the Inspector's command history is 50, shell output is capped as it is written.

**The ceiling that does not**: `LoadSession` reads every row of a conversation and decodes every `parts` blob. Its cost is linear in the length of that one chat and has nothing to do with the size of the database. A 500-turn conversation is slow to open on a 24 MB database and equally slow on a 1 GB one.

Four rules, in the order they will be built:

1. **Measure with a fixture, not in production.** A seeded large database in the test suite — a 500-turn conversation, six figures of tool_runs — with a ceiling on how long opening the list and opening a chat may take. A feature that reads history without a bound then fails a test instead of reaching a user. Built first, because it is the instrument the rest are decided with.
2. **A conversation opens from its tail and pages upward.** Fifty turns, then fifty more on scroll, by keyset (`id < ?`) and never `OFFSET` — offset walks the rows it discards, so its cost grows with how far back you look, while a keyset seek into `idx_messages_session` costs the same at any depth. The index this needs already exists.
3. **tool_runs is a log, not a record.** It grows fastest because it stores what tools actually returned — file contents, page text — and nobody reads it back beyond a few days. It is currently kept forever, and nobody chose that: it is what came free when it was written. A retention policy (full output for N days, then trimmed to head and tail, row kept) is the owner's call, and it buys privacy at the same time as space.
4. **Split the heavy column only if the numbers ask.** `parts` is the fat in a message row, and rule 2 stops it being decoded for turns nobody is looking at. Moving it to a side table is a real door, deliberately left shut until a measurement knocks on it.

Ordered behind the per-session engine work (§134.4), and said so explicitly: the owner is waiting on that, and re-ordering the queue on my own initiative is the mistake that produced §134.2's half-feature.

---

## 138. Decision — "Installed" and "Findable" Are Two Claims (2026-08-18)

Owner, looking at the installer's own log after a clean v1.2.4 install: *"ทำไมบางตัวมี ติดตั้งไปพร้อมกัน ในตอนนี้ไม่ได้อ่ะครับ เช่นตัวอ่านภาพและอื่นๆอีก"*.

The log was telling the truth. poppler, ffmpeg and the starter speech model all reported `Skipping: already installed`, and all three were there and working — they unpack into `$INSTDIR` and `bundledBinary` looks next to Aetox's own exe, so the place the installer writes to and the place the code reads from are the same place by construction.

Tesseract was also installed, and also could not be used. `tesseract.exe` and `tha.traineddata` sat in `C:\Program Files\Tesseract-OCR` exactly as designed, and neither `HKLM` nor `HKCU` had that folder on `PATH` — the UB-Mannheim setup adds it from a checkbox, on a page that `/S` never draws. `image_ocr.go` ran the bare name `tesseract`, got `exec.ErrNotFound`, and told the owner to go and install what he already had.

**The claim that was never checked.** `bundled.go` carried it in prose: *"Tesseract is the exception and needs none of this: its own installer registers itself, so PATH finds it like any other program."* `internal/skill/README.md` repeated it. The bundling doc's own "Untested caveat" section had even written down the check that would have caught it — *confirm `tesseract --version` works afterward* — and that check had never been run. Two claims were being treated as one: **the installer put it on the disk**, which was true and verified, and **the program can find it**, which was neither.

**The fix, and why it is not `bundledBinary`.** `resolveTesseract` tries `PATH` first, then the same `$PROGRAMFILES64\Tesseract-OCR` that `project.nsi` checks before deciding whether to install at all — the two ends now name one address. The order is the reverse of `bundledBinary`'s, deliberately: poppler and ffmpeg are copies we unpacked and pinned, so ours should win over anything else on the machine, while Tesseract is an ordinary system-wide install and someone who put a particular one on `PATH` chose it. Falling back to the bare name when nothing is found stays load-bearing — it is what keeps a genuinely missing Tesseract arriving as `exec.ErrNotFound` and turning into install instructions, instead of a file-not-found about a path the user never typed. Editing the machine's `PATH` from the installer remains the other option and the worse one.

`video_ocr` had the same blindness in its own pre-flight `exec.LookPath` and now asks `tesseractAvailable()`, so a machine that can OCR no longer gets turned away before a single frame is extracted.

**What generalises.** Every install step in `project.nsi` earns two questions, not one. §133's family again, from the other side: there the caller answered "no" where it should have said "unknown"; here the code asked `PATH` a question `PATH` was never going to be able to answer, and read the silence as absence. The other three components escaped this only because they are unpacked into the directory the code already looks in — the property worth keeping is not "bundle everything", it is that **whoever writes the file and whoever reads it must name the same address, in one place**.

Left standing on purpose: `audio_transcribe` has the matching hole in a different shape. The installer ships the 31MB model and no `whisper-cli` to run it with, and `findBinary` resolves through `PATH` alone. Named here rather than fixed, because whether the installer should carry that binary is the owner's call, not a detail of this one.

---

## 139. Decision — A Proposal Is Not Memory, and the User's Own Words Are Not a Guess (2026-08-18)

Owner, with a screenshot of the review page carrying a red line across it: *"บั๊คมาแล้ว แล้วดู ทำไมข้อมูลแบบนี้มันไม่จำ มันจำแค่ตอนเราบอกตรงๆอ่ะ"*. Two faults in one picture, and they turn out to be the same fault seen from both ends: what the agent learns is decided in one place and applied in another, and neither end was checking what the other could see.

### The card that could never be approved

`Error: no remembered line contains "ผู้ใช้เป็นนักพัฒนาระบบ (system developer)"`, above a `REPLACE` card, above an empty สิ่งที่จำไว้แล้ว.

The sequence that produces it is the ordinary one, not a rare one. The agent proposes a line. The user corrects it two messages later. The agent revises the line it just proposed — and **a proposal is not memory**: nothing is, until somebody approves it, which is the guarantee this whole subsystem exists to make. So `old` named a line no file held, `MemoryTool` queued it without looking, and `learned.Apply` discovered it at the click. The card's only working button was ไม่เอา, which records the user refusing a fact they had just asked for.

`tool.go` already had the right instinct, written above the room check and applied to `add` alone: *"Refused here rather than at approval: a proposal that cannot be applied would sit in the user's review list looking like progress."* The same sentence covers `replace` and `remove` and was never extended to them. It is now, and the refusal names the op that would work — the line is not there, so adding it is the move, not revising it.

### The check whose stated reason named a caller that does not exist

The other half was pinned by a test: *"Editing something that is not there is an error the agent can act on, not a silent no-op it would read as success."* The error was right; the caller was imaginary. `Apply`'s own doc comment says it — *called from the approval path, never from a tool* — so the agent is not present, and the only person who could act on it holds one button meaning "do not learn this".

So `before` is now a **locator, not a precondition**. With the door closed above, what still reaches `Apply` stale is the race the design invites: the memory files are plain markdown the user is told to edit. A `replace` whose target is gone adds its body, because what the user approved is that memory should say it; a body already present changes nothing, so two windows clicking one card cannot double a line; a `remove` of something already forgotten is the state that was asked for. The full-scope failure still refuses and still leaves the row waiting, which is what `TestAFailedApplyDoesNotMarkTheProposalApproved` was really guarding.

### Why nothing was remembered until it was ordered

The second half of the owner's sentence is the more expensive one. He had told the agent who he was and what he had built, in the plainest terms available, and no proposal was made at all.

Nothing was broken. The summarizer stopped producing lessons at §136 and now writes only problems, so the **only** path from a fact to memory is the model choosing to call `memory` mid-turn — which makes that tool's description the whole policy. And the description was written entirely against one failure, a memory filling with restatements of the current task, and was silent on its opposite.

Two clauses did it, both aimed at the agent's own guesses and neither saying so:

- *"Keep something you **worked out** across sessions"* — a source test the user's own sentence fails.
- *"Not worth keeping … anything you have **not actually seen borne out**"* — an evidence test that has the agent waiting for corroboration of a fact whose only possible source has just spoken.

Between them the description argued *against* the one case a user would consider obvious. It also gated the worth-keeping clause on *"would change what you do"*, which a biographical fact fails on a plain reading and passes on a real one: who you are talking to changes register, assumptions about expertise, and whose project this is.

The rewrite states what generalises rather than the case that exposed it — **the bar is whether a line stays true and stays worth carrying, never where it came from** — and says outright that a fact the user states about themselves is already the evidence for it, so it is proposed when it is said rather than when it is ordered. *"would change what you do"* survives, attached to the setup half where it belongs and where it does its work. The cost sentence is what holds the other side; it always was, and it is what §132's ratchet weighs. The block absorbed the change without a raise.

**What generalises.** A description that only ever answered "what should this tool refuse" will get the refusing right and the reaching never — the failures you write against are the only ones you see. And a check placed where the wrong party is standing is not a check: `Apply` refused correctly and told nobody who could do anything about it, for as long as it took a user to send a screenshot.

---

## 140. Decision — Name a File Only Where the User Named It (2026-08-18)

Found while fixing §139, in the test that had been guarding against it the whole time. `TestOpenSandboxSaysTheWorkingFolderIsNotTheUsersHome` was red on the owner's machine and green in CI, and it was right both times.

`layer()` headed every folded file with its **absolute path**, so a session with anything to fold sent this on every request:

```
# What you have learned and the user approved (C:\Users\<name>\AppData\Roaming\aetox\memory\MEMORY.md)
```

That is the same charge exactly — the one `environment()` was rewritten to stop paying for the sandbox root — arriving by a different door. It went unseen longer because it only exists once the user has a memory or an identity file at all, which no fresh machine does.

### Why the test could not see it

Thirty-six of the forty-two tests in `internal/prompt` never set `AETOX_DATA_ROOT`, so they read the developer's real Aetox and folded in whatever it had learned. On a fresh checkout that folder is empty, so they passed; in CI it is empty, so they passed; on this machine they passed too, until the day §139's approval put a line in the file and the assertion finally had a string to find. A `TestMain` now gives the whole package one empty data root, because "remember to isolate" is not a mechanism.

That is the more general finding. A test that reads real user state is not merely fragile — it is **an assertion evaluated against a different input on every machine**, and the machine where it happens to be true is the one that decides whether anyone hears about it.

### The line, and the half-fix that was rejected on the way

First attempt printed `filepath.Base(path)` and called it done. It is not: the project memory file is `projects/<name>-<hash>.md`, and the hash is over the absolute project root — so the same repository cloned to a different folder produces a different header. Removing a path and putting a machine-varying token back in its place is half a leak, which is a leak.

The line that settles it is **who chose the name**:

| layer | named by | header |
|---|---|---|
| `Personal instructions` | the user | `(context.md)` — several files, and they may refer to one by name |
| `Project rules` | the user | `(AETOX.md)` — which of the three conventions this repo uses |
| the three memory scopes | Aetox | nothing — each title already names its own scope |

A name the user chose reads the same on every machine and is worth its tokens. A name Aetox generated says nothing the title did not, and one of them is not even stable across clones. Every path is still returned in `Loaded`, which is where the settings badge reads them; nothing on the model's side needs one, because memory is written through the `memory` tool and an approval, and the prompt's own standing rule is to repeat a path a tool reported rather than assemble one.

**What generalises.** Two questions, not one, before anything machine-specific goes into a prompt that ships on every request: does the model need this, and would it read the same on somebody else's machine. `layer()` had never been asked either — the path was there because it was what the caller happened to be holding.

---

## 141. Decision — A Rule Suppressed Twenty Times Was Never Chosen (2026-08-18)

**Trigger:** *"ติดตั้ง golangci-lint แล้วรันกับโปรเจกต์ Aetox … สรุปปัญหาหลักๆ ที่ควรแก้ก่อน"* then, on the plan: *"วางแผนแก้เลยครับอย่าสร้างหนี้ในเทคนิคนะครับ"* (2026-08-18).

**Status:** Approved & done 2026-08-18.

The tree had never been through anything but `go vet`. The first full run — golangci-lint v2.12.2, 23 linters including `gosec` — returned **1,326 findings**. That number is the subject of this entry, because almost none of it was true, and the shape of the untruth is what decided the configuration.

### 141.1 What 1,326 actually was

| | count |
|---|---|
| total | 1,326 |
| in shipping code | 710 |
| in `_test.go` | 616 |

`gosec` alone was 642, and 158 of its 180 file-permission findings were test fixtures writing `0644` into a `t.TempDir()`. Of what remained, the large categories are the program working as designed: `G304` (open a file named by a variable) and `G204` (run a command built at runtime) are not incidental to Aetox, they are the product. A tool that reads your files and runs your commands cannot be linted for reading files and running commands.

Every finding that looked security-shaped was opened and read. **All of them were false**, and the reasons are worth recording because each was a rule mistaking a house pattern for a defect:

- **`G202` SQL injection ×4** — the concatenated fragment is `?,?,?` built from a count, or a clause from `DeskFilter.where()` which emits placeholders and returns its args separately. The FTS `match` string is escaped *and* bound. The code was already right.
- **`G101` hardcoded credentials ×19** — provider names in `internal/provider/catalog.go`, the `Credit` line in `internal/version`, and the Codex **public** client ID, which is public by construction.
- **`G110` decompression bomb ×2** — both deliberate and commented. `zip_install.go` says so outright (*"Unbounded on purpose. The archive is a local file the user picked"*), and `update/apply.go` verifies an ed25519 signature and a SHA-256 at lines 183–186, before it extracts at 277.
- **`G602` slice out of range ×1** — guarded by `if idx+1 >= len(rawArgs)` on the line above.
- **`G115` integer overflow ×7** — includes `byte(c), byte(c>>8)`, which *is* how you split a UTF-16 code unit, and `uint32(code)` in `abnormalExit`, whose comment already explains that reinterpreting a negative exit code as an NTSTATUS is the point.
- **SHA-1 in `config.ProjectKey`** — not cryptography. It names a directory, and its own comment says two implementations must agree to the byte. Changing the algorithm renames every existing user's history and memory folder. The finding is correct about the primitive and wrong about the question.

### 141.2 The rule that decides configuration

Nine of the fifteen `ST1005` findings fire on user-facing Thai error strings beginning with `Windmill`. `GitHub`, `ChatGPT`, `PDF`, `API` all pass the same check — not by design, but because staticcheck skips a word carrying a second capital. One house pattern, "product name, then the Thai sentence", and the rule fires on the single member of it that happens to be spelled with one capital letter. Two of the remaining six are test fixtures reproducing a real WebView2 and a real `net/http` error verbatim, where changing the string is changing the input.

Lowercasing nine would corrupt the UI. Annotating fifteen would put fifteen suppressions into a tree that has **zero** `//nolint` and exactly one `//lint:ignore` (`desktop/artifacts.go:239`, which carries a prose reason, as anything here must).

So: **`ST1005` is off, once, in `.golangci.yml`, with the asymmetry written beside it.** That is the same rule as everywhere else in this repo — a second place answering the same question is หนี้ในระบบ — applied to lint configuration. A rule you would have to suppress twenty times is not a rule you have adopted; it is a rule you have declined and not yet said so. Say it once, in the place that configures rules, rather than twenty times in the places that do not.

### 141.3 What the linter was actually worth

Three real things, and none of them is a style violation.

**`rows.Err()` unchecked at 23 query sites in `desktop/`.** A query that fails mid-iteration returns a short list and no error. In an app whose subject is session history, that is history quietly coming back incomplete. The same line usually carried a second swallow — `if rows.Scan(...) == nil`, dropping unscannable rows just as silently. This is not a new discovery: §6.7 of [ARCHITECTURE.md](../ARCHITECTURE.md) already records `SearchSessions` returning zero results forever because the error went into the floor, *"which is why the bug was invisible outside the failing test."* Twenty-three copies of a defect are one defect, so the fix is one `eachRow` helper owning `Query` / `defer Close` / `Next` / `Err`, and a convention test in the mould of `TestBindingsNeverReturnNilSlices` so the twenty-fourth cannot be written. It also retires the bare `rows.Close()` at `summarize.go:145` for free: the intent there was to release the cursor before the write phase, which is exactly where the helper's `defer` now fires.

**Two `exec.Command` sites that orphan grandchildren.** `noctx` flagged `internal/mcp/client.go:159` and `internal/lsp/lsp.go:184`, and the more interesting corroboration was already in the file — a `ponytail:` note at the MCP site saying *"A server that forks (npx→node, uvx→python) can still orphan grandchildren"*, and §23's own **Not done** list naming `proc.KillOnCancel` on the MCP sites. The linter did not find something unknown; it found the thing the author had written down, and put a number on how long it had been true. See §141.4.

**A reflected XSS on the OAuth loopback page.** `error_description` from the redirect query string went into HTML through `fmt.Fprintf`. Localhost-only and short-lived, so the exposure is small, but `html.EscapeString` costs one call and a test, and "small" is not a reason.

The rest of the real work was six unused symbols, two dead assignments, a `for` loop at `cmd/aetox/main.go:690` whose every path returns — its shape promising a back-out to the provider menu that `pickModelForProvider` has no way to signal — and two `Close()` calls on write paths, in `internal/audit` and `internal/rtk`, where a failed flush was reported as success. The audit log is the worst possible place for a silent write failure.

### 141.4 Why the process leak survived a Job Object

§24's Job Object kills every descendant when Aetox exits, which is why this never showed up as a bug report. The gap it leaves is stated on `proc.KillOnCancel` itself: *"§24's Job Object only reaps them when the app itself exits, which is far too late for a user who pressed Stop."*

Every mid-life teardown leaked. `TestMCPServer` in Settings closes one client and opens another — press it five times against an `npx` server and five `node.exe` remain. `rebuildMCP` closes every client on any add or remove. `lsp.CloseShared()` runs on model and project switch, which is precisely when `tsserver` loses the `typescript-language-server` that spawned it. Off Windows there is no job object at all (`job_other.go` returns `false`), so the same paths leak at ordinary exit too, and the CLI calls neither `Close`.

Two traps made this more than a one-line change. `KillOnCancel` sets `cmd.Cancel`, and `Cmd.Start` refuses a `Cancel` on a command not built by `CommandContext`; worse, `exec.CommandContext(context.Background(), …)` compiles, starts, and silently kills nothing, because the watcher goroutine does not start on a context that can never be done. And the context in scope at both spawn sites is the wrong lifetime — per-call, not per-server — so binding it would kill the server when the first tool call returned. Both now hold their own `context.WithCancel`, cancelled by the thing that owns the process.

MCP needed one more piece. The SDK owns the `cmd`, and its `Close` does the spec's polite ladder — close stdin, wait, `SIGTERM`, wait, `Kill` — all of it aimed at the direct child, and once its `Wait()` returns, `cmd.Cancel` can never fire again. Cancelling after the polite close is dead code; cancelling before it skips the shutdown the protocol prescribes. So `internal/proc` gains a third verb beside `KillTreeOnExit` (die when the app does) and `KillOnCancel` (die when the context does): **`KillTree(pid)` — die now**, called after the graceful close to sweep what the ladder could not reach. On Windows it exports the existing two-pass walk, whose second pass exists for exactly this case, an orphan still naming its dead parent. On Unix it is the negative-PID signal that `Setpgid` at spawn makes possible.

The root cause is not two forgotten calls. `TestEveryExecSiteHidesTheConsole` has scanned every `exec.Command` in the tree for `HideConsole` since the day a console window flashed; **nothing was watching for `KillOnCancel`**, so the two sites that needed it were the two nobody was told about. A sibling guard now exists, with the same opt-out marker for the sites that detach on purpose.

### 141.5 What gates, and what merely reports

A gate that is red on its first day is not a gate. The reasoning is already written on the `unix` job in `ci.yml`: *"a red run nobody can act on stops being read, and then the day Windows genuinely breaks it looks the same as the six days it did not."* Enabling all 23 linters would have started this one at roughly 600.

- **Gating** (each at zero once §141.3 landed): `govet`, `rowserrcheck`, `sqlclosecheck`, `bodyclose`, `ineffassign`, `wastedassign`, `unused`, `unconvert`, `misspell`, `copyloopvar`, `durationcheck`, `makezero`, `reassign`.
- **Reported, not gating:** `gosec` — the same device the `unix` jobs use, for the same reason. Security stays visible on every push without six hundred false positives holding the branch.
- **Not enabled yet, and this is the one place that says why:** `errcheck` (119, mostly `defer Close` on readers, which is the conventional idiom and would need an exclusion list argued line by line), `noctx` (104, of which 85 are `database/sql` calls that will be revisited whole when per-session engines land), `staticcheck`'s `ST` family (`ST1000` alone is 106 missing package comments), `nilerr` (24, all deliberate, several already carrying a comment explaining the fallback), `unparam`, `prealloc`, `gocritic`, `errorlint`.

Placement: `verify.sh` gains a `lint` stage that skips **loudly** when the binary is absent, following the `race` stage that learned that lesson the expensive way. `ci.yml` runs it in `windows` as a gate and in `unix` where the job's `continue-on-error` makes it a report — which is not redundancy but coverage, since a lint run on one GOOS never compiles the other's `_other.go` files, the same blind spot the Unix jobs were created to cover.

`release.yml` is deliberately untouched. A lint failure should not block a tag whose tests all pass.

### 141.6 Two failures the linters did not find, and the run did

Wiring the gate meant running `verify.sh` end to end, which had two stages red before any of this started. Both were confirmed against unmodified HEAD in a clean worktree before being touched, because "my change did not cause this" is a claim, not an assumption.

**`isolateUserDirs` isolated nothing that mattered.** Two MCP tests in `desktop/` failed under `-shuffle=on`, each seeing the other's servers, because both wrote to one `mcp-servers.json`. The helper they both call sets `APPDATA`, `LOCALAPPDATA`, `USERPROFILE`, `XDG_CONFIG_HOME` and `HOME` — and `config.DataRoot` reads `AETOX_DATA_ROOT` *before* it asks the OS for any of them. `TestMain` sets that variable once for the whole package, correctly and for a good reason it states in full: tests were rewriting the developer's real `model-preference.json`, so the app opened on a test model nobody had chosen. So every caller of `isolateUserDirs` shared one directory while reading like it had its own, and the five lines above the missing one made that look deliberate.

The fix is the sixth line, in the helper rather than at the twenty-seven call sites, for the reason `TestMain` gives about itself: the trap is invisible where it bites. It also surfaced a real leak it had been hiding — `TestEveryToolRunsThroughTheRealDispatcher` never closed the store it opened, which had not mattered while the file sat in a directory nobody asserted about, and fails immediately once it sits in a `t.TempDir()`.

**A hand-written copy of a Go struct, three fields out of date.** `svelte-check` had nine errors. Five were `Settings.svelte` reading `cost`, `pricedCalls` and `pricesFetched` off a locally declared `type UsageTotals` that did not have them — a copy of the Go struct, written by hand, and never updated when the cost columns were added to `usage.go`. What let it compile at all was the `as Usage` on the call: an assertion that the generated bindings match a shape maintained separately, made at exactly the point where the two had stopped matching.

That is this section's own subject arriving from the other side. `wailsjs/go/models.ts` is generated from the Go types and had all three fields the whole time; the local declaration was a second place answering the same question, and the cast was the mechanism by which it went unnoticed. Both are gone, and `UsageStats()` is now assigned without an assertion, which means the next field added to the Go struct is either used or unused — never silently absent.

The remaining four were an `Element.prototype.animate` mock in the test setup that lib.dom had outgrown (`replaceState`, and `ready`/`finished` resolving to `Promise<void>` where the interface says `Promise<Animation>`). Completed rather than widened through `unknown`: a mock that stands in for a browser API should fail when it stops resembling one, and `as unknown as Animation` would have bought silence on every future addition for the price of the two lines it saved.

### What this does not decide

- **Whether the eight deferred linters are ever enabled.** Each is a separate judgment against a real count, not a backlog item.
- **The 85 context-less `database/sql` calls.** Real, and the wrong size for this pass; they belong to the per-session engines work, done once rather than twice.
- **`db.Begin()` at three sites hand-rolling their own commit and rollback.** The same shape of root that `eachRow` is, and the same argument applies — but not in a change already touching twenty-three call sites.
- **Anything about the frontend.** `svelte-check --threshold error` remains the whole story there; no ESLint was added and none is proposed.

**What generalises.** A linter reports; it does not know the program. Every finding here that looked like a vulnerability was a rule recognising a shape and not a purpose, and the only way to tell them apart was to open the file — 1,326 findings produced perhaps a dozen true ones, and the dozen were worth the reading. The corollary is the title: when a rule fires many times on something deliberate, the response belongs in the configuration, once, with the reason attached, because twenty annotations are twenty places answering a question the config was built to answer.

---

## 142. Decision — A Tool That Cannot Read Must Say So, Not Return Something (2026-08-18)

**Trigger:** owner, after §141's lint pass, asking whether `image_ocr` reads Thai, Chinese and English — then, on the proposal: *"เห็นด้วย"* (2026-08-18).

**Status:** Approved & done 2026-08-18. Chinese support declined in the same breath: *"เค อังกฤษ ไทยก็พอ"*.

### 142.1 The question was about languages; the defect was about honesty

`image_ocr` runs Tesseract with a hardcoded `-l tha+eng`, and the machine carries `eng`, `osd` and `tha` because [project.nsi](../desktop/build/windows/installer/project.nsi) fetches exactly one language file. So the literal answer is Thai and English, and the owner's answer to whether that should change was that it should not.

The defect is what happens on the third case. Tesseract asked for `tha+eng` does not refuse a page of Chinese — it returns plausible Thai letters and **exit code 0**. A slip reading 转账成功 金额 1,250.00 元 came back as:

```
SEM AKIN ธร พ 1.250.00 7 ว
ข่า โข 8417 2026468188
ว 8 ทั โว «ซี 8829301
```

The digits are correct, because Arabic numerals are Arabic numerals in both images, and that is what makes this worse than a plain failure: `1.250.00` and `8829301` are right, so the nonsense around them reads as text that was genuinely on the page. The tool then reported success and handed it to the model with nothing said about it. This is §141's audit-log finding in a different room — a write that failed reported as a write that worked — with the difference that the reader here is a model that will answer a user from it.

### 142.2 Confidence, because the alternatives were measured and lost

**Tesseract's own script detector (OSD) was tried first and rejected on evidence.** `osd.traineddata` already ships with every install, so it would have cost nothing to run. It fails: on the half-Chinese image it exits 1 with *"Too few characters. Skipping this page"*, and on the Chinese page it reports `Script: Japanese` at confidence 0.50. A detector that gives up on sparse pages gives up on screenshots, which is most of what this tool is pointed at.

**Per-word confidence separates the cases cleanly.** Measured through the shipped code path:

| image | confidence | words |
|---|---|---|
| Thai, rendered sharp | 94.2 | 33 |
| the same Thai, downscaled then blown back up | 93.2 | 33 |
| half Thai, half Chinese | 79.1 | 12 |
| Chinese read through `tha+eng` | **47.7** | 17 |

The second row is the one that decides the design. Confidence measures how well a glyph matched the model, not how sharp the pixels were, so degrading the image cost 1.0 points where the wrong script cost 46. That is the separation needed: **unreadable is not the same as photographed badly**, and a check that could not tell those apart would have to be off by default.

An eyeball average over the same TSV put the Chinese page at 64, not 47.7, because it counted rows whose text is only whitespace and those score high. `meanWordConfidence` drops them, which is both more correct and, by luck, wider.

### 142.3 It warns; it never refuses

Four synthetic images are not a corpus. A real slip photographed at an angle, with glare, over JPEG, will score below anything measured here, and a threshold that refused would refuse those first. So `ocrLowConfidence = 70` produces a sentence appended under the text and nothing else:

> อ่านมาได้ไม่ค่อยมั่นใจ (48%) ข้อความข้างบนอาจไม่ตรงกับภาพ เครื่องมือนี้อ่านได้เฉพาะภาษาไทยกับอังกฤษ ถ้าภาพเป็นภาษาอื่นผลที่ได้จะไม่มีความหมาย

Naming the two languages is the actionable half. A reader told only that confidence is low retries the same unreadable page; one told which two languages exist stops. The sentence goes **after** the text, in the order a person looking at the image would meet them.

Raising it to 80 would also catch the half-and-half case at 79.1. That trade needs real photographs to settle, not more synthetic ones, and is written into the constant's comment rather than left as folklore.

**No rule mentions Chinese.** There is no `if script == chinese` anywhere in this change, which is the point: the tool reports how sure it is, and a model that can read `SEM AKIN ธร พ` alongside "48%" decides for itself. That is the same principle the system prompt is held to — teach what generalises, never hardcode the case that prompted it.

### 142.4 Cost, and the one new failure mode

`runTesseract` now writes `txt` and `tsv` to a temp basename instead of reading stdout, which is **one OCR pass, not two**: 182ms → 184ms over five runs. Reconstructing the text out of the TSV would have saved the file and lost Tesseract's own line breaking, which for Thai is already the weaker half of the output.

The temp directory is new plumbing and therefore a new way to leak, so `TestRunTesseractLeavesNoTempDirBehind` points `TMP`/`TEMP`/`TMPDIR` at a scratch root and asserts nothing named `aetox-ocr-*` survives. A malformed or missing TSV costs the confidence and not the text: the text is what was asked for, the number is the footnote.

`video_ocr` shares `runTesseract` and so inherits all of it, but reports **once for the clip**, weighted by word count. Per-frame percentages would bury the transcript, and weighting by words stops a frame holding two words from counting as much as one holding forty.

### 142.5 What made the tests worth writing

The rule lives in `appendConfidenceNote`, split out from both callers so it can be tested on a machine with no Tesseract — which is every CI runner. Asserting it through a real OCR run would have made the check skip exactly where it is most likely to regress, since the threshold is a number someone will eventually want to move.

Proven to fail, both directions: setting `ocrLowConfidence` to 0 turns `TestAppendConfidenceNoteWarnsBelowThreshold` red, and setting it to 100 turns `TestAppendConfidenceNoteSilentOnGoodReading` red on the two rows that represent real Thai.

### What this does not decide

- **Whether Chinese is ever added.** Declined for now. If it comes back the numbers are on the table: `tessdata_fast` is 2.4MB for `chi_sim` and 2.3MB for `chi_tra`, downloaded at install time exactly as `tha` already is, so the ~12MB installer figure does not move. Appending to `-l` is not the way to do it — Tesseract's multi-language mode trades accuracy, and Thai is already the noisier half. §32's `Provisioner`/`Requirement`/`Option` is the shape that fits.
- **The spaces Thai comes back with** (`โอ น เง ิ น`). Inherent to `tha.traineddata`, not a flag: `--psm` 3, 4, 6 and 11 all produce it identically. The content and every digit are correct.
- **The threshold's real-world value.** 70 is defensible against four images and nothing more.

---

## 143. Decision — The Filter Was Right and Nobody Had Signed the Forms (2026-08-19)

**Trigger:** owner, on a screenshot of four problem cards — *"ผมว่ามันฟ้องเยอะเกินไป ควรเป็นปัญหาที่เป้นปัญหาจริงๆสิ ... มันดีตอนพัฒนา แต่เราจะเสียหน้าตาตอนที่ปล่อยของ"* — plus *"ผมกดแจ้งปัญหานี้แล้วเงียบมาก"* and a request for the sponsor page's copy (2026-08-19).

**Status:** Approved & done 2026-08-19.

### 143.1 Four cards, four failures that were never anyone's behaviour

§141's split gave the problems room a filter with three answers: `exit` (a program the tool ran returned nonzero), `ErrorFromWorld` (a report about the machine), and unmarked (a refusal this codebase authored, which carries a remedy). Only the third becomes a card. The filter was correct and every one of the four cards on the owner's screen walked straight through it, because **nothing had marked them**.

They missed in two different ways, and the difference is the whole entry.

**A `*fs.PathError` was never ours to mark.** Two cards were `read` failing with `GetFileAttributesEx ... The system cannot find the file specified`. That error is written by the operating system; no author here had the chance to call `statereport.New` on it, and the filter's rule was "the author says what it is". So it is recognised by type instead, in `classifyToolError`, once, covering every file tool at the same moment. It cannot swallow a refusal: the sandbox denylist and every rule this codebase enforces are built with `errors.New`/`fmt.Errorf`, and `TestSandboxRefusalIsNotAWorldReport` is there to go red the day that stops being true.

**A soft failure has no error to mark at all.** The other two cards — n8n not answering within 90 seconds, an MCP server still connecting — came from tools that report failure as `Success:false` with a **nil error**, so the model reads a result instead of a crash. That choice is right and it leaves the classifier with nothing to classify. Hence `skill.Output.FromWorld`: the same statement `statereport` makes on an error value, said for the failures that deliberately do not have one. Two sites set it, both of which already said so in their own text — one ends with "start this job again in a moment".

Rows already written stay written. Nothing here deletes a card the user has not decided on; it stops new ones being made.

### 143.2 A confirmed cause is not the only cause

The report button did nothing at all. Measured first: the prefilled URL was **35,659 characters** against Windows' 32,767 command-line ceiling, because the budget capped 4,000 characters of *raw* log while Thai costs three bytes per character in UTF-8 and three characters per byte again once percent-encoded. Nine times. That is a real defect, it is fixed, and a test proves it — removing the budget produces a 148,754-character URL.

**It was not what was stopping the button.** `BrowserOpenURL` validates before it opens, and `ValidateAndSanitizeURL` refuses any URL containing `;|`$\<>*{}[]()~!` or whitespace. `encodeURIComponent` is specified not to encode `!~*'()`. One ordinary parenthesis was enough — and every problem card's reason has one, because `reason` reads "เกิด 3 ครั้ง (ตัวอย่างล่าสุดที่ล้ม: ...)". The length check never ran; the URL was rejected two lines earlier, on every attempt, no matter how short.

Both defects were invisible for the same reason: Wails logs the refusal to `f.logger.Error` and returns. Nothing propagates. The owner saw `ERR | Invalid URL shell metacharacters not allowed` only because they went looking in a log.

What generalises is not the encoding rule. It is that a cause can be **measured, reproduced, and fixed** and still not be the one that mattered — the first diagnosis here was quantitative, correct, and not the answer. The tell was available and unread: the button had never worked *at any length*, which no length-based explanation covers.

### 143.3 The tests could only be written where the answer was

Both fixes are covered by tests that fail without them, and the character check spells Wails' list out as characters rather than a regex so a failure names which one leaked (`expected [ '*', '(', ')', '~', '!' ] to deeply equal []`). The list is copied rather than imported: it lives inside a vendored package this side cannot reach, and a copy that drifts is caught the moment a real URL carries the drifted character.

What could **not** be tested from here is whether the agent acts on any of it. An attempt to drive the checks through the agent produced a table of four green rows in which three rows had not run: two OCR calls read a stale attachment rather than the images provided, one read a missing file once rather than three times, and one declined to call the tool at all. That last refusal was correct behaviour — it was asked to retry a credential-store path, and it should not — which means **a failure cluster cannot be produced by asking the agent to repeat itself**, and the test procedure, not the agent, was wrong.

The one measurement that survived is the useful one: a real Krungthai slip, photographed and attached by the owner, reads at **92.1** confidence over 141 words. §142 set its threshold at 70 against four synthetic images and said in writing that a real photographed slip would sit lower than anything measured. It does not. The threshold has more room than it was given credit for.

### 143.4 The sponsor page says what it can keep

Copy added in all three locales for what a supporter receives: a name on the project's page, kept rather than counted once, and an opening for working together later. It is paired with a row asking the supporter to make contact, because PromptPay tells the developer that a transfer happened and never who to thank — the credit is unpayable unless the person says. Declining to be named is offered in the same sentence.

The contact channel is the GitHub door the problem and feedback rows already use. No personal email was placed into a shipping build; that is the owner's to publish, not this change's to assume.

`SITE_URL` now points at the repository. The button that read "ดูเว็บไซต์ Aetox" while opening GitHub would have been a small lie, so both buttons were relabelled: "ดูโปรเจกต์บน GitHub" for the repository and "วิธีสนับสนุน" for SPONSOR.md.

### What this does not decide

- **An agent asked for work outside its toolset still reports the downstream failure.** `sheet` and `doc` say "file not found" where the truth is "I have no tool that writes this". The owner kept it open deliberately: *"อย่ามองข้ามนะ เก้บมาคิดกัน"*. §143.1 stops those failures becoming cards; it does not make the agent honest about its own remit. Candidates raised and none chosen: the sub-agent's prompt, `FilterRegistry` refusing the brief up front, or the model's judgement.
- **Whether the model acts on a low-confidence OCR note.** §142's central bet, still unverified against a real model, and not verifiable by any prompt that mentions it.

---

## 144. Decision — The Way Back Cannot Be One Button Nobody Found (2026-08-19)

§134.2 opened a door §134's own gate had shut too wide: reading another conversation while a turn runs is allowed, because reading rewrites nothing. What it did not do is leave a way back that a user would find. The owner hit the result over and over and finally said so plainly (19 ส.ค.): *"ตอนสั่งงานเอเจนกำลังทำงาน พอสลับเซสชั่นปุ๊บ งานเอเจนหายไปเลย เหมือนมันผูกกับหน้าที่ผู้ใช้เปิด ความจริงไม่ควร"* — and then, when the first answer was a diagnosis instead of a fix: *"หลายรอบแล้ว ผมรอหลายรอบมาก"*.

He is right about the symptom and, about the engine, wrong — which is precisely why it kept happening. `App.SendMessage` captures the session id before the turn starts and carries it to the end (§134.1); `a.turns` is keyed by session; `turnSessionID()` writes the rows where the question was asked. The work was never bound to the window and never lost. It was **unreachable**, which from a chair is the same thing, and telling a user that their work is safe in a place they cannot get to is not an answer.

### 144.1 The chat that is working is not "another chat"

Every row in the history list went through `peekAtSession`, including the row of the conversation the turn was running in. Peek reads the **store** — so clicking the working chat mid-turn returned the question with nothing under it: no timeline, no streaming answer, composer locked, and a bar along the top explaining that the work was happening in another chat while pointing at this very one. The live array was one field away in `cockpit.peek.live` the whole time.

The only door that actually returned was `leavePeek`, reachable from exactly one button on that bar. A way back that a single button knows about is a way back the user does not have — every other route out looked like the work had been thrown away, and the route people take is the list they already use.

So the guard lives in `peekAtSession` itself rather than in each door: **the chat that is working is never opened for reading, it is returned to.** Whatever else learns to open a conversation later inherits that.

### 144.2 A stamp, instead of two facts standing in for one

Answering "is this the working chat?" required knowing which chat is working, and the window had been inferring it: `active` (the engine's open session) **and** `awaitingReply` (a turn is running somewhere). Two facts about the app standing in for one fact about a conversation — correct right up to the moment it was asked the question that matters.

`cockpit.turnSession` is now stamped when the turn starts, from `CurrentSessionID()` — the same value Go stamps the turn with in the same breath, so the two cannot drift. Written from the list first and corrected by the engine a round-trip later, so there is no frame in which a turn is running and this side does not know whose it is. Cleared in `runLiveTurn`'s `finally` and in `applyAgentDone`, because a stamp nobody clears is a chat that stays unopenable. A window reloaded mid-turn takes it from `TurnInFlight`, which is where it already learns everything else about the turn it walked in on.

`sessionWorking` reads the stamp instead of re-deriving the guess, so the sidebar's working ring and the door's decision are the same answer to the same question (one place answering one question, applied to a fact rather than to a document).

### 144.3 The bar had to keep up with the work

Two smaller lies in the same strip, both of them "the copy cannot keep up with the state" (DESIGN.md):

- It said *"งานยังทำอยู่ในอีกแชทหนึ่ง"* whether or not that was still true. The peek outlives the turn, so a bar that started honest went on insisting the agent was busy for as long as the user kept reading. It now swaps to `cockpit.peekDone` the moment the turn ends, which is also the moment the "open this chat" button appears — one strip, one story.
- It said nothing about what the work was **doing**. The live block is deliberately not drawn over a conversation it does not belong to, and the cost was a turn that looked dead from anywhere except its own chat. The bar now carries one line — the running tool, else a running delegate, else the engine's phase, else *กำลังคิด* — wearing `.typing-status`, so the sweep, its timing and its reduced-motion fallback are the live block's own rather than a second copy of them.

### What this does not decide

**Stages 2 and 3 of §134.4 are still not done, and this is not them.** One engine, one agent context: opening a second conversation *for real* mid-turn still refuses, so the composer in a chat being read is still disabled and typing in two chats at once is still not a capability. What changed is that the conversation that IS working can always be got back to, and that walking away from it no longer looks like losing it. The read-only peek is still the stopgap §134.4 called it, and still goes when per-session live state lands.

---

## 145. Decision — A Specialist Walled Off From The Work Its Own Brief Describes (2026-08-19)

The owner opened a chat with the `github` agent, asked it to do something, and was told the agent could not write a file. He sent the screenshot with three sentences: *"ไปเอากำแพงออกที เอเจนอื่นๆอีก โดนจำกัดไหมไปดู"*, then *"tool พื้นฐานที่ทำให้มันทำงานกับเครื่องนี้ได้ควรจะมีครบนะผมว่า เขียรไฟล์ แก้ไฟลือ่ะ"*, and finally the order this section records: *"ปลดเพดานให้เอเจนเฉพาะทางก่อน 📁 files ⚡ shell ให้พวกมันเข้าถึงได้เต็มที่ครับ"*.

The wall was two layers deep and only the first was visible from the chat.

### 145.1 What each layer was

**The profile's `tools:` allowlist.** Four of the six bundled agents were narrow: `github` held `github, read, list, glob, web_fetch, web_search, skills_list, skill_view`, and `doc`/`sheet`/`deck` held their own writer plus reading and the senses. None could write a plain file. The clearest evidence that this was a wall rather than a design was inside the `github` agent's own brief, which instructs it to work the repository out from the checkout's `origin` — with no `git` and no `shell` to read one with. A prompt telling a worker to do what its tool list forbids is the fault `automation_agent_test.go` already pins for one agent; it was live in another.

**The office desk's ceiling.** Every agent sits at `specialized` (`mode.Office`), whose manifest named `read, write, list, glob` and kept `doc_write, sheet_write, slides_write, shell, desk_terminal` in `chairs:`. So `edit`, `grep`, `apply_patch`, `delete` and `git` could not reach an agent however its profile was written — widening the profiles alone would have changed nothing, which is why both halves moved together.

### 145.2 What moved, and what deliberately did not

`chairs:` now carries the whole of the files group and the whole of the shell group: `doc_write, sheet_write, slides_write, edit, grep, apply_patch, delete, shell, git, desk_terminal` — with `read, write, list, glob` still on the desk itself. Every bundled agent's `tools:` names the same general kit beside its specialist tools.

**`chairs:` rather than `tools:` is the whole of the care taken here.** The distinction §84 introduced does the work: these tools are in the room for the desk's agents and still not on the assistant's desk, so the assistant sitting at the office gained nothing and its tool block did not grow by a byte. Widening `tools:` would have been the easy version and would have charged every specialized session for tools it was deliberately not given.

`delete` is included, at the owner's word *"ให้พวกมันเข้าถึงได้เต็มที่"* and on the strength of the gate in front of it: `internal/safety` rates it `RiskHigh`, so it prompts every time. Rights come from the user's visible list, not from a tool being withheld quietly.

Still refused at the office: the **code** group — `diagnostics` and `symbol` — which is now the only wall left there.

### 145.3 The quiet one was memory

Five of the six agents never named `memory`, and `task.go` gates the per-delegate memory tool on `profile.AllowsTool("memory")`. They had been running with no `MEMORY.md` of their own at all — the folder standard says an agent's memory travels with its package, and for five of six there was nothing to travel. Every agent names it now.

### 145.4 Four tests moved, none was weakened

`git`, `shell` and `apply_patch` were each being used somewhere as "the tool the office refuses", and each was true until this change. They are re-pointed at `symbol`, `diagnostics` and `fs`, which the office still refuses — the assertions are the same assertions.

One is worth reading twice: `desktop/desk_test.go`'s proof that a chair runs on the *target* desk's manifest rather than its caller's now hangs on `fs`, because the office's room has become so nearly co-extensive with the assistant desk that `fs` is the only registered name left that only the caller holds. Its comment says so, and says that the day the office names `fs` too, the test moves to a caller holding something the office refuses rather than being deleted.

### What this does not decide

**The honest refusal is still open.** An agent asked for work no held tool can do should say that, rather than failing downstream — the "ไฟล์หาย" report whose truth was "ผมไม่มีเครื่องมือที่จะสร้างมัน" (18 ส.ค.). Widening the toolsets removes many occasions for it and none of the mechanism. The walls kept on purpose — the code group, `plugin_install`, the `task` family under `forcedDenials` — can still produce exactly that wrong sentence.

---

## 146. Decision — A Tool Nobody Called, Priced On Every Request (2026-08-19)

Laying the whole tool inventory out for the owner produced a question about one line of it: *"notebook_edit แก้ Jupyter notebook รายเซลล์ ลบออกดีไหมอ่ะ เพราะเรายังไม่เคยใช้จริงเลย จะประหยัดโทเค็นไหมครับ"*.

The answer came from `tool_runs` rather than from an opinion. **Zero calls out of 2,687 recorded since the log began on 4 ส.ค.**, against 317 tokens in the block of every desk carrying the files group — paid on every request before the user has typed anything. `symbol` (180) and `fs` (0 tokens; it has no `ToolDefinition` and never reaches a model) had never fired either, and were left alone: the first is a real capability the prompt has never pointed at, and the second costs nothing.

### 146.1 Deleting the file was never one of the options

`read` renders an `.ipynb` as numbered cells through `loadNotebook`/`renderNotebook` — **in the same file as the tool**. Removing `notebook.go` would have taken reading notebooks with it and handed the model raw JSON with every base64 output embedded, which is the exact problem §54 wrote the file to solve.

So the choice was three-way, not two, and the owner took the middle one (*"เห้นด้วยกับ 2 ครับ"*): the type stays compiled in and is **not registered** in `defaults.go`. Notebooks stay readable and become read-only — which is the honest description, because nothing else can edit one. `edit` and `apply_patch` match exact text and the file stores its source JSON-escaped inside an array, so a match either misses or corrupts; `write` would have to re-emit every recorded output to keep them.

### 146.2 A switch, and the reason it is written down in one place

Putting the line back in `defaults.go` is the whole of switching it on, and its comment names the other files that name the tool rather than describe it — the check in `defaults_test.go`, the office's `chairs:`, and the agents' `tools:` lines. Its category, its 317-token pin in `block_standard_test.go`, its coverage case in `desktop/tool_coverage_test.go` and every `deny:` naming it are already written and need nothing: a `deny` on a tool that does not exist fails closed, which is the direction that costs nothing to be wrong in.

The pin is kept deliberately. It is the size this tool comes back at, and coming back over the standard should have to be noticed rather than inherited.

`defaults_test.go` asserts the absence rather than merely omitting the name, so switching it on is a decision somebody makes in the open rather than something that drifts back in on a merge.

### What this does not decide

**Nothing about the notebook pane.** The owner is thinking about a Colab-like room on the code page, and pushed back on the objection that a Python dependency disqualifies it — correctly: `image_ocr` already needs tesseract and the media tools need ffmpeg, and the rule those live under is that the product says so plainly instead of pretending. A notebook room without Python is the same shape of failure as §142's, one screen larger, and that is the thing to fix before the room, not the dependency. When the room lands, this tool is one line away and its category moves to `code` with it.

---

## 147. Decision — A List of What Is Installed Is Not a Room (2026-08-19)

The owner opened the workbench's `+` menu, looked at the four entries under it, and pointed at the last one: *"เห็นไหมครับกล่องเครื่องมือ ผมว่าเอาออกดีกว่า มันไม่ควรไปปนกันโต๊ะทำงาน"*.

He is right twice, and the second reason is the one that decides it.

**It was the wrong kind of thing to be in that menu.** Terminal, browser, files, slides are *places work happens* — you open one to do something in it. `ToolsPane` listed the MCP tools and external skills the user had plugged in, which is not somewhere to work; it is a fact about how this machine is configured. Reading it changes nothing and produces nothing, so it never earned a tab beside the four that do.

**And it was a second place answering a question Settings already answers.** Settings → เครื่องมือ lists the tools, → สกิลภายนอก the skills, → MCP servers the servers they come from — with install, remove and refresh, which the pane could not do. Nothing is lost by deleting it, and the thing that was actually at risk was the two lists disagreeing on a day one of them was updated and the other was not, which is the debt this file's standing rule is about: a second place answering the same question is หนี้ในระบบ.

Gone with it: the pane, both `+` menu entries, `openToolsTab`, the `'tools'` tab kind and the five `toolsPane.*` strings in both owned locales. A layout saved with the tab open skips it on restore, exactly as layouts naming the removed Review panel already did — the rest of that session's tabs still come back.

### What this does not decide

**Nothing about the tools themselves.** No tool was unregistered here and no MCP server was touched — this is one screen fewer, not one capability fewer. Contrast §146, where the tool really did stop being offered.

---

## 148. Decision — Free to Use, Free to Read, Not Free to Take (2026-08-19)

The owner, unprompted, in the middle of cutting v1.3.0: *"เออผมกลัวคนมาก็อปโปรเจคมากเลยครับ แบบโหลดโค้ดในกิตฮับแล้วไปรีแบรนหรือแอบไปขายต่อ"* — then, on the options: *"ผมไม่ให้แก้ครับ ผมพัฒนาให้เขาใช้ก็พอ ไม่ให้แก้ไฟล์ หากแก้แล้วจะไม่ได้รับการสนับสนุนจากผม และไม่อนุมัติให้เคลม อนุญาตให้ใช้ก็พอ"*, and *"Aetox คือของผม ทรัพย์สินทางปัญญาของผม"*.

The uncomfortable half of the answer is that Apache-2.0 permitted precisely the thing he was afraid of, on purpose. Fork, rebrand, close the source, sell it — all of it legal, all of it what §60 chose in July when the fear was that nobody would adopt it rather than that somebody would take it. The fear changed; the licence had not.

### 148.1 The three facts that decided the shape

**He can do it.** `git shortlog -sne --all` returns **553 commits under one name**, his. There is no second copyright holder to ask, no CLA to have failed to collect, no contributor whose patch would have to be reverted. Sole authorship is what makes relicensing a decision rather than a negotiation, and it was checked before anything was written rather than assumed.

**He cannot do it retroactively.** Every commit already pushed is Apache-2.0 permanently, and anyone holding a clone from before today keeps that grant forever. This is stated plainly in the licence itself (§14) rather than left to be discovered: pretending otherwise would be the app lying, one layer up from the version number.

**Nothing in the tree blocks it.** All 31 Go modules actually linked into the binary and all 11 packages bundled into the frontend are MIT, BSD-2/3, Apache-2.0, SIL OFL, or (dompurify) MPL-2.0-or-Apache-2.0 — every one of them permits inclusion in proprietary software given notices. **There is no GPL or AGPL anywhere in what ships.** The copyleft programs Aetox touches (poppler, ffmpeg, git) are separate executables it calls, which is not linking. This was verified against the module cache and `node_modules`, not recalled.

### 148.2 What the licence actually says, and the two carve-outs that keep the product a product

Free to install and run, any number of machines, commercial work included. Free to read, audit, publish findings about, and quote with attribution. Not free to modify, redistribute, sublicense, rebrand, sell, or claim.

Two carve-outs do the real work:

**Your own extensions are yours (§4).** Skills, agents, prompts, configuration, memory files, MCP servers — writing them, distributing them and selling them is expressly permitted, and the Licensor claims no ownership. Without this clause a "no derivative works" licence would have banned the extension mechanism the whole product is built around. A licence that forbids the thing the software is for is not protection, it is a bug in the licence.

**Third-party components are untouched (§6).** Nothing in this licence narrows anyone's rights in the MIT and BSD code Aetox carries; those rights come from their authors, not from here, and [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) now exists so the notices actually travel — an obligation that was being met only by `go.mod` before, which is to say not met.

The repo stays public. That was the owner's call between two options, and it is the one that keeps the landing page honest: it leads with evidence of real work, and the evidence is the source.

### 148.3 A second place that was about to name the wrong licence

`Settings.svelte` drew the About attribution line from a hardcoded string — the same sentence as `version.Credit`, written twice. Changing the licence in one and not the other would have left the running app telling users "Apache-2.0" while `LICENSE` said otherwise, which is the update-check failure of §60's own reasoning transposed one field over. About now reads `App.AppCredit`, so there is one string, in Go, next to the constant the `--version` flag prints.

### What this does not decide

**Nothing is enforced by code, and nothing here is legal advice.** The teeth are the trademark, the ed25519 release key that a fork cannot forge, and the credit line §5(d) now makes it a breach to delete. A licence is a statement of terms, not a lock; anyone determined to ignore it still can, and the remedy is a lawyer rather than a commit. If Aetox is ever sold commercially, this document should be read by one.

**Closing the repository is still available and still not taken.** Making it private would stop new copies at the source, at the price of the evidence the landing page is built on. Today the answer is: read all of it, take none of it.
