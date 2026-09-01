import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  cockpit, fileKind, attachFileFromPath, attachTabContext, attachmentPreview,
  clearPendingFile, sendUserMessage, selectSession, attachImageFromClipboard, attachImageFromPath,
} from '../lib/stores/cockpit.svelte'
import {
  SaveChatFile, SaveChatImage, SaveChatImageData, SendMessage, LoadSession, ReadImageDataURL, ReadFile,
  BrowserGetText,
} from './mocks/wailsApp'
import { workbench } from '../lib/stores/workbench.svelte'
import { setLocale } from '../lib/i18n.svelte'

beforeEach(() => {
  setLocale('en')
  workbench.tabs = []
  cockpit.pendingFiles = []
  cockpit.pendingImages = []
  cockpit.pendingContexts = []
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

    expect(cockpit.pendingFiles).toEqual([{
      relPath: '.aetox-attachments/1-1.mp4',
      label: 'meeting.mp4',
      kind: 'video',
    }])
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
    expect(cockpit.pendingFiles).toEqual([])
  })

  // A PDF is kind 'file' like a .txt, but `read` refuses it — pointing the
  // model at read is what sent it round the houses (browser_open, web_fetch,
  // an HTML wrapper) before giving up on an attached statement.
  it('points a PDF at pdf_read rather than read', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/3-1.pdf' as any)
    await attachFileFromPath('D:/docs/สรุปการเงิน.PDF')
    await sendUserMessage('สรุปเอกสารนี้หน่อย')

    const sent = vi.mocked(SendMessage).mock.calls[0][0] as string
    expect(sent).toContain('pdf_read')
    expect(sent).toContain('.aetox-attachments/3-1.pdf')
  })

  it('still points a plain text file at read', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/4-1.md' as any)
    await attachFileFromPath('D:/docs/notes.md')
    await sendUserMessage('อ่านให้หน่อย')

    const sent = vi.mocked(SendMessage).mock.calls[0][0] as string
    expect(sent).toContain('read it with read')
    expect(sent).not.toContain('pdf_read')
  })

  // The model only ever gets the path, so the transcript has to carry the
  // label itself — otherwise a sent clip leaves no trace in the bubble.
  it('keeps the attachment visible on the sent message', async () => {
    vi.mocked(SaveChatFile).mockResolvedValue('.aetox-attachments/9-1.mp4' as any)
    await attachFileFromPath('D:/v/clip.mp4')
    await sendUserMessage('สรุปคลิปนี้ให้หน่อย')

    expect(cockpit.chat[0]).toMatchObject({
      role: 'user',
      files: [{ label: 'clip.mp4', kind: 'video' }],
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
      files: [{ label: '17-3.mp4', kind: 'video' }],
    })
    expect(cockpit.chat[1]).toMatchObject({
      text: 'รูปนี้อ่านให้หน่อย',
      images: [{ relPath: '.aetox-attachments/17-1.png' }],
    })
    // the thumbnail is read back off disk, so it lands a tick later
    await vi.waitFor(() => expect(cockpit.chat[1].images?.[0].dataUrl).toBe('data:image/png;base64,AAA'))
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
    expect(cockpit.pendingFiles).toHaveLength(1)
    clearPendingFile(0)
    expect(cockpit.pendingFiles).toEqual([])
  })

  // The composer used to hold exactly one file: attaching a second replaced the
  // first, silently, and the question went out carrying only the last thing
  // picked. Twenty screenshots of a bug is an ordinary thing to hand over.
  it('stages every file picked, not just the last one', async () => {
    vi.mocked(SaveChatFile)
      .mockResolvedValueOnce('.aetox-attachments/5-1.pdf' as any)
      .mockResolvedValueOnce('.aetox-attachments/5-2.csv' as any)
    vi.mocked(SaveChatImage).mockResolvedValue('.aetox-attachments/5-3.png' as any)
    vi.mocked(ReadImageDataURL).mockResolvedValue('data:image/png;base64,DDD' as any)

    await attachFileFromPath('D:/docs/spec.pdf')
    await attachFileFromPath('D:/data/ยอดขาย.csv')
    await attachImageFromPath('D:/shots/bug.png')

    expect(cockpit.pendingFiles.map((f) => f.label)).toEqual(['spec.pdf', 'ยอดขาย.csv'])
    expect(cockpit.pendingImages).toHaveLength(1)

    await sendUserMessage('ดูให้หน่อย')

    // Every one of them is named to the model, and every one of them is on the
    // bubble — a marker line that is missing is a file the answer cannot use.
    const sent = vi.mocked(SendMessage).mock.calls[0][0] as string
    expect(sent).toContain('.aetox-attachments/5-1.pdf')
    expect(sent).toContain('.aetox-attachments/5-2.csv')
    expect(sent).toContain('.aetox-attachments/5-3.png')
    expect(cockpit.chat[0].files).toHaveLength(2)
    expect(cockpit.chat[0].images).toHaveLength(1)
    expect(cockpit.pendingFiles).toEqual([])
    expect(cockpit.pendingImages).toEqual([])
  })

  // Removing the third of ten must leave the other nine standing — the x on a
  // card addresses that card, not the whole stack.
  it('removes one staged file without taking the rest with it', async () => {
    vi.mocked(SaveChatFile)
      .mockResolvedValueOnce('.aetox-attachments/6-1.pdf' as any)
      .mockResolvedValueOnce('.aetox-attachments/6-2.pdf' as any)
      .mockResolvedValueOnce('.aetox-attachments/6-3.pdf' as any)
    await attachFileFromPath('D:/a.pdf')
    await attachFileFromPath('D:/b.pdf')
    await attachFileFromPath('D:/c.pdf')

    clearPendingFile(1)

    expect(cockpit.pendingFiles.map((f) => f.label)).toEqual(['a.pdf', 'c.pdf'])
  })
})

