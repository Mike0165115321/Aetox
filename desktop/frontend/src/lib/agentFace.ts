// The wardrobe an เอเจน's face is assembled from.
//
// A catalogue rather than a drawing. The first version of this lived as one
// chain of {#if hair === 4} inside AgentFace.svelte, which worked and would
// have stopped working the first time somebody wanted a ninth haircut: adding
// one meant editing a branch in the middle of a render, and offering one to a
// user meant a number with no name. Here a part is a row, adding one is
// appending to an array, and every part already carries the label a picker
// would show.
//
// Two rules hold this together and both matter more than the drawings:
//
//  1. **A part is identified by its id, never its position.** The hash picks an
//     INDEX, so reordering these arrays would repaint the whole roster — but an
//     agent that names its part in its own file names the id, and that keeps
//     working whatever the order becomes. Append rather than insert.
//  2. **Nothing here reads a file or a store.** Given a name it returns the
//     same face on every machine, forever, which is what lets the face be
//     derived instead of stored (see AgentFace.svelte).
import { coverHue } from './coverHue'

// The one palette every part is drawn from, so a new part cannot invent a
// colour that sits outside the agent's own family.
export type Palette = {
  skin: string
  skinShade: string
  shirt: string
  dark: string
  darkUp: string
  bright: string
  glass: string
}

export function palette(hue: number): Palette {
  return {
    skin: `hsl(${hue} 42% 78%)`,
    skinShade: `hsl(${hue} 36% 66%)`,
    shirt: `hsl(${hue} 38% 42%)`,
    dark: `hsl(${hue} 46% 23%)`,
    darkUp: `hsl(${hue} 48% 31%)`,
    bright: `hsl(${hue} 58% 86%)`,
    glass: `hsl(${hue} 28% 40%)`,
  }
}

// The head every hair is cut for. A part may override it — the long and curly
// cuts sit around a slightly smaller head — but only through this shape, so a
// new part cannot quietly move the eyes off the face.
export type Head = { cy: number; rx: number; ry: number }
const HEAD: Head = { cy: 26, rx: 15.5, ry: 16.5 }

export type Hair = {
  id: string
  label: string
  /** Drawn under the head. Volume that frames the face lives here. */
  behind?: (p: Palette) => string
  /** Drawn over the head. Ordinary hair lives here. */
  front?: (p: Palette) => string
  /** Ears show unless a cut covers them. */
  ears?: boolean
  head?: Head
}

// Append only. See rule 1 above.
export const HAIR: Hair[] = [
  {
    id: 'sidePart',
    label: 'แสกข้าง',
    front: (p) => `<path d="M16.5 25C16.5 12 24 9 32 9s15.5 3 15.5 16c0-7-4-10-12-10-5 0-8 2-11 2-4 0-6.5 3-8 8z" fill="${p.dark}"/>`,
  },
  {
    id: 'bun',
    label: 'มวย',
    front: (p) =>
      `<circle cx="32" cy="7.5" r="5" fill="${p.dark}"/>` +
      `<path d="M17 24C17 12 24 10 32 10s15 2 15 14c0-7-5-10-15-10s-15 3-15 10z" fill="${p.dark}"/>`,
  },
  {
    id: 'bob',
    label: 'บ๊อบ',
    front: (p) => `<path d="M15 30C15 12 23 8.5 32 8.5S49 12 49 30v6h-5V21c-4-4-20-4-24 0v15h-5z" fill="${p.dark}"/>`,
  },
  {
    id: 'curly',
    label: 'หยิก',
    ears: false,
    head: { cy: 27, rx: 14, ry: 15 },
    behind: (p) =>
      `<circle cx="20" cy="16" r="7.5" fill="${p.dark}"/>` +
      `<circle cx="32" cy="11" r="8.5" fill="${p.dark}"/>` +
      `<circle cx="44" cy="16" r="7.5" fill="${p.dark}"/>` +
      `<circle cx="17" cy="24" r="5.5" fill="${p.dark}"/>` +
      `<circle cx="47" cy="24" r="5.5" fill="${p.dark}"/>`,
  },
  {
    id: 'cap',
    label: 'หมวกแก๊ป',
    front: (p) =>
      `<path d="M16 21C16 10 23 6.5 32 6.5S48 10 48 21z" fill="${p.dark}"/>` +
      `<path d="M4 18h13v5H6a2.5 2.5 0 0 1 0-5z" fill="${p.darkUp}"/>` +
      `<rect x="15" y="19.5" width="34" height="4.5" rx="2.2" fill="${p.darkUp}"/>`,
  },
  {
    id: 'beanie',
    label: 'หมวกไหมพรม',
    front: (p) =>
      `<path d="M16 22C16 11 23 7 32 7s16 4 16 15z" fill="${p.dark}"/>` +
      `<rect x="15" y="20.5" width="34" height="5.5" rx="2.7" fill="${p.darkUp}"/>` +
      `<circle cx="32" cy="5" r="3.2" fill="${p.darkUp}"/>`,
  },
  {
    id: 'long',
    label: 'ผมยาว',
    ears: false,
    head: { cy: 27, rx: 14.5, ry: 15.5 },
    behind: (p) => `<path d="M14 44V26c0-11 8-17 18-17s18 6 18 17v18h-6V26c0-7-5-11-12-11s-12 4-12 11v18z" fill="${p.dark}"/>`,
  },
  {
    id: 'neat',
    label: 'เรียบ',
    front: (p) => `<path d="M17 25C17 12 25 9.5 32 9.5S47 12 47 25c-1-9-7-12-15-12s-14 3-15 12z" fill="${p.dark}"/>`,
  },
]

