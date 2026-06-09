# Approval Mode Plan

Date: 2026-06-09
Scope: Codex-like permission modes for Aetox CLI, with minimal UX friction and a stable policy architecture.

## 1. Goal

Replace the current `AutoApprove bool` plus repeated `y/N` prompts with a small approval-mode system that behaves more like Codex:

- `ask`
- `unsafe-only`
- `full-access`

The first implementation should reduce prompt fatigue without weakening the architecture.

## 2. Current State

Current approval behavior is simple:

- `internal/safety` returns `RiskLow` or `RiskHigh`
- `internal/turn.Executor` decides whether to prompt
- `internal/config.Config` only stores `AutoApprove bool`
- high-risk paths prompt repeatedly with `y/N`

This works, but it has two architectural limits:

1. risk classification and approval policy are too tightly coupled
2. the system cannot express Codex-like permission levels cleanly

## 3. Target Outcome

After this change, Aetox should support three stable runtime modes:

### `ask`

- current conservative behavior
- risky actions require confirmation

### `unsafe-only`

- allow read-only and normal in-workspace actions without prompting
- prompt only for destructive, outside-workspace, or especially broad actions

### `full-access`

- no confirmation prompts
- intended for trusted local workflows

## 4. Design Principles

- keep policy decisions out of UI code
- keep safety classification separate from prompt policy
- keep one execution decision point in `internal/turn`
- make modes persist like model preferences
- keep room for future Codex-like categories without another rewrite

## 5. Proposed Architecture

### 5.1 New approval-mode type

Introduce a dedicated mode type instead of reusing `AutoApprove`:

```go
type ApprovalMode string

const (
    ApprovalAsk        ApprovalMode = "ask"
    ApprovalUnsafeOnly ApprovalMode = "unsafe-only"
    ApprovalFullAccess ApprovalMode = "full-access"
)
```

Likely home:

- `internal/safety`
- or a small dedicated package such as `internal/approval`

Recommendation:

- keep it near `internal/safety` first
- do not create a new package unless the first pass becomes crowded

### 5.2 Split classification from policy

Current state:

- `AssessCommand(...) -> RiskLow | RiskHigh`

Proposed state:

- `AssessCommand(...) -> Assessment`
- `Assessment` carries structured effects
- a policy resolver decides whether the current mode should prompt

Suggested shape:

```go
type Effect string

const (
    EffectReadWorkspace         Effect = "read-workspace"
    EffectWriteWorkspace        Effect = "write-workspace"
    EffectDeleteWorkspace       Effect = "delete-workspace"
    EffectMutateGit             Effect = "mutate-git"
    EffectExecuteShell          Effect = "execute-shell"
    EffectUseNetwork            Effect = "use-network"
    EffectTouchOutsideWorkspace Effect = "touch-outside-workspace"
)

type Assessment struct {
    SkillName string
    Effects   []Effect
    Reason    string
}
```

This does not need to be perfect on day one. The first pass only needs enough fidelity to support the three modes cleanly.

### 5.3 Add a policy resolver

Add one policy function:

```go
func ShouldPrompt(mode ApprovalMode, a Assessment) bool
```

Expected behavior:

- `ask`: prompt for all risky effects
- `unsafe-only`: prompt for destructive or boundary-crossing effects
- `full-access`: never prompt

This becomes the only place where mode semantics live.

### 5.4 Keep `internal/turn` as the execution boundary

`internal/turn.Executor` already owns execution-time decisions. Keep that shape.

Change:

- `Executor` receives `ApprovalMode`
- `Executor` calls `ShouldPrompt(...)`
- explicit skills, inferred tools, and model-selected tools all use the same approval policy

This preserves one decision seam.

## 6. UX Plan

### 6.1 Display current mode

Show the current approval mode in the terminal status area, similar to the model status pattern.

Recommended display:

- header right side keeps model and think level
- prompt/status area includes approval mode

Example:

