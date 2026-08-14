// The half of self-update the user actually meets: being told, being shown,
// and — the part that makes it bearable — getting to pick the moment the app
// closes. The engine could already download, verify and swap the whole app;
// this covers the card that arrives on its own and walks through offer →
// downloading → ready → restart (ARCHITECTURE.md §107).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Updater from '../lib/Updater.svelte'
import { updater, listenForUpdates, startDownload } from '../lib/selfUpdate.svelte'
import { StageUpdate, RestartToUpdate, StagedUpdate } from './mocks/wailsApp'
import { EventsOn, BrowserOpenURL } from './mocks/wailsRuntime'

const status = (over: Record<string, unknown> = {}) => ({
  current: '0.9.6', latest: '0.9.7', available: true, disabled: false,
  channel: 'portable', hint: '', url: 'https://example.invalid/releases/v0.9.7',
  checkedAt: '2026-08-14T00:00:00Z', publishedAt: '2026-08-10T09:00:00Z', canAuto: true, ...over,
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

/** The real StageUpdate runs as long as the download does; a mock that resolves
 *  on the spot would skip past every state worth showing. */
const stillDownloading = () =>
  vi.mocked(StageUpdate).mockImplementationOnce(() => new Promise<void>(() => {}))

beforeEach(() => {
  vi.clearAllMocks()
  Object.assign(updater, {
    status: null, dismissed: false, phase: 'idle', done: 0, total: 0, staged: '', error: '',
  })
})

describe('the offer', () => {
  // Nothing on screen until there is something to say. This component is
  // mounted for the whole life of the app.
  it('shows nothing at all until a newer release turns up', () => {
    const { container } = render(Updater)
    expect(container.querySelector('.upd-card')).toBeNull()
  })

  it('offers the update once the check announces one — without having downloaded it', async () => {
    render(Updater)
    announce(status())

    expect(await screen.findByText('มี Aetox v0.9.7 แล้ว')).toBeTruthy()
    // The offer costs nothing until it is accepted: the bytes move on the
    // click, not on the notice.
    expect(vi.mocked(StageUpdate)).not.toHaveBeenCalled()
  })

  // "v0.9.7" alone does not say whether this is an hour old or a month old,
  // which is most of what somebody deciding about it wants to know.
  it('shows when the release went out', async () => {
    render(Updater)
    announce(status())

    expect(await screen.findByText(new Date('2026-08-10T09:00:00Z').toLocaleDateString(undefined, {
      year: 'numeric', month: 'long', day: 'numeric',
    }))).toBeTruthy()
  })

  // "Later" is a real answer, not a snooze that reappears in five minutes. It
  // holds for this run — the next launch asks again, which is the point.
  it('takes "later" for an answer', async () => {
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ไว้ก่อน'))

    await waitFor(() => expect(screen.queryByText('มี Aetox v0.9.7 แล้ว')).toBeNull())
    expect(vi.mocked(StageUpdate)).not.toHaveBeenCalled()
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
    expect(screen.queryByText('ดาวน์โหลด')).toBeNull()
  })

  it('sends an unknown install method to the release page', async () => {
    render(Updater)
    announce(status({ channel: 'unknown', canAuto: false }))
    await fireEvent.click(await screen.findByText('เปิดหน้าดาวน์โหลด'))

    expect(vi.mocked(BrowserOpenURL)).toHaveBeenCalledWith('https://example.invalid/releases/v0.9.7')
  })
})

describe('downloading', () => {
  it('reports the bytes it actually knows about, not a made-up percentage', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))

    await waitFor(() => expect(vi.mocked(StageUpdate)).toHaveBeenCalled())
    expect(await screen.findByText('กำลังดาวน์โหลด v0.9.7')).toBeTruthy()

    reportProgress(53.2 * 1024 * 1024, 53.4 * 1024 * 1024)
    expect(await screen.findByText('53.2MB / 53.4MB')).toBeTruthy()
    const bar = await screen.findByRole('progressbar')
    await waitFor(() => expect(bar.getAttribute('aria-valuenow')).toBe('100'))
  })

  // No Content-Length means no honest percentage. The bar still moves; it just
  // does not invent a number, and there is no "x / 0MB" to puzzle over.
  it('claims no size when the server never sent one', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))
    reportProgress(4_000_000, 0)

    const bar = await screen.findByRole('progressbar')
    await waitFor(() => expect(bar.classList.contains('indeterminate')).toBe(true))
    expect(bar.getAttribute('aria-valuenow')).toBeNull()
    expect(screen.queryByText(/MB \//)).toBeNull()
  })

  // The download is not an interruption — nothing is closing, so there is no
  // "later" to offer and no reason to let a stray click start a second one.
  it('swallows a second start while one is already running', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))
    await waitFor(() => expect(vi.mocked(StageUpdate)).toHaveBeenCalledTimes(1))

    await startDownload()

    expect(vi.mocked(StageUpdate)).toHaveBeenCalledTimes(1)
  })
})

