// The balance in the profile menu, from the outside.
//
// One row, for the provider in use. Listing every account that happened to
// report a figure put a balance next to a provider the user was not talking
// to, which reads as the app naming the wrong engine — worse than showing no
// balance at all. Every other account is a question the Settings page exists
// to answer.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor } from '@testing-library/svelte'
import Sidebar from '../lib/Sidebar.svelte'
import { ProviderAccountFor, CurrentSessionID, SessionMode } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setShell } from '../lib/shell.svelte'

const money = (provider: string, amount: number) => ({
  provider,
  balance: {
    kind: 'money', hasAmount: true, amount, currency: 'USD',
    parts: [{ label: 'toppedUp', amount }], sufficient: true,
    fetchedAt: new Date().toISOString(),
  },
  quotas: [], quotaKnown: true, expectsQuota: false, error: '',
})

const subscription = (provider: string, quotas: any[] = [], quotaKnown = false) => ({
  provider,
  balance: {
    kind: 'subscription', hasAmount: false, amount: 0, currency: '',
    parts: [], sufficient: true, fetchedAt: new Date().toISOString(),
  },
  quotas, quotaKnown, expectsQuota: true, error: '',
})

const openMenu = async () => {
  const { container } = render(Sidebar, { onOpenSettings: () => {} })
  ;(container.querySelector('.side-footer') as HTMLElement).click()
  await waitFor(() => expect(container.querySelector('.acct-menu')).toBeTruthy())
  return container
}

const rows = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('.acct-menu-name')).map((e) => e.textContent?.trim() ?? '')

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.activeView = 'chat'
  cockpit.history.length = 0
  setShell('assistant')
  vi.mocked(CurrentSessionID).mockResolvedValue('20260814-120000.000')
  vi.mocked(SessionMode).mockResolvedValue('')
})

describe('the balance in the profile menu', () => {
  it('asks about the provider in use, and shows only that one', async () => {
    cockpit.model.provider = 'codex'
    vi.mocked(ProviderAccountFor).mockResolvedValue(subscription('codex') as any)

    const container = await openMenu()

    expect(vi.mocked(ProviderAccountFor)).toHaveBeenCalledWith('codex')
    expect(rows(container)).toEqual(['codex'])
    // Nothing about any other account, however much credit it is holding.
    expect(container.textContent).not.toContain('deepseek')
  })

  // A name with nothing under it reads as a number that failed to load, so the
  // row says why there is none and what makes one appear.
  it('says why the in-use provider has no number yet', async () => {
    cockpit.model.provider = 'codex'
    vi.mocked(ProviderAccountFor).mockResolvedValue(subscription('codex') as any)

    const container = await openMenu()

    expect(container.textContent).toContain('ยังไม่รู้ลิมิต')
  })

  it('shows the window once a turn has stated one', async () => {
    cockpit.model.provider = 'codex'
    vi.mocked(ProviderAccountFor).mockResolvedValue(subscription('codex', [
      { window: 'week', remainingPercent: 12, resetAt: '', observedAt: new Date().toISOString() },
    ], true) as any)

    const container = await openMenu()

    expect(container.textContent).toContain('สัปดาห์นี้')
    expect(container.textContent).toContain('เหลือ 12%')
  })

  it('shows the figure when the provider in use keeps a balance', async () => {
    cockpit.model.provider = 'deepseek'
    vi.mocked(ProviderAccountFor).mockResolvedValue(money('deepseek', 1.45) as any)

    const container = await openMenu()

    expect(rows(container)).toEqual(['deepseek'])
    expect(container.textContent).toContain('$1.45')
  })
})
