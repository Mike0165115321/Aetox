# Architecture Review: Aetox CLI (Current State)

อัปเดตล่าสุด: 2026-06-09 (patch: Risk 1 Executor Split, Risk 2 Model Capability Catalog, Risk 3 Shell Audit Log, off-think→off rename)  
โหมดการวิเคราะห์: Existing System Mapping  
Pass level: Full Mode  
วัตถุประสงค์: อัปเดตภาพ current-state architecture ของ Aetox CLI หลัง refactor รอบ risk-mitigation ทั้ง 3 ด้าน — turn executor split, model capability catalog + conservative fallback, shell audit trail, และ off-think→off rename ทั่ว codebase

## 1. ขอบเขตและหลักฐานที่ตรวจ

เอกสารนี้อ้างอิงจากโค้ดและเอกสารที่ตรวจจริงใน repository:

- `cmd/aetox/main.go`
- `internal/app/app.go`
- `internal/cognitive/agent.go`
- `internal/config/config.go`
- `internal/memory/context.go`
- `internal/model/openai_compatible.go`
- `internal/model/provider_catalog.go`
- `internal/model/thinking_capabilities.go`
- `internal/model/types.go`
- `internal/provider/catalog.go`
- `internal/audit/audit.go`
- `internal/safety/safety.go`
- `internal/skill/defaults.go`
- `internal/skill/dispatcher.go`
- `internal/skill/skill.go`
- `internal/skill/shell.go`
- `internal/think/think.go`
- `internal/turn/executor.go`
- `internal/turn/infer.go`
- `internal/turn/result.go`
- `docs/architecture-aetox.md`
- `docs/adr/0001-native-tool-calling-foundation.md`

Inspection limitations:

- เอกสารนี้เน้น current state ของ execution path, provider/model integration, thinking architecture, และ terminal UX state
- เอกสาร target architecture ใน [architecture-aetox.md](E:\Aetox\Aetox-cli\docs\architecture-aetox.md) ถูกใช้เป็นบริบทเปรียบเทียบ ไม่ใช่ source of truth หลัก
- ข้อความที่เป็นข้อเสนออนาคตถูกแยกไว้ในส่วน open questions และ risks เท่านั้น

## 2. Executive Summary

ข้อเท็จจริงที่ยืนยันได้:

- Aetox CLI ยังเป็น Go application แบบ single local process ไม่มี backend service แยก
- runtime path ปัจจุบันไม่ได้เป็นแค่ `app -> skill/agent` แบบเดิมอีกแล้ว แต่มี `internal/turn.Executor` เป็น orchestration layer กลางของหนึ่ง turn
- model layer ถูกแยกเป็น 2 ชั้นชัดขึ้น:
  - `internal/provider` ถือ static provider catalog
  - `internal/model` ถือ runtime bootstrap, live model discovery, request shaping, และ per-model thinking capability resolution
- ระบบรองรับ model-native thinking levels แบบ provider/model aware แล้ว และ normalize ระดับที่ผู้ใช้เลือกก่อนใช้งานจริง
- `ModelPreference` ตอนนี้เก็บมากกว่า provider/model/base URL โดยเก็บ `think_level` และ API key map แยกตาม provider ด้วย
- terminal UI แสดง model status พร้อมระดับคิดที่ใช้อยู่จริงใน header ตลอด และแยก context line ออกจาก model line
- native tool-calling path มีอยู่จริงแล้วใน current state:
  - model contract รองรับ `tools`, `tool_calls`, `tool` messages และ `reasoning_content`
  - `skill.Dispatcher` เผย `ToolDefinitions()` และ `ExecuteTool(...)`
  - `cognitive.Agent` มี bounded tool loop
  - `turn.Executor` ยังบังคับ safety check ก่อน execute tool
