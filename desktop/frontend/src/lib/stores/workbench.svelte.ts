// Right-workbench tab state. One place owns which panes are open (review, terminals,
// browser tabs, files, file editors) so any part of the app — sidebar, chat,
// future agent surfaces — can open a workbench tab without prop drilling. Components
// under lib/workbench/ render from this; nothing else mutates it directly.

import {
  TerminalStart, TerminalClose, ReadFile, ReadWorkbook,
  RelativizePath, SaveChatFile, WorkbenchTabsChanged,
} from '../../../wailsjs/go/main/App'
import type { main, ooxml } from '../../../wailsjs/go/models'
import { t } from '../i18n.svelte'

export type WorkbenchTabKind = 'terminal' | 'browser' | 'files' | 'file' | 'tools'

export type WorkbenchTab = {
  id: string
  kind: WorkbenchTabKind
  name: string
  url?: string // browser tabs
  viewport?: { name: string; w: number; h: number } // browser tabs: device-size emulation; unset = fill the pane
  path?: string // file tabs
  content?: string // file tabs (initial content; editor keeps its own draft)
  // File tabs the editor cannot render. A spreadsheet the agent just wrote is
  // a ZIP, so ReadFile refuses it — the tab shows why and offers to hand the
  // file to the program the OS opens it with, rather than putting the words
  // "binary file cannot be previewed" in an editor and calling that a result.
  unreadable?: string
  // A .xlsx rendered as rows of display text. Read-only and deliberately so:
  // it answers "what did I just get?" without the user leaving the window, and
  // the open-in-Excel button is still there for anything past a glance.
  sheet?: ooxml.WorkbookPreview
  // Which pane draws this file — set by fileView() from the name alone, for
  // everything the webview can show off a URL. No bytes cross the binding for
  // these: the pane points at the file host and the webview streams it, which
  // is why a video is here at all and why an image no longer has a size limit.
  view?: FileView
  // Bumped on every re-read (loadFileTab). The pane is keyed on it so a file
  // the agent rewrote actually reaches the screen — see loadFileTab.
  rev?: number
  // The agent opened this tab, rather than the user. Only `desk_list` reads it,
  // and only to decide what it is allowed to say: a page the agent opened it
  // may describe, a page the user opened it may not (§81's rule about the
  // user's browsing never becoming agent-readable).
  mine?: boolean
}

/** Panes that draw a file straight from its URL, without reading it first. */
export type FileView = 'image' | 'video' | 'audio' | 'pdf'

// One table, one answer. Every one of these is a type the webview renders
// natively, so the routing is the whole implementation — the pane is a tag with
// a src. SVG is here rather than with the text files on purpose: it is a
// picture that happens to be written down, and showing its source when someone
// opens it was the old behaviour by accident, not by choice.
const viewByExt: Record<string, FileView> = {
  png: 'image', jpg: 'image', jpeg: 'image', gif: 'image', webp: 'image',
  bmp: 'image', avif: 'image', ico: 'image', svg: 'image',
  mp4: 'video', webm: 'video', mov: 'video', mkv: 'video', avi: 'video',
  mp3: 'audio', wav: 'audio', m4a: 'audio', flac: 'audio', ogg: 'audio',
  pdf: 'pdf',
}

/** The pane a file belongs in, or undefined for "read it and decide". */
export function fileView(path: string): FileView | undefined {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return viewByExt[ext]
}

export const workbench = $state<{ tabs: WorkbenchTab[]; activeId: string }>({
  tabs: [],
  activeId: '',
})

let browserSeq = 0

export function activateTab(id: string): void {
  workbench.activeId = id
}

/** Remove a tab from the strip (does not stop terminal sessions — use closeTab). */
export function removeTab(id: string): void {
  const idx = workbench.tabs.findIndex((t) => t.id === id)
  if (idx === -1) return
  workbench.tabs.splice(idx, 1)
  if (workbench.activeId === id) workbench.activeId = workbench.tabs.at(-1)?.id ?? ''
}

