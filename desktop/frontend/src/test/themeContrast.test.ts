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

// ---------------------------------------------------------------------------
// Tokens that do not exist. The checks above measure colours that resolve; this
// one catches the failure a contrast check cannot see, because there is no
// colour to measure.
//
// `border: 1px solid var(--border)` is not a dim border — `--border` was never
// declared in any theme, so the whole declaration is invalid at computed-value
// time and the border is simply *not drawn*. It sat in the chat's file cards
// and the workbook preview's grid, and it reads as "the UI looks unfinished"
// rather than as anything a person would think to grep for.
import { readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

// src/test is skipped: nothing in it is shipped UI, and this very file names
// the broken token in prose, which the scan below would otherwise report.
function sourceFiles(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (name === 'test') continue
    if (statSync(p).isDirectory()) sourceFiles(p, out)
    else if (/\.(svelte|css|ts)$/.test(name)) out.push(p)
  }
  return out
}

describe('css custom properties', () => {
  it('never asks for a token no theme declares', () => {
    const files = sourceFiles('src')
    const declared = new Set<string>()
    const used = new Map<string, string[]>()

    for (const f of files) {
      const src = readFileSync(f, 'utf8')
      // A declaration anywhere counts: theme.css, a scoped <style>, or an
      // inline style="--dw:390" in markup.
      for (const m of src.matchAll(/(--[\w-]+)\s*:/g)) declared.add(m[1])
      // A var() carrying a fallback cannot vanish, so it is not this bug.
      for (const m of src.matchAll(/var\((--[\w-]+)\s*(,)?/g)) {
        if (m[2]) continue
        used.set(m[1], [...(used.get(m[1]) ?? []), f])
      }
    }

    // syntaxTheme.ts writes these at runtime under a built-up name
    // ('--syn-' + token), so no literal declaration exists to find.
    const missing = [...used.keys()]
      .filter((k) => !declared.has(k) && !k.startsWith('--syn-'))
      .map((k) => `${k} (used in ${[...new Set(used.get(k))].join(', ')})`)

    expect(missing, `undeclared: ${missing.join(' | ')}`).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// Syntax colours. Chat code blocks used to import one fixed highlight.js
// stylesheet, so every theme got GitHub Dark Dimmed's tokens — on the four
// light themes that measured ten of fourteen tokens under 3:1 against the code
// surface. They now come from syntaxTheme.ts, the same table Monaco builds its
// editor themes from, so the two surfaces cannot drift apart.
import { SYNTAX_THEMES, applySyntaxTheme } from '../lib/syntaxTheme'
import { THEMES } from '../lib/theme.svelte'

const SYNTAX_TOKENS = [
  'comment', 'string', 'number', 'constant', 'keyword', 'function',
  'type', 'tag', 'attribute.name', 'variable', 'delimiter', 'regexp', 'invalid',
]

describe('syntax theme', () => {
  it('covers every theme the UI offers', () => {
    const missing = THEMES.map((t) => t.value).filter((name) => !(name in SYNTAX_THEMES))
    expect(missing, `no syntax palette for: ${missing.join(', ')}`).toHaveLength(0)
  })

  for (const [name, palette] of Object.entries(SYNTAX_THEMES)) {
    it(`gives ${name} a complete palette`, () => {
      const missing = SYNTAX_TOKENS.filter((tok) => !palette.syntax[tok])
      expect(missing, `${name} is missing: ${missing.join(', ')}`).toHaveLength(0)
      expect(palette.chrome['editor.background']).toMatch(/^#[0-9a-f]{6}$/i)
      expect(palette.chrome['editor.foreground']).toMatch(/^#[0-9a-f]{6}$/i)
    })

    // Comments are excluded on purpose: Nord, Catppuccin Latte and the rest
    // deliberately recede theirs, and raising them would stop these being the
    // themes they are named after. Everything else has to be readable on the
    // background the same palette declares, which is what the code block is
    // now painted on.
    it(`keeps ${name}'s code readable on its own background`, () => {
      const bg = palette.chrome['editor.background']
      const failures = Object.entries(palette.syntax)
        .filter(([tok]) => tok !== 'comment')
        .map(([tok, hex]) => [tok, contrast(hex, bg)] as const)
        .filter(([, ratio]) => ratio < 2.1)
        .map(([tok, ratio]) => `${tok} ${ratio.toFixed(2)}:1`)
      expect(failures, `${name}: ${failures.join(', ')}`).toHaveLength(0)
    })
  }

  it('writes the active palette where CSS can reach it', () => {
    applySyntaxTheme('dracula')
    const root = document.documentElement.style
    expect(root.getPropertyValue('--syn-keyword')).toBe(SYNTAX_THEMES.dracula.syntax.keyword)
    // The one token whose Monaco name carries a dot has to survive the trip.
    expect(root.getPropertyValue('--syn-attribute-name')).toBe(SYNTAX_THEMES.dracula.syntax['attribute.name'])
    expect(root.getPropertyValue('--syn-bg')).toBe(SYNTAX_THEMES.dracula.chrome['editor.background'])

    applySyntaxTheme('gruvbox-light')
    expect(root.getPropertyValue('--syn-keyword')).toBe(SYNTAX_THEMES['gruvbox-light'].syntax.keyword)
  })
})