- Gemini ถูกเพิ่มเป็น first-class provider ในโครงเดียวกับค่ายหลักอื่น โดยใช้ OpenAI-compatible runtime แต่มี live model discovery path ของตัวเอง
- **shell มี audit log แล้ว**: `internal/audit` บันทึกทุก shell execution แบบ JSONL append-only ที่ `~/.aetox/shell-audit.log` (non-fatal write); shell ยังไม่อยู่ใน model tool surface
- **`internal/turn` ถูกแยกเป็น 3 ไฟล์**: `executor.go` (orchestration), `infer.go` (intent inference/regex), `result.go` (summarize/fallback/sanitize) — ลด monofile 1,789→718 บรรทัด โดยไม่เปลี่ยน behavior
- **Model Capability Catalog**: `BuildCapabilityCatalog(provider, discoveredModels)` เป็น pure function ที่ enrich live/static model list ด้วย `ResolveThinkingCapabilities`; unknown provider ได้ audit entry `Supported: false`
- **Conservative fallback**: resolver defaults ที่เคยคืน `Supported: false` ตอนนี้คืน capability พื้นฐาน `[low,medium,high,off]` แทน — model ใหม่ที่ไม่รู้จักยังใช้ think ได้
- **off-think → off**: canonical thinking level เปลี่ยนจาก `"off-think"` เป็น `"off"` ทั่ว codebase; DeepSeek, fallback, conservative fallback ล้วนใช้ `"off"`; `think.LevelNoThinking = "off"`

Reasonable inferences:

- seam ระหว่าง "provider metadata" กับ "provider runtime behavior" ดีขึ้นอย่างมีนัยสำคัญ ทำให้เพิ่ม provider ใหม่โดยแตะจุดที่ชัดขึ้น
- thinking architecture ตอนนี้ถูกย้ายออกจาก UI-specific logic ไปอยู่ใน model capability layer มากขึ้น จึงลดความเสี่ยงของ label/behavior drift
- `internal/turn` ถูกแยก concern เป็น 3 ไฟล์ (executor/infer/result) — ลด blast radius ของ monofile และทำให้ infer logic test ได้อิสระจาก orchestration
- `BuildCapabilityCatalog` เป็น pure function ที่ bridge ระหว่าง live discovery (model names) กับ thinking resolution (capabilities) โดยไม่ต้องเพิ่ม HTTP call
- conservative fallback ทำให้ unknown/future model ยังมี thinking พื้นฐาน — model ใหม่ไม่พังเงียบอีกต่อไป

## 3. System Boundary

| Area | Current state |
| --- | --- |
| Frontend | Terminal UI ใน `internal/app`, มี header status + prompt status แยกบรรทัด |
| Backend service | ไม่พบ service แยก; logic ทั้งหมดรันใน process เดียว |
| Database | ไม่พบ database |
| Local persistence | `model-preference.json` ผ่าน `internal/config`, เก็บ provider/model/base URL/think level/provider API keys; `~/.aetox/shell-audit.log` ผ่าน `internal/audit` |
| AI runtime | `internal/cognitive` + `internal/model` + `internal/provider` + `internal/think` + `internal/turn` |
| External model services | OpenRouter, OpenAI-compatible providers (เช่น OpenAI, DeepSeek, Gemini, Groq, Mistral, Together, Perplexity, Cohere, LM Studio/LocalAI), Ollama |
| Background jobs/workers | ไม่พบ |
| Safety layer | `internal/safety` ถูกใช้ทั้ง explicit skill path, inferred tool path, และ model-selected tool path |
| Tests/quality gates | คุณภาพฝั่ง model/provider/think ดีกว่าก่อน แต่ UX/app/turn path ยังเป็น seam ที่มีความเสี่ยงกว่า |

สิ่งที่ไม่พบจากโค้ดปัจจุบัน:

- Web server
- Authentication/authorization
- Queue/worker
- Database migrations
- Deployment manifests

## 4. Architecture Overview

