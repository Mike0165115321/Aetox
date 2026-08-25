// The About page's door into self-update. The page's job is to offer the right
// action per channel — the auto button only when the engine says it can finish
// the job (canAuto), the Scoop command for Scoop, the release page for the
// rest — and to say what went wrong without pretending the app broke.
//
// It is a second VIEW of the update, never a second copy of it: the store both
// doors drive is what says whether a download is running, done, or refused
// (selfUpdate.svelte, ARCHITECTURE.md §107).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import { CheckForUpdate, StageUpdate, RestartToUpdate, AppVersion, RecentDebugLog } from './mocks/wailsApp'
import { BrowserOpenURL } from './mocks/wailsRuntime'
import { updater } from '../lib/selfUpdate.svelte'

const status = (over: Record<string, unknown>) => ({
  current: '0.9.2', latest: '0.9.3', available: true, disabled: false,
  channel: 'portable', hint: '', url: 'https://example.invalid/releases/v0.9.3',
  checkedAt: '2026-08-09T00:00:00Z', publishedAt: '2026-08-08T00:00:00Z', canAuto: true, ...over,
})

const openAbout = async (container: HTMLElement) => {
  const item = Array.from(container.querySelectorAll('.settings-nav-item'))
    .find((el) => el.textContent?.includes('เกี่ยวกับ Aetox'))
  if (!item) throw new Error('About nav item not found')
  await fireEvent.click(item)
  await fireEvent.click(screen.getByText('ตรวจหาการอัปเดต'))
}

beforeEach(() => {
  vi.clearAllMocks()
  // "An update is being installed" is one fact about the machine, not about a
  // component — so it lives in a module-level store both doors read
  // (selfUpdate.svelte), and it outlives a render the way it outlives the page
  // being closed. Which is exactly right in the app and exactly why each test
  // has to start from a machine that is not mid-update.
  Object.assign(updater, {
    status: null, dismissed: false, phase: 'idle', done: 0, total: 0, staged: '', error: '',
  })
})

describe('About: the update door', () => {
  // Download first, restart second — and the button changes to say so. The
  // page must never be the thing that closes the window on its own.
  it('downloads on the first press and offers the restart on the second', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(status({}) as never)
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    await fireEvent.click(await screen.findByText('ดาวน์โหลดอัปเดต'))

    await waitFor(() => expect(vi.mocked(StageUpdate)).toHaveBeenCalled())
    expect(vi.mocked(RestartToUpdate)).not.toHaveBeenCalled()

    await fireEvent.click(await screen.findByText('รีสตาร์ทเพื่ออัปเดต'))
    await waitFor(() => expect(vi.mocked(RestartToUpdate)).toHaveBeenCalled())
  })

  it('keeps the Scoop command for Scoop — that directory is not ours to write', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(
      status({ channel: 'scoop', hint: 'scoop update aetox', canAuto: false }) as never)
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    expect(await screen.findByText('scoop update aetox')).toBeTruthy()
    expect(screen.queryByText('ดาวน์โหลดอัปเดต')).toBeNull()
    expect(vi.mocked(StageUpdate)).not.toHaveBeenCalled()
  })

  // A bug in Aetox goes to the developer as an issue the user submits — the
  // button only opens the prefilled form in their own browser. What rides
  // along: version, OS, and the app's recent internal log (the evidence the
  // user should not have to hunt for). Nothing is sent by the app itself.
  it('prefills the GitHub issue form with version, OS and recent log — and sends nothing', async () => {
    vi.mocked(AppVersion).mockResolvedValue('9.9.9' as never)
    vi.mocked(CheckForUpdate).mockResolvedValue(status({}) as never)
    vi.mocked(RecentDebugLog).mockResolvedValue(['[01:02:03.000] terminal resize replayed the screen'] as never)
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    await fireEvent.click(await screen.findByText('แจ้งปัญหา'))

    await waitFor(() => expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledTimes(1))
    const url = vi.mocked(BrowserOpenURL).mock.calls[0][0] as string
    expect(url.startsWith('https://github.com/Mikedev115/Aetox/issues/new?body=')).toBe(true)
    const body = decodeURIComponent(url.split('body=')[1])
    expect(body).toContain('v9.9.9')
    expect(body).toMatch(/Windows|macOS|Linux/)
    expect(body).toContain('terminal resize replayed the screen')

    // Feedback is the second door to the same place — different opening hint,
    // and no log: an opinion needs no evidence attached.
    await fireEvent.click(screen.getByText('ส่งความคิดเห็น'))
    await waitFor(() => expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledTimes(2))
    const fb = decodeURIComponent((vi.mocked(BrowserOpenURL).mock.calls[1][0] as string).split('body=')[1])
    expect(fb).toContain('v9.9.9')
    expect(fb).not.toContain('terminal resize replayed the screen')
  })

  it('says what failed and re-arms the button, instead of a dead click', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(status({}) as never)
    vi.mocked(StageUpdate).mockRejectedValue(
      new Error('แฮชของไฟล์ไม่ตรงกับ checksums.txt — ไฟล์อาจเสียหรือถูกแก้ ระบบไม่ติดตั้งต่อ'))
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    await fireEvent.click(await screen.findByText('ดาวน์โหลดอัปเดต'))

    await waitFor(() => expect(screen.getByText(/แฮชของไฟล์ไม่ตรง/)).toBeTruthy())
    // The failure left the running build untouched, so trying again is
    // legitimate — and it is a download to retry, not a restart.
    const button = screen.getByText('ดาวน์โหลดอัปเดต').closest('button')
    expect(button?.disabled).toBe(false)
  })
})
