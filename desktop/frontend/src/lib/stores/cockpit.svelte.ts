// The single source of truth for cockpit UI state. Reactive ($state); components
// read slices of it via props from App. Mutate its fields (the Go core can push
// incremental updates here — append a chat message, advance a timeline step) and
// the UI reacts. Do not reassign `cockpit` itself; mutate its properties.

import { emptyCockpitState, type CockpitState, type TreeNode, type Session, type ToolStep, type ToolEvent, type ChatMessage, type MessageVariant, type TurnPart, type PendingFile } from '../types'
import type { CockpitSource } from '../services/cockpit'
import {
  SendMessage, GetProjectStatus, GetModelInfo, OpenProjectFolder, OpenProjectPath,
  SwitchProvider, SwitchThinkLevel, SwitchApprovalMode, SetProviderWireFormat,
  SwitchModel, SetAPIKey, SetProviderBaseURL, ProjectTree, ReadFile,
  ListSessions, LoadSession, NewSession, NewSessionAt, NewChairSession, NewSessionInSpace, CurrentSpace, SessionsInSpace, SessionMode, SessionAgent, CurrentSessionID, SearchSessions, DeleteSession,
  SessionTranscript, TurnInFlight,
  SaveChatImage, SaveChatImageData, SaveChatFile, ReadImageDataURL, CancelTurn, BrowserGetText, RecentProjects,
  ListSessionsForDoor, SearchSessionsForDoor, LoadSessionAnyProject, ClearProjectFocus,
  AnswerUserQuestion, Interject, RetryActiveProvider, PendingUndo, UndoLastTurn,
  CompleteSignIn, SignOut, ImportSignIn,
  ListTaskChips, DismissTaskChip,
  BackgroundTasks,
  BackgroundRuns,
  RateTurn, PendingLearnedCount, PendingIssueCount,
  WorkspaceFolders, AddWorkspaceFolder, RemoveWorkspaceFolder,
  RetryFailedTurn, RegenerateReply, ResendEdited, SwitchVariant,
  ExportSession, ImportSession,
  Stance, Stances, SetStance,
} from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { t } from '../i18n.svelte'
import { shell, setShell, shellForDesk, deskForShell, deskFilterFor, SHELLS, type ShellName } from '../shell.svelte'
import { workbench, switchWorkbenchSession, adoptWorkbenchSession, removeWorkbenchState } from './workbench.svelte'

// Model info comes from a real Go IPC round-trip (GetModelInfo), which is
// only as fast as the whole engine bootstrap (provider client, skill
// discovery, MCP servers, ...) finishing first — so first paint would
// otherwise sit on a "loading" placeholder for however long that takes,
// every single app start. Caching the last-known values in localStorage and
// seeding cockpit.model from them synchronously, before any await, means
// first paint shows the (almost always still-correct) real dropdowns
// immediately; loadRealState's actual GetModelInfo call still runs and
// corrects it silently if anything changed. Empty-state placeholders are
// still the right behavior for a genuine first-ever launch (nothing cached
// yet) — this only smooths every launch after that.
const MODEL_CACHE_KEY = 'lastModelInfo'

function cacheModelInfo(model: CockpitState['model']): void {
  try {
    localStorage.setItem(MODEL_CACHE_KEY, JSON.stringify(model))
  } catch {
    // localStorage unavailable/full — the loading placeholder is the fallback, not fatal.
  }
}

function seedModelFromCache(): Partial<CockpitState['model']> {
  try {
    const raw = localStorage.getItem(MODEL_CACHE_KEY)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

export const cockpit = $state<CockpitState>(emptyCockpitState())
Object.assign(cockpit.model, seedModelFromCache())

export async function hydrate(source: CockpitSource): Promise<void> {
  Object.assign(cockpit, await source.load())
}

function applyModelInfo(info: main.ModelInfo): void {
  Object.assign(cockpit.model, {
    provider: info.provider,
    modelName: info.modelName,
    thinkLevel: info.thinkLevel,
    contextUsed: info.contextUsed,
    contextMax: info.contextMax,
    approval: info.approvalMode,
    wireFormat: info.wireFormat,
    warning: info.warning,
  })
  // warning is live state, not a paint-smoothing hint: a provider that was
  // unreachable last run may well be up now, and seeding a stale "not running"
  // banner before the real GetModelInfo lands would be a lie on every launch.
  cacheModelInfo({ ...cockpit.model, warning: '' })
}

/** Pull the real file tree the Go engine currently has. */
export async function refreshWorkspace(): Promise<void> {
  // Used to fetch CommandHistory() and GitChangedFiles() alongside this, both
  // read only by the Review panel — removed 2026-08-03. The tree already
  // carries git status per node, which is where the M/U badges come from, so
  // nothing was lost by dropping the second git call.
  const tree = await ProjectTree()
  // Go's generated bindings type these fields as plain `string`; the values
  // are always one of the frontend's narrower literals ("dir"/"file", "M"/"U"/"").
  cockpit.tree = tree as unknown as TreeNode[]
}

/** "2 นาทีที่แล้ว" for an RFC3339 stamp. Exported because the browser tab's
 * start page timestamps its rows with it — formatting them in Go would put
 * Thai copy in a binding and duplicate a vocabulary the frontend owns. */
export function agoLabel(iso: string): string {
  const parsed = Date.parse(iso)
  if (Number.isNaN(parsed)) return ''
  const mins = Math.max(0, Math.round((Date.now() - parsed) / 60000))
  if (mins < 1) return t('cockpit.justNow')
  if (mins < 60) return t('cockpit.minutesAgo', { mins })
  const hrs = Math.round(mins / 60)
  if (hrs < 24) return t('cockpit.hoursAgo', { hrs })
  return t('cockpit.daysAgo', { days: Math.round(hrs / 24) })
}

/** Pull this project's chat history (sessions are stored per project in Go). */
export async function refreshSessions(): Promise<void> {
  const [metas, current] = await Promise.all([ListSessions(), CurrentSessionID()])
  cockpit.sessions = metas.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), updatedAt: m.updatedAt, active: m.id === current, mode: m.mode, agent: m.agent,
  }))
  // Keeps the workbench layout keyed to whichever session is actually live —
  // restores it on app start, migrates it when the engine re-keys the chat.
  await adoptWorkbenchSession(current)
}

/** Full-text search this project's history (Thai/English substrings, FTS5). */
export async function searchSessions(query: string): Promise<void> {
  if (!query.trim()) return refreshSessions()
  const [hits, current] = await Promise.all([SearchSessions(query), CurrentSessionID()])
  cockpit.sessions = hits.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), updatedAt: m.updatedAt, active: m.id === current, snippet: m.snippet, mode: m.mode, agent: m.agent, space: m.space,
  }))
}

/** Pull this door's chat history across every project, newest first.
 *
 * Scoped in SQL rather than filtered here — see deskFilterFor for why the
 * difference matters once the history is longer than one page. */
export async function refreshGlobalHistory(): Promise<void> {
  const [metas, current] = await Promise.all([
    ListSessionsForDoor(deskFilterFor(shell.name)), CurrentSessionID(),
  ])
  cockpit.history = metas.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), updatedAt: m.updatedAt, active: m.id === current, projectName: m.projectName, mode: m.mode, agent: m.agent,
  }))
}

/** Full-text search this door's chat history across every project. */
export async function searchGlobalHistory(query: string): Promise<void> {
  if (!query.trim()) return refreshGlobalHistory()
  const [hits, current] = await Promise.all([
    SearchSessionsForDoor(query, deskFilterFor(shell.name)), CurrentSessionID(),
  ])
  cockpit.history = hits.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), updatedAt: m.updatedAt, active: m.id === current,
    snippet: m.snippet, projectName: m.projectName, mode: m.mode, agent: m.agent, space: m.space,
  }))
}

// sendUserMessage folds attachments into the sent text as marker lines, because
// the model only ever reads text — and that text is exactly what the transcript
// stores. So restoring a session has to fold them back out, or the bubble shows
// a raw "[attachment: …] .aetox-attachments/x.mp4" line and no chip at all.
// These two patterns are the inverse of the ones written there; change both together.
const ATTACH_CTX_RE = /\n*\[attachment: [^\]]*\] ([^\n]*):\n```\n([\s\S]*?)\n```/g
const ATTACH_FILE_RE = /\n*\[attachment: user-attached (image|audio|video|file) — [^\]]*\] (\S+)/g

/** The first few lines of an attached file, for its card.
 *
 * Blank lines are dropped rather than shown: three lines is the whole budget,
 * and a markdown file that opens with a heading and a gap would spend two of
 * them on nothing. Long lines are cut before they reach the DOM — a minified
 * JSON is one line of 200KB, and CSS clipping it still means holding it. */
export function attachmentPreview(content: string): string {
  return content
    .split('\n')
    .map((l) => l.trimEnd())
    .filter((l) => l !== '')
    .slice(0, 3)
    .map((l) => (l.length > 200 ? l.slice(0, 200) + '…' : l))
    .join('\n')
}

/** A stored transcript, as the chat draws it.
 *
 * Everything per-message happens in restoreAttachments; what happens here is the
 * one thing a single row cannot answer for itself. A turn that failed is stored
 * as an agent row carrying `errorText`, and the text to retry with is the
 * question above it — raw, straight off the row, because that is what was sent
 * (the processed bubble has its attachment lines folded out).
 *
 * The sentence the user reads is composed here rather than stored, so a chat
 * written in Thai reads in English the moment the user switches language — the
 * store keeps the error, this keeps the wording, exactly as turnEndedBubble does
 * for a failure that happens while you are watching.
 */
export function restoreTranscript(messages: main.SessionMessage[] | null | undefined): ChatMessage[] {
  // A conversation with no rows yet comes back from Go as a nil slice, and a
  // nil slice reaches this window as `null`, not as `[]`. Every caller here
  // then died on `null.map` — inside an async function, so the throw became an
  // unhandled rejection and the click it came from simply did nothing. An
  // empty chat is a real state (a session opened and not yet spoken to); it is
  // not an error, and it is certainly not "the button is broken".
  //
  // Bound once and read everywhere below, rather than coalescing at the .map
  // and then indexing the original: the loop is guarded today only because an
  // empty `out` never enters it, which is a coincidence of this exact code and
  // not something the next edit is told about.
  const rows = messages ?? []
  const out = rows.map(restoreAttachments)
  for (let i = 0; i < out.length; i++) {
    const err = rows[i].errorText
    if (!err || out[i].role !== 'agent') continue
    const question = rows[i - 1]
    const ending = /context canceled/i.test(err)
      ? t('cockpit.turnStopped')
      : t('cockpit.sendError', { err })
    out[i].failed = true
    out[i].failedText = question?.role === 'user' ? question.text : undefined
    out[i].text = out[i].text.trim() ? `${out[i].text}\n\n${ending}` : ending
  }
  return out
}

