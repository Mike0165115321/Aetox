# Aetox CLI — Response Contract & Approval Policy

## 1) Execution flow

- **Conversation (non-skill input)**  
  Sent directly to model and streamed back as normal chat.

- **Skill input (`/` หรือคำสั่ง skill ที่รู้จัก)**  
  รัน `intent -> execute -> normalize -> summarize -> respond`.

  - User sees spinner while skill is running:
    - `กำลังคิด...` for model thinking
    - `กำลังรัน...` for skill execution
  - Spinner is cleared before final answer.

## 2) Tool response status

- `executed (done)`  
  command ผ่านตามปกติ
- `executed (error)`  
  รันเสร็จแต่มีข้อผิดพลาด / timeout / cancel / summarize error fallback
- `executed (blocked)`  
  คำสั่งความเสี่ยงและถูกผู้ใช้ปฏิเสธ

## 3) Approval policy

- Default allow (v1)
  - `git status|log|branch|diff|show`
  - `fs pwd|ls|find|cat`
  - shell command that matches safe patterns
- High-risk requires confirmation
  - shell write/delete actions
  - `write` create/overwrite files
  - unsupported/high-risk git actions
  - non read-only fs actions
- Policy text is shown on confirmation prompt:
  - `Aetox: command '<command>' is high-risk ... confirm?`

If confirm is denied, result ends with `executed (blocked)` and no tool side-effect after confirmation.
