# Inspection Rules

Failure mode prevented:

The agent invents architecture from filenames, framework stereotypes, or user
intent without reading the real project.

## Rules

1. Inspect actual files before making architecture claims.
2. Prefer root-level orientation first, then inspect relevant subdirectories.
3. Read project docs before replacing them with new assumptions.
4. Treat configs, package files, lockfiles, tests, and deployment files as architecture signals.
5. Do not assume a database, backend, AI service, or deployment platform exists
   unless files or user context show it.
6. If a file cannot be read, record that limitation.
7. Keep inspection focused on the requested scope when the project is large.
8. Prefer the narrowest useful scope before mapping the whole system.
9. Use available inspection tools — including codebase graph queries when a
   codebase intelligence MCP is available — to gather evidence faster, then
   interpret the evidence before making architecture claims.
10. Reuse existing architecture maps, handoff notes, ADRs, current-state docs,
    and Mermaid sources before re-mapping stable areas.
11. Do not re-map stable areas unless evidence conflicts, the scope touches
    that boundary, or the user asks for a full re-check.

## Minimum Evidence

Before documenting architecture, gather evidence for:

- Project type
- Entry points
- Major modules
- Data ownership
- External integrations
- Build and runtime path
- Testing or validation surface

Tool output can support evidence, but it is not architecture by itself.

## Inspection Budget for Large Repositories

Start with:

- Top-level structure
- README and existing docs
- Config files
- Entry points
- Routing, module registries, or service registries
- Relevant modules only

Inspect deeper files only when evidence, risk, or selected scope requires it.
