# internal/model — provider abstraction + 11 client implementations

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Debt: §6.2 (imports `internal/provider` — wrong direction) · Migration target: [module-split-2026-07-21.md](../../docs/architecture/module-split-2026-07-21.md)

**What it is:** the one interface the engine talks LLMs through, plus every concrete client. When the module split happens, the interface/types go to `engine/` and the clients to `providers/` — until then everything is one flat package.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Provider` interface + `Message`/`Request`/`Response`/`ToolDefinition`/`ToolCall` ([types.go](types.go)) | The whole engine (`cognitive`, `turn`, `memory`) speaks these types only. Tool calling, streaming, usage, reasoning-content all cross this boundary. |
| `StreamingProvider.StreamComplete(ctx, req, onChunk, onReasoningChunk)` ([types.go](types.go)) | Two separate callbacks, not one tagged stream: `onChunk` is the visible reply, `onReasoningChunk` is a provider's own reasoning/thinking tokens as they arrive (DeepSeek `reasoning_content`, Anthropic `thinking_delta`, Ollama `reasoning_content`) — added 2026-07-23 so the desktop can show live "thinking" text instead of only the final `Response.ReasoningContent`. Both nil-safe; a provider with nothing to say on either just doesn't call it. |
| `BootstrapProvider(BootstrapOptions)` ([bootstrap.go](bootstrap.go)) | The one call front ends make: provider name + model + key + base URL → ready `Provider` (or error + warning). Both `cmd/aetox` and `desktop/app.go` route through it. |
| Factory + catalog ([factory.go](factory.go), [provider_catalog.go](provider_catalog.go)) | Name → client constructor; `SupportedProviders`/`DefaultModel`/`DefaultBaseURL`/`ModelChoices*`/`RequiresAPIKey`/`ResolveModelAPIKey`. Note: `internal/provider/catalog.go` holds overlapping data — the §6.2/§6.3 duplication, not yet reconciled. |
| `ResolveThinkingCapabilities` ([thinking_capabilities.go](thinking_capabilities.go)) | Curated per-provider/model thinking levels; `Native=false` means "guessed", and the desktop hides guessed levels. |

## Clients

- [openai_compatible.go](openai_compatible.go) — the workhorse: OpenAI, DeepSeek, Gemini (OpenAI endpoint), Groq, Mistral, xAI, Z.AI, and friends all reuse it with different base URLs.
- [anthropic.go](anthropic.go) — real Messages-API client (content blocks, `x-api-key` — *not* OpenAI-shaped, hence its own file). Added 2026-07-22, see ARCHITECTURE.md §6.9.
- [ollama.go](ollama.go) · [openrouter.go](openrouter.go) · [noop.go](noop.go) (offline/test stand-in).

## Rules of thumb

- New OpenAI-compatible provider = catalog entry + base URL, **not** a new client file. Only genuinely different wire formats (like Anthropic) earn a file.
- Don't add engine imports here beyond `internal/provider` (and work toward removing that one — dependency direction must end up `providers → engine`, never the reverse).
