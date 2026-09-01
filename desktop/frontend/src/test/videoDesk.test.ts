// The editor's clips open themselves — into ห้องตัด first.
//
// The Go side decides WHICH files (desktop/video_desk.go, and its own tests
// measure that). What is measured here is the window's half: an arrival feeds
// the session's ledger and opens the room, the room follows §187 across
// sessions, a user's close of the room holds (single-tab fallback), and the
// `cutting_room` tool outranks that close because it is somebody asking.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  workbench, openFileTab, adoptWorkbenchSession, switchWorkbenchSession,
  routeDeskEvent, mediaOrigin, mediaLedger, cutroom, closeTab,
  type MediaOrigin,
} from '../lib/stores/workbench.svelte'
import { ReadFile } from './mocks/wailsApp'

// Through the door, not around it (§187.3) — the same rule deskCrossSession
// states: entering anywhere else would test a path the window does not use.
const openMedia = (sessionId: string, data: MediaOrigin) =>
  Promise.resolve(routeDeskEvent('open-media', { sessionId, data }))
const openRoom = (sessionId: string) =>
  Promise.resolve(routeDeskEvent('open-cutroom', { sessionId }))

const clip = (over: Partial<MediaOrigin> = {}): MediaOrigin => ({
  path: 'output/s1/cut.mp4',
  name: 'cut.mp4',
  role: 'result',
  tool: 'kinocut_video_trim',
  operation: 'trim',
  duration: 18,
  sizeMB: 4.2,
  resolution: '1280x720',
  ...over,
})

const source = (path = 'output/s1/talk.mp4'): MediaOrigin => ({
  path, name: path.split('/').pop() ?? path, role: 'source', tool: 'kinocut_video_trim',
})

const saved = (id: string) =>
  JSON.parse(localStorage.getItem(`aetox-workbench:${id}`) ?? '{"tabs":[]}') as {
    tabs: { kind: string; path?: string; mine?: boolean }[]
  }

const roomTab = () => workbench.tabs.find((t) => t.kind === 'cutroom')

beforeEach(() => {
  vi.clearAllMocks()
  workbench.tabs.length = 0
  workbench.activeId = ''
  cutroom.pick = ''
  localStorage.clear()
  vi.mocked(ReadFile).mockResolvedValue('' as any)
})

// Session ids are unique per test: the ledger and the remembered room-close
// live per session in module state, which is exactly what they do in the app.
let n = 0
const freshSession = () => `s-${++n}`

describe('the editor opens its own room', () => {
  it('opens ห้องตัด on the first arrival and puts the result on its player', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)

    await openMedia(s, source())
    await openMedia(s, clip())

    expect(roomTab()).toBeTruthy()
    expect(workbench.activeId).toBe('cutroom')
    // The ledger holds both, the player is on the result.
    expect(mediaLedger().map((r) => r.role)).toEqual(['source', 'result'])
    expect(cutroom.pick).toBe('output/s1/cut.mp4')
    // No loose file tabs beside the room — the room IS the delivery.
    expect(workbench.tabs.filter((t) => t.kind === 'file')).toHaveLength(0)
  })

  it('keeps the origin numbers the tool reported', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)

    await openMedia(s, clip({ plan: { total: 45, kept: [{ start: 5, end: 23 }] } }))

    const origin = mediaOrigin('output/s1/cut.mp4')
    expect(origin?.operation).toBe('trim')
    expect(origin?.plan?.total).toBe(45)
    // Found however the path is spelled: Windows hands the same file back
    // several ways, and a pane that missed would silently show no line.
    expect(mediaOrigin('output\\S1\\cut.mp4')?.operation).toBe('trim')
  })

  it('replaces a re-rendered path in the ledger instead of growing it', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)

    await openMedia(s, clip({ duration: 18 }))
    await openMedia(s, clip({ duration: 12 }))

    expect(mediaLedger()).toHaveLength(1)
    expect(mediaLedger()[0].duration).toBe(12)
  })

  it('feeds the open room without stealing focus for a mere source', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)
    await openMedia(s, clip())
    await openFileTab('notes.md', 'notes.md')
    expect(workbench.activeId).toBe('file-notes.md')

    await openMedia(s, source('output/s1/other.mp4'))

    // In the ledger, but the user stays where they moved to.
    expect(mediaLedger().some((r) => r.path === 'output/s1/other.mp4')).toBe(true)
    expect(workbench.activeId).toBe('file-notes.md')

    // A result is the answer, and it fronts the room again.
    await openMedia(s, clip({ path: 'output/s1/cut2.mp4', name: 'cut2.mp4' }))
    expect(workbench.activeId).toBe('cutroom')
    expect(cutroom.pick).toBe('output/s1/cut2.mp4')
  })

  it('honours the user closing the room, falling back to single tabs', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)
    await openMedia(s, clip())
    await closeTab(roomTab()!)

    await openMedia(s, clip({ path: 'output/s1/cut2.mp4', name: 'cut2.mp4' }))

    // Not reopened against the user's veto — the clip still arrives, quieter.
    expect(roomTab()).toBeUndefined()
    expect(workbench.tabs.map((t) => t.path)).toEqual(['output/s1/cut2.mp4'])
  })

  it('reopens on the cutting_room tool, because that is somebody asking', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)
    await openMedia(s, clip())
    await closeTab(roomTab()!)

    await openRoom(s)

    expect(roomTab()).toBeTruthy()
    expect(workbench.activeId).toBe('cutroom')
    // And the veto is lifted: the next render feeds the room again.
    await openMedia(s, clip({ path: 'output/s1/cut3.mp4', name: 'cut3.mp4' }))
    expect(cutroom.pick).toBe('output/s1/cut3.mp4')
  })

  it('parks a background chat’s work on that chat’s own desk (§187)', async () => {
    const s = freshSession()
    const bg = freshSession()
    await adoptWorkbenchSession(s)

    await openMedia(bg, clip())
    await openRoom(bg)

    // Nothing in front of somebody reading something else...
    expect(workbench.tabs).toHaveLength(0)
    // ...the file parked as a tab, the room parked as its kind.
    expect(saved(bg).tabs.map((t) => t.kind).sort()).toEqual(['cutroom', 'file'])

    // Opening that chat finds the room, its ledger already fed.
    await switchWorkbenchSession(bg)
    expect(mediaLedger().map((r) => r.path)).toEqual(['output/s1/cut.mp4'])
    expect(cutroom.pick).toBe('output/s1/cut.mp4')
  })

  it('ignores an arrival with no file in it', async () => {
    const s = freshSession()
    await adoptWorkbenchSession(s)

    await openMedia(s, { path: '', name: '', role: 'result' })

    expect(workbench.tabs).toHaveLength(0)
    expect(mediaLedger()).toHaveLength(0)
  })
})