```mermaid
flowchart TD
    CLI["cmd/aetox/main.go"]
    Config["internal/config"]
    ProviderCatalog["internal/provider"]
    App["internal/app"]
    Turn["internal/turn.Executor"]
    TurnInfer["internal/turn/infer.go"]
    TurnResult["internal/turn/result.go"]
    Safety["internal/safety"]
    Skill["internal/skill.Dispatcher"]
    Audit["internal/audit"]
    Agent["internal/cognitive.Agent"]
    Memory["internal/memory"]
    Model["internal/model"]
    ModelCap["ModelCapability Catalog"]
    Think["internal/think + thinking capability resolver"]
    External["Remote / Local model providers"]

    CLI --> Config
    CLI --> ProviderCatalog
    CLI --> Model
    CLI --> App
    App --> Turn
    Turn --> TurnInfer
    Turn --> TurnResult
    Turn --> Safety
    Turn --> Skill
    Turn --> Agent
    Agent --> Memory
    Agent --> Model
    Model --> ProviderCatalog
    Model --> ModelCap
    Model --> Think
    Model --> External
    Skill --> Audit
```

สถาปัตยกรรมที่เปลี่ยนชัดจากเอกสารรุ่นก่อน:

- `internal/provider` ถูกแยกออกมาเป็นบ้านของ provider metadata อย่างชัดเจน
- `internal/turn.Executor` กลายเป็น execution orchestration layer แทนการแบก flow ไว้ใน `internal/app` เป็นหลัก; ถูกแยกเป็น 3 ไฟล์: `executor.go` (route/approve), `infer.go` (NLP intent inference), `result.go` (summarize/fallback)
- `internal/model` ไม่ได้เป็นแค่ adapter HTTP อีกต่อไป แต่เป็นชั้น runtime intelligence สำหรับ live model discovery, thinking normalization, และ `ModelCapability` catalog
- `internal/audit` ถูกเพิ่มเป็น package ใหม่สำหรับ shell audit trail (non-fatal, append-only JSONL)
- `BuildCapabilityCatalog` เป็น bridge ระหว่าง live discovery กับ thinking resolution โดยไม่ต้องเพิ่ม HTTP call

## 5. Module Map

| Module | Responsibility | Evidence strength |
| --- | --- | --- |
| `cmd/aetox` | parse flags, load config/preference, prompt model selection, normalize think level, bootstrap provider, persist preference, compose app runtime | Direct |
| `internal/config` | load runtime config defaults, save/load `ModelPreference`, canonicalize per-provider API key storage | Direct |
| `internal/provider` | static provider catalog: aliases, env keys, runtime class, fallback model, provider-level capabilities | Direct |
| `internal/model` | provider abstraction, bootstrap, request/response contract, live model discovery, provider-specific reasoning payload shaping, per-model thinking capability resolution, `ModelCapability` catalog (`BuildCapabilityCatalog`) | Direct |
| `internal/think` | generic think-level parsing/normalization contract used by CLI/runtime; `LevelNoThinking = "off"` | Direct |
| `internal/app` | terminal UX, header/prompt rendering, interactive loop, model switching entrypoint, thinking spinner/status | Direct |
| `internal/turn` | one-turn orchestration (3 files): `executor.go` — route/approve/orchestrate; `infer.go` — intent inference/regex/NLP parsing; `result.go` — summarize/fallback/sanitize | Direct |
| `internal/cognitive` | conversation agent, streaming fallback, bounded model tool loop, context updates, request assembly | Direct |
| `internal/skill` | skill registry/dispatcher plus opt-in tool surface via `ToolDefinition()` and `ExecuteTool(...)` | Direct |
| `internal/safety` | risk assessment before executing commands/tools | Direct |
| `internal/audit` | append-only JSONL audit log for shell execution (`~/.aetox/shell-audit.log`), non-fatal write | Direct |
| `internal/memory` | bounded in-memory conversation context | Direct |

Observed default registered skills:

- `help`
- `echo`
- `time`
- `list`
- `read`
- `github_repo_summary`
- `git`
- `fs`
- `shell`
- `write`
- `delete`
- `plugin_install`

Observed tool-capable surface:

- `time`
- `list`
- `read`
- `write`
- `delete`
- `github_repo_summary`
- `plugin_install`

หมายเหตุ: เอกสารนี้ยืนยันเฉพาะ tool-capable skills ที่ตรวจพบ `ToolDefinition()` และ `ExecuteTool(...)` จากโค้ดที่อ่านจริง

## 6. Runtime Flow

### 6.1 Startup Flow

