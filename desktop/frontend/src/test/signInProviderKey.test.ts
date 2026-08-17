// The connect step, and the rule underneath it: a provider reached by signing
// in must never be asked for an API key.
//
// Codex is the case that made this a bug rather than a nicety. It *requires*
// credentials, so every "requires a key and has none" check in the UI drew a
// password box for it — one whose only possible outcome is a 401, because the
// key a user could paste belongs to api.openai.com and Codex is a ChatGPT
// subscription at chatgpt.com. The composer banner asked that wrong question
// too, so it is covered here as well.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import Chat from '../lib/Chat.svelte'
import Onboarding from '../lib/Onboarding.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { cockpit } from '../lib/stores/cockpit.svelte'
import {
  AcceptsAPIKey, HasAPIKey, RequiresAPIKey, SupportedProviders,
  SignInMethods, StartSignIn, SetAPIKey, SwitchProvider,
} from './mocks/wailsApp'

const baseProps = {
  task: { title: '', steps: [] } as any,
  messages: [] as any,
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
}

const modelOn = (provider: string) =>
  ({ provider, modelName: 'gpt-5.5', thinkLevel: '', approval: 'ask', wireFormat: '' }) as any

// Codex: credentials required, no key that exists. Everything else: keyed.
const signInOnly = (name: string) => name === 'codex'

beforeEach(() => {
  // Call records only — the implementations set below survive it. Without
  // this, "was a key ever sent for a local runtime" reads the previous test's
  // calls and passes on someone else's history.
  vi.clearAllMocks()
  localStorage.clear()
  setLocale('th')
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  cockpit.model.provider = ''
  vi.mocked(RequiresAPIKey).mockImplementation(async (n: string) => n !== 'ollama')
  vi.mocked(HasAPIKey).mockResolvedValue(false)
  vi.mocked(AcceptsAPIKey).mockImplementation(async (name: string) => !signInOnly(name))
  vi.mocked(SupportedProviders).mockResolvedValue(['codex', 'deepseek', 'ollama'])
  vi.mocked(SignInMethods).mockResolvedValue([])
})

describe('composer key banner', () => {
  it('offers no key field for a sign-in provider', async () => {
    const { container } = render(Chat, { ...baseProps, model: modelOn('codex') })
    await waitFor(() => expect(vi.mocked(AcceptsAPIKey)).toHaveBeenCalledWith('codex'))
    await waitFor(() => expect(container.querySelector('.api-key-banner')).toBeNull())
  })

  it('still offers one for a provider whose key is a real credential', async () => {
    const { container } = render(Chat, { ...baseProps, model: modelOn('deepseek') })
    await waitFor(() => expect(container.querySelector('.api-key-banner')).toBeTruthy())
  })
})

describe('the connect step', () => {
  async function openConnect() {
    render(Onboarding)
    ;(await screen.findByText('ไทย')).click()
    await waitFor(() => expect(screen.getByText('ต่อสมองให้ Aetox')).toBeTruthy())
  }

  const answers = () =>
    [...document.querySelectorAll('.ob-big')].map((b) => b.querySelector('.t')?.textContent?.trim())

  const cellNamed = (name: string) =>
    [...document.querySelectorAll('.ob-cell')].find((c) => c.querySelector('.nm')?.textContent?.trim() === name) as HTMLElement

  it('asks what the user already has, in those words, not in brand names', async () => {
    await openConnect()
    expect(answers()).toEqual([
      'มีบัญชีที่จ่ายรายเดือนอยู่แล้ว',
      'ใช้ API คีย์ของผู้ให้บริการอื่นๆ',
      'รันโมเดลภายในเครื่อง Local 100%',
    ])
    // No provider list until the question is answered.
    expect(document.querySelectorAll('.ob-cell').length).toBe(0)
  })

  it('shows each answer the marks of the providers behind it', async () => {
    await openConnect()
    const rows = [...document.querySelectorAll('.ob-big')]
    // codex behind the account answer, deepseek behind the key one, ollama local.
    expect(rows[0].querySelectorAll('.ob-marks .mk').length).toBe(1)
    expect(rows[1].querySelectorAll('.ob-marks .mk').length).toBe(1)
    expect(rows[2].querySelectorAll('.ob-marks .mk').length).toBe(1)
  })

  it('never offers a way out without connecting', async () => {
    await openConnect()
    const links = [...document.querySelectorAll('.ob-link')].map((l) => l.textContent?.trim())
    // Back is the only link. No skip, and no "I have nothing" answer.
    expect(links).toEqual(['ย้อนกลับ'])
    expect(answers()).not.toContain('ยังไม่มีอะไรเลย')
  })

  it('sends the sign-in answer straight to a sign-in, never to a key field', async () => {
    await openConnect()
    ;(document.querySelectorAll('.ob-big')[0] as HTMLElement).click()
    await waitFor(() => expect(screen.getByText('ลงชื่อเข้าใช้เจ้าไหน')).toBeTruthy())

    cellNamed('codex').click()
    await waitFor(() => expect(vi.mocked(StartSignIn)).toHaveBeenCalledWith('codex'))
    expect(document.querySelector('input[type="password"]')).toBeNull()
  })

  it('asks for the key only after the provider is chosen, and says it worked', async () => {
    await openConnect()
    ;(document.querySelectorAll('.ob-big')[1] as HTMLElement).click()
    await waitFor(() => expect(screen.getByText('คีย์ของเจ้าไหน')).toBeTruthy())
    expect(document.querySelector('input[type="password"]')).toBeNull()

    cellNamed('deepseek').click()
    const field = await waitFor(() => {
      const el = document.querySelector('input[type="password"]') as HTMLInputElement | null
      expect(el).toBeTruthy()
      return el!
    })
    expect(field.placeholder).toContain('deepseek')
    field.value = 'sk-test'
    field.dispatchEvent(new Event('input', { bubbles: true }))
    ;(screen.getByText('ต่อเลย')).click()

    await waitFor(() => expect(vi.mocked(SetAPIKey)).toHaveBeenCalledWith('deepseek', 'sk-test'))
    expect(vi.mocked(SwitchProvider)).toHaveBeenCalledWith('deepseek')
    await waitFor(() => expect(screen.getByText('ต่อ deepseek เรียบร้อย')).toBeTruthy())
  })

  it('connects a local runtime on the tile press, with no key asked for', async () => {
    await openConnect()
    ;(document.querySelectorAll('.ob-big')[2] as HTMLElement).click()
    await waitFor(() => expect(screen.getByText('รันด้วยอะไร')).toBeTruthy())

    cellNamed('ollama').click()
    await waitFor(() => expect(vi.mocked(SwitchProvider)).toHaveBeenCalledWith('ollama'))
    expect(vi.mocked(SetAPIKey)).not.toHaveBeenCalled()
  })

  it('goes back from the provider wall to the question', async () => {
    await openConnect()
    ;(document.querySelectorAll('.ob-big')[1] as HTMLElement).click()
    await waitFor(() => expect(screen.getByText('คีย์ของเจ้าไหน')).toBeTruthy())
    ;(screen.getByText('ย้อนกลับ')).click()
    await waitFor(() => expect(screen.getByText('ต่อสมองให้ Aetox')).toBeTruthy())
  })
})
