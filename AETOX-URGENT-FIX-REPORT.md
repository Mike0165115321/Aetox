# AETOX CLI - URGENT FIX REPORT
วันที่ทดสอบ: 2026-06-09  
สถานที่: `E:\Aetox\Aetox-cli`  
ผู้ทดสอบ: Aetox CLI (โดยตรงผ่าน `.\\aetox.exe`)

## สรุปสั้น
ระบบทำงานได้ในแนวคิดหลัก แต่มีจุดสำคัญที่ต้องแก้ด่วนเพื่อให้ใช้งานได้ตามเอกสารและคาดหวัง (โดยเฉพาะ One-shot, `fs find`, และความเสถียรของ provider fallback)  
ระดับความเสี่ยงโดยรวม: **สูงปานกลาง** (ใช้งานยังใช้งานได้บางส่วน แต่มีจุดพังที่ผู้ใช้เจอได้ง่าย)

## 1) รายการที่ทำงานปกติ
- `aetox --help`, `aetox -h`, `aetox --version`, `aetox version`
- `aetox chat help`
- `aetox chat time`
- `aetox chat echo ...`
- `aetox chat list [path]`
- `aetox chat fs pwd / fs ls / fs cat`
- `aetox chat read README.md`
- `aetox chat git status/log/branch/diff/show`
- `aetox chat shell echo HELLO_FROM_AETOX` (คำสั่งที่รองรับโดย shell)
- `aetox chat shell del ...` (executed ได้แม้ไฟล์ไม่อยู่จริง, แค่แสดงข้อความผิดพลาดจาก cmd)
- `aetox chat write` + `aetox chat delete` ในกรณี path ใน sandbox ถูกต้อง
- `aetox chat plugin_install` (พบ usage/validation เมื่อพารามิเตอร์ผิด)
- `aetox chat github_repo_summary https://github.com/openai/openai-cookbook`
- คำสั่งไม่รองรับจะได้ข้อความ error ที่อ่านได้ (เช่น `unknowncmd`)

## 2) ปัญหาที่ต้องแก้ **ด่วน**
| ลำดับ | ความรุนแรง | รายการ | สาเหตุ/การสังเกต | ควรแก้ |
|---|---|---|---|---|
| 1 | สูง | One-shot via stdin ไม่ทำงาน | รัน `echo สวัสดี | .\aetox.exe` แล้วกลับ `usage`/`help` เสมอ | แก้ parser ให้รับ pipe input ตาม README หรืออัปเดต README/Help ให้ตรงพฤติกรรมจริง |
| 2 | สูง | `fs find` ไม่ทำงานทั้ง regex และ glob ตามที่เอกสารบอก | `fs find .*\.md` -> ต้องการ regex แล้ว reject, `fs find "*.md"` -> บอกว่า glob ไม่อนุญาต | กำหนดมาตรฐานเดียว: รองรับ glob หรือ regex อย่างใดอย่างหนึ่ง แล้วให้เอกสารและ error message ตรงกัน |
| 3 | สูง | การคืนค่า error/status ของ `aetox help` และคำสั่งใน shell ไม่สม่ำเสมอ | `--model-provider invalid` ให้ warning ซ้ำหลายบรรทัดในหลาย command พร้อมข้อความ `unsupported model provider: "invalid"` | ตรวจสอบ path ของค่า provider ไม่ให้ “leak” state ระหว่างรัน และลด warning ซ้ำให้ชัดเจน |
| 4 | กลาง | `shell` ใช้ cmd เป็น backend ทำให้ cmdlet ของ PowerShell ใช้ไม่ได้ | `shell Get-Location` error: cmd command ไม่รองรับ | ระบุชัดเจนใน docs ว่า shell backend เป็น cmd-only หรือลองเพิ่มการรองรับ shell mode อัตโนมัติ |
| 5 | กลาง | ข้อความช่วยเหลือ/usage ขัดกัน | `--help` แสดง option และ command ไม่ละเอียดเทียบ `chat help` / README (เช่น listing ใหม่บางตัว) | ทำให้ usage/help มีรายการและโหมดเดียวกันครบถ้วน |
| 6 | กลาง | การจัดการกรณี `fs find` และ `delete` เมื่อ path ผิดต้อง return error type ที่ชัด | พบข้อความ error แต่ไม่สม่ำเสมอระหว่าง done/error status | จัดรูปแบบ output ให้คงเส้นทางเดียว: status + สาเหตุ + คำแนะนำแก้ไข |

## 3) จุดพฤติกรรมที่พบ (ควรดูแลหลังแก้ด่วน)
- `aetox chat echo` ในโหมด chat กลับเป็นข้อความสรุปสำเร็จ แต่โหมด conversation อาจเข้า noop fallback แสดง warning แปลก ๆ หาก provider ตั้งไม่ถูกต้อง  
- `shell` คำสั่งที่ออกจาก cmd สำเร็จดี แต่ไม่มีตัวเลือกให้บังคับ shell engine ในเอกสาร/CLI flags
- `delete` ป้องกัน path นอก sandbox ดี (ดีมาก) ควรรักษาพฤติกรรมนี้ไว้

## 4) ข้อเสนอแนะการแก้ไขแบบลำดับด่วน
1. แก้ parser one-shot stdin ทันที (Priority 1)  
2. ตัดสินใจนโยบาย `fs find` ให้ชัด (glob หรือ regex อย่างใดอย่างหนึ่ง) และซิงก์ error/help/docs (Priority 1)  
3. ปรับ provider fallback ให้ไม่ซ้ำ/ชัดเจน และไม่ล็อก error state ข้าม command (Priority 2)  
4. ปรับ shell docs + behavior ให้ตรงกัน (Priority 2)  
5. รวมข้อความช่วยเหลือ `--help` และ `chat help` ให้เทียบกันได้ (Priority 3)  

## 5) เสนอแนวทางการยืนยันการแก้
ให้รันทดสอบชุดเดียวเดิมอีกครั้ง (repro commands):
- `echo test | .\aetox.exe`
- `aetox chat fs find "*.md"`
- `aetox chat fs find .*\.md`
- `aetox chat shell Get-Location`
- `aetox --help` และ `aetox chat help` เทียบผล
- `aetox chat "hello"` ด้วยสถานะ provider ติดตั้งปกติ

หากต้องการ ผมสามารถทำ pass ต่อด้วย "สคริปต์ทดสอบอัตโนมัติ" ใน root ให้เรียก run ซ้ำได้อัตโนมัติได้เลยครับ.
