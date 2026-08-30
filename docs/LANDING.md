# โครงสร้างแลนดิ้งเพจ (docs/index.html)

เอกสารคู่กับ `docs/index.html` — ไฟล์เดียวจบ (HTML + CSS + JS ในไฟล์เดียว) ใช้เป็น GitHub Pages
ที่ `https://mikedev115.github.io/Aetox/` ภาษาไทยล้วน มีธีมมืด/สว่าง
เลขบรรทัดอ้างอิงจากไฟล์ ณ รอบที่เขียนเอกสารนี้ (1,683 บรรทัด) — ถ้าแก้ไขมาก ให้อัปเดตเลขบรรทัดด้วย

> หมายเหตุ: `promo/` เป็นโปรเจกต์ Svelte แยก ("โรงถ่าย") ไว้ถ่ายภาพ/วิดีโอโปรโม ไม่ใช่ตัวแลนดิ้งเพจ

## ผังไฟล์

| ช่วงบรรทัด | ส่วน | รายละเอียด |
|---|---|---|
| 1–42 | `<head>` | meta, SEO/OG/Twitter card (ภาษาไทย, og:image เป็น absolute URL), favicon SVG inline, ฟอนต์ IBM Plex Sans Thai จาก Google Fonts |
| 10–20 | theme script | อ่าน `localStorage["aetox-theme"]` ก่อน first paint กัน flash ขาว/ดำ |
| 43–~700 | `<style>` ทั้งหมด | CSS variables ธีมมืด (default) + `:root[data-theme="light"]`, spacing scale `--s1..--s6`, radius, shadow |
| 707–748 | SVG defs | wordmark + โลโก้ provider ต่าง ๆ (Simple Icons, CC0) ใช้ผ่าน `<use href="#...">` |
| 749–771 | `<header>` | lockup, เวอร์ชัน `v1.5.11 · Windows`, nav (งานจริง/ทำอะไรได้/ความปลอดภัย/ติดตั้ง/GitHub), ปุ่มสลับธีม |
| 773–774 | smooth scroll wrapper | `#smooth-wrapper` / `#smooth-content` สำหรับ GSAP ScrollSmoother |
| 778–800 | Hero | โลโก้ canvas + brand-ground SVG, H1 "Aetox", lede "ผู้ช่วย AI ในคอมพิวเตอร์ของคุณ", CTA ดาวน์โหลด installer.exe, คำสั่ง scoop + ปุ่ม copy |
| 805–811 | Showcase | ภาพ `assets/hero-app.png` คลิกขยายได้ (lightbox) + คำบรรยาย |
| 814–865 | **01 `#proof`** หลักฐาน | งานจริง CRM 20 เจ้า (15 ส.ค. 2026): stat cards (6:51 นาที / 42 เครื่องมือ / 20 แถว / 6 ช่องว่าง), timeline จำลองการทำงาน, ภาพ `assets/cap-parallel.png`, bnote เรื่องช่องว่างที่เว้นไว้ตั้งใจ |
| 868–942 | **02 `#fix`** ทำอะไรได้ | "สี่อย่างที่มันทำแทนคุณได้" |
| 945–976 | **03 `#doors`** สองประตู | เทียบรันในเครื่อง (ollama/lmstudio) กับใช้ provider (anthropic/openai/gemini/deepseek/kimi/minimax...) |
| 978–1063 | **04 `#light`** เบา | ขนาด/ทรัพยากรเทียบแบรนด์อื่น |
| 1065–1106 | **05 `#privacy`** ความปลอดภัย | ทำงานจบในเครื่อง ไม่มีเซิร์ฟเวอร์กลาง |
| 1108–1147 | **06 `#memory`** ความจำ | จำข้าม session |
| 1149–1203 | **07 `#install`** ติดตั้ง | เลือกโมเดล + วิธีติดตั้ง |
| 1205–1221 | **08 `#tools`** เครื่องมือ | รายการ tool ที่มี |
| 1223–1292 | **09 `#faq`** คำถาม | ก่อนโหลด |
| 1294–1306 | ปิดท้าย (contact) | CTA ดาวน์โหลดซ้ำ + อีเมลติดต่อ |
| 1311–1327 | `<footer>` | © 2026 Mikedev115, ลิงก์ GitHub/Architecture/LICENSE/Issues, bigmark |
| 1332–1335 | `<dialog #lightbox>` | ขยายรูป |
| 1337–~1360 | `<dialog #pvbox>` | รายชื่อ provider ทั้งหมด (เปิดจาก section สองประตู) |
| 1368–1370 | GSAP CDN | gsap + ScrollTrigger + ScrollSmoother (defer) |
| 1371–จบ | `<script>` หลัก | reveal animation, timeline, lightbox, copy button, theme toggle, logo canvas |

## ข้อตกลง/ธรรมเนียมของหน้านี้

- **ไฟล์เดียวจบ** — ไม่แตก CSS/JS ออกไฟล์แยก ยกเว้น GSAP ที่โหลดจาก CDN
- **สีทั้งหมดผ่าน CSS variables** ใน `:root` — แก้ธีมแก้ที่เดียว ธีมสว่างต้องผ่าน WCAG AA (4.5:1)
- **section มีเลขกำกับ 01–09** ใน `.sec-head .idx` — ถ้าแทรก/ลบ section ให้เลื่อนเลขให้เรียงกัน และอัปเดต nav ใน header ให้ตรง
- **รูปคลิกขยายได้** — ใช้ `<button class="shot" data-shot>` ครอบ `<img>` แล้ว JS เปิด lightbox
- **ภาษาไทยเป็นหลัก** — สำนวนพูดตรง ๆ ไม่ใช่ภาษาการตลาด ตัวเลขทุกตัวต้องตรวจซ้ำได้ (มาจากงานจริง/log จริง)
- **ลิงก์ดาวน์โหลด** ชี้ `releases/latest/download/aetox-amd64-installer.exe` — มี 2 จุด (hero + ปิดท้าย) แก้ให้พร้อมกัน
- **เวอร์ชันใน header** (`v1.5.11`) เขียนตรง ๆ — ต้องอัปเดตมือตอน release

## สินทรัพย์ที่หน้านี้ใช้

- `assets/hero-app.png` — ภาพหน้าโปรแกรมจริง (hero showcase)
- `assets/cap-parallel.png` — ภาพตัวแทนสี่คนทำงานพร้อมกัน (section 01)
- `assets/og.png` — ภาพ preview ตอนแชร์ลิงก์ (1200×630, อ้างเป็น absolute URL ใน meta)
- `assets/wordmark.svg` — โลโก้ตัวอักษร (ในหน้า inline เป็น path แทน)
