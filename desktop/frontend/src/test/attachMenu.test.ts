// What the + button offers, and what each row asks the dialog for.
//
// On 31 ส.ค. an owner could not attach a .docx. Nothing in the attachment path
// refused it: the copy takes any file and `read` opens Office documents. The
// only thing missing was the extension, from one hand-written pattern string
// inside the native dialog's collapsed filter dropdown — a place where a type
// that is absent and a type the app cannot take look exactly the same.
//
// So the list moved into the app, where it can be read without opening
// anything. These tests hold the two halves that made it worth moving: every
// row says what happens to that kind of file, and every row asks the Go side
// for its own filter rather than all four sharing one.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Chat from '../lib/Chat.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { GuideTopics, ListChairs, PickAttachments } from './mocks/wailsApp'

const baseProps = {
  messages: [] as any[],
  task: { title: '', steps: [] } as any,
  awaitingReply: false,
  agentStatus: '',
  toolSteps: [] as any[],
  streamingText: '',
  reasoningText: '',
  onSend: () => {},
  onSwitchProvider: async () => {},
  onSwitchThinkLevel: async () => {},
  onSwitchModel: async () => {},
  onSubmitAPIKey: async () => {},
  model: { provider: 'deepseek', modelName: 'v4', thinkLevel: 'high', approval: 'ask', wireFormat: '' } as any,
}

beforeEach(() => {
  setLocale('en')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  cockpit.backgroundTasks = []
  cockpit.backgroundSteps = []
  cockpit.pendingImages = []
  cockpit.pendingFiles = []
  cockpit.pendingContexts = []
  vi.mocked(GuideTopics).mockResolvedValue([] as any)
  vi.mocked(ListChairs).mockResolvedValue([] as any)
  vi.mocked(PickAttachments).mockClear()
  vi.mocked(PickAttachments).mockResolvedValue([] as any)
})

function plus(container: HTMLElement): HTMLButtonElement {
  return container.querySelector('.attach-pick > .icobtn') as HTMLButtonElement
}

function rows(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll('.attach-menu .stance-item'))
}

async function openMenu(container: HTMLElement) {
  await fireEvent.click(plus(container))
  await tick()
}

describe('the attach menu', () => {
  it('opens off the + button and closes again', async () => {
    const { container } = render(Chat, baseProps as any)
    expect(container.querySelector('.attach-menu')).toBeNull()

    await openMenu(container)
    expect(container.querySelector('.attach-menu')).not.toBeNull()

    await fireEvent.click(plus(container))
    await tick()
    expect(container.querySelector('.attach-menu')).toBeNull()
  })

  // The whole point of moving the list out of the dialog: what the app takes
  // is readable without opening anything, and each row says what the
  // assistant will do with that kind of file rather than only naming it.
  it('names every kind this app accepts, and what happens to it', async () => {
    const { container } = render(Chat, baseProps as any)
    await openMenu(container)

    expect(rows(container).map((r) => r.querySelector('.nm')?.textContent)).toEqual([
      'Images', 'Documents', 'Video and audio', 'Other files',
    ])
    for (const row of rows(container)) {
      expect(row.querySelector('.d')?.textContent?.trim()).toBeTruthy()
    }
    // The type that started this has to be visible without opening a dialog.
    expect(container.querySelector('.attach-menu')?.textContent).toContain('docx')
    // Drag and Ctrl+V are the other two ways in, and neither has a button.
    expect(container.querySelector('.attach-menu .folder-note')?.textContent).toContain('Ctrl+V')
  })

  // Both menus open at once, drawn on top of each other (owner, 31 ส.ค., with
  // the screenshot). Every trigger on this row stops its click reaching the
  // outside-click closer — it has to, or the click that opens a menu closes it
  // again — so nothing was clearing the neighbour.
  it('closes the menu beside it rather than drawing over it', async () => {
    const { container } = render(Chat, baseProps as any)

    await fireEvent.click(container.querySelector('.stance-chip') as HTMLButtonElement)
    await tick()
    expect(container.querySelector('.stance-menu')).not.toBeNull()

    await openMenu(container)
    expect(container.querySelector('.attach-menu')).not.toBeNull()
    expect(container.querySelector('.stance-menu')).toBeNull()

    // ...and back, so neither one is the privileged half of the pair.
    await fireEvent.click(container.querySelector('.stance-chip') as HTMLButtonElement)
    await tick()
    expect(container.querySelector('.attach-menu')).toBeNull()
    expect(container.querySelector('.stance-menu')).not.toBeNull()
  })

  // A row that opened the same unfiltered dialog as its neighbour would put
  // the user back in front of the dropdown this menu exists to replace.
  it('asks the dialog for its own row, and closes behind itself', async () => {
    const { container } = render(Chat, baseProps as any)

    for (const [index, group] of ['image', 'document', 'media', ''].entries()) {
      await openMenu(container)
      await fireEvent.click(rows(container)[index])
      await tick()

      expect(vi.mocked(PickAttachments).mock.calls.at(-1)?.[0]).toBe(group)
      expect(container.querySelector('.attach-menu')).toBeNull()
    }
    expect(vi.mocked(PickAttachments)).toHaveBeenCalledTimes(4)
  })
})
