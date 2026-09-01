---
name: aetox-design
description: งานออกแบบภาพในแอตทอกซ์ - โลโก้ ไอคอน แบนเนอร์ ภาพโซเชียล และชุดอัตลักษณ์องค์กร (CIP) พร้อมแคตตาล็อกสไตล์ สีตามอุตสาหกรรม ขนาดมาตรฐานของทุกแพลตฟอร์ม และวิธีเขียนพรอมต์ภาพ อ่านตัวนี้ก่อนรับงานภาพเสมอ เพราะแอตทอกซ์ไม่ได้เจนรูปเอง มันหารูปจริงจากเน็ตมาใช้ หรือวาดเองด้วย SVG กับ HTML และสองทางนั้นเปลี่ยนวิธีรับงานตั้งแต่ประโยคแรก
source: https://github.com/claudekit (design)
license: MIT
copyright: Copyright (c) claudekit contributors
---

# Design work in Aetox

One fact decides everything below, so it goes first.

**This app has no image model.** Nothing here generates a picture from a
description, not a logo, not an icon, not a photo. Saying otherwise, or
quietly producing a prompt and calling it a deliverable, is the failure this
document exists to prevent.

What it has instead are two hands that are real:

- **Find a picture that already exists**, on the web, and put it where the work
  can use it.
- **Draw it itself.** SVG and HTML are text, and `write` writes text. A
  wordmark, an icon set, a banner layout, a chart, a card, those are things to
  author, not to wish for.

So "make me a logo" is answered by choosing one of three roads and saying which
one you took:

1. **Draw it**, an SVG wordmark or lettermark, written here, editable forever,
   and it prints at any size. This is the strongest answer for anything made of
   type and geometry, which is most marks.
2. **Find it**, for photography, backgrounds, textures, and mockup scenes,
   where a real photograph beats anything a description could produce.
3. **Spec it**, when the user has an image model somewhere else (their own
   account, a connected MCP server), the deliverable is a brief and a prompt
   sharp enough to spend money on. The style, colour and industry tables in
   this skill are exactly what makes that prompt good.

Never road 3 by default. It is the one that hands the work back to the user.

## Getting a picture, or a sound

Three tools, in this order. This recipe is the canonical one; other skills point
here rather than repeating it. It is the same recipe for music and sound
effects, from the same kind of source (Pixabay Audio, Freesound, Wikimedia
Commons carry sound the way Unsplash carries photos).

1. **`web_search`**, find the page, not the file. Name the source in the query
   when the licence matters: `unsplash dark server room`, `wikimedia commons
   <subject>`, `pixabay whoosh sound effect`.
2. **`web_fetch`** on that page, it reads the HTML and lists every image URL it
   found, with alt text. That listing is where the actual file URL comes from.
   Fetching an image URL directly is not useful: `web_fetch` reads bytes as
   text, so it returns garbage for a JPEG.
3. **`media_fetch`** with that file URL and a destination path. It downloads,
   checks the bytes really are an image or a sound (a saved HTML error page
   named `.jpg` is the ordinary way this goes wrong, and it refuses that with
   the page named), and reports the format, size, pixels or seconds in the
   same receipt — no shell, no separate verify step.

**Licence, every time.** A picture in a user's deck or brochure is a picture
they are publishing. Prefer sources that say "free to use" on the page
Unsplash, Pexels, Wikimedia Commons, a government or museum open collection
and tell the user where each one came from. Never present a picture found by
image search on an unknown page as cleared for use.

## Drawing it instead

`write` produces the file directly, and for these that is the whole job:

- **SVG**, logos, icons, badges, diagrams, illustrations built from shapes.
  Vector, so it scales, and it stays editable as text.
- **HTML + CSS**, banners, social cards, one-page layouts. To turn one into a
  picture, build it as a single-slide deck and use the slides room's export bar,
  which writes `.png`, `.jpg` and `.webp`. Read `aetox-slides` before writing
  that file: the room has an anatomy, and a file that ignores it will not page.

## คิดสองรอบก่อนวาด

ก่อนเขียน SVG หรือ HTML จริง ร่างแผนสั้นๆ ในหัวก่อน แล้วสอบแผนนั้นกับโจทย์อีกที, สองรอบนี้ถูก
กว่าการแก้ทีหลังเสมอ