// The other way in: a file already inside the sandbox, dragged onto the composer
// from a workbench tab or from a produced-file card in the reply.
describe('the preview on an attachment card', () => {
  it('shows the first lines that actually say something', () => {
    expect(attachmentPreview('\n\n# Installation\n\n## Requirements\n\n- Go 1.22\n- Node 20\n'))
      .toBe('# Installation\n## Requirements\n- Go 1.22')
  })

  it('cuts a long line before it reaches the DOM', () => {
    // A minified file is one line of hundreds of KB. Clipping it in CSS still
    // means holding all of it in the document.
    const preview = attachmentPreview('x'.repeat(5000))
    expect(preview.length).toBeLessThan(210)
    expect(preview.endsWith('…')).toBe(true)
  })

  it('gives an empty string for a file with nothing in it', () => {
    // The card then draws its head alone rather than an empty box under it.
    expect(attachmentPreview('')).toBe('')
    expect(attachmentPreview('\n  \n\n')).toBe('')
  })

  it('survives the round trip through a stored transcript', async () => {
    vi.mocked(ReadFile).mockResolvedValue('# Installation\n\n## Requirements')
    await attachTabContext('file', 'INSTALL.md', 'INSTALL.md')
    await sendUserMessage('อ่านให้หน่อย')

    // Live, from the pending context...
    expect(cockpit.chat[0].contexts?.[0].preview).toBe('# Installation\n## Requirements')

    // ...and after a reload, rebuilt from the attachment block in the stored
    // text, so a reopened question shows the card it was asked with.
    const stored = vi.mocked(SendMessage).mock.calls[0][0] as string
    vi.mocked(LoadSession).mockResolvedValue([{ role: 'user', text: stored, time: '01:00' }] as any)
    await selectSession({ id: 's1' } as any)
    expect(cockpit.chat[0].contexts?.[0].label).toBe('INSTALL.md')
    expect(cockpit.chat[0].contexts?.[0].preview).toBe('# Installation\n## Requirements')
  })
})

