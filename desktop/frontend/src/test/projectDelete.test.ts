// Deleting a โปรเจกต์ from the room that owns it.
//
// Owner, 27 ส.ค.: "หน้าโปรเจคไม่มีปุ่มลบโปรเจคครับผม" — the page could make a
// project and could not unmake one, so the only way to get rid of one was to
// find the folder in Explorer. Every other shelf in the app (ผลงาน, presets,
// skills, MCP servers) has the door beside the thing it deletes.
//
// What is pinned here is the shape of that door rather than the binding call:
// it asks first, through the app's one dialog, and it leaves the list clean
// afterwards. The dialog is the point — a project holds copies of the user's
// files, and a click that removed them with no warning would be the one gesture
// in the room that cannot be walked back.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Projects from '../lib/Projects.svelte'
import { Spaces, SessionsInSpace, DeleteSpace } from './mocks/wailsApp'

const project = {
  name: 'เปิดร้านกาแฟ',
  path: 'C:/data/project/เปิดร้านกาแฟ',
  contextPath: 'C:/data/project/เปิดร้านกาแฟ/context',
  contextFiles: ['สรุป.md'],
  chats: 2,
  updatedAt: new Date().toISOString(),
}

const openTheProject = async () => {
  render(Projects, { onClose: () => {} })
  fireEvent.click(await screen.findByText(project.name, { selector: '.pp-title' }))
  return screen.findByText('ลบโปรเจกต์')
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(Spaces).mockResolvedValue([project])
  vi.mocked(SessionsInSpace).mockResolvedValue([])
})

describe('deleting a project', () => {
  it('asks before it deletes, and does nothing if the answer is no', async () => {
    fireEvent.click(await openTheProject())

    // The dialog names the project verbatim: the user checks the name, not the
    // sentence about it.
    expect(await screen.findByText(project.name, { selector: '.confirm-detail' })).toBeTruthy()
    fireEvent.click(screen.getByText('ยกเลิก'))

    await waitFor(() => expect(document.querySelector('.confirm-overlay')).toBeNull())
    expect(DeleteSpace).not.toHaveBeenCalled()
  })

  it('deletes the project and walks back out to the list', async () => {
    fireEvent.click(await openTheProject())
    // Read back off the disk, never assumed: the page redraws from what Spaces
    // answers after the delete, which is the same rule adding and removing
    // context files follows.
    vi.mocked(Spaces).mockResolvedValue([])
    fireEvent.click(await screen.findByText('ลบ', { selector: '.confirm-go' }))

    await waitFor(() => expect(DeleteSpace).toHaveBeenCalledWith(project.name))
    await waitFor(() => {
      // Out of the project, and the card is gone from the gallery behind it.
      expect(screen.queryByText('ลบโปรเจกต์')).toBeNull()
      expect(screen.queryByText(project.name, { selector: '.pp-title' })).toBeNull()
      expect(screen.getByText(/ยังไม่มีโปรเจกต์/)).toBeTruthy()
    })
  })

  it('says why when the engine refuses', async () => {
    vi.mocked(DeleteSpace).mockRejectedValueOnce(new Error('ลบไม่ได้: โฟลเดอร์ถูกใช้งานอยู่'))

    fireEvent.click(await openTheProject())
    fireEvent.click(await screen.findByText('ลบ', { selector: '.confirm-go' }))

    expect(await screen.findByText(/โฟลเดอร์ถูกใช้งานอยู่/)).toBeTruthy()
  })
})

// The door on the shelf, not only inside the room.
//
// Owner, on the gallery: "หน้านี้มีแต่ปุ่มเพิ่มไม่มีปุ่มลบ". The first cut put
// the delete inside the project, reasoning that a gallery is scanned and
// clicked fast. That is a reason to make it quiet, not a reason to make the
// user open a project to get rid of it — the shelf is where you stand when you
// notice you have one too many.
describe('deleting a project from the gallery', () => {
  it('asks from the card, without making the user open the project first', async () => {
    render(Projects, { onClose: () => {} })

    const del = await screen.findByLabelText(`ลบโปรเจกต์ ${project.name}`)
    fireEvent.click(del)

    expect(await screen.findByText(project.name, { selector: '.confirm-detail' })).toBeTruthy()
    // Still on the list: arming the delete must not walk into the project.
    expect(screen.queryByText('เริ่มแชทในโปรเจกต์นี้')).toBeNull()

    vi.mocked(Spaces).mockResolvedValue([])
    fireEvent.click(screen.getByText('ลบ', { selector: '.confirm-go' }))

    await waitFor(() => expect(DeleteSpace).toHaveBeenCalledWith(project.name))
    await waitFor(() => expect(screen.queryByText(project.name, { selector: '.pp-title' })).toBeNull())
  })

  // The card is a button. A delete nested inside it would be a button inside a
  // button, which browsers do not honour and which would make the whole card
  // ambiguous to click.
  it('keeps the delete out of the card button', async () => {
    render(Projects, { onClose: () => {} })
    await screen.findByText(project.name, { selector: '.pp-title' })

    const card = document.querySelector('.proj-card') as HTMLElement
    expect(card.querySelector('.proj-card-del')).toBeNull()
    expect(document.querySelector('.proj-cardwrap > .proj-card-del')).toBeTruthy()
  })
})
