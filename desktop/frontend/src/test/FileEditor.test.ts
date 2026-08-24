import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import FileEditor from '../lib/FileEditor.svelte'
import { WriteFile } from './mocks/wailsApp'

describe('FileEditor markdown preview', () => {
  it('renders .md files as markdown by default, with a Source toggle', async () => {
    const { container } = render(FileEditor, { path: 'docs/README.md', content: '# Hello World' })
    const preview = container.querySelector('.fe-preview')
    expect(preview).toBeTruthy()
    expect(preview!.querySelector('h1')?.textContent).toBe('Hello World')
    // Editor mount is hidden, not destroyed.
    expect(container.querySelector('.editor-mount')?.classList.contains('fe-hidden')).toBe(true)
    expect(screen.getByText('ซอร์ส')).toBeTruthy()
  })

  it('toggles back to the source editor', async () => {
    const { container } = render(FileEditor, { path: 'a.md', content: '# T' })
    screen.getByText('ซอร์ส').click()
    await waitFor(() => {
      expect(container.querySelector('.fe-preview')).toBeNull()
      expect(container.querySelector('.editor-mount')?.classList.contains('fe-hidden')).toBe(false)
      expect(screen.getByText('พรีวิว')).toBeTruthy()
    })
  })

  it('non-markdown files get no preview and no toggle', () => {
    const { container } = render(FileEditor, { path: 'main.go', content: 'package main' })
    expect(container.querySelector('.fe-preview')).toBeNull()
    expect(screen.queryByText('ซอร์ส')).toBeNull()
    expect(screen.queryByText('พรีวิว')).toBeNull()
  })
})

// Autosave, and what happens when the agent writes the file somebody is reading.
//
// Monaco does not mount under jsdom, so the editor object stays undefined and
// the component falls through to its own draft/base state — which is exactly
// the path these tests need. What cannot be driven from here is typing, so the
// dirty half (the conflict bar) is left to the running app; the clean half is
// the case the owner actually hit and it is pinned here.
describe('FileEditor and a file that changes underneath it', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows what the agent just wrote, without being reopened', async () => {
    const { rerender, container } = render(FileEditor, { path: 'post.md', content: '# ก่อน' })
    expect(container.querySelector('.fe-preview')!.textContent).toContain('ก่อน')

    // The store re-read the file and handed the pane new bytes.
    await rerender({ path: 'post.md', content: '# หลัง' })

    await waitFor(() => {
      expect(container.querySelector('.fe-preview')!.textContent).toContain('หลัง')
    })
    // Nothing was typed here, so there is nothing to ask about.
    expect(container.querySelector('.fe-conflict')).toBeNull()
  })

  // The pane must not write the agent's own bytes back at it as if a person had
  // typed them: an external change is not a reason to save.
  it('does not save a file it was merely handed', async () => {
    const { rerender } = render(FileEditor, { path: 'a.md', content: 'one' })
    await rerender({ path: 'a.md', content: 'two' })

    await new Promise((r) => setTimeout(r, 900))
    expect(WriteFile).not.toHaveBeenCalled()
  })

  // Nothing typed, nothing to save: the button is the status line, and it says
  // so rather than offering an act with no effect.
  it('reports itself saved when there is no draft', () => {
    render(FileEditor, { path: 'a.md', content: 'one' })
    const button = screen.getByText('บันทึกแล้ว') as HTMLButtonElement
    expect(button.disabled).toBe(true)
  })
})
