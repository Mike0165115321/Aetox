# shadcn MCP Server

The CLI includes an MCP server that lets AI assistants search, browse, view, and install items from registries.

---

## Setup

```bash
shadcn mcp        # start the MCP server (stdio)
```

Add it to Aetox as a stdio MCP server (Settings → MCP servers) with the command
`shadcn mcp`, and the registry tools below become available in a chat. (`shadcn
mcp init` writes config for other editors and is not needed here.)

---

## Tools

> **Tip:** MCP tools handle registry operations (search, view, install). For project configuration (aliases, framework, Tailwind version), use `npx shadcn@latest info`, there is no MCP equivalent.

### `shadcn:get_project_registries`

Returns registry names from `components.json`. Errors if no `components.json` exists.

**Input:** none

### `shadcn:list_items_in_registries`

Lists all items from one or more registries. Registries can be configured
namespaces such as `@acme`, public GitHub sources such as `owner/repo`, or
registry catalog URLs. Omit `registries` to list from every registry configured
in `components.json`.

**Input:** `registries` (string[], optional, omit for all configured), `types` (string[], optional, e.g. `["ui", "block"]`), `limit` (number, optional, defaults to 100), `offset` (number, optional)

### `shadcn:search_items_in_registries`

Fuzzy search across registries. Registries can be configured namespaces, public
GitHub sources, or registry catalog URLs. Omit `registries` to search every
registry configured in `components.json`, e.g. "find me a hero" across all
configured registries.

**Input:** `registries` (string[], optional, omit for all configured), `query` (string), `types` (string[], optional, e.g. `["ui", "block"]`), `limit` (number, optional, defaults to 100), `offset` (number, optional)

### `shadcn:view_items_in_registries`

View item details including full file contents.

**Input:** `items` (string[]), e.g.
`["@shadcn/button", "@shadcn/card", "owner/repo/item"]`

### `shadcn:get_item_examples_from_registries`

Find usage examples and demos with source code. Omit `registries` to search
every registry configured in `components.json`.

**Input:** `registries` (string[], optional, omit for all configured), `query` (string), e.g. `"accordion-demo"`, `"button example"`

### `shadcn:get_add_command_for_items`

Returns the CLI install command.

**Input:** `items` (string[]), e.g. `["@shadcn/button"]`

### `shadcn:get_audit_checklist`

Returns a checklist for verifying components (imports, deps, lint, TypeScript).

**Input:** none

---

## Configuring Registries

Namespaced and authenticated registries are set in `components.json`. The
`@shadcn` registry is always built-in. Public GitHub registries can also be used
directly as `owner/repo` registry sources when the repository has a root
`registry.json`; they do not need `components.json` configuration.

```json
{
  "registries": {
    "@acme": "https://acme.com/r/{name}.json",
    "@private": {
      "url": "https://private.com/r/{name}.json",
      "headers": { "Authorization": "Bearer ${MY_TOKEN}" }
    }
  }
}
```

- Names must start with `@`.
- URLs must contain `{name}`.
- `${VAR}` references are resolved from environment variables.

Community registry index: `https://ui.shadcn.com/r/registries.json`