function restoreAttachments(m: main.SessionMessage): ChatMessage {
  const out: ChatMessage = {
    role: m.role === 'agent' ? 'agent' : 'user',
    text: m.text,
    time: m.time,
    id: m.id || undefined,
    rating: (m.rating as ChatMessage['rating']) || undefined,
    reasoning: m.reasoning || undefined,
    thinkSecs: m.thinkSecs || undefined,
    // A reloaded answer keeps its alternates but not their tool timelines —
    // the store has never held one. The switcher works; the toggles come back
    // only for whatever is generated from here on.
    variants: m.variants?.length ? (m.variants as MessageVariant[]) : undefined,
    activeVariant: m.variants?.length ? (m.active ?? 0) : undefined,
    parts: m.parts?.length ? (m.parts as TurnPart[]) : undefined,
    // The stored sequence, folded back into the timeline the bubble has always
    // drawn. This is what the sequence bought: the toggles under a reopened
    // answer used to be empty, because a tool call was never written down
    // anywhere a message could reach.
    steps: stepsFromParts(m.parts as TurnPart[] | undefined),
    producedFiles: filesFromParts(m.parts as TurnPart[] | undefined),
    proposals: proposalsFromParts(m.parts as TurnPart[] | undefined),
  }
  if (out.role === 'agent') return out
  // What is folded out is also kept: editing a restored question has to be able
  // to re-send the exact lines the model was given the first time.
  let suffix = ''
  out.text = out.text
    .replace(ATTACH_CTX_RE, (all, label: string, content: string) => {
      out.contextLabel = label
      // Rebuilt from the stored block rather than persisted separately, so a
      // reopened question shows the same card it showed when it was asked.
      out.contextPreview = attachmentPreview(content)
      suffix += all
      return ''
    })
    .replace(ATTACH_FILE_RE, (all, kind: string, relPath: string) => {
      if (kind === 'image') out.imageRelPath = relPath
      else { out.attachKind = kind as PendingFile['kind']; out.attachLabel = relPath.split('/').pop() }
      suffix += all
      return ''
    })
    .trim()
  if (suffix) out.attachSuffix = suffix
  return out
}

/**
 * Fold a stored turn sequence back into the timeline rows the bubble draws.
 *
 * The layout is deliberately the one it has always been: the answer, and a row
 * of collapsed toggles above it. Drawing the sequence inline was tried and read
 * worse — the same thinking segment appeared twice, once as a toggle and once
 * between the paragraphs, and the answer stopped being the thing your eye lands
 * on. The sequence's job is to be recorded, not to be the layout.
 *
 * What it buys is that those toggles now work on a reopened session. Tool calls
 * were never written down anywhere a message could reach, so every restored
 * answer used to come back with an empty timeline.
 *
 * The last text part is the answer and is already the bubble; every earlier one
 * is narration and becomes a note row, exactly where it sat before.
 */
/**
 * The files a stored turn produced, back out of the same parts.
 *
 * The live path collects these into cockpit.turnFiles as the turn runs; this is
 * the other half, and it exists because the first version had only the live one.
 * Reopening the app dropped every card, and the workbook the answer was still
 * talking about had no way back to it — the exact "the file exists and you
 * cannot reach it" problem the card was built to remove.
 *
 * Deduped: a turn may well write the same file twice, once as a correction.
 */
function filesFromParts(parts?: TurnPart[]): string[] | undefined {
  if (!parts?.length) return undefined
  const files: string[] = []
  for (const part of parts) {
    for (const path of part.tool?.artifacts ?? []) {
      if (path && !files.includes(path)) files.push(path)
    }
  }
  return files.length ? files : undefined
}

/**
 * The changes a stored turn asked to remember, out of the same parts.
 *
 * The card reads its row from the queue by id, so this carries no text: a
 * proposal approved from Settings last week has to come back saying so, and a
 * copy of the sentence frozen into the transcript would keep asking.
 */
function proposalsFromParts(parts?: TurnPart[]): number[] | undefined {
  if (!parts?.length) return undefined
  const ids: number[] = []
  for (const part of parts) {
    const id = part.tool?.proposalId
    if (id && !ids.includes(id)) ids.push(id)
  }
  return ids.length ? ids : undefined
}

function stepsFromParts(parts?: TurnPart[]): ToolStep[] | undefined {
  if (!parts?.length) return undefined
  const lastTextAt = parts.map((p) => p.kind).lastIndexOf('text')
  const steps: ToolStep[] = []
  parts.forEach((part, i) => {
    if (part.kind === 'text') {
      if (i === lastTextAt || !part.text) return
      // An answer an interjection re-placed comes back as the answer it was.
      // As a note it came back at --fs-xs in --text-muted with its markdown
      // showing as source, folded behind the "used N tools" toggle — a reply
      // the user had read, filed away as a footnote to the tools.
      steps.push({
        kind: part.demoted ? 'said' : 'note',
        label: part.text, state: 'done', startedAt: 0,
      })
      return
    }
    if (part.kind === 'thinking') {
      steps.push({ kind: 'thinking', label: '', state: 'done', secs: part.secs, startedAt: 0 })
      return
    }
    const tool = part.tool
    if (!tool) return
    steps.push({
      label: [tool.name, tool.subject].filter(Boolean).join(' '),
      ref: tool.ref,
      state: tool.ok ? 'done' : 'err',
      error: tool.error || undefined,
      secs: tool.secs || undefined,
      added: tool.added || undefined,
      removed: tool.removed || undefined,
      agent: tool.agent || undefined,
      brief: tool.brief || undefined,
      agentKind: tool.agentKind || undefined,
      delegation: tool.delegation,
      startedAt: 0,
    })
  })
  return steps.length ? steps : undefined
}

/** Thumbnails are read back off disk, so they land a moment after the text. */
function hydrateImages(): void {
  cockpit.chat.forEach((m, i) => {
    if (!m.imageRelPath || m.imageDataUrl) return
    ReadImageDataURL(m.imageRelPath)
      .then((url) => { cockpit.chat[i].imageDataUrl = url })
      .catch(() => {}) // file moved or sandbox cleared — the chip still names it
  })
}

/** Open a session from the global history list — switches project first if it belongs to a different one.
 *
 * Opening a chat has to *show* the chat. Every other way into a session already
 * said so — openDesk and switchShell both call setActiveView('chat') — and this
 * one, the one the history list uses, did not. Click a row from Settings or the
 * โปรเจกต์ page and the conversation loaded correctly behind the page you were
 * looking at: the row lit up (the list refreshes at the end of this function),
 * the view never moved, and the only honest reading from the outside is that
 * the row is dead. Same for importChat, which comes through here — the file
 * imported and nothing appeared.
 *
 * First, not last: the awaits below are engine round-trips, and a view that
 * switches after them shows the *old* chat for as long as they take.
 *
 * And the load is allowed to fail out loud. LoadSessionAnyProject has seven
 * separate refusals, each with a written Thai sentence saying exactly what is
 * wrong — the project folder was moved, the desk file is gone, the session is
 * not in this project. Not one of them was ever read: an unhandled rejection
 * here aborts the function and stops, so a row whose session cannot be opened
 * behaves identically to a row that is not wired up. The engine had the answer
 * the whole time and the window threw it away. */
export async function selectGlobalSession(session: Session): Promise<void> {
  if (cockpit.awaitingReply) return peekAtSession(session)
  dropPeek()
  setActiveView('chat')
  let messages: main.SessionMessage[]
  try {
    messages = await LoadSessionAnyProject(session.id)
  } catch (err) {
    cockpit.sessionError = err instanceof Error ? err.message : String(err)
    return
  }
  cockpit.sessionError = ''
  cockpit.todos = []
  cockpit.ask = null
  cockpit.chat = restoreTranscript(messages)
  hydrateImages()
  await refreshDesk()
  await switchWorkbenchSession(session.id)
  const project = await GetProjectStatus()
  Object.assign(cockpit.project, project)
  await refreshWorkspace()
  await refreshProjectFolders()
  await refreshUndo()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
}

/** Pull the folders added to the focused project.
 *
 * Read from Go rather than tracked locally on purpose: this list IS the
 * permission the agent runs with, and a frontend copy that drifts from it would
 * show the user a set of folders that is not the set the tools can reach. */
export async function refreshProjectFolders(): Promise<void> {
  cockpit.projectFolders = await WorkspaceFolders()
}

/** Add a folder to the focused project, with the same rights the project has.
 * Returns the error text to show, or '' — the caller draws it next to the
 * button that asked, because a refusal the user cannot see reads as a
 * broken button. */
export async function addProjectFolder(): Promise<string> {
  try {
    cockpit.projectFolders = await AddWorkspaceFolder()
    return ''
  } catch (err) {
    await refreshProjectFolders()
    return err instanceof Error ? err.message : String(err)
  }
}

/** Drop a folder. The running session loses it immediately — the Go side
 * rebuilds the engine before this resolves. */
export async function removeProjectFolder(path: string): Promise<void> {
  try {
    cockpit.projectFolders = await RemoveWorkspaceFolder(path)
  } catch {
    await refreshProjectFolders()
  }
}

/** Pull the list of every project ever opened (sidebar's project switcher), newest first. */
export async function refreshProjects(): Promise<void> {
  const [metas, current] = await Promise.all([RecentProjects(), GetProjectStatus()])
  cockpit.projects = metas.map((m) => ({
    key: m.key, name: m.name, path: m.rootPath, ago: agoLabel(m.openedAt),
    active: m.rootPath === current.path, snippet: m.snippet,
  }))
}

/** Pull the real project/model state the Go engine is actually running with.
 * On a cold start the engine may still be bootstrapping (provider connect,
 * MCP registration) — an empty provider is treated as "not ready yet": it is
 * never applied (so it can't clobber the localStorage seed cache) and the
 * load retries until the engine reports real state. */
let bootRetries = 0
export async function loadRealState(): Promise<void> {
  const [project, modelInfo] = await Promise.all([GetProjectStatus(), GetModelInfo()])
  Object.assign(cockpit.project, project)
  if (modelInfo.provider) applyModelInfo(modelInfo)
  await refreshWorkspace()
  await refreshProjectFolders()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
  await refreshTaskChips()
  await refreshDesk()
  await restoreLiveTranscript()
  if (!modelInfo.provider && bootRetries < 8) {
    bootRetries += 1
    setTimeout(loadRealState, 1500)
  }
}

/** True between a mid-turn reload and that turn's agent:done — the window is
 * watching a turn it has no promise for, so the event is its only ending. */
let reattachedTurn = false

/** Put the conversation the engine is actually in back on screen.
 *
 * The Go backend outlives a webview reload — an F5, and every Vite HMR full
 * reload under `wails dev` — so it is still holding the session, still has its
 * memory, and the next message continues where the last one left off. The
 * frontend that just booted holds nothing, and nothing was reading the
 * transcript back: `cockpit.chat` stayed empty and the composer's empty state
 * took the screen. It read as being thrown back to the start of the app, while
 * the engine had in fact lost nothing.
 *
 * Read through SessionTranscript, not LoadSession: the engine never lost this
 * session, so there is no context to rebuild — and mid-turn, LoadSession's
 * rebuild would rewrite the memory the running turn is thinking with (it now
 * refuses during a turn for exactly that reason, which would turn this restore
 * into a welcome screen over a working agent).
 *
 * And the turn itself is asked about, not assumed over. A reload used to reset
 * the window to idle while the engine was mid-answer: the question came back,
 * the live block did not, the chunks still streaming in were accumulated into
 * state nothing rendered, and the finished reply had no promise left to land
 * in. A turn that ends while this restore is still in flight lands in the
 * recheck below: its agent:done fired before the flag armed, into a window
 * that skipped it, so the restore asks once more after arming.
 *
 * Restores only when the screen is empty, so a reload mid-answer cannot wipe
 * what has already streamed in. A session with no messages — a cold start, or
 * one the user opened and never spoke to — restores to empty, which is exactly
 * the welcome screen it should be.
 */
async function restoreLiveTranscript(): Promise<void> {
  if (cockpit.chat.length > 0) return
  let turn = { running: false, sessionId: '' }
  try {
    turn = await TurnInFlight()
  } catch {
    // Engine not ready yet — the retry loop in loadRealState comes back here.
  }
  const id = await CurrentSessionID()
  if (!id) return
  try {
    const messages = await SessionTranscript(id)
    if (messages.length > 0) {
      cockpit.chat = restoreTranscript(messages)
      hydrateImages()
      await switchWorkbenchSession(id)
    }
  } catch {
    // Transcript unreadable — the welcome screen is the honest fallback,
    // better than an error bubble in a conversation the user has not started.
  }
  if (turn.running && turn.sessionId === id) {
    // The live block comes back on: chunks and tool events streaming in render
    // again, typing goes into the running turn (Interject), and Stop works.
    cockpit.awaitingReply = true
    reattachedTurn = true
    // The turn may have finished in the moment before the flag armed — its
    // agent:done fired into a window that still skipped the event, and nothing
    // else would ever take awaitingReply back down. One recheck closes the
    // gap; if the real event also ran, the flag is already consumed and this
    // is a no-op.
    try {
      const now = await TurnInFlight()
      if (!now.running) await applyAgentDone({ sessionId: id })
    } catch {
      // Engine unreachable for the recheck — stay armed; agent:done ends it.
    }
  }
}