/** Close a tab, stopping its backing terminal session if it has one. */
export async function closeTab(tab: WorkbenchTab): Promise<void> {
  if (tab.kind === 'terminal') await TerminalClose(tab.id)
  removeTab(tab.id)
}

/** Singleton tab: project file tree. */
export function openFilesTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'files')) {
    workbench.tabs.push({ id: 'files', kind: 'files', name: t('workbench.filesTab') })
  }
  workbench.activeId = 'files'
}

/** Singleton tab: skills + MCP tools the AI can currently use. */
export function openToolsTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'tools')) {
    workbench.tabs.push({ id: 'tools', kind: 'tools', name: t('workbench.toolsTab') })
  }
  workbench.activeId = 'tools'
}

export function openBrowserTab(): string {
  const id = `web-${++browserSeq}`
  workbench.tabs.push({ id, kind: 'browser', name: t('workbench.newTab'), url: '' })
  workbench.activeId = id
  return id
}

// "google.com" -> https://, "E:\site\index.html" -> file:///, and anything
// that already carries a scheme (file:, http:, about:) passes through —
// blindly prepending https:// turns file:/// URLs into a dead https://file/.
export function normalizeUrl(u: string): string {
  if (/^[a-z]:[\\/]/i.test(u)) return 'file:///' + u.replace(/\\/g, '/') // E:\site\index.html (before scheme check: "E:" looks like one)
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(u) || /^(about|data|mailto|javascript):/i.test(u)) return u
  return 'https://' + u // bare domain or host:port
}

/** Tab-strip label for a URL: the host, or the last path segment for a file. */
export function labelForUrl(url: string): string {
  try {
    const p = new URL(url)
    return p.hostname || decodeURIComponent(p.pathname.split('/').pop() || url)
  } catch {
    return url
  }
}

/** The MIME type everything draggable inside Aetox travels as. */
export const TAB_DRAG_MIME = 'application/x-aetox-tab'

/** Mark a drag as carrying one of our files or pages.
 *
 * One definition for all four sources — a workbench tab, a row in the file
 * tree, a produced-file card in the reply, and anything the composer or the
 * desk accepts. The shape was being spelled out at each drag source, which is
 * how a fourth one ends up subtly different from the other three. */
export function setTabDragPayload(e: DragEvent, kind: 'file' | 'browser', ref: string, label: string): void {
  if (!e.dataTransfer) return
  e.dataTransfer.setData(TAB_DRAG_MIME, JSON.stringify({ kind, ref, label }))
  e.dataTransfer.effectAllowed = 'copy'
}

/** Open a URL from outside the workbench (a link clicked in chat, a page
 * dragged in from a real browser) in a new browser tab. */
export function openUrlInWorkbench(url: string): void {
  const id = openBrowserTab()
  const tab = workbench.tabs.find((t) => t.id === id)
  if (!tab) return
  tab.url = url
  tab.name = labelForUrl(url)
}

export async function openTerminalTab(shell: { name: string; path: string }): Promise<void> {
  const id = await TerminalStart(shell.path, 80, 24)
  workbench.tabs.push({ id, kind: 'terminal', name: shell.name })
  workbench.activeId = id
}

/** Open (or re-focus) a file editor tab for a project-relative path.
 *
 * `displayName` overrides the tab label for files whose stored name is not the
 * name the user knows them by — a file dragged in from outside the project is
 * copied in under a generated name (see openPathsInWorkbench), and the tab
 * still has to read as the file they dropped. */
