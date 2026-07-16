# Opencode Architecture Reference

**Source:** [github.com/anomalyco/opencode](https://github.com/anomalyco/opencode) — v1.17.8 (TypeScript monorepo)
**Purpose:** เปรียบเทียบสถาปัตยกรรม Opencode กับ Aetox CLI เพื่อใช้เป็นแนวทางในการพัฒนา

---

## 1. Monorepo Structure (opencode packages/)

```
packages/
├── core/          # Agent engine core — tool execution, permission, config, skill registry, LSP
├── cli/           # Terminal entry point (opencode command)
├── console/       # TUI (Terminal UI) — React/Ink based
├── desktop/       # Electron desktop app
├── tui/           # TUI components (shared)
├── ui/            # React UI components (shared)
├── app/           # App shell
├── llm/           # LLM provider integrations (factory pattern)
├── server/        # HTTP server mode (API)
├── plugin/        # Plugin system (hooks system)
├── sdk/           # SDK for building on opencode
├── function/      # Serverless function support
├── web/           # Marketing/docs website (Astro)
├── docs/          # Documentation content (MDX)
├── identity/      # Auth (GitHub Copilot, ChatGPT Plus/Pro)
├── enterprise/    # Enterprise features
├── slack/         # Slack integration
├── containers/    # Docker/container support
├── stats/         # Telemetry/stats collection
├── storybook/     # UI component library
├── http-recorder/ # HTTP recording for debugging
├── effect-drizzle-sqlite/   # SQLite DB layer (Effect + Drizzle)
├── effect-sqlite-node/       # SQLite Node.js bindings
└── opencode/      # Legacy entry point
```

### Aetox CLI เทียบได้กับ:
```
packages/core + packages/cli + packages/llm
```

---

## 2. Core Architecture (packages/core)

### 2.1 Agent Engine
```
Agent (orchestrator)
├── Tool Execution loop
│   ├── read, edit, glob, grep, bash, write, list
│   ├── task (subagent spawning)
│   ├── websearch, webfetch
│   ├── lsp (language server queries)
│   └── skill (user-defined skills)
├── Permission system
│   ├── per-tool allow/ask/deny
│   ├── per-agent override
│   └── pattern matching (glob)
├── Skill Registry
│   ├── auto-discovered from ~/.agents/skills/
│   ├── auto-discovered from ~/.claude/skills/
│   ├── config paths: skills.paths
│   └── remote: skills.urls
├── Plugin System (hooks)
│   ├── config(), event(), tool.execute.before/after
│   ├── chat.message, chat.params, chat.headers
│   ├── shell.env, permission.ask
│   └── experimental.* transforms
├── MCP Integration
│   ├── local servers (stdio)
│   └── remote servers (SSE + OAuth)
├── LSP Integration
│   ├── auto-load LSP per file type
│   └── LSP queries for code analysis
├── Snapshot / Undo System
│   └── filesystem snapshot for undo/redo
└── Compaction System
    ├── auto compaction when context full
    ├── tail_turns preservation
    └── token budget management
```

### Aetox CLI ปัจจุบัน (v0.3.0-dev)
```
internal/
├── app/          # App orchestration + IO
├── cognitive/    # Agent core (think + tool loop)
├── command/      # Intent parsing (CLI args → mode)
├── config/       # Config management (Go structs)
├── debuglog/     # Debug logging
├── grammar/      # Thai grammar/nlp utilities
├── memory/       # Session memory/context
├── model/        # Model provider abstraction (12 providers)
├── provider/     # Provider-specific implementation
├── safety/       # Approval modes (ask, unsafe-only, full-access)
├── skill/        # Skill registry + built-in skills (fs, write, read, git, shell, etc)
├── think/        # Thinking/reasoning level management
└── turn/         # Turn management (interaction cycle)
```

---

## 3. Config Schema (opencode.json)

opencode ใช้ JSON schema validation (`https://opencode.ai/config.json`)

```typescript
interface Config {
  $schema?: string
  model: string              // "provider/model-id"
  small_model?: string       // small model for cheap tasks
  shell?: string             // default shell
  logLevel?: "DEBUG" | "INFO" | "WARN" | "ERROR"
  username?: string          // custom display name
  
  // Agents
  agent: {
    [name: string]: AgentConfig     // build, plan, general, explore
  }
  default_agent?: string
  
  // Model Providers
  provider: {
    [name: string]: ProviderConfig  // anthropic, openai, etc
  }
  disabled_providers?: string[]
  enabled_providers?: string[]
  
  // Skills
  skills: {
    paths: string[]           // additional skill directories
    urls: string[]            // remote skill catalogs
  }
  
  // References
  references: {
    [alias: string]: ReferenceConfig  // local path or git repo
  }
  
  // MCP Servers
  mcp: {
    [name: string]: McpConfig        // local | remote
  }
  
  // Plugins
  plugin: (string | [string, object])[]
  
  // Permissions
  permission: PermissionConfig  // per-tool allow/ask/deny
  
  // LSP
  lsp: boolean | LspConfig
  
  // Formatter
  formatter: boolean | FormatterConfig
  
  // Commands
  command: {
    [name: string]: { template: string, description: string }
  }
  
  // Features
  share: "manual" | "auto" | "disabled"
  autoupdate: boolean | "notify"
  snapshot: boolean
  instructions: string[]    // AGENTS.md paths
  tool_output: { max_lines: number, max_bytes: number }
  compaction: { auto: boolean, tail_turns: number, ... }
  attachment: { image: ImageAttachmentConfig }
  experimental: { policies?: Policy[], ... }
}
```

### Aetox CLI เทียบ
| Feature | opencode | Aetox CLI |
|---------|----------|-----------|
| Config format | `opencode.json` (JSON Schema) | Go struct + CLI flags + env vars |
| Model providers | ~75+ (ผ่าน Models.dev) | 12 (hardcoded) |
| Provider config | `provider.{name}.options.{apiKey, baseURL}` | `--model-api-key`, `--model-base-url` |
| Skills | Auto-discovered + paths + urls | Registry in code only |
| Plugins | Yes (hook system) | No |
| MCP | Yes (local + remote + OAuth) | No |
| LSP | Yes (auto-load per file type) | No |
| Permissions | Per-tool pattern-based | 3 modes (ask, unsafe-only, full-access) |
| Agents | Multiple (build, plan, general) | Single agent |
| References | Local + git repos | No |
| Formatters | Yes (auto-format on edit) | No |
| Undo/Redo | Snapshot-based | No |
| Commands | Custom `/command` templates | No |

---

## 4. Key Architecture Patterns (opencode)

### 4.1 Effect-TS (Functional Effect System)
opencode ใช้ [Effect-TS](https://effect.website/) เป็น core — Rust-style error handling, dependency injection, structured concurrency

```
Effect<Success, Error, Requirements>
├── .pipe() chaining
├── Effect.gen(function* () { ... })  // generator-based syntax
├── Layer system (DI container)
└── Fiber (lightweight coroutine)
```

### 4.2 Tool Execution Pipeline
```
User Input
  → Intent Parser (slash commands, @mentions, plain text)
  → Agent Loop (while steps_remaining > 0)
    → LLM Call → Tool Call → Permission Check (per-tool)
    → Execute Tool → Read Result → Next Iteration
  → Response
```

### 4.3 Skill System (Auto-Discovery)
```
Scan order:
  1. Built-in: customize-opencode (embedded in code)
  2. External: ~/.claude/skills/<name>/SKILL.md
  3. External: ~/.agents/skills/<name>/SKILL.md
  4. Config: skills.paths (additional directories)
  5. Remote: skills.urls (fetched catalogs)
  
Skill format:
  ---
  name: skill-name
  description: When to trigger + what it does
  ---
  # Skill Name
  (markdown body — instructions, examples, references)
```

### 4.4 Plugin Hooks
```
Plugin = (input: PluginInput, options?) => Promise<Hooks>

Available hooks:
  config(cfg)              # mutate merged config
  event(input)             # all bus events
  chat.message / params / headers
  tool.execute.before / after
  tool.definition
  command.execute.before
  shell.env
  permission.ask
  experimental.chat.messages.transform
  experimental.chat.system.transform
  experimental.session.compacting
  experimental.compaction.autocontinue
  experimental.text.complete
  
Special shapes (non-callback):
  tool: { my_tool: { ... } }      # custom tool definition
  auth: { ... }                    # OAuth flow
  provider: { ... }                # custom LLM provider
```

### 4.5 Permission Architecture
```
Permission = Object<tool_name, Rule>
Rule = "allow" | "ask" | "deny" | Object<pattern, action>

Known permission keys:
  read, edit, glob, grep, list, bash, task
  external_directory, todowrite, question
  webfetch, websearch, lsp, doom_loop, skill

Order matters (last match wins):
  { "git *": "allow", "rm *": "deny", "*": "ask" }

Per-agent override:
  agent: {
    plan: { permission: { edit: "deny" } }
  }
```

### 4.6 Provider Architecture
```
Provider registration:
  1. npm package (e.g., @opencode/provider-anthropic)
  2. auto-registered via plugin plugin system
  3. Config: provider.{name}
  
Each provider has:
  - API key management (env → config → prompt)
  - Model catalog (models, capabilities, pricing)
  - Streaming + tool calling support
  - Thinking/reasoning level support

Models.dev schema:
  - Model ID, name, family
  - Capabilities (tool_call, reasoning, attachments)
  - Limits (context, input, output)
  - Cost (input, output, cache)
  - Status (alpha, beta, active, deprecated)
```

---

## 5. Comparison: Aetox CLI ↔ opencode

### Aetox CLI ที่แข็งแรงอยู่แล้ว
| Component | Status | Notes |
|-----------|--------|-------|
| Agent loop (think → tool → result) | ✅ | cognitive/agent.go |
| Skill registry | ✅ | internal/skill/ |
| Built-in tools (fs, write, read, git, shell) | ✅ | internal/skill/*.go |
| Model providers (12) | ✅ | internal/model/ |
| Approval modes (3 levels) | ✅ | internal/safety/ |
| Config management | ✅ | internal/config/ |
| Debug logging | ✅ | internal/debuglog/ |
| Thinking level | ✅ | internal/think/ |
| Turn management | ✅ | internal/turn/ |

### Aetox CLI ที่ยังขาด / ควรเพิ่ม
| Feature | Priority | Why |
|---------|----------|-----|
| **JSON config** (`aetox.json`) | High | ยืดหยุ่นกว่า CLI flags, รองรับ skill paths, providers, permissions |
| **Skill auto-discovery** | High | scan `~/.agents/skills/` + config paths แบบ opencode |
| **Permission per-tool** | High | แทนที่ 3 modes แบบคร่าวๆ ด้วย pattern matching |
| MCP support | Medium | เชื่อมต่อ external tools ผ่าน Model Context Protocol |
| LSP integration | Medium | ให้ agent อ่าน code context ได้แม่นยำขึ้น |
| Plugin system | Medium | ให้ third-party ขยายความสามารถได้ |
| Snapshot/Undo | Medium | safety net เวลา agent แก้โค้ดผิด |
| Multiple agents | Medium | plan mode (read-only) + build mode |
| Custom commands | Low | `/command` templates |
| Formatter integration | Low | auto-format after edit |
| Remote skill catalogs | Low | `skills.urls` |

### ภาษา
| | opencode | Aetox CLI |
|---|----------|-----------|
| Core | TypeScript (Effect-TS) | **Go** |
| UI | React/Ink (TUI) + React (Desktop) | Terminal (basic) |
| DB | SQLite (Effect + Drizzle) | — |
| Package | npm + Turbrepo | Go modules |

**ข้อดีของ Go สำหรับ Aetox:** single binary, cross-compile ง่าย, performance สูง, concurrent แรง

---

## 6. Architectural Recommendations

### 6.1 Config Layer (เพิ่ม)
```
aetox.json (รากโปรเจกต์)
├── model: { provider, name, api_key, base_url }
├── agent: { plan, build }  // multiple agents
├── skills: { paths, urls }
├── permission: { edit, bash, ... }
├── mcp: { server_name: { type, command/url } }
├── hooks: { shell: { env: {...} } }
```

### 6.2 Permission System Upgrade
```go
type PermissionRule struct {
    Pattern string           // glob pattern, e.g. "rm *"
    Action  PermissionAction // "allow" | "ask" | "deny"
}

type PermissionConfig struct {
    Read   []PermissionRule
    Edit   []PermissionRule
    Bash   []PermissionRule
    Shell  []PermissionRule
    // ... per tool
}

// Resolution: last match wins
func (p *PermissionConfig) Resolve(tool string, args string) PermissionAction
```

### 6.3 Skill Auto-Discovery
```go
// Scan paths in order:
// 1. ~/.agents/skills/<name>/SKILL.md
// 2. ~/.claude/skills/<name>/SKILL.md
// 3. aetox.json → skills.paths
// 4. aetox.json → skills.urls (HTTP fetch)
```

### 6.4 MCP Integration
```go
type MCPServer struct {
    Name    string
    Type    string    // "local" | "remote"
    Command []string  // local: ["npx", "server"]
    URL     string    // remote: "https://..."
    Env     map[string]string
}

type MCPClient struct {
    // stdio (local) or SSE (remote)
    Call(ctx, tool, args) → result
    ListTools(ctx) → []ToolDefinition
}
```

### 6.5 Multiple Agents
```go
type AgentMode string
const (
    AgentBuild AgentMode = "build"  // full read/write/exec
    AgentPlan  AgentMode = "plan"   // read-only
)
```

### 6.6 Snapshot System
```go
type Snapshot struct {
    ID        string
    Timestamp time.Time
    Files     []FileSnapshot  // path → content hash + backup
}

type SnapshotStore interface {
    Create(ctx, files) → Snapshot
    Restore(ctx, id) → error
    List(ctx) → []Snapshot
}
```

---

## 7. Current Aetox CLI Internal Map (v0.3.0-dev)

```
cmd/aetox/main.go          ← Entry point: flags, config, agent init, mode dispatch
internal/
├── app/
│   ├── app.go             ← App orchestrator (interactive + once modes)
│   ├── console.go         ← I/O (stdin/stdout)
│   ├── interactive_input.go ← Terminal input handling
│   └── app_test.go
├── cognitive/
│   ├── agent.go           ← Agent loop: think → tool → result
│   └── agent_test.go
├── command/
│   └── ...                ← Intent parsing (CLI args → Mode{Interactive, Once, Help, Version})
├── config/
│   ├── config.go          ← Config struct + Load/Save
│   └── config_test.go
├── debuglog/              ← Debug logging
├── grammar/               ← Thai NLP utilities
├── memory/                ← Session context/memory
├── model/
│   ├── types.go           ← Provider interfaces + ToolDefinition types
│   ├── factory.go         ← Provider factory
│   ├── bootstrap.go       ← Provider boot sequence
│   ├── provider_catalog.go ← Model catalog
│   ├── thinking_capabilities.go ← Thinking level support
│   ├── noop.go            ← Noop provider (testing)
│   ├── ollama.go          ← Ollama provider
│   ├── openai_compatible.go  ← OpenAI-compatible provider
│   ├── openrouter.go      ← OpenRouter provider
│   └── ..._test.go
├── provider/              ← Provider-specific implementation
├── safety/
│   └── ...                ← Approval mode (ask, unsafe-only, full-access)
├── skill/
│   ├── skill.go           ← Skill interface + Registry
│   ├── dispatcher.go      ← Dispatch by name
│   ├── defaults.go        ← Default skills registration
│   ├── fs.go              ← File system skill
│   ├── write.go           ← Write file skill
│   ├── read.go            ← Read file skill
│   ├── shell.go           ← Shell command skill
│   ├── git.go             ← Git skill
│   ├── github_tools.go    ← GitHub tools skill
│   ├── delete.go          ← Delete file skill
│   ├── echo.go            ← Echo skill (testing)
│   ├── time.go            ← Time skill
│   ├── help.go            ← Help skill
│   ├── input.go           ← Input handling
│   ├── list.go            ← List skill
│   └── output.go          ← Output formatting
├── think/                 ← Thinking/reasoning level
└── turn/                  ← Turn management
```
