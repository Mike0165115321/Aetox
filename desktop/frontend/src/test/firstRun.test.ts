// The "see the first-run screen again" button, and the one thing it must not
// do: throw away work. It resets remembered *preferences*; an unsent message
// and the tabs on the desk are not preferences.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import Onboarding from '../lib/Onboarding.svelte'
import { armFirstRunReplay, takeFirstRunReplay, DONE_KEY, REPLAY_KEY } from '../lib/firstRun'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { HasAPIKey, RequiresAPIKey, AcceptsAPIKey, SupportedProviders } from './mocks/wailsApp'

beforeEach(() => {
  localStorage.clear()
  vi.mocked(HasAPIKey).mockResolvedValue(false)
  vi.mocked(RequiresAPIKey).mockResolvedValue(true)
  vi.mocked(AcceptsAPIKey).mockResolvedValue(true)
  vi.mocked(SupportedProviders).mockResolvedValue(['deepseek', 'anthropic'])
})

describe('armFirstRunReplay', () => {
  it('forgets the remembered look and arms the wizard', () => {
    localStorage.setItem(DONE_KEY, '1')
    localStorage.setItem('aetox-theme', 'light')
    localStorage.setItem('aetox-chat-font-size', '20')
    localStorage.setItem('sidebarWidth', '420px')

    armFirstRunReplay()

    expect(localStorage.getItem(DONE_KEY)).toBeNull()
    expect(localStorage.getItem('aetox-theme')).toBeNull()
    expect(localStorage.getItem('aetox-chat-font-size')).toBeNull()
    expect(localStorage.getItem('sidebarWidth')).toBeNull()
    expect(localStorage.getItem(REPLAY_KEY)).toBe('1')
  })

  it('forgets the room the window was standing in, Settings included', () => {
    // The button lives in Settings, and the view survives a reload by design —
    // so without this the replayed first run opens on the page it was pressed.
    sessionStorage.setItem('aetox.activeView', 'settings')
    sessionStorage.setItem('aetox.settingsSection', 'general')

    armFirstRunReplay()

    expect(sessionStorage.getItem('aetox.activeView')).toBeNull()
    expect(sessionStorage.getItem('aetox.settingsSection')).toBeNull()
  })

  it('keeps what the user typed and the desk they had open', () => {
    localStorage.setItem('aetox-composer-draft', 'half a message')
    localStorage.setItem('aetox-workbench:sess-1', '["a.ts"]')

    armFirstRunReplay()

    expect(localStorage.getItem('aetox-composer-draft')).toBe('half a message')
    expect(localStorage.getItem('aetox-workbench:sess-1')).toBe('["a.ts"]')
  })

  it('is spent by the first read, so the next reload is an ordinary one', () => {
    armFirstRunReplay()
    expect(takeFirstRunReplay()).toBe(true)
    expect(takeFirstRunReplay()).toBe(false)
  })
})

describe('the wizard, replayed', () => {
  it('shows itself even on a machine that is onboarded and has a key', async () => {
    // Both shortcuts on at once: this is every developer machine, and the
    // reason clearing the flag alone never brought the screen back.
    vi.mocked(HasAPIKey).mockResolvedValue(true)
    cockpit.model.provider = 'deepseek'
    localStorage.setItem(DONE_KEY, '1')

    armFirstRunReplay()
    localStorage.setItem(DONE_KEY, '1') // the flag surviving must not matter

    render(Onboarding)
    await waitFor(() => expect(screen.getByText('ยินดีต้อนรับสู่ Aetox')).toBeTruthy())
  })
})
