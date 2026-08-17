import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import Onboarding from '../lib/Onboarding.svelte'
import { HasAPIKey, SupportedProviders, RequiresAPIKey, AcceptsAPIKey, SwitchApprovalMode } from './mocks/wailsApp'

// i18n defaults to Thai; assert against Thai strings.
beforeEach(() => {
  localStorage.clear()
  vi.mocked(HasAPIKey).mockResolvedValue(false)
  vi.mocked(SupportedProviders).mockResolvedValue(['deepseek', 'anthropic'])
  vi.mocked(RequiresAPIKey).mockResolvedValue(true)
  vi.mocked(AcceptsAPIKey).mockResolvedValue(true)
})

describe('Onboarding', () => {
  it('shows the wizard on a fresh machine (no key, no flag)', async () => {
    const { container } = render(Onboarding)
    await waitFor(() => {
      expect(screen.getByText('ยินดีต้อนรับสู่ Aetox')).toBeTruthy()
    })
    // Four screens, the first one lit.
    expect(container.querySelectorAll('.ob-dots i').length).toBe(4)
    expect(container.querySelector('.ob-dots i.on')).toBe(container.querySelectorAll('.ob-dots i')[0])
  })

  it('opens on the language choice, with nothing else to press', async () => {
    render(Onboarding)
    await screen.findByText('ยินดีต้อนรับสู่ Aetox')
    // The whole screen is the question: two languages, no skip, no "start".
    const buttons = [...document.querySelectorAll('.ob-screen button')].map((b) => b.textContent?.trim())
    expect(buttons).toEqual(['ไทย', 'English'])
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

  it('walks language → connect → look → approval → done', async () => {
    // A local runtime is the shortest real connection: one press, no key, no
    // browser. There is no way past this step without connecting something.
    vi.mocked(SupportedProviders).mockResolvedValue(['deepseek', 'ollama'])
    vi.mocked(RequiresAPIKey).mockImplementation(async (n: string) => n !== 'ollama')

    render(Onboarding)
    ;(await screen.findByText('ไทย')).click()
    await waitFor(() => expect(screen.getByText('ต่อสมองให้ Aetox')).toBeTruthy())

    ;(await screen.findByText('รันโมเดลภายในเครื่อง Local 100%')).click()
    const ollama = await waitFor(() => {
      const el = [...document.querySelectorAll('.ob-cell')]
        .find((c) => c.querySelector('.nm')?.textContent?.trim() === 'ollama') as HTMLElement | undefined
      expect(el).toBeTruthy()
      return el!
    })
    ollama.click()
    await waitFor(() => expect(screen.getByText('เลือกหน้าตาที่ชอบ')).toBeTruthy(), { timeout: 3000 })

    ;(await screen.findByText('ถัดไป')).click()
    await waitFor(() => expect(screen.getByText('ให้ผู้ช่วยถามคุณแค่ไหน')).toBeTruthy())

    ;(await screen.findByText('เริ่มใช้ Aetox')).click()
    // The last screen says it worked before it goes away.
    await waitFor(() => expect(screen.getByText('พร้อมแล้ว')).toBeTruthy())
    expect(vi.mocked(SwitchApprovalMode)).toHaveBeenCalledWith('unsafe-only')
    await waitFor(() => expect(localStorage.getItem('aetox.onboarded')).toBe('1'), { timeout: 3000 })
  })
})
