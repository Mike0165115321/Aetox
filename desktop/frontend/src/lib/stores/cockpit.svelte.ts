// The single source of truth for cockpit UI state. Reactive ($state); components
// read slices of it via props from App. Mutate its fields (the Go core can push
// incremental updates here — append a chat message, advance a timeline step) and
// the UI reacts. Do not reassign `cockpit` itself; mutate its properties.

import { emptyCockpitState, type CockpitState, type TreeNode, type Session } from '../types'
import type { CockpitSource } from '../services/cockpit'

export const cockpit = $state<CockpitState>(emptyCockpitState())

export async function hydrate(source: CockpitSource): Promise<void> {
  Object.assign(cockpit, await source.load())
}

function nowLabel(): string {
  return new Date().toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

/** Append a user message. The Go core will later respond by pushing an agent
    message + timeline steps onto the same reactive state. */
export function sendUserMessage(text: string): void {
  const trimmed = text.trim()
  if (!trimmed) return
  cockpit.chat.push({ role: 'user', text: trimmed, time: nowLabel() })
}

/** View state: expand/collapse a folder. */
export function toggleNode(node: TreeNode): void {
  if (node.kind === 'dir') node.open = !node.open
}

/** View state: mark a file as the active/open one. */
export function selectNode(node: TreeNode): void {
  if (node.kind !== 'file') return
  for (const n of cockpit.tree) n.active = false
  node.active = true
}

export function selectSession(session: Session): void {
  for (const s of cockpit.sessions) s.active = false
  session.active = true
}
