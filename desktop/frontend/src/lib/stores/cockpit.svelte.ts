// The single source of truth for cockpit UI state. Reactive ($state); components
// read slices of it via props from App. Mutate its fields (the Go core can push
// incremental updates here — append a chat message, advance a timeline step) and
// the UI reacts. Do not reassign `cockpit` itself; mutate its properties.

import { emptyCockpitState, type CockpitState, type TreeNode, type ChangedFile, type Session, type ToolStep } from '../types'
import type { CockpitSource } from '../services/cockpit'
import {
  SendMessage, GetProjectStatus, GetModelInfo, OpenProjectFolder, OpenProjectPath,
  SwitchProvider, SwitchThinkLevel, SwitchApprovalMode,
  SwitchModel, SetAPIKey, ProjectTree, CommandHistory, GitChangedFiles, ReadFile,
  ListSessions, LoadSession, NewSession, CurrentSessionID, SearchSessions, DeleteSession,
  SaveChatImage, ReadImageDataURL, CancelTurn, BrowserGetText, RecentProjects,
  ListAllSessions, SearchAllSessions, LoadSessionAnyProject, ClearProjectFocus,
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

/** Open a session from the global history list — switches project first if it belongs to a different one. */
export async function selectGlobalSession(session: Session): Promise<void> {
  const messages = await LoadSessionAnyProject(session.id)
  cockpit.chat = messages.map((m) => ({
    role: m.role === 'agent' ? 'agent' as const : 'user' as const,
    text: m.text,
    time: m.time,
  }))
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

export async function submitAPIKey(providerName: string, apiKey: string): Promise<void> {
  applyModelInfo(await SetAPIKey(providerName, apiKey))
}

function nowLabel(): string {
  return new Date().toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

/** Append the user message, then call the Go core and append its reply. */
export async function sendUserMessage(text: string): Promise<void> {
  const trimmed = text.trim()
  const image = cockpit.pendingImage
  const context = cockpit.pendingContext
  if (!trimmed && !image && !context) return
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
  if (context) {
    const kindLabel = context.kind === 'file' ? 'file from a workbench tab' : 'web page text from a workbench browser tab'
    sentText += `\n\n[attachment: ${kindLabel}] ${context.label}:\n\`\`\`\n${context.content}\n\`\`\``
  }
  sentText = sentText.trim()
  cockpit.chat.push({
    role: 'user', text: trimmed, time: nowLabel(),
    imageDataUrl: image?.dataUrl, contextLabel: context?.label,
  })
  cockpit.pendingImage = null
  cockpit.pendingContext = null
  cockpit.awaitingReply = true
  cockpit.agentStatus = ''
  cockpit.toolSteps = []
  cockpit.streamingText = ''
  cockpit.reasoningText = ''
  try {
    const reply = await SendMessage(sentText)
    const steps = cockpit.toolSteps.length ? cockpit.toolSteps.map((s) => ({ ...s })) : undefined
    cockpit.chat.push({ role: 'agent', text: reply, time: nowLabel(), steps })
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
}

/** Abort the turn in flight — the engine's tool loop is unbounded, this is the user's brake. */
export function cancelTurn(): void {
  CancelTurn()
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

/** Live reasoning/thinking text from the Go engine (see desktop/app.go
 * SendMessage's onReasoningChunk) — only fires for providers that stream
 * reasoning tokens (DeepSeek, Anthropic extended thinking, ...); '' means
 * either idle or this provider/turn had none to show. */
export function applyReasoningChunk(chunk: string): void {
  cockpit.reasoningText += chunk
}

/** Live tool call/result feed from the Go engine (see desktop/app.go recordToolAction).
 * "call" opens a running step; "result" ("name: สำเร็จ" | "name: <error>") closes the
 * oldest one still running. */
export function applyToolEvent(ev: { action: string; detail: string }): void {
  if (ev.action === 'call') {
    cockpit.toolSteps.push({ label: ev.detail, state: 'run', startedAt: Date.now() })
    return
  }
  if (ev.action !== 'result') return
  const step = cockpit.toolSteps.find((s) => s.state === 'run')
  if (!step) return
  // note: "ไม่สำเร็จ" also ends with "สำเร็จ" — match the full ": สำเร็จ" suffix
  step.state = ev.detail.endsWith(': สำเร็จ') ? 'done' : 'err'
  step.secs = Math.round((Date.now() - step.startedAt) / 1000)
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
  cockpit.chat = messages.map((m) => ({
    role: m.role === 'agent' ? 'agent' as const : 'user' as const,
    text: m.text,
    time: m.time,
  }))
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
  // Explicit switch (not adopt): a brand-new session starts with an empty
  // workbench; the old session's layout stays saved for when it's reopened.
  await switchWorkbenchSession(await CurrentSessionID())
  await refreshSessions()
  await refreshGlobalHistory()
}
