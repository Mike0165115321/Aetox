// Test double for lib/monacoSetup — the real one imports Vite `?worker`
// modules that don't exist under jsdom.
export function detectLanguage(_path: string): string {
  return 'plaintext'
}
