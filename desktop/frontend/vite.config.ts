import {defineConfig} from 'vitest/config'
import {svelte} from '@sveltejs/vite-plugin-svelte'
import {fileURLToPath} from 'node:url'

const mock = (file: string) =>
  fileURLToPath(new URL(`./src/test/mocks/${file}`, import.meta.url)).replace(/\\/g, '/')

// Under vitest only (`vite build` never sees this plugin): swap the Wails
// bindings and Monaco — native-bridge and web-worker code that can't exist
// under jsdom — for the doubles in src/test/mocks. A pre-resolver plugin
// rather than resolve.alias because alias entries don't fire on relative
// specifiers like '../../wailsjs/go/main/App'.
const MOCKS: [RegExp, string][] = [
  [/wailsjs\/go\/main\/App$/, mock('wailsApp.ts')],
  [/wailsjs\/go\/models$/, mock('wailsModels.ts')],
  [/wailsjs\/runtime\/runtime$/, mock('wailsRuntime.ts')],
  [/^monaco-editor$/, mock('monaco.ts')],
  [/monacoSetup$/, mock('monacoSetup.ts')],
]

const testMocks = () => ({
  name: 'test-mocks',
  enforce: 'pre' as const,
  resolveId(source: string) {
    for (const [re, file] of MOCKS) if (re.test(source)) return file
    return null
  },
})

// KaTeX ships each of its twenty fonts three times over — woff2, woff and ttf
// — and its stylesheet names all three, so Vite emits all sixty files and the
// binary carries every one of them: 1.2MB where 296KB is used. The engine this
// app runs on is WebView2, which is Chromium, which has read woff2 since 2014.
// The other two formats exist for browsers Aetox will never run in.
//
// Rewriting the stylesheet rather than deleting the files: Vite emits an asset
// because something references it, so the reference is what has to go.
const katexWoff2Only = () => ({
  name: 'katex-woff2-only',
  enforce: 'pre' as const,
  transform(code: string, id: string) {
    if (!id.includes('katex') || !id.endsWith('.css')) return null
    return code.replace(/,\s*url\([^)]+\.(?:woff|ttf)\)\s*format\("(?:woff|truetype)"\)/g, '')
  },
})

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [...(process.env.VITEST ? [testMocks()] : []), katexWoff2Only(), svelte()],
  // Svelte 5 ships client and server builds; without the browser condition
  // vitest picks the server one and mount() throws. Test runs only.
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
  test: {
    environment: 'jsdom',
    css: false,
    // What jsdom does not implement and the app relies on — see the file; it is
    // deliberately only environment gaps, never behaviour.
    setupFiles: ['./src/test/setup.ts'],
    // globals gives @testing-library/svelte its afterEach auto-cleanup hook —
    // without it every render stacks in one document and queries go ambiguous.
    globals: true,
  },
})
