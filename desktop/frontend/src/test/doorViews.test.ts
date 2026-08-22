// Which room a door lands on, and what "back" means when it does not land on a
// chat (§158.3, §158.9).
//
// This file exists because of one line. `RESTORABLE_VIEWS` is the second
// register of rooms — `NAV` in desks.ts is the first — and nothing tied them
// together, so a room could be added to the nav and be invisible here. The
// comment above that list already named two rooms it had happened to:
// โปรเจกต์ when it opened, and ระบบออโตเมชั่น the day after. ห้องทำงาน was the
// third, and the cost was not cosmetic — an unregistered room is not an
// overlay, so the workbench's native browser window was never told to hide
// behind it, and an F5 inside it came back somewhere else.
//
// So the first test here is the guard that stops a fourth: the two registers
// have to agree, checked rather than remembered.
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import Workroom from '../lib/Workroom.svelte'
import { NAV } from '../lib/desks'
import { SHELLS, setShell, initShell, offeredShells, shell, homeForShell, shellHasChats } from '../lib/shell.svelte'
import {
  cockpit, setActiveView, restoreActiveView, closeOverlay, isOverlayView,
  RESTORABLE_VIEWS, activeViewStorageKey,
} from '../lib/stores/cockpit.svelte'

beforeEach(() => {
  sessionStorage.clear()
  localStorage.removeItem('aetox-shell')
  cockpit.activeView = 'chat'
  cockpit.space = ''
  setShell('assistant')
})

describe('the two registers of rooms', () => {
  // A `page` is a view over data with no session of its own, which is exactly
  // the set a reload has to be able to come back to: there is no conversation
  // underneath to fall back on that would be the right answer instead.
  it('can come back to every page room in the nav', () => {
    for (const room of NAV.filter((n) => n.kind === 'page')) {
      expect(RESTORABLE_VIEWS, `${room.id} is in the nav but not in RESTORABLE_VIEWS`)
        .toContain(room.id)
    }
  })

  // A door's home is not drawn OVER the window — it is what the window is.
  // Getting this backwards is what put a native browser window on top of
  // ห้องทำงาน, because BrowserPane hides itself for exactly the overlay set.
  it('never treats a door home as an overlay', () => {
    for (const s of SHELLS) {
      expect(RESTORABLE_VIEWS, `${s.name}'s home is not restorable`).toContain(s.home)
      expect(isOverlayView(s.home), `${s.name}'s home reads as an overlay`).toBe(false)
    }
  })

  // The other half of the same rule, so the predicate cannot pass the test
  // above by answering false to everything.
  it('treats a page you visit and leave as an overlay', () => {
    for (const view of ['settings', 'office', 'artifacts', 'projects']) {
      expect(isOverlayView(view), `${view} should be an overlay`).toBe(true)
    }
  })

  // The invariant App.svelte's tab strip now leans on: the strip switches
  // between a conversation and the files opened from it, so a door that holds
  // no conversations must hold no room that could open one either. If that ever
  // stops being true the strip needs a different gate, and this says so here
  // rather than by appearing over ห้องทำงาน with a "Chat" tab in it.
  it('gives a door with no conversations only rooms that are views', () => {
    for (const s of SHELLS.filter((d) => d.desk === '')) {
      expect(shellHasChats(s.name)).toBe(false)
      const rooms = NAV.filter((n) => n.shell === s.name)
      expect(rooms.length).toBeGreaterThan(0)
      for (const room of rooms) expect(room.kind).toBe('page')
    }
  })

  it('knows nothing about a view it was never told', () => {
    expect(isOverlayView('C:/some/file.ts')).toBe(false)
    expect(isOverlayView('')).toBe(false)
  })
})

describe('a door that is built but not offered', () => {
  // The owner, 2026-08-22, looking at ห้องทำงาน in the running app: "ไม่แสดงเลย
  // ดีกว่านะ แต่โค้ดยังมี". An empty room behind a door on the menu is a promise
  // the product cannot keep, and the honest version is no button at all rather
  // than a disabled one.
  it('keeps ทีม off the menu while the room behind it is empty', () => {
    const names = offeredShells().map((s) => s.name)
    expect(names).toEqual(['assistant', 'code'])
    expect(names).not.toContain('team')
  })

  // The half that is easy to forget, and the one that would have bitten this
  // machine: the door was in use when it left, and localStorage outlives the
  // build. Restoring it would strand the window behind a door the menu can no
  // longer take it out of.
  it('does not reopen into a door the menu no longer draws', () => {
    localStorage.setItem('aetox-shell', 'team')
    initShell()
    expect(shell.name).toBe('assistant')
  })

  it('still reopens into a door that is offered', () => {
    localStorage.setItem('aetox-shell', 'code')
    initShell()
    expect(shell.name).toBe('code')
  })

  // Off the menu is not deleted. Everything the door does still works when it
  // is switched to directly, which is what keeps `offered: false` a flag to
  // flip rather than code quietly rotting — every test above and below this one
  // reaches ทีม through setShell and would go silent if the door were removed.
  it('is still routed, filtered and restorable underneath', () => {
    setShell('team')
    expect(homeForShell('team')).toBe('lines')
    expect(shellHasChats('team')).toBe(false)
    expect(isOverlayView('lines')).toBe(false)
    restoreActiveView()
    expect(cockpit.activeView).toBe('lines')
  })
})