**รอบแรก, ร่างแผน** สี่ส่วนสั้นๆ พอ:
- **สี**, 4-6 hex ที่ตั้งชื่อได้ว่าแต่ละสีทำหน้าที่อะไร ไม่ใช่แค่บรรยายอารมณ์
- **ตัวอักษร**, อย่างน้อย 2 บทบาท (ตัวเด่นสำหรับหัวเรื่อง ใช้แต่น้อย + ตัวอ่านสำหรับเนื้อหา) เพิ่มตัวที่ 3
  สำหรับตัวเลข/คำอธิบายถ้าจำเป็น
- **โครงร่าง**, แนวคิดเลย์เอาต์หนึ่งประโยค
- **จุดจำ**, หนึ่งองค์ประกอบที่อยากให้คนจำได้ว่าเป็นงานนี้

**รอบสอง, สอบแผนกับโจทย์** ถามทีละส่วนว่า "ถ้าเป็นโจทย์อื่นที่คล้ายกัน จะออกมาแบบนี้เหมือนกันไหม"
ถ้าใช่ แปลว่าส่วนนั้นเป็นค่าเริ่มต้นที่หยิบมาใช้เอง ไม่ใช่ทางเลือกที่มาจากโจทย์นี้จริงๆ แก้ก่อนลงมือ
แผนที่ผ่านรอบนี้แล้วให้ยึดตามนั้นทั้งหมดตอนวาด ไม่ตัดสินใจสีหรือฟอนต์ใหม่กลางทาง

สามแบบที่มักหยิบมาใช้เองโดยไม่ทันคิด, พื้นครีมอุ่นกับตัวอักษร serif กับสีเน้นดินเผา, พื้นเกือบดำกับ
สีเน้นเขียว/แดงสด, เส้นบางแบบหนังสือพิมพ์คอลัมน์แน่น, ไม่ได้ผิดในตัวเอง แค่ต้องมาจากการเลือกจริง
ไม่ใช่ปฏิกิริยาสะท้อน ถ้าโจทย์หรือผู้ใช้ระบุทิศทางมาแล้ว ทำตามนั้นตรงๆ แม้จะตรงกับแบบใดแบบหนึ่ง
ข้างบนพอดี คำสั่งที่ระบุมาชนะเสมอ

เช็กตัวเองรอบสุดท้ายแบบ Chanel: ก่อนเสร็จ ลองถอดหนึ่งองค์ประกอบออกดู ถ้างานยังสมบูรณ์โดยไม่มีมัน
องค์ประกอบนั้นไม่ควรอยู่ตั้งแต่แรก

*(แนวคิดจาก [anthropics/skills, frontend-design](https://github.com/anthropics/skills/tree/main/skills/frontend-design), Apache-2.0)*

## What is here

Knowledge, not commands. Open one with `skill_view` and a path.

| Question | File |
|---|---|
| Which mark style suits this brand | `references/logo-style-guide.md` |
| What a colour will say about them | `references/logo-color-psychology.md` |
| How to write an image prompt that is worth spending on | `references/logo-prompt-engineering.md` |
| What a corporate identity programme contains | `references/cip-deliverable-guide.md` |
| How those pieces should look together | `references/cip-style-guide.md` |
| Prompting for identity mockups | `references/cip-prompt-engineering.md` |
| Every banner size, per platform, and the styles that work | `references/banner-sizes-and-styles.md` |
| Sizes and craft for social photos | `references/social-photos-design.md` |

The tables are the catalogues, read whole rather than searched, each is small,
and reading it whole is how you see the row you would not have thought to look
for.

| Table | Rows about |
|---|---|
| `data/logo/styles.csv` | mark styles |
| `data/logo/colors.csv` | palettes and what they carry |
| `data/logo/industries.csv` | conventions per industry |
| `data/icon/styles.csv` | icon styles, usable directly when drawing SVG |
| `data/cip/deliverables.csv` | what a full identity programme ships |
| `data/cip/styles.csv` | identity styles |
| `data/cip/industries.csv` | industry conventions |
| `data/cip/mockup-contexts.csv` | scenes to show an identity in |

## Where the neighbouring work lives

- **Decks and presentations**, `aetox-slides`. It owns the anatomy of a deck
  this app can page through and export; this skill does not.
- **Tokens, component specs, and the tables that decide a slide's layout,
  typography and charts**, `aetox-design-system`.
- **Voice, messaging, logo usage rules, approval checklists**, `aetox-brand`.

One accent colour doing every job, three weights of light rather than pure
white on pure black, and the same furniture on every surface: that is the house
look, and it is a reference rather than a rule.
