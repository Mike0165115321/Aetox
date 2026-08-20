// ชี้ให้เอเจนดู, aimed at a slide.
//
// The browser half is covered in pagePick.test.ts. What is different here is
// the ending: a page pick tells the model what the user is looking at, and a
// slide pick tells it which file to change. Everything below is about that
// difference and about the frame the overlay is put into, because a deck is not
// a native window and the two halves reach each other directly.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  deckPick, startDeckPick, stopDeckPick, deckPickChipContent, type PagePick,
} from '../lib/workbench/pagePick.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import {
  DeckPickScript, DeckStopPickScript, DeckCaptureDrawing, SaveChatImageData, ReadImageDataURL,
} from './mocks/wailsApp'

const INK = 'data:image/png;base64,aW5r'

/** A frame with a document that has a window, the way the panel's iframe does.
 *  createHTMLDocument gives no defaultView, and the ink layer is measured
 *  through one. */
function deckFrame(): Document {
  const frame = document.createElement('iframe')
  document.body.appendChild(frame)
  const doc = frame.contentDocument!
  doc.documentElement.innerHTML = '<head></head><body><section class="slide"></section></body>'
  return doc
}

/** The ink the overlay would have left standing, stubbed: jsdom has no canvas. */
function layInk(doc: Document, size?: { w: number; h: number }): HTMLCanvasElement {
  const canvas = doc.createElement('canvas')
  ;(canvas as unknown as { __aetoxOverlay: number }).__aetoxOverlay = 1
  canvas.width = size?.w ?? doc.defaultView!.innerWidth
  canvas.height = size?.h ?? doc.defaultView!.innerHeight
  canvas.toDataURL = () => INK
  doc.documentElement.appendChild(canvas)
  return canvas
}

const PICK: PagePick = {
  selector: 'section.slide:nth-child(2) h2', tag: 'h2', text: 'เครื่องเดียวครบทุกบทบาท',
  html: '<h2>เครื่องเดียว<br>ครบทุกบทบาท</h2>',
  path: 'section.slide', w: 812, h: 168,
  color: '#ffffff', background: 'rgba(0, 0, 0, 0)',
}

const DECK = 'output/20260820-063040.715/xiaomi-17t-pro-sales.html'

/** The window property the injected overlay calls back on. */
function callback(): ((raw: string) => void) | undefined {
  return (window as unknown as Record<string, unknown>).__aetoxDeckPick as never
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.pendingContext = null
  cockpit.pendingImage = null
  cockpit.chat.length = 0
  deckPick.path = null
  deckPick.mode = 'pick'
  deckPick.capturing = false
  document.body.innerHTML = ''
  delete (window as unknown as Record<string, unknown>).__aetoxDeckPick
  vi.mocked(DeckPickScript).mockResolvedValue('window.__armed = 1')
  vi.mocked(DeckStopPickScript).mockResolvedValue('window.__armed = 0')
  vi.mocked(DeckCaptureDrawing).mockResolvedValue('data:image/png;base64,c2hvdA==')
  vi.mocked(SaveChatImageData).mockResolvedValue('images/deck-marks.png')
  vi.mocked(ReadImageDataURL).mockResolvedValue('data:image/png;base64,c2hvdA==')
})

describe('what the model is handed', () => {
  it('names the file and the slide, because this one can be edited', () => {
    const content = deckPickChipContent(DECK, 3, [PICK])
    expect(content).toContain(DECK)
    expect(content).toContain('slide 3')
    // The same body the page chip carries: a selector alone is a guess, and the
    // rendered colour is what greps back to the rule that produced it.
    expect(content).toContain(PICK.selector)
    expect(content).toContain('812×168')
    expect(content).toContain('#ffffff')
  })

  it('says how many when more than one was pointed at', () => {
    const content = deckPickChipContent(DECK, 1, [PICK, { ...PICK, selector: 'p.sub' }])
    expect(content).toContain('Elements')
    expect(content).toContain('[2] p.sub')
  })
})

describe('arming it on a deck', () => {
  it('puts the overlay inside the frame, not in the app', async () => {
    const doc = document.implementation.createHTMLDocument('deck')
    await startDeckPick(doc, DECK, 2)

    expect(deckPick.path).toBe(DECK)
    // The script came from Go rather than from a copy living in the frontend.
    expect(DeckPickScript).toHaveBeenCalledTimes(1)
    // And it ran in the deck's document. The node is taken out again once it
    // has, so what proves it ran is the effect, not a leftover tag.
    expect(doc.querySelector('script')).toBeNull()
  })

  it('turns the composer chip on a real answer', async () => {
    const doc = document.implementation.createHTMLDocument('deck')
    await startDeckPick(doc, DECK, 4)
    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string

    callback()!(JSON.stringify({ __aetox: 'pick', token, cancelled: false, picks: [PICK] }))

    expect(cockpit.pendingContext?.kind).toBe('pick')
    expect(cockpit.pendingContext?.label).toBe(PICK.selector)
    expect(cockpit.pendingContext?.content).toContain('slide 4')
    // Answered once and disarmed, so a second envelope has nothing to land on.
    expect(deckPick.path).toBeNull()
    expect(callback()).toBeUndefined()
  })

  it('ignores an answer to a question nobody asked', async () => {
    const doc = document.implementation.createHTMLDocument('deck')
    await startDeckPick(doc, DECK, 1)

    // A deck is a file the user keeps and may carry any script at all. Without
    // the token it could push whatever it liked into the composer.
    callback()!(JSON.stringify({ __aetox: 'pick', token: 'not-the-one', picks: [PICK] }))

    expect(cockpit.pendingContext).toBeNull()
    expect(deckPick.path).toBe(DECK)
  })

  it('leaves nothing behind when the user cancels', async () => {
    const doc = document.implementation.createHTMLDocument('deck')
    await startDeckPick(doc, DECK, 1)
    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string

    callback()!(JSON.stringify({ __aetox: 'pick', token, cancelled: true, picks: [] }))

    expect(cockpit.pendingContext).toBeNull()
    expect(deckPick.path).toBeNull()
  })

  it('pressing the button again turns it off instead of arming a second one', async () => {
    const doc = document.implementation.createHTMLDocument('deck')
    await startDeckPick(doc, DECK, 1)
    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string
    await startDeckPick(doc, DECK, 1)

    expect(deckPick.path).toBeNull()
    expect(DeckPickScript).toHaveBeenCalledTimes(1)
    // Asserted by behaviour rather than by the property: while the frame is still
    // being told to take the overlay down, a sink stands where the callback was,
    // and what has to be true is that nothing more reaches the composer.
    callback()?.(JSON.stringify({ __aetox: 'pick', token, picks: [PICK] }))
    expect(cockpit.pendingContext).toBeNull()
  })

  it('drops the callback the moment the panel goes, without waiting for the frame', () => {
    stopDeckPick(null)
    expect(deckPick.path).toBeNull()
    expect(callback()).toBeUndefined()
  })
})

