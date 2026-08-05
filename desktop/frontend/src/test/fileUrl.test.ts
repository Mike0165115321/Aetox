import { describe, it, expect } from 'vitest'
import { fileURL } from '../lib/fileUrl'
import { fileView } from '../lib/stores/workbench.svelte'

// The address a pane hands to <img>, <video> and <iframe>. Everything the file
// host can serve arrives through here, so a filename this gets wrong is a file
// that silently does not open.
describe('fileURL', () => {
  it('keeps separators as separators and encodes the rest', () => {
    expect(fileURL('out/report.pdf')).toBe('/aetox-file/out/report.pdf')
  })

  it('accepts the backslashes RelativizePath answers with on Windows', () => {
    expect(fileURL('out\\sub\\clip.mp4')).toBe('/aetox-file/out/sub/clip.mp4')
  })

  it('encodes spaces and Thai filenames', () => {
    expect(fileURL('shots/หน้าจอ 2.png')).toBe(
      '/aetox-file/shots/' + encodeURIComponent('หน้าจอ 2.png'),
    )
  })

  // A literal # would cut the path short at the server, and a literal % would
  // be read as the start of an escape — both turn into a file that isn't found.
  it('encodes characters a URL would otherwise eat', () => {
    expect(fileURL('a#b/c%d.png')).toBe('/aetox-file/a%23b/c%25d.png')
  })

  // The guard on the Go side refuses these anyway; not spelling them into a URL
  // in the first place means the refusal is never reached.
  it('drops empty segments', () => {
    expect(fileURL('/out//report.pdf')).toBe('/aetox-file/out/report.pdf')
  })
})

describe('fileView', () => {
  it('is case-insensitive about extensions', () => {
    expect(fileView('A.PNG')).toBe('image')
    expect(fileView('B.Mp4')).toBe('video')
  })

  it('leaves anything without a pane to be read as text', () => {
    for (const p of ['notes.md', 'main.go', 'data.xlsx', 'deck.pptx', 'Makefile']) {
      expect(fileView(p)).toBeUndefined()
    }
  })
})
