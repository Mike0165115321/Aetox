// The project doors, refused mid-turn, saying so.
//
// Owner, 22 ส.ค.: "มี Chat นึงกำลังทำงาน แต่กดแชตใหม่ไม่ได้" — pressing the +
// beside a project in the โค้ด door while a turn ran did nothing at all. Not a
// refusal, not a message: nothing.
//
// Two gates answer "may this switch happen", and they answer for different
// sets. turnStillRunning() is the window's, and it can only see the chat ON
// SCREEN. guardSessionSwitch is the engine's, and it refuses while a turn runs
// ANYWHERE — because re-rooting moves the sandbox out from under it. So with a
// chat working off screen the local gate passed, the engine refused, and the
// rejected promise was never caught: the click died silently.
//
// What is pinned here is the catch, on all three doors that re-root. The engine
// is the one that decides; the window's job is to repeat what it said.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { OpenProjectPath, OpenProjectFolder, ClearProjectFocus } from './mocks/wailsApp'
import { cockpit, openProject, openFolder, clearProjectFocus } from '../lib/stores/cockpit.svelte'

const busy = 'เอเจนกำลังทำงานอยู่ — รอให้เสร็จ หรือกดหยุดก่อน แล้วค่อยสลับแชท'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.sessionError = ''
  // The window is looking at an idle chat: the working one is off screen, which
  // is the whole case. With awaitingReply true the local gate answers first and
  // the engine is never asked.
  cockpit.awaitingReply = false
})

describe('a project door refused by the engine', () => {
  it('says so when the + beside a project is pressed mid-turn', async () => {
    OpenProjectPath.mockRejectedValueOnce(new Error(busy))
    await openProject('D:/work/app')
    expect(cockpit.sessionError).toBe(busy)
  })

  it('says so when a folder is opened mid-turn', async () => {
    OpenProjectFolder.mockRejectedValueOnce(new Error(busy))
    await openFolder()
    expect(cockpit.sessionError).toBe(busy)
  })

  it('says so when project focus is cleared mid-turn', async () => {
    ClearProjectFocus.mockRejectedValueOnce(new Error(busy))
    await clearProjectFocus()
    expect(cockpit.sessionError).toBe(busy)
  })

  // And the other half: a door that went through clears the last refusal, or
  // the sentence outlives the state it described and sits over a project that
  // opened perfectly well.
  it('clears the refusal once a project does open', async () => {
    cockpit.sessionError = busy
    OpenProjectPath.mockResolvedValueOnce({ root: 'D:/work/app', name: 'app', focused: true })
    await openProject('D:/work/app')
    expect(cockpit.sessionError).toBe('')
  })
})
