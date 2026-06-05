# Aetox CLI

Aetox CLI is the Go rebuild target for the Aetox agentic operating-system idea.
This repository now contains an executable vertical slice (Phase 1 + 2 + Phase 3)
with planner, dispatcher, executor, critic, memory, safety gates, and a small
tool registry.

## Current State

- Python implementation: removed.
- Old tests, configs, caches, scratch files, and runtime data: removed.
- Architecture docs and legacy notes preserved in `docs/`.
- Target implementation language: Go.
- Product shape: local-first command-line agent orchestration.

## Core Idea

Aetox CLI is not a chatbot wrapper. It is a local-first command orchestrator where
small roles cooperate:

- Planner: turns a user goal into a bounded task plan.
- Dispatcher: executes the plan step by step with timeout and retry control.
- Executor: calls tools through a registry.
- Critic: checks step output before marking it as trusted context.
- Memory: keeps only the context needed for the current mode.
- Safety: gates risky actions with explicit approval.

## Current Tooling

- `files` - list/read/write/move/delete in sandbox.
- `web` - fetch URL content.
- `shell` - run shell commands in sandbox directory.

## Current Supported Goals

```bash
aetox "list markdown files in this folder"
aetox "read file README.md"
aetox "write file \"notes.txt\" \"hello from cli\""
aetox --yes "move file old.txt new.txt"
aetox --yes "delete file temp.txt"
aetox --yes "fetch https://example.com"
aetox "list . then read file README.md then read file cmd/aetox/main.go"
aetox --yes "run dir then write file \"phase3.txt\" \"done\""
aetox --yes "run dir"
```

`--yes` bypasses interactive prompts for risky steps during local development, but
you can also answer `y`/`yes` when prompted.

Phase 3 controls:

- `--retries` controls step-level retry attempts before giving up on one action.
- `--plan-retries` controls how many times dispatcher replans after critic escalation.

## How to Run

From the repository root:

```bash
go run ./cmd/aetox "list markdown files in this folder"
```

Or build once:

```bash
go build -o aetox ./cmd/aetox
./aetox "read file README.md"
```

## Documentation Map

- [Aetox CLI architecture handoff](docs/aetox-cli-architecture-handoff.md)
- [Go rebuild roadmap](docs/go-rebuild-roadmap.md)
- [Future agent notes](docs/future-agent-notes.md)
- [Legacy documentation index](docs/legacy-index.md)

## Rebuild Direction

The Go rewrite follows the architecture archive:

1. Parse CLI goal.
2. Build a typed task plan.
3. Execute a safe tool through the registry.
4. Safety gate blocks high-risk actions.
5. Critic validates step results.
6. CLI prints a compact final report.

Only after this slice is stable should persistent memory, richer plugins, and broader
tooling be added.
