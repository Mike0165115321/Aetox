# Aetox CLI (Terminal Chat)

Aetox CLI ตอนนี้เป็นโหมดแชตผ่านเทอมินัล:
- รับข้อความจากผู้ใช้
- ส่งให้ model (default: `noop`)
- ตอบกลับโดยตรง

## โครงสร้างใหม่

- `cmd/aetox/main.go`: CLI bootstrap + flag parsing + mode dispatch
- `internal/app`: app shell layer (`RunInteractive`, `RunOnce`, banner, terminal I/O interface)
- `internal/command`: command intent parsing seam (`help/version/interactive/once`)
- `internal/skill`: skill abstraction + registry + dispatcher seam
- `internal/cognitive`: agent orchestration (`Agent`, context, provider call)
- `internal/model`: provider abstraction (`noop`, `openrouter`, `openai-compatible`, `ollama`) + bootstrap seam
- `internal/config`: runtime config defaults
- `internal/memory`: bounded context store for `cognitive.Agent`

ดูเอกสารสถาปัตยกรรมฉบับปฏิบัติแบบละเอียดได้ที่:
- [docs/architecture-aetox.md](/E:/Aetox/Aetox-cli/docs/architecture-aetox.md)

## เอกสารสถาปัตยกรรม

- [architecture review (scan mode)](/E:/Aetox/Aetox-cli/docs/architecture-review-aetox-cli.md)

## รันทันที

```powershell
cd E:\Aetox\Aetox-cli
go build -o C:\Users\Gigabyte\bin\aetox.exe .\cmd\aetox
aetox.exe --help
```

### แชตผ่านเทอมินัล (พร้อมสกิล)

```powershell
aetox
aetox chat "ช่วยสรุปโปรเจกต์นี้"
```

สกิลที่ใช้งานได้ทันทีในโหมดแชต:
- `help` แสดงสกิลที่พร้อมใช้
- `time` ดูเวลา
- `echo <ข้อความ>` ส่งข้อความกลับ
- `list [path]` รายการไฟล์ใน `--root` (ป้องกันออกนอกโฟลเดอร์)
- `shell <command>` รันคำสั่งระบบในระบบปฏิบัติการ

### ส่งข้อความเดียว (ไม่ต้องรันโหมด interactive)

```powershell
echo "ช่วยอธิบาย Aetox คืออะไร" | aetox
```

### โหมดมาตรฐานแบบ CLI

```powershell
aetox help
aetox version
aetox --no-banner
aetox --version
```

### ตัวอย่าง provider

```powershell
$env:OPENROUTER_API_KEY = "YOUR_OPENROUTER_KEY"
aetox --model-provider=openrouter --model-name="google/gemma-3n-E2B" "ช่วยสรุปโปรเจกต์นี้"
```

```powershell
$env:OPENAI_API_KEY = "YOUR_OPENAI_KEY"
aetox --model-provider=openai --model-name="gpt-4o-mini" "ช่วยสรุปโปรเจกต์นี้"
```

```powershell
$env:GROQ_API_KEY = "YOUR_GROQ_KEY"
aetox --model-provider=groq --model-name="llama-3.3-70b-versatile" "ช่วยสรุปโปรเจกต์นี้"
```

```powershell
# Local Ollama (ต้องรัน ollama serve อยู่แล้ว)
aetox --model-provider=ollama --model-name="llama3.1:8b" "ช่วยสรุปโปรเจกต์นี้"
```

ถ้าการกำหนด provider ล้มเหลว ระบบจะ fallback ไป `noop` เพื่อให้แชตได้ทันที
