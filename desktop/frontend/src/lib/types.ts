// Cockpit view-model. These shapes are the whole contract between the UI and
// whatever feeds it — a mock today, the Go core via Wails bindings later.
// Components render CockpitState; they never know the source.

export type GitStatus = 'M' | 'U' | null

export interface TreeNode {
  label: string
  path: string
  kind: 'dir' | 'file'
  depth: number
  open?: boolean
  status?: GitStatus
  icon?: string
}

export interface OpenFile {
  path: string
  content: string
}

export interface Session {
  id: string
  title: string
  ago: string
  /** RFC3339 stamp behind `ago`. Kept alongside the rendered label because the
   *  sidebar groups rows under วันนี้/เมื่อวาน/…, and a day boundary cannot be
   *  recovered from "3 วันที่แล้ว" — the two answer different questions. */
  updatedAt?: string
  active?: boolean
  snippet?: string
  /** Only set on the cross-project (global) history list. */
  projectName?: string
  /** The โปรเจกต์ this chat was held inside (§90). Only search results ever
   *  carry it: the plain lists drop those rows, because a project's chats
   *  belong to that project's own list. A row with this set was found by
   *  searching and says so. */
  space?: string
  /** The desk this conversation was held at (COMPANY.md §2). '' for the ones
   *  that predate desks — they belong to no desk and are shown unlabelled. */
  mode?: string
  /** Who the conversation was held with (§85): '' for the main assistant,
   *  an agent's name for a direct chat in the office. */
  agent?: string
}

export interface RecentProject {
  key: string
  name: string
  path: string
  ago: string
  active?: boolean
  snippet?: string
}

export interface ProjectInfo {
  name: string
  path: string
  branch: string
  /** false = "ไม่โฟกัสโปรเจกต์" mode — engine rooted at home, tools still fully usable. */
  focused: boolean
  extraBranches: number
  governanceFile: string
  governanceLoaded: boolean
}

export type ApprovalMode = 'ask' | 'unsafe-only' | 'full-access'

export interface ModelStatus {
  provider: string
  modelName: string
  thinkLevel: string
  contextUsed: number
  contextMax: number
  approval: ApprovalMode
  /** Active wire format for providers that speak more than one (e.g. DeepSeek's
   *  "anthropic" vs "openai-compatible"). Empty for single-format providers. */
  wireFormat: string
  /** Why `provider` is not actually answering — the engine fell back to the
   *  built-in aetox provider (e.g. LM Studio's server is not running). Empty
   *  when the named provider really is the one running. */
  warning: string
}

/** One labeled share of the context window; key: system | tools | messages | free. */
export interface ContextSlice {
  key: string
  tokens: number
}

/** Composer context meter payload (GetContextBreakdown). */
export interface ContextBreakdown {
  usedTokens: number
  maxTokens: number
  slices: ContextSlice[]
  /** True once this session has actually sent a round, so usedTokens is the
   *  provider's own count. False means nothing has been sent and the figure is
   *  a forecast of the first request — showing that as "used" made a fresh chat
   *  look like a bill already run up. */
  measured: boolean
}

