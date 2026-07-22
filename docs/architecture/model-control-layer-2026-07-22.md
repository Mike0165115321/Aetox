# Model-Control Layer — Deep Dive

> **Date:** 2026-07-22 · **Status:** Direct (read in full: `internal/turn/executor.go`, `internal/cognitive/agent.go`, `internal/skill/{skill,dispatcher,defaults}.go`, `internal/safety/safety.go`)
> **Scope:** layer 2 of the 5-layer reading map in [ARCHITECTURE.md](../../ARCHITECTURE.md) — "the thing that controls the model": deciding when to call a tool, running the tool, and gating it for safety. **Three cooperating packages, not one:** `internal/turn` (orchestration), `internal/cognitive` (the model conversation), `internal/skill` (what a tool call actually does).

This doc exists because [ARCHITECTURE.md](../../ARCHITECTURE.md)'s file-responsibility table describes `turn.Executor` in one line ("4-phase turn pipeline") — the actual control flow has more branches than that summary can carry, and getting it wrong when extending it (e.g. wiring MCP) is easy. Read this before changing `turn/executor.go` or `cognitive/agent.go`.

---

## 1. The three packages, one sentence each

| Package | Owns | Does not own |
|---|---|---|
| `internal/turn` | **Executor.Execute** — the per-turn decision tree: which of 4 paths handles this input, and the safety gate before any tool actually runs. | Talking to the provider API directly, or defining what a tool does. |
| `internal/cognitive` | **Agent** — holds conversation memory (`internal/memory.Context`), builds provider requests, runs the bounded tool-call loop against the provider, decides when the model is "done." | Deciding *whether* to offer tools this turn, or *approving* a tool call. |
| `internal/skill` | **Registry/Dispatcher** — the 17 built-in tools, name→implementation lookup, `Source` (builtin/external — [ARCHITECTURE.md §6.4](../../ARCHITECTURE.md#64-skill-registry-has-no-corevser-added-boundary--fixed-2026-07-21)). | Approving whether a tool call is allowed to run at all — that's `internal/safety`, called from `turn`, not from here. |

`internal/safety` is a fourth, smaller package this layer depends on for the approval gate (§3) — it's not part of the 3 above because it's pure policy (`AssessCommand`/`ShouldPrompt`), no state, no loop.

---

## 2. `turn.Executor.Execute` — the real 4 phases

Confirmed by reading `internal/turn/executor.go:145-231`. Every user turn goes through this decision tree, checked **in this order**, first match wins:

```mermaid
flowchart TD
    Start(["user input"]) --> Parse["normalizeIntent + inferToolCandidates\n(regex-based tool guessing, e.g. a GitHub URL implies github_repo_summary)"]
    Parse --> P1{"Phase 1:\nhigh-priority inferred tool?\n(shouldExecuteInferredBeforeAgent)"}
    P1 -->|yes| Exec1["executeInferredToolCandidatesLoop\n— runs tool(s) directly, no model call"]
    P1 -->|no| P2{"Phase 2:\nagent supports tool calling\nAND (conversation OR should-use-inferred)?"}
    P2 -->|yes| Loop["executeAgentToolLoop\n→ cognitive.Agent.RespondWithTools\n(model decides tool use, §3)"]
    Loop -->|model didn't use tools, but candidates exist| Exec1b["fallback: executeInferredToolCandidatesLoop"]
    P2 -->|no, or loop not handled| P3{"Phase 3:\ninferred tools for\nnon-tool-capable agents?"}
    P3 -->|yes| Exec3["executeInferredToolCandidatesLoop"]
    P3 -->|no| P4{"Phase 4:\nconversation\nor explicit skill command?"}
    P4 -->|conversation| Stream["agent.RespondStream\n— plain streaming chat, no tools"]
    P4 -->|explicit command, e.g. \"git status\"| SkillTurn["executeSkillTurn\n— direct dispatch, bypasses the model entirely"]
```

**Why 4 phases and not one clean path:** the system supports providers with and without tool-calling, and inputs that unambiguously imply a tool (a raw GitHub URL) alongside ones that need the model's judgment. Phase 1 short-circuits the unambiguous case before spending a model call; Phase 2 is the normal path for a tool-capable model; Phase 3 is the same regex-inference as Phase 1 but as a *fallback* for models without tool-calling; Phase 4 is the two "nothing left to infer" cases — free chat, or a literal skill-name command (`git status`, `read foo.go`) that never needed the model at all.

**Consequence for MCP/orchestrator work:** any new capability that adds tools must show up in `dispatcher.ToolDefinitions()` (feeds Phase 2's tool-capable check) — adding a tool implementation alone, without registering it so `ToolDefinitions()` includes it, means Phase 2 never offers it to the model.

---

## 3. `cognitive.Agent.RespondWithTools` — the model-driven loop

Confirmed by reading `internal/cognitive/agent.go:51-143`. Called from `turn.executeAgentToolLoop` (`executor.go:408-457`), which supplies the `execTool` callback (§4).

- Unbounded loop (OpenCode-style, changed 2026-07-22): runs until the model stops calling tools. The brakes are the approval/permission layer and ctx cancellation (Ctrl+C in the CLI, the desktop Stop button → `App.CancelTurn`). Setting `MaxToolCalls > 0` in `AgentConfig` opts back into a hard cap, after which the reply is `"agent tool loop reached maximum iterations"`.
- Each iteration: one `provider.Complete()` call with `tool_choice:"auto"`. If the response has zero tool calls, that's the model's final answer — loop ends, reply returned.
- If the response has tool calls, each one is executed **serially** via the `execTool` callback (not concurrently) and the result is appended to conversation memory as a `RoleTool` message before the next iteration's `Complete()` call — so the model sees prior tool results before deciding its next move.
- First-iteration provider error falls back to plain `Respond()` (no tools) rather than failing the turn outright; a later-iteration error fails the turn.

**This package has no concept of "is this tool call allowed."** It calls whatever `execTool` gives it and trusts the result. Approval happens one level up, in `turn` (§4) — `cognitive` would run an unapproved dangerous command if `turn` ever called `RespondWithTools` with an `execTool` that skipped the gate.

---

## 4. The safety gate — the one chokepoint every tool call passes through

Confirmed by reading `internal/turn/executor.go:700-741` (`executeTool`) — this is the function both the model-driven loop (§3, via `executeToolCallWithOutcome`) and the inferred-tool paths (Phase 1/3) ultimately call. There is exactly one path into `dispatcher.ExecuteTool` from turn execution, and this is it:

```mermaid
sequenceDiagram
    participant Loop as cognitive.Agent loop (or inferred-tool path)
    participant Exec as turn.Executor.executeTool
    participant Safety as internal/safety
    participant User as approveOrDeny (blocks for input)
    participant Disp as skill.Dispatcher

    Loop->>Exec: executeTool(name, args)
    Exec->>Safety: AssessCommand(name, args) → RiskLevel + Effects
    Exec->>Safety: ShouldPrompt(approvalMode, assessment)
    alt should prompt (ask mode, or unsafe-only + risky command)
        Exec->>User: approveOrDeny(commandLine, reason)
        User-->>Exec: allow / deny
        alt denied
            Exec-->>Loop: "tool execution blocked by user" (Success:false)
        end
    end
    Exec->>Disp: ExecuteTool(name, args)
    Disp-->>Exec: skill.Output
    Exec-->>Loop: receipt (JSON: tool, status, success, output, stderr)
```

**Why this matters for MCP/plugin work** (per `MCP-SUPPORT-PLAN.md`, cited in [ARCHITECTURE.md §8](../../ARCHITECTURE.md)): `safety.AssessCommand` was written for the 17 trusted, self-authored built-ins — it has no concept of `skill.Source` (builtin vs. external) yet. An MCP tool registered with `SourceExternal` goes through the exact same `AssessCommand`/`ShouldPrompt` logic as `read`/`write` today, with no extra scrutiny for the fact that its implementation is third-party code the project didn't write. This is the same gap flagged in `MCP-SUPPORT-PLAN.md`'s "ช่องว่างที่ต้องปิดก่อน production-ready" section — not new, just located precisely here for whoever implements it.

---

## 5. Where MCP would actually plug in

Per `MCP-SUPPORT-PLAN.md` (already the project's own plan, not new here) — restated with exact file anchors from this deep dive:

1. An MCP adapter implements `skill.Tool` (same interface every built-in implements) — `desktop/workbench.go`'s `browserOpenSkill`/`browserReadSkill` are the closest existing example of a non-trivial `Tool` wrapping an external process/UI.
2. It registers via `registry.Register(adapter, skill.SourceExternal)` (or a new `SourceMCP` — not yet added, see [ARCHITECTURE.md §6.4](../../ARCHITECTURE.md#64-skill-registry-has-no-corevser-added-boundary--fixed-2026-07-21) "Still open") in `bootstrapFromConfig` (`desktop/app.go`), the same place `extraSkills` are registered today.
3. Once registered, it automatically appears in `dispatcher.ToolDefinitions()` → Phase 2 of `Execute()` (§2) offers it to the model → `executeTool` (§4) gates it through the *same* safety check every built-in gets, with the gap noted above.

No change to `turn.Executor`'s control flow is needed to add an MCP tool — the seam is entirely at the registry/dispatcher level. The safety-gating gap (§4) is the thing that needs deciding *before* wiring a real MCP client, not the control flow itself.

---

## 6. The exact call path — what a "call" to the provider actually contains

Added 2026-07-22, closing a gap: this doc previously said Agent "builds provider requests" (§1 table) with no field-level detail — moved here from [ARCHITECTURE.md §13.1](../../ARCHITECTURE.md) once RTK's integration settled, since RTK's hook point *is* this exact seam. Confirmed by reading `internal/cognitive/agent.go:309` (`buildRequest`) and `internal/memory/context.go`.

```mermaid
flowchart LR
    U["user message"] --> CtxAdd["memory.Context.Add\n(role=user)"]
    CtxAdd --> Build["Agent.buildRequest\nModel+Messages+MaxTokens+Temp+Tools+ToolChoice+Reasoning/Thinking"]
    Build --> Complete["provider.Complete()"]
    Complete -->|tool_calls| Exec["turn.Executor executes tool\n(internal/skill)"]
    Exec --> Receipt["skill.Output -> modelToolReceipt JSON\n(RTK hook lives here, see §7)"]
    Receipt --> CtxTool["memory.Context.AddMessage\n(role=tool)"]
    CtxTool --> Build
    Complete -->|plain text| Reply["reply to user"]
```

- `model.Request` (`internal/model/types.go:59`) carries `Messages []Message` — the **entire** capped conversation history, not just the new turn — plus `Tools`, `ToolChoice`, fixed `MaxTokens`/`Temperature` per call-site (4096/0.2 for the tool loop, 768/0.2 for plain `Respond`), and per-provider `Reasoning`/`Thinking` config.
- The system prompt ([ARCHITECTURE.md §11](../../ARCHITECTURE.md)) is `messages[0]`, set once at bootstrap and never rebuilt per call — `buildRequest` doesn't touch it, it's just the first element `Context.Messages()` already contains.
- `memory.Context.enforceLimits` (`internal/memory/context.go:101`) is the **only** existing token-budget mechanism before RTK: drops the oldest assistant+tool message block once `maxChars` (128k) is exceeded, hard-caps message count at `maxTurns` (80). It acts *after* messages already exist — RTK (§7) acts *before* a tool-result message is even created, shrinking at the source instead of discarding whole turns later.
- Tool output re-enters the conversation as a `RoleTool` message via `modelToolReceipt` (`internal/turn/executor.go:661`) — one `AddMessage` call per tool call, every loop iteration re-sends the *entire* accumulated history back to `buildRequest`, not just the newest message.

## 7. Where RTK plugs in — and why not at `buildRequest`

Full decision record: [ARCHITECTURE.md §13](../../ARCHITECTURE.md). Summary of the one fact worth restating here: **the hook is not, and should not be, inside `buildRequest`.**

By the time `buildRequest` runs, `Context.Messages()` is already a flat `[]Message` — plain `Content` strings with no record of "this came from `git status`" vs "this came from `shell running go test`." Filtering generically at that point would mean re-guessing each message's origin from its text alone, which is strictly worse than filtering at `modelToolReceipt` (§4/§6 diagram), where the tool **name and args are still right there** and the mapping to an `rtk pipe -f <filter>` name is unambiguous. A second hook at `buildRequest` would be redundant at best (re-filtering already-filtered content) and actively harmful at worst (guessing wrong, corrupting already-correct content) — not built, and not a gap; this is the settled, final placement (`internal/rtk.FilterForTool`/`Filter`, called from `modelToolReceipt` before the existing secret-redaction pass).

---

## Related documents

- [ARCHITECTURE.md](../../ARCHITECTURE.md) — whole-repo map; §4.1 has the file-count table this doc expands on, §6.4 has the Registry/Source fix this doc assumes.
- [MCP-SUPPORT-PLAN.md](../../MCP-SUPPORT-PLAN.md) — the project's own MCP readiness notes; this doc doesn't repeat its content, only anchors it to specific line-level evidence.
- [TEST-REPORT.md](../../TEST-REPORT.md) Module 2 — test coverage for `skill`/`cognitive`/`turn`.
