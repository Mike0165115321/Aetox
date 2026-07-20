// The single source of truth for cockpit UI state. Reactive ($state); components
// read slices of it via props from App. Mutate its fields (the Go core can push
// incremental updates here — append a chat message, advance a timeline step) and
// the UI reacts. Do not reassign `cockpit` itself; mutate its properties.

import { emptyCockpitState, type CockpitState, type TreeNode, type ChangedFile, type Session } from '../types'
import type { CockpitSource } from '../services/cockpit'
import {
  SendMessage, GetProjectStatus, GetModelInfo, OpenProjectFolder,
  SwitchProvider, SwitchThinkLevel, SwitchApprovalMode,
  SwitchModel, SetAPIKey, ProjectTree, CommandHistory, GitChangedFiles, ReadFile,
  ListSessions, LoadSession, NewSession, CurrentSessionID, SearchSessions,
} from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'

export const cockpit = $state<CockpitState>(emptyCockpitState())

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
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const mins = Math.max(0, Math.round((Date.now() - t) / 60000))
  if (mins < 1) return 'เมื่อกี้'
  if (mins < 60) return `${mins} นาที`
  const hrs = Math.round(mins / 60)
  if (hrs < 24) return `${hrs} ชม.`
  return `${Math.round(hrs / 24)} วัน`
}

/** Pull this project's chat history (sessions are stored per project in Go). */
export async function refreshSessions(): Promise<void> {
  const [metas, current] = await Promise.all([ListSessions(), CurrentSessionID()])
  cockpit.sessions = metas.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), active: m.id === current,
  }))
}

/** Full-text search this project's history (Thai/English substrings, FTS5). */
export async function searchSessions(query: string): Promise<void> {
  if (!query.trim()) return refreshSessions()
  const [hits, current] = await Promise.all([SearchSessions(query), CurrentSessionID()])
  cockpit.sessions = hits.map((m) => ({
    id: m.id, title: m.title, ago: agoLabel(m.updatedAt), active: m.id === current, snippet: m.snippet,
  }))
}

/** Pull the real project/model state the Go engine is actually running with. */
export async function loadRealState(): Promise<void> {
  const [project, modelInfo] = await Promise.all([GetProjectStatus(), GetModelInfo()])
  Object.assign(cockpit.project, project)
  applyModelInfo(modelInfo)
  await refreshWorkspace()
  await refreshSessions()
}

/** Let the user pick a real folder via the native dialog; re-points the engine at it. */
export async function openFolder(): Promise<void> {
  const project = await OpenProjectFolder()
  Object.assign(cockpit.project, project)
  cockpit.chat = []
  await refreshWorkspace()
  await refreshSessions()
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
  if (!trimmed) return
  cockpit.chat.push({ role: 'user', text: trimmed, time: nowLabel() })
  try {
    const reply = await SendMessage(trimmed)
    cockpit.chat.push({ role: 'agent', text: reply, time: nowLabel() })
  } catch (err) {
    cockpit.chat.push({ role: 'agent', text: `เกิดข้อผิดพลาด: ${err}`, time: nowLabel() })
  }
  await refreshWorkspace()
  await refreshSessions()
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

/** View state: mark a file as the active/open one, and open it in the viewer. */
export function selectNode(node: TreeNode): void {
  if (node.kind !== 'file') return
  for (const n of cockpit.tree) n.active = false
  node.active = true
  openFile(node.path)
}

/** Open a file tab (fetching its content once), or just switch to it if already open. */
export async function openFile(path: string): Promise<void> {
  if (!cockpit.openFiles.some((f) => f.path === path)) {
    try {
      const content = await ReadFile(path)
      cockpit.openFiles.push({ path, content })
    } catch (err) {
      cockpit.openFiles.push({ path, content: `เปิดไฟล์ไม่ได้: ${err}` })
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

export function setActiveView(view: string): void {
  cockpit.activeView = view
}

/** Switch to a stored session — the transcript loads back and the agent's memory is restored. */
export async function selectSession(session: Session): Promise<void> {
  const messages = await LoadSession(session.id)
  cockpit.chat = messages.map((m) => ({
    role: m.role === 'agent' ? 'agent' as const : 'user' as const,
    text: m.text,
    time: m.time,
  }))
  await refreshSessions()
}

/** Start a blank session (current one is saved first, engine-side). */
export async function newSession(): Promise<void> {
  await NewSession()
  cockpit.chat = []
  await refreshSessions()
}