export interface ChatMessage {
  role: 'user' | 'agent'
  text: string
  time: string
  /** The stored row this reply became. It is what a rating addresses, so a
   * bubble without one cannot be rated — which is the honest state for a turn
   * that failed and was persisted nowhere. */
  id?: number
  /** The verdict already given for this reply, so a reopened session shows the
   * thumb that was pressed rather than a blank pair. */
  rating?: 'good' | 'bad' | 'unknown'
  /** optional badge, e.g. "Thinking (low)" */
  tag?: string
  /** data: URL of an attached image, for inline preview only (not sent to the model). */
  imageDataUrl?: string
  /** Label of a dragged-in file/browser tab, for a small chip on the bubble (content itself is inlined into text). */
  contextLabel?: string
  /** First lines of that attached file, for the card in the bubble. */
  contextPreview?: string
  /** A user-attached video/audio/document, for the same chip. Only the path
   * goes to the model, so without this the bubble showed nothing at all. */
  attachLabel?: string
  attachKind?: PendingFile['kind']
  /** Sandbox path of an attached image on a restored message — the thumbnail is
   * read back from it, since a data URL has no business in the history DB. */
  imageRelPath?: string
  /** The attachment marker lines that were appended to this message when it was
   * sent — the part the model reads and the bubble never shows. Kept so editing
   * the prose can re-send them: without it, fixing a typo would silently detach
   * the file the question is about. */
  attachSuffix?: string
  /** tool calls made during this turn, kept on the reply for a persistent timeline. */
  steps?: ToolStep[]
  /** Finished files this turn made for the user (see CockpitState.turnFiles).
   *  Rebuilt from the message's own parts on reload, so the open button survives
   *  a restart — it did not at first, and the file became unreachable from the
   *  answer that announced it. */
  producedFiles?: string[]
  /** The turn as it happened — prose, thinking segments and tool calls in the
   * order they occurred. When present this is what the bubble draws, so
   * narration appears where it was said instead of collapsed into a panel.
   * Absent on user messages and on every turn from before the sequence existed,
   * which fall back to rendering `text`. */
  parts?: TurnPart[]
  /** the model's thinking for this reply — kept after the turn, collapsed by default. */
  reasoning?: string
  /** seconds the model spent thinking (first→last reasoning chunk). */
  thinkSecs?: number
  /** Every answer this question has had, when it was asked more than once.
   * text/reasoning/thinkSecs/steps above always mirror variants[activeVariant].
   * Fewer than two means no switcher is drawn. */
  variants?: MessageVariant[]
  activeVariant?: number
  /** Sent into a turn that was already running (Interject). The transcript
   *  draws it below the live block instead of above it, because that is where
   *  it happened — the array order is already right, so the flag is cleared
   *  the moment the turn ends and the bubble takes its ordinary place. */
  duringTurn?: boolean
  /** This bubble is a turn that never completed. failedText is exactly what was
   * sent — attachment marker lines and all — so a retry re-sends the same thing
   * rather than a reconstruction of it. */
  failed?: boolean
  failedText?: string
  /** Files put back before this answer was regenerated, named on the bubble:
   * an answer that quietly undid six files would be the worse surprise. */
  revertedFiles?: string[]
  /** A re-run that failed, shown under the answer it could not replace. The
   * previous answer is still the one on screen. */
  error?: string
}

/** One piece of an assistant turn — mirrors turn.TurnPart in Go.
 *
 * The shape a turn actually has: a provider streams prose, a tool call, more
 * prose. Collapsing that to one string is what put narration in a separate
 * panel and made the tool timeline impossible to store. */
export interface TurnPart {
  kind: 'text' | 'thinking' | 'tool'
  /** prose, on a 'text' part */
  text?: string
  /** seconds a 'thinking' segment streamed */
  secs?: number
  tool?: ToolPartInfo
}

export interface ToolPartInfo {
  ref?: string
  name: string
  subject?: string
  agent?: string
  brief?: string
  /** Which pile the worker on a `task` row is in: 'agent' (เอเจน) or 'helper'
   *  (ซับเอเจน). Stamped by the engine — the kind is decided by which home
   *  the profile file lives in, which only the engine can see. */
  agentKind?: string
  ok: boolean
  error?: string
  secs?: number
  added?: number
  removed?: number
  /** Finished files this call made for the user. Unlike the live ToolEvent's
   *  copy, this one is written down with the message — which is what lets the
   *  open button still be there after a restart. */
  artifacts?: string[]
}

/** One of the answers a question received. */
export interface MessageVariant {
  text: string
  reasoning?: string
  thinkSecs?: number
  /** Live only. The store keeps no tool timeline, so a variant read back from a
   * reloaded session has none — the switcher just shows no toggle for it. */
  steps?: ToolStep[]
}