describe('ready to restart', () => {
  // The whole reason for splitting the act in two: the app does not close
  // itself the moment the bytes land.
  it('waits for the user instead of restarting on its own', async () => {
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))

    expect(await screen.findByText('v0.9.7 พร้อมแล้ว')).toBeTruthy()
    expect(vi.mocked(RestartToUpdate)).not.toHaveBeenCalled()

    await fireEvent.click(screen.getByText('รีสตาร์ทเพื่ออัปเดต'))
    await waitFor(() => expect(vi.mocked(RestartToUpdate)).toHaveBeenCalled())
  })

  // And "later" here costs nothing at all, which the card has to say out loud —
  // otherwise it reads as postponing the install rather than the restart.
  it('says that closing the app normally installs it just the same', async () => {
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))

    expect(await screen.findByText(/ปิดแอปตามปกติแล้วเปิดใหม่ทีหลังก็ได้เหมือนกัน/)).toBeTruthy()
  })

  // A webview reload (Vite HMR, or the frontend alone crashing) leaves the Go
  // side holding a staged update this fresh page knows nothing about. Offering
  // the same download twice for a build already on disk is the bug.
  it('picks the staged update back up after the window reloads', async () => {
    vi.mocked(StagedUpdate).mockResolvedValueOnce('0.9.7' as never)
    render(Updater)
    listenForUpdates()

    expect(await screen.findByText('v0.9.7 พร้อมแล้ว')).toBeTruthy()
    expect(vi.mocked(StageUpdate)).not.toHaveBeenCalled()
  })

  // The restart is the one act with a gate on it: it ends the process, and the
  // process is where the turn lives. Refused is not failed — the build is
  // still staged, so the offer must survive.
  it('keeps offering the restart when a running turn refuses it', async () => {
    vi.mocked(RestartToUpdate).mockRejectedValueOnce(
      new Error('เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน แล้วค่อยอัปเดต (การอัปเดตต้องปิดแอป)'))
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))
    await fireEvent.click(await screen.findByText('รีสตาร์ทเพื่ออัปเดต'))

    expect(await screen.findByText(/เอเจนกำลังทำงานอยู่/)).toBeTruthy()
    // Still on offer, and still not offering to download what is already here.
    expect(screen.getByText('รีสตาร์ทเพื่ออัปเดต')).toBeTruthy()
    expect(screen.queryByText('ดาวน์โหลด')).toBeNull()
  })
})

// A refused download — a hash that does not match the signed checksums — is the
// case this whole verification chain exists for. The user must be told in a
// sentence, and told that nothing was replaced.
describe('when it fails', () => {
  it('says what went wrong, promises the running build is intact, and re-arms', async () => {
    vi.mocked(StageUpdate).mockRejectedValueOnce(
      new Error('แฮชของ aetox-portable.zip ไม่ตรงกับ checksums.txt — ไฟล์อาจเสียหรือถูกแก้ ระบบไม่ติดตั้งต่อ'))
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))

    expect(await screen.findByText(/ไม่ตรงกับ checksums.txt/)).toBeTruthy()
    expect(await screen.findByText('เวอร์ชันที่ใช้อยู่ยังอยู่ครบและใช้งานได้ตามปกติ')).toBeTruthy()

    await fireEvent.click(screen.getByText('ลองอีกครั้ง'))
    await waitFor(() => expect(vi.mocked(StageUpdate)).toHaveBeenCalledTimes(2))
  })

  // Hiding the card must not hide the news. A download that finished or failed
  // behind a closed card is still something the user has to be told.
  it('comes back when the phase changes after being dismissed with the ×', async () => {
    stillDownloading()
    render(Updater)
    announce(status())
    await fireEvent.click(await screen.findByText('ดาวน์โหลด'))
    await fireEvent.click(screen.getByLabelText('ซ่อนไว้ก่อน'))
    await waitFor(() => expect(screen.queryByText('กำลังดาวน์โหลด v0.9.7')).toBeNull())

    updater.staged = '0.9.7'
    updater.phase = 'ready'

    expect(await screen.findByText('v0.9.7 พร้อมแล้ว')).toBeTruthy()
  })
})