// A browser tab is photographed by the engine and that is the whole of it. A
// deck has no such call, so the picture is made: the slide is rendered the way
// every export renders it and the ink goes over the top. What is checked here is
// that the two halves meet — the right slide, and the ink at the slide's size.
describe('drawing on a slide', () => {
  it('sends the marks with the slide they were drawn on', async () => {
    const doc = deckFrame()
    await startDeckPick(doc, DECK, 5, 'draw')
    expect(deckPick.mode).toBe('draw')
    // The mode travels to the overlay, or it would come up in pointing mode with
    // a pencil on the button.
    expect(JSON.parse(vi.mocked(DeckPickScript).mock.calls[0][1] as string).mode).toBe('draw')

    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string
    layInk(doc)
    callback()!(JSON.stringify({ __aetox: 'pick', token, drawn: true, picks: [PICK] }))
    await vi.waitFor(() => expect(cockpit.pendingImage).not.toBeNull())

    expect(DeckCaptureDrawing).toHaveBeenCalledWith(DECK, 5, INK)
    expect(cockpit.pendingImage?.relPath).toBe('images/deck-marks.png')
    // The elements under the marks are worth having too, and they say which file.
    expect(cockpit.pendingContext?.content).toContain(DECK)
    expect(deckPick.capturing).toBe(false)
  })

  it('attaches the picture even when the marks landed on nothing nameable', async () => {
    const doc = deckFrame()
    await startDeckPick(doc, DECK, 1, 'draw')
    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string
    layInk(doc)

    callback()!(JSON.stringify({ __aetox: 'pick', token, drawn: true, picks: [] }))
    await vi.waitFor(() => expect(cockpit.pendingImage).not.toBeNull())

    // No chip, because there is nothing to say — but the marks are the point.
    expect(cockpit.pendingContext).toBeNull()
  })

  it('hands over the ink at the slide size, not at the backing store size', async () => {
    const doc = deckFrame()
    const view = doc.defaultView!
    // jsdom draws nothing, so the redraw is stubbed at the prototype: the canvas
    // layInk puts down carries its own toDataURL and is unaffected, which is
    // exactly the split being asserted — the marks go through a second canvas.
    const RESCALED = 'data:image/png;base64,cmVzY2FsZWQ='
    const proto = (view as unknown as { HTMLCanvasElement: { prototype: HTMLCanvasElement } })
      .HTMLCanvasElement.prototype
    proto.getContext = (() => ({ drawImage: () => {} })) as never
    proto.toDataURL = (() => RESCALED) as never

    await startDeckPick(doc, DECK, 2, 'draw')
    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string
    // What a 2x screen leaves behind: a canvas twice the CSS box in each axis.
    layInk(doc, { w: view.innerWidth * 2, h: view.innerHeight * 2 })

    callback()!(JSON.stringify({ __aetox: 'pick', token, drawn: true, picks: [PICK] }))
    await vi.waitFor(() => expect(DeckCaptureDrawing).toHaveBeenCalled())

    // Redrawn at the slide's own size rather than passed straight through, so Go
    // composites 1:1 onto the rendered slide instead of resampling a 2x picture.
    expect(vi.mocked(DeckCaptureDrawing).mock.calls[0][2]).toBe(RESCALED)
  })

  it('says so when the picture cannot be taken, and does not stay busy', async () => {
    vi.mocked(DeckCaptureDrawing).mockRejectedValue(new Error('เด็คนี้มี 8 สไลด์'))
    const doc = deckFrame()
    await startDeckPick(doc, DECK, 9, 'draw')
    const token = vi.mocked(DeckPickScript).mock.calls[0][0] as string
    layInk(doc)

    callback()!(JSON.stringify({ __aetox: 'pick', token, drawn: true, picks: [PICK] }))
    await vi.waitFor(() => expect(cockpit.chat.length).toBe(1))

    expect(cockpit.pendingImage).toBeNull()
    expect(deckPick.capturing).toBe(false)
    // Failing to photograph the marks is not failing altogether: what was under
    // them was read before the controls came off and is already in the composer.
    expect(cockpit.pendingContext?.content).toContain(PICK.selector)
  })

  it('pressing the other mode switches instead of stacking a second overlay', async () => {
    const doc = deckFrame()
    await startDeckPick(doc, DECK, 1, 'pick')
    await startDeckPick(doc, DECK, 1, 'draw')

    expect(deckPick.path).toBe(DECK)
    expect(deckPick.mode).toBe('draw')
    expect(DeckPickScript).toHaveBeenCalledTimes(2)
  })
})