export async function openFileTab(path: string, displayName?: string): Promise<void> {
  const id = `file-${path}`
  let tab = workbench.tabs.find((t) => t.id === id)
  if (!tab) {
    // Pushed before any await, so the id is taken the moment it is claimed.
    // Reading first and pushing after left a window — hundreds of ms for a
    // workbook or a 20MB image — in which a second call saw no tab and pushed
    // a duplicate. The tab strip is `{#each ... (tab.id)}`, and Svelte throws
    // each_key_duplicate on a repeated key, taking the panel down.
    workbench.tabs.push({ id, kind: 'file', name: displayName || path.split('/').pop() || path, path, rev: 0 })
    // Read it back rather than keeping the literal: `workbench` is $state, so
    // what lives in the array is a proxy and the object passed to push is not.
    // Writing to the literal updates the data and tells Svelte nothing — the
    // pane renders once, empty, and never hears that the file arrived. Every
    // other tab opener in this file reads it back for the same reason.
    tab = workbench.tabs.find((t) => t.id === id)!
  }
  workbench.activeId = id
  await loadFileTab(tab, path)
}

/** (Re)read a file tab's contents off disk.
 *
 * Every open re-reads, including a re-open of a tab that is already there. The
 * agent rewrites the same path constantly — regenerate and undo both do it by
 * construction — and the tab id is the path, so a cached tab meant clicking the
 * file the agent had just rewritten showed the previous turn's bytes under the
 * right filename, with nothing on screen saying so. On the panel whose whole
 * job is answering "what did I just get?", that is the worst failure available.
 *
 * All four fields are assigned every time, undefined included: a file that used
 * to fail and now reads must lose its `unreadable`, or the pane keeps showing
 * the old excuse. */
async function loadFileTab(tab: WorkbenchTab, path: string): Promise<void> {
  const next: Pick<WorkbenchTab, 'content' | 'view' | 'sheet' | 'unreadable'> = {}
  const view = fileView(path)
  if (view) {
    // Nothing to load and nothing that can fail here: the pane addresses the
    // file host directly. A file that has gone missing surfaces as the element's
    // own error rather than as an exception this function could catch, which is
    // also why opening a 4GB video is instant.
    next.view = view
  } else if (path.toLowerCase().endsWith('.xlsx')) {
    // A workbook is tried as a grid first. It is the one produced format worth
    // previewing — a spreadsheet is a table, and a table is exactly what a pane
    // can draw. A deck would need a rendering engine and a document a layout
    // engine, so both keep going straight to the open-externally card.
    try {
      next.sheet = await ReadWorkbook(path)
    } catch (err) {
      // A workbook this reader cannot make sense of is still a workbook Excel
      // can open, so the failure falls through to the card rather than
      // becoming a dead end.
      next.unreadable = String(err)
    }
  } else if (/\.(pptx|docx)$/i.test(path)) {
    // Straight to the card, without asking ReadFile first. Routing a deck
    // through the text path made the card report whichever gate fired first —
    // "file too large to preview" for a 1.5MB pptx — for a file that was never
    // previewable at any size. The reason shown must be the real one.
    next.unreadable = t('workbench.officeNoPreview')
  } else {
    try {
      next.content = await ReadFile(path)
    } catch (err) {
      // Not an editor full of an error message. The file is fine — this app
      // just is not the thing that opens it.
      next.unreadable = String(err)
    }
  }
  tab.view = next.view
  tab.sheet = next.sheet
  tab.content = next.content
  tab.unreadable = next.unreadable
  // FileEditor snapshots `content` into local state once and the pane never
  // unmounts, so fresh bytes alone would not reach the screen. Workbench.svelte
  // keys the pane on this.
  tab.rev = (tab.rev ?? 0) + 1
}

/** Absolute OS paths dropped onto the desk — from Explorer, the desktop, another
 * app's save dialog — opened as tabs.
 *
 * A file already inside the project opens where it lies. One from outside is
 * copied in first (the same copy the paperclip button makes), because every
 * pane below this reads through the sandbox: without the copy the desk could
 * only answer a dropped file with "outside project root", which is a rule about
 * this program's internals, not an answer about the file. The copy also means
 * the agent can read what was dropped — the point of putting it on its desk. */
