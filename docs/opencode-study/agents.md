# Multi-Agent / Sub-Agent System — ทำไม opencode ไม่ต้องมี "โหมด" แยกโค้ด

> อ่านจาก `packages/opencode/src/agent/{agent,subagent-permissions}.ts`, `packages/opencode/src/tool/task.ts`, `packages/core/src/v1/config/agent.ts`, `packages/core/src/{agent,config/agent}.ts` @ commit `76ced5418f50ee5cfb4c256af358bc0ab063a51b`
> **ทำไมเรื่องนี้สำคัญกับ Aetox ตอนนี้:** `internal/orchestrator` สร้างไว้แล้ว (`Spawn`/`Get`/`Stop`/`List`) แต่ยังไม่มี front end ไหนเรียกใช้ (ARCHITECTURE.md §10) — เอกสารนี้ตอบคำถามที่ยังไม่ได้ตัดสินใจ: sub-agent ควรมีหน้าตา/สิทธิ์ยังไง, ควร spawn ยังไง, ผลลัพธ์ส่งกลับ parent ยังไง

## 1. Agent profile — มีแค่ permission ruleset ไม่มี "tool allowlist" แยก

`packages/opencode/src/agent/agent.ts:35-56` (v1, ตัวที่ execute จริง):
```ts
{
  name: string
  description?: string
  mode: "subagent" | "primary" | "all"
  native?: boolean
  hidden?: boolean
  topP?: number
  temperature?: number
  color?: string
  permission: PermissionV1.Ruleset   // ← ควบคุมสิทธิ์ทุกอย่างผ่านตัวนี้ตัวเดียว
  model?: { modelID, providerID }
  variant?: string
  prompt?: string                     // system prompt override
  options: Record<string, unknown>
  steps?: number                      // max agentic iteration
}
```

**จุดสำคัญที่สุด:** ไม่มี field แยกสำหรับ "tool ไหนใช้ได้บ้าง" — การจำกัด tool ทำผ่าน **permission ruleset เดียวกัน**กับที่ควบคุม read/write/shell (deny tool ตัวไหนก็แค่ deny action นั้นในของ agent เอง) แปลว่า **agent profile = model override + prompt override + permission ruleset** เท่านั้น ไม่มีแนวคิด "tool registry ต่อ agent" แยกต่างหาก

## 2. `task` tool — กลไก spawn sub-agent ตัวเดียว

Sub-agent ไม่ใช่ mechanism พิเศษ — เป็นแค่ **built-in tool ชื่อ `task`** ตัวหนึ่ง:

1. **เช็ค depth ก่อน** (`task.ts:104-117`): เดินย้อน `parentID` chain นับ hop ถ้า `depth >= subagent_depth` (default `1`) → fail ก่อน spawn เลย (`subagent_depth: 0` = ห้าม spawn sub-agent ใดๆ, `1` = primary spawn ได้ชั้นเดียว sub-agent เองจะ spawn ต่อไม่ได้)
2. **เช็ค permission** (`ctx.ask`, เว้นแต่ `bypassAgentCheck`)
3. **สร้าง session ใหม่ทั้งอัน** (`sessions.create({parentID, agent, permission})`) — เป็น conversation context แยกจริงๆ (ประวัติข้อความของตัวเอง) **ไม่ใช่แชร์ context window เดียวกับ parent**
4. Permission ของ session ลูก derive จาก `deriveSubagentSessionPermission`: สืบทอดเฉพาะ **deny rules** กับ `external_directory` rules ของ parent (ไม่ใช่ทั้ง ruleset) ที่เหลือใช้ permission ของ agent ลูกเอง บวก force-deny `todowrite`/`task` เว้นแต่ agent ลูกอนุญาตเอง
5. **รัน turn loop เต็มรูปแบบ** ผ่าน entry point เดียวกับที่ user เรียกปกติ (`ops.prompt(...)`) — sub-agent มี tool loop ของตัวเอง, hook ของตัวเอง, ไม่ใช่แค่ completion เดียว
6. รองรับทั้ง **foreground** (block parent จนเสร็จ) และ **background** (ผลลัพธ์กลับมาเป็น synthetic user message ทีหลัง, อยู่หลัง experimental flag)
7. **ผลลัพธ์กลับ parent:** parent เห็นแค่**ข้อความสุดท้าย**ของ sub-agent (wrap ด้วย `<task>` tag) — **ไม่เห็น transcript เต็มของ sub-agent**

## 3. Nesting — ใช้ตัวนับความลึก ไม่ใช่เช็ค mode

`mode: primary/subagent/all` ควบคุมแค่ว่า agent ตัวนี้**เลือกเป็น agent หลักของ session ได้ไหม** ไม่ได้ป้องกันการ nest เองเลย

ตัวป้องกัน nesting จริงคือ **`subagent_depth` เป็นตัวนับ** (เดินย้อน parentID) — agent `primary` ที่ถูกเรียกเป็น sub-agent (ผ่าน `mode:"all"`) ก็โดน cap ด้วยตัวนับเดียวกัน ไม่มีการยกเว้นตาม mode บวกกับ deny `task` tool ให้ sub-agent เป็น default (belt-and-suspenders สองชั้น)

