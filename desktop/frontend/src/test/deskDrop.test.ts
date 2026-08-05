// The workbench is the agent's desk, and a desk you cannot put anything down on
// is a display case. Everything the user is already holding — the file the
// agent just made, a page dragged off a real browser, anything at all out of
// Explorer — has to land here and open.
//
// Two halves, both pinned below: what a drop DOES (the store), and that the
// panel accepts the drop at all (the component).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import Workbench from '../lib/workbench/Workbench.svelte'
import FilesPane from '../lib/workbench/FilesPane.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { workbench, openPathsInWorkbench, openFileTab } from '../lib/stores/workbench.svelte'
import { RelativizePath, SaveChatFile, ReadFile, ReadImageDataURL } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
  // A dropped URL mounts a BrowserPane, which watches its host element.
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})

/** jsdom has no DataTransfer, so drags carry a hand-made one. Writable, so a
 *  dragstart handler's setData can be read back. */
function drag(type: string, data: Record<string, string>): DragEvent {
  const store: Record<string, string> = { ...data }
  const e = new Event(type, { bubbles: true, cancelable: true }) as DragEvent
  Object.defineProperty(e, 'dataTransfer', {
    value: {
      get types() { return Object.keys(store) },
      getData: (k: string) => store[k] ?? '',
      setData: (k: string, v: string) => { store[k] = v },
      dropEffect: 'none',
      effectAllowed: 'none',
    },
  })
  return e
}

describe('dropping OS files on the desk', () => {
  it('opens a file that already lives in the project where it lies', async () => {
    RelativizePath.mockResolvedValueOnce('docs/plan.md')
    ReadFile.mockResolvedValueOnce('# plan')

    await openPathsInWorkbench(['D:\\Aetox\\Aetox\\docs\\plan.md'])

    expect(workbench.tabs[0].path).toBe('docs/plan.md')
    expect(workbench.tabs[0].content).toBe('# plan')
    // Copying a file that is already in reach would litter the project.
    expect(SaveChatFile).not.toHaveBeenCalled()
  })

  it('brings a file from outside the project in, and still calls it by its own name', async () => {
    RelativizePath.mockRejectedValueOnce(new Error('path is outside project root'))
    SaveChatFile.mockResolvedValueOnce('.aetox/attachments/s1/1712-3.pdf')

    await openPathsInWorkbench(['C:\\Users\\ASUS\\Downloads\\ใบเสร็จ.pdf'])

    expect(SaveChatFile).toHaveBeenCalledWith('C:\\Users\\ASUS\\Downloads\\ใบเสร็จ.pdf')
    expect(workbench.tabs[0].path).toBe('.aetox/attachments/s1/1712-3.pdf')
    // The copy's generated filename is an implementation detail; the tab has to
    // read as the file the user dropped.
    expect(workbench.tabs[0].name).toBe('ใบเสร็จ.pdf')
  })

  it('says so when a dropped file cannot be brought in at all', async () => {
    RelativizePath.mockRejectedValueOnce(new Error('outside'))
    SaveChatFile.mockRejectedValueOnce(new Error('ไฟล์ใหญ่เกินไป'))

    await openPathsInWorkbench(['E:\\huge.iso'])

    // Silence would read as the desk ignoring the drop — the exact complaint
    // this surface exists to answer.
    expect(workbench.tabs).toHaveLength(1)
    expect(workbench.tabs[0].name).toBe('huge.iso')
    expect(workbench.tabs[0].unreadable).toContain('ไฟล์ใหญ่เกินไป')
  })

  it('re-reads a file that is already open, instead of showing the last turn', async () => {
    ReadFile.mockResolvedValueOnce('ยอดรวม 100')
    await openFileTab('out/สรุป.md')
    expect(workbench.tabs[0].content).toBe('ยอดรวม 100')
    const firstRev = workbench.tabs[0].rev

    // The agent rewrites the same path — regenerate and undo both do this by
    // construction, and the tab id IS the path.
    ReadFile.mockResolvedValueOnce('ยอดรวม 250')
    await openFileTab('out/สรุป.md')

    expect(workbench.tabs).toHaveLength(1)
    expect(workbench.tabs[0].content).toBe('ยอดรวม 250')
    // The pane is keyed on rev; without the bump FileEditor keeps its own copy
    // of the first read and the new bytes never reach the screen.
    expect(workbench.tabs[0].rev).toBeGreaterThan(firstRev!)
  })

  it('drops a stale failure when the file starts reading again', async () => {
    ReadFile.mockRejectedValueOnce(new Error('binary file cannot be previewed'))
    await openFileTab('out/report.html')
    expect(workbench.tabs[0].unreadable).toContain('binary')

    ReadFile.mockResolvedValueOnce('<h1>ok</h1>')
    await openFileTab('out/report.html')

    // A pane that kept the old excuse would explain a failure that is over.
    expect(workbench.tabs[0].unreadable).toBeUndefined()
    expect(workbench.tabs[0].content).toBe('<h1>ok</h1>')
  })

  it('claims the tab id before reading, so a double open cannot duplicate it', async () => {
    // Reads are slow enough to overlap — a workbook unzips, a 20MB image
    // crosses the bridge as ~27MB of base64. Two tabs with the same id throw
    // each_key_duplicate out of the tab strip and take the panel down.
    ReadFile.mockImplementation(() => new Promise((r) => setTimeout(() => r('x'), 30)))
    await Promise.all([openFileTab('out/big.md'), openFileTab('out/big.md')])

    expect(workbench.tabs.filter((t) => t.id === 'file-out/big.md')).toHaveLength(1)
    ReadFile.mockReset()
  })

  it('shows a dropped picture as a picture, without reading it first', async () => {
    await openFileTab('shots/หน้าจอ.png')

    expect(workbench.tabs[0].view).toBe('image')
    // Reading a PNG as text is what produces "binary file cannot be previewed"
    // in front of someone's own screenshot.
    expect(ReadFile).not.toHaveBeenCalled()
    // Nor as base64 across the binding: the pane addresses the file host, which
    // is what took the 20MB ceiling off pictures and let video in at all.
    expect(ReadImageDataURL).not.toHaveBeenCalled()
  })

  // Each of these used to end at the same card. They are the reason the file
  // host exists, so a change that quietly drops one back to "cannot preview"
  // should fail here rather than on someone's desk.
  it.each([
    ['clips/บันทึกหน้าจอ.mp4', 'video'],
    ['clips/note.m4a', 'audio'],
    ['out/รายงาน.pdf', 'pdf'],
    ['assets/logo.svg', 'image'],
  ])('opens %s in its own pane', async (path, view) => {
    await openFileTab(path)

    expect(workbench.tabs[0].view).toBe(view)
    expect(ReadFile).not.toHaveBeenCalled()
  })
})

