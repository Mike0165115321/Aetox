// Pointing at the page, from the app's side.
//
// The half that matters here is what the user gets back: a chip carrying the
// element rather than a description of it, and a mode that does not stay lit
// over a page that has stopped listening.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  pagePick, startPagePick, stopPagePick, pickChipContent, pickChipLabel, type PagePick,
} from '../lib/workbench/pagePick.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import {
  BrowserStartPick, BrowserStopPick, BrowserCapturePNG, SaveChatImageData, ReadImageDataURL,
} from './mocks/wailsApp'
import { EventsOn } from './mocks/wailsRuntime'

const PICK: PagePick = {
  selector: 'button.btn-primary', tag: 'button', text: 'ส่งข้อความ',
  html: '<button class="btn btn-primary" type="submit">ส่งข้อความ</button>',
  path: 'form#composer > div.row', w: 188, h: 36,
  color: '#ffffff', background: '#185fa5',
}

/** Drive the Go-side event the way the engine would. */
function emit(event: string, payload: unknown) {
  const handler = vi.mocked(EventsOn).mock.calls.find((c) => c[0] === event)?.[1] as (p: unknown) => void
  if (!handler) throw new Error(`nothing subscribed to ${event}`)
  handler(payload)
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.pendingContexts = []
  cockpit.pendingImages = []
  cockpit.chat.length = 0
  pagePick.tabId = null
  pagePick.mode = 'pick'
})

describe('what the model is handed', () => {
  it('carries the things that find the element in source, not just its tag', () => {
    const content = pickChipContent('http://localhost:5173/chat', [PICK])

    expect(content).toContain('http://localhost:5173/chat')
    expect(content).toContain('button.btn-primary')
    expect(content).toContain('188×36')
    // The rendered colour is the part that greps to a token in the stylesheet.
    expect(content).toContain('#185fa5')
    expect(content).toContain('form#composer > div.row')
    expect(content).toContain('<button class="btn btn-primary"')
  })

  it('numbers several picks so the question can name one', () => {
    const content = pickChipContent('http://x/', [PICK, { ...PICK, selector: 'a.link' }])
    expect(content).toContain('[1] button.btn-primary')
    expect(content).toContain('[2] a.link')
  })

  it('labels one pick with its selector and many with a count', () => {
    expect(pickChipLabel([PICK])).toBe('button.btn-primary')
    expect(pickChipLabel([PICK, PICK, PICK])).toContain('3')
  })

  it('leaves out a field the page had nothing for', () => {
    const bare = { ...PICK, text: '', path: '', html: '' }
    const content = pickChipContent('http://x/', [bare])
    expect(content).not.toContain('text:')
    expect(content).not.toContain('within:')
    expect(content).not.toContain('html:')
  })
})

describe('the mode', () => {
  it('arms the tab and hands the overlay the live theme and wording', async () => {
    await startPagePick('web-1')

    expect(pagePick.tabId).toBe('web-1')
    const [id, opts] = vi.mocked(BrowserStartPick).mock.calls[0] as [string, string]
    expect(id).toBe('web-1')
    const parsed = JSON.parse(opts)
    expect(parsed.accent).toBeTruthy()
    expect(parsed.hint).toBeTruthy()
  })

  it('turns the composer chip on only once the user has pointed at something', async () => {
    await startPagePick('web-1')
    emit('browser:pick:web-1', { url: 'http://localhost:5173/chat', cancelled: false, picks: [PICK] })

    expect(cockpit.pendingContexts[0]?.kind).toBe('pick')
    expect(cockpit.pendingContexts[0]?.label).toBe('button.btn-primary')
    expect(pagePick.tabId).toBeNull()
  })

  it('attaches nothing when the user left the mode without pointing', async () => {
    await startPagePick('web-1')
    emit('browser:pick:web-1', { url: 'http://x/', cancelled: true, picks: [] })

    expect(cockpit.pendingContexts).toEqual([])
    expect(pagePick.tabId).toBeNull()
  })

  it('presses off on the tab already in that mode', async () => {
    await startPagePick('web-1')
    await startPagePick('web-1')

    expect(pagePick.tabId).toBeNull()
    expect(BrowserStopPick).toHaveBeenCalledWith('web-1')
    expect(BrowserStartPick).toHaveBeenCalledTimes(1)
  })

  it('switches mode rather than toggling off when the other button is pressed', async () => {
    await startPagePick('web-1', 'pick')
    await startPagePick('web-1', 'draw')

    expect(pagePick.tabId).toBe('web-1')
    expect(pagePick.mode).toBe('draw')
    expect(BrowserStartPick).toHaveBeenCalledTimes(2)
  })

  it('stands down when the page navigates out from under it', async () => {
    await startPagePick('web-1')
    emit('browser:meta:web-1', { title: 'somewhere else', url: 'http://elsewhere/' })

    // The overlay went with the old document, so the button must not stay lit.
    expect(pagePick.tabId).toBeNull()
  })

  it('does not leave the mode armed when the tab could not be reached', async () => {
    vi.mocked(BrowserStartPick).mockRejectedValueOnce(new Error('no browser tab "web-9"'))
    await startPagePick('web-9')

    expect(pagePick.tabId).toBeNull()
  })

  it('is a no-op to stop when nothing is picking', () => {
    stopPagePick()
    expect(BrowserStopPick).not.toHaveBeenCalled()
  })
})

