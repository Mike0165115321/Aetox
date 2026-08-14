// Test double for ../../wailsjs/go/main/App — every binding is a vi.fn so a
// test can assert calls or override return values. Defaults are the shapes
// components need at mount (arrays/objects, not undefined) so any page can
// render without per-test setup.
import { vi } from 'vitest'

// Variadic on purpose: the real bindings take arguments, and a zero-arg mock
// types `mock.calls[0]` as an empty tuple — so a test asserting what was sent
// (`mock.calls[0][0]`) fails to type-check even though it passes at runtime.
const arr = () => vi.fn(async (..._args: any[]) => [] as any[])
const str = () => vi.fn(async (..._args: any[]) => '')
const boolFn = (v: boolean) => vi.fn(async (..._args: any[]) => v)
const noop = () => vi.fn(async (..._args: any[]) => undefined)

export const AddMCPServer = noop()
export const BrowserBack = noop()
export const BrowserClickRef = noop()
export const BrowserClose = noop()
export const BrowserForward = noop()
export const BrowserGetText = str()
export const BrowserNavigate = noop()
export const BrowserOpen = noop()
export const BrowserOpenDevTools = noop()
export const BrowserReload = noop()
export const BrowserSetBounds = noop()
export const BrowserSetVisible = noop()
export const BrowserSetZoom = noop()
export const BrowserTypeRef = noop()
export const CancelTurn = noop()
export const Interject = noop()
export const AnswerUserQuestion = noop()
export const AddWorkspaceFolder = arr()
export const ClearProjectFocus = noop()
export const CloseAllBrowserTabs = noop()
export const CommandHistory = arr()
export const AppVersion = vi.fn(async () => '0.8.4')
// Defaults to "checked, nothing new": the About page has to render before any
// button is pressed, and a mock that claimed an update would make every other
// Settings test assert against a banner it never asked for.
export const CheckForUpdate = vi.fn(async () => ({
  current: '0.8.4', latest: '0.8.4', available: false, disabled: false,
  channel: 'portable', hint: '', url: 'https://example.invalid/releases', checkedAt: '', canAuto: false,
}) as any)
export const ApplyUpdate = noop()
export const CurrentSessionID = str()
export const DismissTaskChip = noop()
export const DeleteIdentityFile = noop()
export const DeleteSession = noop()
export const EnabledProviders = arr()
export const GetContextBreakdown = vi.fn(async () => ({}))
const modelInfo = () => ({
  provider: 'aetox', modelName: 'test', thinkLevel: '', approval: 'ask',
  providers: [], models: [], thinkLevels: [], hasKey: true, status: '', wireFormat: '',
})
export const GetModelInfo = vi.fn(async () => modelInfo())
export const GetProjectStatus = vi.fn(async () => ({ focused: false, name: '', root: '' }))
export const GitChangedFiles = arr()
export const GuideTopics = arr()
export const HasAPIKey = boolFn(false)
// Sign-in: the default is "this provider offers none", so Settings renders the
// API-key path unless a test opts a provider into a sign-in.
export const SignInMethods = arr()
export const SignInStatus = vi.fn(async (provider: string) => ({ provider, signed_in: false }))
export const StartSignIn = vi.fn(async (provider: string) => ({ provider, kind: 'browser', url: '' }))
export const CompleteSignIn = vi.fn(async (..._args: any[]) => modelInfo())
export const CancelSignIn = noop()
export const ImportableSignIns = arr()
export const ImportSignIn = vi.fn(async (..._args: any[]) => modelInfo())
export const SignOut = vi.fn(async (..._args: any[]) => modelInfo())
// Connections: the catalog with nothing connected and no token in the
// environment, so the page renders the connect form unless a test says
// otherwise. `for: []` with configured:false is what an unplaced connection
// looks like — carried by every desk, and not drawn as "off".
export const Connections = vi.fn(async () => [
  {
    id: 'github', label: 'GitHub', kind: 'token',
    token_url: 'https://github.com/settings/tokens/new',
    connected: false, env_override: false, for: [], configured: false,
    tools: ['github', 'plugin_install'],
  },
])
export const ConnectAccount = vi.fn(async (..._args: any[]) => ({ login: 'mike', scopes: ['repo'] }))
export const VerifyConnection = vi.fn(async (_id: string) => ({ login: 'mike', scopes: ['repo'] }))
export const EnginesFor = vi.fn(async (..._a: any[]) => [] as any[])
export const UseEngine = noop()
export const SetConnectionStartCommand = noop()
export const StartConnectionServer = noop()
export const CheckConnectionServer = vi.fn(async (_id: string) => true)
export const SetConnectionTargets = noop()
export const DisconnectAccount = noop()
export const InstallSkillFromGitHub = str()
export const ListSubagentProfiles = arr()
export const ReadSubagentProfile = str()
export const SaveSubagentProfile = noop()
export const DeleteSubagentProfile = noop()
export const SetSubagentModel = noop()
// The agent editor's เอื้อมถึงอะไร / ความรู้ / เปิดบทสนทนา panels. Empty by
// default: a test that cares about one of them says so itself.
export const AgentSkills = arr()
export const AgentNeeds = arr()
export const OpenAgentSkillsFolder = noop()
export const ListAllSessions = arr()
export const ListSessionsForDoor = arr()
export const SearchSessionsForDoor = arr()
export const ListPromptPresets = arr()
export const ListTools = arr()
export const ListExternalSkills = arr()
// The Skills page reads its folder and its scan errors from the engine rather
// than naming a path itself — two of the three it used to name were wrong.
export const SkillsDir = vi.fn(async () => 'C:/Users/x/.aetox/skills')
export const SkillScanIssues = arr()
export const OpenSkillsFolder = noop()
export const InstallSkillFromZip = str()
export const ListIdentityFiles = arr()
export const ListMCPServers = arr()
export const ListModelsForProvider = arr()
export const ListSessions = arr()
export const ListTaskChips = arr()
export const BackgroundTasks = arr()
export const ListPendingChanges = arr()
export const ListDecidedChanges = arr()
export const LearningEnabled = boolFn(true)
export const SetLearningEnabled = noop()
export const ApprovePendingChange = noop()
export const RejectPendingChange = noop()
export const LearnedMemory = str()
export const LearnedEntries = arr()
export const SaveLearnedEntry = noop()
export const OpenMemoryFolder = noop()
export const PendingLearnedCount = vi.fn(async (..._args: any[]) => 0)
export const RateTurn = noop()
export const TurnRating = str()
export const ListSkills = arr()
export const ListSpeechModels = arr()
export const SetSpeechModel = noop()
export const SpeechStatus = str()
export const RevealSpeechModel = noop()
export const SpeechModelDirs = arr()
export const OpenSpeechModelDir = noop()
export const LoadSession = noop()
export const LoadSessionAnyProject = noop()
export const ModelStatus = str()
export const NewSession = str()
// The five buttons (COMPANY.md §2). Defaults are the empty office of a fresh
// install: a roster and a feed with nothing in them, which every page has to
// render before it renders anything else.
export const NewSessionAt = str()
export const NewChairSession = str()
// โปรเจกต์ (§90): the room's own session door, plus the engine's answer to
// "which project is the open chat in".
export const NewSessionInSpace = str()
export const CurrentSpace = str()
export const Spaces = arr()
export const SessionsInSpace = arr()
export const CreateSpace = noop()
export const OpenSpaceFolder = noop()
export const AddSpaceContext = arr()
export const RemoveSpaceContext = arr()
export const SessionMode = str()
export const SessionAgent = str()
export const SessionTranscript = arr()
// Idle by default: only a test about the mid-turn reload flips this on.
export const TurnInFlight = vi.fn(async (..._args: any[]) => ({ running: false, sessionId: '' }))
export const ListModes = arr()
export const ListChairs = arr()
// "This agent keeps no opening of its own" — the state that makes the window
// fall back to the four cards it draws for any colleague. A test about an
// agent's own cards overrides it.
export const ChairStarters = vi.fn(async (..._args: any[]) => ({ headline: '', cards: [] as any[] }))
export const SaveChairStarters = noop()
// Which of STARTERS.md / STARTERS.<lang>.md the editor is writing. The base
// file, matching the Thai default the tests run in.
export const ChairStartersFile = vi.fn(async (..._args: any[]) => 'STARTERS.md')
export const ListReceivedJobs = arr()
export const ListSessionsAt = arr()
export const ListArtifacts = arr()
// The gallery asks for one range at a time and gets back {files, range, total}.
// Defaults to an empty week so a test that does not care about the range still
// renders; the tests that do care set files and let `range` echo what was asked.
export const ListArtifactsIn = vi.fn(async (want: string = 'week') => ({
  files: [] as any[], range: want, total: 0,
}))
export const OpenArtifact = noop()
export const DeleteArtifact = noop()
// "Nothing worth drawing" is the right default: a card falls back to its icon,
// so every gallery test renders without having to describe a file's insides.
export const ArtifactPreview = vi.fn(async (..._args: any[]) => ({ kind: 'none' }) as any)
export const OpenAgentsFolder = noop()
export const SaveAgentProfile = noop()
export const OpenSubagentsFolder = noop()
export const OpenPromptsFolder = noop()
export const OpenProjectFolder = noop()
export const OpenProjectPath = noop()
export const PickAttachment = str()
export const PickAttachmentImage = str()
export const ProjectTree = arr()
export const ProviderBaseURL = str()
export const ProviderBaseURLIsCustom = boolFn(false)
export const ProviderWireFormats = arr()
export const TestProviderConnection = str()
export const OpenFileExternally = noop()
export const FileStillThere = boolFn(true)
export const ReadWorkbook = vi.fn(async () => ({ sheets: [] }))
export const ReadFile = str()
export const ReadIdentityFile = str()
export const ReadImageDataURL = str()
export const RecentAgentPages = arr()
export const RecentDebugLog = arr()
export const RecentProjects = arr()
export const RefreshSkills = noop()
export const RelativizePath = str()
export const RemoveExternalSkill = noop()
export const RemoveMCPServer = noop()
export const RemoveWorkspaceFolder = arr()
export const MCPConfigPath = vi.fn(async () => 'C:/Users/x/AppData/Roaming/aetox/mcp-servers.json')
export const OpenMCPFolder = noop()
export const RequiresAPIKey = boolFn(true)
export const SaveChatFile = str()
export const SaveChatImage = str()
export const SaveChatImageData = str()
export const SaveIdentityFile = noop()
export const SaveMCPServer = noop()
export const SearchAllSessions = arr()
export const SearchSessions = arr()
export const RunChatCommand = vi.fn(async (..._args: any[]) => ({ output: '', success: true, durationMs: 0 }))
// A finished turn is { text, parts } now, not a bare string — the sequence is
// what the bubble draws. The default is that shape so any test that does not
// care about the reply still gets something the store can read.
const turnReply = () => vi.fn(async (..._args: any[]) =>
  ({ text: '' } as { text: string; parts?: any[]; messageId?: number }))
