// The single source of truth for cockpit UI state. Reactive ($state); components
// read slices of it via props from App. Mutate its fields (the Go core can push
// incremental updates here — append a chat message, advance a timeline step) and
// the UI reacts. Do not reassign `cockpit` itself; mutate its properties.

import { emptyCockpitState, type CockpitState, type TreeNode, type ChangedFile, type Session, type ToolStep, type ToolEvent, type ChatMessage, type PendingFile } from '../types'
import type { CockpitSource } from '../services/cockpit'
import {
  SendMessage, GetProjectStatus, GetModelInfo, OpenProjectFolder, OpenProjectPath,
  SwitchProvider, SwitchThinkLevel, SwitchApprovalMode, SetProviderWireFormat,
  SwitchModel, SetAPIKey, ProjectTree, CommandHistory, GitChangedFiles, ReadFile,
  ListSessions, LoadSession, NewSession, CurrentSessionID, SearchSessions, DeleteSession,
  SaveChatImage, SaveChatFile, ReadImageDataURL, CancelTurn, BrowserGetText, RecentProjects,
  ListAllSessions, SearchAllSessions, LoadSessionAnyProject, ClearProjectFocus,
  AnswerUserQuestion, SetActiveAgent,
} from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { t } from '../i18n.svelte'
import { switchWorkbenchSession, adoptWorkbenchSession, removeWorkbenchState } from './workbench.svelte'

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
    agent: info.agent,
  })
  cacheModelInfo(cockpit.model)
}

/** Pull the real file tree / command history / git status the Go engine currently has. */
export async function refreshWorkspace(): Promise<void> {
  const [tree, commandHistory, changedFiles] = await Promise.all([
    ProjectTree(), CommandHistory(), GitChangedFiles(),
  ])
  // Go's generated bindings type these fields as plain `string`; the values
  // are always one of the frontend's narrower literals ("dir"/"file", "M"/"U"/"").
  cockpit.tree = tree as unknown as TreeNode[]
  cockpit.commandHistory = commandHistory
  cockpit.changedFiles = changedFiles as unknown as ChangedFile[]
}

function agoLabel(iso: string): string {
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
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), active: m.id === current,
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
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), active: m.id === current, snippet: m.snippet,
  }))
}

/** Pull chat history across every project, newest first (sidebar's global history layer). */
export async function refreshGlobalHistory(): Promise<void> {
  const [metas, current] = await Promise.all([ListAllSessions(), CurrentSessionID()])
  cockpit.history = metas.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), active: m.id === current, projectName: m.projectName,
  }))
}

/** Full-text search chat history across every project. */
export async function searchGlobalHistory(query: string): Promise<void> {
  if (!query.trim()) return refreshGlobalHistory()
  const [hits, current] = await Promise.all([SearchAllSessions(query), CurrentSessionID()])
  cockpit.history = hits.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), active: m.id === current,
    snippet: m.snippet, projectName: m.projectName,
  }))
}

// sendUserMessage folds attachments into the sent text as marker lines, because
// the model only ever reads text — and that text is exactly what the transcript
// stores. So restoring a session has to fold them back out, or the bubble shows
// a raw "[attachment: …] .aetox-attachments/x.mp4" line and no chip at all.
// These two patterns are the inverse of the ones written there; change both together.
const ATTACH_CTX_RE = /\n*\[attachment: [^\]]*\] ([^\n]*):\n```\n[\s\S]*?\n```/g
const ATTACH_FILE_RE = /\n*\[attachment: user-attached (image|audio|video|file) — [^\]]*\] (\S+)/g

