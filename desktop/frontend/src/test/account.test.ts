// The Aetox account page.
//
// What is worth holding here is not that a button calls a binding — it is the
// two promises the copy makes. The page must render with no session and no
// network, and it must not imply that signing in unlocks something, because
// today it does not.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import {
  AccountStatus, StartAccountSignIn, CompleteAccountSignIn, AccountSignOut,
} from './mocks/wailsApp'
import { BrowserOpenURL } from './mocks/wailsRuntime'

// `configured: true` throughout — these are the tests of the page itself, and
// the page only exists on a build with an id server behind it. The closed case
// is its own test at the bottom.
const signedOut = {
  configured: true, signed_in: false, user: {}, display: '',
  providers: ['github', 'google'], server: 'http://localhost:8080',
}
const signedIn = {
  configured: true, signed_in: true, user: { id: 'u1', name: 'Mike', email: 'mike@example.com' },
  display: 'Mike', providers: ['github', 'google'], server: 'http://localhost:8080',
}

// Waits, because the row only appears once the engine has answered whether
// this build has an id server at all. That round trip is the feature switch,
// so a test that clicked before it landed would be testing the closed state by
// accident.
const openAccount = async (container: HTMLElement) => {
  let item: Element | undefined
  await waitFor(() => {
    item = Array.from(container.querySelectorAll('.settings-nav-item'))
      .find((el) => el.textContent?.includes('บัญชี Aetox'))
    if (!item) throw new Error('the account page is not in the settings nav')
  })
  await fireEvent.click(item!)
}

beforeEach(() => {
  vi.mocked(AccountStatus).mockResolvedValue(signedOut as any)
})

describe('Aetox account page', () => {
  it('offers both doors and says plainly that signing in unlocks nothing yet', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openAccount(container)

    await waitFor(() => expect(screen.getByText('ยังไม่ได้เข้าสู่ระบบ')).toBeTruthy())
    expect(screen.getByText('เข้าสู่ระบบด้วย GitHub')).toBeTruthy()
    expect(screen.getByText('เข้าสู่ระบบด้วย Google')).toBeTruthy()
    // The honest line. If the store ever does exist, this assertion is the
    // thing that should fail and force the copy to be rewritten.
    expect(screen.getByText(/ยังไม่ปลดล็อกอะไร/)).toBeTruthy()
    // Which server holds the account is never left to be guessed at.
    expect(screen.getByText('http://localhost:8080')).toBeTruthy()
  })

  it('opens the browser at the URL the engine handed back, and stores nothing until it returns', async () => {
    const { container } = render(Settings, { onClose: () => {} })
    await openAccount(container)
    await waitFor(() => expect(screen.getByText('เข้าสู่ระบบด้วย GitHub')).toBeTruthy())

    await fireEvent.click(screen.getByText('เข้าสู่ระบบด้วย GitHub'))

    await waitFor(() => expect(vi.mocked(StartAccountSignIn).mock.calls.length).toBe(1))
    expect(vi.mocked(StartAccountSignIn).mock.calls[0][0]).toBe('github')
    expect(vi.mocked(BrowserOpenURL).mock.calls.at(-1)?.[0]).toContain('/authorize')
    // Two calls, not one: the second is what blocks on the browser coming back.
    await waitFor(() => expect(vi.mocked(CompleteAccountSignIn).mock.calls.length).toBe(1))
    await waitFor(() => expect(screen.getByText('Mike')).toBeTruthy())
  })

  it('shows who is signed in, and can sign out', async () => {
    vi.mocked(AccountStatus).mockResolvedValue(signedIn as any)
    const { container } = render(Settings, { onClose: () => {} })
    await openAccount(container)

    await waitFor(() => expect(screen.getByText('Mike')).toBeTruthy())
    expect(screen.getByText('mike@example.com')).toBeTruthy()

    vi.mocked(AccountStatus).mockResolvedValue(signedOut as any)
    await fireEvent.click(screen.getByText('ออกจากระบบ'))
    await waitFor(() => expect(vi.mocked(AccountSignOut).mock.calls.length).toBe(1))
    await waitFor(() => expect(screen.getByText('ยังไม่ได้เข้าสู่ระบบ')).toBeTruthy())
  })

  it('says the sign-out only half happened when the server could not be told', async () => {
    vi.mocked(AccountStatus).mockResolvedValue(signedIn as any)
    vi.mocked(AccountSignOut).mockRejectedValueOnce(new Error('dial tcp: no route to host'))
    const { container } = render(Settings, { onClose: () => {} })
    await openAccount(container)
    await waitFor(() => expect(screen.getByText('Mike')).toBeTruthy())

    vi.mocked(AccountStatus).mockResolvedValue(signedOut as any)
    await fireEvent.click(screen.getByText('ออกจากระบบ'))

    // Signed out here either way — the message is about the server, and it
    // says so rather than reporting a clean sign-out or a failed one.
    await waitFor(() => expect(screen.getByText(/แต่บอกเซิร์ฟเวอร์ไม่ได้/)).toBeTruthy())
    expect(screen.getByText('ยังไม่ได้เข้าสู่ระบบ')).toBeTruthy()
  })

  // The state every shipped build is in today. Not a greyed-out row and not a
  // "soon" badge: nothing is deployed behind it, so the page is not in the nav
  // at all, and the settings search cannot find it either.
  it('is not in the settings nav on a build with no id server', async () => {
    vi.mocked(AccountStatus).mockResolvedValue({
      configured: false, signed_in: false, user: {}, display: '',
      providers: ['github', 'google'], server: '',
    } as any)
    const { container } = render(Settings, { onClose: () => {} })
    await waitFor(() => expect(vi.mocked(AccountStatus).mock.calls.length).toBeGreaterThan(0))

    const labels = Array.from(container.querySelectorAll('.settings-nav-item'))
      .map((el) => el.textContent?.trim())
    expect(labels.some((l) => l?.includes('บัญชี Aetox'))).toBe(false)
  })
})
