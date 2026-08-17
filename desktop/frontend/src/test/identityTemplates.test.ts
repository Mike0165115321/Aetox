// The four starting files Settings offers when the identity folder is empty.
//
// What is pinned here is not their wording — it is that they follow the
// language the user picked. They were Thai string literals in identity.svelte.ts
// while every other user-facing string was in the locale files, which made this
// the one screen that ignored the choice: someone who picked English opened the
// file where they write down who they are and found Thai in it.
//
// The filenames are asserted to be the SAME in every language on purpose. They
// are addresses the engine reads (internal/prompt folds every *.md in the
// identity dir into every prompt), not labels, and translating one would quietly
// create a second file instead of opening the first.
import { describe, it, expect, afterEach } from 'vitest'
import { identityTemplates } from '../lib/identity.svelte'
import { setLocale } from '../lib/i18n.svelte'

const names = () => identityTemplates().map((t) => t.name)

afterEach(() => setLocale('th'))

describe('identity templates', () => {
  it('are written in the language the user picked', () => {
    setLocale('th')
    const thai = identityTemplates()
    setLocale('en')
    const english = identityTemplates()

    expect(thai).toHaveLength(4)
    expect(english).toHaveLength(4)

    for (const [i, tpl] of english.entries()) {
      expect(tpl.content).not.toMatch(/[ก-๙]/)
      expect(tpl.content).not.toBe(thai[i].content)
      // Still a markdown file with something to fill in, not an empty string
      // that would write a blank file into the user's folder.
      expect(tpl.content.startsWith('# ')).toBe(true)
      expect(tpl.content.trim().length).toBeGreaterThan(20)
    }
    for (const tpl of thai) {
      expect(tpl.content).toMatch(/[ก-๙]/)
    }
  })

  it('keep the same filenames in every language', () => {
    setLocale('th')
    const thai = names()
    setLocale('en')
    expect(names()).toEqual(thai)
    expect(thai).toEqual(['identity.md', 'thinking.md', 'context.md', 'skills.md'])
  })
})
