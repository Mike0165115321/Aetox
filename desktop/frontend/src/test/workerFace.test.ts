// The mark a worker wears, and the seam between two languages.
//
// A profile names its icon in a markdown file compiled into the Go binary; the
// shape it names lives in this app's icon table. Nothing type-checks across
// that seam, so a name that is one letter off, or a set that drops an icon
// somebody's file still asks for, draws an empty square on the roster and says
// nothing to anybody.
//
// The second half is the fallback, which was the actual bug (owner, 20 ส.ค.:
// "เอเจนมีไอค่อนของตัวเอง ซับเอเจน มีอะไร"). Three of the five bundled เอเจน
// name no icon, and `|| 'bot'` handed them the ซับเอเจน mark, on the เอเจน
// page, under a sidebar item wearing `userRound` for exactly that distinction.
import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { ICONS } from '../lib/icons'
import { workerFace } from '../lib/workerFace'

const PROFILES = '../../internal/subagent/profiles'

function frontMatter(path: string): Record<string, string> {
  const raw = readFileSync(path, 'utf8')
  const end = raw.indexOf('\n---', 4)
  const head = end < 0 ? raw : raw.slice(0, end)
  const out: Record<string, string> = {}
  for (const line of head.split('\n')) {
    const at = line.indexOf(':')
    if (at > 0) out[line.slice(0, at).trim()] = line.slice(at + 1).trim()
  }
  return out
}

// [path, isAgent] for every bundled worker: an เอเจน is a folder with an
// AGENT.md in the agents home, a ซับเอเจน is a file in the subagents home. The
// home is the kind, which is the same rule the engine applies (subagent.KindOf).
function bundled(): [string, boolean][] {
  const out: [string, boolean][] = []
  for (const dir of readdirSync(`${PROFILES}/agents`)) {
    const file = `${PROFILES}/agents/${dir}/AGENT.md`
    if (existsSync(file)) out.push([file, true])
  }
  for (const name of readdirSync(`${PROFILES}/subagents`)) {
    if (name.endsWith('.md')) out.push([`${PROFILES}/subagents/${name}`, false])
  }
  return out
}

describe('the mark a worker wears', () => {
  it('has a bundled roster to check, or this file proves nothing', () => {
    const rows = bundled()
    expect(rows.filter(([, isAgent]) => isAgent).length).toBeGreaterThan(0)
    expect(rows.filter(([, isAgent]) => !isAgent).length).toBeGreaterThan(0)
  })

  it('never names a shape this build does not have', () => {
    for (const [file] of bundled()) {
      const icon = frontMatter(file).icon
      if (!icon) continue
      expect({ file, icon, known: icon in ICONS }).toEqual({ file, icon, known: true })
    }
  })

  it('gives every bundled worker a face of its own', () => {
    // Not a style rule: two workers wearing one shape is a roster you cannot
    // scan, and the page that lists them is the page you pick from.
    const seen = new Map<string, string>()
    for (const [file] of bundled()) {
      const icon = frontMatter(file).icon
      expect({ file, named: Boolean(icon) }).toEqual({ file, named: true })
      if (!icon) continue
      expect({ icon, alreadyWornBy: seen.get(icon) ?? null }).toEqual({ icon, alreadyWornBy: null })
      seen.set(icon, file)
    }
  })

  it('falls back to the kind, never to one mark for both', () => {
    expect(workerFace(undefined, true)).toBe('userRound')
    expect(workerFace(undefined, false)).toBe('bot')
    expect(workerFace('', true)).toBe('userRound')
    // A name the build does not have is "none", not an empty square.
    expect(workerFace('notAnIcon', true)).toBe('userRound')
    expect(workerFace('notAnIcon', false)).toBe('bot')
    // And a real one wins over both.
    expect(workerFace('search', false)).toBe('search')
  })
})
