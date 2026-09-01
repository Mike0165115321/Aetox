// The list of ways to open a tab has always had two groups, and always been
// written twice.
//
// Git, Pull requests and แผนที่โค้ด are gated on the โค้ด desk, because a
// working tree is what that desk is held inside. On any other desk the list
// simply came up shorter, with nothing on screen naming what had gone or why
// (owner, 31 ส.ค., with the screenshot). The heading is that gate written down.
//
// It is asserted on BOTH surfaces here — the + menu and the panel an empty
// desk shows — because they were two copies of the same seven rows, and the
// heading went into one of them. The owner was looking at the other, and the
// change he had just approved appeared not to exist. They are one snippet now,
// and these tests are what stops them becoming two again.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Workbench from '../lib/workbench/Workbench.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { workbench } from '../lib/stores/workbench.svelte'
import { setLocale } from '../lib/i18n.svelte'

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  workbench.tabs.length = 0
  workbench.activeId = ''
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})

async function openPlusMenu(container: HTMLElement) {
  await fireEvent.click(container.querySelector('.plus-btn') as HTMLButtonElement)
  await tick()
}

/** The rows of one surface, headings included, in the order they are drawn. */
function rowsOf(container: HTMLElement, surface: '.plus-menu' | '.insp-start'): string[] {
  const root = container.querySelector(surface)
  if (!root) return []
  return Array.from(root.querySelectorAll('.plus-menu-item, .plus-menu-head'))
    .map((el) => el.textContent?.trim() ?? '')
}

describe('the group heading, on both surfaces that list the tabs', () => {
  for (const surface of ['.plus-menu', '.insp-start'] as const) {
    it(`names the code rows on the desk that has them (${surface})`, async () => {
      cockpit.desk = 'coding'
      const { container } = render(Workbench)
      await openPlusMenu(container)

      // It heads the group rather than floating above the whole list:
      // everything before it is what every desk gets.
      const rows = rowsOf(container, surface)
      expect(rows.indexOf('Code pages')).toBe(4)
      // Anchored to the heading rather than to the end of the list, so adding
      // a row to the code group does not make this test wrong about where the
      // heading sits.
      expect(rows.slice(5)).toEqual(['Git', 'Pull requests', 'Code map'])
      // And exactly one heading: the four rows above have nothing to explain.
      expect(rows.filter((r) => r === 'Code pages')).toHaveLength(1)
    })

    // The heading lives inside the same {#if} as the rows, so it cannot outlive
    // them — a label over an empty stretch, or worse, over the four rows it
    // does not describe.
    it(`leaves with the rows it names (${surface})`, async () => {
      cockpit.desk = 'general'
      const { container } = render(Workbench)
      await openPlusMenu(container)

      const rows = rowsOf(container, surface)
      expect(rows).not.toContain('Code pages')
      expect(rows).not.toContain('Git')
    })
  }

  // One definition, rendered twice. Written as an equality rather than as two
  // lists, so a row added to one place cannot pass by being spelled the same.
  it('draws the same rows in the menu and on the empty desk', async () => {
    cockpit.desk = 'coding'
    const { container } = render(Workbench)
    await openPlusMenu(container)

    expect(rowsOf(container, '.plus-menu')).toEqual(rowsOf(container, '.insp-start'))
    expect(rowsOf(container, '.plus-menu').length).toBeGreaterThan(0)
  })
})