1. `cmd/aetox/main.go` parse global flags รวม `--model-provider`, `--model-name`, `--model-base-url`, `--model-api-key`, `--model-context-tokens`, และ `--think`
2. `model-name` สามารถเขียนรูป `model(think-level)` ได้ และถูก parse แยกจาก `--think`
3. `internal/config.Load` สร้าง runtime config โดยมี default `ThinkLevel = low` เมื่อยังไม่ระบุ
4. ระบบโหลด `ModelPreference` จาก user config directory ถ้ามี
5. ถ้าไม่มี explicit model config และมี stored preference ระบบจะ reuse provider/model/base URL/API key/think level ตามที่เก็บไว้
6. ถ้าเป็น interactive และยังไม่มี stored preference ระบบจะเข้า flow เลือก:
   - provider
   - model
   - thinking level
7. ก่อน bootstrap จริง ระบบ normalize thinking level ผ่าน `model.NormalizeThinkingLevel(provider, model, requestedLevel)`
8. `bootstrapModelWithStatus(...)` สร้าง provider runtime และ compose model status ในรูป `provider/model(level)`
9. ระบบ persist preference กลับลงไฟล์
10. สร้าง `cognitive.Agent`, `skill.Registry`, `skill.Dispatcher`, `turn.Executor`, และ `app.App`

### 6.2 Interactive Turn Flow

```mermaid
flowchart LR
    Input["User input"] --> Meta{"meta command?"}
    Meta -->|yes| AppMeta["/model /help :clear exit handled in app"]
    Meta -->|no| Turn["turn.Executor"]
    Turn --> Infer["infer obvious tool candidates"]
    Infer --> SkillPath{"explicit or inferred tool path?"}
    SkillPath -->|yes| Safety["safety.AssessCommand"]
    Safety -->|approve| SkillExec["skill dispatcher / ExecuteTool"]
    SkillExec --> Result["tool result / summary / status"]
    SkillPath -->|no| AgentLoop{"tool-calling capable?"}
    AgentLoop -->|yes| Agent["cognitive.Agent RespondWithTools"]
    Agent --> ToolExec["dispatcher.ExecuteTool"]
    ToolExec --> Agent
    AgentLoop -->|no| Reply["conversation response / stream"]
```

พฤติกรรมสำคัญที่พบจริง:

- `internal/app` ยัง intercept `/model`, `/help`, `:clear`, `exit` โดยตรง
- status line ถูกแยกเป็น 2 ชั้น:
  - header: title ซ้าย, model status ขวา
  - prompt line: `>` ซ้าย, context usage ขวา
- `turn.Executor.Execute(...)` ทำมากกว่าการ route ธรรมดา:
  - normalize intent
  - infer tool candidates จากข้อความธรรมชาติบางกรณี (logic อยู่ใน `turn/infer.go`)
  - เลือกว่าจะรัน inferred tool path ก่อน agent หรือไม่
  - ใช้ model tool loop ถ้า agent และ dispatcher รองรับ
  - result shaping และ sanitization (logic อยู่ใน `turn/result.go`)
- explicit skill path, inferred tool path, และ model-selected tool path ล้วนผ่าน safety gate
- shell execution ทุกครั้งบันทึก audit log (`internal/audit`) แบบ non-fatal

### 6.3 Agent Tool Loop

ข้อเท็จจริงที่ยืนยันได้:

- `cognitive.Agent.RespondWithTools(...)` ใช้ `model.ToolDefinition`, `model.ToolCall`, และ `model.RoleTool`
- agent จะ:
  1. ส่งข้อความพร้อม tool definitions
  2. ตรวจ `tool_calls` จาก model response
  3. execute tool locally
  4. append tool output กลับเป็น `tool` message
  5. loop ต่อจน model ส่ง final text หรือชน loop limit
- loop limit default คือ 4 รอบ
- ถ้าไม่มี tools หรือ provider ไม่รองรับ tool calling ระบบจะ fallback ไป `Respond(...)`

ข้อสังเกตเชิงสถาปัตยกรรม:

- ADR 0001 ไม่ได้เป็นแค่ target architecture แล้ว แต่บางส่วนลง current state จริงแล้ว
- tool loop อยู่ใน `internal/cognitive` ส่วน policy/safety/result shaping อยู่ใน `internal/turn` มากกว่าถูกผูกไว้กับ `app`

