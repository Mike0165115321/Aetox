// Asking the same question again — the retry / regenerate / switch / edit
// family. What is being pinned down here is mostly bookkeeping the user would
// only notice when it is wrong: which bubble is replaced, which one survives,
// and whether the text a retry sends is the text that was actually sent.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, sendUserMessage, retryFailedTurn, regenerateReply, switchVariant, resendEdited,
} from '../lib/stores/cockpit.svelte'
import {
  SendMessage, RetryFailedTurn, RegenerateReply, SwitchVariant, ResendEdited,
} from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  cockpit.chat.length = 0
  cockpit.awaitingReply = false
  cockpit.pendingImage = null
  cockpit.pendingFile = null
  cockpit.pendingContext = null
})

describe('a turn that failed', () => {
  it('keeps what was actually sent, attachment lines and all', async () => {
    cockpit.pendingFile = { relPath: '.aetox-attachments/clip.mp4', label: 'clip.mp4', kind: 'video' }
    SendMessage.mockRejectedValueOnce(new Error('dial tcp: lookup api.deepseek.com: no such host'))

    await sendUserMessage('ทำไมแบตขึ้นช้า')

    const failed = cockpit.chat.at(-1)!
    expect(failed.failed).toBe(true)
    // The composer's attachment is gone by now, so a retry that re-sent only the
    // visible bubble text would silently drop the video.
    expect(failed.failedText).toContain('ทำไมแบตขึ้นช้า')
    expect(failed.failedText).toContain('clip.mp4')
  })

  it('replaces only the error bubble on retry — the question stays put', async () => {
    SendMessage.mockRejectedValueOnce(new Error('no such host'))
    await sendUserMessage('แบตผม 17')
    expect(cockpit.chat).toHaveLength(2)

    RetryFailedTurn.mockResolvedValueOnce({ text: 'ชาร์จไปเรื่อยๆ ครับ' })
    await retryFailedTurn(1)

    expect(RetryFailedTurn).toHaveBeenCalledWith(cockpit.chat[0].text)
    expect(cockpit.chat).toHaveLength(2)
    expect(cockpit.chat[0].role).toBe('user')
    expect(cockpit.chat[1].text).toBe('ชาร์จไปเรื่อยๆ ครับ')
    expect(cockpit.chat[1].failed).toBeUndefined()
    // Not SendMessage: that would send the question again on top of the copy the
    // failed attempt already left in the model's context.
    expect(SendMessage).toHaveBeenCalledTimes(1)
  })

  it('leaves a retryable bubble behind when the retry fails too', async () => {
    SendMessage.mockRejectedValueOnce(new Error('no such host'))
    await sendUserMessage('แบตผม 17')

    RetryFailedTurn.mockRejectedValueOnce(new Error('still no such host'))
    await retryFailedTurn(1)

    const failed = cockpit.chat.at(-1)!
    expect(failed.failed).toBe(true)
    expect(failed.failedText).toBe(cockpit.chat[0].text)
  })
})

describe('answering again', () => {
  async function answered(reply = 'คำตอบแรก') {
    SendMessage.mockResolvedValueOnce({ text: reply })
    await sendUserMessage('ทำไมแบตขึ้นช้า')
  }

  it('keeps the previous answer switchable instead of overwriting it', async () => {
    await answered()
    RegenerateReply.mockResolvedValueOnce({
      text: 'คำตอบที่สอง',
      variants: [{ text: 'คำตอบแรก' }, { text: 'คำตอบที่สอง' }],
      active: 1,
    })

    await regenerateReply(false)

    // Still one bubble — a second answer is an alternative, not another message.
    expect(cockpit.chat).toHaveLength(2)
    const reply = cockpit.chat[1]
    expect(reply.text).toBe('คำตอบที่สอง')
    expect(reply.activeVariant).toBe(1)
    expect(reply.variants?.map((v) => v.text)).toEqual(['คำตอบแรก', 'คำตอบที่สอง'])
  })

  it('says so when it put files back first', async () => {
    await answered()
    RegenerateReply.mockResolvedValueOnce({
      text: 'ใหม่', variants: [{ text: 'เก่า' }, { text: 'ใหม่' }], active: 1,
      reverted: ['src/a.ts', 'src/b.ts'],
    })

    await regenerateReply(true)

    expect(RegenerateReply).toHaveBeenCalledWith(true)
    expect(cockpit.chat[1].revertedFiles).toEqual(['src/a.ts', 'src/b.ts'])
  })

  it('keeps the answer on screen when the re-run fails', async () => {
    await answered('คำตอบแรก')
    RegenerateReply.mockRejectedValueOnce(new Error('no such host'))

    await regenerateReply(false)

    const reply = cockpit.chat[1]
    // The engine put its memory back, so the answer above is still the real one.
    expect(reply.text).toBe('คำตอบแรก')
    expect(reply.error).toContain('no such host')
    expect(cockpit.chat).toHaveLength(2)
  })

  it('refuses to re-run a failed turn — that is Retry\'s job', async () => {
    SendMessage.mockRejectedValueOnce(new Error('no such host'))
    await sendUserMessage('แบตผม 17')

    await regenerateReply(false)

    expect(RegenerateReply).not.toHaveBeenCalled()
  })

  it('refuses while a turn is in flight', async () => {
    await answered()
    cockpit.awaitingReply = true

    await regenerateReply(false)

    expect(RegenerateReply).not.toHaveBeenCalled()
  })
})