export type Accessory = { id: string; label: string; svg?: (p: Palette) => string }

// `none` appears twice on purpose, and it is the same rule the card's chips are
// drawn under: a badge every card carries is a badge that says nothing. Half a
// roster wearing nothing is what makes the glasses mean something.
export const ACCESSORY: Accessory[] = [
  { id: 'none', label: 'ไม่มี' },
  {
    id: 'glasses',
    label: 'แว่น',
    svg: (p) =>
      `<circle cx="26" cy="27" r="6" fill="none" stroke="${p.glass}" stroke-width="2"/>` +
      `<circle cx="38" cy="27" r="6" fill="none" stroke="${p.glass}" stroke-width="2"/>` +
      `<path d="M31 27h2" stroke="${p.glass}" stroke-width="2"/>`,
  },
  { id: 'none2', label: 'ไม่มี' },
  {
    id: 'headphones',
    label: 'หูฟัง',
    svg: (p) =>
      `<path d="M15 26a17 17 0 0 1 34 0" fill="none" stroke="${p.darkUp}" stroke-width="3.4"/>` +
      `<rect x="10.5" y="24" width="8" height="12" rx="3.5" fill="${p.darkUp}"/>` +
      `<rect x="45.5" y="24" width="8" height="12" rx="3.5" fill="${p.darkUp}"/>`,
  },
]

// What the agent is holding, keyed by the `icon:` its profile already declares.
// Nothing new is asked of whoever writes the file, and an icon this map does not
// know draws no prop rather than a guess — the same fallback rule as workerFace,
// for the same reason: a profile is written by hand by somebody who cannot see
// this list.
export const PROP: Record<string, (p: Palette) => string> = {
  search: (p) =>
    `<circle cx="50" cy="50" r="8" fill="none" stroke="${p.bright}" stroke-width="3.2"/>` +
    `<path d="M56 56l5.5 5.5" stroke="${p.bright}" stroke-width="3.2" stroke-linecap="round"/>`,
  fileText: (p) =>
    `<path d="M43 44h11l7 7v13H43z" fill="${p.bright}"/>` +
    `<path d="M54 44v7h7" fill="${p.skinShade}"/>` +
    `<path d="M47 55h11M47 60h8" stroke="${p.dark}" stroke-width="2.2" stroke-linecap="round"/>`,
  clapperboard: (p) =>
    `<rect x="40" y="47" width="22" height="15" rx="2.5" fill="${p.bright}"/>` +
    `<path d="M40 51.5h22" stroke="${p.dark}" stroke-width="2.6"/>` +
    `<path d="M44 47l3.5 4.5M50 47l3.5 4.5M56 47l3.5 4.5" stroke="${p.dark}" stroke-width="2.2"/>`,
  chartColumn: (p) =>
    `<rect x="43" y="52" width="5" height="10" rx="2" fill="${p.bright}"/>` +
    `<rect x="50.5" y="46" width="5" height="16" rx="2" fill="${p.bright}"/>` +
    `<rect x="58" y="40" width="5" height="22" rx="2" fill="${p.bright}"/>`,
  gitBranch: (p) =>
    `<circle cx="46" cy="46" r="3.8" fill="${p.bright}"/>` +
    `<circle cx="46" cy="60" r="3.8" fill="${p.bright}"/>` +
    `<circle cx="60" cy="46" r="3.8" fill="${p.bright}"/>` +
    `<path d="M46 49.8v6.4M49.8 46h6.4" stroke="${p.bright}" stroke-width="2.6"/>`,
  zap: (p) => `<path d="M53 40l-10 15h7.5l-2.5 11 11-15.5h-7.5z" fill="${p.bright}"/>`,
  slidersHorizontal: (p) =>
    `<path d="M42 50h20M42 60h20" stroke="${p.bright}" stroke-width="2.8" stroke-linecap="round"/>` +
    `<circle cx="51" cy="50" r="3.6" fill="${p.bright}"/>` +
    `<circle cx="56" cy="60" r="3.6" fill="${p.bright}"/>`,
}

