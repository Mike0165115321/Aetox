# MCP Client — อ่านจาก opencode ของจริง

> อ่านจาก `packages/opencode/src/mcp/{index.ts,catalog.ts,oauth-provider.ts,auth.ts}`, `packages/core/src/v1/config/mcp.ts`, `packages/opencode/src/session/tools.ts`, `packages/opencode/src/tool/code-mode.ts` @ commit `76ced5418f50ee5cfb4c256af358bc0ab063a51b`
> **สิ่งที่ยืนยันแล้ว:** opencode **ไม่ได้เขียน stdio JSON-RPC client เอง** — ใช้ `@modelcontextprotocol/sdk` (npm, v1.29.0) ทำ transport/OAuth ทั้งหมด โค้ดของ opencode เองมีแค่: config, connection lifecycle, tool-bridging adapter, token storage
> **ผลต่อแผนเดิม:** `MCP-SUPPORT-PLAN.md` ตอนเขียนแรกสุดสมมติว่าเราต้องเขียน stdio/SSE JSON-RPC client เองทั้งหมด — ก่อนเริ่ม ควรเช็คก่อนว่ามี Go MCP SDK ที่เป็นทางการ/เสถียรพอไหม (เช่น `github.com/modelcontextprotocol/go-sdk`) ถ้ามี ใช้แทนการเขียน transport layer เอง (ladder ข้อ 5: dependency ที่มีอยู่แล้วดีกว่าประดิษฐ์ใหม่) — เหลือแค่ tool-bridging adapter + config + lifecycle ที่ต้องเขียนเองจริงๆ ไม่ว่าจะใช้ SDK ไหนก็ตาม

## 1. Config schema จริง

**ระวัง:** `packages/core/src/config/mcp.ts` (~48 บรรทัด) ที่เราเคยอ้างถึงตอนวางแผนแรก เป็นแค่ draft "v2" ที่**ยังไม่ได้ใช้งานจริง** — shape จริงที่ runtime อ่านคือ `packages/core/src/v1/config/mcp.ts:6-63`

Local (stdio):
```json
{
  "mcp": {
    "myserver": {
      "type": "local",
      "command": ["node", "server.js"],
      "cwd": "./subdir",
      "environment": { "KEY": "value" },
      "enabled": true,
      "timeout": 30000
    }
  }
}
```
- `command` เป็น `string[]` — ตัวแรกคือ argv0 ที่เหลือคือ args
- `cwd` resolve เทียบกับ workspace directory ไม่ใช่ process cwd
- `environment` merge ทับ `process.env` — มี hack เฉพาะ: ถ้า `cmd === "opencode"` (spawn opencode เองเป็น MCP server) จะ inject `BUN_BE_BUN: "1"` เพิ่ม — เป็น workaround เฉพาะ runtime ของเขา ไม่เกี่ยวกับเรา
- `timeout`: doc comment บอก default 5000ms แต่ **โค้ดจริงใช้ 30000ms** — บทเรียน: อย่าเชื่อ doc comment มากกว่าพฤติกรรมจริงตอน port ค่า default

Remote (HTTP/SSE, มี OAuth ได้):
```json
{
  "mcp": {
    "myremote": {
      "type": "remote",
      "url": "https://example.com/mcp",
      "headers": { "Authorization": "Bearer ..." },
      "enabled": true,
      "timeout": 30000,
      "oauth": {
        "clientId": "...", "clientSecret": "...", "scope": "...",
        "callbackPort": 19876,
        "redirectUri": "http://127.0.0.1:19876/mcp/oauth/callback"
      }
    }
  }
}
```
`oauth` ใส่ได้ 3 แบบ: ไม่ใส่ (auto-discovery), object (ระบุ client creds เอง), หรือ `false` (ปิด OAuth ไปเลย)

## 2. Lifecycle — lazy ต่อ workspace directory ไม่ใช่ต่อ session

**ไม่มี "connect ทุก MCP server ตอน boot"** — connection ถูก cache ผ่าน `InstanceState.make(...)` (`ScopedCache` keyed ด้วย workspace directory, `packages/opencode/src/effect/instance-state.ts:26-50`) จุดที่ trigger การ connect ครั้งแรกคือครั้งแรกที่มีอะไรเรียก `MCP.Service` (ปกติคือตอนสร้าง tool list ของ turn แรก)

พอ trigger แล้ว server ที่ `enabled !== false` ทุกตัวถูก connect **พร้อมกัน** (`Effect.forEach(..., {concurrency:"unbounded"})`, `index.ts:505-529`) แล้ว cache client ไว้ใช้ร่วมกันทุก session ในโปรเจกต์เดียวกัน จนกว่า instance จะถูก dispose

