import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import FileEditor from '../lib/FileEditor.svelte'

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