/** The turn the engine announced finished (agent:done).
 *
 * Only a window that reattached after a mid-turn reload consumes this: for the
 * window that sent the message, the SendMessage promise is still the delivery
 * and does everything below itself. The reattached window instead re-reads the
 * transcript from the store — which now holds the finished turn, whatever kind
 * of turn it was — because the live detail it would have assembled from events
 * died with the previous webview.
 */
export async function applyAgentDone(status: { sessionId: string }): Promise<void> {
  if (!reattachedTurn) return
  reattachedTurn = false
  cockpit.awaitingReply = false
  for (const m of cockpit.chat) m.duringTurn = undefined
  cockpit.agentStatus = ''
  cockpit.toolSteps = []
  cockpit.turnFiles = []
  cockpit.turnProposals = []
  cockpit.streamingText = ''
  cockpit.reasoningText = ''
  cockpit.sessionError = ''
  const id = await CurrentSessionID()
  if (status?.sessionId === id) {
    try {
      const messages = await SessionTranscript(id)
      cockpit.chat = restoreTranscript(messages)
      hydrateImages()
    } catch {
      // The store still holds the turn; the next open shows it.
    }
  }
  await refreshWorkspace()
  await refreshUndo()
  await refreshSessions()
  await refreshGlobalHistory()
  // The straggler net, same as sendUserMessage's tail: a message the ending
  // turn could not fold in came back on agent:interjection-missed, and in this
  // window there is no promise tail to send it. alreadyShown because the net
  // has always worked that way — the text goes out as its own turn, and the
  // transcript re-read above is what the screen shows meanwhile.
  const next = queuedMessages.shift()
  if (next !== undefined) await sendUserMessage(next, true)
}

/** The one gate in front of every door that leaves the running turn's chat.
 *
 * The engine refuses these too (desktop/app.go guardSessionSwitch) — one brain,
 * one turn, and every switch rewrites the memory that turn is thinking with.
 * Checking here as well is not a second answer to the same question: it is how
 * the refusal reaches doors whose engine call the UI never awaits with an
 * error surface (Ctrl+N, the desk buttons), as a sentence instead of a click
 * that did nothing. Cleared when the turn ends, where every live panel resets.
 */
function turnStillRunning(): boolean {
  if (!cockpit.awaitingReply) return false
  cockpit.sessionError = t('cockpit.turnBusy')
  return true
}

/** Whether this row's conversation has a turn running in it right now.
 *
 * The sidebar draws it with the ring a working delegation wears (style.css),
 * because it is the same fact both times: this one is still going. Without it,
 * a chat opened while another is working is a list of rows that all look idle
 * — and the whole point of leaving a turn running is being able to walk away
 * from it and still know it is there.
 *
 * One turn at a time today, so "the working chat" is the engine's own session
 * and `active` is the flag that names it — true even while another chat is on
 * screen, which is exactly the case this is for. Written as a function rather
 * than inlined into the markup so that the day sessions run side by side, the
 * answer changes in one place instead of in every list that draws a row. */
export function sessionWorking(s: Session): boolean {
  return !!s.active && cockpit.awaitingReply
}

/** The chat the running turn belongs to — the one its answer has to land in.
 *
 * While another conversation is open for reading, `cockpit.chat` is that other
 * conversation and the working one is held in `cockpit.peek.live`. Every write
 * a turn makes goes through here, so a reply can never be appended to whichever
 * chat happened to be on screen when it arrived. */
function liveChat(): ChatMessage[] {
  return cockpit.peek?.live ?? cockpit.chat
}

/** Open another conversation for reading while a turn runs in this one.
 *
 * The refusal this replaces was honest and too wide. One engine, one context: a
 * running turn cannot have the memory under it rewritten, and that is what
 * *opening* a session does — LoadSession restores the desk, the project and the
 * agent's history. Reading one does none of it. SessionTranscript touches
 * nothing but the database (it was written for the window that reloads mid-turn
 * and needs its own transcript back), and "let me look at the other chat while
 * this finishes" was never the half of the request that was dangerous.
 *
 * So the door opens for reading and stays shut for writing: the composer is
 * disabled while peeking and the live block is not drawn over a conversation it
 * does not belong to (Chat.svelte). What the turn produces goes on landing in
 * the chat it was asked in, whether or not anybody is watching it arrive. */
export async function peekAtSession(session: Session): Promise<void> {
  if (cockpit.peek?.session.id === session.id) return
  setActiveView('chat')
  let messages: main.SessionMessage[]
  try {
    messages = await SessionTranscript(session.id)
  } catch (err) {
    cockpit.sessionError = err instanceof Error ? err.message : String(err)
    return
  }
  cockpit.sessionError = ''
  try {
    // Read before the assignment below, so peeking from inside a peek keeps the
    // working chat rather than stashing the one being read.
    const live = liveChat()
    cockpit.chat = restoreTranscript(messages)
    cockpit.peek = { session, live }
    hydrateImages()
  } catch (err) {
    // The rest of this store is written so a refusal is a sentence rather than
    // a dead click (selectGlobalSession's own note says so). An exception in
    // here was the one way left to get the dead click anyway: it escapes as a
    // rejected promise, the row stays where it was, and nothing on screen
    // changes or explains. Whatever broke, say it.
    cockpit.sessionError = err instanceof Error ? err.message : String(err)
  }
}

/** The gate in front of every door that acts on the conversation on screen,
 *  when that conversation is one the user is only reading.
 *
 * Send is the obvious one. The rest are not: retry, edit-and-resend, regenerate
 * and undo all bail on `awaitingReply` already, which covered them for exactly
 * as long as a peek could only exist during a turn. It outlives the turn — the
 * user goes on reading after it ends — and in that window those doors would act
 * on the engine's session (the working chat) using a bubble picked from the one
 * being read. Same sentence for all of them, one place. */
function peekIsReadOnly(): boolean {
  if (!cockpit.peek) return false
  cockpit.sessionError = t('cockpit.peekReadOnly')
  return true
}

/** Forget a chat that was open for reading, without putting it back on screen.
 *
 * leavePeek RESTORES the working chat; this only ends the reading. Every door
 * that switches for real is about to write cockpit.chat itself, so restoring
 * the held array first would be a chat drawn for one frame and thrown away.
 *
 * It exists because the peek outlives the turn, and I shipped it without this:
 * open another chat while the agent worked, let the turn finish, then switch
 * anywhere at all — the peek was still set, so peekIsReadOnly() refused every
 * message in every conversation and liveChat() handed out a detached array.
 * The app looked broken and stayed broken, which is worse than the refusal it
 * replaced (owner, 18 ส.ค.: "เปลี่ยนเซสชั่นไปดูอันอื่น แชทแม่งพัง กลับมาแล้วพัง").
 *
 * Called from every door rather than from one clever place, because "the peek
 * ended" is a fact each door knows and no shared choke point does. */
function dropPeek(): void {
  cockpit.peek = null
}

/** Put the working chat back on screen. */
export function leavePeek(): void {
  if (!cockpit.peek) return
  cockpit.chat = cockpit.peek.live
  cockpit.peek = null
  cockpit.sessionError = ''
  hydrateImages()
}

/** Stop reading and open the conversation being read for real — the door that
 *  was shut while the turn was running, offered where the user is standing when
 *  it opens. */
export async function openPeeked(): Promise<void> {
  const peeked = cockpit.peek
  if (!peeked || cockpit.awaitingReply) return
  // Cleared first: selectSession replaces cockpit.chat itself, and a peek left
  // standing would make the next liveChat() hand out the old array.
  cockpit.peek = null
  await selectSession(peeked.session)
}

/** Let the user pick a real folder via the native dialog; re-points the engine at it. */
export async function openFolder(): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  const project = await OpenProjectFolder()
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
  await refreshProjectFolders()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
}

/** Switch straight to a previously-opened project (sidebar's project list), no dialog. */
export async function openProject(path: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  const project = await OpenProjectPath(path)
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
  await refreshProjectFolders()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
}

/** Drop project focus: the AI keeps full machine access (files/git/terminal)
 * but is no longer tied to any project — like opening Claude/Codex bare. */
export async function clearProjectFocus(): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  const project = await ClearProjectFocus()
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
  await refreshProjectFolders()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
}

export async function switchProvider(provider: string): Promise<void> {
  applyModelInfo(await SwitchProvider(provider))
}

export async function switchThinkLevel(level: string): Promise<void> {
  applyModelInfo(await SwitchThinkLevel(level))
}

export async function switchApprovalMode(mode: string): Promise<void> {
  applyModelInfo(await SwitchApprovalMode(mode))
}

export async function switchModel(modelName: string): Promise<void> {
  applyModelInfo(await SwitchModel(modelName))
}

export async function submitAPIKey(providerName: string, apiKey: string): Promise<void> {
  applyModelInfo(await SetAPIKey(providerName, apiKey))
}

/**
 * Finish a sign-in started with StartSignIn. Blocks for as long as the user
 * takes to approve in their browser, so the caller shows the prompt and a
 * cancel button rather than a spinner with no way out.
 */
export async function completeSignIn(providerName: string, pasted: string): Promise<void> {
  applyModelInfo(await CompleteSignIn(providerName, pasted))
}

/** Adopt the session the matching official CLI already holds on this machine. */
export async function importSignIn(providerName: string): Promise<void> {
  applyModelInfo(await ImportSignIn(providerName))
}

/** Forget a provider's sign-in. */
export async function signOutProvider(providerName: string): Promise<void> {
  applyModelInfo(await SignOut(providerName))
}

export async function switchWireFormat(format: string): Promise<void> {
  applyModelInfo(await SetProviderWireFormat(format))
}

/** Re-bootstrap if the engine is stuck on the aetox fallback; no-op otherwise. */
export async function retryActiveProvider(): Promise<void> {
  applyModelInfo(await RetryActiveProvider())
}

/** Point a provider at a custom endpoint; '' clears the override. */
export async function setProviderBaseURL(providerName: string, baseURL: string): Promise<void> {
  applyModelInfo(await SetProviderBaseURL(providerName, baseURL))
}

/** The stamp every bubble carries. Exported so anything that has to put one in
 *  the transcript stamps it the same way, rather than inventing a second clock. */
