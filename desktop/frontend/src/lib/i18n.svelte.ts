// Minimal hand-rolled i18n: no dependency, because all we need is "look up a
// string by key for the current locale." th.ts is the source of truth for
// keys (TKey = keyof typeof th) — every other locale file is typed against it.
// Add a new language by adding one locales/<code>.ts file + one line below.

import { th } from './locales/th'
import { en } from './locales/en'
import { zh } from './locales/zh'
import { SetUILocale } from '../../wailsjs/go/main/App'

export type TKey = keyof typeof th

/** The name a language is called in that language, and its strings. */
type LocaleEntry<D> = { name: string; dict: D }

// Two tiers, and the difference between them is who wrote the words.
//
// **Owned** are the languages the owner writes and can read back. They are
// typed complete, so adding a key to th.ts and forgetting en.ts is a compile
// error — the rule that has kept the two honest since there were two.
//
// **Contributed** are the rest: translated by someone else, or by a machine,
// and never verifiable by the person shipping them. They are typed partial ON
// PURPOSE. Holding them to complete would mean every new button in Aetox
// breaks the build until eight translations arrive, which does not produce
// eight translations — it produces a placeholder pasted in to make the build
// go green, and a placeholder is a lie that type-checks.
//
// A key missing from a contributed language falls through to English (see `t`),
// which is a visible gap in a language the reader probably knows, rather than
// an invented string in a language nobody checked.
const owned = {
  th: { name: 'ไทย', dict: th },
  en: { name: 'English', dict: en },
} satisfies Record<string, LocaleEntry<Record<TKey, string>>>

// Names are in the language's own language on purpose: someone who can only
// read Chinese has to be able to find their way out of a screen in Thai.
//
// `zh` and not `zh-Hans` or `zh-CN`: only Simplified is shipped, and the
// resolver folds zh-CN / zh-SG / zh-TW down to it. If Traditional ever arrives
// these become `zh-Hans` / `zh-Hant` — which is why the resolver matches the
// whole tag before the language half (§125.5) rather than cutting at the dash.
const contributed = {
  zh: { name: '简体中文', dict: zh },
} satisfies Record<string, LocaleEntry<Partial<Record<TKey, string>>>>

/** Every language the app has, owned and contributed alike. The one list. */
export const locales: Record<string, LocaleEntry<Partial<Record<TKey, string>>>> = { ...owned, ...contributed }

/** Which tier a language is in — the contributed ones are what the guard test policies. */
export const contributedLocales = Object.keys(contributed)

export type Locale = keyof typeof owned | keyof typeof contributed

export const localeNames = Object.fromEntries(
  Object.entries(locales).map(([code, l]) => [code, l.name]),
) as Record<Locale, string>

const STORAGE_KEY = 'aetox-locale'

export const i18n = $state<{ locale: Locale }>({ locale: 'th' })

export function setLocale(locale: Locale): void {
  i18n.locale = locale
  localStorage.setItem(STORAGE_KEY, locale)
  tellEngine(locale)
  tellDocument(locale)
}

/** Call once before mount so nothing flashes in the wrong language. */
export function initLocale(): void {
  const saved = localStorage.getItem(STORAGE_KEY)
  i18n.locale = saved && saved in locales ? (saved as Locale) : 'th'
  // Push it down on every start too, so the engine's copy self-heals if the
  // two ever drift (fresh install, preference file restored from elsewhere).
  tellEngine(i18n.locale)
  tellDocument(i18n.locale)
}

// The engine needs the language for exactly one thing: Aetox's own built-in
// provider, which is an onboarding surface rather than a model and has to talk
// to the user in their language (ARCHITECTURE.md §40). Failing to reach it is
// never worth breaking the UI over — the built-in falls back to Thai.
function tellEngine(locale: Locale): void {
  void SetUILocale(locale)?.catch?.(() => {})
}

// index.html ships with lang="th" because a document has to say something
// before any script runs. Leaving it there was a second place that decided the
// language and never changed its mind — the wrong answer for every user who
// picked anything else, and what the browser hands to hyphenation, spellcheck
// and a screen reader choosing a voice.
function tellDocument(locale: Locale): void {
  document.documentElement.lang = locale
}

/**
 * Look up `key` in the active locale, with optional {var} substitution.
 *
 * Falls through to English, not to Thai. Thai was the fallback while it was
 * the only other language there was, which made it right by accident; with a
 * contributed language it becomes wrong on sight — a Spanish user meeting a
 * key nobody translated yet would get Thai, a script they cannot even sound
 * out, while the English is sitting right there complete.
 */
export function t(key: TKey, vars?: Record<string, string | number>): string {
  let str = locales[i18n.locale]?.dict[key] ?? owned.en.dict[key] ?? key
  if (vars) {
    for (const [k, v] of Object.entries(vars)) str = str.replaceAll(`{${k}}`, String(v))
  }
  return str
}
