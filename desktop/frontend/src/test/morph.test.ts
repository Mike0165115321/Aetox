// A streaming reply is updated, not rebuilt.
//
// Everything here is one property said different ways: an element that is still
// the same element in the next frame must be the SAME DOM NODE. Assigning
// innerHTML breaks that on every token, and what breaks with it is invisible to
// a test that only reads markup — a CSS animation restarting from zero, a
// selection dropped mid-drag. So these compare node identity, not HTML.
import { describe, it, expect } from 'vitest'
import { morphInto } from '../lib/morph'
import { renderStreamingMarkdown } from '../lib/markdown'

const host = (html: string) => {
  const el = document.createElement('div')
  el.innerHTML = html
  return el
}

describe('morphing a growing answer', () => {
  it('keeps the nodes that did not change when text is appended', () => {
    const el = host('<p>หนึ่ง</p><p>สอง</p>')
    const first = el.children[0]
    const second = el.children[1]

    morphInto(el, '<p>หนึ่ง</p><p>สอง</p><p>สาม</p>')

    expect(el.children[0]).toBe(first)
    expect(el.children[1]).toBe(second)
    expect(el.children).toHaveLength(3)
  })

  it('updates the text of the block still being written, in place', () => {
    const el = host('<p>กำลังเขียน</p>')
    const p = el.children[0]

    morphInto(el, '<p>กำลังเขียนอยู่</p>')

    expect(el.children[0]).toBe(p)
    expect(p.textContent).toBe('กำลังเขียนอยู่')
  })

  it('moves a class without rebuilding either block', () => {
    const el = host('<div class="codeblock live">a</div>')
    const block = el.children[0]

    morphInto(el, '<div class="codeblock">a</div><div class="codeblock live">b</div>')

    expect(el.children[0]).toBe(block)
    expect(block.className).toBe('codeblock')
    expect(el.children[1].className).toBe('codeblock live')
  })

  it('replaces a node that became a different kind of thing', () => {
    const el = host('<p>- หนึ่ง</p>')
    morphInto(el, '<ul><li>หนึ่ง</li></ul>')

    expect(el.children[0].tagName).toBe('UL')
    expect(el.children).toHaveLength(1)
  })

  it('drops what the new render no longer has', () => {
    const el = host('<p>a</p><p>b</p><p>c</p>')
    morphInto(el, '<p>a</p>')

    expect(el.children).toHaveLength(1)
    expect(el.textContent).toBe('a')
  })

  it('removes an attribute the new render dropped', () => {
    const el = host('<div class="codeblock" data-x="1">a</div>')
    morphInto(el, '<div class="codeblock">a</div>')

    expect(el.children[0].hasAttribute('data-x')).toBe(false)
  })
})

// The two halves have to hold together: morphing keeps the <style> element,
// and the drawing's scope keeps its text identical, so the animation the model
// declared is never re-declared and never restarts.
describe('an animated drawing arriving token by token', () => {
  const opening = '<svg viewBox="0 0 40 40" width="100%">'
  const sheet = '<style>@keyframes spin{to{opacity:1}}.d{animation:spin 2s linear infinite}</style>'

  it('keeps the same <style> element with the same text as shapes arrive', () => {
    const el = document.createElement('div')
    morphInto(el, renderStreamingMarkdown(`${opening}${sheet}<circle class="d" r="4" />`))
    const style = el.querySelector('style')
    const before = style?.textContent

    morphInto(el, renderStreamingMarkdown(`${opening}${sheet}<circle class="d" r="4" /><rect width="8" height="8" />`))

    expect(el.querySelector('style')).toBe(style)
    expect(el.querySelector('style')?.textContent).toBe(before)
    expect(el.querySelectorAll('rect')).toHaveLength(1)
  })

  // The scope used to be a fingerprint of the whole drawing, so every token
  // renamed the keyframes and the rule that starts it — an animation that was
  // redeclared sixty times a second and therefore never moved.
  it('keeps its scope while it grows', () => {
    const scopeOf = (svgText: string) =>
      renderStreamingMarkdown(svgText).match(/data-drawing="([^"]+)"/)?.[1]

    const early = scopeOf(`${opening}${sheet}<circle class="d" r="4" />`)
    const later = scopeOf(`${opening}${sheet}<circle class="d" r="4" /><rect width="8" height="8" />`)

    expect(early).toBeTruthy()
    expect(later).toBe(early)
  })

  // Position is what still separates two drawings that open the same way.
  it('still keeps two drawings that open identically apart', () => {
    const one = `${opening}<style>.d{fill:red}</style><circle class="d" r="4" /></svg>`
    const out = renderStreamingMarkdown(`${one}\n\n${one}`)
    // Off the <svg> itself: the scope appears a second time inside each
    // drawing's own scoped stylesheet.
    const scopes = [...out.matchAll(/<svg[^>]*data-drawing="([^"]+)"/g)].map((m) => m[1])

    expect(scopes).toHaveLength(2)
    expect(scopes[0]).not.toBe(scopes[1])
  })
})
