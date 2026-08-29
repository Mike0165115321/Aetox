// Going back further than the last turn, through the doors the composer uses.
//
// The Go side proves the restore itself (desktop/rewind_test.go). What this
// file is about is the half a user actually touches: the list arriving beside
// the undo chip, the offer naming its files before anything happens, and the
// result landing in the transcript in the same words undo already uses — one
// act at two distances should not read as two features.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { cockpit, refreshUndo, rewindTo, pendingRestore } from '../lib/stores/cockpit.svelte'
import { PendingUndo, PendingRestore, RestorePoints, RewindTo } from './mocks/wailsApp'

const points = [
  { id: 'tree-c', at: '2026-08-29T14:32:00+07:00', label: 'third ask' },
  { id: 'tree-b', at: '2026-08-29T14:10:00+07:00', label: 'second ask' },
  { id: 'tree-a', at: '2026-08-29T13:58:00+07:00', label: 'first ask' },
]

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat = []
  cockpit.undoFiles = []
  cockpit.restorePoints = []
  vi.mocked(PendingUndo).mockResolvedValue([] as any)
  vi.mocked(RestorePoints).mockResolvedValue([] as any)
})

describe('the list behind the undo chip', () => {
  it('arrives with the chip, in the same refresh', async () => {
    vi.mocked(PendingUndo).mockResolvedValue(['a.go'] as any)
    vi.mocked(RestorePoints).mockResolvedValue(points as any)

    await refreshUndo()

    expect(cockpit.undoFiles).toEqual(['a.go'])
    expect(cockpit.restorePoints.map((p) => p.label)).toEqual(['third ask', 'second ask', 'first ask'])
  })

  // A safety net that shouts when it is absent is worse than one that is quiet:
  // the same rule refreshUndo already followed, now for the list beside it.
  it('is empty rather than broken when the engine cannot answer', async () => {
    vi.mocked(RestorePoints).mockRejectedValue(new Error('no store'))
    cockpit.restorePoints = points as any

    await refreshUndo()

    expect(cockpit.restorePoints).toEqual([])
  })
})

describe('being asked before anything happens', () => {
  it('names the files one point would put back', async () => {
    vi.mocked(PendingRestore).mockResolvedValue(['parser.go', 'parser_test.go'] as any)

    expect(await pendingRestore('tree-a')).toEqual(['parser.go', 'parser_test.go'])
    expect(PendingRestore).toHaveBeenCalledWith('tree-a')
  })

  it('answers nothing rather than throwing when the point is gone', async () => {
    vi.mocked(PendingRestore).mockRejectedValue(new Error('stale id'))
    expect(await pendingRestore('tree-gone')).toEqual([])
  })
})

describe('going back', () => {
  it('reports what came back, into the transcript', async () => {
    vi.mocked(RewindTo).mockResolvedValue({ files: ['a.go', 'b.go'] } as any)

    await rewindTo('tree-a')

    expect(RewindTo).toHaveBeenCalledWith('tree-a')
    const said = cockpit.chat.at(-1)!
    expect(said.role).toBe('agent')
    expect(said.text).toContain('- a.go')
    expect(said.text).toContain('- b.go')
  })

  // The rule undo already had: a rewind that quietly spares some files is as
  // hard to trust as one that quietly ate them.
  it('says which files it deliberately left alone', async () => {
    vi.mocked(RewindTo).mockResolvedValue({ files: ['a.go'], kept: ['mine.md'] } as any)

    await rewindTo('tree-a')

    expect(cockpit.chat.at(-1)!.text).toContain('- mine.md')
  })

  it('carries the engine\'s own reason when nothing moved', async () => {
    vi.mocked(RewindTo).mockResolvedValue({ files: [], reason: 'that point is no longer on this chat\'s list' } as any)

    await rewindTo('tree-gone')

    expect(cockpit.chat.at(-1)!.text).toContain('no longer on this chat')
  })

  it('refreshes the chip and the list afterwards, so neither offers the present', async () => {
    vi.mocked(RewindTo).mockResolvedValue({ files: ['a.go'] } as any)
    cockpit.undoFiles = ['a.go']
    cockpit.restorePoints = points as any

    await rewindTo('tree-a')

    expect(PendingUndo).toHaveBeenCalled()
    expect(RestorePoints).toHaveBeenCalled()
    expect(cockpit.undoFiles).toEqual([])
  })

  it('says so in the transcript when the call itself fails', async () => {
    vi.mocked(RewindTo).mockRejectedValue(new Error('git is gone'))

    await rewindTo('tree-a')

    expect(cockpit.chat.at(-1)!.text).toContain('git is gone')
  })
})
