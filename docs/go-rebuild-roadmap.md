# Go Rebuild Roadmap

## Guiding Principle

Build a small, reliable CLI core before rebuilding the full agent network.
The first version should feel boringly dependable: clear input, clear plan,
clear permission boundary, clear output.

## Phase 0: Repository Reset

Status: done in this cleanup pass.

- Remove Python implementation.
- Remove tests, configs, scratch files, caches, runtime data, and local secrets.
- Keep architecture docs and extracted handoff notes.
- Rename local folder and GitHub repository.

## Phase 1: Minimal CLI Slice

Goal: prove the end-to-end command path.

Proposed layout:

```text
aetox-cli/
  cmd/aetox/
    main.go
  internal/
    cli/
    planner/
    dispatcher/
    executor/
    critic/
    memory/
    safety/
    tools/
    llm/
  docs/
```

Milestone:

```text
aetox "list markdown files in this folder"
```

Expected behavior:

- Parse command-line goal.
- Build a typed one-step plan.
- Execute a read-only file listing tool.
- Validate output.
- Print a compact report.

## Phase 2: Safety and Tool Contracts

Goal: make tool execution trustworthy.

- Define `Tool` interface.
- Define JSON schema or equivalent typed metadata for each tool.
- Add risk levels: low, medium, high.
- Block delete, move, overwrite, code execution, and external API calls until
  explicitly approved.
- Add path sandbox tests.

## Phase 3: Planner and Critic Loop

Goal: make multi-step tasks reliable.

- Add LLM-backed planner behind `llm.Client`.
- Add prompt templates with strict structured output.
- Add critic verdicts.
- Add retry with critic hint injection.
- Add max retry and timeout rules.

## Phase 4: Memory

Goal: keep context useful without making the system heavy.

- Chat mode: short sliding window.
- Plan mode: previous result plus compact plan summary.
- No background embeddings in the MVP.
- Persistent memory only after a clear user story exists.

## Phase 5: Distribution

Goal: make Aetox CLI easy to install.

- Build single binary releases.
- Add Windows-first install docs.
- Add config file location conventions.
- Add version command and health check.

## First Tool Candidates

- `files.list`
- `files.read`
- `files.write`
- `files.move`
- `files.delete`
- `web.fetch`
- `shell.run`

Only `files.list` and `files.read` should be enabled before safety gates are
complete.

## Non-Goals For v0.1

- Discord bot.
- REST API.
- Persistent vector database.
- Automatic background summarization.
- Full plugin marketplace.
- Multi-provider cloud fallback.