/** One tool call in the live per-turn timeline ("Using browser_read… 12s"). */
/** One tool call/result as the engine sends it — mirrors turn.ToolEvent in Go. */
export interface ToolEvent {
  action: 'call' | 'result' | 'note' | 'thinking'
  name: string
  /** The engine's tool-call id — how a row is recognized across updates. The
   * label cannot serve: it is empty of its subject on the early events. */
  ref?: string
  /** Set only on events from inside a sub-agent: the ref of the `task` call that
   * spawned it. The row is shown as that task's work rather than the agent's own. */
  parent?: string
  subject?: string
  ok?: boolean
  error?: string
  added?: number
  removed?: number
  /** Set only on the `task` call that opens a delegation: which sub-agent it is
   * handing the work to, and the whole brief it handed over. They are what make
   * a delegation render as a named block with its own steps inside, instead of
   * one more row reading "task". */
  agent?: string
  brief?: string
  /** With them, which pile that worker is in: 'agent' (เอเจน) or 'helper'
   * (ซับเอเจน). The chat counts the two apart; empty lands in the helper
   * pile, which is where every delegation lived before the split. */
  agentKind?: string
  /** On a result event: sandbox paths of finished files this call made for the
   * user — set only by tools whose whole output is a file (sheet_write and, in
   * time, the .pptx and .docx writers). `write` and `edit` deliberately leave
   * it empty, or a coding turn would print a card per edited source file. */
  artifacts?: string[]
  /** On a "note" event: the narration the model wrote alongside this round's
   * tool calls — its own words for what it is doing. */
  text?: string
  /** On a "thinking" event: how long that round's reasoning streamed. */
  secs?: number
}

export interface ToolStep {
  /** What this row is. Absent means a tool call, as every row was before
   * "note" (the model's narration between calls) and "thinking" (a reasoning
   * segment's duration) joined the timeline. */
  kind?: 'note' | 'thinking'
  label: string
  /** ToolEvent.ref of the call this row is showing, when the engine sent one. */
  ref?: string
  /** Set when this row is a sub-agent's work, carrying the `task` call's ref. */
  parent?: string
  state: 'run' | 'done' | 'err'
  /** Why it failed, straight from the engine's result event. Only on 'err'. */
  error?: string
  /** Lines a write or edit changed, for the "+9 -0" readout. */
  added?: number
  removed?: number
  /** On a `task` row: the worker doing the job, and the brief it was given. */
  agent?: string
  brief?: string
  /** On a `task` row: 'agent' (เอเจน) or 'helper' (ซับเอเจน) — which of
   * the two count chips this delegation belongs to. Absent (old turns, engine
   * without the stamp) counts as a helper, the pre-split reading. */
  agentKind?: string
  startedAt: number
  /** seconds it took, filled in when the result arrives */
  secs?: number
}

/** A row in the timeline plus whatever ran underneath it. Only a `task` row ever
 * has children — a sub-agent's tool calls, which belong inside its block rather
 * than mixed into the main agent's list. */
export interface TimelineNode {
  step: ToolStep
  children: ToolStep[]
}

/** Fold the flat step list the engine produces into what the timeline draws.
 *
 * Delegation is the only nesting there is, so this is one pass: a row with no
 * parent is the agent's own, and a row carrying a parent belongs to the `task`
 * row with that ref. A child whose parent is not in this list (a persisted turn
 * trimmed oddly, an event that arrived first) is kept at the top level rather
 * than dropped — a visible row in the wrong place beats work that vanished.
 */
export function groupSteps(steps: ToolStep[]): TimelineNode[] {
  const byRef = new Map<string, TimelineNode>()
  const nodes: TimelineNode[] = []
  for (const step of steps) {
    if (!step.parent) {
      const node: TimelineNode = { step, children: [] }
      if (step.ref) byRef.set(step.ref, node)
      nodes.push(node)
      continue
    }
    const parent = byRef.get(step.parent)
    if (parent) parent.children.push(step)
    else nodes.push({ step, children: [] })
  }
  return nodes
}

