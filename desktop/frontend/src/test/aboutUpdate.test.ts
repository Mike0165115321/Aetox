// The About page's one-click update — the VS Code shape: press, download,
// verify, restart into the new build. The page's job is to offer the right
// action per channel: the auto button only when the engine says it can finish
// the job (canAuto), the Scoop command for Scoop, the release page for the
// rest — and to say what went wrong without pretending the app broke.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Settings from '../lib/Settings.svelte'
import { CheckForUpdate, ApplyUpdate } from './mocks/wailsApp'

const status = (over: Record<string, unknown>) => ({
  current: '0.9.2', latest: '0.9.3', available: true, disabled: false,
  channel: 'portable', hint: '', url: 'https://example.invalid/releases/v0.9.3',
  checkedAt: '2026-08-09T00:00:00Z', canAuto: true, ...over,
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
})

describe('About: one-click update', () => {
  it('offers update-and-restart when the engine can finish the job', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(status({}) as never)
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    const button = await screen.findByText('อัปเดตแล้วรีสตาร์ท')
    await fireEvent.click(button)

    await waitFor(() => expect(vi.mocked(ApplyUpdate)).toHaveBeenCalled())
    // The app is about to close itself; the button's last words say so.
    expect(await screen.findByText('กำลังรีสตาร์ท…')).toBeTruthy()
  })

  it('keeps the Scoop command for Scoop — that directory is not ours to write', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(
      status({ channel: 'scoop', hint: 'scoop update aetox', canAuto: false }) as never)
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    expect(await screen.findByText('scoop update aetox')).toBeTruthy()
    expect(screen.queryByText('อัปเดตแล้วรีสตาร์ท')).toBeNull()
    expect(vi.mocked(ApplyUpdate)).not.toHaveBeenCalled()
  })

  it('says what failed and re-arms the button, instead of a dead click', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(status({}) as never)
    vi.mocked(ApplyUpdate).mockRejectedValue(
      new Error('แฮชของไฟล์ไม่ตรงกับ checksums.txt — ไฟล์อาจเสียหรือถูกแก้ ระบบไม่ติดตั้งต่อ'))
    const { container } = render(Settings, { onClose: () => {} })
    await openAbout(container)

    await fireEvent.click(await screen.findByText('อัปเดตแล้วรีสตาร์ท'))

    await waitFor(() => expect(screen.getByText(/แฮชของไฟล์ไม่ตรง/)).toBeTruthy())
    // The failure left the old build untouched, so trying again is legitimate.
    const button = screen.getByText('อัปเดตแล้วรีสตาร์ท').closest('button')
    expect(button?.disabled).toBe(false)
  })
})
