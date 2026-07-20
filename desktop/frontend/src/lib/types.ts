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
  active?: boolean
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
  active?: boolean
  snippet?: string
}

export interface ProjectInfo {
  name: string
  path: string
  branch: string
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
}

export interface ChatMessage {
  role: 'user' | 'agent'
  text: string
  time: string
  /** optional badge, e.g. "Thinking (low)" */
  tag?: string
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

export type DiffKind = 'ctx' | 'add' | 'del'

export interface DiffLine {
  ln: number
  text: string
  kind: DiffKind
}

export interface DiffView {
  file: string
  hunk: string
  lines: DiffLine[]
}

export type TestState = 'running' | 'pass' | 'fail'

export interface TestCase {
  name: string
  state: TestState
}

export interface TestRun {
  command: string
  cases: TestCase[]
}

export interface ChangedFile {
  path: string
  status: GitStatus
}

export interface CockpitState {
  project: ProjectInfo
  tree: TreeNode[]
  sessions: Session[]
  model: ModelStatus
  chat: ChatMessage[]
  task: TaskState
  changedFiles: ChangedFile[]
  diff: DiffView
  test: TestRun
  commandHistory: string[]
  openFiles: OpenFile[]
  /** 'chat' or an open file's path — which tab the main panel currently shows. */
  activeView: string
}

/** A blank, well-formed state so the UI renders before the source hydrates. */
export function emptyCockpitState(): CockpitState {
  return {
    project: { name: '', path: '', branch: '', extraBranches: 0, governanceFile: '', governanceLoaded: false },
    tree: [],
    sessions: [],
    model: { provider: '', modelName: '', thinkLevel: '', contextUsed: 0, contextMax: 0, approval: 'ask' },
    chat: [],
    task: { elapsed: '', steps: [] },
    changedFiles: [],
    diff: { file: '', hunk: '', lines: [] },
    test: { command: '', cases: [] },
    commandHistory: [],
    openFiles: [],
    activeView: 'chat',
  }
}