/** True when a row is a delegation: the engine named a sub-agent on it, or
 * something ran underneath it. Both are checked because a delegate that used no
 * tools has no children, and a `task` call whose `agent` was left to default has
 * no name. */
export function isDelegation(node: TimelineNode): boolean {
  // Narration is free text — it may well start with the word "task".
  if (node.step.kind) return false
  return Boolean(node.step.agent) || node.children.length > 0 || node.step.label.startsWith('task ') || node.step.label === 'task'
}

/** An image attached in the composer, staged before send. */
export interface PendingImage {
  /** Sandbox-relative path — what the model/tools (e.g. image_ocr) operate on. */
  relPath: string
  /** data: URL for the composer's own thumbnail preview. */
  dataUrl: string
}

/** A non-image file attached from the composer's "+" — copied into the sandbox
 * and handed to the model as a path rather than inlined, because a clip is for
 * a tool to open, not for the message to carry. */
export interface PendingFile {
  /** Sandbox-relative path — what audio_transcribe / video_ocr / read operate on. */
  relPath: string
  /** Chip label — the original file name. */
  label: string
  /** Decides which tool the model is pointed at. */
  kind: 'audio' | 'video' | 'file'
}

/** A workbench tab (file or browser) dragged into the composer, staged before
 * send — its content is inlined into the message so the model reads it
 * directly, no tool call needed. */
export interface PendingContext {
  kind: 'file' | 'browser'
  /** Chip label — file name or page title/URL. */
  label: string
  content: string
}

export type StepStatus = 'done' | 'active' | 'wait'

export interface ChangeSummary {
  items: string[]
  footer: string
  badge: string
}

export interface TimelineStep {
  time: string
  title: string
  detail: string
  status: StepStatus
  change?: ChangeSummary
}

export interface TaskState {
  elapsed: string
  steps: TimelineStep[]
}

// DiffView / TestRun / ChangedFile lived here for the Review panel, which was
// removed on 2026-08-03. Two of its four sections — the diff and the test run —
// were never written to by anything and had shown an empty box since the day
// they shipped; the two that worked duplicated what the file tree already says
// with its M/U badges. It was a code-review surface in a product whose promise
// is finished work for people who do not read diffs.

/** One folder the user added to the focused project. It carries the same rights
 *  the project folder does — the list is the permission, so what is drawn here
 *  is exactly what the agent can reach. */
export interface ProjectFolder {
  path: string
  name: string
  /** No longer on disk. Kept on the list (an unplugged drive is not a decision
   *  to remove it) but drawn as unavailable, or it reads as reachable. */
  missing: boolean
}

