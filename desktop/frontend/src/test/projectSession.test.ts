// Starting a chat inside a โปรเจกต์ (COMPANY.md §84, DECISIONS §90).
//
// This exists because of the shape of the bug it pins, not because the feature
// needed a test to be believed. The room's button called the Go binding
// directly: the engine opened a new session inside the project, the window went
// on showing the session that was already in front of the user, and the chat
// the click had just created was unreachable — reported as "มันพาเด้งมาหน้าหลัก
// แล้วโปรเจคก็หายไปเลย". Two facts, engine-side and window-side, and nothing
// keeping them one.
//
// So what is pinned is the seam: the door has to move both, and the chat has to
// say which project it is in afterwards. A page that calls a binding directly
// will fail here.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Projects from '../lib/Projects.svelte'
import Chat from '../lib/Chat.svelte'
import Sidebar from '../lib/Sidebar.svelte'
import { NewSessionInSpace, NewSessionAt, Spaces, SessionsInSpace, CurrentSessionID } from './mocks/wailsApp'
import { cockpit } from '../lib/stores/cockpit.svelte'

const deskButton = (label: string): HTMLButtonElement => {
  const el = Array.from(document.querySelectorAll('.desk-btn'))
    .find((b) => b.querySelector('.t')?.textContent?.trim() === label)
  if (!el) throw new Error("desk button not found: " + label)
  return el as HTMLButtonElement
}

const chatProps = {
  task: { title: '', steps: [] } as any,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  messages: [] as any[],
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

const project = {
  name: 'เปิดร้านกาแฟ',
  path: 'C:/data/project/เปิดร้านกาแฟ',
  contextPath: 'C:/data/project/เปิดร้านกาแฟ/context',
  contextFiles: [],
  chats: 0,
  updatedAt: new Date().toISOString(),
}

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.desk = ''
  cockpit.chair = ''
  cockpit.space = ''
  cockpit.activeView = 'chat'
  cockpit.chat.length = 0
  vi.mocked(CurrentSessionID).mockResolvedValue('20260807-120000.000')
  vi.mocked(Spaces).mockResolvedValue([project])
  vi.mocked(SessionsInSpace).mockResolvedValue([])
})

describe('starting a chat inside a project', () => {
  it('opens the session in the engine and moves the window with it', async () => {
    render(Projects, { onClose: () => {} })

    fireEvent.click(await screen.findByText(project.name, { selector: '.pp-title' }))
    fireEvent.click(await screen.findByText('เริ่มแชทในโปรเจกต์นี้'))

    await waitFor(() => expect(NewSessionInSpace).toHaveBeenCalledWith(project.name))
    // The half that was missing. Without it the engine is in the project and
    // the window is still in the chat it was showing before the click.
    await waitFor(() => {
      expect(cockpit.space).toBe(project.name)
      expect(cockpit.desk).toBe('assistant')
      expect(cockpit.chair).toBe('')
      expect(cockpit.activeView).toBe('chat')
    })
  })
})

// Where the window says you are standing. A project chat runs at the
// assistant's desk — that is how it gets its tools — so the desk rule alone lit
// up ผู้ช่วย while the room the chat belongs to stayed dark. True of the engine
// and wrong on screen: "ทำไมมันพามาที่ผู้ช่วยล่ะครับ … แต่นี่คือแชทของโปรเจคนะครับ".
describe('the room the window is standing in', () => {
  it('is the project when the open chat belongs to one', async () => {
    cockpit.desk = 'assistant'
    cockpit.space = project.name

    render(Sidebar, { onOpenSettings: () => {} })

    await waitFor(() => {
      expect(deskButton('โปรเจกต์').classList.contains('active')).toBe(true)
      expect(deskButton('ผู้ช่วย').classList.contains('active')).toBe(false)
    })
  })

  // And the nav has to be a way OUT of the project, not only a light on it.
  // "Already at this desk" was true of a project chat — it runs at the
  // assistant's — so the button did nothing at all: "ทำไมกดปุ่มหน้าผู้ช่วย
  // ไม่ได้ครับ".
  it('leaves the project when the desk it runs at is clicked', async () => {
    cockpit.desk = 'assistant'
    cockpit.space = project.name

    render(Sidebar, { onOpenSettings: () => {} })
    fireEvent.click(deskButton('ผู้ช่วย'))

    await waitFor(() => {
      expect(NewSessionAt).toHaveBeenCalledWith('assistant')
      expect(cockpit.space).toBe('')
    })
  })

  it('is the desk again for a chat in no project', async () => {
    cockpit.desk = 'assistant'
    cockpit.space = ''

    render(Sidebar, { onOpenSettings: () => {} })

    await waitFor(() => expect(deskButton('ผู้ช่วย').classList.contains('active')).toBe(true))
  })
})

describe('a chat that belongs to a project', () => {
  it('says which project it is in, with a way back to the room', async () => {
    cockpit.space = project.name

    render(Chat, chatProps)

    // A trail, not a sentence: the project is the level above this chat.
    fireEvent.click(await screen.findByText(project.name, { selector: '.crumb-up' }))
    await waitFor(() => expect(cockpit.activeView).toBe('projects'))
  })

  // Most chats are held outside every project and must look exactly as they did
  // before this room existed.
  it('says nothing at all when the chat is in no project', () => {
    render(Chat, chatProps)

    expect(document.querySelector('.crumb-strip')).toBeNull()
  })
})
