# Aetox CLI

Aetox CLI is the next rebuild target for the Aetox agentic operating-system idea.
This repository has intentionally been reduced to documentation and architecture
notes so the old Python implementation can be replaced with a clean Go codebase.

## Current State

This repo is now a design archive and rebuild handoff, not a runnable app.

- Existing Python implementation: removed.
- Old tests, configs, caches, scratch files, and runtime data: removed.
- Architecture ideas, legacy notes, and extracted design decisions: preserved.
- Target implementation language: Go.
- Target product shape: local-first command-line agent orchestration.

## Core Idea

Aetox CLI is not meant to be a chatbot wrapper. It is a local-first command
orchestrator where a small set of roles cooperate:

- Planner: turns a user goal into a bounded task plan.
- Dispatcher: executes the plan step by step, with timeout and retry control.
- Executor: calls tools through a registry instead of hardcoded branches.
- Critic: checks step output before it becomes trusted context.
- Memory: keeps only the context needed for the current mode.
- Safety: gates destructive actions with risk scoring and user approval.

The strongest surviving design principle from AetoxOS/AetoxClaw is:

> System intelligence beats model size when planning, context, tool use, and
> safety are designed as first-class architecture.

## Documentation Map

- [Aetox CLI architecture handoff](docs/aetox-cli-architecture-handoff.md)
- [Go rebuild roadmap](docs/go-rebuild-roadmap.md)
- [Future agent notes](docs/future-agent-notes.md)
- [Legacy documentation index](docs/legacy-index.md)

Older documents are kept as source material. Some legacy Thai documents may
render oddly in terminals depending on encoding, but the Markdown files remain
as historical architecture evidence.

## Rebuild Direction

The Go rewrite should start from a small vertical slice:

1. `aetox "goal"` accepts a command-line goal.
2. Planner returns a typed task plan.
3. Dispatcher runs a single safe tool through a registry.
4. Permission gate blocks risky actions.
5. Critic validates the result.
6. CLI prints a compact final report.

Only after that slice works should persistent memory, external interfaces, and
more tools be added.
