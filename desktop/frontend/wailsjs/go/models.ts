export namespace account {
	
	export class User {
	    id: string;
	    email?: string;
	    name?: string;
	    avatar_url?: string;
	    created_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.name = source["name"];
	        this.avatar_url = source["avatar_url"];
	        this.created_at = source["created_at"];
	    }
	}

}

export namespace capability {
	
	export class Status {
	    capability: string;
	    installed: boolean;
	    approx_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capability = source["capability"];
	        this.installed = source["installed"];
	        this.approx_bytes = source["approx_bytes"];
	    }
	}

}

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
	    tools?: string[];
	    source?: string;
	
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
	        this.tools = source["tools"];
	        this.source = source["source"];
	    }
	}

}

export namespace connect {
	
	export class Account {
	    login: string;
	    name?: string;
	    scopes?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.name = source["name"];
	        this.scopes = source["scopes"];
	    }
	}
	export class Status {
	    id: string;
	    label: string;
	    kind: string;
	    token_url?: string;
	    connected: boolean;
	    login?: string;
	    source?: string;
	    env_override: boolean;
	    for: string[];
	    configured: boolean;
	    tools: string[];
	    family?: string;
	    home_agent?: string;
	    default_agents?: string[];
	    needs_base_url: boolean;
	    base_url?: string;
	    base_url_hint?: string;
	    start_command?: string;
	    reachable?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.token_url = source["token_url"];
	        this.connected = source["connected"];
	        this.login = source["login"];
	        this.source = source["source"];
	        this.env_override = source["env_override"];
	        this.for = source["for"];
	        this.configured = source["configured"];
	        this.tools = source["tools"];
	        this.family = source["family"];
	        this.home_agent = source["home_agent"];
	        this.default_agents = source["default_agents"];
	        this.needs_base_url = source["needs_base_url"];
	        this.base_url = source["base_url"];
	        this.base_url_hint = source["base_url_hint"];
	        this.start_command = source["start_command"];
	        this.reachable = source["reachable"];
	    }
	}

}

export namespace github {
	
	export class CheckRun {
	    name: string;
	    status: string;
	    conclusion: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.conclusion = source["conclusion"];
	    }
	}
	export class PRFile {
	    path: string;
	    status: string;
	    additions: number;
	    deletions: number;
	    patch: string;
	
	    static createFrom(source: any = {}) {
	        return new PRFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	        this.patch = source["patch"];
	    }
	}
	export class PullRequest {
	    number: number;
	    title: string;
	    state: string;
	    draft: boolean;
	    body: string;
	    author: string;
	    headRef: string;
	    headSHA: string;
	    baseRef: string;
	    mergeable?: boolean;
	    mergeableState: string;
	    additions: number;
	    deletions: number;
	    changedFiles: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new PullRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.title = source["title"];
	        this.state = source["state"];
	        this.draft = source["draft"];
	        this.body = source["body"];
	        this.author = source["author"];
	        this.headRef = source["headRef"];
	        this.headSHA = source["headSHA"];
	        this.baseRef = source["baseRef"];
	        this.mergeable = source["mergeable"];
	        this.mergeableState = source["mergeableState"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	        this.changedFiles = source["changedFiles"];
	        this.url = source["url"];
	    }
	}

}

export namespace main {
	
	export class AccountState {
	    configured: boolean;
	    signed_in: boolean;
	    user: account.User;
	    display: string;
	    providers: string[];
	    server: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.signed_in = source["signed_in"];
	        this.user = this.convertValues(source["user"], account.User);
	        this.display = source["display"];
	        this.providers = source["providers"];
	        this.server = source["server"];
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
	export class Address {
	    url: string;
	    fallback: string;
	    query: string;
	    searchUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Address(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fallback = source["fallback"];
	        this.query = source["query"];
	        this.searchUrl = source["searchUrl"];
	    }
	}
	export class AgentGate {
	    blocked: boolean;
	    kind?: string;
	    capability?: string;
	    missing: string[];
	