## 7. State and Persistence

### 7.1 Conversation State

ข้อเท็จจริงที่ยืนยันได้:

- `internal/memory.Context` เก็บข้อความใน RAM
- `cognitive.Agent` เติมทั้ง assistant text, `reasoning_content`, และ tool-call/tool-result messages ลง context
- การ `:clear` จะ reset context
- การ switch model สร้าง agent ใหม่ จึงมีผลเป็นการตัดบริบทเดิมของ session

ผลเชิงสถาปัตยกรรม:

- conversation state ยังเป็น ephemeral state
- ระบบมี session continuity เฉพาะใน process ปัจจุบัน ไม่ข้ามการรัน

### 7.2 Persisted State

ข้อเท็จจริงที่ยืนยันได้:

- `ModelPreference` เก็บ:
  - `provider`
  - `model`
  - `base_url`
  - `think_level`
  - `provider_api_keys`
- API keys ถูกเก็บแบบ map แยกตาม canonical provider key
- ตอน persist ระบบ normalize provider และ think level ก่อนเขียน

ผลเชิงสถาปัตยกรรม:

- persistence layer ยังเล็กและเฉพาะเรื่อง model/session preferences
- preference schema รองรับ multi-provider environment มากขึ้น โดยไม่ต้องมี database

### 7.3 Shell Audit Log

ข้อเท็จจริงที่ยืนยันได้:

- `internal/audit.WriteShell(...)` ถูกเรียกจาก `shellSkill.Execute()` ทุกครั้งที่ shell ทำงานจริง
- audit log เก็บเป็น JSONL append-only ที่ `~/.aetox/shell-audit.log`
- directory `~/.aetox` ถูกสร้างอัตโนมัติถ้ายังไม่มี
- audit write failure ไม่ทำให้ shell execution ล้มเหลว (`_ = audit.WriteShell(...)`)
- แต่ละ entry เก็บ: `time`, `command`, `workdir`, `success`, `duration_ms`, `error` (ถ้ามี)
- `sanitizeCommand()` seam เตรียมไว้สำหรับ sanitize command ในอนาคต (v1 return ค่าเดิม)
- shell ไม่อยู่ใน `ToolDefinitions()` — model ไม่สามารถเลือก shell เองได้; ต้องผ่าน explicit `/shell` path หรือ inferred path ที่มี safety gate เท่านั้น

ผลเชิงสถาปัตยกรรม:

- shell execution มี trace ย้อนหลังได้โดยไม่เพิ่ม coupling กับ safety หรือ turn layer
- audit เป็น package แยก (`internal/audit`) — สามารถ reuse ได้ในอนาคตโดยไม่กระทบ skill/safety/turn
- ยังไม่มี stdout/stderr ใน audit log (v1 intentionally minimal)

## 8. Model Integration Architecture

### 8.1 Two-Layer Provider Architecture

สถาปัตยกรรมปัจจุบันแบ่งชัดเป็น 2 ชั้น:

1. `internal/provider`
   - ไม่มี HTTP
   - ถือ static metadata เท่านั้น
   - รับผิดชอบ alias normalization, env key list, runtime class, fallback model, provider-level capability flags

2. `internal/model`
   - มี HTTP/runtime behavior
   - รับผิดชอบ bootstrap provider, live model discovery, request shaping, response parsing, tool call handling, thinking normalization

ผลเชิงสถาปัตยกรรม:

- provider addition มี locality ดีขึ้น
- static fallback กับ live runtime knowledge ถูกแยกออกจากกันชัดขึ้น

### 8.2 Provider Families ที่พบจริง

- `noop`
- `openrouter`
- OpenAI-compatible family ผ่าน adapter เดียว:
  - `openai`
  - `deepseek`
  - `gemini`
  - `groq`
  - `mistral`
  - `together`
  - `perplexity`
  - `cohere`
  - `lmstudio`
  - `localai`
- `ollama`

### 8.3 Live Model Discovery

ข้อเท็จจริงที่ยืนยันได้:

