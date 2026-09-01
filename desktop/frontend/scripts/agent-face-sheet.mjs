// Builds docs/agent-face.html — the standard for what an เอเจน looks like.
//
// GENERATED, never hand-edited, and that is the entire point. A reference page
// drawn by hand is a second drawing of the same thing, and the day somebody
// appends a haircut to agentFace.ts is the day the two stop agreeing. Here the
// page imports the real catalogue and the real theme tokens, so adding a part
// and re-running is the whole update.
//
//   npm run faces
//
// esbuild rather than a TS runner because it is already a vite dependency —
// this adds no package to the tree. The bundle is evaluated from memory; nothing
// is written except the page.
import { build } from 'esbuild'
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const frontend = resolve(here, '..')
const repo = resolve(frontend, '../..')

const bundled = await build({
  entryPoints: [resolve(frontend, 'src/lib/agentFace.ts')],
  bundle: true,
  format: 'esm',
  write: false,
  platform: 'neutral',
})
const mod = await import('data:text/javascript;base64,' + Buffer.from(bundled.outputFiles[0].text).toString('base64'))
const { HAIR, ACCESSORY, PROP, PROP_MIN_PX, resolveFace, faceSVG, palette } = mod

// The app's own tokens, inlined from source at build time rather than copied.
// Opening this page from anywhere still shows Aetox's colours, and changing a
// token in the app changes them here on the next run.
const tokens = ['src/styles/palette.css', 'src/styles/theme.css', 'src/styles/type.css']
  .map((p) => readFileSync(resolve(frontend, p), 'utf8'))
  .join('\n')

const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

function tile(name, size, o = {}) {
  const f = resolveFace(name, size, o)
  return `<span class="agent-face" style="--h:${f.hue}; width:${size}px; height:${size}px"><svg viewBox="0 0 64 64">${faceSVG(f)}</svg></span>`
}

// The seven that ship, read from their own AGENT.md so this page cannot claim a
// roster the app does not have.
const BUNDLED = [
  ['research', 'search', 'Nova', 'นักวิจัยข้อมูล', 'ไล่หลายแหล่งทั้งทางการและคอมมูนิตี้ เทียบคู่แข่ง แล้วส่งข้อค้นพบที่ตามกลับไปหาต้นทางได้'],
  ['doc', 'fileText', 'Meridian', 'ผู้ดูแลเอกสาร', 'ตอบว่าเอกสารแบบไหนต้องมีอะไร ตรวจร่างที่มีอยู่ และร่างขึ้นใหม่เมื่อถึงเวลา'],
  ['video', 'clapperboard', 'Kino', 'ช่างทำวิดีโอ', 'ออกแบบฉากขึ้นใหม่เป็น HTML แล้วเรนเดอร์ออกมาเป็นคลิป จากเนื้อหาที่ผู้ใช้มี'],
  ['sheet', 'chartColumn', 'Tally', 'ผู้ดูแลตัวเลข', 'รวบรวม จัดระเบียบ ตั้งสูตร ตรวจความถูกต้อง'],
  ['github', 'gitBranch', 'Hub', 'ผู้ดูแลรีโป', 'เปิดและรีวิว PR ไล่ CI ที่แดง จัดการ issue ตั้งรีโปให้ได้มาตรฐาน'],
  ['automation', 'zap', 'Cog', 'ช่างออโตเมชั่น', 'ออกแบบ ต่อโหนด และแก้ workflow บนเครื่องมืออัตโนมัติที่ผู้ใช้เชื่อมไว้'],
  ['editor', 'slidersHorizontal', 'Reel', 'ช่างตัดต่อ', 'ดูฟุตเทจที่มีอยู่ ตัด ต่อ ใส่ซับ จัดเสียง แล้วส่งออกเป็นคลิปที่เอาไปใช้ได้จริง'],
]

const USER_WRITTEN = ['ผู้ตรวจสัญญา', 'ผู้ช่วยขาย', 'บัญชี', 'legal-reviewer', 'qa', 'devops']

const cast = BUNDLED.map(
  ([id, icon, display, role]) =>
    `<div class="one">${tile(id, 88, { icon })}<div class="one-n">${esc(display)}</div><div class="one-r">${esc(role)}</div><div class="one-i">${esc(id)}</div></div>`
).join('')

