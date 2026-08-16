// Temporary: renders through the real pipeline and writes the result to disk so
// it can be looked at. Deleted after the owner has seen it.
import { it } from 'vitest'
import { writeFileSync } from 'node:fs'
import { renderMarkdown } from '../lib/markdown'

const CASES: [string, string][] = [
  [
    'จากภาพในสกรีนช็อต — ที่เคยพัง',
    `บริเวณสีจางใต้กราฟคือพื้นที่ที่เราต้องการหา\n\n\\[\n\\int_0^2 x^2\\,dx\n\\]\n\nผลลัพธ์คือ\n\n\\[\n\\frac{8}{3}\n\\]\n\nจำจากภาพได้ว่า:\n\n\\[\n\\boxed{\\text{อนุพันธ์ = ดูความชัน}}\n\\]\n\n\\[\n\\boxed{\\text{ปริพันธ์ = รวมพื้นที่หรือปริมาณสะสม}}\n\\]`,
  ],
  [
    'สมการในประโยค (\\( \\) และ $ $)',
    'ให้ \\(f(x) = x^2\\) แล้วความชันที่จุด $x = 3$ คือ \\(f\'(3) = 6\\) ส่วนพจน์แรกของลำดับคือ $a_1$',
  ],
  ['$$ ทั้งสองแบบ', 'ค่าเฉลี่ยคือ\n\n$$\\bar{x} = \\frac{1}{n}\\sum_{i=1}^{n} x_i$$'],
  [
    'รากที่สอง (KaTeX วาดด้วย SVG) — ต้องไม่มีปุ่มคัดลอก/บันทึกงอกมา',
    'สูตรกำลังสอง\n\n\\[\nx = \\frac{-b \\pm \\sqrt{b^2 - 4ac}}{2a}\n\\]',
  ],
  [
    'เมทริกซ์ ลิมิต และเศษส่วนซ้อน',
    '\\[\n\\lim_{h \\to 0} \\frac{f(x+h) - f(x)}{h}\n\\qquad\n\\begin{pmatrix} a & b \\\\ c & d \\end{pmatrix}\n\\]',
  ],
  [
    'สมการยาวมาก — ต้องเลื่อนแนวนอนในกล่องตัวเอง ไม่ดันหน้าออกข้าง',
    '\\[\n\\int_0^\\infty \\frac{x^3}{e^x - 1}\\,dx = \\frac{\\pi^4}{15} \\quad\\text{และ}\\quad \\sum_{n=1}^{\\infty} \\frac{1}{n^2} = \\frac{\\pi^2}{6} \\quad\\text{และ}\\quad \\prod_{p} \\frac{1}{1 - p^{-s}} = \\zeta(s) \\quad\\text{และ}\\quad e^{i\\pi} + 1 = 0',
  ],
  ['เงิน — ต้องไม่ถูกอ่านเป็นสมการ', 'แพ็กเกจเริ่มที่ $5 ต่อเดือน ตัวเต็ม $20 ช่วงลดเหลือ $12-$15 ครับ'],
  ['LaTeX ในบล็อกโค้ด — ต้องโชว์เป็นซอร์ส', '```latex\n\\[ \\int_0^2 x^2\\,dx \\]\n```'],
  ['LaTeX ในโค้ดบรรทัดเดียว — ต้องโชว์เป็นซอร์ส', 'เขียนว่า `$x^2$` หรือ `\\(x^2\\)` ก็ได้'],
  ['คำสั่งที่ KaTeX ไม่รู้จัก — ต้องคืนต้นฉบับ ไม่ใช่ error แดง', '\\[ \\thisIsNotACommand{x} + 1 \\]'],
  [
    'รูปวาดจริงข้าง ๆ สมการ — ต้องยังได้กรอบและปุ่มของมัน',
    '\\[\\sqrt{x}\\]\n\n<svg viewBox="0 0 200 60" width="100%"><rect x="2" y="10" width="70" height="34" rx="4" fill="none" stroke="currentColor" stroke-width="2"/><text x="14" y="33" font-size="13" fill="currentColor">อนุพันธ์</text><path d="M78 27 h36" stroke="currentColor" stroke-width="2"/><path d="M108 22 l8 5 -8 5" fill="currentColor"/><rect x="122" y="10" width="74" height="34" rx="4" fill="none" stroke="currentColor" stroke-width="2"/><text x="132" y="33" font-size="13" fill="currentColor">ความชัน</text></svg>',
  ],
]

it('writes a preview of the real render', () => {
  const sections = CASES.map(
    ([title, source]) =>
      `<section><h2>${title}</h2>` +
      `<div class="src"><pre>${source.replace(/&/g, '&amp;').replace(/</g, '&lt;')}</pre></div>` +
      `<div class="markdown-body out">${renderMarkdown(source)}</div>` +
      `</section>`
  ).join('\n')
  writeFileSync('dist/math-preview.body.html', sections, 'utf8')
})