export function nowLabel(): string {
  return new Date().toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

// Typing while a turn is running goes STRAIGHT into that turn — the engine folds
// it in on its next tool-loop round, or keeps the turn alive if it was already
// writing the answer (App.Interject → cognitive.Agent.Interject). Waiting for the
// engine to be free was the old behaviour and the owner's complaint.
//
// This queue is what is left of it: the straggler net. A message that lands in
// the moment between the loop's last check and the reply arriving comes back on
// `agent:interjection-missed` and goes out as its own turn.
export const queuedMessages = $state<string[]>([])

/** Drop anything still waiting — Stop has to mean stop, including what was typed under it. */
export function clearQueuedMessages(): void {
  queuedMessages.length = 0
}

/**
 * The engine handed back a message it could not fold into the turn that was
 * ending. Its bubble is already on screen (it was shown the moment it was typed),
 * so it goes out as its own turn with `alreadyShown` set.
 */
export function applyMissedInterjections(texts: string[]): void {
  for (const text of texts) {
    if (text.trim()) queuedMessages.push(text)
  }
}

/**
 * Append the user message, then call the Go core and append its reply.
 *
 * `alreadyShown` is for the straggler path above: the bubble is on screen and the
 * attachment markers are already folded into `text`, so both steps are skipped.
 */
/**
 * refreshUndo asks what an undo would touch right now, so the chip is offered
 * only when there is something to offer. Silent on failure: undo is a safety
 * net, and a net that shouts when it is absent is worse than one that is quiet.
 */
export async function refreshUndo(): Promise<void> {
  try {
    cockpit.undoFiles = (await PendingUndo()) ?? []
  } catch {
    cockpit.undoFiles = []
  }
}

/**
 * undoLastTurn puts back every file the last turn changed. The result is posted
 * into the transcript rather than shown as a toast: undoing is a real event in
 * the session's history, and a message the user can scroll back to is the only
 * record of it that survives.
 */
export async function undoLastTurn(): Promise<void> {
  if (peekIsReadOnly()) return
  try {
    const result = await UndoLastTurn()
    const files = result?.files ?? []
    const text = files.length > 0
      ? [t('cockpit.undoDone', { count: String(files.length) }), ...files.map((f) => `- ${f}`)].join('\n')
      : (result?.reason || t('cockpit.undoNothing'))
    cockpit.chat.push({ role: 'agent', text, time: nowLabel() })
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.undoFailed', { err: String(err) }), time: nowLabel() })
  }
  cockpit.undoFiles = []
  await refreshWorkspace()
}

/** The bubble a turn that came back with no answer leaves behind.
 *
 * Stop is not an error. The engine reports a cancelled turn as `context
 * canceled` (Go's one canonical string for it), and showing that as "เกิด
 * ข้อผิดพลาด" told the user the app broke when the app did exactly what they
 * pressed. The retry chip rides along either way, because a stopped question is
 * still a question the user may want re-run.
 *
 * Whatever streamed before the turn ended is kept in BOTH cases — read here,
 * before runLiveTurn's cleanup erases it. Keeping it only for Stop was an
 * accident of which case got written first: a quota that runs out or a
 * connection that drops mid-answer ends the turn exactly as abruptly, and half
 * an answer the user watched arrive is not the app's to throw away because of
 * how it stopped arriving. The engine agrees from its own side — App.runTurn
 * returns the partial reply alongside the error — but Wails discards a return
 * value when the error is non-nil, so the live preview is the only copy that
 * reaches here.
 *
 * And the streamed text is not the work. The preview holds only the round the
 * model is writing *now*: the engine erases it (discardAnswerPreview) at the
 * end of every round that ends in a tool call, which is precisely the moment
 * anybody presses Stop. So the bubble used to be built from a field that was
 * empty by construction, and a turn stopped after twenty tool calls left one
 * line reading "หยุดการทำงานแล้ว" over nothing — while `finally` below cleared
 * the timeline that held every one of them.
 *
 * turnArtifacts is the same snapshot the successful path takes, and it is taken
 * here for the same reason: the tool rows, the narration between them (§59) and
 * the thinking are what the turn *did*, and a turn that was stopped did not do
 * less of it. Stop means stop, not discard.
 */
function turnEndedBubble(err: unknown, sentText: string): ChatMessage {
  const partial = cockpit.streamingText.trim()
  const ending = /context canceled/i.test(String(err))
    ? t('cockpit.turnStopped')
    : t('cockpit.sendError', { err: String(err) })
  return {
    role: 'agent',
    text: partial ? `${partial}\n\n${ending}` : ending,
    time: nowLabel(), failed: true, failedText: sentText,
    ...turnArtifacts(),
  }
}

export async function sendUserMessage(text: string, alreadyShown = false): Promise<void> {
  const trimmed = text.trim()
  const image = cockpit.pendingImage
  const context = cockpit.pendingContext
  const file = cockpit.pendingFile
  if (!trimmed && !image && !context && !file) return
  // Reading another conversation is not being in it. The engine's session is
  // still the one the turn belongs to, so a message typed here would be
  // answered into a chat the user is not looking at.
  if (peekIsReadOnly()) return
  // The model only ever sees text, so an attached image is handed to it as a
  // sandboxed path reference it can pass to image_ocr — the bubble itself
  // shows just the caption + thumbnail, not that reference line. A dragged-in
  // file/browser tab instead inlines its actual content directly — no tool
  // call needed for the model to "see" it.
  // Explicit source tags so the model can tell attachment types apart —
  // a user-attached image vs a dragged-in workbench tab, and for tabs,
  // a file on disk vs a live web page. Only the model sees these lines.
  // Built apart from the prose so the two can be recombined later: editing the
  // question re-sends these lines verbatim, and the bubble never shows them.
  let attachSuffix = ''
  if (image) attachSuffix += `\n\n[attachment: user-attached image — read it with image_ocr] ${image.relPath}`
  if (file) {
    // Point at the tool that actually opens this kind of file. Naming the wrong
    // one costs a wasted turn; naming none costs the model guessing.
    // A PDF branches on the extension rather than on kind: it is still a
    // 'file' everywhere else (chip, icon, the restore regex below), and only
    // the tool differs. `read` refuses it — a PDF is a binary container.
    const how = file.kind === 'audio'
      ? 'read it with audio_transcribe'
      : file.kind === 'video'
        ? 'read its speech with audio_transcribe, its on-screen text with video_ocr'
        : file.relPath.toLowerCase().endsWith('.pdf')
          ? 'read it with pdf_read'
          : 'read it with read'
    attachSuffix += `\n\n[attachment: user-attached ${file.kind} — ${how}] ${file.relPath}`
  }
  if (context) {
    const kindLabel =
      context.kind === 'file' ? 'file from a workbench tab'
      : context.kind === 'pick' ? 'what the user pointed at in the workbench browser — the elements themselves, not the page'
      : 'web page text from a workbench browser tab'
    attachSuffix += `\n\n[attachment: ${kindLabel}] ${context.label}:\n\`\`\`\n${context.content}\n\`\`\``
  }
  const sentText = (trimmed + attachSuffix).trim()
  if (!alreadyShown) {
    cockpit.chat.push({
      role: 'user', text: trimmed, time: nowLabel(),
      // Sent into a turn that is already running: it belongs *after* what has
      // streamed so far, not above it. Pushed into the same list either way —
      // the order in the array is already chronological — and the flag only
      // tells the transcript to draw it below the live block until that block
      // is gone (Chat.svelte).
      duringTurn: cockpit.awaitingReply || undefined,
      imageDataUrl: image?.dataUrl,
      contextLabel: context?.label,
      contextPreview: context ? attachmentPreview(context.content) : undefined,
      attachLabel: file?.label, attachKind: file?.kind,
      attachSuffix: attachSuffix || undefined,
    })
  }
  cockpit.pendingImage = null
  cockpit.pendingContext = null
  cockpit.pendingFile = null
  // A turn is already running: hand it the message rather than starting a second
  // one. Its answer arrives inside that turn's reply, so nothing below runs — the
  // live state (toolSteps, streaming text) belongs to the turn still in flight and
  // resetting it here would blank the UI mid-work.
  if (cockpit.awaitingReply) {
    await Interject(sentText)
    return
  }
  await runLiveTurn(async () => {
    try {
      const reply = await SendMessage(sentText)
      liveChat().push({
        role: 'agent', text: reply.text, parts: reply.parts as TurnPart[] | undefined,
        time: nowLabel(), id: reply.messageId || undefined, ...turnArtifacts(),
      })
    } catch (err) {
      // The engine persists nothing for a turn that failed, so the text it was
      // given rides on the bubble: it is the only copy left to retry with.
      liveChat().push(turnEndedBubble(err, sentText))
    }
  })
  // Only stragglers land here now — anything typed under a running turn went
  // straight into it via Interject. Their bubbles are already on screen.
  const next = queuedMessages.shift()
  if (next !== undefined) await sendUserMessage(next, true)
}

/**
 * Wraps one engine call in the live-turn scaffolding: the typing bubble, the
 * tool timeline, the streamed text, and the refreshes that follow a turn.
 *
 * Shared by every path that produces an answer — a send, a retry, a regenerate,
 * an edited resend — because the reset is the part that is easy to get subtly
 * wrong, and four half-copies of it would strand `awaitingReply` on the first
 * path anyone forgot.
 */
async function runLiveTurn(call: () => Promise<void>): Promise<void> {
  cockpit.awaitingReply = true
  cockpit.agentStatus = ''
  cockpit.toolSteps = []
  cockpit.streamingText = ''
  cockpit.reasoningText = ''
  // The question card belongs to the turn that raised it: it is drawn inside
  // the live block (Chat.svelte's `{#if awaitingReply}`), so a stale one is
  // invisible while the chat is idle and then reappears the instant the next
  // turn starts, offering options to a tool that stopped listening.
  //
  // The checklist used to be cleared here for the same reason, and while it
  // lived in that block the reason was right — the previous turn's todos, every
  // item already struck through, sitting under "กำลังคิดคำตอบ…" for work nobody
  // asked for. It is drawn on the strip now (SessionStrip.svelte), under the
  // heading แผน, where the same rows say something true: this is the session's
  // latest plan, and nothing has replaced it yet. A plan that vanishes the
  // moment the turn that wrote it ends is a plan nobody can work from — which
  // is the whole reason วางแผน mode (internal/mode/stance.go) exists.
  //
  // todo_write replaces it wholesale (applyTodos), so a turn that plans anew
  // overwrites this; a turn that does not, leaves the last plan standing.
  //
  // With one exception, and it is the half of the old rule that was worth
  // keeping: a plan whose every row is struck through is a *record*, not work,
  // and asking the next question is the moment that record stops being about
  // anything. Left in, a finished checklist sat over every later turn with no
  // way to get rid of it — the panel had no off switch, because clearing used
  // to be somebody else's job. Unfinished plans still survive: what is kept is
  // work in flight, what is dropped is a receipt.
  if (cockpit.todos.length > 0 && cockpit.todos.every((td) => td.status === 'completed')) {
    cockpit.todos = []
  }
  cockpit.ask = null
  try {
    await call()
  } finally {
    cockpit.awaitingReply = false
    // The live block is gone, so anything that was drawn below it takes its
    // ordinary place in the transcript. The array order never changed — it was
    // chronological all along — only where those bubbles were painted.
    for (const m of liveChat()) m.duringTurn = undefined
    cockpit.agentStatus = ''
    cockpit.toolSteps = []
    cockpit.turnFiles = []
    cockpit.turnProposals = []
    cockpit.streamingText = ''
    cockpit.reasoningText = ''
    // Cleared at both ends, like toolSteps. A turn that died with a question
    // still on screen left a card whose tool is no longer listening — pressing
    // an option answered nothing. The checklist is deliberately not cleared
    // here; see the note at the head of this function.
    cockpit.ask = null
    // The "still working" banner (turnStillRunning) explained a refusal that
    // just stopped being true. A stale one over an idle chat would be a lie.
    cockpit.sessionError = ''
  }
  await refreshWorkspace()
  // The turn may have started delegations it chose not to collect — they are
  // the tray's to show from here on (§105), and this is the moment they stop
  // being visible anywhere else.
  await refreshBackgroundTasks()
  // After the turn, not before: the chip has to reflect what this turn just did.
  await refreshUndo()
  await refreshSessions()
  await refreshGlobalHistory()
}

/**
 * Snapshots what the turn that just finished produced, so it survives the reset
 * above — the live panels alone vanish the moment the turn completes.
 * Must be called before runLiveTurn's finally runs, i.e. inside the call.
 */
function turnArtifacts(): Pick<ChatMessage, 'steps' | 'reasoning' | 'thinkSecs' | 'producedFiles' | 'proposals'> {
  const steps = cockpit.toolSteps.length ? cockpit.toolSteps.map((s) => ({ ...s })) : undefined
  const reasoning = cockpit.reasoningText.trim() || undefined
  const thinkSecs = reasoning ? Math.max(1, Math.round((thinkLastAt - thinkStartedAt) / 1000)) : undefined
  const producedFiles = cockpit.turnFiles.length ? [...cockpit.turnFiles] : undefined
  const proposals = cockpit.turnProposals.length ? [...cockpit.turnProposals] : undefined
  return { steps, reasoning, thinkSecs, producedFiles, proposals }
}

/**
 * Re-run a turn that failed. The error bubble goes; the question stays exactly
 * where it is, because it is still the question being asked.
 *
 * The engine rebuilds its memory from the transcript first (App.RetryFailedTurn)
 * — a failed turn leaves its question in the model's context and nowhere else,
 * so sending the text again without that would ask it twice.
 */
export async function retryFailedTurn(index: number): Promise<void> {
  const failed = cockpit.chat[index]
  const text = failed?.failedText
  if (!text || cockpit.awaitingReply || peekIsReadOnly()) return
  cockpit.chat.splice(index, 1)
  await runLiveTurn(async () => {
    try {
      const reply = await RetryFailedTurn(text)
      cockpit.chat.push({
        role: 'agent', text: reply.text, parts: reply.parts as TurnPart[] | undefined,
        time: nowLabel(), id: reply.messageId || undefined, ...turnArtifacts(),
      })
    } catch (err) {
      cockpit.chat.push(turnEndedBubble(err, text))
    }
  })
}

/**
 * Re-run a turn that failed, with the question reworded first.
 *
 * The sibling of retryFailedTurn above, for when the answer is not "try that
 * again" but "I asked it wrong". Both end at App.RetryFailedTurn, which drops
 * the failed row and the question above it before sending — so handing it
 * different text simply asks the new question in place of the old one.
 *
 * Deliberately not routed through resendEdited: that one splices the last two
 * messages on the assumption they are a completed exchange, and a failed turn
 * is not one. Aiming at the failed bubble by index is what keeps the two cases
 * from having to trust the same guess about the tail.
 *
 * No revert prompt, unlike the edit-after-success path: this is the retry
 * button's road, and retrying a failed turn has never offered one.
 */
export async function editFailedTurn(failedIndex: number, text: string): Promise<void> {
  const trimmed = text.trim()
  if (!trimmed || cockpit.awaitingReply || peekIsReadOnly()) return
  const failed = cockpit.chat[failedIndex]
  if (!failed?.failed) return
  const question = cockpit.chat[failedIndex - 1]
  if (question?.role !== 'user') return
  // The attachment belongs to the question, not to its wording — same rule as
  // resendEdited, for the same reason.
  const sent = (trimmed + (question.attachSuffix ?? '')).trim()
  const previous = cockpit.chat.splice(failedIndex - 1, 2)
  cockpit.chat.push({ ...previous[0], text: trimmed, time: nowLabel() })
  await runLiveTurn(async () => {
    try {
      const reply = await RetryFailedTurn(sent)
      cockpit.chat.push({
        role: 'agent', text: reply.text, parts: reply.parts as TurnPart[] | undefined,
        time: nowLabel(), id: reply.messageId || undefined, ...turnArtifacts(),
      })
    } catch (err) {
      cockpit.chat.push(turnEndedBubble(err, sent))
    }
  })
}

/**
 * Answer the last question again, keeping the previous answer switchable.
 *
 * revertFiles puts back whatever the previous attempt wrote. Answering again
 * without it is the genuinely risky option — the turn's tools run a second time
 * on top of their own output — so the caller offers it whenever there is
 * anything to revert.
 */
export async function regenerateReply(revertFiles: boolean): Promise<void> {
  const last = cockpit.chat.at(-1)
  if (!last || last.role !== 'agent' || last.failed || cockpit.awaitingReply || peekIsReadOnly()) return
  // Whatever is on screen becomes variant 0 — with its tool timeline, which the
  // engine's own list cannot carry (the store keeps no timeline).
  const previous: MessageVariant[] = last.variants ?? [
    { text: last.text, reasoning: last.reasoning, thinkSecs: last.thinkSecs, steps: last.steps },
  ]
  await runLiveTurn(async () => {
    try {
      const result = await RegenerateReply(revertFiles)
      const artifacts = turnArtifacts()
      const variants: MessageVariant[] = result.variants.map((v, i) => ({ ...v, steps: previous[i]?.steps }))
      variants[result.active] = { ...variants[result.active], steps: artifacts.steps }
      Object.assign(last, artifacts, {
        text: result.text, parts: result.parts as TurnPart[] | undefined,
        time: nowLabel(), variants, activeVariant: result.active,
        revertedFiles: result.reverted?.length ? result.reverted : undefined,
        error: undefined,
      })
    } catch (err) {
      // The answer on screen is still the real one — the engine put its memory
      // back. Say what went wrong under it rather than replacing it — and a
      // Stop is not a "went wrong", it is the user declining the re-answer.
      last.error = /context canceled/i.test(String(err))
        ? t('cockpit.turnStopped')
        : t('cockpit.regenerateError', { err: String(err) })
    }
  })
}

/** Show a different one of the stored answers, and continue the conversation
 *  from it: the engine rewrites its memory to match (App.SwitchVariant). */
export async function switchVariant(index: number): Promise<void> {
  const last = cockpit.chat.at(-1)
  if (!last || !last.variants || cockpit.awaitingReply || peekIsReadOnly()) return
  const local = last.variants[index]
  try {
    const result = await SwitchVariant(index)
    Object.assign(last, {
      text: result.text,
      activeVariant: result.active,
      reasoning: local?.reasoning || undefined,
      thinkSecs: local?.thinkSecs || undefined,
      steps: local?.steps,
      error: undefined,
    })
  } catch (err) {
    last.error = t('cockpit.regenerateError', { err: String(err) })
  }
}

/**
 * Replace the last question with a corrected one and answer it fresh.
 *
 * The old exchange is deleted rather than kept as a variant: two answers to two
 * different questions are not alternatives to each other.
 */
export async function resendEdited(text: string, revertFiles: boolean): Promise<void> {
  const trimmed = text.trim()
  if (!trimmed || cockpit.awaitingReply || cockpit.chat.length < 2 || peekIsReadOnly()) return
  const question = cockpit.chat[cockpit.chat.length - 2]
  if (question.role !== 'user') return
  // The attachment goes with the question, not with its wording: fixing a typo
  // must not detach the file the question is about.
  const sent = (trimmed + (question.attachSuffix ?? '')).trim()
  const previous = cockpit.chat.splice(cockpit.chat.length - 2, 2)
  cockpit.chat.push({ ...previous[0], text: trimmed, time: nowLabel() })
  await runLiveTurn(async () => {
    try {
      const reply = await ResendEdited(sent, revertFiles)
      cockpit.chat.push({
        role: 'agent', text: reply.text, parts: reply.parts as TurnPart[] | undefined,
        time: nowLabel(), id: reply.messageId || undefined, ...turnArtifacts(),
      })
    } catch (err) {
      cockpit.chat.push(turnEndedBubble(err, sent))
    }
  })
}

/** Abort the turn in flight — the engine's tool loop is unbounded, this is the user's brake. */
export function cancelTurn(): void {
  clearQueuedMessages()
  CancelTurn()
}

/** ask_user tool: the model is blocked waiting for the user to pick an option. */
export function applyAskUser(payload: { question: string; options: string[] }): void {
  cockpit.ask = payload
}

export function applyAskDone(): void {
  cockpit.ask = null
}

/** Deliver the user's choice (an option click or free text) to the blocked tool. */
export function answerAsk(answer: string): void {
  if (!answer.trim()) return
  cockpit.ask = null
  AnswerUserQuestion(answer)
}

/** todo_write tool: the model replaced its task checklist. */
export function applyTodos(todos: CockpitState['todos']): void {
  cockpit.todos = Array.isArray(todos) ? todos : []
}

/** The user putting down a plan that is not going to be finished.
 *
 * The engine has no say here, and that is the point: this list is only ever
 * written by the model, and once it outlives the turn that wrote it, the only
 * person who can know it has been abandoned is the one who abandoned it. A
 * finished plan clears itself at the next turn (runLiveTurn); this is for the
 * other case — the work changed direction and the checklist is now about a
 * question nobody is asking any more. */
export function clearPlan(): void {
  cockpit.todos = []
}

/** suggest_task tool: the agent's pending side-work chips, replaced wholesale. */
export function applyTaskChips(chips: CockpitState['taskChips']): void {
  cockpit.taskChips = Array.isArray(chips) ? chips : []
}

/** Chips suggested before this view mounted — fetch what the backend holds. */
export async function refreshTaskChips(): Promise<void> {
  try {
    applyTaskChips((await ListTaskChips()) as CockpitState['taskChips'])
  } catch {
    // Engine not ready yet — the tasks:changed event will bring them later.
  }
}

/** How many proposals are waiting for the user to decide on them.
 *
 * The whole learning design rests on nothing taking effect without approval,
 * which only holds up if the user knows there is something to approve. This is
 * the number behind that mark. */
export function applyPendingLearned(count: number): void {
  cockpit.pendingLearned = Number(count) || 0
}

/** The queue as it stands before any proposal arrives in this run — anything
 * left waiting from a previous session is still waiting. */
export async function refreshPendingLearned(): Promise<void> {
  try {
    applyPendingLearned(await PendingLearnedCount())
  } catch {
    // Engine not ready yet — learning:changed will bring it later.
  }
}

/** The other queue: failures that keep happening, waiting to be reported.
 *
 * Fetched rather than carried on the event. `learning:changed` fires for both
 * kinds and its payload is the lessons count — one number, one meaning. The
 * alternative was an object two listeners would have to keep agreeing about,
 * for a count cheap enough to just ask for. */
export async function refreshPendingIssues(): Promise<void> {
  try {
    cockpit.pendingIssues = Number(await PendingIssueCount()) || 0
  } catch {
    // Engine not ready yet — learning:changed will bring it later.
  }
}

/** Take one repeated failure to the assistant instead of to the developer.
 *
 * The third door on the problems page, and the one the other two could not
 * open: a failure may be Aetox's fault, this machine's, or the agent's own way
 * of calling a tool, and the user is asked to judge that with nothing to go on.
 * The assistant can actually look.
 *
 * Sent with sendUserMessage, so the problem lands in the chat as the user's own
 * visible message rather than as a hidden instruction — owner's requirement,
 * and the honest shape anyway: a question you cannot see was never asked on
 * your behalf.
 *
 * Same four steps as startTaskChip below, for the same reasons written there. */
export async function consultAboutIssue(prompt: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  await newSessionHere()
  await sendUserMessage(prompt)
}

/** Start a suggested task: consume the chip, then run its prompt in a fresh
 *  session. The prompt was written to stand alone (suggest_task requires it),
 *  so the new session needs nothing from the one that suggested it. */
export async function startTaskChip(chip: CockpitState['taskChips'][number]): Promise<void> {
  // Guarded here, not just inside newSessionHere: with the chip consumed and
  // the new session refused, sendUserMessage would hand the chip's prompt to
  // the turn already running — as an interjection into a conversation it was
  // never about.
  if (turnStillRunning()) return
  dropPeek()
  await DismissTaskChip(chip.id)
  await newSessionHere()
  await sendUserMessage(chip.prompt)
}

export async function dismissTaskChip(id: string): Promise<void> {
  await DismissTaskChip(id)
}

/** Live turn-progress text from the Go engine (see desktop/app.go emitAgentStatus). */
export function applyAgentStatus(status: string): void {
  cockpit.agentStatus = status
}

/**
 * Live answer text from the Go engine (desktop/app.go emitChatChunk).
 *
 * One event carries three things, told apart by `replace`:
 *   { text, replace: false } — a fragment the model just wrote, append it
 *   { text: '', replace: true } — that round was not the answer, erase it
 *   { text: reply, replace: true } — the finished reply, the authority
 *
 * The last one replacing rather than appending is what makes doubling
 * impossible: however much streamed first, the delivery overwrites it. A bare
 * string is still accepted so a frontend reload against an older engine build
 * degrades to the previous append-only behaviour instead of rendering [object].
 */
export function applyAgentChunk(payload: string | { text: string; replace: boolean }): void {
  if (typeof payload === 'string') {
    cockpit.streamingText += payload
    return
  }
  if (payload.replace) cockpit.streamingText = payload.text
  else cockpit.streamingText += payload.text
}

// First/last reasoning-chunk timestamps this turn, for the "thought for Xs" label.
let thinkStartedAt = 0
let thinkLastAt = 0

/** Live reasoning/thinking text from the Go engine (see desktop/app.go
 * SendMessage's onReasoningChunk) — only fires for providers that stream
 * reasoning tokens (DeepSeek, Anthropic extended thinking, ...); '' means
 * either idle or this provider/turn had none to show. */
export function applyReasoningChunk(chunk: string): void {
  const now = Date.now()
  if (!cockpit.reasoningText) thinkStartedAt = now
  thinkLastAt = now
  cockpit.reasoningText += chunk
}

/**
 * The tray's data: the engine's own register of delegations, asked directly.
 *
 * Polled, not event-assembled. A `task` tool call completes the instant the
 * handle comes back — the moment the work STARTS — so the event stream shows
 * every delegation as finished from birth; only the register in Go knows
 * running from waiting from done. The poll is armed by the things that change
 * the register (a routed background event, a turn ending) and re-arms itself
 * only while something is still running, so an idle session polls zero times.
 */
let bgPollTimer: ReturnType<typeof setTimeout> | null = null
// Finished tasks this window has already sent a collect-turn for, so a slow
// model turn cannot be asked twice about the same result. Session-scoped like
// the lists it guards; cleared with them in selectSession.
const autoCollected = new Set<string>()
export function resetBackgroundWork(): void {
  cockpit.backgroundSteps = []
  cockpit.backgroundTasks = []
  cockpit.backgroundRuns = []
  autoCollected.clear()
  // The timer goes with them. It is armed for the session whose delegations it
  // was watching, and leaving it running would refill the list this just
  // emptied — with the previous session's work, under the new session's chat.
  if (bgPollTimer) clearTimeout(bgPollTimer)
  bgPollTimer = null
}
export async function refreshBackgroundTasks(): Promise<void> {
  try {
    // Both in one pass, so a run and its rows are never a poll apart: read
    // separately, a phase could show 3 done above three rows that are still
    // running, and the card would be arguing with itself.
    const [tasks, runs] = await Promise.all([BackgroundTasks(), BackgroundRuns()])
    cockpit.backgroundTasks = tasks
    cockpit.backgroundRuns = runs
  } catch {
    // The engine may be mid-bootstrap; the next trigger tries again.
  }
  autoCollectFinished()
  const active = cockpit.backgroundTasks.some((t) => t.state === 'running' || t.state === 'waiting')
  if (bgPollTimer) clearTimeout(bgPollTimer)
  bgPollTimer = active ? setTimeout(refreshBackgroundTasks, 2000) : null
}

/**
 * A finished background task does not wait to be noticed (owner, 14 ส.ค.:
 * "มันควรจะรู้ตัวเองสิครับ ว่าถ้ามันเสร็จ"). The moment the poll sees one done
 * and uncollected, a "[ระบบ]" message goes into the chat telling the model to
 * collect and report — the same door the tray's เก็บผล button and the user's
 * own typing go through, so there is still exactly one path into the engine.
 *
 * Only while the chat is idle: a running turn either collects it itself or, if
 * it forgets, the refresh at that turn's end lands here anyway. One task per
 * refresh, because the collect-turn's own end triggers the next refresh — a
 * backlog drains itself one report at a time instead of racing.
 *
 * A question (state 'waiting') is deliberately NOT auto-answered: it needs the
 * user, and the tray is already showing it.
 */
function autoCollectFinished(): void {
  if (cockpit.awaitingReply) return
  const ready = cockpit.backgroundTasks.find(
    (t) => (t.state === 'done' || t.state === 'failed') && !t.collected && !autoCollected.has(t.id),
  )
  if (!ready) return
  autoCollected.add(ready.id)
  void sendUserMessage(t('chat.bgFinishedPrompt', { id: ready.id, agent: ready.agent }))
}

/** Which list an event's row belongs in.
 *
 * A sub-agent outlives the turn that started it (internal/subagent/runner.go),
 * so its steps keep arriving after the live block is gone — and toolSteps is
 * cleared at both ends of a turn, so they would reappear inside the NEXT turn's
 * timeline, drawn as work the user's new question caused. That is the same
 * shape as the stale-checklist bug fixed in runLiveTurn, and it is worse here:
 * these rows are real work, not a leftover.
 *
 * The test is the delegation they belong to. Every event from inside a
 * sub-agent carries the `task` call's ref as `parent`; if that row is not in
 * this turn's list, the delegation started in an earlier one and everything it
 * does now is background. An event with no parent is always the main agent's,
 * and the main agent only ever works inside a turn. */
function listFor(ev: ToolEvent): ToolStep[] {
  if (!ev.parent) return cockpit.toolSteps
  const inThisTurn = cockpit.toolSteps.some((s) => s.ref === ev.parent)
  return inThisTurn ? cockpit.toolSteps : cockpit.backgroundSteps
}

/** Give a delegation's own row the id its work is registered under.
 *
 * Only the events from *inside* a sub-agent carry that id: the `task` call that
 * opened the delegation completes before the register has a handle to hand back,
 * so the row is born without one and learns it from the first step its worker
 * runs. Nothing else can supply it — `parent` is the provider's call id, a
 * different namespace entirely.
 *
 * It is worth the join because the row cannot answer "is this still going" on
 * its own: `task` returns the moment the work starts, so by the row's own
 * account every delegation finished a second after it began. With the id, the
 * card asks the register instead (Chat.svelte cardState/cardSecs) — the same
 * question the tray has always asked (§105).
 *
 * The row may have left the live list already: a delegate outlives its turn, so
 * its first step can arrive after the transcript took the row. Same objects
 * either way, so stamping the one in the message updates what is on screen.
 */
function joinDelegationToRegister(ev: ToolEvent): void {
  if (!ev.parent || !ev.task) return
  // A delegation running inside this turn is a delegation the register knows
  // about, and until this the poll was armed only by a *background* event or by
  // the turn ending — so for the whole time a delegate worked inside its turn,
  // the list its card reads was empty and the card fell back to the row it was
  // written to ignore. The clock froze exactly where it had before.
  if (!bgPollTimer) void refreshBackgroundTasks()
  const live = cockpit.toolSteps.find((s) => s.ref === ev.parent)
  if (live) {
    live.task ||= ev.task
    return
  }
  for (let i = cockpit.chat.length - 1; i >= 0; i--) {
    const row = cockpit.chat[i].steps?.find((s) => s.ref === ev.parent)
    if (row) {
      row.task ||= ev.task
      return
    }
  }
}

/** Live tool call/result feed from the Go engine (turn.ToolEvent, relayed by
 * desktop/app.go recordToolAction). "call" opens a running step; "result"
 * closes the oldest one still running. */
export function applyToolEvent(ev: ToolEvent): void {
  const steps = listFor(ev)
  // Background work just made a sound — make sure the tray is listening. The
  // poll re-arms itself while anything runs, so this only matters as the
  // starter (first event after a reload, say); an armed timer means it is
  // already being watched and one more Go call would be noise.
  if (steps === cockpit.backgroundSteps && !bgPollTimer) void refreshBackgroundTasks()
  joinDelegationToRegister(ev)
  // The loop's own story, interleaved between the calls it explains (§59).
  // Both land as finished rows: there is nothing running to close later.
  if (ev.action === 'note') {
    const text = ev.text?.trim()
    if (text) {
      steps.push({
        kind: 'note', label: text, parent: ev.parent || undefined,
        state: 'done', startedAt: Date.now(),
      })
    }
    return
  }
  // A finished answer the user typed over. The engine has already erased the
  // live preview by the time this arrives (OnContentReset runs first), so this
  // row is the only copy left — and it is prose, drawn as the markdown it was
  // written as rather than as a narration line.
  if (ev.action === 'said') {
    const text = ev.text?.trim()
    if (text) {
      steps.push({
        kind: 'said', label: text, parent: ev.parent || undefined,
        state: 'done', startedAt: Date.now(),
      })
    }
    return
  }
  if (ev.action === 'thinking') {
    steps.push({
      kind: 'thinking', label: '', parent: ev.parent || undefined,
      state: 'done', startedAt: Date.now(), secs: Math.max(1, ev.secs ?? 1),
    })
    return
  }
  const label = [ev.name, ev.subject].filter(Boolean).join(' ')
  // A row is recognized by the engine's call id, not by its label. The label is
  // incomplete on the early events — a model may stream a write's content long
  // before its path — so matching on it drew a second row the moment the name
  // arrived. Falls back to the label for engines that send no id.
  // A sub-agent's rows are matched within their own scope: two delegates (or a
  // delegate and the main agent) can be running `grep` at the same moment, and
  // without the parent in the key one would claim the other's row.
  const running = (s: ToolStep) =>
    s.state === 'run' && (s.parent ?? '') === (ev.parent ?? '') &&
    (ev.ref && s.ref ? s.ref === ev.ref : s.label === label)
  if (ev.action === 'call') {
    // A call is announced repeatedly while the model writes it — once the tool
    // name is known, then as the content streams — and once more when it
    // actually runs. The row is reused: the counter climbs and the elapsed
    // clock keeps running instead of the timeline growing a row per line.
    const open = steps.find(running)
    if (open) {
      if (ev.added) open.added = ev.added
      // Let the row name itself once the subject shows up.
      if (ev.subject) open.label = label
      // Same rule for a delegation's facts. The row is usually born from the
      // streaming progress event, which fires while the model is still writing
      // the call's arguments — agent, brief and kind are unknowable then and
      // arrive only on the executor's own call event, after this row exists.
      // Dropping them here is how a doc job got counted as "ซับเอเจน 1
      // ตัว" on a live turn while the written-down parts said agent/doc.
      if (ev.agent) open.agent = ev.agent
      if (ev.brief) open.brief = ev.brief
      if (ev.agentKind) open.agentKind = ev.agentKind
      // Taken on false as well as true, unlike the three above: `false` is the
      // whole point of this one. A `task collect` says nothing else about
      // itself, and dropping its no would leave the row on the label guess it
      // was sent to overrule.
      if (ev.delegation !== undefined) open.delegation = ev.delegation
      return
    }
    steps.push({
      label, ref: ev.ref, parent: ev.parent || undefined, task: ev.task || undefined,
      // Only a `task` call carries these, and they arrive on the first event —
      // the delegation is named before its first delegate step shows up.
      agent: ev.agent || undefined, brief: ev.brief || undefined,
      agentKind: ev.agentKind || undefined, delegation: ev.delegation,
      state: 'run', startedAt: Date.now(), added: ev.added || undefined,
    })
    return
  }
  if (ev.action !== 'result') return
  const step = steps.find(running) ?? steps.find((s) => s.state === 'run')
  if (!step) return
  if (ev.subject) step.label = label
  step.state = ev.ok ? 'done' : 'err'
  step.secs = Math.round((Date.now() - step.startedAt) / 1000)
  step.error = ev.ok ? undefined : ev.error
  // Only a write/edit carries these; everything else reports 0 and shows nothing.
  step.added = ev.added || undefined
  step.removed = ev.removed || undefined
  // A finished file the user asked for. Deduped because a turn may well write
  // the same workbook twice — a first pass and a correction — and the answer
  // should offer it once.
  for (const path of ev.artifacts ?? []) {
    if (path && !cockpit.turnFiles.includes(path)) cockpit.turnFiles.push(path)
  }
  // What the turn asked to remember. Deduped like the files above: asking twice
  // in one turn answers with the id already waiting (desktop/pending.go treats
  // the second attempt as a duplicate), and one proposal deserves one card.
  if (ev.proposalId && !cockpit.turnProposals.includes(ev.proposalId)) {
    cockpit.turnProposals.push(ev.proposalId)
  }
}

/** Copy an image (from a native file-picker or a drop) into the sandbox, and stage it as the composer's pending attachment. */
export async function attachImageFromPath(absPath: string): Promise<void> {
  try {
    const relPath = await SaveChatImage(absPath)
    const dataUrl = await ReadImageDataURL(relPath)
    cockpit.pendingImage = { relPath, dataUrl }
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
  }
}

/** Stage an image pasted into the composer — a screenshot, or a chart copied
 *  out of an answer with the drawing's own คัดลอก button.
 *
 * The picker and the drop path both hand SaveChatImage a real OS path, which is
 * a route a clipboard image simply does not have: it is bytes and nothing else.
 * So the bytes go over as a data URL and the engine writes them into the same
 * per-session attachment folder a picked file lands in — from there on it is an
 * ordinary attachment, readable by every skill, on the same relative path. */
export async function attachImageFromClipboard(file: File): Promise<void> {
  try {
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as string)
      reader.onerror = () => reject(new Error('อ่านรูปจากคลิปบอร์ดไม่ได้'))
      reader.readAsDataURL(file)
    })
    const relPath = await SaveChatImageData(dataUrl)
    cockpit.pendingImage = { relPath, dataUrl: await ReadImageDataURL(relPath) }
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
  }
}