const mine = USER_WRITTEN.map(
  (n) => `<div class="one">${tile(n, 76)}<div class="one-n">${esc(n)}</div><div class="one-i">hue ${resolveFace(n, 76).hue}</div></div>`
).join('')

const card = ([id, icon, display, role, desc]) => `
      <div class="chair-card agc">
        <div class="chair-body">
          <div class="chair-who">
            ${tile(id, 48, { icon })}
            <span class="chair-who-t"><span class="chair-name">${esc(display)}</span><span class="chair-role">${esc(role)}</span></span>
          </div>
          <p class="chair-desc">${esc(desc)}</p>
          <div class="chair-chips"><span class="chip">${esc(id)}</span></div>
          <div class="chair-foot"><span class="chair-talk">คุยกับ ${esc(display)}</span><span class="chair-gear">⚙</span></div>
        </div>
      </div>`

const sizes = [88, 48, 38, 24]
  .map(
    (s) =>
      `<div class="size-row"><span class="size-n">${s}px</span>${BUNDLED.map(([id, icon]) => tile(id, s, { icon })).join('')}</div>`
  )
  .join('')

// Every part in the catalogue, drawn on the same neutral hue so the shapes are
// what differ. A part appended to agentFace.ts appears here on the next run
// without this file being touched.
const NEUTRAL = 210
const hairSheet = HAIR.map((h) => {
  const f = resolveFace('x', 88, { hue: NEUTRAL, hair: h.id, accessory: 'none' })
  return `<div class="part"><span class="agent-face" style="--h:${NEUTRAL}; width:64px; height:64px"><svg viewBox="0 0 64 64">${faceSVG(f)}</svg></span><div class="part-l">${esc(h.label)}</div><code>${esc(h.id)}</code></div>`
}).join('')

const accSheet = ACCESSORY.map((a) => {
  const f = resolveFace('x', 88, { hue: NEUTRAL, hair: 'neat', accessory: a.id })
  return `<div class="part"><span class="agent-face" style="--h:${NEUTRAL}; width:64px; height:64px"><svg viewBox="0 0 64 64">${faceSVG(f)}</svg></span><div class="part-l">${esc(a.label)}</div><code>${esc(a.id)}</code></div>`
}).join('')

const propSheet = Object.keys(PROP)
  .map((k) => {
    const f = resolveFace('x', 88, { hue: NEUTRAL, hair: 'neat', accessory: 'none', icon: k })
    return `<div class="part"><span class="agent-face" style="--h:${NEUTRAL}; width:64px; height:64px"><svg viewBox="0 0 64 64">${faceSVG(f)}</svg></span><code>${esc(k)}</code></div>`
  })
  .join('')

const swatch = Object.entries(palette(NEUTRAL))
  .map(([k, v]) => `<div class="sw"><span style="background:${v}"></span><code>${esc(k)}</code></div>`)
  .join('')