export const SendMessage = turnReply()
// Re-running a turn. RegenerateReply/SwitchVariant hand back the whole answer
// list, so their default is the shape the store destructures, not an empty one.
export const RetryFailedTurn = turnReply()
export const ResendEdited = turnReply()
const rerun = () => vi.fn(async (..._args: any[]) =>
  ({ text: '', variants: [] as any[], active: 0 } as { text: string; variants: any[]; active: number; reverted?: string[] }))
export const RegenerateReply = rerun()
export const SwitchVariant = rerun()
export const PendingUndo = arr()
export const UndoLastTurn = vi.fn(async (..._args: any[]) => ({ files: [] as string[] }))
export const SetAPIKey = vi.fn(async () => modelInfo())
export const SetProviderEnabled = arr()
export const SetProviderWireFormat = vi.fn(async () => modelInfo())
export const SupportedProviders = arr()
export const SupportedThinkLevels = arr()
export const SwitchApprovalMode = vi.fn(async () => modelInfo())
export const SwitchModel = vi.fn(async () => modelInfo())
export const SetUILocale = noop()
export const SwitchProvider = vi.fn(async () => modelInfo())
export const SwitchThinkLevel = vi.fn(async () => modelInfo())
export const TerminalAttach = str()
export const TerminalClose = noop()
export const TerminalResize = noop()
export const TerminalShells = arr()
export const TerminalStart = str()
export const TerminalWrite = noop()
export const TestMCPServer = vi.fn(async () => ({ name: '', command: [], status: 'connected', disabled: false, tools: 0 }))
export const SavePromptPreset = noop()
export const DeletePromptPreset = noop()
export const PickPresetImage = str()
export const RemovePresetImage = noop()
export const ToggleMCPServer = noop()
// The desks and the team, shared by the MCP page, the connections page and the
// agent editor. Empty here and filled per test: one test needs a target whose
// name matches the agent it opens, and a fixture that lives only in this file
// cannot be varied without leaking the variation into every test after it.
export const PlacementTargets = arr()
export const SetMCPServerTargets = noop()
export const ToolCounts = vi.fn(async () => ({ builtin: 0, workbench: 0, skill: 0, mcp: 0 }))
export const UsageStats = vi.fn(async () => ({
  today: [], week: [], all: [], daily: [] as any[], heatmap: [] as any[],
  totals: {
    promptTokens: 0, completionTokens: 0, cachedTokens: 0, uncachedTokens: 0,
    cacheRows: 0, calls: 0, sessions: 0, messages: 0,
    activeDays: 0, currentStreak: 0, topModel: '', topModelShare: 0,
  },
}))
export const WorkbenchTabsChanged = noop()
export const WorkspaceFolders = arr()
export const WriteFile = noop()
