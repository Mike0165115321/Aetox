import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import CapabilityProgress from '../lib/CapabilityProgress.svelte'
import {
  capabilities, noteCapabilityRequest, dismissCapabilities, retryCapabilities,
} from '../lib/capabilities.svelte'
import { InstallCapabilities } from './mocks/wailsApp'

// i18n defaults to Thai; assert against Thai strings.
beforeEach(() => {
  dismissCapabilities()
  capabilities.requested = []
  capabilities.index = 0
  capabilities.of = 0
  capabilities.percent = -1
  vi.mocked(InstallCapabilities).mockClear()
})

describe('CapabilityProgress', () => {
  // The install is started from a screen the user immediately leaves, so this
  // card is the only thing that ever says the download happened. Silence here
  // is the whole failure it exists to prevent.
  it('says nothing while nothing is happening', () => {
    const { container } = render(CapabilityProgress)
    expect(container.querySelector('.upd-card')).toBeNull()
  })

  it('shows which download of how many, and a real bar when the size is known', async () => {
    render(CapabilityProgress)
    noteCapabilityRequest(['pdf', 'speech'])
    capabilities.index = 1
    capabilities.of = 3
    capabilities.percent = 42

    await waitFor(() => expect(screen.getByText('กำลังเพิ่มความสามารถ')).toBeTruthy())
    // index is zero-based on the wire and one-based on screen: nobody reads
    // "1 จาก 3" while the first of three is running.
    expect(screen.getByText('2 จาก 3')).toBeTruthy()

    const bar = document.querySelector('.upd-bar') as HTMLElement
    expect(bar.getAttribute('aria-valuenow')).toBe('42')
    expect(bar.classList.contains('indeterminate')).toBe(false)
  })

  // -1 is what the engine sends when the server gave no Content-Length. A bar
  // pinned at 0% would read as a stalled download rather than an unmeasured one.
  it('goes indeterminate rather than showing a stuck zero', async () => {
    render(CapabilityProgress)
    noteCapabilityRequest(['media'])
    capabilities.percent = -1

    await waitFor(() => {
      const bar = document.querySelector('.upd-bar') as HTMLElement
      expect(bar.classList.contains('indeterminate')).toBe(true)
      expect(bar.getAttribute('aria-valuenow')).toBeNull()
    })
  })

  it('offers to resume after a failure, with the set that was asked for', async () => {
    render(CapabilityProgress)
    noteCapabilityRequest(['pdf', 'speech'])
    capabilities.phase = 'error'
    capabilities.error = 'ffmpeg: ดาวน์โหลดไม่สำเร็จ'

    await waitFor(() => expect(screen.getByText('ทำไม่สำเร็จ')).toBeTruthy())
    expect(screen.getByText('ffmpeg: ดาวน์โหลดไม่สำเร็จ')).toBeTruthy()
    // Says the true thing: finished parts stay finished.
    expect(screen.getByText(/ทำต่อจากเดิม/)).toBeTruthy()

    await retryCapabilities()
    expect(vi.mocked(InstallCapabilities)).toHaveBeenCalledWith(['pdf', 'speech'])
  })

  it('clears itself after saying it worked', async () => {
    vi.useFakeTimers()
    try {
      render(CapabilityProgress)
      noteCapabilityRequest(['pdf'])
      capabilities.phase = 'done'
      await vi.advanceTimersByTimeAsync(0)
      expect(screen.getByText('เพิ่มความสามารถแล้ว')).toBeTruthy()

      // A finished bar left up becomes one more thing to tidy away.
      await vi.advanceTimersByTimeAsync(10_000)
      expect(capabilities.phase).toBe('idle')
    } finally {
      vi.useRealTimers()
    }
  })
})