export interface CockpitState {
  project: ProjectInfo
  /** Folders added to the focused project, in the order they were added. */
  projectFolders: ProjectFolder[]
  projects: RecentProject[]
  tree: TreeNode[]
  sessions: Session[]
  /** All chat history across every project, newest first — sidebar's global history layer.
   *  Chats held inside a โปรเจกต์ are not in it: they live in `spaceHistory`. */
  history: Session[]
  /** The open chat's project's own chats (§90), newest first. Empty whenever
   *  the open chat is in no project — which is most of the time, and is what
   *  puts `history` back on screen. */
  spaceHistory: Session[]
  model: ModelStatus
  chat: ChatMessage[]
  task: TaskState
  openFiles: OpenFile[]
  /** 'chat', 'settings', 'office', 'artifacts', or an open file's path —
   *  which surface the window currently shows. */
  activeView: string
  /** The desk the open session was created at, '' for the full desk. Read back
   *  from the engine rather than remembered here: a session is born at a desk
   *  and never moves, so this changes only when a different session is opened
   *  (COMPANY.md §6.3). */
  desk: string
  /** The agent the open session talks to directly (§85), '' for the main
   *  assistant. Same lifecycle as desk: fixed at birth, read back, never
   *  remembered independently. */
  chair: string
  /** The โปรเจกต์ the open session is being held inside (COMPANY.md §84), ''
   *  for a chat held outside every project. Same lifecycle as desk and chair:
   *  fixed when the session is born, read back from the engine when one is
   *  reopened. It is what the chat shows so a project chat does not look like
   *  any other chat — a session filed somewhere with nothing on screen saying
   *  so reads as the project having vanished. */
  space: string
  /** True from the moment a message is sent until the reply (or an error) arrives. */
  awaitingReply: boolean
  /** Files the last turn changed, from PendingUndo. Empty when there is nothing
   *  to undo — no snapshot yet, not a git repo, or the turn touched no file. */
  undoFiles: string[]
  /** Live turn-progress text from the Go engine's status reporter ("กำลังคิดคำตอบ...", etc), '' when idle. */
  agentStatus: string
  /** Tool calls of the turn in flight, appended live from agent:tool events. */
  toolSteps: ToolStep[]
  /** Sandbox paths of finished files this turn produced — a spreadsheet, a deck,
   *  a document. Collected live from agent:tool results so the reply can show
   *  them as cards with an open button: the file panel is where you go looking,
   *  and a deliverable should not need looking for. Reset with toolSteps. */
  turnFiles: string[]
  /** Reply text streamed so far this turn, appended live from agent:chunk events. '' when idle. */
  streamingText: string
  /** Model's reasoning/thinking tokens streamed so far this turn, from agent:reasoning events. '' when idle or the provider doesn't stream reasoning. */
  reasoningText: string
  /** Image staged in the composer, not yet sent. */
  pendingImage: PendingImage | null
  /** Why the last attempt to open a session from the history list failed, in
   *  the engine's own words. '' when the last one worked. */
  sessionError: string
  /** File/browser tab dragged into the composer, staged before send. */
  pendingContext: PendingContext | null
  /** Non-image file attached in the composer, staged before send. */
  pendingFile: PendingFile | null
  /** Question the model is blocked on (ask_user tool), null when none. */
  ask: { question: string; options: string[] } | null
  /** The model's task checklist (todo_write tool), replaced wholesale each call. */
  todos: { content: string; status: 'pending' | 'in_progress' | 'completed' }[]
  /** Side work the agent flagged with suggest_task — chips the user can start or dismiss. */
  taskChips: TaskChip[]
  /** How many things the agent wants to remember and is waiting to be allowed to.
   * Surfaced as a mark on the way into settings: an approval queue nobody is
   * told about is one that never gets emptied, which would quietly turn
   * "nothing takes effect without you" into "nothing takes effect". */
  pendingLearned: number
  /** A one-shot request from another page for what Settings should open on:
   * the team page's configure/create doors land in the shared profile editor
   * this way. Carries the *kind* because it came from the roster — Settings
   * must never re-derive it from a file. Consumed and cleared on arrival. */
  settingsIntent: { section: string; agent?: string; createAgent?: boolean } | null
}

/** One suggested side task from the agent (suggest_task tool). */
export interface TaskChip {
  id: string
  title: string
  tldr: string
  prompt: string
  createdAt: string
}

/** A blank, well-formed state so the UI renders before the source hydrates. */
export function emptyCockpitState(): CockpitState {
  return {
    project: { name: '', path: '', branch: '', focused: false, extraBranches: 0, governanceFile: '', governanceLoaded: false },
    projectFolders: [],
    projects: [],
    tree: [],
    sessions: [],
    history: [],
    spaceHistory: [],
    model: { provider: '', modelName: '', thinkLevel: '', contextUsed: 0, contextMax: 0, approval: 'ask', wireFormat: '', warning: '' },
    chat: [],
    task: { elapsed: '', steps: [] },
    turnFiles: [],
    openFiles: [],
    activeView: 'chat',
    desk: '',
    chair: '',
    space: '',
    awaitingReply: false,
    undoFiles: [],
    agentStatus: '',
    toolSteps: [],
    streamingText: '',
    reasoningText: '',
    ask: null,
    todos: [],
    taskChips: [],
    pendingLearned: 0,
    settingsIntent: null,
    pendingImage: null,
    sessionError: '',
    pendingContext: null,
    pendingFile: null,
  }
}
