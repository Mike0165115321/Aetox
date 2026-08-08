// Taking a drawing out of the app.
//
// A chart the model draws is coloured entirely by the theme it was drawn on —
// currentColor and var(--text-secondary) resolve against the app's live CSS,
// and nowhere else. Serialize the SVG as it stands and every fill dies with
// the stylesheet it left behind: a .svg that opens black-on-white with
// invisible lines, or not at all. So nothing here ships raw SVG. The drawing
// is cloned, the browser is asked what colour every element ACTUALLY is
// (getComputedStyle resolves the variables and currentColor to literal rgb),
// the answers are baked onto the clone as style attributes, and the clone is
// rasterized to PNG on a canvas — an image that looks exactly like what the
// user is pointing at, anywhere they paste it.

import { SaveDrawing } from '../../wailsjs/go/main/App'

const RASTER_SCALE = 2 // export at 2x the on-screen size; chat drawings are small

// The properties a drawing is coloured and lettered with. Baking every
// computed property would swell the markup a hundredfold for no pixel of
// difference — these are the ones the prompt's drawing layer teaches, plus
// the font pair so <text> keeps the app's face instead of Times.
const BAKED_PROPS = [
  'fill', 'fill-opacity', 'stroke', 'stroke-width', 'stroke-opacity',
  'stroke-dasharray', 'stroke-linecap', 'stroke-linejoin', 'opacity',
  'color', 'font-family', 'font-size', 'font-weight', 'letter-spacing',
]

function bakeThemeColors(svg: SVGSVGElement): SVGSVGElement {
  const clone = svg.cloneNode(true) as SVGSVGElement
  const source = [svg as Element, ...svg.querySelectorAll('*')]
  const target = [clone as Element, ...clone.querySelectorAll('*')]
  for (let i = 0; i < source.length && i < target.length; i++) {
    const computed = getComputedStyle(source[i])
    const style = (target[i] as SVGElement).style
    if (!style) continue
    for (const prop of BAKED_PROPS) {
      const value = computed.getPropertyValue(prop)
      if (value !== '') style.setProperty(prop, value)
    }
  }
  return clone
}

// The surface the drawing sits on, for the PNG's background. Transparent is
// the wrong answer: a dark theme's near-white lines pasted into a white
// document vanish — the background the user sees is part of the picture.
function surfaceBehind(el: Element): string {
  for (let node: Element | null = el; node; node = node.parentElement) {
    const bg = getComputedStyle(node).backgroundColor
    if (bg && bg !== 'transparent' && !/rgba\(.*,\s*0\)$/.test(bg)) return bg
  }
  return getComputedStyle(document.body).backgroundColor || '#ffffff'
}

function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('drawing did not rasterize'))
    image.src = url
  })
}

async function drawingToPNGBlob(svg: SVGSVGElement): Promise<Blob> {
  const width = Math.max(svg.clientWidth, 320)
  const box = svg.viewBox.baseVal
  const ratio =
    box && box.width > 0 ? box.height / box.width
    : svg.clientHeight > 0 ? svg.clientHeight / width
    : 9 / 16
  const height = Math.max(1, Math.round(width * ratio))

  const clone = bakeThemeColors(svg)
  clone.setAttribute('width', String(width))
  clone.setAttribute('height', String(height))
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg')

  const markup = new XMLSerializer().serializeToString(clone)
  const url = URL.createObjectURL(new Blob([markup], { type: 'image/svg+xml;charset=utf-8' }))
  try {
    const image = await loadImage(url)
    const canvas = document.createElement('canvas')
    canvas.width = width * RASTER_SCALE
    canvas.height = height * RASTER_SCALE
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('no canvas')
    ctx.fillStyle = surfaceBehind(svg)
    ctx.fillRect(0, 0, canvas.width, canvas.height)
    ctx.drawImage(image, 0, 0, canvas.width, canvas.height)
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('no image'))), 'image/png')
    )
  } finally {
    URL.revokeObjectURL(url)
  }
}

/** Put the drawing on the clipboard as a PNG, ready to paste anywhere. */
export async function copyDrawing(svg: SVGSVGElement): Promise<void> {
  const blob = await drawingToPNGBlob(svg)
  await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })])
}

/** Save the drawing as a PNG file; the engine side owns the dialog. */
export async function saveDrawing(svg: SVGSVGElement): Promise<void> {
  const blob = await drawingToPNGBlob(svg)
  const dataURL = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(new Error('encode failed'))
    reader.readAsDataURL(blob)
  })
  await SaveDrawing(dataURL)
}
