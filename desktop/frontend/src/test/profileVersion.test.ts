// The version row in the profile menu: which Aetox this is, and whether a
// newer one is out.
//
// It exists because the answer used to live only in Settings → About, a page
// most people never open — so "a new release exists" was news that reached
// whoever went looking for it and nobody else. This is a third VIEW of the one
// update state (selfUpdate.svelte, ARCHITECTURE.md §107), never a second copy:
// it must never ask GitHub on its own terms, and must never be able to name a
// release the About page has not heard of.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, waitFor, fireEvent } from '@testing-library/svelte'
import Sidebar from '../lib/Sidebar.svelte'
import {
  AppVersion, CheckForUpdate, StageUpdate, RestartToUpdate, ProviderAccountFor,
  CurrentSessionID, SessionMode,
} from './mocks/wailsApp'
import { updater } from '../lib/selfUpdate.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setShell } from '../lib/shell.svelte'

const status = (over: Record<string, unknown> = {}) => ({
  current: '1.5.8', latest: '1.5.9', available: true, disabled: false,
  channel: 'portable', hint: '', url: 'https://example.invalid/releases/v1.5.9',
  checkedAt: '2026-08-26T00:00:00Z', publishedAt: '2026-08-26T00:00:00Z', canAuto: true, ...over,
})

const openMenu = async () => {
  const { container } = render(Sidebar, { onOpenSettings: () => {} })
  ;(container.querySelector('.side-footer') as HTMLElement).click()
  await waitFor(() => expect(container.querySelector('.ver-menu')).toBeTruthy())
  return container
}

const text = (container: HTMLElement, sel: string) =>
  (container.querySelector(sel) as HTMLElement | null)?.textContent?.trim() ?? ''

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.activeView = 'chat'
  cockpit.history.length = 0
  setShell('assistant')
  vi.mocked(CurrentSessionID).mockResolvedValue('20260826-120000.000')
  vi.mocked(SessionMode).mockResolvedValue('')
  vi.mocked(ProviderAccountFor).mockResolvedValue(null as never)
  // The update state outlives any one component the way it outlives the menu
  // being closed, so every test has to start from a machine that is not
  // mid-update and has not asked anybody anything yet.
  Object.assign(updater, {
    current: '', status: null, announced: false, checking: false, checkError: '',
    dismissed: false, phase: 'idle', done: 0, total: 0, staged: '', error: '',
  })
})

describe('the version row in the profile menu', () => {
  it('names the running build, from Go rather than from a literal', async () => {
    vi.mocked(AppVersion).mockResolvedValue('1.5.8' as never)
    const container = await openMenu()

    await waitFor(() => expect(text(container, '.ver-name')).toBe('Aetox v1.5.8'))
  })

  // The daily check (update_notify.go) is what normally keeps this current;
  // this covers the gap before its first answer lands. Once per run, not once
  // per open — a menu is opened all day and GitHub is asked 60 times an hour.
  it('asks once when nothing is known yet, and not again on the next open', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(status({ available: false, latest: '1.5.8' }) as never)
    const container = await openMenu()

    await waitFor(() => expect(vi.mocked(CheckForUpdate)).toHaveBeenCalledTimes(1))
    ;(container.querySelector('.side-footer') as HTMLElement).click()
    ;(container.querySelector('.side-footer') as HTMLElement).click()
    await waitFor(() => expect(container.querySelector('.ver-menu')).toBeTruthy())

    expect(vi.mocked(CheckForUpdate)).toHaveBeenCalledTimes(1)
    expect(text(container, '.ver-note')).toBe('ใช้เวอร์ชันล่าสุดอยู่แล้ว')
  })

  // The whole point of the row: the news arrives where people already look,
  // and the act is one click from it.
  it('shows a newer release and downloads it on one click, then offers the restart', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(status() as never)
    const container = await openMenu()

    await waitFor(() => expect(text(container, '.ver-new')).toBe('มี Aetox v1.5.9 แล้ว'))

    await fireEvent.click(container.querySelector('.ver-go') as HTMLElement)
    await waitFor(() => expect(vi.mocked(StageUpdate)).toHaveBeenCalled())
    // Downloading is not restarting. The window closes when the user says so.
    expect(vi.mocked(RestartToUpdate)).not.toHaveBeenCalled()

    await waitFor(() => expect(text(container, '.ver-new')).toBe('v1.5.9 พร้อมแล้ว'))
    await fireEvent.click(container.querySelector('.ver-go') as HTMLElement)
    await waitFor(() => expect(vi.mocked(RestartToUpdate)).toHaveBeenCalled())
  })

  // Scoop installed us, so Scoop upgrades us: Aetox does not write into
  // someone else's package directory. internal/update already made that call —
  // this row must not second-guess it with a download button.
  it('offers the command instead of a download when the channel owns the files', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(
      status({ channel: 'scoop', hint: 'scoop update aetox', canAuto: false }) as never)
    const container = await openMenu()

    await waitFor(() => expect(text(container, '.ver-cmd')).toBe('scoop update aetox'))
    expect(container.querySelector('.ver-go')).toBeNull()
    expect(vi.mocked(StageUpdate)).not.toHaveBeenCalled()
  })

  // Offline is not "no update" and it is certainly not "the app is broken".
  it('says the check could not run, without claiming anything about releases', async () => {
    vi.mocked(CheckForUpdate).mockRejectedValue(new Error('dial tcp: no such host'))
    const container = await openMenu()

    await waitFor(() => expect(text(container, '.ver-note')).toBe('ตอนนี้ตรวจไม่ได้ ลองใหม่อีกครั้งได้'))
    expect(container.querySelector('.ver-new')).toBeNull()
  })

  // A Store install cannot update itself and must never be sent to the GitHub
  // releases page — the files there install a SECOND copy beside it.
  it('says so when checking is switched off, and keeps the channel’s own page', async () => {
    vi.mocked(CheckForUpdate).mockResolvedValue(
      status({ available: false, disabled: true, latest: '', channel: 'store',
        url: 'https://apps.microsoft.com/detail/9N4KKBRRSCZZ' }) as never)
    const container = await openMenu()

    await waitFor(() => expect(text(container, '.ver-note')).toBe('ปิดการตรวจอัปเดตไว้'))
    expect(container.querySelector('.ver-new')).toBeNull()
  })
})