describe('switching between answers', () => {
  it('shows the chosen answer and its own thinking', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'คำตอบที่สอง' })
    await sendUserMessage('ทำไมแบตขึ้นช้า')
    Object.assign(cockpit.chat[1], {
      variants: [
        { text: 'คำตอบแรก', reasoning: 'first pass', thinkSecs: 3 },
        { text: 'คำตอบที่สอง', reasoning: 'second pass', thinkSecs: 5 },
      ],
      activeVariant: 1,
    })

    SwitchVariant.mockResolvedValueOnce({
      text: 'คำตอบแรก',
      variants: [{ text: 'คำตอบแรก' }, { text: 'คำตอบที่สอง' }],
      active: 0,
    })
    await switchVariant(0)

    const reply = cockpit.chat[1]
    expect(reply.text).toBe('คำตอบแรก')
    expect(reply.activeVariant).toBe(0)
    // The thinking panel has to follow the answer, not stay on the one that was
    // generated last.
    expect(reply.reasoning).toBe('first pass')
    expect(reply.thinkSecs).toBe(3)
    // The engine is told too: the conversation must continue from this answer.
    expect(SwitchVariant).toHaveBeenCalledWith(0)
  })

  it('does nothing on a bubble that was only ever answered once', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'คำตอบเดียว' })
    await sendUserMessage('ถาม')

    await switchVariant(1)

    expect(SwitchVariant).not.toHaveBeenCalled()
  })
})

describe('editing the question', () => {
  it('replaces the exchange rather than keeping the old answer', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'ตอบคำถามเดิม' })
    await sendUserMessage('ทำไมแบตขึ้นชา')
    ResendEdited.mockResolvedValueOnce({ text: 'ตอบคำถามที่แก้แล้ว' })

    await resendEdited('ทำไมแบตขึ้นช้า', false)

    expect(ResendEdited).toHaveBeenCalledWith('ทำไมแบตขึ้นช้า', false)
    expect(cockpit.chat).toHaveLength(2)
    expect(cockpit.chat[0].text).toBe('ทำไมแบตขึ้นช้า')
    // The answer to the wording the user says they did not mean is gone, not
    // parked as a variant: two answers to two different questions are not
    // alternatives to each other.
    expect(cockpit.chat[1].text).toBe('ตอบคำถามที่แก้แล้ว')
  })

  it('re-sends the attachment with the edited wording', async () => {
    cockpit.pendingImage = { relPath: '.aetox-attachments/shot.png', dataUrl: 'data:image/png;base64,x' }
    SendMessage.mockResolvedValueOnce({ text: 'ตอบ' })
    await sendUserMessage('อ่านรูปนี้')
    ResendEdited.mockResolvedValueOnce({ text: 'ตอบใหม่' })

    await resendEdited('อ่านรูปนี้ให้ละเอียด', false)

    // The picture is attached to the question, not to its wording. Fixing a typo
    // must not quietly detach it — the chip would still be on the bubble while
    // the model had stopped receiving the file.
    const [sent] = ResendEdited.mock.calls.at(-1)!
    expect(sent).toContain('อ่านรูปนี้ให้ละเอียด')
    expect(sent).toContain('.aetox-attachments/shot.png')
    expect(cockpit.chat[0].imageDataUrl).toBe('data:image/png;base64,x')
    expect(cockpit.chat[0].text).toBe('อ่านรูปนี้ให้ละเอียด')
  })

  it('ignores an empty edit', async () => {
    SendMessage.mockResolvedValueOnce({ text: 'ตอบ' })
    await sendUserMessage('ถาม')

    await resendEdited('   ', false)

    expect(ResendEdited).not.toHaveBeenCalled()
    expect(cockpit.chat).toHaveLength(2)
  })
})