// Below this the prop is four pixels of noise competing with the head, so it is
// dropped rather than shrunk. A legible 24px row is worth more than a complete
// one.
export const PROP_MIN_PX = 32

// Deliberately not coverHue. Sharing one hash would tie every cyan agent to the
// same haircut, and the two roster names that already land four degrees apart
// would arrive as the same person twice.
export function wardrobeHash(s: string): number {
  let h = 2166136261
  for (const ch of s) {
    h ^= ch.codePointAt(0)!
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** What a profile may say about its own face. Every field is optional and an
 *  unknown value falls back to the derived one, never to an error: these arrive
 *  from a .md written by hand by somebody who cannot see this file. */
export type FaceOverrides = {
  hue?: number
  hair?: string
  accessory?: string
  icon?: string
}

export type Face = {
  hue: number
  p: Palette
  hair: Hair
  accessory: Accessory
  head: Head
  ears: boolean
  prop: string
  smiles: boolean
}

function pick<T extends { id: string }>(list: T[], id: string | undefined, index: number): T {
  if (id) {
    const named = list.find((x) => x.id === id)
    if (named) return named
  }
  return list[index % list.length]
}

export function resolveFace(name: string, size: number, o: FaceOverrides = {}): Face {
  const hue = o.hue ?? coverHue(name)
  const w = wardrobeHash(name)
  const hair = pick(HAIR, o.hair, w)
  const accessory = pick(ACCESSORY, o.accessory, w >>> 3)
  return {
    hue,
    p: palette(hue),
    hair,
    accessory,
    head: hair.head ?? HEAD,
    ears: hair.ears ?? true,
    prop: size >= PROP_MIN_PX && o.icon && o.icon in PROP ? o.icon : '',
    smiles: w % 3 !== 0,
  }
}

/** The whole character as one markup string, so the component stays a frame and
 *  a test can assert on parts without mounting anything. */
export function faceSVG(f: Face): string {
  const { p, head } = f
  let s =
    `<path d="M3 64c0-13 12-19 29-19s29 6 29 19z" fill="${p.shirt}"/>` +
    `<path d="M25 45l7 8 7-8z" fill="${p.skin}"/>` +
    `<rect x="27.5" y="35" width="9" height="11" fill="${p.skinShade}"/>`
  if (f.ears) {
    s += `<circle cx="16.5" cy="28" r="3.4" fill="${p.skinShade}"/><circle cx="47.5" cy="28" r="3.4" fill="${p.skinShade}"/>`
  }
  if (f.hair.behind) s += f.hair.behind(p)
  s += `<ellipse cx="32" cy="${head.cy}" rx="${head.rx}" ry="${head.ry}" fill="${p.skin}"/>`
  if (f.hair.front) s += f.hair.front(p)
  s += `<circle cx="26" cy="27" r="2.6" fill="#14161a"/><circle cx="38" cy="27" r="2.6" fill="#14161a"/>`
  if (f.accessory.svg) s += f.accessory.svg(p)
  s += f.smiles
    ? `<path d="M29 35q3 2.5 6 0" stroke="#14161a" stroke-width="1.9" fill="none" stroke-linecap="round"/>`
    : `<path d="M29 35h6" stroke="#14161a" stroke-width="1.9" stroke-linecap="round"/>`
  if (f.prop) s += PROP[f.prop](p)
  return s
}
