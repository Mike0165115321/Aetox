// The door switcher on the wordmark (COMPANY.md §2, DECISIONS §86).
//
// Two claims are worth pinning, and neither is "a menu opens": picking the
// other door has to land you at that door's desk (a session is born at a desk
// and never moves, so switching buildings means opening one there), and
// picking the door you are already behind must not throw away the conversation
// in front of you.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import TopBar from '../lib/TopBar.svelte'
import { NewSessionAt, CurrentSessionID, SessionMode, ListModes } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { shell, setShell } from '../lib/shell.svelte'

const props = {
  inspectorCollapsed: false, onToggleInspector: () => {},
  sidebarCollapsed: false, onToggleSidebar: () => {},
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = 'assistant'
  cockpit.activeView = 'chat'
  setShell('assistant')
  vi.mocked(CurrentSessionID).mockResolvedValue('20260805-130000.000')
  vi.mocked(SessionMode).mockResolvedValue('assistant')
})

describe('the door switcher', () => {
  it('names the door you are behind and offers the other one', async () => {
    render(TopBar, props)

    await fireEvent.click(screen.getByLabelText(/สลับระหว่าง/))
    // Both doors, each with the one line that says what is behind it.
    expect(screen.getAllByText('ผู้ช่วย').length).toBeGreaterThan(0)
    expect(screen.getByText('โค้ด')).toBeTruthy()
    expect(screen.getByText('อ่านโค้ด สร้างระบบ ดีบัก')).toBeTruthy()
  })

  // A door sign is read at a glance on the way to a click. The manifest
  // description — accurate, and as long as accuracy needs — was tried here and
  // wrapped to three lines apiece; these stay short on purpose, and a test is
  // the only thing that keeps a "just one more clause" from creeping back.
  it('keeps each door sign short enough to read at a glance', async () => {
    render(TopBar, props)

    await fireEvent.click(screen.getByLabelText(/สลับระหว่าง/))
    for (const line of Array.from(document.querySelectorAll('.door-item .d'))) {
      expect((line.textContent ?? '').length).toBeLessThanOrEqual(32)
    }
  })

  it('walks you to the other door and opens a session at its desk', async () => {
    render(TopBar, props)

    await fireEvent.click(screen.getByLabelText(/สลับระหว่าง/))
    await fireEvent.click(screen.getByText('โค้ด'))

    await waitFor(() => expect(vi.mocked(NewSessionAt).mock.calls[0][0]).toBe('coding'))
    expect(shell.name).toBe('code')
    expect(cockpit.activeView).toBe('chat')
  })

  // Re-picking the door you are already behind is the same non-event as
  // clicking the room you are already standing in.
  it('does nothing when you pick the door you are already behind', async () => {
    render(TopBar, props)

    await fireEvent.click(screen.getByLabelText(/สลับระหว่าง/))
    await fireEvent.click(screen.getAllByText('ผู้ช่วย')[0])

    expect(vi.mocked(NewSessionAt)).not.toHaveBeenCalled()
    expect(shell.name).toBe('assistant')
  })
})