function restoreAttachments(m: main.SessionMessage): ChatMessage {
  const out: ChatMessage = {
    role: m.role === 'agent' ? 'agent' : 'user',
    text: m.text,
    time: m.time,
    reasoning: m.reasoning || undefined,
    thinkSecs: m.thinkSecs || undefined,
  }
  if (out.role === 'agent') return out
  out.text = out.text
    .replace(ATTACH_CTX_RE, (_all, label: string) => { out.contextLabel = label; return '' })
    .replace(ATTACH_FILE_RE, (_all, kind: string, relPath: string) => {
      if (kind === 'image') out.imageRelPath = relPath
      else { out.attachKind = kind as PendingFile['kind']; out.attachLabel = relPath.split('/').pop() }
      return ''
    })
    .trim()
  return out
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

/** Open a session from the global history list — switches project first if it belongs to a different one. */
export async function selectGlobalSession(session: Session): Promise<void> {
  const messages = await LoadSessionAnyProject(session.id)
  cockpit.todos = []
  cockpit.ask = null
  cockpit.chat = messages.map(restoreAttachments)
  hydrateImages()
  await switchWorkbenchSession(session.id)
  const project = await GetProjectStatus()
  Object.assign(cockpit.project, project)
  await refreshWorkspace()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
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
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
  if (!modelInfo.provider && bootRetries < 8) {
    bootRetries += 1
    setTimeout(loadRealState, 1500)
  }
}

/** Let the user pick a real folder via the native dialog; re-points the engine at it. */
export async function openFolder(): Promise<void> {
  const project = await OpenProjectFolder()
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
}

/** Switch straight to a previously-opened project (sidebar's project list), no dialog. */
export async function openProject(path: string): Promise<void> {
  const project = await OpenProjectPath(path)
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
  await refreshSessions()
  await refreshProjects()
  await refreshGlobalHistory()
}

/** Drop project focus: the AI keeps full machine access (files/git/terminal)
 * but is no longer tied to any project — like opening Claude/Codex bare. */
export async function clearProjectFocus(): Promise<void> {
  const project = await ClearProjectFocus()
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
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

/** Switch which agent profile answers (§44). Same shape as every other switch:
 *  the binding returns the fresh ModelInfo, so the composer chip needs no
 *  second round-trip and cannot show a stale agent. */
export async function switchAgent(name: string): Promise<void> {
  applyModelInfo(await SetActiveAgent(name))
}

export async function submitAPIKey(providerName: string, apiKey: string): Promise<void> {
  applyModelInfo(await SetAPIKey(providerName, apiKey))
}

export async function switchWireFormat(format: string): Promise<void> {
  applyModelInfo(await SetProviderWireFormat(format))
}

function nowLabel(): string {
  return new Date().toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

// Typing while a turn is running is normal use, not a mistake — but the Go
// engine holds one conversation, so a second SendMessage fired into a live turn
// races the first and is simply lost. Those messages wait here instead and go
// out the instant the engine is free; the composer stays usable throughout.
export const queuedMessages = $state<string[]>([])

/** Drop anything still waiting — Stop has to mean stop, including what was typed under it. */
export function clearQueuedMessages(): void {
  queuedMessages.length = 0
}

/** Append the user message, then call the Go core and append its reply. */
export async function sendUserMessage(text: string): Promise<void> {
  if (cockpit.awaitingReply) {
    // Attachments stay staged in the composer and are picked up when this
    // actually sends, so a queued message keeps whatever was attached to it.
    if (text.trim()) queuedMessages.push(text)
    return
  }
  const trimmed = text.trim()
  const image = cockpit.pendingImage
  const context = cockpit.pendingContext
  const file = cockpit.pendingFile
  if (!trimmed && !image && !context && !file) return
  // The model only ever sees text, so an attached image is handed to it as a
  // sandboxed path reference it can pass to image_ocr — the bubble itself
  // shows just the caption + thumbnail, not that reference line. A dragged-in
  // file/browser tab instead inlines its actual content directly — no tool
  // call needed for the model to "see" it.
  // Explicit source tags so the model can tell attachment types apart —
  // a user-attached image vs a dragged-in workbench tab, and for tabs,
  // a file on disk vs a live web page. Only the model sees these lines.
  let sentText = trimmed
  if (image) sentText += `\n\n[attachment: user-attached image — read it with image_ocr] ${image.relPath}`
  if (file) {
    // Point at the tool that actually opens this kind of file. Naming the wrong
    // one costs a wasted turn; naming none costs the model guessing.
    const how = file.kind === 'audio'
      ? 'read it with audio_transcribe'
      : file.kind === 'video'
        ? 'read its speech with audio_transcribe, its on-screen text with video_ocr'
        : 'read it with read'
    sentText += `\n\n[attachment: user-attached ${file.kind} — ${how}] ${file.relPath}`
  }
  if (context) {
    const kindLabel = context.kind === 'file' ? 'file from a workbench tab' : 'web page text from a workbench browser tab'
    sentText += `\n\n[attachment: ${kindLabel}] ${context.label}:\n\`\`\`\n${context.content}\n\`\`\``
  }
  sentText = sentText.trim()
  cockpit.chat.push({
    role: 'user', text: trimmed, time: nowLabel(),
    imageDataUrl: image?.dataUrl, contextLabel: context?.label,
    attachLabel: file?.label, attachKind: file?.kind,
  })
  cockpit.pendingImage = null
  cockpit.pendingContext = null
  cockpit.pendingFile = null
  cockpit.awaitingReply = true
  cockpit.agentStatus = ''
  cockpit.toolSteps = []
  cockpit.streamingText = ''
  cockpit.reasoningText = ''
  try {
    const reply = await SendMessage(sentText)
    const steps = cockpit.toolSteps.length ? cockpit.toolSteps.map((s) => ({ ...s })) : undefined
    // Keep the thinking on the finished message (collapsed) — the live panel
    // alone would vanish the moment the turn completes.
    const reasoning = cockpit.reasoningText.trim() || undefined
    const thinkSecs = reasoning ? Math.max(1, Math.round((thinkLastAt - thinkStartedAt) / 1000)) : undefined
    cockpit.chat.push({ role: 'agent', text: reply, time: nowLabel(), steps, reasoning, thinkSecs })
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.sendError', { err: String(err) }), time: nowLabel() })
  } finally {
    cockpit.awaitingReply = false
    cockpit.agentStatus = ''
    cockpit.toolSteps = []
    cockpit.streamingText = ''
    cockpit.reasoningText = ''
  }
  await refreshWorkspace()
  await refreshSessions()
  await refreshGlobalHistory()
  // Engine is free again: whatever was typed under the running turn goes now.
  const next = queuedMessages.shift()
  if (next !== undefined) await sendUserMessage(next)
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

/** Live turn-progress text from the Go engine (see desktop/app.go emitAgentStatus). */
export function applyAgentStatus(status: string): void {
  cockpit.agentStatus = status
}

/** Live reply text from the Go engine (see desktop/app.go SendMessage's onChunk).
 * One call with the whole reply for a tool-using turn, or many small calls for a
 * plain streamed conversational one — either way, just keep appending. */
export function applyAgentChunk(chunk: string): void {
  cockpit.streamingText += chunk
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

/** Live tool call/result feed from the Go engine (turn.ToolEvent, relayed by
 * desktop/app.go recordToolAction). "call" opens a running step; "result"
 * closes the oldest one still running. */
export function applyToolEvent(ev: ToolEvent): void {
  const label = [ev.name, ev.subject].filter(Boolean).join(' ')
  // A row is recognized by the engine's call id, not by its label. The label is
  // incomplete on the early events — a model may stream a write's content long
  // before its path — so matching on it drew a second row the moment the name
  // arrived. Falls back to the label for engines that send no id.
  const running = (s: ToolStep) =>
    s.state === 'run' && (ev.ref && s.ref ? s.ref === ev.ref : s.label === label)
  if (ev.action === 'call') {
    // A call is announced repeatedly while the model writes it — once the tool
    // name is known, then as the content streams — and once more when it
    // actually runs. The row is reused: the counter climbs and the elapsed
    // clock keeps running instead of the timeline growing a row per line.
    const open = cockpit.toolSteps.find(running)
    if (open) {
      if (ev.added) open.added = ev.added
      // Let the row name itself once the subject shows up.
      if (ev.subject) open.label = label
      return
    }
    cockpit.toolSteps.push({ label, ref: ev.ref, state: 'run', startedAt: Date.now(), added: ev.added || undefined })
    return
  }
  if (ev.action !== 'result') return
  const step = cockpit.toolSteps.find(running) ?? cockpit.toolSteps.find((s) => s.state === 'run')
  if (!step) return
  if (ev.subject) step.label = label
  step.state = ev.ok ? 'done' : 'err'
  step.secs = Math.round((Date.now() - step.startedAt) / 1000)
  step.error = ev.ok ? undefined : ev.error
  // Only a write/edit carries these; everything else reports 0 and shows nothing.
  step.added = ev.added || undefined
  step.removed = ev.removed || undefined
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
  try {
    const content = kind === 'file' ? await ReadFile(ref) : await BrowserGetText(ref)
    cockpit.pendingContext = { kind, label, content }
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: t('cockpit.attachError', { err: String(err) }), time: nowLabel() })
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

const activeViewStorageKey = 'aetox.activeView'

export function setActiveView(view: string): void {
  cockpit.activeView = view
  // Survive an F5 *within this run* only: remember chat/settings (file tabs
  // don't persist, so a stored file path would point at nothing after
  // reload). sessionStorage, not localStorage — a real app relaunch must
  // always land on chat, never reopen straight into Settings because that's
  // where a previous session happened to be force-quit.
  if (view === 'chat' || view === 'settings') {
    try {
      sessionStorage.setItem(activeViewStorageKey, view)
    } catch {
      /* storage unavailable — view just won't persist */
    }
  }
}

/** Restore the last chat/settings view after a frontend reload (same run only). */
export function restoreActiveView(): void {
  try {
    const saved = sessionStorage.getItem(activeViewStorageKey)
    if (saved === 'settings' || saved === 'chat') cockpit.activeView = saved
  } catch {
    /* storage unavailable */
  }
}

/** Switch to a stored session — the transcript loads back and the agent's memory is restored. */
export async function selectSession(session: Session): Promise<void> {
  const messages = await LoadSession(session.id)
  cockpit.todos = []
  cockpit.ask = null
  cockpit.chat = messages.map(restoreAttachments)
  hydrateImages()
  await switchWorkbenchSession(session.id)
  await refreshSessions()
  await refreshGlobalHistory()
}

/** Permanently delete a session (any project); clears the chat if it was the open one. */
export async function deleteSession(session: Session): Promise<void> {
  await DeleteSession(session.id)
  removeWorkbenchState(session.id)
  if (session.active) cockpit.chat = []
  await refreshSessions()
  await refreshGlobalHistory()
}

/** Start a blank session (current one is saved first, engine-side). */
export async function newSession(): Promise<void> {
  await NewSession()
  cockpit.chat = []
  cockpit.todos = []
  cockpit.ask = null
  // Explicit switch (not adopt): a brand-new session starts with an empty
  // workbench; the old session's layout stays saved for when it's reopened.
  await switchWorkbenchSession(await CurrentSessionID())
  await refreshSessions()
  await refreshGlobalHistory()
}