describe('leaving a page', () => {
  // The bug: closing Settings from behind ทีม ran setActiveView('chat') and
  // handed that door another door's conversation — a sidebar with one room and
  // no history, over a chat that lives at the storefront.
  it('goes back to the door you are standing behind', () => {
    setShell('team')
    setActiveView('settings')
    closeOverlay()
    expect(cockpit.activeView).toBe('lines')
  })

  it('still goes back to the chat behind a door that holds one', () => {
    setShell('assistant')
    setActiveView('office')
    closeOverlay()
    expect(cockpit.activeView).toBe('chat')

    setShell('code')
    setActiveView('settings')
    closeOverlay()
    expect(cockpit.activeView).toBe('chat')
  })

  // Standing in ห้องทำงาน there is nothing behind it, so "back" is a no-op
  // rather than a jump. This is what makes removing the page's back button
  // safe: the key that would have used it has nowhere to go either.
  it('is a no-op in the room that is itself the door', () => {
    setShell('team')
    setActiveView('lines')
    closeOverlay()
    expect(cockpit.activeView).toBe('lines')
  })
})

describe('reopening the window', () => {
  // A relaunch remembers the DOOR — that is localStorage and outlives the run
  // on purpose — but not the room. Reading the view off the door is what stops
  // the ทีม door opening onto a conversation it does not have.
  it('lands on the door home when the run remembers no room', () => {
    setShell('team')
    restoreActiveView()
    expect(cockpit.activeView).toBe('lines')
  })

  it('lands on chat behind the doors that have one', () => {
    for (const name of ['assistant', 'code'] as const) {
      cockpit.activeView = 'settings'
      setShell(name)
      restoreActiveView()
      expect(cockpit.activeView).toBe(homeForShell(name))
      expect(cockpit.activeView).toBe('chat')
    }
  })

  // An F5 *within* a run is the case sessionStorage is for, and ห้องทำงาน has
  // to survive it like every other room.
  it('comes back to the room an F5 left behind', () => {
    setShell('team')
    setActiveView('lines')
    expect(sessionStorage.getItem(activeViewStorageKey)).toBe('lines')

    cockpit.activeView = 'chat'
    restoreActiveView()
    expect(cockpit.activeView).toBe('lines')
  })

  // A stored value that is no longer a room must not win over the door's home,
  // or a renamed room would strand the window on a view nothing renders.
  it('ignores a stored room the app no longer has', () => {
    setShell('team')
    sessionStorage.setItem(activeViewStorageKey, 'floorplan')
    restoreActiveView()
    expect(cockpit.activeView).toBe('lines')
  })
})

describe('ห้องทำงาน', () => {
  // The page is the ทีม door's home, drawn inside the layout rather than over
  // it, so the way out is the door menu and the rooms row that stay on screen.
  // A back button here had to invent a destination, and it invented the wrong
  // one.
  it('offers no way back, because there is nothing behind it', () => {
    render(Workroom)
    expect(screen.queryByText('กลับไปที่แอป')).toBeNull()
    expect(document.querySelector('.settings-back')).toBeNull()
  })

  // .in-main drops the outline and the radius .page-shell wears when it is a
  // full-window overlay — inside .main those would be a card inside a card.
  it('wears the layout shape, not the overlay shape', () => {
    render(Workroom)
    expect(document.querySelector('.page-shell.in-main')).toBeTruthy()
  })

  // Copy that cannot lie (§158.9): the room says it is unbuilt instead of
  // drawing an empty list that implies the list works.
  it('says plainly that it is not built yet', () => {
    render(Workroom)
    expect(screen.getByText('ห้องนี้ยังสร้างไม่เสร็จ')).toBeTruthy()
  })
})
