# Plugin Hook System — event-driven extension points

> อ่านจาก `packages/plugin/src/index.ts`, `packages/opencode/src/plugin/{index,loader,shared,pty-environment}.ts`, `packages/opencode/src/session/{tools,prompt,llm/request,compaction,processor}.ts`, `packages/opencode/src/tool/registry.ts`, `packages/core/src/plugin/*.ts` @ commit `76ced5418f50ee5cfb4c256af358bc0ab063a51b`
> **หมายเหตุ:** เหมือนกับ permission system — มี "v1" (`packages/opencode/src/**`, ตัวที่ execute จริง) กับ "v2" (`packages/core/src/plugin/*.ts`, Effect-based, ยังเป็นแค่ config-transform layer ไม่ได้ขับ tool loop) เอกสารนี้อ้าง v1 เป็นหลักเพราะเป็นตัวที่มี call site จริงให้ตรวจสอบ

## 1. Hooks shape เต็ม (`packages/plugin/src/index.ts:222-335`)

ทุก hook แบบ "trigger" มี signature เดียวกันหมด: **`(input, output) => Promise<void>`** — **ไม่ return ค่าใหม่ แต่ mutate `output` object ตรงๆ**

| Hook | เรียกจากไหนจริง | ทำอะไรได้ |
|---|---|---|
| `event` | event bus subscription, fan-out ทุก event (ไม่ผ่าน `trigger()`) | สังเกตการณ์อย่างเดียว, fire-and-forget ไม่ await |
| `config` | ครั้งเดียวตอน plugin โหลดเสร็จ | เห็น config ที่ merge แล้ว |
| `chat.message` | `session/prompt.ts:999` ก่อนเริ่ม turn | แก้ `parts`ของ user message ก่อนส่งโมเดล |
| `chat.params` | `session/llm/request.ts:114-132` | แก้ temperature/topP/topK/maxOutputTokens/options ก่อนยิง provider |
| `chat.headers` | `session/llm/request.ts:134-146` | แก้ HTTP headers ที่ส่งไป provider |
| `permission.ask` | **ไม่มี call site เลย** (ดูข้อ 4) | เอกสารบอกว่า deny/ask/allow ได้ แต่โค้ดจริงไม่เคยเรียก |
| `command.execute.before` | `session/prompt.ts:1460` | แก้ parts ของ slash command ก่อนรัน |
| `tool.execute.before` | `session/tools.ts` (built-in+MCP tool ทุกตัว), `session/prompt.ts` (task tool) | mutate field ใน `args` ได้ (**แทนที่ทั้ง object ไม่ได้** ดูข้อ 4) |
| `shell.env` | `plugin/pty-environment.ts`, `tool/shell.ts`, PTY HTTP route | แก้ env vars ก่อน spawn shell |
| `tool.execute.after` | เหมือน `tool.execute.before` | mutate title/output/metadata ได้เต็มที่ (เพราะเป็น object เดียวกับที่ return จริง) |
| `experimental.chat.messages.transform` | `session/prompt.ts:1255`, `session/compaction.ts:350` | rewrite ประวัติ message ทั้ง context |
| `experimental.chat.system.transform` | `session/llm/request.ts:69` | rewrite system prompt |
| `experimental.session.compacting` / `.autocontinue` | `session/compaction.ts:343-350,454` | ควบคุม logic การบีบอัด context |
| `experimental.text.complete` | `session/processor.ts:516` | rewrite ข้อความที่ stream ออกมาทีละ chunk |
| `tool.definition` | `tool/registry.ts:313` | rewrite description/schema ของ tool ก่อนส่งให้โมเดลเห็น |
| `provider` / `auth` / `tool` (object shape, ไม่ใช่ callback) | โหลดตอน plugin init | ประกาศ custom provider/auth flow/tool ใหม่ทั้งตัว |

## 2. การเรียกจริง — sequential, deterministic order

```ts
// loader.ts:280-293
for (const hook of s.hooks) {
  const fn = hook[name]
  if (!fn) continue
  yield* Effect.promise(async () => fn(input, output))
}
return output
```
เรียงตาม**ลำดับที่ plugin ถูกลงทะเบียน** เสมอ (comment ในซอร์สยืนยันว่าจงใจ ไม่ใช่ race)

## 3. Loading — npm package หรือไฟล์ local ก็ได้

Config key `plugin` ใน `opencode.json`: `string | [string, Options]` (npm spec หรือ `[spec, options]`) นอกจากนี้ยัง auto-discover จาก `{plugin,plugins}/*.{ts,js}` relative to config dir โดยไม่ต้องประกาศ

Pipeline: install (npm-install อัตโนมัติถ้าจำเป็น) → หา entrypoint (`package.json` `exports["./server"]`/`main`) → เช็ค version compatibility (เฉพาะ npm plugin, local file ข้าม) → `import()` แบบ dynamic

Built-in plugin (auth provider สำหรับ Copilot/Codex/GitLab/ฯลฯ) import ตรงๆ ไม่ผ่าน npm resolve และ**โหลดก่อน**ทุก user plugin เสมอ

Plugin โหลดแบบ **sequential ไม่ parallel** (จงใจ เพื่อให้ registration order deterministic)

## 4. Mutation power และข้อจำกัดที่พลาดง่าย

**`tool.execute.before` แทนที่ `args` ทั้งก้อนไม่ได้ — mutate field ได้เท่านั้น**

