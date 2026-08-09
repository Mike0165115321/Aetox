// Test double for ../../wailsjs/go/models — only the runtime values matter
// (type-only imports vanish at compile time).
export namespace config {
  export class MCPServerConfig {
    name = ''
    command?: string[]
    cwd?: string
    environment?: Record<string, string>
    url?: string
    headers?: Record<string, string>
    timeout_ms?: number
    disabled?: boolean
    constructor(source: any = {}) {
      Object.assign(this, source)
    }
  }
}

export namespace main {}

// The agent editor builds a StarterSet to hand to SaveChairStarters, so this
// one is a runtime value rather than a type. createFrom is what the generated
// class offers and what the caller uses; the real one also revives nested
// classes, which nothing here needs — the object is on its way out to Go.
export namespace subagent {
  export class StarterSet {
    headline = ''
    cards: any[] = []
    static createFrom(source: any = {}) {
      return new StarterSet(source)
    }
    constructor(source: any = {}) {
      Object.assign(this, source)
    }
  }
}
