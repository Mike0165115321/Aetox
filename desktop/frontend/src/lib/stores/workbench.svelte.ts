// Right-workbench tab state. One place owns which panes are open (review, terminals,
// browser tabs, files, file editors) so any part of the app — sidebar, chat,
// future agent surfaces — can open a workbench tab without prop drilling. Components
// under lib/workbench/ render from this; nothing else mutates it directly.

import {
  TerminalStart, TerminalShells, TerminalClose, BrowserClose, BrowserCloseForTeardown, ReadFile, ReadWorkbook,
  RelativizePath, SaveChatFile, WorkbenchTabsChanged, ResolveAddress, BrowserDevices,
} from '../../../wailsjs/go/main/App'
import type { main, ooxml } from '../../../wailsjs/go/models'
import { t } from '../i18n.svelte'

export type WorkbenchTabKind = 'terminal' | 'browser' | 'files' | 'file' | 'decks' | 'git' | 'repomap' | 'pr'

export type WorkbenchTab = {
  id: string
  kind: WorkbenchTabKind
  name: string
  url?: string // browser tabs
  // Browser tabs: the other scheme to try if `url` fails, for an address the
  // user typed without one. Spent by BrowserPane on the navigation it arrived
  // with — see address.go's `guess` for why a guess without a second try is
  // the bug this closes.
  fallback?: string
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
  // This .html is a slide deck rather than a page, so it opens in SlidesPane.
  // Decided from the bytes rather than from the name (isDeck) — see there for
  // why the marker is `section.slide` and not a second declaration beside it.
  deck?: boolean
  // Which pane draws this file — set by fileView() from the name alone, for
  // everything the webview can show off a URL. No bytes cross the binding for
  // these: the pane points at the file host and the webview streams it, which
  // is why a video is here at all and why an image no longer has a size limit.
  view?: FileView
  // Bumped on every re-read (loadFileTab). The pane is keyed on it so a file
  // the agent rewrote actually reaches the screen — see loadFileTab.
  rev?: number
  // Which shell this terminal was started from, as the path Go spawned. Kept
  // so the tab can be started again: a PTY dies with the app, but "a PowerShell
  // on this desk" is a fact about the desk and survives with it.
  shell?: string
  // The agent opened this tab, rather than the user. Only `desk_list` reads it,
  // and only to decide what it is allowed to say: a page the agent opened it
  // may describe, a page the user opened it may not (§81's rule about the
  // user's browsing never becoming agent-readable).
  mine?: boolean
  // Browser tabs the agent opened: whose conversation this page belongs to.
  // It is what lets the strip refuse a background chat's page (§187 closed for
  // files; this closes it for the browser) and what lets that page survive a
  // switch away — its native window parks in `workbench.foreign` instead of
  // being torn down under a working agent. Unset = the user's own tab, which
  // lives and dies with the desk it was opened on, as it always has.
  sessionId?: string
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

/** Whether an .html file is a slide deck.
 *
 * One marker, doing two jobs: `.slide` is what the pane pages through and what
 * the exporters cut on, so it is also what identifies the file. A
 * `<meta name="aetox-deck">` beside it was drafted and dropped — it would be a
 * second place answering one question, and the day the two disagree nobody can
 * say which is right (docs/architecture/html-deck-2026-08-19.md).
 *
 * `<section>` is the contract; `<div>` is read too, because that is what the
 * presentation templates people install are written with, and a file that is a
 * deck in every browser should not open here as source code. Which of the two a
 * given document is actually cut on is `internal/deck`'s answer, not this one —
 * this is a routing hint, and it only has to know that the file is a deck at
 * all.
 *
 * An .html without the marker is a web page and still opens as source, which is
 * the behaviour every existing page keeps. */
export function isDeck(path: string, content: string): boolean {
  if (!/\.html?$/i.test(path)) return false
  return /<(?:section|div)[^>]*\bclass\s*=\s*("[^"]*\bslide\b|'[^']*\bslide\b)/i.test(content)
}