describe('attaching a file dragged in from the workbench', () => {
  // The file-level beforeEach only clears SendMessage; these assert on the
  // bindings the tests above also call.
  beforeEach(() => {
    vi.mocked(SaveChatFile).mockClear()
    vi.mocked(ReadFile).mockClear()
    vi.mocked(ReadImageDataURL).mockClear()
  })

  it('inlines it when it is text', async () => {
    vi.mocked(ReadFile).mockResolvedValue('# บันทึก' as any)
    await attachTabContext('file', 'docs/notes.md', 'notes.md')

    expect(cockpit.pendingContexts).toEqual([{ kind: 'file', label: 'notes.md', content: '# บันทึก' }])
    expect(cockpit.pendingFiles).toEqual([])
  })

  // A restored layout brings browser tabs back as addresses, and the page
  // itself loads only when the tab is looked at. Dragging one that has not
  // been clicked since the app started used to print the engine's own
  // `no browser tab "web-1"` into the chat: an internal id, for a state the
  // user can fix in one click, in a sentence that does not say which click.
  it('says how to fix a page that is listed but not open', async () => {
    workbench.tabs = [{ id: 'web-1', kind: 'browser', name: 'aetox.app', url: 'https://aetox.app' } as any]
    vi.mocked(BrowserGetText).mockRejectedValue(new Error('no browser tab "web-1"'))

    await attachTabContext('browser', 'web-1', 'aetox.app')

    expect(cockpit.pendingContexts).toEqual([])
    const said = cockpit.chat.at(-1)?.text ?? ''
    expect(said).toContain('Click the tab')
    expect(said).not.toContain('web-1')
  })

  // Every other failure still arrives whole. A message this layer cannot
  // explain is worth more raw than smoothed into the one it can.
  it('still passes through a failure it has no better sentence for', async () => {
    workbench.tabs = [{ id: 'web-1', kind: 'browser', name: 'aetox.app', url: 'https://aetox.app' } as any]
    vi.mocked(BrowserGetText).mockRejectedValue(new Error('page crashed'))

    await attachTabContext('browser', 'web-1', 'aetox.app')

    expect(cockpit.chat.at(-1)?.text).toContain('page crashed')
  })

  it('hands over the path when it is not text, instead of failing', async () => {
    // ReadFile refuses a PDF — it is a binary container. That refusal used to
    // surface as "แนบไม่สำเร็จ: Error: binary file cannot be previewed" and the
    // attach was simply lost.
    vi.mocked(ReadFile).mockRejectedValue(new Error('binary file cannot be previewed'))
    await attachTabContext('file', 'output/รายงาน.pdf', 'รายงาน.pdf')

    expect(cockpit.pendingFiles).toEqual([{
      relPath: 'output/รายงาน.pdf', label: 'รายงาน.pdf', kind: 'file',
    }])
    expect(cockpit.chat).toHaveLength(0) // no error line

    // ...and the model is pointed at the tool that opens it.
    await sendUserMessage('อ่านให้หน่อย')
    expect(vi.mocked(SendMessage).mock.calls[0][0] as string).toContain('pdf_read')
  })

  it('takes the same route when the file is merely too large to inline', async () => {
    vi.mocked(ReadFile).mockRejectedValue(new Error('file too large to preview (4194304 bytes)'))
    await attachTabContext('file', 'data/ยอดขาย.csv', 'ยอดขาย.csv')

    expect(cockpit.pendingFiles[0]?.relPath).toBe('data/ยอดขาย.csv')
    expect(cockpit.chat).toHaveLength(0)
  })

  it('reports a failure that is not about inlining, instead of staging a dead path', async () => {
    // ReadFile has seven refusals; only "binary" and "too large" mean the file
    // is fine. Treating all seven as benign staged an attachment pointing at
    // nothing, and the first sign of trouble was the model's own read failing
    // mid-turn — a wasted turn the user cannot attribute to the drag.
    vi.mocked(ReadFile).mockRejectedValue(new Error('open out/gone.md: no such file or directory'))
    await attachTabContext('file', 'out/gone.md', 'gone.md')

    expect(cockpit.pendingFiles).toEqual([])
    expect(cockpit.pendingContexts).toEqual([])
    expect(cockpit.chat).toHaveLength(1)
    expect(cockpit.chat[0].text).toContain('no such file')
  })

  it('shows a picture as a picture, with no second copy on disk', async () => {
    vi.mocked(ReadImageDataURL).mockResolvedValue('data:image/png;base64,BBB' as any)
    await attachTabContext('file', 'shots/หน้าจอ.png', 'หน้าจอ.png')

    expect(cockpit.pendingImages).toEqual([{ relPath: 'shots/หน้าจอ.png', dataUrl: 'data:image/png;base64,BBB' }])
    // Already inside the sandbox — copying it again would leave two of them.
    expect(SaveChatFile).not.toHaveBeenCalled()
    expect(ReadFile).not.toHaveBeenCalled()
  })
})

// An image on the clipboard has no path, and every attach route in this app
// took one — so Ctrl+V, the most ordinary way there is to show the assistant a
// screenshot, did nothing at all. Including a chart copied straight out of an
// answer with the drawing's own คัดลอก button: the copy succeeded and there was
// nowhere in the app to put it.
describe('pasting an image into the composer', () => {
  const pngFile = () =>
    new File([Uint8Array.from([137, 80, 78, 71])], 'clip.png', { type: 'image/png' })

  it('writes the pasted bytes into the sandbox and stages them', async () => {
    vi.mocked(SaveChatImageData).mockResolvedValue('.aetox-attachments/20260808-1.png' as any)
    vi.mocked(ReadImageDataURL).mockResolvedValue('data:image/png;base64,CCC' as any)

    await attachImageFromClipboard(pngFile())

    // The bytes go over as a data URL — there is no path to send.
    expect(vi.mocked(SaveChatImageData).mock.calls[0][0]).toMatch(/^data:image\/png;base64,/)
    expect(cockpit.pendingImages).toEqual([{
      relPath: '.aetox-attachments/20260808-1.png',
      dataUrl: 'data:image/png;base64,CCC',
    }])
  })

  // A refused paste has to say so. Staging nothing and printing nothing is the
  // silence this whole fix exists to end.
  it('says why when the engine refuses the paste', async () => {
    vi.mocked(SaveChatImageData).mockRejectedValue(new Error('ยังแนบรูปชนิด tiff ไม่ได้'))

    await attachImageFromClipboard(pngFile())

    expect(cockpit.pendingImages).toEqual([])
    expect(cockpit.chat.at(-1)?.text).toContain('ยังแนบรูปชนิด tiff ไม่ได้')
  })
})
