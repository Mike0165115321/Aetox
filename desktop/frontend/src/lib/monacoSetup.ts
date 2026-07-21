// Wires Monaco's language workers through Vite's `?worker` import so they run
// on real Web Workers instead of falling back to the main thread (which Monaco
// does silently but slower, with a console warning). Side-effect-only module —
// import it once, before any `monaco.editor.create` call. ES module caching
// means importing it from every FileEditor instance still only runs this once.
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'
import JsonWorker from 'monaco-editor/language/json/json.worker?worker'
import CssWorker from 'monaco-editor/language/css/css.worker?worker'
import HtmlWorker from 'monaco-editor/language/html/html.worker?worker'
import TsWorker from 'monaco-editor/language/typescript/ts.worker?worker'

self.MonacoEnvironment = {
  getWorker(_workerId: string, label: string) {
    switch (label) {
      case 'json':
        return new JsonWorker()
      case 'css':
      case 'scss':
      case 'less':
        return new CssWorker()
      case 'html':
      case 'handlebars':
      case 'razor':
        return new HtmlWorker()
      case 'typescript':
      case 'javascript':
        return new TsWorker()
      default:
        return new EditorWorker()
    }
  },
}

const extToLanguage: Record<string, string> = {
  go: 'go', ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript',
  mjs: 'javascript', cjs: 'javascript', json: 'json', css: 'css', scss: 'scss',
  html: 'html', svelte: 'html', vue: 'html', md: 'markdown', py: 'python',
  yaml: 'yaml', yml: 'yaml', sh: 'shell', bash: 'shell', sql: 'sql', rs: 'rust',
  toml: 'ini', ini: 'ini', xml: 'xml', dockerfile: 'dockerfile',
}

/** Map a project-relative file path to a Monaco language id, defaulting to plaintext. */
export function detectLanguage(path: string): string {
  const name = path.split('/').pop() ?? path
  const ext = name.includes('.') ? name.split('.').pop()!.toLowerCase() : name.toLowerCase()
  return extToLanguage[ext] ?? 'plaintext'
}
