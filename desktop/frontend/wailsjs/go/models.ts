export namespace command {
	
	export class Preset {
	    name: string;
	    description: string;
	    body: string;
	    path: string;
	    builtin: boolean;
	    image: string;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.body = source["body"];
	        this.path = source["path"];
	        this.builtin = source["builtin"];
	        this.image = source["image"];
	    }
	}

}

export namespace config {
	
	export class MCPServerConfig {
	    name: string;
	    command?: string[];
	    cwd?: string;
	    environment?: Record<string, string>;
	    url?: string;
	    headers?: Record<string, string>;
	    timeout_ms?: number;
	    disabled?: boolean;
	    for: string[];
	
	    static createFrom(source: any = {}) {
	        return new MCPServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.cwd = source["cwd"];
	        this.environment = source["environment"];
	        this.url = source["url"];
	        this.headers = source["headers"];
	        this.timeout_ms = source["timeout_ms"];
	        this.disabled = source["disabled"];
	        this.for = source["for"];
	    }
	}

}

export namespace main {
	
	export class AgentPage {
	    url: string;
	    title: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.title = source["title"];
	        this.time = source["time"];
	    }
	}
	export class Artifact {
	    name: string;
	    path: string;
	    sessionId?: string;
	    size: number;
	    modified: string;
	    root: string;
	
	    static createFrom(source: any = {}) {
	        return new Artifact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.sessionId = source["sessionId"];
	        this.size = source["size"];
	        this.modified = source["modified"];
	        this.root = source["root"];
	    }
	}
	export class Chair {
	    name: string;
	    description: string;
	    tools: string[];
	    builtin: boolean;
	    overrides?: boolean;
	    path?: string;
	    jobs: number;
	    lastUsed?: string;
	
	    static createFrom(source: any = {}) {
	        return new Chair(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.tools = source["tools"];
	        this.builtin = source["builtin"];
	        this.overrides = source["overrides"];
	        this.path = source["path"];
	        this.jobs = source["jobs"];
	        this.lastUsed = source["lastUsed"];
	    }
	}
	export class ChangedFile {
	    path: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	    }
	}
	export class ContextSlice {
	    key: string;
	    tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new ContextSlice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.tokens = source["tokens"];
	    }
	}
	export class ContextBreakdown {
	    usedTokens: number;
	    maxTokens: number;
	    slices: ContextSlice[];
	    measured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContextBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usedTokens = source["usedTokens"];
	        this.maxTokens = source["maxTokens"];
	        this.slices = this.convertValues(source["slices"], ContextSlice);
	        this.measured = source["measured"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class DayPoint {
	    day: string;
	    model: string;
	    promptTokens: number;
	    completionTokens: number;
	    cachedTokens: number;
	    cacheRows: number;
	
	    static createFrom(source: any = {}) {
	        return new DayPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.model = source["model"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.cachedTokens = source["cachedTokens"];
	        this.cacheRows = source["cacheRows"];
	    }
	}
	export class DeskFilter {
	    desks: string[];
	    exclude: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeskFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.desks = source["desks"];
	        this.exclude = source["exclude"];
	    }
	}
	export class DeskTab {
	    kind: string;
	    name: string;
	    path?: string;
	    url?: string;
	    mine: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeskTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.url = source["url"];
	        this.mine = source["mine"];
	    }
	}
	export class IdentityFile {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new IdentityFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class MCPServerInfo {
	    name: string;
	    command?: string[];
	    url?: string;
	    environment?: Record<string, string>;
	    headers?: Record<string, string>;
	    cwd?: string;
	    timeoutMs?: number;
	    disabled: boolean;
	    for: string[];
	    status: string;
	    tools: number;
	    err?: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPServerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.url = source["url"];
	        this.environment = source["environment"];
	        this.headers = source["headers"];
	        this.cwd = source["cwd"];
	        this.timeoutMs = source["timeoutMs"];
	        this.disabled = source["disabled"];
	        this.for = source["for"];
	        this.status = source["status"];
	        this.tools = source["tools"];
	        this.err = source["err"];
	    }
	}
	export class MCPTarget {
	    id: string;
	    name: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	    }
	}
	export class ModelInfo {
	    provider: string;
	    modelName: string;
	    thinkLevel: string;
	    approvalMode: string;
	    contextUsed: number;
	    contextMax: number;
	    wireFormat: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.modelName = source["modelName"];
	        this.thinkLevel = source["thinkLevel"];
	        this.approvalMode = source["approvalMode"];
	        this.contextUsed = source["contextUsed"];
	        this.contextMax = source["contextMax"];
	        this.wireFormat = source["wireFormat"];
	        this.warning = source["warning"];
	    }
	}
	export class PendingChange {
	    id: number;
	    kind: string;
	    scope: string;
	    target: string;
	    op: string;
	    before: string;
	    body: string;
	    reason: string;
	    evidence: string;
	    source: string;
	    state: string;
	    createdAt: string;
	    decidedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.scope = source["scope"];
	        this.target = source["target"];
	        this.op = source["op"];
	        this.before = source["before"];
	        this.body = source["body"];
	        this.reason = source["reason"];
	        this.evidence = source["evidence"];
	        this.source = source["source"];
	        this.state = source["state"];
	        this.createdAt = source["createdAt"];
	        this.decidedAt = source["decidedAt"];
	    }
	}
	export class ProjectMeta {
	    key: string;
	    name: string;
	    rootPath: string;
	    openedAt: string;
	    snippet?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.rootPath = source["rootPath"];
	        this.openedAt = source["openedAt"];
	        this.snippet = source["snippet"];
	    }
	}
	export class ProjectStatus {
	    name: string;
	    path: string;
	    branch: string;
	    focused: boolean;
	    governanceFile: string;
	    governanceLoaded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProjectStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.branch = source["branch"];
	        this.focused = source["focused"];
	        this.governanceFile = source["governanceFile"];
	        this.governanceLoaded = source["governanceLoaded"];
	    }
	}
	export class ReceivedJob {
	    id: number;
	    chair: string;
	    sessionId: string;
	    request: string;
	    answer: string;
	    toolSeq?: string;
	    toolCount: number;
	    durationMs: number;
	    outcome: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new ReceivedJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.chair = source["chair"];
	        this.sessionId = source["sessionId"];
	        this.request = source["request"];
	        this.answer = source["answer"];
	        this.toolSeq = source["toolSeq"];
	        this.toolCount = source["toolCount"];
	        this.durationMs = source["durationMs"];
	        this.outcome = source["outcome"];
	        this.time = source["time"];
	    }
	}
	export class SessionVariant {
	    text: string;
	    reasoning?: string;
	    thinkSecs?: number;
	    parts?: turn.TurnPart[];
	
	    static createFrom(source: any = {}) {
	        return new SessionVariant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.reasoning = source["reasoning"];
	        this.thinkSecs = source["thinkSecs"];
	        this.parts = this.convertValues(source["parts"], turn.TurnPart);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RegenerateResult {
	    text: string;
	    parts?: turn.TurnPart[];
	    variants: SessionVariant[];
	    active: number;
	    reverted?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RegenerateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.parts = this.convertValues(source["parts"], turn.TurnPart);
	        this.variants = this.convertValues(source["variants"], SessionVariant);
	        this.active = source["active"];
	        this.reverted = source["reverted"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunBlockResult {
	    output: string;
	    success: boolean;
	    durationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new RunBlockResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output = source["output"];
	        this.success = source["success"];
	        this.durationMs = source["durationMs"];
	    }
	}
	export class SessionMessage {
	    id?: number;
	    role: string;
	    text: string;
	    time: string;
	    rating?: string;
	    reasoning?: string;
	    thinkSecs?: number;
	    variants?: SessionVariant[];
	    active?: number;
	    parts?: turn.TurnPart[];
	
	    static createFrom(source: any = {}) {
	        return new SessionMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.time = source["time"];
	        this.rating = source["rating"];
	        this.reasoning = source["reasoning"];
	        this.thinkSecs = source["thinkSecs"];
	        this.variants = this.convertValues(source["variants"], SessionVariant);
	        this.active = source["active"];
	        this.parts = this.convertValues(source["parts"], turn.TurnPart);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionMeta {
	    id: string;
	    title: string;
	    updatedAt: string;
	    mode?: string;
	    agent?: string;
	    snippet?: string;
	    space?: string;
	    projectKey?: string;
	    projectName?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.updatedAt = source["updatedAt"];
	        this.mode = source["mode"];
	        this.agent = source["agent"];
	        this.snippet = source["snippet"];
	        this.space = source["space"];
	        this.projectKey = source["projectKey"];
	        this.projectName = source["projectName"];
	    }
	}
	
	export class ShellProfile {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ShellProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class SignInPrompt {
	    provider: string;
	    kind: string;
	    url: string;
	    user_code?: string;
	    verification_uri?: string;
	
	    static createFrom(source: any = {}) {
	        return new SignInPrompt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.kind = source["kind"];
	        this.url = source["url"];
	        this.user_code = source["user_code"];
	        this.verification_uri = source["verification_uri"];
	    }
	}
	export class SkillInfo {
	    name: string;
	    description: string;
	    source: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.source = source["source"];
	        this.category = source["category"];
	    }
	}
	export class Space {
	    name: string;
	    path: string;
	    contextPath: string;
	    contextFiles: string[];
	    chats: number;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Space(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.contextPath = source["contextPath"];
	        this.contextFiles = source["contextFiles"];
	        this.chats = source["chats"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class SpeechDirInfo {
	    path: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechDirInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.label = source["label"];
	    }
	}
	export class SpeechModelInfo {
	    path: string;
	    name: string;
	    sizeMB: number;
	    store: string;
	    managed: boolean;
	    active: boolean;
	    where: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.sizeMB = source["sizeMB"];
	        this.store = source["store"];
	        this.managed = source["managed"];
	        this.active = source["active"];
	        this.where = source["where"];
	    }
	}
	export class TaskChip {
	    id: string;
	    title: string;
	    tldr: string;
	    prompt: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskChip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.tldr = source["tldr"];
	        this.prompt = source["prompt"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ToolCounts {
	    builtin: number;
	    workbench: number;
	    mcp: number;
	    skill: number;
	
	    static createFrom(source: any = {}) {
	        return new ToolCounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.builtin = source["builtin"];
	        this.workbench = source["workbench"];
	        this.mcp = source["mcp"];
	        this.skill = source["skill"];
	    }
	}
	export class TreeNode {
	    label: string;
	    path: string;
	    kind: string;
	    depth: number;
	    status?: string;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new TreeNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.depth = source["depth"];
	        this.status = source["status"];
	        this.icon = source["icon"];
	    }
	}
	export class TurnReply {
	    text: string;
	    parts?: turn.TurnPart[];
	    messageId?: number;
	
	    static createFrom(source: any = {}) {
	        return new TurnReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.parts = this.convertValues(source["parts"], turn.TurnPart);
	        this.messageId = source["messageId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UndoResult {
	    files: string[];
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new UndoResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.reason = source["reason"];
	    }
	}
	export class UsageRow {
	    model: string;
	    promptTokens: number;
	    completionTokens: number;
	    cachedTokens: number;
	    uncachedTokens: number;
	    cacheRows: number;
	    calls: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.cachedTokens = source["cachedTokens"];
	        this.uncachedTokens = source["uncachedTokens"];
	        this.cacheRows = source["cacheRows"];
	        this.calls = source["calls"];
	    }
	}
	export class UsageTotals {
	    promptTokens: number;
	    completionTokens: number;
	    cachedTokens: number;
	    uncachedTokens: number;
	    cacheRows: number;
	    calls: number;
	    sessions: number;
	    messages: number;
	    activeDays: number;
	    currentStreak: number;
	    topModel: string;
	    topModelShare: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageTotals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.cachedTokens = source["cachedTokens"];
	        this.uncachedTokens = source["uncachedTokens"];
	        this.cacheRows = source["cacheRows"];
	        this.calls = source["calls"];
	        this.sessions = source["sessions"];
	        this.messages = source["messages"];
	        this.activeDays = source["activeDays"];
	        this.currentStreak = source["currentStreak"];
	        this.topModel = source["topModel"];
	        this.topModelShare = source["topModelShare"];
	    }
	}
	export class UsageStats {
	    today: UsageRow[];
	    week: UsageRow[];
	    all: UsageRow[];
	    totals: UsageTotals;
	    daily: DayPoint[];
	    heatmap: DayPoint[];
	
	    static createFrom(source: any = {}) {
	        return new UsageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.today = this.convertValues(source["today"], UsageRow);
	        this.week = this.convertValues(source["week"], UsageRow);
	        this.all = this.convertValues(source["all"], UsageRow);
	        this.totals = this.convertValues(source["totals"], UsageTotals);
	        this.daily = this.convertValues(source["daily"], DayPoint);
	        this.heatmap = this.convertValues(source["heatmap"], DayPoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class WorkspaceFolder {
	    path: string;
	    name: string;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceFolder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.missing = source["missing"];
	    }
	}

}

export namespace mode {
	
	export class Mode {
	    name: string;
	    description: string;
	    categories?: string[];
	    tools?: string[];
	    deny?: string[];
	    mcp?: string[];
	    chairs?: string[];
	    dispatch?: string[];
	    prompt: string;
	    path?: string;
	    builtin: boolean;
	    overrides?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Mode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.categories = source["categories"];
	        this.tools = source["tools"];
	        this.deny = source["deny"];
	        this.mcp = source["mcp"];
	        this.chairs = source["chairs"];
	        this.dispatch = source["dispatch"];
	        this.prompt = source["prompt"];
	        this.path = source["path"];
	        this.builtin = source["builtin"];
	        this.overrides = source["overrides"];
	    }
	}

}

export namespace model {
	
	export class GuideTopic {
	    id: string;
	    question: string;
	
	    static createFrom(source: any = {}) {
	        return new GuideTopic(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.question = source["question"];
	    }
	}

}

export namespace oauth {
	
	export class Method {
	    provider: string;
	    label: string;
	    kind: string;
	    risk: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Method(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.risk = source["risk"];
	        this.note = source["note"];
	    }
	}
	export class Status {
	    provider: string;
	    signed_in: boolean;
	    label?: string;
	    account?: string;
	    expires_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.signed_in = source["signed_in"];
	        this.label = source["label"];
	        this.account = source["account"];
	        this.expires_at = source["expires_at"];
	    }
	}

}

export namespace ooxml {
	
	export class SheetPreview {
	    name: string;
	    rows: string[][];
	    truncated: boolean;
	    totalRows: number;
	
	    static createFrom(source: any = {}) {
	        return new SheetPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.rows = source["rows"];
	        this.truncated = source["truncated"];
	        this.totalRows = source["totalRows"];
	    }
	}
	export class WorkbookPreview {
	    sheets: SheetPreview[];
	
	    static createFrom(source: any = {}) {
	        return new WorkbookPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sheets = this.convertValues(source["sheets"], SheetPreview);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace skill {
	
	export class DiscoveredSkill {
	    name: string;
	    description: string;
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscoveredSkill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.dir = source["dir"];
	    }
	}

}

export namespace subagent {
	
	export class Profile {
	    name: string;
	    description: string;
	    model?: string;
	    tools?: string[];
	    deny?: string[];
	    steps?: number;
	    desk?: string;
	    prompt: string;
	    path?: string;
	    builtin: boolean;
	    overrides?: boolean;
	    invalid?: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.model = source["model"];
	        this.tools = source["tools"];
	        this.deny = source["deny"];
	        this.steps = source["steps"];
	        this.desk = source["desk"];
	        this.prompt = source["prompt"];
	        this.path = source["path"];
	        this.builtin = source["builtin"];
	        this.overrides = source["overrides"];
	        this.invalid = source["invalid"];
	    }
	}

}

export namespace turn {
	
	export class ToolPart {
	    ref?: string;
	    name: string;
	    subject?: string;
	    agent?: string;
	    brief?: string;
	    agentKind?: string;
	    ok: boolean;
	    error?: string;
	    secs?: number;
	    added?: number;
	    removed?: number;
	    artifacts?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ToolPart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.name = source["name"];
	        this.subject = source["subject"];
	        this.agent = source["agent"];
	        this.brief = source["brief"];
	        this.agentKind = source["agentKind"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.secs = source["secs"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.artifacts = source["artifacts"];
	    }
	}
	export class TurnPart {
	    kind: string;
	    text?: string;
	    secs?: number;
	    tool?: ToolPart;
	
	    static createFrom(source: any = {}) {
	        return new TurnPart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.secs = source["secs"];
	        this.tool = this.convertValues(source["tool"], ToolPart);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace update {
	
	export class Status {
	    current: string;
	    latest: string;
	    available: boolean;
	    disabled: boolean;
	    channel: string;
	    hint: string;
	    url: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.available = source["available"];
	        this.disabled = source["disabled"];
	        this.channel = source["channel"];
	        this.hint = source["hint"];
	        this.url = source["url"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