- `mode: ask`
- `mode: unsafe-only`
- `mode: full-access`

Recommendation:

- keep model status primary
- keep approval mode secondary
- do not overload the model label with permission state

### 6.2 Selection flow

Add a lightweight mode selector:

- startup default from config or preference
- slash command for changing mode in-session

Recommended command:

- `/approval`

Selection options:

- `ask`
- `unsafe-only`
- `full-access`

### 6.3 Remove repeated prompt fatigue

The user complaint is not about the prompt UI itself. It is about being asked too often.

So v1 should prioritize:

- better default mode behavior
- in-session switching
- persistent saved mode

Do not spend time polishing the prompt dialog before the policy is fixed.

## 7. Persistence Plan

Add approval mode to stored preferences/config.

Two valid implementation paths:

### Option A: store in `ModelPreference`

Pros:

- easy to ship
- one existing persistence file

Cons:

- mixes model selection state with runtime permission state

### Option B: create a broader CLI preference shape

Pros:

- cleaner long-term boundary

Cons:

- more work now

Recommendation:

- v1: add approval mode to existing preference storage
- later: split into a broader runtime preference object if more CLI session state accumulates

## 8. Implementation Plan by File

### `internal/safety/safety.go`

Change:

- replace or extend `RiskLow/RiskHigh` model
- add structured effects
- add approval mode type
- add policy resolver

Expected result:

- safety classifies actions
- policy decides prompting

### `internal/config/config.go`

Change:

- replace `AutoApprove bool` with `ApprovalMode string`
- normalize default mode
- persist/load mode

Expected result:

- approval behavior is configurable and persistent

### `internal/turn/executor.go`

Change:

- accept approval mode in executor options
- call policy resolver for every approval decision
- stop hardcoding prompt behavior around `RiskHigh` only

Expected result:

- one approval decision path for all tool execution modes

### `internal/app/app.go`

Change:

- display current approval mode
- add slash command entrypoint for changing it

Expected result:

- user always sees the current permission level

### `cmd/aetox/main.go`

Change:

- parse approval mode from flags or persisted preference
- wire mode into config and executor
- implement `/approval` switching flow

Expected result:

- end-to-end selection, persistence, and runtime wiring

## 9. Suggested Day-1 Scope

Keep tomorrow focused on this slice only:

1. add `ApprovalMode`
2. add `ShouldPrompt(...)`
3. wire it through config and executor
4. persist the selected mode
5. show mode in UI
6. add `/approval`

Do not do these tomorrow unless required:

- advanced filesystem boundary detection
- network-aware shell parsing
- command allowlists by provider
- session-scoped temporary overrides

## 10. Acceptance Criteria

The first version is done when all of these are true:

1. user can choose `ask`, `unsafe-only`, or `full-access`
2. selected mode is visible in the UI
3. selected mode persists across runs
4. `unsafe-only` noticeably reduces `y/N` prompts
5. explicit skill execution, inferred tool execution, and model-selected tool execution all use the same policy path
6. `full-access` removes confirmation prompts entirely

## 11. Risks

### Risk 1: fake Codex behavior

If the system only renames `AutoApprove` without changing the policy model, the UI will look better but behavior will remain crude.

Mitigation:

- implement structured assessment plus policy resolver, even if the first effect set is small

### Risk 2: `internal/turn` grows too much

If approval logic stays inline in many branches, complexity will keep growing.

Mitigation:

- one helper path for prompt decisions

### Risk 3: effect classification is too weak

If all writes are treated the same, `unsafe-only` may still feel noisy.

Mitigation:

- distinguish at least:
  - read
  - write
  - delete
  - git mutate
  - shell
  - outside-workspace

## 12. Recommendation

Build the first version as a policy architecture change, not just a UI option.

That means:

- do not keep `AutoApprove` as the real model
- do not let `app` decide approval semantics
- do not hardcode tomorrow's three modes in multiple files

The right v1 is small, but the seam must be correct.
