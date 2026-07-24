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

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [...(process.env.VITEST ? [testMocks()] : []), svelte()],
  // Svelte 5 ships client and server builds; without the browser condition
  // vitest picks the server one and mount() throws. Test runs only.
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
  test: {
    environment: 'jsdom',
    css: false,
    // globals gives @testing-library/svelte its afterEach auto-cleanup hook —
    // without it every render stacks in one document and queries go ambiguous.
    globals: true,
  },
})
