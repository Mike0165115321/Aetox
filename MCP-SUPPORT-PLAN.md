# MCP + External Skills — Readiness Notes

บันทึกจากการสำรวจโค้ดจริง (2026-07-21) ก่อนเริ่ม implement MCP support ภายนอก
**แก้แผนหลัก 2026-07-22** หลังศึกษาซอร์สจริงของ opencode +verify ว่า Go มี
official MCP SDK — ดูหัวข้อ "แผนที่แนะนำ — MCP" ที่เปลี่ยนจากเขียน JSON-RPC
transport เองมาเป็นใช้ `github.com/modelcontextprotocol/go-sdk`

## สถานะปัจจุบัน

**พร้อม:** [internal/skill/skill.go](internal/skill/skill.go) มี `Tool` interface
(`ToolDefinition()` คืน JSON schema + `ExecuteTool(ctx, args)`) ที่รูปร่างตรงกับ
MCP tool อยู่แล้ว. [internal/skill/dispatcher.go](internal/skill/dispatcher.go)
ดึง `ToolDefinition()` จากทุก skill ใน registry ไปให้ model เรียกผ่าน tool-calling
loop โดยไม่สนใจว่า skill นั้น implement เองหรือห่อ remote tool มา — **ไม่ต้องแก้
dispatcher/registry/tool-loop เลย** ถ้าจะเพิ่ม MCP.

**ยังไม่มีเลย:** ไม่มี MCP client ในโปรเจกต์ — แต่ (อัปเดต 2026-07-22 หลังศึกษา
opencode + verify เอง) **ไม่ต้องเขียน stdio/SSE JSON-RPC transport เอง** ดู
"แผนที่แนะนำ — MCP" ด้านล่าง ที่แก้ตามนี้แล้ว