// `foreign` is the shadow rack: live browser tabs whose conversation is NOT on
// screen. Each still gets a (hidden) BrowserPane — the native window has to
// exist for the background agent to keep browsing it — but none of them is in
// the strip, so another chat's page can neither appear here, steal focus, nor
// be snapshotted into this session's layout (the leak of 1 ก.ย.). A tab moves
// strip↔foreign as its session leaves and returns to the screen.
export const workbench = $state<{ tabs: WorkbenchTab[]; activeId: string; foreign: WorkbenchTab[] }>({
  tabs: [],
  activeId: '',
  foreign: [],
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

/** The engine closed a browser tab: take its chip off the strip.
 *
 * `workbench:open-browser` had no partner, so a tab closed from the Go side —
 * the agent's own `browser tabs close`, or the orphan sweep after a reload —
 * stayed on the strip forever, pointing at a native view that no longer existed.
 * Worse than a dead chip: BrowserPane latches `opened` once it has called
 * BrowserOpen, so the pane behind it would never try again either. A black
 * rectangle with a URL in the address bar and nothing that could fix it (owner,
 * 24 ส.ค.).
 *
 * Only browser tabs, and only by id: this is the mirror of one act on one tab,
 * not a resync of the strip. `removeTab` rather than `closeTab` because the
 * close already happened — calling back into Go would be this window asking the
 * engine to do again what the engine just told it about. */
export function browserTabClosedByEngine(id: string): void {
  const tab = workbench.tabs.find((t) => t.id === id)
  if (tab?.kind !== 'browser') return
  removeTab(id)
}

/** Close a tab, stopping whatever is running behind it.
 *
 * This is the × on the strip and the only thing in the window that means a
 * PERSON closed a tab. It has to say so here, where the click is, rather than
 * leaving it to the pane's teardown: an unmount happens for several reasons and
 * only one of them is this one, so a close inferred from a lifecycle hook
 * reports every reason as the user (browser-tab-lifetime-2026-08-25.md). */
export async function closeTab(tab: WorkbenchTab): Promise<void> {
  if (tab.kind === 'terminal') await TerminalClose(tab.id)
  if (tab.kind === 'browser') BrowserClose(tab.id)
  removeTab(tab.id)
}

/** Singleton tab: project file tree. */
export function openFilesTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'files')) {
    workbench.tabs.push({ id: 'files', kind: 'files', name: t('workbench.filesTab') })
  }
  workbench.activeId = 'files'
}

/** Singleton tab: the slide decks this workspace has produced.
 *
 * A room rather than a tab per deck, because the question it answers is "what
 * presentations are there" — asked before anyone knows which file to open, so a
 * file tab cannot be the answer. Opening a deck from the file tree still lands
 * in the same viewer; the room only adds the list in front of it. */
export function openDecksTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'decks')) {
    workbench.tabs.push({ id: 'decks', kind: 'decks', name: t('workbench.decksTab') })
  }
  workbench.activeId = 'decks'
}

/** Singleton tab: the working tree (DECISIONS §161.4).
 *
 * โค้ด desk only, and the menu entry is drawn only there — a repository is what
 * that desk is held inside, and the storefront deliberately has no project to
 * report on. The pane says so itself rather than leaving the absence to be
 * discovered. */
/** Singleton tab: the project's pull requests (desktop/pr_room.go).
 *
 * โค้ด desk only, same gate and same reason as Git below: it is the focused
 * repository's own pull requests, and the storefront has no repository. The
 * pane says why it is empty rather than leaving the absence to be discovered. */
export function openPRTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'pr')) {
    workbench.tabs.push({ id: 'pr', kind: 'pr', name: t('workbench.prTab') })
  }
  workbench.activeId = 'pr'
}

export function openGitTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'git')) {
    workbench.tabs.push({ id: 'git', kind: 'git', name: t('workbench.gitTab') })
  }
  workbench.activeId = 'git'
}

/** Singleton tab: แผนที่โค้ด — the project drawn as dots and lines (owner, 29 ส.ค.).
 *
 * โค้ด desk only, same gate and same reason as Git one function up: the map is
 * of the focused project, and the storefront has none. The pane calls the same
 * analysis the model's repo_map tool runs, so what the user sees and what the
 * model saw cannot drift. */
