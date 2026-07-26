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
	    }
	}

}

export namespace main {
	
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
	
	    static createFrom(source: any = {}) {
	        return new ContextBreakdown(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usedTokens = source["usedTokens"];
	        this.maxTokens = source["maxTokens"];
	        this.slices = this.convertValues(source["slices"], ContextSlice);
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
	    disabled: boolean;
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
	        this.disabled = source["disabled"];
	        this.status = source["status"];
	        this.tools = source["tools"];
	        this.err = source["err"];
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
	export class SessionMessage {
	    role: string;
	    text: string;
	    time: string;
	    reasoning?: string;
	    thinkSecs?: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.text = source["text"];
	        this.time = source["time"];
	        this.reasoning = source["reasoning"];
	        this.thinkSecs = source["thinkSecs"];
	    }
	}
	export class SessionMeta {
	    id: string;
	    title: string;
	    updatedAt: string;
	    snippet?: string;
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
	        this.snippet = source["snippet"];
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
	export class SkillInfo {
	    name: string;
	    description: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.source = source["source"];
	    }
	}
	export class ToolCounts {
	    builtin: number;
	    skill: number;
	    mcp: number;
	
	    static createFrom(source: any = {}) {
	        return new ToolCounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.builtin = source["builtin"];
	        this.skill = source["skill"];
	        this.mcp = source["mcp"];
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
	    prompt: string;
	    path?: string;
	    builtin: boolean;
	    overrides?: boolean;
	
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
	        this.prompt = source["prompt"];
	        this.path = source["path"];
	        this.builtin = source["builtin"];
	        this.overrides = source["overrides"];
	    }
	}

}