export function clearPendingImage(): void {
  cockpit.pendingImage = null
}

const AUDIO_EXT = ['mp3', 'wav', 'm4a', 'flac', 'ogg', 'aac', 'wma']
const VIDEO_EXT = ['mp4', 'mov', 'mkv', 'webm', 'avi', 'wmv', 'flv']
export const IMAGE_EXT = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp']

export function fileKind(path: string): 'image' | 'audio' | 'video' | 'file' {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (IMAGE_EXT.includes(ext)) return 'image'
  if (AUDIO_EXT.includes(ext)) return 'audio'
  if (VIDEO_EXT.includes(ext)) return 'video'
  return 'file'
}

/** Attach any non-image file: copied into the sandbox, then handed to the model
 * as a path. A 300MB clip cannot be inlined into a prompt — and should not be:
 * the tools exist precisely to open it. */
export async function attachFileFromPath(absPath: string): Promise<void> {
  try {
    const relPath = await SaveChatFile(absPath)
    const kind = fileKind(absPath)
    cockpit.pendingFile = {
      relPath,
      label: absPath.split(/[\\/]/).pop() ?? relPath,
      kind: kind === 'image' ? 'file' : kind,
    }
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
  }
}

export function clearPendingFile(): void {
  cockpit.pendingFile = null
}


