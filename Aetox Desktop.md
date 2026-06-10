## Desktop Stack Decision

Aetox Desktop จะใช้ **Wails + Go + Svelte/React** เป็น stack หลัก

เหตุผลหลักคือ Aetox มี core runtime ที่เขียนด้วย Go อยู่แล้ว การใช้ Wails ทำให้สามารถนำ core เดิม เช่น turn executor, model provider, skill dispatcher, safety, audit และ Aetox.md governance มาใช้ต่อได้โดยไม่ต้องย้ายระบบไปภาษาอื่น

Frontend จะใช้ Svelte หรือ React สำหรับสร้าง UI แบบ desktop cockpit โดยมี Go เป็น backend engine และ Wails เป็น bridge ระหว่าง UI กับ core runtime

### Decision

```txt
Desktop Framework: Wails
Backend/Core: Go
Frontend: Svelte หรือ React
Preferred Frontend: Svelte ถ้าต้องการความเบาและเรียบง่าย
Alternative: React ถ้าต้องการ ecosystem และ component เยอะกว่า
```

### Rationale

* เบากว่า Electron
* ไม่ต้องทิ้ง Go core เดิม
* เหมาะกับ desktop app ที่ต้องเข้าถึง filesystem, command, git และ local runtime
* ทำ UI สวยแบบ web technology ได้
* เหมาะกับคนพัฒนาคนเดียว เพราะลดความซับซ้อนด้าน backend/frontend bridge
* สามารถแยก Aetox Core ออกจาก Aetox Desktop ได้ชัดเจน

### Architecture Direction

```txt
Aetox Desktop = UI / Cockpit
Aetox Core = Execution Engine
Wails = Bridge between UI and Go runtime
```

Frontend ต้องทำหน้าที่เป็นหน้าจอควบคุมเท่านั้น
logic หลักของ agent, model, tool execution, safety และ verification ต้องอยู่ใน Go core

### Non-Goal

Aetox Desktop จะไม่ใช้ Electron เป็น stack หลัก เพราะ Electron หนักกว่าและไม่เหมาะกับเป้าหมายที่ต้องการ desktop app ที่เบาและกินทรัพยากรน้อย

Tauri เป็นตัวเลือกที่ดีและเบามาก แต่ไม่ใช่ตัวเลือกหลักของ Aetox ตอนนี้ เพราะ backend หลักของ Tauri คือ Rust ซึ่งจะเพิ่มภาระในการ bridge กับ Go core เดิม
