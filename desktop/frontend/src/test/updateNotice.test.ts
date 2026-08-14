// The half of self-update the user actually meets: being told, and being
// shown. The engine could already download, verify and swap the whole app —
// but it only ever ran when someone went digging in Settings, and the only
// sign of life was a word on a button. This covers the notice that arrives on
// its own and the dialog that replaces it once the user says yes.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Updater from '../lib/Updater.svelte'
import { updater, listenForUpdates, startUpdate } from '../lib/selfUpdate.svelte'
import { ApplyUpdate } from './mocks/wailsApp'
import { EventsOn, BrowserOpenURL } from './mocks/wailsRuntime'

const status = (over: Record<string, unknown> = {}) => ({
  current: '0.9.6', latest: '0.9.7', available: true, disabled: false,
  channel: 'portable', hint: '', url: 'https://example.invalid/releases/v0.9.7',
  checkedAt: '2026-08-14T00:00:00Z', canAuto: true, ...over,
})

/** Drives the Go-side event the automatic check emits. */
function announce(st: Record<string, unknown>) {
  const off = listenForUpdates()
  const call = vi.mocked(EventsOn).mock.calls.find((c) => c[0] === 'update:available')
  if (!call) throw new Error('nothing subscribed to update:available')
  ;(call[1] as (st: unknown) => void)(st)
  return off
}

function reportProgress(done: number, total: number) {
  const call = vi.mocked(EventsOn).mock.calls.find((c) => c[0] === 'update:progress')
  if (!call) throw new Error('nothing subscribed to update:progress')
  ;(call[1] as (p: { done: number; total: number }) => void)({ done, total })
}

beforeEach(() => {
  vi.clearAllMocks()
  Object.assign(updater, {
    status: null, dismissed: false, applying: false, pct: -1, restarting: false, error: '',
  })
})

describe('the notice', () => {
  // Nothing on screen until there is something to say. This component is
  // mounted for the whole life of the app.
  it('shows nothing at all until a newer release turns up', () => {
    const { container } = render(Updater)
    expect(container.querySelector('.upd-notice')).toBeNull()
    expect(container.querySelector('.upd-overlay')).toBeNull()
  })

  it('offers the update once the check announces one — without having downloaded it', async () => {
    render(Updater)
    announce(status())

    expect(await screen.findByText('มี Aetox v0.9.7 แล้ว')).toBeTruthy()
    // The offer costs nothing until it is accepted: the bytes move on the
    // click, not on the notice.
    expect(vi.mocked(ApplyUpdate)).not.toHaveBeenCalled()
  })

  // "Later" is a real answer, not a snooze that reappears in five minutes. It
  // holds for this run — the next launch asks again, which is the point.
  it('takes "later" for an answer', async () => {
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ไว้ก่อน'))

    await waitFor(() => expect(screen.queryByText('มี Aetox v0.9.7 แล้ว')).toBeNull())
    expect(vi.mocked(ApplyUpdate)).not.toHaveBeenCalled()
  })

  // ...unless the answer was about a different version. Saying "later" to
  // v0.9.7 is not consent to never hearing about v0.9.8.
  it('speaks up again when a newer release than the dismissed one appears', async () => {
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ไว้ก่อน'))
    announce(status({ latest: '0.9.8' }))

    expect(await screen.findByText('มี Aetox v0.9.8 แล้ว')).toBeTruthy()
  })

  // Scoop installed us, so Scoop upgrades us — Aetox never writes into another
  // package manager's directory. The channel decision is internal/update's
  // (Status.canAuto / .hint) and this is where that answer has to show up.
  it('offers Scoop its command instead of a button that would touch its folder', async () => {
    render(Updater)
    announce(status({ channel: 'scoop', hint: 'scoop update aetox', canAuto: false }))

    expect(await screen.findByText('scoop update aetox')).toBeTruthy()
    expect(screen.queryByText('อัปเดตเลย')).toBeNull()
  })

  it('sends an unknown install method to the release page', async () => {
    render(Updater)
    announce(status({ channel: 'unknown', canAuto: false }))
    await fireEvent.click(await screen.findByText('เปิดหน้าดาวน์โหลด'))

    expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledWith('https://example.invalid/releases/v0.9.7')
  })
})

describe('the progress dialog', () => {
  // The real ApplyUpdate runs for as long as the download does; a mock that
  // resolves on the spot would skip straight past every state worth showing.
  const stillDownloading = () =>
    vi.mocked(ApplyUpdate).mockImplementationOnce(() => new Promise<void>(() => {}))

  it('replaces the notice and tracks the real download', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('อัปเดตเลย'))

    await waitFor(() => expect(vi.mocked(ApplyUpdate)).toHaveBeenCalled())
    // One surface at a time: the offer is over, the act has begun.
    expect(screen.queryByText('มี Aetox v0.9.7 แล้ว')).toBeNull()

    reportProgress(4_000_000, 10_000_000)
    const bar = await screen.findByRole('progressbar')
    await waitFor(() => expect(bar.getAttribute('aria-valuenow')).toBe('40'))
    expect(await screen.findByText('กำลังอัปเดต Aetox… 40%')).toBeTruthy()
  })

  // No Content-Length means no honest percentage. The bar still moves; it just
  // does not invent a number.
  it('claims no percentage when the server never said how big the file is', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('อัปเดตเลย'))
    reportProgress(4_000_000, 0)

    const bar = await screen.findByRole('progressbar')
    await waitFor(() => expect(bar.classList.contains('indeterminate')).toBe(true))
    expect(bar.getAttribute('aria-valuenow')).toBeNull()
    expect(await screen.findByText('กำลังอัปเดต Aetox…')).toBeTruthy()
  })

  // The window is about to close and come back. Saying so up front is the
  // difference between a restart and what looks like a crash.
  it('warns that the app will close itself before it does', async () => {
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('อัปเดตเลย'))

    expect(await screen.findByText('แอปจะปิดแล้วเปิดกลับมาให้เองเมื่อเสร็จ')).toBeTruthy()
    expect(await screen.findByText('กำลังเปิดใหม่…')).toBeTruthy()
  })

  // A refused download — a hash that does not match the signed checksums — is
  // the case this whole verification chain exists for. The user must be told
  // in a sentence, and told that nothing was replaced.
  it('says what went wrong, promises the old build is intact, and re-arms', async () => {
    vi.mocked(ApplyUpdate).mockRejectedValueOnce(
      new Error('แฮชของ aetox-portable.zip ไม่ตรงกับ checksums.txt — ไฟล์อาจเสียหรือถูกแก้ ระบบไม่ติดตั้งต่อ'))
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('อัปเดตเลย'))

    expect(await screen.findByText(/ไม่ตรงกับ checksums.txt/)).toBeTruthy()
    expect(await screen.findByText('เวอร์ชันเดิมยังอยู่ครบและใช้งานได้ตามปกติ — กดลองใหม่ได้')).toBeTruthy()

    await fireEvent.click(screen.getByText('ลองอีกครั้ง'))
    await waitFor(() => expect(vi.mocked(ApplyUpdate)).toHaveBeenCalledTimes(2))
  })

  // The exe under this window is being replaced. Both doors — the notice and
  // the About button — reach the same act, so the guard has to live with the
  // act rather than on either button.
  it('swallows a second start while the swap is already running', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('อัปเดตเลย'))
    await waitFor(() => expect(vi.mocked(ApplyUpdate)).toHaveBeenCalledTimes(1))

    await startUpdate()

    expect(vi.mocked(ApplyUpdate)).toHaveBeenCalledTimes(1)
  })
})