- Ollama ใช้ `/api/tags`
- OpenAI-compatible family ใช้ `/models`
- Gemini มี path พิเศษ:
  - ใช้ OpenAI-compatible base URL สำหรับ runtime chat completions
  - แต่ derive native Google models endpoint สำหรับ discovery
  - filter เฉพาะ model ที่รองรับ `generateContent`

ผลเชิงสถาปัตยกรรม:

- Gemini ไม่ถูกยัดเข้า generic `/models` path แบบฝืนๆ
- discovery seam รองรับ provider-specific quirks ได้โดยไม่ทำให้ provider catalog ปนกับ HTTP logic

### 8.4 Thinking Capability Architecture

ข้อเท็จจริงที่ยืนยันได้:

- generic think levels ถูก parse ใน `internal/think`; canonical level สำหรับปิด thinking คือ `"off"` (เดิม `"off-think"` — ถูก rename ทั่ว codebase)
- model/provider-specific support ถูก resolve ใน `internal/model/thinking_capabilities.go`
- current config default คือ `low`
- ก่อน runtime ใช้งานจริง ระดับที่ผู้ใช้เลือกจะถูก normalize ผ่าน `model.NormalizeThinkingLevel(provider, model, requestedLevel)`

Observed capability examples:

- DeepSeek:
  - native levels: `off`, `high`, `max`
  - default: `high`
- Gemini:
  - `gemini-2.5*`: `none`, `minimal`, `low`, `medium`, `high`
  - `gemini-2.5-pro`: `minimal`, `low`, `medium`, `high`
  - `gemini-3*`: `minimal`, `low`, `medium`, `high`
  - `gemini-2.0-flash-lite`: ไม่รองรับ thinking
- OpenAI/OpenRouter/Groq:
  - รองรับต่างกันตาม model family และถูก resolve ผ่าน capability resolver

ข้อสังเกตสำคัญ:

- UI และ config ใช้ generic think levels
- provider runtime รับค่า native ที่ถูก normalize แล้ว
- label ที่ผู้ใช้เห็นกับ behavior ที่ provider ได้รับเริ่มผูกกันดีขึ้นกว่าก่อน

### 8.4a Model Capability Catalog

ข้อเท็จจริงที่ยืนยันได้:

- `ModelCapability` struct:
  - `Provider`, `Model`, `Discovered` (true=จาก live API), `Thinking` (ThinkingCapabilities)
- `BuildCapabilityCatalog(providerName, discoveredModels)`:
  - pure function — ไม่ทำ HTTP; รับ `[]string` จาก discovery layer เป็น input
  - `discoveredModels == nil` → ใช้ static `provider.RecommendedModels`, `Discovered: false`
  - `discoveredModels != nil` → ใช้ list ที่ส่งมา, `Discovered: true`
  - `discoveredModels == []` → คืน empty catalog
  - unknown provider → คืน audit entry ต่อ model โดย `Supported: false, Source: "unknown-provider"`
  - deduplicate ด้วย model name (case-insensitive, preserve first occurrence)
- conservative fallback:
  - 4 resolver defaults (OpenAI, Gemini, OpenRouter, Groq) เปลี่ยนจาก `Supported: false, Levels: nil` → `Supported: true, Levels: [low,medium,high,off], Source: "conservative-fallback"`
  - model ใหม่ที่ไม่รู้จักยังใช้ thinking พื้นฐานได้ — ไม่พังเงียบ
  - DeepSeek resolver default คืน `fallbackThinkingCapabilities` อยู่แล้ว → ไม่ต้องแก้
- `gemini-2.0-flash-lite` ยังคง `Supported: false` (documented fact — model นี้ไม่รองรับ thinking จริง)

### 8.5 Provider-Specific Reasoning Payload Shaping

ข้อเท็จจริงที่ยืนยันได้:

- DeepSeek ใช้ `thinking` และ `reasoning_effort`
- OpenAI ใช้ `reasoning_effort`
- Gemini ใช้ `reasoning_effort`
- Groq ใช้ `reasoning_effort` และปิด `include_reasoning`
- OpenRouter ใช้ `reasoning` object