/** Stage a dragged-in workbench tab (file or browser) as the composer's pending
 * context — read fresh from disk/page rather than trusting any stale in-memory
 * copy, so the model sees what's there now. */
export async function attachTabContext(kind: 'file' | 'browser', ref: string, label: string): Promise<void> {
  // A browser tab only has a native window once it has loaded a URL, so a tab
  // still showing its start page has no text to read. Asking anyway came back
  // as `no browser tab "web-2"` — an internal id, in a sentence about
  // attaching an image, for something the user cannot act on.
  if (kind === 'browser') {
    // A browser tab only has a native window once it has loaded a URL, so a tab
    // still showing its start page has no text to read. Asking anyway came back
    // as `no browser tab "web-2"` — an internal id, in a sentence about
    // attaching an image, for something the user cannot act on.
    if (!workbench.tabs.find((t) => t.id === ref)?.url) {
      cockpit.chat.push({ role: 'agent', text: t('cockpit.attachEmptyPage'), time: nowLabel() })
      return
    }
    try {
      cockpit.pendingContext = { kind, label, content: await BrowserGetText(ref) }
    } catch (err) {
      cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
    }
    return
  }

  // A picture goes in as a picture, with its thumbnail in the chip — same as
  // one attached with the paperclip. Already inside the sandbox, so unlike
  // attachImageFromPath there is nothing to copy.
  if (fileKind(ref) === 'image') {
    try {
      cockpit.pendingImage = { relPath: ref, dataUrl: await ReadImageDataURL(ref) }
      return
    } catch {
      // Not really an image, or too big to inline — fall through and hand over
      // the path instead.
    }
  }

  try {
    cockpit.pendingContext = { kind, label, content: await ReadFile(ref) }
  } catch (err) {
    // Exactly two of ReadFile's refusals mean "the file is fine, inlining it is
    // not": binary (a PDF, a workbook, a clip) and too large. Those go in as a
    // path plus the tool that opens it, which is what the paperclip already
    // does for the same files — dragging a PDF onto the composer used to end at
    // the raw Go error instead.
    //
    // The other five — no project open, outside the sandbox, not found, is a
    // directory, unreadable — mean the attach genuinely failed, and a catch
    // that swallowed all seven staged a dead path the model was then told to
    // read. That costs a turn and reads as the model's fault. So this matches
    // the two by name and reports everything else; an unrecognised message is
    // reported, never assumed benign.
    const msg = String(err)
    if (!/binary file cannot be previewed|too large/i.test(msg)) {
      cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: msg }), time: nowLabel() })
      return
    }
    const fk = fileKind(ref)
    cockpit.pendingFile = { relPath: ref, label, kind: fk === 'image' ? 'file' : fk }
  }
}

