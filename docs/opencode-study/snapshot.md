# Filesystem Snapshot / Undo — ความสามารถที่ Aetox ยังไม่มีเลย

> อ่านจาก `packages/core/src/snapshot.ts`, `packages/core/src/git.ts`, `packages/core/src/session/runner/llm.ts`, `packages/opencode/src/session/revert.ts`, `packages/core/test/snapshot.test.ts` @ commit `76ced5418f50ee5cfb4c256af358bc0ab063a51b`
> **แก้ไขจากที่เข้าใจผิดตอนแรก:** `packages/core/src/database/migration/20260605003541_add_session_context_snapshot.ts` **ไม่เกี่ยวกับ filesystem snapshot** — เป็นตาราง `session_context_epoch` สำหรับ compact ประวัติการสนทนา/system prompt คนละเรื่องกันเลย อย่าสับสน

## 1. Trigger — ต่อ "step" ของ LLM ไม่ใช่ต่อ tool call

```
capture()  ← ก่อนเรียก LLM ใน step นี้ (llm.ts:217)
... LLM ตอบ, เรียก tool กี่ตัวก็ได้ ...
capture()  ← หลัง step settle (llm.ts:318)
```
เทิร์นที่แก้ไฟล์ 5 ครั้งได้ capture แค่ 2 ครั้ง (ก่อน/หลัง) แล้ว diff ระหว่างสอง snapshot คือ "ไฟล์ที่เปลี่ยนใน step นี้" ผูกไว้กับ assistant message นั้นสำหรับ revert ทีหลัง — **ถูกกว่า capture ทุก tool call มาก**

Snapshot เป็น no-op ถ้า: (ก) โปรเจกต์ไม่ใช่ git repo, หรือ (ข) user ปิดผ่าน config `snapshots: false` (default เปิด)

## 2. เก็บอะไร — git tree object จริง ไม่ใช่ copy ไฟล์ ไม่ใช่ diff format ของตัวเอง

แต่ละ `capture()`:
1. stage working-tree ปัจจุบันเข้า **shadow git repo แยกต่างหาก** (`git add`/`rm --cached` เทียบเท่า)
2. `git write-tree` → คืน **tree SHA** เป็น snapshot ID

Shadow repo อยู่ที่ `<global data dir>/snapshot/<projectID>/<hash(worktree path)>/` — สร้างด้วย `git init` โดยตั้ง `--work-tree` ชี้ไปที่ worktree จริงของโปรเจกต์ (ไม่ใช่ shadow dir) และ **link object store เข้ากับ `.git/objects` ของ repo จริงผ่าน git alternates file** — ทำให้ reuse blob ที่ commit ไปแล้วได้โดยไม่ copy ซ้ำ เขียนเฉพาะ blob ใหม่ (ไฟล์ที่ยังไม่ commit/untracked) ลง object store ของตัวเอง

ข้อจำกัด: ไฟล์ untracked ที่ใหญ่กว่า 2MB ไม่ capture, ไฟล์ที่ `.gitignore` ไม่นับ

**สรุป: ใช้ `git` CLI plumbing ตรงๆ** (`write-tree`, `read-tree`, `update-index`, `checkout-index`, `ls-tree`, `diff`, `check-ignore`) — ไม่มี storage backend อื่นเลย ยืนยันจาก test file ที่ assert พฤติกรรมผ่าน `git init` จริงเท่านั้น

## 3. Restore/undo — เลือกได้ทีละไฟล์ ไม่ใช่ทั้ง snapshot เดียว

`restore({files: Map<path, treeID>})` — **แต่ละไฟล์ผูกกับ tree ID ของตัวเองได้อิสระ** ไม่ใช่ "restore ทั้งหมดจาก snapshot เดียว" ทำให้ session revert เลือก mix ไฟล์จากหลายจุดในประวัติได้ (เช่น revert ไฟล์ A กลับไป step 2 แต่ไฟล์ B กลับไป step 4)

Implementation: ต่อไฟล์ ถ้ามีอยู่ใน tree เป้าหมาย → `git checkout <tree> -- <file>` (เขียนไฟล์จริงเพราะ `--work-tree` ชี้ไปที่ worktree จริง) ถ้าไม่มีในนั้น (ยังไม่เคยมีไฟล์นี้ตอนนั้น) → ลบไฟล์จริงทิ้ง

มี `checkout(snapshot)` แบบหยาบกว่าด้วย — แทนที่ทั้ง scoped tree รวดเดียว (ไฟล์ที่ไม่มีใน target tree จะไม่ถูกลบ ต่างจาก `restore`)

**Two-way undo:** `stage()` capture snapshot ปัจจุบันไว้ก่อน (`original`) ก่อน apply restore เพื่อให้ `clear()` restore กลับไปที่ก่อน revert ได้ถ้า user ยกเลิก — revert เองก็ undo ได้จนกว่าจะ `commit()`

`preview()` สร้าง diff ที่ restore *จะ* ทำโดยไม่แตะ worktree จริง — ใช้ throwaway git index (`GIT_INDEX_FILE` env override) — เอาไว้โชว์ UI ก่อน user ยืนยัน

## 4. Storage location & format

- `<global data>/snapshot/<projectID>/<hash(worktree)>/` — git dir จริง ตั้งค่า `core.autocrlf=false`, `core.longpaths=true`, `index.version=4` เพื่อ perf กับ tree ใหญ่
- Snapshot ID = git tree SHA ตรงๆ (`Git.TreeID`, branded string)
- ต้องมี `git` binary ในเครื่อง + โปรเจกต์เป็น git repo อยู่แล้ว — ไม่ standalone

## ผลต่อ Aetox

Aetox ยังไม่มี safety-net แบบนี้เลยตอน agent แก้ไฟล์ผิด — ตอนนี้พึ่ง git ปกติของ user เอง (ถ้า user commit บ่อยพอ) กับ approval gate ก่อนแก้ (`internal/safety`) เท่านั้น ไม่มี "undo สิ่งที่ agent เพิ่งทำ" แบบละเอียดต่อไฟล์

**ทำไมถึงคุ้มทำทีหลัง (ไม่ใช่ตอนนี้):**
- ต้องมี `git` เป็น dependency runtime อยู่แล้ว (Aetox มี `internal/skill/git.go` ที่ shell out ไป `git` อยู่แล้ว — ไม่ใช่ dependency ใหม่)
- ไม่ต้องเพิ่ม storage infra ใหม่ (ใช้ git object store, ไม่ใช่ DB/ไฟล์ format ของตัวเอง)
- Design ทั้งหมด (shadow repo + alternates, capture ต่อ turn, restore ต่อไฟล์) พอร์ตเป็น Go ได้ตรงๆ เพราะเป็นแค่ `os/exec` เรียก `git` plumgin command เหมือนที่ `internal/skill/git.go` ทำอยู่แล้ว ไม่ต้องเขียน git library เอง

**เมื่อจะทำ:** ตามลำดับความสำคัญเดิม (MCP → plugin hooks → orchestrator wiring) นี่ยังไม่อยู่ในคิว แต่ถือเป็น "ถูกและเร็วที่จะทำ" เมื่อถึงเวลา เพราะต้นทุนทางเทคนิคต่ำกว่าฟีเจอร์อื่นในลิสต์มาก (ไม่มี protocol ใหม่ ไม่มี process lifecycle ใหม่ ไม่มี network ใหม่ — แค่เรียก `git` เพิ่มอีกไม่กี่ subcommand)