export async function openPathsInWorkbench(paths: string[]): Promise<void> {
  for (const abs of paths) {
    const label = abs.split(/[\\/]/).pop() || abs
    let rel = ''
    try {
      rel = await RelativizePath(abs)
    } catch {
      try {
        rel = await SaveChatFile(abs) // outside the project: bring a copy in
      } catch (err) {
        openDropError(abs, label, String(err))
        continue
      }
    }
    await openFileTab(rel, label)
  }
}

/** A dropped file that could not be brought in at all (too large, unreadable,
 * no project open) still gets a tab. Silence would read as the desk ignoring
 * the drop, which is the thing this whole surface is meant to stop. */
function openDropError(abs: string, label: string, reason: string): void {
  // Keyed on the full path, not the basename. Two files named the same from
  // different folders failing for different reasons collapsed onto one tab that
  // kept the FIRST reason — a pane whose only content is a reason, showing the
  // wrong one.
  const id = `drop-error-${abs}`
  const existing = workbench.tabs.find((t) => t.id === id)
  if (existing) existing.unreadable = reason // dropped again, failing differently
  else workbench.tabs.push({ id, kind: 'file', name: label, unreadable: reason })
  workbench.activeId = id
}

// ---------- pages this browser has shown ----------
//
// The other half of the browser tab's start page. RecentAgentPages covers what
// the agent opened with browser_open; this covers every other navigation the
// workbench browser completed — an address typed in the bar, a link clicked
// inside a page — so the list answers "what has been open here", not only
// "what did the agent open".
//
// Kept in localStorage rather than the tool_runs table the agent's own opens
// live in, deliberately: tool_runs feeds tool_runs_fts, which the agent
// searches as its own memory (session_search). Putting the user's personal
// browsing there would make it agent-readable, which is a far bigger decision
// than a start-page list gets to make. This stays in the UI layer.

export type VisitedPage = { url: string; title: string; time: string }

const visitsKey = 'aetox-browser-history'
const maxVisits = 200

/** Record a completed navigation. Called from BrowserPane's browser:meta
 *  handler, which fires for the agent's opens and the user's alike. */
export function recordVisit(url: string, title: string): void {
  if (!url || url === 'about:blank') return
  // Local files are left to RecentAgentPages, which checks on the way out that
  // the file still exists. A file:// row remembered here could not be checked
  // from the frontend, and a row that opens the engine's "not found" page is
  // the dead end the start page exists to prevent. A web page has no such
  // problem: a 404 still has back, reload and the address bar.
  if (url.startsWith('file:')) return
  const next = [{ url, title, time: new Date().toISOString() }, ...recentVisits().filter((v) => v.url !== url)]
  try {
    localStorage.setItem(visitsKey, JSON.stringify(next.slice(0, maxVisits)))
  } catch {
    // Quota, or a browser refusing storage — history is not worth an error.
  }
}

export function recentVisits(): VisitedPage[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(visitsKey) ?? '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.filter((v): v is VisitedPage => !!v && typeof v.url === 'string' && typeof v.time === 'string')
  } catch {
    return []
  }
}

// ---------- per-session persistence ----------
// Each chat session remembers its workbench layout (browser URLs, file paths,
// singleton panes) so switching back restores what was open. Terminals are
// live processes and can't be restored — they're closed on switch and skipped
// in snapshots. Stored in localStorage keyed by session id; the Go session
// store never learns about UI layout.

type SavedTab = { kind: WorkbenchTabKind; name: string; url?: string; path?: string }

let boundSessionId: string | null = null

const wbKey = (sessionId: string) => `aetox-workbench:${sessionId}`

/** Persist the current layout under the bound session. Reads workbench.tabs /
 * activeId reactively — run it from a component $effect to autosave. */