export function clearPendingContext(): void {
  cockpit.pendingContext = null
}

/** View state: expand/collapse a folder. */
export function toggleNode(node: TreeNode): void {
  if (node.kind === 'dir') node.open = !node.open
}

/** Flat tree + depth → rows hidden under a collapsed folder. Shared by every tree view. */
export function visibleTree(tree: TreeNode[]): TreeNode[] {
  const out: TreeNode[] = []
  let collapseDepth = Infinity
  for (const n of tree) {
    if (n.depth > collapseDepth) continue
    collapseDepth = Infinity
    out.push(n)
    if (n.kind === 'dir' && !n.open) collapseDepth = n.depth
  }
  return out
}

/** Open a file tab (fetching its content once), or just switch to it if already open. */
export async function openFile(path: string): Promise<void> {
  if (!cockpit.openFiles.some((f) => f.path === path)) {
    try {
      const content = await ReadFile(path)
      cockpit.openFiles.push({ path, content })
    } catch (err) {
      cockpit.openFiles.push({ path, content: t('workbench.openFileError', { err: String(err) }) })
    }
  }
  cockpit.activeView = path
}

/** Close a file tab; falls back to Chat (or another open file) if it was active. */
export function closeFile(path: string): void {
  const idx = cockpit.openFiles.findIndex((f) => f.path === path)
  if (idx === -1) return
  cockpit.openFiles.splice(idx, 1)
  if (cockpit.activeView !== path) return
  cockpit.activeView = cockpit.openFiles.at(-1)?.path ?? 'chat'
}

/** Where the open room survives an F5. Exported for the same reason
 *  SETTINGS_SECTION_KEY below is: firstRun.ts has to clear it, and a key spelled
 *  twice fails silently — the reset would look like it worked and the window
 *  would come back on the page the button was pressed from. */
export const activeViewStorageKey = 'aetox.activeView'

// The views a reload may land back on. One list read by both halves: they used
// to spell the same set out twice, and each new room had to be remembered in
// two places — โปรเจกต์ was missed when it opened, and ระบบออโตเมชั่น the day
// after. File tabs are deliberately absent: they do not persist, so a stored
// path would point at nothing.
const RESTORABLE_VIEWS = ['chat', 'settings', 'office', 'artifacts', 'projects']

/** The rooms that draw over the whole window, as opposed to `chat`, which is
 *  the layout underneath them (App.svelte renders each as a .settings-overlay).
 *
 *  Derived from the list above rather than written out again, because that
 *  comment's lesson applies here twice over: a room added to the set has to
 *  become an overlay everywhere at once, and the place that forgets is not the
 *  one that shows a blank page — it is the native browser window.
 *
 *  A workbench browser tab is a real OS window (desktop/browser_windows.go). It
 *  composites *above* the app's own webview whatever the DOM does, so it has to
 *  be told to hide whenever something is drawn over its pane; a z-index cannot
 *  reach it. BrowserPane knew about `settings` and only `settings`, so opening
 *  ทีมเอเจน, ผลงาน or โปรเจกต์ with a page loaded left that page floating on top
 *  of the room (owner, 2026-08-14: "ทำไมมีหลุดมาอ่ะครับ"). */
export const isOverlayView = (view: string): boolean =>
  view !== 'chat' && RESTORABLE_VIEWS.includes(view)

export function setActiveView(view: string): void {
  cockpit.activeView = view
  // Survive an F5 *within this run* only: remember chat/settings (file tabs
  // don't persist, so a stored file path would point at nothing after
  // reload). sessionStorage, not localStorage — a real app relaunch must
  // always land on chat, never reopen straight into Settings because that's
  // where a previous session happened to be force-quit.
  if (RESTORABLE_VIEWS.includes(view)) {
    try {
      sessionStorage.setItem(activeViewStorageKey, view)
    } catch {
      /* storage unavailable — view just won't persist */
    }
  }
}

/** Where the Settings page remembers which of its sections is open.
 *
 * Exported so that a room can send the user to the right page rather than to
 * the top of Settings with directions. Settings.svelte reads this same constant
 * — the key is written down once, because a second spelling of it would fail
 * silently and look like the page simply ignoring where it was told to go. */
export const SETTINGS_SECTION_KEY = 'aetox.settingsSection'

/** Open Settings already showing one section.
 *
 * ระบบออโตเมชั่น needs it: connecting an engine is a Settings job (one register,
 * one form, one place a token is typed), but the room is where the user is
 * standing when they decide to. Handing them a page they have to search is the
 * kind of small rudeness that makes people give up. */
export function openSettingsAt(section: string): void {
  try {
    sessionStorage.setItem(SETTINGS_SECTION_KEY, section)
  } catch {
    /* storage unavailable — Settings opens where it last was */
  }
  setActiveView('settings')
}

/** Restore the last room after a frontend reload (same run only). */
export function restoreActiveView(): void {
  try {
    const saved = sessionStorage.getItem(activeViewStorageKey)
    if (saved && RESTORABLE_VIEWS.includes(saved)) {
      cockpit.activeView = saved
    }
  } catch {
    /* storage unavailable */
  }
}

/** Switch to a stored session — the transcript loads back and the agent's memory is restored. */
export async function selectSession(session: Session): Promise<void> {
  if (cockpit.awaitingReply) return peekAtSession(session)
  dropPeek()
  const messages = await LoadSession(session.id)
  cockpit.todos = []
  cockpit.ask = null
  // Background work is the session's (§105), and opening another session
  // re-bootstraps the engine, which stops it — the register these mirror is
  // about to be replaced, so showing yesterday's rows over it would be a lie.
  resetBackgroundWork()
  cockpit.chat = restoreTranscript(messages)
  hydrateImages()
  // Opening a session takes the engine back to the desk it was held at, so the
  // nav has to follow it rather than keep pointing at where the user was.
  await refreshDesk()
  await switchWorkbenchSession(session.id)
  await refreshSessions()
  await refreshGlobalHistory()
}

/** Record what the user thought of one reply.
 *
 * Pressing the thumb that is already lit withdraws the rating rather than
 * doing nothing: a verdict you cannot take back is one people stop giving.
 *
 * The bubble is updated first and the call is not awaited for the redraw —
 * this is a one-click gesture on a row of small buttons, and a round-trip's
 * worth of nothing happening is how a user concludes it did not register. */
export async function rateReply(message: ChatMessage, verdict: 'good' | 'bad'): Promise<void> {
  if (!message.id) return
  const next = message.rating === verdict ? 'unknown' : verdict
  message.rating = next
  try {
    await RateTurn(message.id, next)
  } catch {
    // A rating is not worth an error bubble in the conversation. Put the
    // button back where it was so the UI keeps telling the truth about what
    // is stored.
    message.rating = message.rating === next ? undefined : message.rating
  }
}

