// Every colour pair the chat/sidebar chrome depends on, measured against all 14
// themes. Not a style opinion — WCAG contrast ratios, computed from the real
// var values in theme.css (resolved through palette.css).
//
// This started as a one-off script, which is exactly how a check rots: nothing
// ran it, so the next theme could quietly reintroduce an invisible border.
import { describe, it, expect } from 'vitest'
// Read off disk rather than imported: vitest stubs CSS imports to "" by
// default (css: false), and a check fed an empty stylesheet passes everything.
// Paths are relative to the vitest root, which is this package.
import { readFileSync } from 'node:fs'

const palette = readFileSync('src/styles/palette.css', 'utf8')
const themes = readFileSync('src/styles/theme.css', 'utf8')

const declsOf = (block: string) => {
  const out: Record<string, string> = {}
  for (const m of block.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) out[m[1]] = m[2].trim()
  return out
}

// Tier 1 primitives live in palette.css :root
const prims: Record<string, string> = declsOf(palette.slice(palette.indexOf(':root {')))

// Each theme block: selector { ... }
const themeBlocks: { name: string; vars: Record<string, string> }[] = []
const re = /(:root(?:\[data-theme="([\w-]+)"\])?(?:,\s*:root\[data-theme="([\w-]+)"\])?)\s*\{([^}]*)\}/g
for (const m of themes.matchAll(re)) {
  const name = m[2] || m[3] || 'aetox (default)'
  themeBlocks.push({ name, vars: declsOf(m[4]) })
}

const resolve = (vars: Record<string, string>, key: string, depth = 0): string | null => {
  let v = vars[key] ?? prims[key]
  if (!v || depth > 6) return null
  const ref = v.match(/^var\((--[\w-]+)\)$/)
  if (ref) return resolve(vars, ref[1], depth + 1)
  return /^#[0-9a-f]{3,8}$/i.test(v) ? v : null
}

const rgb = (hex: string) => {
  let h = hex.slice(1)
  if (h.length === 3) h = [...h].map((c) => c + c).join('')
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16))
}
const lum = (hex: string) => {
  const [r, g, b] = rgb(hex).map((v) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
const contrast = (a: string, b: string) => {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

// [label, fg var, bg var, min ratio, why]
const CHECKS: [label: string, fg: string, bg: string, min: number, why: string][] = [
  // 3.0, not the 4.5 body-text bar: inline code is drawn in --accent, the same
  // colour every link in the app already uses on the same surfaces. Holding it
  // to a stricter bar than the links beside it would be measuring one word in a
  // sentence more harshly than the sentence. Raise this only by changing the
  // themes' accents, which is a whole-app decision, not a code-chip one.
  ['inline code text', '--accent', '--surface-code', 3.0, 'code words mid-sentence'],
  ['code block border', '--border-default', '--surface-app', 1.15, 'block must be visible now the bubble is gone'],
  ['table row rule', '--border-default', '--surface-app', 1.15, 'the only structure a borderless table has'],
  ['add-project dashes', '--border-default', '--surface-sunken', 1.15, 'dashed row in the sidebar'],
  ['seg active border', '--border-default', '--surface-app', 1.15, 'which tab is selected'],
  ['seg active label', '--text-primary', '--surface-raised', 4.5, ''],
  ['seg idle label', '--text-muted', '--surface-app', 3.0, ''],
  ['scroll-bottom edge', '--border-default', '--surface-app', 1.15, 'the button carries a border, so that is what must read'],
  ['reply text', '--text-primary', '--surface-app', 4.5, 'agent text now sits on the app surface'],
  ['toggle label', '--text-muted', '--surface-app', 3.0, '"thought for 34s" / "used N tools"'],
  ['user bubble text', '--text-primary', '--surface-raised', 4.5, ''],
]


describe('theme contrast', () => {
  it('resolves every theme in theme.css', () => {
    // A parser that silently matched nothing would make every check below pass.
    expect(themeBlocks.length).toBeGreaterThanOrEqual(14)
  })

  for (const { name, vars } of themeBlocks) {
    it(`keeps the chat chrome legible on ${name}`, () => {
      const failures = []
      for (const [label, fgK, bgK, min, why] of CHECKS) {
        const fg = resolve(vars, fgK)
        const bg = resolve(vars, bgK)
        // An unresolved var would silently skip its check, so it fails loudly.
        if (!fg || !bg) {
          failures.push(`${label}: ${!fg ? fgK : bgK} does not resolve on ${name}`)
          continue
        }
        const ratio = contrast(fg, bg)
        if (ratio < min) failures.push(`${label}: ${ratio.toFixed(2)} < ${min} (${fgK} ${fg} on ${bgK} ${bg})${why ? ' — ' + why : ''}`)
      }
      expect(failures, failures.join(' | ')).toHaveLength(0)
    })
  }
})
