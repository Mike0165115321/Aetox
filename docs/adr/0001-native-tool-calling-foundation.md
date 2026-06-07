# ADR 0001: Adopt Native Tool Calling as the Foundation for Agentic Skills

Date: 2026-06-07
Status: Accepted
Scope: Short-term architecture decision for the next agentic capability slice

## Context

Aetox CLI currently supports local skills, but skill execution is still command-driven rather than model-driven.

Current state from the codebase:

- `internal/plan` classifies input as `conversation` or `skill` by matching a known command name.
- `internal/app` routes known skills before the request reaches the model.
- `internal/cognitive.Agent` expects the model to return final text only.
- `internal/model` only models text messages and text responses.
- `internal/skill` exposes a registry and dispatcher for local command execution, but no structured tool schema.

This works for explicit commands such as `list` or `shell`, but it does not support the agent pattern used by systems like Codex, aider, or OpenCode, where the model selects a tool based on the task and returns a structured tool call.

Provider constraints verified on 2026-06-07:

- OpenRouter supports tool calling using the OpenAI-style `tools` and `tool_calls` protocol.
- DeepSeek supports tool calls in its OpenAI-compatible API.
- DeepSeek thinking mode also supports tool calls, but when a tool call happens the client must round-trip `reasoning_content` in subsequent requests.
- DeepSeek documents that `deepseek-chat` and `deepseek-reasoner` are deprecated on July 24, 2026 at 15:59 UTC.

References:

- OpenRouter tool calling docs: https://openrouter.ai/docs/guides/features/tool-calling
- DeepSeek tool calls docs: https://api-docs.deepseek.com/guides/tool_calls
- DeepSeek thinking mode docs: https://api-docs.deepseek.com/guides/thinking_mode
- DeepSeek quick start and model deprecation note: https://api-docs.deepseek.com/

## Decision

Aetox will adopt native API tool calling as the canonical mechanism for model-driven skill execution.

This decision includes the following architecture rules:

1. Aetox will use provider-native `tools`, `tool_calls`, and `tool` result messages instead of embedding tool protocols only inside the system prompt.
2. Explicit slash commands and command-style skills will remain supported for direct user control.
3. Not every `Skill` will automatically become a model-callable tool. Tool exposure will be opt-in and allowlisted.
4. Phase 1 will expose only low-risk tools to the model.
5. Phase 1 tool loops will be non-streaming first. Streaming can be added after the tool-call contract is stable.

## Architectural Shape

### 1. Model contract will become structured

`internal/model` will be extended so the request and response contract can represent:

- tool definitions
- assistant tool calls
- tool result messages
- optional provider-specific reasoning metadata when needed

Minimum new concepts:

- `ToolDefinition`
- `ToolCall`
- `ToolResult`
- `RoleTool`
- optional `ReasoningContent`

This is necessary because the current model layer only captures plain text content and would silently discard tool calls.

### 2. Tool capability will be separate from command capability

The current `skill.Skill` interface is suitable for command-style execution, but it is not enough as the sole contract for model-selected tools.

Aetox should introduce an additional opt-in capability for model-callable tools, for example:

```go
type Tool interface {
    Skill
    ToolDefinition() model.ToolDefinition
    ExecuteTool(ctx context.Context, args map[string]any) (Output, error)
}
```

Implications:

- `Skill` remains the base abstraction for local command usage
- only tool-capable skills are advertised to the model
- `help` and other CLI-oriented skills do not need to become tools
- command parsing and JSON argument parsing stay separate

### 3. Dispatcher will support structured tool execution

`internal/skill.Dispatcher` will keep command execution and gain structured tool execution, for example:

- `Execute(ctx, input string)` for explicit user commands
- `ExecuteTool(ctx, name string, args map[string]any)` for model-selected tool calls

This keeps the slash-command UX intact while enabling agentic execution paths.

### 4. Agent will own the tool loop

`internal/cognitive.Agent` will gain a tool loop:

