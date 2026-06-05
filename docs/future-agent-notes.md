# Future Agent Notes

## What To Preserve

- Local-first posture.
- Stateless or near-stateless execution.
- Short, explicit context injection.
- Planner, Dispatcher, Executor, Critic separation.
- Tool registry as the only execution gateway.
- Risk-based permission model.
- Plan history scoped to a single task.
- Model provider abstraction.

## What To Avoid Repeating

- Letting the implementation and docs disagree on the product name.
- Mixing Discord/API/CLI before the CLI core is stable.
- Treating Python module layout as the architecture source of truth.
- Putting too much responsibility into prompt text.
- Adding memory persistence before the transient memory rules are solid.
- Keeping generated caches, coverage files, and scratch experiments in the repo.

## Suggested First Go Interfaces

```go
type Tool interface {
    Name() string
    Description() string
    Risk(action string, params map[string]any) safety.RiskLevel
    Schema() ToolSchema
    Execute(ctx context.Context, action string, params map[string]any) (StepResult, error)
}
```

```go
type LLMClient interface {
    Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
    GenerateStructured(ctx context.Context, req GenerateRequest, schema any) error
}
```

## First Test Stories

- Read-only command does not ask for permission.
- Delete command asks for permission and stops on denial.
- A failed tool step returns a structured error.
- Dispatcher retries only up to the configured limit.
- Critic failure does not crash the whole run.
- Plan context includes previous step output but not full raw history.

## Naming Notes

Use `Aetox CLI` for product language.

Suggested binary name:

```text
aetox
```

Suggested GitHub repo slug:

```text
Aetox-cli
```

This can be changed later without affecting the product name.
