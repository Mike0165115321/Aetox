import { describe, it, expect } from 'vitest'
import { HAIR, ACCESSORY, PROP, PROP_MIN_PX, resolveFace, faceSVG } from '../lib/agentFace'

// The wardrobe is a catalogue a profile may point into by name, which makes
// these ids part of the file format rather than an implementation detail. What
// is guarded here is the small set of rules that lets a part be ADDED later
// without repainting anybody or breaking a file somebody already wrote.
describe('agent face wardrobe', () => {
  // A duplicate id makes `hair: bob` ambiguous, and the loser would be
  // whichever one happens to sit later in the array.
  it('gives every part a unique id', () => {
    const hair = HAIR.map((h) => h.id)
    const acc = ACCESSORY.map((a) => a.id)
    expect(new Set(hair).size).toBe(hair.length)
    expect(new Set(acc).size).toBe(acc.length)
  })

  // The whole reason a face can be derived instead of stored. If this ever
  // stops holding, an agent's face changes under them between two launches.
  it('draws the same face for a name every time', () => {
    const a = faceSVG(resolveFace('research', 88, { icon: 'search' }))
    const b = faceSVG(resolveFace('research', 88, { icon: 'search' }))
    expect(a).toBe(b)
  })

  // Two hashes, not one — sharing coverHue would tie every agent of a colour to
  // the same haircut. This is the cheapest observable proof they are separate.
  it('varies hair independently of hue', () => {
    const shapes = new Set(['doc', 'github', 'sheet', 'editor'].map((n) => resolveFace(n, 88).hair.id))
    expect(shapes.size).toBeGreaterThan(1)
  })

  // These fields arrive from a .md written by hand by somebody who cannot see
  // this file. A typo has to land on the derived face, never on a blank square.
  it('falls back to the derived part when a profile names one that does not exist', () => {
    const typo = resolveFace('research', 88, { hair: 'mohawk' })
    expect(typo.hair.id).toBe(resolveFace('research', 88).hair.id)
    expect(faceSVG(typo)).toContain('<ellipse')
  })

  it('honours a part the profile does name', () => {
    expect(resolveFace('research', 88, { hair: 'beanie' }).hair.id).toBe('beanie')
    expect(resolveFace('research', 88, { accessory: 'headphones' }).accessory.id).toBe('headphones')
  })

  // The override exists so the seven bundled agents can be spread across the
  // wheel by hand; coverHue clusters them, and two of them land four degrees
  // apart. Nothing about coverHue changes for anyone who does not set this.
  it('lets a profile choose its own hue', () => {
    expect(resolveFace('research', 88, { hue: 28 }).hue).toBe(28)
    expect(resolveFace('research', 88).hue).toBe(resolveFace('research', 24).hue)
  })

  // Below the threshold the prop is a few pixels of noise beside the head, and
  // the row it sits in is the @ menu, where the name is doing the work anyway.
  it('drops the prop below the small size', () => {
    expect(resolveFace('research', PROP_MIN_PX, { icon: 'search' }).prop).toBe('search')
    expect(resolveFace('research', PROP_MIN_PX - 1, { icon: 'search' }).prop).toBe('')
  })

  // An icon no prop is drawn for is the ordinary case, not an error: `icon:` is
  // the profile's MARK and this map only covers the ones worth holding.
  it('draws no prop for an icon it has none for', () => {
    expect(resolveFace('research', 88, { icon: 'userRound' }).prop).toBe('')
    expect(Object.keys(PROP).length).toBeGreaterThan(0)
  })

  // Every part combination has to produce a person. A hair that forgot to draw
  // the head, or a size that dropped the body, would ship as an empty tile on
  // somebody's roster and nowhere else.
  it('draws a whole person for every part in the catalogue', () => {
    for (const h of HAIR) {
      for (const a of ACCESSORY) {
        const svg = faceSVG(resolveFace('x', 88, { hair: h.id, accessory: a.id, icon: 'search' }))
        expect(svg, `${h.id}/${a.id}`).toContain('<ellipse')
        expect(svg, `${h.id}/${a.id}`).toContain('#14161a')
      }
    }
  })
})
