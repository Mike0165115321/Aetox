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

/** Pull the real project/model state the Go engine is actually running with. */
export async function loadRealState(): Promise<void> {
  const [project, modelInfo] = await Promise.all([GetProjectStatus(), GetModelInfo()])
  Object.assign(cockpit.project, project)
  applyModelInfo(modelInfo)
  await refreshWorkspace()
}

/** Let the user pick a real folder via the native dialog; re-points the engine at it. */
export async function openFolder(): Promise<void> {
  const project = await OpenProjectFolder()
  Object.assign(cockpit.project, project)
  await refreshWorkspace()
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
}

/** View state: expand/collapse a folder. */
export function toggleNode(node: TreeNode): void {
  if (node.kind === 'dir') node.open = !node.open
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

export function selectSession(session: Session): void {
  for (const s of cockpit.sessions) s.active = false
  session.active = true
}
