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
  ListSessions, LoadSession, NewSession, CurrentSessionID, SearchSessions, DeleteSession,
  SaveChatImage, SaveChatFile, ReadImageDataURL, CancelTurn, BrowserGetText, RecentProjects,
  ListAllSessions, SearchAllSessions, LoadSessionAnyProject, ClearProjectFocus,
  AnswerUserQuestion, Interject, RetryActiveProvider, PendingUndo, UndoLastTurn,
  CompleteSignIn, SignOut, ImportSignIn,
  ListTaskChips, DismissTaskChip,
  RetryFailedTurn, RegenerateReply, ResendEdited, SwitchVariant,
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
  }
  if (out.role === 'agent') return out
  // What is folded out is also kept: editing a restored question has to be able
  // to re-send the exact lines the model was given the first time.
  let suffix = ''
  out.text = out.text
    .replace(ATTACH_CTX_RE, (all, label: string) => { out.contextLabel = label; suffix += all; return '' })
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

function stepsFromParts(parts?: TurnPart[]): ToolStep[] | undefined {
  if (!parts?.length) return undefined
  const lastTextAt = parts.map((p) => p.kind).lastIndexOf('text')
  const steps: ToolStep[] = []
  parts.forEach((part, i) => {
    if (part.kind === 'text') {
      if (i !== lastTextAt && part.text) {
        steps.push({ kind: 'note', label: part.text, state: 'done', startedAt: 0 })
      }
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
  await refreshUndo()
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
  await refreshTaskChips()
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

function nowLabel(): string {
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

export async function sendUserMessage(text: string, alreadyShown = false): Promise<void> {
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
    const kindLabel = context.kind === 'file' ? 'file from a workbench tab' : 'web page text from a workbench browser tab'
    attachSuffix += `\n\n[attachment: ${kindLabel}] ${context.label}:\n\`\`\`\n${context.content}\n\`\`\``
  }
  const sentText = (trimmed + attachSuffix).trim()
  if (!alreadyShown) {
    cockpit.chat.push({
      role: 'user', text: trimmed, time: nowLabel(),
      imageDataUrl: image?.dataUrl, contextLabel: context?.label,
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
      cockpit.chat.push({
        role: 'agent', text: reply.text, parts: reply.parts as TurnPart[] | undefined,
        time: nowLabel(), ...turnArtifacts(),
      })
    } catch (err) {
      // The engine persists nothing for a turn that failed, so the text it was
      // given rides on the bubble: it is the only copy left to retry with.
      cockpit.chat.push({
        role: 'agent', text: t('cockpit.sendError', { err: String(err) }), time: nowLabel(),
        failed: true, failedText: sentText,
      })
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
  try {
    await call()
  } finally {
    cockpit.awaitingReply = false
    cockpit.agentStatus = ''
    cockpit.toolSteps = []
    cockpit.turnFiles = []
    cockpit.streamingText = ''
    cockpit.reasoningText = ''
  }
  await refreshWorkspace()
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
function turnArtifacts(): Pick<ChatMessage, 'steps' | 'reasoning' | 'thinkSecs' | 'producedFiles'> {
  const steps = cockpit.toolSteps.length ? cockpit.toolSteps.map((s) => ({ ...s })) : undefined
  const reasoning = cockpit.reasoningText.trim() || undefined
  const thinkSecs = reasoning ? Math.max(1, Math.round((thinkLastAt - thinkStartedAt) / 1000)) : undefined
  const producedFiles = cockpit.turnFiles.length ? [...cockpit.turnFiles] : undefined
  return { steps, reasoning, thinkSecs, producedFiles }
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
  if (!text || cockpit.awaitingReply) return
  cockpit.chat.splice(index, 1)
  await runLiveTurn(async () => {
    try {
      const reply = await RetryFailedTurn(text)
      cockpit.chat.push({
        role: 'agent', text: reply.text, parts: reply.parts as TurnPart[] | undefined,
        time: nowLabel(), ...turnArtifacts(),
      })
    } catch (err) {
      cockpit.chat.push({
        role: 'agent', text: t('cockpit.sendError', { err: String(err) }), time: nowLabel(),
        failed: true, failedText: text,
      })
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
  if (!last || last.role !== 'agent' || last.failed || cockpit.awaitingReply) return
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
      // back. Say what went wrong under it rather than replacing it.
      last.error = t('cockpit.regenerateError', { err: String(err) })
    }
  })
}

/** Show a different one of the stored answers, and continue the conversation
 *  from it: the engine rewrites its memory to match (App.SwitchVariant). */
export async function switchVariant(index: number): Promise<void> {
  const last = cockpit.chat.at(-1)
  if (!last || !last.variants || cockpit.awaitingReply) return
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
  if (!trimmed || cockpit.awaitingReply || cockpit.chat.length < 2) return
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
        time: nowLabel(), ...turnArtifacts(),
      })
    } catch (err) {
      cockpit.chat.push({
        role: 'agent', text: t('cockpit.sendError', { err: String(err) }), time: nowLabel(),
        failed: true, failedText: sent,
      })
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

/** Start a suggested task: consume the chip, then run its prompt in a fresh
 *  session. The prompt was written to stand alone (suggest_task requires it),
 *  so the new session needs nothing from the one that suggested it. */
export async function startTaskChip(chip: CockpitState['taskChips'][number]): Promise<void> {
  await DismissTaskChip(chip.id)
  await newSession()
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

/** Live tool call/result feed from the Go engine (turn.ToolEvent, relayed by
 * desktop/app.go recordToolAction). "call" opens a running step; "result"
 * closes the oldest one still running. */
export function applyToolEvent(ev: ToolEvent): void {
  // The loop's own story, interleaved between the calls it explains (§59).
  // Both land as finished rows: there is nothing running to close later.
  if (ev.action === 'note') {
    const text = ev.text?.trim()
    if (text) {
      cockpit.toolSteps.push({
        kind: 'note', label: text, parent: ev.parent || undefined,
        state: 'done', startedAt: Date.now(),
      })
    }
    return
  }
  if (ev.action === 'thinking') {
    cockpit.toolSteps.push({
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
    const open = cockpit.toolSteps.find(running)
    if (open) {
      if (ev.added) open.added = ev.added
      // Let the row name itself once the subject shows up.
      if (ev.subject) open.label = label
      return
    }
    cockpit.toolSteps.push({
      label, ref: ev.ref, parent: ev.parent || undefined,
      // Only a `task` call carries these, and they arrive on the first event —
      // the delegation is named before its first delegate step shows up.
      agent: ev.agent || undefined, brief: ev.brief || undefined,
      state: 'run', startedAt: Date.now(), added: ev.added || undefined,
    })
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
  // A finished file the user asked for. Deduped because a turn may well write
  // the same workbook twice — a first pass and a correction — and the answer
  // should offer it once.
  for (const path of ev.artifacts ?? []) {
    if (path && !cockpit.turnFiles.includes(path)) cockpit.turnFiles.push(path)
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
