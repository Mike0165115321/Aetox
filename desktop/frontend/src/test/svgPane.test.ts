// An .svg on the desk is a drawing, not a photograph.
//
// internal/prompt's drawing layer teaches one way to draw: size in viewBox
// units, width="100%", colour through var(--surface-raised) and friends,
// because the app's theme decides the palette. A model that then writes the
// drawing to a FILE used to get nothing at all — the pane pointed an <img> at
// it, and an <img> is a separate document: the percentage width resolved
// against a shrink-to-fit box (0×0) and every var() resolved to black. Measured
// in Chromium on the owner's own code-structure-map.svg, 29 ส.ค.: the image
// loaded fine and laid out at 0×0, so the pane drew an empty checkerboard.
//
// The file is drawn into the app's own document now, through the same door a
// drawing in an answer takes.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import ImagePane from '../lib/workbench/ImagePane.svelte'
import { setLocale } from '../lib/i18n.svelte'

const MAP =
  '<svg viewBox="0 0 640 372" width="100%" xmlns="http://www.w3.org/2000/svg">' +
  '<defs><marker id="ah"><path d="M0 0L10 5L0 10z" fill="var(--text-secondary)"/></marker></defs>' +
  '<rect x="28" y="12" width="596" height="44" fill="var(--surface-raised)"/>' +
  '<text x="34" y="37" fill="var(--text-primary)">Front ends</text></svg>'

const serve = (body: string, ok = true) => {
  const fetched = vi.fn(async () => ({ ok, status: ok ? 200 : 404, text: async () => body }))
  vi.stubGlobal('fetch', fetched)
  return fetched
}

// Scoped to the drawing, never to the pane: the header's open-externally button
// carries an Icon, and an Icon is an <svg>, so a bare querySelector('svg') finds
// the furniture and passes before the file has even been read.
const drawn = (c: HTMLElement) => c.querySelector('.draw svg')

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
})

const props = { src: '/aetox-file/docs/map.svg', name: 'map.svg', path: 'docs/map.svg' }

describe('an .svg on the desk', () => {
  it('draws it into the document, where the theme and the width live', async () => {
    serve(MAP)
    const { container } = render(ImagePane, props)

    await waitFor(() => expect(drawn(container)).toBeTruthy())
    const svg = drawn(container)!
    // The drawing itself, not a link to it: an <img> is what could not resolve
    // var() or find a width.
    expect(container.querySelector('img')).toBeNull()
    expect(svg.querySelector('rect')?.getAttribute('fill')).toBe('var(--surface-raised)')
    expect(svg.textContent).toContain('Front ends')
    // And the box it needs. A vector has no pixels to shrink-wrap, so the
    // wrapper has to take its width from the pane — the whole 0×0 failure.
    expect(container.querySelector('.img-wrap')?.classList.contains('drawing')).toBe(true)
  })

  // The hazards of putting a file's markup in the app's document are the same
  // ones a drawing in an answer carries, and they go through the same door.
  it('strips a script and scopes what would reach the app', async () => {
    serve(
      '<svg viewBox="0 0 10 10"><script>fetch("http://evil")</script>' +
      '<style>.row{display:none}</style><rect class="row" onclick="alert(1)" width="5" height="5"/></svg>'
    )
    const { container } = render(ImagePane, props)

    await waitFor(() => expect(drawn(container)).toBeTruthy())
    expect(container.querySelector('.draw script')).toBeNull()
    expect(container.querySelector('.draw rect')?.getAttribute('onclick')).toBeNull()
    // The stylesheet survives, aimed only at the drawing that brought it — the
    // app's own .row must keep its display.
    const css = container.querySelector('.draw style')?.textContent ?? ''
    expect(css).toContain('[data-drawing=')
    expect(drawn(container)?.getAttribute('data-drawing')).toBeTruthy()
  })

  // Two maps open side by side both define `id="ah"` for their arrowhead, and
  // `url(#ah)` finds the first one in the document. The scope is the file's
  // path, so the second drawing keeps its own marker.
  it('keeps two files’ ids apart', async () => {
    serve(MAP)
    const a = render(ImagePane, props)
    const b = render(ImagePane, { ...props, path: 'docs/other.svg', name: 'other.svg' })

    await waitFor(() => expect(a.container.querySelector('.draw marker')).toBeTruthy())
    await waitFor(() => expect(b.container.querySelector('.draw marker')).toBeTruthy())
    const idA = a.container.querySelector('.draw marker')?.getAttribute('id')
    const idB = b.container.querySelector('.draw marker')?.getAttribute('id')
    expect(idA).not.toBe('ah')
    expect(idA).not.toBe(idB)
  })

  // A pane that draws nothing and says nothing is the bug this change is about.
  it('says so when the file cannot be read', async () => {
    serve('', false)
    render(ImagePane, props)
    await waitFor(() => expect(screen.getByText(/could not read the drawing/i)).toBeTruthy())
  })

  it('says so when the file holds no drawing', async () => {
    serve('just some text')
    render(ImagePane, props)
    await waitFor(() => expect(screen.getByText(/no drawing/i)).toBeTruthy())
  })

  // Everything that is not an .svg keeps the route it had: bytes stay bytes,
  // streamed by the file host rather than read into a string.
  it('leaves a photograph on the file host', async () => {
    const fetched = serve(MAP)
    const { container } = render(ImagePane, { src: '/aetox-file/shot.png', name: 'shot.png', path: 'shot.png' })

    await waitFor(() => expect(container.querySelector('img')).toBeTruthy())
    expect(container.querySelector('img')?.getAttribute('src')).toBe('/aetox-file/shot.png')
    expect(fetched).not.toHaveBeenCalled()
  })
})