/** Export one conversation to a file the user picks — 'markdown' to read,
 * 'json' to re-import on any Aetox. Engine-side dialog; a cancel returns "". */
export async function exportChat(session: Session, format: 'markdown' | 'json'): Promise<void> {
  await ExportSession(session.id, format)
}

/** Import a conversation exported from any Aetox, then open it. */
export async function importChat(): Promise<void> {
  const id = await ImportSession()
  if (!id) return // dialog closed — a decision, not a failure
  await selectGlobalSession({ id } as Session)
}

/** Permanently delete a session (any project); clears the chat if it was the open one. */
export async function deleteSession(session: Session): Promise<void> {
  // Only the open chat is off-limits mid-turn — it is the one the turn is
  // writing into. The rest of the history stays deletable; a long turn must
  // not freeze the whole list.
  if (session.active && turnStillRunning()) return
  await DeleteSession(session.id)
  removeWorkbenchState(session.id)
  if (session.active) cockpit.chat = []
  await refreshSessions()
  await refreshGlobalHistory()
}

/** The engine refused to open/switch a session. Its refusals are written
 * sentences (busy, unknown desk, deleted chair file); an unhandled rejection
 * here used to swallow every one, leaving a click that visibly did nothing. */
function showSessionRefusal(err: unknown): void {
  cockpit.sessionError = err instanceof Error ? err.message : String(err)
}

/** "New chat" — the + button, Ctrl+N, and the palette's row.
 *
 * It lands on the main desk of the door you are standing at: ผู้ช่วย from the
 * storefront, โต๊ะโค้ด from the workshop. It does NOT keep the room you were
 * in, and that is the whole point of the change (owner, 14 ส.ค.).
 *
 * It used to call the engine's bare NewSession, which leaves desk and chair
 * exactly as they were — so pressing it inside ระบบออโตเมชั่น handed you a
 * blank chat still seated with the automation specialist, in a room you were
 * trying to leave. A control at a fixed address that does a different thing in
 * every room is not one control, it is five with one label.
 *
 * Scoped to the door rather than always ผู้ช่วย because the doors are the one
 * boundary a keystroke has no business crossing: mid-task in the workshop,
 * Ctrl+N must not put you in the storefront.
 *
 * A new chat is also in no project — the engine has always done that
 * (startNewSession clears a.space); going through newSessionAt is what finally
 * makes the window say the same thing instead of drawing a project the session
 * had already left. */
export async function newSession(): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  // Before the session call, like openDesk: a refusal is reported inside the
  // chat, so a user who pressed this from Settings has to be looking at it.
  setActiveView('chat')
  await newSessionAt(deskForShell(shell.name))
}

/** Start a blank session without moving: same desk, same chair, same door.
 *
 * The one caller is a task chip, and it is not a "new chat" — it is a piece of
 * work the agent in this room noticed while doing something else. A stale doc
 * the workshop grepped past is the workshop's job; carrying it out to ผู้ช่วย
 * because the chip happened to be clicked would hand it to someone who was
 * never in the conversation. */
async function newSessionHere(): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  try {
    await NewSession()
  } catch (err) {
    showSessionRefusal(err)
    return
  }
  cockpit.sessionError = ''
  await afterNewSession()
}

/** Start a blank session at one desk (COMPANY.md §2 — the five buttons).
 *
 * A desk cannot be changed inside a session, so this is the only way onto one:
 * opening a desk opens a session there. It costs nothing — every turn is
 * already persisted, so the conversation being left is in the history list
 * before the click, not lost by it. */
export async function newSessionAt(desk: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  try {
    await NewSessionAt(desk)
  } catch (err) {
    // Refused — nothing below may run: the desk/shell repaint would dress the
    // window as a place the engine never went.
    showSessionRefusal(err)
    return
  }
  cockpit.sessionError = ''
  cockpit.desk = desk
  cockpit.chair = ''
  // A chat opened from the nav is in no project — the engine says the same
  // thing on its side (startNewSession), and the two have to agree or the
  // window keeps drawing a room the session is no longer in.
  cockpit.space = ''
  setShell(shellForDesk(desk))
  await afterNewSession()
}

/** The open chat's project's own chats, for the sidebar column.
 *
 * A chat inside a project runs at the assistant's desk, so the general history
 * — which now drops project chats on purpose — cannot show it, and standing
 * inside one used to mean a column of unrelated conversations with the chat you
 * were in nowhere on it. Empty whenever the chat is in no project, which is
 * what puts the general list back. */
export async function refreshSpaceHistory(): Promise<void> {
  if (!cockpit.space) {
    cockpit.spaceHistory = []
    return
  }
  const [metas, current] = await Promise.all([SessionsInSpace(cockpit.space), CurrentSessionID()])
  cockpit.spaceHistory = (metas ?? []).map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), updatedAt: m.updatedAt,
    active: m.id === current, mode: m.mode, agent: m.agent,
  }))
}

/** Start a chat inside a โปรเจกต์ (COMPANY.md §84).
 *
 * A session, not a view change. Calling the binding without this was the whole
 * bug: the engine opened a new session inside the project while the window went
 * on showing the session that was already there, so the chat the click had just
 * created was unreachable and the project it belonged to looked like it had
 * vanished. Every other door onto a session goes through one of these functions
 * for exactly this reason — the engine's session and the window's session are
 * two facts, and only these keep them one. */
export async function newSpaceSession(space: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  try {
    await NewSessionInSpace(space)
  } catch (err) {
    showSessionRefusal(err)
    return
  }
  cockpit.sessionError = ''
  // A project chat is an assistant chat that happens to be filed somewhere:
  // the desk is the assistant's, and no agent sits in it.
  cockpit.desk = 'assistant'
  cockpit.chair = ''
  cockpit.space = space
  setShell('assistant')
  await afterNewSession()
}

/** Open a direct chat with one of the office's agents (§85). The desk is
 *  implied — a chair only exists in the office — and the engine refuses a
 *  name that is not an office agent, so a stale card cannot open a chat as
 *  somebody else. */
export async function newChairSession(chair: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  try {
    await NewChairSession(chair)
  } catch (err) {
    showSessionRefusal(err)
    return
  }
  cockpit.sessionError = ''
  cockpit.desk = 'specialized'
  cockpit.chair = chair
  cockpit.space = '' // for the same reason newSessionAt clears it
  setShell('assistant') // the office is behind the storefront door (§86)
  await afterNewSession()
}

async function afterNewSession(): Promise<void> {
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  // Explicit switch (not adopt): a brand-new session starts with an empty
  // workbench; the old session's layout stays saved for when it's reopened.
  await switchWorkbenchSession(await CurrentSessionID())
  await refreshSessions()
  await refreshGlobalHistory()
}

/** Walk through the other door (§86): remember it, and land on its desk.
 *
 * The doors are UI shells, so switching one costs a render — but the desk
 * behind it is engine state, and a session is born at a desk and never moves.
 * So arriving at a door whose desk you are not already at opens a session
 * there, exactly as clicking that desk's button would. Arriving at the door
 * you are already behind does nothing at all. */
export async function switchShell(name: ShellName): Promise<void> {
  const def = SHELLS.find((s) => s.name === name)
  if (!def || shell.name === name) return
  // Before setShell, or the door's chrome would switch around a chat that
  // refused to follow it.
  if (turnStillRunning()) return
  dropPeek()
  // Decided before the session switch, which refreshes the project list on the
  // way past: reading it afterwards would make this depend on what a call three
  // lines up happens to leave behind.
  const resume = name === 'code' && !cockpit.project.focused ? cockpit.projects[0]?.path : ''
  setShell(name)
  setActiveView('chat')
  if (cockpit.desk !== def.desk) await newSessionAt(def.desk)
  // The storefront does not focus a project — that is what §19 and §86 have
  // said all along, and the window was contradicting it: its chat still
  // carried the project picker, so walking through this door with a project
  // open left the assistant rooted in it while the sidebar showed
  // conversations. One engine has one root; the door has to actually set it,
  // not merely stop drawing the control (owner's call, 2026-08-06).
  //
  // Coming back the other way, a project is required rather than optional, so
  // the door re-opens the one you were last in. Without it every switch would
  // cost a trip to the sidebar to say what the window already knows — the
  // list is ordered by opened_at, so the first entry is that project.
  if (name === 'assistant') {
    if (cockpit.project.focused) await clearProjectFocus()
  } else if (resume) {
    await openProject(resume)
  }
}

/** Show a desk: its open session if this is already the desk in front of you,
 *  otherwise a new session on it.
 *
 * The difference matters more than it looks. Re-clicking the desk you are
 * already at has to be a no-op on the conversation — a nav button that threw
 * away what you were doing would make the whole row unusable — while clicking
 * a different one is exactly the "open a new session" that changing desks
 * means (COMPANY.md §2). */
export async function openDesk(desk: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  setActiveView('chat')
  // "Already here" is the desk AND the project together. A project chat runs at
  // the assistant's desk, so comparing desks alone said the user was already at
  // ผู้ช่วย while they were standing inside a project — and the button did
  // nothing, with no way back out through the nav. The third coordinate has to
  // be part of the comparison or it is not the same place.
  if (cockpit.desk === desk && !cockpit.space) return
  await newSessionAt(desk)
}

/** Read back which desk the engine's current session is at.
 *
 * Asked rather than remembered: the engine is the one that knows, a session's
 * desk is fixed for its whole life, and a UI that kept its own copy would be
 * the second answer that can disagree. */
export async function refreshDesk(): Promise<void> {
  try {
    const id = await CurrentSessionID()
    cockpit.desk = id ? await SessionMode(id) : ''
    cockpit.chair = id ? await SessionAgent(id) : ''
    // Asked, not remembered, for the same reason as the two above: reopening a
    // chat from history has to put its project back on screen, and the engine
    // is the one that read it off the row.
    cockpit.space = await CurrentSpace()
    // Asked too, and for a reason the three above do not have: a stance is not
    // fixed at birth, so reopening a chat has to put back the dial it was left
    // on. The engine read it off the row; nothing here could know it.
    cockpit.stance = await Stance()
    if (cockpit.stances.length === 0) cockpit.stances = await Stances()
    await refreshSpaceHistory()
    // The door follows the desk, never the other way round: reopening a coding
    // session from the assistant's history has to land you in the workshop, or
    // the window would be showing one desk's rooms around another desk's chat.
    setShell(shellForDesk(cockpit.desk))
  } catch {
    cockpit.desk = '' // engine not up yet — the full desk is the honest default
    cockpit.chair = ''
    cockpit.stance = '' // and ลงมือ is its counterpart: the stance that withholds nothing
  }
}

/** Turn the dial: how the open session runs from the next turn on (§106).
 *
 * The only session coordinate with a setter. The engine re-bootstraps in place
 * and carries the conversation over, so nothing here clears the transcript —
 * that is the difference between a stance and a desk, expressed as the absence
 * of the four lines newSessionAt needs.
 *
 * The engine's answer is what lands in state, never the value that was asked
 * for: a name this build does not implement comes back as ลงมือ, and the
 * picker has to show what is actually in force rather than what was clicked. */
export async function setStance(next: string): Promise<void> {
  if (turnStillRunning()) return
  dropPeek()
  try {
    cockpit.stance = await SetStance(next)
    cockpit.sessionError = ''
  } catch (err) {
    // Refused — a turn is in flight, and the guard is the same one that stops a
    // session switch mid-turn (desktop/app.go guardSessionSwitch), reached by a
    // different route: this rebuilds the agent under a turn that would then
    // finish on a context it did not start with. Same refusal, same surface,
    // and the dial stays where it was rather than showing a stance nothing is
    // enforcing.
    showSessionRefusal(err)
  }
}
