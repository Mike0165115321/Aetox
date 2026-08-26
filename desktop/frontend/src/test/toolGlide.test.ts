import { describe, it, expect } from 'vitest'
import { toolGlide } from '../lib/toolGlide'

// The block marking the live tool call is drawn two ways — a bar that travels
// (this action) and a per-row block in CSS — and `glide-on` is the switch that
// guarantees only one of them shows. These check the switch, because getting it
// wrong is visible on screen rather than merely wrong on paper.

function timeline(states: Array<'run' | 'done'>): HTMLElement {
  const node = document.createElement('div')
  node.className = 'tool-steps'
  for (const state of states) {
    const row = document.createElement('div')
    row.className = `tool-step ${state}`
    node.append(row)
  }
  document.body.append(node)
  return node
}

describe('the block that follows the live tool row', () => {
  it('hands the row to the travelling bar when one call is running', () => {
    const node = timeline(['done', 'run'])
    toolGlide(node)

    expect(node.querySelector('.tool-glide')).not.toBeNull()
    expect(node.classList.contains('glide-on')).toBe(true)
  })

  // Providers do issue parallel calls, and one bar cannot be in two places, so
  // the per-row block has to take over — which it only does with the class off.
  it('falls back to the per-row block when two calls run at once', () => {
    const node = timeline(['run', 'run'])
    toolGlide(node)

    expect(node.classList.contains('glide-on')).toBe(false)
  })

  // The gap between two calls is an ordinary state: the model narrates, and for
  // those seconds nothing is running. The class must stay ON there, even though
  // no bar is drawn — because the NEXT row is dressed by whatever this class
  // says at the instant it is inserted. With the class off across the gap, an
  // arriving row wore the per-row block at full strength, lost it one frame
  // later, and then had the same block ramped back up under it as the bar faded
  // in. Full, gone, fading in, on every new row: the white blink the owner saw
  // (26 ส.ค.).
  it('keeps the class on across a gap, so an arriving row is not dressed in a block', () => {
    const node = timeline(['done', 'done'])
    toolGlide(node)

    expect(node.classList.contains('glide-on')).toBe(true)
    expect((node.querySelector('.tool-glide') as HTMLElement).style.opacity).toBe('0')
  })

  // A finished timeline read back from the store is the same shape, and must
  // not leave a bar sitting on a call nobody is waiting for.
  it('draws no bar on a timeline where everything is over', () => {
    const node = timeline(['done'])
    toolGlide(node)

    expect((node.querySelector('.tool-glide') as HTMLElement).style.opacity).toBe('0')
  })

  it('takes its bar with it when the timeline goes away', () => {
    const node = timeline(['run'])
    const { destroy } = toolGlide(node)
    destroy()

    expect(node.querySelector('.tool-glide')).toBeNull()
  })
})
