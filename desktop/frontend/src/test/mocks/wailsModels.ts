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