export function openRepoMapTab(): void {
  if (!workbench.tabs.some((t) => t.kind === 'repomap')) {
    workbench.tabs.push({ id: 'repomap', kind: 'repomap', name: t('workbench.repoMapTab') })
  }
  workbench.activeId = 'repomap'
}

// The device rows, loaded once from Go for the picker to draw. The list itself
// lives in browser_device.go and this is the only copy of it on this side.
export const deviceList = $state<{ rows: { name: string; w: number; h: number }[] }>({ rows: [] })

export async function loadDevices(): Promise<void> {
  if (deviceList.rows.length) return
  deviceList.rows = (await BrowserDevices()) ?? []
}

export function openBrowserTab(): string {
  const id = `web-${++browserSeq}`
  workbench.tabs.push({ id, kind: 'browser', name: t('workbench.newTab'), url: '' })
  workbench.activeId = id
  return id
}

// Where a line typed into the address bar goes.
//
// This used to classify the text itself, in a copy of the rules Go already had
// in normalizeWorkbenchURL — and both copies ended by stamping https:// onto
// whatever was left, so typing ยูทูป produced https://ยูทูป, which the engine
// punycoded to xn--o3cit6gb and DNS refused. The address bar had one job where
// every browser's has two, and neither copy had ever been asked to tell an
// address from a search.
//
// Go answers that now, once, in address.go. What stays here is the policy, and
// the policy is the half that is genuinely ours: an address bar SEARCHES. The
// agent's `open` refuses the same input and names web_search instead, because
// it already has one. Same question, two callers, two right answers.
export async function resolveAddressBarInput(u: string): Promise<{ url: string; fallback: string }> {
  const addr = await ResolveAddress(u)
  // A search is a place once it has been turned into one, and nobody falls back
  // from Google to Google over plain http.
  return addr.url ? { url: addr.url, fallback: addr.fallback } : { url: addr.searchUrl, fallback: '' }
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
export function openUrlInWorkbench(url: string, fallback = ''): void {
  const id = openBrowserTab()
  const tab = workbench.tabs.find((t) => t.id === id)
  if (!tab) return
  // Armed before the URL, because the URL is what the pane's effect watches:
  // set the other way round, the first navigation goes out unarmed.
  tab.fallback = fallback
  tab.url = url
  tab.name = labelForUrl(url)
}

export async function openTerminalTab(shell: { name: string; path: string }): Promise<void> {
  const id = await TerminalStart(shell.path, 80, 24)
  workbench.tabs.push({ id, kind: 'terminal', name: shell.name, shell: shell.path })
  workbench.activeId = id
}

/** Open (or re-focus) a file editor tab for a project-relative path.
 *
 * `displayName` overrides the tab label for files whose stored name is not the
 * name the user knows them by — a file dragged in from outside the project is
 * copied in under a generated name (see openPathsInWorkbench), and the tab
 * still has to read as the file they dropped. */
export async function openFileTab(path: string, displayName?: string, mine = false): Promise<void> {
  const id = `file-${path}`
  let tab = workbench.tabs.find((t) => t.id === id)
  // `mine` is set once, when the tab is created, and never changed after: it
  // answers "who put this here", which is a fact about the first time. A file
  // the user opened does not become the agent's because the agent re-opened it
  // — and that matters, because `desk close` refuses anything that is not the
  // agent's own (§81's rule, one hand heavier).
  if (!tab) {
    // Pushed before any await, so the id is taken the moment it is claimed.
    // Reading first and pushing after left a window — hundreds of ms for a
    // workbook or a 20MB image — in which a second call saw no tab and pushed
    // a duplicate. The tab strip is `{#each ... (tab.id)}`, and Svelte throws
    // each_key_duplicate on a repeated key, taking the panel down.
    workbench.tabs.push({ id, kind: 'file', name: displayName || path.split('/').pop() || path, path, rev: 0, mine })
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

/** `desk close` — take a file tab the AGENT opened back off the desk.
 *
 * The Go side has already refused a path it cannot see and a tab that is not
 * the agent's, off the mirror the frontend pushes it (WorkbenchTabsChanged).
 * This checks the same two things again rather than trusting that, because the
 * mirror is one report behind by construction: a tab the user closed in the
 * moment between the report and the event is a tab this must not act on, and
 * the array here is the only copy that is never stale. */
export function closeAgentFileTab(path: string): void {
  const tab = workbench.tabs.find((t) => t.kind === 'file' && t.path === path && t.mine)
  if (tab) void closeTab(tab)
}

/** `desk_open` routed by whose desk it is (§187).
 *
 * The event names its session now, because a chat working in the background
 * kept putting files on whichever desk was on screen — and the on-screen
 * session's next snapshot then persisted the stray as its own, so the leak
 * survived restarts. A background session's file goes into that session's
 * SAVED desk instead: the user finds it there when they open the chat, which
 * is what "วางไฟล์บนโต๊ะแล้ว" honestly means for a desk nobody is looking at.
 * An event with no session (an older engine mid-upgrade) keeps today's
 * behaviour rather than dropping the file. */
export async function openAgentFileTabFor(sessionId: string, path: string, name: string): Promise<void> {
  if (!sessionId || sessionId === boundSessionId) {
    await openFileTab(path, name, true)
    return
  }
  patchSavedTabs(sessionId, (tabs) => {
    if (tabs.some((t) => t.kind === 'file' && t.path === path)) return tabs
    return [...tabs, { kind: 'file', name, path, mine: true }]
  })
}

/** `desk close`, routed the same way — the agent may only take back its own
 * tab, and only from its own session's desk, live or saved. */
export function closeAgentFileTabFor(sessionId: string, path: string): void {
  if (!sessionId || sessionId === boundSessionId) {
    closeAgentFileTab(path)
    return
  }
  patchSavedTabs(sessionId, (tabs) => tabs.filter((t) => !(t.kind === 'file' && t.path === path && t.mine)))
}

/** A page the agent opened (or moved to), routed by whose page it is.
 *
 * On screen: the tab draws (or fronts) in the strip, as it always did. A
 * background chat's page parks instead — a live entry on the shadow rack so
 * its native window exists for the agent still browsing it, plus a chip on
 * that session's saved desk, found there when its chat opens. It never touches
 * the strip on screen (the leak of 1 ก.ย.: another chat's page appearing,
 * fronting itself, and being snapshotted as this session's own).
 *
 * The agent tab pool is shared across conversations (browser_tabs.go), so an
 * open can CLAIM a tab another chat was holding: the tab follows its newest
 * owner, moving between strip and rack accordingly. */
export function openAgentBrowserTabFor(sessionId: string, id: string, url: string): void {
  if (!id) return
  const live = !sessionId || sessionId === boundSessionId
  const stripAt = workbench.tabs.findIndex((tab) => tab.id === id)
  const rackAt = workbench.foreign.findIndex((tab) => tab.id === id)
  if (live) {
    if (rackAt >= 0) {
      // Coming home: the shown chat claimed a parked window. Same object, same
      // id — the pane remounts onto the existing native window (host.open is
      // idempotent on a live id), so the page arrives without a reload.
      const [tab] = workbench.foreign.splice(rackAt, 1)
      if (sessionId) tab.sessionId = sessionId
      if (url) tab.url = url
      workbench.tabs.push(tab)
    } else if (stripAt < 0) {
      workbench.tabs.push({
        id, kind: 'browser', name: t('workbench.newTab'), url, mine: true,
        sessionId: sessionId || boundSessionId || undefined,
      })
    }
    workbench.activeId = id
    return
  }
  let tab: WorkbenchTab
  if (stripAt >= 0) {
    // A background chat claimed a tab sitting on the shown desk: the page is
    // that chat's now, so it leaves this strip — left in place, its
    // navigations would keep drawing here, the exact leak this routing closes.
    ;[tab] = workbench.tabs.splice(stripAt, 1)
    if (workbench.activeId === id) workbench.activeId = workbench.tabs.at(-1)?.id ?? ''
    workbench.foreign.push(tab)
  } else if (rackAt >= 0) {
    tab = workbench.foreign[rackAt]
  } else {
    tab = { id, kind: 'browser', name: t('workbench.newTab'), url, mine: true }
    workbench.foreign.push(tab)
  }
  tab.sessionId = sessionId
  if (url) {
    tab.url = url
    tab.name = labelForUrl(url)
  }
  patchSavedTabs(sessionId, (tabs) => {
    const at = tabs.findIndex((s) => s.kind === 'browser' && s.id === id)
    const chip: SavedTab = {
      kind: 'browser', id, mine: true,
      url: url || tabs[at]?.url || '',
      name: url ? labelForUrl(url) : tabs[at]?.name ?? t('workbench.newTab'),
    }
    if (at >= 0) return tabs.map((s, i) => (i === at ? chip : s))
    return [...tabs, chip]
  })
}

/** The desk's one door on the window side (§187.3).
 *
 * Every agent-originated desk event arrives here, and every KIND declares in
 * this one switch what a background arrival means — park it on that session's
 * saved desk, or state why it draws live. §187's leak existed because that
 * question was asked nowhere; a kind with no answer here falls to the default,
 * which touches nothing and says so, instead of guessing at a desk.
 *
 * sessionId '' is the Go door's explicit "no per-session owner" (the
 * engine-log terminal — §187.2; the browser left that club on 1 ก.ย.) and
 * draws live, which is the pre-§187 behaviour made a stated policy instead
 * of an accident. */
export function routeDeskEvent(kind: string, payload: Record<string, unknown>): void {
  const sessionId = typeof payload.sessionId === 'string' ? payload.sessionId : ''
  const str = (k: string) => (typeof payload[k] === 'string' ? (payload[k] as string) : '')
  switch (kind) {
    // A file tab is pure UI, so both destinations exist: live for the chat on
    // screen, the saved desk for one that is not.
    case 'open-file':
      void openAgentFileTabFor(sessionId, str('path'), str('name'))
      return
    case 'close-file':
      closeAgentFileTabFor(sessionId, str('path'))
      return
      return
    // The browser gained its per-session owner (the change desk_events.go
    // promised): the event names whose page this is, and the two destinations
    // are the same as open-file's. On screen draws live; a background chat's
    // page goes to the shadow rack — its native window kept alive, hidden,
    // for the agent still working it — and onto that session's saved desk,
    // where its user will find it. It never touches the strip on screen, so
    // it can neither steal focus nor be snapshotted as somebody else's.
    //
    // sessionId '' is still honoured as "draw live" for the surfaces that
    // genuinely have no owner (§187.2).
    case 'open-browser': {
      const id = str('id')
      openAgentBrowserTabFor(sessionId, id, str('url'))
      return
    }
    case 'close-browser': {
      const id = str('id')
      const foreignAt = workbench.foreign.findIndex((tab) => tab.id === id)
      if (foreignAt >= 0) {
        // A parked page closing closes everywhere it is remembered: the shadow
        // rack, and the owner's saved desk — or its chat would reopen onto a
        // chip pointing at a window that no longer exists.
        const owner = workbench.foreign[foreignAt].sessionId ?? ''
        workbench.foreign.splice(foreignAt, 1)
        if (owner) patchSavedTabs(owner, (tabs) => tabs.filter((tab) => !(tab.kind === 'browser' && tab.id === id)))
        return
      }
      browserTabClosedByEngine(id)
      return
    }
    // The PTY already lives on the Go side; this only mounts a pane on it.
    // Live like the browser, because both are native resources that exist
    // before the tab does. The shell it was started from rides along so the
    // saved layout can start it again — the process cannot come back, and the
    // terminal's place on the desk can.
    case 'open-terminal': {
      const id = str('id')
      if (!workbench.tabs.some((tab) => tab.id === id)) {
        workbench.tabs.push({ id, kind: 'terminal', name: str('name'), shell: str('path'), mine: true })
      }
      workbench.activeId = id
      return
    }
    default:
      // A desk event nobody wrote a policy for must not touch a desk.
      console.warn(`desk event "${kind}" has no routing policy — nothing was drawn`)
  }
}

/** Rewrite one background session's saved desk, and keep the Go mirror true.
 *
 * The mirror push matters as much as the storage write: desk_list and desk
 * close judge against what WorkbenchTabsChanged last reported for that
 * conversation, and a file parked only in localStorage would be a tab the
 * agent was told it put down and is then told does not exist. */
function patchSavedTabs(sessionId: string, change: (tabs: SavedTab[]) => SavedTab[]): void {
  let saved: { tabs: SavedTab[]; activeIdx: number }
  try {
    saved = JSON.parse(localStorage.getItem(wbKey(sessionId)) ?? '') as typeof saved
  } catch {
    saved = { tabs: [], activeIdx: -1 }
  }
  const before = saved.tabs ?? []
  const tabs = change(before)
  // A newly arrived file is what that chat will want on top; a removal keeps
  // the focus clamped to a tab that still exists.
  const activeIdx = tabs.length > before.length ? tabs.length - 1 : Math.min(saved.activeIdx, tabs.length - 1)
  localStorage.setItem(wbKey(sessionId), JSON.stringify({ tabs, activeIdx }))
  void WorkbenchTabsChanged(
    sessionId,
    tabs.map((t) => ({
      kind: t.kind,
      name: t.name,
      path: t.path ?? '',
      url: t.url ?? '',
      mine: t.mine ?? false,
    })) as main.DeskTab[],
  )
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
 * Every field below is assigned every time, undefined included: a file that used
 * to fail and now reads must lose its `unreadable`, or the pane keeps showing
 * the old excuse. */
async function loadFileTab(tab: WorkbenchTab, path: string, keepPane = false): Promise<void> {
  const next: Pick<WorkbenchTab, 'content' | 'view' | 'sheet' | 'unreadable'> = {}
  const wasText = tab.content !== undefined
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
  // Assigned every time like the four above, false included: a deck the user
  // edited back into a plain page must lose the slide pane, or the pane keeps
  // paging through sections that are no longer there.
  tab.deck = next.content !== undefined && isDeck(path, next.content)
  // Workbench.svelte keys the pane on this, so bumping it rebuilds the pane.
  //
  // Which is what an open wants and the opposite of what a re-read behind the
  // user's back wants: a rebuilt editor loses the caret, the scroll position
  // and the undo stack, and this path exists precisely for the case where they
  // are still typing in it (`keepPane`). Text-to-text is therefore left to
  // FileEditor, which patches the change into the model in place. Anything that
  // changes WHICH pane draws the file — text that became unreadable, a workbook
  // this is not any more — still has to rebuild, or the pane on screen is one
  // for a file that no longer exists.
  const sameKindOfFile = wasText && next.content !== undefined
  if (!keepPane || !sameKindOfFile) tab.rev = (tab.rev ?? 0) + 1
}

/** The agent changed a file on disk. Put the new bytes in front of whoever is
 * looking at it.
 *
 * Owner, 24 ส.ค.: *"ผมทำงานอยู่ มันปรับเนื้อหาในเอกสารแล้วผมยังเห็นอันเก่าอยู่"*.
 * A file pane read the file once, when it was opened. `loadFileTab`'s own
 * comment already said re-reading matters and every re-open did it — but a tab
 * sitting open through a turn was never re-opened, so it kept the bytes it was
 * born with while the agent edited the same path underneath it.
 *
 * Driven from Go (`workbench:files-changed`, off the same parse ไฟล์ที่สร้าง
 * หรือแก้ reads) rather than from a timer or a watcher: the engine knows which
 * call touched which path, and polling the disk for files nobody changed is
 * work done on the chance it was wanted.
 *
 * Not scoped to the session that wrote it, deliberately. A pane shows a file on
 * disk; the file on disk changed. Which conversation changed it is not a reason
 * to keep showing the user something that is no longer true. */
export async function filesChangedOnDisk(paths: string[]): Promise<void> {
  const wanted = new Set((paths ?? []).map(samePathKey).filter(Boolean))
  if (!wanted.size) return
  for (const tab of workbench.tabs) {
    if (tab.kind !== 'file' || !tab.path || !wanted.has(samePathKey(tab.path))) continue
    await loadFileTab(tab, tab.path, true)
  }
}

/** One spelling of a path, for asking "is this the same file".
 *
 * Windows is the reference platform: `Desktop\A.md` and `desktop/a.md` are one
 * file there, and a comparison that says otherwise leaves the pane stale for
 * the reason the user can least guess. Lowercasing is wrong on a case-sensitive
 * filesystem in the one case where two files differ only by case — a trade
 * taken on purpose, because the cost is one needless re-read and the cost the
 * other way is the bug this function exists to fix. */
function samePathKey(path: string): string {
  return path.trim().replace(/\\/g, '/').replace(/^\.\//, '').toLowerCase()
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
// singleton panes, terminals) so switching back — or closing the app and
// opening it again — restores what was open. Stored in localStorage keyed by
// session id; the Go session store never learns about UI layout.
//
// Terminals were the exception until 30 ส.ค., on the reasoning that a PTY is a
// live process and cannot be restored. The process cannot; the terminal can.
// What the exception actually produced was a panel that came back in pieces,
// and the owner named that as the bug rather than as its own feature:
// *"จริงๆมันก็ทั้งหมดนั่นแหละครับ และจริงๆมันไม่ควรแตกแยกอะไรแบบนี้ด้วย ผมถึงพูดว่าข้างขวา"*.
// So a terminal comes back as a fresh shell of the same kind — the scrollback
// and whatever was running are gone, which is what closing an app means, and
// the desk is whole.

// `mine` survives the round trip on purpose: it is the fact "the agent put
// this here", and desk close's whole safety rule keys on it. Dropped in the
// save (as it was until §187), an agent-opened tab came back as the user's
// after one switch away, and the agent could no longer take back its own tab.
// `id` is saved for browser tabs only: it is the name of the live native
// window, and it is what lets a session switch re-adopt a page whose agent
// kept working in the background (the shadow rack) instead of reloading a
// copy beside it. A dead id — app restarted, window gone — falls back to the
// URL, which is what the id-less save always did.
type SavedTab = { kind: WorkbenchTabKind; name: string; url?: string; path?: string; shell?: string; mine?: boolean; id?: string }

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
  // The session is named: the workbench is kept per chat on this side already,
  // and the mirror the agent reads is per chat on the other (desk_list). An
  // unnamed report lands on whichever conversation Go thinks is current, which
  // is the one on screen — and a chat working in the background would then be
  // told about somebody else's desk as if it were its own.
  void WorkbenchTabsChanged(
    boundSessionId ?? '',
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
  // "Restorable" has to mean what restoreWorkbench can actually rebuild: a file
  // tab with no path (a failed drop) is saved and then skipped on the way back,
  // which shortens the list the saved index was counted against and restores
  // focus to whatever slid into that slot.
  const restorable = workbench.tabs.filter((t) => t.kind !== 'file' || !!t.path)
  const activeIdx = restorable.findIndex((t) => t.id === workbench.activeId)
  if (!boundSessionId) return
  const tabs: SavedTab[] = restorable.map(({ kind, name, url, path, shell, mine, id }) => ({
    kind, name, url, path, shell, mine,
    // The window's name, browser tabs only — see SavedTab on why it is saved.
    ...(kind === 'browser' ? { id } : {}),
  }))
  localStorage.setItem(wbKey(boundSessionId), JSON.stringify({ tabs, activeIdx }))
}

async function restoreWorkbench(sessionId: string): Promise<void> {
  // Everything holding something outside the DOM is closed HERE, by name.
  //
  // The browser half used to be left to the pane's teardown, and the line below
  // said so. It stopped being true on 2026-08-25, when BrowserPane's onDestroy
  // correctly gave up closing anything (an unmount happens for several reasons
  // and only one of them is a person). Nothing failed loudly: a native browser
  // window is a real OS window composited above the app, so switching sessions
  // with a page open left it on screen, at its last bounds, over the chat, with
  // no pane alive to hide or move it. Terminals were closed on this line all
  // along, which is the shape the browser should always have had.
  //
  // ForTeardown, not BrowserClose: this is the window discarding its own strip,
  // not a person clicking ×, and the agent is told its page is gone rather than
  // that somebody shut it.
  for (const tab of workbench.tabs) {
    if (tab.kind === 'terminal') TerminalClose(tab.id)
    else if (tab.kind === 'browser') {
      // An agent's page survives the switch: its chat may still be working it,
      // and the snapshot above just filed it (by id) on that chat's saved
      // desk. It parks on the shadow rack — window alive, hidden — and the
      // restore below re-adopts it the moment its session is back on screen.
      // The user's own tabs keep their old lifecycle: torn down here, brought
      // back by URL when this session returns.
      if (tab.mine && tab.sessionId) workbench.foreign.push(tab)
      else BrowserCloseForTeardown(tab.id)
    }
  }
  workbench.tabs = []
  workbench.activeId = ''
  let saved: { tabs: SavedTab[]; activeIdx: number }
  try {
    saved = JSON.parse(localStorage.getItem(wbKey(sessionId)) ?? '') as typeof saved
  } catch {
    return
  }
  for (const s of saved.tabs ?? []) {
    // A layout saved before the Review panel — or the Tools panel, removed
    // 2026-08-19 — still names it. Such a tab is simply skipped rather than
    // special-cased into an error: the rest of that session's tabs must still
    // come back.
    //
    // Every singleton room the snapshot admits has to be named here. The save
    // filter takes everything that is not a terminal, so a room added later and
    // not added to this list is saved and then silently dropped — the code map
    // or the working tree, open when you left a chat and gone when you came
    // back to it.
    if (s.kind === 'files') openFilesTab()
    else if (s.kind === 'decks') openDecksTab()
    else if (s.kind === 'git') openGitTab()
    else if (s.kind === 'pr') openPRTab()
    else if (s.kind === 'repomap') openRepoMapTab()
    else if (s.kind === 'file' && s.path) await openFileTab(s.path, s.name, s.mine ?? false)
    else if (s.kind === 'terminal') await restoreTerminalTab(s)
    else if (s.kind === 'browser') {
      // The saved id first: if that window is alive on the shadow rack (its
      // agent kept browsing while this chat was off screen), adopt it whole —
      // same object, same native window, current page, no reload. Only a dead
      // id falls to the URL rebuild below.
      const rackAt = s.id ? workbench.foreign.findIndex((f) => f.id === s.id) : -1
      if (rackAt >= 0) {
        const [tab] = workbench.foreign.splice(rackAt, 1)
        workbench.tabs.push(tab)
      } else {
        const id = openBrowserTab()
        const tab = workbench.tabs.find((t) => t.id === id)
        if (tab) {
          tab.url = s.url ?? ''; tab.name = s.name
          // `mine` survives for browser tabs now too (§187's rule): the desk
          // close safety judges by it, and a rebuilt agent page is still the
          // agent's page.
          tab.mine = s.mine
          if (s.mine) tab.sessionId = sessionId
        }
      }
    }
  }
  workbench.activeId = workbench.tabs[saved.activeIdx]?.id ?? workbench.tabs.at(-1)?.id ?? ''
}

/** Start a terminal for a tab the saved layout is bringing back.
 *
 * A new shell, in this session's own folder — TerminalStart spawns into the
 * sandbox root of whichever chat is current, so the restored terminal opens
 * where the chat works rather than where the old one happened to be.
 *
 * The shell is chosen from what the machine actually has, in three steps: the
 * exact path it was started from, then the same profile by name (an update
 * moves pwsh.exe and the saved path stops existing), then whatever comes
 * first. A layout written before terminals were saved has neither and lands on
 * the third, which is the same shell the + menu opens by default.
 *
 * A failure is skipped rather than raised: one shell that will not start must
 * not take the rest of the desk down with it.
 */
async function restoreTerminalTab(s: SavedTab): Promise<void> {
  try {
    const shells = await TerminalShells()
    const path = shells.find((sh) => sh.path === s.shell)?.path
      ?? shells.find((sh) => sh.name === s.name)?.path
      ?? shells[0]?.path
    if (!path) return
    const id = await TerminalStart(path, 80, 24)
    workbench.tabs.push({ id, kind: 'terminal', name: s.name, shell: path, mine: s.mine })
  } catch {
    // No shell, or the spawn failed. The tab is simply not there.
  }
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

/** Drop a deleted session's stored layout — and its parked pages, whose
 * windows nothing else would ever close once the session that owned them is
 * gone. */
export function removeWorkbenchState(sessionId: string): void {
  localStorage.removeItem(wbKey(sessionId))
  for (const tab of workbench.foreign.filter((f) => f.sessionId === sessionId)) {
    BrowserCloseForTeardown(tab.id)
  }
  workbench.foreign = workbench.foreign.filter((f) => f.sessionId !== sessionId)
}
