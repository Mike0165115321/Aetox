import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, fileKind, attachFileFromPath, clearPendingFile, sendUserMessage, selectSession,
} from '../lib/stores/cockpit.svelte'
import { SaveChatFile, SendMessage, LoadSession, ReadImageDataURL } from './mocks/wailsApp'

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

  // The model only ever gets the path, so the transcript has to carry the
  // label itself — otherwise a sent clip leaves no trace in the bubble.
  it('keeps the attachment visible on the sent message', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/9-1.mp4' as any)
    await attachFileFromPath('D:/v/clip.mp4')
    await sendUserMessage('สรุปคลิปนี้ให้หน่อย')

    expect(cockpit.chat[0]).toMatchObject({
      role: 'user',
      attachLabel: 'clip.mp4',
      attachKind: 'video',
    })
  })

  // What the DB stores is the sent text, markers and all. Reopening a session
  // must fold them back into chips instead of printing the raw path line.
  it('restores attachments when a stored session is reopened', async () => {
    vi.mocked(LoadSession).mockResolvedValue([
      {
        role: 'user', time: '00:52',
        text: 'คลิปนี้สรุปให้หน่อย\n\n[attachment: user-attached video — read its speech with audio_transcribe, its on-screen text with video_ocr] .aetox-attachments/17-3.mp4',
      },
      {
        role: 'user', time: '00:50',
        text: 'รูปนี้อ่านให้หน่อย\n\n[attachment: user-attached image — read it with image_ocr] .aetox-attachments/17-1.png',
      },
    ] as any)
    vi.mocked(ReadImageDataURL).mockResolvedValue('data:image/png;base64,AAA' as any)

    await selectSession({ id: 's1' } as any)

    expect(cockpit.chat[0]).toMatchObject({
      text: 'คลิปนี้สรุปให้หน่อย', // the marker line is gone from the bubble
      attachLabel: '17-3.mp4',
      attachKind: 'video',
    })
    expect(cockpit.chat[1]).toMatchObject({
      text: 'รูปนี้อ่านให้หน่อย',
      imageRelPath: '.aetox-attachments/17-1.png',
    })
    // the thumbnail is read back off disk, so it lands a tick later
    await vi.waitFor(() => expect(cockpit.chat[1].imageDataUrl).toBe('data:image/png;base64,AAA'))
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