## 4. Built-in presets — ต่างกันแค่ permission + prompt เท่านั้น

| agent | mode | permission ต่างจาก defaults | prompt |
|---|---|---|---|
| `build` | primary | `question: allow`, `plan_enter: allow` | ค่า default |
| `plan` | primary | `question: allow`, `plan_exit: allow`, `task.general: deny`, **`edit: deny` ยกเว้น `.opencode/plans/*.md`** | ค่า default |
| `general` | subagent | `todowrite: deny` | ค่า default |
| `explore` | subagent | deny ทุกอย่างยกเว้น `grep/glob/list/bash/webfetch/websearch/read` | persona เฉพาะ (file-search specialist) |
| `compaction`/`title`/`summary` | primary, hidden | deny ทุกอย่าง (`"*": deny`) | prompt เฉพาะงาน |

ทั้งหมด merge บน `defaults` ตัวเดียวกัน (`"*": allow`, `question: deny`, `plan_enter/exit: deny`, `read: allow` ยกเว้น `.env*` ที่ ask) แล้ว **user config merge ทับสุดท้าย**

**ข้อสรุปที่สำคัญที่สุดของทั้งเอกสารนี้:** `plan` mode ที่ opencode ขายเป็นฟีเจอร์เด่น (read-only planning mode) **ไม่ใช่ code path แยก** — เป็น primary agent ตัวเดียวกับ `build` เป๊ะ (tool loop เดียวกัน, entry point เดียวกัน) ต่างกันแค่ permission ruleset ที่ deny `edit` เกือบทุกที่ "โหมด" ในระบบนี้คือ**bundle ของ (permission ruleset + prompt + model override)** ไม่ใช่ branch ใน execution engine

## ผลต่อการ wire `internal/orchestrator`

นี่คือคำตอบตรงสำหรับคำถามที่ ARCHITECTURE.md §10 ทิ้งไว้ ("ยังไม่ได้ตัดสินใจว่า sub-agent ควรมี profile/tool-filtering ยังไง"):

1. **ไม่ต้องสร้าง "tool filtering" แยกจาก permission system** — `internal/safety.PermissionConfig` ที่เรามีอยู่แล้วพอสำหรับทำ agent profile ได้เลย แค่เพิ่ม concept "agent profile = {PermissionConfig, SystemPrompt override, Model override}" แล้วให้ `internal/orchestrator.Spawn` รับ profile นี้ตอนสร้าง `cognitive.Agent` ใหม่ — ไม่ต้องเพิ่ม field ใหม่ใน `skill.Registry`/`Dispatcher` เลย

2. **Sub-agent = session ใหม่ + turn loop เดิม ไม่ใช่กลไกพิเศษ** — ตรงกับที่ `internal/orchestrator`ออกแบบไว้แล้ว (สร้าง `cognitive.Agent` ใหม่ต่อ spawn, มี ID+Info snapshot) พอดี ไม่ต้องเปลี่ยนสถาปัตยกรรม แค่ต้องมี **built-in tool ชื่อ `task`** (หรือชื่ออื่นที่ไม่ชนกับที่มีอยู่) ที่: (ก) เช็ค depth จาก parent chain, (ข) เรียก `orchestrator.Spawn` ด้วย profile ที่เลือก, (ค) รัน turn เต็มของ agent ใหม่, (ง) คืนแค่คำตอบสุดท้ายให้ parent

3. **Depth cap ควรมี default เข้มกว่า opencode หรือเท่ากัน** — `subagent_depth: 1` ของ opencode (primary spawn sub ได้ชั้นเดียว, sub เองห้าม spawn ต่อ) เป็นค่าที่สมเหตุสมผลสำหรับ Aetox เช่นกัน ไม่จำเป็นต้อง config ซับซ้อนกว่านี้ตอนเริ่มต้น

4. **Permission ของ sub-agent สืบทอดแค่ deny + external_directory จาก parent** ไม่ใช่ทั้ง ruleset — ถ้า Aetox ทำตาม จะป้องกันไม่ให้ sub-agent มีสิทธิ์กว้างกว่าที่ parent ตั้งใจ (parent deny `shell` → sub-agent deny `shell` ด้วยเสมอ แม้ profile ของ sub-agent เองจะ allow) แต่ sub-agent ไม่ได้สิทธิ์ allow ทั้งหมดของ parent มาฟรีๆ (ต้องมีสิทธิ์ของตัวเองด้วย)

5. **"Plan mode" ที่เคยคุยกันไว้ (competitor-research.md อ้างถึง Aider's Architect mode ด้วย)** ไม่ต้องเป็นฟีเจอร์ใหญ่ — แค่เป็น agent profile ตัวหนึ่งที่ deny `write`/`delete`/`shell` (mutate effects ทั้งหมด) เท่านั้น ใช้โครง `safety.PermissionConfig` ที่มีอยู่แล้วได้ทันที

**ลำดับความสำคัญ:** เรื่องนี้ยังไม่ใช่คิวถัดไป (MCP client มาก่อนตามที่วางแผนไว้) แต่ตอนถึงคิว wiring `internal/orchestrator` จริง เอกสารนี้คือจุดเริ่มต้นที่ตรงประเด็นแล้ว ไม่ต้องออกแบบใหม่จากศูนย์