describe('drawing on the page', () => {
  it('tells the overlay which mode it is, and how its buttons read', async () => {
    await startPagePick('web-1', 'draw')

    const opts = JSON.parse((vi.mocked(BrowserStartPick).mock.calls[0] as [string, string])[1])
    expect(opts.mode).toBe('draw')
    expect(opts.doneLabel).toBeTruthy()
    expect(opts.cancelLabel).toBeTruthy()
    expect(opts.markUnit).toBeTruthy()
  })

  // The bar is drawn on a page whose background nobody knows, so it owns its
  // colours. Sending the app's panel colour is what makes it vanish on a white
  // page under a light theme.
  it('sends the accent and nothing else of the theme', async () => {
    await startPagePick('web-1', 'draw')

    const opts = JSON.parse((vi.mocked(BrowserStartPick).mock.calls[0] as [string, string])[1])
    expect(opts.accent).toBeTruthy()
    expect(opts.panel).toBeUndefined()
    expect(opts.text).toBeUndefined()
    expect(opts.border).toBeUndefined()
  })

  it('photographs the marks, attaches the picture, then takes the ink down', async () => {
    vi.mocked(BrowserCapturePNG).mockResolvedValueOnce('data:image/png;base64,AAAA')
    vi.mocked(SaveChatImageData).mockResolvedValueOnce('.aetox-attachments/s1/shot.png')
    vi.mocked(ReadImageDataURL).mockResolvedValueOnce('data:image/png;base64,AAAA')

    await startPagePick('web-1', 'draw')
    emit('browser:pick:web-1', { url: 'http://x/', cancelled: false, drawn: true, picks: [PICK] })
    await vi.waitFor(() => expect(cockpit.pendingImages).toHaveLength(1))

    expect(cockpit.pendingImages[0]?.relPath).toBe('.aetox-attachments/s1/shot.png')
    // What was under the marks rides along as text; the picture is the point.
    expect(cockpit.pendingContexts[0]?.content).toContain('marks')
    // The ink must come down only after the picture exists, never before.
    expect(BrowserStopPick).toHaveBeenCalledWith('web-1')
    expect(vi.mocked(BrowserCapturePNG).mock.invocationCallOrder[0])
      .toBeLessThan(vi.mocked(BrowserStopPick).mock.invocationCallOrder[0])
  })

  it('still takes the ink down when the picture could not be taken', async () => {
    vi.mocked(BrowserCapturePNG).mockRejectedValueOnce(new Error('the page did not answer with a picture'))

    await startPagePick('web-1', 'draw')
    emit('browser:pick:web-1', { url: 'http://x/', cancelled: false, drawn: true, picks: [] })
    await vi.waitFor(() => expect(BrowserStopPick).toHaveBeenCalledWith('web-1'))

    expect(cockpit.pendingImages).toEqual([])
    expect(cockpit.chat.at(-1)?.text).toContain('picture')
  })
})