const html = `<!doctype html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>หน้าเอเจน — มาตรฐาน</title>
<style>
${tokens}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--surface-app); color: var(--text-primary);
  font-family: var(--sans); font-size: var(--fs-lg); line-height: 1.5;
  padding: 34px 30px 72px;
}
h1 { font-size: var(--fs-xl); font-weight: 600; margin: 0 0 5px; }
.lede { color: var(--text-muted); font-size: var(--fs-sm); margin: 0 0 8px; max-width: 76ch; }
.warn { color: var(--text-dim); font-size: var(--fs-2xs); margin: 0 0 30px; }
.warn code { color: var(--accent); }
section { margin: 0 0 40px; max-width: 900px; }
h2 { font-size: var(--fs-sm); font-weight: 600; margin: 0 0 4px; }
.note { font-size: var(--fs-sm); color: var(--text-muted); margin: 0 0 16px; max-width: 76ch; }
hr { height: 1px; background: var(--border-subtle); border: 0; margin: 0 0 26px; max-width: 900px; }
code { font-family: var(--mono); font-size: var(--fs-2xs); color: var(--text-muted); }

/* The face itself, mirroring AgentFace.svelte's own block. It is the one piece
   of style here that is a copy rather than an import, because it lives inside a
   Svelte component; if that block changes, change this one. */
.agent-face {
  display: block; flex: none; overflow: hidden; border-radius: 26%;
  background: linear-gradient(158deg, hsl(var(--h) 44% 26%), hsl(var(--h) 40% 17%));
  box-shadow: inset 0 0 0 1px hsl(var(--h) 40% 38% / .55), inset 0 1px 0 hsl(var(--h) 60% 62% / .18);
}
.agent-face svg { display: block; width: 100%; height: 100%; }

.gal { display: flex; flex-wrap: wrap; gap: 20px; }
.one { width: 118px; text-align: center; }
.one .agent-face { margin: 0 auto 9px; }
.one-n { font-size: var(--fs-sm); color: var(--text-primary); }
.one-r { font-size: var(--fs-2xs); color: var(--text-secondary); }
.one-i { font-family: var(--mono); font-size: var(--fs-2xs); color: var(--text-dim); }

/* Mirrors style.css .chair-card.agc — the card the roster actually draws. */
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 12px; }
.chair-card.agc {
  border: none; border-radius: var(--r-md); background: var(--surface-raised);
  display: flex; flex-direction: column; gap: 8px; padding: 12px;
}
.agc .chair-body { flex: 1; display: flex; flex-direction: column; gap: 7px; }
.agc .chair-who { display: flex; align-items: center; gap: 10px; }
.chair-who-t { display: flex; flex-direction: column; }
.agc .chair-name { font-size: var(--fs-lg); letter-spacing: -.01em; font-weight: 500; }
.agc .chair-role { font-size: var(--fs-2xs); color: var(--text-dim); }
.agc .chair-desc {
  font-size: var(--fs-sm); color: var(--text-secondary); margin: 0;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
.agc .chair-chips { display: flex; gap: 5px; }
.agc .chip {
  font-size: var(--fs-2xs); line-height: 18px; padding: 0 8px; border-radius: 999px;
  background: var(--surface-sunken); color: var(--text-muted);
}
.agc .chair-foot { display: flex; gap: 8px; margin-top: auto; }
.agc .chair-talk {
  flex: 1; display: inline-flex; align-items: center; justify-content: center;
  border-radius: var(--r-sm); padding: 6px 10px;
  background: var(--surface-sunken); color: var(--text-secondary); font-size: var(--fs-sm);
}
.agc .chair-gear {
  width: 30px; display: grid; place-items: center; border-radius: var(--r-sm);
  background: var(--surface-sunken); color: var(--text-muted);
}

.size-row { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; flex-wrap: wrap; }
.size-n { font-family: var(--mono); font-size: var(--fs-2xs); color: var(--text-dim); width: 42px; flex: none; }
.parts { display: flex; flex-wrap: wrap; gap: 14px; }
.part { width: 84px; text-align: center; }
.part .agent-face { margin: 0 auto 6px; }
.part-l { font-size: var(--fs-2xs); color: var(--text-secondary); }
.sw { display: inline-flex; align-items: center; gap: 7px; margin: 0 14px 8px 0; }
.sw span { width: 22px; height: 22px; border-radius: var(--r-sm); display: block; }
pre {
  background: var(--surface-sunken); border-radius: var(--r-md); padding: 12px 14px;
  font-family: var(--mono); font-size: var(--fs-sm); color: var(--text-secondary);
  overflow-x: auto; margin: 0;
}
</style>
</head>
<body>

<h1>หน้าเอเจน — มาตรฐาน</h1>
<p class="lede">เอเจนหนึ่งคนมีหน้าเดียว และหน้านั้นถูกวาดจากชื่อ ไม่ใช่เก็บเป็นไฟล์รูป เอเจนที่ผู้ใช้เขียนเองจึงมีหน้าตั้งแต่วินาทีที่วางไฟล์ โดยไม่มีใครต้องหารูปให้</p>
<p class="warn">หน้านี้ถูกสร้างจาก <code>src/lib/agentFace.ts</code> กับโทเคนจริงใน <code>src/styles/</code> ห้ามแก้มือ เพิ่มชิ้นส่วนแล้วสั่ง <code>npm run faces</code></p>

<section>
  <h2>คณะที่มากับแอป</h2>
  <p class="note">ชื่อที่แสดงกับบทบาทในหน้านี้เป็นตัวอย่าง ยังไม่ได้ลงไฟล์จริง ส่วน id ใต้ชื่อคือชื่อโฟลเดอร์ ซึ่งเป็นสิ่งที่ task เมนู @ ไฟล์ความจำ และประวัติงาน ผูกอยู่ และห้ามเปลี่ยน</p>
  <div class="gal">${cast}</div>
</section>
<hr>

<section>
  <h2>การ์ดบนหน้าเอเจน</h2>
  <p class="note">มาร์ก 48px ชื่อ บทบาท ประโยคเดียวว่าทำอะไร ชิปเฉพาะสิ่งที่ต่างจากคนอื่น แล้วปุ่มคุย ป้ายที่ทุกใบใส่คือป้ายที่ไม่ได้บอกอะไร</p>
  <div class="grid">${BUNDLED.slice(0, 3).map(card).join('')}</div>
</section>
<hr>

<section>
  <h2>ทุกขนาดที่ใช้จริง</h2>
  <p class="note">ต่ำกว่า ${PROP_MIN_PX}px ของที่ถืออยู่ในมือถูกตัดออกเอง เพราะมันเหลือไม่กี่พิกเซลแล้วไปแย่งความสนใจกับหัว แถวล่างสุดคือเมนู @</p>
  ${sizes}
</section>
<hr>

<section>
  <h2>ตู้เสื้อผ้า · ทรงผม</h2>
  <p class="note">ทรงผมคือสิ่งที่แยกคนออกจากกันตอนย่อเล็ก ไม่ใช่ใบหน้า ทุกทรงวาดบนสีกลางสีเดียวกันในนี้ เพื่อให้เห็นว่าต่างกันที่รูปทรงจริง ๆ ${HAIR.length} ทรง</p>
  <div class="parts">${hairSheet}</div>
</section>
<hr>

<section>
  <h2>ตู้เสื้อผ้า · ของบนหน้า</h2>
  <p class="note">มีสองช่องที่เป็น "ไม่มี" โดยตั้งใจ ครึ่งหนึ่งของคณะที่ไม่ใส่อะไรเลย คือสิ่งที่ทำให้แว่นมีความหมาย</p>
  <div class="parts">${accSheet}</div>
</section>
<hr>

<section>
  <h2>ของที่ถือ</h2>
  <p class="note">มาจาก <code>icon:</code> ที่โปรไฟล์ประกาศอยู่แล้ว ไม่มีอะไรใหม่ที่คนเขียนไฟล์ต้องกรอก และ icon ที่ไม่มีในรายการนี้แปลว่าไม่ถืออะไร ไม่ใช่ข้อผิดพลาด</p>
  <div class="parts">${propSheet}</div>
</section>
<hr>

<section>
  <h2>สีที่ทุกชิ้นส่วนวาดจาก</h2>
  <p class="note">ชิ้นส่วนใหม่เลือกสีเองไม่ได้ มันได้แค่ชุดนี้ ซึ่งคำนวณจากสีประจำตัวของเอเจน ตู้เสื้อผ้าจึงโตได้โดยที่คณะไม่มีวันหลุดโทน</p>
  <div>${swatch}</div>
</section>
<hr>

<section>
  <h2>เอเจนที่ผู้ใช้เขียนเอง</h2>
  <p class="note">ไทยหรืออังกฤษก็ได้ ไม่มีใครตั้งค่าอะไรเลย ทุกคนได้หน้าทันทีและได้หน้าเดิมทุกครั้งที่เปิดแอป</p>
  <div class="gal">${mine}</div>
</section>
<hr>

<section>
  <h2>ถ้าโปรไฟล์อยากเลือกเอง</h2>
  <p class="note">ฝั่งหน้าจอรองรับแล้ว ยังไม่ได้ต่อถึงไฟล์ ทุกช่องเป็นตัวเลือก เขียนผิดตกกลับไปที่หน้าที่คิดจากชื่อ ไม่ใช่ช่องว่าง และชิ้นส่วนถูกอ้างด้วยชื่อ ไม่ใช่ลำดับ ดังนั้นตู้เสื้อผ้าโตได้โดยไม่มีใครเปลี่ยนหน้า</p>
  <pre>---
description: เอเจนหาข้อมูลเชิงลึก …
icon: search
hue: 196
hair: beanie
accessory: glasses
---</pre>
</section>

</body>
</html>
`

const out = resolve(repo, 'docs/agent-face.html')
writeFileSync(out, html)
console.log('wrote', out, '·', HAIR.length, 'hair ·', ACCESSORY.length, 'accessories ·', Object.keys(PROP).length, 'props')
