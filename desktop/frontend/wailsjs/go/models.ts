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
	export class ModelInfo {
	    provider: string;
	    modelName: string;
	    thinkLevel: string;
	    approvalMode: string;
	    contextUsed: number;
	    contextMax: number;
	
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
	    }
	}
	export class ProjectStatus {
	    name: string;
	    path: string;
	    branch: string;
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
	        this.governanceFile = source["governanceFile"];
	        this.governanceLoaded = source["governanceLoaded"];
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

}

