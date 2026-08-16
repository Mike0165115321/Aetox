// The composer's popovers, measured against the pane rather than the screen.
//
// The workbench can take most of the window, and the chat column left behind is
// narrower than the menus that open in it. With a hard min-width those menus
// grew leftward out of the pane and their labels were cut off at the edge —
// perfectly laid out, half of it outside the window.
//
// Read off disk for the same reason themeContrast.test.ts does: vitest stubs
// CSS imports to "", and a rule checked against an empty stylesheet passes.
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

const css = readFileSync('src/style.css', 'utf8')

/** The declarations of the first rule whose selector matches exactly. */
function rule(selector: string): string {
  const re = new RegExp(`(^|[\\}\\/*\\n])\\s*${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*\\{([^}]*)\\}`, 'm')
  const m = css.match(re)
  if (!m) throw new Error(`no rule for ${selector}`)
  return m[2]
}

describe('the composer bounds its own popovers', () => {
  it('names the composer box as the container they are measured against', () => {
    const box = rule('.composer .box')
    expect(box).toContain('container-name:composer')
    // Inline only: the composer's height is the prompt's business, and size
    // containment on the block axis would stop it growing with the text.
    expect(box).toContain('container-type:inline-size')
  })

  // Both menus on this row had a hard min-width, and both overflowed the same
  // way. Whichever is fixed alone, the other is the next bug report.
  it.each(['.model-menu', '.ctx-menu'])('%s never grows past the composer', (selector) => {
    const decls = rule(selector)
    expect(decls).toContain('max-width:100cqi')
    expect(decls).toMatch(/min-width:min\(\d+px, ?100cqi\)/)
  })

  // Bounding the width was not enough on its own: measured from the chip, a
  // full container width still overflows the left edge by whatever sits
  // between the chip and the box's right edge — the send button. The anchor
  // has to be the box, which means the chip's wrapper must not be positioned.
  it.each(['.model-menu', '.ctx-menu'])('%s is anchored to the composer box, not to its chip', (selector) => {
    expect(rule(selector)).toContain('right:var(--composer-pad-x)')
  })

  it.each(['.model-pick', '.ctx-pick'])('%s does not become the anchor by being positioned', (selector) => {
    expect(rule(selector)).not.toContain('position:relative')
  })

  it('keeps a dropdown list inside the menu that opens it', () => {
    expect(rule('.updrop-list')).toMatch(/max-width:min\(240px, ?calc\(100cqi - 32px\)\)/)
  })

  // Side by side is the shape that reads, so it holds for as long as it fits.
  it('stacks a row label above its control only once there is no room beside it', () => {
    const at = css.indexOf('@container composer (max-width: 220px)')
    expect(at).toBeGreaterThan(-1)
    expect(css.slice(at)).toContain('flex-direction:column')
  })

  it('lets a long row label trim itself instead of widening the menu', () => {
    const lbl = rule('.mm-row .lbl')
    expect(lbl).toContain('text-overflow:ellipsis')
    expect(lbl).not.toContain('flex:none')
  })
})

describe('the chips above the input', () => {
  it('wrap rather than running off the end of the pane', () => {
    expect(rule('.composer > .focus-row')).toContain('flex-wrap:wrap')
  })

  it('trim a single chip that is longer than the whole row', () => {
    expect(rule('.composer > .focus-row .focus-chip .t')).toContain('text-overflow:ellipsis')
  })
})