**Half-finished (พบระหว่างสำรวจ):** `plugin_install` skill
([internal/skill/github_tools.go:224](internal/skill/github_tools.go#L224))
ดาวน์โหลด `aetox-plugin.json` manifest + ไฟล์จาก GitHub repo แล้วเขียนลง
`~/.agents/skills/<name>/` ได้จริง — แต่ไม่มีจุดไหนใน codebase โหลดไฟล์พวกนั้น
กลับเข้า registry ตอน bootstrap เลย ([internal/skill/defaults.go](internal/skill/defaults.go)
ลงทะเบียนแต่ skill ที่ compile เข้ามาตรง ๆ เท่านั้น). เท่ากับตอนนี้
"ติดตั้ง skill ภายนอก" ผ่าน tool นี้ไม่มีผลอะไรหลังดาวน์โหลดเสร็จ.

## แผนที่แนะนำ — MCP (แก้ 2026-07-22 หลังศึกษา opencode + verify SDK จริง)

**เปลี่ยนจากแผนเดิม (ด้านล่างนี้คือของใหม่ ของเดิมที่บอก "เขียน stdio JSON-RPC
client เอง" ผิด):** อ่าน [docs/opencode-study/mcp.md](docs/opencode-study/mcp.md)
พบว่า opencode เองก็ไม่เขียน JSON-RPC/OAuth transport เอง — ใช้ npm package
ทางการ `@modelcontextprotocol/sdk` แล้วเขียนแค่ config/lifecycle/tool-adapter
รอบตัวมัน เช็คแล้วพบว่า Go มี**ของเทียบเท่าที่เป็นทางการจริง**:
`github.com/modelcontextprotocol/go-sdk` (maintained ร่วมกับ Google, v1.6.1,
4.8k stars ณ วันที่เช็ค — verified ผ่าน WebFetch ไปที่ repo/pkg.go.dev จริง
ไม่ใช่เดา) รองรับครบทั้ง:
- `mcp.CommandTransport` — stdio ไป subprocess (แทน local server spawn)
- `mcp.SSEClientTransport` / `mcp.StreamableClientTransport` — remote server
- `mcp.Client.Connect(ctx, transport, nil)` → `*mcp.ClientSession`
- `session.Tools(ctx, nil)` (iterator) / `session.ListTools(ctx, params)`
- `session.CallTool(ctx, &mcp.CallToolParams{Name, Arguments})`
- OAuth (`go-sdk/auth`, `go-sdk/oauthex`) — experimental ตั้งแต่ v1.4.0,
  ยังไม่ต้อง depend ตอน phase แรก (local/stdio server ก่อน, ดูลำดับด้านล่าง)

**Ladder ที่ใช้ตัดสินใจ:** ข้อ 5 ("already-installed/available dependency
solves it") — SDK ทางการมี, เสถียรพอ (28 release), มาจากทีมเดียวกับ spec เอง
→ ใช้แทนการเขียน JSON-RPC/OAuth เองทั้งหมด เหลืองานจริงแค่ config +
process/connection lifecycle + tool-bridging adapter (ส่วนที่ opencode เองก็
ต้องเขียนเองเหมือนกัน ไม่มี SDK ไหนทำให้)

### ขั้นตอน

1. `go get github.com/modelcontextprotocol/go-sdk` (root module — ตรวจ
   `go.work`/`go.mod` ทุกโมดูลที่ต้องใช้ ตาม pattern ที่มีอยู่แล้วสำหรับ
   dependency ข้าม `desktop`/root)
2. สร้าง `internal/mcp` package:
   - `Server` config struct (ดู config schema ด้านล่าง) + `Client` wrapper
     ที่เก็บ `*mcp.Client`/`*mcp.ClientSession` ต่อ server หนึ่งตัว
   - **Connection scope: ต่อ process ไม่ใช่ต่อ workspace-directory cache แบบ
     opencode** — Aetox ไม่มี shared gateway/instance ข้าม CLI กับ Desktop
     (ARCHITECTURE.md §10 ตัดสินใจไว้แล้วว่าไม่ทำ gateway ตอนนี้) ต่อ process
     เดียวง่ายกว่าและตรงสถาปัตยกรรมปัจจุบัน ไม่ต้องทำ `ScopedCache`
   - Connect **lazy** (ครั้งแรกที่ `dispatcher.ToolDefinitions()`/`ExecuteTool`
     ถูกเรียกและยังไม่ connect) ไม่ใช่ connect ทุก server ตอน boot — ตาม
     opencode's lesson (server ที่ config ไว้แต่ไม่ได้ใช้ไม่ควรหน่วง startup)
   - Error handling แบบ Status-based (`{status: connected|failed|needs_auth}`)
     ไม่ throw/panic ข้าม layer — server connect ไม่สำเร็จ = หายจาก tool list
     เงียบๆ + log, agent loop ไม่พัง (เหมือน opencode's `index.ts:236-370`)
   - **Cross-platform process cleanup สำหรับ local/stdio server** — จุดที่
     opencode เองยังทำไม่ครบ (`pgrep -P` เป็น POSIX-only, no-op บน win32)
     Aetox ต้องทำให้ครบทั้งสอง OS จริงๆ (process group บน Windows ผ่าน
     `CREATE_NEW_PROCESS_GROUP`/job object, process group บน Unix ผ่าน
     `Setpgid`) เพราะ target หลักคือ Windows (ดู go.mod: มี `golang.org/x/sys`,
     `wailsapp/go-webview2` อยู่แล้ว — เข้ากับ pattern ที่มีอยู่)
3. เขียน adapter struct ที่ implement `skill.Tool` ต่อ MCP tool หนึ่งตัว —
   pattern เดียวกับ opencode's `McpCatalog.convertTool`
   ([docs/opencode-study/mcp.md](docs/opencode-study/mcp.md) §3): ดึง schema
   จาก `session.Tools()`, `ExecuteTool` เรียก `session.CallTool()`, ตั้งชื่อ
   `sanitize(serverName)+"_"+sanitize(toolName)` กันชื่อชนข้าม server
4. Config: MCP server list ใน `internal/config` — shape อิง opencode's schema
   จริง (ไม่ใช่ draft ที่เราเคยดูตอนแรก, ดู mcp.md §1):
   ```go
   type MCPServer struct {
       Type        string            // "local" | "remote"
       Command     []string          // local: argv0 + args
       Cwd         string            // local: relative ต่อ sandbox root
       Environment map[string]string // local: merge ทับ os.Environ()
       URL         string            // remote
       Headers     map[string]string // remote
       Enabled     bool              // default true
       TimeoutMs   int               // default 30000 (ไม่ใช่ 5000 — opencode's
                                      // เองก็มี doc comment ผิดจุดนี้ อย่า copy)
   }
   ```
   Phase แรก: รองรับแค่ `type: "local"` (stdio) ก่อน — ครอบคลุม MCP server
   ส่วนใหญ่ที่พบจริง (npx/uvx-based) และไม่ต้องแตะ OAuth เลย `type: "remote"`
   + OAuth ค่อยทำ phase 2 เมื่อมีความต้องการจริง (ladder ข้อ 1: อย่าสร้าง
   OAuth flow ที่ยังไม่มีใครขอใช้)
5. Register adapter เข้า `Registry` ตอน `bootstrapFromConfig`
   ([desktop/app.go](desktop/app.go)) เหมือนที่ `workbenchTools` (browser_open/
   browser_read) ทำอยู่ตอนนี้ — pattern เดียวกันเป๊ะ ใช้ซ้ำได้เลย **ต่างจาก
   `workbenchTools` ตรงที่ควร register เป็น `SourceExternal` และตั้ง
   permission rule เริ่มต้นแบบระมัดระวัง** (ดูหัวข้อ "Safety gate สำหรับ MCP
   tool" ด้านล่าง — เรื่องนี้พร้อมทำได้แล้วเพราะ `safety.PermissionConfig`
   เสร็จไปแล้ว)

### จุดที่ Aetox ทำได้ดีกว่า opencode ตั้งแต่ต้น (ไม่ต้อง scope ลงมาให้เท่า)

- **Permission granularity ต่อ argument**: opencode ให้ MCP tool ทุกตัว
  match ด้วย pattern `"*"` เท่านั้น (อนุญาต/ปฏิเสธได้แค่ทั้งตัว, mcp.md §5)
  — `safety.PermissionConfig.Resolve` ของเรารองรับ `Pattern` ต่อ args ได้อยู่
  แล้ว ไม่ต้อง limit ตัวเองแบบนั้น แค่ตอน adapter ส่ง args เข้า
  `resolveApproval` ให้ join เป็น string ที่ match pattern ได้จริง (เหมือนที่
  `toolCallToArgs` ทำกับ built-in tools อยู่แล้วใน `internal/turn/executor.go`)

## ช่องว่างสถาปัตยกรรม: ไม่มีเส้นแบ่ง core vs user-added

`RegisterDefaults()` ([internal/skill/defaults.go](internal/skill/defaults.go))
ยัด built-in ทั้งหมด (`read`, `write`, `shell`, `git`, ...) กับของที่ควรเป็น
"เพิ่มทีหลังได้" (`plugin_install`, MCP adapter ในอนาคต) ลง `Registry` เดียวกัน
แบบแบนราบ — ไม่มี field หรือ namespace บอกว่า skill ไหนมาจากไหน. ผลคือ:

- Gate สิทธิ์ต่างกันไม่ได้ (built-in เชื่อใจได้ vs MCP/plugin จาก third-party ควรเข้มกว่า)
- แสดงใน UI แยกกลุ่มไม่ได้ (เช่น Settings.svelte ที่จะโชว์ "core tools" vs "installed skills")
- ชื่อชนกันได้เงียบ ๆ — `Registry.Register()` เขียนทับ key เดิมโดยไม่เตือน ถ้า user
  ติดตั้ง skill ชื่อซ้ำกับ built-in

ก่อนต่อ MCP/plugin_install ให้ใช้งานจริง ควรแยก `Registry` เป็นสอง scope (core /
user-added) หรืออย่างน้อยเพิ่ม field `Source string` ("builtin" | "mcp" |
"plugin") ใน `Skill`/`Tool` metadata ก่อน.

## เทียบกับ opencode — ยังไม่ถึงจริง (2026-07-21)

เอกสารเดิม [docs/architecture-reference-opencode.md](docs/architecture-reference-opencode.md)
(ตอนนั้น Aetox v0.3.0-dev, ก่อนมี desktop/session/browser) เทียบไว้ว่า Aetox ขาด
หลัก ๆ 4 อย่างจาก opencode: **MCP, skill auto-discovery, permission per-tool
(pattern-based), plugin hook system**. เช็คซ้ำวันนี้:

| ช่องว่างเดิม | สถานะตอนนี้ |
| --- | --- |
| Session persistence | ✅ ปิดแล้ว — SQLite + FTS5 ([desktop/db.go](desktop/db.go), [desktop/sessions.go](desktop/sessions.go)) ตามทัน opencode's DB layer |
| Desktop UI | ✅ ปิดแล้ว — Wails + Svelte workbench |
| MCP | ❌ ยังไม่มี — แผนแก้แล้ว 2026-07-22 (ดู "แผนที่แนะนำ — MCP" ด้านบน: ใช้ `github.com/modelcontextprotocol/go-sdk` แทนเขียน transport เอง) |
| Skill auto-discovery (`~/.agents/skills/`, `~/.claude/skills/`) | ✅ ปิดแล้ว 2026-07-22 — [internal/skill/discovery.go](internal/skill/discovery.go): `DiscoverSkills` scan ทั้งสอง path หา `<dir>/*/SKILL.md`, parse frontmatter (`name`/`description`) + body, ห่อเป็น `markdownSkill` (`skill.Tool`, ตอนถูกเรียกคืน body ให้โมเดลทำตามเอง — รูปแบบเดียวกับ opencode/Claude Code) `RegisterDiscovered` ลงทะเบียนเป็น `SourceExternal`, ชนชื่อ built-in แล้ว skip ไม่ fatal (ทดสอบไว้) เรียกจากทั้ง `cmd/aetox/main.go` และ `desktop/app.go`'s `bootstrapFromConfig` ผ่าน `skill.DefaultDiscoveryPaths()` **หมายเหตุ:** `plugin_install` (ด้านล่าง) ยัง half-finished เหมือนเดิม — ไฟล์ที่มันดาวน์โหลดมาจาก `aetox-plugin.json` manifest จะถูก auto-discover กลับเข้า registry ได้ก็ต่อเมื่อ bundle นั้นมีไฟล์ `SKILL.md` อยู่ในนั้นจริง ๆ (ไม่การันตี, แล้วแต่ manifest ของแต่ละ plugin) |
| Permission per-tool (pattern เช่น `"rm *": "deny"`) | ✅ ปิดแล้ว 2026-07-22 — `safety.PermissionConfig`/`PermissionRule` ([internal/safety/safety.go](internal/safety/safety.go)): rule ระบุ `Tool`+`Pattern` แบบ glob (`*`/`?`) + `Action` (`allow`/`ask`/`deny`), **last-match-wins** เหมือน opencode `Resolve()` ถูกเช็คก่อน `ApprovalMode` เดิมเสมอใน `turn.Executor.resolveApproval` (ทั้ง 3 จุดที่เคยเรียก `safety.ShouldPrompt` ตรง ๆ) — เมื่อ rule match `deny`/`allow` จะข้าม prompt ไปเลย, `ask` บังคับ prompt แม้ approval mode จะเป็น full-access ก็ตาม โหลด/เซฟจาก `~/.config/aetox/permissions.json` (`config.LoadPermissions`/`SavePermissions`, pattern เดียวกับ `model-preference.json`) ยัง**ไม่มี UI** ให้ผู้ใช้แก้ rule ผ่าน Settings — ต้องแก้ json เอง (ยังไม่ scope ของรอบนี้) |
| Plugin hook system (`tool.execute.before/after`, `chat.message`, ...) | ❌ ยังไม่มี |

**สรุป:** ปิดไปแล้ว 2 ใน 4 ช่องว่างเดิม (auto-discovery, permission pattern) —
เหลือ MCP client (มีแผนใหม่แล้วด้านบน ใช้ `go-sdk` แทนเขียน transport เอง)
กับ plugin hook system `skill.Tool` shape ยังใกล้เคียง MCP tool เหมือนเดิม
ไม่มีอะไรเปลี่ยนในส่วนนั้น

**Safety gate สำหรับ MCP tool:** ตั้ง permission rule เริ่มต้นแบบระมัดระวังตอน
register adapter เช่น `{Tool: "<serverName>_*", Pattern: "*", Action: "ask"}`
ให้ MCP tool ทุกตัวต้องผ่านการอนุมัติเสมอโดย default (ไม่ auto-run แม้ใน
full-access) — ใช้ `safety.PermissionConfig` ที่มีอยู่แล้วได้ทันที ไม่ต้องรอ
`SourceMCP` ใหม่ก่อน (ตั้ง rule ด้วย tool name prefix ตรง ๆ ก็พอ) ปิด gap ที่
`docs/architecture/model-control-layer-2026-07-22.md` §4 ระบุไว้ได้เลยตอน
implement — ดูรายละเอียดเพิ่มที่ [docs/opencode-study/mcp.md](docs/opencode-study/mcp.md)
§5 กับ [docs/opencode-study/permissions.md](docs/opencode-study/permissions.md)

## ช่องว่างที่ต้องปิดก่อน production-ready

- **Safety tier**: 3 ระดับปัจจุบัน (ask / unsafe-only / full-access) ออกแบบมา
  สำหรับ 17 built-in tools ที่เขียนเอง เชื่อใจได้ — MCP server จาก third-party
  รันโค้ดที่เราไม่ควบคุม ต้องคิดว่าจะ gate ยังไง **อัปเดต 2026-07-22: มีเครื่องมือ
  พร้อมแก้แล้ว** (`safety.PermissionConfig`, ดูหัวข้อด้านบน) เหลือแค่ต้อง
  register rule จริงตอน implement MCP adapter ไม่ใช่ gap ที่ยังไม่มีทางแก้
- **plugin_install loader**: ถ้าจะให้ทางนี้ใช้งานได้จริงด้วย ต้องเขียน loader
  ที่ scan `~/.agents/skills/` ตอน bootstrap แล้ว register กลับเข้า registry —
  แต่ skill ที่ดาวน์โหลดมาเป็น "ไฟล์" (ไม่ใช่ compiled Go) จะ execute ยังไงต้อง
  ตัดสินใจก่อน (interpreter เช่น script, หรือจำกัดเฉพาะ prompt/markdown skill
  ที่ไม่ต้องรันโค้ด).

## ไฟล์ที่เกี่ยวข้อง

- [internal/skill/skill.go](internal/skill/skill.go) — `Skill`/`Tool` interface, `Registry`
- [internal/skill/dispatcher.go](internal/skill/dispatcher.go) — tool-loop wiring
- [internal/skill/defaults.go](internal/skill/defaults.go) — built-in registration
- [internal/skill/github_tools.go](internal/skill/github_tools.go) — `plugin_install` (half-finished)
- [desktop/app.go](desktop/app.go) `bootstrapFromConfig` — ตัวอย่าง pattern ต่อ extra skills (`workbenchTools`)
- [desktop/workbench.go](desktop/workbench.go) — ตัวอย่าง skill ที่เรียก external process/UI จริง (ใกล้เคียงที่สุดกับ MCP adapter)
- [internal/safety/safety.go](internal/safety/safety.go) — `PermissionConfig`/`PermissionRule`, ใช้ gate MCP tool ได้ทันที
- [docs/opencode-study/mcp.md](docs/opencode-study/mcp.md) — งานวิจัยเต็มที่แผนด้านบนอิงมา (config shape, lifecycle, tool bridging, OAuth, error handling — อ่านจากซอร์ส opencode จริง)
