// The pane behind a file card. A file the agent produced can be gone by the
// time anyone opens it — the agent can delete files, and session output folders
// age out — and the pane used to say "this app cannot preview it, but a program
// on your machine can" about a file that was not there at all. Only the click
// revealed otherwise, as a raw OS error in red.
//
// The card itself stays: it is history, and the turn really did produce that
// file. What must not outlive the file is the offer.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import ExternalFilePane from '../lib/workbench/ExternalFilePane.svelte'
import { setLocale } from '../lib/i18n.svelte'
import { FileStillThere, OpenFileExternally } from './mocks/wailsApp'

beforeEach(() => {
  vi.clearAllMocks()
  setLocale('en')
  vi.mocked(FileStillThere).mockResolvedValue('there' as any)
})

const props = { path: 'output/s1/report.xlsx', reason: 'spreadsheets open in Excel' }

describe('the file pane', () => {
  it('offers to open a file that is still there', async () => {
    render(ExternalFilePane, props)
    await waitFor(() => expect(screen.getByRole('button')).toBeTruthy())
    expect(screen.getByText('report.xlsx')).toBeTruthy()
    expect(screen.queryByText(/is gone/i)).toBeNull()
  })

  it('says the file is gone, and offers nothing, when it is', async () => {
    vi.mocked(FileStillThere).mockResolvedValue('gone' as any)
    render(ExternalFilePane, props)

    await waitFor(() => expect(screen.getByText(/is gone/i)).toBeTruthy())
    // The claim that a program on this machine can open it is itself the bug.
    expect(screen.queryByText(/cannot preview/i)).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  // The gap between asking and clicking: it can vanish in between, and the
  // answer must still be a sentence rather than an OS error string.
  it('turns a gone-file failure on click into the same plain answer', async () => {
    render(ExternalFilePane, props)
    await waitFor(() => expect(screen.getByRole('button')).toBeTruthy())

    vi.mocked(OpenFileExternally).mockRejectedValueOnce(new Error('file-gone'))
    await fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(screen.getByText(/is gone/i)).toBeTruthy())
    expect(screen.queryByText(/file-gone/)).toBeNull()
  })

  // Any other failure is a real one and keeps saying so in its own words.
  it('still reports a genuine failure verbatim', async () => {
    render(ExternalFilePane, props)
    await waitFor(() => expect(screen.getByRole('button')).toBeTruthy())

    vi.mocked(OpenFileExternally).mockRejectedValueOnce(new Error('permission denied'))
    await fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(screen.getByText(/permission denied/)).toBeTruthy())
    expect(screen.queryByText(/is gone/i)).toBeNull()
  })

  // The answer that used to be folded into "gone": the engine could not resolve
  // the path or was not allowed to look. Reported 2026-08-18 against a .docx the
  // agent had just written to D:\ from an unfocused session — the pane called it
  // deleted and took away the only control that could have shown otherwise.
  it('keeps the offer when the engine cannot say', async () => {
    vi.mocked(FileStillThere).mockResolvedValue('unknown' as any)
    render(ExternalFilePane, props)

    await waitFor(() => expect(screen.getByRole('button')).toBeTruthy())
    expect(screen.queryByText(/is gone/i)).toBeNull()
  })

  // A file that never made it in at all has no path — that case predates this
  // and must keep its own answer rather than being called "gone".
  it('keeps its own answer for a file that never arrived', async () => {
    render(ExternalFilePane, { path: '', reason: '', name: 'dropped.bin' })
    expect(screen.getByText('dropped.bin')).toBeTruthy()
    expect(screen.queryByText(/is gone/i)).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
