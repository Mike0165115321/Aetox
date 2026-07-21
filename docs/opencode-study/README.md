# Opencode Deep-Dive — Reading Real Source, Not Just Comparing Tables

> **Source studied:** `github.com/anomalyco/opencode` @ commit `76ced5418f50ee5cfb4c256af358bc0ab063a51b` (2026-07-21), shallow-cloned into a scratch directory and read directly (not guessed from docs/READMEs).
> **Why this exists:** `docs/architecture-reference-opencode.md` and `docs/competitor-research.md` compare Aetox to opencode at the *package/feature* level (structural, from opencode's own docs). This directory goes one level deeper — for each capability we still need to build, we read opencode's actual shipped implementation to extract the real design decisions (data shapes, call sites, precedence rules, failure modes) instead of re-deriving them blind. Per the user's direction (2026-07-22): "many parts need to reach opencode parity anyway — study them, split into sections."
> **Caveat:** opencode is mid-migration between two parallel implementations in several areas (a legacy "v1" system that's actually wired into the runtime, and an Effect-based "v2" rewrite that isn't yet driving execution). Each doc below states explicitly which version was read for which claim. File:line citations point at the commit above — re-clone at that commit if a citation needs re-verifying (the scratch clone itself is not persisted).

## Sections

| Doc | Covers | Priority for Aetox |
|---|---|---|
| [mcp.md](mcp.md) | MCP client: config shape, connection lifecycle, tool bridging, OAuth, permission integration, error handling | **Next up** — `MCP-SUPPORT-PLAN.md` already scoped this as the next build item |
| [permissions.md](permissions.md) | opencode's real permission engine (glob matching, precedence, per-agent layering, persistence) vs. the `safety.PermissionConfig` we just shipped | Reference/validation for what we already built + gaps to consider |
| [snapshot.md](snapshot.md) | Filesystem snapshot/undo (git-tree-backed) — a capability Aetox has none of yet | Not yet scheduled, but cheap to build once MCP is done (git-only, no new infra) |
| [plugin-hooks.md](plugin-hooks.md) | Event-driven plugin hook system (`tool.execute.before/after`, `chat.message`, ...) | Scheduled after MCP per the original gap-priority order |
| [agents.md](agents.md) | Multi-agent / sub-agent system (`task` tool, agent profiles, nesting) | Directly informs wiring `internal/orchestrator` (built, unused — ARCHITECTURE.md §10) |

## Cross-cutting findings worth reading first

These surfaced in more than one research pass and shape how the sections below should be read together:

1. **opencode doesn't hand-roll protocol-level plumbing when a good library exists.** MCP uses the official `@modelcontextprotocol/sdk` npm package for the JSON-RPC/OAuth machinery — opencode only supplies transport config, storage, and the tool-bridging adapter. `MCP-SUPPORT-PLAN.md`'s original plan assumed we'd hand-write a stdio JSON-RPC client; check for a comparable official/mature Go MCP SDK before doing that (ladder rung 5: already-installed/available dependency beats reinventing a JSON-RPC+OAuth stack).
2. **"Modes"/agent presets are just named permission-ruleset + prompt bundles, not separate code paths.** opencode's `plan` agent is structurally identical to `build` — same tool loop, same everything — except its permission ruleset denies `edit` outside a scratch directory. This is directly reusable for Aetox: we don't need a special "plan mode" execution path, just an agent-profile concept that swaps in a different `safety.PermissionConfig` + system prompt.
3. **Not every advertised extension point is actually wired.** opencode's `Hooks` type documents a `permission.ask` hook, but no code anywhere calls it — permission decisions are 100% ruleset-driven, plugins cannot currently influence them despite the type existing. Lesson for our own docs: always grep for the call site, not just the type/schema, before citing a feature as real.
4. **Precedence is almost always "last write wins over a flat, order-built list,"** never a specificity-scoring algorithm. This shows up in both the permission engine (rule arrays, `Array.findLast`) and agent permission layering (defaults → global config → per-agent config, appended in that order). Simpler to port to Go than a "most specific pattern wins" system would be, and matches what we already built in `internal/safety.PermissionConfig.Resolve`.
5. **Where opencode chose robustness over cleanliness, it's usually because a real-world integration forced it**, not fashion: two-transport MCP fallback (StreamableHTTP → SSE), process-tree cleanup for stdio children, tolerating malformed `outputSchema` from third-party MCP servers, pagination-cursor-loop guards. These are exactly the kind of "hard-won lesson" details worth porting even though they add code, because the alternative is a support burden later — ponytail's "don't skip real error handling" boundary applies here, not the lazy-shortcut ladder.