ผลเชิงสถาปัตยกรรม:

- client layer รู้จัก transport shape ของแต่ละ provider
- แต่การตัดสินว่า level ไหนใช้ได้ ย้ายขึ้นไปอยู่ capability layer มากขึ้น

### 8.6 UI Status Contract

ข้อเท็จจริงที่ยืนยันได้:

- model status ถูก compose จาก `formatModelModeLabel(provider, model, thinkLevel)`
- header line แสดง `provider/model(level)` ด้านขวา
- prompt line แสดง `context used/limit tokens` แยกต่างหาก
- thinking indicator เปลี่ยนข้อความตามว่าเป็น conversation หรือ skill และตามว่าอยู่ใน `off` หรือไม่

ผลเชิงสถาปัตยกรรม:

- terminal UX เริ่มสะท้อน runtime state จริง มากกว่าจะเป็น label เชิงตกแต่ง
- model switching, persisted preference, และ header status ใช้ข้อมูล normalized ชุดเดียวกัน

## 9. Safety and Execution Boundaries

ข้อเท็จจริงที่ยืนยันได้:

- `internal/safety` ไม่ได้ใช้เฉพาะ explicit command path แล้ว
- `turn.Executor` เรียก `safety.AssessCommand(...)` ทั้งใน:
  - explicit skill execution
  - inferred tool execution
  - model-selected tool execution
- high-risk command/tool ต้องผ่าน approval prompt

ข้อสังเกตสำคัญ:

- safety boundary ถูกดันเข้าใกล้ execution seam มากขึ้น
- สถาปัตยกรรมปัจจุบันสอดคล้องกับ ADR 0001 มากกว่าเดิมตรงที่ tool calling ไม่ bypass safety

ยังเป็นความจริงอยู่:

- `shell` ยังเป็น boundary ที่เสี่ยงสูง — แม้ตอนนี้มี audit log (`~/.aetox/shell-audit.log`) ให้ trace ย้อนหลังได้แล้ว แต่ยังไม่มี rollback หรือ sandbox
- shell ไม่อยู่ใน model tool surface (`ToolDefinitions()`) — model เลือก shell เองไม่ได้; ต้องผ่าน explicit `/shell` path เท่านั้น
- audit write เป็น non-fatal: ถ้าเขียน log ไม่ได้ shell ยังทำงานต่อ

## 10. Quality Gates

ข้อสังเกตจากโครงสร้างและการเปลี่ยนล่าสุด:

- regression protection ฝั่ง model/provider/thinking ดีขึ้น — logic ถูกแยก seam ชัดขึ้น; เพิ่ม snapshot tests (`TestBuildCapabilityCatalog_KnownPrefixesResolveToSupported`) ป้องกัน drift
- `internal/turn` ถูกแยกเป็น 3 ไฟล์ (executor/infer/result) โดยไม่เปลี่ยน behavior — test coverage ยัง 100% โดยไม่ต้องแก้ test เดิม
- `internal/audit` มี test 7 ตัว (JSONL write, directory creation, failed command, append, auto-time)
- `BuildCapabilityCatalog` มี test 8 ตัว (discovered, fallback, unknown provider, source audit, dedup, static mode)
- `internal/model`, `internal/provider`, `internal/think`, และ `cmd/aetox` เป็นพื้นที่ที่มีสัญญาเชิงพฤติกรรมชัด
- `internal/turn` test suite (28 tests) ยืนยันว่า file split ไม่ได้เปลี่ยน behavior

Reasonable inference:

- การย้าย logic จาก ad hoc UI handling ไปสู่แยก concern + catalog + audit ทำให้ระบบมี seam สำหรับ test และ extension ชัดเจนขึ้นอย่างมีนัยสำคัญ

## 11. Risks and Open Questions

Risks:

1. **provider catalog ยังมี static fallback models/capabilities** — ยังมีโอกาส drift จาก provider reality เมื่อ upstream เปลี่ยน; conservative fallback ช่วยลดผลกระทบแล้ว แต่ยังไม่ใช่ live detection
2. **per-model thinking support** — หลายค่ายยังใช้ family heuristics มากกว่าการ discover จาก live metadata ทุกกรณี; conservative fallback + snapshot test ช่วยลด blast radius แล้ว; `isKnownOpenRouterReasoningModel` ยัง hardcoded
3. **`internal/turn.Executor`** — ถูกแยกเป็น 3 ไฟล์แล้ว (executor/infer/result) ลด monofile 1,789→718 บรรทัด; แต่ `executeSkillTurn` กับ `executeInferredTool` ยังมี approval logic ซ้ำ — ควร extract `ensureApproval()` ในรอบถัดไป
4. **tool surface มีทิศทางขยายเร็วขึ้น** — ต้องคุม allowlist และ safety policy ให้ชัด; shell ถูกกั้นจาก model tool surface แล้ว
5. **UI status label drift** — provider เปลี่ยน native semantics โดยไม่อัปเดต resolver จะทำให้ label ถูกแต่ behavior ผิด; conservative fallback + snapshot test ลดความเสี่ยง
6. **shell audit log (mitigated v1)** ✅ — มี audit trail (`~/.aetox/shell-audit.log` แบบ JSONL), shell ไม่อยู่ใน tool surface; ยังไม่มี rollback/sandbox/command allowlist
7. **`infer.go` ยัง 883 บรรทัด** — regex-based NLP รวมกันในไฟล์เดียว; ควรแยก parse per-tool (write parser, list parser) ในรอบถัดไป
8. **ไม่มี caller ใช้ `BuildCapabilityCatalog` จริง** — catalog ถูกสร้างแล้วแต่ยังไม่มีใครเรียกใช้; ต้อง integrate เข้า startup/model-selection flow

Open questions:

1. จะย้ายจาก family-based thinking heuristics ไปสู่ live capability discovery สำหรับค่ายหลักอื่นนอกจาก Gemini เมื่อไร
2. provider catalog ควรถือ model-family metadata เพิ่มอีกหรือควรคงให้ `internal/model` เป็น runtime intelligence layer ต่อไป
3. tool allowlist สำหรับ model-selected execution ควรผูกกับ provider/model capability matrix ระดับไหน
4. จะ persist conversation/session state ข้ามการรันหรือคง intentionally ephemeral ต่อไป
5. `internal/turn` ควรถูกแยกเป็น submodules เพิ่มหรือยัง หรือยังอยู่ในจุดที่ refactor เฉพาะ contract ก็พอ

## 12. Validation Gate

1. Claim traceability: ผ่าน  
   ทุก claim สำคัญอ้างอิงจากไฟล์ที่ตรวจจริง หรือถูกระบุเป็น reasonable inference/open question

2. Scope alignment: ผ่าน  
   เอกสารยังเป็น current-state review ของทั้งระบบ แต่เพิ่มน้ำหนักในส่วน architecture ที่เปลี่ยนจริงล่าสุด: provider catalog, thinking capability, tool loop, turn orchestration, และ UI model state

3. Handoff readiness: ผ่าน  
   เอกสารมี system boundary, module map, runtime flow, state/persistence, model integration shape, risks, และ open questions เพียงพอสำหรับงานต่อเนื่อง

## 13. Recommended Next Use

เอกสารนี้เหมาะสำหรับใช้:

- onboarding ผู้พัฒนาใหม่ให้เข้าใจว่า Aetox ตอนนี้ไม่ได้เป็นแค่ CLI chat + skill router แบบเดิมแล้ว
- ใช้คุย refactor รอบถัดไปของ `internal/turn`:
  - extract `ensureApproval()` แก้ approval logic ซ้ำใน `executeSkillTurn`/`executeInferredTool`
  - แยก `infer.go` เป็น per-tool parser (write parser, list parser)
- integrate `BuildCapabilityCatalog` เข้า startup/model-selection flow
- ใช้เป็นฐานก่อนแตก ADR เพิ่มเรื่อง:
  - live capability discovery แทน family heuristics
  - tool allowlist per provider/model capability matrix
  - session persistence (Risk 4)
- ใช้แยก current state ออกจาก target architecture ใน [architecture-aetox.md](E:\Aetox\Aetox-cli\docs\architecture-aetox.md)
