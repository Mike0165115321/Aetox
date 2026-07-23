// Right-workbench tab state. One place owns which panes are open (review, terminals,
// browser tabs, files, file editors) so any part of the app — sidebar, chat,
// future agent surfaces — can open a workbench tab without prop drilling. Components
// under lib/workbench/ render from this; nothing else mutates it directly.

import { TerminalStart, TerminalClose, ReadFile } from '../../../wailsjs/go/main/App'
import { t } from '../i18n.svelte'

export type WorkbenchTabKind = 'review' | 'terminal' | 'browser' | 'files' | 'file' | 'tools'

export type WorkbenchTab = {
  id: string
  kind: WorkbenchTabKind
  name: string
  url?: string // browser tabs
  path?: string // file tabs
  content?: string // file tabs (initial content; editor keeps its own draft)
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

/** Singleton tab: Review panels (files changed / diff / tests / history). */
export function openReview(): void {
  if (!workbench.tabs.some((t) => t.kind === 'review')) {
    workbench.tabs.unshift({ id: 'review', kind: 'review', name: t('workbench.reviewTab') })
  }
  workbench.activeId = 'review'
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

/** Open a URL from outside the workbench (e.g. a link clicked in chat) in a new browser tab. */
export function openUrlInWorkbench(url: string): void {
  const id = openBrowserTab()
  const tab = workbench.tabs.find((t) => t.id === id)
  if (tab) tab.url = url
}

export async function openTerminalTab(shell: { name: string; path: string }): Promise<void> {
  const id = await TerminalStart(shell.path, 80, 24)
  workbench.tabs.push({ id, kind: 'terminal', name: shell.name })
  workbench.activeId = id
}

/** Open (or re-focus) a file editor tab for a project-relative path. */
export async function openFileTab(path: string): Promise<void> {
  const id = `file-${path}`
  if (!workbench.tabs.some((t) => t.id === id)) {
    let content: string
    try {
      content = await ReadFile(path)
    } catch (err) {
      content = t('workbench.openFileError', { err: String(err) })
    }
    workbench.tabs.push({ id, kind: 'file', name: path.split('/').pop() ?? path, path, content })
  }
  workbench.activeId = id
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
export function saveWorkbenchSnapshot(): void {
  const restorable = workbench.tabs.filter((t) => t.kind !== 'terminal')
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
    if (s.kind === 'review') openReview()
    else if (s.kind === 'files') openFilesTab()
    else if (s.kind === 'tools') openToolsTab()
    else if (s.kind === 'file' && s.path) await openFileTab(s.path)
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
