import '@fontsource-variable/inter'
import '@fontsource-variable/noto-sans-thai'
// 4.3 MB across 101 unicode-range subsets, and that number is a disk number,
// not a memory one: the browser decodes only the subsets holding characters
// actually on screen. It is scoped to :lang(zh) in style.css rather than added
// to the global stack — see the note there.
import '@fontsource-variable/noto-sans-sc'
import './styles/palette.css'
import './styles/theme.css'
import './styles/type.css'
import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { initTheme } from './lib/theme.svelte'
import { initEditorFont } from './lib/editorFont.svelte'
import { initChatFont } from './lib/chatFont.svelte'
import { initSystemZoom } from './lib/systemFont.svelte'
import { initLocale } from './lib/i18n.svelte'
import { initEditorTheme } from './lib/editorTheme.svelte'
import { initTreeFont } from './lib/treeFont.svelte'
import { initUiFont } from './lib/uiFont.svelte'
import { initShell } from './lib/shell.svelte'
import { initRunnableLanguages } from './lib/runnable.svelte'

initTheme()
initShell()
initRunnableLanguages()
initEditorFont()
initChatFont()
initSystemZoom()
initLocale()
initEditorTheme()
initTreeFont()
initUiFont()

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