// The tree is where you go to FIND a file, and it was the one place a file
// could not be picked up: the tab strip and the reply's file cards were both
// drag sources, the tree was not.
describe('the file tree as a drag source', () => {
  it('lets a file be dragged, and a folder not', async () => {
    cockpit.tree = [
      { label: 'docs', path: 'docs', kind: 'dir', depth: 0, open: false },
      { label: 'README.md', path: 'README.md', kind: 'file', depth: 0 },
    ]
    const { container } = render(FilesPane)

    const [dir, file] = [...container.querySelectorAll('.row')] as HTMLElement[]
    expect(dir.getAttribute('draggable')).toBe('false') // nothing to attach
    expect(file.getAttribute('draggable')).toBe('true')
  })

  it('carries the same payload the tab strip and the file cards do', async () => {
    cockpit.tree = [{ label: 'README.md', path: 'docs/README.md', kind: 'file', depth: 0 }]
    const { container } = render(FilesPane)

    const e = drag('dragstart', {})
    ;(container.querySelector('.row') as HTMLElement).dispatchEvent(e)

    expect(JSON.parse(e.dataTransfer!.getData('application/x-aetox-tab'))).toEqual({
      kind: 'file', ref: 'docs/README.md', label: 'README.md',
    })
  })
})

describe('the panel as a drop target', () => {
  it('opens a page dragged in from a real browser', async () => {
    const { container } = render(Workbench)
    const desk = container.querySelector('.wb')!

    desk.dispatchEvent(drag('drop', { 'text/uri-list': 'https://aetox.dev/docs\n' }))
    await vi.waitFor(() => expect(workbench.tabs).toHaveLength(1))

    expect(workbench.tabs[0].kind).toBe('browser')
    expect(workbench.tabs[0].url).toBe('https://aetox.dev/docs')
    expect(workbench.tabs[0].name).toBe('aetox.dev')
  })

  it('opens a file card dragged over from the chat', async () => {
    ReadFile.mockResolvedValueOnce('เนื้อหา')
    const { container } = render(Workbench)
    const desk = container.querySelector('.wb')!

    desk.dispatchEvent(drag('drop', {
      'application/x-aetox-tab': JSON.stringify({ kind: 'file', ref: 'out/สรุป.md', label: 'สรุป.md' }),
    }))
    await vi.waitFor(() => expect(workbench.tabs).toHaveLength(1))

    expect(workbench.tabs[0].path).toBe('out/สรุป.md')
  })

  it('ignores dragged text that is not an address', async () => {
    const { container } = render(Workbench)
    const desk = container.querySelector('.wb')!

    desk.dispatchEvent(drag('drop', { 'text/plain': 'สามคำที่ลากมา' }))
    await Promise.resolve()

    // Dragging a selection out of a page must not navigate to https://<words>.
    expect(workbench.tabs).toHaveLength(0)
  })

  // The store having the right data is not the same as the panel showing it.
  // openFileTab pushes the tab before reading the file so the id is claimed
  // synchronously; hold on to the object that was pushed rather than the proxy
  // in the array and every later write is invisible to Svelte — the store looks
  // perfect and the pane stays black. That shipped, and 246 passing tests did
  // not notice, because every one of them asserted on the store.
  it('actually draws the file it opened', async () => {
    ReadFile.mockResolvedValueOnce('# บันทึก')
    const { container } = render(Workbench)

    await openFileTab('docs/CHANGELOG.md')

    await vi.waitFor(() => {
      const pane = container.querySelector('.fe-path')
      expect(pane?.textContent).toBe('docs/CHANGELOG.md')
    })
  })

  it('draws the reason when the file cannot be read', async () => {
    ReadFile.mockRejectedValueOnce(new Error('binary file cannot be previewed'))
    const { container } = render(Workbench)

    // A container with no pane of its own — the card is the right answer here,
    // and stays it. (This used to be a .pdf, back when a PDF had nowhere to go.)
    await openFileTab('out/bundle.zip')

    await vi.waitFor(() => expect(container.textContent).toContain('binary file cannot be previewed'))
  })

  it('leaves an OS file drop to the native handler that knows its path', async () => {
    const { container } = render(Workbench)
    const desk = container.querySelector('.wb')!

    const e = drag('drop', { Files: '' })
    desk.dispatchEvent(e)
    await Promise.resolve()

    // The DOM event carries no readable path — Wails resolves those and routes
    // them by drop coordinates (App.svelte). Swallowing it here would lose the
    // drop entirely, so the handler must decline it.
    expect(e.defaultPrevented).toBe(false)
    expect(workbench.tabs).toHaveLength(0)
  })
})
