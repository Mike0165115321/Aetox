// Minimal hand-rolled i18n: no dependency, because all we need is "look up a
// string by key for the current locale." th.ts is the source of truth for
// keys (TKey = keyof typeof th) — every other locale file is typed against
// it, so a missing translation is a compile error, not a silent fallback.
// Add a new language by adding one locales/<code>.ts file + one line here.

import { th } from './locales/th'
import { en } from './locales/en'

export type Locale = 'th' | 'en'
export type TKey = keyof typeof th

export const localeNames: Record<Locale, string> = { th: 'ไทย', en: 'English' }

const dictionaries: Record<Locale, Record<TKey, string>> = { th, en }
const STORAGE_KEY = 'aetox-locale'

export const i18n = $state<{ locale: Locale }>({ locale: 'th' })

export function setLocale(locale: Locale): void {
  i18n.locale = locale
  localStorage.setItem(STORAGE_KEY, locale)
}

/** Call once before mount so nothing flashes in the wrong language. */
export function initLocale(): void {
  const saved = localStorage.getItem(STORAGE_KEY)
  i18n.locale = saved === 'en' ? 'en' : 'th'
}

/** Look up `key` in the active locale, falling back to th, with optional {var} substitution. */
export function t(key: TKey, vars?: Record<string, string | number>): string {
  let str = dictionaries[i18n.locale][key] ?? dictionaries.th[key] ?? key
  if (vars) {
    for (const [k, v] of Object.entries(vars)) str = str.replaceAll(`{${k}}`, String(v))
  }
  return str
}
