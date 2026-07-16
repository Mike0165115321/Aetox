import './styles/palette.css'
import './styles/theme.css'
import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { initTheme } from './lib/theme.svelte'

initTheme()

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
