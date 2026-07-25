import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, fileKind, attachFileFromPath, clearPendingFile, sendUserMessage,
} from '../lib/stores/cockpit.svelte'
import { SaveChatFile, SendMessage } from './mocks/wailsApp'

beforeEach(() => {
  cockpit.pendingFile = null
  cockpit.pendingImage = null
  cockpit.pendingContext = null
  cockpit.chat = []
  vi.mocked(SendMessage).mockClear()
  vi.mocked(SendMessage).mockResolvedValue('ok' as any)
})

describe('attaching a file from the composer', () => {
  it('classifies by extension so the model can be pointed at the right tool', () => {
    expect(fileKind('C:/x/shot.PNG')).toBe('image')
    expect(fileKind('/home/a/talk.m4a')).toBe('audio')
    expect(fileKind('meeting.mkv')).toBe('video')
    expect(fileKind('notes.pdf')).toBe('file')
    expect(fileKind('noext')).toBe('file')
  })

  it('stages a clip as a sandbox path, never as inlined content', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/1-1.mp4' as any)
    await attachFileFromPath('D:/videos/meeting.mp4')

    expect(cockpit.pendingFile).toEqual({
      relPath: '.aetox-attachments/1-1.mp4',
      label: 'meeting.mp4',
      kind: 'video',
    })
  })

  it('names the tool that opens this kind of file', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/2-1.m4a' as any)
    await attachFileFromPath('D:/rec/standup.m4a')
    await sendUserMessage('สรุปให้หน่อย')

    const sent = vi.mocked(SendMessage).mock.calls[0][0] as string
    expect(sent).toContain('สรุปให้หน่อย')
    expect(sent).toContain('.aetox-attachments/2-1.m4a')
    expect(sent).toContain('audio_transcribe')
    // A 300MB clip must never end up inside the prompt.
    expect(sent).not.toContain('```')
    expect(cockpit.pendingFile).toBeNull()
  })

  it('a video points at both tools, since it has speech and a screen', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/3-1.mp4' as any)
    await attachFileFromPath('D:/v/demo.mp4')
    await sendUserMessage('')

    const sent = vi.mocked(SendMessage).mock.calls[0][0] as string
    expect(sent).toContain('audio_transcribe')
    expect(sent).toContain('video_ocr')
  })

  it('can be removed before sending', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/4-1.pdf' as any)
    await attachFileFromPath('D:/doc/spec.pdf')
    expect(cockpit.pendingFile).not.toBeNull()
    clearPendingFile()
    expect(cockpit.pendingFile).toBeNull()
  })
})
