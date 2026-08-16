// An equation in an answer, from the outside.
//
// A model asked about an integral writes LaTeX. Before 16 ส.ค. nothing on this
// surface knew that, so markdown read the equation as prose and printed what
// was left of it: `\[` is a backslash-escaped bracket, so a display equation
// arrived as a bare `[` on its own line; `x^2` and `a_1` handed their `^` and
// `_` to the emphasis parser; and the rest came out as source. The owner's
// screenshot of `\int_0^2 x^2\,dx` sitting between two orphaned brackets is the
// case this file exists for.

import { describe, it, expect } from 'vitest'
import { renderMarkdown, renderStreamingMarkdown } from '../lib/markdown'

// KaTeX's own markup is what "drawn" means here: the wrapper class, and the
// symbol itself present as a character rather than as its LaTeX name.
const drawn = (html: string) => html.includes('class="katex')

// What the reader actually sees, which is not the same as what is in the HTML.
// The LaTeX legitimately appears twice off-screen — on data-tex, which is what
// the คัดลอก button hands to the clipboard, and it must never appear as text.
// `.katex-mathml` is dropped for the same reason: KaTeX hides it in a 1px clip
// for screen readers, so counting it as visible would pass a test on markup
// nobody can read.
function seen(html: string): string {
  const host = document.createElement('div')
  host.innerHTML = html
  for (const hidden of host.querySelectorAll('.katex-mathml')) hidden.remove()
  return host.textContent ?? ''
}

describe('an equation in an answer', () => {
  it('draws display maths written with \\[ \\]', () => {
    const out = renderMarkdown('พื้นที่ใต้กราฟคือ\n\n\\[\n\\int_0^2 x^2\\,dx\n\\]')

    expect(drawn(out)).toBe(true)
    expect(out).toContain('katex-display')
    expect(out).toContain('∫')
    // The two failures from the screenshot, named directly.
    expect(seen(out)).not.toContain('\\int')
    expect(out).not.toMatch(/<p>\s*\[\s*<\/p>/)
  })

  it('draws display maths written with $$', () => {
    const out = renderMarkdown('ผลลัพธ์คือ\n\n$$\\frac{8}{3}$$')

    expect(drawn(out)).toBe(true)
    expect(out).toContain('katex-display')
    expect(seen(out)).not.toContain('\\frac')
  })

  it('draws inline maths inside a sentence', () => {
    const out = renderMarkdown('ให้ \\(x^2\\) เป็นตัวแปร และ $a_1$ เป็นพจน์แรก')

    expect(drawn(out)).toBe(true)
    expect(out).not.toContain('\\(')
    expect(seen(out)).not.toContain('a_1')
    // The emphasis parser is what used to eat these.
    expect(out).not.toContain('<em>')
  })

  // A display equation written mid-sentence never reaches the block tokenizer,
  // because a block tokenizer is only offered the start of a block.
  it('draws display maths written inside a paragraph', () => {
    const out = renderMarkdown('สรุปว่า \\[E = mc^2\\] ตามที่ว่ามา')

    expect(drawn(out)).toBe(true)
    expect(seen(out)).not.toContain('mc^2')
  })

  // Thai inside \text{} is most of what the equations in this app say, and
  // KaTeX's strict mode refuses it outright.
  it('keeps Thai inside \\text{} and \\boxed{}', () => {
    const out = renderMarkdown('\\[\\boxed{\\text{อนุพันธ์ = ดูความชัน}}\\]')

    expect(drawn(out)).toBe(true)
    expect(out).toContain('อนุพันธ์')
    expect(seen(out)).not.toContain('\\boxed')
  })

  // The คัดลอก button on a display equation hands over the LaTeX the model
  // wrote, not KaTeX's layout text — that flattens every fraction onto one line
  // and has to be retyped wherever it is pasted.
  it('carries its own source for the คัดลอก button', () => {
    const out = renderMarkdown('\\[\n\\frac{8}{3}\n\\]')

    expect(out).toContain('math-copy')
    expect(out).toContain('data-tex="\\frac{8}{3}"')
  })

  // A control bigger than the `x` it hangs off is noise in the middle of a
  // sentence, so only display equations get one.
  it('does not put a button on inline maths', () => {
    const out = renderMarkdown('ให้ \\(x^2\\) เป็นตัวแปร')

    expect(drawn(out)).toBe(true)
    expect(out).not.toContain('math-copy')
  })
})

describe('what is not an equation', () => {
  // The reason a bare `$` needs guarding at all: it is also how money is
  // written, and a price list is not an equation named "5 และ ".
  it('leaves prices alone', () => {
    const out = renderMarkdown('ราคา $5 และ $10 ครับ')

    expect(drawn(out)).toBe(false)
    expect(out).toContain('$5')
    expect(out).toContain('$10')
  })

  it('leaves a price range alone', () => {
    const out = renderMarkdown('ประมาณ $5-$10 ต่อเดือน')

    expect(drawn(out)).toBe(false)
    expect(out).toContain('$5-$10')
  })

  // Inside a fence the LaTeX is the thing being shown, not the thing being said.
  it('leaves maths inside a code block as source', () => {
    const out = renderMarkdown('```latex\n\\[ x^2 \\]\n```')

    expect(drawn(out)).toBe(false)
    expect(out).toContain('x^2')
  })

  it('leaves maths inside inline code as source', () => {
    const out = renderMarkdown('เขียนว่า `$x^2$` ก็ได้')

    expect(drawn(out)).toBe(false)
    expect(out).toContain('<code>')
  })

  // KaTeX refuses what it cannot read. The reader is better served by the
  // source than by a red error about it — and mid-stream, half an equation is
  // unreadable for a few frames on its way to being correct.
  it('gives back the source when KaTeX refuses it', () => {
    const out = renderMarkdown('\\[ \\thisIsNotACommand{x} \\]')

    expect(drawn(out)).toBe(false)
    expect(out).toContain('thisIsNotACommand')
  })
})

describe('an equation arriving', () => {
  it('does not flash an error while the closing delimiter is still missing', () => {
    const out = renderStreamingMarkdown('พื้นที่คือ\n\n\\[\n\\int_0^2 x^')

    expect(out).not.toContain('katex-error')
  })

  it('is drawn as soon as it closes', () => {
    const out = renderStreamingMarkdown('พื้นที่คือ\n\n\\[\n\\int_0^2 x^2\\,dx\n\\]')

    expect(drawn(out)).toBe(true)
  })
})

// A square root's overbar, a stretched brace and an arrow over a vector are all
// <svg> inside the equation — parts of a letter, not pictures. confine() frames
// a drawing with คัดลอก and บันทึก buttons, and hanging those off a radical sign
// would also renumber the real drawings around it.
describe('KaTeX draws in SVG too', () => {
  it('does not frame a square root as a drawing', () => {
    const out = renderMarkdown('\\[\\sqrt{x}\\]')

    expect(drawn(out)).toBe(true)
    expect(out).not.toContain('drawing-box')
    expect(out).not.toContain('drawing-copy')
  })

  it('leaves a real drawing beside an equation framed', () => {
    const out = renderMarkdown(
      '\\[\\sqrt{x}\\]\n\n<svg viewBox="0 0 10 10" width="100%"><rect width="5" height="5" /></svg>'
    )

    expect(out).toContain('drawing-box')
    expect(out.match(/drawing-box/g)?.length).toBe(1)
  })
})
