import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import ProviderAccount from '../lib/ProviderAccount.svelte'

const balance = (over: Record<string, unknown> = {}) => ({
  kind: 'money', hasAmount: true, amount: 12.4, currency: 'USD',
  parts: [], sufficient: true, fetchedAt: new Date().toISOString(), ...over,
})

const account = (over: Record<string, unknown> = {}) => ({
  provider: 'deepseek', balance: balance(), quotas: [], quotaKnown: false, expectsQuota: false, error: '', ...over,
})

describe('provider account line', () => {
  it('shows the figure and its breakdown', () => {
    const { container } = render(ProviderAccount, {
      account: account({
        balance: balance({ parts: [{ label: 'granted', amount: 8 }, { label: 'toppedUp', amount: 4.4 }] }),
      }),
    })
    expect(container.textContent).toContain('$12.40')
    expect(container.textContent).toContain('เครดิตแถม $8.00')
    expect(container.textContent).toContain('เติมเอง $4.40')
  })

  // Being out of credit is not information, it is "the next thing you click
  // will fail" — so it replaces the breakdown rather than sitting beside it.
  it('an empty account warns instead of listing parts', () => {
    const { container } = render(ProviderAccount, {
      account: account({
        balance: balance({ amount: 0, sufficient: false, parts: [{ label: 'granted', amount: 0 }] }),
      }),
    })
    expect(container.textContent).toContain('ยอดไม่พอ')
    expect(container.textContent).not.toContain('เครดิตแถม')
    expect(container.querySelector('.acct-danger')).toBeTruthy()
  })

  it('a local runtime says there is nothing to spend', () => {
    const { container } = render(ProviderAccount, {
      account: account({ provider: 'ollama', balance: balance({ kind: 'free', hasAmount: false }) }),
    })
    expect(container.textContent).toContain('ไม่มีค่าใช้จ่าย')
  })

  // The window label comes from the data. A fixed "this week" would be wrong
  // on every provider whose window is a minute.
  it('labels each window from the provider’s own dialect', () => {
    const { container } = render(ProviderAccount, {
      account: account({
        provider: 'codex',
        balance: balance({ kind: 'subscription', hasAmount: false }),
        quotaKnown: true,
        quotas: [
          { window: '5h', remainingPercent: 66, resetAt: new Date(Date.now() + 2 * 3600_000).toISOString(), observedAt: new Date().toISOString() },
          { window: 'week', remainingPercent: 12, resetAt: new Date(Date.now() + 3 * 86400_000).toISOString(), observedAt: new Date().toISOString() },
        ],
      }),
    })
    expect(container.textContent).toContain('5 ชั่วโมงนี้')
    expect(container.textContent).toContain('เหลือ 66%')
    expect(container.textContent).toContain('สัปดาห์นี้')
    expect(container.textContent).toContain('เหลือ 12%')
    // The nearly-empty window is a warning, the roomy one is a readout.
    const fills = container.querySelectorAll('.acct-fill')
    expect(fills[0].className).toContain('ok')
    expect(fills[1].className).toContain('warn')
  })

  // Never presented as live — it is whatever the last real turn happened to say.
  it('stamps a quota with when it was actually observed', () => {
    const { container } = render(ProviderAccount, {
      account: account({
        quotaKnown: true,
        quotas: [{ window: 'minute', remainingPercent: 71, resetAt: '', observedAt: new Date().toISOString() }],
      }),
    })
    expect(container.textContent).toContain('จากการคุยครั้งล่าสุด')
  })

  // The compact form is for the profile menu, which is for glancing. A sentence
  // explaining why there is no number is a Settings job.
  it('compact drops the explanatory blanks but keeps real figures', () => {
    const explained = render(ProviderAccount, {
      account: account({ provider: 'openai', balance: balance({ kind: 'web-only', hasAmount: false }) }),
      compact: true,
    })
    expect(explained.container.textContent).not.toContain('ดูยอดได้ในหน้าเว็บ')

    const figure = render(ProviderAccount, { account: account(), compact: true })
    expect(figure.container.textContent).toContain('$12.40')
  })

  // It was printed twice on the Codex card: once by the blank-explanation
  // branch and once by the timestamp branch, which both decided on their own
  // that a quota was missing.
  it('says the limit is unknown exactly once', () => {
    const { container } = render(ProviderAccount, {
      account: account({
        provider: 'codex',
        balance: balance({ kind: 'subscription', hasAmount: false }),
        expectsQuota: true,
      }),
    })
    const said = container.textContent?.match(/ยังไม่รู้ลิมิต/g) ?? []
    expect(said.length).toBe(1)
    // And not alongside a caption for a bar that is not there.
    expect(container.textContent).not.toContain('เป็นซับสคริปชัน')
  })

  // DeepSeek bills per token and has no window at all, so "the limit is not
  // known yet" would be waiting for a number that is never coming. The catalog
  // decides this, not the shape of the balance.
  it('stays quiet about limits for a provider that has none', () => {
    const { container } = render(ProviderAccount, { account: account({ expectsQuota: false }) })
    expect(container.textContent).not.toContain('ยังไม่รู้ลิมิต')
  })

  // A failed fetch is a third kind of blank: the provider should have answered
  // and did not, which is not the same as having nothing to report.
  it('a failed fetch says so rather than showing zero', () => {
    const { container } = render(ProviderAccount, {
      account: account({ balance: balance({ hasAmount: false, amount: 0 }), error: 'deepseek: status 401' }),
    })
    expect(container.textContent).toContain('อ่านยอดไม่ได้')
    expect(container.textContent).not.toContain('$0.00')
  })
})