**Teardown ที่น่าสนใจ:** ตอน dispose, ถ้า transport เป็น `StdioClientTransport` จะไล่ process tree ด้วย `pgrep -P` แล้ว SIGTERM ลูกหลานทั้งหมดก่อนปิด client (`index.ts:540-550`) — เป็น POSIX-only, **no-op บน win32** (`index.ts:420`) เพราะ MCP server ที่ spawn ผ่าน `npx`/`uvx`/`docker` มักแตก process ลูกที่ไม่ตายตาม SIGTERM ของ process ตรงที่ spawn เอง **นี่คือช่องโหว่ที่เราต้องแก้เองใน Go** เพราะ Aetox เป็น cross-platform (ต้องใช้ process group บน Windows, ไม่ใช่แค่ kill PID เดียว)

รองรับ add/remove/reconnect แบบไม่ต้อง restart (`MCP.Service.add/connect/disconnect`) และมี `ToolListChangedNotificationSchema` handler ที่ re-list tool สดเมื่อ server ส่ง notification มา (ไม่ได้ fetch tool list แค่ครั้งเดียวตอน connect)

## 3. Tool bridging — มี adapter จริง ไม่ใช่ pass-through เฉยๆ

Pipeline: `MCP SDK Tool type` → `MCP.McpTool` (cache entry `{def, client, timeout}`) → **`McpCatalog.convertTool`** (`packages/opencode/src/mcp/catalog.ts:42-83`, จุดแปลงจริง) → `ai` SDK `dynamicTool(...)` → `session/tools.ts:390-451` (wrap เพิ่ม plugin hooks + permission gate + OTel span) → ลงทะเบียนใน **tool map เดียวกัน**กับ built-in tools (`tools.ts:50,99-100`) — ไม่มีการแยก type ที่ layer ล่างสุดเลย

จุดสำคัญของ `convertTool`:
- normalize `inputSchema` เป็น strict JSON Schema (`additionalProperties:false`)
- `result.isError` → throw (join text blocks) ให้ error โผล่เป็น tool error ปกติ
- ไม่มี content ธรรมดาแต่มี `structuredContent` → fallback stringify เป็น text block

**Naming scheme:** `sanitize(serverName) + "_" + sanitize(toolName)` (`catalog.ts:117-119`, ตัดอักขระที่ไม่ใช่ `[a-zA-Z0-9_-]` ทิ้ง) — กันชื่อชนกันข้าม server แบบ deterministic ไม่ error

**MCP resources** (คนละอย่างกับ tools) มี synthetic tool 3 ตัวห่อให้: `list_mcp_resources`, `list_mcp_resource_templates`, `read_mcp_resource` — inject เฉพาะตอนมี client ที่ support `resources` capability

**Code mode** (experimental flag): มี path คู่ขนานที่ **จงใจ duplicate** logic ของ `convertTool` แทนที่จะ reuse (comment ยืนยันว่าจงใจ ไม่ใช่ลืม) — ไม่เกี่ยวกับเราตอนนี้ ข้ามได้

## 4. OAuth — SDK ขับเคลื่อน เราแค่เก็บ token

opencode implement `OAuthClientProvider` interface ของ SDK เอง (`McpOAuthProvider`, `oauth-provider.ts:26`) — **ตัว SDK เป็นคนขับ flow ทั้งหมด** (OAuth 2.1 + PKCE + dynamic client registration ตาม RFC 7591), opencode แค่เสียบ storage/callback

Pattern ที่น่าเอาไปใช้: `McpOAuthPendingProvider` เก็บ token **ใน memory เท่านั้น**ระหว่าง flow ยังไม่เสร็จ มี `commit()` ให้ flush ลง disk เมื่อสำเร็จจริง — กัน partial/interrupted OAuth ทำให้ auth file เสียหาย

Storage: JSON file `mcp-auth.json` (mode `0o600`, มี file lock กันเขียนพร้อมกัน), ทุก entry เก็บ `serverUrl` ไว้ด้วยและเช็คว่าตรงกับ config ปัจจุบันก่อนใช้ token — กัน token ของ server เก่ารั่วไปใช้กับ URL ใหม่ถ้า user แก้ config

**Refresh:** ไม่มี logic เขียนเองเลย — SDK จัดการ refresh cycle ผ่าน `tokens()`/`saveTokens()` ที่ opencode implement ไว้ ตอนเจอ 401 (`UnauthorizedError`) เท่านั้น

## 5. Permission integration — ใช้ gate เดียวกับ built-in แต่ **granularity หยาบกว่า**

**นี่คือ finding ที่สำคัญที่สุดสำหรับเรา:** MCP tool ไหลผ่าน permission gate เดียวกับ built-in tool เป๊ะ — ไม่มีการเข้มงวดพิเศษ ไม่มี sandbox แยก ไม่มี trust tier ต่างหาก **แต่** built-in tool อย่าง `shell`/`edit` ส่ง pattern ที่เจาะจง (เช่น shell command จริง, file path จริง) เข้า permission matcher ในขณะที่ MCP tool ทุกตัวส่ง `patterns: ["*"]` เสมอ (`session/tools.ts:408`) — แปลว่า user **อนุญาต/ปฏิเสธ MCP tool ได้แค่ทั้งตัว ไม่สามารถ scope ตาม argument ได้แบบที่ทำกับ `bash`/`edit` ได้**

