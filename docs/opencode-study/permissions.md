# Permission Engine — เทียบของจริง opencode กับ `safety.PermissionConfig` ที่เราเพิ่งสร้าง

> อ่านจาก `packages/core/src/permission.ts`, `packages/schema/src/permission.ts`, `packages/core/src/util/wildcard.ts`, `packages/core/src/plugin/agent.ts`, `packages/core/src/config/plugin/agent.ts`, `packages/core/src/permission/{sql,saved}.ts`, `packages/opencode/src/tool/bash.ts` @ commit `76ced5418f50ee5cfb4c256af358bc0ab063a51b`
> **หมายเหตุสำคัญ:** repo นี้มี**สองระบบ permission คู่ขนาน** — `v1/permission.ts` (legacy, ยัง wire อยู่กับ runtime บางส่วน, config เป็น nested object `{bash:{"rm *":"deny","*":"allow"}}`) กับ **v2** (`core/src/permission.ts`, เป็นระบบปัจจุบัน, config เป็น **ordered array** ของ `{action, resource, effect}`) เอกสารนี้อ้างอิง v2 เป็นหลักเพราะเป็นตัวที่ active จริง

## 1. Matching algorithm — glob เขียนเอง ไม่ใช่ regex/lib จริง

`packages/core/src/util/wildcard.ts:1-14` แปลง glob เป็น regex เอง (`*`→`.*`, `?`→ตัวเดียว), ไม่รองรับ `**` แยกจาก `*`, ไม่มี bracket class ใช้กับ**ทั้ง** `action` และ `resource` field เหมือนกัน มี normalize `\` → `/` (รองรับ Windows path) และ case-insensitive เฉพาะบน `win32`

**เทียบกับของเรา** (`internal/safety/safety.go`'s `globMatch`): เราทำ per-character escape ผ่าน `regexp.QuoteMeta` ต่อ rune ซึ่งปลอดภัยกว่า (ไม่มี edge case จาก unescaped regex metachar หลุดเข้าไป) — ของ opencode ใช้ `.replace(/[.+^${}()|[\]\\]/g, "\\$&")` ครั้งเดียวทับทั้ง string ก่อนแทน `*`/`?` ทีหลัง ผลลัพธ์เหมือนกันในทางปฏิบัติ ไม่มีอะไรต้องแก้

## 2. Precedence — `Array.findLast` ล้วนๆ ไม่มี specificity scoring

```ts
// permission.ts:76-90
export function evaluate(action, resource, ...rulesets) {
  return rulesets.flat().findLast(rule =>
    Wildcard.match(action, rule.action) && Wildcard.match(resource, rule.resource)
  ) ?? { action, resource: "*", effect: "ask" }
}
```

ยืนยันว่า **"last-match-wins" ของเราตรงกับของจริง** — ไม่มี "pattern เจาะจงกว่าชนะ" ใดๆ ทั้งสิ้น ลำดับเกิดจาก**ตำแหน่งใน array ที่ build ขึ้นมา** เท่านั้น (ดูข้อ 3)

**จุดที่เราต้องคิดเพิ่ม — deny มี pass แยกก่อน saved rules:**
```ts
// permission.ts:147-162 (ย่อ)
function denied(input, rules) {
  return input.resources.some(r => evaluate(input.action, r, rules).effect === "deny")
}
const rules = configured(...)            // เฉพาะ agent/global config rules
if (denied(input, rules)) return {effect:"deny", ...}   // เช็ค deny ก่อน โดยยังไม่รวม saved rules
const all = [...rules, ...savedRules()]  // saved ("always allow") เอามาต่อทีหลัง
```
แปลว่า: **deny ที่มาจาก config/agent ruleset ไม่มีทาง "always allow" ของ user มา override ได้** เพราะ deny ถูกเช็คจาก rule set ที่ยังไม่รวม saved-allow เลย แต่ `ask` จาก config **override ได้** ด้วย saved-allow (เพราะ saved ถูกต่อท้ายและ `findLast` เลือกตัวหลัง)

**ผลต่อ Go ของเรา:** ถ้าจะเพิ่ม "always allow" (persist การอนุมัติ) ใน `internal/safety` ทีหลัง ต้องทำ deny ให้ "แข็ง" แบบนี้ — เช็ค deny จาก config-level rules ก่อนเสมอ แล้วค่อยรวม persisted-allow เข้ามา ไม่ใช่โยนทุกอย่างลง slice เดียวแล้ว `Resolve()` ตรงๆ (ตอนนี้เรายังไม่มี persisted rules เลย เป็นแค่ config-level เท่านั้น — จุดนี้เป็น future work ไม่ใช่บั๊กปัจจุบัน)

เมื่อ action เดียวมีหลาย `resources` (เช่น edit หลายไฟล์พร้อมกัน) ผลลัพธ์รวม = **worst-case ชนะ**: มี deny ที่ไหนก็ deny ทั้งหมด, ไม่มี deny แต่มี ask ที่ไหนก็ ask, ไม่งั้น allow

## 3. Per-agent override vs global config — ต่อ array เรียงลำดับ ไม่มี "merge algorithm" จริงจัง

ไม่มีฟังก์ชัน merge เฉพาะ — ลำดับความสำคัญเกิดจาก**การ push ต่อท้าย array**เท่านั้น รวมกับ `findLast`:

```
agent.permissions = [
  ...built-in defaults (เช่น "*": allow),
  ...built-in ต่อ agent นี้เอง (เช่น plan: "edit *": deny),
  ...global config.permissions (จาก opencode.json ราก),
  ...agents.<name>.permissions (per-agent override ใน config),
]
```
เพราะ `findLast`: **per-agent config ชนะ global config ชนะ built-in default เสมอ** ถ้า pattern overlap — แต่ทั้งหมดนี้เป็น "ผลข้างเคียง" ของลำดับ push ไม่ใช่ concept "layer" ที่มี precedence rule ของตัวเอง ถ้า pattern สอง rule ใน array เดียวกัน overlap กันแบบไม่ชัดเจนว่าอันไหน "เจาะจงกว่า" ก็อาศัยตำแหน่งเท่านั้น ไม่มี tiebreak อัจฉริยะ

**ผลต่อ Go ของเรา:** ตอนจะเพิ่ม per-agent profile (เชื่อมกับ `agents.md`/`internal/orchestrator`) ให้ต่อ `PermissionConfig.Rules` แบบเดียวกัน: `defaults + globalConfig + agentProfile` เรียงลำดับ ไม่ต้องสร้าง merge function ซับซ้อน — `Resolve()` ที่มีอยู่แล้ว (last-match-wins) รองรับ pattern นี้ได้ทันทีแค่ต่อ slice ให้ถูกลำดับตอน build

## 4. Argument matching — เฉพาะ `bash` เท่านั้นที่ match ทั้ง command string ดิบ

```ts
// tool/bash.ts:142-149
yield* permission.assert({
  action: "bash",
  resources: [input.command],   // ทั้ง command string ตามที่พิมพ์ ไม่ parse
  save: [input.command],
  ...
})
```
**ไม่มีการ parse เป็น argv/token ใดๆ ก่อน match** — `resource` ที่ใช้ match คือ raw string เป๊ะๆ (มี tokenizer แยกอยู่ (`shellTokens`) แต่ใช้แค่เป็น**advisory warning** เรื่อง path นอก workspace เท่านั้น ไม่ได้เอาไปทำ pattern matching) มี TODO comment ในซอร์สยืนยันว่า tree-sitter-based parsing เป็นแผนอนาคตที่ยังไม่ทำ

**เทียบกับของเรา:** `internal/safety.AssessCommand` เรา parse `args[0]`/`args[1:]` เป็น token แยกแล้วเช็ค flag เฉพาะ (`-rf`, `--force` ฯลฯ) — **ละเอียดกว่า opencode ในจุดนี้อยู่แล้ว** ไม่ต้องถอยไปทำแบบ raw-string-only เพียงแต่ `safety.PermissionConfig.Resolve` (permission rule ของ user) ตอนนี้ join args ด้วย space เป็น string เดียวก่อน match — เหมือนแนวทาง opencode สำหรับ `bash`/`shell` แต่ยังคง `AssessCommand`'s token-level risk assessment ไว้เป็นชั้นแยก ถือว่าออกแบบมาดีอยู่แล้ว ไม่ต้องแก้

## 5. Persistence — SQLite ต่อ project, saved rule เป็น allow เท่านั้น

`permission/sql.ts`: ตาราง `permission` (Drizzle/SQLite) คีย์ unique `(project_id, action, resource)`
`permission/saved.ts`: `add({projectID, action, resources})` insert ทีละแถวต่อ resource, **effect เป็น `"allow"` เสมอ ไม่มี deny/ask ที่ persist ได้**

พฤติกรรมที่น่าสนใจ: ตอบ `"always"` ครั้งเดียวจะ (ก) insert ลง DB และ (ข) **re-evaluate ทุก pending prompt ที่ค้างอยู่ใน session เดียวกันทันที** — ถ้ามี 3 คำสั่งเดียวกันรอ approve พร้อมกัน กด allow-always ตัวแรกแล้วอีกสองตัวหายไปเอง ไม่ต้องรอ DB round-trip ใหม่

**ผลต่อ Go ของเรา:** ตอนนี้ `config.LoadPermissions`/`SavePermissions` ของเราเป็น flat JSON file ไม่ใช่ต่อ project และไม่มี "always allow" runtime flow (user ต้องแก้ json เอง) — ถ้าจะทำ "always allow" แบบ interactive ในอนาคต ตาม opencode ควร: (1) scope ต่อ project ไม่ใช่ global, (2) saved rule เป็น allow-only เพื่อความปลอดภัย (deny ต้องมาจาก config ที่ควบคุมได้เท่านั้น ไม่ควร persist จาก runtime prompt), (3) propagate ไปยัง pending prompt อื่นในเซสชันเดียวกันทันที ไม่ต้องรอ reload

## ตารางสรุปเทียบ

| ประเด็น | opencode v2 | Aetox (`internal/safety`) ตอนนี้ |
|---|---|---|
| Pattern algorithm | glob→regex เขียนเอง, match ทั้ง action+resource | เหมือนกัน (`globMatch`, per-rune QuoteMeta ปลอดภัยกว่าเล็กน้อย) |
| Precedence | `findLast` บน flat array | เหมือนกัน (`Resolve` ก็ last-match-wins) |
| Deny vs saved-allow | deny เช็คจาก config-only rules ก่อนเสมอ, saved-allow override ไม่ได้ | ยังไม่มี persisted/saved rules เลย — ยังไม่ใช่ปัญหาตอนนี้ แต่ต้องจำ pattern นี้ไว้ถ้าจะเพิ่ม |
| Per-agent layering | defaults → global → per-agent, append order ล้วนๆ | ยังไม่มี agent-profile concept — ดู `agents.md`, ต่อ `Rules` แบบเดียวกันได้ทันทีเมื่อสร้าง |
| Argument matching | เฉพาะ bash, raw string, ไม่ parse | มี `AssessCommand` แยก parse token อยู่แล้ว (ดีกว่า) + `PermissionConfig` join string (เหมือนกัน) |
| Persistence | SQLite ต่อ project, allow-only, propagate ไป pending prompts อื่นทันที | JSON file, ไม่ต่อ project, ไม่มี runtime "always allow" flow เลย — gap ถ้าจะทำ interactive persist ในอนาคต |

**สรุปภาพรวม:** สิ่งที่เราสร้างใน `internal/safety.PermissionConfig` (จบไปแล้วในรอบก่อนหน้า) **ตรงกับสถาปัตยกรรมจริงของ opencode ในทุกจุดสำคัญ** ไม่มีอะไรต้องรื้อ ส่วนที่ opencode มีแต่เรายังไม่มีคือ (1) per-agent permission layering (รอ agent-profile concept, ดู `agents.md`) และ (2) runtime "always allow" ที่ persist ต่อ project — ทั้งสองไม่ใช่ของเร่งด่วน เพิ่มได้ทีหลังโดยไม่ต้องแก้ core design ของ `Resolve()`
