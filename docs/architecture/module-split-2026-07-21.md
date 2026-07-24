# Module Split Proposal — 3 Go Modules by Separation of Concerns

> **Date:** 2026-07-21
> **Status:** Proposed. The `go.mod`/`go.work` scaffold this doc describes was **deleted 2026-07-25** ([ARCHITECTURE.md §28](../../ARCHITECTURE.md)) after four days of empty directories confused readers about where the code lives. The reason for the split below — `internal/model` importing `internal/provider` — is unchanged and still unaddressed. Re-create the structure when migration actually begins, not before.

## Why Split?

Current single-module `github.com/Mike0165115321/Aetox` has one real architecture flaw:

```
internal/model ──imports──→ internal/provider
```

**Abstraction depends on implementation** — `model` (Provider interface, Message types) imports `provider` (catalog/runtime enums). Any consumer of `model` transitively pulls in the provider catalog.

Other than this, the rest of `internal/` is well-structured. The split isn't fixing a broken codebase — it's creating future boundaries.

## The 3 Modules

```
aetox/                          ← go.work workspace root
│
├── engine/                     ← go.mod: github.com/Mike0165115321/Aetox/engine
│   ● Cognitive loop (agent, think, plan, turn)
│   ● Skill registry & dispatcher (skill, command)
│   ● Memory & safety (memory, safety)
│   ● Model interface (model/types.go — Provider, Message, Request, Response)
│   ● Config & audit (config, audit, debuglog, grammar)
│
├── providers/                  ← go.mod: github.com/Mike0165115321/Aetox/providers
│   ● depends on: engine (for model interface)
│   ● Provider implementations (OpenAI, Ollama, OpenRouter, Anthropic, Gemini)
│   ● Provider factory + catalog (SupportedProviders, DefaultModel, etc.)
│   ● Thinking level normalization
│
├── cli/                        ← go.mod: github.com/Mike0165115321/Aetox/cli
│   ● depends on: engine + providers
│   ● CLI entry point (flag parsing, interactive menu, stdin/stdout)
│   ● Currently in: cmd/aetox/
│
├── desktop/                    ← (stays in root go.mod for now)
│   ● Wails GUI (Svelte frontend + Go backend)
│   ● WebView2 browser, SQLite sessions, terminal emulation
│
├── internal/                   ← (code lives here until migration)
│   ● All current source files
│   ● Will be migrated to engine/ + providers/ + cli/ in phases
│
└── go.work                     ← workspace connecting engine + providers + cli
```

## What Goes Where (Phase 1)

### → `engine/` (from `internal/`)

| Source | Target | Notes |
|--------|--------|-------|
| `internal/cognitive/` | `engine/cognitive/` | Agent loop |
| `internal/think/` | `engine/think/` | Thinking levels |
| `internal/plan/` | `engine/plan/` | Planning |
| `internal/turn/` | `engine/turn/` | Turn executor |
| `internal/memory/` | `engine/memory/` | Context/memory |
| `internal/skill/` | `engine/skill/` | Skill registry + tools |
| `internal/safety/` | `engine/safety/` | Permission gates |
| `internal/command/` | `engine/command/` | Command parsing |
| `internal/config/` | `engine/config/` | Config types |
| `internal/audit/` | `engine/audit/` | Audit log |
| `internal/debuglog/` | `engine/debuglog/` | Debug log |
| `internal/grammar/` | `engine/grammar/` | Grammar |
| `internal/model/types.go` | `engine/model/` | **Interface only** (Provider, Message, etc.) |
| `internal/app/` | `engine/app/` | Orchestration (shared by CLI + desktop) |

### → `providers/` (from `internal/model/` + `internal/provider/`)

| Source | Target | Notes |
|--------|--------|-------|
| `internal/model/factory.go` | `providers/` | NewProvider, ProviderOptions |
| `internal/model/bootstrap.go` | `providers/` | BootstrapProvider, BootstrapResult |
| `internal/model/noop.go` | `providers/` | Noop provider impl |
| `internal/model/ollama.go` | `providers/` | Ollama provider impl |
| `internal/model/openai_compatible.go` | `providers/` | OpenAI-compatible impl |
| `internal/model/openrouter.go` | `providers/` | OpenRouter impl |
| `internal/model/provider_catalog.go` | `providers/` | SupportedProviders, DefaultModel, etc. |
| `internal/model/thinking_capabilities.go` | `providers/` | Thinking level normalization |
| `internal/provider/catalog.go` | `providers/` | Provider runtime catalog |
| All test files | `providers/` | |

**Package name change:** `package model` → `package providers`
**Import in consumers:** `providers.NewProvider()`, `providers.SupportedProviders()`

### → `cli/` (from `cmd/aetox/`)

| Source | Target | Notes |
|--------|--------|-------|
| `cmd/aetox/main.go` | `cli/main.go` | Entry point |
| `cmd/aetox/main_windows.go` | `cli/main_windows.go` | Win32 UTF-8 |
| `cmd/aetox/main_other.go` | `cli/main_other.go` | Non-Windows |
| `cmd/aetox/main_test.go` | `cli/main_test.go` | Tests |

## Dependency Graph After Migration

```
                     ┌──────────┐
                     │  cli/    │
                     │  main.go │
                     └────┬─────┘
                          │
              ┌───────────┼───────────┐
              │           │           │
              ▼           ▼           ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ engine/  │ │ engine/  │ │ engine/  │
        │ cognitive│ │ skill/   │ │ app/     │
        └────┬─────┘ └────┬─────┘ └────┬─────┘
             │            │            │
             ▼            ▼            │
        ┌──────────┐ ┌──────────┐      │
        │ engine/  │ │ engine/  │      │
        │ model/   │ │ safety/  │      │
        │ (interface)│ └──────────┘     │
        └────┬─────┘                   │
             │                         │
             ▼                         │
        ┌──────────┐                   │
        │providers/│◄──────────────────┘
        │ (impl)   │
        └──────────┘
```

Key property: **engine/ has zero dependency on providers/** — the dependency arrow points FROM implementations TO the interface, not the other way around.

## What This Unlocks

1. **CLI without Wails** — `go build ./cli` doesn't download webview2/sqlite
2. **Engine as library** — `import "github.com/Mike0165115321/Aetox/engine"` for embedding
3. **Provider plugin model** — providers/ module is the contract for third-party providers
4. **Desktop stays heavy** — desktop/ keeps all GUI deps, doesn't infect other modules
5. **Directional Cognition isolation** — cognitive code in engine/ can be developed/tested without provider implementations

## What Stays the Same Until Migration

- `internal/` and `cmd/` — all source files remain, project builds from root `go.mod` as before
- `desktop/` — still uses root `go.mod`, no changes
- go.work is ignored when building from root — only affects workspace-aware commands
