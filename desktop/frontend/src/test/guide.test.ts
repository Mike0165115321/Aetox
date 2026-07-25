import { describe, it, expect, beforeEach } from 'vitest'
import { cockpit, pushGuideExchange } from '../lib/stores/cockpit.svelte'
import { setLocale, t } from '../lib/i18n.svelte'

const TOPIC_KEYS = ['guide.q1', 'guide.q2', 'guide.q3', 'guide.q4', 'guide.q5', 'guide.q6'] as const

beforeEach(() => {
  cockpit.chat = []
  setLocale('th')
})

describe('Aetox guide', () => {
  it('appends the exchange without going near a model', () => {
    pushGuideExchange('คำถาม', 'คำตอบ')
    expect(cockpit.chat).toHaveLength(2)
    expect(cockpit.chat[0]).toMatchObject({ role: 'user', text: 'คำถาม' })
    expect(cockpit.chat[1]).toMatchObject({ role: 'agent', text: 'คำตอบ' })
  })

  // The whole reason the guide lives in the locale files: the owner's
  // requirement was that canned answers follow the UI language.
  it('every topic has a question and an answer in both languages', () => {
    for (const locale of ['th', 'en'] as const) {
      setLocale(locale)
      for (const key of TOPIC_KEYS) {
        const q = t(key)
        const a = t(key.replace('.q', '.a') as typeof key)
        expect(q, `${locale} ${key}`).toBeTruthy()
        expect(q.startsWith('guide.'), `${locale} ${key} is a missing key`).toBe(false)
        expect(a.startsWith('guide.'), `${locale} answer for ${key} is a missing key`).toBe(false)
        expect(a.length, `${locale} answer for ${key} is too short to be an answer`).toBeGreaterThan(80)
      }
    }
  })

  it('switching language switches the answers, not just the labels', () => {
    setLocale('th')
    const thai = t('guide.a1')
    setLocale('en')
    const english = t('guide.a1')
    expect(english).not.toBe(thai)
    expect(thai).toMatch(/[฀-๿]/)      // contains Thai
    expect(english).not.toMatch(/[฀-๿]/) // and the English one does not
  })

  it('the skills-vs-presets answer says who invokes each, since that is the actual difference', () => {
    setLocale('en')
    const a = t('guide.a1').toLowerCase()
    expect(a).toContain('you invoke')
    expect(a).toContain('the model invokes')
  })
})