โค้ดจริง (`tools.ts:106-125`): wrap `args` เป็น `{args}` ส่งให้ hook, แต่จุดเรียก `item.execute(args, ctx)` ใช้**ตัวแปร `args` เดิม** (closure variable) ไม่ได้อ่านจาก `output.args` กลับมา — เพราะ `args` ใน `{args}` เป็น object reference เดียวกัน hook แก้ **field ข้างใน** (`output.args.foo = ...`) มีผลจริง แต่ถ้า hook เขียน `output.args = {...}` (สร้าง object ใหม่ทั้งก้อน) **จะถูกเมิน** เพราะไม่มีจุดไหน reassign กลับ — เป็น footgun ที่ต้องตัดสินใจให้ชัดเจนตอน design ระบบ Go ของเรา ไม่ใช่ copy พฤติกรรมนี้มาโดยไม่ตั้งใจ (แนะนำ: ถ้า Go ทำ hook ระบบ ให้ hook คืนค่าใหม่แทนการ mutate in-place จะชัดเจนและปลอดภัยกว่า — Go ไม่มี "mutate reference" แบบ JS object อยู่แล้วโดยธรรมชาติ)

**`permission.ask` เป็น hook ที่ตายแล้ว — เอกสารมีแต่โค้ดไม่เรียก**

ยืนยันแล้วว่าไม่มี call site ไหนเรียก `plugin.trigger("permission.ask", ...)` เลยทั้ง repo การอนุมัติ/ปฏิเสธมาจาก ruleset evaluator (`Permission.evaluate`) ล้วนๆ plugin **ไม่มีทาง** deny tool call ผ่าน hook นี้ได้ในปัจจุบัน แม้ type จะ advertise ไว้ว่าทำได้

**บทเรียนสำหรับเอกสารของเราเอง:** ก่อนอ้างว่า feature ไหน "มีจริง" ต้อง grep หา call site จริง ไม่ใช่แค่เจอ type/schema definition — เป็นความผิดพลาดที่เกิดขึ้นได้แม้กับ codebase ที่เป็น production จริงขนาดนี้

**ไม่มี hook ไหน abort tool call แบบ clean ได้** — throw/reject ใน hook กลายเป็น unhandled failure ที่ทำให้ทั้ง operation พัง ไม่ใช่ signal "denied" ที่ควบคุมได้

## 5. Error isolation — ไม่สมมาตรกันระหว่าง load-time กับ per-turn

- **Load/dispose/config() ครั้งแรก:** ห่อด้วย `Effect.tryPromise` + catch + log — plugin หนึ่งพังไม่ทำให้ตัวอื่นโหลดไม่ได้
- **Hook ตอน trigger ระหว่าง turn (`trigger()`):** **ไม่มี error isolation เลย** — `yield* Effect.promise(async () => fn(input, output))` ไม่มี try/catch ห่อ ทั้งใน `trigger()` เองและจุดเรียกทุกจุด (`tools.ts`, `prompt.ts`, `request.ts`) เป็น unhandled defect ที่ทำให้ทั้ง tool call/turn พังถ้า hook throw
- **`event` hook:** fire-and-forget (`void hook["event"]?.(...)`) ไม่ await เลย — error หายไปเงียบๆ เป็น unhandled rejection

**ผลต่อ Go ของเรา:** ความไม่สมมาตรนี้ในซอร์สจริงอ่านเหมือนเป็น**ความพลาด**มากกว่าการออกแบบตั้งใจ (เพราะ load-time isolate ไว้ดีแล้ว แต่ per-turn ไม่ทำเหมือนกัน) — ตอนออกแบบ hook system เวอร์ชัน Go ควรตัดสินใจให้ชัดตั้งแต่แรกว่า hook ที่ throw ระหว่าง turn ควร isolate (log แล้ว skip hook นั้น, turn เดินต่อ) หรือควร fail ทั้ง turn — แนะนำ isolate เพราะ hook เป็นของ third-party/optional โดยธรรมชาติ ไม่ควรทำให้ built-in flow พังเพราะ plugin คนอื่นเขียนพลาด

## สรุปสำหรับ Go implementation

1. **Hook shape:** ใช้ pattern คืนค่าใหม่ (`func(input) (output, error)`) แทน mutate-in-place — เข้ากับ Go idiom อยู่แล้ว และเลี่ยง footgun ข้อ 4 ได้ตั้งแต่ต้น
2. **Loading:** local file/plugin ก่อน (Go ไม่มี npm dynamic install ที่เทียบเท่าง่ายๆ) — อาจใช้ Go plugin system หรือ subprocess-based extension (เช่นเดียวกับแนวทาง `internal/skill/discovery.go`'s SKILL.md ที่เราเพิ่งทำ ซึ่งเป็น "ไฟล์ markdown ให้โมเดลอ่านเอง" ไม่ใช่ compiled code — เข้าใกล้ opencode's skill format มากกว่า plugin format)
3. **Sequential order:** เก็บ pattern นี้ไว้ — deterministic ordering สำคัญกว่า throughput สำหรับ hook ที่เรียกไม่บ่อย
4. **Error isolation ระหว่าง turn:** ทำให้ดีกว่า opencode ตั้งแต่แรก — wrap ทุก hook call ด้วย recover/error-return ที่ log แล้ว skip แทนที่จะ panic/fail ทั้ง turn
5. **`permission.ask` เป็นตัวอย่างเตือนใจ:** ถ้าจะประกาศ hook ไหนว่า "ควบคุม permission ได้" ต้อง wire เข้า `internal/turn.Executor.resolveApproval` จริงๆ ตั้งแต่ commit แรก ไม่ใช่ประกาศ interface ไว้ก่อนแล้วไม่ต่อสาย
6. **ลำดับความสำคัญเดิมยังใช้ได้:** plugin hooks มาหลัง MCP client ตามที่วางแผนไว้ — เอกสารนี้แค่เตรียมความรู้ล่วงหน้า ไม่ได้เปลี่ยนลำดับ
