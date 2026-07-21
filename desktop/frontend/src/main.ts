import './styles/palette.css'
import './styles/theme.css'
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

initTheme()
initEditorFont()
initChatFont()
initSystemZoom()
initLocale()
initEditorTheme()
initTreeFont()

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
