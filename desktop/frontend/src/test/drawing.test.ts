// A drawing in an answer, from the outside.
//
// internal/prompt's `drawing` layer asks the model for inline <svg> when the
// thing being explained is how several parts relate. That only works because
// this renderer already lets SVG through — DOMPurify passes it and strips the
// dangerous parts. Nothing here *enables* that; what these tests pin is that
// nothing may quietly take it away, because the failure would be silent: a
// model doing exactly what it was told, and answers that render blank.

import { describe, it, expect } from 'vitest'
import { renderMarkdown, renderStreamingMarkdown } from '../lib/markdown'

const svg = '<svg viewBox="0 0 100 40" width="100%"><rect width="60" height="20" fill="currentColor" /><text x="4" y="34" font-size="9">โต๊ะ</text></svg>'

describe('a drawing in an answer', () => {
  it('renders, shapes and text intact', () => {
    const out = renderMarkdown(`นี่คือผัง\n\n${svg}`)

    expect(out).toContain('<svg')
    expect(out).toContain('<rect')
    expect(out).toContain('currentColor')
    expect(out).toContain('โต๊ะ')
  })

  // The reason a drawing is safe to render at all. An answer is model output,
  // and the model reaches the machine through tools that ask first — a script
  // that runs by being *displayed* would be a way around every one of them.
  it('strips scripts and handlers out of one', () => {
    const out = renderMarkdown(
      '<svg><script>fetch("http://evil")</script><rect onclick="alert(1)" width="5" height="5" /></svg>'
    )

    expect(out).toContain('<rect')
    expect(out).not.toContain('<script')
    expect(out).not.toContain('onclick')
    expect(out).not.toContain('evil')
  })
})

describe('a drawing arriving one token at a time', () => {
  // The picture builds itself: shapes are drawn as they arrive rather than
  // held back to the end. What keeps that watchable is the viewBox in the
  // opening tag — the box has its final size before a single shape is in it,
  // so nothing shoves the reply down the page as the rest lands.
  it('draws the shapes that have arrived, inside a closed tag', () => {
    const partial = 'นี่คือผัง\n\n<svg viewBox="0 0 100 40" width="100%"><rect width="60" height="20" />'

    const out = renderStreamingMarkdown(partial)

    expect(out).toContain('นี่คือผัง')
    expect(out).toContain('viewBox="0 0 100 40"')
    expect(out).toContain('<rect')
    expect(out).toContain('</svg>')
  })

  // A half-written element must not reach the parser: `<rect width="60` builds
  // an attribute out of whatever follows it, closing tag included.
  it('drops the element still being written', () => {
    const out = renderStreamingMarkdown('<svg viewBox="0 0 100 40"><rect width="60" height="20" /><circle cx="4')

    expect(out).toContain('<rect')
    expect(out).not.toContain('<circle')
    expect(out).toContain('</svg>')
  })

  // Until the opening tag closes there is no viewBox, and an unsized drawing
  // is exactly the jumping this is here to avoid. It lasts a few tokens.
  it('waits for the opening tag before drawing anything', () => {
    const out = renderStreamingMarkdown('นี่คือผัง\n\n<svg viewBox="0 0 100')

    expect(out).toContain('นี่คือผัง')
    expect(out).not.toContain('<svg')
  })

  // Growing markup must not change what has already been drawn — only add to
  // it. A shape that moves between frames is the flicker, arriving late.
  it('keeps earlier shapes put as later ones arrive', () => {
    const head = '<svg viewBox="0 0 100 40" width="100%"><rect width="60" height="20" />'
    const early = renderStreamingMarkdown(head)
    const later = renderStreamingMarkdown(`${head}<circle cx="40" cy="10" r="5" />`)

    const drawnSoFar = early.slice(0, early.indexOf('</svg>'))
    expect(drawnSoFar).toContain('<rect width="60" height="20">')
    expect(later.startsWith(drawnSoFar)).toBe(true)
  })

  // Streaming does not get its own weaker sanitizer: the same guard runs on a
  // half-arrived drawing as on a finished one.
  it('strips handlers out of a drawing that is still arriving', () => {
    const out = renderStreamingMarkdown('<svg viewBox="0 0 10 10" onload="alert(1)"><rect width="5" height="5" />')

    expect(out).not.toContain('onload')
    expect(out).not.toContain('alert')
    expect(out).toContain('<rect')
  })

  it('draws it the moment the closing tag lands', () => {
    const out = renderStreamingMarkdown(`นี่คือผัง\n\n${svg}`)

    expect(out).toContain('<svg')
    expect(out).toContain('<rect')
  })

  // Text after a finished drawing must not be swallowed by the guard: the
  // scan looks at the last opening tag, and that one is closed.
  it('keeps writing after a drawing is done', () => {
    const out = renderStreamingMarkdown(`${svg}\n\nสรุปคือแยกกันอยู่แล้ว`)

    expect(out).toContain('<svg')
    expect(out).toContain('สรุปคือแยกกันอยู่แล้ว')
  })

  // An answer that merely mentions the string in prose or in a fenced block is
  // not a drawing, and must not be truncated at the word.
  it('leaves ordinary text that says svg alone', () => {
    const out = renderStreamingMarkdown('รูปแบบไฟล์ที่ใช้คือ svg ครับ')

    expect(out).toContain('รูปแบบไฟล์ที่ใช้คือ svg ครับ')
  })
})
