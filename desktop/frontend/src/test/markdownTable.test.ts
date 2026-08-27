// A wide table has to scroll, and for two months it silently could not.
//
// `.markdown-body table` carried `display:block; width:max-content;
// max-width:100%; overflow-x:auto` with a comment saying that let a wide table
// scroll instead of squeezing. Measured in a real browser on 2026-08-27 against
// the reading column's actual width, it does the opposite: max-width clamps the
// used width and the table lays its columns out inside the clamp, so
// scrollWidth comes back equal to clientWidth. A 1,994px table reported 860 by
// 860 and dropped its last column off the right edge (owner's screenshot).
//
// jsdom lays nothing out, so it cannot re-measure that. What it CAN pin is the
// structural half the measurement pointed at: the element that scrolls must not
// be the element that sizes itself to its content, so there have to be two.
import { describe, it, expect } from 'vitest'
import { renderMarkdown } from '../lib/markdown'

const TABLE = ['| a | b |', '| --- | --- |', '| 1 | 2 |'].join('\n')

describe('a rendered table gets a window to scroll in', () => {
  it('wraps the table rather than asking it to scroll itself', () => {
    const host = document.createElement('div')
    host.innerHTML = renderMarkdown(TABLE)

    const table = host.querySelector('table')
    expect(table).toBeTruthy()
    const win = table!.parentElement
    expect(win?.className).toBe('table-scroll')
  })

  it('wraps every table on the page, not just the first', () => {
    const host = document.createElement('div')
    host.innerHTML = renderMarkdown(`${TABLE}\n\ntext between\n\n${TABLE}`)
    expect(host.querySelectorAll('table').length).toBe(2)
    expect(host.querySelectorAll('.table-scroll').length).toBe(2)
    for (const t of host.querySelectorAll('table')) {
      expect(t.parentElement?.className).toBe('table-scroll')
    }
  })

  // Re-rendering the same content is ordinary here: a file preview re-renders
  // on every keystroke. Windows inside windows would nest a scrollbar per pass.
  it('does not nest a window inside a window on a second pass', () => {
    const host = document.createElement('div')
    host.innerHTML = renderMarkdown(TABLE)
    const once = host.innerHTML
    host.innerHTML = renderMarkdown(TABLE)
    expect(host.innerHTML).toBe(once)
    expect(host.querySelectorAll('.table-scroll').length).toBe(1)
  })

  it('leaves prose alone', () => {
    const host = document.createElement('div')
    host.innerHTML = renderMarkdown('just a paragraph')
    expect(host.querySelector('.table-scroll')).toBeNull()
  })
})
