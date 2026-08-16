import '@fontsource-variable/inter'
import '@fontsource-variable/noto-sans-thai'
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
