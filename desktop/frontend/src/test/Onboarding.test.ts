import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import Onboarding from '../lib/Onboarding.svelte'
import {
  HasAPIKey, SupportedProviders, RequiresAPIKey, AcceptsAPIKey, SwitchApprovalMode,
  CapabilityStatuses, InstallCapabilities,
} from './mocks/wailsApp'

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
    // Five screens, the first one lit. The fifth is the capability step, and
    // it is counted even here, where CapabilityStatuses answers empty: a row
    // of dots whose length depends on what is already installed reads as a
    // bug rather than as a shorter setup.
    expect(container.querySelectorAll('.ob-dots i').length).toBe(5)
    expect(container.querySelector('.ob-dots i.on')).toBe(container.querySelectorAll('.ob-dots i')[0])
  })

  it('opens on the language choice, with nothing else to press', async () => {
    render(Onboarding)
    await screen.findByText('ยินดีต้อนรับสู่ Aetox')
    // The whole screen is the question: the languages, no skip, no "start".
    //
    // Compared against the language list rather than a written-out pair. The
    // pair was the assertion until Chinese arrived and broke a test that was
    // still telling the truth about the screen — what it means to pin is "the
    // ONLY buttons here are languages", and a hard-coded list says that for
    // exactly as long as the list does not change.
    const { localeNames } = await import('../lib/i18n.svelte')
    const buttons = [...document.querySelectorAll('.ob-screen button')].map((b) => b.textContent?.trim())
    expect(buttons).toEqual(Object.values(localeNames))
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
  // Two of three missing, so the screen has to show what is missing and price
  // only that — an offer to reinstall something already on the machine is the
  // fastest way to make the whole screen untrustworthy.
  async function walkToApproval() {
    vi.mocked(SupportedProviders).mockResolvedValue(['deepseek', 'ollama'])
    vi.mocked(RequiresAPIKey).mockImplementation(async (n: string) => n !== 'ollama')
    render(Onboarding)
    ;(await screen.findByText('ไทย')).click()
    ;(await screen.findByText('รันโมเดลภายในเครื่อง Local 100%')).click()
    const ollama = await waitFor(() => {
      const el = [...document.querySelectorAll('.ob-cell')]
        .find((c) => c.querySelector('.nm')?.textContent?.trim() === 'ollama') as HTMLElement | undefined
      expect(el).toBeTruthy()
      return el!
    })
    ollama.click()
    ;(await screen.findByText('ถัดไป', undefined, { timeout: 3000 })).click()
    await waitFor(() => expect(screen.getByText('ให้ผู้ช่วยถามคุณแค่ไหน')).toBeTruthy())
  }

  const MB = 1024 * 1024
  function someMissing() {
    // The suite does not clear mocks between tests, so a call recorded by the
    // previous one would satisfy a not-called assertion's opposite here.
    vi.mocked(InstallCapabilities).mockClear()
    // Rows are capabilities, not downloads: speech is 8MB of engine plus 31MB
    // of model, quoted once as 39MB because ticking it takes both.
    vi.mocked(CapabilityStatuses).mockResolvedValue([
      { capability: 'pdf', installed: false, approx_bytes: 20 * MB },
      { capability: 'media', installed: true, approx_bytes: 0 },
      { capability: 'speech', installed: false, approx_bytes: 39 * MB },
    ])
  }

  it('offers only what is missing, priced per row, everything ticked', async () => {
    someMissing()
    await walkToApproval()
    ;(await screen.findByText('เริ่มใช้ Aetox')).click()

    await waitFor(() => expect(screen.getByText('เพิ่มความสามารถให้ Aetox')).toBeTruthy())
    expect(screen.getByText('อ่าน PDF')).toBeTruthy()
    expect(screen.getByText('ถอดเสียงเป็นข้อความ')).toBeTruthy()
    // Already installed, so not offered and not charged for.
    expect(screen.queryByText('ดูวิดีโอและฟังเสียง')).toBeNull()

    // Each row carries its own size, which is the only thing that makes an
    // untick an informed decision rather than a guess.
    expect(screen.getByText('20MB')).toBeTruthy()
    expect(screen.getByText('39MB')).toBeTruthy()

    const rows = [...document.querySelectorAll('.ob-screen .ob-row')]
    expect(rows.length).toBe(2)
    expect(rows.every((r) => r.getAttribute('aria-pressed') === 'true')).toBe(true)
    expect(screen.getByText('ติดตั้งที่เลือก · 59MB')).toBeTruthy()
  })

  it('unticking a row drops it from both the price and the request', async () => {
    someMissing()
    await walkToApproval()
    ;(await screen.findByText('เริ่มใช้ Aetox')).click()
    ;(await screen.findByText('อ่าน PDF')).click()

    await waitFor(() => expect(screen.getByText('ติดตั้งที่เลือก · 39MB')).toBeTruthy())
    ;(await screen.findByText('ติดตั้งที่เลือก · 39MB')).click()

    await waitFor(() => expect(screen.getByText('พร้อมแล้ว')).toBeTruthy())
    // Capability names, never component ids: which downloads stand behind one
    // tick is the engine's business.
    expect(vi.mocked(InstallCapabilities)).toHaveBeenCalledWith(['speech'])
  })

  it('unticking everything turns the button into the way out, not a dead end', async () => {
    someMissing()
    await walkToApproval()
    ;(await screen.findByText('เริ่มใช้ Aetox')).click()
    ;(await screen.findByText('อ่าน PDF')).click()
    ;(await screen.findByText('ถอดเสียงเป็นข้อความ')).click()

    // No disabled primary button: it says what pressing it now does.
    const skip = await screen.findByText('ข้ามไว้ก่อน')
    expect((skip.closest('button') as HTMLButtonElement).disabled).toBe(false)
    skip.click()

    await waitFor(() => expect(screen.getByText('พร้อมแล้ว')).toBeTruthy())
    expect(vi.mocked(InstallCapabilities)).not.toHaveBeenCalled()
  })

  it('"later" finishes setup without downloading anything', async () => {
    someMissing()
    await walkToApproval()
    ;(await screen.findByText('เริ่มใช้ Aetox')).click()
    ;(await screen.findByText('ไว้ทีหลัง')).click()

    await waitFor(() => expect(screen.getByText('พร้อมแล้ว')).toBeTruthy())
    expect(vi.mocked(InstallCapabilities)).not.toHaveBeenCalled()
    await waitFor(() => expect(localStorage.getItem('aetox.onboarded')).toBe('1'), { timeout: 3000 })
  })
})