/** Tell the Go side what is open, so `desk_list` can answer.
 *
 * Pushed on every change rather than tracked on the Go side by watching the
 * events it sends: the agent is not the only one opening and closing tabs, and
 * a list rebuilt from its own actions would be wrong the first time the user
 * closed something. The frontend is where the truth is, so the frontend says. */
export function reportDeskTabs(): void {
  // Plain objects, cast rather than built through main.DeskTab.createFrom: the
  // binding serializes them to the same JSON, and a value-import of the
  // generated models module would make this file need Wails at runtime — which
  // it does not, and which breaks every test that renders the workbench.
  void WorkbenchTabsChanged(
    workbench.tabs.map((t) => ({
      kind: t.kind,
      name: t.name,
      path: t.path ?? '',
      url: t.url ?? '',
      mine: t.mine ?? false,
    })) as main.DeskTab[],
  )
}

export function saveWorkbenchSnapshot(): void {
  // "Restorable" has to mean what restoreWorkbench can actually rebuild, not
  // just "not a terminal": a file tab with no path (a failed drop) is saved and
  // then skipped on the way back, which shortens the list the saved index was
  // counted against and restores focus to whatever slid into that slot.
  const restorable = workbench.tabs.filter((t) => t.kind !== 'terminal' && (t.kind !== 'file' || !!t.path))
  const activeIdx = restorable.findIndex((t) => t.id === workbench.activeId)
  if (!boundSessionId) return
  const tabs: SavedTab[] = restorable.map(({ kind, name, url, path }) => ({ kind, name, url, path }))
  localStorage.setItem(wbKey(boundSessionId), JSON.stringify({ tabs, activeIdx }))
}

async function restoreWorkbench(sessionId: string): Promise<void> {
  for (const tab of workbench.tabs) {
    if (tab.kind === 'terminal') TerminalClose(tab.id)
  }
  workbench.tabs = [] // unmounts panes; BrowserPane's onDestroy closes its native window
  workbench.activeId = ''
  let saved: { tabs: SavedTab[]; activeIdx: number }
  try {
    saved = JSON.parse(localStorage.getItem(wbKey(sessionId)) ?? '') as typeof saved
  } catch {
    return
  }
  for (const s of saved.tabs ?? []) {
    // A layout saved before the Review panel was removed still names it. It is
    // simply skipped rather than special-cased into an error: the rest of that
    // session's tabs must still come back.
    if (s.kind === 'files') openFilesTab()
    else if (s.kind === 'tools') openToolsTab()
    else if (s.kind === 'file' && s.path) await openFileTab(s.path, s.name)
    else if (s.kind === 'browser') {
      const id = openBrowserTab()
      const tab = workbench.tabs.find((t) => t.id === id)
      if (tab) { tab.url = s.url ?? ''; tab.name = s.name }
    }
  }
  workbench.activeId = workbench.tabs[saved.activeIdx]?.id ?? workbench.tabs.at(-1)?.id ?? ''
}

/** Explicit session switch (sidebar click, new session): save the old
 * session's layout, then replace the workbench with the new one's. */
export async function switchWorkbenchSession(sessionId: string): Promise<void> {
  if (!sessionId || sessionId === boundSessionId) return
  saveWorkbenchSnapshot()
  boundSessionId = sessionId
  await restoreWorkbench(sessionId)
}

/** Passive id observation (app start, or the engine minting a real id for the
 * chat in progress): first sighting restores; a later id change means the
 * current conversation was re-keyed, so the open tabs migrate to the new id. */
export async function adoptWorkbenchSession(sessionId: string): Promise<void> {
  if (!sessionId || sessionId === boundSessionId) return
  const firstBind = boundSessionId === null
  boundSessionId = sessionId
  if (firstBind) await restoreWorkbench(sessionId)
  else saveWorkbenchSnapshot()
}

/** Drop a deleted session's stored layout. */
export function removeWorkbenchState(sessionId: string): void {
  localStorage.removeItem(wbKey(sessionId))
}