Default เมื่อไม่มี rule ตรง = `"ask"` เหมือนกันทั้ง MCP และ built-in — ไม่มี hardcode "MCP ต้อง ask เสมอ" เป็นพิเศษ

**การตัดสินใจสำหรับ Go client ของเรา:** ถ้าอยากได้ granularity ระดับ argument สำหรับ MCP tool (เหมือนที่เรามีให้ `shell`/`git`/`fs` อยู่แล้วใน `internal/safety.AssessCommand`) ต้องออกแบบเอง — opencode เองก็ไม่ได้ทำ (ไม่ใช่ oversight เป็นทางเลือกที่เขาเลือกไม่ทำ) เราได้เปรียบตรงนี้อยู่แล้วเพราะ `safety.PermissionConfig` ของเรารองรับ pattern ต่อ args ได้ทันที — แค่ตอน register MCP adapter ต้องคิดว่าจะ join tool call args เป็น string ยังไงให้ pattern match ได้จริง (ดู `permissions.md` เรื่อง bash ที่ opencode ใช้ raw string ไม่ parse)

## 6. Error handling — ไม่ throw exception ข้ามชั้น

- Connect fail → เก็บเป็น `Status` value (`failed`/`needs_auth`/`needs_client_registration`) ไม่ throw — server ที่ fail แค่หายไปจาก tool list เฉยๆ ไม่ทำให้ agent loop พัง
- Remote: ลอง **StreamableHTTP ก่อน SSE** เสมอ (fallback เฉพาะตอนไม่ใช่ auth error) — เพราะ MCP server จริงในโลกจำนวนมากรองรับแค่ transport เดียว
- Connect timeout 30s default, มี global override (`experimental.mcp_timeout`)
- `Effect.acquireUseRelease` รับประกันปิด transport แม้ connect fail — กัน socket/process leak
- Runtime disconnect → mark `failed`, **ไม่ auto-reconnect** ต้อง connect ใหม่เอง
- Tool call ส่ง `onprogress` callback (แม้จะเป็น no-op) เพื่อให้ SDK reset timeout ตอนมี progress notification เข้ามา — **ถ้าไม่ใส่ callback นี้ timeout จะไม่ reset** สำหรับ tool call ที่ใช้เวลานาน (จุดพลาดง่ายตอน implement ใหม่)
- `outputSchema` ที่ server ส่งมาพังบ่อย (real-world MCP server จำนวนมากไม่ conform) → มี fallback retry แบบตัด `outputSchema` ทิ้งถ้า validate fail แทนที่จะ fail ทั้ง server
- Pagination cursor มี cap 1000 หน้า + ตรวจ cursor ซ้ำ (กัน server ส่ง cursor วนไม่รู้จบ)

## สรุปสิ่งที่ต้องตัดสินใจตอนออกแบบ MCP client เวอร์ชัน Go

| ประเด็น | ทางเลือก | ข้อเสนอ |
|---|---|---|
| Transport library | เขียน stdio JSON-RPC เอง vs. หา Go SDK ที่เป็นทางการ | เช็ค `github.com/modelcontextprotocol/go-sdk` ก่อน — ถ้าเสถียรพอ ใช้เลย ประหยัดงานเขียน+เทส transport/OAuth เยอะมาก |
| Process cleanup (stdio) | kill PID เดียว vs. process tree | ต้องทำ process-group ให้ครบทั้ง Windows/Unix เพราะ Aetox cross-platform — opencode เองก็ทำไม่ครบ (ข้าม win32) |
| Permission granularity สำหรับ MCP tool | wildcard เดียวเหมือน opencode vs. ให้ pattern ต่อ args ได้ | เราได้เปรียบเพราะ `safety.PermissionConfig` รองรับอยู่แล้ว — ใช้ให้เป็นประโยชน์ ไม่ต้อง limit ตัวเองแบบ opencode |
| Connection scope | ต่อ process (เหมือน CLI เดี่ยว) vs. ต่อ workspace/project (เหมือน opencode) | Aetox มีทั้ง CLI process แยกกับ Desktop process ต่อ instance อยู่แล้ว (ไม่มี shared gateway ตาม ARCHITECTURE.md §10) — ต่อ process ง่ายกว่าและตรงกับสถาปัตยกรรมปัจจุบัน ไม่ต้อง cache แบบ opencode |
| Error surfacing | throw ข้าม layer vs. Status value | เอาแบบ opencode: connect fail = server หายจาก tool list เงียบๆ + log ไม่ throw ทำให้ agent loop พัง |