	    static createFrom(source: any = {}) {
	        return new AgentGate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocked = source["blocked"];
	        this.kind = source["kind"];
	        this.capability = source["capability"];
	        this.missing = source["missing"];
	    }
	}
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
	export class AgentSkillInfo {
	    name: string;
	    description: string;
	    bundled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentSkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.bundled = source["bundled"];
	    }
	}
	export class Artifact {
	    name: string;
	    path: string;
	    sessionId?: string;
	    size: number;
	    modified: string;
	    root: string;
	    folder?: string;
	
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
	        this.folder = source["folder"];
	    }
	}
	export class ArtifactPage {
	    files: Artifact[];
	    range: string;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], Artifact);
	        this.range = source["range"];
	        this.total = source["total"];
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
	export class ArtifactPreview {
	    kind: string;
	    text?: string;
	    dataUrl?: string;
	    rows?: string[][];
	    sheet?: string;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.dataUrl = source["dataUrl"];
	        this.rows = source["rows"];
	        this.sheet = source["sheet"];
	    }
	}
	export class BackgroundPhase {
	    title: string;
	    planned: number;
	    done: number;
	    failed: number;
	    running: number;
	    waiting: number;
	    tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new BackgroundPhase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.planned = source["planned"];
	        this.done = source["done"];
	        this.failed = source["failed"];
	        this.running = source["running"];
	        this.waiting = source["waiting"];
	        this.tokens = source["tokens"];
	    }
	}
	export class BackgroundRun {
	    id: string;
	    name: string;
	    brief?: string;
	    startedAt: string;
	    running: boolean;
	    tokens: number;
	    phases: BackgroundPhase[];
	
	    static createFrom(source: any = {}) {
	        return new BackgroundRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.brief = source["brief"];
	        this.startedAt = source["startedAt"];
	        this.running = source["running"];
	        this.tokens = source["tokens"];
	        this.phases = this.convertValues(source["phases"], BackgroundPhase);
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
	export class BackgroundTask {
	    id: string;
	    agent: string;
	    label: string;
	    startedAt: string;
	    toolCalls: number;
	    model?: string;
	    tokens: number;
	    run?: string;
	    phase?: string;
	    tokensIn: number;
	    tokensOut: number;
	    cachedIn: number;
	    cacheReported: boolean;
	    state: string;
	    elapsedMs?: number;
	    question?: string;
	    collected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BackgroundTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.agent = source["agent"];
	        this.label = source["label"];
	        this.startedAt = source["startedAt"];
	        this.toolCalls = source["toolCalls"];
	        this.model = source["model"];
	        this.tokens = source["tokens"];
	        this.run = source["run"];
	        this.phase = source["phase"];
	        this.tokensIn = source["tokensIn"];
	        this.tokensOut = source["tokensOut"];
	        this.cachedIn = source["cachedIn"];
	        this.cacheReported = source["cacheReported"];
	        this.state = source["state"];
	        this.elapsedMs = source["elapsedMs"];
	        this.question = source["question"];
	        this.collected = source["collected"];
	    }
	}
	export class BusyLayer {
	    id: string;
	    label: string;
	    note: string;
	    on: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BusyLayer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.note = source["note"];
	        this.on = source["on"];
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
	    icon: string;
	
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
	        this.icon = source["icon"];
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
	export class CompressReport {
	    files: number;
	    skipped: number;
	    before: number;
	    after: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CompressReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.skipped = source["skipped"];
	        this.before = source["before"];
	        this.after = source["after"];
	        this.error = source["error"];
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
	    cachedTokens: number;
	    sweptItems?: number;
	    sweptTokens?: number;
	    summaries?: number;
	
	    static createFrom(source: any = {}) {
	        return new ContextBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usedTokens = source["usedTokens"];
	        this.maxTokens = source["maxTokens"];
	        this.slices = this.convertValues(source["slices"], ContextSlice);
	        this.measured = source["measured"];
	        this.cachedTokens = source["cachedTokens"];
	        this.sweptItems = source["sweptItems"];
	        this.sweptTokens = source["sweptTokens"];
	        this.summaries = source["summaries"];
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
	export class Deck {
	    path: string;
	    name: string;
	    slides: number;
	    sessionId?: string;
	    modified: string;
	
	    static createFrom(source: any = {}) {
	        return new Deck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.slides = source["slides"];
	        this.sessionId = source["sessionId"];
	        this.modified = source["modified"];
	    }
	}
	export class DeckFormat {
	    id: string;
	    ext: string;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeckFormat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ext = source["ext"];
	        this.ready = source["ready"];
	    }
	}
	export class DeckPage {
	    decks: Deck[];
	    range: string;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new DeckPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decks = this.convertValues(source["decks"], Deck);
	        this.range = source["range"];
	        this.total = source["total"];
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
	export class DelegateWorker {
	    name: string;
	    for: string;
	    agent: boolean;
	    on: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DelegateWorker(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.for = source["for"];
	        this.agent = source["agent"];
	        this.on = source["on"];
	    }
	}
	export class DelegateReach {
	    off: boolean;
	    tokens: number;
	    workers: DelegateWorker[];
	
	    static createFrom(source: any = {}) {
	        return new DelegateReach(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.off = source["off"];
	        this.tokens = source["tokens"];
	        this.workers = this.convertValues(source["workers"], DelegateWorker);
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
	export class DelegateSettings {
	    agents: DelegateReach;
	    helpers: DelegateReach;
	    tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new DelegateSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agents = this.convertValues(source["agents"], DelegateReach);
	        this.helpers = this.convertValues(source["helpers"], DelegateReach);
	        this.tokens = source["tokens"];
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
	export class EditedFile {
	    path: string;
	    label: string;
	    dir?: string;
	    status: string;
	    gone: boolean;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new EditedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.label = source["label"];
	        this.dir = source["dir"];
	        this.status = source["status"];
	        this.gone = source["gone"];
	        this.time = source["time"];
	    }
	}
	export class EditPage {
	    files: EditedFile[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new EditPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], EditedFile);
	        this.total = source["total"];
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
	
	export class GitBranch {
	    name: string;
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitBranch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.current = source["current"];
	    }
	}
	export class GitFileChange {
	    path: string;
	    status: string;
	    added: number;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new GitFileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.added = source["added"];
	        this.removed = source["removed"];
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
	    allowed?: string[];
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
	        this.allowed = source["allowed"];
	        this.err = source["err"];
	    }
	}
	export class MemoryScopeInfo {
	    scope: string;
	    orphan: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MemoryScopeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.orphan = source["orphan"];
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
	export class ModelListing {
	    model: string;
	    input: number;
	    output: number;
	    priced: boolean;
	    free: boolean;
	    context: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.priced = source["priced"];
	        this.free = source["free"];
	        this.context = source["context"];
	    }
	}
	export class PRCreated {
	    number: number;
	    url: string;
	    base: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new PRCreated(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.url = source["url"];
	        this.base = source["base"];
	        this.error = source["error"];
	    }
	}
	export class PRRoom {
	    repo: string;
	    reason: string;
	    connected: boolean;
	    items: github.PullRequest[];
	
	    static createFrom(source: any = {}) {
	        return new PRRoom(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.reason = source["reason"];
	        this.connected = source["connected"];
	        this.items = this.convertValues(source["items"], github.PullRequest);
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
	export class PlacementTarget {
	    id: string;
	    name: string;
	    detail?: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new PlacementTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.detail = source["detail"];
	        this.kind = source["kind"];
	    }
	}
	export class PriceSource {
	    name: string;
	    fetched: string;
	
	    static createFrom(source: any = {}) {
	        return new PriceSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fetched = source["fetched"];
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
	export class ProviderAccount {
	    provider: string;
	    balance: model.Balance;
	    quotas: model.Quota[];
	    quotaKnown: boolean;
	    expectsQuota: boolean;
	    quotaFetched: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderAccount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.balance = this.convertValues(source["balance"], model.Balance);
	        this.quotas = this.convertValues(source["quotas"], model.Quota);
	        this.quotaKnown = source["quotaKnown"];
	        this.expectsQuota = source["expectsQuota"];
	        this.quotaFetched = source["quotaFetched"];
	        this.error = source["error"];
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
	export class ReadyRow {
	    id: string;
	    state: string;
	    where?: string;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadyRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.state = source["state"];
	        this.where = source["where"];
	        this.bytes = source["bytes"];
	    }
	}
	export class ReceivedJob {
	    id: number;
	    chair: string;
	    sessionId: string;
	    request: string;
	    brief: string;
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
	        this.brief = source["brief"];
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
	export class RemoteDevice {
	    id: string;
	    label: string;
	    pairedAt: string;
	    lastSeen: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.pairedAt = source["pairedAt"];
	        this.lastSeen = source["lastSeen"];
	    }
	}
	export class RemoteStatus {
	    running: boolean;
	    address: string;
	    subnet: string;
	    devices: RemoteDevice[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.address = source["address"];
	        this.subnet = source["subnet"];
	        this.devices = this.convertValues(source["devices"], RemoteDevice);
	        this.error = source["error"];
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
	export class RepoMapEdge {
	    from: number;
	    to: number;
	
	    static createFrom(source: any = {}) {
	        return new RepoMapEdge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class RepoMapNode {
	    path: string;
	    dir: string;
	    refs: number;
	    symbols: number;
	
	    static createFrom(source: any = {}) {
	        return new RepoMapNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.dir = source["dir"];
	        this.refs = source["refs"];
	        this.symbols = source["symbols"];
	    }
	}
	export class RepoMapGraph {
	    focused: boolean;
	    root?: string;
	    nodes: RepoMapNode[];
	    edges: RepoMapEdge[];
	    totalFiles: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RepoMapGraph(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.focused = source["focused"];
	        this.root = source["root"];
	        this.nodes = this.convertValues(source["nodes"], RepoMapNode);
	        this.edges = this.convertValues(source["edges"], RepoMapEdge);
	        this.totalFiles = source["totalFiles"];
	        this.error = source["error"];
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
	
	export class RestorePoint {
	    id: string;
	    at: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new RestorePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.label = source["label"];
	    }
	}
	export class RunBlockResult {
	    output: string;
	    success: boolean;
	    durationMs: number;
	    lines: number;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunBlockResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output = source["output"];
	        this.success = source["success"];
	        this.durationMs = source["durationMs"];
	        this.lines = source["lines"];
	        this.truncated = source["truncated"];
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
	    errorText?: string;
	
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
	        this.errorText = source["errorText"];
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
	
	export class ShellOption {
	    setting: string;
	    label: string;
	    selected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ShellOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.setting = source["setting"];
	        this.label = source["label"];
	        this.selected = source["selected"];
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
	export class Source {
	    kind: string;
	    label: string;
	    path: string;
	    dir?: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.path = source["path"];
	        this.dir = source["dir"];
	        this.time = source["time"];
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
	export class TTSVoiceInfo {
	    id: string;
	    name: string;
	    lang: string;
	    gender: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TTSVoiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.lang = source["lang"];
	        this.gender = source["gender"];
	        this.active = source["active"];
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
	export class ToolPart {
	    id: string;
	    title?: string;
	    includes?: string;
	    license?: string;
	    homepage?: string;
	    approxBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new ToolPart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.includes = source["includes"];
	        this.license = source["license"];
	        this.homepage = source["homepage"];
	        this.approxBytes = source["approxBytes"];
	    }
	}
	export class ToolInstallPlan {
	    capability: string;
	    parts: ToolPart[];
	    totalBytes: number;
	    dest: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolInstallPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capability = source["capability"];
	        this.parts = this.convertValues(source["parts"], ToolPart);
	        this.totalBytes = source["totalBytes"];
	        this.dest = source["dest"];
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
	export class TurnStatus {
	    running: boolean;
	    sessionId: string;
	    working: string[];
	
	    static createFrom(source: any = {}) {
	        return new TurnStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.sessionId = source["sessionId"];
	        this.working = source["working"];
	    }
	}
	export class UndoResult {
	    files: string[];
	    kept?: string[];
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new UndoResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.kept = source["kept"];
	        this.reason = source["reason"];
	    }
	}
	export class UsageRow {
	    model: string;
	    provider: string;
	    promptTokens: number;
	    completionTokens: number;
	    cachedTokens: number;
	    uncachedTokens: number;
	    cacheRows: number;
	    calls: number;
	    cost: number;
	    priced: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UsageRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.cachedTokens = source["cachedTokens"];
	        this.uncachedTokens = source["uncachedTokens"];
	        this.cacheRows = source["cacheRows"];
	        this.calls = source["calls"];
	        this.cost = source["cost"];
	        this.priced = source["priced"];
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
	    cost: number;
	    pricedCalls: number;
	    pricesFetched: string;
	
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
	        this.cost = source["cost"];
	        this.pricedCalls = source["pricedCalls"];
	        this.pricesFetched = source["pricesFetched"];
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
	
	export class VideoReadiness {
	    rows: ReadyRow[];
	    capabilities: string[];
	    missingBytes: number;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VideoReadiness(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], ReadyRow);
	        this.capabilities = source["capabilities"];
	        this.missingBytes = source["missingBytes"];
	        this.ready = source["ready"];
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
	export class VideoToolingStatus {
	    installed: boolean;
	    missingRequired: string[];
	
	    static createFrom(source: any = {}) {
	        return new VideoToolingStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.missingRequired = source["missingRequired"];
	    }
	}
	export class VoiceEngineInfo {
	    id: string;
	    label: string;
	    install: string;
	    active: boolean;
	    hasModels: boolean;
	    installCommand: string[];
	    models: string[];
	    activeModel: string;
	
	    static createFrom(source: any = {}) {
	        return new VoiceEngineInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.install = source["install"];
	        this.active = source["active"];
	        this.hasModels = source["hasModels"];
	        this.installCommand = source["installCommand"];
	        this.models = source["models"];
	        this.activeModel = source["activeModel"];
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
	export class deviceProfile {
	    name: string;
	    w: number;
	    h: number;
	    dpr: number;
	    mobile: boolean;
	
	    static createFrom(source: any = {}) {
	        return new deviceProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.w = source["w"];
	        this.h = source["h"];
	        this.dpr = source["dpr"];
	        this.mobile = source["mobile"];
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
	    connections?: string[];
	    chairs?: string[];
	    dispatch?: string[];
	    memory?: string;
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
	        this.connections = source["connections"];
	        this.chairs = source["chairs"];
	        this.dispatch = source["dispatch"];
	        this.memory = source["memory"];
	        this.prompt = source["prompt"];
	        this.path = source["path"];
	        this.builtin = source["builtin"];
	        this.overrides = source["overrides"];
	    }
	}

}

export namespace model {
	
	export class Quota {
	    window: string;
	    remainingPercent: number;
	    // Go type: time
	    resetAt: any;
	    // Go type: time
	    observedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Quota(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.window = source["window"];
	        this.remainingPercent = source["remainingPercent"];
	        this.resetAt = this.convertValues(source["resetAt"], null);
	        this.observedAt = this.convertValues(source["observedAt"], null);
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
	export class BalancePart {
	    label: string;
	    amount: number;
	
	    static createFrom(source: any = {}) {
	        return new BalancePart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.amount = source["amount"];
	    }
	}
	export class Balance {
	    kind: string;
	    hasAmount: boolean;
	    amount: number;
	    currency: string;
	    parts: BalancePart[];
	    sufficient: boolean;
	    quotas?: Quota[];
	    // Go type: time
	    fetchedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Balance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.hasAmount = source["hasAmount"];
	        this.amount = source["amount"];
	        this.currency = source["currency"];
	        this.parts = this.convertValues(source["parts"], BalancePart);
	        this.sufficient = source["sufficient"];
	        this.quotas = this.convertValues(source["quotas"], Quota);
	        this.fetchedAt = this.convertValues(source["fetchedAt"], null);
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
	    bundled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiscoveredSkill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.dir = source["dir"];
	        this.bundled = source["bundled"];
	    }
	}

}

export namespace subagent {
	
	export class Need {
	    kind: string;
	    id: string;
	    label: string;
	    reason: string;
	    one_of?: Need[];
	
	    static createFrom(source: any = {}) {
	        return new Need(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.label = source["label"];
	        this.reason = source["reason"];
	        this.one_of = this.convertValues(source["one_of"], Need);
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
	export class Profile {
	    name: string;
	    description: string;
	    model?: string;
	    tools?: string[];
	    deny?: string[];
	    steps?: number;
	    desk?: string;
	    icon?: string;
	    needs?: string[];
	    publisher?: string;
	    package?: string;
	    version?: string;
	    requires_app?: string;
	    prompt: string;
	    path?: string;
	    builtin: boolean;
	    overrides?: boolean;
	    notice?: string;
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
	        this.icon = source["icon"];
	        this.needs = source["needs"];
	        this.publisher = source["publisher"];
	        this.package = source["package"];
	        this.version = source["version"];
	        this.requires_app = source["requires_app"];
	        this.prompt = source["prompt"];
	        this.path = source["path"];
	        this.builtin = source["builtin"];
	        this.overrides = source["overrides"];
	        this.notice = source["notice"];
	        this.invalid = source["invalid"];
	    }
	}
	export class Requirement {
	    entry: string;
	    met: boolean;
	    options: Need[];
	
	    static createFrom(source: any = {}) {
	        return new Requirement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entry = source["entry"];
	        this.met = source["met"];
	        this.options = this.convertValues(source["options"], Need);
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
	export class Starter {
	    title: string;
	    prompt: string;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new Starter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.prompt = source["prompt"];
	        this.icon = source["icon"];
	    }
	}
	export class StarterSet {
	    headline?: string;
	    cards: Starter[];
	
	    static createFrom(source: any = {}) {
	        return new StarterSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.headline = source["headline"];
	        this.cards = this.convertValues(source["cards"], Starter);
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

export namespace turn {
	
	export class ToolPart {
	    ref?: string;
	    name: string;
	    subject?: string;
	    agent?: string;
	    brief?: string;
	    agentKind?: string;
	    delegation?: boolean;
	    ok: boolean;
	    error?: string;
	    secs?: number;
	    added?: number;
	    removed?: number;
	    count?: number;
	    range?: string;
	    problems?: number;
	    diff?: string;
	    artifacts?: string[];
	    proposalId?: number;
	
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
	        this.delegation = source["delegation"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.secs = source["secs"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.count = source["count"];
	        this.range = source["range"];
	        this.problems = source["problems"];
	        this.diff = source["diff"];
	        this.artifacts = source["artifacts"];
	        this.proposalId = source["proposalId"];
	    }
	}
	export class TurnPart {
	    kind: string;
	    text?: string;
	    demoted?: boolean;
	    secs?: number;
	    tool?: ToolPart;
	
	    static createFrom(source: any = {}) {
	        return new TurnPart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.demoted = source["demoted"];
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
	    publishedAt: string;
	    highlights?: string[];
	    canAuto: boolean;
	
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
	        this.publishedAt = source["publishedAt"];
	        this.highlights = source["highlights"];
	        this.canAuto = source["canAuto"];
	    }
	}

}

