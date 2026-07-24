import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import Onboarding from '../lib/Onboarding.svelte'
import { HasAPIKey, SupportedProviders, RequiresAPIKey } from './mocks/wailsApp'

// i18n defaults to Thai; assert against Thai strings.
beforeEach(() => {
  localStorage.clear()
  vi.mocked(HasAPIKey).mockResolvedValue(false)
  vi.mocked(SupportedProviders).mockResolvedValue(['deepseek', 'anthropic'])
  vi.mocked(RequiresAPIKey).mockResolvedValue(true)
})

describe('Onboarding', () => {
  it('shows the wizard on a fresh machine (no key, no flag)', async () => {
    render(Onboarding)
    await waitFor(() => {
      expect(screen.getByText('ยินดีต้อนรับสู่ Aetox')).toBeTruthy()
    })
    expect(screen.getByText('1/3')).toBeTruthy()
  })

  it('never shows for an install that already has a working key, and marks itself done', async () => {
    vi.mocked(HasAPIKey).mockResolvedValue(true)
    // cockpit.model.provider must be non-empty for the has-key check to run.
    const { cockpit } = await import('../lib/stores/cockpit.svelte')
    cockpit.model.provider = 'deepseek'

    render(Onboarding)
    await waitFor(() => {
      expect(localStorage.getItem('aetox.onboarded')).toBe('1')
    })
    expect(screen.queryByText('ยินดีต้อนรับสู่ Aetox')).toBeNull()
  })

  it('skip closes the wizard and sets the flag so it stays gone', async () => {
    render(Onboarding)
    const skip = await screen.findByText('ข้าม')
    skip.click()
    await waitFor(() => {
      expect(localStorage.getItem('aetox.onboarded')).toBe('1')
      expect(screen.queryByText('ยินดีต้อนรับสู่ Aetox')).toBeNull()
    })
  })

  it('walks step 1 → 2 → 3', async () => {
    render(Onboarding)
    ;(await screen.findByText('ถัดไป')).click() // step 1 → 2
    await waitFor(() => expect(screen.getByText('เลือกโมเดล AI ของคุณ')).toBeTruthy())
    ;(await screen.findByText('ถัดไป')).click() // step 2 → 3 (no key entered — allowed)
    await waitFor(() => expect(screen.getByText('ให้ผู้ช่วยถามคุณแค่ไหน?')).toBeTruthy())
    ;(await screen.findByText('เริ่มใช้ Aetox')).click()
    await waitFor(() => expect(localStorage.getItem('aetox.onboarded')).toBe('1'))
  })
})