1. send user and conversation context plus tool definitions
2. inspect the model response
3. if the model returned tool calls, execute them locally
4. append tool results back into the model conversation
5. continue until the model returns a final answer or a loop limit is reached

Phase 1 constraints:

- no parallel tool calls
- no streaming during tool loops
- bounded loop count, such as 4 to 8 tool rounds
- clear failure message when the model asks for an unknown or blocked tool

### 5. Safety remains a first-class boundary

Tool calling must not bypass `internal/safety`.

The safety layer should be extended from command-name heuristics to structured tool-call assessment:

- tool name
- structured arguments
- approval requirement
- model-driven versus explicit user-driven invocation

Phase 1 rule:

- `shell` is not exposed as an automatic model tool
- `shell` remains available only through explicit user command paths

## Initial Tool Exposure

Phase 1 model-callable tools:

- `time`
- `list`

Deferred from Phase 1:

- `shell`
- future file-editing tools
- git tools
- multi-step write tools

Reason:

- `time` and `list` are bounded and easy to validate
- `shell` is too broad and high-risk for the first model-driven rollout
- coding-assistant parity should be reached through narrower tools first, not through unrestricted shell execution

## Provider Strategy

### OpenRouter

OpenRouter will be supported through native tool calling request fields. Tool support is model-dependent, so Aetox must not assume every selected OpenRouter model supports tools.

### DeepSeek

DeepSeek will be supported through its OpenAI-compatible tool call interface.

Phase 1 should avoid depending on DeepSeek thinking mode in the tool loop, because thinking mode with tool calls requires correct `reasoning_content` round-tripping. That can be added in a later phase after the base loop is stable.

### Model defaults

The existing DeepSeek defaults should be updated away from `deepseek-chat` and `deepseek-reasoner`, because DeepSeek documents both names as deprecated on July 24, 2026 at 15:59 UTC.

## Consequences

Positive:

- moves Aetox toward true agentic behavior instead of command matching only
- aligns with provider-native APIs instead of prompt-only emulation
- keeps explicit CLI control while adding model-selected tools
- creates a clean path toward safer file, search, and git tools

Negative:

- increases complexity in `internal/model` and `internal/cognitive`
- requires new tests around loop behavior, tool serialization, and failure handling
- creates provider-specific edge cases, especially around DeepSeek reasoning metadata

## Alternatives Rejected

### 1. Put tool schema only in the system prompt

Rejected because provider-native tool calling already exists and the prompt-only approach is weaker, less reliable, and harder to validate.

### 2. Make every skill automatically callable by the model

Rejected because command skills and model tools do not have the same safety or argument-shape requirements.

### 3. Expose `shell` first as the main agent tool

Rejected because it is the broadest and riskiest capability in the current system.

## Rollout Plan

### Phase 0: Compatibility cleanup

- update DeepSeek default model selection away from deprecated names
- add provider capability flags for tool support
- add tests for capability detection and default selection

### Phase 1: Model and skill contracts

- extend `internal/model` with tool-aware request and response types
- introduce tool-capable skill metadata and structured execution
- add dispatcher support for structured tool calls
- write tests using `noop` or mock providers

### Phase 2: Agent loop

- implement non-streaming tool loop in `internal/cognitive.Agent`
- expose only `time` and `list`
- enforce loop limits and safety checks
- return final answer after tool results are fed back to the model

### Phase 3: Safer coding tools

- add narrow read-oriented tools such as file read, search, and git status
- keep `shell` manual until narrower tools prove sufficient

### Phase 4: Advanced behaviors

- streaming with tool calls
- optional parallel tool calls
- optional DeepSeek thinking-mode support with `reasoning_content`

## Implementation Notes

The smallest safe implementation slice is:

1. extend model request and response contracts
2. add opt-in tool-capable skills
3. implement a non-streaming tool loop
4. expose only `time` and `list`

This is the shortest path that changes the architecture meaningfully without opening the high-risk surface too early.
