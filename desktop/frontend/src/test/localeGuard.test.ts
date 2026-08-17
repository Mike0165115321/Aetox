// The guard on languages nobody here can read.
//
// Thai and English are checked by a person and by the type system: they are
// typed complete, so a key added to one and forgotten in the other does not
// build. A contributed language has neither protection — it is typed partial on
// purpose (an incomplete translation is better than a placeholder that lies),
// and there is nobody on this side who can look at Vietnamese and tell whether
// it says what it should.
//
// So what is checked here is everything that can be checked WITHOUT reading the
// language: that the keys are real, that nothing is blank, and above all that
// the {placeholders} survived. A translation that drops {count} does not fail,
// it renders "You have  items" to a real user, forever, and no test that only
// counted keys would ever notice.
import { describe, it, expect } from 'vitest'
import { locales, contributedLocales } from '../lib/i18n.svelte'
import { th } from '../lib/locales/th'
import { en } from '../lib/locales/en'

const placeholders = (s: string) => (s.match(/\{[a-zA-Z0-9_]+\}/g) ?? []).sort()

describe('owned locales', () => {
  it('cover exactly the same keys', () => {
    // The type system already says this. Asserted anyway because the type is
    // `Record<keyof typeof th, string>` — it catches a MISSING key and says
    // nothing about an extra one that no screen will ever ask for.
    expect(Object.keys(en).sort()).toEqual(Object.keys(th).sort())
  })

  it('agree on every placeholder', () => {
    for (const key of Object.keys(th) as (keyof typeof th)[]) {
      expect({ key, vars: placeholders(en[key]) }).toEqual({ key, vars: placeholders(th[key]) })
    }
  })
})

describe.each(contributedLocales)('contributed locale: %s', (code) => {
  const dict = locales[code].dict as Record<string, string>

  it('is called something in its own language', () => {
    expect(locales[code].name.trim()).not.toBe('')
  })

  it('defines no key the app does not have', () => {
    // A stray key is a translation of something that no longer exists, or a
    // typo'd one — either way a string nobody will ever see, and a missing
    // translation hiding behind a full-looking file.
    const known = new Set(Object.keys(th))
    for (const key of Object.keys(dict)) expect({ code, key, known: known.has(key) }).toEqual({ code, key, known: true })
  })

  it('has nothing blank', () => {
    // An empty string is worse than an absent key: absent falls through to
    // English, empty renders as nothing at all.
    for (const [key, value] of Object.entries(dict)) expect({ key, empty: value.trim() === '' }).toEqual({ key, empty: false })
  })

  it('keeps every placeholder the English has', () => {
    for (const [key, value] of Object.entries(dict)) {
      const expected = placeholders(en[key as keyof typeof en] ?? '')
      expect({ code, key, vars: placeholders(value) }).toEqual({ code, key, vars: expected })
    }
  })
})
